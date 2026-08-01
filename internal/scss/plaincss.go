// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import (
	"fmt"
	"strings"
)

// This file implements dart-sass's plain-CSS parsing mode. A file resolved with
// a ".css" extension (loaded through @use/@forward or a top-level @import) is
// parsed as plain CSS, not SCSS: SassScript is not evaluated, functions are not
// called, operators/parentheses/variables/interpolation are rejected, nested
// style rules are preserved verbatim (never flattened to descendant selectors),
// and an at-rule that is a direct child of a top-level style rule bubbles to the
// document root, wrapping a copy of that rule around its body. Values are
// re-serialised through dart's canonical whitespace normalisation.

type pcssParser struct {
	src string
	pos int
	// funcDepth is the nesting depth inside a plain-CSS custom-function body
	// (`@function --a() { … }`). Inside such a body a descriptor declaration
	// (`result: …`) takes a verbatim token-stream value that may include braces
	// and other CSS-invalid characters, exactly as dart-sass parses it, so a
	// value like `{}#&%^*` is captured whole rather than mistaken for a nested
	// style rule.
	funcDepth int
}

// parsePlainCSS parses plain-CSS source into the output CSS node tree, applying
// top-level at-rule bubbling and plain-@import hoisting.
func parsePlainCSS(src string) (nodes []cssNode, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = rethrowIfNotSass(r)
		}
	}()
	p := &pcssParser{src: src}
	nodes = p.statements(true)
	nodes = bubblePlainTop(nodes)
	return nodes, nil
}

func (p *pcssParser) fail(format string, args ...any) {
	line, col := 1, 1
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

func (p *pcssParser) eof() bool { return p.pos >= len(p.src) }

func (p *pcssParser) peek() byte {
	if p.pos >= len(p.src) {
		return 0
	}
	return p.src[p.pos]
}

func (p *pcssParser) peekAt(n int) byte {
	if p.pos+n >= len(p.src) {
		return 0
	}
	return p.src[p.pos+n]
}

// skipWS advances over whitespace and comments. Loud comments (/* */) are
// returned (in source order) so the caller can preserve them as nodes; silent
// comments (//) and whitespace are discarded.
func (p *pcssParser) skipWS() []cssNode {
	var comments []cssNode
	for !p.eof() {
		c := p.peek()
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f':
			p.pos++
		case c == '/' && p.peekAt(1) == '*':
			start := p.pos
			p.pos += 2
			for !p.eof() && !(p.peek() == '*' && p.peekAt(1) == '/') {
				p.pos++
			}
			if !p.eof() {
				p.pos += 2
			}
			comments = append(comments, &cssComment{text: p.src[start:p.pos]})
		default:
			return comments
		}
	}
	return comments
}

// statements parses a sequence of plain-CSS statements. When top is false it
// stops at (and does not consume) the closing brace of an enclosing block.
func (p *pcssParser) statements(top bool) []cssNode {
	var nodes []cssNode
	for {
		nodes = append(nodes, p.skipWS()...)
		if p.eof() {
			break
		}
		if p.peek() == ';' { // empty statement / trailing separator
			p.pos++
			continue
		}
		if p.peek() == '}' {
			if top {
				p.fail("unexpected %q", "}")
			}
			break
		}
		n := p.statement()
		// dart-sass consumes a top-level `@charset` (case-sensitively) for encoding
		// detection and never emits it; the serializer re-derives its own @charset
		// only for non-ASCII output. This holds in plain-CSS mode too.
		if top {
			if ar, ok := n.(*cssAtRule); ok && ar.name == "charset" {
				continue
			}
		}
		nodes = append(nodes, n)
	}
	return nodes
}

