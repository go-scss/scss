// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import (
	"strings"
	"testing"
)

// mustErrImp compiles src with a map importer and requires a compile error.
func mustErrImp(t *testing.T, src string, files map[string]string) {
	t.Helper()
	if _, err := renderImp(t, src, files); err == nil {
		t.Errorf("expected error for %q, got none", src)
	}
}

// wantCSS compiles src and checks the full CSS output.
func wantCSS(t *testing.T, src, want string) {
	t.Helper()
	if got := compile(t, src); got != want {
		t.Errorf("for %q:\n want: %q\n  got: %q", src, want, got)
	}
}

// --- Block A: modern CSS if() ---

func TestCssIfExpression(t *testing.T) {
	cases := []struct{ in, out string }{
		// Resolved sass() branches.
		{"a {b: if(sass(true): c; else: d)}", "a {\n  b: c;\n}\n"},
		{"a {b: if(sass(false): c; else: d)}", "a {\n  b: d;\n}\n"},
		{"a {b: if(else: c)}", "a {\n  b: c;\n}\n"},
		{"a {b: if(not sass(false): c; else: d)}", "a {\n  b: c;\n}\n"},
		{"a {b: if((sass(true)): c; else: d)}", "a {\n  b: c;\n}\n"},
		{"a {b: if(not (sass(true)): c; else: d)}", "a {\n  b: d;\n}\n"},
		{"a {b: if(sass(true) and sass(false): c; else: d)}", "a {\n  b: d;\n}\n"},
		{"a {b: if(sass(false) or sass(true): c; else: d)}", "a {\n  b: c;\n}\n"},
		// Unresolved (opaque) branches re-serialise.
		{"a {b: if(css(): c; else: d)}", "a {\n  b: if(css(): c; else: d);\n}\n"},
		{"a {b: if(sass(true) and css(): c; else: d)}", "a {\n  b: if(css(): c; else: d);\n}\n"},
		{"a {b: if(sass(false) and css(): c; else: d)}", "a {\n  b: d;\n}\n"},
		{"a {b: if(css() or sass(true): c; else: d)}", "a {\n  b: c;\n}\n"},
		{"a {b: if(css(1) and css(2): c)}", "a {\n  b: if(css(1) and css(2): c);\n}\n"},
		{"a {b: if(css(1) or css(2): c)}", "a {\n  b: if(css(1) or css(2): c);\n}\n"},
		// Parenthesized opaque group keeps its parens at top level.
		{"a {b: if((css()): c)}", "a {\n  b: if((css()): c);\n}\n"},
		// A reduced-away and/or drops the redundant parens.
		{"a {b: if(sass(true) and (var(--not) css()): c)}", "a {\n  b: if(var(--not) css(): c);\n}\n"},
		// Arbitrary-substitution juxtaposition (var/attr/if/--x/interp).
		{"a {b: if(var(--not) css(): c)}", "a {\n  b: if(var(--not) css(): c);\n}\n"},
		{"a {b: if(css() var(--and): c)}", "a {\n  b: if(css() var(--and): c);\n}\n"},
		{"a {b: if(attr(x) css(): c)}", "a {\n  b: if(attr(x) css(): c);\n}\n"},
		{"a {b: if(if(x) css(): c)}", "a {\n  b: if(if(x) css(): c);\n}\n"},
		{"a {b: if(--x(1) css(): c)}", "a {\n  b: if(--x(1) css(): c);\n}\n"},
		{"a {b: if(#{css()}: c; else: d)}", "a {\n  b: if(css(): c; else: d);\n}\n"},
		{"a {b: if(css(1) and var(--x) css(2): c)}", "a {\n  b: if(css(1) and var(--x) css(2): c);\n}\n"},
		{"a {b: if(css() var(--a) var(--b): c)}", "a {\n  b: if(css() var(--a) var(--b): c);\n}\n"},
		// No else, all false => null.
		{"a {b: if(sass(false): c) == null}", "a {\n  b: true;\n}\n"},
		// Legacy if() still works (comma form).
		{"a {b: if(true, 1, 2)}", "a {\n  b: 1;\n}\n"},
		{"a {b: if(false, 1, 2)}", "a {\n  b: 2;\n}\n"},
		// Whitespace-only special-function args collapse to empty.
		{"a {b: if(css(\n): c)}", "a {\n  b: if(css(): c);\n}\n"},
		// case-insensitive keywords.
		{"a {b: if(NOT sass(false): c; else: d)}", "a {\n  b: c;\n}\n"},
	}
	for _, c := range cases {
		wantCSS(t, c.in, c.out)
	}
}

