// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

// This file ports Dart Sass's lib/src/extend/functions.dart: selector
// unification (unifyComplex/unifyCompound), the weave algorithm that interleaves
// parent selectors, and the superselector predicates.

var rootishPseudoClasses = map[string]bool{
	"root": true, "scope": true, "host": true, "host-context": true,
}

// unifyComplex returns a selector list matching only elements matched by every
// complex selector in complexes, or (nil,false) if none can be produced.
func unifyComplex(complexes []*selComplex) ([]*selComplex, bool) {
	if len(complexes) == 1 {
		return complexes, true
	}

	var unifiedBase *compoundSel
	var leadingCombinator *combinator
	var trailingCombinator *combinator
	anyLineBreak := false
	for _, complex := range complexes {
		if complex.isUseless() {
			return nil, false
		}
		if complex.lineBreak {
			anyLineBreak = true
		}
		if len(complex.components) == 1 && len(complex.leadingCombinators) == 1 {
			nlc := complex.leadingCombinators[0]
			if leadingCombinator == nil {
				leadingCombinator = &nlc
			} else if *leadingCombinator != nlc {
				return nil, false
			}
		}
		base := complex.components[len(complex.components)-1]
		if len(base.combinators) == 1 {
			ntc := base.combinators[0]
			if trailingCombinator != nil && *trailingCombinator != ntc {
				return nil, false
			}
			trailingCombinator = &ntc
		}
		if unifiedBase == nil {
			unifiedBase = base.selector
		} else {
			u, ok := unifyCompound(unifiedBase, base.selector)
			if !ok {
				return nil, false
			}
			unifiedBase = u
		}
	}

	var withoutBases []*selComplex
	for _, complex := range complexes {
		if len(complex.components) > 1 {
			withoutBases = append(withoutBases, &selComplex{
				leadingCombinators: complex.leadingCombinators,
				components:         complex.components[:len(complex.components)-1],
				lineBreak:          complex.lineBreak,
			})
		}
	}

	var baseLeading []combinator
	if leadingCombinator != nil {
		baseLeading = []combinator{*leadingCombinator}
	}
	var baseTrailing []combinator
	if trailingCombinator != nil {
		baseTrailing = []combinator{*trailingCombinator}
	}
	base := &selComplex{
		leadingCombinators: baseLeading,
		components:         []complexComponent{{selector: unifiedBase, combinators: baseTrailing}},
		lineBreak:          anyLineBreak,
	}

	if len(withoutBases) == 0 {
		return weave([]*selComplex{base}, false), true
	}
	last := withoutBases[len(withoutBases)-1].concatenate(base, false)
	woven := append(append([]*selComplex{}, withoutBases[:len(withoutBases)-1]...), last)
	return weave(woven, false), true
}

// unifyCompound returns a compound selector matching both compound1 and
// compound2, preserving pseudo-class/element ordering, or (nil,false).
func unifyCompound(compound1, compound2 *compoundSel) (*compoundSel, bool) {
	result := compound1.components
	var pseudoResult []simpleSel
	pseudoElementFound := false

	for _, simple := range compound2.components {
		ps, isPseudo := simple.(*pseudoSel)
		if pseudoElementFound && isPseudo {
			u, ok := ps.unify(pseudoResult)
			if !ok {
				return nil, false
			}
			pseudoResult = u
		} else {
			if isPseudo && ps.isElement() {
				pseudoElementFound = true
			}
			u, ok := simple.unify(result)
			if !ok {
				return nil, false
			}
			result = u
		}
	}
	return &compoundSel{components: append(append([]simpleSel{}, result...), pseudoResult...)}, true
}

// unifyUniversalAndElement unifies two universal/type selectors.
func unifyUniversalAndElement(selector1, selector2 simpleSel) (simpleSel, bool) {
	ns1, name1, ok1 := namespaceAndName(selector1)
	ns2, name2, ok2 := namespaceAndName(selector2)
	if !ok1 || !ok2 {
		panic(selErr("must be a UniversalSelector or a TypeSelector"))
	}

	var namespace *string
	switch {
	case nsEqual(ns1, ns2) || (ns2 != nil && *ns2 == "*"):
		namespace = ns1
	case ns1 != nil && *ns1 == "*":
		namespace = ns2
	default:
		return nil, false
	}

	var name *string
	switch {
	case strEqualPtr(name1, name2) || name2 == nil:
		name = name1
	case name1 == nil || *name1 == "*":
		name = name2
	default:
		return nil, false
	}

	if name == nil {
		return &universalSel{ns: namespace}, true
	}
	return &typeSel{name: qname{name: *name, ns: namespace}}, true
}

