// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import "testing"

// TestCombineRecordOwn covers recordOwn's empty guard and its append path.
func TestCombineRecordOwn(t *testing.T) {
	c := &combineNode{}
	c.recordOwn(nil)
	if len(c.own) != 0 {
		t.Fatalf("empty recordOwn added nodes: %#v", c.own)
	}
	r := &cssStyleRule{rawSel: "a", raw: true}
	c.recordOwn([]cssNode{r})
	if len(c.own) != 1 || c.own[0] != r {
		t.Fatalf("own = %#v", c.own)
	}
}

// TestCombineRecordUse covers recordUse: a nil child is ignored; a first load of
// a CSS-bearing module sweeps the pending own comments into the edge; a non-CSS
// child or a repeat load leaves them in place.
func TestCombineRecordUse(t *testing.T) {
	// nil child: no-op.
	c := &combineNode{}
	c.recordUse(nil, true)
	if len(c.edges) != 0 {
		t.Fatalf("nil child recorded an edge")
	}

	// First load of a CSS-bearing child: sweep the pending comment into pre.
	c = &combineNode{own: []cssNode{&cssComment{text: "/* pre */"}}}
	cssChild := &combineNode{own: []cssNode{&cssStyleRule{rawSel: "x", raw: true}}}
	c.recordUse(cssChild, true)
	if len(c.own) != 0 {
		t.Fatalf("own not swept: %#v", c.own)
	}
	if len(c.edges) != 1 || len(c.edges[0].pre) != 1 {
		t.Fatalf("edge/pre = %#v", c.edges)
	}

	// Non-CSS child: comment stays as own, edge has no pre.
	c = &combineNode{own: []cssNode{&cssComment{text: "/* pre */"}}}
	emptyChild := &combineNode{}
	c.recordUse(emptyChild, true)
	if len(c.own) != 1 || len(c.edges) != 1 || c.edges[0].pre != nil {
		t.Fatalf("non-css child swept: own %#v edges %#v", c.own, c.edges)
	}

	// Repeat load (firstLoad false): no sweep even for a CSS-bearing child.
	c = &combineNode{own: []cssNode{&cssComment{text: "/* pre */"}}}
	c.recordUse(cssChild, false)
	if len(c.own) != 1 || c.edges[0].pre != nil {
		t.Fatalf("repeat load swept: own %#v", c.own)
	}
}

// TestCombineCssDiamondAndBody covers combineCss's diamond dedup (a shared child
// is visited once) and the branch that places pre-module comments into the CSS
// body once real CSS already exists, plus skipping a CSS-less child.
func TestCombineCssDiamondAndBody(t *testing.T) {
	shared := &combineNode{own: []cssNode{&cssStyleRule{rawSel: "s", raw: true}}}
	empty := &combineNode{}
	// left and right both use shared; root uses left then right, and carries a
	// pre-comment before right that must land in the CSS body (CSS already
	// present from left/shared). root also uses a CSS-less module (skipped).
	left := &combineNode{}
	left.recordUse(shared, true)
	right := &combineNode{}
	right.recordUse(shared, false)
	root := &combineNode{}
	root.recordUse(empty, true) // skipped: no CSS
	root.recordUse(left, true)
	// Manually attach a pre-comment on the edge to right, after CSS exists.
	root.edges = append(root.edges, combineEdge{child: right, pre: []cssNode{&cssComment{text: "/* mid */"}}})

	out := combineCss(root)
	// shared's rule must appear exactly once (diamond dedup).
	count := 0
	sawMid := false
	for _, n := range out {
		if r, ok := n.(*cssStyleRule); ok && r.rawSel == "s" {
			count++
		}
		if cm, ok := n.(*cssComment); ok && cm.text == "/* mid */" {
			sawMid = true
		}
	}
	if count != 1 {
		t.Fatalf("shared rule emitted %d times, want 1", count)
	}
	if !sawMid {
		t.Fatalf("mid comment not emitted")
	}
}

// TestLeadsWithStrippedComment covers every branch of the leading-blank helper.
func TestLeadsWithStrippedComment(t *testing.T) {
	strip := &cssComment{text: "/*# sourceMappingURL=x */"}
	rule := &cssStyleRule{rawSel: "a", raw: true, nodes: []cssNode{&cssDeclaration{name: "x"}}}
	invisible := &cssStyleRule{} // empty selector => empty container, skipped

	// stripped comment then real content: true.
	if !leadsWithStrippedComment([]cssNode{invisible, strip, rule}) {
		t.Fatal("want true for stripped-then-rule")
	}
	// a non-stripped node first: false.
	if leadsWithStrippedComment([]cssNode{rule, strip}) {
		t.Fatal("want false when real content leads")
	}
	// only a stripped comment (no following content): false.
	if leadsWithStrippedComment([]cssNode{strip}) {
		t.Fatal("want false for a lone stripped comment")
	}
	// nothing visible at all: false.
	if leadsWithStrippedComment([]cssNode{invisible}) {
		t.Fatal("want false for no visible content")
	}
}