func TestCssIfErrors(t *testing.T) {
	// sass() inside a raw arbitrary-substitution clause is illegal.
	mustErr(t, "a {b: if(sass(true) var(--x): c)}")
	// Missing colon.
	mustErr(t, "a {b: if(css() c)}")
	// Missing close paren.
	mustErr(t, "a {b: if(css(): c")
	// Unterminated interpolation in a condition.
	mustErr(t, "a {b: if(#{css(): c)}")
}

func TestIndentedBracketContinuation(t *testing.T) {
	// A multi-line bracketed value in the indented syntax.
	src := "a\n  b: if(\n    css(): c)\n"
	res, err := Render(src, true, false, nil)
	if err != nil {
		t.Fatalf("indented if: %v", err)
	}
	if !strings.Contains(res.CSS, "if(css(): c)") {
		t.Errorf("got %q", res.CSS)
	}
	// bracketDelta must ignore brackets inside strings and comments.
	if d := bracketDelta(`a: "([{" // )]}`); d != 0 {
		t.Errorf("bracketDelta strings/comments: %d", d)
	}
	if d := bracketDelta("a: b(/* ) */ c)"); d != 0 {
		t.Errorf("bracketDelta block comment: %d", d)
	}
	if d := bracketDelta("a) b("); d != 0 {
		t.Errorf("bracketDelta balanced: %d", d)
	}
}

// --- Block B: sass:meta first-class functions/mixins ---

func TestMetaGetFunctionCall(t *testing.T) {
	base := "@use \"sass:meta\"; @use \"sass:math\"; "
	wantCSS(t, base+"@function add($v){@return $v + 1} a{b: meta.call(meta.get-function(add), 2)}",
		"a {\n  b: 3;\n}\n")
	wantCSS(t, base+"a{b: meta.call(meta.get-function(\"rgb\"), 1, 2, 3)}",
		"a {\n  b: rgb(1, 2, 3);\n}\n")
	wantCSS(t, base+"a{b: meta.call(meta.get-function(\"round\", $module: \"math\"), 0.6)}",
		"a {\n  b: 1;\n}\n")
	// Deprecated string form.
	wantCSS(t, base+"a{b: meta.call(\"rgb\", 1, 2, 3)}", "a {\n  b: rgb(1, 2, 3);\n}\n")
	// $css: true plain-CSS function.
	wantCSS(t, base+"a{b: meta.call(meta.get-function(\"foo\", $css: true), 1, 2)}",
		"a {\n  b: foo(1, 2);\n}\n")
	// type-of / inspect / equality.
	wantCSS(t, base+"a{b: meta.type-of(meta.get-function(\"rgb\"))}", "a {\n  b: function;\n}\n")
	wantCSS(t, base+"a{b: meta.inspect(meta.get-function(\"lighten\"))}", "a {\n  b: get-function(\"lighten\");\n}\n")
	wantCSS(t, base+"a{b: meta.get-function(\"lighten\") == meta.get-function(\"lighten\")}", "a {\n  b: true;\n}\n")
	wantCSS(t, base+"a{b: meta.get-function(\"lighten\") == meta.get-function(\"darken\")}", "a {\n  b: false;\n}\n")
	wantCSS(t, base+"@function u(){@return 1} a{b: meta.get-function(u) == meta.get-function(u)}", "a {\n  b: true;\n}\n")
	wantCSS(t, base+"a{b: meta.get-function(\"rgb\") == 1}", "a {\n  b: false;\n}\n")
}

