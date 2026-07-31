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
	// scope is this evaluator's @extend module scope. @use/@forward/meta.load-css
	// each spawn a sub-evaluator with its own scope; @import inlines into the
	// current one. Cross-module @extend propagates downstream→upstream through
	// scope.upstream at the single global finalize (applyAllExtends).
	scope *moduleScope
	// allScopes is the compilation-wide registry of every module scope, shared
	// by reference with every sub-evaluator so the entry evaluator can finalise
	// extends across the whole module graph in one pass.
	allScopes *[]*moduleScope
	// incomingConfig is the evaluated `with (...)` configuration passed into
	// this module by the @use/@forward that loaded it. It flows onward through
	// this module's own @forward rules (a @forward propagates its importer's
	// configuration to the module it forwards, merged with its own `with`).
	incomingConfig map[string]Value
	// inSupportsDecl is set while evaluating the name/value of a @supports
	// declaration condition. In that context calculations are left unsimplified
	// (dart-sass's _inSupportsDeclaration), so `(a: calc(1 + 2))` keeps its
	// `calc(1 + 2)` text rather than reducing to `3`.
	inSupportsDecl bool
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
	// inKeyframes marks a frame whose direct children are keyframe blocks (the
	// body of a @keyframes at-rule): their leading token is a keyframe selector
	// (from/to/percentage), not a CSS selector, and no `&`/@extend applies.
	inKeyframes bool
	// inKeyframeBlock marks the body of a keyframe block (e.g. `to { … }`), where
	// declarations and at-rules are allowed but nested style rules are an error.
	inKeyframeBlock bool
	// atRoot marks the direct children of a query-less @at-root: the parent
	// selector stays available so an explicit `&` resolves against it, but the
	// selector is NOT implicitly prefixed with the parent (dart nestWithin with
	// implicitParent=false). It applies to one nesting level only.
	atRoot bool
	// sealed marks that a child (typically a nested @media) has bubbled ABOVE
	// this frame's container node, so the container must be split: the next
	// node that stays in this frame gets a fresh copy of the container appended
	// after the bubbled node, preserving source order (dart's copy-on-bubble in
	// _addChild). See liveContainer.
	sealed bool
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
	e := &evaluator{
		env:          newEnvironment(),
		root:         &cssRoot{},
		importer:     importer,
		loaded:       map[string]*module{},
		sharedLoaded: map[string]*module{},
	}
	e.scope = &moduleScope{ev: e}
	scopes := []*moduleScope{e.scope}
	e.allScopes = &scopes
	return e
}

// moduleScope is one @extend module boundary. Its evaluator supplies the ordered
// selector/extend events; upstream lists the modules it depends on (@use/
// @forward/meta.load-css); incoming accumulates the extends that flow into it
// from downstream modules during the global finalize.
type moduleScope struct {
	ev               *evaluator
	upstream         []*moduleScope
	store            *extensionStore
	downstreamStores []*extensionStore
	visited          bool
}

// adoptScope wires a freshly spawned sub-evaluator into this compilation's
// shared scope registry so its rules take part in the global extend finalize.
func (e *evaluator) adoptScope(sub *evaluator) {
	sub.allScopes = e.allScopes
	*e.allScopes = append(*e.allScopes, sub.scope)
}

// dependsOn records that this evaluator's module @uses/@forwards/loads another
// module, so downstream extends written here reach that upstream module's rules.
func (e *evaluator) dependsOn(up *moduleScope) {
	if up == nil || up == e.scope {
		return
	}
	for _, u := range e.scope.upstream {
		if u == up {
			return
		}
	}
	e.scope.upstream = append(e.scope.upstream, up)
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
	e.applyAllExtends()
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
	// A variable binding consumes its value: a bare slash number is stored as
	// its quotient (dart-sass's VariableDeclaration.withoutSlash).
	val := numWithoutSlash(e.evalExpr(n.Value))
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
	mod.setVar(name, numWithoutSlash(e.evalExpr(n.Value)))
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
	// dart-sass omits a declaration whose value serializes to nothing, such as a
	// list made up entirely of nulls: `h: (null null null)` produces no output.
	if _, isList := v.(*List); isList && serializeValue(v, false) == "" {
		return true
	}
	return false
}

