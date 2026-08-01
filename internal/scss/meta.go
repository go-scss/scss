// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import (
	"sort"
	"strings"
)

func init() {
	metaFns["calc-name"] = fnCalcName
	metaFns["get-function"] = fnGetFunction
	metaFns["get-mixin"] = fnGetMixin
	metaFns["call"] = fnCall
	metaFns["module-functions"] = fnModuleFunctions
	metaFns["module-variables"] = fnModuleVariables
	metaFns["module-mixins"] = fnModuleMixins
	metaFns["accepts-content"] = fnAcceptsContent
	// These reflection built-ins consult the module tables (moduleHas*), which
	// transitively reference metaFns; registering them here rather than in the
	// map literal breaks the static initialisation cycle.
	metaFns["global-variable-exists"] = fnGlobalVariableExists
	metaFns["function-exists"] = fnFunctionExists
	metaFns["mixin-exists"] = fnMixinExists
}

func fnGlobalVariableExists(ci *callInfo) Value {
	if mv, ok := ci.get(1, "module"); ok {
		return boolean(ci.e.moduleHasVar(ci.e.asString(mv).Text, ci.str(0, "name").Text))
	}
	name := normIdent(ci.str(0, "name").Text)
	if _, ok := ci.e.env.scopes[0][name]; ok {
		return sassTrue
	}
	// A `@use as *` module's variable is a global variable in this scope.
	for _, mod := range ci.e.env.globalModules {
		if _, ok := mod.getVar(name); ok {
			return sassTrue
		}
	}
	return sassFalse
}

func fnFunctionExists(ci *callInfo) Value {
	name := normIdent(ci.str(0, "name").Text)
	if mv, ok := ci.get(1, "module"); ok {
		return boolean(ci.e.moduleHasFunc(ci.e.asString(mv).Text, name))
	}
	if _, ok := ci.e.env.getFunc(name); ok {
		return sassTrue
	}
	if _, ok := ci.e.env.globalModuleFunc(name); ok {
		return sassTrue
	}
	_, ok := globalFns[normIdent(strings.ToLower(name))]
	return boolean(ok)
}

func fnMixinExists(ci *callInfo) Value {
	if mv, ok := ci.get(1, "module"); ok {
		return boolean(ci.e.moduleHasMixin(ci.e.asString(mv).Text, ci.str(0, "name").Text))
	}
	name := normIdent(ci.str(0, "name").Text)
	if _, ok := ci.e.env.getMixin(name); ok {
		return sassTrue
	}
	_, ok := ci.e.env.globalModuleMixin(name)
	return boolean(ok)
}

// This file implements the first-class function and mixin value types and the
// reflective sass:meta built-ins that operate on them: get-function, get-mixin,
// call, module-functions/-variables/-mixins, accepts-content, calc-name and the
// @include-only forms apply and load-css.

// SassFunction is a first-class reference to a function (get-function()).
type SassFunction struct {
	name    string      // for inspect(): get-function("name")
	user    *funcEntry  // non-nil for a user-defined function
	builtin builtinFunc // non-nil for a built-in
	key     string      // stable identity for built-in equality
	css     bool        // a plain-CSS function reference ($css: true)
}

func (*SassFunction) isTruthy() bool    { return true }
func (*SassFunction) sep() Separator    { return SepUndecided }
func (f *SassFunction) asList() []Value { return []Value{f} }
func (f *SassFunction) equals(o Value) bool {
	of, ok := o.(*SassFunction)
	if !ok {
		return false
	}
	if f.user != nil || of.user != nil {
		return f.user == of.user
	}
	return f.css == of.css && f.key == of.key
}

// SassMixin is a first-class reference to a mixin (get-mixin()).
type SassMixin struct {
	name    string
	user    *mixinEntry
	builtin string // built-in mixin identity (e.g. "meta.apply")
}

