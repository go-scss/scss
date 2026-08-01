// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import "testing"

// TestOperatorKeywordCase covers dart-sass matching the SassScript operator
// keywords `and`, `or`, `not` case-sensitively: only the lowercase spelling is
// an operator, so `NOT()` calls a function named NOT and `true AND false` is a
// three-element space list. Outputs are byte-verified against dart-sass 1.102.
func TestOperatorKeywordCase(t *testing.T) {
	cases := []struct{ in, out string }{
		// Uppercase spellings are ordinary identifiers.
		{"@function NOT() {@return 1}\na { b: NOT(); }\n", "a {\n  b: 1;\n}\n"},
		{"a { b: NOT true; }\n", "a {\n  b: NOT true;\n}\n"},
		{"a { b: true AND false; }\n", "a {\n  b: true AND false;\n}\n"},
		{"a { b: true OR false; }\n", "a {\n  b: true OR false;\n}\n"},
		// Lowercase spellings still operate.
		{"a { b: not true; }\n", "a {\n  b: false;\n}\n"},
		{"a { b: true and false; }\n", "a {\n  b: false;\n}\n"},
		{"a { b: false or true; }\n", "a {\n  b: true;\n}\n"},
	}
	for _, c := range cases {
		if got := compile(t, c.in); got != c.out {
			t.Errorf("for %q:\n want: %q\n  got: %q", c.in, c.out, got)
		}
	}
}
