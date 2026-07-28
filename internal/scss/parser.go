// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import (
	"fmt"
	"strings"
)

// SassError is a compilation error with a message.
type SassError struct {
	Msg  string
	Line int
	Col  int
}

func (e *SassError) Error() string { return e.Msg }

type parser struct {
	src    string
	pos    int
	indent bool // indented (.sass) syntax already converted to braces
	arith  int  // nesting depth inside grouping parens (where "/" divides)
}

func newParser(src string) *parser {
	return &parser{src: src}
}

// parseStylesheet parses the whole source into a list of statements.
func parseStylesheet(src string) (stmts []Stmt, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = rethrowIfNotSass(r)
		}
	}()
	p := newParser(src)
	stmts = p.parseStatements(true)
	return stmts, nil
}

// rethrowIfNotSass classifies a recovered panic value: a *SassError is returned
// so the caller can surface it as an error, while any other value (a genuine
// runtime bug) is re-panicked. Centralising this keeps the recover guards in
// Render, parseStylesheet and tryDeclaration identical and testable.
func rethrowIfNotSass(r any) *SassError {
	if se, ok := r.(*SassError); ok {
		return se
	}
	panic(r)
}

func (p *parser) fail(format string, args ...any) {
	line := 1
	col := 1
	for i := 0; i < p.pos && i < len(p.src); i++ {
		if p.src[i] == '\n' {
			line++
			col = 1
		} else {
			col++
		}
	}
	panic(&SassError{Msg: fmt.Sprintf(format, args...), Line: line, Col: col})
}

// --- low-level scanning ---

func (p *parser) eof() bool { return p.pos >= len(p.src) }

func (p *parser) peek() byte {
	if p.pos >= len(p.src) {
		return 0
	}
	return p.src[p.pos]
}

func (p *parser) peekAt(n int) byte {
	if p.pos+n >= len(p.src) {
		return 0
	}
	return p.src[p.pos+n]
}

func (p *parser) next() byte {
	c := p.src[p.pos]
	p.pos++
	return c
}

func (p *parser) match(s string) bool {
	if strings.HasPrefix(p.src[p.pos:], s) {
		p.pos += len(s)
		return true
	}
	return false
}

// wsKeepLoud skips whitespace and silent (//) comments but stops at a loud
// (/* */) comment so it can be preserved in the output.
func (p *parser) wsKeepLoud() {
	for !p.eof() {
		c := p.peek()
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f':
			p.pos++
		case c == '/' && p.peekAt(1) == '/':
			for !p.eof() && p.peek() != '\n' {
				p.pos++
			}
		default:
			return
		}
	}
}

// ws skips whitespace and comments. Returns true if anything was skipped.
func (p *parser) ws() bool {
	start := p.pos
	for !p.eof() {
		c := p.peek()
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f':
			p.pos++
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
		default:
			return p.pos != start
		}
	}
	return p.pos != start
}

func isNameStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c >= 0x80
}