func namespaceAndName(s simpleSel) (ns *string, name *string, ok bool) {
	switch v := s.(type) {
	case *universalSel:
		return v.ns, nil, true
	case *typeSel:
		return v.name.ns, strptr(v.name.name), true
	}
	return nil, nil, false
}

func strEqualPtr(a, b *string) bool {
	if (a == nil) != (b == nil) {
		return false
	}
	return a == nil || *a == *b
}

// weave interleaves parenthesized selectors. See functions.dart's weave.
func weave(complexes []*selComplex, forceLineBreak bool) []*selComplex {
	if len(complexes) == 1 {
		complex := complexes[0]
		if !forceLineBreak || complex.lineBreak {
			return complexes
		}
		return []*selComplex{{complex.leadingCombinators, complex.components, true}}
	}

	prefixes := []*selComplex{complexes[0]}
	for _, complex := range complexes[1:] {
		if len(complex.components) == 1 {
			for i := range prefixes {
				prefixes[i] = prefixes[i].concatenate(complex, forceLineBreak)
			}
			continue
		}
		var next []*selComplex
		lastComp := complex.components[len(complex.components)-1]
		for _, prefix := range prefixes {
			parents := weaveParents(prefix, complex)
			for _, parentPrefix := range parents {
				next = append(next, parentPrefix.withAdditionalComponent(lastComp, forceLineBreak))
			}
		}
		prefixes = next
	}
	return prefixes
}

// weaveParents interleaves prefix's components with base's components except the
// last. Returns (nil,false) if the intersection is empty.
func weaveParents(prefix, base *selComplex) []*selComplex {
	leadingCombinators, ok := mergeLeadingCombinators(prefix.leadingCombinators, base.leadingCombinators)
	if !ok {
		return nil
	}

	queue1 := append([]complexComponent{}, prefix.components...)
	baseExceptLast := base.components[:len(base.components)-1]
	queue2 := append([]complexComponent{}, baseExceptLast...)

	trailingCombinators, ok := mergeTrailingCombinators(&queue1, &queue2, nil)
	if !ok {
		return nil
	}

	rootish1, has1 := firstIfRootish(&queue1)
	rootish2, has2 := firstIfRootish(&queue2)
	switch {
	case has1 && has2:
		rootish, ok := unifyCompound(rootish1.selector, rootish2.selector)
		if !ok {
			return nil
		}
		queue1 = append([]complexComponent{{selector: rootish, combinators: rootish1.combinators}}, queue1...)
		queue2 = append([]complexComponent{{selector: rootish, combinators: rootish2.combinators}}, queue2...)
	case has1:
		queue1 = append([]complexComponent{rootish1}, queue1...)
		queue2 = append([]complexComponent{rootish1}, queue2...)
	case has2:
		queue1 = append([]complexComponent{rootish2}, queue1...)
		queue2 = append([]complexComponent{rootish2}, queue2...)
	}

	groups1 := groupSelectors(queue1)
	groups2 := groupSelectors(queue2)

	lcs := longestCommonSubsequence(groups2, groups1, func(group1, group2 []complexComponent) ([]complexComponent, bool) {
		if componentsEqual(group1, group2) {
			return group1, true
		}
		if complexIsParentSuperselector(group1, group2) {
			return group2, true
		}
		if complexIsParentSuperselector(group2, group1) {
			return group1, true
		}
		if !mustUnify(group1, group2) {
			return nil, false
		}
		unified, ok := unifyComplex([]*selComplex{
			{components: group1},
			{components: group2},
		})
		if !ok || len(unified) != 1 {
			return nil, false
		}
		return unified[0].components, true
	})

	var choices [][][]complexComponent
	for _, group := range lcs {
		g := group
		var opt [][]complexComponent
		for _, chunk := range chunks(&groups1, &groups2, func(seq [][]complexComponent) bool {
			return complexIsParentSuperselector(seq[0], g)
		}) {
			var flat []complexComponent
			for _, components := range chunk {
				flat = append(flat, components...)
			}
			opt = append(opt, flat)
		}
		choices = append(choices, opt)
		choices = append(choices, [][]complexComponent{group})
		groups1 = groups1[1:]
		groups2 = groups2[1:]
	}
	var lastOpt [][]complexComponent
	for _, chunk := range chunks(&groups1, &groups2, func(seq [][]complexComponent) bool { return len(seq) == 0 }) {
		var flat []complexComponent
		for _, components := range chunk {
			flat = append(flat, components...)
		}
		lastOpt = append(lastOpt, flat)
	}
	choices = append(choices, lastOpt)
	choices = append(choices, trailingCombinators...)

	// Filter empty choices.
	var filtered [][][]complexComponent
	for _, c := range choices {
		if len(c) != 0 {
			filtered = append(filtered, c)
		}
	}

	var out []*selComplex
	for _, path := range pathsComponents(filtered) {
		var comps []complexComponent
		for _, components := range path {
			comps = append(comps, components...)
		}
		out = append(out, &selComplex{
			leadingCombinators: leadingCombinators,
			components:         comps,
			lineBreak:          prefix.lineBreak || base.lineBreak,
		})
	}
	return out
}

