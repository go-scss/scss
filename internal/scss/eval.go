// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import (
	"fmt"
	"strings"
)

// Importer resolves an @use/@import/@forward URL to source text.
type Importer func(url string) (source string, resolvedURL string, ok bool)

type evaluator struct {
	env          *environment
	root         *cssRoot
	importer     Importer
	loadedURLs   []string
	warnings     []string
	extendEvents []extendEvent
	loadStack    []string
	loaded       map[string]*module
	// sharedLoaded deduplicates module loads across the WHOLE compilation
	// (keyed by resolved path), so a module reached through several paths — a
	// diamond dependency — is evaluated and emitted exactly once. It is shared
	// by reference with every sub-evaluator spawned during the compile.
	sharedLoaded  map[string]*module
	currentParent selectorList
	forwarded     []forwardedMod
	callDepth     int
	// incomingConfig is the evaluated `with (...)` configuration passed into
	// this module by the @use/@forward that loaded it. It flows onward through
	// this module's own @forward rules (a @forward propagates its importer's
	// configuration to the module it forwards, merged with its own `with`).
	incomingConfig map[string]Value
}

// maxCallDepth bounds mixin/content/function recursion. Dart Sass terminates on
// its native stack; this guard converts what would be an unrecoverable Go stack
// overflow into a normal Sass error for pathological (effectively infinite)
// recursion, well above any depth real stylesheets reach.
const maxCallDepth = 4000

func (e *evaluator) enter() {
	e.callDepth++
	if e.callDepth > maxCallDepth {
		e.fail("Recursion depth limit exceeded (%d).", maxCallDepth)
	}
}

func (e *evaluator) leave() { e.callDepth-- }

// extendEvent records, in document order, either a style rule to register with
// the extension store (rule != nil) or an @extend to apply (ext != nil). This
// mirrors Dart Sass's evaluator, which calls addSelector/addExtension as it
// visits, so boxes update transitively in the correct order.
type extendEvent struct {
	rule *cssStyleRule
	ext  *pendingExtend
}

// pendingExtend captures an @extend: the enclosing rule (its box supplies the
// extender), the parsed target selector list, whether it's !optional and the
// enclosing media-query context.
type pendingExtend struct {
	rule     *cssStyleRule
	targets  *selList
	optional bool
	media    []string
}

type frame struct {
	container     cssContainer
	rootContainer cssContainer
	mediaParent   cssContainer    // nearest non-media ancestor (where @media nodes go)
	mediaQueries  []mediaQuery    // effective enclosing media queries (nil at top)
	mediaSources  map[string]bool // source query strings for bubbling decisions
	mediaRuleNode cssContainer    // immediate enclosing @media node (for nesting)
	parentSel     selectorList
	hasParent     bool
	directDecls   bool
	block         *cssStyleRule
	group         *groupInfo
	atContainer   bool
	// declPrefix is the nested-property namespace ("name-") in effect; every
	// declaration emitted in this frame — including those produced by an
	// @include or @content inside a nested property block — is prefixed with it.
	declPrefix string
}

// groupInfo tracks source-statement grouping for blank-line insertion.
type groupInfo struct {
	pending          bool
	any              bool
	prevWasStyleRule bool
	curIsStyleRule   bool
}

func newEvaluator(importer Importer) *evaluator {
	return &evaluator{
		env:          newEnvironment(),
		root:         &cssRoot{},
		importer:     importer,
		loaded:       map[string]*module{},
		sharedLoaded: map[string]*module{},
	}
}

// controlSignal is used to unwind @return / @content from nested evaluation.
type returnSignal struct{ value Value }

func (e *evaluator) run(stmts []Stmt) {
	fr := &frame{
		container:     e.root,
		rootContainer: e.root,
		mediaParent:   e.root,
		atContainer:   true,
		group:         &groupInfo{},
	}
	e.evalBody(stmts, fr, true)
	e.applyExtends()
}

