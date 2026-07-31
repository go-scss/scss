// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import (
	"strings"
	"unicode/utf8"
)

// This file ports Dart Sass's lib/src/parse/selector.dart: a recursive-descent
// parser producing the selector AST in sel_ast.go. It supports the full CSS
// selector grammar plus Sass parent (&) and placeholder (%) selectors.

type selParser struct {
	src         string
	pos         int
	allowParent bool
	plainCss    bool
}

func newSelParser(src string, allowParent, plainCss bool) *selParser {
	return &selParser{src: src, allowParent: allowParent, plainCss: plainCss}
}

func (p *selParser) fail(msg string) { panic(selErr(msg)) }

func (p *selParser) eof() bool { return p.pos >= len(p.src) }

// peekAt returns the byte at offset from the current position, or 0 at EOF.
func (p *selParser) peekAt(off int) byte {
	i := p.pos + off
	if i < 0 || i >= len(p.src) {
		return 0
	}
	return p.src[i]
}

func (p *selParser) peek() byte { return p.peekAt(0) }

func (p *selParser) readChar() byte {
	c := p.src[p.pos]
	p.pos++
	return c
}

func (p *selParser) scanChar(c byte) bool {
	if p.peek() == c {
		p.pos++
		return true
	}
	return false
}

func (p *selParser) expectChar(c byte) {
	if !p.scanChar(c) {
		p.fail("expected \"" + string(c) + "\".")
	}
}

func (p *selParser) whitespace() {
	for !p.eof() {
		c := p.peek()
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' {
			p.pos++
		} else if c == '/' && p.peekAt(1) == '*' {
			p.pos += 2
			for !p.eof() && !(p.peek() == '*' && p.peekAt(1) == '/') {
				p.pos++
			}
			if !p.eof() {
				p.pos += 2
			}
		} else {
			break
		}
	}
}

// --- entry points ---

func parseSelectorListStrErr(s string, allowParent, plainCss bool) (list *selList, err error) {
	defer func() {
		if r := recover(); r != nil {
			// The selector scanner only ever panics *SassError (via p.fail); a
			// non-SassError would fail this assertion and propagate as a panic.
			err = r.(*SassError)
		}
	}()
	p := newSelParser(s, allowParent, plainCss)
	list = p.selectorList()
	p.whitespace()
	if !p.eof() {
		p.fail("expected selector.")
	}
	return list, nil
}

func mustParseSelectorList(s string) *selList {
	list, err := parseSelectorListStrErr(s, true, false)
	if err != nil {
		panic(err)
	}
	return list
}

func parseComplexSelectorStr(s string, allowParent bool) *selComplex {
	p := newSelParser(s, allowParent, false)
	cx := p.complexSelector(false)
	p.whitespace()
	if !p.eof() {
		p.fail("expected selector.")
	}
	return cx
}

func parseCompoundSelectorStr(s string, allowParent bool) *compoundSel {
	p := newSelParser(s, allowParent, false)
	c := p.compoundSelector()
	if !p.eof() {
		p.fail("expected selector.")
	}
	return c
}

func parseSimpleSelectorStr(s string, allowParent bool) simpleSel {
	p := newSelParser(s, allowParent, false)
	sel := p.simpleSelector(nil)
	if !p.eof() {
		p.fail("unexpected token.")
	}
	return sel
}

// --- grammar ---

func (p *selParser) selectorList() *selList {
	components := []*selComplex{p.complexSelector(false)}
	p.whitespace()
	for p.scanChar(',') {
		p.whitespace()
		if p.peek() == ',' {
			continue
		}
		if p.eof() {
			break
		}
		components = append(components, p.complexSelector(false))
	}
	return &selList{components: components}
}