// firstIfRootish removes and returns the first rootish component of queue.
func firstIfRootish(queue *[]complexComponent) (complexComponent, bool) {
	if len(*queue) == 0 {
		return complexComponent{}, false
	}
	first := (*queue)[0]
	for _, simple := range first.selector.components {
		if ps, ok := simple.(*pseudoSel); ok && ps.isClass && rootishPseudoClasses[ps.normalizedName] {
			*queue = (*queue)[1:]
			return first, true
		}
	}
	return complexComponent{}, false
}

func mergeLeadingCombinators(c1, c2 []combinator) ([]combinator, bool) {
	if len(c1) > 1 || len(c2) > 1 {
		return nil, false
	}
	if len(c1) == 0 {
		return c2, true
	}
	if len(c2) == 0 {
		return c1, true
	}
	if combSliceEqual(c1, c2) {
		return c1, true
	}
	return nil, false
}

// mergeTrailingCombinators merges trailing combinators of the two component
// queues, prepending results. Returns (nil,false) if unmergeable.
func mergeTrailingCombinators(components1, components2 *[]complexComponent, result [][][]complexComponent) ([][][]complexComponent, bool) {
	lastCombs := func(c []complexComponent) []combinator {
		if len(c) == 0 {
			return nil
		}
		return c[len(c)-1].combinators
	}
	combinators1 := lastCombs(*components1)
	combinators2 := lastCombs(*components2)
	if len(combinators1) == 0 && len(combinators2) == 0 {
		return result, true
	}
	if len(combinators1) > 1 || len(combinators2) > 1 {
		return nil, false
	}

	var c1v, c2v *combinator
	if len(combinators1) == 1 {
		c1v = &combinators1[0]
	}
	if len(combinators2) == 1 {
		c2v = &combinators2[0]
	}

	removeLast := func(q *[]complexComponent) complexComponent {
		last := (*q)[len(*q)-1]
		*q = (*q)[:len(*q)-1]
		return last
	}
	prepend := func(choices [][]complexComponent) {
		result = append([][][]complexComponent{choices}, result...)
	}

	switch {
	case c1v != nil && c2v != nil && *c1v == combFollowingSibling && *c2v == combFollowingSibling:
		component1 := removeLast(components1)
		component2 := removeLast(components2)
		if component1.selector.isSuper(component2.selector) {
			prepend([][]complexComponent{{component2}})
		} else if component2.selector.isSuper(component1.selector) {
			prepend([][]complexComponent{{component1}})
		} else {
			choices := [][]complexComponent{{component1, component2}, {component2, component1}}
			if unified, ok := unifyCompound(component1.selector, component2.selector); ok {
				choices = append(choices, []complexComponent{{selector: unified, combinators: []combinator{combinators1[0]}}})
			}
			prepend(choices)
		}
	case c1v != nil && c2v != nil &&
		((*c1v == combFollowingSibling && *c2v == combNextSibling) || (*c1v == combNextSibling && *c2v == combFollowingSibling)):
		var nextComponents, followingComponents *[]complexComponent
		if *c1v == combFollowingSibling {
			followingComponents, nextComponents = components1, components2
		} else {
			nextComponents, followingComponents = components1, components2
		}
		next := removeLast(nextComponents)
		following := removeLast(followingComponents)
		if following.selector.isSuper(next.selector) {
			prepend([][]complexComponent{{next}})
		} else {
			choices := [][]complexComponent{{following, next}}
			if unified, ok := unifyCompound(following.selector, next.selector); ok {
				choices = append(choices, []complexComponent{{selector: unified, combinators: next.combinators}})
			}
			prepend(choices)
		}
	case c1v != nil && c2v != nil && *c1v == combChild && (*c2v == combNextSibling || *c2v == combFollowingSibling):
		prepend([][]complexComponent{{removeLast(components2)}})
	case c1v != nil && c2v != nil && (*c1v == combNextSibling || *c1v == combFollowingSibling) && *c2v == combChild:
		prepend([][]complexComponent{{removeLast(components1)}})
	case c1v != nil && c2v != nil && *c1v == *c2v:
		unified, ok := unifyCompound(removeLast(components1).selector, removeLast(components2).selector)
		if !ok {
			return nil, false
		}
		prepend([][]complexComponent{{{selector: unified, combinators: []combinator{combinators1[0]}}}})
	case c1v != nil && c2v == nil:
		if *c1v == combChild && len(*components2) > 0 &&
			(*components2)[len(*components2)-1].selector.isSuper((*components1)[len(*components1)-1].selector) {
			*components2 = (*components2)[:len(*components2)-1]
		}
		prepend([][]complexComponent{{removeLast(components1)}})
	case c1v == nil && c2v != nil:
		if *c2v == combChild && len(*components1) > 0 &&
			(*components1)[len(*components1)-1].selector.isSuper((*components2)[len(*components2)-1].selector) {
			*components1 = (*components1)[:len(*components1)-1]
		}
		prepend([][]complexComponent{{removeLast(components2)}})
	}

	return mergeTrailingCombinators(components1, components2, result)
}