// liveContainer returns the container that new nodes staying in this frame must
// be appended to. When the frame is sealed — a nested rule bubbled above the
// container — a fresh copy of the container (an @media/@supports wrapper) is
// spun up after the bubbled node so the residual content keeps its source-order
// position, mirroring dart's parent split in _addChild.
func (e *evaluator) liveContainer(fr *frame) cssContainer {
	if !fr.sealed {
		return fr.container
	}
	fr.sealed = false
	// A frame is only ever sealed from evalMedia's bubble branch, which requires
	// an enclosing @media/@supports — so the container is always an at-rule node
	// that also serves as this frame's mediaRuleNode.
	at := fr.container.(*cssAtRule)
	fresh := &cssAtRule{name: at.name, params: at.params, hasBody: true}
	fresh.blankBefore = lastVisibleIsStyleRule(fr.mediaParent)
	fr.mediaParent.appendNode(fresh)
	fr.container = fresh
	fr.mediaRuleNode = fresh
	return fresh
}

// ensureBlock returns the open style-rule block for this frame, lazily creating
// it (as a copy of the enclosing parent selector) the first time content needs a
// home — e.g. a declaration or a loud comment inside a @media that has bubbled
// out of a style rule. This is dart's on-demand CssStyleRule under the bubbled
// at-rule.
func (e *evaluator) ensureBlock(fr *frame) *cssStyleRule {
	if fr.block == nil {
		fr.block = &cssStyleRule{selector: fr.parentSel, original: fr.parentSel, mediaContext: mediaContextOf(fr)}
		fr.block.blankBefore = e.consumeGroup(fr)
		e.liveContainer(fr).appendNode(fr.block)
		if !fr.parentSel.isEmpty() {
			e.extendEvents = append(e.extendEvents, extendEvent{rule: fr.block})
		}
	}
	return fr.block
}

func (e *evaluator) addDecl(fr *frame, d *cssDeclaration) {
	if fr.directDecls {
		fr.container.appendNode(d)
		return
	}
	e.ensureBlock(fr).appendNode(d)
}

// --- style rules ---

func (e *evaluator) evalStyleRule(n *StyleRule, fr *frame) {
	if fr.inKeyframes {
		e.evalKeyframeBlock(n, fr)
		return
	}
	if fr.inKeyframeBlock {
		e.fail("Style rules may not be used within keyframe blocks.")
	}
	selStr := e.resolveInterp(n.Selector)
	child := parseSelectorList(selStr)
	var resolved selectorList
	if fr.hasParent {
		resolved = resolveNestingImpl(child, fr.parentSel, !fr.atRoot)
	} else {
		resolved = child
	}
	rule := &cssStyleRule{selector: resolved, original: resolved, mediaContext: mediaContextOf(fr)}
	rule.blankBefore = e.consumeGroup(fr)
	container := e.liveContainer(fr)
	container.appendNode(rule)
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

// evalKeyframeBlock evaluates one keyframe block (e.g. `from, 15%, to { … }`)
// inside a @keyframes at-rule. The leading token is a keyframe selector, not a
// CSS selector: it is resolved (interpolation only), canonicalised and emitted
// verbatim. Declarations and at-rules land inside it; nested style rules are an
// error, and the block never bubbles out of the @keyframes container.
func (e *evaluator) evalKeyframeBlock(n *StyleRule, fr *frame) {
	sel := normalizeKeyframeSelector(strings.TrimSpace(e.resolveInterp(n.Selector)))
	rule := &cssStyleRule{raw: true, rawSel: sel}
	rule.blankBefore = e.consumeGroup(fr)
	e.liveContainer(fr).appendNode(rule)
	child := &frame{
		container:       rule,
		rootContainer:   rule,
		mediaParent:     rule,
		directDecls:     true,
		inKeyframeBlock: true,
		group:           &groupInfo{},
	}
	e.evalRuleBody(n.Body, child)
}

// normalizeKeyframeSelector canonicalises a keyframe selector list the way
// dart-sass serializes it: single-space after each comma, and the exponent
// marker of a scientific-notation percentage lower-cased (13E+1% -> 13e+1%).
func normalizeKeyframeSelector(s string) string {
	parts := strings.Split(s, ",")
	out := make([]string, len(parts))
	for i, p := range parts {
		p = strings.TrimSpace(p)
		if strings.ContainsAny(p, "0123456789") {
			p = strings.ReplaceAll(p, "E", "e")
		}
		out[i] = p
	}
	return strings.Join(out, ", ")
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
			e.env.pushControlScope()
			defer e.env.popScope()
			e.evalBody(cl.Body, fr, fr.atContainer)
			return
		}
	}
	if n.HasElse {
		e.env.pushControlScope()
		defer e.env.popScope()
		e.evalBody(n.Else, fr, fr.atContainer)
	}
}

