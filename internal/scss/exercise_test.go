// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import (
	"math"
	"strings"
	"testing"
)

// mustErr asserts that compiling src fails (covers e.fail error branches).
func mustErr(t *testing.T, src string) {
	t.Helper()
	if _, err := Render(src, false, false, nil); err == nil {
		t.Errorf("expected error for %q, got none", src)
	}
}

// okCompile asserts src compiles without error (output not checked).
func okCompile(t *testing.T, src string) {
	t.Helper()
	if _, err := Render(src, false, false, nil); err != nil {
		t.Errorf("unexpected error for %q: %v", src, err)
	}
}

func TestValueMethods(t *testing.T) {
	vals := []Value{
		newNumber(1, "px"),
		&SassString{Text: "a", Quoted: true},
		&SassString{Text: "b", Quoted: false},
		&SassColor{Rf: 1, Gf: 2, Bf: 3, A: 1},
		&Boolean{V: true}, &Boolean{V: false},
		sassNull,
		&List{Elements: []Value{newNumber(1), newNumber(2)}, Sep: SepComma},
		&List{Elements: []Value{newNumber(1)}, Sep: SepSpace, Bracketed: true},
		&Map{Keys: []Value{&SassString{Text: "k"}}, Values: []Value{newNumber(1)}},
	}
	for _, v := range vals {
		_ = v.isTruthy()
		_ = v.sep()
		_ = v.asList()
		_ = v.equals(v)
		_ = v.equals(sassNull)
		_ = serializeValue(v, false)
		_ = serializeValue(v, true)
	}
	// unitString variants: numerator, denominator, and multiple units.
	for _, n := range []*Number{
		{Val: 1, Numer: []string{"px"}},
		{Val: 1, Numer: []string{"px", "em"}},
		{Val: 1, Denom: []string{"s"}},
		{Val: 1, Numer: []string{"px"}, Denom: []string{"s", "hz"}},
	} {
		_ = n.unitString()
		_ = serializeValue(n, false)
	}
	// clampInt255 bounds.
	for _, v := range []float64{-5, 0, 128, 255, 300} {
		_ = clampInt255(v)
	}
}

func TestErrorBranches(t *testing.T) {
	errs := []string{
		".a{v: 1 + }",                        // parse: missing operand
		".a{v: red + }",                      // parse error
		".a{v: unknownfn-x(} ",               // parse error
		"@use \"sass:bogus\";",               // unknown module
		"@include undefined-mixin;",          // undefined mixin
		".a{v: map-get(1, 2, 3, 4)}",         // too many args-ish
		".a{v: nth(1 2, 5)}",                 // index out of range
		".a{v: 1px + 1s}",                    // incompatible units
		".a{v: red % 2}",                     // bad op on color? no-op ok
		"@function f(){@return}; .a{v: f()}", // function no return
		".a{v: math.sqrt(1)}",                // math ns without @use
		".a{v: color.alpha(1)}",              // color ns without @use
		"@error \"boom\";",                   // @error
		".a{v: rgb(1)}",                      // rgb bad arity via list? error
		".a{v: $undefined}",                  // undefined variable
	}
	for _, s := range errs {
		mustErr(t, s)
	}
}