func (*SassMixin) isTruthy() bool    { return true }
func (*SassMixin) sep() Separator    { return SepUndecided }
func (m *SassMixin) asList() []Value { return []Value{m} }
func (m *SassMixin) equals(o Value) bool {
	om, ok := o.(*SassMixin)
	if !ok {
		return false
	}
	if m.user != nil || om.user != nil {
		return m.user == om.user
	}
	return m.builtin == om.builtin
}

// --- resolution helpers ---

// resolveModuleFuncs returns the function table for a module referenced by its
// namespace string, and whether the namespace names a (user or built-in) module.
func (e *evaluator) moduleByName(name string) (*module, bool) {
	if m, ok := e.env.modules[name]; ok {
		return m, true
	}
	return nil, false
}

// moduleHasVar reports whether the module named by namespace exports a global
// variable with the given (dash-insensitive) name.
func (e *evaluator) moduleHasVar(namespace, name string) bool {
	if m, ok := e.moduleByName(namespace); ok {
		_, ok := m.getVar(normIdent(name))
		return ok
	}
	if _, real, ok := e.builtinModule(namespace); ok {
		_, ok := builtinModuleVar(real, name)
		return ok
	}
	return false
}

// moduleHasFunc reports whether the module named by namespace exports a function
// with the given name.
func (e *evaluator) moduleHasFunc(namespace, name string) bool {
	if m, ok := e.moduleByName(namespace); ok {
		_, ok := m.getFunc(normIdent(name))
		return ok
	}
	if reg, _, ok := e.builtinModule(namespace); ok {
		_, ok := reg[normIdent(name)]
		return ok
	}
	return false
}

// moduleHasMixin reports whether the module named by namespace exports a mixin
// with the given name.
func (e *evaluator) moduleHasMixin(namespace, name string) bool {
	if m, ok := e.moduleByName(namespace); ok {
		_, ok := m.getMixin(normIdent(name))
		return ok
	}
	if _, real, ok := e.builtinModule(namespace); ok {
		n := normIdent(name)
		return real == "meta" && (n == "apply" || n == "load-css")
	}
	return false
}

func (e *evaluator) getFunctionValue(name string, css bool, moduleName string) Value {
	if css {
		return &SassFunction{name: name, css: true, key: "css:" + normIdent(name)}
	}
	if moduleName != "" {
		if m, ok := e.moduleByName(moduleName); ok {
			if fn, ok := m.getFunc(normIdent(name)); ok {
				return &SassFunction{name: name, user: fn}
			}
		}
		if real, ok := e.env.builtinAliases[moduleName]; ok {
			if bf, ok := moduleRegistry(real)[normIdent(strings.ToLower(name))]; ok {
				return &SassFunction{name: name, builtin: bf, key: real + "." + normIdent(strings.ToLower(name))}
			}
		}
		e.fail("There is no module with the namespace \"%s\".", moduleName)
	}
	if fn, ok := e.env.getFunc(name); ok {
		return &SassFunction{name: name, user: fn}
	}
	if fn, ok := e.env.globalModuleFunc(normIdent(name)); ok {
		return &SassFunction{name: name, user: fn}
	}
	if bf, ok := globalFns[normIdent(strings.ToLower(name))]; ok {
		return &SassFunction{name: name, builtin: bf, key: "global:" + normIdent(strings.ToLower(name))}
	}
	e.fail("Function not found: %s", name)
	return nil
}

func (e *evaluator) getMixinValue(name, moduleName string) Value {
	if moduleName != "" {
		if m, ok := e.moduleByName(moduleName); ok {
			if mx, ok := m.getMixin(normIdent(name)); ok {
				return &SassMixin{name: name, user: mx}
			}
		}
		if real, ok := e.env.builtinAliases[moduleName]; ok && real == "meta" {
			if normIdent(name) == "apply" || normIdent(name) == "load-css" {
				return &SassMixin{name: name, builtin: "meta." + normIdent(name)}
			}
		}
		e.fail("There is no module with the namespace \"%s\".", moduleName)
	}
	if mx, ok := e.env.getMixin(name); ok {
		return &SassMixin{name: name, user: mx}
	}
	if mx, ok := e.env.globalModuleMixin(normIdent(name)); ok {
		return &SassMixin{name: name, user: mx}
	}
	e.fail("Mixin not found: %s", name)
	return nil
}

