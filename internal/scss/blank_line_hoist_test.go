// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import "testing"

// TestRootBlankLineHoisted verifies dart-sass's top-level blank-line separation
// when the separating boundary falls on an invisible (empty, fully hoisted)
// parent rule. dart attaches the blank to the group boundary, so the next
// visible rule carries it; a naive per-node flag drops it with the invisible
// parent. Expectations are byte-exact against dart-sass 1.102.
func TestRootBlankLineHoisted(t *testing.T) {
	cases := []struct{ in, out string }{
		// Two top-level rules whose only content is a hoisted nested rule: the
		// invisible parents disappear, but the blank between the groups remains.
		{
			"a b { c & { m: 1; } }\nd { e & { m: 1; } }\n",
			"c a b {\n  m: 1;\n}\n\ne d {\n  m: 1;\n}\n",
		},
		// Nested rules hoisted from the SAME parent stay fused (no blank), while
		// the next source statement's group is separated by a blank.
		{
			"foo { & a { m: 1; } b & { m: 1; } }\nfoo { & c { m: 1; } d & { m: 1; } }\n",
			"foo a {\n  m: 1;\n}\nb foo {\n  m: 1;\n}\n\nfoo c {\n  m: 1;\n}\nd foo {\n  m: 1;\n}\n",
		},
		// A comment does not introduce a blank before the following rule even
		// across a group boundary.
		{
			"a { x: 1; }\n/* c */\nb { y: 2; }\n",
			"a {\n  x: 1;\n}\n\n/* c */\nb {\n  y: 2;\n}\n",
		},
	}
	for _, c := range cases {
		if got := compile(t, c.in); got != c.out {
			t.Errorf("for %q:\n want: %q\n  got: %q", c.in, c.out, got)
		}
	}
}
