// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import "testing"

// TestAtRootParentReference exercises the query-less @at-root behaviour where the
// enclosing parent selector stays available for an explicit `&` reference while
// bare selectors escape to the root (dart nestWithin with implicitParent=false).
// The behaviour is verified byte-exactly against dart-sass 1.102.
func TestAtRootParentReference(t *testing.T) {
	cases := []struct{ in, out string }{
		// @at-root & shorthand: & resolves to the parent, placed at root.
		{
			".foo {\n  @at-root & {\n    a: b;\n  }\n}\n",
			".foo {\n  a: b;\n}\n",
		},
		// Bare selector inside a block @at-root escapes (no parent prefix); an
		// explicit & still resolves to the parent.
		{
			".foo {\n  @at-root {\n    .bar { a: b; }\n    & { c: d; }\n    &.x { e: f; }\n  }\n}\n",
			".bar {\n  a: b;\n}\n\n.foo {\n  c: d;\n}\n\n.foo.x {\n  e: f;\n}\n",
		},
		// A suffix on & and a further nested selector: & resolves, then the inner
		// selector nests implicitly under the resolved parent.
		{
			"test {\n  @at-root {\n    &post foo {\n      bar: baz;\n    }\n  }\n}\n",
			"testpost foo {\n  bar: baz;\n}\n",
		},
		// Deeper nesting: & resolves to the parent, the inner rule nests normally.
		{
			"test {\n  @at-root {\n    & {\n      foo {\n        bar: baz;\n      }\n    }\n  }\n}\n",
			"test foo {\n  bar: baz;\n}\n",
		},
		// @at-root propagates through @media without an intervening style rule, so
		// the inner selector still escapes the parent prefix.
		{
			"foo {\n  @at-root {\n    @media print {\n      bar { color: red; }\n    }\n  }\n}\n",
			"@media print {\n  bar {\n    color: red;\n  }\n}\n",
		},
	}
	for _, c := range cases {
		if got := compile(t, c.in); got != c.out {
			t.Errorf("for %q:\n want: %q\n  got: %q", c.in, c.out, got)
		}
	}
}