func TestMetaGetFunctionErrors(t *testing.T) {
	base := "@use \"sass:meta\"; "
	mustErr(t, base+"a{b: meta.get-function(\"nope\")}")
	mustErr(t, base+"a{b: meta.get-function(\"x\", $module: \"nomod\")}")
	mustErr(t, base+"a{b: meta.call(1)}")
}

func TestMetaMixins(t *testing.T) {
	base := "@use \"sass:meta\"; "
	// apply positional / named / content.
	wantCSS(t, base+"@mixin m($a){b: $a} a{@include meta.apply(meta.get-mixin(\"m\"), c)}",
		"a {\n  b: c;\n}\n")
	wantCSS(t, base+"@mixin m($a){b: $a} a{@include meta.apply(meta.get-mixin(\"m\"), $a: c)}",
		"a {\n  b: c;\n}\n")
	wantCSS(t, base+"@mixin m{b {@content}} a{@include meta.apply($mixin: meta.get-mixin(\"m\")){x: y}}",
		"a b {\n  x: y;\n}\n")
	// accepts-content true/false and builtin.
	wantCSS(t, base+"@mixin m{@content} a{b: meta.accepts-content(meta.get-mixin(\"m\"))}", "a {\n  b: true;\n}\n")
	wantCSS(t, base+"@mixin m{@if true {@content}} a{b: meta.accepts-content(meta.get-mixin(\"m\"))}", "a {\n  b: true;\n}\n")
	wantCSS(t, base+"@mixin m{x: y} a{b: meta.accepts-content(meta.get-mixin(\"m\"))}", "a {\n  b: false;\n}\n")
	wantCSS(t, base+"a{b: meta.accepts-content(meta.get-mixin(apply, meta))}", "a {\n  b: true;\n}\n")
	// type-of / inspect / equality for mixins.
	wantCSS(t, base+"@mixin m{x:y} a{b: meta.type-of(meta.get-mixin(\"m\"))}", "a {\n  b: mixin;\n}\n")
	wantCSS(t, base+"@mixin m{x:y} a{b: meta.inspect(meta.get-mixin(\"m\"))}", "a {\n  b: get-mixin(\"m\");\n}\n")
	wantCSS(t, base+"@mixin m{x:y} a{b: meta.get-mixin(\"m\") == meta.get-mixin(\"m\")}", "a {\n  b: true;\n}\n")
	wantCSS(t, base+"a{b: meta.get-mixin(apply, meta) == meta.get-mixin(apply, meta)}", "a {\n  b: true;\n}\n")
	wantCSS(t, base+"@mixin m{x:y} a{b: meta.get-mixin(\"m\") == 1}", "a {\n  b: false;\n}\n")
}

func TestMetaMixinErrors(t *testing.T) {
	base := "@use \"sass:meta\"; "
	mustErr(t, base+"a{b: meta.get-mixin(\"nope\")}")
	mustErr(t, base+"a{b: meta.get-mixin(\"x\", $module: \"nomod\")}")
	mustErr(t, base+"a{@include meta.apply(1)}")
	mustErr(t, base+"a{@include meta.apply()}")
	mustErr(t, base+"a{b: meta.accepts-content(1)}")
	mustErr(t, base+"a{@include meta.apply(meta.get-mixin(apply, meta))}")
}