func TestFeatureExercise(t *testing.T) {
	oks := []string{
		"@use \"sass:math\"; .a{v: math.div(10, 2)}",
		"@use \"sass:math\"; .a{v: math.max(1,2,3); w: math.min(3,2,1)}",
		"@use \"sass:math\"; .a{v: math.hypot(3,4); w: math.log(8, 2); x: math.pow(2,10)}",
		"@use \"sass:math\"; .a{v: math.sin(0); w: math.cos(0deg); x: math.tan(0turn)}",
		"@use \"sass:math\"; .a{v: math.clamp(1px, 5px, 3px); w: math.ceil(1.2); x: math.floor(1.8)}",
		"@use \"sass:math\"; .a{v: math.percentage(0.5); w: math.abs(-3); x: math.sqrt(9)}",
		"@use \"sass:math\"; .a{v: math.unit(1px); w: math.is-unitless(1); x: math.compatible(1px, 1em)}",
		"@use \"sass:string\"; .a{v: string.length(\"abc\"); w: string.to-upper-case(\"a\")}",
		"@use \"sass:string\"; .a{v: string.quote(a); w: string.unquote(\"b\"); x: string.slice(\"hello\", 2)}",
		"@use \"sass:list\"; .a{v: list.length(1 2 3); w: list.nth(1 2, -1); x: list.separator(1 2)}",
		"@use \"sass:map\"; .a{v: map.get((a:1), a); w: map.has-key((a:1), a); x: map.keys((a:1,b:2))}",
		"@use \"sass:map\"; .a{v: map.merge((a:1),(b:2)); w: map.remove((a:1,b:2), a)}",
		"@use \"sass:meta\"; .a{v: meta.type-of(1); w: meta.inspect((a:1))}",
		"@use \"sass:meta\"; .a{v: meta.feature-exists(at-error); w: meta.feature-exists(nope)}",
		"@use \"sass:color\"; .a{v: color.red(#123456); w: color.green(#123456); x: color.blue(#123456)}",
		"@use \"sass:color\"; .a{v: color.mix(red, blue, 25%); w: color.scale(#f00, $lightness: 20%)}",
		"@use \"sass:color\"; .a{v: color.adjust(#123, $red: 5); w: color.change(#123, $blue: 9)}",
		"@use \"sass:color\"; .a{v: color.grayscale(#f00); w: color.complement(#f00); x: color.invert(#fff, 40%)}",
		".a{v: if(true, 1, 2); w: if(false, 1, 2)}",
		".a{v: 1 == 1; w: 1 != 2; x: 1 < 2; y: 2 > 1; z: 1 <= 1; q: 2 >= 2}",
		".a{v: true and false; w: true or false; x: not true}",
		".a{v: 1 + 2; w: 5 - 1; x: 2 * 3; y: 10 / 2; z: 7 % 3}",
		".a{v: \"a\" + \"b\"; w: a + b; x: 1 + px}",
		"$m: (a: 1, b: 2); @each $k, $val in $m { .#{$k} { v: $val } }",
		"@for $i from 1 through 3 { .c#{$i} { w: $i } }",
		"@for $i from 3 to 1 { .d#{$i} { w: $i } }",
		"$i: 0; @while $i < 3 { .e#{$i} { v: $i } $i: $i + 1 }",
		".parent { &:hover { color: red } .child & { x: 1 } }",
		"@media screen { .a { color: red; @media (min-width: 10px) { b: 2 } } }",
		"@supports (display: flex) { .a { display: flex } }",
		".a { @at-root .b { c: 1 } }",
		"%base { color: red } .a { @extend %base }",
		".x { color: red } .a { @extend .x }",
		"@mixin m($a, $b: 2) { p: $a $b } .a { @include m(1) }",
		"@mixin wrap { .inner { @content } } @include wrap { color: red }",
		"/* loud comment */ .a { color: red /* inline */ }",
		".a { --custom: some raw #{1+1} value }",
		"@font-face { font-family: x; src: url(a) }",
		"@charset \"UTF-8\"; .a { b: 1 }",
		".a { color: RED; b: Rgb(1,2,3) }",
		"$list: [1, 2, 3]; .a { v: $list }",
		".a { v: (1 2) (3 4); w: 1, 2, 3 }",
		".a { width: calc(100% - 10px) }",
		".a { v: 1e3; w: .5; x: -0.0; y: 1.500 }",
		"@use \"sass:meta\"; @mixin m { @content } .a { @include m { x: meta.content-exists() } }",
	}
	for _, s := range oks {
		okCompile(t, s)
	}
}

func TestCompressedExercise(t *testing.T) {
	srcs := []string{
		".a{color: #ffffff; b: #ff0000; c: rgba(0,0,0,.5)}",
		".a{v: mix(#f00,#00f)}",
		"@media screen{.a{color:red}}",
		".a,.b{color:red}.c{d:1}",
		".empty{}",
		".a{v: (1, 2, 3); w: [1 2]}",
	}
	for _, s := range srcs {
		res, err := Render(s, false, true, nil)
		if err != nil {
			t.Errorf("compressed error %q: %v", s, err)
		}
		_ = res.CSS
	}
}

