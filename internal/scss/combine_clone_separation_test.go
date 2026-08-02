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

// TestImportCloneComposesIndependently covers the per-@import-clone composition:
// a module loaded FRESH while an @import inlines is duplicated, and the duplicate
// must carry ONLY the extends of the @import's own subtree — never the canonical
// module's cross-module extends. Program: `importer` @imports `imported`, which
// @uses `shared` for the first time (fresh inside the @import) and extends it;
// `used` @uses the now-loaded `shared` and extends it too. dart-sass 1.102 prints
// the import copy with in-imported and the canonical copy with in-used — DIFFERENT
// extend sets, the whole point of the isolation.
func TestImportCloneComposesIndependently(t *testing.T) {
	imp := cssMapImporter(map[string]string{
		"_importer.scss": "@import \"imported\";\n",
		"_imported.scss": "@use \"shared\";\nin-imported {@extend shared}\n",
		"_used.scss":     "@use \"shared\";\nin-used {@extend shared}\n",
		"_shared.scss":   "shared {x: y}\n",
	})
	css, e := runProgram(t, "@use \"importer\";\n@use \"used\";\n", imp)

	want := "shared, in-imported {\n  x: y;\n}\n\nshared, in-used {\n  x: y;\n}\n"
	if css != want {
		t.Fatalf("clone/original not composed independently:\ngot:\n%q\nwant:\n%q", css, want)
	}

	// Exactly one duplicate was produced (the fresh-during-import `shared`), it is a
	// distinct rule with its own box, and it composed to `shared, in-imported`.
	if n := len(*e.importClones); n != 1 {
		t.Fatalf("importClones = %d, want 1", n)
	}
	ic := (*e.importClones)[0]
	if len(ic.rules) != 1 {
		t.Fatalf("clone rules = %d, want 1 (the single `shared` rule)", len(ic.rules))
	}
	if got := ic.rules[0].selector.String(); got != "shared, in-imported" {
		t.Fatalf("clone selector = %q, want %q", got, "shared, in-imported")
	}
	if ic.rules[0].box == nil {
		t.Fatal("clone rule has no extension box")
	}
}

// TestReEmitCloneComposition covers the re-emit path (a module @used again inside
// an @import after it was already loaded canonically): the re-emitted copy must
// carry only the import subtree's extend. `used` @uses+extends `shared` (loading
// it), then a root @import of `imported` (which @uses+extends `shared`) re-emits a
// copy carrying in-imported, while the canonical copy carries both because the
// root is downstream of `used`. Oracle: dart-sass 1.102.
func TestReEmitCloneComposition(t *testing.T) {
	imp := cssMapImporter(map[string]string{
		"_used.scss":     "@use \"shared\";\nin-used {@extend shared}\n",
		"_imported.scss": "@use \"shared\";\nin-imported {@extend shared}\n",
		"_shared.scss":   "shared {x: y}\n",
	})
	css, e := runProgram(t, "@use \"used\";\n@import \"imported\";\n", imp)

	want := "shared, in-used, in-imported {\n  x: y;\n}\n\nshared, in-imported {\n  x: y;\n}\n"
	if css != want {
		t.Fatalf("re-emit clone not composed independently:\ngot:\n%q\nwant:\n%q", css, want)
	}
	// A re-emit produced exactly one clone and it composed to `shared, in-imported`.
	if n := len(*e.importClones); n != 1 {
		t.Fatalf("importClones = %d, want 1", n)
	}
	if got := (*e.importClones)[0].rules[0].selector.String(); got != "shared, in-imported" {
		t.Fatalf("re-emit clone selector = %q, want %q", got, "shared, in-imported")
	}
}

// TestImportCloneThroughDeepUse covers that "reached through an @import" is driven
// by the import subtree, not importDepth: `shared` is re-emitted even though it is
// @used two @use levels deep inside the imported file, where importDepth has reset
// to 0 in the sub-evaluator. dart-sass 1.102 prints two isolated `.in-shared`
// copies (one per its two independent users). Oracle: dart-sass 1.102.
func TestImportCloneThroughDeepUse(t *testing.T) {
	imp := cssMapImporter(map[string]string{
		"_used-by-input.scss":    "@use \"shared\";\n.in-used-by-input {@extend .in-shared}\n",
		"_imported.scss":         "@use \"used-by-imported\";\n",
		"_used-by-imported.scss": "@use \"shared\";\n.in-used-by-imported {@extend .in-shared}\n",
		"_shared.scss":           ".in-shared {a: b}\n",
	})
	css, e := runProgram(t, "@use \"used-by-input\";\n@import \"imported\";\n", imp)

	want := ".in-shared, .in-used-by-input {\n  a: b;\n}\n\n.in-shared, .in-used-by-imported {\n  a: b;\n}\n"
	if css != want {
		t.Fatalf("deep-@use re-emit not composed:\ngot:\n%q\nwant:\n%q", css, want)
	}
	// The deep @use produces a fresh clone (of used-by-imported's CSS) and a
	// re-emit clone (of the already-loaded shared); both compose independently and
	// the combine tree emits the visible copy once.
	if n := len(*e.importClones); n == 0 {
		t.Fatal("no importClone recorded for the deep-@use duplicate")
	}
}

