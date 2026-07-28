// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import "strings"

// parseSupports parses a @supports rule. The condition is parsed into a
// structured SupportsCond tree (mirroring dart-sass's SupportsCondition
// hierarchy) so it can be re-serialized canonically at evaluation time.
func (p *parser) parseSupports() Stmt {
	p.ws()
	cond := p.parseSupportsCondition(false)
	p.ws()
	body := p.parseBlock()
	return &Supports{Cond: cond, Body: body}
}

// parseSupportsCondition mirrors dart-sass StylesheetParser._supportsCondition.
func (p *parser) parseSupportsCondition(inParen bool) SupportsCond {
	if p.scanSupportsKeyword("not") {
		p.wsCond(inParen)
		return &SupportsNegation{Cond: p.parseSupportsConditionInParens()}
	}
	cond := p.parseSupportsConditionInParens()
	p.wsCond(inParen)
	op := ""
	for p.looksLikeIdentifier() {
		if op != "" {
			p.expectSupportsKeyword(op)
		} else if p.scanSupportsKeyword("or") {
			op = "or"
		} else {
			p.expectSupportsKeyword("and")
			op = "and"
		}
		p.wsCond(inParen)
		right := p.parseSupportsConditionInParens()
		cond = &SupportsOperation{Left: cond, Right: right, Op: op}
		p.wsCond(inParen)
	}
	return cond
}

// parseSupportsConditionInParens mirrors _supportsConditionInParens.
func (p *parser) parseSupportsConditionInParens() SupportsCond {
	if p.lookingAtInterpolatedIdentifier() {
		ident := p.interpolatedIdentifier()
		if plain, ok := ident.isPlain(); ok && strings.EqualFold(plain, "not") {
			p.fail("%q is not a valid identifier here.", plain)
		}
		if p.peek() == '(' {
			p.next()
			args := p.interpolatedDeclarationValue(true, true, true, false)
			if p.peek() != ')' {
				p.fail("Expected \")\".")
			}
			p.next()
			return &SupportsFunc{Name: ident, Args: args}
		}
		if e, ok := loneInterpExpr(ident); ok {
			return &SupportsInterp{Expr: e}
		}
		p.fail("Expected @supports condition.")
	}

	if p.peek() != '(' {
		p.fail("Expected @supports condition.")
	}
	p.next() // (
	p.ws()
	if p.scanSupportsKeyword("not") {
		p.ws()
		cond := p.parseSupportsConditionInParens()
		if p.peek() != ')' {
			p.fail("Expected \")\".")
		}
		p.next()
		return &SupportsNegation{Cond: cond}
	}
	if p.peek() == '(' {
		cond := p.parseSupportsCondition(true)
		if p.peek() != ')' {
			p.fail("Expected \")\".")
		}
		p.next()
		return cond
	}

	// A declaration `(name: value)`, an operation over an interpolation, or a raw
	// `SupportsAnything` fallback. We parse the declaration name and check for the
	// colon, backtracking to the raw fallback when there is none. A `--`-prefixed
	// name marks a custom property only when the colon is actually present.
	start := p.pos
	name, custom, ok := p.trySupportsDeclName()
	if !ok {
		p.pos = start
		ident := p.interpolatedIdentifier()
		if op, ok := p.trySupportsOperation(ident); ok {
			if p.peek() != ')' {
				p.fail("Expected \")\".")
			}
			p.next()
			return op
		}
		contents := &Interp{}
		contents.Parts = append(contents.Parts, ident.Parts...)
		rest := p.interpolatedDeclarationValue(true, true, false, false)
		contents.Parts = append(contents.Parts, rest.Parts...)
		if p.peek() == ':' {
			p.fail("Expected \")\".")
		}
		if p.peek() != ')' {
			p.fail("Expected \")\".")
		}
		p.next()
		return &SupportsAnything{Contents: mergeInterp(contents)}
	}

	if custom {
		// A custom-property value is captured raw but, unlike a function or
		// SupportsAnything, its newlines collapse to spaces (dart-sass serializes
		// it through a StringExpression rather than raw interpolation).
		raw := p.interpolatedDeclarationValue(true, false, true, true)
		if p.peek() != ')' {
			p.fail("Expected \")\".")
		}
		p.next()
		return &SupportsDecl{Name: name, RawValue: raw, Custom: true}
	}
	p.ws()
	value := p.parseExpression()
	if p.peek() != ')' {
		p.fail("Expected \")\".")
	}
	p.next()
	return &SupportsDecl{Name: name, Value: value}
}

