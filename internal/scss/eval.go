// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import (
	"fmt"
	"sort"
	"strings"
)

// Importer resolves an @use/@import/@forward URL to source text. referrer is the
// canonical URL of the stylesheet issuing the load — the file whose code the
// evaluator is currently running (a module's own URL for its top-level rules, or
// a mixin/content block's defining file for a dynamic load nested inside it).
// Mirroring dart-sass's Importer.canonicalize(url, baseUrl:), an importer resolves
// url relative to referrer first, then against its configured load paths. referrer
// is empty for a load issued by the entry stylesheet (which has no canonical URL).
//
// forImport is true only for a legacy @import; it mirrors dart-sass's
// canonicalize(url, forImport:) and asks the importer to prefer an import-only
// file (x.import.scss / _x.import.scss, or index.import.scss inside a directory)
// over the ordinary candidate of the same name. @use, @forward and meta.load-css
// pass false.
type Importer func(url, referrer string, forImport bool) (source string, resolvedURL string, ok bool)

type evaluator struct {
	env      *environment
	root     *cssRoot
	importer Importer
	// currentURL is the canonical URL of the stylesheet whose code is currently
	// executing in this evaluator. It is the referrer threaded to the importer so
	// relative loads resolve relative to the file that issued them: a module's own
	// URL while its top-level statements run, or a mixin/content block's DEFINING
	// file while its body runs (dart-sass resolves each load against its AST node's
	// span.sourceUrl). It is empty for the entry stylesheet.
	currentURL   string
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
	// importClones is the compilation-wide registry of per-@import CSS clones,
	// shared like allScopes. Each records a duplicated module's fresh extension
	// store, its duplicated rules, its source scope and its import subtree; the
	// finalize pass (composeImportClones) applies each import subtree's @extends to
	// its own duplicate, isolated from the canonical modules.
	importClones *[]*importClone
	// importSubtree, when non-nil, is the subtree collector of the legacy @import
	// currently inlining. It is threaded to every sub-evaluator spawned while the
	// import runs (adoptScope), so dependsOn — from the inlined content and from
	// any module loaded because of the import — records the whole clone subgraph.
	// A dependency created while it is set is import-only: it feeds the per-clone
	// composition instead of carrying the import's extends into the canonical graph.
	importSubtree *importSubtreeCtx
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
	// importDepth counts how many nested legacy @import inlinings are currently on
	// the stack. A @forward evaluated while this is >0 additionally merges its
	// re-exported members into the importing scope (dart's import-forward
	// behaviour); at depth 0 a @forward only records an export.
	importDepth int
	// combine is this evaluator's module node in the deferred combine tree: its
	// own top-level CSS and, in @use/@forward order, the modules it loads (each
	// carrying the pre-module comments captured before it). See combine.go.
	combine *combineNode
	// lastLoadedCombine / lastLoadFirst carry, from loadModule back to the
	// top-level evalBody hook, the combine node of the module a @use/@forward
	// statement just loaded and whether that was its first load in the whole
	// compilation (dart's firstLoad). Reset before each top-level statement.
	lastLoadedCombine *combineNode
	lastLoadFirst     bool
	// combineActive is true only for a module's OUTERMOST top-level statement
	// loop (set by run/runModule). Nested evalBody re-entries — a mixin body, a
	// control-flow body or an inlined @import that happens to target the root
	// container — must not re-record combine nodes, or the same CSS would be
	// counted at both the inner statement and the outer @include/@import.
	combineActive bool
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
	// ruleChain lists the style-rule nodes lexically enclosing this frame, in the
	// order they nest. Because the CSS tree is flat (nesting is carried in the
	// compound selector), all of them are direct children of the same output
	// container. It lets a hoisted @at-root body tell whether the node it lands
	// after is one of its own ancestor rules — in which case the first hoisted
	// node continues that rule with no blank line — versus an unrelated sibling.
	ruleChain []cssNode
	// braceLine is the 1-based source line of the enclosing style rule's opening
	// `{` (0 at the top level or in the indented syntax). Every block segment
	// this frame materialises for its direct declarations (including the fresh
	// copies split around nested rules) carries it, so the serializer can decide
	// whether a first-child loud comment trails the opening brace.
	braceLine int
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
	e.combine = &combineNode{}
	scopes := []*moduleScope{e.scope}
	e.allScopes = &scopes
	clones := []*importClone{}
	e.importClones = &clones
	return e
}

