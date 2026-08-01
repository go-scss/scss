// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import "strings"

// This file ports Dart Sass's CSS media-query model (lib/src/ast/css/media_query
// .dart and lib/src/parse/media_query.dart): parsing, canonical serialization
// and the merge algorithm used to bubble and combine nested @media rules.

// mediaQuery is a single parsed media query.
type mediaQuery struct {
	modifier    *string // "not"/"only", lowercased on merge
	mtype       *string // media type ("screen", "print", ...)
	conjunction bool    // conditions joined by "and" (true) vs "or" (false)
	conditions  []string
}

func (q mediaQuery) matchesAllTypes() bool {
	return q.mtype == nil || strings.EqualFold(*q.mtype, "all")
}

func (q mediaQuery) String() string {
	var sb strings.Builder
	if q.modifier != nil {
		sb.WriteString(*q.modifier)
		sb.WriteByte(' ')
	}
	if q.mtype != nil {
		sb.WriteString(*q.mtype)
		if len(q.conditions) > 0 {
			sb.WriteString(" and ")
		}
	}
	if len(q.conditions) == 1 && strings.HasPrefix(q.conditions[0], "(not ") {
		inner := q.conditions[0]
		sb.WriteString("not ")
		sb.WriteString(inner[len("(not ") : len(inner)-1])
	} else {
		sep := " or "
		if q.conjunction {
			sep = " and "
		}
		sb.WriteString(strings.Join(q.conditions, sep))
	}
	return sb.String()
}

// --- parsing ---

// parseMediaQueryList parses a comma-separated media query list.
func parseMediaQueryList(s string) []mediaQuery {
	p := newSelParser(s, false, false)
	var queries []mediaQuery
	for {
		p.whitespace()
		queries = append(queries, p.mediaQuery())
		p.whitespace()
		if !p.scanChar(',') {
			break
		}
	}
	if !p.eof() {
		p.fail("expected no more input.")
	}
	return queries
}

func (p *selParser) mediaQuery() mediaQuery {
	if p.peek() == '(' {
		conditions := []string{p.mediaInParens()}
		p.whitespace()
		conjunction := true
		if p.scanKeyword("and") {
			p.expectWhitespaceMedia()
			conditions = append(conditions, p.mediaLogicSequence("and")...)
		} else if p.scanKeyword("or") {
			p.expectWhitespaceMedia()
			conjunction = false
			conditions = append(conditions, p.mediaLogicSequence("or")...)
		}
		return mediaQuery{conjunction: conjunction, conditions: conditions}
	}

	var modifier *string
	var mtype *string
	identifier1 := p.identifier()

	if strings.EqualFold(identifier1, "not") {
		p.expectWhitespaceMedia()
		if !p.lookingAtIdentifier() {
			return mediaQuery{conjunction: true, conditions: []string{"(not " + p.mediaInParens() + ")"}}
		}
	}

	p.whitespace()
	if !p.lookingAtIdentifier() {
		t := identifier1
		return mediaQuery{modifier: nil, mtype: &t, conjunction: true}
	}

	identifier2 := p.identifier()
	if strings.EqualFold(identifier2, "and") {
		p.expectWhitespaceMedia()
		t := identifier1
		mtype = &t
	} else {
		p.whitespace()
		m := identifier1
		modifier = &m
		t := identifier2
		mtype = &t
		if p.scanKeyword("and") {
			p.expectWhitespaceMedia()
		} else {
			return mediaQuery{modifier: modifier, mtype: mtype, conjunction: true}
		}
	}

	if p.scanKeyword("not") {
		p.expectWhitespaceMedia()
		return mediaQuery{modifier: modifier, mtype: mtype, conjunction: true,
			conditions: []string{"(not " + p.mediaInParens() + ")"}}
	}

	return mediaQuery{modifier: modifier, mtype: mtype, conjunction: true,
		conditions: p.mediaLogicSequence("and")}
}

func (p *selParser) mediaLogicSequence(operator string) []string {
	var result []string
	for {
		result = append(result, p.mediaInParens())
		p.whitespace()
		if !p.scanKeyword(operator) {
			return result
		}
		p.expectWhitespaceMedia()
	}
}

