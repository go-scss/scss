// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import "testing"

// importPlainCSSNested compiles an entry that nests a legacy `@import` of a
// plain-CSS file inside a style rule, using an importer that resolves the bare
// URL to a `.css` source so the loader takes the plain-CSS branch.
func importPlainCSSNested(t *testing.T, input, plain string) string {
	t.Helper()
	res, err := Render(input, false, false, renestImporter(map[string]string{
		"input":     input,
		"plain.css": plain,
	}))
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}
	return res.CSS
}

// TestImportPlainCSSNestedTwoLevels mirrors sass-spec
// css/plain/style_rule/nesting/through_import::two_levels: a nested @import of a
// .css file is parsed as PLAIN CSS, so its native CSS nesting is preserved
// verbatim while the outer rule is re-nested under the enclosing selector.
// Verified against dart-sass 1.102: `a {@import "plain"}` + `b {c {d: e}}` gives
// `a b { c { d: e; } }`.
func TestImportPlainCSSNestedTwoLevels(t *testing.T) {
	got := importPlainCSSNested(t, "a {@import \"plain\"}\n", "b {c {d: e}}\n")
	want := "a b {\n  c {\n    d: e;\n  }\n}\n"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

// TestImportPlainCSSNestedParentRef mirrors through_import::top_level_parent: a
// plain-CSS file whose top-level rule references the parent (`&`, native CSS
// nesting) is kept verbatim and wrapped in a copy of the enclosing selector.
// Verified against dart-sass 1.102: `a {@import "plain"}` + `& {b {c: d}}` gives
// `a { & { b { c: d; } } }`.
func TestImportPlainCSSNestedParentRef(t *testing.T) {
	got := importPlainCSSNested(t, "a {@import \"plain\"}\n", "& {b {c: d}}\n")
	want := "a {\n  & {\n    b {\n      c: d;\n    }\n  }\n}\n"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

// TestImportPlainCSSNestedOneLevel guards the simple descendant case still held
// by the plain-CSS path: a flat plain-CSS rule gains the enclosing selector as a
// descendant. Verified against dart-sass 1.102.
func TestImportPlainCSSNestedOneLevel(t *testing.T) {
	got := importPlainCSSNested(t, "a {@import \"plain\"}\n", "b {c: d}\n")
	want := "a b {\n  c: d;\n}\n"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