func (e *evaluator) evalBody(stmts []Stmt, fr *frame, containerBody bool) {
	for _, s := range stmts {
		if containerBody {
			fr.group.pending = true
			fr.group.curIsStyleRule = isStyleRuleStmt(s)
			fr.block = nil
		}
		e.evalStmt(s, fr)
	}
}

func isStyleRuleStmt(s Stmt) bool {
	_, ok := s.(*StyleRule)
	return ok
}

// consumeGroup marks the current group as started and reports whether a blank
// line must precede this node (previous source group was a style rule).
func (e *evaluator) consumeGroup(fr *frame) bool {
	gi := fr.group
	if !gi.pending {
		return false
	}
	gi.pending = false
	blank := gi.any && gi.prevWasStyleRule
	gi.prevWasStyleRule = gi.curIsStyleRule
	gi.any = true
	return blank
}

func (e *evaluator) evalStmt(s Stmt, fr *frame) {
	switch n := s.(type) {
	case *VarDecl:
		e.evalVarDecl(n)
	case *Declaration:
		e.evalDeclaration(n, fr)
	case *StyleRule:
		e.evalStyleRule(n, fr)
	case *MixinDef:
		e.env.mixins[normIdent(n.Name)] = &mixinEntry{def: n, env: e.env}
	case *FunctionDef:
		e.env.funcs[normIdent(n.Name)] = &funcEntry{def: n, env: e.env}
	case *Include:
		e.evalInclude(n, fr)
	case *If:
		e.evalIf(n, fr)
	case *Each:
		e.evalEach(n, fr)
	case *For:
		e.evalFor(n, fr)
	case *While:
		e.evalWhile(n, fr)
	case *Media:
		e.evalMedia(n, fr)
	case *Supports:
		e.evalSupports(n, fr)
	case *AtRoot:
		e.evalAtRoot(n, fr)
	case *Extend:
		e.evalExtend(n, fr)
	case *ContentStmt:
		e.evalContent(n, fr)
	case *Return:
		panic(returnSignal{value: e.evalExpr(n.Value)})
	case *Warn:
		e.warnings = append(e.warnings, e.stringify(e.evalExpr(n.Value)))
	case *Debug:
		e.warnings = append(e.warnings, e.stringify(e.evalExpr(n.Value)))
	case *ErrorStmt:
		e.fail("%s", e.stringify(e.evalExpr(n.Value)))
	case *LoudComment:
		e.evalLoudComment(n, fr)
	case *AtRule:
		e.evalGenericAtRule(n, fr)
	case *Use:
		e.evalUse(n, fr)
	case *Forward:
		e.evalForward(n, fr)
	case *Import:
		e.evalImport(n, fr)
	}
}

func (e *evaluator) fail(format string, args ...any) {
	panic(&SassError{Msg: fmt.Sprintf(format, args...)})
}

// --- declarations & variables ---

func (e *evaluator) evalVarDecl(n *VarDecl) {
	if n.Namespace != "" {
		e.assignNamespacedVar(n)
		return
	}
	if n.Default {
		if v, ok := e.env.getVar(n.Name); ok {
			if _, isNull := v.(*Null); !isNull {
				return
			}
		}
	}
	val := e.evalExpr(n.Value)
	e.env.setVar(n.Name, val, n.Global)
}

// assignNamespacedVar writes to a variable of another module (`ns.$var: value`).
// The module's own members share the very map exposed as mod.vars, so the write
// is visible to that module's functions and mixins. dart-sass forbids assigning
// to a variable the module does not already define.
func (e *evaluator) assignNamespacedVar(n *VarDecl) {
	mod, ok := e.env.modules[n.Namespace]
	if !ok {
		e.fail("There is no module with the namespace \"%s\".", n.Namespace)
	}
	name := normIdent(n.Name)
	if _, exists := mod.vars[name]; !exists {
		e.fail("Undefined variable.")
	}
	if n.Default {
		if v, ok := mod.vars[name]; ok {
			if _, isNull := v.(*Null); !isNull {
				return
			}
		}
	}
	mod.vars[name] = e.evalExpr(n.Value)
}

