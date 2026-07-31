// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import (
	"math"
	"strconv"
	"strings"
)

// numberCalcRepr serializes a non-finite number the way dart-sass does: wrapped
// in a calc() expression (calc(infinity), calc(-infinity), calc(NaN)), with the
// unit expressed as a multiplication when present.
func numberCalcRepr(n *Number) string {
	var core string
	switch {
	case math.IsNaN(n.Val):
		core = "NaN"
	case n.Val > 0:
		core = "infinity"
	default:
		core = "-infinity"
	}
	if unit := unitOutput(n); unit != "" {
		return "calc(" + core + " * 1" + unit + ")"
	}
	return "calc(" + core + ")"
}

// --- output tree ---

type cssNode interface{ cssNode() }

type cssContainer interface {
	appendNode(cssNode)
	children() []cssNode
}

type cssRoot struct{ nodes []cssNode }

func (r *cssRoot) appendNode(n cssNode) { r.nodes = append(r.nodes, n) }
func (r *cssRoot) children() []cssNode  { return r.nodes }

type cssStyleRule struct {
	selector     selectorList
	nodes        []cssNode
	original     selectorList // pre-extend selector (extender + child nesting)
	mediaContext []string     // enclosing media-query context for @extend
	box          *box         // extension box; selector is set from it post-extend
	blankBefore  bool
	raw          bool   // plain-CSS rule: emit rawSel verbatim, never extend/resolve
	rawSel       string // verbatim selector for a plain-CSS rule
}

func (r *cssStyleRule) cssNode()             {}
func (r *cssStyleRule) appendNode(n cssNode) { r.nodes = append(r.nodes, n) }
func (r *cssStyleRule) children() []cssNode  { return r.nodes }

type cssDeclaration struct {
	name   string
	value  Value  // evaluated value (nil for custom properties)
	raw    string // raw text for custom properties
	custom bool
}

func (d *cssDeclaration) cssNode() {}

type cssComment struct {
	text        string
	blankBefore bool
}

func (c *cssComment) cssNode() {}

type cssAtRule struct {
	name        string
	params      string
	nodes       []cssNode
	hasBody     bool
	blankBefore bool
}

func (a *cssAtRule) cssNode()             {}
func (a *cssAtRule) appendNode(n cssNode) { a.nodes = append(a.nodes, n) }
func (a *cssAtRule) children() []cssNode  { return a.nodes }

// --- serializer ---

type serializer struct {
	compressed bool
	sb         strings.Builder
}

func serialize(root *cssRoot, compressed bool) string {
	s := &serializer{compressed: compressed}
	s.emitChildren(root.children(), 0, true)
	out := s.sb.String()
	if compressed {
		out = strings.TrimRight(out, "\n;")
		if out != "" {
			out += "\n"
		}
		// A compressed stylesheet containing non-ASCII characters is prefixed
		// with a UTF-8 byte-order mark, as dart-sass does.
		if hasNonASCII(out) && !strings.HasPrefix(out, "\uFEFF") {
			out = "\uFEFF" + out
		}
		return out
	}
	// Every emitted node ends in "\n" in expanded mode, so the result is already
	// newline-terminated (or empty) after trimming leading blank lines.
	out = strings.TrimLeft(out, "\n")
	// An expanded stylesheet containing non-ASCII characters is prefixed with an
	// @charset rule, as dart-sass does — unless the output already begins with one
	// (an author-written @charset is kept rather than duplicated).
	if hasNonASCII(out) && !strings.HasPrefix(out, "@charset ") {
		out = "@charset \"UTF-8\";\n" + out
	}
	return out
}

// hasNonASCII reports whether s contains any byte outside the US-ASCII range,
// which in a UTF-8 stream marks the presence of a non-ASCII code point.
func hasNonASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return true
		}
	}
	return false
}

func (s *serializer) indent(depth int) {
	if s.compressed {
		return
	}
	for i := 0; i < depth; i++ {
		s.sb.WriteString("  ")
	}
}

