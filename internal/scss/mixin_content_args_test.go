// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import "testing"

// TestMixinContentArguments locks down `@content(...)` argument passing and the
// `using (...)` content-block parameter list. A content block is a closure: its
// arguments are evaluated in the mixin's environment at the `@content` site,
// while its body runs in the caller's lexical environment with those arguments
// bound to the `using` parameters. All expectations are byte-exact against
// dart-sass 1.102 (sass-spec non_conformant/mixin/content/arguments/*).
func TestMixinContentArguments(t *testing.T) {
	cases := []struct{ in, out string }{
		// Positional arguments bind to the content block's `using` parameters.
		{
			"@mixin m { @content(v1, v2); }\na { @include m using ($a, $b) { x: $a; y: $b; } }\n",
			"a {\n  x: v1;\n  y: v2;\n}\n",
		},
		// A bare `@content` with a parameterised block falls back to the block's
		// own default values.
		{
			"@mixin m { @content; }\na { @include m using ($a: d1, $b: d2) { x: $a; y: $b; } }\n",
			"a {\n  x: d1;\n  y: d2;\n}\n",
		},
		// A spread list argument is expanded into positional content arguments.
		{
			"@mixin m { @content((p q)...); }\na { @include m using ($a, $b) { x: $a; y: $b; } }\n",
			"a {\n  x: p;\n  y: q;\n}\n",
		},
		// Content arguments are lexically scoped to the block: a mixin invoked from
		// within the block resolves variables in its own closure, not the block's.
		{
			"$var: top;\n@mixin m($var) { @content(carg); }\n@mixin inner { z: $var; }\na { @include m(marg) using ($var) { p: $var; @include inner; } }\n",
			"a {\n  p: carg;\n  z: top;\n}\n",
		},
		// `using` is a case-insensitive keyword needing no surrounding whitespace.
		{
			"a { @mixin m { @content(1, 2); } @include m()UsInG($a,$b){ x: $a; y: $b; } }\n",
			"a {\n  x: 1;\n  y: 2;\n}\n",
		},
	}
	for _, c := range cases {
		if got := compile(t, c.in); got != c.out {
			t.Errorf("for %q:\n want: %q\n  got: %q", c.in, c.out, got)
		}
	}
}

// TestMixinContentTrailingBlock locks down the parent-selector split when a
// nested rule is emitted from inside a mixin, @content or control-flow body: the
// declarations following the nested rule must land in a fresh copy of the
// enclosing selector, preserving source order, exactly as dart-sass 1.102 does
// for a style rule's own inline children.
func TestMixinContentTrailingBlock(t *testing.T) {
	cases := []struct{ in, out string }{
		// A nested rule emitted by an @include splits the caller's block: the
		// declarations after it (both the mixin's own trailing content and the
		// caller's later statements) reopen the parent selector.
		{
			"@mixin hux { no: p; div { some: nested; } /* end */ }\na { hey: ho; @include hux; after: x; }\n",
			"a {\n  hey: ho;\n  no: p;\n}\na div {\n  some: nested;\n}\na {\n  /* end */\n  after: x;\n}\n",
		},
		// The same split happens for a nested rule inside an @if body.
		{
			"a { x: 0; @if true { p: 1; b { q: 2; } r: 3; } s: 4; }\n",
			"a {\n  x: 0;\n  p: 1;\n}\na b {\n  q: 2;\n}\na {\n  r: 3;\n  s: 4;\n}\n",
		},
		// And for each iteration of an @each body.
		{
			"a { @each $i in 1 2 { b { q: $i; } r: $i; } }\n",
			"a b {\n  q: 1;\n}\na {\n  r: 1;\n}\na b {\n  q: 2;\n}\na {\n  r: 2;\n}\n",
		},
		// A recursive mixin keeps each invocation's local variables private: the
		// inner call neither sees nor clobbers the outer call's $var.
		{
			"@mixin wl($recurse) { $var: before; @if ($recurse) { @include wl($recurse: false); } var: $var; $var: after; }\n.el { @include wl($recurse: true); }\n",
			".el {\n  var: before;\n  var: before;\n}\n",
		},
		// A content block that both takes an argument and emits a nested rule: the
		// argument stays bound across the split.
		{
			"@mixin m { @content(1); }\na { @include m using ($n) { p: $n; b { q: $n; } r: $n; } }\n",
			"a {\n  p: 1;\n}\na b {\n  q: 1;\n}\na {\n  r: 1;\n}\n",
		},
	}
	for _, c := range cases {
		if got := compile(t, c.in); got != c.out {
			t.Errorf("for %q:\n want: %q\n  got: %q", c.in, c.out, got)
		}
	}
}
