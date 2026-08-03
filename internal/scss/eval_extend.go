// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

// This file ports Dart Sass's lib/src/extend/extension_store.dart: the @extend
// engine that tracks selectors and extensions and applies the latter to the
// former, including transitive extension, unification and trimming. The
// selector.extend()/selector.replace() module functions reuse the same core via
// extendOrReplace.

type extendMode int

const (
	extendNormal extendMode = iota
	extendReplace
	extendAllTargets
)

// extender wraps a complex selector participating in an extension.
type extender struct {
	selector    *selComplex
	specificity int
	isOriginal  bool
	ext         *extension // back-reference for media-context checks
}

func (e *extender) assertCompatibleMediaContext(mc []string) {
	if e.ext == nil {
		return
	}
	expected := e.ext.mediaContext
	if expected == nil {
		return
	}
	if mc != nil && strSliceEqual(expected, mc) {
		return
	}
	panic(selErr("You may not @extend selectors across media queries."))
}

type extension struct {
	extender     *extender
	target       simpleSel
	mediaContext []string
	optional     bool
}

// box holds a selector list that extension may update in place.
type box struct{ value *selList }

// selectorSet tracks a simple selector and the boxes that contain it.
type selectorSet struct {
	simple simpleSel
	boxes  []*box
}

// targetExtensions groups all extensions targeting a single simple selector.
type targetExtensions struct {
	target  simpleSel
	sources map[string]*extension // key: complexKey(extender.selector)
	order   []string              // deterministic iteration order
}

func (t *targetExtensions) put(key string, ext *extension) {
	if _, ok := t.sources[key]; !ok {
		t.order = append(t.order, key)
	}
	t.sources[key] = ext
}

func (t *targetExtensions) values() []*extension {
	out := make([]*extension, 0, len(t.order))
	for _, k := range t.order {
		out = append(out, t.sources[k])
	}
	return out
}

// extensionStore is the Go port of Dart Sass's ExtensionStore.
type extensionStore struct {
	selectors            map[string]*selectorSet      // key: simpleKey
	extensions           map[string]*targetExtensions // key: simpleKey(target)
	extensionsByExtender map[string][]*extension      // key: simpleKey
	mediaContexts        map[*box][]string
	sourceSpecificity    map[string]int // key: simpleKey
	originals            map[string]bool
	mode                 extendMode
}

func newExtensionStore(mode extendMode) *extensionStore {
	return &extensionStore{
		selectors:            map[string]*selectorSet{},
		extensions:           map[string]*targetExtensions{},
		extensionsByExtender: map[string][]*extension{},
		mediaContexts:        map[*box][]string{},
		sourceSpecificity:    map[string]int{},
		originals:            map[string]bool{},
		mode:                 mode,
	}
}

// clone returns a deep, independent copy of the store: its extensions map and
// every selector registration. Selector ASTs (simpleSel/selComplex) are treated
// as immutable and shared, exactly as extension does; the only mutable state —
// each box's selector value and the maps themselves — is copied. Shared pointer
// identity is preserved WITHIN the copy: a box or extension reachable through
// several maps clones to a single new instance, so the copy extends exactly as
// the original would, yet a subsequent addSelector/addExtension/addExtensions on
// either store never disturbs the other.
//
// This is step 1 of the per-@import-clone extension rebuild. A legacy @import
// duplicates a module's CSS (reEmitImportedCSS), and each duplicate needs its
// own store so the importing context can extend the duplicated rules without
// touching the original module's rules. clone() supplies that per-duplicate
// store; wiring downstream composition onto it is a later wave.
func (s *extensionStore) clone() *extensionStore {
	c := newExtensionStore(s.mode)
	boxMap := map[*box]*box{}
	extMap := map[*extension]*extension{}

	cloneBox := func(b *box) *box {
		if b == nil {
			return nil
		}
		if nb, ok := boxMap[b]; ok {
			return nb
		}
		nb := &box{value: b.value}
		boxMap[b] = nb
		return nb
	}
	cloneExtension := func(e *extension) *extension {
		if e == nil {
			return nil
		}
		if ne, ok := extMap[e]; ok {
			return ne
		}
		ne := &extension{target: e.target, optional: e.optional}
		if e.mediaContext != nil {
			ne.mediaContext = append([]string(nil), e.mediaContext...)
		}
		extMap[e] = ne
		if e.extender != nil {
			ne.extender = &extender{
				selector:    e.extender.selector,
				specificity: e.extender.specificity,
				isOriginal:  e.extender.isOriginal,
				ext:         ne,
			}
		}
		return ne
	}

	for k, set := range s.selectors {
		ns := &selectorSet{simple: set.simple}
		for _, b := range set.boxes {
			ns.boxes = append(ns.boxes, cloneBox(b))
		}
		c.selectors[k] = ns
	}
	for k, te := range s.extensions {
		nt := &targetExtensions{
			target:  te.target,
			sources: make(map[string]*extension, len(te.sources)),
			order:   append([]string(nil), te.order...),
		}
		for ck, e := range te.sources {
			nt.sources[ck] = cloneExtension(e)
		}
		c.extensions[k] = nt
	}
	for k, exts := range s.extensionsByExtender {
		ne := make([]*extension, len(exts))
		for i, e := range exts {
			ne[i] = cloneExtension(e)
		}
		c.extensionsByExtender[k] = ne
	}
	for b, mc := range s.mediaContexts {
		c.mediaContexts[cloneBox(b)] = append([]string(nil), mc...)
	}
	for k, v := range s.sourceSpecificity {
		c.sourceSpecificity[k] = v
	}
	for k, v := range s.originals {
		c.originals[k] = v
	}
	return c
}

