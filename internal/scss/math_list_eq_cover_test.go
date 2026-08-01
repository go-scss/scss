// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import "testing"

// TestUnitStrictEquality covers Number.equals's dart-compatible unit-strict
// behaviour: a unitless number never equals a number with units, convertible
// units compare after conversion, and incompatible units are unequal.
func TestUnitStrictEquality(t *testing.T) {
	cases := []struct{ in, out string }{
		// unitless vs unit -> false (hasUnits mismatch branch)
		{"a{b: 10 == 10px}", "a {\n  b: false;\n}\n"},
		{"a{b: 10px == 10}", "a {\n  b: false;\n}\n"},
		{"a{b: 10 != 10px}", "a {\n  b: true;\n}\n"},
		// convertible units -> compare after conversion
		{"a{b: 1in == 96px}", "a {\n  b: true;\n}\n"},
		{"a{b: 1cm == 10mm}", "a {\n  b: true;\n}\n"},
		// incompatible units -> false (convertUnits fail branch)
		{"a{b: 1px == 1s}", "a {\n  b: false;\n}\n"},
		// identical units and plain unitless equality still hold
		{"a{b: 10px == 10px}", "a {\n  b: true;\n}\n"},
		{"a{b: 5 == 5}", "a {\n  b: true;\n}\n"},
		// a number never equals a non-number
		{"a{b: 5 == c}", "a {\n  b: false;\n}\n"},
		// map keys use the same strict equality
		{"@use 'sass:map'; a{b: map.get((1px: x), 1)}", ""},
		{"@use 'sass:map'; a{b: map.get((1px: x), 1px)}", "a {\n  b: x;\n}\n"},
	}
	for _, c := range cases {
		expectEq(t, c.in, c.out)
	}
}

// TestUnitStringDenominator covers the pure-denominator unit serialisation
// (negative exponent form) added to Number.unitString.
func TestUnitStringDenominator(t *testing.T) {
	cases := []struct{ in, out string }{
		{"@use 'sass:math'; a{b: math.unit(math.div(1, 1px))}", "a {\n  b: \"px^-1\";\n}\n"},
		{"@use 'sass:math'; a{b: math.unit(math.div(math.div(math.div(1, 1px), 3em), 4rad))}", "a {\n  b: \"(px*em*rad)^-1\";\n}\n"},
		{"@use 'sass:math'; a{b: math.unit(math.div(1px, 1em))}", "a {\n  b: \"px/em\";\n}\n"},
		{"@use 'sass:math'; a{b: math.unit(1px * 1em)}", "a {\n  b: \"px*em\";\n}\n"},
		{"@use 'sass:math'; a{b: math.unit(1)}", "a {\n  b: \"\";\n}\n"},
	}
	for _, c := range cases {
		expectEq(t, c.in, c.out)
	}
}

// TestHypotUnits covers hypot's unit preservation, the unitless path and the
// zero-argument error.
func TestHypotUnits(t *testing.T) {
	expectEq(t,
		"@use 'sass:math'; a{b: math.hypot(3cm, 4mm * 10, 5q * 40, math.div(6in, 2.54), 7px * math.div(96, 2.54))}",
		"a {\n  b: 11.6189500386cm;\n}\n")
	expectEq(t, "@use 'sass:math'; a{b: math.hypot(3, 4)}", "a {\n  b: 5;\n}\n")
	mustErr(t, "@use 'sass:math'; a{b: math.hypot()}")
}

