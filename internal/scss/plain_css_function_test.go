// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import "testing"

// TestPlainCSSFunctionBody covers dart-sass's plain-CSS custom-function parsing:
// inside a `@function` body every descriptor declaration takes a verbatim
// token-stream value (it may contain `{}` and other CSS-invalid characters), and
// a nested style rule inside the body is still parsed as a rule. Each expected
// output is byte-verified against dart-sass 1.102.
func TestPlainCSSFunctionBody(t *testing.T) {
	cases := []struct{ in, want string }{
		// Token-stream value with braces and special characters.
		{"@function --a() { result: {}#&%^*; }",
			"@function --a() {\n  result: {}#&%^*;\n}\n"},
		// Two descriptors; the second is `}`-terminated, so its value keeps the
		// trailing space exactly as dart-sass reproduces it.
		{"@function --a() { result: a; RESULT: b }",
			"@function --a() {\n  result: a;\n  RESULT: b ;\n}\n"},
		// A nested style rule inside the body is not a declaration: the `{` before
		// any `:` makes funcDeclaration defer to the general statement parser.
		{"@function --a() { .x { result: y } }",
			"@function --a() {\n  .x {\n    result: y ;\n  }\n}\n"},
		// A colon inside the value (url(a:b)) is not the name/value separator.
		{"@function --a() { result: url(a:b) }",
			"@function --a() {\n  result: url(a:b) ;\n}\n"},
		// The at-rule name is matched case-insensitively.
		{"@FUNCTION --a() { result: {}#&%^*; }",
			"@FUNCTION --a() {\n  result: {}#&%^*;\n}\n"},
	}
	for _, c := range cases {
		if got := pcss(t, c.in); got != c.want {
			t.Errorf("plain-CSS @function %q:\n got %q\nwant %q", c.in, got, c.want)
		}
	}
}

// TestPlainCSSFunctionUnterminated covers the fall-through path in
// funcDeclaration when the descriptor has no colon and the input ends: the
// general statement parser then reports the missing ":".
func TestPlainCSSFunctionUnterminated(t *testing.T) {
	imp := func(url, _ string) (string, string, bool) {
		if url == "p" {
			return "@function --a() { result", "p.css", true
		}
		return "", "", false
	}
	if _, err := Render(`@use "p";`, false, false, imp); err == nil {
		t.Fatal("expected an error for an unterminated plain-CSS @function body")
	}
}