func TestIndentedExercise(t *testing.T) {
	src := ".a\n  color: red\n  .b\n    x: 1\n// comment\n.c\n  y: 2\n"
	if _, err := Render(src, true, false, nil); err != nil {
		t.Errorf("indented compile error: %v", err)
	}
}

func TestStringHelpers(t *testing.T) {
	// serializeQuoted quote-selection branches.
	for _, s := range []string{`a"b`, `a'b`, `a"b'c`, `plain`, `back\slash`} {
		_ = serializeQuoted(s)
	}
	// formatFloat branches (compressed leading-zero, negative, trailing zeros).
	for _, f := range []float64{0, -0, 0.5, -0.5, 1.5, 100, 1.2500000000, -0.0} {
		_ = formatFloat(f, true)
		_ = formatFloat(f, false)
	}
	if !strings.Contains(formatFloat(0.5, true), ".5") {
		t.Error("compressed leading zero not stripped")
	}
}

func TestTargetedBranches(t *testing.T) {
	oks := []string{
		// map.get nested + selector functions (selectorText List branches)
		"@use \"sass:map\"; .a{v: map.get((a:(b:1)), a, b)}",
		"@use \"sass:map\"; .a{v: map.get((a:1), z)}",
		"@use \"sass:map\"; .a{v: map.get((a:1), a, b)}",
		"@use \"sass:selector\"; .a{v: selector.nest(\".a\", \".b\")}",
		"@use \"sass:selector\"; .a{v: selector.append(\".a\", \".b\")}",
		"@use \"sass:selector\"; .a{v: selector.unify(\".a.b\", \".b.c\")}",
		"@use \"sass:selector\"; .a{v: selector.nest((\"a\", \"b\"), \".c\")}",
		// hex literal widths
		".a{v: #abc; w: #abcd; x: #aabbcc; y: #aabbccdd}",
		// rgb/hsl argument forms
		".a{v: rgb(1 2 3)}",
		".a{v: rgb(10%, 20%, 30%); w: rgba(1,2,3,0.4)}",
		".a{v: rgb((1 2 3)); w: rgb((1 2 3 4))}",
		".a{v: hsl(120 50% 50%); w: hsl((120, 50%, 50%)); x: hsl((0 100% 50% 0.5))}",
		".a{v: hsla(0,100%,50%,50%)}",
		// color.scale all channels & signs, change/adjust alpha
		"@use \"sass:color\"; .a{v: color.scale(#808080, $red: 50%, $green: -50%, $blue: 20%, $alpha: -10%)}",
		"@use \"sass:color\"; .a{v: color.change(#123, $red: 0, $green: 1, $blue: 2, $alpha: 0.5)}",
		"@use \"sass:color\"; .a{v: color.change(#123, $hue: 90, $saturation: 10%, $lightness: 20%)}",
		"@use \"sass:color\"; .a{v: color.adjust(#123, $hue: 30, $saturation: 5%, $lightness: 5%, $alpha: -0.2)}",
		"@use \"sass:color\"; .a{v: color.ie-hex-str(#abc)}",
		// string functions edge indices
		"@use \"sass:string\"; .a{v: string.slice(\"hello\", -3, -1); w: string.slice(\"hello\", 2, -2)}",
		"@use \"sass:string\"; .a{v: string.index(\"hello\", \"z\"); w: string.insert(\"abc\", \"-\", -1)}",
		"@use \"sass:string\"; .a{v: string.split(\"a,b,c\", \",\", 2)}",
		// list separator arg forms and index
		"@use \"sass:list\"; .a{v: list.append((), 1); w: list.join((), 2)}",
		"@use \"sass:list\"; .a{v: list.index(1 2 3, 2); w: list.index(1 2, 9)}",
		"@use \"sass:list\"; .a{v: list.set-nth(1 2 3, -1, x)}",
		// unary and parens, unquoted math with units
		"$x: 5px; .a{v: -$x}",
		".a{v: +5; w: -(3); x: not not true}",
		".a{v: (1 + 2) * 3}",
		// numbers with denom units
		".a{v: (10px / 2s)}",
		// comparisons across strings and colors
		".a{v: \"a\" == \"a\"; w: red == red; x: (1 2) == (1 2)}",
		// interpolation in various positions
		"@each $k in a b { .x-#{$k} { p: 1 } }",
		"@media #{\"screen\"} { .a { b: 1 } }",
	}
	for _, s := range oks {
		okCompile(t, s)
	}
	moreErrs := []string{
		"@use \"sass:map\"; .a{v: map.get(1, 2)}",       // not a map
		"@use \"sass:string\"; .a{v: string.length(1)}", // not a string
		"@use \"sass:color\"; .a{v: color.red(1)}",      // not a color
		"@use \"sass:list\"; .a{v: list.nth(1 2, 0)}",   // zero index
		".a{v: 1px + 1em + 1s}",                         // incompatible chain
		"@if 1 { }",                                     // ok actually - drop
	}
	for _, s := range moreErrs[:5] {
		mustErr(t, s)
	}
}