func (s *serializer) emitChildren(nodes []cssNode, depth int, top bool) {
	visible := make([]cssNode, 0, len(nodes))
	for _, n := range nodes {
		if isEmptyContainer(n) {
			continue
		}
		visible = append(visible, n)
	}
	for i, n := range visible {
		if !s.compressed && i > 0 && needsBlankBefore(visible[i-1], n, top) {
			s.sb.WriteString("\n")
		}
		s.emitNode(n, depth)
	}
}

func isEmptyContainer(n cssNode) bool {
	switch v := n.(type) {
	case *cssStyleRule:
		if v.raw {
			return !hasVisible(v.nodes)
		}
		return v.selector.isEmpty() || v.selector.list.isInvisible() || !hasVisible(v.nodes)
	case *cssAtRule:
		return v.hasBody && !hasVisible(v.nodes)
	}
	return false
}

func hasVisible(nodes []cssNode) bool {
	for _, n := range nodes {
		if !isEmptyContainer(n) {
			return true
		}
	}
	return false
}

// needsBlankBefore reproduces dart-sass's rule: a blank line precedes a node
// that starts a new source-level group when the previous sibling is a plain
// style rule.
func needsBlankBefore(prev, cur cssNode, top bool) bool {
	return blankBeforeOf(cur)
}

func setBlankBefore(n cssNode, b bool) {
	switch v := n.(type) {
	case *cssStyleRule:
		v.blankBefore = b
	case *cssAtRule:
		v.blankBefore = b
	case *cssComment:
		v.blankBefore = b
	}
}

func blankBeforeOf(n cssNode) bool {
	switch v := n.(type) {
	case *cssStyleRule:
		return v.blankBefore
	case *cssAtRule:
		return v.blankBefore
	case *cssComment:
		return v.blankBefore
	}
	return false
}

func (s *serializer) emitNode(n cssNode, depth int) {
	switch v := n.(type) {
	case *cssStyleRule:
		if v.raw {
			s.emitRule(v.rawSel, v.nodes, depth)
		} else {
			s.emitRule(v.selector.serialize(s.compressed), v.nodes, depth)
		}
	case *cssAtRule:
		s.emitAtRule(v, depth)
	case *cssDeclaration:
		s.emitDecl(v, depth)
	case *cssComment:
		s.indent(depth)
		s.sb.WriteString(v.text)
		if !s.compressed {
			s.sb.WriteString("\n")
		}
	}
}

func (s *serializer) emitRule(selector string, nodes []cssNode, depth int) {
	s.indent(depth)
	s.sb.WriteString(selector)
	if s.compressed {
		s.sb.WriteString("{")
		s.emitDeclList(nodes, depth+1)
		s.sb.WriteString("}")
	} else {
		s.sb.WriteString(" {\n")
		s.emitDeclList(nodes, depth+1)
		s.indent(depth)
		s.sb.WriteString("}\n")
	}
}

func (s *serializer) emitAtRule(a *cssAtRule, depth int) {
	s.indent(depth)
	s.sb.WriteString("@")
	s.sb.WriteString(a.name)
	if a.params != "" {
		if !(s.compressed && strings.HasPrefix(a.params, "(")) {
			s.sb.WriteString(" ")
		}
		s.sb.WriteString(a.params)
	}
	if !a.hasBody {
		if s.compressed {
			s.sb.WriteString(";")
		} else {
			s.sb.WriteString(";\n")
		}
		return
	}
	if s.compressed {
		s.sb.WriteString("{")
		s.emitDeclList(a.nodes, depth+1)
		s.sb.WriteString("}")
	} else {
		s.sb.WriteString(" {\n")
		s.emitDeclList(a.nodes, depth+1)
		s.indent(depth)
		s.sb.WriteString("}\n")
	}
}

// emitDeclList emits the children of a block, handling declaration separators
// and nested rules/at-rules within.
func (s *serializer) emitDeclList(nodes []cssNode, depth int) {
	visible := make([]cssNode, 0, len(nodes))
	for _, n := range nodes {
		if isEmptyContainer(n) {
			continue
		}
		visible = append(visible, n)
	}
	if s.compressed {
		for i, n := range visible {
			if _, isDecl := n.(*cssDeclaration); isDecl && i > 0 {
				if _, prevDecl := visible[i-1].(*cssDeclaration); prevDecl {
					s.sb.WriteString(";")
				}
			}
			s.emitNode(n, depth)
		}
		return
	}
	for _, n := range visible {
		// dart-sass does not insert blank lines between nodes inside a block;
		// blank-line separation only applies at the top level of the stylesheet.
		s.emitNode(n, depth)
	}
}

