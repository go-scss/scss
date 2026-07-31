// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import "strings"

// selectorList is the evaluator-facing wrapper around the ported Dart Sass
// selector AST (*selList). It carries the resolved selector for a style rule and
// bridges to serialization and @extend. A nil list represents an empty selector.
type selectorList struct {
	list *selList
}

// isEmpty reports whether this selector has no complex selectors.
func (s selectorList) isEmpty() bool {
	return s.list == nil || len(s.list.components) == 0
}

// serialize renders the selector list for CSS output.
func (s selectorList) serialize(compressed bool) string {
	if s.list == nil {
		return ""
	}
	var sb strings.Builder
	s.list.write(&sb, compressed)
	return sb.String()
}

func (s selectorList) String() string { return s.serialize(false) }

// asSassList renders the selector as a SassScript value, mirroring dart-sass's
// SelectorList.asSassList: a comma-separated list whose elements are the complex
// selectors, each a space-separated list of unquoted strings for its leading
// combinators, compound selectors and trailing combinators. This is what `&`
// evaluates to in an expression context (e.g. list.length(&), @each … in &).
func (s selectorList) asSassList() Value {
	if s.list == nil {
		return sassNull
	}
	complexes := make([]Value, 0, len(s.list.components))
	for _, cx := range s.list.components {
		var parts []Value
		for _, c := range cx.leadingCombinators {
			parts = append(parts, &SassString{Text: c.String()})
		}
		for _, comp := range cx.components {
			var sb strings.Builder
			comp.selector.write(&sb, false)
			parts = append(parts, &SassString{Text: sb.String()})
			for _, c := range comp.combinators {
				parts = append(parts, &SassString{Text: c.String()})
			}
		}
		complexes = append(complexes, &List{Elements: parts, Sep: SepSpace})
	}
	return &List{Elements: complexes, Sep: SepComma}
}

// parseSelectorList parses a resolved selector string (interpolation already
// substituted) into structured form, keeping parent selectors for later
// nesting resolution.
func parseSelectorList(str string) selectorList {
	return selectorList{list: mustParseSelectorList(str)}
}

// resolveNesting resolves child's parent selectors (&) against parent, matching
// Dart Sass's SelectorList.nestWithin with implicitParent = true.
func resolveNesting(child, parent selectorList) selectorList {
	return resolveNestingImpl(child, parent, true)
}

// resolveNestingImpl resolves child's parent selectors against parent. When
// implicitParent is false (the direct children of a query-less @at-root), a
// selector without an explicit `&` is emitted as-is rather than being prefixed
// with the parent, matching dart nestWithin(implicitParent: false).
func resolveNestingImpl(child, parent selectorList, implicitParent bool) selectorList {
	if child.list == nil {
		return child
	}
	return selectorList{list: child.list.nestWithin(parent.list, implicitParent, false)}
}

// nestWithin returns a new selector list representing sl nested within parent.
// See Dart Sass lib/src/ast/selector/list.dart.
func (sl *selList) nestWithin(parent *selList, implicitParent, preserveParent bool) *selList {
	if parent == nil {
		if preserveParent {
			return sl
		}
		if p := selListFirstParentWithSuffix(sl); p != nil {
			panic(selErr("A top-level selector may not contain a parent selector with a suffix."))
		}
		return sl
	}

	var rows [][]*selComplex
	for _, complex := range sl.components {
		if preserveParent || !complexContainsParent(complex) {
			if !implicitParent {
				rows = append(rows, []*selComplex{complex})
				continue
			}
			var row []*selComplex
			for _, parentComplex := range parent.components {
				row = append(row, parentComplex.concatenate(complex, false))
			}
			rows = append(rows, row)
			continue
		}

		var newComplexes []*selComplex
		for _, component := range complex.components {
			resolved := nestWithinCompound(component, parent)
			if resolved == nil {
				if len(newComplexes) == 0 {
					newComplexes = append(newComplexes, &selComplex{
						leadingCombinators: complex.leadingCombinators,
						components:         []complexComponent{component},
					})
				} else {
					for i := range newComplexes {
						newComplexes[i] = newComplexes[i].withAdditionalComponent(component, false)
					}
				}
			} else if len(newComplexes) == 0 {
				if len(complex.leadingCombinators) == 0 {
					newComplexes = append(newComplexes, resolved...)
				} else {
					for _, rc := range resolved {
						lead := complex.leadingCombinators
						if len(rc.leadingCombinators) != 0 {
							lead = append(append([]combinator{}, complex.leadingCombinators...), rc.leadingCombinators...)
						}
						newComplexes = append(newComplexes, &selComplex{
							leadingCombinators: lead,
							components:         rc.components,
							lineBreak:          rc.lineBreak,
						})
					}
				}
			} else {
				prev := newComplexes
				newComplexes = nil
				for _, nc := range prev {
					for _, rc := range resolved {
						newComplexes = append(newComplexes, nc.concatenate(rc, false))
					}
				}
			}
		}
		rows = append(rows, newComplexes)
	}

	return &selList{components: flattenVertically(rows)}
}

