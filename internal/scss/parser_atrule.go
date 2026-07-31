// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import "strings"

func (p *parser) parseAtRule() Stmt {
	p.next() // @
	name := p.scanIdentifier()
	switch name {
	case "mixin":
		return p.parseMixinDef()
	case "include":
		return p.parseInclude()
	case "function":
		return p.parseFunctionDef()
	case "return":
		return p.parseReturn()
	case "if":
		return p.parseIf()
	case "each":
		return p.parseEach()
	case "for":
		return p.parseFor()
	case "while":
		return p.parseWhile()
	case "at-root":
		return p.parseAtRoot()
	case "media":
		return p.parseMedia()
	case "supports":
		return p.parseSupports()
	case "extend":
		return p.parseExtend()
	case "content":
		return p.parseContent()
	case "import":
		return p.parseImport()
	case "use":
		return p.parseUse()
	case "forward":
		return p.parseForward()
	case "warn":
		return &Warn{Value: p.parseAtRuleExprEnd()}
	case "debug":
		return &Debug{Value: p.parseAtRuleExprEnd()}
	case "error":
		return &ErrorStmt{Value: p.parseAtRuleExprEnd()}
	default:
		return p.parseGenericAtRule(name)
	}
}

func (p *parser) parseAtRuleExprEnd() Expr {
	p.ws()
	e := p.parseExpression()
	p.consumeStatementEnd()
	return e
}

func (p *parser) parseMixinDef() Stmt {
	p.ws()
	name := p.scanIdentifier()
	var params *ParamList
	p.ws()
	if p.peek() == '(' {
		params = p.parseParamList()
	} else {
		params = &ParamList{}
	}
	body := p.parseBlock()
	return &MixinDef{Name: name, Params: params, Body: body}
}

func (p *parser) parseInclude() Stmt {
	p.ws()
	ns, name := p.scanNamespacedName()
	inc := &Include{Namespace: ns, Name: name, Args: &ArgList{}}
	p.ws()
	if p.peek() == '(' {
		inc.Args = p.parseArgList()
		p.ws()
	}
	if p.match("using") {
		p.ws()
		inc.ContentParams = p.parseParamList()
		p.ws()
	}
	if p.peek() == '{' {
		if inc.ContentParams == nil {
			inc.ContentParams = &ParamList{}
		}
		inc.Content = p.parseBlock()
		if inc.Content == nil {
			// An empty "{}" block is still a content block: keep it non-nil so
			// meta.content-exists() reports true.
			inc.Content = []Stmt{}
		}
	} else {
		p.consumeStatementEnd()
	}
	return inc
}

func (p *parser) parseFunctionDef() Stmt {
	p.ws()
	name := p.scanIdentifier()
	p.ws()
	params := p.parseParamList()
	body := p.parseBlock()
	return &FunctionDef{Name: name, Params: params, Body: body}
}

func (p *parser) parseReturn() Stmt {
	p.ws()
	e := p.parseExpression()
	p.consumeStatementEnd()
	return &Return{Value: e}
}

func (p *parser) parseIf() Stmt {
	node := &If{}
	p.ws()
	cond := p.parseExpression()
	body := p.parseBlock()
	node.Clauses = append(node.Clauses, IfClause{Cond: cond, Body: body})
	for {
		save := p.pos
		p.ws()
		if !p.match("@else") {
			p.pos = save
			break
		}
		p.ws()
		if p.match("if") {
			p.ws()
			c := p.parseExpression()
			b := p.parseBlock()
			node.Clauses = append(node.Clauses, IfClause{Cond: c, Body: b})
			continue
		}
		node.Else = p.parseBlock()
		node.HasElse = true
		break
	}
	return node
}

func (p *parser) parseEach() Stmt {
	p.ws()
	var vars []string
	for {
		if p.peek() != '$' {
			p.fail("Expected variable.")
		}
		p.next()
		vars = append(vars, p.scanIdentifier())
		p.ws()
		if p.peek() == ',' {
			p.next()
			p.ws()
			continue
		}
		break
	}
	if !p.match("in") {
		p.fail("Expected \"in\".")
	}
	p.ws()
	list := p.parseExpression()
	body := p.parseBlock()
	return &Each{Vars: vars, List: list, Body: body}
}