// statement parses a single at-rule, declaration or style rule.
func (p *pcssParser) statement() cssNode {
	if p.peek() == '@' {
		return p.atRule()
	}
	// Inside a custom-function body every descriptor is a declaration whose
	// value is a verbatim token stream (it may contain `{}` and other CSS-
	// invalid characters), so parse `name: token-stream` without letting a `{`
	// in the value be mistaken for the start of a nested block.
	if p.funcDepth > 0 {
		if d, ok := p.funcDeclaration(); ok {
			return d
		}
	}
	// Custom properties (--foo) take a verbatim token-stream value that may
	// include braces, so they are recognised before the prelude scan.
	if save := p.pos; p.peek() == '-' && p.peekAt(1) == '-' {
		name := p.readIdent()
		p.skipInlineWS()
		if p.peek() == ':' {
			p.pos++
			raw := p.readCustomValue()
			return &cssDeclaration{name: name, raw: raw, custom: true}
		}
		p.pos = save
	}
	prelude, term := p.scanPrelude()
	if term == '{' {
		p.pos++ // consume '{'
		body := p.statements(false)
		if p.peek() != '}' {
			p.fail("expected %q", "}")
		}
		p.pos++ // consume '}'
		return &cssStyleRule{raw: true, rawSel: normSelector(prelude), nodes: body}
	}
	// declaration
	if term == ';' {
		p.pos++ // consume ';'
	}
	idx := findDeclColon(prelude)
	if idx < 0 {
		p.fail("expected %q", ":")
	}
	name := strings.TrimSpace(prelude[:idx])
	val := normPlainValue(prelude[idx+1:])
	return &cssDeclaration{name: name, value: &SassString{Text: val}}
}

// atRule parses a plain-CSS at-rule, preserving its name (case included) and
// normalising its prelude and (recursively parsed) body.
func (p *pcssParser) atRule() cssNode {
	p.pos++ // consume '@'
	name := p.readIdent()
	prelude, term := p.scanPrelude()
	params := strings.TrimSpace(prelude)
	switch {
	case strings.EqualFold(name, "import"):
		params = collapseWS(params)
	case strings.EqualFold(name, "media") && params != "":
		params = normMediaParams(params)
	default:
		params = normAtParams(params)
	}
	at := &cssAtRule{name: name, params: params}
	if term == '{' {
		p.pos++
		at.hasBody = true
		if strings.EqualFold(name, "function") {
			p.funcDepth++
			at.nodes = p.statements(false)
			p.funcDepth--
		} else {
			at.nodes = p.statements(false)
		}
		if p.peek() != '}' {
			p.fail("expected %q", "}")
		}
		p.pos++
	} else if term == ';' {
		p.pos++
	}
	return at
}

// funcDeclaration parses a `name: token-stream` descriptor inside a plain-CSS
// custom-function body. It returns ok=false (without consuming input) when the
// current statement is not a colon-delimited declaration, so the caller can
// fall back to the general statement parser.
func (p *pcssParser) funcDeclaration() (cssNode, bool) {
	save := p.pos
	start := p.pos
	for !p.eof() {
		switch p.peek() {
		case ':':
			name := strings.TrimSpace(p.src[start:p.pos])
			p.pos++ // consume ':'
			raw := p.readCustomValue()
			return &cssDeclaration{name: name, raw: raw, custom: true}, true
		case '{', ';', '}':
			// Not a colon-delimited declaration (e.g. a nested style rule); let
			// the general statement parser handle it.
			p.pos = save
			return nil, false
		default:
			p.pos++
		}
	}
	p.pos = save
	return nil, false
}

// scanPrelude reads up to (but not consuming) the next top-level '{', ';' or
// '}', or end of input, skipping strings and balanced (), [] groups. It returns
// the raw prelude text and the terminating byte (0 at end of input).
func (p *pcssParser) scanPrelude() (string, byte) {
	start := p.pos
	for !p.eof() {
		c := p.peek()
		switch {
		case c == '"' || c == '\'':
			p.skipString()
		case c == '(':
			p.skipBalanced('(', ')')
		case c == '[':
			p.skipBalanced('[', ']')
		case c == '{' || c == ';' || c == '}':
			return p.src[start:p.pos], c
		default:
			p.pos++
		}
	}
	return p.src[start:p.pos], 0
}