func (p *selParser) mediaInParens() string {
	p.expectChar('(')
	p.whitespace()
	if p.peek() == '(' || (p.lookingAtIdentifier() && p.matchesKeywordCS("not")) {
		// Try to parse a structured nested condition. If it doesn't cleanly
		// consume the group up to the closing ')', the resolved text came from
		// interpolation whose keyword casing/structure dart leaves opaque
		// (e.g. `(#{"(a) AnD (b)"})` -> `((a) AnD (b))`): backtrack and re-read
		// the whole parenthesized group verbatim.
		save := p.pos
		cond := p.mediaCondition()
		p.whitespace()
		if p.peek() == ')' {
			p.expectChar(')')
			return "(" + cond + ")"
		}
		p.pos = save
	}
	// The feature text is emitted verbatim. A literal `(name:value)` feature has
	// already had its single post-colon space inserted by the parse-time media
	// parser (parser_media.go), so re-splitting here would be redundant; a feature
	// whose text originates from a string or interpolation (`("min-width:0")`,
	// `(#{$bar})`) must NOT be re-spaced, matching dart-sass, which only
	// canonicalises structurally-parsed features and leaves interpolated ones as
	// written.
	result := "(" + p.declarationValue(false) + ")"
	p.expectChar(')')
	return result
}

// mediaCondition parses a nested <media-condition>, normalizing the and/or/not
// keyword casing while preserving feature text.
func (p *selParser) mediaCondition() string {
	if p.scanKeywordCS("not") {
		p.whitespace()
		return "not " + p.mediaInParens()
	}
	first := p.mediaInParens()
	p.whitespace()
	op := ""
	if p.matchesKeywordCS("and") {
		op = "and"
	} else if p.matchesKeywordCS("or") {
		op = "or"
	}
	if op == "" {
		return first
	}
	parts := []string{first}
	for p.scanKeywordCS(op) {
		p.whitespace()
		parts = append(parts, p.mediaInParens())
		p.whitespace()
	}
	return strings.Join(parts, " "+op+" ")
}

func (p *selParser) expectWhitespaceMedia() {
	if !isWhitespaceByte(p.peekAt(-1)) && p.peek() != '(' && !p.eof() {
		// dart requires whitespace/comment; be lenient but keep boundary intent.
	}
	p.whitespace()
}

// scanKeyword consumes an identifier if it case-insensitively equals text.
func (p *selParser) scanKeyword(text string) bool {
	start := p.pos
	if !p.lookingAtIdentifier() {
		return false
	}
	id := p.identifier()
	if strings.EqualFold(id, text) {
		return true
	}
	p.pos = start
	return false
}

// matchesKeywordCS and scanKeywordCS are the CASE-SENSITIVE counterparts used
// when reparsing an already-resolved media condition inside parentheses. Unlike
// the parse-time stylesheet grammar (which normalizes raw and/or/not casing into
// the query interpolation), the evaluator sees resolved text in which a raw
// keyword is already lowercase; a mixed-case keyword can therefore only have
// come from interpolation, which dart leaves opaque — so `(#{"NoT (a)"})`
// serializes as `(NoT (a))` while a raw `(NoT (a))` was normalized to `not (a)`.
func (p *selParser) matchesKeywordCS(text string) bool {
	start := p.pos
	if !p.lookingAtIdentifier() {
		return false
	}
	id := p.identifier()
	p.pos = start
	return id == text
}

func (p *selParser) scanKeywordCS(text string) bool {
	start := p.pos
	if !p.lookingAtIdentifier() {
		return false
	}
	if p.identifier() == text {
		return true
	}
	p.pos = start
	return false
}

// --- merge ---

type mediaMergeKind int

const (
	mediaMergeSuccess mediaMergeKind = iota
	mediaMergeEmpty
	mediaMergeUnrepresentable
)

func lowerPtr(s *string) *string {
	if s == nil {
		return nil
	}
	l := strings.ToLower(*s)
	return &l
}

func everyContains(sub, super []string) bool {
	set := map[string]bool{}
	for _, s := range super {
		set[s] = true
	}
	for _, s := range sub {
		if !set[s] {
			return false
		}
	}
	return true
}

