// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

// This file ports the selector predicate visitors from Dart Sass's
// lib/src/ast/selector.dart: isInvisible, isBogus, isUseless and
// containsParentSelector.

// selErr builds a selector-processing error as a *SassError so it's surfaced as
// a normal Sass error by the evaluator's and Render's panic recovery.
func selErr(msg string) *SassError { return &SassError{Msg: msg} }

// --- isInvisible ---

func (sl *selList) isInvisible() bool { return sl.invisible(true) }

func (sl *selList) invisible(includeBogus bool) bool {
	for _, cx := range sl.components {
		if !cx.invisible(includeBogus) {
			return false
		}
	}
	return true
}

func (cx *selComplex) isInvisible() bool { return cx.invisible(true) }

func (cx *selComplex) invisible(includeBogus bool) bool {
	for _, comp := range cx.components {
		if compoundInvisible(comp.selector, includeBogus) {
			return true
		}
	}
	if includeBogus && cx.isBogusOtherThanLeadingCombinator() {
		return true
	}
	return false
}

func compoundInvisible(c *compoundSel, includeBogus bool) bool {
	for _, simple := range c.components {
		if simpleInvisible(simple, includeBogus) {
			return true
		}
	}
	return false
}

func simpleInvisible(s simpleSel, includeBogus bool) bool {
	switch v := s.(type) {
	case *placeholderSel:
		return true
	case *pseudoSel:
		if v.selector == nil {
			return false
		}
		if v.name == "not" {
			return includeBogus && v.selector.isBogus()
		}
		return v.selector.invisible(includeBogus)
	}
	return false
}

// --- isBogus ---

func (sl *selList) isBogus() bool {
	for _, cx := range sl.components {
		if cx.bogus(true) {
			return true
		}
	}
	return false
}

func (cx *selComplex) isBogus() bool                           { return cx.bogus(true) }
func (cx *selComplex) isBogusOtherThanLeadingCombinator() bool { return cx.bogus(false) }

func (cx *selComplex) bogus(includeLeadingCombinator bool) bool {
	if len(cx.components) == 0 {
		return len(cx.leadingCombinators) != 0
	}
	threshold := 0
	if !includeLeadingCombinator {
		threshold = 1
	}
	if len(cx.leadingCombinators) > threshold {
		return true
	}
	if len(cx.components[len(cx.components)-1].combinators) != 0 {
		return true
	}
	for _, component := range cx.components {
		if len(component.combinators) > 1 {
			return true
		}
		if compoundBogus(component.selector, includeLeadingCombinator) {
			return true
		}
	}
	return false
}

func compoundBogus(c *compoundSel, includeLeadingCombinator bool) bool {
	for _, simple := range c.components {
		if ps, ok := simple.(*pseudoSel); ok && ps.selector != nil {
			if ps.name == "has" {
				if ps.selector.isBogusOtherThanLeadingCombinator() {
					return true
				}
			} else if ps.selector.isBogus() {
				return true
			}
		}
	}
	return false
}

func (sl *selList) isBogusOtherThanLeadingCombinator() bool {
	for _, cx := range sl.components {
		if cx.bogus(false) {
			return true
		}
	}
	return false
}

// --- isUseless ---

func (cx *selComplex) isUseless() bool {
	if len(cx.leadingCombinators) > 1 {
		return true
	}
	for _, component := range cx.components {
		if len(component.combinators) > 1 {
			return true
		}
		if compoundUseless(component.selector) {
			return true
		}
	}
	return false
}

func compoundUseless(c *compoundSel) bool {
	for _, simple := range c.components {
		if ps, ok := simple.(*pseudoSel); ok && ps.selector != nil {
			if pseudoIsBogus(ps) {
				return true
			}
		}
	}
	return false
}

// pseudoIsBogus reports whether a pseudo selector's argument selector is bogus.
func pseudoIsBogus(ps *pseudoSel) bool {
	if ps.selector == nil {
		return false
	}
	if ps.name == "has" {
		return ps.selector.isBogusOtherThanLeadingCombinator()
	}
	return ps.selector.isBogus()
}

// --- containsParentSelector ---

func selListContainsParent(sl *selList) bool {
	for _, cx := range sl.components {
		if complexContainsParent(cx) {
			return true
		}
	}
	return false
}

func complexContainsParent(cx *selComplex) bool {
	for _, comp := range cx.components {
		if compoundContainsParent(comp.selector) {
			return true
		}
	}
	return false
}

func compoundContainsParent(c *compoundSel) bool {
	for _, simple := range c.components {
		switch v := simple.(type) {
		case *parentSel:
			return true
		case *pseudoSel:
			if v.selector != nil && selListContainsParent(v.selector) {
				return true
			}
		}
	}
	return false
}

// firstParentWithSuffix returns the first parent selector with a suffix, if any.
func selListFirstParentWithSuffix(sl *selList) *parentSel {
	for _, cx := range sl.components {
		for _, comp := range cx.components {
			for _, simple := range comp.selector.components {
				switch v := simple.(type) {
				case *parentSel:
					if v.suffix != nil {
						return v
					}
				case *pseudoSel:
					if v.selector != nil {
						if p := selListFirstParentWithSuffix(v.selector); p != nil {
							return p
						}
					}
				}
			}
		}
	}
	return nil
}
