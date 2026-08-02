// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import (
	"strconv"
	"strings"
)

func alwaysFalse(*parser) bool { return false }

func (p *parser) parseExpression() Expr { return p.parseCommaList(alwaysFalse) }

func (p *parser) parseExpressionStop(stop func(*parser) bool) Expr {
	return p.parseCommaList(stop)
}

func (p *parser) parseCommaList(stop func(*parser) bool) Expr {
	first := p.parseSpaceList(stop)
	p.ws()
	if p.peek() != ',' {
		return first
	}
	elements := []Expr{first}
	for p.peek() == ',' {
		p.next()
		p.ws()
		if p.eof() || stop(p) || p.stopChar() {
			break
		}
		elements = append(elements, p.parseSpaceList(stop))
		p.ws()
	}
	return &ListExpr{Elements: elements, Sep: SepComma}
}

func (p *parser) stopChar() bool {
	switch p.peek() {
	case ')', ']', '}', ';', 0:
		return true
	case '{':
		// A block-opening brace terminates an expression (a control at-rule
		// prelude such as `@each $a in b, {`). Reached only for a literal `{`;
		// `#{…}` interpolation is consumed as a unit by the value parser before
		// the cursor can rest on its brace. This lets a trailing comma before the
		// brace close a one-element comma list, as dart-sass permits.
		return true
	}
	return false
}

func (p *parser) parseSpaceList(stop func(*parser) bool) Expr {
	first := p.parseValue(stop)
	elements := []Expr{first}
	for {
		prevUnicodeRange := p.lastWasUnicodeRange
		save := p.pos
		had := p.ws()
		// dart-sass separates space-list elements at token boundaries even without
		// intervening whitespace: `"x"foo"y"` -> `"x" foo "y"`, `literal$input` ->
		// `literal $input`. A quote or a `$` always starts a fresh token, so a run
		// of values is adjacent when the previous element ended in a quote or the
		// next one opens with a quote or a variable.
		prev := byte(0)
		if save > 0 {
			prev = p.src[save-1]
		}
		// A close paren/bracket also ends a token: dart-sass separates a following
		// value into a fresh space-list element even with no whitespace
		// (`(1 + 2)px` -> `3 px`, `url(x)no-repeat` -> `url(x) no-repeat`). A "%"
		// likewise ends a percentage literal, so a value glued to it starts a
		// fresh element (`2%3` -> `2% 3`, `50%foo` -> `50% foo`); a "%" that is
		// the modulo operator is already consumed before reaching this loop. A
		// "&" parent reference is always its own token on both sides, so it too
		// separates glued neighbours (`&&` -> `& &`, `--&` -> `-- &`,
		// `foo&bar` -> `foo & bar`).
		// A leftover "-" at this point can only be one the additive parser handed
		// back because it begins a fresh value rather than a binary subtraction
		// (`1--em` -> `1 --em`, `1 -2` -> `1 -2`); it therefore always starts a
		// new space-list element.
		// A `!` glued to the previous value opens a fresh token: a CSS/Sass flag
		// (`c!important` -> `c !important`, dart-sass normalises the missing space)
		// or a `!`-prefixed value. A Sass variable flag (`!default`/`!global`/
		// `!optional`) still ends the value list because canStartValue rejects it.
		adjacentBoundary := prevUnicodeRange || prev == '"' || prev == '\'' ||
			prev == ')' || prev == ']' || prev == '%' || prev == '&' ||
			p.peek() == '"' || p.peek() == '\'' || p.peek() == '$' || p.peek() == '&' ||
			p.peek() == '-' || p.peek() == '!' || (p.peek() == '#' && p.peekAt(1) == '{')
		if (!had && !adjacentBoundary) || stop(p) || p.stopChar() || p.peek() == ',' || !p.canStartValue() {
			p.pos = save
			break
		}
		elements = append(elements, p.parseValue(stop))
	}
	if len(elements) == 1 {
		return first
	}
	return &ListExpr{Elements: elements, Sep: SepSpace}
}

func (p *parser) atFlag() bool {
	return strings.HasPrefix(p.src[p.pos:], "!default") || strings.HasPrefix(p.src[p.pos:], "!global") || strings.HasPrefix(p.src[p.pos:], "!optional")
}

func (p *parser) canStartValue() bool {
	c := p.peek()
	if c == '!' && p.atFlag() {
		return false
	}
	switch {
	case isDigit(c):
		return true
	case c == '.':
		return isDigit(p.peekAt(1))
	case c == '$' || c == '(' || c == '[' || c == '"' || c == '\'' || c == '&' || c == '!':
		return true
	case c == '#':
		return true
	case c == '-' || c == '+':
		return true
	case c == '%':
		return true
	case c == '/':
		return false
	case isNameStart(c) || c == '\\':
		return true
	}
	return false
}

