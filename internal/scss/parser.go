// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import (
	"fmt"
	"strings"
	"unicode/utf8"
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
	// mediaFeatureStop suppresses top-level `<`/`>` comparison operators while
	// parsing a media feature's operand expression, so the media-query parser
	// can treat them as range operators. It is cleared on descent into any
	// nested grouping (parens, brackets, function args, interpolation) so a
	// comparison written there (e.g. `(width < (1 < 2))`) still parses.
	mediaFeatureStop bool
	// lastWasUnicodeRange records whether the most recently parsed value element
	// was a unicode-range token (`U+A?`). A unicode range forms a token boundary,
	// so a following identifier starts a fresh space-list element even without
	// intervening whitespace (`U+A?BCDE` -> `U+A? BCDE`).
	lastWasUnicodeRange bool
	// cssFuncDepth is the nesting depth inside a plain-CSS custom function
	// (`@function --a() { … }`), where a `result:` declaration takes a verbatim
	// custom-property-style token-stream value rather than a SassScript value.
	cssFuncDepth int
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
		sb.WriteString(p.scanEscape(sb.Len() == 0))
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
			sb.WriteString(p.scanEscape(false))
		} else if isNameChar(c) {
			sb.WriteByte(p.next())
		} else {
			break
		}
	}
	return sb.String()
}

// scanEscape consumes a "\" escape and returns dart-sass's canonical rendering
// of it. dart decodes the escape to a code point and re-serializes: a valid name
// character becomes the bare character (`\r` -> `r`), a control character or a
// leading digit becomes a lowercase-hex escape with a trailing space
// (`\1` -> `\1 `), and anything else keeps a backslash before the literal
// character (`\\` -> `\\`, `\.` -> `\.`). identifierStart selects the stricter
// name-start / leading-digit rules that apply to the first character.
func (p *parser) scanEscape(identifierStart bool) string {
	p.next() // backslash
	if p.eof() || isNewlineByte(p.peek()) {
		p.fail("Expected escape sequence.")
	}
	var value rune
	if isHexByte(p.peek()) {
		var v int
		for i := 0; i < 6; i++ {
			if p.eof() || !isHexByte(p.peek()) {
				break
			}
			v = v*16 + hexDigitValue(p.next())
		}
		if !p.eof() && isWhitespaceByte(p.peek()) {
			p.next()
		}
		value = rune(v)
	} else {
		value = p.nextRune()
	}
	return canonicalEscape(value, identifierStart)
}

// nextRune consumes one UTF-8 code point at the cursor. Callers guarantee the
// cursor is not at EOF, so the decode always advances by at least one byte.
func (p *parser) nextRune() rune {
	r, size := utf8.DecodeRuneInString(p.src[p.pos:])
	p.pos += size
	return r
}

func hexDigitValue(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	default:
		return int(c-'A') + 10
	}
}

func isNameStartRune(r rune) bool {
	return r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r >= 0x80
}

func isNameRune(r rune) bool {
	return isNameStartRune(r) || (r >= '0' && r <= '9') || r == '-'
}

// canonicalEscape renders a decoded escape code point the way dart-sass does.
func canonicalEscape(value rune, identifierStart bool) string {
	if (identifierStart && isNameStartRune(value)) || (!identifierStart && isNameRune(value)) {
		return string(value)
	}
	if value <= 0x1F || value == 0x7F || (identifierStart && value >= '0' && value <= '9') {
		var sb strings.Builder
		sb.WriteByte('\\')
		if value > 0xF {
			sb.WriteByte(hexCharFor(int(value >> 4)))
		}
		sb.WriteByte(hexCharFor(int(value & 0xF)))
		sb.WriteByte(' ')
		return sb.String()
	}
	return "\\" + string(value)
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
		return p.parseVariableDecl("")
	case c == '@':
		return p.parseAtRule()
	case c == '/' && p.peekAt(1) == '*':
		return p.parseLoudComment()
	default:
		if ns, ok := p.atNamespacedVarDecl(); ok {
			p.scanIdentifier() // namespace
			p.next()           // '.'
			return p.parseVariableDecl(ns)
		}
		return p.parseDeclarationOrStyleRule()
	}
}

// atNamespacedVarDecl reports, without consuming input, whether the upcoming
// tokens form a namespaced variable assignment `ns.$var:` (a write to a module
// member). The `.$` sequence after a leading identifier is unambiguous: a
// selector cannot contain `$` and Sass has no bare expression statements.
func (p *parser) atNamespacedVarDecl() (string, bool) {
	i := p.pos
	if i >= len(p.src) || !(isNameStart(p.src[i]) || p.src[i] == '-') {
		return "", false
	}
	start := i
	for i < len(p.src) && isNameChar(p.src[i]) {
		i++
	}
	if i+1 >= len(p.src) || p.src[i] != '.' || p.src[i+1] != '$' {
		return "", false
	}
	return p.src[start:i], true
}