// --- meta functions ---

func fnGetFunction(ci *callInfo) Value {
	name := ci.str(0, "name").Text
	css := false
	if v, ok := ci.get(1, "css"); ok {
		css = v.isTruthy()
	}
	moduleName := ""
	if v, ok := ci.get(2, "module"); ok {
		if _, isNull := v.(*Null); !isNull {
			moduleName = ci.e.asString(v).Text
		}
	}
	return ci.e.getFunctionValue(name, css, moduleName)
}

func fnGetMixin(ci *callInfo) Value {
	name := ci.str(0, "name").Text
	moduleName := ""
	if v, ok := ci.get(1, "module"); ok {
		if _, isNull := v.(*Null); !isNull {
			moduleName = ci.e.asString(v).Text
		}
	}
	return ci.e.getMixinValue(name, moduleName)
}

func fnCall(ci *callInfo) Value {
	fnv := ci.require(0, "function")
	var pos []Value
	if len(ci.positional) > 1 {
		pos = ci.positional[1:]
	}
	named := map[string]Value{}
	for k, v := range ci.named {
		if k != "function" {
			named[k] = v
		}
	}
	return ci.e.callFunctionValue(fnv, pos, named)
}

func (e *evaluator) callFunctionValue(fnv Value, pos []Value, named map[string]Value) Value {
	switch f := fnv.(type) {
	case *SassFunction:
		if f.css {
			return &SassString{Text: f.name + "(" + joinCSSArgs(pos, named) + ")", Quoted: false}
		}
		if f.user != nil {
			// call() resolves its arguments before dispatch, so the original spread
			// separator is unavailable; a rest parameter defaults to comma.
			return e.callUserResolved(f.user, pos, named, SepComma)
		}
		return f.builtin(&callInfo{positional: pos, named: named, e: e, fn: f.name})
	case *SassString:
		// Deprecated call("name", ...): resolve by name.
		resolved := e.getFunctionValue(f.Text, false, "")
		return e.callFunctionValue(resolved, pos, named)
	default:
		e.fail("$function: %s is not a function reference.", serializeValue(fnv, false))
		return nil
	}
}

// joinCSSArgs renders resolved arguments for a plain-CSS function reference.
func joinCSSArgs(pos []Value, named map[string]Value) string {
	parts := make([]string, 0, len(pos)+len(named))
	for _, v := range pos {
		parts = append(parts, serializeValue(v, false))
	}
	for k, v := range named {
		parts = append(parts, "$"+k+": "+serializeValue(v, false))
	}
	return strings.Join(parts, ", ")
}

func fnModuleFunctions(ci *callInfo) Value {
	ns := ci.str(0, "module").Text
	if reg, real, ok := ci.e.builtinModule(ns); ok {
		out := &Map{}
		for _, name := range sortedKeys(reg) {
			out.set(&SassString{Text: name, Quoted: true},
				&SassFunction{name: name, builtin: reg[name], key: real + "." + name})
		}
		return out
	}
	m := ci.e.requireModule(ns)
	funcs := m.allFuncs()
	out := &Map{}
	for _, name := range sortedKeys(funcs) {
		out.set(&SassString{Text: name, Quoted: true}, &SassFunction{name: name, user: funcs[name]})
	}
	return out
}

func fnModuleMixins(ci *callInfo) Value {
	ns := ci.str(0, "module").Text
	if _, real, ok := ci.e.builtinModule(ns); ok {
		out := &Map{}
		if real == "meta" {
			for _, name := range []string{"apply", "load-css"} {
				out.set(&SassString{Text: name, Quoted: true}, &SassMixin{name: name, builtin: "meta." + name})
			}
		}
		return out
	}
	m := ci.e.requireModule(ns)
	mixins := m.allMixins()
	out := &Map{}
	for _, name := range sortedKeys(mixins) {
		out.set(&SassString{Text: name, Quoted: true}, &SassMixin{name: name, user: mixins[name]})
	}
	return out
}