func (p *selParser) complexSelector(lineBreak bool) *selComplex {
	var lastCompound *compoundSel
	var combinators []combinator
	var initialCombinators []combinator
	var components []complexComponent

loop:
	for {
		p.whitespace()
		switch p.peek() {
		case '+':
			p.readChar()
			combinators = append(combinators, combNextSibling)
		case '>':
			p.readChar()
			combinators = append(combinators, combChild)
		case '~':
			p.readChar()
			combinators = append(combinators, combFollowingSibling)
		case 0:
			break loop
		default:
			if p.isCompoundStart() {
				if lastCompound != nil {
					components = append(components, complexComponent{selector: lastCompound, combinators: combinators})
				} else if len(combinators) > 0 {
					initialCombinators = combinators
				}
				nextCompound := p.compoundSelector()
				lastCompound = nextCompound
				combinators = nil
				if p.peek() == '&' {
					p.fail("\"&\" may only used at the beginning of a compound selector.")
				}
			} else {
				break loop
			}
		}
	}

	if len(combinators) > 0 && p.plainCss {
		p.fail("expected selector.")
	} else if lastCompound != nil {
		components = append(components, complexComponent{selector: lastCompound, combinators: combinators})
	} else if len(combinators) > 0 {
		initialCombinators = combinators
	} else {
		p.fail("expected selector.")
	}

	return &selComplex{leadingCombinators: initialCombinators, components: components, lineBreak: lineBreak}
}

func (p *selParser) isCompoundStart() bool {
	switch p.peek() {
	case '[', '.', '#', '%', ':', '&', '*', '|':
		return true
	}
	return p.lookingAtIdentifier()
}

func (p *selParser) compoundSelector() *compoundSel {
	components := []simpleSel{p.simpleSelector(nil)}
	for p.isSimpleSelectorStart(p.peek()) {
		ap := p.plainCss
		components = append(components, p.simpleSelector(&ap))
	}
	return &compoundSel{components: components}
}

func (p *selParser) isSimpleSelectorStart(c byte) bool {
	switch c {
	case '*', '[', '.', '#', '%', ':':
		return true
	case '&':
		return p.plainCss
	}
	return false
}

func (p *selParser) simpleSelector(allowParent *bool) simpleSel {
	ap := p.allowParent
	if allowParent != nil {
		ap = *allowParent
	}
	switch p.peek() {
	case '[':
		return p.attributeSelector()
	case '.':
		return p.classSelector()
	case '#':
		return p.idSelector()
	case '%':
		sel := p.placeholderSelector()
		if p.plainCss {
			p.fail("Placeholder selectors aren't allowed in plain CSS.")
		}
		return sel
	case ':':
		return p.pseudoSelector()
	case '&':
		sel := p.parentSelector()
		if !ap {
			p.fail("Parent selectors aren't allowed here.")
		}
		return sel
	default:
		return p.typeOrUniversalSelector()
	}
}

func (p *selParser) attributeSelector() *attrSel {
	p.expectChar('[')
	p.whitespace()
	name := p.attributeName()
	p.whitespace()
	if p.scanChar(']') {
		return &attrSel{name: name}
	}
	op := p.attributeOperator()
	p.whitespace()

	var value string
	next := p.peek()
	if next == '\'' || next == '"' {
		value = p.string()
	} else {
		value = p.identifier()
	}
	p.whitespace()

	var modifier *string
	next = p.peek()
	if isAlpha(next) {
		m := string(p.readChar())
		modifier = &m
	}
	p.expectChar(']')
	return &attrSel{name: name, op: op, value: value, modifier: modifier}
}

func (p *selParser) attributeName() qname {
	if p.scanChar('*') {
		p.expectChar('|')
		star := "*"
		return qname{name: p.identifier(), ns: &star}
	}
	if p.scanChar('|') {
		empty := ""
		return qname{name: p.identifier(), ns: &empty}
	}
	nameOrNamespace := p.identifier()
	if p.peek() != '|' || p.peekAt(1) == '=' {
		return qname{name: nameOrNamespace}
	}
	p.readChar()
	ns := nameOrNamespace
	return qname{name: p.identifier(), ns: &ns}
}