func TestMetaModuleReflection(t *testing.T) {
	files := map[string]string{
		"other": "@function b(){@return b value} @function c(){@return c value} $v: 1; @mixin mm{x: y}",
		"empty": "// nothing",
	}
	base := "@use \"sass:meta\"; @use \"other\"; "
	// module-functions: call each.
	res, err := renderImp(t, base+"a{ @each $n, $f in meta.module-functions(\"other\") { #{$n}: meta.call($f) } }", files)
	if err != nil {
		t.Fatalf("module-functions: %v", err)
	}
	if !strings.Contains(res.CSS, "b: b value") || !strings.Contains(res.CSS, "c: c value") {
		t.Errorf("module-functions got %q", res.CSS)
	}
	// module-variables.
	if r, err := renderImp(t, base+"a{b: meta.inspect(meta.module-variables(\"other\"))}", files); err != nil || !strings.Contains(r.CSS, `("v": 1)`) {
		t.Errorf("module-variables got %q err %v", r.CSS, err)
	}
	// module-mixins type + apply.
	if r, err := renderImp(t, base+"a{b: meta.type-of(meta.module-mixins(\"other\"))}", files); err != nil || !strings.Contains(r.CSS, "b: map") {
		t.Errorf("module-mixins got %q err %v", r.CSS, err)
	}
	// built-in module reflection.
	if r, err := renderImp(t, "@use \"sass:meta\"; @use \"sass:map\"; a{b: meta.type-of(map.get(meta.module-functions(\"meta\"), \"inspect\"))}", files); err != nil || !strings.Contains(r.CSS, "function") {
		t.Errorf("built-in module-functions got %q err %v", r.CSS, err)
	}
	if r, err := renderImp(t, "@use \"sass:meta\"; a{b: meta.inspect(meta.module-mixins(\"meta\"))}", files); err != nil || !strings.Contains(r.CSS, "apply") {
		t.Errorf("built-in module-mixins got %q err %v", r.CSS, err)
	}
	if r, err := renderImp(t, "@use \"sass:meta\"; a{b: meta.inspect(meta.module-variables(\"meta\"))}", files); err != nil || !strings.Contains(r.CSS, "()") {
		t.Errorf("built-in module-variables got %q err %v", r.CSS, err)
	}
	// error: unknown module.
	mustErrImp(t, base+"a{b: meta.module-functions(\"nomod\")}", files)
}

func withMap(files map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range files {
		out[k] = v
	}
	return out
}

func TestMetaLoadCss(t *testing.T) {
	files := map[string]string{
		"other":    "a {b: c}",
		"configed": "$x: default !default; a {b: $x}",
		"emptymod": "// no css",
	}
	if r, err := renderImp(t, "@use \"sass:meta\"; @include meta.load-css(\"other\");", files); err != nil || !strings.Contains(r.CSS, "b: c") {
		t.Errorf("load-css got %q err %v", r.CSS, err)
	}
	if r, err := renderImp(t, "@use \"sass:meta\"; @include meta.load-css(\"configed\", $with: (\"x\": v));", files); err != nil || !strings.Contains(r.CSS, "b: v") {
		t.Errorf("load-css with config got %q err %v", r.CSS, err)
	}
	if r, err := renderImp(t, "@use \"sass:meta\"; @include meta.load-css($url: \"emptymod\");", files); err != nil || strings.TrimSpace(r.CSS) != "" {
		t.Errorf("load-css empty got %q err %v", r.CSS, err)
	}
	mustErrImp(t, "@use \"sass:meta\"; @include meta.load-css(\"nope\");", files)
	mustErrImp(t, "@use \"sass:meta\"; @include meta.load-css();", files)
	// Module loop.
	loop := map[string]string{"selfx": "@use \"sass:meta\"; @include meta.load-css(\"selfx\");"}
	mustErrImp(t, "@use \"sass:meta\"; @include meta.load-css(\"selfx\");", loop)
	// Parse error in loaded module.
	bad := map[string]string{"bad": "a {"}
	func() {
		defer func() { _ = recover() }()
		_, _ = renderImp(t, "@use \"sass:meta\"; @include meta.load-css(\"bad\");", bad)
	}()
}

func TestMetaCalcName(t *testing.T) {
	base := "@use \"sass:meta\"; "
	wantCSS(t, base+"a{b: meta.calc-name(calc(var(--c)))}", "a {\n  b: \"calc\";\n}\n")
	wantCSS(t, base+"a{b: meta.calc-name(clamp(1%, 2px, 3px))}", "a {\n  b: \"clamp\";\n}\n")
	mustErr(t, base+"a{b: meta.calc-name(1)}")
}

