// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import "testing"

// simpleOf returns the single simple selector of a one-compound selector text,
// e.g. ".a" -> the class simple selector. targetsSimpleErr on anything else.
func simpleOf(t *testing.T, text string) simpleSel {
	t.Helper()
	s := mustParseSelectorList(text).components[0].singleCompound().singleSimple()
	if s == nil {
		t.Fatalf("selector %q is not a single simple", text)
	}
	return s
}

// TestExtensionStoreCloneWhitebox exercises every branch of clone(): shared box
// identity (a box reachable through both `selectors` and `mediaContexts` clones
// to one instance), shared extension identity (an extension reachable through
// both `extensions` and `extensionsByExtender`), the nil-entry guards, the
// non-nil extender/mediaContext copy, and the scalar-map copies. It then proves
// the copy is fully independent of the original.
func TestExtensionStoreCloneWhitebox(t *testing.T) {
	simpleA := simpleOf(t, ".a")
	complexB := mustParseSelectorList(".b").components[0]

	e := &extension{target: simpleA, optional: true, mediaContext: []string{"screen"}}
	e.extender = &extender{selector: complexB, specificity: complexB.specificity(), isOriginal: true, ext: e}

	s := newExtensionStore(extendNormal)
	b := &box{value: mustParseSelectorList(".a")}
	// A box shared between a selectorSet and mediaContexts, plus a nil box entry
	// to drive the defensive nil guard in cloneBox.
	s.selectors[simpleKey(simpleA)] = &selectorSet{simple: simpleA, boxes: []*box{b, nil}}
	s.mediaContexts[b] = []string{"screen"}
	// The same extension reachable through extensions and extensionsByExtender,
	// plus a nil entry to drive cloneExtension's nil guard.
	te := &targetExtensions{target: simpleA, sources: map[string]*extension{}}
	te.put(complexKey(complexB), e)
	s.extensions[simpleKey(simpleA)] = te
	s.extensionsByExtender[complexKey(complexB)] = []*extension{e, nil}
	s.sourceSpecificity[complexKey(complexB)] = 42
	s.originals[complexKey(complexB)] = true

	c := s.clone()

	if c == s {
		t.Fatal("clone returned the same store")
	}
	if c.mode != s.mode {
		t.Fatalf("mode not copied: %v", c.mode)
	}

	// Box identity within the clone: the box in selectors is a NEW pointer, and
	// the mediaContexts key is that same new box (memoised), not the original.
	set := c.selectors[simpleKey(simpleA)]
	if len(set.boxes) != 2 {
		t.Fatalf("boxes len = %d, want 2", len(set.boxes))
	}
	nb := set.boxes[0]
	if nb == b {
		t.Fatal("box not cloned (shared with original)")
	}
	if set.boxes[1] != nil {
		t.Fatal("nil box entry not preserved")
	}
	if _, ok := c.mediaContexts[nb]; !ok {
		t.Fatal("mediaContexts key is not the cloned box (identity not preserved)")
	}
	if _, ok := c.mediaContexts[b]; ok {
		t.Fatal("mediaContexts still keyed by the original box")
	}

	// Extension identity within the clone: the extension in extensions and the
	// one in extensionsByExtender are the same NEW instance, its extender back-
	// references it, and its slices are independent copies.
	ce := c.extensions[simpleKey(simpleA)].sources[complexKey(complexB)]
	if ce == e {
		t.Fatal("extension not cloned")
	}
	if c.extensionsByExtender[complexKey(complexB)][0] != ce {
		t.Fatal("extension identity not shared across maps in the clone")
	}
	if c.extensionsByExtender[complexKey(complexB)][1] != nil {
		t.Fatal("nil extension entry not preserved")
	}
	if ce.extender == nil || ce.extender.ext != ce {
		t.Fatal("cloned extender back-reference wrong")
	}
	if ce.extender.specificity != e.extender.specificity || !ce.extender.isOriginal {
		t.Fatal("extender scalar fields not copied")
	}
	if len(ce.mediaContext) != 1 || ce.mediaContext[0] != "screen" {
		t.Fatalf("mediaContext not copied: %v", ce.mediaContext)
	}
	if c.sourceSpecificity[complexKey(complexB)] != 42 || !c.originals[complexKey(complexB)] {
		t.Fatal("scalar maps not copied")
	}

	// Independence: mutating the clone must not touch the original and vice versa.
	ce.mediaContext[0] = "print"
	if e.mediaContext[0] != "screen" {
		t.Fatal("mediaContext slice shared with original")
	}
	nb.value = mustParseSelectorList(".changed")
	if (selectorList{list: b.value}).String() != ".a" {
		t.Fatal("box value shared with original")
	}
	c.originals["new"] = true
	if s.originals["new"] {
		t.Fatal("originals map shared with original")
	}
	s.sourceSpecificity["other"] = 7
	if _, ok := c.sourceSpecificity["other"]; ok {
		t.Fatal("sourceSpecificity map shared with original")
	}
}

