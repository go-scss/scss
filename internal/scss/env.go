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
}

type funcEntry struct {
	def *FunctionDef
	env *environment
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
	mixins     map[string]*mixinEntry
	funcs      map[string]*funcEntry
	modules    map[string]*module // namespace -> module
	// content holds the @content block passed to the current mixin invocation.
	content     []Stmt
	contentEnv  *environment
	contentArgs *ParamList
	// builtinAliases maps a namespace to a built-in module name (e.g. "m"->"math").
	builtinAliases map[string]string
	// builtinGlobals lists built-in modules imported with "as *".
	builtinGlobals []string
}

func newEnvironment() *environment {
	return &environment{
		scopes:     []map[string]Value{{}},
		semiGlobal: []bool{true},
		mixins:     map[string]*mixinEntry{},
		funcs:      map[string]*funcEntry{},
		modules:    map[string]*module{},
	}
}

// pushScope opens an opaque scope: a @function/@mixin/style-rule boundary that
// blocks implicit write-through to the global scope.
func (e *environment) pushScope() {
	e.scopes = append(e.scopes, map[string]Value{})
	e.semiGlobal = append(e.semiGlobal, false)
}

// pushControlScope opens a control-flow (@if/@each/@for/@while) scope. It stays
// transparent to global assignment iff the current innermost scope is, so
// implicit writes to a global variable propagate to global at the stylesheet
// root but are trapped once a rule/function/mixin boundary intervenes.
func (e *environment) pushControlScope() {
	e.scopes = append(e.scopes, map[string]Value{})
	e.semiGlobal = append(e.semiGlobal, e.semiGlobal[len(e.semiGlobal)-1])
}

func (e *environment) popScope() {
	e.scopes = e.scopes[:len(e.scopes)-1]
	e.semiGlobal = e.semiGlobal[:len(e.semiGlobal)-1]
}

// defineVar declares a variable in the innermost scope (parameter binding and
// loop variables), unconditionally shadowing any enclosing variable.
func (e *environment) defineVar(name string, val Value) {
	e.scopes[len(e.scopes)-1][normIdent(name)] = val
}

func (e *environment) getVar(name string) (Value, bool) {
	name = normIdent(name)
	for i := len(e.scopes) - 1; i >= 0; i-- {
		if v, ok := e.scopes[i][name]; ok {
			return v, true
		}
	}
	return nil, false
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
		e.scopes[0][name] = val
		return
	}
	for i := len(e.scopes) - 1; i >= 0; i-- {
		if _, ok := e.scopes[i][name]; ok {
			if i == 0 && !e.semiGlobal[len(e.semiGlobal)-1] {
				break // global-only binding trapped behind an opaque boundary
			}
			e.scopes[i][name] = val
			return
		}
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

// module is a compiled @use/@forward stylesheet exposing members.
type module struct {
	vars   map[string]Value
	mixins map[string]*mixinEntry
	funcs  map[string]*funcEntry
	env    *environment
	// scope is this module's @extend boundary. It lets a downstream module that
	// @uses/@forwards this one propagate its extends onto this module's rules at
	// the global finalize. nil for plain-CSS modules (they expose no members).
	scope *moduleScope
	// forwards records the modules this stylesheet re-exports via @forward,
	// each with its optional prefix. A namespaced variable assignment to a
	// forwarded member must reach the original defining module's storage (the
	// map its own functions and mixins read), so writes propagate down this
	// chain, matching dart-sass's ForwardedModuleView.
	forwards []forwardedMod
}

// setVar writes a member variable, propagating the write to whichever forwarded
// module actually defines it so the defining module's functions observe the new
// value. Returns whether the variable was found (locally or downstream).
func (m *module) setVar(name string, val Value) bool {
	found := false
	if _, ok := m.vars[name]; ok {
		m.vars[name] = val
		found = true
	}
	for _, fw := range m.forwards {
		if !strings.HasPrefix(name, fw.prefix) {
			continue
		}
		if fw.mod.setVar(strings.TrimPrefix(name, fw.prefix), val) {
			found = true
		}
	}
	return found
}
