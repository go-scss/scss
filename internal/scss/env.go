// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import "strings"

// normIdent canonicalises a Sass identifier: hyphens and underscores are
// interchangeable everywhere (variables, functions, mixins), so both fold to a
// single spelling for table keys.
func normIdent(s string) string {
	if !strings.Contains(s, "_") {
		return s
	}
	return strings.ReplaceAll(s, "_", "-")
}

// callable holds a user-defined mixin or function together with its defining
// environment (for lexical scoping of module members).
type mixinEntry struct {
	def *MixinDef
	env *environment
	// defDepth is the number of scopes open in env when the mixin was defined —
	// the length of its lexical scope chain. Like a function, a mixin runs in
	// exactly these scopes (plus a fresh one for its parameters), so its body sees
	// only its params and whatever was visible where it was declared, never the
	// dynamic caller's inner scopes. This isolates each invocation (including
	// recursion) instead of mutating the shared definition environment.
	defDepth int
}

type funcEntry struct {
	def *FunctionDef
	env *environment
	// defDepth is the number of scopes open in env when the function was defined
	// — the length of its lexical scope chain. A call runs in exactly these
	// scopes (plus a fresh one for the arguments), so the body sees only its
	// params and whatever was visible where it was declared, never the dynamic
	// caller's inner scopes. This mirrors dart-sass running a callable in its
	// captured closure rather than at the call site.
	defDepth int
	// builtin, when non-nil, makes this entry a built-in module function re-exported
	// through a user stylesheet's `@forward "sass:math"` (etc.). def/env are unused
	// in that case; a call dispatches straight to the native implementation. name is
	// the built-in's bare identifier, passed through as callInfo.fn.
	builtin builtinFunc
	name    string
}

// environment holds variable scopes plus mixin/function/module tables.
type environment struct {
	scopes []map[string]Value
	// semiGlobal[i] marks scope i as transparent to the global scope for the
	// purpose of variable assignment: an implicit (non-!global) assignment to a
	// variable that only exists at global scope writes THROUGH to global rather
	// than creating a scope-local shadow. The global scope is semi-global by
	// definition, and a control-flow scope (@if/@each/@for/@while body) is
	// semi-global iff its parent is — i.e. no @function/@mixin/style-rule
	// boundary lies between it and the root. This mirrors dart-sass's
	// Environment.scope(semiGlobal:) chained down from the global scope.
	semiGlobal []bool
	// mixins and funcs are scope stacks parallel to scopes: a @function/@mixin
	// declared inside a style rule, another function, or a mixin is visible only
	// within that scope and shadows an outer definition of the same name, exactly
	// as dart-sass stores callables in its Environment scope chain. Index 0 is the
	// module's own global table; pushScope/popScope keep the three stacks in step.
	mixins  []map[string]*mixinEntry
	funcs   []map[string]*funcEntry
	modules map[string]*module // namespace -> module
	// content holds the @content block passed to the current mixin invocation.
	content     []Stmt
	contentEnv  *environment
	contentArgs *ParamList
	// builtinAliases maps a namespace to a built-in module name (e.g. "m"->"math").
	builtinAliases map[string]string
	// builtinGlobals lists built-in modules imported with "as *".
	builtinGlobals []string
	// globalModules lists the user modules imported with `@use ... as *`. Unlike a
	// namespaced @use, their members are resolvable by bare name in THIS module's
	// scope, but they are NOT part of this module's exported API (they are not
	// re-exported the way a @forward'd member is). This mirrors dart-sass's
	// separation of a module's resolution environment (_globalModules) from the
	// members it exports (Environment.toModule), and keeps a `@use as *` member
	// from leaking transitively to a downstream module that @uses this one.
	globalModules []*module
	// importedModules lists the modules whose members were re-exported into THIS
	// module through a legacy @import of a @forwarding stylesheet. Unlike a
	// `@use as *`, an @import inlines the members: they shadow the module's own
	// global scope (dart's "forwarded definitions take precedence through
	// imports"), a later import wins over an earlier one (hence reverse-order
	// resolution), and an implicit assignment to such a name writes through to the
	// source module's slot even from a nested scope.
	importedModules []importedMod
}

// importedMod is a module re-exported into an environment through @import, with
// the optional prefix a `@forward ... as p-*` applied to its member names.
type importedMod struct {
	mod    *module
	prefix string
}

func newEnvironment() *environment {
	return &environment{
		scopes:     []map[string]Value{{}},
		semiGlobal: []bool{true},
		mixins:     []map[string]*mixinEntry{{}},
		funcs:      []map[string]*funcEntry{{}},
		modules:    map[string]*module{},
	}
}

