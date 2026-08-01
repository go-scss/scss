// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import (
	"strings"
	"testing"
)

func TestMoreFeatures(t *testing.T) {
	cases := []struct{ in, want string }{
		{".a{v: hsl(120, 100%, 50%)}", "hsl(120, 100%, 50%)"},
		{".a{v: hsla(0, 100%, 50%, 0.5)}", "hsla(0, 100%, 50%, 0.5)"},
		{".a{v: change-color(#123456, $hue: 0)}", "#"},
		{"@use \"sass:map\"; .a{v: map.set((a:1), b, 2)}", "a: 1, b: 2"},
		{".a{v: append(1 2, 3, comma)}", "1, 2, 3"},
		{".a{v: join(1, 2, space)}", "1 2"},
		{".a{v: str-insert(\"ab\", \"X\", -1)}", "abX"},
		{".a{v: zip(1 2 3, a b)}", "1 a, 2 b"},
		{".a{v: 2px * 3px * 1}", ""}, // px*px complex unit
		{".a{v: quote(1)}", "\"1\""},
		{".a{v: (1 2 3)}", "1 2 3"},
	}
	for _, c := range cases {
		got := compile(t, c.in)
		if c.want != "" && !strings.Contains(got, c.want) {
			t.Errorf("%q => want %q got %q", c.in, c.want, got)
		}
	}
}

func TestGlobalAndDefault(t *testing.T) {
	if got := compile(t, "$x: 1; .a{ $x: 2 !global; } .b{v:$x}"); !strings.Contains(got, "v: 2") {
		t.Errorf("!global: %q", got)
	}
	if got := compile(t, "$x: 1 !default; $x: 2; .a{v:$x}"); !strings.Contains(got, "v: 2") {
		t.Errorf("!default keep: %q", got)
	}
	if got := compile(t, "$x: 1; $x: 2 !default; .a{v:$x}"); !strings.Contains(got, "v: 1") {
		t.Errorf("!default skip: %q", got)
	}
}

func TestCompressedColorNames(t *testing.T) {
	if got := compileC(t, ".a{c: #008000}"); !strings.Contains(got, "green") {
		t.Errorf("green name: %q", got)
	}
	if got := compileC(t, ".a{c: #abcdef}"); !strings.Contains(got, "#abcdef") {
		t.Errorf("hex kept: %q", got)
	}
	if got := compileC(t, ".a{c: rgba(1,2,3,0)}"); !strings.Contains(got, "rgba(1,2,3,0)") {
		t.Errorf("transparent rgba: %q", got)
	}
}

func TestNumberFormatting(t *testing.T) {
	cases := map[string]string{
		".a{v: 0.5}":              "0.5",
		".a{v: 100000}":           "100000",
		".a{v: 3.14159265358979}": "3.1415926536",
		".a{v: -0}":               "0",
		".a{v: 1e3}":              "1000",
	}
	for in, want := range cases {
		if got := compile(t, in); !strings.Contains(got, want) {
			t.Errorf("%q => want %q got %q", in, want, got)
		}
	}
	if got := compileC(t, ".a{v: 0.5}"); !strings.Contains(got, ".5") {
		t.Errorf("compressed leading zero: %q", got)
	}
}

func TestNestedMediaThreeDeep(t *testing.T) {
	got := compile(t, "@media (min-width: 1px) {@media (max-width: 9px) {.x{y:1}}}")
	if !strings.Contains(got, "and") || !strings.Contains(got, ".x") {
		t.Errorf("nested media merge: %q", got)
	}
}

func TestAtRootWithQuery(t *testing.T) {
	// `without: media` drops the @media frame but keeps the enclosing `.a` rule,
	// which is re-materialised at the root (byte-exact vs dart-sass 1.102).
	got := compile(t, "@media x {.a{@at-root (without: media) {.b{y:1}}}}")
	if want := ".a .b {\n  y: 1;\n}\n"; got != want {
		t.Errorf("at-root query:\n want: %q\n  got: %q", want, got)
	}
}

func TestWarnDebugError(t *testing.T) {
	res, _ := Render("@debug 1 + 1; .a{x:1}", false, false, nil)
	if len(res.Warnings) == 0 {
		t.Error("expected debug output")
	}
	if _, err := Render("@error \"custom\";", false, false, nil); err == nil || !strings.Contains(err.Error(), "custom") {
		t.Errorf("error message: %v", err)
	}
}

func TestKeywordArgsToFunction(t *testing.T) {
	got := compile(t, "@function f($a, $b) { @return $a - $b; } .a{v: f($b: 1, $a: 5)}")
	if !strings.Contains(got, "v: 4") {
		t.Errorf("keyword args: %q", got)
	}
}

func TestSpreadArgs(t *testing.T) {
	got := compile(t, "@function f($a, $b, $c) { @return $a + $b + $c; } $l: 1 2 3; .a{v: f($l...)}")
	if !strings.Contains(got, "v: 6") {
		t.Errorf("spread: %q", got)
	}
}

