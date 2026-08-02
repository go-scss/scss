// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import "testing"

// runProgram compiles a program through a manual evaluator so a whitebox test can
// inspect the compilation state (importClones) alongside the serialized CSS.
func runProgram(t *testing.T, src string, imp Importer) (string, *evaluator) {
	t.Helper()
	stmts, err := parseStylesheet(src)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	e := newEvaluator(imp)
	e.run(stmts)
	return serialize(e.root, false), e
}

// TestFreshImportCloneSeparatesCombineBoxes covers the step-3 combine-tree box
// separation: a module loaded FRESH while an @import is inlining, and also @used
// canonically, was previously referenced twice in the combine tree through ONE
// style rule + box. This asserts the @import copy is now an independent clone
// (its own rule, its own box, registered as an importClone with a mirror) while
// the CSS still serialises byte-for-byte as before separation.
//
// Program: `importer` @imports `imported`, which @uses `shared` for the first
// time (fresh inside the @import); `used` @uses the now-loaded `shared`. Oracle:
// dart-sass 1.102 prints two `shared` copies; go-scss's pre-step-4 output over-
// extends both identically (both carry in-used and in-imported), which is exactly
// what this behaviour-preserving step must keep until step 4 composes each clone.
func TestFreshImportCloneSeparatesCombineBoxes(t *testing.T) {
	imp := cssMapImporter(map[string]string{
		"_importer.scss": "@import \"imported\";\n",
		"_imported.scss": "@use \"shared\";\nin-imported {@extend shared}\n",
		"_used.scss":     "@use \"shared\";\nin-used {@extend shared}\n",
		"_shared.scss":   "shared {x: y}\n",
	})
	css, e := runProgram(t, "@use \"importer\";\n@use \"used\";\n", imp)

	want := "shared, in-used, in-imported {\n  x: y;\n}\n\nshared, in-used, in-imported {\n  x: y;\n}\n"
	if css != want {
		t.Fatalf("output not byte-identical to pre-separation:\ngot:\n%q\nwant:\n%q", css, want)
	}

	// Exactly one duplicate was separated by the fresh-during-import path, and it
	// carries a mirror (the re-emit path, which produces already-independent
	// clones, records no mirror).
	var mirrored []*importClone
	for _, ic := range *e.importClones {
		if len(ic.mirror) > 0 {
			mirrored = append(mirrored, ic)
		}
	}
	if len(mirrored) != 1 {
		t.Fatalf("fresh-during-import importClones with a mirror = %d, want 1", len(mirrored))
	}
	ic := mirrored[0]
	if len(ic.mirror) != 1 {
		t.Fatalf("mirror pairs = %d, want 1 (the single `shared` rule)", len(ic.mirror))
	}
	mp := ic.mirror[0]

	// The clone is a distinct rule with a distinct box from the canonical source —
	// the whole point of the separation — yet its final selector mirrors the
	// source's, so the bytes are unchanged.
	if mp.clone == mp.orig {
		t.Fatal("clone rule is the same object as the source (not separated)")
	}
	if mp.clone.box == nil || mp.orig.box == nil {
		t.Fatal("clone or source rule has no extension box")
	}
	if mp.clone.box == mp.orig.box {
		t.Fatal("clone shares the source's extension box (boxes not separated)")
	}
	if mp.clone.selector.String() != mp.orig.selector.String() {
		t.Fatalf("clone selector %q not mirrored from source %q", mp.clone.selector.String(), mp.orig.selector.String())
	}
	if mp.clone.box.value != mp.orig.selector.list {
		t.Fatal("clone box value not mirrored to the source's final selector")
	}
}

// TestFreshImportCloneNotSeparatedWhenNested covers the guard that a module loaded
// fresh inside an @import that is itself nested in a style rule (fr.hasParent) is
// NOT clone-separated: emitModuleCSS re-nests it under the enclosing selector,
// which a plain source-selector mirror cannot reproduce, so it stays on the
// shared-node path with no importClone recorded. Oracle: dart-sass 1.102.
func TestFreshImportCloneNotSeparatedWhenNested(t *testing.T) {
	imp := cssMapImporter(map[string]string{
		"_imported.scss": "@use \"used\";\nin-imported {a: b}\n",
		"_used.scss":     "in-used {c: d}\n",
	})
	css, e := runProgram(t, "outer {@import \"imported\"}\n", imp)

	want := "outer in-used {\n  c: d;\n}\nouter in-imported {\n  a: b;\n}\n"
	if css != want {
		t.Fatalf("nested-@import output changed:\ngot:\n%q\nwant:\n%q", css, want)
	}
	for _, ic := range *e.importClones {
		if len(ic.mirror) > 0 {
			t.Fatal("a nested fresh-during-import module was clone-separated; it must not be")
		}
	}
}