// pushScope opens an opaque scope: a @function/@mixin/style-rule boundary that
// blocks implicit write-through to the global scope.
func (e *environment) pushScope() {
	e.scopes = append(e.scopes, map[string]Value{})
	e.semiGlobal = append(e.semiGlobal, false)
	e.mixins = append(e.mixins, map[string]*mixinEntry{})
	e.funcs = append(e.funcs, map[string]*funcEntry{})
}

// pushControlScope opens a control-flow (@if/@each/@for/@while) scope. It stays
// transparent to global assignment iff the current innermost scope is, so
// implicit writes to a global variable propagate to global at the stylesheet
// root but are trapped once a rule/function/mixin boundary intervenes.
func (e *environment) pushControlScope() {
	e.scopes = append(e.scopes, map[string]Value{})
	e.semiGlobal = append(e.semiGlobal, e.semiGlobal[len(e.semiGlobal)-1])
	e.mixins = append(e.mixins, map[string]*mixinEntry{})
	e.funcs = append(e.funcs, map[string]*funcEntry{})
}

func (e *environment) popScope() {
	e.scopes = e.scopes[:len(e.scopes)-1]
	e.semiGlobal = e.semiGlobal[:len(e.semiGlobal)-1]
	e.mixins = e.mixins[:len(e.mixins)-1]
	e.funcs = e.funcs[:len(e.funcs)-1]
}

// defineMixin declares a mixin in the innermost scope, shadowing any enclosing
// definition of the same name for the lifetime of that scope.
func (e *environment) defineMixin(name string, m *mixinEntry) {
	e.mixins[len(e.mixins)-1][normIdent(name)] = m
}

// defineFunc declares a function in the innermost scope.
func (e *environment) defineFunc(name string, f *funcEntry) {
	e.funcs[len(e.funcs)-1][normIdent(name)] = f
}

// getMixin resolves a mixin by walking the scope chain innermost-first.
func (e *environment) getMixin(name string) (*mixinEntry, bool) {
	name = normIdent(name)
	for i := len(e.mixins) - 1; i >= 0; i-- {
		if m, ok := e.mixins[i][name]; ok {
			return m, true
		}
	}
	return nil, false
}

// getFunc resolves a function by walking the scope chain innermost-first.
func (e *environment) getFunc(name string) (*funcEntry, bool) {
	name = normIdent(name)
	for i := len(e.funcs) - 1; i >= 0; i-- {
		if f, ok := e.funcs[i][name]; ok {
			return f, true
		}
	}
	return nil, false
}

// closureAt returns an environment that shares this one's module-level state
// (modules, global-module lists, builtin aliases, @content) but restricts the
// variable/mixin/function scope chains to their first n scopes, each still the
// live map so global definitions added later remain visible. Pushing a scope on
// the result never disturbs the original. It realises a callable's lexical
// closure: the body runs in the scopes visible where it was defined, not the
// dynamic caller's inner scopes.
func (e *environment) closureAt(n int) *environment {
	c := *e
	c.scopes = append([]map[string]Value(nil), e.scopes[:n]...)
	c.semiGlobal = append([]bool(nil), e.semiGlobal[:n]...)
	c.mixins = append([]map[string]*mixinEntry(nil), e.mixins[:n]...)
	c.funcs = append([]map[string]*funcEntry(nil), e.funcs[:n]...)
	return &c
}

// globalMixins returns the module's own global mixin table (scope 0).
func (e *environment) globalMixins() map[string]*mixinEntry { return e.mixins[0] }

// globalFuncs returns the module's own global function table (scope 0).
func (e *environment) globalFuncs() map[string]*funcEntry { return e.funcs[0] }

// defineVar declares a variable in the innermost scope (parameter binding and
// loop variables), unconditionally shadowing any enclosing variable.
func (e *environment) defineVar(name string, val Value) {
	e.scopes[len(e.scopes)-1][normIdent(name)] = val
}

func (e *environment) getVar(name string) (Value, bool) {
	name = normIdent(name)
	// Inner (non-global) scopes shadow everything.
	for i := len(e.scopes) - 1; i >= 1; i-- {
		if v, ok := e.scopes[i][name]; ok {
			return v, true
		}
	}
	// A member re-exported through a legacy @import shadows the module's own
	// global scope (later imports winning over earlier ones).
	if v, ok := e.importedModuleVar(name); ok {
		return v, true
	}
	if v, ok := e.scopes[0][name]; ok {
		return v, true
	}
	// A bare name not otherwise bound may be exposed by a `@use as *` module
	// (dart-sass Environment.getVariable falls back to _globalModules).
	for _, m := range e.globalModules {
		if v, ok := m.getVar(name); ok {
			return v, true
		}
	}
	return nil, false
}

