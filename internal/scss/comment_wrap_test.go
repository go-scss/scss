// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import "testing"

// TestLoudCommentSelectorWrap verifies that a loud comment which is the sole
// content of a @media bubbled out of a style rule is wrapped in the enclosing
// parent selector, exactly as a declaration would be. Byte-exact against
// dart-sass 1.102.
func TestLoudCommentSelectorWrap(t *testing.T) {
	cases := []struct{ in, out string }{
		// A @media at the root wraps nothing; nested under div it re-materialises
		// the div rule around the comment.
		{
			"@media (min-width: 640px) {\n  /* c */\n}\n\ndiv {\n  @media (min-width: 320px) {\n    /* c */\n  }\n}\n",
			"@media (min-width: 640px) {\n  /* c */\n}\n@media (min-width: 320px) {\n  div {\n    /* c */\n  }\n}\n",
		},
		// A loud comment directly inside a style rule stays in that rule.
		{
			".a {\n  /* c */\n  x: 1;\n}\n",
			".a {\n  /* c */\n  x: 1;\n}\n",
		},
	}
	for _, c := range cases {
		if got := compile(t, c.in); got != c.out {
			t.Errorf("for %q:\n want: %q\n  got: %q", c.in, c.out, got)
		}
	}
}