// parseValue parses a full operator expression (one list element).
func (p *parser) parseValue(stop func(*parser) bool) Expr {
	p.lastWasUnicodeRange = false
	return p.parseOr()
}

func (p *parser) parseOr() Expr {
	left := p.parseAnd()
	for {
		save := p.pos
		p.ws()
		if p.matchOperator("or") {
			p.ws()
			right := p.parseAnd()
			left = &Binary{Op: "or", Left: left, Right: right}
		} else {
			p.pos = save
			return left
		}
	}
}

func (p *parser) parseAnd() Expr {
	left := p.parseNot()
	for {
		save := p.pos
		p.ws()
		if p.matchOperator("and") {
			p.ws()
			right := p.parseNot()
			left = &Binary{Op: "and", Left: left, Right: right}
		} else {
			p.pos = save
			return left
		}
	}
}

func (p *parser) parseNot() Expr {
	save := p.pos
	if p.matchOperator("not") {
		p.ws()
		return &Unary{Op: "not", Expr: p.parseNot()}
	}
	p.pos = save
	return p.parseEquality()
}

func (p *parser) parseEquality() Expr {
	left := p.parseRelational()
	for {
		save := p.pos
		p.ws()
		if p.match("==") {
			p.ws()
			left = &Binary{Op: "==", Left: left, Right: p.parseRelational()}
		} else if p.match("!=") {
			p.ws()
			left = &Binary{Op: "!=", Left: left, Right: p.parseRelational()}
		} else {
			p.pos = save
			return left
		}
	}
}

func (p *parser) parseRelational() Expr {
	left := p.parseAdditive()
	for {
		save := p.pos
		p.ws()
		var op string
		if p.mediaFeatureStop && (p.peek() == '<' || p.peek() == '>') {
			// Inside a media feature's top-level operand, a `<`/`>`/`<=`/`>=` is
			// a range operator owned by the media-query parser, not a SassScript
			// comparison.
			p.pos = save
			return left
		}
		if p.match("<=") {
			op = "<="
		} else if p.match(">=") {
			op = ">="
		} else if p.peek() == '<' {
			p.next()
			op = "<"
		} else if p.peek() == '>' {
			p.next()
			op = ">"
		} else {
			p.pos = save
			return left
		}
		p.ws()
		left = &Binary{Op: op, Left: left, Right: p.parseAdditive()}
	}
}

func (p *parser) parseAdditive() Expr {
	left := p.parseMultiplicative()
	for {
		save := p.pos
		before := p.ws()
		c := p.peek()
		if c != '+' && c != '-' {
			p.pos = save
			return left
		}
		after := isSpaceByte(p.peekAt(1))
		// A "-" with no whitespace after it begins a new unary value (a
		// space-separated list element) rather than a binary operator when what
		// follows starts an interpolated identifier (`"q"-a`, `c -d`, `c -#{x}`)
		// — always — or a number (`c -1`, `10px -5px`) when there was
		// whitespace before the "-". Otherwise (`c -(d)`, `"q"-2`, `"q"-"r"`,
		// `1-2`) the "-" is a binary subtraction.
		if c == '-' && !after && (p.minusBeginsIdent() || (before && p.minusBeginsNumber())) {
			p.pos = save
			return left
		}
		p.next()
		p.ws()
		p.arith++
		right := p.parseMultiplicative()
		p.arith--
		left = &Binary{Op: string(c), Left: left, Right: right}
	}
}

// minusBeginsIdent reports whether the "-" at the cursor introduces an
// interpolated identifier (`-d`, `-\9`, `--x`, `-#{x}`), which dart-sass always
// lexes as the start of a fresh value.
func (p *parser) minusBeginsIdent() bool {
	n := p.peekAt(1)
	switch {
	case isNameStart(n) || n == '\\' || n == '-':
		return true
	case n == '#' && p.peekAt(2) == '{':
		return true
	}
	return false
}

// minusBeginsNumber reports whether the "-" at the cursor introduces a number
// (`-1`, `-.5`), which dart-sass lexes as a fresh value only when preceded by
// whitespace.
func (p *parser) minusBeginsNumber() bool {
	n := p.peekAt(1)
	return isDigit(n) || (n == '.' && isDigit(p.peekAt(2)))
}