func TestForwardShowHide(t *testing.T) {
	files := map[string]string{
		"up":   "@function a(){@return A} @function b(){@return B} $v: 1; $w: 2; @mixin ma{x: a}",
		"midS": "@forward \"up\" show a, $v;",
		"midH": "@forward \"up\" hide b, $w;",
		"midP": "@forward \"up\" as p-* show p-a;",
	}
	// show exposes only a and $v; $w is hidden (undefined variable errors).
	if r, err := renderImp(t, "@use \"midS\" as *; x{a: a(); v: $v}", files); err != nil || !strings.Contains(r.CSS, "a: A") {
		t.Errorf("forward show got %q err %v", r.CSS, err)
	}
	mustErrImp(t, "@use \"midS\" as *; x{w: $w}", files)
	// hide removes b and $w.
	if r, err := renderImp(t, "@use \"midH\" as *; x{a: a(); v: $v}", files); err != nil || !strings.Contains(r.CSS, "a: A") {
		t.Errorf("forward hide got %q err %v", r.CSS, err)
	}
	mustErrImp(t, "@use \"midH\" as *; x{w: $w}", files)
	// prefixed show matches prefixed names.
	if r, err := renderImp(t, "@use \"midP\" as *; x{a: p-a()}", files); err != nil || !strings.Contains(r.CSS, "a: A") {
		t.Errorf("forward prefixed show got %q err %v", r.CSS, err)
	}
}

// --- Block C: CSS special functions ---

func TestSpecialFunctions(t *testing.T) {
	cases := []struct{ in, out string }{
		{"a {b: url(!)}", "a {\n  b: url(!);\n}\n"},
		{"a {b: url(http://c.d/e!f)}", "a {\n  b: url(http://c.d/e!f);\n}\n"},
		{"a {b: URL(!)}", "a {\n  b: url(!);\n}\n"},
		{"a {b: url(#{1 + 1})}", "a {\n  b: url(2);\n}\n"},
		{"a {b: url( c )}", "a {\n  b: url(c);\n}\n"},
		{`a {b: url("q")}`, "a {\n  b: url(\"q\");\n}\n"}, // fallback to normal function (quotes)
		{"a {b: -c-url(0)}", "a {\n  b: url(0);\n}\n"},
		{"a {b: type(@#$%^&*({[]})_+=)}", "a {\n  b: type(@#$%^&*({[]})_+=);\n}\n"},
		{"a {b: type(#{0})}", "a {\n  b: type(0);\n}\n"},
		{"a {b: attr(c, %)}", "a {\n  b: attr(c, %);\n}\n"},
		{"a {b: -a-calc(0)}", "a {\n  b: -a-calc(0);\n}\n"},
		{"a {b: element(#foo)}", "a {\n  b: element(#foo);\n}\n"},
		{"a {b: expression(2 > 1)}", "a {\n  b: expression(2 > 1);\n}\n"},
		{`a {b: expression("Smile :-)")}`, "a {\n  b: expression(\"Smile :-)\");\n}\n"},
		{"a {b: progid:DXImageTransform.Microsoft.gradient(x)}", "a {\n  b: progid:DXImageTransform.Microsoft.gradient(x);\n}\n"},
		{"a {b: -c-progid:d(0)}", "a {\n  b: -c-progid:d(0);\n}\n"},
		// var() stays a normal function (evaluates fallback).
		{"a {b: var(--c, 1 + 2)}", "a {\n  b: var(--c, 3);\n}\n"},
	}
	for _, c := range cases {
		wantCSS(t, c.in, c.out)
	}
}

func TestSpecialFunctionColorPassthrough(t *testing.T) {
	base := "@use \"sass:color\"; "
	wantCSS(t, base+"a {b: rgb(var(--x), 2, 3)}", "a {\n  b: rgb(var(--x), 2, 3);\n}\n")
	wantCSS(t, base+"a {b: rgb(attr(c, %), 2, 3)}", "a {\n  b: rgb(attr(c, %), 2, 3);\n}\n")
	// two-argument special form.
	wantCSS(t, base+"a {b: rgb(var(--x), 0.5)}", "a {\n  b: rgb(var(--x), 0.5);\n}\n")
}