func simpleKey(s simpleSel) string    { return selSimpleString(s) }
func complexKey(c *selComplex) string { return c.String() }

func strSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// --- static extend / replace (selector.extend / selector.replace) ---

func extendOrReplace(selector, source, targets *selList, mode extendMode) *selList {
	store := newExtensionStore(mode)
	if !selector.isInvisible() {
		for _, c := range selector.components {
			store.originals[complexKey(c)] = true
		}
	}

	for _, complex := range targets.components {
		compound := complex.singleCompound()
		if compound == nil {
			panic(selErr("Can't extend complex selector " + complex.String() + "."))
		}
		extensions := map[string]*targetExtensions{}
		for _, simple := range compound.components {
			te := &targetExtensions{target: simple, sources: map[string]*extension{}}
			for _, sc := range source.components {
				e := &extension{target: simple, optional: true}
				e.extender = &extender{selector: sc, specificity: sc.specificity(), ext: e}
				te.put(complexKey(sc), e)
			}
			extensions[simpleKey(simple)] = te
		}
		selector = store.extendList(selector, extensions, nil)
	}
	return selector
}

// --- incremental API used by the evaluator ---

func (s *extensionStore) addSelector(selector *selList, mediaContext []string) *box {
	if !selector.isInvisible() {
		for _, c := range selector.components {
			s.originals[complexKey(c)] = true
		}
	}
	if len(s.extensions) != 0 {
		selector = s.extendList(selector, s.extensions, mediaContext)
	}
	b := &box{value: selector}
	if mediaContext != nil {
		s.mediaContexts[b] = mediaContext
	}
	s.registerSelector(selector, b)
	return b
}

func (s *extensionStore) registerSelector(list *selList, b *box) {
	for _, complex := range list.components {
		for _, component := range complex.components {
			for _, simple := range component.selector.components {
				k := simpleKey(simple)
				set := s.selectors[k]
				if set == nil {
					set = &selectorSet{simple: simple}
					s.selectors[k] = set
				}
				if !boxIn(set.boxes, b) {
					set.boxes = append(set.boxes, b)
				}
				if ps, ok := simple.(*pseudoSel); ok && ps.selector != nil {
					s.registerSelector(ps.selector, b)
				}
			}
		}
	}
}

func boxIn(list []*box, b *box) bool {
	for _, x := range list {
		if x == b {
			return true
		}
	}
	return false
}