// TestFreshImportCloneNotSeparatedWhenNested covers the guard that a module loaded
// fresh inside an @import that is itself nested in a style rule (fr.hasParent) is
// NOT clone-separated: emitModuleCSS re-nests it under the enclosing selector, so
// it stays on the shared-node path with no importClone recorded. Oracle: dart-sass
// 1.102.
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
	if n := len(*e.importClones); n != 0 {
		t.Fatalf("a nested fresh-during-import module was clone-separated (%d importClones); it must not be", n)
	}
}

// TestReEmitOutsideImportIsNoOp covers reEmitImportedCSS's guard arms: a plain
// @use of an already-loaded module outside any @import (importSubtree nil) emits
// nothing extra, and an empty-CSS module short-circuits. A diamond @use of the
// same module must print its CSS exactly once.
func TestReEmitOutsideImportIsNoOp(t *testing.T) {
	imp := cssMapImporter(map[string]string{
		"_a.scss":      "@use \"shared\";\n",
		"_b.scss":      "@use \"shared\";\n",
		"_shared.scss": ".s {x: y}\n",
	})
	css, _ := runProgram(t, "@use \"a\";\n@use \"b\";\n", imp)
	want := ".s {\n  x: y;\n}\n"
	if css != want {
		t.Fatalf("diamond @use printed shared more than once:\ngot:\n%q\nwant:\n%q", css, want)
	}
}

// TestRegisterCloneBoxesNew covers registerCloneBoxes: a style rule with a parsed
// selector (box + registration + marked original + returned), a raw plain-CSS rule
// with no parsed selector (no box, not returned), an at-rule whose nested style
// rule is reached by recursion, an invisible-selector rule (registered but not
// marked original), and inert declaration/comment leaves that are skipped.
func TestRegisterCloneBoxesNew(t *testing.T) {
	store := newExtensionStore(extendNormal)
	rule := &cssStyleRule{selector: parseSelectorList(".x")}
	raw := &cssStyleRule{raw: true} // selector.list is nil
	nested := &cssStyleRule{selector: parseSelectorList(".y")}
	atr := &cssAtRule{name: "media", nodes: []cssNode{nested}}
	invisible := &cssStyleRule{selector: selectorList{list: mustParseSelectorList("%p")}}
	rules := registerCloneBoxes([]cssNode{rule, raw, atr, invisible, &cssDeclaration{}, &cssComment{}}, store)

	if len(rules) != 3 {
		t.Fatalf("returned rules = %d, want 3 (.x, .y, placeholder)", len(rules))
	}
	if rule.box == nil || rule.box.value != rule.selector.list {
		t.Fatal("style rule got no box wrapping its selector")
	}
	if raw.box != nil {
		t.Fatal("raw rule must not get a box")
	}
	if nested.box == nil {
		t.Fatal("nested style rule under at-rule not registered")
	}
	if _, ok := store.selectors[simpleKey(simpleOf(t, ".x"))]; !ok {
		t.Fatal(".x not registered in store")
	}
	if _, ok := store.selectors[simpleKey(simpleOf(t, ".y"))]; !ok {
		t.Fatal(".y not registered in store")
	}
	// A visible selector is marked as an original; an invisible one is not.
	if !store.originals[complexKey(rule.selector.list.components[0])] {
		t.Fatal(".x should be marked as an original")
	}
	if store.originals[complexKey(invisible.selector.list.components[0])] {
		t.Fatal("invisible placeholder must not be marked as an original")
	}
}