// importClone is a legacy-@import CSS duplicate: the duplicated style rules (for
// writing extended selectors back after composition), each paired with the
// ORIGINAL rule it was cloned from (origins) so composition can attribute it to
// its owning module, the source module scope whose CSS was duplicated, and the
// import subtree whose @extends compose onto it.
//
// dart-sass's _combineCss(clone: true) clones the whole transitively-loaded
// subtree of a legacy @import and resolves @extend among the clones with dart's
// _extendModules — each cloned module keeps its OWN ExtensionStore, isolated from
// the canonical modules AND from its sibling clone modules. go-scss reproduces
// that in composeImportClones: the clone rules are partitioned per owning module
// into per-module clone stores, seeded with each module's own extends, then
// composed downstream-first over the subtree's module graph, so a sibling's
// @extend never leaks across a diamond. The origins pairing (rules[i] cloned from
// origins[i]) is what lets composition recover each clone rule's module.
type importClone struct {
	rules   []*cssStyleRule
	origins []*cssStyleRule
	source  *moduleScope
	subtree *importSubtreeCtx
}

// importSubtreeCtx collects, for one outermost legacy @import, every module scope
// reached while it inlines (the site scope plus every module @used/@imported
// transitively during the inlining) and the dependency edges recorded among them.
// All CSS duplicates produced by that @import share it, and composition resolves
// @extend across every clone of the subtree together, over this module graph.
type importSubtreeCtx struct {
	scopes []*moduleScope
	seen   map[*moduleScope]bool
	edges  []subtreeEdge
}

// subtreeEdge records that, while a legacy @import inlined, module `down`
// depended on module `up` (a @use/@forward/load reached during the import).
// Composition treats `down` as downstream of `up`, so `down`'s clone extends
// propagate onto `up`'s clone rules — the import-site half of the clone module
// graph that a module's canonical upstream edges do not carry.
type subtreeEdge struct{ down, up *moduleScope }

func (c *importSubtreeCtx) add(m *moduleScope) {
	if m == nil || c.seen[m] {
		return
	}
	c.seen[m] = true
	c.scopes = append(c.scopes, m)
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
	// ownStore caches this module's own extend store (buildOwnStore) so it can be
	// built once and reused: applyAllExtends pass 1 seeds `store` from it, and a
	// legacy @import that re-emits this module's CSS clones it for the duplicate
	// (cloneStoreFor). Building it early for a re-imported module is invisible to
	// evaluation — a rule's box is read only by the extend engine at finalize.
	ownStore *extensionStore
	visited  bool
}

// adoptScope wires a freshly spawned sub-evaluator into this compilation's
// shared scope registry so its rules take part in the global extend finalize.
func (e *evaluator) adoptScope(sub *evaluator) {
	sub.allScopes = e.allScopes
	sub.importClones = e.importClones
	sub.importSubtree = e.importSubtree
	*e.allScopes = append(*e.allScopes, sub.scope)
}