func (s *extensionStore) addExtension(extenderList *selList, target simpleSel, optional bool, mediaContext []string) {
	targetK := simpleKey(target)
	set := s.selectors[targetK]
	existingExtensions := s.extensionsByExtender[targetK]

	var newExtensions map[string]*targetExtensions
	sources := s.extensions[targetK]
	if sources == nil {
		sources = &targetExtensions{target: target, sources: map[string]*extension{}}
		s.extensions[targetK] = sources
	}

	for _, complex := range extenderList.components {
		if complex.isUseless() {
			continue
		}
		e := &extension{target: target, mediaContext: mediaContext, optional: optional}
		e.extender = &extender{selector: complex, specificity: complex.specificity(), ext: e}

		ck := complexKey(complex)
		if _, ok := sources.sources[ck]; ok {
			// Extension already present; nothing new to compute for output.
			continue
		}
		sources.put(ck, e)

		for _, simple := range simpleSelectorsIn(complex) {
			sk := simpleKey(simple)
			s.extensionsByExtender[sk] = append(s.extensionsByExtender[sk], e)
			if _, ok := s.sourceSpecificity[sk]; !ok {
				s.sourceSpecificity[sk] = complex.specificity()
			}
		}

		if set != nil || existingExtensions != nil {
			if newExtensions == nil {
				newExtensions = map[string]*targetExtensions{}
			}
			te := newExtensions[targetK]
			if te == nil {
				te = &targetExtensions{target: target, sources: map[string]*extension{}}
				newExtensions[targetK] = te
			}
			te.put(ck, e)
		}
	}

	if newExtensions == nil {
		return
	}

	if existingExtensions != nil {
		// Dart captures `existingExtensions` as a live reference to
		// `_extensionsByExtender[target]` and mutates that same list in place while
		// registering the new extenders above (`.add`), so by the time it iterates
		// (`.toList()`), the snapshot includes those just-added extenders. A newly
		// added extender that itself contains `target` (self-composition, e.g.
		// `:has(:not(.x))` extending `.x`) must therefore be re-extended too. Go's
		// slice-header capture would miss the loop's appends, so re-read the current
		// slice from the map to preserve Dart's aliasing semantics. The nil guard
		// above still reflects the entry-time state, matching Dart's local variable.
		additional := s.extendExistingExtensions(s.extensionsByExtender[targetK], newExtensions)
		// extendExistingExtensions only ever keys `additional` by targets it has
		// already confirmed present in newExtensions, so dst is always non-nil.
		for k, te := range additional {
			dst := newExtensions[k]
			for _, ck := range te.order {
				dst.put(ck, te.sources[ck])
			}
		}
	}

	if set != nil {
		s.extendExistingSelectors(set.boxes, newExtensions)
	}
}

// addExtensions merges the extension records from downstream module stores into
// this (upstream) store and re-extends this store's registered selectors and
// extensions with them. This is Dart Sass's ExtensionStore.addExtensions and is
// what makes cross-module @extend both work and stay isolated: a downstream
// extend only re-extends selectors THIS module registered, never selectors that
// a different downstream module contributed, so sibling modules never leak into
// one another. A private placeholder target (`%-x`/`%_x`) is a module-private
// member and never crosses a module boundary, so it is skipped.
func (s *extensionStore) addExtensions(others []*extensionStore) {
	var newExtensions map[string]*targetExtensions
	add := func(targetK string, target simpleSel, ck string, e *extension) {
		if newExtensions == nil {
			newExtensions = map[string]*targetExtensions{}
		}
		te := newExtensions[targetK]
		if te == nil {
			te = &targetExtensions{target: target, sources: map[string]*extension{}}
			newExtensions[targetK] = te
		}
		te.put(ck, e)
	}
	// A downstream extend re-extends only the targets this module already knew
	// about BEFORE the merge — its own registered selectors and its own extends'
	// extenders. Selectors/extenders introduced by ANOTHER downstream module
	// during this same merge must not become extendable, or sibling modules
	// would leak into each other (e.g. a diamond where left @extends a selector
	// that only right defines). Snapshot the extendable target set up front.
	extendable := map[string]bool{}
	for k := range s.selectors {
		extendable[k] = true
	}
	for k := range s.extensionsByExtender {
		extendable[k] = true
	}
	for _, other := range others {
		for _, targetK := range sortedKeys(other.extensions) {
			src := other.extensions[targetK]
			if ph, ok := src.target.(*placeholderSel); ok && ph.isPrivate() {
				continue
			}
			for _, ck := range src.order {
				if existing := s.extensions[targetK]; existing != nil {
					if _, ok := existing.sources[ck]; ok {
						continue
					}
				}
				e := src.sources[ck]
				if s.extensions[targetK] == nil {
					s.extensions[targetK] = &targetExtensions{target: src.target, sources: map[string]*extension{}}
				}
				s.extensions[targetK].put(ck, e)
				for _, simple := range simpleSelectorsIn(e.extender.selector) {
					sk := simpleKey(simple)
					s.extensionsByExtender[sk] = append(s.extensionsByExtender[sk], e)
					if _, ok := s.sourceSpecificity[sk]; !ok {
						s.sourceSpecificity[sk] = e.extender.selector.specificity()
					}
				}
				// Only extensions targeting a selector or extender this module
				// already knew about (before the merge) re-extend its output.
				if extendable[targetK] {
					add(targetK, src.target, ck, e)
				}
			}
		}
	}
	if newExtensions == nil {
		return
	}
	for _, targetK := range sortedKeys(newExtensions) {
		existingExtensions := s.extensionsByExtender[targetK]
		if len(existingExtensions) == 0 {
			continue
		}
		// extendExistingExtensions only keys `additional` by targets it has
		// confirmed present in newExtensions, so dst is always non-nil.
		additional := s.extendExistingExtensions(existingExtensions, newExtensions)
		for k, te := range additional {
			dst := newExtensions[k]
			for _, ck := range te.order {
				dst.put(ck, te.sources[ck])
			}
		}
	}
	// Every newExtensions target came from the pre-merge `extendable` snapshot,
	// which for a freshly-built store is exactly its registered selectors (each
	// extender simple is a rule simple), so every target has a selector set.
	seen := map[*box]bool{}
	var boxes []*box
	for _, targetK := range sortedKeys(newExtensions) {
		for _, b := range s.selectors[targetK].boxes {
			if !seen[b] {
				seen[b] = true
				boxes = append(boxes, b)
			}
		}
	}
	s.extendExistingSelectors(boxes, newExtensions)
}