// TestExtensionStoreCloneExtendsIdentically checks that a clone reproduces the
// original's extend behaviour: given a store carrying `.b { @extend .a }`, both
// the original and its clone extend a freshly registered `.a` to `.a, .b`
// (dart-sass's result for that program), and extending only the clone leaves the
// original's already-registered selectors untouched.
func TestExtensionStoreCloneExtendsIdentically(t *testing.T) {
	simpleA := simpleOf(t, ".a")

	s := newExtensionStore(extendNormal)
	s.addExtension(mustParseSelectorList(".b"), simpleA, false, nil)
	c := s.clone()

	got := selectorList{list: s.addSelector(mustParseSelectorList(".a"), nil).value}.String()
	if got != ".a, .b" {
		t.Fatalf("original addSelector = %q, want %q", got, ".a, .b")
	}
	gotC := selectorList{list: c.addSelector(mustParseSelectorList(".a"), nil).value}.String()
	if gotC != ".a, .b" {
		t.Fatalf("clone addSelector = %q, want %q", gotC, ".a, .b")
	}

	// Extending only the clone (with .c) must not change the original's store.
	before := len(s.extensionsByExtender)
	c.addExtension(mustParseSelectorList(".c"), simpleA, false, nil)
	if len(s.extensionsByExtender) != before {
		t.Fatal("extending the clone mutated the original's extensionsByExtender")
	}
}

// TestCloneStoreFor covers the three arms of cloneStoreFor: a module with no
// extend scope (fresh empty store), a module whose own store must be built and
// cached, and a subsequent call reusing the cached own store.
func TestCloneStoreFor(t *testing.T) {
	e := newEvaluator(nil)

	if st := e.cloneStoreFor(&module{}); st == nil {
		t.Fatal("nil-scope module got no store")
	}

	ms := &moduleScope{ev: newEvaluator(nil)}
	m := &module{scope: ms}
	st := e.cloneStoreFor(m)
	if st == nil || ms.ownStore == nil {
		t.Fatal("own store not built and cached")
	}
	cached := ms.ownStore
	st2 := e.cloneStoreFor(m)
	if st2 == nil || ms.ownStore != cached {
		t.Fatal("second call rebuilt the own store instead of reusing the cache")
	}
	if st2 == st {
		t.Fatal("cloneStoreFor returned the same store instance twice")
	}
}

// TestRegisterCloneBoxes covers registerCloneBoxes across a style rule with a
// parsed selector (gets a box + registration), a raw plain-CSS rule with no
// parsed selector (no box), an at-rule whose nested style rule is registered,
// and inert leaf nodes (declaration/comment) that are skipped.
func TestRegisterCloneBoxes(t *testing.T) {
	store := newExtensionStore(extendNormal)
	rule := &cssStyleRule{selector: parseSelectorList(".x")}
	raw := &cssStyleRule{raw: true} // selector.list is nil
	nested := &cssStyleRule{selector: parseSelectorList(".y")}
	atr := &cssAtRule{name: "media", nodes: []cssNode{nested}}
	registerCloneBoxes([]cssNode{rule, raw, atr, &cssDeclaration{}, &cssComment{}}, store)

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
}
