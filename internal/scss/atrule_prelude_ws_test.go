// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import "testing"

// TestUnknownAtRulePreludeWhitespace covers the whitespace normalization applied
// to a truly-unknown at-rule's prelude (sass-spec libsass-closed-issues/
// issue_1263). dart-sass's almostAnyValue reader folds a newline-free run of
// space/tab characters to the run's final whitespace character, keeps the tab
// when it is that final character, preserves line breaks and the indentation that
// follows them (dropping only a line's trailing whitespace), and never touches
// text inside a quoted string. Each expected output is byte-verified against
// dart-sass 1.102.
func TestUnknownAtRulePreludeWhitespace(t *testing.T) {
	cases := []struct{ in, want string }{
		// The issue_1263 fixture: multiple interior spaces collapse to one; the
		// bare name keeps its `;`; a directly-attached prelude gains one space.
		{"foo {\n  @ap#{'pl'}y;\n  @apply(--bar);\n  @apply  (  --bar  );\n" +
			"  @ap#{'pl'}y   (   --bar , --foo  )  ;\n}\n",
			"foo {\n  @apply;\n  @apply (--bar);\n  @apply ( --bar );\n" +
				"  @apply ( --bar , --foo );\n}\n"},
		// A run of only spaces collapses to a single space.
		{"foo {\n  @apply a  b;\n}\n", "foo {\n  @apply a b;\n}\n"},
		// space-then-tab keeps the tab (the run's final ws char).
		{"foo {\n  @apply a \tb;\n}\n", "foo {\n  @apply a\tb;\n}\n"},
		// tab-then-space keeps the space.
		{"foo {\n  @apply a\t b;\n}\n", "foo {\n  @apply a b;\n}\n"},
		// A run carrying a newline preserves the break and the following indent
		// while dropping the line's trailing space.
		{"foo {\n  @asdf x \n     y;\n}\n", "foo {\n  @asdf x\n     y;\n}\n"},
		// Interior spaces inside a quoted string are left untouched; only the run
		// between the string and the next token collapses.
		{"foo {\n  @apply \"a  b\"  c;\n}\n", "foo {\n  @apply \"a  b\" c;\n}\n"},
		// Whitespace collapsing survives an interpolation boundary: the run after
		// the interpolation folds to a single space.
		{"foo {\n  @apply #{1}  x;\n}\n", "foo {\n  @apply 1 x;\n}\n"},
		// A prelude that is only whitespace up to end-of-input yields a bare rule
		// (exercises the run loop's end-of-input exit).
		{"@foo a  ", "@foo a;\n"},
	}
	for _, c := range cases {
		res, err := Render(c.in, false, false, nil)
		if err != nil {
			t.Errorf("prelude %q: unexpected error: %v", c.in, err)
			continue
		}
		if res.CSS != c.want {
			t.Errorf("prelude %q:\n got %q\nwant %q", c.in, res.CSS, c.want)
		}
	}
}