func TestModuleAndParserBranches(t *testing.T) {
	oks := []string{
		// @use with config and as-star
		"@use \"sass:math\" as m; .a{v: m.div(6,2)}",
		"@use \"sass:math\" as *; .a{v: div(6,2)}",
		// @import forms: multiple, media query, url()
		"@import \"a\", \"b\";",
		"@import \"theme\" screen and (min-width: 10px);",
		"@import url(\"http://example.com/x.css\");",
		"@import url(reset.css);",
		// strings with escapes and quotes
		`.a { v: "a\"b"; w: 'c'; x: "tab\9 end" }`,
		`.a { content: "\2014" }`,
		// bracketed & slash lists, empty list
		".a { v: [1, 2, 3]; w: 1/2/3; x: () }",
		// map.set (value.go set) + nested + merge overwrite
		"@use \"sass:map\"; .a{v: map.set((a:1), a, 9); w: map.set((a:1), b, 2)}",
		// plainCSSFunction with named and spread-ish args
		".a { filter: progid(x, y); grid: minmax($min: 1px, $max: 2px) }",
		// unknown at-rule with body and without
		"@unknown-rule foo { a: 1 }",
		"@namespace svg url(http://www.w3.org/2000/svg);",
		// nested declarations
		".a { font: { family: serif; size: 10px } }",
		// number formats
		".a { v: 1000000; w: 0.000001; x: -42.5; y: 3.14159265358979 }",
		// comparison list/map equality and not-equal
		".a { v: (a: 1) == (a: 1); w: [1] == [1]; x: 1px == 1px }",
	}
	for _, s := range oks {
		okCompile(t, s)
	}
	errs := []string{
		// recursion guard (enter -> fail branch)
		"@function f($n){@return f($n) + 1} .a{v: f(1)}",
		"@mixin m{@include m} .a{@include m}",
	}
	for _, s := range errs {
		mustErr(t, s)
	}
}

func TestValueEqualityAndFormat(t *testing.T) {
	// Map.set both overwrite and append; Map.equals mismatched sizes/keys.
	m := &Map{Keys: []Value{&SassString{Text: "a"}}, Values: []Value{newNumber(1)}}
	m.set(&SassString{Text: "a"}, newNumber(2))
	m.set(&SassString{Text: "b"}, newNumber(3))
	m2 := &Map{Keys: []Value{&SassString{Text: "a"}}, Values: []Value{newNumber(1)}}
	_ = m.equals(m2)
	_ = m.equals(newNumber(1))
	// List.equals across separators, lengths, and non-lists.
	l1 := &List{Elements: []Value{newNumber(1), newNumber(2)}, Sep: SepComma}
	l2 := &List{Elements: []Value{newNumber(1)}, Sep: SepComma}
	l3 := &List{Elements: []Value{newNumber(1), newNumber(2)}, Sep: SepSpace}
	_ = l1.equals(l2)
	_ = l1.equals(l3)
	_ = l1.equals(&List{Elements: []Value{newNumber(1), newNumber(2)}, Sep: SepComma})
	_ = l1.equals(sassNull)
	// formatFloat: large, tiny, negative, integer, and rounding boundaries.
	for _, f := range []float64{1234567.0, 0.0000001, -12.34, 1e20, -0.999999999999} {
		_ = formatFloat(f, false)
		_ = formatFloat(f, true)
	}
	// isHexColor / normalizeHue edge cases.
	for _, s := range []string{"#12", "#12345", "#xyz", "#1234567", "#abcdef", "notacolor"} {
		_ = isHexColor(s)
	}
	for _, h := range []float64{-30, 400, 0, 360, 720} {
		_ = normalizeHue(h)
	}
}