func (s *serializer) emitDecl(d *cssDeclaration, depth int) {
	s.indent(depth)
	s.sb.WriteString(d.name)
	if d.custom {
		// Custom-property values are reproduced verbatim (including the space
		// after the colon) in both output styles.
		s.sb.WriteString(":")
		s.sb.WriteString(d.raw)
		if !s.compressed {
			s.sb.WriteString(";\n")
		}
		return
	}
	val := serializeValue(d.value, s.compressed)
	if s.compressed {
		s.sb.WriteString(":")
		s.sb.WriteString(val)
	} else {
		s.sb.WriteString(": ")
		s.sb.WriteString(val)
		s.sb.WriteString(";\n")
	}
}

// --- value serialization ---

func serializeValue(v Value, compressed bool) string {
	return serializeValueQ(v, compressed, true)
}

// serializeValueQ serializes v, threading a `quote` flag that controls whether
// quoted strings keep their quotes. Interpolation (`#{}`) serializes with
// quote=false, and dart-sass propagates that recursively through list and map
// elements, so `#{"a" "b"}` becomes `a b` rather than `"a" "b"`.
func serializeValueQ(v Value, compressed, quote bool) string {
	switch x := v.(type) {
	case *Number:
		// A number that still carries as-slash provenance serializes back as the
		// literal "left/right" it came from, recursively (so 1/2/3/4/5 stays a
		// flat slash chain rather than the collapsed quotient).
		if x.slashL != nil && x.slashR != nil {
			return serializeValueQ(x.slashL, compressed, quote) + "/" + serializeValueQ(x.slashR, compressed, quote)
		}
		// dart-sass serializes a number that isn't expressible as a plain CSS
		// value — a non-finite magnitude, or one carrying complex units (more
		// than one numerator unit, or any denominator units) — wrapped in a
		// calc() expression, e.g. calc(1px * 1rad), calc(infinity * 1px / 1em).
		// The in-calc term writer already renders that form exactly.
		if math.IsInf(x.Val, 0) || math.IsNaN(x.Val) || x.hasComplexUnits() {
			var sb strings.Builder
			sb.WriteString("calc(")
			writeCalcTerm(&sb, x, compressed)
			sb.WriteByte(')')
			return sb.String()
		}
		return formatFloat(x.Val, compressed) + unitOutput(x)
	case *SassColor:
		return serializeColor(x, compressed, false)
	case *SassString:
		if x.Quoted && quote {
			return serializeQuoted(x.Text)
		}
		return serializeUnquoted(x.Text, compressed, quote)
	case *Boolean:
		if x.V {
			return "true"
		}
		return "false"
	case *Null:
		return ""
	case *List:
		return serializeList(x, compressed, quote)
	case *Map:
		return serializeMap(x, compressed, quote)
	case *SassCalculation:
		return serializeCalculation(x, compressed)
	}
	return ""
}

func unitOutput(n *Number) string {
	if len(n.Numer) == 0 && len(n.Denom) == 0 {
		return ""
	}
	return n.unitString()
}

func serializeList(l *List, compressed, quote bool) string {
	elems := make([]string, 0, len(l.Elements))
	for _, e := range l.Elements {
		if _, isNull := e.(*Null); isNull {
			continue
		}
		// CSS output never parenthesizes a nested list — dart-sass flattens the
		// appearance, joining with the separator (so a slash list inside a slash
		// list renders "c / d / e / f", not "(c / d) / (e / f)"). Only inspect,
		// which has its own path, adds the disambiguating parentheses.
		s := serializeValueQ(e, compressed, quote)
		elems = append(elems, s)
	}
	var sep string
	switch l.Sep {
	case SepComma:
		if compressed {
			sep = ","
		} else {
			sep = ", "
		}
	case SepSlash:
		if compressed || l.SlashLit {
			sep = "/"
		} else {
			sep = " / "
		}
	default:
		sep = " "
	}
	body := strings.Join(elems, sep)
	if l.Bracketed {
		return "[" + body + "]"
	}
	return body
}