func (e *evaluator) evalDeclaration(n *Declaration, fr *frame) {
	name := fr.declPrefix + e.resolveInterp(n.Name)
	if n.Custom {
		raw := strings.TrimRight(e.resolveInterp(n.RawValue), " \t\n\r\f")
		e.addDecl(fr, &cssDeclaration{name: name, raw: raw, custom: true})
		return
	}
	if n.Value != nil {
		val := e.evalExpr(n.Value)
		if !isBlankValue(val) {
			e.addDecl(fr, &cssDeclaration{name: name, value: val})
		}
	}
	// Nested properties use a "name-" prefix that applies to every declaration
	// emitted in the block, including those produced by @include or @content.
	if len(n.Body) > 0 {
		child := *fr
		child.declPrefix = name + "-"
		for _, sub := range n.Body {
			e.evalStmt(sub, &child)
		}
	}
}

func isBlankValue(v Value) bool {
	if _, isNull := v.(*Null); isNull {
		return true
	}
	if str, isStr := v.(*SassString); isStr && !str.Quoted && str.Text == "" {
		return true
	}
	return false
}

func (e *evaluator) addDecl(fr *frame, d *cssDeclaration) {
	if fr.directDecls {
		fr.container.appendNode(d)
		return
	}
	if fr.block == nil {
		fr.block = &cssStyleRule{selector: fr.parentSel, original: fr.parentSel, mediaContext: mediaContextOf(fr)}
		fr.block.blankBefore = e.consumeGroup(fr)
		fr.container.appendNode(fr.block)
		if !fr.parentSel.isEmpty() {
			e.extendEvents = append(e.extendEvents, extendEvent{rule: fr.block})
		}
	}
	fr.block.appendNode(d)
}

// --- style rules ---

func (e *evaluator) evalStyleRule(n *StyleRule, fr *frame) {
	selStr := e.resolveInterp(n.Selector)
	child := parseSelectorList(selStr)
	var resolved selectorList
	if fr.hasParent {
		resolved = resolveNesting(child, fr.parentSel)
	} else {
		resolved = child
	}
	rule := &cssStyleRule{selector: resolved, original: resolved, mediaContext: mediaContextOf(fr)}
	rule.blankBefore = e.consumeGroup(fr)
	fr.container.appendNode(rule)
	e.extendEvents = append(e.extendEvents, extendEvent{rule: rule})
	child2 := &frame{
		container:     fr.container,
		rootContainer: fr.rootContainer,
		mediaParent:   fr.mediaParent,
		mediaQueries:  fr.mediaQueries,
		mediaSources:  fr.mediaSources,
		mediaRuleNode: fr.mediaRuleNode,
		parentSel:     resolved,
		hasParent:     true,
		block:         rule,
		group:         fr.group,
	}
	savedParent := e.currentParent
	e.currentParent = resolved
	e.evalRuleBody(n.Body, child2)
	e.currentParent = savedParent
}

// evalRuleBody evaluates a style-rule body, handling declarations (into the
// current block), nested declarations, and hoisted nested rules.
func (e *evaluator) evalRuleBody(stmts []Stmt, fr *frame) {
	e.env.pushScope()
	defer e.env.popScope()
	for _, s := range stmts {
		switch s.(type) {
		case *StyleRule, *Media, *Supports, *AtRoot, *AtRule:
			fr.block = nil
		}
		e.evalStmt(s, fr)
	}
}

// --- includes / mixins ---