// TestWriteBackCloneSelectors covers writeBackCloneSelectors across every arm: a
// rule with no box (skipped), a raw rule whose box changed off its original (flips
// off raw + writes back), a raw rule whose box is unchanged (stays raw), and a
// normal boxed rule (writes back).
func TestWriteBackCloneSelectors(t *testing.T) {
	orig := mustParseSelectorList(".r")
	changed := mustParseSelectorList(".r, .x")

	noBox := &cssStyleRule{selector: selectorList{list: orig}}
	rawChanged := &cssStyleRule{raw: true, original: selectorList{list: orig}, box: &box{value: changed}}
	rawSame := &cssStyleRule{raw: true, original: selectorList{list: orig}, box: &box{value: orig}}
	boxed := &cssStyleRule{selector: selectorList{list: orig}, box: &box{value: changed}}

	writeBackCloneSelectors([]*cssStyleRule{noBox, rawChanged, rawSame, boxed})

	if noBox.selector.list != orig {
		t.Fatal("box-less rule must be left untouched")
	}
	if rawChanged.raw {
		t.Fatal("a raw rule whose selector changed must flip off raw")
	}
	if rawChanged.selector.list != changed {
		t.Fatal("raw-changed rule did not write back its composed selector")
	}
	if !rawSame.raw {
		t.Fatal("a raw rule whose selector is unchanged must stay raw")
	}
	if boxed.selector.list != changed {
		t.Fatal("boxed rule did not write back its composed selector")
	}
}

// TestComposeImportClonesSubtreeNil covers composeImportClones' skip of a clone
// with no subtree collector (defensive; ordinary clones always carry one).
func TestComposeImportClonesSubtreeNil(t *testing.T) {
	e := newEvaluator(nil)
	rule := &cssStyleRule{selector: selectorList{list: mustParseSelectorList(".r")}}
	rule.box = &box{value: rule.selector.list}
	*e.importClones = append(*e.importClones, &importClone{
		store: newExtensionStore(extendNormal),
		rules: []*cssStyleRule{rule},
	})
	// No panic and the rule is untouched (subtree nil -> continue).
	e.composeImportClones(map[*moduleScope]*extensionStore{}, nil)
	if rule.selector.String() != ".r" {
		t.Fatalf("subtree-less clone was composed: %q", rule.selector.String())
	}
}

// TestComposeImportClonesEmptyStores covers the arm where a clone's subtree has no
// pristine stores to merge (every subtree scope missing from the map), so
// addExtensions is skipped but the selectors are still written back.
func TestComposeImportClonesEmptyStores(t *testing.T) {
	e := newEvaluator(nil)
	sc := &moduleScope{ev: newEvaluator(nil)}
	rule := &cssStyleRule{selector: selectorList{list: mustParseSelectorList(".r")}}
	rule.box = &box{value: rule.selector.list}
	*e.importClones = append(*e.importClones, &importClone{
		store:   newExtensionStore(extendNormal),
		rules:   []*cssStyleRule{rule},
		subtree: &importSubtreeCtx{scopes: []*moduleScope{sc}, seen: map[*moduleScope]bool{sc: true}},
	})
	// pristine map is empty, so no stores -> addExtensions skipped, writeback runs.
	e.composeImportClones(map[*moduleScope]*extensionStore{}, []*moduleScope{sc})
	if rule.selector.String() != ".r" {
		t.Fatalf("empty-subtree clone changed: %q", rule.selector.String())
	}
}

// TestImportSubtreeCtxAdd covers add's three arms: a nil scope (ignored), a fresh
// scope (recorded in order), and a repeat (deduplicated).
func TestImportSubtreeCtxAdd(t *testing.T) {
	c := &importSubtreeCtx{seen: map[*moduleScope]bool{}}
	a := &moduleScope{}
	b := &moduleScope{}
	c.add(nil)
	c.add(a)
	c.add(b)
	c.add(a) // dedup
	if len(c.scopes) != 2 || c.scopes[0] != a || c.scopes[1] != b {
		t.Fatalf("subtree scopes = %v, want [a b] in order", c.scopes)
	}
}

// TestSortScopesByRank covers ordering subtree scopes into the finalize order.
func TestSortScopesByRank(t *testing.T) {
	a := &moduleScope{}
	b := &moduleScope{}
	d := &moduleScope{}
	rank := map[*moduleScope]int{a: 2, b: 0, d: 1}
	scopes := []*moduleScope{a, b, d}
	sortScopesByRank(scopes, rank)
	if scopes[0] != b || scopes[1] != d || scopes[2] != a {
		t.Fatalf("sorted = %v, want [b d a] by ascending rank", scopes)
	}
}