func TestSpecialFunctionUrlEdges(t *testing.T) {
	// An invalid URL character ("(") backtracks to an ordinary function call.
	wantCSS(t, "a {b: url(a(b))}", "a {\n  b: url(a(b));\n}\n")
	// A space that is not followed by ")" backtracks to a normal function.
	wantCSS(t, "a {b: url(a b)}", "a {\n  b: url(a b);\n}\n")
	// An escape inside a url() is preserved.
	wantCSS(t, `a {b: url(a\)b)}`, "a {\n  b: url(a\\)b);\n}\n")
	// EOF inside url() backtracks then fails to parse (unterminated).
	mustErr(t, "a {b: url(abc")
}

// --- targeted branch coverage ---

func TestCssIfBranchCoverage(t *testing.T) {
	ok := []string{
		// raw-continuation loop: and/or/juxtapose after an initial raw clause.
		"a{b: if(var(--a) and css(): c)}",
		"a{b: if(var(--a) or css(): c)}",
		"a{b: if(var(--a) css() var(--b): c)}",
		// paren / op / not clauses flattened into a raw clause.
		"a{b: if((css()) var(--x): c)}",
		"a{b: if((css() and css()) var(--x): c)}",
		"a{b: if((not css()) var(--x): c)}",
		// arbitrary substitutions consumed via tryArbitrarySubstitution.
		"a{b: if(css() #{var(--x)}: c)}",
		"a{b: if(css() attr(x): c)}",
		"a{b: if(css() if(y): c)}",
		"a{b: if(css() --z(1): c)}",
		// interpolation-only clause via ifGroup.
		"a{b: if(#{css()} and css(): c)}",
		// captureIfArgs: string / interpolation / nested parens in a css clause.
		`a{b: if(css("a)b"): c)}`,
		"a{b: if(css(#{1}): c)}",
		"a{b: if(css((1)): c)}",
	}
	for _, s := range ok {
		if _, err := Render(s, false, false, nil); err != nil {
			t.Errorf("%q: %v", s, err)
		}
	}
	// tryArbitrarySubstitution reset: "var" not followed by "(".
	mustErr(t, "a{b: if(css() var: c)}")
	// sass() without "(".
	mustErr(t, "a{b: if(sass true: c)}")
	// A function clause with an interpolated (non-plain) name is not an
	// arbitrary substitution.
	if isArbitrarySubstitution(&ifCondFunc{name: []any{&InterpExpr{Expr: &Ident{Name: "x"}}}}) {
		t.Error("interpolated-name function should not be an arbitrary substitution")
	}
}

func TestMetaValueContexts(t *testing.T) {
	base := "@use \"sass:meta\"; @use \"sass:list\"; "
	// isTruthy for function/mixin values.
	wantCSS(t, base+"a{b: if(meta.get-function(rgb), y, n)}", "a {\n  b: y;\n}\n")
	wantCSS(t, base+"@mixin m{x:y} a{b: if(meta.get-mixin(\"m\"), y, n)}", "a {\n  b: y;\n}\n")
	// sep / asList via list functions.
	if got := compile(t, base+"a{b: list.length(meta.get-function(rgb) meta.get-function(hsl))}"); !strings.Contains(got, "b: 2") {
		t.Errorf("function asList: %q", got)
	}
	if got := compile(t, base+"@mixin m{x:y} a{b: list.separator((meta.get-mixin(\"m\"), meta.get-mixin(\"m\")))}"); !strings.Contains(got, "comma") {
		t.Errorf("mixin sep: %q", got)
	}
	// named args to call and to a $css function.
	wantCSS(t, base+"a{b: meta.call($function: meta.get-function(rgb), $red:1,$green:2,$blue:3)}", "a {\n  b: rgb(1, 2, 3);\n}\n")
	if got := compile(t, base+"a{b: meta.call(meta.get-function(f, $css: true), $x: 1)}"); !strings.Contains(got, "f($x: 1)") {
		t.Errorf("css named: %q", got)
	}
	// module argument that is not a string.
	mustErr(t, base+"a{b: meta.get-function(x, $module: 5)}")
}