// mustUnify reports whether complex1 and complex2 share a unique simple selector.
func mustUnify(complex1, complex2 []complexComponent) bool {
	var uniqueSelectors []simpleSel
	for _, component := range complex1 {
		for _, simple := range component.selector.components {
			if isUniqueSimple(simple) {
				uniqueSelectors = append(uniqueSelectors, simple)
			}
		}
	}
	if len(uniqueSelectors) == 0 {
		return false
	}
	for _, component := range complex2 {
		for _, simple := range component.selector.components {
			if isUniqueSimple(simple) && containsSimple(uniqueSelectors, simple) {
				return true
			}
		}
	}
	return false
}

func isUniqueSimple(simple simpleSel) bool {
	if _, ok := simple.(*idSel); ok {
		return true
	}
	if ps, ok := simple.(*pseudoSel); ok && ps.isElement() {
		return true
	}
	return false
}

// chunks returns all orderings of initial subsequences of the two slices, and
// removes them from the slices. done marks the extent of the subsequence.
func chunks[T any](queue1, queue2 *[]T, done func([]T) bool) [][]T {
	var chunk1 []T
	for !done(*queue1) {
		chunk1 = append(chunk1, (*queue1)[0])
		*queue1 = (*queue1)[1:]
	}
	var chunk2 []T
	for !done(*queue2) {
		chunk2 = append(chunk2, (*queue2)[0])
		*queue2 = (*queue2)[1:]
	}
	if len(chunk1) == 0 && len(chunk2) == 0 {
		return nil
	}
	if len(chunk1) == 0 {
		return [][]T{chunk2}
	}
	if len(chunk2) == 0 {
		return [][]T{chunk1}
	}
	return [][]T{
		append(append([]T{}, chunk1...), chunk2...),
		append(append([]T{}, chunk2...), chunk1...),
	}
}