func simpleSelectorsIn(complex *selComplex) []simpleSel {
	var out []simpleSel
	for _, component := range complex.components {
		for _, simple := range component.selector.components {
			out = append(out, simple)
			if ps, ok := simple.(*pseudoSel); ok && ps.selector != nil {
				for _, c := range ps.selector.components {
					out = append(out, simpleSelectorsIn(c)...)
				}
			}
		}
	}
	return out
}

func (s *extensionStore) extendExistingExtensions(extensions []*extension, newExtensions map[string]*targetExtensions) map[string]*targetExtensions {
	var additional map[string]*targetExtensions
	for _, ext := range append([]*extension{}, extensions...) {
		sources := s.extensions[simpleKey(ext.target)]
		selectors := s.extendComplex(ext.extender.selector, newExtensions, ext.mediaContext)
		if selectors == nil {
			continue
		}
		containsExtension := selectors[0].equal(ext.extender.selector)
		if containsExtension {
			selectors = selectors[1:]
		}
		for _, complex := range selectors {
			withExtender := &extension{target: ext.target, mediaContext: ext.mediaContext, optional: ext.optional}
			withExtender.extender = &extender{selector: complex, specificity: ext.extender.specificity, ext: withExtender}
			ck := complexKey(complex)
			if _, ok := sources.sources[ck]; ok {
				continue
			}
			sources.put(ck, withExtender)
			for _, component := range complex.components {
				for _, simple := range component.selector.components {
					sk := simpleKey(simple)
					s.extensionsByExtender[sk] = append(s.extensionsByExtender[sk], withExtender)
				}
			}
			if _, ok := newExtensions[simpleKey(ext.target)]; ok {
				if additional == nil {
					additional = map[string]*targetExtensions{}
				}
				tk := simpleKey(ext.target)
				te := additional[tk]
				if te == nil {
					te = &targetExtensions{target: ext.target, sources: map[string]*extension{}}
					additional[tk] = te
				}
				te.put(ck, withExtender)
			}
		}
	}
	return additional
}

func (s *extensionStore) extendExistingSelectors(boxes []*box, newExtensions map[string]*targetExtensions) {
	for _, b := range boxes {
		oldValue := b.value
		b.value = s.extendList(b.value, newExtensions, s.mediaContexts[b])
		if b.value == oldValue {
			continue
		}
		s.registerSelector(b.value, b)
	}
}