func (p *parser) parseMultiplicative() Expr {
	left := p.parseUnary()
	for {
		save := p.pos
		p.ws()
		c := p.peek()
		if c == '%' {
			// A "%" is the modulo operator only when a right-hand operand
			// follows; a "%" with nothing (or a non-operand) after it is a
			// literal token (e.g. `b: c %`), left for the space-list parser.
			p.next()
			p.ws()
			if !p.canStartValue() {
				p.pos = save
				return left
			}
			p.arith++
			right := p.parseUnary()
			p.arith--
			left = &Binary{Op: "%", Left: left, Right: right}
		} else if c == '*' {
			p.next()
			p.ws()
			p.arith++
			right := p.parseUnary()
			p.arith--
			left = &Binary{Op: "*", Left: left, Right: right}
		} else if c == '/' {
			p.next()
			p.ws()
			right := p.parseUnary()
			b := &Binary{Op: "/", Left: left, Right: right}
			b.Slash = p.arith == 0 && isLiteralNumberish(left) && isLiteralNumberish(right)
			left = b
		} else {
			p.pos = save
			return left
		}
	}
}

func isSpaceByte(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f'
}

// consumeStringEscape decodes a CSS escape sequence in a quoted string, with the
// leading backslash already consumed. A run of 1-6 hex digits denotes a code
// point (with one optional trailing whitespace consumed); a backslash before a
// newline is a line continuation; any other escaped character is literal.
func (p *parser) consumeStringEscape() string {
	if p.eof() {
		return "�"
	}
	c := p.peek()
	if isHexDigit(c) {
		hex := make([]byte, 0, 6)
		for len(hex) < 6 && !p.eof() && isHexDigit(p.peek()) {
			hex = append(hex, p.next())
		}
		if !p.eof() && isSpaceByte(p.peek()) {
			p.next()
		}
		cp, _ := strconv.ParseInt(string(hex), 16, 32)
		if cp == 0 || cp > 0x10FFFF || (cp >= 0xD800 && cp <= 0xDFFF) {
			return "�"
		}
		return string(rune(cp))
	}
	if c == '\n' {
		p.next()
		return ""
	}
	return string(p.next())
}

func isLiteralNumberish(e Expr) bool {
	switch v := e.(type) {
	case *NumberLit:
		return true
	case *Binary:
		// A nested slash division (already resolved as slash-preserving) is itself
		// numberish, so a chain like 1/2/3/4/5 keeps the slash all the way up
		// rather than dividing at the second "/".
		return v.Op == "/" && v.Slash
	case *Unary:
		return (v.Op == "-" || v.Op == "+") && isLiteralNumberish(v.Expr)
	case *FuncCall:
		// A bare CSS calculation (calc(), clamp(), sqrt(), sin(), …) counts as a
		// numberish operand for the slash-list rule, exactly as dart-sass treats a
		// CalculationExpression. The legacy-gated names (min/max/round/abs) parse
		// as global Sass functions, not calculations, so they do NOT preserve the
		// slash — matching dart, where "min(2, 4) / 3" divides but "sqrt(4) / 3"
		// stays a slash-separated list.
		if v.Namespace != "" {
			return false
		}
		name := strings.ToLower(normIdent(v.Name))
		_, isCalc := calcArity[name]
		return isCalc && !calcLegacyGated[name]
	}
	return false
}

func (p *parser) parseUnary() Expr {
	c := p.peek()
	if c == '-' || c == '+' {
		// A "-" immediately followed by an identifier start is part of the
		// identifier (e.g. -webkit-foo, or a function named -real-channel),
		// not a unary negation.
		if c == '-' {
			n := p.peekAt(1)
			if isNameStart(n) || n == '\\' {
				return p.parsePrimary()
			}
			// A second leading hyphen makes this a custom-property-style
			// identifier (`--x`, `--`, `---`, `--1`), e.g. inside `var(--1)`,
			// not repeated unary negation.
			if n == '-' {
				return p.parsePrimary()
			}
		}
		p.next()
		p.ws()
		return &Unary{Op: string(c), Expr: p.parseUnary()}
	}
	return p.parsePrimary()
}

