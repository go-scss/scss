// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import "strings"

// walkRules visits every style rule in the tree.
func walkRules(c cssContainer, fn func(*cssStyleRule)) {
	for _, n := range c.children() {
		switch v := n.(type) {
		case *cssStyleRule:
			fn(v)
			walkRules(v, fn)
		case *cssAtRule:
			walkRules(v, fn)
		}
	}
}

func (e *evaluator) applyExtends() {
	if len(e.extends) == 0 {
		return
	}
	// Iterate to a fixpoint (bounded) so transitive extends resolve.
	for iter := 0; iter < len(e.extends)+2; iter++ {
		changed := false
		walkRules(e.root, func(r *cssStyleRule) {
			for ei := len(e.extends) - 1; ei >= 0; ei-- {
				ex := e.extends[ei]
				var additions []complexSelector
				for _, cx := range r.selector.complexes {
					for _, extCx := range ex.extenders.complexes {
						if nc, ok := extendComplex(cx, ex.target, extCx); ok {
							if !containsComplex(r.selector.complexes, nc) && !containsComplex(additions, nc) {
								additions = append(additions, nc)
							}
						}
					}
				}
				if len(additions) > 0 {
					r.selector.complexes = append(r.selector.complexes, additions...)
					changed = true
				}
			}
		})
		if !changed {
			break
		}
	}
}

func containsComplex(list []complexSelector, c complexSelector) bool {
	cs := c.serialize(false)
	for _, x := range list {
		if x.serialize(false) == cs {
			return true
		}
	}
	return false
}

// extendComplex returns cx with target replaced by the extender ext, if present.
func extendComplex(cx complexSelector, target string, ext complexSelector) (complexSelector, bool) {
	for i, part := range cx.parts {
		before, after, ok := compoundSplit(part.compound, target)
		if !ok {
			continue
		}
		var np complexSelector
		np.parts = append(np.parts, cx.parts[:i]...)
		for j, ep := range ext.parts {
			comb := ep.combinator
			comp := ep.compound
			if j == 0 {
				comb = part.combinator
				comp = before + comp
			}
			if j == len(ext.parts)-1 {
				comp = comp + after
			}
			np.parts = append(np.parts, component{combinator: comb, compound: comp})
		}
		np.parts = append(np.parts, cx.parts[i+1:]...)
		return np, true
	}
	return complexSelector{}, false
}

// compoundSplit finds target as a discrete unit in compound, returning the text
// before and after it. ok is false when target is not present as a unit.
func compoundSplit(compound, target string) (before, after string, ok bool) {
	from := 0
	for {
		idx := strings.Index(compound[from:], target)
		if idx < 0 {
			return "", "", false
		}
		idx += from
		end := idx + len(target)
		nextOK := end >= len(compound) || !isNameChar(compound[end])
		prevOK := idx == 0 || isBoundaryBefore(compound[idx-1], target[0])
		if nextOK && prevOK {
			return compound[:idx], compound[end:], true
		}
		from = idx + 1
	}
}

func isBoundaryBefore(prev byte, targetFirst byte) bool {
	// For class/id/placeholder targets, any preceding char is a boundary.
	if targetFirst == '.' || targetFirst == '#' || targetFirst == '%' || targetFirst == '[' || targetFirst == ':' {
		return true
	}
	// For element targets, previous char must be a combinator/space boundary.
	return !isNameChar(prev) && prev != '.' && prev != '#' && prev != '%'
}

// prunePlaceholders removes selectors that reference placeholder classes.
func (e *evaluator) prunePlaceholders(c cssContainer) {
	for _, n := range c.children() {
		switch v := n.(type) {
		case *cssStyleRule:
			var kept []complexSelector
			for _, cx := range v.selector.complexes {
				if !complexHasPlaceholder(cx) {
					kept = append(kept, cx)
				}
			}
			v.selector.complexes = kept
			e.prunePlaceholders(v)
		case *cssAtRule:
			e.prunePlaceholders(v)
		}
	}
}

func complexHasPlaceholder(cx complexSelector) bool {
	for _, p := range cx.parts {
		if strings.Contains(p.compound, "%") {
			return true
		}
	}
	return false
}

// normalizeMediaQuery reproduces dart-sass media-query spacing.
func normalizeMediaQuery(q string) string {
	q = strings.TrimSpace(q)
	var sb strings.Builder
	for i := 0; i < len(q); i++ {
		c := q[i]
		if c == ':' {
			sb.WriteByte(':')
			// ensure single space after colon inside feature
			for i+1 < len(q) && q[i+1] == ' ' {
				i++
			}
			if i+1 < len(q) && q[i+1] != ')' {
				sb.WriteByte(' ')
			}
			continue
		}
		if c == ' ' {
			sb.WriteByte(' ')
			for i+1 < len(q) && q[i+1] == ' ' {
				i++
			}
			continue
		}
		sb.WriteByte(c)
	}
	return sb.String()
}