// trySupportsDeclName parses a declaration name followed by a `:`. It returns
// ok=false (so the caller backtracks) when there is no colon. A `--`-prefixed
// name is reported as a custom property, but only once the colon confirms it is
// a declaration rather than a raw `SupportsAnything`.
func (p *parser) trySupportsDeclName() (e Expr, custom, ok bool) {
	defer func() {
		if r := recover(); r != nil {
			rethrowIfNotSass(r)
			e, custom, ok = nil, false, false
		}
	}()
	if p.looksLikeCustomProperty() {
		e = &Ident{Name: p.scanIdentifier()}
		custom = true
	} else {
		e = p.parseExpression()
	}
	p.ws()
	if p.peek() != ':' {
		return nil, false, false
	}
	p.next()
	return e, custom, true
}

// trySupportsOperation mirrors _trySupportsOperation: if a lone interpolation is
// followed by `and`/`or`, it becomes an operation; otherwise nothing is consumed.
func (p *parser) trySupportsOperation(ident *Interp) (SupportsCond, bool) {
	e, isLone := loneInterpExpr(ident)
	if !isLone {
		return nil, false
	}
	before := p.pos
	p.ws()
	var operation SupportsCond
	op := ""
	for p.looksLikeIdentifier() {
		if op != "" {
			p.expectSupportsKeyword(op)
		} else if p.scanSupportsKeyword("and") {
			op = "and"
		} else if p.scanSupportsKeyword("or") {
			op = "or"
		} else {
			p.pos = before
			return nil, false
		}
		p.ws()
		right := p.parseSupportsConditionInParens()
		var left SupportsCond
		if operation != nil {
			left = operation
		} else {
			left = &SupportsInterp{Expr: e}
		}
		operation = &SupportsOperation{Left: left, Right: right, Op: op}
		p.ws()
	}
	if operation == nil {
		p.pos = before
		return nil, false
	}
	return operation, true
}

// loneInterpExpr reports whether an interpolated identifier is exactly a single
// `#{...}` expression with no surrounding literal text.
func loneInterpExpr(i *Interp) (Expr, bool) {
	if len(i.Parts) == 1 {
		if ie, ok := i.Parts[0].(*InterpExpr); ok {
			return ie.Expr, true
		}
	}
	return nil, false
}

// looksLikeCustomProperty reports whether the scanner is at an unquoted
// identifier beginning with `--`.
func (p *parser) looksLikeCustomProperty() bool {
	return p.peek() == '-' && p.peekAt(1) == '-'
}

// lookingAtInterpolatedIdentifier mirrors _lookingAtInterpolatedIdentifier: an
// identifier that may begin with interpolation.
func (p *parser) lookingAtInterpolatedIdentifier() bool {
	c := p.peek()
	if isNameStart(c) || c == '\\' {
		return true
	}
	if c == '#' && p.peekAt(1) == '{' {
		return true
	}
	if c == '-' {
		n := p.peekAt(1)
		if n == '-' || isNameStart(n) || n == '\\' {
			return true
		}
		if n == '#' && p.peekAt(2) == '{' {
			return true
		}
	}
	return false
}

// interpolatedIdentifier reads an identifier that may contain `#{...}`
// interpolation, returning it as an Interp. It fails when not positioned at a
// valid (possibly interpolated) identifier, matching dart-sass — a lone `-` or a
// non-identifier byte such as `*` is not an identifier, so a SupportsAnything
// fallback must begin with one.
func (p *parser) interpolatedIdentifier() *Interp {
	if !p.lookingAtInterpolatedIdentifier() {
		p.fail("Expected identifier.")
	}
	var parts []any
	var sb strings.Builder
	flush := func() {
		if sb.Len() > 0 {
			parts = append(parts, sb.String())
			sb.Reset()
		}
	}
	if p.peek() == '-' {
		sb.WriteByte(p.next())
	}
	for {
		c := p.peek()
		switch {
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
		case c == '\\':
			sb.WriteString(p.scanEscape(sb.Len() == 0 && len(parts) == 0))
		case isNameChar(c):
			sb.WriteByte(p.next())
		default:
			flush()
			return &Interp{Parts: parts}
		}
	}
}