func (p *parser) parsePrimary() Expr {
	c := p.peek()
	switch {
	case isDigit(c) || (c == '.' && isDigit(p.peekAt(1))):
		return p.parseNumber()
	case c == '$':
		p.next()
		return &VarRef{Name: p.scanIdentifier()}
	case c == '(':
		return p.parseParenOrMap()
	case c == '[':
		return p.parseBracketList()
	case c == '"' || c == '\'':
		return p.parseStringLiteral()
	case c == '&':
		p.next()
		return &Parent{}
	case c == '!':
		// A `!` flag (e.g. `!important`) may carry whitespace after the bang;
		// dart-sass normalises `! important` to `!important`.
		p.next()
		p.ws()
		id := p.scanIdentifier()
		return &Ident{Name: "!" + id}
	case c == '#':
		return p.parseHashValue()
	case c == '%':
		// A lone "%" (no operands) is a literal token, e.g. `b: %` or `b: % c`.
		p.next()
		return &Ident{Name: "%"}
	case isNameStart(c) || c == '\\' || c == '-':
		return p.parseIdentValue()
	case c == '/':
		// A value may begin with "/" (e.g. `/bar`, or the right operand of a
		// degenerate slash chain like `1/ / /bar`). dart-sass treats it as a
		// slash separator with an empty left-hand side, serialising as
		// "/" + right.
		p.next()
		p.ws()
		return &Binary{Op: "/", Left: nil, Right: p.parseUnary()}
	}
	p.fail("Expected expression.")
	return nil
}

func (p *parser) parseNumber() Expr {
	start := p.pos
	for isDigit(p.peek()) {
		p.next()
	}
	if p.peek() == '.' && isDigit(p.peekAt(1)) {
		p.next()
		for isDigit(p.peek()) {
			p.next()
		}
	}
	// exponent
	if (p.peek() == 'e' || p.peek() == 'E') && (isDigit(p.peekAt(1)) || ((p.peekAt(1) == '+' || p.peekAt(1) == '-') && isDigit(p.peekAt(2)))) {
		p.next()
		if p.peek() == '+' || p.peek() == '-' {
			p.next()
		}
		for isDigit(p.peek()) {
			p.next()
		}
	}
	numStr := p.src[start:p.pos]
	val, _ := strconv.ParseFloat(numStr, 64)
	unit := ""
	if p.peek() == '%' {
		p.next()
		unit = "%"
	} else if p.unitStarts() {
		unit = p.scanUnit()
	}
	return &NumberLit{Val: val, Unit: unit}
}

// unitStarts reports whether an identifier usable as a number's unit begins at
// the cursor. A unit may start with a single "-" but never "--" (`1--em` is the
// number `1` followed by the identifier `--em`, not a unit `--em`), and a "-"
// before a digit is subtraction, not a unit (`1-2` is `1 - 2`).
func (p *parser) unitStarts() bool {
	c := p.peek()
	if isNameStart(c) || c == '\\' {
		return true
	}
	if c == '-' {
		n := p.peekAt(1)
		return isNameStart(n) || n == '\\'
	}
	return false
}

// scanUnit reads a number's unit identifier. Unlike a general identifier, a "-"
// directly followed by a digit terminates the unit, so `1em-2em` lexes as
// `1em - 2em` (subtraction) rather than a number with unit `em-2em`, matching
// dart-sass's dedicated unit lexer. A trailing "-" that is *not* followed by a
// digit stays part of the unit (`10px- 10px` -> the unit `px-` then `10px`).
func (p *parser) scanUnit() string {
	var sb strings.Builder
	if p.peek() == '-' {
		sb.WriteByte(p.next())
	}
	for !p.eof() {
		c := p.peek()
		if c == '\\' {
			sb.WriteString(p.scanEscape(false))
			continue
		}
		if c == '-' && (isDigit(p.peekAt(1)) || (p.peekAt(1) == '.' && isDigit(p.peekAt(2)))) {
			break
		}
		if isNameChar(c) {
			sb.WriteByte(p.next())
			continue
		}
		break
	}
	return sb.String()
}