func TestMetaModuleMembers(t *testing.T) {
	files := map[string]string{"other": "@function f(){@return F} @mixin m{x:y} $v: 1"}
	base := "@use \"sass:meta\"; @use \"other\"; "
	if r, err := renderImp(t, base+"a{b: meta.call(meta.get-function(f, $module: \"other\"))}", files); err != nil || !strings.Contains(r.CSS, "b: F") {
		t.Errorf("get-function module: %q %v", r.CSS, err)
	}
	if r, err := renderImp(t, base+"a{@include meta.apply(meta.get-mixin(m, $module: \"other\"))}", files); err != nil || !strings.Contains(r.CSS, "x: y") {
		t.Errorf("get-mixin module: %q %v", r.CSS, err)
	}
}

func TestAcceptsContentNesting(t *testing.T) {
	base := "@use \"sass:meta\"; "
	// @content nested inside every container statement type.
	mixins := []string{
		"@mixin m{a {@content}}",
		"@mixin m{@media x {@content}}",
		"@mixin m{@supports (x: y) {@content}}",
		"@mixin m{@each $i in 1 {@content}}",
		"@mixin m{@for $i from 1 through 1 {@content}}",
		"@mixin m{@while false {@content}}",
		"@mixin m{@at-root {@content}}",
		"@mixin m{@include inner {@content}} @mixin inner {@content}",
	}
	for _, def := range mixins {
		src := base + def + " a{b: meta.accepts-content(meta.get-mixin(\"m\"))}"
		if got := compile(t, src); !strings.Contains(got, "b: true") {
			t.Errorf("accepts-content for %q: %q", def, got)
		}
	}
}

func TestSpecialFnBranchCoverage(t *testing.T) {
	// Special-function names used bare (no "(") are ordinary identifiers.
	wantCSS(t, "a {b: element}", "a {\n  b: element;\n}\n")
	wantCSS(t, "a {b: type}", "a {\n  b: type;\n}\n")
	// progid without "(" is an error.
	mustErr(t, "a {b: progid:d}")
	// url() with an escape and a leading vendor prefix.
	wantCSS(t, `a {b: url(a\ b)}`, "a {\n  b: url(a\\ b);\n}\n")
}

func TestCompileHelpersCoverage(t *testing.T) {
	if bracketDelta("a: b) c(") != 0 {
		t.Error("bracketDelta unbalanced net zero")
	}
	if bracketDelta("a / b") != 0 {
		t.Error("bracketDelta lone slash")
	}
	if bracketDelta("x // ( comment") != 0 {
		t.Error("bracketDelta line comment")
	}
	// A continuation line beginning with a closing bracket clamps depth to zero.
	got := joinBracketContinuations([]string{")a", "b"})
	if len(got) != 2 {
		t.Errorf("joinBracketContinuations clamp: %v", got)
	}
}

func TestCallUserResolvedErrors(t *testing.T) {
	base := "@use \"sass:meta\"; "
	// A user function that never returns a value.
	mustErr(t, base+"@function f($x){x: y} a{b: meta.call(meta.get-function(f), 1)}")
	// An @error inside the called function propagates.
	mustErr(t, base+"@function f(){@error boom} a{b: meta.call(meta.get-function(f))}")
}