// dependsOn records that this evaluator's module @uses/@forwards/loads another
// module, so downstream extends written here reach that upstream module's rules.
//
// While a legacy @import inlines (importSubtree set), the dependency is instead
// recorded into the import's clone subgraph and NOT added to the canonical
// upstream edges: dart resolves an @import's @extends within a clone of its
// subtree, isolated from the canonically-reached modules, so the import's extends
// must not propagate into a module that is also reached by a plain @use chain
// (composeImportClones applies them to the duplicated CSS instead).
func (e *evaluator) dependsOn(up *moduleScope) {
	if up == nil || up == e.scope {
		return
	}
	if e.importSubtree != nil {
		e.importSubtree.add(e.scope)
		e.importSubtree.add(up)
		e.importSubtree.edges = append(e.importSubtree.edges, subtreeEdge{down: e.scope, up: up})
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
	e.combineActive = true
	e.evalBody(stmts, fr, true)
	e.applyAllExtends()
	// Reassemble the top-level output from the deferred combine tree (dart's
	// _combineCss): per-module @import regions and pre-@use comments are
	// interleaved across the module graph — an ordering the inline-emitted flat
	// tree cannot express. The extends applied above mutate the same rule nodes
	// the combine tree references, so their results carry through.
	e.root.nodes = combineCss(e.combine)
	combineTopLevelGroups(e.root)
}

// isCSSImport reports whether a node is a plain-CSS @import at-rule.
func isCSSImport(n cssNode) bool {
	a, ok := n.(*cssAtRule)
	return ok && a.name == "import"
}

// isImportRegionNode reports whether a node may appear inside the leading import
// region: a CSS @import rule or a comment (dart lets comments interleave imports).
func isImportRegionNode(n cssNode) bool {
	if isCSSImport(n) {
		return true
	}
	_, ok := n.(*cssComment)
	return ok
}

func (e *evaluator) evalBody(stmts []Stmt, fr *frame, containerBody bool) {
	// A module's OWN top level is the OUTERMOST statement loop whose container is
	// this evaluator's root (not a nested @media/@at-root/mixin/@import body).
	// Only there do statements contribute to the deferred combine tree: their
	// emitted nodes are this module's own CSS, and a @use/@forward is a combine
	// edge to a used module. combineActive is cleared for the duration of this
	// call so nested re-entries don't re-record the same nodes.
	topLevel := e.combineActive && containerBody && fr.container == e.root
	e.combineActive = false
	for _, s := range stmts {
		if containerBody {
			fr.group.pending = true
			fr.group.curIsStyleRule = isStyleRuleStmt(s)
			fr.block = nil
		} else if isNestedRuleStmt(s) {
			// Inside a style rule (mixin / @content / control-flow body), a nested
			// rule or bubbling at-rule closes the current block: the declarations
			// that follow it must land in a fresh copy of the enclosing selector,
			// preserving source order — exactly as evalRuleBody does for a rule's
			// own direct children. dart-sass splits the parent this way whether the
			// nested rule is written inline or emitted from an @include.
			fr.block = nil
		}
		var before int
		if topLevel {
			before = len(e.root.nodes)
			e.lastLoadedCombine = nil
		}
		e.evalStmt(s, fr)
		if topLevel {
			e.recordCombineStmt(s, before)
		}
		if containerBody && isStyleRuleStmt(s) {
			// dart marks _parent.children.last isGroupEnd after a top-level style
			// rule completes (when not lexically inside a style rule). Recording it
			// here — at the single container-body chokepoint, in every evaluator —
			// lets combineTopLevelGroups reconstruct dart's blank-line separation
			// over the final combined module tree.
			markGroupEnd(fr.container)
		}
	}
}

// recordCombineStmt folds one top-level statement's effect into this module's
// combine node. A @use/@forward that loaded a real (CSS-bearing) module becomes
// an edge to that module — its inline-emitted CSS (root.nodes since before) is
// deliberately dropped from this module's own CSS, since combineCss reproduces
// it from the edge. Every other statement's emitted nodes are this module's own
// top-level CSS. A @use of a built-in module (sass:math, …) loads no combine
// node and emits nothing, so it is ignored.
func (e *evaluator) recordCombineStmt(s Stmt, before int) {
	switch s.(type) {
	case *Use, *Forward:
		if e.lastLoadedCombine != nil {
			e.combine.recordUse(e.lastLoadedCombine, e.lastLoadFirst)
			return
		}
	}
	added := e.root.nodes[before:]
	if len(added) > 0 {
		e.combine.recordOwn(append([]cssNode(nil), added...))
	}
}

// isNestedRuleStmt reports whether a statement, when it appears inside a style
// rule, produces a nested rule or an at-rule that bubbles above the current
// block — the statements after which trailing declarations need a fresh block.
func isNestedRuleStmt(s Stmt) bool {
	switch v := s.(type) {
	case *StyleRule, *Media, *Supports, *AtRoot:
		return true
	case *AtRule:
		// Only an at-rule with a block bubbles and closes the enclosing block; a
		// childless at-rule (`@apply x;`) stays inline like a declaration, so it
		// must not force the following siblings into a fresh parent block.
		return !v.NoBody
	}
	return false
}

func isStyleRuleStmt(s Stmt) bool {
	_, ok := s.(*StyleRule)
	return ok
}

// hoistGroup builds the initial blank-line group state for an @at-root body that
// is hoisted into an already-populated container. dart separates the first
// hoisted node from a preceding top-level style rule with a blank line — EXCEPT
// when that preceding node is the very rule the body was hoisted out of, which
// the first hoisted node continues without a blank (`foo { color: blue; @at-root
// bar { … } }` emits `foo` then `bar` gap-free, but a second, independent hoist
// after `bar` is blank-separated). A body landing in an empty container, or one
// whose origin rule is the last visible node, therefore starts a fresh group; a
// body landing after any other node inherits that node's group-end state so the
// normal style-rule separation applies.
func hoistGroup(target cssContainer, chain []cssNode) *groupInfo {
	ch := target.children()
	var last cssNode
	for i := len(ch) - 1; i >= 0; i-- {
		if !isEmptyContainer(ch[i]) {
			last = ch[i]
			break
		}
	}
	if last == nil {
		return &groupInfo{}
	}
	for _, a := range chain {
		if a == last {
			return &groupInfo{}
		}
	}
	_, isSR := last.(*cssStyleRule)
	return &groupInfo{any: true, prevWasStyleRule: isSR}
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
		e.env.defineMixin(n.Name, &mixinEntry{def: n, env: e.env, defDepth: len(e.env.scopes), srcURL: e.currentURL})
	case *FunctionDef:
		e.env.defineFunc(n.Name, &funcEntry{def: n, env: e.env, defDepth: len(e.env.scopes)})
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
		// A top-level `!default` declaration is the point at which a module's
		// incoming `with (...)` configuration is applied: a configured value
		// overrides the default, and the variable comes into existence here (not
		// before), matching dart-sass's Configuration consumption.
		if len(e.env.scopes) == 1 && e.incomingConfig != nil {
			if cv, ok := e.incomingConfig[normIdent(n.Name)]; ok {
				delete(e.incomingConfig, normIdent(n.Name))
				// A null configured value is consumed but leaves the variable
				// unset, so the `!default` clause falls through to its own value.
				if _, isNull := cv.(*Null); !isNull {
					e.env.setVar(n.Name, cv, n.Global)
					return
				}
			}
		}
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
	if _, exists := mod.getVar(name); !exists {
		e.fail("Undefined variable.")
	}
	if n.Default {
		if v, ok := mod.getVar(name); ok {
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
		// The raw token stream is reproduced verbatim; whitespace was already
		// folded at parse time and any re-indentation happens at serialization.
		raw := e.resolveInterp(n.RawValue)
		e.addDecl(fr, &cssDeclaration{name: name, raw: raw, custom: true, nameCol: n.NameCol, endLine: n.EndLine})
		return
	}
	if n.Value != nil {
		val := e.evalExpr(n.Value)
		if !isBlankValue(val) {
			e.addDecl(fr, &cssDeclaration{name: name, value: val, endLine: n.EndLine})
		}
	}
	// Nested properties use a "name-" prefix that applies to every declaration
	// emitted in the block, including those produced by @include or @content.
	// The body evaluates against this same frame (only the prefix is swapped, then
	// restored) so a trailing style-rule block it opens is reused by the following
	// sibling declarations — otherwise a copy of the frame would hold that block
	// and each later declaration would open a fresh, redundant parent block.
	if len(n.Body) > 0 {
		saved := fr.declPrefix
		fr.declPrefix = name + "-"
		for _, sub := range n.Body {
			e.evalStmt(sub, fr)
		}
		fr.declPrefix = saved
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
		fr.block = &cssStyleRule{selector: fr.parentSel, original: fr.parentSel, mediaContext: mediaContextOf(fr), braceLine: fr.braceLine}
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
	rule := &cssStyleRule{selector: resolved, original: resolved, mediaContext: mediaContextOf(fr), braceLine: n.BraceLine, closeLine: n.CloseBraceLine}
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
		ruleChain:     append(append([]cssNode(nil), fr.ruleChain...), rule),
		braceLine:     n.BraceLine,
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
		if isNestedRuleStmt(s) {
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
	pos, named, restSep := e.evalArgs(n.Args)
	e.invokeMixin(m, pos, named, restSep, n.Content, n.ContentParams, callEnv, e.currentURL, fr)
}

func (e *evaluator) evalContent(n *ContentStmt, fr *frame) {
	content := e.env.content
	contentEnv := e.env.contentEnv
	contentParams := e.env.contentArgs
	contentURL := e.env.contentURL
	if content == nil {
		return
	}
	// Arguments to `@content(...)` are evaluated in the mixin's environment (the
	// current scope), exactly as any other call's arguments are. The content
	// block itself then runs in the caller's environment (contentEnv), with those
	// arguments bound to the block's `using (...)` parameters — dart-sass's
	// ContentBlock closure: parameters are declared at the include site while the
	// body executes in the lexical scope where the block was written.
	pos, named, restSep := e.evalArgs(n.Args)
	e.enter()
	defer e.leave()
	saved := e.env
	// The content block's statements belong to the include site's file, so a
	// dynamic load inside them resolves relative to that file, not the mixin's.
	savedURL := e.currentURL
	e.currentURL = contentURL
	e.env = contentEnv
	e.env.pushScope()
	func() {
		defer func() {
			e.env.popScope()
			e.env = saved
			e.currentURL = savedURL
		}()
		e.bindResolved(contentParams, pos, named, restSep)
		e.evalBody(content, fr, fr.atContainer)
	}()
}

func (e *evaluator) lookupMixin(ns, name string) *mixinEntry {
	name = normIdent(name)
	if ns != "" {
		if mod, ok := e.env.modules[ns]; ok {
			if m, ok := mod.getMixin(name); ok {
				return m
			}
		}
		return nil
	}
	if m, ok := e.env.getMixin(name); ok {
		return m
	}
	if m, ok := e.env.globalModuleMixin(name); ok {
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
		// Binding an @each variable clears a scalar slash number's provenance,
		// exactly as a variable declaration does (dart-sass applies withoutSlash
		// when it resolves the loop variable): iterating `a 3/4 b` yields `0.75`
		// for the middle element, while a nested list keeps its inner slash.
		if len(n.Vars) == 1 {
			e.env.defineVar(n.Vars[0], numWithoutSlash(item))
		} else {
			parts := destructure(item, len(n.Vars))
			for i, v := range n.Vars {
				e.env.defineVar(v, numWithoutSlash(parts[i]))
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

	at := &cssAtRule{name: "media", params: mediaQueriesString(effective), hasBody: true, braceLine: n.BraceLine}
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
		ruleChain:       fr.ruleChain,
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
		ruleChain:     fr.ruleChain,
	}
	e.evalBody(n.Body, child, !fr.hasParent)
}

func (e *evaluator) evalAtRoot(n *AtRoot, fr *frame) {
	// @at-root introduces a nested, opaque variable scope, exactly like a style
	// rule (dart-sass wraps the body in Environment.scope() without semiGlobal).
	// Consequences the suite pins: an implicit (non-!global) assignment to a
	// variable that exists only at the global scope creates a scope-local shadow
	// rather than mutating the global, and any variable/mixin/function declared
	// inside the body is local to it and does not persist once the body closes.
	e.env.pushScope()
	defer e.env.popScope()

	// An @at-root query controls which enclosing frames the body escapes. The
	// default (no query) is `(without: rule)`: climb out of the style rules but
	// STAY within any @media/@supports frame, so `@media screen { .foo { @at-root
	// .bar { … } } }` keeps its media wrapper. `(with: …)` names the frames to
	// KEEP (all others excluded); `(without: …)` names the frames to EXCLUDE (all
	// others kept); `all` matches every frame; `rule` matches style rules.
	include, names := false, map[string]bool{"rule": true}
	if n.Query != nil {
		var ok bool
		if include, names, ok = parseAtRootQuery(e.resolveInterp(n.Query)); !ok {
			e.fail(`Expected "with" or "without".`)
		}
	}
	all := names["all"]
	excludes := func(name string) bool { return (all || names[name]) != include }
	excludesRule := (all || names["rule"]) != include

	// fr.container is the innermost enclosing at-rule node the body sits in — a
	// @media/@supports, a @keyframes, @font-face or any unknown at-rule (style
	// rules never open a container, they open a block). If the query excludes
	// that frame, the body escapes it and lands at the document root; otherwise
	// the frame is kept. This single-frame reconstruction keeps no partial
	// ancestry above an escaped at-rule, which covers every query form the suite
	// exercises.
	frameName := ""
	if ar, ok := fr.container.(*cssAtRule); ok {
		frameName = ar.name
	}
	if frameName != "" && excludes(frameName) {
		child := &frame{
			container:     e.root,
			rootContainer: e.root,
			mediaParent:   e.root,
			atContainer:   true,
			group:         hoistGroup(e.root, fr.ruleChain),
		}
		if !excludesRule {
			// The enclosing style rule is kept, so its selector is re-materialised
			// at the root and the body nests inside it normally.
			child.parentSel = fr.parentSel
			child.hasParent = fr.hasParent
		}
		e.evalBody(n.Body, child, true)
		return
	}

	// The media/supports frame is kept. Style rules are escaped iff the query
	// excludes them (the default); otherwise the body continues in the current
	// rule context.
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
		atRoot:      excludesRule,
		atContainer: true,
		group:       hoistGroup(fr.container, fr.ruleChain),
	}
	e.evalBody(n.Body, child, true)
}

// stripCSSComments removes `/* … */` and `// …` comments, replacing each with a
// single space so tokens on either side stay separated. It is used to normalise
// an @at-root query, whose grammar admits comments as whitespace, before the
// keyword/name split.
func stripCSSComments(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if i+1 < len(s) && s[i] == '/' && s[i+1] == '*' {
			j := strings.Index(s[i+2:], "*/")
			if j < 0 {
				b.WriteByte(' ')
				break
			}
			b.WriteByte(' ')
			i += 2 + j + 2
			continue
		}
		if i+1 < len(s) && s[i] == '/' && s[i+1] == '/' {
			nl := strings.IndexByte(s[i:], '\n')
			b.WriteByte(' ')
			if nl < 0 {
				break
			}
			i += nl // keep the newline itself
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// parseAtRootQuery interprets an @at-root query such as `(without: media)` or
// `(with: rule keyframes)`, returning whether the listed names form an include
// (`with:`) list, the set of lower-cased names, and whether the query is
// well-formed. dart requires a `with:`/`without:` keyword followed by at least
// one name; anything else is a compile error, so ok is false for a missing
// keyword or an empty name list.
func parseAtRootQuery(q string) (include bool, names map[string]bool, ok bool) {
	names = map[string]bool{}
	q = stripCSSComments(q)
	q = strings.TrimSpace(q)
	q = strings.TrimPrefix(q, "(")
	q = strings.TrimSuffix(q, ")")
	colon := strings.IndexByte(q, ':')
	if colon < 0 {
		return false, names, false
	}
	key := strings.ToLower(strings.TrimSpace(q[:colon]))
	if key != "with" && key != "without" {
		return false, names, false
	}
	include = key == "with"
	for _, f := range strings.Fields(q[colon+1:]) {
		names[strings.ToLower(f)] = true
	}
	if len(names) == 0 {
		return false, names, false
	}
	return include, names, true
}

// --- generic at-rules ---

func (e *evaluator) evalGenericAtRule(n *AtRule, fr *frame) {
	name := n.Name
	if n.NameInterp != nil {
		name = e.resolveInterp(n.NameInterp)
	}
	params := ""
	if n.Value != nil {
		params = e.resolveInterp(n.Value)
	}
	at := &cssAtRule{name: name, params: params, hasBody: !n.NoBody, braceLine: n.BraceLine, closeLine: n.CloseBraceLine}
	if n.NoBody {
		// A childless at-rule (`@b c;`) behaves like a declaration: inside a
		// style rule it stays within the enclosing selector's block, interleaving
		// with declarations rather than bubbling to the root.
		if fr.hasParent {
			e.ensureBlock(fr).appendNode(at)
		} else {
			at.blankBefore = e.consumeGroup(fr)
			e.liveContainer(fr).appendNode(at)
		}
		return
	}
	at.blankBefore = e.consumeGroup(fr)
	e.liveContainer(fr).appendNode(at)
	keyframes := isKeyframesAtRule(name)
	// A @keyframes body holds keyframe blocks, but a stray declaration written
	// directly in it (dart tolerates this) is emitted verbatim rather than being
	// wrapped in an empty style rule, so treat the body as declaration-direct.
	direct := isDeclarationAtRule(name) || keyframes
	// Inside a style rule, an unknown at-rule with a block re-materialises the
	// enclosing selector around its declarations (dart: `div { @foo { a: b } }`
	// -> `@foo { div { a: b } }`), so it carries the parent selector. At the top
	// level (no enclosing selector) its declarations stay direct. Under an
	// enclosing @at-root the parent style rule is suppressed, so the at-rule's
	// declarations also stay direct and the suppression carries through (dart:
	// `p { @at-root { @foo { a: b } } }` -> `@foo { a: b }`, not `@foo { p { a: b } }`).
	child := &frame{
		container:     at,
		rootContainer: at,
		mediaParent:   at,
		parentSel:     fr.parentSel,
		hasParent:     fr.hasParent,
		directDecls:   direct || !fr.hasParent || fr.atRoot,
		atRoot:        fr.atRoot,
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
	// The indented syntax doesn't require a closing `*/`; dart-sass appends one
	// when the parsed comment body doesn't already end with it.
	if !strings.HasSuffix(text, "*/") {
		text += " */"
	}
	c := &cssComment{text: text, col: n.Col, line: n.Line}
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
		// An @extend nested in a bubbling frame (a @media/@supports directly
		// inside a style rule) still runs within that enclosing rule: it applies
		// to the rule re-materialised in the current media context. Only a truly
		// rule-less position — the top level, or inside a media at the top level —
		// is an error. dart raises "@extend may only be used within style rules."
		if fr.hasParent && !fr.parentSel.isEmpty() {
			e.ensureBlock(fr)
		} else {
			e.fail("@extend may only be used within style rules.")
		}
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
	// own @extends), exactly as a standalone stylesheet would. A module reached
	// by a legacy @import may already have had its own store built (and cached)
	// when the duplicate CSS was re-emitted; reuse it so buildOwnStore — which
	// stamps each rule's box — runs exactly once per module.
	for _, m := range *e.allScopes {
		if m.ownStore == nil {
			m.ownStore = m.ev.buildOwnStore()
		}
		m.store = m.ownStore
	}
	// Snapshot every module's pristine own-store BEFORE pass 2 mutates it (pass 2
	// aliases m.store = m.ownStore and merges downstream extends into it). The
	// per-@import clones compose against these snapshots so a clone gets each
	// subtree module's OWN extends without the canonical cross-module extends the
	// original accumulates. Built only when there is a clone to compose.
	var pristine map[*moduleScope]*extensionStore
	order := e.scopeFinalizeOrder()
	if len(*e.importClones) != 0 {
		pristine = make(map[*moduleScope]*extensionStore, len(*e.allScopes))
		for _, m := range *e.allScopes {
			pristine[m] = m.ownStore.clone()
		}
	}
	// Pass 2: finalise downstream-first. Merging a module's (already downstream-
	// enriched) store into each module it depends on carries extends transitively
	// upstream while keeping sibling modules isolated — a downstream extend only
	// re-extends the upstream module's own registered selectors, never selectors
	// introduced by a different downstream module.
	for _, m := range order {
		if len(m.downstreamStores) != 0 {
			m.store.addExtensions(m.downstreamStores)
		}
		m.ev.writeBackSelectors()
		for _, up := range m.upstream {
			up.downstreamStores = append(up.downstreamStores, m.store)
		}
	}
	// Pass 3 (step 4): compose each legacy-@import CSS duplicate independently from
	// its own subtree's extends, replacing the step-3 behaviour-preserving mirror.
	if pristine != nil {
		e.composeImportClones(pristine, order)
	}
}

// composeImportClones resolves @extend across every legacy-@import CSS duplicate,
// reproducing dart-sass _combineCss(clone: true) + _extendModules over the cloned
// subgraph. All duplicates produced by one @import share an import subtree; they
// are composed together, PER MODULE: each clone rule is attributed to its owning
// module (via the origins pairing) and registered in a per-(subtree, module) clone
// store, then those stores are composed downstream-first over the subtree's module
// graph exactly as the canonical pass 2 composes the real per-module stores. A
// module's own extends are seeded from its PRISTINE own-store, and a downstream
// module's extends propagate only onto the modules it depends on — so a sibling in
// a diamond never leaks its extender across, matching the canonical isolation.
func (e *evaluator) composeImportClones(pristine map[*moduleScope]*extensionStore, order []*moduleScope) {
	rank := make(map[*moduleScope]int, len(order))
	for i, m := range order {
		rank[m] = i
	}
	// Attribute every tracked rule to its owning module (a module's own rules are
	// its evaluator's rule events; a rule inlined from a used module stays with
	// THAT module, never the one whose CSS re-emitted it).
	ruleModule := map[*cssStyleRule]*moduleScope{}
	for _, m := range *e.allScopes {
		for _, ev := range m.ev.extendEvents {
			if ev.rule != nil {
				ruleModule[ev.rule] = m
			}
		}
	}
	// Group the duplicates by their shared @import subtree, preserving first-seen
	// order for determinism, then compose each subtree's clones together.
	bySubtree := map[*importSubtreeCtx][]*importClone{}
	var subtrees []*importSubtreeCtx
	for _, ic := range *e.importClones {
		if ic.subtree == nil {
			writeBackCloneSelectors(ic.rules)
			continue
		}
		if _, ok := bySubtree[ic.subtree]; !ok {
			subtrees = append(subtrees, ic.subtree)
		}
		bySubtree[ic.subtree] = append(bySubtree[ic.subtree], ic)
	}
	for _, st := range subtrees {
		composeCloneSubtree(bySubtree[st], st, ruleModule, pristine, rank)
	}
}

// composeCloneSubtree composes one @import's CSS duplicates as a cloned module
// graph. It builds a per-module clone store from the duplicated rules, seeds each
// with the module's own pristine extends, and propagates downstream-first over the
// combined edge set (the modules' canonical upstream edges plus the import-site
// dependency edges recorded in the subtree). This is the clone-side mirror of the
// canonical applyAllExtends pass 2.
func composeCloneSubtree(clones []*importClone, st *importSubtreeCtx, ruleModule map[*cssStyleRule]*moduleScope, pristine map[*moduleScope]*extensionStore, rank map[*moduleScope]int) {
	// Per-module clone stores: register each duplicated rule under the module that
	// owns its original. Modules with rules in several duplicates of this subtree
	// (a diamond re-emitted more than once) share one store, so their extends
	// resolve together exactly as dart's single cloned module does.
	stores := map[*moduleScope]*extensionStore{}
	var mods []*moduleScope
	store := func(m *moduleScope) *extensionStore {
		s := stores[m]
		if s == nil {
			s = newExtensionStore(extendNormal)
			stores[m] = s
			mods = append(mods, m)
		}
		return s
	}
	for _, ic := range clones {
		for i, r := range ic.rules {
			m := ruleModule[ic.origins[i]]
			if m == nil {
				m = ic.source
			}
			registerCloneRule(r, store(m))
		}
	}
	// A module reached during the import that contributed extends but no cloned
	// rule (e.g. the importing stylesheet's own @extend written mid-@import) still
	// takes part: give it an empty store so its extends propagate onto the clones.
	for _, m := range st.scopes {
		store(m)
	}
	// Seed each module's own extends from its pristine own-store, mirroring the
	// canonical pass 1 (own selectors extended by own extends) on the clones.
	for _, m := range mods {
		if ps := pristine[m]; ps != nil {
			stores[m].addExtensions([]*extensionStore{ps})
		}
	}
	// Combined upstream edges: a module's canonical upstream plus the import-site
	// dependency edges recorded while inlining. ups[m] lists the modules m depends
	// on (its clone store propagates onto each), restricted to this subtree.
	ups := map[*moduleScope][]*moduleScope{}
	seenEdge := map[[2]*moduleScope]bool{}
	// down is always a subtree module (an edge source is either a module with a
	// clone store or an st.scopes member, both given a store above); up may point
	// outside the subtree (a module's canonical upstream not reached here), so only
	// up is filtered against the store set.
	addEdge := func(down, up *moduleScope) {
		if _, ok := stores[up]; !ok {
			return
		}
		if down == up || seenEdge[[2]*moduleScope{down, up}] {
			return
		}
		seenEdge[[2]*moduleScope{down, up}] = true
		ups[down] = append(ups[down], up)
	}
	for _, m := range mods {
		for _, up := range m.upstream {
			addEdge(m, up)
		}
	}
	for _, ed := range st.edges {
		addEdge(ed.down, ed.up)
	}
	// Downstream-first order over the COMBINED edges (not the canonical rank, which
	// omits the import-site edges): a post-order DFS over ups yields upstream-first,
	// reversed to downstream-first. The subtree is a DAG (Sass forbids loops), and
	// modules are visited in canonical-rank order first so the result is stable.
	mods = subtreeFinalizeOrder(mods, ups, rank)
	// Propagate downstream-first: by the time an upstream module is processed,
	// every downstream module's clone store (own + its own downstream extends) has
	// been merged in, so transitive extends resolve while siblings stay isolated.
	merged := map[*moduleScope][]*extensionStore{}
	for _, m := range mods {
		if ds := merged[m]; len(ds) != 0 {
			stores[m].addExtensions(ds)
		}
		for _, up := range ups[m] {
			merged[up] = append(merged[up], stores[m])
		}
	}
	for _, ic := range clones {
		writeBackCloneSelectors(ic.rules)
	}
}

// subtreeFinalizeOrder returns the subtree's modules in downstream-first order
// over the combined edge set ups (a module before every module it depends on): a
// post-order DFS over ups yields upstream-first, which is reversed. Roots are
// visited in canonical-rank order so the result is deterministic.
func subtreeFinalizeOrder(mods []*moduleScope, ups map[*moduleScope][]*moduleScope, rank map[*moduleScope]int) []*moduleScope {
	roots := append([]*moduleScope(nil), mods...)
	sortScopesByRank(roots, rank)
	visited := map[*moduleScope]bool{}
	var post []*moduleScope
	var visit func(*moduleScope)
	visit = func(m *moduleScope) {
		if visited[m] {
			return
		}
		visited[m] = true
		for _, up := range ups[m] {
			visit(up)
		}
		post = append(post, m)
	}
	for _, m := range roots {
		visit(m)
	}
	for i, j := 0, len(post)-1; i < j; i, j = i+1, j-1 {
		post[i], post[j] = post[j], post[i]
	}
	return post
}

// registerCloneRule registers one duplicated style rule under a fresh extension
// box in store and marks its selectors as originals (so the extend engine's
// trimming preserves them exactly as a standalone stylesheet's own selectors).
func registerCloneRule(v *cssStyleRule, store *extensionStore) {
	v.box = &box{value: v.selector.list}
	store.registerSelector(v.selector.list, v.box)
	if !v.selector.list.isInvisible() {
		for _, c := range v.selector.list.components {
			store.originals[complexKey(c)] = true
		}
	}
}

// sortScopesByRank orders subtree scopes into the compilation's downstream-first
// finalize order so a clone's composition applies its subtree extends in the same
// order the canonical pass 2 does (stable for scopes sharing a rank, which the
// pre-assigned ranks make total).
func sortScopesByRank(scopes []*moduleScope, rank map[*moduleScope]int) {
	sort.SliceStable(scopes, func(i, j int) bool {
		return rank[scopes[i]] < rank[scopes[j]]
	})
}

// writeBackCloneSelectors copies each duplicated rule's composed selector box back
// onto the rule for serialization, matching writeBackSelectors for the canonical
// rules. A raw plain-CSS duplicate is re-serialised from its extended selector
// only once an @extend actually changed it.
func writeBackCloneSelectors(rules []*cssStyleRule) {
	for _, r := range rules {
		if r.box == nil {
			continue
		}
		if r.raw && r.original.list != nil && r.box.value != r.original.list {
			r.raw = false
		}
		r.selector = selectorList{list: r.box.value}
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
			// A plain-CSS rule is emitted verbatim (raw) unless a downstream
			// @extend actually changed its selector box, in which case it must be
			// re-serialised from the extended selector — flip it off `raw`. An
			// untargeted plain-CSS rule keeps its byte-identical verbatim output.
			if ev.rule.raw && ev.rule.box.value != ev.rule.original.list {
				ev.rule.raw = false
			}
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
	// final=false: a SassString interpolated directly contributes its raw text,
	// so a newline survives to be re-escaped by an enclosing quoted string (dart
	// writes the string's `.text` verbatim). A string reached through a list or
	// map is instead collapsed by serializeList/serializeMap, which serialize
	// their elements with final=true.
	return serializeValueQ(v, false, false, false)
}