func (p *parser) parseLoudComment() Stmt {
	start := p.pos
	col := p.columnAt(start)
	p.pos += 2
	for !p.eof() && !(p.peek() == '*' && p.peekAt(1) == '/') {
		p.pos++
	}
	if !p.eof() {
		p.pos += 2
	}
	text := convertCommentNewlines(p.src[start:p.pos])
	return &LoudComment{Text: commentInterp(text), Col: col}
}

// convertCommentNewlines reproduces dart-sass's ScssParser._loudComment newline
// handling inside a loud comment body: everything CSS treats as a newline (bare
// CR, CR LF, and form feed) is converted to a single LF, matching the sass-spec
// `css/comment converts_newlines` cases. A CR immediately followed by LF drops
// the CR (the LF is kept); a lone CR or an FF becomes LF.
func convertCommentNewlines(text string) string {
	if !strings.ContainsAny(text, "\r\f") {
		return text
	}
	var b strings.Builder
	b.Grow(len(text))
	for i := 0; i < len(text); i++ {
		switch text[i] {
		case '\r':
			if i+1 < len(text) && text[i+1] == '\n' {
				// CR LF: drop the CR, the LF is copied on the next iteration.
				continue
			}
			b.WriteByte('\n')
		case '\f':
			b.WriteByte('\n')
		default:
			b.WriteByte(text[i])
		}
	}
	return b.String()
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

func (p *parser) parseVariableDecl(ns string) Stmt {
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
	return &VarDecl{Name: name, Namespace: ns, Value: val, Default: def, Global: glob}
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
	nameCol := p.columnAt(p.pos)
	name := p.parseInterpolatedText(func(pp *parser) bool {
		c := pp.peek()
		return c == ':' || c == '{' || c == '}' || c == ';' || c == 0
	})
	plain, isPlain := name.isPlain()
	trimmedPlain := strings.TrimSpace(plain)
	// A declaration is a custom property when the *plain prefix* of its name
	// (the leading text before any interpolation) starts with "--", exactly as
	// dart-sass keys on `name.initialPlain`. This is decided at parse time, so a
	// fully-interpolated name (`#{--x}: 1 + 2`) is a normal declaration whose
	// value is SassScript, while `--#{x}: 1 + 2` is a custom property whose value
	// is a literal token stream.
	custom := strings.HasPrefix(initialPlain(name), "--")
	// Inside a plain-CSS custom function, a plain `result:` declaration (any
	// case) carries a verbatim token-stream value, like a custom property.
	if !custom && p.cssFuncDepth > 0 && isPlain && strings.EqualFold(trimmedPlain, "result") {
		custom = true
	}
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
		return &Declaration{Name: trimInterp(name), Custom: true, RawValue: raw, NameCol: nameCol}, true
	}
	// dart-sass: a name immediately followed (no whitespace) by an interpolated
	// identifier could be a pseudo-class selector rather than a property, e.g.
	// `a:nth-child(2n)` or `a:lang(nb)`. When such a construct is then followed
	// by a block, dart forces it to be reparsed as a style rule. See
	// _declarationOrBuffer's `couldBeSelector` in stylesheet.dart.
	posAfterColon := p.pos
	p.ws()
	couldBeSelector := p.pos == posAfterColon && p.lookingAtInterpolatedIdentifier()
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
		if couldBeSelector {
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

// initialPlain returns the leading plain (non-interpolated) text of an Interp,
// which is the empty string when the name begins with interpolation.
func initialPlain(i *Interp) string {
	if len(i.Parts) > 0 {
		if s, ok := i.Parts[0].(string); ok {
			return strings.TrimLeft(s, " \t\n\r\f")
		}
	}
	return ""
}

// columnAt returns the 0-based column (in bytes, counting a tab as one column,
// matching dart-sass's SourceSpan columns) of a source position.
func (p *parser) columnAt(pos int) int {
	col := 0
	for i := pos - 1; i >= 0 && p.src[i] != '\n'; i-- {
		col++
	}
	return col
}

func isCustomWS(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f'
}

// parseCustomPropertyValue scans a custom-property (`--*`) value as a verbatim
// token stream, evaluating SassScript only inside `#{...}`. Unlike a normal
// declaration value, `//` is NOT a line comment (it is literal text) while
// `/* */` loud comments are preserved verbatim. Whitespace is folded exactly as
// dart-sass folds it in `_interpolatedDeclarationValue`: a whitespace character
// is only emitted when it is the last of a run (immediately before a
// non-whitespace character) or when it directly follows a newline, and runs of
// newlines collapse to a single line feed. Re-indentation of the collapsed
// value is performed at serialization time.
func (p *parser) parseCustomPropertyValue() *Interp {
	var parts []any
	var sb strings.Builder
	var brackets []byte // stack of expected closers
	wroteNewline := false
	flush := func() {
		if sb.Len() > 0 {
			parts = append(parts, sb.String())
			sb.Reset()
		}
	}
	for !p.eof() {
		c := p.peek()
		if len(brackets) == 0 && (c == ';' || c == '}') {
			break
		}
		switch {
		case c == '\\':
			// An escape is copied verbatim (with its escaped character) so that,
			// e.g., "\;" does not terminate the value.
			sb.WriteByte(p.next())
			if !p.eof() {
				sb.WriteByte(p.next())
			}
			wroteNewline = false
		case c == '"' || c == '\'':
			flush()
			parts = append(parts, p.quotedStringToInterp()...)
			wroteNewline = false
		case c == '/' && p.peekAt(1) == '*':
			sb.WriteByte(p.next()) // /
			sb.WriteByte(p.next()) // *
			for !p.eof() && !(p.peek() == '*' && p.peekAt(1) == '/') {
				sb.WriteByte(p.next())
			}
			if !p.eof() {
				sb.WriteByte(p.next()) // *
				sb.WriteByte(p.next()) // /
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
		case c == ' ' || c == '\t':
			if wroteNewline || !isCustomWS(p.peekAt(1)) {
				sb.WriteByte(c)
			}
			p.next()
		case c == '\n' || c == '\r' || c == '\f':
			if p.pos == 0 || !(p.src[p.pos-1] == '\n' || p.src[p.pos-1] == '\r' || p.src[p.pos-1] == '\f') {
				sb.WriteByte('\n')
			}
			p.next()
			wroteNewline = true
		case c == '(' || c == '[' || c == '{':
			sb.WriteByte(p.next())
			brackets = append(brackets, closeBracket(c))
			wroteNewline = false
		case c == ')' || c == ']' || c == '}':
			// A depth-0 '}' is already handled by the stop check above; ')'/']'
			// reaching here at depth 0 are stray but tolerated verbatim.
			if len(brackets) > 0 {
				brackets = brackets[:len(brackets)-1]
			}
			sb.WriteByte(p.next())
			wroteNewline = false
		default:
			sb.WriteByte(p.next())
			wroteNewline = false
		}
	}
	flush()
	if len(parts) == 0 {
		parts = []any{""}
	}
	return &Interp{Parts: parts}
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

// parseAtRulePrelude reads an unknown at-rule's prelude up to the top-level
// "{", ";", "}" or EOF. It behaves like parseInterpolatedText but preserves
// loud comments (`/* … */`) verbatim, matching dart-sass, which keeps loud
// comments that appear within an unknown directive's value.
func (p *parser) parseAtRulePrelude() *Interp {
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
		c := p.peek()
		if depth == 0 && (c == '{' || c == ';' || c == '}') {
			break
		}
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
			flush()
			parts = append(parts, p.quotedStringToInterp()...)
		case c == '(' || c == '[':
			depth++
			sb.WriteByte(p.next())
		case c == ')' || c == ']':
			if depth > 0 {
				depth--
			}
			sb.WriteByte(p.next())
		case c == '/' && p.peekAt(1) == '/' && depth == 0:
			for !p.eof() && p.peek() != '\n' {
				p.pos++
			}
		case c == '/' && p.peekAt(1) == '*':
			sb.WriteString(p.scanLoudComment())
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
			flush()
			parts = append(parts, p.quotedStringToInterp()...)
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

// quotedStringToInterp scans a single quoted string and returns its
// interpolation parts (surrounding quotes and escapes preserved verbatim, each
// #{…} yielded as an InterpExpr). Sass processes interpolation inside quoted
// strings in every prelude/value context, so all interpolated-text scanners
// share this rather than copying the string raw.
func (p *parser) quotedStringToInterp() []any {
	var out []any
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
			if sb.Len() > 0 {
				out = append(out, sb.String())
				sb.Reset()
			}
			p.pos += 2
			p.ws()
			e := p.parseExpression()
			p.ws()
			if p.peek() != '}' {
				p.fail("Expected \"}\".")
			}
			p.next()
			out = append(out, &InterpExpr{Expr: e})
			continue
		}
		sb.WriteByte(p.next())
		if c == q {
			break
		}
	}
	if sb.Len() > 0 {
		out = append(out, sb.String())
	}
	return out
}