func (p *parser) parseParenOrMap() Expr {
	// A grouping paren starts a fresh comparison context (the media-feature
	// range-operator suppression applies only at the feature's top level).
	savedStop := p.mediaFeatureStop
	p.mediaFeatureStop = false
	defer func() { p.mediaFeatureStop = savedStop }()
	p.next() // (
	// Parentheses do NOT force a division context: a "/" between literals inside
	// them keeps its slash provenance (so "(1 2/3 4)" preserves the 2/3). The
	// grouping itself strips provenance from its scalar result at eval time (via
	// the Paren node), which is why "(1/2)" still collapses to 0.5. A grouping
	// also CLEARS any surrounding arithmetic context, so a "/" inside it keeps
	// its slash even when the parens are an operand of an outer operation
	// (`x + (5/6 7/8)` -> `x5/6 7/8`, not `x0.833… 0.875`).
	savedArith := p.arith
	p.arith = 0
	defer func() { p.arith = savedArith }()
	p.ws()
	if p.peek() == ')' {
		p.next()
		return &ListExpr{Elements: nil, Sep: SepUndecided}
	}
	first := p.parseSpaceList(alwaysFalse)
	p.ws()
	if p.peek() == ':' {
		// map
		p.next()
		p.ws()
		val := p.parseSpaceList(alwaysFalse)
		m := &MapExpr{Keys: []Expr{first}, Values: []Expr{val}}
		p.ws()
		for p.peek() == ',' {
			p.next()
			p.ws()
			if p.peek() == ')' {
				break
			}
			k := p.parseSpaceList(alwaysFalse)
			p.ws()
			if p.peek() != ':' {
				p.fail("Expected \":\".")
			}
			p.next()
			p.ws()
			v := p.parseSpaceList(alwaysFalse)
			m.Keys = append(m.Keys, k)
			m.Values = append(m.Values, v)
			p.ws()
		}
		if p.peek() != ')' {
			p.fail("Expected \")\".")
		}
		p.next()
		return m
	}
	if p.peek() == ',' {
		elements := []Expr{first}
		for p.peek() == ',' {
			p.next()
			p.ws()
			if p.peek() == ')' {
				break
			}
			elements = append(elements, p.parseSpaceList(alwaysFalse))
			p.ws()
		}
		if p.peek() != ')' {
			p.fail("Expected \")\".")
		}
		p.next()
		return &ListExpr{Elements: elements, Sep: SepComma}
	}
	if p.peek() != ')' {
		p.fail("Expected \")\".")
	}
	p.next()
	return &Paren{Expr: first}
}

func (p *parser) parseBracketList() Expr {
	savedStop := p.mediaFeatureStop
	p.mediaFeatureStop = false
	defer func() { p.mediaFeatureStop = savedStop }()
	p.next() // [
	p.ws()
	if p.peek() == ']' {
		p.next()
		return &ListExpr{Elements: nil, Sep: SepUndecided, Bracketed: true}
	}
	first := p.parseSpaceList(alwaysFalse)
	p.ws()
	sep := SepSpace
	elements := []Expr{first}
	if p.peek() == ',' {
		sep = SepComma
		for p.peek() == ',' {
			p.next()
			p.ws()
			if p.peek() == ']' {
				break
			}
			elements = append(elements, p.parseSpaceList(alwaysFalse))
			p.ws()
		}
	} else if l, ok := first.(*ListExpr); ok && l.Sep == SepSpace {
		if p.peek() != ']' {
			p.fail("Expected \"]\".")
		}
		p.next()
		return &ListExpr{Elements: l.Elements, Sep: SepSpace, Bracketed: true}
	}
	if p.peek() != ']' {
		p.fail("Expected \"]\".")
	}
	p.next()
	if len(elements) == 1 && sep == SepSpace {
		return &ListExpr{Elements: elements, Sep: SepUndecided, Bracketed: true}
	}
	return &ListExpr{Elements: elements, Sep: sep, Bracketed: true}
}

func (p *parser) parseStringLiteral() Expr {
	q := p.next()
	var parts []any
	var sb strings.Builder
	flush := func() {
		parts = append(parts, sb.String())
		sb.Reset()
	}
	for !p.eof() {
		c := p.peek()
		if c == q {
			p.next()
			break
		}
		if c == '\\' {
			p.next()
			sb.WriteString(p.consumeStringEscape())
			continue
		}
		if c == '#' && p.peekAt(1) == '{' {
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
			continue
		}
		sb.WriteByte(p.next())
	}
	flush()
	return &StringLit{Parts: &Interp{Parts: parts}, Quoted: true}
}

