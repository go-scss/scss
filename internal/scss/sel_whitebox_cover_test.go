// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import (
	"strings"
	"testing"
)

// wbCpx parses a selector string and returns its first complex selector.
func wbCpx(s string) *selComplex { return parseSelectorList(s).list.components[0] }

// wbComps returns the component list of the first complex selector.
func wbComps(s string) []complexComponent { return wbCpx(s).components }

// wbCompound returns the compound of the first component (a single-compound
// selector such as "a" or ":host").
func wbCompound(s string) *compoundSel { return wbCpx(s).components[0].selector }

// wbSimple returns the first simple selector of a single-compound selector.
func wbSimple(s string) simpleSel { return wbCompound(s).components[0] }

// TestSelectorWhiteboxHelpers exercises the low-level selector-algorithm helpers
// directly, hitting the branches that source-level cases cannot deterministically
// reach.
func TestSelectorWhiteboxHelpers(t *testing.T) {
	child := combChild
	fsib := combFollowingSibling

	// compatibleWithPreviousCombinator arms.
	if compatibleWithPreviousCombinator(&child, wbComps("a")) {
		t.Error("compatible: previous=child with parents should be false")
	}
	sibParents := []complexComponent{{selector: wbCompound("b"), combinators: []combinator{fsib}}}
	if !compatibleWithPreviousCombinator(&fsib, sibParents) {
		t.Error("compatible: following-sibling parents should be true")
	}
	if compatibleWithPreviousCombinator(&fsib, wbComps("a")) {
		t.Error("compatible: non-sibling parent should be false")
	}

	// mustUnify + isUniqueSimple (id and pseudo-element unique selectors).
	if !mustUnify(wbComps("#x a"), wbComps("b #x")) {
		t.Error("mustUnify shared id should be true")
	}
	if !mustUnify(wbComps("a::before"), wbComps("a::before")) {
		t.Error("mustUnify shared pseudo-element should be true")
	}

	// complexIsSuperselector: complex2 ends with a combinator -> false.
	c2trailing := []complexComponent{{selector: wbCompound("b"), combinators: []combinator{child}}}
	if complexIsSuperselector(wbComps("a"), c2trailing) {
		t.Error("superselector with trailing combinator on complex2 should be false")
	}
	// complexIsSuperselector: a parent with more than one combinator -> false.
	c2multi := []complexComponent{
		{selector: wbCompound("a"), combinators: []combinator{child, child}},
		{selector: wbCompound("b")},
	}
	if complexIsSuperselector(wbComps("b"), c2multi) {
		t.Error("superselector with multi-combinator parent should be false")
	}

	// mergeTrailingCombinators: both following-sibling, one side super.
	m1 := []complexComponent{{selector: wbCompound("a"), combinators: []combinator{fsib}}}
	m2 := []complexComponent{{selector: wbCompound("*"), combinators: []combinator{fsib}}}
	if _, ok := mergeTrailingCombinators(&m1, &m2, nil); !ok {
		t.Error("mergeTrailingCombinators sibling-super should succeed")
	}
	// mergeTrailingCombinators: same combinator, non-unifiable compounds -> false.
	m3 := []complexComponent{{selector: wbCompound("a"), combinators: []combinator{child}}}
	m4 := []complexComponent{{selector: wbCompound("b"), combinators: []combinator{child}}}
	if _, ok := mergeTrailingCombinators(&m3, &m4, nil); ok {
		t.Error("mergeTrailingCombinators non-unifiable child should fail")
	}

	// universalSel.unify with a lone :host -> (nil,false).
	u := &universalSel{}
	if _, ok := u.unify([]simpleSel{wbSimple(":host")}); ok {
		t.Error("universal.unify(:host) should fail")
	}
	// pseudoSel.unify: non-host pseudo whose partner is :host.
	if _, ok := wbSimple(":hover").unify([]simpleSel{wbSimple(":host")}); ok {
		t.Error("pseudo.unify(:host) should fail")
	}
	// defaultUnify: a class whose partner is :host.
	if _, ok := wbSimple(".a").unify([]simpleSel{wbSimple(":host")}); ok {
		t.Error("class.unify(:host) should fail")
	}

	// pseudoSel.specificity default arm and equal argument-mismatch arm.
	if wbSimple(":hover").specificity() != 1000 {
		t.Error("pseudo :hover specificity")
	}
	if wbSimple(":nth-child(2)").equal(wbSimple(":nth-child(3)")) {
		t.Error(":nth-child(2) should not equal :nth-child(3)")
	}

	// selComplex.equal with differing leading combinators (also combSliceEqual).
	if wbCpx("a").equal(&selComplex{leadingCombinators: []combinator{child}, components: wbComps("a")}) {
		t.Error("complexes with differing leading combinators should not be equal")
	}

	// writeCombinators with more than one combinator (the i>0 spacing arm).
	var sb strings.Builder
	writeCombinators(&sb, []combinator{child, child}, false)
	if sb.String() == "" {
		t.Error("writeCombinators produced nothing")
	}

	// bogus() on an empty complex with a leading combinator.
	if !(&selComplex{leadingCombinators: []combinator{child}}).bogus(true) {
		t.Error("empty complex with leading combinator is bogus")
	}
	// isUseless() with more than one leading combinator.
	if !(&selComplex{leadingCombinators: []combinator{child, child}}).isUseless() {
		t.Error("complex with two leading combinators is useless")
	}

	// flattenVertically with multiple multi-element queues.
	rows := [][]*selComplex{{wbCpx("a"), wbCpx("b")}, {wbCpx("c"), wbCpx("d")}}
	if got := flattenVertically(rows); len(got) != 4 {
		t.Errorf("flattenVertically: want 4 got %d", len(got))
	}
}

