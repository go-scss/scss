// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import (
	"strings"
	"testing"
)

// pcssImp compiles `src` (an entry stylesheet) with a map importer that serves
// every value under a canonical URL ending in ".css", so @use/@import of those
// keys exercises the plain-CSS loading path.
func pcssImp(t *testing.T, src string, files map[string]string) string {
	t.Helper()
	imp := func(url string) (string, string, bool) {
		if s, ok := files[url]; ok {
			return s, url + ".css", true
		}
		return "", "", false
	}
	res, err := Render(src, false, false, imp)
	if err != nil {
		t.Fatalf("compile %q: %v", src, err)
	}
	return res.CSS
}

// pcss compiles `@use "p"` where p resolves to a plain-CSS file with body css.
func pcss(t *testing.T, css string) string {
	t.Helper()
	return pcssImp(t, `@use "p";`, map[string]string{"p": css})
}

func TestPlainCSSValues(t *testing.T) {
	cases := []struct{ in, want string }{
		{"a {\n  and: true and false;\n  or: true or false;\n  not: not true;\n}", "a {\n  and: true and false;\n  or: true or false;\n  not: not true;\n}\n"},
		{"a {\n  x: null;\n}", "a {\n  x: null;\n}\n"},
		{"a {\n  single-equals: alpha(opacity=65);\n}", "a {\n  single-equals: alpha(opacity=65);\n}\n"},
		{"a {b: 1/2/foo/bar}", "a {\n  b: 1/2/foo/bar;\n}\n"},
		{"a {b: 1/ / /bar}", "a {\n  b: 1///bar;\n}\n"},
		{"a {b: 1///bar}", "a {\n  b: 1///bar;\n}\n"},
		{"a {b: 1px solid   red}", "a {\n  b: 1px solid red;\n}\n"},
		{"a {b: 1 ,2,  3}", "a {\n  b: 1, 2, 3;\n}\n"},
		{"a {b: hsl(0, 100%, 50%)}", "a {\n  b: hsl(0, 100%, 50%);\n}\n"},
		{"a {b: foo( 1 , 2 )}", "a {\n  b: foo(1, 2);\n}\n"},
		{"a {b: var(--c, )}", "a {\n  b: var(--c, );\n}\n"},
		{"a {b: calc(1px)}", "a {\n  b: 1px;\n}\n"},
		{"a {b: calc(1px + 1%)}", "a {\n  b: calc(1px + 1%);\n}\n"},
		{"a {b: url(whatever)}", "a {\n  b: url(whatever);\n}\n"},
		{"a {b: url( http://a/b )}", "a {\n  b: url(http://a/b);\n}\n"},
		{"a {b: \"hello\"}", "a {\n  b: \"hello\";\n}\n"},
		{"a {b: 1/}", "a {\n  b: 1/;\n}\n"},
		{".hacks {\n  *x: y;\n  :x: y;\n  #x: y;\n  .x: y;\n}", ".hacks {\n  *x: y;\n  :x: y;\n  #x: y;\n  .x: y;\n}\n"},
		{"a {--b: c}", "a {\n  --b: c;\n}\n"},
		{"a {--b: {c: d}}", "a {\n  --b: {c: d};\n}\n"},
		{"a {--b: `~@#$%^&*()_-+={[]}|?/><}", "a {\n  --b: `~@#$%^&*()_-+={[]}|?/><;\n}\n"},
	}
	for _, c := range cases {
		if got := pcss(t, c.in); got != c.want {
			t.Errorf("value %q:\n got %q\nwant %q", c.in, got, c.want)
		}
	}
}

