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
		if p.peek() == '"' || p.peek() == '\'' {
			url := p.scanQuotedString()
			p.ws()
			// plain CSS import? url ending .css or with media query
			if isPlainImport(url, p) {
				rest := p.parseInterpolatedText(func(pp *parser) bool {
					c := pp.peek()
					return c == ',' || c == ';' || c == '}' || c == 0
				})
				txt, _ := rest.isPlain()
				imp.Imports = append(imp.Imports, ImportItem{URL: url, Plain: true, RawText: strings.TrimSpace(txt)})
			} else {
				imp.Imports = append(imp.Imports, ImportItem{URL: url})
			}
		} else if pfx := p.pos; p.match("url(") || p.match("URL(") {
			// url(...) form -> plain import; the url() wrapper is preserved
			// verbatim so it round-trips as `@import url(...)`.
			depth := 1
			for !p.eof() && depth > 0 {
				c := p.next()
				if c == '(' {
					depth++
				} else if c == ')' {
					depth--
				}
			}
			imp.Imports = append(imp.Imports, ImportItem{URL: p.src[pfx:p.pos], Plain: true, RawText: ""})
		} else {
			p.fail("Expected string.")
		}
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

func isPlainImport(url string, p *parser) bool {
	if strings.HasSuffix(url, ".css") {
		return true
	}
	if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") || strings.HasPrefix(url, "//") {
		return true
	}
	// media query following the url (not ',' ';' '}')
	save := p.pos
	p.ws()
	c := p.peek()
	p.pos = save
	if c != ',' && c != ';' && c != '}' && c != 0 {
		return true
	}
	return false
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
