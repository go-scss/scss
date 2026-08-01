// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import "testing"

// TestNumberUnitLexing covers dart-sass's dedicated unit lexer: a number's unit
// may start with a single "-" but never "--", and a "-" directly followed by a
// digit (or ".digit") ends the unit so the remainder is subtraction. A "-" that
// begins a fresh value also separates it into its own space-list element. Every
// expected output is byte-verified against dart-sass 1.102.
func TestNumberUnitLexing(t *testing.T) {
	cases := []struct{ in, out string }{
		// A single leading dash is a unit; a following "-<digit>" is subtraction.
		{"a { b: 1-em; }\n", "a {\n  b: 1-em;\n}\n"},
		{"a { b: 1-em-2-em; }\n", "a {\n  b: -1-em;\n}\n"},
		{"a { b: 1-A-em; }\n", "a {\n  b: 1-A-em;\n}\n"},
		// A double leading dash is not a unit: the number stands alone and the
		// "--…" identifier is a separate space-list element.
		{"a { b: 1--em; }\n", "a {\n  b: 1 --em;\n}\n"},
		{"a { b: 1--em-2--em; }\n", "a {\n  b: 1 --em-2--em;\n}\n"},
		// Dimension subtraction with no surrounding whitespace now evaluates.
		{"a { b: 10px-5px; }\n", "a {\n  b: 5px;\n}\n"},
		{"a { b: 1.5em-0.75em; }\n", "a {\n  b: 0.75em;\n}\n"},
		{"a { b: 1em-.75em; }\n", "a {\n  b: 0.25em;\n}\n"},
		// A "-" followed by whitespace stays part of the unit (a trailing dash),
		// leaving the next dimension as its own element.
		{"a { b: 10px- 10px; }\n", "a {\n  b: 10px- 10px;\n}\n"},
		// A bare "-<digit>" is subtraction, not a unit.
		{"a { b: 5-2; }\n", "a {\n  b: 3;\n}\n"},
		{"a { b: 5-a; }\n", "a {\n  b: 5-a;\n}\n"},
	}
	for _, c := range cases {
		if got := compile(t, c.in); got != c.out {
			t.Errorf("for %q:\n want: %q\n  got: %q", c.in, c.out, got)
		}
	}
}

// TestNumberUnitType confirms the parse split changes value types the way
// dart-sass reports them: `1--em` is a two-element list, `1-em` a number.
func TestNumberUnitType(t *testing.T) {
	cases := []struct{ in, out string }{
		{"@use \"sass:meta\";\na { b: meta.type-of(1-em); }\n", "a {\n  b: number;\n}\n"},
		{"@use \"sass:meta\";\na { b: meta.type-of(1--em); }\n", "a {\n  b: list;\n}\n"},
	}
	for _, c := range cases {
		if got := compile(t, c.in); got != c.out {
			t.Errorf("for %q:\n want: %q\n  got: %q", c.in, c.out, got)
		}
	}
}