func TestPlainCSSNesting(t *testing.T) {
	cases := []struct{ in, want string }{
		{"a {b {c: d}}", "a {\n  b {\n    c: d;\n  }\n}\n"},
		{"a {& {b: c}}", "a {\n  & {\n    b: c;\n  }\n}\n"},
		{"a {.b&.c {d: e}}", "a {\n  .b&.c {\n    d: e;\n  }\n}\n"},
		{"a {+ b {c: d}}", "a {\n  + b {\n    c: d;\n  }\n}\n"},
		{"a, b {c, d {e: f}}", "a, b {\n  c, d {\n    e: f;\n  }\n}\n"},
		{"a {\n  b: c;\n  d {e: f}\n  g: h;\n}", "a {\n  b: c;\n  d {\n    e: f;\n  }\n  g: h;\n}\n"},
		// at-rule bubbling: direct child of a top-level rule hoists.
		{"a {@media b {c: d}}", "@media b {\n  a {\n    c: d;\n  }\n}\n"},
		{"a {@supports (b: c) {d: e}}", "@supports (b: c) {\n  a {\n    d: e;\n  }\n}\n"},
		{"a {@b {c: d}}", "@b {\n  a {\n    c: d;\n  }\n}\n"},
		// nested at-rules (parent is a nested rule) stay put.
		{"a { b {@media c {d: e}}}", "a {\n  b {\n    @media c {\n      d: e;\n    }\n  }\n}\n"},
		{"a { b {@c {d: e}}}", "a {\n  b {\n    @c {\n      d: e;\n    }\n  }\n}\n"},
		// interleaved: outer at-rule bubbles, inner (under nested rule) stays.
		{"a {\n  @media b {\n    c {\n      @media (d) {\n        e: f;\n      }\n    }\n  }\n}", "@media b {\n  a {\n    c {\n      @media (d) {\n        e: f;\n      }\n    }\n  }\n}\n"},
		// split: content before/after a bubbled at-rule stays in copies of the rule.
		{"a {x: y; @media b {c: d}; z: w}", "a {\n  x: y;\n}\n@media b {\n  a {\n    c: d;\n  }\n}\na {\n  z: w;\n}\n"},
	}
	for _, c := range cases {
		if got := pcss(t, c.in); got != c.want {
			t.Errorf("nesting %q:\n got %q\nwant %q", c.in, got, c.want)
		}
	}
}

func TestPlainCSSAtRules(t *testing.T) {
	cases := []struct{ in, want string }{
		{"@media (a) AnD (b) {x {y: z}}", "@media (a) and (b) {\n  x {\n    y: z;\n  }\n}\n"},
		{"@media (a)and (b) {x {y: z}}", "@media (a) and (b) {\n  x {\n    y: z;\n  }\n}\n"},
		{"@function --a(--b <color>) {result: c}", "@function --a(--b <color>) {\n  result: c;\n}\n"},
		{"@FUNCTION --a() {\n  result: $b;\n}", "@FUNCTION --a() {\n  result: $b;\n}\n"},
		{"@charset \"utf-8\";\na{b:c}", "@charset \"utf-8\";\na {\n  b: c;\n}\n"},
		// @import passthrough and hoisting above a preceding style rule.
		{"@import \"whatever\";", "@import \"whatever\";\n"},
		{"@import url(whatever);", "@import url(whatever);\n"},
		{"a{b:c} @import \"x.css\";", "@import \"x.css\";\na {\n  b: c;\n}\n"},
		// media params that fail to parse fall back to whitespace collapse.
		{"@media  screen {a{b:c}}", "@media screen {\n  a {\n    b: c;\n  }\n}\n"},
		// loud comment preserved as a node.
		{"/* hi */\na{b:c}", "/* hi */\na {\n  b: c;\n}\n"},
	}
	for _, c := range cases {
		if got := pcss(t, c.in); got != c.want {
			t.Errorf("atrule %q:\n got %q\nwant %q", c.in, got, c.want)
		}
	}
}

func TestPlainCSSErrors(t *testing.T) {
	for _, c := range []struct{ in, msg string }{
		{"}", "unexpected"},
		{"a {b: c", "expected \"}\""},
		{"a {b c}", "expected \":\""},
		{"@media a {x{y:z}", "expected \"}\""},
	} {
		_, err := parsePlainCSS(c.in)
		if err == nil || !strings.Contains(err.Error(), c.msg) {
			t.Errorf("parsePlainCSS(%q) err = %v, want containing %q", c.in, err, c.msg)
		}
	}
}

