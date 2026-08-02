// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss_test

import (
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-scss/scss"
)

func TestCompileStringBasic(t *testing.T) {
	res, err := scss.CompileString(".a { x: 1 + 1; }", nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.CSS != ".a {\n  x: 2;\n}\n" {
		t.Errorf("got %q", res.CSS)
	}
}

// TestMediaBraceTrailingComment verifies that a loud comment written on the
// opening-brace line of an @media prelude is attached to that brace line rather
// than dropped to its own line. Output byte-verified against dart-sass 1.102.0
// (sass-spec libsass-closed-issues/issue_1567).
func TestMediaBraceTrailingComment(t *testing.T) {
	res, err := scss.CompileString("@media screen {  /* c */\n  a { x: 1 }\n}\n", nil)
	if err != nil {
		t.Fatal(err)
	}
	want := "@media screen { /* c */\n  a {\n    x: 1;\n  }\n}\n"
	if res.CSS != want {
		t.Errorf("got %q want %q", res.CSS, want)
	}
}

// TestCloseBraceTrailingComment verifies that a loud comment written on the
// same source line as a rule's or at-rule's closing brace is attached to that
// brace line (`} /* c */`) rather than dropped to its own line. Byte-verified
// against dart-sass 1.102.0 (sass-spec libsass-closed-issues/issue_1007).
func TestCloseBraceTrailingComment(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"a {\n  x: 1\n} /* end */\n", "a {\n  x: 1;\n} /* end */\n"},
		{"@font-face {\n  font-family: x\n} /* trail */\n", "@font-face {\n  font-family: x;\n} /* trail */\n"},
	} {
		res, err := scss.CompileString(tc.in, nil)
		if err != nil {
			t.Fatal(err)
		}
		if res.CSS != tc.want {
			t.Errorf("in %q: got %q want %q", tc.in, res.CSS, tc.want)
		}
	}
}

// TestFunctionBodyCommentDropped verifies that a loud comment inside a
// user-defined function body is discarded rather than leaked into the output: a
// function produces a value, not CSS. Byte-verified against dart-sass 1.102.0
// (sass-spec libsass-closed-issues/issue_646).
func TestFunctionBodyCommentDropped(t *testing.T) {
	res, err := scss.CompileString("@function foo() {\n  /* $bar: 1; */\n @return true;\n}\n\nfoo {\n  foo: foo();\n}\n", nil)
	if err != nil {
		t.Fatal(err)
	}
	want := "foo {\n  foo: true;\n}\n"
	if res.CSS != want {
		t.Errorf("got %q want %q", res.CSS, want)
	}
}

// TestSplatBeforePositional verifies that an explicit positional argument
// written after a spread argument still binds before the spread's elements
// (`f([1, 2]..., 3)` binds 3 first), matching dart-sass's rest-arguments-last
// binding order. Byte-verified against dart-sass 1.102.0 (sass-spec
// callable/arguments::function/error/splat/before_positional).
func TestSplatBeforePositional(t *testing.T) {
	res, err := scss.CompileString("a {b: rgb([1, 2]..., 3)}", nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := "a {\n  b: rgb(3, 1, 2);\n}\n"; res.CSS != want {
		t.Errorf("builtin: got %q want %q", res.CSS, want)
	}
	res, err = scss.CompileString("@function f($a,$b,$c){@return ($a, $b, $c)}\na {x: f([1, 2]..., 3)}\n", nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := "a {\n  x: 3, 1, 2;\n}\n"; res.CSS != want {
		t.Errorf("user func: got %q want %q", res.CSS, want)
	}
}

// TestMediaFeatureStringVerbatim verifies that a media/import feature written
// structurally (`(min-width:0)`) is canonicalised with a post-colon space,
// while a feature whose text originates from a string or interpolation
// (`("min-width:0")`, `(#{$bar})`) is emitted verbatim without re-spacing.
// Byte-verified against dart-sass 1.102.0 (sass-spec libsass-closed-issues
// issue_1218 and issue_1322).
func TestMediaFeatureStringVerbatim(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"@media (orientation:landscape) { a{x:1} }", "@media (orientation: landscape) {\n  a {\n    x: 1;\n  }\n}\n"},
		{"@media screen and (\"min-width:0\") { a{x:1} }", "@media screen and (min-width:0) {\n  a {\n    x: 1;\n  }\n}\n"},
		{"$f: \"orientation:landscape\";\n@media (#{$f}) { a{x:1} }", "@media (orientation:landscape) {\n  a {\n    x: 1;\n  }\n}\n"},
		{"@import url(foo.css) (min-width:400px);", "@import url(foo.css) (min-width: 400px);\n"},
		{"$bar: \"min-width:400px\";\n@import url(foo.css) (#{$bar});", "@import url(foo.css) (min-width:400px);\n"},
	} {
		res, err := scss.CompileString(tc.in, nil)
		if err != nil {
			t.Fatal(err)
		}
		if res.CSS != tc.want {
			t.Errorf("in %q: got %q want %q", tc.in, res.CSS, tc.want)
		}
	}
}