// importedModuleVar resolves a name against the @import-re-exported modules,
// latest import first.
func (e *environment) importedModuleVar(name string) (Value, bool) {
	for i := len(e.importedModules) - 1; i >= 0; i-- {
		im := e.importedModules[i]
		if rest, ok := strings.CutPrefix(name, im.prefix); ok {
			if v, ok := im.mod.getVar(rest); ok {
				return v, true
			}
		}
	}
	return nil, false
}

// setImportedModuleVar writes through to whichever @import-re-exported module
// exposes the name (latest import first), returning whether one did.
func (e *environment) setImportedModuleVar(name string, val Value) bool {
	for i := len(e.importedModules) - 1; i >= 0; i-- {
		im := e.importedModules[i]
		if rest, ok := strings.CutPrefix(name, im.prefix); ok {
			if im.mod.setVar(rest, val) {
				return true
			}
		}
	}
	return false
}

// globalModuleFunc resolves a bare function name against the `@use as *` modules.
func (e *environment) globalModuleFunc(name string) (*funcEntry, bool) {
	name = normIdent(name)
	for _, m := range e.globalModules {
		if f, ok := m.getFunc(name); ok {
			return f, true
		}
	}
	return nil, false
}

// globalModuleMixin resolves a bare mixin name against the `@use as *` modules.
func (e *environment) globalModuleMixin(name string) (*mixinEntry, bool) {
	name = normIdent(name)
	for _, m := range e.globalModules {
		if mx, ok := m.getMixin(name); ok {
			return mx, true
		}
	}
	return nil, false
}

// setGlobalModuleVar writes a global assignment through to whichever `@use as *`
// module already exposes the variable, returning whether one did. dart-sass
// routes a global variable assignment to the owning global module before the
// importing module's own global scope.
func (e *environment) setGlobalModuleVar(name string, val Value) bool {
	name = normIdent(name)
	for _, m := range e.globalModules {
		if m.setVar(name, val) {
			return true
		}
	}
	return false
}

func (e *environment) hasVar(name string) bool {
	_, ok := e.getVar(name)
	return ok
}

// setVar implements Sass assignment scoping (dart-sass Environment.setVariable):
// an existing binding is updated in the innermost scope that holds it, EXCEPT a
// binding that only exists at the global scope is written through to global just
// when the current scope is still transparent to it (semi-global); otherwise —
// behind a rule/function/mixin boundary — a fresh scope-local shadow is created.
// A name bound nowhere is created in the current innermost scope.
func (e *environment) setVar(name string, val Value, global bool) {
	name = normIdent(name)
	if global {
		if e.setImportedModuleVar(name, val) || e.setGlobalModuleVar(name, val) {
			return
		}
		e.scopes[0][name] = val
		return
	}
	// Existing binding in an inner (non-global) scope: update it in place.
	for i := len(e.scopes) - 1; i >= 1; i-- {
		if _, ok := e.scopes[i][name]; ok {
			e.scopes[i][name] = val
			return
		}
	}
	// A member re-exported through a legacy @import is an existing binding (in the
	// source module) that an implicit assignment updates, even from a nested
	// scope — dart treats it as already declared, unlike a `@use as *` member.
	if e.setImportedModuleVar(name, val) {
		return
	}
	if _, ok := e.scopes[0][name]; ok {
		// Own global binding: writable in place only while the current scope is
		// still transparent to global; behind an opaque boundary a local shadow is
		// created instead.
		if e.semiGlobal[len(e.semiGlobal)-1] {
			e.scopes[0][name] = val
			return
		}
	} else if e.semiGlobal[len(e.semiGlobal)-1] && e.setGlobalModuleVar(name, val) {
		// Not bound anywhere and at the global level: an implicit assignment to a
		// name a `@use as *` module exposes writes through to that module.
		return
	}
	e.scopes[len(e.scopes)-1][name] = val
}

// setGlobalIfAbsent assigns at global scope only if absent (for @use with defaults).
func (e *environment) setGlobalIfAbsent(name string, val Value) {
	name = normIdent(name)
	if _, ok := e.scopes[0][name]; !ok {
		e.scopes[0][name] = val
	}
}

