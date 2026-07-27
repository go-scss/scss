// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import (
	"strings"
	"testing"
)

func compile(t *testing.T, src string) string {
	t.Helper()
	res, err := Render(src, false, false, nil)
	if err != nil {
		t.Fatalf("compile error for %q: %v", src, err)
	}
	return res.CSS
}

func compileC(t *testing.T, src string) string {
	t.Helper()
	res, err := Render(src, false, true, nil)
	if err != nil {
		t.Fatalf("compile error for %q: %v", src, err)
	}
	return res.CSS
}

func expectEq(t *testing.T, src, want string) {
	t.Helper()
	got := compile(t, src)
	if got != want {
		t.Errorf("for %q:\n want: %q\n  got: %q", src, want, got)
	}
}

func TestBasicFeatures(t *testing.T) {
	cases := []struct{ in, out string }{
		{".a{color:red}", ".a {\n  color: red;\n}\n"},
		{"$x:1px; .a{w:$x}", ".a {\n  w: 1px;\n}\n"},
		{".a{.b{x:1}}", ".a .b {\n  x: 1;\n}\n"},
		{".a{&:hover{x:1}}", ".a:hover {\n  x: 1;\n}\n"},
		{".a{w: 1 + 2}", ".a {\n  w: 3;\n}\n"},
		{".a{w: 2px * 3}", ".a {\n  w: 6px;\n}\n"},
		{".a{w: 10px - 4px}", ".a {\n  w: 6px;\n}\n"},
		{".a{w: 6 % 4}", ".a {\n  w: 2;\n}\n"},
		{".a{w: \"a\" + \"b\"}", ".a {\n  w: \"ab\";\n}\n"},
		{".a{w: a + b}", ".a {\n  w: ab;\n}\n"},
		{".a{w: true and false}", ".a {\n  w: false;\n}\n"},
		{".a{w: true or false}", ".a {\n  w: true;\n}\n"},
		{".a{w: not true}", ".a {\n  w: false;\n}\n"},
		{".a{w: 1 == 1}", ".a {\n  w: true;\n}\n"},
		{".a{w: 1 < 2}", ".a {\n  w: true;\n}\n"},
		{".a{w: 3 >= 2}", ".a {\n  w: true;\n}\n"},
		{".a{w: 1 != 2}", ".a {\n  w: true;\n}\n"},
		{"$x:5px; .a{w: -$x}", ".a {\n  w: -5px;\n}\n"},
		{".a{w: null}", ""},
		{".a{w: 1 2 3}", ".a {\n  w: 1 2 3;\n}\n"},
		{".a{w: (1, 2, 3)}", ".a {\n  w: 1, 2, 3;\n}\n"},
		{".a{w: [1 2]}", ".a {\n  w: [1 2];\n}\n"},
	}
	for _, c := range cases {
		expectEq(t, c.in, c.out)
	}
}

func TestControlFlow(t *testing.T) {
	if got := compile(t, ".a{@if false{x:1}@else if false{x:2}@else{x:3}}"); !strings.Contains(got, "x: 3") {
		t.Errorf("else branch: %q", got)
	}
	if got := compile(t, ".a{@for $i from 1 to 3{m#{$i}:$i}}"); !strings.Contains(got, "m1: 1") || strings.Contains(got, "m3") {
		t.Errorf("for-to: %q", got)
	}
	if got := compile(t, ".a{@for $i from 3 through 1{m#{$i}:$i}}"); !strings.Contains(got, "m3: 3") {
		t.Errorf("for-through-descending: %q", got)
	}
	if got := compile(t, "$i:0; .a{@while $i<2{m#{$i}:1; $i:$i+1}}"); !strings.Contains(got, "m0") || !strings.Contains(got, "m1") {
		t.Errorf("while: %q", got)
	}
	if got := compile(t, ".a{@each $x in a b {p:$x}}"); !strings.Contains(got, "p: a") {
		t.Errorf("each: %q", got)
	}
	if got := compile(t, "$m:(a:1,b:2); .a{@each $k,$v in $m {#{$k}:$v}}"); !strings.Contains(got, "a: 1") {
		t.Errorf("each-map: %q", got)
	}
}