// TestVarArgsSpreadSeparator covers the separator a var-args ($rest) parameter
// adopts: dart-sass gives it the separator of a spread list argument (space or
// comma), even when leading positional arguments precede the spread, and defaults
// to comma when nothing is spread. Byte-exact against dart-sass 1.102.
func TestVarArgsSpreadSeparator(t *testing.T) {
	m := "@mixin f($a, $b...) { b: $b; }\n"
	cases := []struct{ in, want string }{
		// A spread space-list makes the rest arglist space-separated.
		{m + "$l: 3 4 5;\n.x{@include f(1, 2, $l...)}", "b: 2 3 4 5;"},
		// A spread comma-list keeps it comma-separated.
		{m + "$l: 3, 4, 5;\n.x{@include f(1, 2, $l...)}", "b: 2, 3, 4, 5;"},
		// No spread: the rest arglist defaults to comma.
		{m + ".x{@include f(1, 2, 3, 4)}", "b: 2, 3, 4;"},
		// A function's var-args behaves the same way.
		{"@function g($a, $b...){@return \"#{$b}\"}\n$l: 3 4 5;\n.x{v: g(1, 2, $l...)}", "v: \"2 3 4 5\";"},
	}
	for _, c := range cases {
		if got := compile(t, c.in); !strings.Contains(got, c.want) {
			t.Errorf("for %q: want %q in %q", c.in, c.want, got)
		}
	}
}

// TestKeywordArgDashUnderscore covers the interchangeability of `-` and `_` in
// keyword argument names: a caller may spell the keyword with underscores while
// the parameter is declared with hyphens (or vice versa), and a spread map key
// is matched the same way. Byte-exact against dart-sass 1.102.
func TestKeywordArgDashUnderscore(t *testing.T) {
	cases := []struct{ in, want string }{
		// Keyword underscores match a hyphenated parameter, and vice versa.
		{"@mixin m($yada-yada) { hi: $yada-yada; } .a{@include m($yada_yada: 1)}", "hi: 1"},
		{"@function f($cool_arg) { @return $cool-arg; } .a{v: f($cool-arg: 2)}", "v: 2"},
		// A spread map with an underscore key binds a hyphenated parameter.
		{"@mixin m($foo-bar) { x: $foo-bar; } $m: (foo_bar: 3); .a{@include m($m...)}", "x: 3"},
	}
	for _, c := range cases {
		if got := compile(t, c.in); !strings.Contains(got, c.want) {
			t.Errorf("for %q: want %q in %q", c.in, c.want, got)
		}
	}
}

func TestPlaceholderNotEmitted(t *testing.T) {
	got := compile(t, "%p { x: 1; }")
	if strings.Contains(got, "%p") || strings.TrimSpace(got) != "" {
		t.Errorf("placeholder emitted: %q", got)
	}
}

func TestPlainCSSFunctionPassthrough(t *testing.T) {
	got := compile(t, ".a{ width: calc(100% - 10px); background: url(x.png) }")
	if !strings.Contains(got, "calc(100% - 10px)") || !strings.Contains(got, "url(x.png)") {
		t.Errorf("passthrough: %q", got)
	}
}

func TestListIndexAndSetnthNegative(t *testing.T) {
	if got := compile(t, ".a{v: nth(a b c, -1)}"); !strings.Contains(got, "v: c") {
		t.Errorf("negative nth: %q", got)
	}
}

func TestEmptyAndNullHandling(t *testing.T) {
	if got := compile(t, ".a{ x: null; y: 1 }"); strings.Contains(got, "x:") {
		t.Errorf("null decl emitted: %q", got)
	}
	if got := compile(t, ".a{ x: 1 null 2 }"); !strings.Contains(got, "x: 1 2") {
		t.Errorf("null in list: %q", got)
	}
}

// TestNodeMarkers exercises the empty interface-tag methods so the AST node set
// is fully covered and provably implements Stmt/Expr.
func TestNodeMarkers(t *testing.T) {
	stmts := []Stmt{
		&StyleRule{}, &Declaration{}, &VarDecl{}, &MixinDef{}, &Include{},
		&FunctionDef{}, &Return{}, &If{}, &Each{}, &For{}, &While{}, &AtRoot{},
		&Media{}, &Supports{}, &Extend{}, &ContentStmt{}, &Import{}, &Use{},
		&Forward{}, &Warn{}, &Debug{}, &ErrorStmt{}, &LoudComment{}, &AtRule{},
	}
	for _, s := range stmts {
		s.stmt()
	}
	exprs := []Expr{
		&NumberLit{}, &StringLit{}, &ColorLit{}, &BoolLit{}, &NullLit{}, &VarRef{},
		&Ident{}, &Parent{}, &Binary{}, &Unary{}, &FuncCall{}, &ListExpr{},
		&MapExpr{}, &Paren{}, &InterpExpr{},
	}
	for _, e := range exprs {
		e.expr()
	}
}
