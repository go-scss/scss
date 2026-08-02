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

// TestUseAndImportIntoDiamondExtend covers the sass-spec case of the same name:
// a module @used AND @imported that itself @uses two sibling modules which both
// extend a shared module. The @import (and a second @import of a file that @uses
// the module) each duplicate the whole subtree; every copy must be sibling-isolated
// — the shared rule is extended by both siblings' extendees, but neither sibling's
// cross-extender (`@extend the-other-sibling !optional`) leaks across the diamond.
// dart-sass 1.102 prints three identical copies. This is the per-module clone store
// case: an aggregate clone store would leak `right-extender` into the shared rule.
func TestUseAndImportIntoDiamondExtend(t *testing.T) {
	imp := cssMapImporter(map[string]string{
		"_downstream.scss": "@use \"left\";\n@use \"right\";\n",
		"_left.scss":       "@use \"shared\";\nleft-extendee {@extend in-shared}\nleft-extender {@extend right-extendee !optional}\n",
		"_right.scss":      "@use \"shared\";\nright-extendee {@extend in-shared}\nright-extender {@extend left-extendee !optional}\n",
		"_shared.scss":     "in-shared {x: y}\n",
		"_imported.scss":   "@use \"downstream\";\n",
	})
	css, _ := runProgram(t, "@use \"downstream\";\n@import \"downstream\";\n@import \"imported\";\n", imp)

	one := "in-shared, right-extendee, left-extendee {\n  x: y;\n}\n"
	want := one + "\n" + one + "\n" + one
	if css != want {
		t.Fatalf("diamond @use+@import not sibling-isolated per clone:\ngot:\n%q\nwant:\n%q", css, want)
	}
}

// TestUseIntoUseAndUseIntoImportIntoUse covers a clone whose composition order is
// driven by the IMPORT-site edge, not the canonical module rank: `importer`
// @imports `imported`, which @uses+extends `shared`; the import copy of `shared`
// must be extended by `in-imported` even though, canonically, `importer` is not
// downstream of `shared`. Composing in canonical-rank order would visit `shared`
// before `importer` and miss the extend; the subtree-local downstream-first order
// fixes it. dart-sass 1.102 prints the import copy as `shared, in-imported` and the
// canonical copy (via `used`) as `shared, in-used`.
func TestUseIntoUseAndUseIntoImportIntoUse(t *testing.T) {
	imp := cssMapImporter(map[string]string{
		"_importer.scss": "@import \"imported\";\n",
		"_imported.scss": "@use \"shared\";\nin-imported {@extend shared}\n",
		"_used.scss":     "@use \"shared\";\nin-used {@extend shared}\n",
		"_shared.scss":   "shared {x: y}\n",
	})
	css, _ := runProgram(t, "@use \"importer\";\n@use \"used\";\n", imp)

	want := "shared, in-imported {\n  x: y;\n}\n\nshared, in-used {\n  x: y;\n}\n"
	if css != want {
		t.Fatalf("import-edge composition order lost the clone's own extend:\ngot:\n%q\nwant:\n%q", css, want)
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

// TestPairCloneRules covers pairCloneRules: a style rule with a parsed selector is
// paired (origin+clone returned), a raw plain-CSS rule with no parsed selector is
// skipped, an at-rule's nested style rule is reached by recursion, and inert
// declaration/comment leaves are skipped. The pairing is by matching index into
// the structurally identical clone tree.
func TestPairCloneRules(t *testing.T) {
	rule := &cssStyleRule{selector: parseSelectorList(".x")}
	raw := &cssStyleRule{raw: true} // selector.list is nil
	nested := &cssStyleRule{selector: parseSelectorList(".y")}
	atr := &cssAtRule{name: "media", nodes: []cssNode{nested}}
	orig := []cssNode{rule, raw, atr, &cssDeclaration{}, &cssComment{}}
	clone := cloneCSSNodes(orig)

	origins, clones := pairCloneRules(orig, clone)
	if len(origins) != 2 || len(clones) != 2 {
		t.Fatalf("paired %d rules, want 2 (.x and nested .y)", len(origins))
	}
	if origins[0] != rule {
		t.Fatal("first origin should be the .x rule")
	}
	// The clone must be a distinct object carrying the same selector list.
	if clones[0] == rule || clones[0].selector.list != rule.selector.list {
		t.Fatal("clone of .x is not a distinct rule over the same selector")
	}
	clonedAt := clone[2].(*cssAtRule)
	if origins[1] != nested || clones[1] != clonedAt.nodes[0] {
		t.Fatal("nested style rule under at-rule not paired via recursion")
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

// TestComposeImportClonesSubtreeNil covers composeImportClones' handling of a clone
// with no subtree collector (defensive; ordinary clones always carry one): it is
// written back verbatim, never composed.
func TestComposeImportClonesSubtreeNil(t *testing.T) {
	e := newEvaluator(nil)
	rule := &cssStyleRule{selector: selectorList{list: mustParseSelectorList(".r")}}
	rule.box = &box{value: rule.selector.list}
	*e.importClones = append(*e.importClones, &importClone{
		rules:   []*cssStyleRule{rule},
		origins: []*cssStyleRule{rule},
	})
	// No panic and the rule is untouched (subtree nil -> writeback only).
	e.composeImportClones(map[*moduleScope]*extensionStore{}, nil)
	if rule.selector.String() != ".r" {
		t.Fatalf("subtree-less clone was composed: %q", rule.selector.String())
	}
}

// TestComposeImportClonesOwnExtendOnly covers a clone whose subtree module has an
// own extend but no downstream edges: the module's own pristine extends are seeded
// onto its clone rules, and the extended selector is written back.
func TestComposeImportClonesOwnExtendOnly(t *testing.T) {
	e := newEvaluator(nil)
	sub := newEvaluator(nil)
	sc := &moduleScope{ev: sub}
	// The module owns two rules: `.r {a: b}` and `.e {@extend .r}`.
	target := &cssStyleRule{selector: selectorList{list: mustParseSelectorList(".r")}}
	extender := &cssStyleRule{selector: selectorList{list: mustParseSelectorList(".e")}}
	sub.extendEvents = []extendEvent{{rule: target}, {rule: extender}}

	// Pristine own-store for the module: `.e` extends `.r`.
	own := newExtensionStore(extendNormal)
	own.addExtension(mustParseSelectorList(".e"), simpleOf(t, ".r"), false, nil)

	// A single duplicate carrying clones of both rules.
	cloneTarget := &cssStyleRule{selector: selectorList{list: mustParseSelectorList(".r")}}
	cloneExtender := &cssStyleRule{selector: selectorList{list: mustParseSelectorList(".e")}}
	st := &importSubtreeCtx{seen: map[*moduleScope]bool{}}
	st.add(sc)
	*e.importClones = append(*e.importClones, &importClone{
		rules:   []*cssStyleRule{cloneTarget, cloneExtender},
		origins: []*cssStyleRule{target, extender},
		source:  sc,
		subtree: st,
	})
	e.composeImportClones(map[*moduleScope]*extensionStore{sc: own}, []*moduleScope{sc})
	if got := cloneTarget.selector.String(); got != ".r, .e" {
		t.Fatalf("own extend not composed onto clone: %q, want %q", got, ".r, .e")
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