func (e *evaluator) evalEach(n *Each, fr *frame) {
	list := e.evalExpr(n.List)
	items := iterationItems(list)
	e.env.pushControlScope()
	defer e.env.popScope()
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
	e.env.pushControlScope()
	defer e.env.popScope()
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
	e.env.pushControlScope()
	defer e.env.popScope()
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
	blank := e.consumeGroup(fr)
	// If this @media bubbled to a container strictly above the enclosing frame's
	// own container, that container must be split so any following siblings land
	// after this node in source order rather than folding back into the block
	// that was opened before the bubble (dart#777). The leading blank line is
	// then governed by the landing site's previous sibling, not the source group
	// the node was written in.
	if parent != fr.container {
		blank = lastVisibleIsStyleRule(parent)
		fr.sealed = true
	}
	at.blankBefore = blank
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
		atRoot:        fr.atRoot,
		// A @media nested in a keyframe block stays inside it and keeps its
		// declaration-direct, no-style-rules character.
		directDecls:     fr.inKeyframeBlock,
		inKeyframeBlock: fr.inKeyframeBlock,
		atContainer:     !fr.hasParent,
		group:           &groupInfo{},
	}
	e.evalBody(n.Body, child, !fr.hasParent)
}

func (e *evaluator) evalSupports(n *Supports, fr *frame) {
	cond := e.serializeSupportsCond(n.Cond)
	at := &cssAtRule{name: "supports", params: cond, hasBody: true}
	at.blankBefore = e.consumeGroup(fr)
	// @supports bubbles above style rules only (dart's `through` set is
	// CssStyleRule), so it lands in the nearest non-style-rule container —
	// staying INSIDE any enclosing @media rather than escaping above it.
	e.liveContainer(fr).appendNode(at)
	child := &frame{
		container:     at,
		rootContainer: at,
		mediaParent:   at,
		// The enclosing media context is preserved for query MERGING (a nested
		// @media still intersects with the outer queries) even though physical
		// bubbling stops at this @supports boundary, mirroring dart keeping
		// _mediaQueries live across a supports rule.
		mediaQueries:  fr.mediaQueries,
		mediaSources:  fr.mediaSources,
		mediaRuleNode: at,
		parentSel:     fr.parentSel,
		hasParent:     fr.hasParent,
		atRoot:        fr.atRoot,
		atContainer:   !fr.hasParent,
		group:         &groupInfo{},
	}
	e.evalBody(n.Body, child, !fr.hasParent)
}

