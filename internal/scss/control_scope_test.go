// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import "testing"

// TestControlFlowScoping locks down dart-sass variable scoping across control
// directives (@if/@each/@for/@while). A control-flow body opens its own variable
// scope; an implicit (non-!global) assignment to a variable that only exists at
// the global scope writes through to global ONLY while no @function/@mixin/style
// rule boundary intervenes (dart's semi-global scope). All expectations are
// byte-exact against dart-sass 1.102.
func TestControlFlowScoping(t *testing.T) {
	cases := []struct{ in, out string }{
		// @if inside a style rule does not leak an implicit assignment to a
		// global-only variable, nor persist it into the rule body.
		{
			"$x: 1;\n.a {\n  @if true { $x: 2; }\n  v: $x;\n}\nb { w: $x; }\n",
			".a {\n  v: 1;\n}\n\nb {\n  w: 1;\n}\n",
		},
		// @if inside a @function likewise traps the implicit assignment locally.
		{
			"$x: 1;\n@function f() { @if true { $x: 9; } @return $x; }\n.a { v: f(); w: $x; }\n",
			".a {\n  v: 1;\n  w: 1;\n}\n",
		},
		// A variable that lives in the enclosing (non-global) rule scope IS updated
		// by a nested @if assignment.
		{
			".a {\n  $y: 1;\n  @if true { $y: 2; }\n  v: $y;\n}\n",
			".a {\n  v: 2;\n}\n",
		},
		// At the stylesheet root the control scope stays semi-global, so a nested
		// @if writes through to the pre-declared global.
		{
			"$x: 1;\n@if true {\n  @if true { $x: 5; }\n}\n.a { v: $x; }\n",
			".a {\n  v: 5;\n}\n",
		},
		// Top-level @if/@else assigning a pre-declared global updates it.
		{
			"$x: null;\n@if true { $x: a; } @else { $x: b; }\n.a { v: $x; }\n",
			".a {\n  v: a;\n}\n",
		},
		// The @else branch also opens a semi-global control scope.
		{
			"$x: null;\n@if false { $x: a; } @else { $x: b; }\n.a { v: $x; }\n",
			".a {\n  v: b;\n}\n",
		},
		// @each reassigning a global-only variable from within a rule does not
		// persist past the loop.
		{
			"$x: 1;\n.a {\n  @each $i in 1 2 { $x: $i; }\n  v: $x;\n}\n",
			".a {\n  v: 1;\n}\n",
		},
		// @for at the root writes through to a pre-declared global.
		{
			"$x: 0;\n@for $i from 1 through 3 { $x: $i; }\n.a { v: $x; }\n",
			".a {\n  v: 3;\n}\n",
		},
		// @while at the root writes through to a pre-declared global.
		{
			"$x: 0;\n@while $x < 3 { $x: $x + 1; }\n.a { v: $x; }\n",
			".a {\n  v: 3;\n}\n",
		},
	}
	for _, c := range cases {
		if got := compile(t, c.in); got != c.out {
			t.Errorf("for %q:\n want: %q\n  got: %q", c.in, c.out, got)
		}
	}
}