func (p *parser) parseFor() Stmt {
	p.ws()
	if p.peek() != '$' {
		p.fail("Expected variable.")
	}
	p.next()
	v := p.scanIdentifier()
	p.ws()
	if !p.match("from") {
		p.fail("Expected \"from\".")
	}
	p.ws()
	from := p.parseForBound()
	through := false
	if p.match("through") {
		through = true
	} else if p.match("to") {
		through = false
	} else {
		p.fail("Expected \"to\" or \"through\".")
	}
	p.ws()
	to := p.parseExpression()
	body := p.parseBlock()
	return &For{Var: v, From: from, To: to, Through: through, Body: body}
}

// parseForBound parses the @for start expression, stopping before to/through.
func (p *parser) parseForBound() Expr {
	return p.parseExpressionStop(func(pp *parser) bool {
		save := pp.pos
		defer func() { pp.pos = save }()
		if pp.match("to") || pp.match("through") {
			c := pp.peek()
			return !isNameChar(c)
		}
		return false
	})
}

func (p *parser) parseWhile() Stmt {
	p.ws()
	cond := p.parseExpression()
	body := p.parseBlock()
	return &While{Cond: cond, Body: body}
}

func (p *parser) parseAtRoot() Stmt {
	p.ws()
	var query *Interp
	if p.peek() == '(' {
		query = p.parseInterpolatedText(func(pp *parser) bool { return pp.peek() == '{' })
		p.ws()
	}
	if p.peek() == '{' {
		return &AtRoot{Query: query, Body: p.parseBlock()}
	}
	// shorthand: @at-root <selector> { ... }
	rule := p.parseStyleRule()
	return &AtRoot{Query: query, Body: []Stmt{rule}}
}

func (p *parser) parseMedia() Stmt {
	q := p.parseInterpolatedText(func(pp *parser) bool { return pp.peek() == '{' })
	body := p.parseBlock()
	return &Media{Query: trimInterp(q), Body: body}
}

func (p *parser) parseExtend() Stmt {
	sel := p.parseInterpolatedText(func(pp *parser) bool {
		c := pp.peek()
		return c == ';' || c == '}' || c == 0
	})
	optional := false
	plain, ok := sel.isPlain()
	if ok && strings.Contains(plain, "!optional") {
		plain = strings.Replace(plain, "!optional", "", 1)
		sel = literalInterp(plain)
		optional = true
	}
	p.consumeStatementEnd()
	return &Extend{Selector: trimInterp(sel), Optional: optional}
}

func (p *parser) parseContent() Stmt {
	cs := &ContentStmt{Args: &ArgList{}}
	p.ws()
	if p.peek() == '(' {
		cs.Args = p.parseArgList()
	}
	p.consumeStatementEnd()
	return cs
}

func (p *parser) parseImport() Stmt {
	imp := &Import{}
	for {
		p.ws()
		var item ImportItem
		urlForm := false
		if p.peek() == '"' || p.peek() == '\'' {
			item.URL = p.scanQuotedString()
		} else if pfx := p.pos; p.match("url(") || p.match("URL(") {
			// url(...) form -> always a plain passthrough import; the url() wrapper
			// is preserved verbatim so it round-trips as `@import url(...)`.
			depth := 1
			for !p.eof() && depth > 0 {
				c := p.next()
				if c == '(' {
					depth++
				} else if c == ')' {
					depth--
				}
			}
			item.URL = p.src[pfx:p.pos]
			item.Plain = true
			urlForm = true
		} else {
			p.fail("Expected string.")
		}
		p.ws()
		mods := p.tryImportModifiers()
		item.Mods = mods
		if !urlForm && (isPlainImportURL(item.URL) || mods != nil) {
			item.Plain = true
		}
		imp.Imports = append(imp.Imports, item)
		p.ws()
		if p.peek() == ',' {
			p.next()
			continue
		}
		break
	}
	p.consumeStatementEnd()
	return imp
}

