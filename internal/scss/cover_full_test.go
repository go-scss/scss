// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import (
	"math"
	"strings"
	"testing"
)

// TestErrorRecoveryPaths drives the parser/eval error branches that surface as
// compilation errors, each triggered by a precise malformed construct.
func TestErrorRecoveryPaths(t *testing.T) {
	bad := []string{
		// parser statement-level
		"}",             // unexpected "}" at top level
		".a{x:1",        // unterminated block -> Expected "}"
		"$x 5;",         // variable decl without ":"
		".a{$x: 1)}",    // stray token after value -> Expected ";"
		".a{ foo: 1) }", // declaration value trailed by stray ")" (tryDeclaration default)
		// @each / @for / control
		"@each 1 in a {}",       // @each expected variable
		"@each $x a {}",         // @each expected "in"
		"@for 1 from 1 to 2 {}", // @for expected variable
		"@for $i x 1 to 2 {}",   // @for expected "from"
		"@for $i from 1 x 3 {}", // @for expected "to"/"through"
		// includes / functions / params
		"@function f {}",                        // parseParamList expected "("
		"@function f(1) {}",                     // parseParamList expected variable
		"@function f($a $b){}",                  // parseParamList expected ")"
		"@function f($a){@return $a} .x{v:f()}", // missing argument
		// expressions
		".a{v:$undef}",                         // undefined variable
		".a{v: (a:1, a:2)}",                    // duplicate map key
		".a{v: \"x\" * \"y\"}",                 // undefined string "*"
		".a{v: \"x\" % \"y\"}",                 // undefined string "%"
		".a{v: (1}",                            // paren expected ")"
		".a{v: (1, 2}",                         // comma list expected ")"
		".a{v: (a: 1, b: 2}",                   // map expected ")"
		".a{v: (a: 1, b 2)}",                   // map expected ":"
		".a{v: [1 2}",                          // space bracket expected "]"
		".a{v: [1, 2}",                         // comma bracket expected "]"
		".a{v: calc(#{1px) }",                  // calc interpolation expected "}"
		".a{v: foo(1; 2)}",                     // arg list expected ")" or ","
		".a{v: set-nth(1 2, 5, x)}",            // set-nth invalid index
		".a{v: nth(1 2, 9)}",                   // nth invalid index
		".a{v: if(true, 1)}",                   // if() needs three args
		".a{v: map.get(1, x)}",                 // asMapVal not a map
		"@import 123;",                         // import expected string
		"@use foo;",                            // scanQuotedString expected string
		"@use \"x\" with 1",                    // parseConfig expected "("
		"@use \"x\" with (1: 2)",               // parseConfig expected variable
		"@use \"x\" with ($a 2)",               // parseConfig expected ":"
		"@forward \"a\\62 c\";",                // scanQuotedString escape (then load fails)
		"@use \"sass:math\"; .a{ v: math.() }", // scanIdentifier empty after namespace
		".a{ v: \"p#{1)\" }",                   // string-literal interpolation expected "}"
		".a{ v: #{1) }",                        // hash interpolation expected "}"
		".a{ v: (1px / 1s) + 1px }",            // convertUnitList length mismatch (incompatible units)
	}
	for _, src := range bad {
		mustErr(t, src)
	}
}