// readCustomValue captures a custom-property value verbatim from just after the
// colon up to the terminating ';' or the enclosing block's '}', tracking nested
// braces/brackets/parens and strings.
func (p *pcssParser) readCustomValue() string {
	start := p.pos
	depth := 0
	for !p.eof() {
		c := p.peek()
		switch {
		case c == '"' || c == '\'':
			p.skipString()
		case c == '{' || c == '(' || c == '[':
			depth++
			p.pos++
		case c == '}' || c == ')' || c == ']':
			if depth == 0 {
				raw := p.src[start:p.pos]
				return raw
			}
			depth--
			p.pos++
		case c == ';':
			if depth == 0 {
				raw := p.src[start:p.pos]
				p.pos++ // consume ';'
				return raw
			}
			p.pos++
		default:
			p.pos++
		}
	}
	return p.src[start:p.pos]
}

func (p *pcssParser) skipString() {
	quote := p.peek()
	p.pos++
	for !p.eof() {
		c := p.peek()
		if c == '\\' {
			p.pos += 2
			continue
		}
		p.pos++
		if c == quote {
			return
		}
	}
}

func (p *pcssParser) skipBalanced(open, close byte) {
	depth := 0
	for !p.eof() {
		c := p.peek()
		switch {
		case c == '"' || c == '\'':
			p.skipString()
		case c == open:
			depth++
			p.pos++
		case c == close:
			depth--
			p.pos++
			if depth == 0 {
				return
			}
		default:
			p.pos++
		}
	}
}

func (p *pcssParser) readIdent() string {
	start := p.pos
	for !p.eof() {
		c := p.peek()
		if isNameByte(c) {
			p.pos++
			continue
		}
		break
	}
	return p.src[start:p.pos]
}

func (p *pcssParser) skipInlineWS() {
	for !p.eof() {
		c := p.peek()
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' {
			p.pos++
			continue
		}
		break
	}
}

func isNameByte(c byte) bool {
	return c == '-' || c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9') || c >= 0x80
}

func isIdentStart(c byte) bool {
	return c == '-' || c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c >= 0x80
}

// findDeclColon locates the property/value separator colon in a declaration
// prelude, honouring the IE-hack leading prefix (a leading ':' immediately
// followed by an identifier is part of the property name) and skipping colons
// inside strings and balanced groups.
func findDeclColon(prelude string) int {
	i := 0
	for i < len(prelude) && isSpace(prelude[i]) {
		i++
	}
	if i < len(prelude) && prelude[i] == ':' && i+1 < len(prelude) && isIdentStart(prelude[i+1]) {
		i++ // leading hack colon is part of the property
	}
	depth := 0
	for ; i < len(prelude); i++ {
		c := prelude[i]
		switch {
		case c == '"' || c == '\'':
			i = skipStringIn(prelude, i)
		case c == '(' || c == '[':
			depth++
		case c == ')' || c == ']':
			if depth > 0 {
				depth--
			}
		case c == ':' && depth == 0:
			return i
		}
	}
	return -1
}

func skipStringIn(s string, i int) int {
	quote := s[i]
	i++
	for i < len(s) {
		if s[i] == '\\' {
			i += 2
			continue
		}
		if s[i] == quote {
			return i
		}
		i++
	}
	return i - 1
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f'
}

// --- selector / prelude normalisation ---

// normSelector collapses whitespace runs to a single space and renders each
// comma as ", ", matching dart's plain-CSS selector serialisation.
func normSelector(s string) string {
	s = strings.TrimSpace(s)
	var b strings.Builder
	space := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if isSpace(c) {
			space = true
			continue
		}
		if c == ',' {
			b.WriteString(", ")
			space = false
			continue
		}
		if space && b.Len() > 0 && b.String()[b.Len()-1] != ' ' {
			b.WriteByte(' ')
		}
		space = false
		b.WriteByte(c)
	}
	return b.String()
}