// isPlainImportURL mirrors dart-sass isPlainImportUrl: a URL that resolves to a
// plain-CSS import (rather than a Sass module) when it ends in `.css`, is
// protocol-relative (`//`), or is an absolute `http(s)://` URL.
func isPlainImportURL(url string) bool {
	if strings.HasSuffix(url, ".css") {
		return true
	}
	return strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") || strings.HasPrefix(url, "//")
}

// tryImportModifiers mirrors dart-sass StylesheetParser.tryImportModifiers: it
// parses the media/supports modifiers after an @import URL into an Interp whose
// parts serialize canonically at evaluation time. It returns nil when no
// modifier is present.
func (p *parser) tryImportModifiers() *Interp {
	if !p.lookingAtInterpolatedIdentifier() && p.peek() != '(' {
		return nil
	}
	var parts []any
	space := func() {
		if len(parts) > 0 {
			parts = append(parts, " ")
		}
	}
	for {
		switch {
		case p.lookingAtInterpolatedIdentifier():
			space()
			ident := p.interpolatedIdentifier()
			parts = append(parts, ident.Parts...)
			name, isPlain := ident.isPlain()
			lname := strings.ToLower(name)
			if !(isPlain && lname == "and") && p.peek() == '(' {
				p.next() // (
				if isPlain && lname == "supports" {
					query := p.parseImportSupportsQuery()
					if _, isDecl := query.(*SupportsDecl); !isDecl {
						parts = append(parts, "(")
						parts = append(parts, &supportsPart{Cond: query}, ")")
					} else {
						parts = append(parts, &supportsPart{Cond: query})
					}
				} else {
					// An unknown modifier function's arguments are captured as a raw
					// declaration value that preserves newlines verbatim (dart-sass
					// emits them literally), unlike a supports function whose value is
					// re-serialized with collapsed whitespace.
					parts = append(parts, "(")
					val := p.interpolatedDeclarationValue(true, true, true, false)
					parts = append(parts, val.Parts...)
					parts = append(parts, ")")
				}
				if p.peek() != ')' {
					p.fail("Expected \")\".")
				}
				p.next()
				p.ws()
			} else {
				p.ws()
				if p.peek() == ',' {
					p.next()
					parts = append(parts, ", ", &mediaPart{Query: p.captureMediaQueryList()})
					return &Interp{Parts: parts}
				}
			}
		case p.peek() == '(':
			space()
			parts = append(parts, &mediaPart{Query: p.captureMediaQueryList()})
			return &Interp{Parts: parts}
		default:
			return &Interp{Parts: parts}
		}
	}
}

// captureMediaQueryList captures the remainder of an @import modifier list as a
// media-query list, up to the statement terminator. Its text is normalized
// through the media-query serializer at evaluation time.
func (p *parser) captureMediaQueryList() *Interp {
	return p.parseInterpolatedText(func(pp *parser) bool {
		c := pp.peek()
		return c == ';' || c == '}' || c == 0
	})
}

// parseImportSupportsQuery mirrors dart-sass StylesheetParser._importSupportsQuery:
// the supports condition following `supports(` in an @import modifier list.
func (p *parser) parseImportSupportsQuery() SupportsCond {
	p.ws()
	if p.scanSupportsKeyword("not") {
		p.ws()
		return &SupportsNegation{Cond: p.parseSupportsConditionInParens()}
	}
	if p.peek() == '(' {
		return p.parseSupportsCondition(false)
	}
	if p.lookingAtInterpolatedIdentifier() {
		start := p.pos
		name := p.interpolatedIdentifier()
		if p.peek() == '(' {
			p.next()
			val := p.interpolatedDeclarationValue(true, true, true, true)
			if p.peek() != ')' {
				p.fail("Expected \")\".")
			}
			p.next()
			return &SupportsFunc{Name: name, Args: val}
		}
		p.pos = start
	}
	var name Expr
	custom := false
	if p.looksLikeCustomProperty() {
		name = &Ident{Name: p.scanIdentifier()}
		custom = true
	} else {
		name = p.parseExpression()
	}
	p.ws()
	if p.peek() != ':' {
		p.fail("Expected \":\".")
	}
	p.next()
	if custom {
		return &SupportsDecl{Name: name, RawValue: p.interpolatedDeclarationValue(true, false, true, true), Custom: true}
	}
	p.ws()
	return &SupportsDecl{Name: name, Value: p.parseExpression()}
}

