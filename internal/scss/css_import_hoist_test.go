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

// TestCSSImportAlreadyOrdered guards the combine's no-op import path: when every
// top-level @import already precedes the rules, output is unchanged (the
// out-of-order collection is empty and the tree is returned as-is).
func TestCSSImportAlreadyOrdered(t *testing.T) {
	got := renderWith(t, "@import url(a);\na {b: c}\n", nil)
	want := "@import url(a);\na {\n  b: c;\n}\n"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

// TestSplitModuleImports drives splitModuleImports / hoistOutOfOrderImports /
// indexAfterImports across their branches: an out-of-order import after a rule
// is lifted into the leading region, a comment interleaves the imports, and the
// comment after the last import stays with the CSS body.
func TestSplitModuleImports(t *testing.T) {
	imp := func(p string) *cssAtRule { return &cssAtRule{name: "import", params: p} }
	// [import a][comment][rule][import b] -> region [import a][comment][import b],
	// body [rule].
	own := []cssNode{
		imp("url(a)"),
		&cssComment{text: "/* c */"},
		&cssStyleRule{rawSel: "x", raw: true},
		imp("url(b)"),
	}
	region, body := splitModuleImports(own)
	if len(region) != 3 || !isCSSImport(region[0]) || !isCSSImport(region[2]) {
		t.Fatalf("region = %#v", region)
	}
	if len(body) != 1 {
		t.Fatalf("body = %#v", body)
	}
	if _, ok := body[0].(*cssStyleRule); !ok {
		t.Fatalf("expected rule in body, got %#v", body[0])
	}

	// A trailing comment after the last import belongs to the body, not the region.
	own2 := []cssNode{imp("url(a)"), &cssComment{text: "/* after */"}, &cssStyleRule{rawSel: "x", raw: true}}
	region2, body2 := splitModuleImports(own2)
	if len(region2) != 1 || len(body2) != 2 {
		t.Fatalf("region2 %#v body2 %#v", region2, body2)
	}

	// No imports at all: everything is body, and hoistOutOfOrderImports is a no-op.
	own3 := []cssNode{&cssStyleRule{rawSel: "x", raw: true}}
	region3, body3 := splitModuleImports(own3)
	if len(region3) != 0 || len(body3) != 1 {
		t.Fatalf("region3 %#v body3 %#v", region3, body3)
	}
}
