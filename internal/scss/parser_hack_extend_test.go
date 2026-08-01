// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import "testing"

// TestStarPropertyHack verifies the IE "star hack": a declaration name may begin
// with a leading `*` when it is immediately followed by an identifier (dart-sass
// parses `*width: …` as a property), while a bare `*` or `*` followed by a
// selector character remains the universal selector. Byte-exact against
// dart-sass 1.102.
func TestStarPropertyHack(t *testing.T) {
	cases := []struct{ in, out string }{
		// The hack: `*name` is a property.
		{
			"foo {\n  *width: 10px;\n  *-x: 5px;\n}\n",
			"foo {\n  *width: 10px;\n  *-x: 5px;\n}\n",
		},
		// The universal selector and its combinations stay style rules.
		{"* { a: b; }\n", "* {\n  a: b;\n}\n"},
		{"*.foo { c: d; }\n", "*.foo {\n  c: d;\n}\n"},
		{"*:hover { e: f; }\n", "*:hover {\n  e: f;\n}\n"},
	}
	for _, c := range cases {
		if got := compile(t, c.in); got != c.out {
			t.Errorf("for %q:\n want: %q\n  got: %q", c.in, c.out, got)
		}
	}
}

// TestExtendOptionalFlag verifies that `!optional` is stripped from an @extend
// whether or not the extendee contains interpolation (dart), and that a
// non-`optional` bang after the extendee is an error.
func TestExtendOptionalFlag(t *testing.T) {
	// Interpolated extendee with the flag: the placeholder is unused and
	// optional, so nothing is emitted for `.test`.
	if got := compile(t, ".test {\n  @extend #{'%m'} !optional\n}\n.a { x: y; }\n"); got != ".a {\n  x: y;\n}\n" {
		t.Errorf("interpolated !optional: got %q", got)
	}
	// Literal extendee with the flag behaves the same.
	if got := compile(t, ".test {\n  @extend %m !optional;\n}\n.a { x: y; }\n"); got != ".a {\n  x: y;\n}\n" {
		t.Errorf("literal !optional: got %q", got)
	}
	// A bang followed by something other than `optional` is an error.
	mustErr(t, ".test { @extend .a !foo; }\n.a { x: y; }")
}
