// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import "testing"

// sassAwareImporter resolves a bare URL to a `.sass` partial when one is present
// in the file map, else to a `.scss` partial, returning the resolved key so the
// loader can pick the right grammar by extension.
func sassAwareImporter(files map[string]string) Importer {
	return func(url, _ string, _ bool) (string, string, bool) {
		for _, cand := range []string{url, url + ".sass", "_" + url + ".sass", url + ".scss", "_" + url + ".scss"} {
			if src, ok := files[cand]; ok {
				return src, cand, true
			}
		}
		return "", "", false
	}
}

// TestUseIndentedModule verifies that a `.sass` module reached through @use is
// parsed with the indented grammar (parseModuleSource's .sass branch), not the
// SCSS grammar, so its rules and members are evaluated correctly.
func TestUseIndentedModule(t *testing.T) {
	files := map[string]string{
		"_other.sass": "a\n  syntax: sass\n",
	}
	res, err := Render("@use \"other\"\n", false, false, sassAwareImporter(files))
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}
	want := "a {\n  syntax: sass;\n}\n"
	if res.CSS != want {
		t.Fatalf("got %q want %q", res.CSS, want)
	}
}

// TestImportIndentedModule verifies that a legacy @import of a `.sass` file is
// likewise parsed with the indented grammar.
func TestImportIndentedModule(t *testing.T) {
	files := map[string]string{
		"_other.sass": "a\n  syntax: sass\n",
	}
	res, err := Render("@import \"other\"\n", false, false, sassAwareImporter(files))
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}
	want := "a {\n  syntax: sass;\n}\n"
	if res.CSS != want {
		t.Fatalf("got %q want %q", res.CSS, want)
	}
}

// TestUseScssModuleStillScss guards parseModuleSource's non-.sass branch: a
// `.scss` module keeps SCSS parsing even when the importer would also expose a
// `.sass` sibling (here only the .scss exists).
func TestUseScssModuleStillScss(t *testing.T) {
	files := map[string]string{
		"_other.scss": "a {syntax: scss}\n",
	}
	res, err := Render("@use \"other\"\n", false, false, sassAwareImporter(files))
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}
	want := "a {\n  syntax: scss;\n}\n"
	if res.CSS != want {
		t.Fatalf("got %q want %q", res.CSS, want)
	}
}