// TestSelectorWhiteboxResidual covers the last superselector/specificity arms.
func TestSelectorWhiteboxResidual(t *testing.T) {
	// pseudoSel.specificity default arm: a pseudo-class with a selector argument
	// whose name is outside the specially-scored set.
	if wbSimple(":host(.x)").specificity() != 1000 {
		t.Error(":host(.x) specificity should be 1000")
	}

	// complexIsSuperselector: an intervening `>`-combined parent is incompatible
	// with a preceding following-sibling combinator.
	c1 := []complexComponent{
		{selector: wbCompound("a"), combinators: []combinator{combFollowingSibling}},
		{selector: wbCompound("c"), combinators: []combinator{combFollowingSibling}},
		{selector: wbCompound("d")},
	}
	c2 := []complexComponent{
		{selector: wbCompound("a"), combinators: []combinator{combFollowingSibling}},
		{selector: wbCompound("x"), combinators: []combinator{combChild}},
		{selector: wbCompound("c"), combinators: []combinator{combFollowingSibling}},
		{selector: wbCompound("d")},
	}
	if complexIsSuperselector(c1, c2) {
		t.Error("a ~ c ~ d should not be a superselector of a ~ x > c ~ d")
	}

	// parseSelectorList: a trailing comma at end-of-input breaks cleanly.
	if got := parseSelectorList("a,"); len(got.list.components) != 1 {
		t.Errorf("trailing-comma parse: want 1 complex, got %d", len(got.list.components))
	}

	// selectorPseudoIsSuperselector negative arms for ::slotted and :current.
	slot1 := wbSimple("::slotted(a)").(*pseudoSel)
	if selectorPseudoIsSuperselector(slot1, wbCompound("::slotted(b)"), nil) {
		t.Error("::slotted(a) should not be a superselector of ::slotted(b)")
	}
	cur1 := wbSimple(":current(a)").(*pseudoSel)
	if selectorPseudoIsSuperselector(cur1, wbCompound(":current(b)"), nil) {
		t.Error(":current(a) should not be a superselector of :current(b)")
	}
}

// TestWeaveAndUnifyWhitebox drives weaveParents and unifyComplex through the
// rootish, unify and leading/trailing-combinator arms.
func TestWeaveAndUnifyWhitebox(t *testing.T) {
	// rootish weaving: both, left-only and right-only.
	_ = weaveParents(wbCpx(":root .a"), wbCpx(":root .b"))
	_ = weaveParents(wbCpx(":root .a"), wbCpx(".x .b"))
	_ = weaveParents(wbCpx(".x .a"), wbCpx(":root .b"))
	// two rootish parents that cannot unify (:root vs :host).
	_ = weaveParents(wbCpx(":root .a"), wbCpx(":host .b"))
	// lcs unification of parent groups sharing a unique id but differing.
	_ = weaveParents(wbCpx("#x.y .a"), wbCpx("#x.z .b"))
	_ = weaveParents(wbCpx("#x .y .a"), wbCpx("#x .z .b"))
	_ = weaveParents(wbCpx(".m .a"), wbCpx(".m .b"))
	// lcs where the unify of shared-id parents fails (conflicting type selectors).
	_ = weaveParents(wbCpx("a#x .c"), wbCpx("b#x .d"))
	// lcs parent-superselector arms in both directions.
	_ = weaveParents(wbCpx(".y.z .a"), wbCpx(".y .b"))
	_ = weaveParents(wbCpx(".y .a"), wbCpx(".y.z .b"))

	// weaveParents where the base's trailing combinators cannot merge -> nil.
	prefix := &selComplex{components: []complexComponent{
		{selector: wbCompound("a"), combinators: []combinator{combChild}},
	}}
	base := &selComplex{components: []complexComponent{
		{selector: wbCompound("b"), combinators: []combinator{combChild}},
		{selector: wbCompound("z")},
	}}
	if got := weaveParents(prefix, base); got != nil {
		t.Errorf("weaveParents with conflicting trailing combinators: want nil got %v", got)
	}

	// unifyComplex over complexes carrying both leading and trailing combinators.
	mk := func() *selComplex {
		return &selComplex{
			leadingCombinators: []combinator{combChild},
			components: []complexComponent{
				{selector: wbCompound("a"), combinators: []combinator{combFollowingSibling}},
			},
		}
	}
	_, _ = unifyComplex([]*selComplex{mk(), mk()})
}