// TestRandomFunction covers every math.random branch: the float paths (no arg
// and explicit null), unit-ignoring integer limit, and each error case.
func TestRandomFunction(t *testing.T) {
	// no-arg and null both yield a float in [0, 1).
	expectEq(t, "@use 'sass:math'; a{b: math.random() >= 0 and math.random() < 1}", "a {\n  b: true;\n}\n")
	expectEq(t, "@use 'sass:math'; a{b: math.random(null) >= 0 and math.random(null) < 1}", "a {\n  b: true;\n}\n")
	// integer limit of 1 is deterministic; units are ignored.
	expectEq(t, "@use 'sass:math'; a{b: math.random(1)}", "a {\n  b: 1;\n}\n")
	expectEq(t, "@use 'sass:math'; a{b: math.random(1px)}", "a {\n  b: 1;\n}\n")
	expectEq(t, "@use 'sass:math'; a{b: math.random(1.0000000000001)}", "a {\n  b: 1;\n}\n")
	// a larger limit stays within range.
	expectEq(t, "@use 'sass:math'; a{b: math.random(100) > 0 and math.random(100) <= 100}", "a {\n  b: true;\n}\n")
	// named argument form.
	expectEq(t, "@use 'sass:math'; a{b: math.random($limit: 10) > 0}", "a {\n  b: true;\n}\n")
	// error branches.
	mustErr(t, "@use 'sass:math'; a{b: math.random(c)}")
	mustErr(t, "@use 'sass:math'; a{b: math.random(1.5)}")
	mustErr(t, "@use 'sass:math'; a{b: math.random(0)}")
	mustErr(t, "@use 'sass:math'; a{b: math.random(-1)}")
	// global random alias.
	expectEq(t, "a{b: random(1)}", "a {\n  b: 1;\n}\n")
}

// TestListSeparatorDefaulting covers asListVal's separator inference: a scalar
// and an empty map both coerce to undecided, an empty map reports space, and
// list.join adopts the other operand's separator.
func TestListSeparatorDefaulting(t *testing.T) {
	cases := []struct{ in, out string }{
		// scalar (undecided) adopts the second list's separator
		{"@use 'sass:list'; a{b: list.join(c, (d, e))}", "a {\n  b: c, d, e;\n}\n"},
		{"@use 'sass:list'; a{b: list.join(c, list.slash(d, e))}", "a {\n  b: c / d / e;\n}\n"},
		{"@use 'sass:list'; a{b: list.join(c, d e)}", "a {\n  b: c d e;\n}\n"},
		{"@use 'sass:list'; a{b: list.join(c, d)}", "a {\n  b: c d;\n}\n"},
		// non-empty map stays comma-separated
		{"@use 'sass:list'; a{b: list.join((x: 1), (2, 3))}", "a {\n  b: x 1, 2, 3;\n}\n"},
	}
	for _, c := range cases {
		expectEq(t, c.in, c.out)
	}
	// An empty map has an undecided separator, so list.separator reports space
	// and appending a value yields a decided space separator.
	empty := "@use 'sass:list'; @use 'sass:map';\n$m: map.remove((a: b), a);\n"
	expectEq(t, empty+"a{b: list.separator($m)}", "a {\n  b: space;\n}\n")
	expectEq(t, empty+"a{b: list.separator(list.join($m, (1, 2)))}", "a {\n  b: comma;\n}\n")
	expectEq(t, empty+"a{b: list.separator(list.join($m, 1 2))}", "a {\n  b: space;\n}\n")
}

// TestJoinBracketedAuto covers list.join's $bracketed handling: the literal
// "auto" inherits list1's bracketing while any other value is coerced to a
// boolean.
func TestJoinBracketedAuto(t *testing.T) {
	cases := []struct{ in, out string }{
		{"@use 'sass:list'; a{b: list.join(c d, e f, $bracketed: auto)}", "a {\n  b: c d e f;\n}\n"},
		{"@use 'sass:list'; a{b: list.join([c d], e f, $bracketed: auto)}", "a {\n  b: [c d e f];\n}\n"},
		{"@use 'sass:list'; a{b: list.join(c d, e f, $bracketed: true)}", "a {\n  b: [c d e f];\n}\n"},
		{"@use 'sass:list'; a{b: list.join([c d], e f, $bracketed: false)}", "a {\n  b: c d e f;\n}\n"},
	}
	for _, c := range cases {
		expectEq(t, c.in, c.out)
	}
}

// TestUniqueIDDistinct covers fnUniqueID: consecutive calls return different
// unquoted identifiers.
func TestUniqueIDDistinct(t *testing.T) {
	okCompile(t, `@use 'sass:map';
@use 'sass:string';
$ids: ();
@for $i from 1 through 50 {
  $id: string.unique-id();
  @if map.has-key($ids, $id) { @error "collision"; }
  $ids: map.merge($ids, ($id: null));
}`)
}