// TestSassTrailingCommaContinuation verifies indented-syntax trailing-comma
// line continuation: a selector list and a @forward member list continue across
// a trailing comma, but an @extend selector (and other at-rule preludes and
// expressions) do not — a trailing comma there is a one-element list separator
// and the next line is a fresh statement. Byte-verified against dart-sass
// 1.102.0 (sass-spec directives/extend/whitespace::multiple_selectors/comma/sass
// and directives/forward/whitespace::show/after_comma/sass).
func TestSassTrailingCommaContinuation(t *testing.T) {
	opt := &scss.Options{Syntax: scss.SyntaxIndented}
	// @extend: no continuation — g extends only `a`, and `d` is a separate rule.
	res, err := scss.CompileString("a\n  b: c\nd\n  e: f\n\ng\n  @extend a,\n  d\n", opt)
	if err != nil {
		t.Fatal(err)
	}
	if want := "a, g {\n  b: c;\n}\n\nd {\n  e: f;\n}\n"; res.CSS != want {
		t.Errorf("@extend: got %q want %q", res.CSS, want)
	}
	// Selector list: continuation across the trailing comma.
	res, err = scss.CompileString("a,\nb\n  c: d\n", opt)
	if err != nil {
		t.Fatal(err)
	}
	if want := "a,\nb {\n  c: d;\n}\n"; res.CSS != want {
		t.Errorf("selector: got %q want %q", res.CSS, want)
	}
}