// TestExtendComplexLeadingCombinators drives the @extend engine's handling of a
// target selector that carries leading combinators, reaching extendComplex's
// leading-combinator arms via the static selector.extend entry point.
func TestExtendComplexLeadingCombinators(t *testing.T) {
	// A selector list whose complex has a single leading combinator.
	oneLeading := &selList{components: []*selComplex{{
		leadingCombinators: []combinator{combChild},
		components:         wbComps(".a"),
	}}}
	source := parseSelectorList(".c").list
	targets := parseSelectorList(".a").list
	_ = extendOrReplace(oneLeading, source, targets, extendNormal)

	// A complex with two leading combinators is dropped (extendComplex -> nil).
	twoLeading := &selList{components: []*selComplex{{
		leadingCombinators: []combinator{combChild, combChild},
		components:         wbComps(".a"),
	}}}
	_ = extendOrReplace(twoLeading, source, targets, extendNormal)

	// addExtension skips a useless extender complex (two leading combinators).
	store := newExtensionStore(extendNormal)
	uselessExtender := &selList{components: []*selComplex{{
		leadingCombinators: []combinator{combChild, combChild},
		components:         wbComps(".b"),
	}}}
	store.addExtension(uselessExtender, wbSimple(".a"), false, nil)
}

// TestExtendStoreResidualArms constructs extension-store state directly to reach
// the defensive transitive-extend arms that normal document-order @extend flow
// filters out earlier: a useless extender in unifyExtenders, a useless option in
// extendCompound, a leading-combinator extender in extendExistingExtensions and a
// no-op box in extendExistingSelectors.
func TestExtendStoreResidualArms(t *testing.T) {
	target := wbSimple(".a")
	tk := simpleKey(target)

	// unifyExtenders returns nil when a non-original extender is useless.
	store := newExtensionStore(extendNormal)
	useless := &extender{
		selector:   &selComplex{leadingCombinators: []combinator{combChild, combChild}, components: wbComps(".x")},
		isOriginal: false,
	}
	if got := store.unifyExtenders([]*extender{useless}, nil); got != nil {
		t.Errorf("unifyExtenders(useless): want nil, got %v", got)
	}

	// extendCompound drops options that become useless once the component's
	// trailing combinator is appended.
	store2 := newExtensionStore(extendNormal)
	store2.sourceSpecificity[tk] = 0
	te := &targetExtensions{target: target, sources: map[string]*extension{}}
	ext := &extension{target: target}
	ext.extender = &extender{selector: wbCpx(".b"), ext: ext}
	te.put(complexKey(wbCpx(".b")), ext)
	exts := map[string]*targetExtensions{tk: te}
	comp := complexComponent{selector: wbCompound(".a"), combinators: []combinator{combChild, combChild}}
	_ = store2.extendCompound(comp, exts, nil, false)

	// extendExistingExtensions skips an existing extender that cannot be extended
	// (its selector carries more than one leading combinator).
	store3 := newExtensionStore(extendNormal)
	store3.extensions[tk] = &targetExtensions{target: target, sources: map[string]*extension{}}
	badExt := &extension{target: target}
	badExt.extender = &extender{selector: &selComplex{
		leadingCombinators: []combinator{combChild, combChild},
		components:         wbComps(".b"),
	}}
	newExts := map[string]*targetExtensions{
		simpleKey(wbSimple(".c")): {target: wbSimple(".c"), sources: map[string]*extension{}},
	}
	_ = store3.extendExistingExtensions([]*extension{badExt}, newExts)

	// extendExistingSelectors leaves a box untouched when it contains none of the
	// extended targets (extendList returns the identical list).
	store4 := newExtensionStore(extendNormal)
	b := &box{value: parseSelectorList(".unrelated").list}
	noopExts := map[string]*targetExtensions{
		tk: {target: target, sources: map[string]*extension{}},
	}
	store4.extendExistingSelectors([]*box{b}, noopExts)
}