// TestFeatureRecoveryPaths drives non-error branches reachable through valid but
// unusual SCSS.
func TestFeatureRecoveryPaths(t *testing.T) {
	ok := []string{
		"@mixin m { @content } .a { @include m }",                        // @content with no block
		"@mixin m { @content(red) } .a{ @include m using ($c){ z: 1 } }", // @content(args)+using
		"@mixin m { x: 1 } .a { @include m }",                            // bindResolved params nil
		".a { --custom: 1 }",                                             // scanIdentifier leading "--"
		".a { v: #ab }",                                                  // hash value -> Ident (not a color)
		".a { font: 10px { family: serif } }",                            // selectorLike default (number value)
		".a { v: 1e+3 }",                                                 // number exponent with sign
		".a { v: 1e-2 }",                                                 // number exponent negative
		".a { v: [] }",                                                   // empty bracket list
		".a { v: [1, 2,] }",                                              // trailing comma bracket list
		".a { v: () }",                                                   // empty paren list
		"/* lead #{1 + 1} */ .a{x:1}",                                    // loud comment with leading text
		"@import url(foo(bar));",                                         // url() import with nested parens
		"@for $i from 1 + 1 through 4 { .b#{$i}{x:1} }",                  // multi-token @for bound
		".a { b c { x: 1 } }",                                            // selectorLike space-list nesting
		".a { & b { x: 1 } }",                                            // selectorLike Parent nesting
		"& { x: 1 }",                                                     // resolveNesting with no parent
		".a { > .b { x: 1 } }",                                           // leading combinator serialize
		".a { .b {} c: 1 }",                                              // empty container skipped
		".a { color: false or 2 }",                                       // or with falsy left
		".a { v: 5 % -3 }",                                               // modulo with sign adjust
		"@media screen  and  (color) { .a{x:1} }",                        // media query multi-space collapse
		"@font-face { font-family: x }",                                  // generic at-rule (nil interp path)
		"@page { margin: 0 }",                                            // at-rule with nil value interp
		".a { \\41 : 1 }",                                                // escaped identifier char
		".foo\\.bar { x: 1 }",                                            // escape mid-identifier in selector
		"@each $a, $b in (1 2, 3 4, 5) { .c { x: $a } }",                 // destructure fewer values -> null
		"a { x: 1 } .b { @extend a }",                                    // extend element (isBoundaryBefore)
		"ab b { x: 1 } .c { @extend b }",                                 // extend retries past non-boundary
		"> .b { x: 1 }",                                                  // top-level leading combinator
		".a > { x: 1 }",                                                  // trailing combinator (parseComplex break)
		".a { b: (1 / foo) }",                                            // slash binary with non-number
		".a { v: -1 / -2 }",                                              // isLiteralNumberish unary
		".a { v: foo / bar }",                                            // canStartValue "/"
		"/* a #{1} b #{2} */ .a{x:1}",                                    // loud comment multi interp
		"/* #{\"{\"} */ .a{x:1}",                                         // loud comment nested braces in interp
		".a { v: \"pre #{1 + 1} post\" }",                                // quoted-string interpolation
		".a { v: -foo-bar }",                                             // scanIdentifier leading "-"
		".a { v: foo\\9 zz }",                                            // scanIdentifier escape in body
		".a { foo: 1 { bar: 2 } }",                                       // selectorLike default (number value)
		".a { foo: & bar { x: 1 } }",                                     // selectorLike Parent + list-true
		".a { :hover { color: red } }",                                   // empty interpolated name -> [""]
		".a[data-x=\"#{ x{y} }\"] { color: red }",                        // scanQuotedRaw balanced braces in interp
		"@import \"a.css\" url(b);",                                      // plain @import with url() trailer
		"--top-level: 1",                                                 // top-level custom property
		"@-webkit-keyframes spin { from { top: 0 } }",                    // scanIdentifier leading "-" (at-rule)
		".a { v: 1, 2, }",                                                // trailing comma -> parseCommaList break
		".a { color: red; .b {} }",                                       // empty nested rule skipped in block
		".a { color: red; @media screen {} }",                            // empty nested at-rule skipped in block
		"@media screen { .empty {} .a { color: red } }",                  // empty rule inside a block (emitDeclList skip)
	}
	for _, src := range ok {
		okCompile(t, src)
	}
}