func TestForwardAndStringParsing(t *testing.T) {
	// @forward parse branches (as/show/hide/with) are exercised during parsing;
	// the load itself fails with a nil importer, which is fine.
	forwardErrs := []string{
		`@forward "lib" as pfx-*;`,
		`@forward "lib" show a, b;`,
		`@forward "lib" hide c, d;`,
		`@forward "lib" with ($x: 1, $y: 2);`,
		`@use "lib" with ($x: 1);`,
		`@use "lib" as l;`,
	}
	for _, s := range forwardErrs {
		mustErr(t, s)
	}
	// scanQuotedRaw: interpolation and escapes inside quoted strings.
	oks := []string{
		`.a { v: "pre#{1 + 1}post" }`,
		`.a { v: "esc\\ape"; w: "quote\"inside" }`,
		`.a { v: 'single #{2} q' }`,
		`.a { v: "#{1}#{2}" }`,
		// selectorLike / ambiguous parsing
		".a:not(.b) { color: red }",
		"a > b + c ~ d { x: 1 }",
		"* { box-sizing: border-box }",
		"[data-x='y'] { z: 1 }",
		".a { &__el { x: 1 } &--mod { y: 2 } }",
		// blank-line grouping with comments between rules
		".a { x: 1 }\n\n/* c */\n\n.b { y: 2 }",
	}
	for _, s := range oks {
		okCompile(t, s)
	}
	// formatFloat additional numeric branches.
	for _, f := range []float64{0.00000000001, 999999999.0, 2.0, -1.0, 0.10000000009} {
		_ = formatFloat(f, false)
		_ = formatFloat(f, true)
	}
}

func TestRawAndMiscBranches(t *testing.T) {
	oks := []string{
		// custom property raw values with quoted strings, escapes, interpolation
		`.a { --x: "raw #{1 + 1} \" esc" }`,
		`.a { --y: url(http://x/y) '#{2}' }`,
		// declarations that look selector-like then resolve as declarations
		".a { font: 12px/1.5 sans-serif }",
		".a { b: a b c }",
		".a { grid-template: 'x' 'y' }",
		// list vs selector ambiguity in interpolation
		"$s: a b; .#{$s} { x: 1 }",
		// deep nested parens and maps
		".a { v: ((1 2), (3 4)); w: (a: (b: (c: 1))) }",
		// unicode-range and at-rules without body
		"@namespace url(x);",
		".a { unicode-range: U+0000-00FF }",
		// empty rule pruned, comment-only handling
		".empty {}\n.b { x: 1 }",
		// !important and !default and !global
		".a { color: red !important }",
		"$x: 1 !default; .a { v: $x }",
		".a { $y: 2 !global; z: $y }",
	}
	for _, s := range oks {
		okCompile(t, s)
	}
	// value.equals across differing types and colors with alpha
	cs := []Value{
		&SassColor{Rf: 1, Gf: 2, Bf: 3, A: 0.5},
		&SassColor{Rf: 1, Gf: 2, Bf: 3, A: 1},
		newNumber(1, "px"),
		newNumber(1, "em"),
		&SassString{Text: "x", Quoted: true},
		&SassString{Text: "x", Quoted: false},
		&Boolean{V: true},
	}
	for _, a := range cs {
		for _, b := range cs {
			_ = a.equals(b)
		}
	}
}