func serializeMap(m *Map, compressed, quote bool) string {
	parts := make([]string, len(m.Keys))
	kvsep := ": "
	sep := ", "
	if compressed {
		kvsep = ":"
		sep = ","
	}
	for i := range m.Keys {
		parts[i] = serializeValueQ(m.Keys[i], compressed, quote) + kvsep + serializeValueQ(m.Values[i], compressed, quote)
	}
	return "(" + strings.Join(parts, sep) + ")"
}

// stringNeedsCharEscape reports whether a code point must be written as a hex
// escape inside a quoted string: the control characters and the private-use
// areas, as dart-sass does. Tab (U+0009) is the one control character dart
// leaves literal inside quoted strings, so it is excluded here.
func stringNeedsCharEscape(r rune) bool {
	if (r < 0x20 && r != '\t') || r == 0x7F {
		return true
	}
	return (r >= 0xE000 && r <= 0xF8FF) ||
		(r >= 0xF0000 && r <= 0xFFFFD) ||
		(r >= 0x100000 && r <= 0x10FFFD)
}

func isHexRune(r rune) bool {
	return (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
}

// isPrivateUseRune reports whether r is in a Unicode Private Use Area.
func isPrivateUseRune(r rune) bool {
	return (r >= 0xE000 && r <= 0xF8FF) ||
		(r >= 0xF0000 && r <= 0xFFFFD) ||
		(r >= 0x100000 && r <= 0x10FFFD)
}

// serializeUnquoted renders an unquoted string. When final (i.e. emitted as a
// CSS value rather than woven into a `#{}` interpolation), dart-sass collapses a
// newline to a space, swallowing the whitespace that follows it, and — in
// expanded mode — writes Private Use Area characters as hex escapes since there
// is no useful way to render them directly. Interpolation instead keeps the raw
// text, so a string's newlines survive to be re-escaped by an enclosing quoted
// string. (The fast path skips the rune walk when nothing can change.)
func serializeUnquoted(text string, compressed, final bool) string {
	hasNewline := strings.ContainsRune(text, '\n')
	hasPUA := !compressed && strings.ContainsFunc(text, isPrivateUseRune)
	if !final || (!hasNewline && !hasPUA) {
		return text
	}
	var sb strings.Builder
	runes := []rune(text)
	afterNewline := false
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch {
		case r == '\n':
			sb.WriteByte(' ')
			afterNewline = true
		case r == ' ':
			if !afterNewline {
				sb.WriteByte(' ')
			}
		default:
			afterNewline = false
			if !compressed && isPrivateUseRune(r) {
				sb.WriteByte('\\')
				sb.WriteString(strconv.FormatInt(int64(r), 16))
				if i+1 < len(runes) {
					if n := runes[i+1]; isHexRune(n) || n == ' ' || n == '\t' {
						sb.WriteByte(' ')
					}
				}
			} else {
				sb.WriteRune(r)
			}
		}
	}
	return sb.String()
}

func serializeQuoted(text string) string {
	// Prefer double quotes; switch to single quotes only when the text contains a
	// double quote but no single quote, matching dart-sass.
	quote := byte('"')
	if strings.Contains(text, "\"") && !strings.Contains(text, "'") {
		quote = '\''
	}
	var sb strings.Builder
	sb.WriteByte(quote)
	runes := []rune(text)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch {
		case r == rune(quote):
			sb.WriteByte('\\')
			sb.WriteRune(r)
		case r == '\\':
			sb.WriteString(`\\`)
		case stringNeedsCharEscape(r):
			sb.WriteByte('\\')
			sb.WriteString(strconv.FormatInt(int64(r), 16))
			// A trailing space terminates the escape when the next character
			// could otherwise be read as part of it.
			if i+1 < len(runes) {
				if n := runes[i+1]; isHexRune(n) || n == ' ' || n == '\t' {
					sb.WriteByte(' ')
				}
			}
		default:
			sb.WriteRune(r)
		}
	}
	sb.WriteByte(quote)
	return sb.String()
}