func (p *parser) parseHashValue() Expr {
	if p.peekAt(1) == '{' {
		savedStop := p.mediaFeatureStop
		p.mediaFeatureStop = false
		defer func() { p.mediaFeatureStop = savedStop }()
		p.pos += 2
		p.ws()
		e := p.parseExpression()
		p.ws()
		if p.peek() != '}' {
			p.fail("Expected \"}\".")
		}
		p.next()
		// `#{1}bar` / `#{1}#{2}`: interpolation immediately followed by more
		// identifier text is a single interpolated identifier.
		if isNameChar(p.peek()) || p.peek() == '\\' || (p.peek() == '#' && p.peekAt(1) == '{') {
			return p.maybeInterpCall(p.continueInterpIdent([]any{&InterpExpr{Expr: e}}))
		}
		// `#{$f}(a, b)`: an interpolation directly followed by "(" names a plain
		// CSS function call whose name is the interpolation.
		if p.peek() == '(' {
			return &FuncCall{NameInterp: &Interp{Parts: []any{&InterpExpr{Expr: e}}}, Args: p.parseArgList()}
		}
		return &InterpExpr{Expr: e}
	}
	start := p.pos
	p.next() // #
	// A "#" that leads with a digit is a hex colour (`#123`, `#abcdef`); dart
	// reads only hex digits here. A "#" followed by an identifier is either a
	// hex colour whose body happens to be all hex digits (`#abcdef`) or, when
	// the identifier is not a valid colour, a CSS ID token (`#ab`, `#axc`,
	// `#abcde`) which serialises verbatim.
	if isDigit(p.peek()) {
		for isHexDigit(p.peek()) {
			p.next()
		}
		hex := p.src[start:p.pos]
		if isHexColor(hex) {
			return &ColorLit{Color: newHexColor(strings.ToLower(hex[:1]) + hex[1:])}
		}
		return &Ident{Name: hex}
	}
	name := p.scanIdentifier()
	// An ID token may embed interpolation (`#a#{b}`), yielding an unquoted
	// interpolated string rather than a bare identifier.
	if p.peek() == '#' && p.peekAt(1) == '{' {
		return p.continueInterpIdent([]any{"#" + name})
	}
	hex := "#" + name
	if isHexColor(hex) {
		return &ColorLit{Color: newHexColor(strings.ToLower(hex[:1]) + hex[1:])}
	}
	return &Ident{Name: hex}
}

// parseUnicodeRange parses a CSS unicode-range token (the cursor is on the
// leading "u"/"U"). It is emitted verbatim, preserving case: `U+1`, `u+1a2b`,
// `U+1A2B-F9E8`, `U+????`, `U+A?`. A "?" wildcard precludes a range end.
func (p *parser) parseUnicodeRange() Expr {
	var sb strings.Builder
	sb.WriteByte(p.next()) // u/U
	sb.WriteByte(p.next()) // +
	digits := 0
	for digits < 6 && isHexDigit(p.peek()) {
		sb.WriteByte(p.next())
		digits++
	}
	questions := 0
	for digits+questions < 6 && p.peek() == '?' {
		sb.WriteByte(p.next())
		questions++
	}
	if questions == 0 && p.peek() == '-' && isHexDigit(p.peekAt(1)) {
		sb.WriteByte(p.next()) // -
		for n := 0; n < 6 && isHexDigit(p.peek()); n++ {
			sb.WriteByte(p.next())
		}
	}
	p.lastWasUnicodeRange = true
	return &Ident{Name: sb.String()}
}