// TestBuiltinBranches exercises builtin-function branches via public functions.
func TestBuiltinBranches(t *testing.T) {
	cases := map[string]string{
		"@use \"sass:math\"; .a{v: math.cos(100grad)}":                        "",      // trig grad unit
		".a{v: math.clamp(5, 1, 10)}":                                         "5",     // clamp value below min
		".a{v: comparable(1, 2px)}":                                           "",      // comparable unitless
		".a{v: nth((a:1,b:2), 1)}":                                            "a",     // asListVal on map
		".a{v: join((), ())}":                                                 "",      // join undecided sep -> space
		".a{v: join(1, 2, comma, true)}":                                      "",      // join bracketed arg
		".a{v: inspect(keywords((a: 1)))}":                                    "a: 1",  // keywords on a map arg
		".a{v: length(map.get((), x))}":                                       "",      // asMapVal empty list
		".a{v: map.get((a: (b: 1)), a, c)}":                                   "",      // nested map key not found
		".a{v: map.get((a: 1), a, b)}":                                        "",      // nested cur not a map
		".a{v: selector.unify(a b, c)}":                                       "",      // selectorText list space join
		".a{v: selector.unify(1, 2)}":                                         "",      // selectorText serialize fallback
		".a{v: inspect((1, 2))}":                                              "1, 2",  // inspect comma list
		".a{v: inspect(list.slash(1, 2))}":                                    "1 / 2", // inspect slash list
		"@mixin m($a...){ x: inspect(keywords($a)) } .a{ @include m($x: 1) }": "",      // keywords
		".a{v: unique-id()}":                                                  "uid",   // unique-id
		".a{v: str-insert(\"abc\", \"X\", -100)}":                             "",      // str-insert pos<0
		".a{v: str-insert(\"abc\", \"X\", 100)}":                              "",      // str-insert pos>len
		".a{v: str-slice(\"abc\", -100)}":                                     "",      // str-slice start<1
		".a{v: str-slice(\"abc\", 3, 1)}":                                     "",      // str-slice start>end
		".a{v: string.split(\"abc\", \"\")}":                                  "",      // str-split empty separator
		".a{v: saturation(red)}":                                              "",      // fnSaturation
		".a{v: mix(rgba(255,0,0,0), blue, 100%)}":                             "",      // fnMix w*a==-1
		".a{v: lighten(white, 50%)}":                                          "",      // clampPct v>100
		".a{v: min(1, 2...)}":                                                 "",      // evalArgs spread scalar default
		".a{v: foo(1 2 3...)}":                                                "...",   // plainCSSFunction spread
	}
	for src, want := range cases {
		src = "@use \"sass:math\"; @use \"sass:list\"; @use \"sass:map\"; @use \"sass:string\"; @use \"sass:selector\"; @use \"sass:meta\"; " + src
		got := compile(t, src)
		if want != "" && !strings.Contains(got, want) {
			t.Errorf("%q => want substr %q got %q", src, want, got)
		}
	}
}

// renderImp compiles src with a map-backed importer.
func renderImp(t *testing.T, src string, files map[string]string) (Result, error) {
	t.Helper()
	imp := func(url string) (string, string, bool) {
		if s, ok := files[url]; ok {
			return s, url, true
		}
		return "", "", false
	}
	return Render(src, false, false, imp)
}

// TestModulePaths exercises @use/@forward/@import branches needing an importer.
func TestModulePaths(t *testing.T) {
	files := map[string]string{
		"m":      "$x: 1 !default; @mixin mm { a: 1 }",
		"inner":  "$iv: 7;",
		"outer":  `@forward "inner" as p-*;`,
		"selfa":  `@use "selfa";`,
		"bad":    ".a{ ",
		"badimp": ".a{ ",
	}

	// @use as * (NoNS global merge).
	if _, err := renderImp(t, `@use "m" as *; .a{ v: $x }`, files); err != nil {
		t.Errorf("use as *: %v", err)
	}
	// @use ... with (...) including a !default-flagged config var.
	if _, err := renderImp(t, `@use "m" with ($x: 9 !default); .a{ v: m.$x }`, files); err != nil {
		t.Errorf("use with default config: %v", err)
	}
	// @use cached (same module twice).
	if _, err := renderImp(t, `@use "m" as a; @use "m" as b; .z{ v: a.$x b.$x }`, files); err != nil {
		t.Errorf("use cached: %v", err)
	}
	// @forward "sass:..." early return.
	if _, err := renderImp(t, `@forward "sass:math";`, files); err != nil {
		t.Errorf("forward sass: %v", err)
	}
	// @forward with prefix then @use (forwarded member merge with prefix).
	if _, err := renderImp(t, `@use "outer"; .a{ v: outer.$p-iv }`, files); err != nil {
		t.Errorf("forward prefix: %v", err)
	}
	// namespaced undefined variable.
	if _, err := renderImp(t, `@use "m" as a; .a{ v: a.$nope }`, files); err == nil {
		t.Error("namespaced undefined: want error")
	}
	// namespaced missing mixin.
	if _, err := renderImp(t, `@use "m" as a; .a{ @include a.nope }`, files); err == nil {
		t.Error("namespaced missing mixin: want error")
	}
	// module loop.
	if _, err := renderImp(t, `@use "selfa";`, files); err == nil {
		t.Error("module loop: want error")
	}
	// parse error inside a @used module.
	if _, err := renderImp(t, `@use "bad";`, files); err == nil {
		t.Error("bad module: want error")
	}
	// parse error inside an @imported module.
	if _, err := renderImp(t, `@import "badimp";`, files); err == nil {
		t.Error("bad import: want error")
	}
}

