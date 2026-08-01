// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import "testing"

// TestAtRootUnknownAtRule verifies that @at-root's suppression of the enclosing
// style rule carries through an unknown at-rule: a bare declaration inside
// `@foo` stays direct (no re-materialised parent selector) and a nested style
// rule escapes to the root. Byte-exact against dart-sass 1.102.
func TestAtRootUnknownAtRule(t *testing.T) {
	cases := []struct{ in, out string }{
		// Declaration inside @foo under @at-root: no `p` wrapper.
		{
			"p {\n  @at-root {\n    @foo { bar: bat; }\n  }\n}\n",
			"@foo {\n  bar: bat;\n}\n",
		},
		// A nested style rule inside @foo under @at-root escapes to the root.
		{
			"p {\n  @at-root {\n    @foo { .x { bar: bat; } }\n  }\n}\n",
			"@foo {\n  .x {\n    bar: bat;\n  }\n}\n",
		},
		// Control: WITHOUT @at-root the selector is re-materialised as before.
		{
			"p {\n  @foo { bar: bat; }\n}\n",
			"@foo {\n  p {\n    bar: bat;\n  }\n}\n",
		},
	}
	for _, c := range cases {
		if got := compile(t, c.in); got != c.out {
			t.Errorf("for %q:\n want: %q\n  got: %q", c.in, c.out, got)
		}
	}
}