func fnModuleVariables(ci *callInfo) Value {
	ns := ci.str(0, "module").Text
	if _, real, ok := ci.e.builtinModule(ns); ok {
		out := &Map{}
		if real == "math" {
			for _, name := range sortedKeys(mathModuleVars) {
				out.set(&SassString{Text: name, Quoted: true}, mathModuleVars[name])
			}
		}
		return out
	}
	m := ci.e.requireModule(ns)
	vars := m.allVars()
	out := &Map{}
	for _, name := range sortedKeys(vars) {
		out.set(&SassString{Text: name, Quoted: true}, vars[name])
	}
	return out
}

// builtinModule resolves a namespace to a built-in module's function registry
// and canonical name.
func (e *evaluator) builtinModule(ns string) (map[string]builtinFunc, string, bool) {
	real, ok := e.env.builtinAliases[ns]
	if !ok {
		return nil, "", false
	}
	// A built-in alias always names one of the known modules, so its registry is
	// never nil.
	return moduleRegistry(real), real, true
}

func (e *evaluator) requireModule(name string) *module {
	if m, ok := e.moduleByName(name); ok {
		return m
	}
	e.fail("There is no module with the namespace \"%s\".", name)
	return nil
}

func fnAcceptsContent(ci *callInfo) Value {
	v := ci.require(0, "mixin")
	m, ok := v.(*SassMixin)
	if !ok {
		ci.e.fail("$mixin: %s is not a mixin reference.", serializeValue(v, false))
	}
	if m.builtin != "" {
		return boolean(m.builtin == "meta.apply")
	}
	return boolean(bodyHasContent(m.user.def.Body))
}

func fnCalcName(ci *callInfo) Value {
	v := ci.require(0, "calc")
	c, ok := v.(*SassCalculation)
	if !ok {
		ci.e.fail("$calc: %s is not a calculation.", serializeValue(v, false))
	}
	return &SassString{Text: c.Name, Quoted: true}
}

// bodyHasContent reports whether a mixin body contains an @content anywhere.
func bodyHasContent(body []Stmt) bool {
	for _, s := range body {
		if _, ok := s.(*ContentStmt); ok {
			return true
		}
		for _, sub := range stmtBodies(s) {
			if bodyHasContent(sub) {
				return true
			}
		}
	}
	return false
}

// stmtBodies returns the nested statement bodies of a container statement.
func stmtBodies(s Stmt) [][]Stmt {
	switch n := s.(type) {
	case *StyleRule:
		return [][]Stmt{n.Body}
	case *Media:
		return [][]Stmt{n.Body}
	case *Supports:
		return [][]Stmt{n.Body}
	case *AtRoot:
		return [][]Stmt{n.Body}
	case *AtRule:
		return [][]Stmt{n.Body}
	case *Each:
		return [][]Stmt{n.Body}
	case *For:
		return [][]Stmt{n.Body}
	case *While:
		return [][]Stmt{n.Body}
	case *Include:
		return [][]Stmt{n.Content}
	case *If:
		out := make([][]Stmt, 0, len(n.Clauses)+1)
		for _, c := range n.Clauses {
			out = append(out, c.Body)
		}
		if n.HasElse {
			out = append(out, n.Else)
		}
		return out
	}
	return nil
}

// --- @include forms: apply and load-css ---

func (e *evaluator) isMetaNamespace(ns string) bool {
	return e.env.builtinAliases[ns] == "meta"
}