func (p *selParser) attributeOperator() string {
	switch p.readChar() {
	case '=':
		return "="
	case '~':
		p.expectChar('=')
		return "~="
	case '|':
		p.expectChar('=')
		return "|="
	case '^':
		p.expectChar('=')
		return "^="
	case '$':
		p.expectChar('=')
		return "$="
	case '*':
		p.expectChar('=')
		return "*="
	default:
		p.fail("Expected \"]\".")
		return ""
	}
}

func (p *selParser) classSelector() *classSel {
	p.expectChar('.')
	return &classSel{name: p.identifier()}
}

func (p *selParser) idSelector() *idSel {
	p.expectChar('#')
	return &idSel{name: p.identifier()}
}

func (p *selParser) placeholderSelector() *placeholderSel {
	p.expectChar('%')
	return &placeholderSel{name: p.identifier()}
}

func (p *selParser) parentSelector() *parentSel {
	p.expectChar('&')
	var suffix *string
	if p.lookingAtIdentifierBody() {
		s := p.identifierBody()
		suffix = &s
	}
	if p.plainCss && suffix != nil {
		p.fail("Parent selectors can't have suffixes in plain CSS.")
	}
	return &parentSel{suffix: suffix}
}

func (p *selParser) pseudoSelector() *pseudoSel {
	p.expectChar(':')
	element := p.scanChar(':')
	name := p.identifier()

	if !p.scanChar('(') {
		return newPseudo(name, element, nil, nil)
	}
	p.whitespace()

	unvendored := unvendor(name)
	var argument *string
	var selector *selList
	if element {
		if selectorPseudoElements[unvendored] {
			selector = p.selectorList()
		} else {
			s := p.declarationValue(true)
			argument = &s
		}
	} else if selectorPseudoClasses[unvendored] {
		selector = p.selectorList()
	} else if unvendored == "nth-child" || unvendored == "nth-last-child" {
		s := p.aNPlusB()
		p.whitespace()
		if isWhitespaceByte(p.peekAt(-1)) && p.peek() != ')' {
			p.expectIdentifier("of")
			s += " of"
			p.whitespace()
			selector = p.selectorList()
		}
		argument = &s
	} else {
		s := strings.TrimRight(p.declarationValue(true), " \t\n\r\f")
		argument = &s
	}
	p.expectChar(')')
	return newPseudo(name, element, argument, selector)
}

var selectorPseudoClasses = map[string]bool{
	"not": true, "is": true, "matches": true, "where": true, "current": true,
	"any": true, "has": true, "host": true, "host-context": true,
}

var selectorPseudoElements = map[string]bool{"slotted": true}

func (p *selParser) typeOrUniversalSelector() simpleSel {
	if p.scanChar('*') {
		if !p.scanChar('|') {
			return &universalSel{}
		}
		if p.scanChar('*') {
			star := "*"
			return &universalSel{ns: &star}
		}
		star := "*"
		return &typeSel{name: qname{name: p.identifier(), ns: &star}}
	} else if p.scanChar('|') {
		if p.scanChar('*') {
			empty := ""
			return &universalSel{ns: &empty}
		}
		empty := ""
		return &typeSel{name: qname{name: p.identifier(), ns: &empty}}
	}

	nameOrNamespace := p.identifier()
	if !p.scanChar('|') {
		return &typeSel{name: qname{name: nameOrNamespace}}
	} else if p.scanChar('*') {
		return &universalSel{ns: &nameOrNamespace}
	}
	ns := nameOrNamespace
	return &typeSel{name: qname{name: p.identifier(), ns: &ns}}
}

