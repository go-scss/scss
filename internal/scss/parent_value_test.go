// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import "testing"

// TestParentSelectorValue checks that `&` in an expression context evaluates to
// dart-sass's SelectorList.asSassList: a comma list of the complex selectors,
// each a space list of unquoted strings for its compound selectors and
// combinators. Verified byte-exact against dart-sass 1.102.
func TestParentSelectorValue(t *testing.T) {
	cases := []struct{ in, out string }{
		// A descendant selector is one comma-list element of two space parts.
		{
			"@use \"sass:list\";\n.test .nest {\n  n: list.length(&);\n  @each $s in & { l: list.length($s); }\n}\n",
			".test .nest {\n  n: 1;\n  l: 2;\n}\n",
		},
		// A comma selector list has one element per complex selector.
		{
			"@use \"sass:list\";\n.test, .other {\n  n: list.length(&);\n  @each $s in & { l: list.length($s); }\n}\n",
			".test, .other {\n  n: 2;\n  l: 1;\n  l: 1;\n}\n",
		},
		// Combinators contribute their own string parts to the inner space list
		// (\".a\", \">\", \".b\" => 3), while the outer comma list has one element.
		{
			"@use \"sass:list\";\n.a > .b {\n  outer: list.length(&);\n  @each $s in & { inner: list.length($s); }\n}\n",
			".a > .b {\n  outer: 1;\n  inner: 3;\n}\n",
		},
		// Interpolating `&` serializes the list back to the selector text.
		{
			".foo {\n  bar: #{&};\n}\n",
			".foo {\n  bar: .foo;\n}\n",
		},
	}
	for _, c := range cases {
		if got := compile(t, c.in); got != c.out {
			t.Errorf("for %q:\n want: %q\n  got: %q", c.in, c.out, got)
		}
	}
}