// extendList extends list using extensions.
func (s *extensionStore) extendList(list *selList, extensions map[string]*targetExtensions, mediaQueryContext []string) *selList {
	var extended []*selComplex
	for i, complex := range list.components {
		result := s.extendComplex(complex, extensions, mediaQueryContext)
		if result == nil {
			if extended != nil {
				extended = append(extended, complex)
			}
		} else {
			if extended == nil {
				extended = append([]*selComplex{}, list.components[:i]...)
			}
			extended = append(extended, result...)
		}
	}
	if extended == nil {
		return list
	}
	return &selList{components: s.trim(extended, func(c *selComplex) bool { return s.originals[complexKey(c)] })}
}

func (s *extensionStore) extendComplex(complex *selComplex, extensions map[string]*targetExtensions, mediaQueryContext []string) []*selComplex {
	if len(complex.leadingCombinators) > 1 {
		return nil
	}

	var extendedNotExpanded [][]*selComplex
	isOriginal := s.originals[complexKey(complex)]
	for i, component := range complex.components {
		extended := s.extendCompound(component, extensions, mediaQueryContext, isOriginal)
		if extended == nil {
			if extendedNotExpanded != nil {
				extendedNotExpanded = append(extendedNotExpanded, []*selComplex{{
					components: []complexComponent{component},
					lineBreak:  complex.lineBreak,
				}})
			}
		} else if extendedNotExpanded != nil {
			extendedNotExpanded = append(extendedNotExpanded, extended)
		} else if i != 0 {
			extendedNotExpanded = [][]*selComplex{
				{{leadingCombinators: complex.leadingCombinators, components: complex.components[:i], lineBreak: complex.lineBreak}},
				extended,
			}
		} else if len(complex.leadingCombinators) == 0 {
			extendedNotExpanded = [][]*selComplex{extended}
		} else {
			var withLeading []*selComplex
			for _, nc := range extended {
				if len(nc.leadingCombinators) == 0 || combSliceEqual(complex.leadingCombinators, nc.leadingCombinators) {
					withLeading = append(withLeading, &selComplex{
						leadingCombinators: complex.leadingCombinators,
						components:         nc.components,
						lineBreak:          complex.lineBreak || nc.lineBreak,
					})
				}
			}
			extendedNotExpanded = [][]*selComplex{withLeading}
		}
	}
	if extendedNotExpanded == nil {
		return nil
	}

	first := true
	var out []*selComplex
	for _, path := range pathsComplex(extendedNotExpanded) {
		for _, outputComplex := range weave(path, complex.lineBreak) {
			if first && s.originals[complexKey(complex)] {
				s.originals[complexKey(outputComplex)] = true
			}
			first = false
			out = append(out, outputComplex)
		}
	}
	return out
}

func (s *extensionStore) extendCompound(component complexComponent, extensions map[string]*targetExtensions, mediaQueryContext []string, inOriginal bool) []*selComplex {
	var targetsUsed map[string]bool
	if !(s.mode == extendNormal || len(extensions) < 2) {
		targetsUsed = map[string]bool{}
	}

	simples := component.selector.components
	var options [][]*extender
	for i, simple := range simples {
		extended := s.extendSimple(simple, extensions, mediaQueryContext, targetsUsed)
		if extended == nil {
			if options != nil {
				options = append(options, []*extender{extenderForSimple(simple, s.sourceSpecificity[simpleKey(simple)])})
			}
		} else {
			if options == nil {
				options = [][]*extender{}
				if i != 0 {
					options = append(options, []*extender{s.extenderForCompound(simples[:i])})
				}
			}
			options = append(options, extended...)
		}
	}
	if options == nil {
		return nil
	}

	if targetsUsed != nil && len(targetsUsed) != len(extensions) {
		return nil
	}

	if len(options) == 1 {
		var result []*selComplex
		for _, ext := range options[0] {
			ext.assertCompatibleMediaContext(mediaQueryContext)
			complex := ext.selector.withAdditionalCombinators(component.combinators, false)
			if complex.isUseless() {
				continue
			}
			result = append(result, complex)
		}
		return result
	}

	extenderPaths := pathsExtender(options)
	var result []*selComplex
	if s.mode != extendReplace {
		first := extenderPaths[0]
		var simplesOut []simpleSel
		for _, ext := range first {
			last := ext.selector.components[len(ext.selector.components)-1].selector
			simplesOut = append(simplesOut, last.components...)
		}
		result = append(result, &selComplex{
			components: []complexComponent{{
				selector:    &compoundSel{components: simplesOut},
				combinators: component.combinators,
			}},
		})
	}

	startIdx := 1
	if s.mode == extendReplace {
		startIdx = 0
	}
	for _, path := range extenderPaths[startIdx:] {
		extended := s.unifyExtenders(path, mediaQueryContext)
		if extended == nil {
			continue
		}
		for _, complex := range extended {
			withCombinators := complex.withAdditionalCombinators(component.combinators, false)
			if !withCombinators.isUseless() {
				result = append(result, withCombinators)
			}
		}
	}

	origKey := ""
	if inOriginal && s.mode != extendReplace {
		origKey = complexKey(result[0])
	}
	isOrig := func(c *selComplex) bool {
		return origKey != "" && complexKey(c) == origKey
	}
	return s.trim(result, isOrig)
}

