// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import "strings"

// This file implements the modern CSS if() function, distinct from Sass's
// legacy if($condition, $if-true, $if-false) control function. The modern form
// is a series of "condition: value" branches separated by ";", e.g.
//
//	if(sass($x): a; css(): b; else: c)
//
// A sass() clause is evaluated at compile time; any other clause (a plain-CSS
// function such as css(), var(--x), style(), media()) is opaque and preserved
// verbatim. When every relevant clause resolves at compile time the whole call
// collapses to the winning branch's value; otherwise it re-serialises to a
// canonical CSS if(). This tracks dart-sass's IfExpression semantics exactly.

// IfCssExpr is a modern CSS if() expression: an ordered list of branches.
type IfCssExpr struct{ Branches []IfCssBranch }

// IfCssBranch is one "condition: value" pair. A nil Cond is the else branch.
type IfCssBranch struct {
	Cond  ifCond
	Value Expr
}

func (*IfCssExpr) expr() {}

// ifCond is a node in a CSS if() condition tree.
type ifCond interface {
	ifCond()
	// isArbitrarySubstitution reports whether this clause may expand to multiple
	// CSS tokens at render time (if()/var()/attr()/custom properties, or raw
	// interpolation), which governs whether adjacent clauses may be juxtaposed.
	isArbitrarySubstitution() bool
}

type ifCondParen struct{ inner ifCond }
type ifCondNot struct{ inner ifCond }
type ifCondOp struct {
	op    string // "and" | "or"
	items []ifCond
}
type ifCondFunc struct {
	name []any // interpolation parts (string | *InterpExpr)
	args []any
}
type ifCondSass struct{ expr Expr }
type ifCondRaw struct{ parts []any }

func (*ifCondParen) ifCond() {}
func (*ifCondNot) ifCond()   {}
func (*ifCondOp) ifCond()    {}
func (*ifCondFunc) ifCond()  {}
func (*ifCondSass) ifCond()  {}
func (*ifCondRaw) ifCond()   {}

func (*ifCondParen) isArbitrarySubstitution() bool { return false }
func (*ifCondNot) isArbitrarySubstitution() bool   { return false }
func (*ifCondOp) isArbitrarySubstitution() bool    { return false }
func (*ifCondSass) isArbitrarySubstitution() bool  { return false }
func (*ifCondRaw) isArbitrarySubstitution() bool   { return true }
func (f *ifCondFunc) isArbitrarySubstitution() bool {
	plain, ok := plainParts(f.name)
	if !ok {
		return false
	}
	switch strings.ToLower(plain) {
	case "if", "var", "attr":
		return true
	}
	return strings.HasPrefix(plain, "--")
}

// plainParts returns the concatenated literal text of an interpolation part
// list, and false if any part is a dynamic interpolation.
func plainParts(parts []any) (string, bool) {
	var sb strings.Builder
	for _, p := range parts {
		s, ok := p.(string)
		if !ok {
			return "", false
		}
		sb.WriteString(s)
	}
	return sb.String(), true
}

// --- parser ---

// tryParseArgList attempts a normal (legacy) argument list, recovering a parse
// failure so the caller can fall back to the modern if() grammar. It reports
// success; on failure the parser position is left for the caller to restore.
func (p *parser) tryParseArgList() (al *ArgList, ok bool) {
	savedArith := p.arith
	defer func() {
		if r := recover(); r != nil {
			_ = rethrowIfNotSass(r) // re-panic non-Sass errors (real bugs)
			p.arith = savedArith
			al, ok = nil, false
		}
	}()
	return p.parseArgList(), true
}

// parseModernIf parses a modern CSS if() starting at the opening "(".
func (p *parser) parseModernIf() Expr {
	p.next() // (
	p.ws()
	var branches []IfCssBranch
	for p.peek() != ')' {
		var cond ifCond
		if !p.scanIdentEq("else", false) {
			cond = p.ifConditionExpression()
		}
		p.ws()
		if p.peek() != ':' {
			p.fail("Expected \":\".")
		}
		p.next()
		p.ws()
		val := p.parseExpression()
		branches = append(branches, IfCssBranch{Cond: cond, Value: val})
		p.ws()
		if p.peek() != ';' {
			break
		}
		p.next()
		p.ws()
	}
	if p.peek() != ')' {
		p.fail("Expected \")\".")
	}
	p.next()
	return &IfCssExpr{Branches: branches}
}

