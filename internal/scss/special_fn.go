// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import "strings"

// This file implements CSS special functions whose arguments are preserved
// verbatim rather than parsed as SassScript: url(), the vendor-prefixed and
// legacy calc()/expression()/element() forms, progid:...() and the generic
// type() function. Their bodies may contain arbitrary tokens (url(!),
// type(@#$%^&*...), url(http://a/b!c)) that ordinary argument parsing rejects;
// only #{} interpolations inside them are evaluated. This mirrors dart-sass's
// trySpecialFunction / _tryUrlContents.

// trySpecialFunction parses name(...) as a verbatim CSS special function when
// name is one of the recognised special forms, evaluating only interpolations.
// It reports whether it consumed a special function; the scanner is unchanged on
// a false return so the caller can parse an ordinary function call.
func (p *parser) trySpecialFunction(name string) (Expr, bool) {
	// The caller only invokes this at an opening "(".
	lower := strings.ToLower(name)
	if lower == "type" || lower == "attr" {
		return p.parseCalc(lower), true
	}
	norm := unvendor(lower)
	vendored := norm != lower
	switch {
	case norm == "url":
		return p.tryUrlContents()
	case norm == "expression", norm == "element":
		return p.parseCalc(lower), true
	case norm == "calc" && vendored:
		return p.parseCalc(lower), true
	}
	return nil, false
}

// tryProgid parses an IE progid:...() special function. It assumes the current
// identifier was "progid" and the scanner is at the ":".
func (p *parser) tryProgid(name string) Expr {
	p.next() // :
	var sb strings.Builder
	sb.WriteString(strings.ToLower(name))
	sb.WriteByte(':')
	for isAlpha(p.peek()) || p.peek() == '.' {
		sb.WriteByte(p.next())
	}
	if p.peek() != '(' {
		p.fail("Expected \"(\".")
	}
	return p.parseCalc(sb.String())
}

// tryUrlContents parses url(...) as a raw unquoted URL. If the contents are not
// a valid unquoted URL (e.g. they contain quotes or a nested "(") it restores
// the scanner and returns false so url() is parsed as an ordinary function.
func (p *parser) tryUrlContents() (Expr, bool) {
	save := p.pos
	p.next() // (
	p.skipURLSpace()
	var parts []any
	var sb strings.Builder
	sb.WriteString("url(")
	flush := func() {
		parts = append(parts, sb.String())
		sb.Reset()
	}
	for {
		c := p.peek()
		switch {
		case p.eof():
			p.pos = save
			return nil, false
		case c == '\\':
			sb.WriteString(p.scanEscape(false))
		case c == '#' && p.peekAt(1) == '{':
			flush()
			p.pos += 2
			p.ws()
			e := p.parseExpression()
			p.ws()
			if p.peek() != '}' {
				p.fail("Expected \"}\".")
			}
			p.next()
			parts = append(parts, &InterpExpr{Expr: e})
		case isURLChar(c):
			sb.WriteByte(p.next())
		case isSpaceByte(c):
			p.skipURLSpace()
			if p.peek() != ')' {
				p.pos = save
				return nil, false
			}
		case c == ')':
			p.next()
			sb.WriteByte(')')
			flush()
			return &StringLit{Parts: &Interp{Parts: parts}, Quoted: false}, true
		default:
			p.pos = save
			return nil, false
		}
	}
}

// skipURLSpace skips whitespace (but not comments) inside a url().
func (p *parser) skipURLSpace() {
	for isSpaceByte(p.peek()) {
		p.pos++
	}
}

// isURLChar reports whether c may appear literally in an unquoted url().
func isURLChar(c byte) bool {
	return c == '!' || c == '%' || c == '&' || c == '#' || (c >= '*' && c <= '~') || c >= 0x80
}