func (e *evaluator) evalInclude(n *Include, fr *frame) {
	// The sass:meta module exposes two mixins as @include-only forms.
	if n.Namespace != "" && e.isMetaNamespace(n.Namespace) {
		switch normIdent(n.Name) {
		case "apply":
			e.evalMetaApply(n, fr)
			return
		case "load-css":
			e.evalMetaLoadCss(n, fr)
			return
		}
	}
	m := e.lookupMixin(n.Namespace, n.Name)
	if m == nil {
		e.fail("Undefined mixin.")
	}
	callEnv := e.env
	pos, named := e.evalArgs(n.Args)
	e.invokeMixin(m, pos, named, n.Content, callEnv, fr)
}

func (e *evaluator) evalContent(n *ContentStmt, fr *frame) {
	content := e.env.content
	contentEnv := e.env.contentEnv
	if content == nil {
		return
	}
	e.enter()
	defer e.leave()
	saved := e.env
	e.env = contentEnv
	e.env.pushScope()
	func() {
		defer func() {
			e.env.popScope()
			e.env = saved
		}()
		e.evalBody(content, fr, fr.atContainer)
	}()
}

func (e *evaluator) lookupMixin(ns, name string) *mixinEntry {
	name = normIdent(name)
	if ns != "" {
		if mod, ok := e.env.modules[ns]; ok {
			if m, ok := mod.mixins[name]; ok {
				return m
			}
		}
		return nil
	}
	if m, ok := e.env.mixins[name]; ok {
		return m
	}
	return nil
}

// --- control flow ---

func (e *evaluator) evalIf(n *If, fr *frame) {
	for _, cl := range n.Clauses {
		if e.evalExpr(cl.Cond).isTruthy() {
			e.evalBody(cl.Body, fr, fr.atContainer)
			return
		}
	}
	if n.HasElse {
		e.evalBody(n.Else, fr, fr.atContainer)
	}
}

func (e *evaluator) evalEach(n *Each, fr *frame) {
	list := e.evalExpr(n.List)
	items := iterationItems(list)
	for _, item := range items {
		if len(n.Vars) == 1 {
			e.env.defineVar(n.Vars[0], item)
		} else {
			parts := destructure(item, len(n.Vars))
			for i, v := range n.Vars {
				e.env.defineVar(v, parts[i])
			}
		}
		e.evalBody(n.Body, fr, fr.atContainer)
	}
}

func iterationItems(v Value) []Value {
	switch x := v.(type) {
	case *Map:
		out := make([]Value, len(x.Keys))
		for i := range x.Keys {
			out[i] = &List{Elements: []Value{x.Keys[i], x.Values[i]}, Sep: SepSpace}
		}
		return out
	case *List:
		return x.Elements
	case *Null:
		return nil
	default:
		return []Value{v}
	}
}

func destructure(v Value, n int) []Value {
	elems := v.asList()
	out := make([]Value, n)
	for i := 0; i < n; i++ {
		if i < len(elems) {
			out[i] = elems[i]
		} else {
			out[i] = sassNull
		}
	}
	return out
}

func (e *evaluator) evalFor(n *For, fr *frame) {
	from := e.evalNumber(n.From)
	to := e.evalNumber(n.To)
	// The loop variable inherits `from`'s units; `to` is coerced into those
	// units to obtain the numeric bound, matching dart-sass exactly (e.g.
	// `@for $i from 5mm through 1cm` iterates 5mm..10mm).
	start := int(from.Val)
	var endVal float64
	if len(from.Numer) == 1 && len(from.Denom) == 0 {
		endVal = to.coerceValueToUnit(from.Numer[0])
	} else {
		endVal = to.Val
	}
	end := int(endVal)
	mkVar := func(i int) *Number {
		return &Number{Val: float64(i), Numer: from.Numer, Denom: from.Denom}
	}
	if start <= end {
		last := end
		if !n.Through {
			last = end - 1
		}
		for i := start; i <= last; i++ {
			e.env.defineVar(n.Var, mkVar(i))
			e.evalBody(n.Body, fr, fr.atContainer)
		}
	} else {
		last := end
		if !n.Through {
			last = end + 1
		}
		for i := start; i >= last; i-- {
			e.env.defineVar(n.Var, mkVar(i))
			e.evalBody(n.Body, fr, fr.atContainer)
		}
	}
}