// nestWithinCompound resolves parent selectors within a single component.
func nestWithinCompound(component complexComponent, parent *selList) []*selComplex {
	simples := component.selector.components
	containsSelectorPseudo := false
	for _, simple := range simples {
		if ps, ok := simple.(*pseudoSel); ok && ps.selector != nil && selListContainsParent(ps.selector) {
			containsSelectorPseudo = true
			break
		}
	}
	_, firstIsParent := simples[0].(*parentSel)
	if !containsSelectorPseudo && !firstIsParent {
		return nil
	}

	resolvedSimples := simples
	if containsSelectorPseudo {
		resolvedSimples = make([]simpleSel, len(simples))
		for i, simple := range simples {
			if ps, ok := simple.(*pseudoSel); ok && ps.selector != nil && selListContainsParent(ps.selector) {
				resolvedSimples[i] = ps.withSelector(ps.selector.nestWithin(parent, false, false))
			} else {
				resolvedSimples[i] = simple
			}
		}
	}

	parentSelector, firstIsParent := simples[0].(*parentSel)
	if !firstIsParent {
		return []*selComplex{{
			components: []complexComponent{{
				selector:    &compoundSel{components: resolvedSimples},
				combinators: component.combinators,
			}},
		}}
	} else if len(simples) == 1 && parentSelector.suffix == nil {
		return parent.withAdditionalCombinators(component.combinators).components
	}

	out := make([]*selComplex, 0, len(parent.components))
	for _, complex := range parent.components {
		lastComponent := complex.components[len(complex.components)-1]
		if len(lastComponent.combinators) != 0 {
			panic(selErr("Selector \"" + complex.String() + "\" can't be used as a parent in a compound selector."))
		}
		suffix := parentSelector.suffix
		lastSimples := lastComponent.selector.components
		var lastComponents []simpleSel
		if suffix == nil {
			lastComponents = append(append([]simpleSel{}, lastSimples...), resolvedSimples[1:]...)
		} else {
			suffixed, err := lastSimples[len(lastSimples)-1].addSuffix(*suffix)
			if err != nil {
				panic(err)
			}
			lastComponents = append(lastComponents, lastSimples[:len(lastSimples)-1]...)
			lastComponents = append(lastComponents, suffixed)
			lastComponents = append(lastComponents, resolvedSimples[1:]...)
		}
		last := &compoundSel{components: lastComponents}
		comps := append([]complexComponent{}, complex.components[:len(complex.components)-1]...)
		comps = append(comps, complexComponent{selector: last, combinators: component.combinators})
		out = append(out, &selComplex{
			leadingCombinators: complex.leadingCombinators,
			components:         comps,
			lineBreak:          complex.lineBreak,
		})
	}
	return out
}

// unify returns a selector list matching both sl and other, or nil.
func (sl *selList) unify(other *selList) *selList {
	var contents []*selComplex
	for _, complex1 := range sl.components {
		for _, complex2 := range other.components {
			if unified, ok := unifyComplex([]*selComplex{complex1, complex2}); ok {
				contents = append(contents, unified...)
			}
		}
	}
	if len(contents) == 0 {
		return nil
	}
	return &selList{components: contents}
}

func (cx *selComplex) String() string {
	var sb strings.Builder
	cx.write(&sb, false)
	return sb.String()
}

// flattenVertically interleaves rows column-first: [[1a,1b],[2a,2b]] -> 1a,2a,1b,2b.
func flattenVertically(rows [][]*selComplex) []*selComplex {
	queues := make([][]*selComplex, 0, len(rows))
	for _, r := range rows {
		if len(r) > 0 {
			queues = append(queues, r)
		}
	}
	if len(queues) == 1 {
		return queues[0]
	}
	var result []*selComplex
	for len(queues) > 0 {
		var kept [][]*selComplex
		for _, q := range queues {
			result = append(result, q[0])
			if len(q) > 1 {
				kept = append(kept, q[1:])
			}
		}
		queues = kept
	}
	return result
}

// isIdentifierString reports whether s is a valid CSS identifier (Dart Sass's
// Parser.isIdentifier), used to decide attribute-value quoting.
func isIdentifierString(s string) bool {
	if s == "" {
		return false
	}
	i := 0
	if s[i] == '-' {
		i++
		if i < len(s) && s[i] == '-' {
			return identBodyOnly(s[i+1:])
		}
	}
	if i >= len(s) {
		return false
	}
	c := s[i]
	if c == '\\' {
		return true
	}
	if !isNameStart(c) {
		return false
	}
	return identBodyOnly(s[i+1:])
}

func identBodyOnly(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\\' {
			return true
		}
		if !isNameChar(c) {
			return false
		}
	}
	return true
}