// TestPlainCSSImportPaths covers the @use/@import wiring for .css resolution,
// including nested @import (which combines with the enclosing selector) and the
// diamond re-use of an already-loaded module.
func TestPlainCSSImportPaths(t *testing.T) {
	if got := pcssImp(t, `@import "p";`, map[string]string{"p": "c {d: e}"}); got != "c {\n  d: e;\n}\n" {
		t.Errorf("top-level @import css: %q", got)
	}
	if got := pcssImp(t, `a {@import "p"}`, map[string]string{"p": "b {c: d}"}); got != "a b {\n  c: d;\n}\n" {
		t.Errorf("nested @import css: %q", got)
	}
	// @use twice reuses the loaded module (no duplicate emission).
	if got := pcssImp(t, `@use "p"; @use "p" as q;`, map[string]string{"p": "c {d: e}"}); got != "c {\n  d: e;\n}\n" {
		t.Errorf("diamond @use css: %q", got)
	}
	// A malformed plain-CSS file surfaces its parse error through @use and @import.
	badImp := func(url string) (string, string, bool) { return "}", url + ".css", true }
	if _, err := Render(`@use "p";`, false, false, badImp); err == nil {
		t.Error("@use of malformed .css: want error")
	}
	if _, err := Render(`@import "p";`, false, false, badImp); err == nil {
		t.Error("@import of malformed .css: want error")
	}
}

func TestPlainValueUnits(t *testing.T) {
	for _, c := range []struct {
		in string
		ok bool
	}{
		{"1px", true}, {"50%", true}, {"1.5", true}, {"+2", true}, {"-3em", true},
		{"", false}, {".", false}, {"abc", false}, {"1%x", false}, {"1 2", false}, {"1px!", false},
	} {
		if got := isSimpleNumber(c.in); got != c.ok {
			t.Errorf("isSimpleNumber(%q) = %v, want %v", c.in, got, c.ok)
		}
	}
}

// TestPlainCSSEdgeBranches exercises the scanner's defensive and rarely-taken
// branches (escapes, unterminated strings, brace/paren depth in custom-property
// and url() values, media-query parse fallback, selector attribute matchers).
func TestPlainCSSEdgeBranches(t *testing.T) {
	// Inputs that must parse and serialise to an exact result.
	ok := []struct{ in, want string }{
		{"a[b=c] {x: y}", "a[b=c] {\n  x: y;\n}\n"},
		{"a[b=\"c d\"] {x: y}", "a[b=\"c d\"] {\n  x: y;\n}\n"},
		{"a {--b: \"x;y\" z}", "a {\n  --b: \"x;y\" z;\n}\n"},
		{"a {--b: foo(;) bar}", "a {\n  --b: foo(;) bar;\n}\n"},
		{"a {b: url(a(b) \"c\")}", "a {\n  b: url(a(b) \"c\");\n}\n"},
		{"a {b: \"x\\\"y\"}", "a {\n  b: \"x\\\"y\";\n}\n"},
		{"a {b: url(x) [1]}", "a {\n  b: url(x) [1];\n}\n"},
		{"\"x\": y;\na {b: c}", "\"x\": y;\na {\n  b: c;\n}\n"},
		{"@media ) {a{b:c}}", "@media ) {\n  a {\n    b: c;\n  }\n}\n"},
		{"@supports (a: \"b:c\") {x{y:z}}", "@supports (a: \"b:c\") {\n  x {\n    y: z;\n  }\n}\n"},
		{"@supports (a : b) {x{y:z}}", "@supports (a: b) {\n  x {\n    y: z;\n  }\n}\n"},
		{"a \"x\\\"y\" {z: w}", "a \"x\\\"y\" {\n  z: w;\n}\n"},
	}
	for _, c := range ok {
		nodes, err := parsePlainCSS(c.in)
		if err != nil {
			t.Errorf("parse %q: %v", c.in, err)
			continue
		}
		if got := serialize(&cssRoot{nodes: nodes}, false); got != c.want {
			t.Errorf("edge %q:\n got %q\nwant %q", c.in, got, c.want)
		}
	}
	// Inputs whose parse must fail (exercising eof/return paths in the scanner).
	bad := []string{
		"a {b: c}\nd e",       // fail() newline counting + missing colon
		"a {--b: c",           // readCustomValue eof + missing '}'
		"a {b: \"xyz",         // stringLit eof inside a value + missing '}'
		"a \"unterminated {z", // skipString eof in a selector prelude
		"a {\"bad: y}",        // skipStringIn eof in findDeclColon
	}
	for _, in := range bad {
		if _, err := parsePlainCSS(in); err == nil {
			t.Errorf("parsePlainCSS(%q) = nil error, want failure", in)
		}
	}
}