// aNPlusB parses an An+B microsyntax and returns its normalized text.
func (p *selParser) aNPlusB() string {
	var sb strings.Builder
	switch p.peek() {
	case 'e', 'E':
		p.expectIdentifier("even")
		return "even"
	case 'o', 'O':
		p.expectIdentifier("odd")
		return "odd"
	case '+', '-':
		sb.WriteByte(p.readChar())
	}

	if isDigit(p.peek()) {
		for isDigit(p.peek()) {
			sb.WriteByte(p.readChar())
		}
		p.whitespace()
		if !p.scanIdentChar('n') {
			return sb.String()
		}
	} else {
		p.expectIdentChar('n')
	}
	sb.WriteByte('n')
	p.whitespace()

	next := p.peek()
	if next != '+' && next != '-' {
		return sb.String()
	}
	sb.WriteByte(p.readChar())
	p.whitespace()
	if !isDigit(p.peek()) {
		p.fail("Expected a number.")
	}
	for isDigit(p.peek()) {
		sb.WriteByte(p.readChar())
	}
	return sb.String()
}

func (p *selParser) scanIdentChar(c byte) bool {
	if p.peek() == c {
		p.readChar()
		return true
	}
	return false
}

func (p *selParser) expectIdentChar(c byte) {
	if !p.scanIdentChar(c) {
		p.fail("Expected \"" + string(c) + "\".")
	}
}

func (p *selParser) expectIdentifier(text string) {
	for i := 0; i < len(text); i++ {
		if !p.scanIdentChar(text[i]) {
			p.fail("Expected \"" + text + "\".")
		}
	}
}

// declarationValue reads a CSS value up to an unbalanced ")" (a pseudo arg).
func (p *selParser) declarationValue(allowEmpty bool) string {
	var sb strings.Builder
	var brackets []byte
	wroteNewline := false
	for {
		next := p.peek()
		switch next {
		case 0:
			goto done
		case '\\':
			sb.WriteString(p.escape())
			wroteNewline = false
		case '"', '\'':
			sb.WriteString(p.rawString())
			wroteNewline = false
		case '/':
			if p.peekAt(1) == '*' {
				sb.WriteString(p.rawComment())
			} else {
				sb.WriteByte(p.readChar())
			}
			wroteNewline = false
		case ' ', '\t':
			if wroteNewline || !isWhitespaceByte(p.peekAt(1)) {
				sb.WriteByte(' ')
			}
			p.readChar()
		case '\n', '\r', '\f':
			if !isNewlineByte(p.peekAt(-1)) {
				sb.WriteByte('\n')
			}
			p.readChar()
			wroteNewline = true
		case '(', '{', '[':
			sb.WriteByte(next)
			brackets = append(brackets, oppositeBracket(p.readChar()))
			wroteNewline = false
		case ')', '}', ']':
			if len(brackets) == 0 {
				goto done
			}
			sb.WriteByte(next)
			p.readChar()
			brackets = brackets[:len(brackets)-1]
			wroteNewline = false
		case ';':
			if len(brackets) == 0 {
				goto done
			}
			sb.WriteByte(p.readChar())
		default:
			if p.lookingAtIdentifier() {
				sb.WriteString(p.identifier())
			} else {
				sb.WriteByte(p.readChar())
			}
			wroteNewline = false
		}
	}
done:
	if !allowEmpty && sb.Len() == 0 {
		p.fail("Expected token.")
	}
	return sb.String()
}

