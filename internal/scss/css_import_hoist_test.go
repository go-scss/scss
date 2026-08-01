// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import "testing"

// TestCSSImportHoistedAboveRule verifies dart-sass's rule that a plain-CSS
// @import emitted after a style rule is spliced back above it.
func TestCSSImportHoistedAboveRule(t *testing.T) {
	files := map[string]string{
		"_rule.scss":   "a {b: c}\n",
		"_import.scss": "@import url(http://example.com);\n",
	}
	got := renderWith(t, "@import \"rule\";\n@import \"import\";\n", files)
	want := "@import url(http://example.com);\na {\n  b: c;\n}\n"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

// TestCSSImportAlreadyOrdered guards hoistCSSImports's no-op path: when every
// top-level @import already precedes the rules, output is unchanged (the
// out-of-order collection is empty and the tree is returned as-is).
func TestCSSImportAlreadyOrdered(t *testing.T) {
	got := renderWith(t, "@import url(a);\na {b: c}\n", nil)
	want := "@import url(a);\na {\n  b: c;\n}\n"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

// TestCSSImportHoistUnit drives hoistCSSImports directly across its branches:
// an empty region node run, a comment inside the region, an out-of-order import
// after a rule, and a trailing rule left behind.
func TestCSSImportHoistUnit(t *testing.T) {
	imp := func(p string) *cssAtRule { return &cssAtRule{name: "import", params: p} }
	// [import a][comment][rule][import b] -> [import a][comment][import b][rule]
	root := &cssRoot{nodes: []cssNode{
		imp("url(a)"),
		&cssComment{text: "/* c */"},
		&cssStyleRule{rawSel: "x", raw: true, blankBefore: true},
		imp("url(b)"),
	}}
	hoistCSSImports(root)
	if !isCSSImport(root.nodes[0]) || !isCSSImport(root.nodes[2]) {
		t.Fatalf("expected imports at 0 and 2, got %#v", root.nodes)
	}
	if _, ok := root.nodes[3].(*cssStyleRule); !ok {
		t.Fatalf("expected rule last, got %#v", root.nodes[3])
	}
	if root.nodes[3].(*cssStyleRule).blankBefore {
		t.Fatalf("first post-import node should sit flush against imports")
	}
	// No out-of-order import: unchanged.
	root2 := &cssRoot{nodes: []cssNode{imp("url(a)"), &cssStyleRule{rawSel: "x", raw: true}}}
	hoistCSSImports(root2)
	if len(root2.nodes) != 2 || !isCSSImport(root2.nodes[0]) {
		t.Fatalf("no-op path altered tree: %#v", root2.nodes)
	}
}
