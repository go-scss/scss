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
	}
	return false
}

func (p *parser) parseSpaceList(stop func(*parser) bool) Expr {
	first := p.parseValue(stop)
	elements := []Expr{first}
	for {
		save := p.pos
		had := p.ws()
		if !had || stop(p) || p.stopChar() || p.peek() == ',' || !p.canStartValue() {
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
	case c == '/':
		return false
	case isNameStart(c) || c == '\\':
		return true
	}
	return false
}

// parseValue parses a full operator expression (one list element).
func (p *parser) parseValue(stop func(*parser) bool) Expr { return p.parseOr() }

func (p *parser) parseOr() Expr {
	left := p.parseAnd()
	for {
		save := p.pos
		p.ws()
		if p.matchKeyword("or") {
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
		if p.matchKeyword("and") {
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
	if p.matchKeyword("not") {
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
		// A "-" with space before but not after starts a new unary value.
		if c == '-' && before && !after {
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

func (p *parser) parseMultiplicative() Expr {
	left := p.parseUnary()
	for {
		save := p.pos
		p.ws()
		c := p.peek()
		if c == '*' || c == '%' {
			p.next()
			p.ws()
			p.arith++
			right := p.parseUnary()
			p.arith--
			left = &Binary{Op: string(c), Left: left, Right: right}
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

func isLiteralNumberish(e Expr) bool {
	switch v := e.(type) {
	case *NumberLit:
		return true
	case *Unary:
		return (v.Op == "-" || v.Op == "+") && isLiteralNumberish(v.Expr)
	}
	return false
}

func (p *parser) parseUnary() Expr {
	c := p.peek()
	if c == '-' || c == '+' {
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
		p.next()
		id := p.scanIdentifier()
		return &Ident{Name: "!" + id}
	case c == '#':
		return p.parseHashValue()
	case isNameStart(c) || c == '\\' || c == '-':
		return p.parseIdentValue()
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
	} else if p.looksLikeIdentifier() {
		unit = p.scanIdentifier()
	}
	return &NumberLit{Val: val, Unit: unit}
}

func (p *parser) parseParenOrMap() Expr {
	p.next() // (
	p.arith++
	defer func() { p.arith-- }()
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
			if !p.eof() {
				sb.WriteByte('\\')
				sb.WriteByte(p.next())
			}
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
		p.pos += 2
		p.ws()
		e := p.parseExpression()
		p.ws()
		if p.peek() != '}' {
			p.fail("Expected \"}\".")
		}
		p.next()
		return &InterpExpr{Expr: e}
	}
	start := p.pos
	p.next() // #
	for isHexDigit(p.peek()) {
		p.next()
	}
	hex := p.src[start:p.pos]
	if isHexColor(hex) {
		return &ColorLit{Color: newHexColor(strings.ToLower(hex[:1]) + hex[1:])}
	}
	return &Ident{Name: hex}
}

func isHexDigit(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

func (p *parser) parseIdentValue() Expr {
	name := p.scanIdentifier()
	// namespaced access: ns.func(  or  ns.$var
	if p.peek() == '.' {
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
		return &FuncCall{Name: name, Args: p.parseArgList()}
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
	case "calc", "clamp", "env":
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

func (p *parser) parseArgList() *ArgList {
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
		if p.match("...") {
			arg.Spread = true
			p.ws()
		}
		al.Args = append(al.Args, arg)
		if p.peek() == ',' {
			p.next()
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