func (e *evaluator) evalWhile(n *While, fr *frame) {
	for e.evalExpr(n.Cond).isTruthy() {
		e.evalBody(n.Body, fr, fr.atContainer)
	}
}

// --- media / supports / at-root ---

func (e *evaluator) evalMedia(n *Media, fr *frame) {
	queries := parseMediaQueryList(e.resolveInterp(n.Query))

	var mergedQueries []mediaQuery
	representable := true
	if fr.mediaQueries != nil {
		mergedQueries, representable = mergeMediaQueryLists(fr.mediaQueries, queries)
		if representable && len(mergedQueries) == 0 {
			return // empty intersection: this rule matches nothing.
		}
	}

	var effective []mediaQuery
	var sources map[string]bool
	var parent cssContainer
	switch {
	case fr.mediaQueries == nil:
		effective, sources, parent = queries, mediaQuerySet(queries), fr.mediaParent
	case representable:
		// Bubble above the enclosing media rules, merging with them.
		effective = mergedQueries
		sources = map[string]bool{}
		for k := range fr.mediaSources {
			sources[k] = true
		}
		for _, q := range fr.mediaQueries {
			sources[q.String()] = true
		}
		for _, q := range queries {
			sources[q.String()] = true
		}
		parent = fr.mediaParent
	default:
		// Unrepresentable merge: keep nested inside the enclosing media rule.
		effective, sources = queries, mediaQuerySet(queries)
		parent = fr.mediaRuleNode
	}
	if parent == nil {
		parent = fr.rootContainer
	}

	at := &cssAtRule{name: "media", params: mediaQueriesString(effective), hasBody: true}
	at.blankBefore = e.consumeGroup(fr)
	parent.appendNode(at)
	child := &frame{
		container:     at,
		rootContainer: at,
		mediaParent:   parent,
		mediaQueries:  effective,
		mediaSources:  sources,
		mediaRuleNode: at,
		parentSel:     fr.parentSel,
		hasParent:     fr.hasParent,
		atContainer:   !fr.hasParent,
		group:         &groupInfo{},
	}
	e.evalBody(n.Body, child, !fr.hasParent)
}

func (e *evaluator) evalSupports(n *Supports, fr *frame) {
	cond := normalizeMediaQuery(e.resolveInterp(n.Condition))
	at := &cssAtRule{name: "supports", params: cond, hasBody: true}
	at.blankBefore = e.consumeGroup(fr)
	fr.mediaParent.appendNode(at)
	child := &frame{
		container:     at,
		rootContainer: at,
		mediaParent:   at,
		parentSel:     fr.parentSel,
		hasParent:     fr.hasParent,
		atContainer:   !fr.hasParent,
		group:         &groupInfo{},
	}
	e.evalBody(n.Body, child, !fr.hasParent)
}

func (e *evaluator) evalAtRoot(n *AtRoot, fr *frame) {
	child := &frame{
		container:     e.root,
		rootContainer: e.root,
		mediaParent:   e.root,
		atContainer:   true,
		group:         &groupInfo{},
	}
	e.evalBody(n.Body, child, true)
}

// --- generic at-rules ---

func (e *evaluator) evalGenericAtRule(n *AtRule, fr *frame) {
	params := ""
	if n.Value != nil {
		params = e.resolveInterp(n.Value)
	}
	at := &cssAtRule{name: n.Name, params: params, hasBody: !n.NoBody}
	at.blankBefore = e.consumeGroup(fr)
	fr.container.appendNode(at)
	if n.NoBody {
		return
	}
	direct := isDeclarationAtRule(n.Name)
	child := &frame{
		container:     at,
		rootContainer: at,
		mediaParent:   at,
		directDecls:   direct,
		atContainer:   true,
		group:         &groupInfo{},
	}
	e.evalBody(n.Body, child, true)
}