// TestExprTrailingCommaBeforeBrace verifies that a trailing comma ending a
// control at-rule's expression closes a one-element comma list rather than
// erroring, both in SCSS (`@each $a in b, { … }`) and the indented syntax.
// Byte-verified against dart-sass 1.102.0 (sass-spec
// directives/each::sass/multiline/in_expression).
func TestExprTrailingCommaBeforeBrace(t *testing.T) {
	res, err := scss.CompileString("@each $a in b, { c { .#{$a} { d: $a } } }", nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := "c .b {\n  d: b;\n}\n"; res.CSS != want {
		t.Errorf("scss: got %q want %q", res.CSS, want)
	}
	res, err = scss.CompileString("@each $a in b,\n c\n  .#{$a}\n    d: $a\n", &scss.Options{Syntax: scss.SyntaxIndented})
	if err != nil {
		t.Fatal(err)
	}
	if want := "c .b {\n  d: b;\n}\n"; res.CSS != want {
		t.Errorf("sass: got %q want %q", res.CSS, want)
	}
}

// TestEscapedColonSelector verifies that a backslash-escaped colon at the end
// of a type selector (`something\:`) is identifier text, so the statement is a
// style rule rather than being misread as a `something\` declaration terminated
// by the colon. Byte-verified against dart-sass 1.102.0 (sass-spec
// libsass-closed-issues/issue_2625).
func TestEscapedColonSelector(t *testing.T) {
	res, err := scss.CompileString("something\\:{ padding: 2px; }", nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := "something\\: {\n  padding: 2px;\n}\n"; res.CSS != want {
		t.Errorf("got %q want %q", res.CSS, want)
	}
}

// TestBangFlagBoundary verifies that a `!` glued to the preceding value opens a
// fresh token, so `c!important` (and the indented `c!` continued by `important`
// on the next line) serializes as `c !important`, while a Sass `!default` flag
// still terminates the value. Byte-verified against dart-sass 1.102.0 (sass-spec
// css/important::syntax/sass/multiline/after_bang).
func TestBangFlagBoundary(t *testing.T) {
	res, err := scss.CompileString("a { b: c!important; }", nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := "a {\n  b: c !important;\n}\n"; res.CSS != want {
		t.Errorf("!important: got %q want %q", res.CSS, want)
	}
	res, err = scss.CompileString("a\n  b: c!\n    important\n", &scss.Options{Syntax: scss.SyntaxIndented})
	if err != nil {
		t.Fatal(err)
	}
	if want := "a {\n  b: c !important;\n}\n"; res.CSS != want {
		t.Errorf("sass: got %q want %q", res.CSS, want)
	}
	// A Sass !default flag still ends the value list (not treated as a value).
	res, err = scss.CompileString("$x: c!default;\na { b: $x }", nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := "a {\n  b: c;\n}\n"; res.CSS != want {
		t.Errorf("!default: got %q want %q", res.CSS, want)
	}
}

// TestBlankListElement verifies dart-sass list-blank semantics: an unquoted
// empty string (e.g. from `#{""}`) is dropped from a space- or comma-separated
// list but kept in a slash-separated list, and a quoted empty string is always
// kept. Byte-verified against dart-sass 1.102.0 (sass-spec
// libsass-closed-issues/issue_1092).
func TestBlankListElement(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"a { b: foo #{\"\"} }", "a {\n  b: foo;\n}\n"},
		{"a { b: foo #{\" \"} }", "a {\n  b: foo  ;\n}\n"},
		{"a { b: (foo, #{\"\"}, bar) }", "a {\n  b: foo, bar;\n}\n"},
		{"a { b: 1 / #{\"\"} / bar }", "a {\n  b: 1//bar;\n}\n"},
		{"a { b: (foo \"\" bar) }", "a {\n  b: foo \"\" bar;\n}\n"},
	} {
		res, err := scss.CompileString(tc.in, nil)
		if err != nil {
			t.Fatal(err)
		}
		if res.CSS != tc.want {
			t.Errorf("in %q: got %q want %q", tc.in, res.CSS, tc.want)
		}
	}
}

func TestCompileStringCompressed(t *testing.T) {
	res, err := scss.CompileString(".a{x:1}", &scss.Options{Style: scss.Compressed})
	if err != nil {
		t.Fatal(err)
	}
	if res.CSS != ".a{x:1}\n" {
		t.Errorf("got %q", res.CSS)
	}
}

func TestCompileStringError(t *testing.T) {
	if _, err := scss.CompileString(".a{x:$undef}", nil); err == nil {
		t.Error("expected error")
	}
}

func TestCompileStringIndented(t *testing.T) {
	res, err := scss.CompileString(".a\n  x: 1\n", &scss.Options{Syntax: scss.SyntaxIndented})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.CSS, "x: 1") {
		t.Errorf("got %q", res.CSS)
	}
}

func TestCompileFile(t *testing.T) {
	res, err := scss.Compile(filepath.Join("testdata", "corpus", "variables.scss"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.CSS, "color: #3498db") {
		t.Errorf("got %q", res.CSS)
	}
	if len(res.LoadedURLs) == 0 {
		t.Error("expected loaded URLs")
	}
}

func TestCompileFileMissing(t *testing.T) {
	if _, err := scss.Compile("testdata/does-not-exist.scss", nil); err == nil {
		t.Error("expected error")
	}
}

func TestUseImport(t *testing.T) {
	res, err := scss.Compile(filepath.Join("testdata", "imports", "main.scss"), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"color: #336699", "display: block", "width: 12px"} {
		if !strings.Contains(res.CSS, want) {
			t.Errorf("missing %q in %q", want, res.CSS)
		}
	}
}

func TestLegacyImport(t *testing.T) {
	res, err := scss.Compile(filepath.Join("testdata", "imports", "legacy.scss"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.CSS, "color: #336699") {
		t.Errorf("got %q", res.CSS)
	}
}

func TestForward(t *testing.T) {
	// A file that @forwards and is then @used.
	src := `@use "fwd"; .a { color: fwd.$brand; }`
	res, err := scss.CompileString(src, &scss.Options{BaseDir: filepath.Join("testdata", "imports")})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.CSS, "#336699") {
		t.Errorf("forward failed: %q", res.CSS)
	}
}

func TestPlainCSSImport(t *testing.T) {
	res, err := scss.CompileString("@import \"https://example.com/x.css\";", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.CSS, "@import \"https://example.com/x.css\"") {
		t.Errorf("got %q", res.CSS)
	}
}

func TestCustomImporter(t *testing.T) {
	imp := func(url string) (string, string, bool) {
		if url == "virtual" {
			return "$x: 42px;", "virtual", true
		}
		return "", "", false
	}
	res, err := scss.CompileString("@use \"virtual\"; .a{w: virtual.$x}", &scss.Options{Importer: imp})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.CSS, "42px") {
		t.Errorf("got %q", res.CSS)
	}
}

// TestImporterWithReferrer covers the referrer-aware public importer form: it is
// given the canonical URL of the file issuing each load and resolves the URL
// relative to that file first. This reproduces the sass-spec through_other_mixin
// scenario — a meta.load-css inside a mixin defined in subdir/ must resolve
// "upstream" relative to subdir/ — proving the referrer is threaded to the public
// ImporterWithReferrer hook (which takes precedence over Importer).
func TestImporterWithReferrer(t *testing.T) {
	files := map[string]string{
		"_upstream.scss":         "a {b: in main dir}\n",
		"subdir/_midstream.scss": "@use \"sass:meta\";\n@mixin load-css($m) { @include meta.load-css($m); }\n",
		"subdir/_upstream.scss":  "a {b: in subdir}\n",
	}
	try := func(base, url string) (string, string, bool) {
		joined := path.Clean(path.Join(base, url))
		key := path.Join(path.Dir(joined), "_"+path.Base(joined)+".scss")
		key = strings.TrimPrefix(key, "./")
		if s, ok := files[key]; ok {
			return s, key, true
		}
		return "", "", false
	}
	var sawReferrer bool
	imp := func(url, referrer string, _ bool) (string, string, bool) {
		if referrer != "" {
			sawReferrer = true
			if s, key, ok := try(path.Dir(referrer), url); ok {
				return s, key, true
			}
		}
		return try(".", url)
	}
	src := "@use \"subdir/midstream\";\n@include midstream.load-css(\"upstream\");\n"
	res, err := scss.CompileString(src, &scss.Options{ImporterWithReferrer: imp})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.CSS, "in subdir") || strings.Contains(res.CSS, "in main dir") {
		t.Errorf("referrer-relative resolution failed: %q", res.CSS)
	}
	if !sawReferrer {
		t.Error("importer never received a non-empty referrer")
	}
}