// TestDivisionByZeroInfinity covers infinity/NaN serialization (numberCalcRepr)
// and the sign() helper via math.div by zero.
func TestDivisionByZeroInfinity(t *testing.T) {
	cases := map[string]string{
		".a{v: math.div(1, 0)}":   "calc(infinity)",
		".a{v: math.div(-1, 0)}":  "calc(-infinity)",
		".a{v: math.div(1px, 0)}": "calc(infinity * 1px)",
	}
	for src, want := range cases {
		got := compile(t, "@use \"sass:math\"; "+src)
		if !strings.Contains(got, want) {
			t.Errorf("%q => want %q got %q", src, want, got)
		}
	}
}

// TestUnitConversionBranches exercises number.go convertUnitList branches.
func TestUnitConversionBranches(t *testing.T) {
	okCompile(t, ".a{ v: (6px / 2s) == (3px / 1000ms) }") // denominator conversion (non-multiply)
	okCompile(t, ".a{ v: (2px / 1s) + (1px / 2000ms) }")  // denominator alignment
}

// TestMapEquality covers value.go map equals mismatch branch.
func TestMapEquality(t *testing.T) {
	if got := compile(t, ".a{ v: (a: 1) == (b: 1) }"); !strings.Contains(got, "false") {
		t.Errorf("map neq: %q", got)
	}
	if got := compile(t, ".a{ v: (a: 1) == (a: 2) }"); !strings.Contains(got, "false") {
		t.Errorf("map val neq: %q", got)
	}
}

// TestCompressedAtRule covers the compressed no-body at-rule branch.
func TestCompressedAtRule(t *testing.T) {
	if got := compileC(t, "@import \"https://x/y.css\";"); !strings.Contains(got, "@import") {
		t.Errorf("compressed import: %q", got)
	}
}

// TestIndentedTabs covers convertIndented tab-indent handling.
func TestIndentedTabs(t *testing.T) {
	res, err := Render(".a\n\tx: 1\n", true, false, nil)
	if err != nil {
		t.Fatalf("indented tabs: %v", err)
	}
	if !strings.Contains(res.CSS, "x: 1") {
		t.Errorf("got %q", res.CSS)
	}
}

// --- direct-call coverage of defensive / marker code ---

type fakeValue struct{}

func (fakeValue) isTruthy() bool    { return true }
func (fakeValue) equals(Value) bool { return false }
func (fakeValue) sep() Separator    { return SepSpace }
func (fakeValue) asList() []Value   { return nil }

type fakeExpr struct{}

func (fakeExpr) expr() {}