func (e *evaluator) evalAtRoot(n *AtRoot, fr *frame) {
	// The default @at-root query is `(without: rule)`: it climbs out of the
	// enclosing style rules but STAYS within any @media/@supports frame. Only an
	// explicit query (handled below by escaping fully to the document root)
	// removes the at-rule frames as well. Preserving the media context here is
	// what keeps `@media screen { .foo { @at-root .bar { … } } }` wrapped.
	if n.Query == nil {
		child := &frame{
			container:     fr.container,
			rootContainer: fr.container,
			mediaParent:   fr.mediaParent,
			mediaQueries:  fr.mediaQueries,
			mediaSources:  fr.mediaSources,
			mediaRuleNode: fr.mediaRuleNode,
			// The parent selector stays available so an explicit `&` inside the
			// @at-root body resolves against it (dart), but atRoot suppresses the
			// implicit parent prefix so bare selectors escape to the root.
			parentSel:   fr.parentSel,
			hasParent:   fr.hasParent,
			atRoot:      true,
			atContainer: true,
			group:       &groupInfo{},
		}
		e.evalBody(n.Body, child, true)
		return
	}
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
	e.liveContainer(fr).appendNode(at)
	if n.NoBody {
		return
	}
	keyframes := isKeyframesAtRule(n.Name)
	// A @keyframes body holds keyframe blocks, but a stray declaration written
	// directly in it (dart tolerates this) is emitted verbatim rather than being
	// wrapped in an empty style rule, so treat the body as declaration-direct.
	direct := isDeclarationAtRule(n.Name) || keyframes
	child := &frame{
		container:     at,
		rootContainer: at,
		mediaParent:   at,
		directDecls:   direct,
		atContainer:   true,
		inKeyframes:   keyframes,
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

// isKeyframesAtRule reports whether an at-rule name is @keyframes or one of its
// vendor-prefixed spellings, whose body holds keyframe blocks (selectors are
// `from`/`to`/percentages) rather than ordinary style rules.
func isKeyframesAtRule(name string) bool {
	n := strings.ToLower(name)
	return n == "keyframes" || strings.HasSuffix(n, "-keyframes")
}

func (e *evaluator) evalLoudComment(n *LoudComment, fr *frame) {
	text := e.resolveInterp(n.Text)
	c := &cssComment{text: text}
	// In a selector context with no open block yet (a loud comment that is the
	// sole content of a @media bubbled out of a style rule), the comment is
	// wrapped in the enclosing parent rule, exactly as a declaration would be, so
	// it lands under `div { … }` inside the bubbled at-rule rather than loose.
	if fr.block == nil && fr.hasParent && !fr.directDecls && !fr.parentSel.isEmpty() {
		e.ensureBlock(fr).appendNode(c)
		return
	}
	c.blankBefore = e.consumeGroup(fr)
	if fr.block != nil {
		fr.block.appendNode(c)
		return
	}
	e.liveContainer(fr).appendNode(c)
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

// applyAllExtends finalises @extend across the whole module graph. Each module
// scope gets its own extension store carrying its own extends plus every extend
// that flows in from a module downstream of it (a module that @uses/@forwards/
// loads it), mirroring Dart Sass's per-module ExtensionStore and its downstream→
// upstream propagation. Scopes are finalised downstream-first so that, by the
// time an upstream module is processed, every downstream extend targeting its
// rules has been collected.
func (e *evaluator) applyAllExtends() {
	// Pass 1: build each module's own store (its own selectors extended by its
	// own @extends), exactly as a standalone stylesheet would.
	for _, m := range *e.allScopes {
		m.store = m.ev.buildOwnStore()
	}
	// Pass 2: finalise downstream-first. Merging a module's (already downstream-
	// enriched) store into each module it depends on carries extends transitively
	// upstream while keeping sibling modules isolated — a downstream extend only
	// re-extends the upstream module's own registered selectors, never selectors
	// introduced by a different downstream module.
	for _, m := range e.scopeFinalizeOrder() {
		if len(m.downstreamStores) != 0 {
			m.store.addExtensions(m.downstreamStores)
		}
		m.ev.writeBackSelectors()
		for _, up := range m.upstream {
			up.downstreamStores = append(up.downstreamStores, m.store)
		}
	}
}

// scopeFinalizeOrder returns every module scope in downstream-first order: a
// depth-first post-order over upstream edges (which yields upstream-first) is
// reversed. Sass forbids module loops, so the graph is a DAG.
func (e *evaluator) scopeFinalizeOrder() []*moduleScope {
	var post []*moduleScope
	var visit func(*moduleScope)
	visit = func(m *moduleScope) {
		if m.visited {
			return
		}
		m.visited = true
		for _, up := range m.upstream {
			visit(up)
		}
		post = append(post, m)
	}
	for _, m := range *e.allScopes {
		visit(m)
	}
	for i, j := 0, len(post)-1; i < j; i, j = i+1, j-1 {
		post[i], post[j] = post[j], post[i]
	}
	return post
}

// buildOwnStore replays this module's own selector/extend events into a fresh
// store (mirroring Dart Sass's evaluator visitation order), so each rule's box
// carries the selector extended by the module's OWN @extends. Cross-module
// extends are layered on later by addExtensions. Even an empty module gets a
// store so it can carry a downstream module's extends transitively upstream (a
// pass-through `midstream` that only @uses another module).
func (e *evaluator) buildOwnStore() *extensionStore {
	store := newExtensionStore(extendNormal)
	for _, ev := range e.extendEvents {
		if ev.rule != nil {
			ev.rule.box = store.addSelector(ev.rule.selector.list, ev.rule.mediaContext)
			continue
		}
		e.applyExtendToStore(store, ev.ext)
	}
	return store
}

// writeBackSelectors copies each of this module's rule boxes' final values into
// the rule nodes for serialization. Called after cross-module extends land.
func (e *evaluator) writeBackSelectors() {
	for _, ev := range e.extendEvents {
		if ev.rule != nil && ev.rule.box != nil {
			ev.rule.selector = selectorList{list: ev.rule.box.value}
		}
	}
}

// applyExtendToStore adds a single @extend's targets to store, using the
// enclosing rule's (possibly already-extended) selector as the extender.
func (e *evaluator) applyExtendToStore(store *extensionStore, ext *pendingExtend) {
	if ext.rule.box == nil {
		// The enclosing block never registered a selector (e.g. an @extend nested
		// inside a property-declaration block, which has no style-rule selector of
		// its own): there is nothing to extend from.
		return
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

// stringifyInterp renders a value for #{} interpolation. Strings lose their
// quotes, and dart-sass propagates that unquoting recursively into list and map
// elements, so `#{"a" "b"}` yields `a b`.
func (e *evaluator) stringifyInterp(v Value) string {
	return serializeValueQ(v, false, false)
}