func isHexDigit(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

// continueInterpIdent finishes an interpolated identifier value whose leading
// pieces are already parsed. It keeps consuming adjacent name characters,
// escapes, and `#{}` interpolations — with no intervening whitespace — into a
// single unquoted string expression, matching how dart-sass lexes an identifier
// that embeds interpolation (e.g. `foo#{1}bar` -> the unquoted string `foo1bar`).
func (p *parser) continueInterpIdent(parts []any) *StringLit {
	var sb strings.Builder
	flush := func() {
		if sb.Len() > 0 {
			parts = append(parts, sb.String())
			sb.Reset()
		}
	}
	for {
		if p.peek() == '#' && p.peekAt(1) == '{' {
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
			continue
		}
		c := p.peek()
		if c == '\\' {
			sb.WriteString(p.scanEscape(false))
			continue
		}
		if isNameChar(c) {
			sb.WriteByte(p.next())
			continue
		}
		break
	}
	flush()
	return &StringLit{Parts: &Interp{Parts: parts}, Quoted: false}
}

// maybeInterpCall turns an interpolated identifier that is directly followed by
// "(" into a plain CSS function call whose name is the interpolation
// (`foo#{1}bar(arg)`, `#{$f}(arg)`). Anything else passes through unchanged.
func (p *parser) maybeInterpCall(sl *StringLit) Expr {
	if p.peek() == '(' {
		return &FuncCall{NameInterp: sl.Parts, Args: p.parseArgList()}
	}
	return sl
}

func (p *parser) parseIdentValue() Expr {
	// A unicode range (`U+1A2B`, `u+1a2b`, `U+1-B`, `U+A?`) begins with a lone
	// "u"/"U" immediately followed by "+" and a hex digit or "?".
	if (p.peek() == 'u' || p.peek() == 'U') && p.peekAt(1) == '+' &&
		(isHexDigit(p.peekAt(2)) || p.peekAt(2) == '?') {
		return p.parseUnicodeRange()
	}
	name := p.scanIdentifier()
	// An identifier that embeds interpolation (`foo#{1}bar`) is an unquoted
	// interpolated string, not a bare identifier / function / namespace access —
	// unless it is directly followed by "(", which makes it a plain CSS function
	// call whose name is the interpolation (`foo#{1}bar(arg)`).
	if p.peek() == '#' && p.peekAt(1) == '{' {
		return p.maybeInterpCall(p.continueInterpIdent([]any{name}))
	}
	// IE progid:...() special function (uses ":" rather than "("); the name may
	// be vendor-prefixed (e.g. -c-progid).
	if p.peek() == ':' && unvendor(strings.ToLower(name)) == "progid" {
		return p.tryProgid(name)
	}
	// namespaced access: ns.func(  or  ns.$var. The "." must introduce a member
	// (a "$" variable or an identifier); a "." that begins a spread ("..." after
	// a bare value, as in `a...`/`null...`) is not namespace access and is left
	// for the argument-list parser to consume.
	if p.peek() == '.' && (p.peekAt(1) == '$' || isNameStart(p.peekAt(1)) || p.peekAt(1) == '\\' || p.peekAt(1) == '-') {
		p.next()
		if p.peek() == '$' {
			p.next()
			return &VarRef{Namespace: name, Name: p.scanIdentifier()}
		}
		sub := p.scanIdentifier()
		if p.peek() == '(' {
			return &FuncCall{Namespace: name, Name: sub, Args: p.parseArgList()}
		}
		return &Ident{Name: name + "." + sub}
	}
	if p.peek() == '(' {
		if isCalcLike(name) {
			return p.parseCalc(name)
		}
		// CSS special functions preserve their arguments verbatim.
		if e, ok := p.trySpecialFunction(name); ok {
			return e
		}
		// if() is ambiguous between the legacy Sass control function and the
		// modern CSS if() syntax. Try the legacy argument list first and fall
		// back to the modern grammar (which uses ":"/";" branch separators).
		if strings.EqualFold(name, "if") {
			save := p.pos
			if al, ok := p.tryParseArgList(); ok {
				return &FuncCall{Name: name, Args: al}
			}
			p.pos = save
			return p.parseModernIf()
		}
		return &FuncCall{Name: name, Args: p.parseArgListOpt(strings.EqualFold(name, "var"))}
	}
	switch strings.ToLower(name) {
	case "true":
		return &BoolLit{V: true}
	case "false":
		return &BoolLit{V: false}
	case "null":
		return &NullLit{}
	}
	return &Ident{Name: name}
}

// isCalcLike reports whether a function's contents form a CSS calculation that
// must be preserved rather than evaluated as Sass arithmetic.
func isCalcLike(name string) bool {
	switch strings.ToLower(name) {
	case "env":
		return true
	}
	return false
}

// parseCalc captures a calc-like function's raw balanced contents (evaluating
// only #{} interpolations) and returns it as an unquoted string expression.
func (p *parser) parseCalc(name string) Expr {
	p.next() // (
	var parts []any
	var sb strings.Builder
	sb.WriteString(name)
	sb.WriteByte('(')
	depth := 1
	flush := func() {
		if sb.Len() > 0 {
			parts = append(parts, sb.String())
			sb.Reset()
		}
	}
	for !p.eof() && depth > 0 {
		c := p.peek()
		// A silent comment is stripped and, together with any adjacent
		// whitespace, collapsed to a single space, as dart-sass does inside a
		// special function's argument text.
		if c == '/' && p.peekAt(1) == '/' {
			for !p.eof() && p.peek() != '\n' {
				p.pos++
			}
			for isSpaceByte(p.peek()) {
				p.pos++
			}
			sb.WriteByte(' ')
			continue
		}
		if c == '#' && p.peekAt(1) == '{' {
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
			continue
		}
		if c == '"' || c == '\'' {
			flush()
			parts = append(parts, p.quotedStringToInterp()...)
			continue
		}
		if c == '(' {
			depth++
		} else if c == ')' {
			depth--
			if depth == 0 {
				sb.WriteByte(')')
				p.next()
				break
			}
		}
		sb.WriteByte(p.next())
	}
	flush()
	return &StringLit{Parts: &Interp{Parts: parts}, Quoted: false}
}

func (p *parser) matchKeyword(kw string) bool {
	if strings.HasPrefix(strings.ToLower(p.src[p.pos:min(p.pos+len(kw)+1, len(p.src))]), kw) {
		after := p.peekAt(len(kw))
		if !isNameChar(after) {
			p.pos += len(kw)
			return true
		}
	}
	return false
}

// matchOperator matches a SassScript operator keyword (`and`, `or`, `not`)
// case-sensitively. Unlike the contextual keyword `using`, dart-sass treats
// these as operators only in lowercase: `NOT`/`AND`/`OR` are ordinary
// identifiers, so `NOT()` calls a function named NOT and `true AND false` is a
// three-element space list.
func (p *parser) matchOperator(kw string) bool {
	if strings.HasPrefix(p.src[p.pos:], kw) {
		after := p.peekAt(len(kw))
		if !isNameChar(after) {
			p.pos += len(kw)
			return true
		}
	}
	return false
}

// --- parameter and argument lists ---

func (p *parser) parseParamList() *ParamList {
	if p.peek() != '(' {
		p.fail("Expected \"(\".")
	}
	p.next()
	pl := &ParamList{}
	for {
		p.ws()
		if p.peek() == ')' {
			p.next()
			break
		}
		if p.peek() != '$' {
			p.fail("Expected variable.")
		}
		p.next()
		name := p.scanIdentifier()
		param := Param{Name: name}
		p.ws()
		if p.match("...") {
			param.Rest = true
			p.ws()
		} else if p.peek() == ':' {
			p.next()
			p.ws()
			param.Default = p.parseSpaceList(alwaysFalse)
			p.ws()
		}
		pl.Params = append(pl.Params, param)
		if p.peek() == ',' {
			p.next()
			continue
		}
		if p.peek() == ')' {
			p.next()
			break
		}
		p.fail("Expected \")\".")
	}
	return pl
}

func (p *parser) parseArgList() *ArgList { return p.parseArgListOpt(false) }

// parseArgListOpt parses a call's argument list. When allowEmptySecondArg is
// set (dart-sass permits this only for var()), a trailing empty second argument
// after a single real argument is preserved as an empty unquoted string rather
// than discarded, so `var(--c,)` round-trips as `var(--c, )`.
func (p *parser) parseArgListOpt(allowEmptySecondArg bool) *ArgList {
	// Function arguments are their own arithmetic context: a "/" between literal
	// numbers here forms a slash-separated list (e.g. the alpha in
	// `hsl(180 60% 50% / 0.4)`), even when the call is nested in parentheses.
	savedArith := p.arith
	p.arith = 0
	savedStop := p.mediaFeatureStop
	p.mediaFeatureStop = false
	defer func() { p.arith = savedArith; p.mediaFeatureStop = savedStop }()
	p.next() // (
	al := &ArgList{}
	for {
		p.ws()
		if p.peek() == ')' {
			p.next()
			break
		}
		arg := Arg{}
		// named argument?
		if p.peek() == '$' {
			save := p.pos
			p.next()
			nm := p.scanIdentifier()
			p.ws()
			if p.peek() == ':' {
				p.next()
				p.ws()
				arg.Name = nm
				arg.Value = p.parseSpaceList(alwaysFalse)
			} else {
				p.pos = save
				arg.Value = p.parseArgValue()
			}
		} else {
			arg.Value = p.parseArgValue()
		}
		p.ws()
		// Microsoft-filter "single equals" operator: `alpha(c=d)`, `foo(a=b)`.
		// dart-sass parses a lone "=" inside an argument as a singleEquals binary
		// operation whose value serialises as "<left>=<right>", enabling legacy
		// IE filter syntax to round-trip through a function call.
		if p.peek() == '=' && p.peekAt(1) != '=' {
			p.next()
			p.ws()
			rhs := p.parseArgValue()
			arg.Value = &Binary{Op: "=", Left: arg.Value, Right: rhs}
			p.ws()
		}
		if p.match("...") {
			arg.Spread = true
			p.ws()
		}
		al.Args = append(al.Args, arg)
		if p.peek() == ',' {
			p.next()
			if allowEmptySecondArg && len(al.Args) == 1 && al.Args[0].Name == "" && !al.Args[0].Spread {
				save := p.pos
				p.ws()
				if p.peek() == ')' {
					al.Args = append(al.Args, Arg{Value: &StringLit{Parts: literalInterp("")}})
				}
				p.pos = save
			}
			continue
		}
		if p.peek() == ')' {
			p.next()
			break
		}
		p.fail("Expected \")\" or \",\".")
	}
	return al
}

func (p *parser) parseArgValue() Expr {
	return p.parseSpaceList(alwaysFalse)
}