func TestDefensiveDirectCalls(t *testing.T) {
	e := newEvaluator(nil)

	// arith / numberArith default (unknown operator) -> nil.
	if v := e.arith("~", &SassString{Text: "a"}, &SassString{Text: "b"}); v != nil {
		t.Errorf("arith default: want nil got %v", v)
	}
	if v := e.numberArith("~", newNumber(1), newNumber(2)); v != nil {
		t.Errorf("numberArith default: want nil got %v", v)
	}

	// evalArgs(nil) -> empty pos/named.
	pos, named := e.evalArgs(nil)
	if len(pos) != 0 || len(named) != 0 {
		t.Errorf("evalArgs(nil): %v %v", pos, named)
	}

	// bindResolved(nil, ...) -> no-op (defensive nil ParamList).
	e.bindResolved(nil, nil, nil)

	// evalExpr with an unhandled Expr type fails.
	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Error("evalExpr(unknown) did not panic")
			}
		}()
		e.evalExpr(fakeExpr{})
	}()

	// lookupBuiltin with an alias to a non-existent module -> not found; also
	// exercises moduleRegistry's default branch.
	e.env.builtinAliases = map[string]string{"x": "bogus"}
	if _, ok := e.lookupBuiltin("x", "foo"); ok {
		t.Error("lookupBuiltin bogus module: want !ok")
	}
	if reg := moduleRegistry("bogus"); reg != nil {
		t.Error("moduleRegistry bogus: want nil")
	}

	// typeName / serializeValue fallbacks for an unknown Value type.
	if got := typeName(fakeValue{}); got != "unknown" {
		t.Errorf("typeName fallback: %q", got)
	}
	if got := serializeValue(fakeValue{}, false); got != "" {
		t.Errorf("serializeValue fallback: %q", got)
	}

	// blankBeforeOf on a declaration node -> false.
	if blankBeforeOf(&cssDeclaration{}) {
		t.Error("blankBeforeOf(decl): want false")
	}

	// unitString on a unit-less number.
	if got := newNumber(1).unitString(); got != "" {
		t.Errorf("unitString: %q", got)
	}

	// numberCalcRepr direct (all three cores + unit form).
	for _, tc := range []struct {
		n    *Number
		want string
	}{
		{&Number{Val: math.Inf(1)}, "calc(infinity)"},
		{&Number{Val: math.Inf(-1)}, "calc(-infinity)"},
		{&Number{Val: math.NaN()}, "calc(NaN)"},
		{&Number{Val: math.Inf(1), Numer: []string{"px"}}, "calc(infinity * 1px)"},
	} {
		if got := numberCalcRepr(tc.n); got != tc.want {
			t.Errorf("numberCalcRepr: want %q got %q", tc.want, got)
		}
	}

	// cssNode marker methods.
	var _ cssNode = (*cssStyleRule)(nil)
	(&cssStyleRule{}).cssNode()
	(&cssDeclaration{}).cssNode()
	(&cssComment{}).cssNode()
	(&cssAtRule{}).cssNode()

	// selectorList.String.
	sl := selectorList{complexes: []complexSelector{parseComplex(".a")}}
	if s := sl.String(); s == "" {
		t.Error("selectorList.String empty")
	}

	// sign() both directions.
	if sign(-2) != -1 || sign(2) != 1 {
		t.Error("sign")
	}

	// canStartValue rejects a leading "/" (grammar never feeds it one, but the
	// enumeration branch is exercised directly).
	if newParser("/x").canStartValue() {
		t.Error("canStartValue('/'): want false")
	}

	// isLiteralNumberish accepts a signed numeric literal (Unary case).
	if !isLiteralNumberish(&Unary{Op: "-", Expr: &NumberLit{Val: 1}}) {
		t.Error("isLiteralNumberish(-1): want true")
	}

	// resolveNesting with an empty parent returns the child unchanged.
	if got := resolveNesting(parseSelectorList(".a"), selectorList{}); len(got.complexes) != 1 {
		t.Errorf("resolveNesting empty parent: %v", got)
	}

	// parseComplex handles a trailing combinator/whitespace (break on i>=n).
	_ = parseComplex(".a > ")

	// peekAt past end of source returns 0.
	if newParser("a").peekAt(10) != 0 {
		t.Error("peekAt out of range: want 0")
	}

	// commentInterp on empty text yields a single empty part.
	if got := commentInterp(""); len(got.Parts) != 1 {
		t.Errorf("commentInterp(\"\"): %v", got.Parts)
	}

	// selectorLike default branch (a non-selector value type).
	if selectorLike(&NumberLit{Val: 1}) {
		t.Error("selectorLike(number): want false")
	}

	// resolveInterp tolerates a nil Interp.
	if got := e.resolveInterp(nil); got != "" {
		t.Errorf("resolveInterp(nil): %q", got)
	}

	// evalMedia falls back to the root container when the frame has no media
	// parent (never nil through the public API).
	fr := &frame{container: e.root, rootContainer: e.root, group: &groupInfo{}}
	e.evalMedia(&Media{Query: &Interp{Parts: []any{"screen"}}}, fr)

	// evalBinary "/" slash-flagged with non-number operands routes to arith.
	if v := e.evalBinary(&Binary{Op: "/", Slash: true,
		Left:  &StringLit{Parts: &Interp{Parts: []any{"a"}}, Quoted: true},
		Right: &NumberLit{Val: 2}}); v == nil {
		t.Error("evalBinary slash arith: want value")
	}
}

// TestRethrowIfNotSass covers both branches of the shared recover classifier.
func TestRethrowIfNotSass(t *testing.T) {
	se := &SassError{Msg: "boom"}
	if got := rethrowIfNotSass(se); got != se {
		t.Errorf("rethrowIfNotSass(SassError): want passthrough got %v", got)
	}
	defer func() {
		r := recover()
		if r != "not-a-sass-error" {
			t.Errorf("rethrowIfNotSass re-panic: got %v", r)
		}
	}()
	rethrowIfNotSass("not-a-sass-error")
}

// TestParseBlockExpectBrace covers parseBlock's opening-brace guard via direct
// construction.
func TestParseBlockExpectBrace(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("parseBlock without '{' did not panic")
		}
	}()
	p := newParser("nope")
	p.parseBlock()
}
