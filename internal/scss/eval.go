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
	env           *environment
	root          *cssRoot
	importer      Importer
	loadedURLs    []string
	warnings      []string
	extends       []extendRule
	loadStack     []string
	loaded        map[string]*module
	currentParent selectorList
	forwarded     []forwardedMod
}

type extendRule struct {
	target    string // compound selector being extended (e.g. ".foo" or "%bar")
	extenders selectorList
	optional  bool
}

type frame struct {
	container     cssContainer
	rootContainer cssContainer
	mediaParent   cssContainer // nearest non-media ancestor (where @media nodes go)
	mediaQuery    string       // effective enclosing media query (for merging)
	parentSel     selectorList
	hasParent     bool
	directDecls   bool
	block         *cssStyleRule
	group         *groupInfo
	atContainer   bool
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
		env:      newEnvironment(),
		root:     &cssRoot{},
		importer: importer,
		loaded:   map[string]*module{},
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
	e.prunePlaceholders(e.root)
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
		e.evalDeclaration(n, fr, "")
	case *StyleRule:
		e.evalStyleRule(n, fr)
	case *MixinDef:
		e.env.mixins[n.Name] = &mixinEntry{def: n, env: e.env}
	case *FunctionDef:
		e.env.funcs[n.Name] = &funcEntry{def: n, env: e.env}
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
		e.evalUse(n)
	case *Forward:
		e.evalForward(n)
	case *Import:
		e.evalImport(n, fr)
	}
}

func (e *evaluator) fail(format string, args ...any) {
	panic(&SassError{Msg: fmt.Sprintf(format, args...)})
}

// --- declarations & variables ---

func (e *evaluator) evalVarDecl(n *VarDecl) {
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

func (e *evaluator) evalDeclaration(n *Declaration, fr *frame, prefix string) {
	name := prefix + e.resolveInterp(n.Name)
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
	// nested properties use a "name-" prefix
	for _, sub := range n.Body {
		if d, ok := sub.(*Declaration); ok {
			e.evalDeclaration(d, fr, name+"-")
		} else {
			e.evalStmt(sub, fr)
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
		fr.block = &cssStyleRule{selector: fr.parentSel, original: fr.parentSel}
		fr.block.blankBefore = e.consumeGroup(fr)
		fr.container.appendNode(fr.block)
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
	rule := &cssStyleRule{selector: resolved, original: resolved}
	rule.blankBefore = e.consumeGroup(fr)
	fr.container.appendNode(rule)
	child2 := &frame{
		container:     fr.container,
		rootContainer: fr.rootContainer,
		mediaParent:   fr.mediaParent,
		mediaQuery:    fr.mediaQuery,
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
	m := e.lookupMixin(n.Namespace, n.Name)
	if m == nil {
		e.fail("Undefined mixin.")
	}
	callEnv := e.env
	defEnv := m.env
	saved := e.env
	e.env = defEnv
	e.env.pushScope()
	e.bindArgs(m.def.Params, n.Args, callEnv)
	savedContent := e.env.content
	savedContentEnv := e.env.contentEnv
	e.env.content = n.Content
	e.env.contentEnv = callEnv
	func() {
		defer func() {
			e.env.content = savedContent
			e.env.contentEnv = savedContentEnv
			e.env.popScope()
			e.env = saved
		}()
		e.evalBody(m.def.Body, fr, fr.atContainer)
	}()
}

func (e *evaluator) evalContent(n *ContentStmt, fr *frame) {
	content := e.env.content
	contentEnv := e.env.contentEnv
	if content == nil {
		return
	}
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
	start := int(from.Val)
	end := int(to.Val)
	if start <= end {
		last := end
		if !n.Through {
			last = end - 1
		}
		for i := start; i <= last; i++ {
			e.env.defineVar(n.Var, newNumber(float64(i)))
			e.evalBody(n.Body, fr, fr.atContainer)
		}
	} else {
		last := end
		if !n.Through {
			last = end + 1
		}
		for i := start; i >= last; i-- {
			e.env.defineVar(n.Var, newNumber(float64(i)))
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
	query := normalizeMediaQuery(e.resolveInterp(n.Query))
	merged := query
	if fr.mediaQuery != "" {
		merged = query + " and " + fr.mediaQuery
	}
	parent := fr.mediaParent
	if parent == nil {
		parent = fr.rootContainer
	}
	at := &cssAtRule{name: "media", params: merged, hasBody: true}
	at.blankBefore = e.consumeGroup(fr)
	parent.appendNode(at)
	child := &frame{
		container:     at,
		rootContainer: at,
		mediaParent:   parent,
		mediaQuery:    merged,
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
	target := strings.TrimSpace(e.resolveInterp(n.Selector))
	e.extends = append(e.extends, extendRule{
		target:    target,
		extenders: fr.parentSel,
		optional:  n.Optional,
	})
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