func isNameChar(c byte) bool {
	return isNameStart(c) || (c >= '0' && c <= '9') || c == '-'
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// scanIdentifier reads a CSS identifier (with escapes and leading -/--).
func (p *parser) scanIdentifier() string {
	var sb strings.Builder
	for p.peek() == '-' {
		sb.WriteByte(p.next())
	}
	if p.peek() == '\\' {
		sb.WriteString(p.scanEscape())
	} else if isNameStart(p.peek()) || isDigit(p.peek()) && sb.Len() > 0 {
		sb.WriteByte(p.next())
	} else if p.eof() || !isNameStart(p.peek()) {
		if sb.Len() == 0 {
			p.fail("Expected identifier.")
		}
	}
	for !p.eof() {
		c := p.peek()
		if c == '\\' {
			sb.WriteString(p.scanEscape())
		} else if isNameChar(c) {
			sb.WriteByte(p.next())
		} else {
			break
		}
	}
	return sb.String()
}

func (p *parser) scanEscape() string {
	// Preserve escape verbatim (sufficient for round-tripping identifiers).
	var sb strings.Builder
	sb.WriteByte(p.next()) // backslash
	if !p.eof() {
		sb.WriteByte(p.next())
	}
	return sb.String()
}

// looksLikeIdentifier reports whether an identifier starts at the current pos.
func (p *parser) looksLikeIdentifier() bool {
	c := p.peek()
	if isNameStart(c) || c == '\\' {
		return true
	}
	if c == '-' {
		n := p.peekAt(1)
		return n == '-' || isNameStart(n) || n == '\\'
	}
	return false
}

// --- statements ---

func (p *parser) parseStatements(top bool) []Stmt {
	var stmts []Stmt
	for {
		p.wsKeepLoud()
		if p.eof() {
			if !top {
				p.fail("Expected \"}\".")
			}
			return stmts
		}
		if p.peek() == '}' {
			if top {
				p.fail("unexpected \"}\".")
			}
			return stmts
		}
		if p.peek() == ';' {
			p.next() // skip empty statement
			continue
		}
		s := p.parseStatement()
		if s != nil {
			stmts = append(stmts, s)
		}
	}
}

func (p *parser) parseStatement() Stmt {
	c := p.peek()
	switch {
	case c == '$':
		return p.parseVariableDecl()
	case c == '@':
		return p.parseAtRule()
	case c == '/' && p.peekAt(1) == '*':
		return p.parseLoudComment()
	default:
		return p.parseDeclarationOrStyleRule()
	}
}

func (p *parser) parseLoudComment() Stmt {
	start := p.pos
	p.pos += 2
	for !p.eof() && !(p.peek() == '*' && p.peekAt(1) == '/') {
		p.pos++
	}
	if !p.eof() {
		p.pos += 2
	}
	text := p.src[start:p.pos]
	return &LoudComment{Text: commentInterp(text)}
}

// commentInterp preserves a loud comment verbatim, expanding only #{...}.
func commentInterp(text string) *Interp {
	var parts []any
	var sb strings.Builder
	for i := 0; i < len(text); i++ {
		if text[i] == '#' && i+1 < len(text) && text[i+1] == '{' {
			if sb.Len() > 0 {
				parts = append(parts, sb.String())
				sb.Reset()
			}
			depth := 1
			j := i + 2
			for j < len(text) && depth > 0 {
				if text[j] == '{' {
					depth++
				} else if text[j] == '}' {
					depth--
					if depth == 0 {
						break
					}
				}
				j++
			}
			sub := newParser(text[i+2 : j])
			parts = append(parts, &InterpExpr{Expr: sub.parseExpression()})
			i = j
			continue
		}
		sb.WriteByte(text[i])
	}
	if sb.Len() > 0 {
		parts = append(parts, sb.String())
	}
	if len(parts) == 0 {
		parts = []any{""}
	}
	return &Interp{Parts: parts}
}

func (p *parser) parseVariableDecl() Stmt {
	p.next() // $
	name := p.scanIdentifier()
	p.ws()
	if p.peek() != ':' {
		p.fail("Expected \":\".")
	}
	p.next()
	p.ws()
	val := p.parseExpression()
	def := false
	glob := false
	for {
		p.ws()
		if p.match("!default") {
			def = true
		} else if p.match("!global") {
			glob = true
		} else {
			break
		}
	}
	p.consumeStatementEnd()
	return &VarDecl{Name: name, Value: val, Default: def, Global: glob}
}

func (p *parser) consumeStatementEnd() {
	p.ws()
	if p.eof() || p.peek() == '}' {
		return
	}
	if p.peek() == ';' {
		p.next()
		return
	}
	p.fail("Expected \";\".")
}

func (p *parser) parseDeclarationOrStyleRule() Stmt {
	start := p.pos
	// Try declaration first; on failure, backtrack to style rule.
	if decl, ok := p.tryDeclaration(); ok {
		return decl
	}
	p.pos = start
	return p.parseStyleRule()
}

// tryDeclaration attempts to parse a property declaration. It returns ok=false
// (leaving pos unspecified) if the construct is actually a style rule.
func (p *parser) tryDeclaration() (stmt Stmt, ok bool) {
	defer func() {
		if r := recover(); r != nil {
			rethrowIfNotSass(r)
			stmt, ok = nil, false
		}
	}()
	name := p.parseInterpolatedText(func(pp *parser) bool {
		c := pp.peek()
		return c == ':' || c == '{' || c == '}' || c == ';' || c == 0
	})
	plain, isPlain := name.isPlain()
	trimmedPlain := strings.TrimSpace(plain)
	custom := isPlain && strings.HasPrefix(trimmedPlain, "--")
	// A plain name that is empty (leading `:`/`::` pseudo) or that starts with a
	// character that can't begin a CSS property is actually a selector.
	if isPlain && !custom {
		if trimmedPlain == "" || isSelectorLeadByte(trimmedPlain[0]) {
			return nil, false
		}
	}
	if p.peek() != ':' {
		return nil, false
	}
	p.next() // :
	if custom {
		raw := p.parseCustomPropertyValue()
		p.consumeStatementEnd()
		return &Declaration{Name: trimInterp(name), Custom: true, RawValue: raw}, true
	}
	p.ws()
	if p.peek() == '{' {
		// nested declaration namespace, no value
		body := p.parseBlock()
		return &Declaration{Name: trimInterp(name), Body: body}, true
	}
	val := p.parseExpression()
	p.ws()
	switch p.peek() {
	case ';', '}', 0:
		p.consumeStatementEnd()
		return &Declaration{Name: trimInterp(name), Value: val}, true
	case '{':
		if selectorLike(val) {
			return nil, false
		}
		body := p.parseBlock()
		return &Declaration{Name: trimInterp(name), Value: val, Body: body}, true
	default:
		return nil, false
	}
}

// isSelectorLeadByte reports whether c can begin a selector but not a CSS
// property name, so a statement starting with it must be a style rule.
func isSelectorLeadByte(c byte) bool {
	switch c {
	case '.', '%', '[', '*', '&', '>', '+', '~', ':':
		return true
	}
	return false
}

// selectorLike reports whether a parsed value is better understood as part of a
// selector (identifiers/combinators only, no numbers/strings/colors/operators).
func selectorLike(e Expr) bool {
	switch v := e.(type) {
	case *Ident:
		return true
	case *ListExpr:
		if v.Sep == SepComma {
			return false
		}
		for _, el := range v.Elements {
			if !selectorLike(el) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func (p *parser) parseCustomPropertyValue() *Interp {
	return p.parseInterpolatedText(func(pp *parser) bool {
		c := pp.peek()
		return c == ';' || c == '}' || c == 0
	})
}

func (p *parser) parseStyleRule() Stmt {
	sel := p.parseInterpolatedText(func(pp *parser) bool {
		return pp.peek() == '{' || pp.peek() == '}' || pp.peek() == ';' || pp.peek() == 0
	})
	if p.peek() != '{' {
		p.fail("Expected \"{\".")
	}
	body := p.parseBlock()
	return &StyleRule{Selector: trimInterp(sel), Body: body}
}

func (p *parser) parseBlock() []Stmt {
	p.ws()
	if p.peek() != '{' {
		p.fail("Expected \"{\".")
	}
	p.next()
	stmts := p.parseStatements(false)
	// parseStatements(false) only returns on '}' (it fails on EOF), so the closing
	// brace is guaranteed here.
	p.next()
	return stmts
}

func trimInterp(i *Interp) *Interp {
	// trim leading/trailing whitespace on plain string parts
	out := &Interp{}
	parts := append([]any(nil), i.Parts...)
	for len(parts) > 0 {
		if s, ok := parts[0].(string); ok {
			parts[0] = strings.TrimLeft(s, " \t\n\r\f")
			if parts[0].(string) == "" {
				parts = parts[1:]
				continue
			}
		}
		break
	}
	for len(parts) > 0 {
		last := len(parts) - 1
		if s, ok := parts[last].(string); ok {
			parts[last] = strings.TrimRight(s, " \t\n\r\f")
			if parts[last].(string) == "" {
				parts = parts[:last]
				continue
			}
		}
		break
	}
	out.Parts = parts
	return out
}

// parseInterpolatedText reads text (as an Interp) up to a stop predicate,
// honoring strings, brackets, and #{...} interpolation.
func (p *parser) parseInterpolatedText(stop func(*parser) bool) *Interp {
	var parts []any
	var sb strings.Builder
	depth := 0
	flush := func() {
		if sb.Len() > 0 {
			parts = append(parts, sb.String())
			sb.Reset()
		}
	}
	for !p.eof() {
		if depth == 0 && stop(p) {
			break
		}
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
		case c == '"' || c == '\'':
			sb.WriteString(p.scanQuotedRaw())
		case c == '(' || c == '[':
			depth++
			sb.WriteByte(p.next())
		case c == ')' || c == ']':
			if depth > 0 {
				depth--
			}
			sb.WriteByte(p.next())
		case c == '/' && p.peekAt(1) == '/' && depth == 0:
			// line comment ends the run's trailing content
			for !p.eof() && p.peek() != '\n' {
				p.pos++
			}
		case c == '/' && p.peekAt(1) == '*':
			// skip block comment inside prelude
			p.pos += 2
			for !p.eof() && !(p.peek() == '*' && p.peekAt(1) == '/') {
				p.pos++
			}
			if !p.eof() {
				p.pos += 2
			}
			sb.WriteByte(' ')
		default:
			sb.WriteByte(p.next())
		}
	}
	flush()
	if len(parts) == 0 {
		parts = []any{""}
	}
	return &Interp{Parts: parts}
}

// scanQuotedRaw copies a quoted string verbatim (including quotes).
func (p *parser) scanQuotedRaw() string {
	var sb strings.Builder
	q := p.next()
	sb.WriteByte(q)
	for !p.eof() {
		c := p.peek()
		if c == '\\' {
			sb.WriteByte(p.next())
			if !p.eof() {
				sb.WriteByte(p.next())
			}
			continue
		}
		if c == '#' && p.peekAt(1) == '{' {
			// keep interpolation raw
			sb.WriteByte(p.next())
			sb.WriteByte(p.next())
			d := 1
			for !p.eof() && d > 0 {
				cc := p.next()
				if cc == '{' {
					d++
				} else if cc == '}' {
					d--
				}
				sb.WriteByte(cc)
			}
			continue
		}
		sb.WriteByte(p.next())
		if c == q {
			break
		}
	}
	return sb.String()
}
