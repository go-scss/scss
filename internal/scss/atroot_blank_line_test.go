// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import "testing"

// TestAtRootBlankLines locks down dart-sass's leading-blank-line policy for
// @at-root bodies hoisted to the document root. dart separates the first hoisted
// node from a preceding top-level style rule with a blank line EXCEPT when that
// rule is the one the body was hoisted out of, which the first node continues
// gap-free; a second, independent hoist after it is blank-separated again. All
// expectations are byte-exact against dart-sass 1.102.
func TestAtRootBlankLines(t *testing.T) {
	cases := []struct{ in, out string }{
		// The first @at-root continues its origin rule (no blank); the second,
		// independent @at-root out of the same rule is blank-separated.
		{
			"foo {\n  color: blue;\n  @at-root bar { x: 1; }\n  @at-root baz { y: 2; }\n}\n",
			"foo {\n  color: blue;\n}\nbar {\n  x: 1;\n}\n\nbaz {\n  y: 2;\n}\n",
		},
		// A hoist out of a deeply nested rule still continues the top-level rule it
		// escaped, gap-free (the CSS tree is flat: `b` carries selector `a b`).
		{
			"a {\n  color: blue;\n  b { @at-root c { x: 1; } }\n}\n",
			"a {\n  color: blue;\n}\nc {\n  x: 1;\n}\n",
		},
		// When the enclosing rule emits nothing (dropped as empty), consecutive
		// hoists from a loop are ordinary top-level siblings: blank-separated.
		{
			".c {\n  @for $i from 1 through 2 {\n    @at-root .b#{$i} { x: $i; }\n  }\n}\n",
			".b1 {\n  x: 1;\n}\n\n.b2 {\n  x: 2;\n}\n",
		},
		// Two separate top-level rules, the second carrying a hoist: the second
		// rule is blank-separated from the first, its hoist continues it gap-free.
		{
			"foo {\n  color: blue;\n  @at-root bar { color: red; }\n}\nfoo {\n  color: blue;\n  @at-root bar { color: red; }\n}\n",
			"foo {\n  color: blue;\n}\nbar {\n  color: red;\n}\n\nfoo {\n  color: blue;\n}\nbar {\n  color: red;\n}\n",
		},
	}
	for _, c := range cases {
		if got := compile(t, c.in); got != c.out {
			t.Errorf("for %q:\n want: %q\n  got: %q", c.in, c.out, got)
		}
	}
}