func (p *parser) parseUse() Stmt {
	p.ws()
	url := p.scanQuotedString()
	use := &Use{URL: url}
	p.ws()
	if p.match("as") {
		p.ws()
		if p.peek() == '*' {
			p.next()
			use.NoNS = true
		} else {
			use.Namespace = p.scanIdentifier()
		}
		p.ws()
	}
	if p.match("with") {
		p.ws()
		use.Config = p.parseConfig()
	}
	p.consumeStatementEnd()
	return use
}

func (p *parser) parseForward() Stmt {
	p.ws()
	url := p.scanQuotedString()
	fwd := &Forward{URL: url}
	for {
		p.ws()
		if p.match("as") {
			p.ws()
			prefix := p.scanIdentifier()
			if p.peek() == '*' {
				p.next()
			}
			fwd.Prefix = prefix
			continue
		}
		if p.match("show") {
			fwd.HasShow = true
			fwd.Show = p.parseMemberList()
			continue
		}
		if p.match("hide") {
			fwd.HasHide = true
			fwd.Hide = p.parseMemberList()
			continue
		}
		if p.match("with") {
			p.ws()
			fwd.Config = p.parseConfig()
			continue
		}
		break
	}
	p.consumeStatementEnd()
	return fwd
}

func (p *parser) parseMemberList() []string {
	var out []string
	for {
		p.ws()
		if p.peek() == '$' {
			p.next()
			out = append(out, "$"+p.scanIdentifier())
		} else {
			out = append(out, p.scanIdentifier())
		}
		p.ws()
		if p.peek() == ',' {
			p.next()
			continue
		}
		break
	}
	return out
}

func (p *parser) parseConfig() []ConfigVar {
	if p.peek() != '(' {
		p.fail("Expected \"(\".")
	}
	p.next()
	var cfg []ConfigVar
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
		p.ws()
		if p.peek() != ':' {
			p.fail("Expected \":\".")
		}
		p.next()
		p.ws()
		val := p.parseSpaceList(alwaysFalse)
		def := false
		p.ws()
		if p.match("!default") {
			def = true
			p.ws()
		}
		cfg = append(cfg, ConfigVar{Name: name, Value: val, Default: def})
		p.ws()
		if p.peek() == ',' {
			p.next()
			continue
		}
	}
	return cfg
}

func (p *parser) parseGenericAtRule(name string) Stmt {
	value := p.parseInterpolatedText(func(pp *parser) bool {
		c := pp.peek()
		return c == '{' || c == ';' || c == '}' || c == 0
	})
	v := trimInterp(value)
	if plain, ok := v.isPlain(); ok && plain == "" {
		v = nil
	}
	if p.peek() == '{' {
		body := p.parseBlock()
		return &AtRule{Name: name, Value: v, Body: body}
	}
	p.consumeStatementEnd()
	return &AtRule{Name: name, Value: v, NoBody: true}
}

func (p *parser) scanNamespacedName() (ns, name string) {
	first := p.scanIdentifier()
	if p.peek() == '.' {
		p.next()
		return first, p.scanIdentifier()
	}
	return "", first
}

func (p *parser) scanQuotedString() string {
	q := p.peek()
	if q != '"' && q != '\'' {
		p.fail("Expected string.")
	}
	p.next()
	var sb strings.Builder
	for !p.eof() {
		c := p.next()
		if c == '\\' && !p.eof() {
			sb.WriteByte(p.next())
			continue
		}
		if c == q {
			break
		}
		sb.WriteByte(c)
	}
	return sb.String()
}