// merge returns the intersection of q and other. See CssMediaQuery.merge.
func (q mediaQuery) merge(other mediaQuery) (mediaQuery, mediaMergeKind) {
	if !q.conjunction || !other.conjunction {
		return mediaQuery{}, mediaMergeUnrepresentable
	}

	ourModifier := lowerPtr(q.modifier)
	ourType := lowerPtr(q.mtype)
	theirModifier := lowerPtr(other.modifier)
	theirType := lowerPtr(other.mtype)

	if ourType == nil && theirType == nil {
		return mediaQuery{
			conjunction: true,
			conditions:  append(append([]string{}, q.conditions...), other.conditions...),
		}, mediaMergeSuccess
	}

	ourNot := ourModifier != nil && *ourModifier == "not"
	theirNot := theirModifier != nil && *theirModifier == "not"

	var modifier, mtype *string
	var conditions []string

	switch {
	case ourNot != theirNot:
		if strEqualPtr(ourType, theirType) {
			negative := q.conditions
			positive := other.conditions
			if !ourNot {
				negative, positive = other.conditions, q.conditions
			}
			if everyContains(negative, positive) {
				return mediaQuery{}, mediaMergeEmpty
			}
			return mediaQuery{}, mediaMergeUnrepresentable
		} else if q.matchesAllTypes() || other.matchesAllTypes() {
			return mediaQuery{}, mediaMergeUnrepresentable
		}
		if ourNot {
			modifier = theirModifier
			mtype = theirType
			conditions = other.conditions
		} else {
			modifier = ourModifier
			mtype = ourType
			conditions = q.conditions
		}
	case ourNot:
		if !strEqualPtr(ourType, theirType) {
			return mediaQuery{}, mediaMergeUnrepresentable
		}
		more, fewer := q.conditions, other.conditions
		if len(other.conditions) > len(q.conditions) {
			more, fewer = other.conditions, q.conditions
		}
		if everyContains(fewer, more) {
			modifier = ourModifier
			mtype = ourType
			conditions = more
		} else {
			return mediaQuery{}, mediaMergeUnrepresentable
		}
	case q.matchesAllTypes():
		modifier = theirModifier
		if other.matchesAllTypes() && ourType == nil {
			mtype = nil
		} else {
			mtype = theirType
		}
		conditions = append(append([]string{}, q.conditions...), other.conditions...)
	case other.matchesAllTypes():
		modifier = ourModifier
		mtype = ourType
		conditions = append(append([]string{}, q.conditions...), other.conditions...)
	case !strEqualPtr(ourType, theirType):
		return mediaQuery{}, mediaMergeEmpty
	default:
		if ourModifier != nil {
			modifier = ourModifier
		} else {
			modifier = theirModifier
		}
		mtype = ourType
		conditions = append(append([]string{}, q.conditions...), other.conditions...)
	}

	// Preserve original (non-lowercased) modifier/type text where it came from.
	resultType := q.mtype
	if !strEqualPtr(mtype, ourType) {
		resultType = other.mtype
	}
	resultModifier := q.modifier
	if !strEqualPtr(modifier, ourModifier) {
		resultModifier = other.modifier
	}
	if mtype == nil {
		resultType = nil
	}
	if modifier == nil {
		resultModifier = nil
	}
	return mediaQuery{modifier: resultModifier, mtype: resultType, conjunction: true, conditions: conditions}, mediaMergeSuccess
}

// mergeMediaQueryLists merges two query lists. representable is false when the
// intersection can't be represented (nil in dart); an empty result means the
// intersection is empty.
func mergeMediaQueryLists(q1, q2 []mediaQuery) (result []mediaQuery, representable bool) {
	for _, a := range q1 {
		for _, b := range q2 {
			merged, kind := a.merge(b)
			switch kind {
			case mediaMergeEmpty:
				continue
			case mediaMergeUnrepresentable:
				return nil, false
			case mediaMergeSuccess:
				result = append(result, merged)
			}
		}
	}
	return result, true
}

func mediaQueriesString(qs []mediaQuery) string {
	parts := make([]string, len(qs))
	for i, q := range qs {
		parts[i] = q.String()
	}
	return strings.Join(parts, ", ")
}

func mediaQuerySet(qs []mediaQuery) map[string]bool {
	set := map[string]bool{}
	for _, q := range qs {
		set[q.String()] = true
	}
	return set
}
