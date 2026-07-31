// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import "strings"

// This file ports Dart Sass's parse-time media-query model (the
// `_mediaQueryList`/`_mediaQuery`/`_mediaInParens`/`_mediaLogicSequence` group in
// lib/src/parse/stylesheet.dart). A `@media` prelude is parsed into an
// interpolation that embeds SassScript expressions for feature values and range
// bounds, rather than being captured as opaque text. The interpolation is
// resolved at evaluation time (feature expressions evaluated with dart's exact
// serialization) and the resulting plain text is then reparsed by the canonical
// media-query model (media.go) for merging and final serialization.

// interpBuf accumulates the parts of an Interp under construction: literal text
// runs coalesce into a single string, while embedded expressions (from feature
// values or `#{}` interpolation) become *InterpExpr parts.
type interpBuf struct {
	parts []any
	sb    strings.Builder
}

func (b *interpBuf) flush() {
	if b.sb.Len() > 0 {
		b.parts = append(b.parts, b.sb.String())
		b.sb.Reset()
	}
}

func (b *interpBuf) writeByte(c byte)     { b.sb.WriteByte(c) }
func (b *interpBuf) writeString(s string) { b.sb.WriteString(s) }

func (b *interpBuf) addExpr(e Expr) {
	b.flush()
	b.parts = append(b.parts, &InterpExpr{Expr: e})
}

func (b *interpBuf) done() *Interp {
	b.flush()
	if len(b.parts) == 0 {
		return &Interp{Parts: []any{""}}
	}
	return &Interp{Parts: b.parts}
}

// readInterpExpr consumes a `#{ … }` interpolation at the cursor and returns its
// expression.
func (p *parser) readInterpExpr() Expr {
	p.pos += 2 // #{
	p.ws()
	e := p.parseExpression()
	p.ws()
	if p.peek() != '}' {
		p.fail("Expected \"}\".")
	}
	p.next()
	return e
}

// parseMediaQueryInterp parses a media-query list (up to the stop predicate)
// into an interpolation with embedded SassScript. Outside of media features the
// prelude — media types, modifiers, top-level and/or/not keywords and
// interpolation — is copied verbatim (its casing and whitespace are normalized
// later by the canonical model); inside a feature's parentheses the value and
// range bounds are parsed as SassScript expressions.
func (p *parser) parseMediaQueryInterp(stop func(*parser) bool) *Interp {
	b := &interpBuf{}
	for !p.eof() {
		if stop(p) {
			break
		}
		c := p.peek()
		switch {
		case c == '#' && p.peekAt(1) == '{':
			b.addExpr(p.readInterpExpr())
		case c == '(':
			p.mediaInParens(b)
		case c == '/' && p.peekAt(1) == '/':
			for !p.eof() && p.peek() != '\n' {
				p.pos++
			}
		case c == '/' && p.peekAt(1) == '*':
			p.pos += 2
			for !p.eof() && !(p.peek() == '*' && p.peekAt(1) == '/') {
				p.pos++
			}
			if !p.eof() {
				p.pos += 2
			}
			b.writeByte(' ')
		default:
			b.writeByte(p.next())
		}
	}
	return trimInterp(b.done())
}

// mediaInParens parses a parenthesized media condition/feature at the cursor
// (the cursor is on the opening `(`). It mirrors dart's `_mediaInParens`: a
// nested condition, a `not (...)`, or a feature that is a bare name, a
// `name: value` pair, or a range comparison.
func (p *parser) mediaInParens(b *interpBuf) {
	p.next() // (
	b.writeByte('(')
	p.ws()
	switch {
	case p.peek() == '(':
		p.mediaInParens(b)
		p.ws()
		p.mediaLogicTail(b)
	case p.matchMediaKeyword("not"):
		// A raw `not` condition-negation is normalized to lowercase (dart), so
		// that interpolation-origin `NoT` — which is opaque and reaches the
		// evaluator unnormalized — is the only mixed-case form preserved.
		b.writeString("not ")
		p.ws()
		p.mediaOrInterp(b)
	default:
		b.addExpr(p.parseExprUntilComparison())
		p.ws()
		if p.peek() == ':' {
			p.next()
			p.ws()
			b.writeString(": ")
			b.addExpr(p.parseExpression())
		} else {
			p.mediaRange(b)
		}
	}
	p.ws()
	if p.peek() != ')' {
		p.fail("expected \")\".")
	}
	p.next()
	b.writeByte(')')
}

// mediaLogicTail parses an `and`/`or` sequence following a nested condition.
func (p *parser) mediaLogicTail(b *interpBuf) {
	switch {
	case p.matchMediaKeyword("and"):
		b.writeString(" and ")
		p.ws()
		p.mediaLogicSequence(b, "and")
	case p.matchMediaKeyword("or"):
		b.writeString(" or ")
		p.ws()
		p.mediaLogicSequence(b, "or")
	}
}

// mediaLogicSequence parses a sequence of conditions joined by operator.
func (p *parser) mediaLogicSequence(b *interpBuf, operator string) {
	for {
		p.mediaOrInterp(b)
		p.ws()
		if !p.matchMediaKeyword(operator) {
			return
		}
		b.writeString(" " + operator + " ")
		p.ws()
	}
}

// mediaOrInterp parses either a `#{}` interpolation or a parenthesized
// condition (dart's `_mediaOrInterp`).
func (p *parser) mediaOrInterp(b *interpBuf) {
	if p.peek() == '#' && p.peekAt(1) == '{' {
		b.addExpr(p.readInterpExpr())
	} else {
		p.mediaInParens(b)
	}
}

// mediaRange parses the range-comparison tail of a media feature (dart's range
// handling in `_mediaInParens`): one or two comparison operators, each followed
// by a SassScript operand. If the cursor is not on a comparison operator (a bare
// boolean feature such as `(hover)`), it is a no-op.
func (p *parser) mediaRange(b *interpBuf) {
	next := p.peek()
	if next != '<' && next != '>' && next != '=' {
		return
	}
	p.next()
	b.writeByte(' ')
	b.writeByte(next)
	if (next == '<' || next == '>') && p.peek() == '=' {
		p.next()
		b.writeByte('=')
	}
	b.writeByte(' ')
	p.ws()
	b.addExpr(p.parseExprUntilComparison())
	p.ws()
	if (next == '<' || next == '>') && p.peek() == next {
		p.next()
		b.writeByte(' ')
		b.writeByte(next)
		if p.peek() == '=' {
			p.next()
			b.writeByte('=')
		}
		b.writeByte(' ')
		p.ws()
		b.addExpr(p.parseExprUntilComparison())
	}
}

// parseExprUntilComparison parses a SassScript expression whose top-level
// `<`/`>` comparison operators are left for the media-query range parser
// (dart's `_expressionUntilComparison`).
func (p *parser) parseExprUntilComparison() Expr {
	saved := p.mediaFeatureStop
	p.mediaFeatureStop = true
	e := p.parseExpression()
	p.mediaFeatureStop = saved
	return e
}

// matchMediaKeyword consumes the identifier at the cursor when it equals text
// case-insensitively, and does not consume anything otherwise.
func (p *parser) matchMediaKeyword(text string) bool {
	if !p.looksLikeIdentifier() {
		return false
	}
	save := p.pos
	if strings.EqualFold(p.scanIdentifier(), text) {
		return true
	}
	p.pos = save
	return false
}
