// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss_test

import (
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