// scanIdentEq consumes an identifier iff it equals kw (case-insensitively unless
// cs), returning whether it matched. The position is unchanged on no match.
func (p *parser) scanIdentEq(kw string, cs bool) bool {
	c := p.peek()
	if !(isNameStart(c) || c == '\\' || c == '-') {
		return false
	}
	save := p.pos
	id := p.scanIdentifier()
	if id == kw || (!cs && strings.EqualFold(id, kw)) {
		return true
	}
	p.pos = save
	return false
}

func (p *parser) ifConditionExpression() ifCond {
	if p.scanIdentEq("not", false) {
		p.ws()
		return &ifCondNot{inner: p.ifGroup()}
	}
	groups := []ifCond{p.ifGroup()}
	op := ""
	p.ws()
	for {
		switch {
		case op != "or" && p.scanIdentEq("and", false):
			p.ws()
			if op == "" {
				op = "and"
			}
			groups = append(groups, p.ifGroup())
		case op != "and" && p.scanIdentEq("or", false):
			p.ws()
			if op == "" {
				op = "or"
			}
			groups = append(groups, p.ifGroup())
		case ifJuxtaposable(p.peek()) && groups[len(groups)-1].isArbitrarySubstitution():
			return p.ifConditionRaw(collapseGroups(groups, op), p.ifGroup())
		default:
			if sub := p.tryArbitrarySubstitution(); sub != nil {
				return p.ifConditionRaw(collapseGroups(groups, op), sub)
			}
			return collapseGroups(groups, op)
		}
		p.ws()
	}
}

// ifJuxtaposable reports whether c can begin a clause juxtaposed to a preceding
// arbitrary-substitution clause (i.e. it is not a boundary character).
func ifJuxtaposable(c byte) bool {
	return c != ')' && c != ':' && c != ';' && c != 0
}

func collapseGroups(groups []ifCond, op string) ifCond {
	if len(groups) == 1 {
		return groups[0]
	}
	return &ifCondOp{op: op, items: groups}
}

// ifConditionRaw consumes the rest of a condition that mixes arbitrary
// substitutions with operators/adjacent clauses into a single raw-text clause.
func (p *parser) ifConditionRaw(preceding, next ifCond) ifCond {
	buf := append([]any{}, p.condToInterp(preceding)...)
	buf = appendPart(buf, " ")
	buf = append(buf, p.condToInterp(next)...)
	lastGroup := next
	op := ""
	if o, ok := preceding.(*ifCondOp); ok {
		op = o.op
	}
	p.ws()
	for {
		switch {
		case op != "or" && p.scanIdentEq("and", false):
			p.ws()
			if op == "" {
				op = "and"
			}
			g := p.ifGroup()
			buf = appendPart(buf, " and ")
			buf = append(buf, p.condToInterp(g)...)
		case op != "and" && p.scanIdentEq("or", false):
			p.ws()
			if op == "" {
				op = "or"
			}
			g := p.ifGroup()
			buf = appendPart(buf, " or ")
			buf = append(buf, p.condToInterp(g)...)
		case ifJuxtaposable(p.peek()) && lastGroup.isArbitrarySubstitution():
			g := p.ifGroup()
			lastGroup = g
			buf = appendPart(buf, " ")
			buf = append(buf, p.condToInterp(g)...)
		default:
			if sub := p.tryArbitrarySubstitution(); sub != nil {
				lastGroup = sub
				buf = appendPart(buf, " ")
				buf = append(buf, p.condToInterp(sub)...)
				p.ws()
				continue
			}
			return &ifCondRaw{parts: buf}
		}
		p.ws()
	}
}

func (p *parser) ifGroup() ifCond {
	switch {
	case p.peek() == '(':
		p.next()
		p.ws()
		inner := p.ifConditionExpression()
		p.ws()
		if p.peek() != ')' {
			p.fail("Expected \")\".")
		}
		p.next()
		return &ifCondParen{inner: inner}
	case p.scanIdentEq("sass", true):
		if p.peek() != '(' {
			p.fail("Expected \"(\".")
		}
		p.next()
		p.ws()
		e := p.parseExpression()
		p.ws()
		if p.peek() != ')' {
			p.fail("Expected \")\".")
		}
		p.next()
		return &ifCondSass{expr: e}
	default:
		name := p.interpolatedIdent()
		if _, plain := plainParts(name); !plain && p.peek() != '(' {
			return &ifCondRaw{parts: name}
		}
		if p.peek() != '(' {
			p.fail("Expected \"(\".")
		}
		p.next()
		args := p.captureIfArgs()
		return &ifCondFunc{name: name, args: args}
	}
}

