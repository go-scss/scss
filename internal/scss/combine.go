// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

// This file implements dart-sass's deferred module-CSS combination
// (async_evaluate.dart `_combineCss` / `_indexAfterImports` and
// `_registerCommentsForModule`).
//
// dart never emits a @used module's CSS at the @use site. Each module keeps
// only its OWN top-level CSS; the entry stylesheet combines the whole module
// graph once, at the end, in reverse topological order. Two orderings fall out
// of that combine that a naive "emit each module inline, then hoist imports
// globally" model cannot reproduce:
//
//   - Per-module import regions. Each module contributes its own leading run of
//     plain-CSS @imports (and the comments interleaved among them) to a shared
//     import block at the very top, while the comment that sits between a
//     module's last @import and its first rule stays down with that module's
//     CSS. A single global "pull every @import to the top" pass fuses these.
//
//   - Pre-module comments. A top-level loud comment written before a `@use`
//     (or `@forward`) of a module that transitively contains CSS is hoisted to
//     sit just above that module's combined CSS — ahead of the modules IT uses.
//     dart moves these comments out of the current stylesheet into a per-module
//     `preModuleComments` bucket at the moment the module is first loaded
//     (`_registerCommentsForModule`), but only when the loaded module actually
//     contains CSS; otherwise the comments stay as the current module's own CSS.
//
// go-scss still emits module CSS inline while evaluating (that arrangement feeds
// the @extend machinery and is discarded here), but alongside it every
// evaluator records this combine tree — its own top-level nodes and an ordered
// list of the modules it @uses/@forwards, each carrying the pre-module comments
// captured before it. combineCss walks that tree to produce the final top-level
// node list, exactly mirroring dart's visitModule recursion.

// combineNode is one module's contribution to the deferred combine: its own
// top-level CSS nodes (children excluded) and its @use/@forward children in
// source order.
type combineNode struct {
	own      []cssNode
	edges    []combineEdge
	hasCSS   bool // memoised transitivelyContainsCss
	hasCSSOK bool
}

// combineEdge links a module to one module it @uses/@forwards, carrying the
// pre-module comments captured immediately before that load site.
type combineEdge struct {
	child *combineNode
	pre   []cssNode
}

// recordOwn appends a top-level statement's own emitted nodes to this module's
// combine contribution. Called for every top-level statement that is not a
// module load.
func (c *combineNode) recordOwn(nodes []cssNode) {
	if len(nodes) == 0 {
		return
	}
	c.own = append(c.own, nodes...)
}

// recordUse records a @use/@forward of child at this position, mirroring dart's
// _registerCommentsForModule: when child transitively contains CSS and this is
// its first load in the compilation, the comments accumulated so far in this
// module's own CSS are swept out into the edge as child's pre-module comments
// (dart clears _root.children). A non-CSS or repeat load leaves them in place.
func (c *combineNode) recordUse(child *combineNode, firstLoad bool) {
	if child == nil {
		return
	}
	var pre []cssNode
	if firstLoad && len(c.own) > 0 && child.containsCSS() {
		pre = c.own
		c.own = nil
	}
	c.edges = append(c.edges, combineEdge{child: child, pre: pre})
}

// containsCSS reports dart's transitivelyContainsCss: the module's own CSS is
// non-empty, or some module it uses transitively contains CSS. Pre-module
// comments are not part of a module's own CSS (they were swept into an edge), so
// they do not count here — matching dart, where they live outside module.css.
func (c *combineNode) containsCSS() bool {
	if c.hasCSSOK {
		return c.hasCSS
	}
	// Guard against re-entrancy on a cyclic query (the module graph is a DAG, so
	// this only bounds pathological shared re-entry): memoise false first.
	c.hasCSSOK = true
	res := len(c.own) > 0
	if !res {
		for _, e := range c.edges {
			if e.child.containsCSS() {
				res = true
				break
			}
		}
	}
	c.hasCSS = res
	return res
}

// combineCss walks the combine tree rooted at root and returns the entry
// stylesheet's final top-level node list, reproducing dart's _combineCss:
// modules are visited in reverse topological order (a module's used modules
// first), each module's leading @import region is collected into a shared import
// block hoisted to the top, and pre-module comments are placed into the import
// block while it is still the only content and into the CSS body thereafter.
func combineCss(root *combineNode) []cssNode {
	var imports, css []cssNode
	seen := map[*combineNode]bool{}
	var visit func(m *combineNode)
	visit = func(m *combineNode) {
		if seen[m] {
			return
		}
		seen[m] = true
		for _, e := range m.edges {
			if !e.child.containsCSS() {
				continue
			}
			if len(e.pre) > 0 {
				// Intermix the pre-module comments with the plain-CSS @import block
				// until real CSS appears, then treat them as ordinary CSS.
				if len(css) == 0 {
					imports = append(imports, e.pre...)
				} else {
					css = append(css, e.pre...)
				}
			}
			visit(e.child)
		}
		region, body := splitModuleImports(m.own)
		imports = append(imports, region...)
		css = append(css, body...)
	}
	visit(root)
	return append(imports, css...)
}

// splitModuleImports partitions a module's own top-level nodes into its leading
// plain-CSS @import region (hoisted to the shared import block) and the rest of
// its CSS body. Out-of-order @imports — a plain-CSS @import that follows other
// CSS — are first lifted into the region, reproducing dart's evaluation-time
// _endOfImports splicing so they too are hoisted.
func splitModuleImports(own []cssNode) (region, body []cssNode) {
	own = hoistOutOfOrderImports(own)
	idx := indexAfterImports(own)
	return own[:idx], own[idx:]
}

// indexAfterImports returns the index of the first node after the module's
// leading run of plain-CSS @imports, treating interleaved comments as part of
// the run (dart's _indexAfterImports). A comment after the last @import but
// before the first rule therefore stays with the CSS body.
func indexAfterImports(nodes []cssNode) int {
	last := -1
	for i, n := range nodes {
		if isCSSImport(n) {
			last = i
			continue
		}
		if _, ok := n.(*cssComment); ok {
			continue
		}
		break
	}
	return last + 1
}

// hoistOutOfOrderImports lifts every plain-CSS @import that appears after the
// leading import region up to the end of that region, so a late @import still
// joins the hoisted import block (dart's _endOfImports out-of-order splicing).
// Node identity is preserved; blank-line separation is recomputed afterwards by
// combineTopLevelGroups, so no separator bookkeeping is needed here.
func hoistOutOfOrderImports(nodes []cssNode) []cssNode {
	b := 0
	for b < len(nodes) && isImportRegionNode(nodes[b]) {
		b++
	}
	var out, rest []cssNode
	for _, n := range nodes[b:] {
		if isCSSImport(n) {
			out = append(out, n)
		} else {
			rest = append(rest, n)
		}
	}
	if len(out) == 0 {
		return nodes
	}
	res := make([]cssNode, 0, len(nodes))
	res = append(res, nodes[:b]...)
	res = append(res, out...)
	res = append(res, rest...)
	return res
}