// evalMetaApply implements `@include meta.apply($mixin, args...)`.
func (e *evaluator) evalMetaApply(n *Include, fr *frame) {
	pos, named, restSep := e.evalArgs(n.Args)
	var mixinVal Value
	if v, ok := named["mixin"]; ok {
		mixinVal = v
		delete(named, "mixin")
	} else if len(pos) > 0 {
		mixinVal = pos[0]
		pos = pos[1:]
	} else {
		e.fail("Missing argument $mixin.")
	}
	m, ok := mixinVal.(*SassMixin)
	if !ok {
		e.fail("$mixin: %s is not a mixin reference.", serializeValue(mixinVal, false))
	}
	if m.user == nil {
		e.fail("The mixin %s can't be applied with meta.apply().", m.name)
	}
	e.invokeMixin(m.user, pos, named, restSep, n.Content, n.ContentParams, e.env, fr)
}

// evalMetaLoadCss implements `@include meta.load-css($url, $with:)`.
func (e *evaluator) evalMetaLoadCss(n *Include, fr *frame) {
	pos, named, _ := e.evalArgs(n.Args)
	var urlVal Value
	if v, ok := named["url"]; ok {
		urlVal = v
	} else if len(pos) > 0 {
		urlVal = pos[0]
	} else {
		e.fail("Missing argument $url.")
	}
	url := e.asString(urlVal).Text
	var config []ConfigVar
	if v, ok := named["with"]; ok {
		if mp, ok := v.(*Map); ok {
			for i := range mp.Keys {
				if ks, ok := mp.Keys[i].(*SassString); ok {
					config = append(config, ConfigVar{Name: ks.Text, Value: &preEvaluated{mp.Values[i]}})
				}
			}
		}
	}
	e.loadCSSInto(url, config, fr)
}

// invokeMixin runs a user mixin with pre-resolved arguments, an optional
// @content block and the environment in which that content was written.
func (e *evaluator) invokeMixin(m *mixinEntry, pos []Value, named map[string]Value, restSep Separator, content []Stmt, contentParams *ParamList, contentEnv *environment, fr *frame) {
	e.enter()
	defer e.leave()
	saved := e.env
	// Run the mixin in its lexical closure (the scopes visible where it was
	// defined) rather than mutating the shared definition environment. This keeps
	// each invocation's local variables private — recursion and nested includes no
	// longer see or clobber an outer invocation's scope.
	e.env = m.env.closureAt(m.defDepth)
	e.env.pushScope()
	e.bindResolved(m.def.Params, pos, named, restSep)
	savedContent := e.env.content
	savedContentEnv := e.env.contentEnv
	savedContentArgs := e.env.contentArgs
	e.env.content = content
	e.env.contentEnv = contentEnv
	e.env.contentArgs = contentParams
	defer func() {
		e.env.content = savedContent
		e.env.contentEnv = savedContentEnv
		e.env.contentArgs = savedContentArgs
		e.env.popScope()
		e.env = saved
	}()
	e.evalBody(m.def.Body, fr, fr.atContainer)
}

// preEvaluated wraps an already-computed Value as an Expr (for splicing resolved
// configuration into @use of a loaded module).
type preEvaluated struct{ v Value }

func (*preEvaluated) expr() {}

