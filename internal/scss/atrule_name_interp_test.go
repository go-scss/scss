// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import "testing"

// TestAtRuleNameInterpolation covers dart-sass's interpolated at-rule names
// (`@#{expr}…`). Any interpolated at-rule is parsed as a generic (unknown)
// at-rule — none of the built-in at-rules' parse-time behaviour is triggered —
// except @keyframes, whose behaviour is applied at eval time once the resolved
// name is known. Each expected output is byte-verified against dart-sass 1.102.
func TestAtRuleNameInterpolation(t *testing.T) {
	cases := []struct{ in, want string }{
		// Interpolation at the beginning, followed by literal text.
		{`@#{"a"}-x;`, "@a-x;\n"},
		// Interpolation in the middle of the name.
		{`@interopl#{"mid"}dle;`, "@interoplmiddle;\n"},
		// Interpolation entirely composing the name, with a plain prelude/body.
		{`@#{"foo"} bar {x: y}`, "@foo bar {\n  x: y;\n}\n"},
		// An empty prelude drops to nil (`@a;`, not `@a ;`).
		{`@#{"a"} ;`, "@a;\n"},
		// An escape following an interpolation is decoded (\32 -> "2").
		{"@#{\"a\"}\\32;", "@a2;\n"},
		// @keyframes is the runtime exception: its body holds keyframe blocks.
		{`@#{"keyframes"} k { from {o: 0} }`,
			"@keyframes k {\n  from {\n    o: 0;\n  }\n}\n"},
		// @media, when interpolated, is NOT treated as a real media query: it is
		// generic, so the parenthesised prelude is kept verbatim and a nested
		// rule stays nested.
		{`@#{"media"} (x) { a {b: c} }`,
			"@media (x) {\n  a {\n    b: c;\n  }\n}\n"},
	}
	for _, c := range cases {
		res, err := Render(c.in, false, false, nil)
		if err != nil {
			t.Errorf("interpolated at-rule %q: unexpected error: %v", c.in, err)
			continue
		}
		if res.CSS != c.want {
			t.Errorf("interpolated at-rule %q:\n got %q\nwant %q", c.in, res.CSS, c.want)
		}
	}
}

// TestAtRuleNameMissing covers the fall-through in scanInterpolatedIdentifier
// when nothing consumable starts the name: the canonical scanIdentifier then
// reports the missing identifier.
func TestAtRuleNameMissing(t *testing.T) {
	if _, err := Render(`@;`, false, false, nil); err == nil {
		t.Fatal("expected an error for an at-rule with no name")
	}
}