// TestCollectRuleMirror covers the lockstep clone/original walk across every node
// kind: a top-level style rule with a nested style rule (both paired), an at-rule
// whose nested style rule is reached through recursion, and inert declaration /
// comment leaves that are skipped.
func TestCollectRuleMirror(t *testing.T) {
	origNested := &cssStyleRule{selector: parseSelectorList(".n")}
	origTop := &cssStyleRule{selector: parseSelectorList(".t"), nodes: []cssNode{
		origNested, &cssDeclaration{name: "p", value: &SassString{Text: "v"}},
	}}
	origAtChild := &cssStyleRule{selector: parseSelectorList(".m")}
	origAt := &cssAtRule{name: "media", nodes: []cssNode{origAtChild}}
	orig := []cssNode{origTop, origAt, &cssComment{text: "/* c */"}}

	clones := cloneCSSNodes(orig)
	mirror := collectRuleMirror(clones, orig)

	if len(mirror) != 3 {
		t.Fatalf("mirror pairs = %d, want 3 (.t, .n, .m)", len(mirror))
	}
	// Every pair links a distinct clone to the matching original by selector.
	for _, mp := range mirror {
		if mp.clone == mp.orig {
			t.Fatal("mirror pair points a clone at its own original object")
		}
		if mp.clone.selector.String() != mp.orig.selector.String() {
			t.Fatalf("mispaired: clone %q vs orig %q", mp.clone.selector.String(), mp.orig.selector.String())
		}
	}
	// The nested rule under the top rule and the rule under the at-rule must both
	// be present (recursion into cssStyleRule.nodes and cssAtRule.nodes).
	seen := map[string]bool{}
	for _, mp := range mirror {
		seen[mp.orig.selector.String()] = true
	}
	for _, want := range []string{".t", ".n", ".m"} {
		if !seen[want] {
			t.Fatalf("rule %q missing from mirror", want)
		}
	}
}

// TestImportCloneMirrorPostPass covers the applyAllExtends pass-3 mirror loop over
// both branches of the box guard: a clone rule with a box (value mirrored too) and
// a clone rule without a box (selector-only mirror). The source rule's final
// selector is copied verbatim onto each clone.
func TestImportCloneMirrorPostPass(t *testing.T) {
	e := newEvaluator(nil)
	orig := &cssStyleRule{
		selector: selectorList{list: mustParseSelectorList(".a, .b")},
		original: selectorList{list: mustParseSelectorList(".a")},
		raw:      false,
	}
	cloneWithBox := &cssStyleRule{
		selector: selectorList{list: mustParseSelectorList(".stale")},
		box:      &box{value: mustParseSelectorList(".stale")},
	}
	cloneNoBox := &cssStyleRule{selector: selectorList{list: mustParseSelectorList(".stale2")}}
	*e.importClones = append(*e.importClones, &importClone{
		mirror: []ruleMirror{{clone: cloneWithBox, orig: orig}, {clone: cloneNoBox, orig: orig}},
	})

	e.applyAllExtends()

	if cloneWithBox.selector.String() != ".a, .b" {
		t.Fatalf("boxed clone selector = %q, want .a, .b", cloneWithBox.selector.String())
	}
	if cloneWithBox.box.value != orig.selector.list {
		t.Fatal("boxed clone box value not mirrored to the source's final selector")
	}
	if cloneWithBox.original.String() != ".a" {
		t.Fatalf("boxed clone original = %q, want .a", cloneWithBox.original.String())
	}
	if cloneNoBox.selector.String() != ".a, .b" {
		t.Fatalf("box-less clone selector = %q, want .a, .b", cloneNoBox.selector.String())
	}
}

// TestCloneStoreForScopeNil covers the nil-scope arm of the scope-keyed own-store
// clone directly (a fresh empty store), complementing TestCloneStoreFor which
// reaches the build-and-cache arms through cloneStoreFor.
func TestCloneStoreForScopeNil(t *testing.T) {
	e := newEvaluator(nil)
	if st := e.cloneStoreForScope(nil); st == nil {
		t.Fatal("nil scope returned no store")
	}
}