// pathsComponents returns all paths through the given choice lists.
func pathsComponents(choices [][][]complexComponent) [][][]complexComponent {
	result := [][][]complexComponent{{}}
	for _, choice := range choices {
		var next [][][]complexComponent
		for _, option := range choice {
			for _, path := range result {
				np := append(append([][]complexComponent{}, path...), option)
				next = append(next, np)
			}
		}
		result = next
	}
	return result
}

// groupSelectors groups complex into longest sub-lists where components without
// combinators only appear at the end.
func groupSelectors(complex []complexComponent) [][]complexComponent {
	var groups [][]complexComponent
	var group []complexComponent
	for _, component := range complex {
		group = append(group, component)
		if len(component.combinators) == 0 {
			groups = append(groups, group)
			group = nil
		}
	}
	if len(group) > 0 {
		groups = append(groups, group)
	}
	return groups
}

func componentsEqual(a, b []complexComponent) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !a[i].equal(&b[i]) {
			return false
		}
	}
	return true
}

// listIsSuperselector reports whether list1 is a superselector of list2.
func listIsSuperselector(list1, list2 []*selComplex) bool {
	for _, complex1 := range list2 {
		any := false
		for _, complex2 := range list1 {
			if complex2.isSuper(complex1) {
				any = true
				break
			}
		}
		if !any {
			return false
		}
	}
	return true
}

var tempPlaceholderComponent = complexComponent{
	selector: &compoundSel{components: []simpleSel{&placeholderSel{name: "<temp>"}}},
}

func complexIsParentSuperselector(complex1, complex2 []complexComponent) bool {
	if len(complex1) > len(complex2) {
		return false
	}
	base := tempPlaceholderComponent
	c1 := append(append([]complexComponent{}, complex1...), base)
	c2 := append(append([]complexComponent{}, complex2...), base)
	return complexIsSuperselector(c1, c2)
}

// complexIsSuperselector reports whether complex1 is a superselector of complex2.
func complexIsSuperselector(complex1, complex2 []complexComponent) bool {
	if len(complex1[len(complex1)-1].combinators) != 0 {
		return false
	}
	if len(complex2[len(complex2)-1].combinators) != 0 {
		return false
	}

	i1 := 0
	i2 := 0
	var previousCombinator *combinator
	for {
		remaining1 := len(complex1) - i1
		remaining2 := len(complex2) - i2
		if remaining1 > remaining2 {
			return false
		}

		component1 := complex1[i1]
		if len(component1.combinators) > 1 {
			return false
		}
		if remaining1 == 1 {
			for _, parent := range complex2 {
				if len(parent.combinators) > 1 {
					return false
				}
			}
			var parents []complexComponent
			if component1.selector.hasComplicated() {
				parents = complex2[i2 : len(complex2)-1]
			}
			return compoundIsSuperselector(component1.selector, complex2[len(complex2)-1].selector, parents)
		}

		endOfSubselector := i2
		for {
			component2 := complex2[endOfSubselector]
			if len(component2.combinators) > 1 {
				return false
			}
			var parents []complexComponent
			if component1.selector.hasComplicated() {
				parents = complex2[i2:endOfSubselector]
			}
			if compoundIsSuperselector(component1.selector, component2.selector, parents) {
				break
			}
			endOfSubselector++
			if endOfSubselector == len(complex2)-1 {
				return false
			}
		}

		if !compatibleWithPreviousCombinator(previousCombinator, complex2[i2:endOfSubselector]) {
			return false
		}

		component2 := complex2[endOfSubselector]
		combinator1 := firstCombinator(component1.combinators)
		combinator2 := firstCombinator(component2.combinators)
		if !isSupercombinator(combinator1, combinator2) {
			return false
		}

		i1++
		i2 = endOfSubselector + 1
		previousCombinator = combinator1

		if len(complex1)-i1 == 1 {
			if combinator1 != nil && *combinator1 == combFollowingSibling {
				for _, component := range complex2[i2 : len(complex2)-1] {
					if !isSupercombinator(combinator1, firstCombinator(component.combinators)) {
						return false
					}
				}
			} else if combinator1 != nil {
				if len(complex2)-i2 > 1 {
					return false
				}
			}
		}
	}
}