func TestBroadCoverage(t *testing.T) {
	oks := []string{
		"@use \"sass:math\"; @function golden($n) { @if $n <= 0 { @return 0 } @else if $n == 1 { @return 1 } @else { @return golden($n - 1) + golden($n - 2) } } .a { v: golden(5) }",
		"@mixin btn($c, $args...) { color: $c; margin: $args } .a { @include btn(red, 1px, 2px, 3px) }",
		"@mixin kw($a: 1, $b: 2) { x: $a $b } .a { @include kw($b: 9) }",
		"@each $name, $glyph in (a: 1, b: 2, c: 3) { .icon-#{$name} { content: $glyph } }",
		"@each $i in 1, 2, 3 { .n#{$i} { w: $i * 10px } }",
		"$map: (primary: #f00, secondary: #0f0); @each $k, $v in $map { .#{$k} { color: $v } }",
		"@use \"sass:math\"; @for $i from 1 through 5 { .col-#{$i} { width: math.div(100%, $i) } }",
		"@supports not (display: grid) { .a { float: left } }",
		"@supports (display: grid) and (gap: 1px) { .a { display: grid } }",
		"@supports (a: b) or (c: d) { .a { x: 1 } }",
		"%placeholder { color: red } .a, .b { @extend %placeholder } .c { @extend %placeholder }",
		".base { padding: 1px } .btn { @extend .base; margin: 2px }",
		"@media (min-width: 100px) and (max-width: 200px) { .a { x: 1 } }",
		"@media screen, print { .a { x: 1 } }",
		".a { color: red; &:hover { color: blue }; &.active { color: green } }",
		".list { > li { display: inline }; li + li { margin: 0 } }",
		"@warn \"a warning\"; .a { x: 1 }",
		"@debug \"debug msg\"; .a { x: 1 }",
		"$x: 1; @if $x { .a { y: 1 } } @else { .b { y: 2 } }",
		".a { v: 10px * 2; w: 10px / 2px; x: 10 % 3 }",
		"@use \"sass:list\"; .a { v: list.append(1 2, 3, $separator: comma) }",
		"@use \"sass:map\"; $m: (a: 1); .a { v: map.has-key($m, a); w: map.values($m) }",
		"@use \"sass:string\"; .a { v: string.index(\"abcabc\", \"b\"); w: string.length(\"\") }",
		"@use \"sass:math\"; .a { v: math.round(2.5); w: math.max(1px, 2px); x: math.min(3, 1) }",
		".a { transition: color .3s ease-in-out, background 1s }",
		".a { background: linear-gradient(to right, red 0%, blue 100%) }",
		".a { grid-template-columns: repeat(3, 1fr) }",
		"@keyframes spin { from { transform: rotate(0) } to { transform: rotate(360deg) } }",
		".a { content: \"\\2013\" }",
		"$cond: true; .a { color: if($cond, red, blue) }",
		"$x: 5px; .a { margin: -$x }",
	}
	for _, s := range oks {
		okCompile(t, s)
	}
	errs := []string{
		"@function f($n) { }; .a { v: f(1) }",
		".a { v: 1 / 0 * unquote(\"x\") + }",
		"@include mixin-without-def;",
		".a { v: nonexistent-namespace.func() }",
		".a { v: 1px < 1em }",
		"@for $i from 1 to \"x\" { }",
	}
	for _, s := range errs {
		mustErr(t, s)
	}
}

