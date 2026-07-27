// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import "strings"

// component is one compound selector plus the combinator that precedes it.
type component struct {
	combinator string // "", ">", "+", "~"
	compound   string
}

// complexSelector is a sequence of components.
type complexSelector struct{ parts []component }

// selectorList is a comma-separated list of complex selectors.
type selectorList struct{ complexes []complexSelector }

// parseSelectorList parses a resolved selector string into structured form.
func parseSelectorList(s string) selectorList {
	var out selectorList
	for _, part := range splitTopLevel(s, ',') {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out.complexes = append(out.complexes, parseComplex(part))
	}
	return out
}

func parseComplex(s string) complexSelector {
	var cs complexSelector
	i := 0
	n := len(s)
	pendingComb := ""
	for i < n {
		// skip whitespace
		for i < n && isSpaceByte(s[i]) {
			i++
		}
		if i >= n {
			break
		}
		c := s[i]
		if c == '>' || c == '+' || c == '~' {
			pendingComb = string(c)
			i++
			continue
		}
		// read a compound (until whitespace or top-level combinator)
		start := i
		depth := 0
		for i < n {
			ch := s[i]
			if ch == '[' || ch == '(' {
				depth++
			} else if ch == ']' || ch == ')' {
				depth--
			} else if depth == 0 && (isSpaceByte(ch) || ch == '>' || ch == '+' || ch == '~') {
				break
			}
			i++
		}
		cs.parts = append(cs.parts, component{combinator: pendingComb, compound: s[start:i]})
		pendingComb = ""
	}
	return cs
}

// splitTopLevel splits s on sep at bracket/paren depth zero.
func splitTopLevel(s string, sep byte) []string {
	var out []string
	depth := 0
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '[', '(':
			depth++
		case ']', ')':
			depth--
		case sep:
			if depth == 0 {
				out = append(out, s[start:i])
				start = i + 1
			}
		}
	}
	out = append(out, s[start:])
	return out
}

func (cs complexSelector) hasParent() bool {
	for _, p := range cs.parts {
		if strings.Contains(p.compound, "&") {
			return true
		}
	}
	return false
}

// resolveNesting combines a child selector list with the parent selector list.
func resolveNesting(child, parent selectorList) selectorList {
	if len(parent.complexes) == 0 {
		return child
	}
	var out selectorList
	for _, childCx := range child.complexes {
		if childCx.hasParent() {
			for _, parentCx := range parent.complexes {
				out.complexes = append(out.complexes, spliceParent(childCx, parentCx))
			}
		} else {
			for _, parentCx := range parent.complexes {
				merged := complexSelector{}
				merged.parts = append(merged.parts, parentCx.parts...)
				merged.parts = append(merged.parts, childCx.parts...)
				out.complexes = append(out.complexes, merged)
			}
		}
	}
	return out
}

// spliceParent replaces every "&" in childCx with parentCx.
func spliceParent(childCx, parentCx complexSelector) complexSelector {
	var result complexSelector
	for _, part := range childCx.parts {
		if !strings.Contains(part.compound, "&") {
			result.parts = append(result.parts, part)
			continue
		}
		before, after := splitAmp(part.compound)
		// parent components: first inherits this part's combinator; last gets suffix.
		for i, pp := range parentCx.parts {
			comb := pp.combinator
			compound := pp.compound
			if i == 0 {
				comb = part.combinator
				compound = before + compound
			}
			if i == len(parentCx.parts)-1 {
				compound = compound + after
			}
			result.parts = append(result.parts, component{combinator: comb, compound: compound})
		}
	}
	return result
}

// splitAmp splits a compound at its first "&" into prefix/suffix.
func splitAmp(compound string) (before, after string) {
	idx := strings.Index(compound, "&")
	return compound[:idx], compound[idx+1:]
}

// serialize renders the selector list for output.
func (sl selectorList) serialize(compressed bool) string {
	parts := make([]string, len(sl.complexes))
	for i, cx := range sl.complexes {
		parts[i] = cx.serialize(compressed)
	}
	sep := ", "
	if compressed {
		sep = ","
	}
	return strings.Join(parts, sep)
}

func (cx complexSelector) serialize(compressed bool) string {
	var sb strings.Builder
	for i, p := range cx.parts {
		if i > 0 {
			if p.combinator != "" {
				if compressed {
					sb.WriteString(p.combinator)
				} else {
					sb.WriteString(" ")
					sb.WriteString(p.combinator)
					sb.WriteString(" ")
				}
			} else {
				sb.WriteString(" ")
			}
		} else if p.combinator != "" {
			// leading combinator (rare at top level)
			sb.WriteString(p.combinator)
			if !compressed {
				sb.WriteString(" ")
			}
		}
		sb.WriteString(p.compound)
	}
	return sb.String()
}

func (sl selectorList) String() string { return sl.serialize(false) }