func isDeclarationAtRule(name string) bool {
	switch strings.ToLower(name) {
	case "font-face", "page", "font-feature-values", "counter-style", "viewport":
		return true
	}
	return false
}

func (e *evaluator) evalLoudComment(n *LoudComment, fr *frame) {
	text := e.resolveInterp(n.Text)
	c := &cssComment{text: text}
	c.blankBefore = e.consumeGroup(fr)
	if fr.block != nil {
		fr.block.appendNode(c)
		return
	}
	fr.container.appendNode(c)
}

// --- @extend ---

func (e *evaluator) evalExtend(n *Extend, fr *frame) {
	if fr.block == nil {
		e.fail("@extend may only be used within style rules.")
	}
	target := strings.TrimSpace(e.resolveInterp(n.Selector))
	list, err := parseSelectorListStrErr(target, false, false)
	if err != nil {
		panic(err)
	}
	e.extendEvents = append(e.extendEvents, extendEvent{ext: &pendingExtend{
		rule:     fr.block,
		targets:  list,
		optional: n.Optional,
		media:    mediaContextOf(fr),
	}})
}

// mediaContextOf returns the @extend media-query context for a frame: nil at the
// top level, otherwise a single-element key from the effective merged query.
func mediaContextOf(fr *frame) []string {
	if len(fr.mediaQueries) == 0 {
		return nil
	}
	out := make([]string, len(fr.mediaQueries))
	for i, q := range fr.mediaQueries {
		out[i] = q.String()
	}
	return out
}

// applyExtends replays recorded selector/extend events into an extension store
// (mirroring Dart Sass's evaluator visitation order), then writes each rule's
// extended selector back for serialization.
func (e *evaluator) applyExtends() {
	if len(e.extendEvents) == 0 {
		return
	}
	store := newExtensionStore(extendNormal)
	for _, ev := range e.extendEvents {
		if ev.rule != nil {
			ev.rule.box = store.addSelector(ev.rule.selector.list, ev.rule.mediaContext)
			continue
		}
		ext := ev.ext
		if ext.rule.box == nil {
			// The enclosing block never registered a selector (e.g. an @extend
			// nested inside a property-declaration block, which has no style-rule
			// selector of its own): there is nothing to extend from.
			continue
		}
		for _, complex := range ext.targets.components {
			compound := complex.singleCompound()
			if compound == nil {
				e.fail("complex selectors may not be extended.")
			}
			simple := compound.singleSimple()
			if simple == nil {
				e.fail("compound selectors may no longer be extended.")
			}
			store.addExtension(ext.rule.box.value, simple, ext.optional, ext.media)
		}
	}
	for _, ev := range e.extendEvents {
		if ev.rule != nil {
			ev.rule.selector = selectorList{list: ev.rule.box.value}
		}
	}
}

// --- interpolation ---

func (e *evaluator) resolveInterp(i *Interp) string {
	if i == nil {
		return ""
	}
	var sb strings.Builder
	for _, part := range i.Parts {
		switch p := part.(type) {
		case string:
			sb.WriteString(p)
		case *InterpExpr:
			sb.WriteString(e.stringifyInterp(e.evalExpr(p.Expr)))
		}
	}
	return sb.String()
}

// stringify renders a value as it appears in @debug/@warn/@error and interp.
func (e *evaluator) stringify(v Value) string {
	return serializeValue(v, false)
}

// stringifyInterp renders a value for #{} interpolation (unquoted strings).
func (e *evaluator) stringifyInterp(v Value) string {
	if s, ok := v.(*SassString); ok {
		return s.Text
	}
	return serializeValue(v, false)
}