func (s *extensionStore) unifyExtenders(extenders []*extender, mediaQueryContext []string) []*selComplex {
	var toUnify []*selComplex
	var originals []simpleSel
	originalsLineBreak := false
	for _, ext := range extenders {
		if ext.isOriginal {
			last := ext.selector.components[len(ext.selector.components)-1].selector
			originals = append(originals, last.components...)
			originalsLineBreak = originalsLineBreak || ext.selector.lineBreak
		} else if ext.selector.isUseless() {
			return nil
		} else {
			toUnify = append(toUnify, ext.selector)
		}
	}
	if originals != nil {
		toUnify = append([]*selComplex{{
			components: []complexComponent{{selector: &compoundSel{components: originals}}},
			lineBreak:  originalsLineBreak,
		}}, toUnify...)
	}
	complexes, ok := unifyComplex(toUnify)
	if !ok {
		return nil
	}
	for _, ext := range extenders {
		ext.assertCompatibleMediaContext(mediaQueryContext)
	}
	return complexes
}

func (s *extensionStore) extendSimple(simple simpleSel, extensions map[string]*targetExtensions, mediaQueryContext []string, targetsUsed map[string]bool) [][]*extender {
	withoutPseudo := func(simple simpleSel) []*extender {
		te := extensions[simpleKey(simple)]
		if te == nil {
			return nil
		}
		if targetsUsed != nil {
			targetsUsed[simpleKey(simple)] = true
		}
		var out []*extender
		if s.mode != extendReplace {
			out = append(out, extenderForSimple(simple, s.sourceSpecificity[simpleKey(simple)]))
		}
		for _, ext := range te.values() {
			out = append(out, ext.extender)
		}
		return out
	}

	if ps, ok := simple.(*pseudoSel); ok && ps.selector != nil {
		if extended := s.extendPseudo(ps, extensions, mediaQueryContext); extended != nil {
			var out [][]*extender
			for _, pseudo := range extended {
				w := withoutPseudo(pseudo)
				if w == nil {
					w = []*extender{extenderForSimple(pseudo, s.sourceSpecificity[simpleKey(pseudo)])}
				}
				out = append(out, w)
			}
			return out
		}
	}

	w := withoutPseudo(simple)
	if w == nil {
		return nil
	}
	return [][]*extender{w}
}

func (s *extensionStore) extenderForCompound(simples []simpleSel) *extender {
	compound := &compoundSel{components: simples}
	return &extender{
		selector:    &selComplex{components: []complexComponent{{selector: compound}}},
		specificity: s.sourceSpecificityFor(compound),
		isOriginal:  true,
	}
}

func extenderForSimple(simple simpleSel, specificity int) *extender {
	return &extender{
		selector:    &selComplex{components: []complexComponent{{selector: &compoundSel{components: []simpleSel{simple}}}}},
		specificity: specificity,
		isOriginal:  true,
	}
}