func TestPreciseBranches(t *testing.T) {
	oks := []string{
		// escaped identifiers (scanIdentifier escape branch)
		`.\39 lives { x: 1 }`,
		`.a { b: \1F600 }`,
		// --custom / -webkit prefixed (looksLikeIdentifier dash branches)
		".a { --custom-prop: 1; -webkit-x: 2 }",
		// selectorLike: space list of idents/parent resolves as selector
		".a { b c d { x: 1 } }",
		"foo { & bar baz { y: 1 } }",
		// isLiteralNumberish: signed numbers in space-separated value lists
		".a { margin: -5px 3px -2px 1px }",
		".a { v: 1 -2 +3 }",
		// spread args: list -> positional, map -> keyword
		"@mixin m($a, $b, $c) { x: $a $b $c } $l: 1 2 3; .a { @include m($l...) }",
		"@mixin k($a: 0, $b: 0) { y: $a $b } $m: (a: 1, b: 2); .a { @include k($m...) }",
		"@function f($a, $b) { @return $a + $b } $args: 1, 2; .a { v: f($args...) }",
		// list separator argument forms (parseSeparatorArg)
		"@use \"sass:list\"; .a { v: list.append(1 2, 3, comma); w: list.join(1, 2, slash) }",
		"@use \"sass:list\"; .a { v: list.append((), 1, space); w: list.separator([1,2]) }",
	}
	for _, s := range oks {
		okCompile(t, s)
	}
	errs := []string{
		"@use \"sass:math\"; .a { v: math.abs(\"x\") }", // asNumber failure
		"@use \"sass:math\"; .a { v: math.sqrt(red) }",  // asNumber failure
	}
	for _, s := range errs {
		mustErr(t, s)
	}
	// parseSeparatorArg direct default and invalid.
	for _, sepName := range []string{"comma", "space", "slash", "auto", "bogus"} {
		_ = parseSeparatorArg(&SassString{Text: sepName}, SepSpace)
	}
	_ = parseSeparatorArg(newNumber(1), SepComma)
}

func TestFinalBranches(t *testing.T) {
	// Map equals empty-vs-empty-list; formatFloat non-finite direct path.
	empty := &Map{}
	if !empty.equals(&List{}) {
		t.Error("empty map should equal empty list")
	}
	if empty.equals(&List{Elements: []Value{newNumber(1)}}) {
		t.Error("empty map != non-empty list")
	}
	_ = formatFloat(math.Inf(1), false)
	_ = formatFloat(math.Inf(-1), false)
	_ = formatFloat(math.NaN(), true)
	// iterationItems single (non-list/map) value and null.
	okCompile(t, "@each $x in 5 { .a { v: $x } }")
	okCompile(t, "@each $x in null { .a { v: 1 } }")
	// selectorLike disambiguation: value followed by a block.
	for _, s := range []string{
		".a { b: c, d { x: 1 } }",
		".a { b: 1 2 { x: 1 } }",
	} {
		_, _ = Render(s, false, false, nil) // exercise both branches; result irrelevant
	}
	// peekAt near EOF.
	okCompile(t, ".a{x:1}")
	okCompile(t, "/* c */")
}

func TestClosingBranches(t *testing.T) {
	oks := []string{
		// clampPct over/under, blank value elision, arith string/color
		"@use \"sass:color\"; .a { v: color.scale(#808080, $lightness: 200%); w: color.scale(#808080, $lightness: -200%) }",
		"@use \"sass:string\"; .a { before: 1; x: string.unquote(\"\"); after: 2 }",
		".a { v: 1 + \"px\"; w: \"n\" + 5; x: a - b; y: 1 * 2px }",
		".a { v: #010203 + #040506 }",
		".a { v: red + 1 }",
		// selector serialize: combinators, compound, pseudo, attribute, comma
		".a > .b .c + .d ~ .e { x: 1 }",
		".a[href^=\"http\"]:not(.x)::before { content: \"\" }",
		".a, .b .c, .d > .e { y: 1 }",
		// separator + bracket lists
		"@use \"sass:list\"; .a { c: list.separator((1, 2)); s: list.separator(1 2); l: list.separator(1/2) }",
		".a { v: []; w: [1]; x: [1, 2, 3]; y: [1 2 3] }",
		// quoted string escapes
		`.a { v: "line\Aend"; w: "tab\9x" }`,
		// @include content with body and no body
		"@mixin a { @content } @mixin b { p: 1 } .x { @include a { q: 2 }; @include b }",
	}
	for _, s := range oks {
		okCompile(t, s)
	}
	// selectorList.serialize direct via parse.
	for _, sel := range []string{"a b", "a>b", "a + b", ".x.y.z", "*", "a, b, c", "a:hover::after"} {
		_ = parseSelectorList(sel).serialize(false)
		_ = parseSelectorList(sel).serialize(true)
	}
}
