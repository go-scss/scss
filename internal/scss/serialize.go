// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import (
	"bytes"
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
	braceLine    int    // 1-based source line of the rule's `{` (0 = unknown), for trailing-comment placement
}

func (r *cssStyleRule) cssNode()             {}
func (r *cssStyleRule) appendNode(n cssNode) { r.nodes = append(r.nodes, n) }
func (r *cssStyleRule) children() []cssNode  { return r.nodes }

type cssDeclaration struct {
	name    string
	value   Value  // evaluated value (nil for custom properties)
	raw     string // raw text for custom properties
	custom  bool
	nameCol int // source column of the name (custom properties), for re-indentation
	endLine int // 1-based source line where the value/body ends (0 = unknown), for trailing-comment placement
}

func (d *cssDeclaration) cssNode() {}

type cssComment struct {
	text        string
	col         int // 0-based source column of the opening `/*`, for re-indentation
	blankBefore bool
	line        int // 1-based source line of the opening `/*` (0 = unknown), for trailing-comment placement
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
	// sb is a bytes.Buffer (not strings.Builder) so a just-written trailing
	// newline can be truncated when a loud comment attaches to the previous line.
	sb bytes.Buffer
}

// trimTrailingNewline removes a single trailing "\n" from the output buffer, so
// a trailing loud comment can be re-attached to the previous line with a space.
func (s *serializer) trimTrailingNewline() {
	b := s.sb.Bytes()
	if n := len(b); n > 0 && b[n-1] == '\n' {
		s.sb.Truncate(n - 1)
	}
}

