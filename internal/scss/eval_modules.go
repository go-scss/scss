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
		// `@use ... as *` exposes the module's members by bare name in THIS module
		// but does NOT re-export them: it joins the resolution environment as a
		// global module rather than being merged into this module's own tables.
		e.env.globalModules = append(e.env.globalModules, mod)
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
	// The module's own global scope is the lowest-precedence source.
	for k, v := range e.env.scopes[0] {
		out[k] = v
	}
	// Members re-exported through a legacy @import shadow the module's own global
	// scope; a later import wins over an earlier one.
	for _, im := range e.env.importedModules {
		for k, v := range im.mod.allVars() {
			out[im.prefix+k] = v
		}
	}
	// Inner scopes shadow everything, innermost last.
	for i := 1; i < len(e.env.scopes); i++ {
		for k, v := range e.env.scopes[i] {
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
	base = strings.TrimPrefix(base, "_")
	// The default namespace is the basename up to its first ".", which discards
	// every file extension at once (dart-sass _defaultNamespace), so
	// `@use "other.foo.bar.baz.scss"` namespaces as `other`.
	if i := strings.IndexByte(base, '.'); i >= 0 {
		base = base[:i]
	}
	return base
}

func (e *evaluator) evalForward(n *Forward, fr *frame) {
	if strings.HasPrefix(n.URL, "sass:") {
		real := strings.TrimPrefix(n.URL, "sass:")
		if !builtinModuleNames[real] {
			e.fail("Can't find stylesheet to import: %s", n.URL)
		}
		if len(n.Config) > 0 {
			e.fail("Built-in modules can't be configured.")
		}
		// Re-export the built-in module's public members so a downstream @use of THIS
		// stylesheet can reach them through the forwarding chain (with any as-prefix
		// and show/hide filter applied), exactly as a @forward of a user module does.
		mod := filterForwarded(builtinModuleAsModule(real), n, n.Prefix)
		if e.importDepth > 0 {
			e.importForwardedModule(mod, n.Prefix)
		}
		e.forwarded = append(e.forwarded, forwardedMod{mod: mod, prefix: normIdent(n.Prefix)})
		return
	}
	mod := filterForwarded(e.loadModule(n.URL, e.buildConfig(throughForwardConfig(e.incomingConfig, n), n.Config), fr), n, n.Prefix)
	// A @forward re-exports its members but does NOT make them callable in this
	// stylesheet (dart keeps _forwardedModules separate from the resolution
	// scope). The one exception is a @forward reached through a legacy @import,
	// which inlines the forwarded members into the importing scope.
	if e.importDepth > 0 {
		e.importForwardedModule(mod, n.Prefix)
	}
	// track for downstream @use of THIS stylesheet. The prefix is canonicalised so
	// the accessors' prefix arithmetic matches the dash/underscore-folded member
	// keys (a `@forward ... as d_*` prefixes the hyphen-spelled `$d-c`).
	e.forwarded = append(e.forwarded, forwardedMod{mod: mod, prefix: normIdent(n.Prefix)})
}

// builtinModuleAsModule materialises a built-in module (sass:math, sass:color, …)
// as a user-facing *module so it can be re-exported through @forward. Its native
// functions are wrapped as built-in funcEntries and its exported constant
// variables are copied verbatim; built-in modules expose no user mixins here.
func builtinModuleAsModule(real string) *module {
	funcs := map[string]*funcEntry{}
	for name, fn := range moduleRegistry(real) {
		funcs[normIdent(name)] = &funcEntry{builtin: fn, name: name}
	}
	vars := map[string]Value{}
	if real == "math" {
		for k, v := range mathModuleVars {
			vars[k] = v
		}
	}
	return &module{vars: vars, mixins: map[string]*mixinEntry{}, funcs: funcs, env: newEnvironment()}
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
	// The filtered view materialises the permitted subset of the module's full
	// (own + forwarded) API into its own maps, so reads never expose a hidden
	// member. A namespaced assignment to a still-visible variable must reach the
	// underlying module's real storage, so the view delegates writes to it.
	out := &module{vars: map[string]Value{}, mixins: map[string]*mixinEntry{}, funcs: map[string]*funcEntry{}, env: mod.env, writeThrough: mod}
	for k, v := range mod.allVars() {
		if varAllowed(k) {
			out.vars[k] = v
		}
	}
	for k, v := range mod.allMixins() {
		if nameAllowed(k) {
			out.mixins[k] = v
		}
	}
	for k, v := range mod.allFuncs() {
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

// importForwardedModule inlines a module's re-exported members into the current
// environment for the legacy @import path, where a @forward reached through
// @import contributes its members to the importing scope. Functions and mixins
// are copied into the environment's tables (a later @import's definition winning);
// variables are exposed through an imported-module reference so reads see the
// source's live slot, writes propagate to it, and a later import shadows an
// earlier one. The enumeration accessors compose any nested @forward.
func (e *evaluator) importForwardedModule(mod *module, prefix string) {
	for k, v := range mod.allMixins() {
		e.env.defineMixin(prefix+k, v)
	}
	for k, v := range mod.allFuncs() {
		e.env.defineFunc(prefix+k, v)
	}
	// A @forward reached through an @import nested inside a style rule (or other
	// local scope) makes its variables resolve to the source module's live slots
	// at that scope, exactly as dart-sass does: the import shadows a same-scope
	// variable set BEFORE it, yet a write AFTER it still reaches the source slot
	// (so the source's own functions observe it). The importedModules reference
	// added below already provides those live reads and write-through, but it is
	// consulted only AFTER the inner scopes; so a local of the same name set before
	// the import is cleared from the inner scopes to let the live slot win. At the
	// global scope there are no shadowing inner scopes, so nothing is cleared.
	for k := range mod.allVars() {
		name := normIdent(prefix + k)
		for i := len(e.env.scopes) - 1; i >= 1; i-- {
			delete(e.env.scopes[i], name)
		}
	}
	e.env.importedModules = append(e.env.importedModules, importedMod{mod: mod, prefix: normIdent(prefix)})
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
	// A module reached through a legacy @import that is itself nested inside a
	// style rule (importDepth > 0 && fr.hasParent) contributes its already-
	// serialized CSS as if it were nested content: dart re-nests each top-level
	// node under the enclosing selector rather than lifting it to the stylesheet
	// root. This is the same re-nesting meta.load-css and a direct nested @import
	// perform (placeLoadedCSS); a @forward reached while inlining such an @import
	// funnels its forwarded module's CSS here, so it must re-nest too.
	if e.importDepth > 0 && fr.hasParent {
		// The module's own top-level rules were flagged as top-level group ends when
		// it was compiled; re-nested as rule-body content they are no longer group
		// ends, so a declaration split after them stays gap-free (dart never blanks
		// rule-body siblings), matching a directly-nested @import.
		for _, n := range nodes {
			clearGroupEnd(n)
		}
		e.renestLoadedCSS(nodes, fr)
		return
	}
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

// reEmitImportedCSS re-emits a deep clone of an already-loaded module's CSS at
// the current site when that module is reached through a legacy @import
// (importDepth > 0). @import duplicates CSS even when the module has been @used
// and loaded once, so a second @import of a module — directly or through a
// forwarding `.import.scss` — prints its CSS again (dart's _combineCss with
// clone: true). A plain @use of an already-loaded module stays deduped and does
// nothing here.
func (e *evaluator) reEmitImportedCSS(m *module, fr *frame) {
	if e.importDepth == 0 || len(m.cssNodes) == 0 {
		return
	}
	e.emitModuleCSS(cloneCSSNodes(m.cssNodes), fr)
}

func (e *evaluator) loadModule(url string, config map[string]Value, fr *frame) *module {
	if m, ok := e.loaded[url]; ok {
		e.noteLoadedCombine(m, false)
		e.reEmitImportedCSS(m, fr)
		return m
	}
	// @use/@forward/meta.load-css resolve a real module, never an import-only file.
	src, resolved, ok := e.resolve(url, false)
	if !ok {
		e.fail("Can't find stylesheet to import: %s", url)
	}
	// A module reached through more than one path (a diamond) is loaded once for
	// the whole compilation: reuse the singleton and do NOT re-emit its CSS —
	// unless it is reached through a legacy @import, which duplicates the CSS.
	if m, ok := e.sharedLoaded[resolved]; ok {
		e.loaded[url] = m
		// A diamond dependency: this module is reused, but the current module
		// still depends on it, so downstream extends here must reach its rules.
		e.dependsOn(m.scope)
		e.noteLoadedCombine(m, false)
		e.reEmitImportedCSS(m, fr)
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
		// A plain-CSS module gets its own @extend scope so a downstream @extend
		// reaches its rules (dart treats plain CSS as a real module); its output
		// stays verbatim until an extend actually targets one of its selectors.
		e.registerPlainCSSExtendScope(nodes)
		mod := emptyModule()
		mod.cssNodes = nodes
		// A plain-CSS module's own CSS is its parsed nodes; it uses no modules,
		// so its combine node has those nodes as own CSS and no edges.
		mod.combine = &combineNode{own: nodes}
		e.loaded[url] = mod
		e.sharedLoaded[resolved] = mod
		e.noteLoadedCombine(mod, true)
		return mod
	}
	stmts, err := parseModuleSource(resolved, src)
	if err != nil {
		panic(err)
	}
	sub := newEvaluator(e.importer)
	sub.loadStack = append(append([]string(nil), e.loadStack...), resolved)
	sub.currentURL = resolved
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
	// A `!global` variable assignment creates a slot in the module's global scope
	// regardless of whether its enclosing code ever runs, so a module always
	// exposes the same members (dart pre-declares every global variable it can
	// assign). Pre-seed those slots to null; an executed assignment overwrites.
	sub.hoistGlobalVarSlots(stmts, sub.env, map[string]bool{})
	sub.runModule(stmts)
	e.warnings = append(e.warnings, sub.warnings...)
	e.loadedURLs = append(e.loadedURLs, resolved)
	// include used module CSS in our output, as a chunk that participates in
	// the importing stylesheet's top-level blank-line grouping.
	moduleNodes := sub.root.children()
	e.emitModuleCSS(moduleNodes, fr)
	// The module's exported API is its OWN public members (its global scope plus
	// its own function/mixin tables); anything it @forwards is reached through the
	// forwards chain by the read/write/enumeration accessors. A `@use ... as *`
	// this stylesheet performed lives in sub.env.globalModules and is deliberately
	// NOT part of the export, so it does not leak to a downstream consumer.
	mod := &module{
		vars:     sub.env.scopes[0],
		mixins:   publicMixins(sub.env.globalMixins()),
		funcs:    publicFuncs(sub.env.globalFuncs()),
		env:      sub.env,
		scope:    sub.scope,
		forwards: sub.forwarded,
		cssNodes: moduleNodes,
		combine:  sub.combine,
	}
	e.loaded[url] = mod
	e.sharedLoaded[resolved] = mod
	e.noteLoadedCombine(mod, true)
	return mod
}

// noteLoadedCombine records, for the top-level evalBody hook, the combine node
// of the module a @use/@forward just loaded and whether this was its first load
// in the whole compilation (dart's firstLoad, which gates the pre-module-comment
// sweep). A module whose combine node is nil (a built-in never reaches here)
// records a nil node, so the hook treats the load as contributing no CSS.
func (e *evaluator) noteLoadedCombine(m *module, firstLoad bool) {
	e.lastLoadedCombine = m.combine
	e.lastLoadFirst = firstLoad
}

// registerPlainCSSExtendScope gives a loaded plain-CSS module its own @extend
// module scope so a downstream @extend can reach the module's rules, exactly as
// dart-sass treats a plain-CSS file as a real module with its own extension
// store. The rules keep being emitted verbatim (raw); only a rule that a
// downstream @extend actually targets is re-serialised from its extended
// selector (writeBackSelectors flips it off `raw` once its box changes), so
// every untargeted plain-CSS rule's output is byte-for-byte unchanged. A rule
// whose verbatim selector does not round-trip through the Sass selector parser
// (an exotic plain-CSS selector) is left as pure raw and simply not extendable.
func (e *evaluator) registerPlainCSSExtendScope(nodes []cssNode) {
	sub := newEvaluator(e.importer)
	registerPlainRulesForExtend(sub, nodes)
	if len(sub.extendEvents) == 0 {
		return
	}
	e.adoptScope(sub)
	e.dependsOn(sub.scope)
}

// registerPlainRulesForExtend records each top-level plain-CSS style rule whose
// verbatim selector parses and round-trips as an extendable selector, so a
// downstream @extend can target it. Round-tripping guarantees the un-extended
// serialisation stays byte-identical to the original raw selector.
func registerPlainRulesForExtend(sub *evaluator, nodes []cssNode) {
	for _, n := range nodes {
		sr, ok := n.(*cssStyleRule)
		if !ok || !sr.raw {
			continue
		}
		list, err := parseSelectorListStrErr(sr.rawSel, false, true)
		if err != nil {
			continue
		}
		sl := selectorList{list: list}
		if sl.serialize(false) != sr.rawSel {
			continue
		}
		sr.selector = sl
		sr.original = sl
		sub.extendEvents = append(sub.extendEvents, extendEvent{rule: sr})
	}
}

// cloneCSSNodes deep-copies a slice of already-resolved CSS output nodes so a
// legacy @import can emit its own copy of an already-loaded module's CSS. The
// copy is independent of the original (fresh child slices), so re-nesting or a
// later blank-line regrouping of one copy never disturbs the other.
func cloneCSSNodes(in []cssNode) []cssNode {
	if in == nil {
		return nil
	}
	out := make([]cssNode, len(in))
	for i, n := range in {
		out[i] = cloneCSSNode(n)
	}
	return out
}

func cloneCSSNode(n cssNode) cssNode {
	switch v := n.(type) {
	case *cssStyleRule:
		c := *v
		c.nodes = cloneCSSNodes(v.nodes)
		// The clone is a fresh, independent rule: it must not share the original's
		// extension box (that box belongs to the module's own extend scope).
		c.box = nil
		return &c
	case *cssAtRule:
		c := *v
		c.nodes = cloneCSSNodes(v.nodes)
		return &c
	case *cssDeclaration:
		c := *v
		return &c
	default:
		// The only remaining output node type is a comment (cssRoot is never a
		// child). Copy it by value so the clone is independent.
		c := *n.(*cssComment)
		return &c
	}
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
	e.combineActive = true
	e.evalBody(stmts, fr, true)
}

// hoistGlobalVarSlots pre-declares (as null) every `!global` variable a module
// could assign, walking into all nested bodies so a declaration guarded by a
// never-taken branch still creates the slot. Only bare (non-namespaced) `!global`
// assignments hoist; a plain top-level `$x: v` gets its slot when it runs, and a
// namespaced `ns.$x: v` writes to another module.
func (e *evaluator) hoistGlobalVarSlots(stmts []Stmt, env *environment, seen map[string]bool) {
	for _, s := range stmts {
		switch n := s.(type) {
		case *VarDecl:
			if n.Global && n.Namespace == "" {
				env.setGlobalIfAbsent(n.Name, sassNull)
			}
		case *StyleRule:
			e.hoistGlobalVarSlots(n.Body, env, seen)
		case *Declaration:
			e.hoistGlobalVarSlots(n.Body, env, seen)
		case *MixinDef:
			e.hoistGlobalVarSlots(n.Body, env, seen)
		case *FunctionDef:
			e.hoistGlobalVarSlots(n.Body, env, seen)
		case *Include:
			e.hoistGlobalVarSlots(n.Content, env, seen)
		case *If:
			for _, c := range n.Clauses {
				e.hoistGlobalVarSlots(c.Body, env, seen)
			}
			e.hoistGlobalVarSlots(n.Else, env, seen)
		case *Each:
			e.hoistGlobalVarSlots(n.Body, env, seen)
		case *For:
			e.hoistGlobalVarSlots(n.Body, env, seen)
		case *While:
			e.hoistGlobalVarSlots(n.Body, env, seen)
		case *AtRoot:
			e.hoistGlobalVarSlots(n.Body, env, seen)
		case *Media:
			e.hoistGlobalVarSlots(n.Body, env, seen)
		case *Supports:
			e.hoistGlobalVarSlots(n.Body, env, seen)
		case *AtRule:
			e.hoistGlobalVarSlots(n.Body, env, seen)
		case *Import:
			// A legacy @import inlines the file into this module, so a `!global`
			// slot declared in the imported source belongs to this module too.
			e.hoistImportedGlobalVarSlots(n, env, seen)
		}
	}
}

// hoistImportedGlobalVarSlots resolves the sources of a legacy @import and hoists
// their `!global` variable slots into env, guarding against import cycles.
func (e *evaluator) hoistImportedGlobalVarSlots(n *Import, env *environment, seen map[string]bool) {
	for _, item := range n.Imports {
		if item.Plain {
			continue
		}
		// Pre-hoisting a legacy @import's global var slots: resolve exactly as the
		// real import will, honouring import-only precedence.
		src, resolved, ok := e.resolve(item.URL, true)
		if !ok || seen[resolved] || strings.HasSuffix(resolved, ".css") {
			continue
		}
		seen[resolved] = true
		stmts, err := parseModuleSource(resolved, src)
		if err != nil {
			continue
		}
		e.hoistGlobalVarSlots(stmts, env, seen)
	}
}

func (e *evaluator) resolve(url string, forImport bool) (string, string, bool) {
	if e.importer == nil {
		return "", "", false
	}
	return e.importer(url, e.currentURL, forImport)
}

// evalImportBody inlines a legacy @import's statements into the current frame.
// At a container (stylesheet root or an at-rule body) the imported statements
// are ordinary top-level nodes and take part in the container's blank-line
// grouping. Inlined into a style rule, they are rule-body content: nested rules
// are blank-separated from nothing (dart never blanks rule-body siblings), so
// only the block is reset before each nested block statement (dart splits the
// enclosing rule) without opening a blank-line group or a new variable scope —
// the import shares the importing scope.
func (e *evaluator) evalImportBody(stmts []Stmt, fr *frame) {
	if fr.atContainer {
		e.evalBody(stmts, fr, true)
		return
	}
	for _, s := range stmts {
		switch s.(type) {
		case *StyleRule, *Media, *Supports, *AtRoot, *AtRule:
			fr.block = nil
		}
		e.evalStmt(s, fr)
	}
}

func (e *evaluator) evalImport(n *Import, fr *frame) {
	for _, item := range n.Imports {
		if item.Plain {
			params := "\"" + item.URL + "\""
			if item.Raw != "" {
				// Preserve the original quote style of a plain-CSS passthrough import.
				params = item.Raw
			}
			if item.URLInterp != nil {
				// A url() prelude carrying interpolation: evaluate it so
				// `url("...#{$x}...")` resolves at compile time.
				params = e.resolveInterp(item.URLInterp)
			} else if len(item.URL) >= 4 && strings.EqualFold(item.URL[:4], "url(") {
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
		// A legacy @import prefers an import-only file (x.import.scss) of the same
		// name over the ordinary file, so ask the importer with forImport = true.
		src, resolved, ok := e.resolve(item.URL, true)
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
		// A @forward reached while inlining an @import contributes its re-exported
		// members to the importing scope (unlike a @forward run at a real module's
		// top level), so mark that we are inside an import.
		e.importDepth++
		// A `@use ... as *` inside the imported file joins the resolution scope only
		// while that file is being inlined; it is not re-exported to the importing
		// module (dart keeps such a transitive `as *` from leaking through @import),
		// so restore the global-module list once the import finishes.
		savedGlobals := e.env.globalModules
		// The inlined statements belong to the imported file, so a relative load
		// nested inside them (a further @import/@use/@forward, or a mixin's
		// meta.load-css) resolves relative to the imported file, not its importer.
		savedURL := e.currentURL
		e.currentURL = resolved
		if stmtsHaveForward(stmts) {
			saved := e.incomingConfig
			e.incomingConfig = e.implicitConfigSnapshot()
			e.evalImportBody(stmts, fr)
			e.incomingConfig = saved
		} else {
			e.evalImportBody(stmts, fr)
		}
		e.currentURL = savedURL
		e.env.globalModules = savedGlobals
		e.importDepth--
	}
}