// module is a compiled @use/@forward stylesheet exposing members. A module's
// exported API is kept distinct from the resolution environment that ran it: the
// vars/mixins/funcs maps hold only the module's OWN public definitions (its
// env.scopes[0]/funcs/mixins), while members re-exported via @forward are reached
// through the forwards chain. This mirrors dart-sass, where a `@use x as *` inside
// this stylesheet joins the resolution environment WITHOUT being re-exported,
// whereas a `@forward x` is re-exported WITHOUT becoming locally callable.
type module struct {
	vars   map[string]Value // OWN global variables (aliases env.scopes[0])
	mixins map[string]*mixinEntry
	funcs  map[string]*funcEntry
	env    *environment
	// scope is this module's @extend boundary. It lets a downstream module that
	// @uses/@forwards this one propagate its extends onto this module's rules at
	// the global finalize. nil for plain-CSS modules (they expose no members).
	scope *moduleScope
	// forwards records the modules this stylesheet re-exports via @forward, each
	// with its optional prefix. Reads compose own-then-forwarded (own wins);
	// writes go forwarded-first (dart's _modulesByVariable precedence) so a
	// namespaced assignment reaches the original defining module's storage.
	forwards []forwardedMod
	// writeThrough, when non-nil, is the underlying module a filtered @forward
	// view delegates namespaced writes to. Reads use the view's own (already
	// filtered) maps so hidden members never leak; writes reach real storage.
	writeThrough *module
	// cssNodes is the module's top-level CSS as emitted at its first load. A
	// legacy @import that transitively reaches this already-loaded module re-emits
	// a deep clone of these nodes at the import site (dart's _combineCss(clone:
	// true)): @import duplicates CSS even when it has been @used and loaded once.
	cssNodes []cssNode
	// combine is this module's node in the deferred combine tree (dart's per-
	// module CSS + preModuleComments). The entry stylesheet walks the tree at
	// finalize (combineCss) to interleave each module's leading @import region and
	// pre-@use comments exactly as dart's _combineCss does.
	combine *combineNode
}

// getVar reads a member variable exposed by this module's public API: its own
// global definitions win over anything it @forwards (dart's own-before-forwarded
// read order).
func (m *module) getVar(name string) (Value, bool) {
	if v, ok := m.vars[name]; ok {
		return v, true
	}
	for _, fw := range m.forwards {
		if rest, ok := strings.CutPrefix(name, fw.prefix); ok {
			if v, ok := fw.mod.getVar(rest); ok {
				return v, true
			}
		}
	}
	return nil, false
}

// getFunc reads a public function exposed by this module (own before forwarded).
func (m *module) getFunc(name string) (*funcEntry, bool) {
	if f, ok := m.funcs[name]; ok {
		return f, true
	}
	for _, fw := range m.forwards {
		if rest, ok := strings.CutPrefix(name, fw.prefix); ok {
			if f, ok := fw.mod.getFunc(rest); ok {
				return f, true
			}
		}
	}
	return nil, false
}

// getMixin reads a public mixin exposed by this module (own before forwarded).
func (m *module) getMixin(name string) (*mixinEntry, bool) {
	if mx, ok := m.mixins[name]; ok {
		return mx, true
	}
	for _, fw := range m.forwards {
		if rest, ok := strings.CutPrefix(name, fw.prefix); ok {
			if mx, ok := fw.mod.getMixin(rest); ok {
				return mx, true
			}
		}
	}
	return nil, false
}

// allVars enumerates every variable this module exports: forwarded members first
// (unprefixed→prefixed) then the module's own, so an own definition overrides a
// forwarded one of the same name.
func (m *module) allVars() map[string]Value {
	out := map[string]Value{}
	for _, fw := range m.forwards {
		for k, v := range fw.mod.allVars() {
			out[fw.prefix+k] = v
		}
	}
	for k, v := range m.vars {
		out[k] = v
	}
	return out
}

// allFuncs is allVars for functions.
func (m *module) allFuncs() map[string]*funcEntry {
	out := map[string]*funcEntry{}
	for _, fw := range m.forwards {
		for k, v := range fw.mod.allFuncs() {
			out[fw.prefix+k] = v
		}
	}
	for k, v := range m.funcs {
		out[k] = v
	}
	return out
}

// allMixins is allVars for mixins.
func (m *module) allMixins() map[string]*mixinEntry {
	out := map[string]*mixinEntry{}
	for _, fw := range m.forwards {
		for k, v := range fw.mod.allMixins() {
			out[fw.prefix+k] = v
		}
	}
	for k, v := range m.mixins {
		out[k] = v
	}
	return out
}

// setVar writes a member variable, propagating the write to whichever forwarded
// module actually defines it so the defining module's functions observe the new
// value. Returns whether the variable was found (locally or downstream).
//
// A forwarded definition takes precedence over the module's own definition of
// the same name: dart-sass's _EnvironmentModule.setVariable consults the
// forwarded modules (its _modulesByVariable index) before its own global scope,
// so `ns.$x: v` on a module that both defines and @forwards `$x` updates the
// forwarded slot and leaves the module's own `$x` untouched.
func (m *module) setVar(name string, val Value) bool {
	found := false
	for _, fw := range m.forwards {
		if !strings.HasPrefix(name, fw.prefix) {
			continue
		}
		if fw.mod.setVar(strings.TrimPrefix(name, fw.prefix), val) {
			found = true
		}
	}
	if found {
		return true
	}
	if m.writeThrough != nil && m.writeThrough.setVar(name, val) {
		return true
	}
	if _, ok := m.vars[name]; ok {
		m.vars[name] = val
		return true
	}
	return false
}