// TestImporterWithReferrerPrecedence confirms ImporterWithReferrer takes
// precedence over a simultaneously-set legacy Importer.
func TestImporterWithReferrerPrecedence(t *testing.T) {
	ref := func(url, _ string, _ bool) (string, string, bool) {
		if url == "v" {
			return "$x: 9px;", "v", true
		}
		return "", "", false
	}
	legacy := func(string) (string, string, bool) { return "$x: 1px;", "v", true }
	res, err := scss.CompileString("@use \"v\"; .a{w: v.$x}",
		&scss.Options{Importer: legacy, ImporterWithReferrer: ref})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.CSS, "9px") {
		t.Errorf("ImporterWithReferrer should win over Importer: %q", res.CSS)
	}
}

// TestFileImporterReferrerRelative exercises the built-in filesystem importer's
// referrer-relative branch on real files: a meta.load-css issued from a mixin in
// sub/_mid.scss resolves "up" relative to sub/, picking sub/_up.scss rather than
// the entry directory's _up.scss.
func TestFileImporterReferrerRelative(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(p, s string) {
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(dir, "input.scss"), "@use \"sub/mid\";\n@include mid.load-css(\"up\");\n")
	write(filepath.Join(dir, "_up.scss"), "a {b: main}\n")
	write(filepath.Join(sub, "_mid.scss"), "@use \"sass:meta\";\n@mixin load-css($m) { @include meta.load-css($m); }\n")
	write(filepath.Join(sub, "_up.scss"), "a {b: sub}\n")
	res, err := scss.Compile(filepath.Join(dir, "input.scss"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.CSS, "b: sub") || strings.Contains(res.CSS, "b: main") {
		t.Errorf("on-disk referrer-relative resolution failed: %q", res.CSS)
	}
}

func TestForwardConfigShowHide(t *testing.T) {
	files := map[string]string{
		"base":   "$brand: blue !default;\n@mixin m { x: 1 }\n@function f() { @return 2 }\n.g { y: 3 }",
		"fwdcfg": `@forward "base" with ($brand: red);`,
		"fwdsh":  `@forward "base" show $brand, m;`,
		"fwdas":  `@forward "base" as b-*;`,
	}
	imp := func(url string) (string, string, bool) {
		if s, ok := files[url]; ok {
			return s, url, true
		}
		return "", "", false
	}
	cases := []string{
		`@use "fwdcfg" as c; .a { color: c.$brand }`,
		`@use "fwdsh" as c; .a { color: c.$brand; @include c.m }`,
		`@use "fwdas" as c; .a { color: c.$b-brand }`,
	}
	for _, src := range cases {
		if _, err := scss.CompileString(src, &scss.Options{Importer: imp}); err != nil {
			t.Errorf("%q: %v", src, err)
		}
	}
}

func TestLoadPathsAndExtensions(t *testing.T) {
	// LoadPaths resolution + partial/extension candidates via the file importer.
	res, err := scss.CompileString(`@use "colors"; .a { c: colors.$brand }`,
		&scss.Options{LoadPaths: []string{filepath.Join("testdata", "imports")}})
	if err != nil {
		t.Fatalf("load path import: %v", err)
	}
	if !strings.Contains(res.CSS, "#") {
		t.Errorf("got %q", res.CSS)
	}
}

func TestCompileIndentedFile(t *testing.T) {
	res, err := scss.Compile(filepath.Join("testdata", "corpus", "indented.sass"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.CSS == "" {
		t.Error("empty output")
	}
}

func TestUnresolvedImportPassthrough(t *testing.T) {
	// A relative @import that cannot be resolved becomes a plain CSS @import.
	res, err := scss.CompileString(`@import "nonexistent-xyz";`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.CSS, "@import") {
		t.Errorf("got %q", res.CSS)
	}
}