// interpolatedDeclarationValue mirrors _interpolatedDeclarationValue: it captures
// raw declaration-value text as an Interp, preserving loud comments and
// interpolation, dropping silent comments, and collapsing runs of whitespace
// (following dart-sass exactly). When collapseNL is set, newlines are treated as
// ordinary whitespace and collapse to a single space (matching how dart-sass
// serializes a custom-property @supports value through a StringExpression);
// otherwise newlines are preserved with their following indentation (as for a
// function or SupportsAnything fallback).
func (p *parser) interpolatedDeclarationValue(allowEmpty, allowSemicolon, allowColon, collapseNL bool) *Interp {
	var parts []any
	var sb strings.Builder
	flush := func() {
		if sb.Len() > 0 {
			parts = append(parts, sb.String())
			sb.Reset()
		}
	}
	var brackets []byte
	wroteNewline := false
	for {
		c := p.peek()
		switch {
		case c == '\\':
			sb.WriteString(p.scanEscape(false))
			wroteNewline = false
		case c == '"' || c == '\'':
			sb.WriteString(p.scanQuotedRaw())
			wroteNewline = false
		case c == '/' && p.peekAt(1) == '*':
			sb.WriteString(p.scanLoudComment())
			wroteNewline = false
		case c == '/' && p.peekAt(1) == '/':
			for !p.eof() && p.peek() != '\n' {
				p.pos++
			}
			wroteNewline = false
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
			wroteNewline = false
		case collapseNL && isWhitespaceByte(c) && isWhitespaceByte(p.peekAt(1)):
			p.next()
		case collapseNL && isWhitespaceByte(c):
			sb.WriteByte(' ')
			p.next()
		case (c == ' ' || c == '\t') && !wroteNewline && isWhitespaceByte(p.peekAt(1)):
			p.next()
		case c == ' ' || c == '\t':
			sb.WriteByte(p.next())
		case c == '\n' || c == '\r' || c == '\f':
			if p.pos == 0 || !isNewlineByte(p.src[p.pos-1]) {
				sb.WriteByte('\n')
			}
			p.next()
			wroteNewline = true
		case c == '(' || c == '{' || c == '[':
			brackets = append(brackets, closeBracket(c))
			sb.WriteByte(p.next())
			wroteNewline = false
		case c == ')' || c == '}' || c == ']':
			if len(brackets) == 0 {
				flush()
				return finishInterp(parts)
			}
			want := brackets[len(brackets)-1]
			if c != want {
				p.fail("Expected %q.", string(want))
			}
			brackets = brackets[:len(brackets)-1]
			sb.WriteByte(p.next())
			wroteNewline = false
		case c == ';':
			if !allowSemicolon && len(brackets) == 0 {
				flush()
				return finishInterp(parts)
			}
			sb.WriteByte(p.next())
			wroteNewline = false
		case c == ':':
			if !allowColon && len(brackets) == 0 {
				flush()
				return finishInterp(parts)
			}
			sb.WriteByte(p.next())
			wroteNewline = false
		case c == 0:
			flush()
			return finishInterp(parts)
		default:
			if p.looksLikeIdentifier() {
				sb.WriteString(p.scanIdentifier())
			} else {
				sb.WriteByte(p.next())
			}
			wroteNewline = false
		}
	}
}

// scanLoudComment copies a `/* ... */` comment verbatim, including delimiters.
func (p *parser) scanLoudComment() string {
	start := p.pos
	p.pos += 2
	for !p.eof() && !(p.peek() == '*' && p.peekAt(1) == '/') {
		p.pos++
	}
	if !p.eof() {
		p.pos += 2
	}
	return p.src[start:p.pos]
}

func finishInterp(parts []any) *Interp {
	if len(parts) == 0 {
		parts = []any{""}
	}
	return &Interp{Parts: parts}
}

// mergeInterp coalesces adjacent literal string parts so isPlain works on merged
// SupportsAnything contents.
func mergeInterp(i *Interp) *Interp {
	var out []any
	for _, part := range i.Parts {
		if s, ok := part.(string); ok && len(out) > 0 {
			if prev, ok := out[len(out)-1].(string); ok {
				out[len(out)-1] = prev + s
				continue
			}
		}
		out = append(out, part)
	}
	return finishInterp(out)
}

func closeBracket(c byte) byte {
	switch c {
	case '(':
		return ')'
	case '{':
		return '}'
	default:
		return ']'
	}
}

// wsCond consumes whitespace and comments; in a parenthesized context newlines
// are consumed too (as they always are here — kept explicit to mirror dart).
func (p *parser) wsCond(inParen bool) { p.ws() }

// scanSupportsKeyword scans a whole-word keyword (case-insensitively) at the
// current position, consuming it only on a match.
func (p *parser) scanSupportsKeyword(kw string) bool {
	if !p.looksLikeIdentifier() {
		return false
	}
	save := p.pos
	id := p.scanIdentifier()
	if strings.EqualFold(id, kw) {
		return true
	}
	p.pos = save
	return false
}

// expectSupportsKeyword scans and requires a whole-word keyword.
func (p *parser) expectSupportsKeyword(kw string) {
	if !p.scanSupportsKeyword(kw) {
		p.fail("Expected %q.", kw)
	}
}