// loadCSSInto compiles a module and splices its resolved CSS at the include
// point, matching @include meta.load-css().
func (e *evaluator) loadCSSInto(url string, config []ConfigVar, fr *frame) {
	// A built-in module (sass:math, sass:color, …) contributes no CSS, so
	// meta.load-css of one emits nothing; a configuration would be an error.
	if strings.HasPrefix(url, "sass:") {
		if len(config) > 0 {
			e.fail("Built-in modules can't be configured.")
		}
		if !builtinModuleNames[strings.TrimPrefix(url, "sass:")] {
			e.fail("Can't find stylesheet to import: %s", url)
		}
		return
	}
	src, resolved, ok := e.resolve(url)
	if !ok {
		e.fail("Can't find stylesheet to import: %s", url)
	}
	// A .css file is parsed as plain CSS, preserving its native nesting, and is
	// re-nested under any enclosing selector rather than re-parsed as Sass.
	if strings.HasSuffix(resolved, ".css") {
		nodes, err := parsePlainCSS(src)
		if err != nil {
			panic(err)
		}
		e.loadedURLs = append(e.loadedURLs, resolved)
		e.placeLoadedCSS(nodes, fr)
		return
	}
	for _, s := range e.loadStack {
		if s == resolved {
			e.fail("Module loop: %s", resolved)
		}
	}
	stmts, err := parseStylesheet(src)
	if err != nil {
		panic(err)
	}
	sub := newEvaluator(e.importer)
	sub.loadStack = append(append([]string(nil), e.loadStack...), resolved)
	sub.loadedURLs = e.loadedURLs
	sub.sharedLoaded = e.sharedLoaded
	e.adoptScope(sub)
	e.dependsOn(sub.scope)
	// Record the $with configuration as the incoming config so it is applied at
	// each top-level `!default` declaration (and propagated through the module's
	// own @forward rules), exactly as @use ... with (...) does (loadModule).
	cfg := map[string]Value{}
	for _, c := range config {
		cfg[normIdent(c.Name)] = numWithoutSlash(e.evalExpr(c.Value))
	}
	sub.incomingConfig = cfg
	sub.runModule(stmts)
	e.warnings = append(e.warnings, sub.warnings...)
	e.loadedURLs = append(e.loadedURLs, resolved)
	e.placeLoadedCSS(sub.root.children(), fr)
}

// placeLoadedCSS splices a loaded module's already-resolved top-level CSS into
// the current output. At the top level the nodes are appended verbatim, exactly
// as before. Inside an enclosing style rule, dart-sass re-nests the loaded CSS
// under that rule's selector — the CSS is resolved in a clean context, so its
// own `&` refers to its own parents, and only its outermost rules pick up the
// including selector. This mirrors dart's context-dependent placement of
// meta.load-css output (async_evaluate.dart `_visitStyleRule` re-parenting).
func (e *evaluator) placeLoadedCSS(nodes []cssNode, fr *frame) {
	if !fr.hasParent {
		e.emitModuleCSS(nodes, fr)
		return
	}
	for _, n := range nodes {
		e.renestLoadedNode(n, fr)
	}
}

// renestLoadedNode re-nests one top-level node of loaded CSS under the enclosing
// selector fr.parentSel. A Sass-resolved style rule (no `&` — it was flattened
// in its own context) gains the parent as a leading descendant selector. A
// plain-CSS rule keeps its verbatim selector: if that selector references the
// parent (`&`, native CSS nesting) the whole rule is wrapped in a copy of the
// enclosing selector; otherwise the enclosing selector is prepended as a
// descendant. Non-style-rule nodes are placed unchanged.
func (e *evaluator) renestLoadedNode(n cssNode, fr *frame) {
	parent := fr.parentSel
	r, ok := n.(*cssStyleRule)
	if !ok {
		if c, isComment := n.(*cssComment); isComment {
			// A loud comment carries no selector, so dart keeps it nested inside a
			// copy of the enclosing rule (e.g. `a { /* … */ }`) rather than lifting
			// it to the top level.
			fr.container.appendNode(&cssStyleRule{
				raw:         true,
				rawSel:      parent.serialize(false),
				nodes:       []cssNode{c},
				blankBefore: c.blankBefore,
			})
			return
		}
		fr.container.appendNode(n)
		return
	}
	if r.raw {
		child, err := parseSelectorListStrErr(r.rawSel, true, true)
		if err == nil && selListContainsParent(child) {
			// Native-CSS `&`: keep the rule literal, wrapped under the parent.
			fr.container.appendNode(&cssStyleRule{
				raw:         true,
				rawSel:      parent.serialize(false),
				nodes:       []cssNode{r},
				blankBefore: r.blankBefore,
			})
			return
		}
		combined := resolveNestingImpl(selectorList{list: child}, parent, true)
		r.rawSel = combined.serialize(false)
		fr.container.appendNode(r)
		return
	}
	resolved := resolveNestingImpl(r.selector, parent, true)
	r.selector = resolved
	r.original = resolved
	fr.container.appendNode(r)
}

// sortedKeys returns the keys of a string-keyed map in sorted order.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