func TestMixinsAndFunctions(t *testing.T) {
	expectEq(t, "@mixin m($a,$b:2){x:$a;y:$b} .c{@include m(1)}", ".c {\n  x: 1;\n  y: 2;\n}\n")
	expectEq(t, "@mixin m{@content} .c{@include m{z:9}}", ".c {\n  z: 9;\n}\n")
	expectEq(t, "@function f($n){@return $n*2} .c{w:f(3)}", ".c {\n  w: 6;\n}\n")
	expectEq(t, "@mixin m($a...){x:$a} .c{@include m(1,2,3)}", ".c {\n  x: 1, 2, 3;\n}\n")
	expectEq(t, "@mixin m($a,$b){x:$a $b} .c{@include m($b:2,$a:1)}", ".c {\n  x: 1 2;\n}\n")
}

func TestExtend(t *testing.T) {
	got := compile(t, "%p{x:1} .a{@extend %p} .b{@extend %p}")
	if !strings.Contains(got, ".b, .a") {
		t.Errorf("extend order: %q", got)
	}
	got = compile(t, ".base{x:1} .a{@extend .base}")
	if !strings.Contains(got, ".base, .a") {
		t.Errorf("extend class: %q", got)
	}
	// optional extend of missing target should not error
	if _, err := Render(".a{@extend %missing !optional}", false, false, nil); err != nil {
		t.Errorf("optional extend errored: %v", err)
	}
}

func TestBuiltins(t *testing.T) {
	cases := []struct{ in, want string }{
		{".a{w:percentage(0.5)}", "50%"},
		{".a{w:round(1.6)}", "2"},
		{".a{w:ceil(1.1)}", "2"},
		{".a{w:floor(1.9)}", "1"},
		{".a{w:abs(-3)}", "3"},
		{".a{w:max(1,5,3)}", "5"},
		{".a{w:min(4,2,8)}", "2"},
		{".a{w:length(1 2 3)}", "3"},
		{".a{w:nth(a b c, 2)}", "b"},
		{".a{w:type-of(1)}", "number"},
		{".a{w:type-of(a)}", "string"},
		{".a{w:type-of(#fff)}", "color"},
		{".a{w:unquote(\"x\")}", "x"},
		{".a{w:quote(x)}", "\"x\""},
		{".a{w:to-upper-case(\"ab\")}", "\"AB\""},
		{".a{w:str-length(\"abc\")}", "3"},
		{".a{w:map-get((a:1,b:2), b)}", "2"},
		{".a{w:if(true, 1, 2)}", "1"},
		{".a{w:red(#ff0000)}", "255"},
		{".a{w:unit(5px)}", "\"px\""},
		{".a{w:unitless(5)}", "true"},
		{".a{w:comparable(1px, 1cm)}", "true"},
		{".a{w:list-separator((1,2))}", "comma"},
		{".a{w:index(a b c, b)}", "2"},
		{".a{w:map-has-key((a:1), a)}", "true"},
	}
	for _, c := range cases {
		got := compile(t, c.in)
		if !strings.Contains(got, c.want) {
			t.Errorf("for %q want contains %q, got %q", c.in, c.want, got)
		}
	}
}