// TestPlainCSSScannerFns drives the lower-level scanning helpers directly to
// reach their balanced-group, escape and end-of-input branches.
func TestPlainCSSScannerFns(t *testing.T) {
	if got := findDeclColon("f(x)[y]: z"); got != 7 {
		t.Errorf("findDeclColon groups: %d", got)
	}
	if got := findDeclColon("\"a\\\"b\": z"); got != 6 {
		t.Errorf("findDeclColon string escape: %d", got)
	}
	if got := findDeclColon("no colon here"); got != -1 {
		t.Errorf("findDeclColon none: %d", got)
	}
	if got := findDeclColon("a): b"); got != 2 {
		t.Errorf("findDeclColon stray close: %d", got)
	}
	if got := findDeclColon("  x: y"); got != 3 {
		t.Errorf("findDeclColon leading space: %d", got)
	}
	// A custom property terminated by ';' (not the block close) followed by a
	// normal declaration exercises readCustomValue's depth-0 ';' return.
	if n, err := parsePlainCSS("a {--b: c; e: f}"); err != nil {
		t.Errorf("custom then decl: %v", err)
	} else if got := serialize(&cssRoot{nodes: n}, false); got != "a {\n  --b: c;\n  e: f;\n}\n" {
		t.Errorf("custom then decl: %q", got)
	}
	if got := normPlainValue("url(abc"); got != "url(abc)" {
		t.Errorf("normPlainValue url eof: %q", got)
	}
	// Custom-property value with bracketed and parenthesised groups + trailing
	// slash exercises readCustomValue depth handling and peekAt at end-of-input.
	nodes, err := parsePlainCSS("a {--b: [x] (y)}\t/")
	if err == nil {
		t.Fatal("expected trailing-slash parse to fail")
	}
	_ = nodes
	if n, err := parsePlainCSS("a {--b: [x] (y) z}"); err != nil {
		t.Errorf("custom brackets: %v", err)
	} else if got := serialize(&cssRoot{nodes: n}, false); got != "a {\n  --b: [x] (y) z;\n}\n" {
		t.Errorf("custom brackets: %q", got)
	}
}

// TestPlainCSSScssSide covers the SCSS-syntax changes that support plain-CSS
// interop: var() spread expansion and its empty second argument, the silent
// comment inside a special function, and @import url() serialisation.
func TestPlainCSSScssSide(t *testing.T) {
	cases := []struct{ src, want string }{
		{"$x: --c, d;\na {b: var($x...)}", "a {\n  b: var(--c, d);\n}\n"},
		{"$x: --c;\na {b: var($x...)}", "a {\n  b: var(--c);\n}\n"},
		{"a {b: var(--c,)}", "a {\n  b: var(--c, );\n}\n"},
		{"a {b: var(--c, d)}", "a {\n  b: var(--c, d);\n}\n"},
		{"a {b: element(//\n  c)}", "a {\n  b: element( c);\n}\n"},
		{"a {b: -a-calc(c //\n  )}", "a {\n  b: -a-calc(c  );\n}\n"},
		{"@import url(foo.css);", "@import url(foo.css);\n"},
		{"@import URL(foo.css);", "@import URL(foo.css);\n"},
	}
	for _, c := range cases {
		res, err := Render(c.src, false, false, nil)
		if err != nil {
			t.Errorf("render %q: %v", c.src, err)
			continue
		}
		if res.CSS != c.want {
			t.Errorf("scss %q:\n got %q\nwant %q", c.src, res.CSS, c.want)
		}
	}
}

func TestPlainHelpers(t *testing.T) {
	if got := normSelector("a   b  ,  c"); got != "a b, c" {
		t.Errorf("normSelector: %q", got)
	}
	if got := normAtParams("(b:c)"); got != "(b: c)" {
		t.Errorf("normAtParams: %q", got)
	}
	if got := normAtParams("\"x : y\""); got != "\"x : y\"" {
		t.Errorf("normAtParams string: %q", got)
	}
	if got := lastIdent("-webkit-calc"); got != "-webkit-calc" {
		t.Errorf("lastIdent: %q", got)
	}
	// A custom property whose leading token is not a colon-declaration is a rule.
	if got := pcss(t, "--x foo {y: z}"); got != "--x foo {\n  y: z;\n}\n" {
		t.Errorf("custom-as-selector: %q", got)
	}
}