func firstCombinator(combs []combinator) *combinator {
	if len(combs) == 0 {
		return nil
	}
	c := combs[0]
	return &c
}

func compatibleWithPreviousCombinator(previous *combinator, parents []complexComponent) bool {
	if len(parents) == 0 {
		return true
	}
	if previous == nil {
		return true
	}
	if *previous != combFollowingSibling {
		return false
	}
	for _, component := range parents {
		fc := firstCombinator(component.combinators)
		if fc == nil || (*fc != combFollowingSibling && *fc != combNextSibling) {
			return false
		}
	}
	return true
}

func isSupercombinator(c1, c2 *combinator) bool {
	if (c1 == nil) == (c2 == nil) && (c1 == nil || *c1 == *c2) {
		return true
	}
	if c1 == nil && c2 != nil && *c2 == combChild {
		return true
	}
	if c1 != nil && *c1 == combFollowingSibling && c2 != nil && *c2 == combNextSibling {
		return true
	}
	return false
}

// compoundIsSuperselector reports whether compound1 is a superselector of
// compound2, given compound2's parents (for selector-pseudo arguments).
func compoundIsSuperselector(compound1, compound2 *compoundSel, parents []complexComponent) bool {
	if !compound1.hasComplicated() && !compound2.hasComplicated() {
		if len(compound1.components) > len(compound2.components) {
			return false
		}
		for _, simple1 := range compound1.components {
			any := false
			for _, simple2 := range compound2.components {
				if simple1.isSuper(simple2) {
					any = true
					break
				}
			}
			if !any {
				return false
			}
		}
		return true
	}

	pseudo1, index1, found1 := findPseudoElementIndexed(compound1)
	pseudo2, index2, found2 := findPseudoElementIndexed(compound2)
	if found1 && found2 {
		return pseudo1.isSuper(pseudo2) &&
			compoundComponentsIsSuperselector(compound1.components[:index1], compound2.components[:index2], parents) &&
			compoundComponentsIsSuperselector(compound1.components[index1+1:], compound2.components[index2+1:], parents)
	}
	if found1 || found2 {
		return false
	}

	for _, simple1 := range compound1.components {
		if ps, ok := simple1.(*pseudoSel); ok && ps.selector != nil {
			if !selectorPseudoIsSuperselector(ps, compound2, parents) {
				return false
			}
		} else {
			any := false
			for _, simple2 := range compound2.components {
				if simple1.isSuper(simple2) {
					any = true
					break
				}
			}
			if !any {
				return false
			}
		}
	}
	return true
}

func findPseudoElementIndexed(compound *compoundSel) (*pseudoSel, int, bool) {
	for i, simple := range compound.components {
		if ps, ok := simple.(*pseudoSel); ok && ps.isElement() {
			return ps, i, true
		}
	}
	return nil, 0, false
}

func compoundComponentsIsSuperselector(compound1, compound2 []simpleSel, parents []complexComponent) bool {
	if len(compound1) == 0 {
		return true
	}
	if len(compound2) == 0 {
		star := "*"
		compound2 = []simpleSel{&universalSel{ns: &star}}
	}
	return compoundIsSuperselector(&compoundSel{components: compound1}, &compoundSel{components: compound2}, parents)
}

