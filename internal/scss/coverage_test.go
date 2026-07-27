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
	got := compile(t, "@media x {.a{@at-root (without: media) {.b{y:1}}}}")
	if !strings.Contains(got, ".b") {
		t.Errorf("at-root query: %q", got)
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
