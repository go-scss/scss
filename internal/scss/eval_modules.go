// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import (
	"path"
	"strings"
)

var builtinModuleNames = map[string]bool{
	"math": true, "color": true, "string": true, "list": true,
	"map": true, "selector": true, "meta": true,
}

func (e *evaluator) evalUse(n *Use, fr *frame) {
	ns := e.namespaceFor(n.URL, n.Namespace)
	if strings.HasPrefix(n.URL, "sass:") {
		modName := strings.TrimPrefix(n.URL, "sass:")
		if !builtinModuleNames[modName] {
			e.fail("Can't find module \"%s\".", n.URL)
		}
		if n.NoNS {
			e.env.builtinGlobals = append(e.env.builtinGlobals, modName)
		} else {
			if e.env.builtinAliases == nil {
				e.env.builtinAliases = map[string]string{}
			}
			e.env.builtinAliases[ns] = modName
		}
		return
	}
	mod := e.loadModule(n.URL, e.buildConfig(nil, n.Config), fr)
	if n.NoNS {
		e.mergeModuleGlobally(mod, "")
	} else {
		e.env.modules[ns] = mod
	}
}

// buildConfig evaluates a `with (...)` clause in the current context and merges
// it onto a base configuration (the importing module's own incoming config, for
// @forward; nil for @use). A guarded (`!default`) entry is applied only when the
// base does not already configure that variable with a non-null value, so a
// downstream configuration wins over an upstream `@forward ... with (... !default)`;
// a plain entry always overrides. Names are canonicalised so configuration is
// hyphen/underscore-insensitive. Returns nil when the result is empty.
func (e *evaluator) buildConfig(base map[string]Value, cfg []ConfigVar) map[string]Value {
	out := map[string]Value{}
	for k, v := range base {
		out[k] = v
	}
	for _, c := range cfg {
		name := normIdent(c.Name)
		// A `with (...)` configuration binding consumes its value like any other
		// variable assignment, so a slash number is stored as its quotient.
		v := numWithoutSlash(e.evalExpr(c.Value))
		if c.Default {
			if existing, ok := out[name]; ok {
				if _, isNull := existing.(*Null); !isNull {
					continue
				}
			}
		}
		out[name] = v
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// throughForwardConfig adjusts an incoming (implicit or explicit) configuration
// as it passes through a @forward rule, mirroring dart-sass
// Configuration.throughForward: an `as prefix-*` clause unprefixes the visible
// names (only names carrying the prefix survive, with the prefix stripped), and
// a show/hide clause limits the configurable variables to the permitted set.
// The names in show/hide are the members' *forwarded* (already unprefixed) names.
func throughForwardConfig(base map[string]Value, n *Forward) map[string]Value {
	if len(base) == 0 {
		return base
	}
	out := base
	if n.Prefix != "" {
		prefix := normIdent(n.Prefix)
		np := map[string]Value{}
		for k, v := range out {
			if rest, ok := strings.CutPrefix(k, prefix); ok {
				np[rest] = v
			}
		}
		out = np
	}
	if n.HasShow {
		shown := forwardVarNames(n.Show)
		np := map[string]Value{}
		for k, v := range out {
			if shown[k] {
				np[k] = v
			}
		}
		out = np
	} else if n.HasHide {
		hidden := forwardVarNames(n.Hide)
		np := map[string]Value{}
		for k, v := range out {
			if !hidden[k] {
				np[k] = v
			}
		}
		out = np
	}
	return out
}

// forwardVarNames extracts the canonicalised variable names ("$"-prefixed
// entries) from a @forward show/hide member list.
func forwardVarNames(members []string) map[string]bool {
	out := map[string]bool{}
	for _, m := range members {
		if strings.HasPrefix(m, "$") {
			out[normIdent(m[1:])] = true
		}
	}
	return out
}

// implicitConfigSnapshot captures every variable currently visible in the
// evaluator's environment (all scopes, innermost winning), the way dart-sass's
// Environment.toImplicitConfiguration does. A legacy @import of a stylesheet that
// contains @forward rules passes this snapshot down as the base configuration so
// variables from the importing scope configure the forwarded modules' !default
// variables (dart's import configuration flow).
func (e *evaluator) implicitConfigSnapshot() map[string]Value {
	out := map[string]Value{}
	for _, scope := range e.env.scopes {
		for k, v := range scope {
			out[k] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// stmtsHaveForward reports whether a parsed stylesheet contains any top-level
// @forward rule (dart only builds an implicit import configuration when the
// imported file forwards something, since @use ignores it).
func stmtsHaveForward(stmts []Stmt) bool {
	for _, s := range stmts {
		if _, ok := s.(*Forward); ok {
			return true
		}
	}
	return false
}

func (e *evaluator) namespaceFor(url, explicit string) string {
	if explicit != "" {
		return explicit
	}
	base := url
	if strings.HasPrefix(base, "sass:") {
		return strings.TrimPrefix(base, "sass:")
	}
	base = path.Base(base)
	base = strings.TrimSuffix(base, ".scss")
	base = strings.TrimSuffix(base, ".sass")
	base = strings.TrimPrefix(base, "_")
	return base
}

func (e *evaluator) evalForward(n *Forward, fr *frame) {
	if strings.HasPrefix(n.URL, "sass:") {
		return
	}
	mod := filterForwarded(e.loadModule(n.URL, e.buildConfig(throughForwardConfig(e.incomingConfig, n), n.Config), fr), n, n.Prefix)
	e.mergeModuleGlobally(mod, n.Prefix)
	// track for downstream @use of THIS stylesheet
	e.forwarded = append(e.forwarded, forwardedMod{mod: mod, prefix: n.Prefix})
}

// filterForwarded applies a @forward's show/hide clauses, returning a module
// exposing only the permitted members (an unfiltered forward returns mod as-is).
// Variables are named with a leading "$" in show/hide lists; functions and
// mixins share their bare name.
func filterForwarded(mod *module, n *Forward, prefix string) *module {
	if !n.HasShow && !n.HasHide {
		return mod
	}
	names := map[string]bool{}
	vars := map[string]bool{}
	for _, m := range n.Show {
		if strings.HasPrefix(m, "$") {
			vars[normIdent(m[1:])] = true
		} else {
			names[normIdent(m)] = true
		}
	}
	for _, m := range n.Hide {
		if strings.HasPrefix(m, "$") {
			vars[normIdent(m[1:])] = true
		} else {
			names[normIdent(m)] = true
		}
	}
	// show/hide match the members' *forwarded* (prefixed) names.
	nameAllowed := func(k string) bool {
		if n.HasShow {
			return names[normIdent(prefix+k)]
		}
		return !names[normIdent(prefix+k)]
	}
	varAllowed := func(k string) bool {
		if n.HasShow {
			return vars[normIdent(prefix+k)]
		}
		return !vars[normIdent(prefix+k)]
	}
	// The filtered view copies member values, but a namespaced assignment to a
	// still-visible variable must reach the underlying module's real storage, so
	// the view forwards writes to the module it filters.
	out := &module{vars: map[string]Value{}, mixins: map[string]*mixinEntry{}, funcs: map[string]*funcEntry{}, env: mod.env, forwards: []forwardedMod{{mod: mod}}}
	for k, v := range mod.vars {
		if varAllowed(k) {
			out.vars[k] = v
		}
	}
	for k, v := range mod.mixins {
		if nameAllowed(k) {
			out.mixins[k] = v
		}
	}
	for k, v := range mod.funcs {
		if nameAllowed(k) {
			out.funcs[k] = v
		}
	}
	return out
}

// isPrivateMember reports whether a member name is private to its module.
// dart-sass treats a leading "-" or "_" as private; normIdent has already
// folded "_" to "-", so a single hyphen prefix identifies both spellings.
func isPrivateMember(name string) bool {
	return strings.HasPrefix(name, "-")
}

// publicFuncs returns the subset of a module's functions that form its public
// API: private members (leading "-"/"_") are visible only inside the defining
// module, never through a namespace, @use "as *" or @forward. The map is copied
// so filtering never mutates the module's own resolution environment.
func publicFuncs(in map[string]*funcEntry) map[string]*funcEntry {
	out := make(map[string]*funcEntry, len(in))
	for k, v := range in {
		if isPrivateMember(k) {
			continue
		}
		out[k] = v
	}
	return out
}

// publicMixins is publicFuncs for mixins.
func publicMixins(in map[string]*mixinEntry) map[string]*mixinEntry {
	out := make(map[string]*mixinEntry, len(in))
	for k, v := range in {
		if isPrivateMember(k) {
			continue
		}
		out[k] = v
	}
	return out
}

func (e *evaluator) mergeModuleGlobally(mod *module, prefix string) {
	for k, v := range mod.vars {
		e.env.setGlobalIfAbsent(prefix+k, v)
	}
	for k, v := range mod.mixins {
		e.env.mixins[normIdent(prefix+k)] = v
	}
	for k, v := range mod.funcs {
		e.env.funcs[normIdent(prefix+k)] = v
	}
}

type forwardedMod struct {
	mod    *module
	prefix string
}

// emitModuleCSS appends a loaded module's top-level CSS to the output as a
// single chunk that takes part in the importing stylesheet's blank-line
// grouping. dart-sass separates a module's emitted CSS from surrounding
// top-level statements exactly as it separates ordinary source groups: a blank
// line precedes the chunk when the previous group was a style rule, and the
// chunk counts (for what follows) as its own last visible node.
func (e *evaluator) emitModuleCSS(nodes []cssNode, fr *frame) {
	var firstVisible, lastVisible cssNode
	for _, n := range nodes {
		if isEmptyContainer(n) {
			continue
		}
		if firstVisible == nil {
			firstVisible = n
		}
		lastVisible = n
	}
	if firstVisible != nil {
		_, lastIsRule := lastVisible.(*cssStyleRule)
		fr.group.curIsStyleRule = lastIsRule
		setBlankBefore(firstVisible, e.consumeGroup(fr))
	}
	for _, n := range nodes {
		e.root.appendNode(n)
	}
}

// parseModuleSource parses a resolved stylesheet source, honouring the resolved
// file's syntax: a `.sass` file is written in the indented grammar and must be
// converted to bracketed SCSS before parsing, exactly as the entry file is.
func parseModuleSource(resolved, src string) ([]Stmt, error) {
	if strings.HasSuffix(resolved, ".sass") {
		src = convertIndented(src)
	}
	return parseStylesheet(src)
}

func (e *evaluator) loadModule(url string, config map[string]Value, fr *frame) *module {
	if m, ok := e.loaded[url]; ok {
		return m
	}
	src, resolved, ok := e.resolve(url)
	if !ok {
		e.fail("Can't find stylesheet to import: %s", url)
	}
	// A module reached through more than one path (a diamond) is loaded once for
	// the whole compilation: reuse the singleton and do NOT re-emit its CSS.
	if m, ok := e.sharedLoaded[resolved]; ok {
		e.loaded[url] = m
		// A diamond dependency: this module is reused, but the current module
		// still depends on it, so downstream extends here must reach its rules.
		e.dependsOn(m.scope)
		return m
	}
	for _, s := range e.loadStack {
		if s == resolved {
			e.fail("Module loop: %s", resolved)
		}
	}
	// A file resolved with a .css extension is parsed as plain CSS and emitted
	// at the top level; it exposes no Sass members.
	if strings.HasSuffix(resolved, ".css") {
		nodes, err := parsePlainCSS(src)
		if err != nil {
			panic(err)
		}
		e.loadedURLs = append(e.loadedURLs, resolved)
		e.emitModuleCSS(nodes, fr)
		mod := emptyModule()
		e.loaded[url] = mod
		e.sharedLoaded[resolved] = mod
		return mod
	}
	stmts, err := parseModuleSource(resolved, src)
	if err != nil {
		panic(err)
	}
	sub := newEvaluator(e.importer)
	sub.loadStack = append(append([]string(nil), e.loadStack...), resolved)
	sub.loadedURLs = e.loadedURLs
	sub.sharedLoaded = e.sharedLoaded
	e.adoptScope(sub)
	e.dependsOn(sub.scope)
	// Remember the incoming configuration so this module's own @forward rules can
	// propagate it downstream. Unlike a pre-seed of the global scope, dart-sass
	// applies configuration at each top-level `!default` declaration, so a
	// configured variable does not "exist" (meta.variable-exists) until its own
	// declaration line runs — where the configured value overrides the default.
	sub.incomingConfig = config
	sub.runModule(stmts)
	e.warnings = append(e.warnings, sub.warnings...)
	e.loadedURLs = append(e.loadedURLs, resolved)
	// include used module CSS in our output, as a chunk that participates in
	// the importing stylesheet's top-level blank-line grouping.
	e.emitModuleCSS(sub.root.children(), fr)
	mod := &module{
		vars:     sub.env.scopes[0],
		mixins:   publicMixins(sub.env.mixins),
		funcs:    publicFuncs(sub.env.funcs),
		env:      sub.env,
		scope:    sub.scope,
		forwards: sub.forwarded,
	}
	// Forwarded members are already exposed in the module's own tables: while the
	// stylesheet ran, each @forward called mergeModuleGlobally on the sub-
	// evaluator, seeding scopes[0]/funcs/mixins (which back mod.vars/funcs/mixins)
	// while letting the module's OWN definitions win over a forwarded name.
	e.loaded[url] = mod
	e.sharedLoaded[resolved] = mod
	return mod
}

// emptyModule builds a module with no members, used for plain-CSS files loaded
// through @use/@forward: they contribute CSS but expose nothing to Sass.
func emptyModule() *module {
	return &module{
		vars:   map[string]Value{},
		mixins: map[string]*mixinEntry{},
		funcs:  map[string]*funcEntry{},
		env:    newEnvironment(),
	}
}

// runModule evaluates a loaded module's body. @extend is NOT finalised here:
// the module's rules may still be extended by downstream modules, so every
// scope is finalised together at the end of the compilation (applyAllExtends).
func (e *evaluator) runModule(stmts []Stmt) {
	fr := &frame{container: e.root, rootContainer: e.root, mediaParent: e.root, atContainer: true, group: &groupInfo{}}
	e.evalBody(stmts, fr, true)
}

func (e *evaluator) resolve(url string) (string, string, bool) {
	if e.importer == nil {
		return "", "", false
	}
	return e.importer(url)
}

func (e *evaluator) evalImport(n *Import, fr *frame) {
	for _, item := range n.Imports {
		if item.Plain {
			params := "\"" + item.URL + "\""
			if len(item.URL) >= 4 && strings.EqualFold(item.URL[:4], "url(") {
				params = item.URL
			}
			if item.Mods != nil {
				if m := e.resolveImportMods(item.Mods); m != "" {
					params += " " + m
				}
			}
			at := &cssAtRule{name: "import", params: params, hasBody: false}
			at.blankBefore = e.consumeGroup(fr)
			fr.container.appendNode(at)
			continue
		}
		src, resolved, ok := e.resolve(item.URL)
		if !ok {
			// treat as plain CSS import passthrough
			at := &cssAtRule{name: "import", params: "\"" + item.URL + "\"", hasBody: false}
			at.blankBefore = e.consumeGroup(fr)
			fr.container.appendNode(at)
			continue
		}
		// A .css file imported at the top level is parsed as plain CSS and
		// injected verbatim (nesting preserved). A nested @import of a .css file
		// resolves through the ordinary path so its first level combines with the
		// enclosing selector, matching dart-sass.
		if !fr.hasParent && strings.HasSuffix(resolved, ".css") {
			nodes, err := parsePlainCSS(src)
			if err != nil {
				panic(err)
			}
			e.emitModuleCSS(nodes, fr)
			continue
		}
		stmts, err := parseModuleSource(resolved, src)
		if err != nil {
			panic(err)
		}
		// A legacy @import inlines the file into the current module. If the file
		// forwards other modules, dart snapshots the importing scope's variables
		// as an implicit configuration that flows into those @forward rules, so a
		// variable set before the @import configures a forwarded module's
		// !default variable. The snapshot is taken once, at the import site.
		if stmtsHaveForward(stmts) {
			saved := e.incomingConfig
			e.incomingConfig = e.implicitConfigSnapshot()
			e.evalBody(stmts, fr, true)
			e.incomingConfig = saved
		} else {
			e.evalBody(stmts, fr, true)
		}
	}
}