func TestModules(t *testing.T) {
	expectEq(t, "@use \"sass:math\"; .a{w:math.div(10,4)}", ".a {\n  w: 2.5;\n}\n")
	expectEq(t, "@use \"sass:math\" as m; .a{w:m.max(1,2)}", ".a {\n  w: 2;\n}\n")
	expectEq(t, "@use \"sass:string\"; .a{w:string.length(\"ab\")}", ".a {\n  w: 2;\n}\n")
	expectEq(t, "@use \"sass:list\"; .a{w:list.nth(a b, 1)}", ".a {\n  w: a;\n}\n")
	expectEq(t, "@use \"sass:map\"; .a{w:map.get((x:9), x)}", ".a {\n  w: 9;\n}\n")
	expectEq(t, "@use \"sass:meta\"; .a{w:meta.type-of(1)}", ".a {\n  w: number;\n}\n")
}

func TestColorOutput(t *testing.T) {
	expectEq(t, ".a{c:red}", ".a {\n  c: red;\n}\n")
	expectEq(t, ".a{c:#ff0000}", ".a {\n  c: #ff0000;\n}\n")
	if got := compileC(t, ".a{c:#ffffff}"); got != ".a{c:#fff}\n" {
		t.Errorf("compressed white: %q", got)
	}
	if got := compileC(t, ".a{c:#ff0000}"); got != ".a{c:red}\n" {
		t.Errorf("compressed red: %q", got)
	}
	expectEq(t, ".a{c:rgb(1,2,3)}", ".a {\n  c: rgb(1, 2, 3);\n}\n")
	expectEq(t, ".a{c:rgba(1,2,3,0.5)}", ".a {\n  c: rgba(1, 2, 3, 0.5);\n}\n")
}

func TestInterpolationAndCustomProps(t *testing.T) {
	expectEq(t, "$p:margin; .a{#{$p}-top:1px}", ".a {\n  margin-top: 1px;\n}\n")
	expectEq(t, ".a{content:\"v#{1+1}\"}", ".a {\n  content: \"v2\";\n}\n")
	expectEq(t, ":root{--x: 10px}", ":root {\n  --x: 10px;\n}\n")
}

func TestErrors(t *testing.T) {
	bad := []string{
		".a{w:$undefined}",
		".a{w:foo.bar(1)}",
		".a{",
		".a{w:1px + 1s}",
		"@error \"boom\";",
		".a{@include missing}",
		".a{w: 1 < a}",
	}
	for _, src := range bad {
		if _, err := Render(src, false, false, nil); err == nil {
			t.Errorf("expected error for %q", src)
		}
	}
}

func TestIndentedSyntax(t *testing.T) {
	src := ".a\n  color: red\n  .b\n    x: 1\n"
	got, err := Render(src, true, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.CSS, ".a .b") || !strings.Contains(got.CSS, "color: red") {
		t.Errorf("indented: %q", got.CSS)
	}
}

func TestMediaAndAtRules(t *testing.T) {
	if got := compile(t, ".a{@media print{x:1}}"); !strings.Contains(got, "@media print") {
		t.Errorf("media bubble: %q", got)
	}
	if got := compile(t, "@supports (a:b){.a{x:1}}"); !strings.Contains(got, "@supports (a: b)") {
		t.Errorf("supports: %q", got)
	}
	if got := compile(t, "@font-face{font-family:x}"); !strings.Contains(got, "@font-face") {
		t.Errorf("font-face: %q", got)
	}
	if got := compile(t, ".a{@at-root .b{x:1}}"); !strings.Contains(got, ".b {") || strings.Contains(got, ".a .b") {
		t.Errorf("at-root: %q", got)
	}
}

func TestComments(t *testing.T) {
	if got := compile(t, ".a{x:1 // silent\n}"); strings.Contains(got, "silent") {
		t.Errorf("silent comment leaked: %q", got)
	}
	if got := compile(t, "/* loud */ .a{x:1}"); !strings.Contains(got, "/* loud */") {
		t.Errorf("loud comment dropped: %q", got)
	}
}

func TestWarnDebug(t *testing.T) {
	res, err := Render("@warn \"hi\"; @debug \"yo\"; .a{x:1}", false, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Warnings) != 2 {
		t.Errorf("expected 2 warnings, got %d", len(res.Warnings))
	}
}