func (s *extensionStore) extendPseudo(pseudo *pseudoSel, extensions map[string]*targetExtensions, mediaQueryContext []string) []*pseudoSel {
	selector := pseudo.selector
	extended := s.extendList(selector, extensions, mediaQueryContext)
	if extended == selector {
		return nil
	}

	var complexes []*selComplex = extended.components
	if pseudo.normalizedName == "not" &&
		!anyComplexMultiComponent(selector.components) &&
		anyComplexSingleComponent(extended.components) {
		var filtered []*selComplex
		for _, c := range extended.components {
			if len(c.components) <= 1 {
				filtered = append(filtered, c)
			}
		}
		complexes = filtered
	}

	var expanded []*selComplex
	for _, complex := range complexes {
		sc := complex.singleCompound()
		var innerPseudo *pseudoSel
		if sc != nil {
			if ss := sc.singleSimple(); ss != nil {
				if ip, ok := ss.(*pseudoSel); ok {
					innerPseudo = ip
				}
			}
		}
		if innerPseudo == nil || innerPseudo.selector == nil {
			expanded = append(expanded, complex)
			continue
		}
		switch pseudo.normalizedName {
		case "not":
			if innerPseudo.normalizedName == "is" || innerPseudo.normalizedName == "matches" || innerPseudo.normalizedName == "where" {
				expanded = append(expanded, innerPseudo.selector.components...)
			}
		case "is", "matches", "where", "any", "current", "nth-child", "nth-last-child":
			if innerPseudo.name == pseudo.name && strEqualPtr(innerPseudo.argument, pseudo.argument) {
				expanded = append(expanded, innerPseudo.selector.components...)
			}
		case "has", "host", "host-context", "slotted":
			expanded = append(expanded, complex)
		}
	}
	complexes = expanded

	if pseudo.normalizedName == "not" && len(selector.components) == 1 {
		var result []*pseudoSel
		for _, complex := range complexes {
			result = append(result, pseudo.withSelector(&selList{components: []*selComplex{complex}}))
		}
		if len(result) == 0 {
			return nil
		}
		return result
	}
	return []*pseudoSel{pseudo.withSelector(&selList{components: complexes})}
}

func anyComplexMultiComponent(cs []*selComplex) bool {
	for _, c := range cs {
		if len(c.components) > 1 {
			return true
		}
	}
	return false
}

func anyComplexSingleComponent(cs []*selComplex) bool {
	for _, c := range cs {
		if len(c.components) == 1 {
			return true
		}
	}
	return false
}

func (s *extensionStore) trim(selectors []*selComplex, isOriginal func(*selComplex) bool) []*selComplex {
	if len(selectors) > 100 {
		return selectors
	}
	var result []*selComplex
	numOriginals := 0
outer:
	for i := len(selectors) - 1; i >= 0; i-- {
		complex1 := selectors[i]
		if isOriginal(complex1) {
			for j := 0; j < numOriginals; j++ {
				if result[j].equal(complex1) {
					rotateSliceComplex(result, 0, j+1)
					continue outer
				}
			}
			numOriginals++
			result = append([]*selComplex{complex1}, result...)
			continue
		}

		maxSpecificity := 0
		for _, component := range complex1.components {
			if sp := s.sourceSpecificityFor(component.selector); sp > maxSpecificity {
				maxSpecificity = sp
			}
		}

		superFound := false
		for _, complex2 := range result {
			if complex2.specificity() >= maxSpecificity && complex2.isSuper(complex1) {
				superFound = true
				break
			}
		}
		if superFound {
			continue
		}
		for _, complex2 := range selectors[:i] {
			if complex2.specificity() >= maxSpecificity && complex2.isSuper(complex1) {
				superFound = true
				break
			}
		}
		if superFound {
			continue
		}
		result = append([]*selComplex{complex1}, result...)
	}
	return result
}

func rotateSliceComplex(list []*selComplex, start, end int) {
	element := list[end-1]
	for i := start; i < end; i++ {
		next := list[i]
		list[i] = element
		element = next
	}
}

func (s *extensionStore) sourceSpecificityFor(compound *compoundSel) int {
	specificity := 0
	for _, simple := range compound.components {
		if sp := s.sourceSpecificity[simpleKey(simple)]; sp > specificity {
			specificity = sp
		}
	}
	return specificity
}

// pathsComplex / pathsExtender: dart's paths() over the two element types.
func pathsComplex(choices [][]*selComplex) [][]*selComplex {
	result := [][]*selComplex{{}}
	for _, choice := range choices {
		var next [][]*selComplex
		for _, option := range choice {
			for _, path := range result {
				np := append(append([]*selComplex{}, path...), option)
				next = append(next, np)
			}
		}
		result = next
	}
	return result
}

func pathsExtender(choices [][]*extender) [][]*extender {
	result := [][]*extender{{}}
	for _, choice := range choices {
		var next [][]*extender
		for _, option := range choice {
			for _, path := range result {
				np := append(append([]*extender{}, path...), option)
				next = append(next, np)
			}
		}
		result = next
	}
	return result
}
