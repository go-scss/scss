// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import "testing"

// TestAsSlashSerialization exercises the as-slash provenance a "/" between two
// number literals carries: it serializes back as "left/right" (recursively for a
// nested chain) until the value is consumed, whereupon it becomes the quotient.
func TestAsSlashSerialization(t *testing.T) {
	cases := []struct{ src, want string }{
		// Preserved: a slash number that reaches a declaration value directly.
		{"a{b: 1/2}", "a {\n  b: 1/2;\n}\n"},
		{"a{b: 1/2/3/4/5}", "a {\n  b: 1/2/3/4/5;\n}\n"},
		{"a{b: 1 2/3 4}", "a {\n  b: 1 2/3 4;\n}\n"},
		{"a{b: (1 2/3 4)}", "a {\n  b: 1 2/3 4;\n}\n"},
		{"a{b: #{1/2}}", "a {\n  b: 1/2;\n}\n"},
		// Consumed: parenthesized scalar, variable, arithmetic, function results.
		{"a{b: (1/2)}", "a {\n  b: 0.5;\n}\n"},
		{"$a: 1/2;\nb{c: $a}", "b {\n  c: 0.5;\n}\n"},
		{"a{b: 1/2 + 1}", "a {\n  b: 1.5;\n}\n"},
		{"@function f($x){@return 1 $x 2}\nc{d: f(1/2)}", "c {\n  d: 1 0.5 2;\n}\n"},
		{"@use \"sass:list\";\nc{d: list.join(1/2, 3/4)}", "c {\n  d: 0.5 0.75;\n}\n"},
		{"@use \"sass:list\";\nc{d: list.nth(3 1/2 4, 2)}", "c {\n  d: 0.5;\n}\n"},
	}
	for _, c := range cases {
		if got := compile(t, c.src); got != c.want {
			t.Errorf("for %q:\n want: %q\n  got: %q", c.src, c.want, got)
		}
	}
}

// TestAsSlashArgBinding covers the remaining as-slash consumption points: a rest
// spread of a list (each element consumed individually) or of a scalar/map, a
// mixin default value, a `@use ... with` configuration, and a namespaced module
// variable assignment.
func TestAsSlashArgBinding(t *testing.T) {
	cases := []struct{ src, want string }{
		{"@use \"sass:list\";\nc{d: list.join(1/2 3/4...)}", "c {\n  d: 0.5 0.75;\n}\n"},
		{"@use \"sass:list\";\nc{d: list.join(1/2..., (\"list2\": 3/4)...)}", "c {\n  d: 0.5 0.75;\n}\n"},
		{"@mixin m($x: 1/2){c{d: $x}}\n@include m;", "c {\n  d: 0.5;\n}\n"},
	}
	for _, c := range cases {
		if got := compile(t, c.src); got != c.want {
			t.Errorf("for %q:\n want: %q\n  got: %q", c.src, c.want, got)
		}
	}
}

// TestIfSpread covers if()'s spread-argument path, which cannot select lazily:
// both the true and false branches, plus the too-few-arguments error.
func TestIfSpread(t *testing.T) {
	wantCSS(t, "c{d: if(true, 1/2 null...)}", "c {\n  d: 0.5;\n}\n")
	wantCSS(t, "c{d: if(false, a b...)}", "c {\n  d: b;\n}\n")
	// A spread map supplies a named argument, exercising the named branch of the
	// argument picker together with the false-branch selection.
	wantCSS(t, "c{d: if(false, x, (\"if-false\": y)...)}", "c {\n  d: y;\n}\n")
	mustErr(t, "c{d: if(true...)}")
}

// TestModuloNonFinite covers SassScript "%" routed through the floored modulo,
// including the infinite-operand edges and the signed-zero results that only
// surface once divided into an infinity.
func TestModuloNonFinite(t *testing.T) {
	wantCSS(t, "a{b: 1px % calc(-infinity * 1px)}", "a {\n  b: calc(NaN * 1px);\n}\n")
	wantCSS(t, "a{b: -1px % calc(infinity * 1px)}", "a {\n  b: calc(NaN * 1px);\n}\n")
	wantCSS(t, "a{b: 1px % calc(infinity * 1px)}", "a {\n  b: 1px;\n}\n")
	// mod(-7, 7) is +0 (floored, positive divisor) so 1 / it is +infinity;
	// mod(7, -7) is -0 (negative divisor) so 1 / it is -infinity.
	wantCSS(t, "@use \"sass:math\";\na{b: math.div(1, mod(-7, 7))}", "a {\n  b: calc(infinity);\n}\n")
	wantCSS(t, "@use \"sass:math\";\na{b: math.div(1, mod(7, -7))}", "a {\n  b: calc(-infinity);\n}\n")
}

// TestSlashListSeparatorAndErrors covers the slash separator reported for a
// constructed slash list and the "too many slash elements" color error that a
// nested as-slash chain (3 / 4 / 5) must still raise.
func TestSlashListSeparatorAndErrors(t *testing.T) {
	wantCSS(t, "@use \"sass:list\";\na{b: list.separator(list.slash(x, y))}", "a {\n  b: slash;\n}\n")
	mustErr(t, "@use \"sass:color\";\n.a{v: color(srgb 1 2 3 / 4 / 5)}")
}

// TestInfinityMapKey covers Infinity comparing equal to itself so it works as a
// map key rather than being lost to a NaN difference.
func TestInfinityMapKey(t *testing.T) {
	wantCSS(t,
		"@use \"sass:map\";\n@use \"sass:math\";\n@use \"sass:meta\";\na{b: meta.inspect(map.get(((math.div(1, 0)): b), math.div(1, 0)))}",
		"a {\n  b: b;\n}\n")
}