func TestCssIfFinalCoverage(t *testing.T) {
	ok := []string{
		// not around an opaque clause serialises as "not ...".
		"a{b: if(not css(): c)}",
		// and/or continuation inside a raw clause.
		"a{b: if(var(--a) css() and css(): c)}",
		"a{b: if(var(--a) css() or css(): c)}",
		// a raw (interpolation) clause juxtaposed with another clause.
		"a{b: if(css() #{var(--x)} attr(y): c)}",
		// captureIfArgs bracket branch.
		"a{b: if(css([x]): c)}",
		// evalIfOp values==nil (all clauses resolve, no short circuit).
		"a{b: if(sass(true) and sass(true): c; else: d)}",
		"a{b: if(sass(false) or sass(false): c; else: d)}",
	}
	for _, s := range ok {
		if _, err := Render(s, false, false, nil); err != nil {
			t.Errorf("%q: %v", s, err)
		}
	}
	// ifGroup / interpolatedIdent / captureIfArgs error paths.
	mustErr(t, "a{b: if((css(): c)}")     // paren not closed
	mustErr(t, "a{b: if(sass(x: c)}")     // sass not closed
	mustErr(t, "a{b: if(foo: c)}")        // plain ident clause needs "("
	mustErr(t, "a{b: if(@: c)}")          // empty clause identifier
	mustErr(t, "a{b: if(#{css() : c)}")   // unterminated interp in a name
	mustErr(t, "a{b: if(css(#{1): c)}")   // unterminated interp in args
	mustErr(t, "a{b: if(css() #{1 : c)}") // unterminated interp substitution
}

func TestMetaFinalCoverage(t *testing.T) {
	base := "@use \"sass:meta\"; @use \"sass:list\"; "
	// asList / sep on single function and mixin values.
	if got := compile(t, base+"a{b: list.length(meta.get-function(rgb))}"); !strings.Contains(got, "b: 1") {
		t.Errorf("function asList single: %q", got)
	}
	if got := compile(t, base+"a{b: list.separator(meta.get-function(rgb))}"); !strings.Contains(got, "space") {
		t.Errorf("function sep: %q", got)
	}
	if got := compile(t, base+"@mixin m{x:y} a{b: list.length(meta.get-mixin(\"m\"))}"); !strings.Contains(got, "b: 1") {
		t.Errorf("mixin asList single: %q", got)
	}
	if got := compile(t, base+"@mixin m{x:y} a{b: list.separator(meta.get-mixin(\"m\"))}"); !strings.Contains(got, "space") {
		t.Errorf("mixin sep: %q", got)
	}
	// stmtBodies: @content inside a generic at-rule and inside @else.
	wantCSS(t, base+"@mixin m{@font-face {@content}} a{b: meta.accepts-content(meta.get-mixin(\"m\"))}", "a {\n  b: true;\n}\n")
	wantCSS(t, base+"@mixin m{@if false {} @else {@content}} a{b: meta.accepts-content(meta.get-mixin(\"m\"))}", "a {\n  b: true;\n}\n")
}

func TestNamespacedIdentValue(t *testing.T) {
	// A namespaced identifier that is not a function call is an unquoted string.
	wantCSS(t, "a {b: foo.bar}", "a {\n  b: foo.bar;\n}\n")
}

func TestBracketDeltaEscape(t *testing.T) {
	// A backslash-escaped quote inside a string does not close it; the "(" is
	// inside the string and ignored, so "(" ... ")" nets to zero.
	if d := bracketDelta(`("a\"b")`); d != 0 {
		t.Errorf("bracketDelta escape: %d", d)
	}
}

func TestInterpolatedIdentDirect(t *testing.T) {
	// literal text followed by an interpolation, then more literal text.
	p := newParser("pre#{1}post(")
	parts := p.interpolatedIdent()
	if len(parts) != 3 {
		t.Errorf("interpolatedIdent parts = %d, want 3: %#v", len(parts), parts)
	}
	// an escape sequence within an identifier.
	p2 := newParser(`a\9b(`)
	if got, _ := plainParts(p2.interpolatedIdent()); got == "" {
		t.Errorf("interpolatedIdent escape produced empty")
	}
	// an empty identifier is an error.
	func() {
		defer func() {
			if recover() == nil {
				t.Error("expected panic for empty identifier")
			}
		}()
		newParser(":x").interpolatedIdent()
	}()
}

func TestSpecialFnUrlInterpError(t *testing.T) {
	mustErr(t, "a {b: url(#{1)}") // unterminated interpolation inside url()
}
