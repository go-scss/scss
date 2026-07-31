// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import "testing"

// TestUnknownAtRuleSelectorWrap verifies that an unknown at-rule with a block,
// when nested in a style rule, bubbles out and re-materialises the enclosing
// selector around its contents (dart), while at the top level its declarations
// stay direct. Byte-exact against dart-sass 1.102.
func TestUnknownAtRuleSelectorWrap(t *testing.T) {
	cases := []struct{ in, out string }{
		// Top level: declaration stays direct.
		{
			"@foo { bar: bat; }\n",
			"@foo {\n  bar: bat;\n}\n",
		},
		// Nested in a rule: the selector wraps the declaration inside the at-rule.
		{
			"div {\n  @foo { font: a; }\n  @bar { color: b; }\n}\n",
			"@foo {\n  div {\n    font: a;\n  }\n}\n@bar {\n  div {\n    color: b;\n  }\n}\n",
		},
		// A nested style rule inside the at-rule nests under the parent selector.
		{
			"div {\n  @foo { .x { bar: bat; } }\n}\n",
			"@foo {\n  div .x {\n    bar: bat;\n  }\n}\n",
		},
	}
	for _, c := range cases {
		if got := compile(t, c.in); got != c.out {
			t.Errorf("for %q:\n want: %q\n  got: %q", c.in, c.out, got)
		}
	}
}