// collapseWS trims and collapses internal whitespace runs to single spaces.
func collapseWS(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// normMediaParams normalises a media query list through the shared media-query
// parser (lowercasing and/or/not/only and canonicalising whitespace), falling
// back to plain whitespace collapse if the query cannot be parsed.
func normMediaParams(s string) (out string) {
	defer func() {
		if recover() != nil {
			out = collapseWS(s)
		}
	}()
	return mediaQueriesString(parseMediaQueryList(s))
}

// normAtParams normalises an at-rule prelude: whitespace is collapsed and every
// colon is rendered ": " (no space before), matching dart's rendering of
// @media/@supports feature queries. Strings are preserved verbatim.
func normAtParams(s string) string {
	s = collapseWS(s)
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '"' || c == '\'' {
			j := skipStringIn(s, i)
			b.WriteString(s[i : j+1])
			i = j
			continue
		}
		if c == ':' {
			for b.Len() > 0 && b.String()[b.Len()-1] == ' ' {
				// no space before a colon
				buf := b.String()
				b.Reset()
				b.WriteString(buf[:len(buf)-1])
			}
			b.WriteString(": ")
			for i+1 < len(s) && s[i+1] == ' ' {
				i++
			}
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}

// --- value normalisation ---

type pcssVal struct {
	src string
	pos int
}

// normPlainValue re-serialises a plain-CSS declaration value with dart's
// canonical whitespace: comma-separated elements join with ", ", space-separated
// elements with " ", and slash-separated elements with "/" (one slash per source
// slash, surrounding whitespace removed).
func normPlainValue(s string) string {
	v := &pcssVal{src: s}
	return v.list()
}

func (v *pcssVal) eof() bool { return v.pos >= len(v.src) }

func (v *pcssVal) peek() byte {
	if v.pos >= len(v.src) {
		return 0
	}
	return v.src[v.pos]
}

func (v *pcssVal) skipWS() {
	for !v.eof() && isSpace(v.peek()) {
		v.pos++
	}
}

func (v *pcssVal) list() string {
	var segs []string
	for {
		v.skipWS()
		segs = append(segs, v.spaceSlashSeq())
		v.skipWS()
		if v.peek() == ',' {
			v.pos++
			continue
		}
		break
	}
	return strings.Join(segs, ", ")
}

func (v *pcssVal) spaceSlashSeq() string {
	var b strings.Builder
	b.WriteString(v.atom())
	for {
		start := v.pos
		slashes := 0
		for !v.eof() {
			c := v.peek()
			if isSpace(c) {
				v.pos++
				continue
			}
			if c == '/' {
				slashes++
				v.pos++
				continue
			}
			break
		}
		if v.pos == start {
			break
		}
		c := v.peek()
		if v.eof() || c == ',' || c == ')' {
			if slashes > 0 {
				b.WriteString(strings.Repeat("/", slashes))
			}
			break
		}
		if slashes > 0 {
			b.WriteString(strings.Repeat("/", slashes))
		} else {
			b.WriteByte(' ')
		}
		b.WriteString(v.atom())
	}
	return b.String()
}

func (v *pcssVal) atom() string {
	var b strings.Builder
	for !v.eof() {
		c := v.peek()
		if isSpace(c) || c == ',' || c == '/' || c == ')' {
			break
		}
		if c == '"' || c == '\'' {
			b.WriteString(v.stringLit())
			continue
		}
		if c == '(' {
			v.pos++ // consume '('
			name := b.String()
			if strings.EqualFold(lastIdent(name), "url") {
				b.WriteByte('(')
				b.WriteString(v.rawUntilClose())
				b.WriteByte(')')
			} else {
				inner := v.list()
				if v.peek() == ')' {
					v.pos++
				}
				// calc() of a single numeric term simplifies to that term, as
				// dart-sass does even in plain CSS.
				if strings.EqualFold(lastIdent(name), "calc") && isSimpleNumber(inner) {
					b.Reset()
					b.WriteString(name[:len(name)-len("calc")])
					b.WriteString(inner)
				} else {
					b.WriteByte('(')
					b.WriteString(inner)
					b.WriteByte(')')
				}
			}
			continue
		}
		b.WriteByte(c)
		v.pos++
	}
	return b.String()
}

func (v *pcssVal) stringLit() string {
	start := v.pos
	quote := v.peek()
	v.pos++
	for !v.eof() {
		c := v.peek()
		if c == '\\' {
			v.pos += 2
			continue
		}
		v.pos++
		if c == quote {
			break
		}
	}
	return v.src[start:v.pos]
}

func (v *pcssVal) rawUntilClose() string {
	start := v.pos
	depth := 0
	for !v.eof() {
		c := v.peek()
		if c == '"' || c == '\'' {
			v.stringLit()
			continue
		}
		if c == '(' {
			depth++
			v.pos++
			continue
		}
		if c == ')' {
			if depth == 0 {
				raw := v.src[start:v.pos]
				v.pos++ // consume ')'
				return strings.TrimSpace(raw)
			}
			depth--
			v.pos++
			continue
		}
		v.pos++
	}
	return strings.TrimSpace(v.src[start:v.pos])
}

// isSimpleNumber reports whether s is a single numeric term (optional sign,
// digits with optional decimal point, optional %/identifier unit) — the form
// dart-sass reduces a single-argument calc() to.
func isSimpleNumber(s string) bool {
	i := 0
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		i++
	}
	digits := false
	for i < len(s) && ((s[i] >= '0' && s[i] <= '9') || s[i] == '.') {
		if s[i] != '.' {
			digits = true
		}
		i++
	}
	if !digits {
		return false
	}
	if i == len(s) {
		return true
	}
	if s[i] == '%' {
		return i == len(s)-1
	}
	for ; i < len(s); i++ {
		if !isNameByte(s[i]) {
			return false
		}
	}
	return true
}

// lastIdent returns the trailing identifier run of s (used to detect a url(...)
// function whose name may be preceded by other value characters).
func lastIdent(s string) string {
	i := len(s)
	for i > 0 && isNameByte(s[i-1]) {
		i--
	}
	return s[i:]
}

// --- top-level bubbling & import hoisting ---

// bubblePlainTop hoists at-rules out of top-level style rules and moves plain
// @import rules to the document top.
func bubblePlainTop(nodes []cssNode) []cssNode {
	var out []cssNode
	for _, n := range nodes {
		if sr, ok := n.(*cssStyleRule); ok {
			frags := bubbleRule(sr)
			// dart-sass evaluates a plain-CSS file through the normal evaluator, so
			// after each top-level style-rule statement its last emitted node is
			// marked a group end (isGroupEnd) — the last bubble fragment here, be it
			// a trailing rule fragment or the at-rule the rule bubbled into. This
			// lets a plain-CSS module's rules take part in the deferred cross-module
			// blank-line separation without over-marking the intermediate fragments.
			if len(frags) > 0 {
				setGroupEnd(frags[len(frags)-1])
			}
			out = append(out, frags...)
		} else {
			out = append(out, n)
		}
	}
	return hoistImports(out)
}

// bubbleRule splits a top-level style rule so that any at-rule that is a direct
// child bubbles to the document root, wrapping a copy of the rule's selector
// around the at-rule's body; runs of other children stay within the rule.
func bubbleRule(sr *cssStyleRule) []cssNode {
	var result []cssNode
	var pending []cssNode
	flush := func() {
		if len(pending) > 0 {
			result = append(result, &cssStyleRule{raw: true, rawSel: sr.rawSel, nodes: pending})
			pending = nil
		}
	}
	for _, ch := range sr.nodes {
		if at, ok := ch.(*cssAtRule); ok && at.hasBody {
			flush()
			inner := &cssStyleRule{raw: true, rawSel: sr.rawSel, nodes: at.nodes}
			result = append(result, &cssAtRule{name: at.name, params: at.params, hasBody: true, nodes: []cssNode{inner}})
		} else {
			pending = append(pending, ch)
		}
	}
	flush()
	if len(result) > 0 {
		setBlankBefore(result[0], sr.blankBefore)
	}
	return result
}

// hoistImports moves bodiless @import rules to the front, preserving order.
func hoistImports(nodes []cssNode) []cssNode {
	var imports, rest []cssNode
	for _, n := range nodes {
		if at, ok := n.(*cssAtRule); ok && at.name == "import" && !at.hasBody {
			imports = append(imports, n)
		} else {
			rest = append(rest, n)
		}
	}
	if len(imports) == 0 {
		return nodes
	}
	return append(imports, rest...)
}
