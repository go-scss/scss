// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import (
	"math"
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
	selector    selectorList
	nodes       []cssNode
	original    selectorList // pre-extend selector for @extend matching
	blankBefore bool
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
		return out
	}
	out = strings.TrimLeft(out, "\n")
	if len(out) > 0 && !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return out
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
		return len(v.selector.complexes) == 0 || !hasVisible(v.nodes)
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
		s.emitRule(v.selector.serialize(s.compressed), v.nodes, depth)
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
	switch x := v.(type) {
	case *Number:
		if math.IsInf(x.Val, 0) || math.IsNaN(x.Val) {
			return numberCalcRepr(x)
		}
		return formatFloat(x.Val, compressed) + unitOutput(x)
	case *SassColor:
		if compressed {
			return x.compressedRepr()
		}
		return x.expandedRepr()
	case *SassString:
		if x.Quoted {
			return serializeQuoted(x.Text)
		}
		return x.Text
	case *Boolean:
		if x.V {
			return "true"
		}
		return "false"
	case *Null:
		return ""
	case *List:
		return serializeList(x, compressed)
	case *Map:
		return serializeMap(x, compressed)
	}
	return ""
}

func unitOutput(n *Number) string {
	if len(n.Numer) == 0 && len(n.Denom) == 0 {
		return ""
	}
	return n.unitString()
}

func serializeList(l *List, compressed bool) string {
	elems := make([]string, 0, len(l.Elements))
	for _, e := range l.Elements {
		if _, isNull := e.(*Null); isNull {
			continue
		}
		s := serializeValue(e, compressed)
		if _, isList := e.(*List); isList {
			if sub := e.(*List); sub.Sep == l.Sep && !sub.Bracketed && len(sub.Elements) > 1 {
				s = "(" + s + ")"
			}
		}
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
		sep = "/"
	default:
		sep = " "
	}
	body := strings.Join(elems, sep)
	if l.Bracketed {
		return "[" + body + "]"
	}
	return body
}

func serializeMap(m *Map, compressed bool) string {
	parts := make([]string, len(m.Keys))
	kvsep := ": "
	sep := ", "
	if compressed {
		kvsep = ":"
		sep = ","
	}
	for i := range m.Keys {
		parts[i] = serializeValue(m.Keys[i], compressed) + kvsep + serializeValue(m.Values[i], compressed)
	}
	return "(" + strings.Join(parts, sep) + ")"
}

func serializeQuoted(text string) string {
	// Prefer double quotes; escape embedded double quotes and backslashes minimally.
	hasDouble := strings.Contains(text, "\"")
	hasSingle := strings.Contains(text, "'")
	quote := byte('"')
	if hasDouble && !hasSingle {
		quote = '\''
	}
	var sb strings.Builder
	sb.WriteByte(quote)
	for i := 0; i < len(text); i++ {
		c := text[i]
		if c == quote || c == '\\' {
			if c == '\\' && i+1 < len(text) {
				// keep existing escape sequences intact
				sb.WriteByte(c)
				continue
			}
			sb.WriteByte('\\')
		}
		sb.WriteByte(c)
	}
	sb.WriteByte(quote)
	return sb.String()
}
