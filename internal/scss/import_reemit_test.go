// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import "testing"

// TestImportDuplicatesUsedModuleCSS verifies dart's rule that a legacy @import
// duplicates a module's CSS even when the module has already been @used and
// loaded once: `@use "shared"; @import "imported"` where _imported.scss also
// `@use`s shared prints shared's `a {b: c}` twice (sass-spec
// directives/use/css/import::use_module_used_by_import). Oracle: dart-sass 1.102.
func TestImportDuplicatesUsedModuleCSS(t *testing.T) {
	imp := cssMapImporter(map[string]string{
		"_shared.scss":   "a {b: c}\n",
		"_imported.scss": "@use \"shared\";\n",
	})
	res, err := Render("@use \"shared\";\n@import \"imported\";\n", false, false, imp)
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}
	want := "a {\n  b: c;\n}\n\na {\n  b: c;\n}\n"
	if res.CSS != want {
		t.Fatalf("got:\n%q\nwant:\n%q", res.CSS, want)
	}
}

// TestReEmitImportedCSSOutsideImport verifies that a plain @use of an
// already-loaded module (importDepth == 0) is deduped and does NOT re-emit its
// CSS: a shared module reached through two @use paths (a diamond) is printed
// once.
func TestReEmitImportedCSSOutsideImport(t *testing.T) {
	imp := cssMapImporter(map[string]string{
		"_shared.scss": "a {b: c}\n",
		"_left.scss":   "@use \"shared\";\n",
		"_right.scss":  "@use \"shared\";\n",
	})
	res, err := Render("@use \"left\";\n@use \"right\";\n", false, false, imp)
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}
	want := "a {\n  b: c;\n}\n"
	if res.CSS != want {
		t.Fatalf("got:\n%q\nwant:\n%q", res.CSS, want)
	}
}

// TestReEmitImportedCSSNoNodes covers the guard that a module with no CSS emits
// nothing on re-emit even inside an @import.
func TestReEmitImportedCSSNoNodes(t *testing.T) {
	e := newEvaluator(nil)
	e.importDepth = 1
	fr := &frame{container: e.root, rootContainer: e.root, mediaParent: e.root, atContainer: true, group: &groupInfo{}}
	e.reEmitImportedCSS(&module{}, fr)
	if len(e.root.nodes) != 0 {
		t.Fatalf("re-emitted %d nodes for an empty module, want 0", len(e.root.nodes))
	}
}

// TestCloneCSSNodes exercises the deep-clone across every output node type: the
// clone must be structurally independent (mutating a clone leaves the original
// untouched) and a style-rule clone must drop the original's extension box.
func TestCloneCSSNodes(t *testing.T) {
	if cloneCSSNodes(nil) != nil {
		t.Fatalf("cloneCSSNodes(nil) should be nil")
	}
	orig := []cssNode{
		&cssStyleRule{rawSel: "a", raw: true, box: &box{}, nodes: []cssNode{
			&cssDeclaration{name: "b", value: &SassString{Text: "c"}},
		}},
		&cssAtRule{name: "media", params: "screen", hasBody: true, nodes: []cssNode{
			&cssComment{text: "/* x */"},
		}},
		&cssComment{text: "/* top */"}, // exercises the default (comment) branch
	}
	clone := cloneCSSNodes(orig)
	if len(clone) != len(orig) {
		t.Fatalf("clone length %d, want %d", len(clone), len(orig))
	}
	sr := clone[0].(*cssStyleRule)
	if sr.box != nil {
		t.Fatalf("cloned style rule kept the original extension box")
	}
	if sr == orig[0] || &sr.nodes[0] == &orig[0].(*cssStyleRule).nodes[0] {
		t.Fatalf("style rule clone shares storage with the original")
	}
	// Mutating the clone's nested declaration must not touch the original.
	sr.nodes[0].(*cssDeclaration).name = "MUT"
	if orig[0].(*cssStyleRule).nodes[0].(*cssDeclaration).name != "b" {
		t.Fatalf("mutating the clone changed the original")
	}
	at := clone[1].(*cssAtRule)
	if &at.nodes[0] == &orig[1].(*cssAtRule).nodes[0] {
		t.Fatalf("at-rule clone shares child storage with the original")
	}
	cm := clone[2].(*cssComment)
	if cm == orig[2] {
		t.Fatalf("comment clone shares storage with the original")
	}
}