func (p *parser) tryArbitrarySubstitution() ifCond {
	if p.peek() == '#' && p.peekAt(1) == '{' {
		p.pos += 2
		p.ws()
		e := p.parseExpression()
		p.ws()
		if p.peek() != '}' {
			p.fail("Expected \"}\".")
		}
		p.next()
		return &ifCondRaw{parts: []any{&InterpExpr{Expr: e}}}
	}
	save := p.pos
	var name []any
	switch {
	case p.scanIdentEq("if", false):
		name = []any{"if"}
	case p.scanIdentEq("var", false):
		name = []any{"var"}
	case p.scanIdentEq("attr", false):
		name = []any{"attr"}
	case p.peek() == '-' && p.peekAt(1) == '-':
		name = p.interpolatedIdent()
	default:
		return nil
	}
	if p.peek() != '(' {
		p.pos = save
		return nil
	}
	p.next()
	args := p.captureIfArgs()
	return &ifCondFunc{name: name, args: args}
}

// interpolatedIdent reads a CSS identifier that may embed #{} interpolations,
// returning it as an interpolation part list.
func (p *parser) interpolatedIdent() []any {
	var parts []any
	var sb strings.Builder
	for {
		if p.peek() == '#' && p.peekAt(1) == '{' {
			if sb.Len() > 0 {
				parts = append(parts, sb.String())
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
			parts = append(parts, &InterpExpr{Expr: e})
			continue
		}
		c := p.peek()
		if c == '\\' {
			sb.WriteString(p.scanEscape())
			continue
		}
		if isNameChar(c) {
			sb.WriteByte(p.next())
			continue
		}
		break
	}
	if sb.Len() > 0 {
		parts = append(parts, sb.String())
	}
	if len(parts) == 0 {
		p.fail("Expected identifier.")
	}
	return parts
}

// captureIfArgs captures a function's balanced argument text (evaluating only
// #{} interpolations) up to the matching ")", which it consumes.
func (p *parser) captureIfArgs() []any {
	var parts []any
	var sb strings.Builder
	flush := func() {
		if sb.Len() > 0 {
			parts = append(parts, sb.String())
			sb.Reset()
		}
	}
	depth := 0
	for !p.eof() {
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
		if c == '"' || c == '\'' {
			sb.WriteString(p.scanQuotedRaw())
			continue
		}
		if c == '(' || c == '[' || c == '{' {
			depth++
		} else if c == ']' || c == '}' {
			depth--
		} else if c == ')' {
			if depth == 0 {
				p.next()
				break
			}
			depth--
		}
		sb.WriteByte(p.next())
	}
	flush()
	return trimEdgeWhitespace(parts)
}

// trimEdgeWhitespace strips leading/trailing whitespace from the literal edges
// of an interpolation part list, matching how dart-sass renders a CSS special
// function's arguments (a whitespace-only body becomes empty).
func trimEdgeWhitespace(parts []any) []any {
	if len(parts) == 0 {
		return parts
	}
	if s, ok := parts[0].(string); ok {
		parts[0] = strings.TrimLeft(s, " \t\r\n\f")
	}
	last := len(parts) - 1
	if s, ok := parts[last].(string); ok {
		parts[last] = strings.TrimRight(s, " \t\r\n\f")
	}
	var out []any
	for _, p := range parts {
		if s, ok := p.(string); ok && s == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}

func appendPart(parts []any, s string) []any {
	if n := len(parts); n > 0 {
		if last, ok := parts[n-1].(string); ok {
			parts[n-1] = last + s
			return parts
		}
	}
	return append(parts, s)
}

// condToInterp flattens a condition subtree to interpolation parts for inclusion
// in a raw clause. A sass() clause is illegal inside a raw clause (it cannot be
// re-serialised), matching dart-sass.
func (p *parser) condToInterp(c ifCond) []any {
	switch n := c.(type) {
	case *ifCondParen:
		out := appendPart(nil, "(")
		out = append(out, p.condToInterp(n.inner)...)
		return appendPart(out, ")")
	case *ifCondNot:
		out := appendPart(nil, "not ")
		return append(out, p.condToInterp(n.inner)...)
	case *ifCondOp:
		var out []any
		for i, it := range n.items {
			if i > 0 {
				out = appendPart(out, " "+n.op+" ")
			}
			out = append(out, p.condToInterp(it)...)
		}
		return out
	case *ifCondFunc:
		out := append([]any{}, n.name...)
		out = appendPart(out, "(")
		out = append(out, n.args...)
		return appendPart(out, ")")
	case *ifCondRaw:
		return n.parts
	default: // *ifCondSass
		p.fail("if() conditions with arbitrary substitutions may not contain sass() expressions.")
		return nil
	}
}

// --- evaluator ---

// evalIfCss evaluates a modern CSS if(): it resolves as many branches as
// possible at compile time, returning the winning value, or re-serialises the
// residual branches into a canonical CSS if() string.
func (e *evaluator) evalIfCss(x *IfCssExpr) Value {
	type resolved struct {
		cond string
		val  Value
	}
	var results []resolved
	started := false
	for _, br := range x.Branches {
		var r any = true
		if br.Cond != nil {
			r = e.evalIfCond(br.Cond)
		}
		if s, ok := r.(string); ok {
			started = true
			results = append(results, resolved{cond: s, val: e.evalExpr(br.Value)})
			continue
		}
		if r.(bool) {
			if started {
				results = append(results, resolved{cond: "else", val: e.evalExpr(br.Value)})
			} else {
				return e.evalExpr(br.Value)
			}
		}
	}
	if !started {
		return sassNull
	}
	var sb strings.Builder
	sb.WriteString("if(")
	for i, res := range results {
		if i > 0 {
			sb.WriteString("; ")
		}
		sb.WriteString(res.cond)
		sb.WriteString(": ")
		sb.WriteString(serializeValue(res.val, false))
	}
	sb.WriteByte(')')
	return &SassString{Text: sb.String(), Quoted: false}
}

// evalIfCond returns either a string (an unresolved CSS clause) or a bool (a
// compile-time-resolved sass() clause).
func (e *evaluator) evalIfCond(c ifCond) any {
	switch n := c.(type) {
	case *ifCondParen:
		if s, ok := e.evalIfCond(n.inner).(string); ok {
			return "(" + s + ")"
		}
		return e.evalIfCond(n.inner)
	case *ifCondNot:
		r := e.evalIfCond(n.inner)
		if s, ok := r.(string); ok {
			return "not " + s
		}
		return !r.(bool)
	case *ifCondOp:
		return e.evalIfOp(n)
	case *ifCondFunc:
		return e.performIfInterp(n.name) + "(" + e.performIfInterp(n.args) + ")"
	case *ifCondSass:
		return e.evalExpr(n.expr).isTruthy()
	default: // *ifCondRaw
		return e.performIfInterp(c.(*ifCondRaw).parts)
	}
}

func (e *evaluator) evalIfOp(n *ifCondOp) any {
	type kept struct {
		cond ifCond
		s    string
	}
	var values []kept
	for _, it := range n.items {
		r := e.evalIfCond(it)
		if s, ok := r.(string); ok {
			values = append(values, kept{cond: it, s: s})
			continue
		}
		b := r.(bool)
		if !b && n.op == "and" {
			return false
		}
		if b && n.op == "or" {
			return true
		}
	}
	if values == nil {
		return n.op == "and"
	}
	// A lone surviving parenthesized clause needs no parentheses here: the
	// operation is itself a group, so the redundant parens are dropped.
	if len(values) == 1 {
		if _, ok := values[0].cond.(*ifCondParen); ok {
			s := values[0].s
			return s[1 : len(s)-1]
		}
	}
	parts := make([]string, len(values))
	for i, v := range values {
		parts[i] = v.s
	}
	return strings.Join(parts, " "+n.op+" ")
}

// performIfInterp renders an interpolation part list to text, evaluating #{}
// expressions.
func (e *evaluator) performIfInterp(parts []any) string {
	var sb strings.Builder
	for _, p := range parts {
		switch v := p.(type) {
		case string:
			sb.WriteString(v)
		case *InterpExpr:
			sb.WriteString(e.stringifyInterp(e.evalExpr(v.Expr)))
		}
	}
	return sb.String()
}