func serialize(root *cssRoot, compressed bool) string {
	s := &serializer{compressed: compressed}
	s.emitChildren(root.children(), 0)
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
	// @charset rule, as dart-sass does. A source-level @charset is dropped during
	// parsing, so this is the only @charset the output can carry.
	if hasNonASCII(out) {
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

func (s *serializer) emitChildren(nodes []cssNode, depth int) {
	emittedAny := false
	// An invisible node (an empty parent whose nested rules were hoisted out) is
	// dropped from the output, but it still carries its source group's blank-line
	// credit. dart attaches the separating blank to the group boundary, not to the
	// dropped rule, so hold a dropped node's blankBefore and apply it to the next
	// visible node — otherwise sibling rules hoisted from separate parents (e.g.
	// `a b { c & {} }` `d { e & {} }`) would fuse without the blank dart emits.
	pendingBlank := false
	for _, n := range nodes {
		if isEmptyContainer(n) {
			if blankBeforeOf(n) {
				pendingBlank = true
			}
			continue
		}
		if !s.compressed && emittedAny && (pendingBlank || blankBeforeOf(n)) {
			s.sb.WriteString("\n")
		}
		s.emitNode(n, depth)
		emittedAny = true
		pendingBlank = false
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
		// dart drops an empty @media/@supports (they carry no meaning without
		// content) but KEEPS every other empty at-rule that had a block — an
		// empty @keyframes, @font-face or unknown at-rule still serialises as
		// `@name {}`. A childless at-rule declared without a block (hasBody
		// false, e.g. `@import`) is never an empty container.
		if v.name == "media" || v.name == "supports" {
			return v.hasBody && !hasVisible(v.nodes)
		}
		return false
	}
	return false
}

// lastVisibleIsStyleRule reports whether the last emitted (non-empty) child of a
// container is a style rule. A node that bubbles into a container computes its
// leading blank line from this — dart separates a rule from a preceding style
// rule, and only a style rule, regardless of source nesting.
func lastVisibleIsStyleRule(c cssContainer) bool {
	ch := c.children()
	for i := len(ch) - 1; i >= 0; i-- {
		if isEmptyContainer(ch[i]) {
			continue
		}
		_, ok := ch[i].(*cssStyleRule)
		return ok
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
			s.emitRule(v.rawSel, v.nodes, depth, v.braceLine)
		} else {
			s.emitRule(v.selector.serialize(s.compressed), v.nodes, depth, v.braceLine)
		}
	case *cssAtRule:
		s.emitAtRule(v, depth)
	case *cssDeclaration:
		s.emitDecl(v, depth)
	case *cssComment:
		s.emitComment(v, depth, false)
	}
}

// emitComment serialises a preserved loud comment, reproducing dart-sass's
// Serializer.visitCssComment: sourceMappingURL/sourceURL comments are dropped,
// and a multi-line comment's continuation lines are re-indented to the current
// output depth (bounded by the comment's own source column).
func (s *serializer) emitComment(v *cssComment, depth int, trailing bool) {
	// Ignore sourceMappingURL and sourceURL comments (dart-sass drops these).
	if isSourceURLComment(v.text) {
		return
	}
	// A trailing comment is attached to the previous line: the caller has already
	// written the separating space in place of this comment's leading indent.
	if !trailing {
		s.indent(depth)
	}
	// In compressed output there is no indentation to re-base against, so the
	// comment is emitted verbatim; only expanded output re-indents continuation
	// lines to the current depth.
	if min, hasMin := commentMinIndentation(v.text); s.compressed || !hasMin || min < 0 {
		s.sb.WriteString(v.text)
	} else {
		if v.col < min {
			min = v.col
		}
		s.writeCommentReindented(v.text, min, depth)
	}
	if !s.compressed {
		s.sb.WriteString("\n")
	}
}

// isSourceURLComment reports whether text is a `/*# sourceMappingURL=` or
// `/*# sourceURL=` comment, which dart-sass omits from serialized output.
func isSourceURLComment(text string) bool {
	return strings.HasPrefix(text, "/*# sourceMappingURL=") ||
		strings.HasPrefix(text, "/*# sourceURL=")
}

// commentMinIndentation ports dart-sass's Serializer._minimumIndentation for
// loud comments: it returns the least leading indentation (space/tab count) of
// any non-blank line after the first. hasMin is false when the comment is a
// single line (no newline), which is emitted verbatim. A returned min of -1
// means every line after the first is blank.
func commentMinIndentation(text string) (int, bool) {
	n := len(text)
	i := 0
	for i < n && text[i] != '\n' {
		i++
	}
	if i >= n {
		return 0, false // single line
	}
	i++ // consume first newline
	min := -1
	for i < n {
		start := i
		for i < n && (text[i] == ' ' || text[i] == '\t') {
			i++
		}
		if i >= n {
			break // trailing blank
		}
		if text[i] == '\n' {
			i++
			continue // blank line
		}
		if col := i - start; min == -1 || col < min {
			min = col
		}
		for i < n && text[i] != '\n' {
			i++
		}
		if i < n {
			i++ // consume newline
		}
	}
	return min, true
}

// writeCommentReindented ports dart-sass's Serializer._writeWithIndent for loud
// comments: the first line is written verbatim; each subsequent non-blank line
// has minIndent leading whitespace columns replaced by the current output
// indentation. Runs of blank lines are preserved as bare newlines, and a
// trailing run of whitespace collapses to a single trailing space.
func (s *serializer) writeCommentReindented(text string, minIndent, depth int) {
	n := len(text)
	i := 0
	for i < n && text[i] != '\n' {
		s.sb.WriteByte(text[i])
		i++
	}
	for i < n { // text[i] == '\n'
		newlines := 0
		var lineStart int
		for {
			i++ // consume the newline
			newlines++
			lineStart = i
			for i < n && (text[i] == ' ' || text[i] == '\t') {
				i++
			}
			if i >= n {
				s.sb.WriteByte(' ')
				return
			}
			if text[i] == '\n' {
				continue // blank line: fold into the newline run
			}
			break
		}
		for k := 0; k < newlines; k++ {
			s.sb.WriteByte('\n')
		}
		s.indent(depth)
		start := lineStart
		if minIndent > 0 {
			start += minIndent
		}
		if start > i {
			start = i
		}
		s.sb.WriteString(text[start : i+1])
		i++ // past the non-whitespace char already written
		for i < n && text[i] != '\n' {
			s.sb.WriteByte(text[i])
			i++
		}
	}
}

func (s *serializer) emitRule(selector string, nodes []cssNode, depth, braceLine int) {
	s.indent(depth)
	// A multi-line selector (comma list split across lines) indents each of its
	// continuation lines to the rule's depth, matching dart-sass.
	if !s.compressed && depth > 0 && strings.Contains(selector, "\n") {
		selector = strings.ReplaceAll(selector, "\n", "\n"+strings.Repeat("  ", depth))
	}
	s.sb.WriteString(selector)
	if s.compressed {
		s.sb.WriteString("{")
		s.emitDeclList(nodes, depth+1, 0)
		s.sb.WriteString("}")
	} else {
		s.sb.WriteString(" {\n")
		s.emitDeclList(nodes, depth+1, braceLine)
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
	// A kept but childless at-rule (an empty @keyframes/@font-face/unknown rule
	// that dart preserves) collapses to `{}` on the same line rather than an
	// open block spanning two lines.
	if !hasVisible(a.nodes) {
		if s.compressed {
			s.sb.WriteString("{}")
		} else {
			s.sb.WriteString(" {}\n")
		}
		return
	}
	if s.compressed {
		s.sb.WriteString("{")
		s.emitDeclList(a.nodes, depth+1, 0)
		s.sb.WriteString("}")
	} else {
		s.sb.WriteString(" {\n")
		s.emitDeclList(a.nodes, depth+1, 0)
		s.indent(depth)
		s.sb.WriteString("}\n")
	}
}

// emitDeclList emits the children of a block, handling declaration separators
// and nested rules/at-rules within. braceLine is the 1-based source line of the
// block's opening `{` (0 = unknown), used to attach a first-child loud comment
// that trails the brace.
func (s *serializer) emitDeclList(nodes []cssNode, depth, braceLine int) {
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
	// dart-sass does not insert blank lines between nodes inside a block; blank-
	// line separation only applies at the top level of the stylesheet. A loud
	// comment on the same source line as the node it follows (or as the opening
	// brace, when it is the first child) is written on that line rather than its
	// own, reproducing dart's _isTrailingComment.
	var prev cssNode
	for _, n := range visible {
		if c, ok := n.(*cssComment); ok && !isSourceURLComment(c.text) &&
			trailingCommentAttaches(c, prev, braceLine) {
			s.trimTrailingNewline()
			s.sb.WriteString(" ")
			s.emitComment(c, depth, true)
		} else {
			s.emitNode(n, depth)
		}
		prev = n
	}
}

// trailingCommentAttaches reports whether loud comment c should be written on
// the same output line as the node it follows, reproducing dart-sass's
// _isTrailingComment for the cases go-scss models: a comment following a
// declaration on the same source line, or a first-child comment on the block's
// opening-brace line. A comment following a nested block, or one with no line
// information (compressed output, indented syntax), stays on its own line.
func trailingCommentAttaches(c *cssComment, prev cssNode, braceLine int) bool {
	if c.line == 0 {
		return false
	}
	if prev == nil {
		return braceLine != 0 && c.line == braceLine
	}
	if d, ok := prev.(*cssDeclaration); ok {
		return d.endLine != 0 && c.line == d.endLine
	}
	return false
}

func (s *serializer) emitDecl(d *cssDeclaration, depth int) {
	s.indent(depth)
	s.sb.WriteString(d.name)
	if d.custom {
		// Custom-property values are reproduced verbatim (including the space
		// after the colon). Multiline values are re-indented to the current
		// nesting level exactly as dart-sass does.
		s.sb.WriteString(":")
		if s.compressed {
			s.sb.WriteString(d.raw)
			return
		}
		s.writeCustomValue(d.raw, d.nameCol, depth)
		s.sb.WriteString(";\n")
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

// writeCustomValue emits a custom-property value, re-indenting continuation
// lines of a multiline value to the current output depth, reproducing
// dart-sass's Serializer._writeCustomProperty / _minimumIndentation logic.
func (s *serializer) writeCustomValue(value string, nameCol, depth int) {
	minIndent, kind := customMinIndentation(value)
	switch kind {
	case customIndentNone:
		// No newline: emit verbatim.
		s.sb.WriteString(value)
	case customIndentTrailing:
		// Value ends in a newline with no following content: trim trailing
		// whitespace and add a single space.
		s.sb.WriteString(strings.TrimRight(value, " \t\n\r\f"))
		s.sb.WriteByte(' ')
	default:
		if nameCol < minIndent {
			minIndent = nameCol
		}
		s.writeReindented(value, minIndent, depth)
	}
}

type customIndentKind int

const (
	customIndentNone customIndentKind = iota
	customIndentValue
	customIndentTrailing
)

// customMinIndentation returns the least indentation (in leading space/tab
// characters) of any non-blank line after the first newline of value. kind
// distinguishes "no newline", "trailing newline only", and "has continuation".
func customMinIndentation(value string) (int, customIndentKind) {
	nl := strings.IndexByte(value, '\n')
	if nl < 0 {
		return 0, customIndentNone
	}
	// A value that ends with a newline (ignoring trailing spaces/tabs) is a
	// trailing-whitespace value: dart-sass trims it and appends a single space
	// rather than re-indenting. Because the value contains a newline, trimming
	// trailing spaces/tabs always stops at a non-whitespace byte (at worst that
	// newline), so end >= 1.
	end := len(value)
	for value[end-1] == ' ' || value[end-1] == '\t' {
		end--
	}
	if value[end-1] == '\n' || value[end-1] == '\r' || value[end-1] == '\f' {
		return 0, customIndentTrailing
	}
	// The value ends with content, so every continuation line begins before the
	// end of the string; the least leading indentation is the re-indent base.
	min := len(value)
	pos := nl + 1
	for pos < len(value) {
		j := pos
		for value[j] == ' ' || value[j] == '\t' {
			j++
		}
		if ind := j - pos; ind < min {
			min = ind
		}
		k := strings.IndexByte(value[pos:], '\n')
		if k < 0 {
			break
		}
		pos += k + 1
	}
	return min, customIndentValue
}

// writeReindented writes value with each continuation line re-indented: it
// strips up to minIndent leading whitespace characters and prepends the current
// output indentation (depth levels of two spaces).
func (s *serializer) writeReindented(value string, minIndent, depth int) {
	i := 0
	// First line, up to the first newline, verbatim.
	for i < len(value) && value[i] != '\n' {
		s.sb.WriteByte(value[i])
		i++
	}
	for i < len(value) {
		// value[i] == '\n'
		s.sb.WriteByte('\n')
		i++
		s.indent(depth)
		stripped := 0
		for i < len(value) && stripped < minIndent && (value[i] == ' ' || value[i] == '\t') {
			i++
			stripped++
		}
		for i < len(value) && value[i] != '\n' {
			s.sb.WriteByte(value[i])
			i++
		}
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

// isBlankListElem reports whether a list element is "blank" in the sense of
// dart-sass's Value.isBlank, which governs which elements are dropped from CSS
// list output: sassNull, or an unbracketed list all of whose elements are
// themselves blank (which makes the empty list `()` blank). Unlike the
// declaration-level isBlankValue, an empty string is NOT blank here.
func isBlankListElem(v Value) bool {
	switch x := v.(type) {
	case *Null:
		return true
	case *List:
		if x.Bracketed {
			return false
		}
		for _, e := range x.Elements {
			if !isBlankListElem(e) {
				return false
			}
		}
		return true
	}
	return false
}

func serializeList(l *List, compressed, quote bool) string {
	elems := make([]string, 0, len(l.Elements))
	for _, e := range l.Elements {
		// dart-sass omits "blank" elements from CSS list output: sassNull and any
		// unbracketed list whose members are all themselves blank (so an empty
		// list `()` in `1 2 () 3` renders as `1 2 3`, not `1 2  3`).
		if isBlankListElem(e) {
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