// selectorPseudoIsSuperselector reports whether pseudo1 (with a selector arg) is
// a superselector of compound2.
func selectorPseudoIsSuperselector(pseudo1 *pseudoSel, compound2 *compoundSel, parents []complexComponent) bool {
	selector1 := pseudo1.selector
	if selector1 == nil {
		panic(selErr("Selector must have a selector argument."))
	}

	switch pseudo1.normalizedName {
	case "is", "matches", "any", "where":
		selectors := selectorPseudoArgs(compound2, pseudo1.name, true)
		for _, selector2 := range selectors {
			if selector1.isSuperList(selector2) {
				return true
			}
		}
		for _, complex1 := range selector1.components {
			if len(complex1.leadingCombinators) != 0 {
				continue
			}
			target := append(append([]complexComponent{}, parents...), complexComponent{selector: compound2})
			if complexIsSuperselector(complex1.components, target) {
				return true
			}
		}
		return false

	case "has", "host", "host-context":
		for _, selector2 := range selectorPseudoArgs(compound2, pseudo1.name, true) {
			if selector1.isSuperList(selector2) {
				return true
			}
		}
		return false

	case "slotted":
		for _, selector2 := range selectorPseudoArgs(compound2, pseudo1.name, false) {
			if selector1.isSuperList(selector2) {
				return true
			}
		}
		return false

	case "not":
		for _, complex := range selector1.components {
			if complex.isBogus() {
				return false
			}
			any := false
			for _, simple2 := range compound2.components {
				lastCompound := complex.components[len(complex.components)-1].selector
				switch s2 := simple2.(type) {
				case *typeSel:
					for _, simple1 := range lastCompound.components {
						if t1, ok := simple1.(*typeSel); ok && !t1.equal(s2) {
							any = true
						}
					}
				case *idSel:
					for _, simple1 := range lastCompound.components {
						if i1, ok := simple1.(*idSel); ok && !i1.equal(s2) {
							any = true
						}
					}
				case *pseudoSel:
					if s2.selector != nil && s2.name == pseudo1.name {
						if listIsSuperselector(s2.selector.components, []*selComplex{complex}) {
							any = true
						}
					}
				}
				if any {
					break
				}
			}
			if !any {
				return false
			}
		}
		return true

	case "current":
		for _, selector2 := range selectorPseudoArgs(compound2, pseudo1.name, true) {
			if selector1.equal(selector2) {
				return true
			}
		}
		return false

	case "nth-child", "nth-last-child":
		for _, simple := range compound2.components {
			pseudo2, ok := simple.(*pseudoSel)
			if !ok || pseudo2.name != pseudo1.name {
				continue
			}
			if !strEqualPtr(pseudo2.argument, pseudo1.argument) {
				continue
			}
			if pseudo2.selector == nil {
				continue
			}
			if selector1.isSuperList(pseudo2.selector) {
				return true
			}
		}
		return false

	default:
		panic(selErr("unreachable"))
	}
}

func selectorPseudoArgs(compound *compoundSel, name string, isClass bool) []*selList {
	var out []*selList
	for _, simple := range compound.components {
		if ps, ok := simple.(*pseudoSel); ok && ps.isClass == isClass && ps.name == name && ps.selector != nil {
			out = append(out, ps.selector)
		}
	}
	return out
}

// longestCommonSubsequence ports dart's utils.dart LCS with a select callback.
func longestCommonSubsequence[T any](list1, list2 []T, sel func(a, b T) (T, bool)) []T {
	lengths := make([][]int, len(list1)+1)
	for i := range lengths {
		lengths[i] = make([]int, len(list2)+1)
	}
	type selCell struct {
		val T
		ok  bool
	}
	selections := make([][]selCell, len(list1))
	for i := range selections {
		selections[i] = make([]selCell, len(list2))
	}
	for i := 0; i < len(list1); i++ {
		for j := 0; j < len(list2); j++ {
			v, ok := sel(list1[i], list2[j])
			selections[i][j] = selCell{v, ok}
			if !ok {
				if lengths[i+1][j] > lengths[i][j+1] {
					lengths[i+1][j+1] = lengths[i+1][j]
				} else {
					lengths[i+1][j+1] = lengths[i][j+1]
				}
			} else {
				lengths[i+1][j+1] = lengths[i][j] + 1
			}
		}
	}
	var backtrack func(i, j int) []T
	backtrack = func(i, j int) []T {
		if i == -1 || j == -1 {
			return nil
		}
		s := selections[i][j]
		if s.ok {
			return append(backtrack(i-1, j-1), s.val)
		}
		if lengths[i+1][j] > lengths[i][j+1] {
			return backtrack(i, j-1)
		}
		return backtrack(i-1, j)
	}
	return backtrack(len(list1)-1, len(list2)-1)
}