func (p *selParser) rawComment() string {
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

// rawString consumes a quoted string and returns it verbatim (with quotes).
func (p *selParser) rawString() string {
	start := p.pos
	q := p.readChar()
	for !p.eof() {
		c := p.peek()
		if c == '\\' {
			p.pos += 2
			continue
		}
		if c == q {
			p.readChar()
			break
		}
		p.readChar()
	}
	return p.src[start:p.pos]
}

// string consumes a quoted string and returns its interpreted (unquoted) value.
func (p *selParser) string() string {
	q := p.readChar()
	var sb strings.Builder
	for !p.eof() {
		c := p.peek()
		if c == q {
			p.readChar()
			break
		}
		if c == '\\' {
			if p.peekAt(1) == '\n' {
				p.pos += 2
				continue
			}
			sb.WriteString(p.escapeValue())
			continue
		}
		sb.WriteByte(p.readChar())
	}
	return sb.String()
}

// --- identifiers with escapes ---

func (p *selParser) lookingAtIdentifier() bool {
	c := p.peek()
	if isNameStart(c) || c == '\\' {
		return true
	}
	if c == '-' {
		c1 := p.peekAt(1)
		return c1 == '-' || c1 == '\\' || isNameStart(c1)
	}
	return false
}

func (p *selParser) lookingAtIdentifierBody() bool {
	c := p.peek()
	return isNameChar(c) || c == '\\'
}

func (p *selParser) identifier() string {
	var sb strings.Builder
	if p.scanChar('-') {
		sb.WriteByte('-')
		if p.scanChar('-') {
			sb.WriteByte('-')
			sb.WriteString(p.identifierBody())
			return sb.String()
		}
	}
	c := p.peek()
	switch {
	case c == 0:
		p.fail("Expected identifier.")
	case isNameStart(c):
		sb.WriteByte(p.readChar())
	case c == '\\':
		sb.WriteString(p.escapeCanonical(true))
	default:
		p.fail("Expected identifier.")
	}
	sb.WriteString(p.identifierBody())
	return sb.String()
}

func (p *selParser) identifierBody() string {
	var sb strings.Builder
	for !p.eof() {
		c := p.peek()
		if isNameChar(c) {
			sb.WriteByte(p.readChar())
		} else if c == '\\' {
			sb.WriteString(p.escapeCanonical(false))
		} else {
			break
		}
	}
	return sb.String()
}

// escapeCanonical consumes a "\" escape inside an identifier and returns
// dart-sass's canonical rendering of it: the escape is decoded to a code point
// and re-serialized (`\9` -> `\9 `, `\41` -> `A`, `\.` -> `\.`), matching how
// dart normalizes escapes in selector and media-query identifiers.
func (p *selParser) escapeCanonical(identifierStart bool) string {
	p.readChar() // backslash
	if p.eof() {
		return canonicalEscape(0xFFFD, identifierStart)
	}
	var value rune
	if isHexByte(p.peek()) {
		v := 0
		for i := 0; i < 6 && !p.eof() && isHexByte(p.peek()); i++ {
			v = v*16 + hexDigitValue(p.readChar())
		}
		if !p.eof() && isWhitespaceByte(p.peek()) {
			p.readChar()
		}
		value = rune(v)
	} else {
		value = p.nextRune()
	}
	return canonicalEscape(value, identifierStart)
}

// nextRune consumes one UTF-8 code point at the cursor.
func (p *selParser) nextRune() rune {
	r, size := utf8.DecodeRuneInString(p.src[p.pos:])
	p.pos += size
	return r
}

// escape consumes a "\" escape and returns it verbatim (source-preserving).
func (p *selParser) escape() string {
	start := p.pos
	p.readChar() // backslash
	if !p.eof() {
		p.readChar()
	}
	// hex escapes may run up to 6 digits plus optional trailing space
	for i := 0; i < 5 && isHexByte(p.peek()); i++ {
		p.readChar()
	}
	return p.src[start:p.pos]
}

// escapeValue consumes a "\" escape inside a quoted string, returning the
// escaped char literally.
func (p *selParser) escapeValue() string {
	start := p.pos
	p.readChar()
	if !p.eof() {
		p.readChar()
	}
	return p.src[start:p.pos]
}

// --- byte helpers ---

func isAlpha(c byte) bool { return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') }

func isWhitespaceByte(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f'
}

func isNewlineByte(c byte) bool { return c == '\n' || c == '\r' || c == '\f' }

func isHexByte(c byte) bool {
	return isDigit(c) || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

func oppositeBracket(c byte) byte {
	switch c {
	case '(':
		return ')'
	case '{':
		return '}'
	case '[':
		return ']'
	}
	return c
}
