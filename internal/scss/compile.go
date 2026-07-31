// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import "strings"

// Result holds a compilation result.
type Result struct {
	CSS        string
	LoadedURLs []string
	Warnings   []string
}

// Render compiles Sass/SCSS source to CSS.
func Render(source string, indented bool, compressed bool, importer Importer) (res Result, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = rethrowIfNotSass(r)
		}
	}()
	if indented {
		source = convertIndented(source)
	}
	stmts, perr := parseStylesheet(source)
	if perr != nil {
		return Result{}, perr
	}
	e := newEvaluator(importer)
	e.run(stmts)
	css := serialize(e.root, compressed)
	return Result{CSS: css, LoadedURLs: e.loadedURLs, Warnings: e.warnings}, nil
}

// convertIndented converts indented (.sass) syntax to bracketed SCSS.
//
// The conversion runs in two phases. First it folds physical source lines into
// logical statements: a physical line whose header is syntactically incomplete
// (an unclosed bracket or string, a trailing binary operator/comma, an
// unfinished at-rule header such as "@for $i from") continues onto the next
// physical line instead of opening a child block. Second it walks the logical
// statements by indentation, emitting "{ ... }" for statements that gain a
// deeper-indented child and, for childless statements, either "{}" (selectors
// and block at-rules) or ";" (declarations and statement at-rules).

// scanState walks s and reports the net (){}[] bracket depth and whether a
// quoted string is still open at the end, ignoring "//" line comments and
// spanning "/* */" block comments. It is used to decide whether a logical line
// is still open across a newline.
func scanState(s string) (depth int, inString bool) {
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case '"', '\'':
			i++
			closed := false
			for i < len(s) {
				if s[i] == '\\' {
					i++
				} else if s[i] == c {
					closed = true
					break
				}
				i++
			}
			if !closed {
				return depth, true
			}
		case '/':
			if i+1 < len(s) && s[i+1] == '/' {
				for i < len(s) && s[i] != '\n' {
					i++
				}
			} else if i+1 < len(s) && s[i+1] == '*' {
				i += 2
				for i+1 < len(s) && !(s[i] == '*' && s[i+1] == '/') {
					i++
				}
				i++
			}
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			if depth > 0 {
				depth--
			}
		}
	}
	return depth, false
}

func indentOf(s string) int {
	n := 0
	for _, c := range s {
		switch c {
		case ' ':
			n++
		case '\t':
			n += 8
		default:
			return n
		}
	}
	return n
}

// Statement kinds recognised by the indented preprocessor.
const (
	kSelector = iota
	kProperty
	kVariable
	kAtRule
)

// blockAtRules names the at-rules that always open a braced block and therefore
// must be written "{}" when childless in the indented syntax; every other
// at-rule (including unknown ones) is a ";"-terminated statement when childless.
var blockAtRules = map[string]bool{
	"function": true, "mixin": true, "if": true, "else": true,
	"each": true, "for": true, "while": true, "media": true,
	"supports": true, "at-root": true, "keyframes": true,
	"-webkit-keyframes": true, "-moz-keyframes": true, "-o-keyframes": true,
	"font-face": true,
}

// expandShorthand rewrites the indented-syntax mixin shorthands at the start of
// a line: "=name" -> "@mixin name" and "+name" -> "@include name". A leading
// "+" is only an include when an identifier follows immediately ("+a"); "+ a"
// and a bare "+" are the sibling combinator and stay a selector.
func expandShorthand(line string) string {
	lead := len(line) - len(strings.TrimLeft(line, " \t"))
	rest := line[lead:]
	switch {
	case strings.HasPrefix(rest, "="):
		return line[:lead] + "@mixin " + rest[1:]
	case strings.HasPrefix(rest, "+") && len(rest) > 1 && sassNameStart(rest[1]):
		return line[:lead] + "@include " + rest[1:]
	}
	return line
}

func sassNameStart(c byte) bool {
	return c == '_' || c == '-' || c == '#' ||
		(c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isIdentChar(c byte) bool {
	return c == '_' || c == '-' ||
		(c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// topColon returns the index of the first top-level ':' in s (skipping "::",
// and colons inside brackets, strings or interpolation), or -1 if there is none.
func topColon(s string) int {
	depth := 0
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case '"', '\'':
			i++
			for i < len(s) && s[i] != c {
				if s[i] == '\\' {
					i++
				}
				i++
			}
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			if depth > 0 {
				depth--
			}
		case ':':
			if depth == 0 {
				if i+1 < len(s) && s[i+1] == ':' {
					i++
					continue
				}
				return i
			}
		}
	}
	return -1
}

// isPropertyDecl reports whether s (the trimmed header of a logical line) is a
// property declaration rather than a selector. In the indented syntax a
// property requires whitespace (or end of header) after its colon, so "a: b" is
// a property while "a:b" is a selector; custom properties ("--x") are always
// declarations.
func isPropertyDecl(s string) bool {
	ci := topColon(s)
	if strings.HasPrefix(s, "--") {
		return ci >= 0
	}
	if ci < 0 {
		return false
	}
	if ci+1 >= len(s) {
		return true
	}
	switch s[ci+1] {
	case ' ', '\t', '\n':
		return true
	}
	return false
}

func lineKind(header string) int {
	if header == "" {
		return kSelector
	}
	switch header[0] {
	case '$':
		return kVariable
	case '@':
		return kAtRule
	}
	if isPropertyDecl(header) {
		return kProperty
	}
	return kSelector
}

// atKeyword returns the lower-case keyword of an at-rule header ("@each ..." ->
// "each"), or "" if header is not an at-rule.
func atKeyword(header string) string {
	if header == "" || header[0] != '@' {
		return ""
	}
	i := 1
	for i < len(header) && (isIdentChar(header[i]) || header[i] == '-') {
		i++
	}
	return strings.ToLower(header[1:i])
}

// hasWord reports whether w appears in s as a whole identifier token.
func hasWord(s, w string) bool {
	from := 0
	for {
		i := strings.Index(s[from:], w)
		if i < 0 {
			return false
		}
		i += from
		before := i == 0 || !isIdentChar(s[i-1])
		after := i+len(w) >= len(s) || !isIdentChar(s[i+len(w)])
		if before && after {
			return true
		}
		from = i + 1
	}
}

// lastToken returns the trailing identifier word of s (letters/digits/-/_), or
// "" when s does not end in one.
func lastToken(s string) string {
	end := len(s)
	start := end
	for start > 0 && isIdentChar(s[start-1]) {
		start--
	}
	if start == end {
		return ""
	}
	return s[start:end]
}

// exprKeywords continue any expression header when they trail a line.
var exprKeywords = map[string]bool{"and": true, "or": true, "not": true}

// atKeywords continue an at-rule header when they trail a line.
var atKeywords = map[string]bool{
	"from": true, "through": true, "to": true, "in": true, "if": true,
	"as": true, "with": true, "using": true, "show": true, "hide": true,
}

// trailingContinues reports whether the final segment seg (the text after the
// last newline of the accumulated header) ends with an operator, comma,
// backslash or continuation keyword that expects more input. expr enables the
// arithmetic/expression operators; at enables the at-rule keywords.
func trailingContinues(seg string, expr, at bool) bool {
	t := strings.TrimRight(stripLineComment(seg), " \t")
	if t == "" {
		return false
	}
	switch last := t[len(t)-1]; last {
	case ',', '\\':
		return true
	case '=', '<', '>', '~', '!':
		if expr {
			return true
		}
	case '+', '-', '*', '/', '%':
		// Arithmetic operators only continue when spaced, so unit suffixes such
		// as "10%" or values like "1px" are not mistaken for a dangling operator.
		if expr && len(t) >= 2 && (t[len(t)-2] == ' ' || t[len(t)-2] == '\t') {
			return true
		}
	}
	if w := lastToken(t); w != "" {
		if expr && exprKeywords[w] {
			return true
		}
		if at && atKeywords[w] {
			return true
		}
	}
	return false
}

// stripLineComment removes a trailing "//" line comment from a single segment,
// respecting quotes.
func stripLineComment(s string) string {
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case '"', '\'':
			i++
			for i < len(s) && s[i] != c {
				if s[i] == '\\' {
					i++
				}
				i++
			}
		case '/':
			if i+1 < len(s) && s[i+1] == '/' {
				return s[:i]
			}
		}
	}
	return s
}

// atRuleHeaderIncomplete reports whether an at-rule header still needs more
// input to satisfy its own grammar (independently of trailing operators).
func atRuleHeaderIncomplete(kw, acc string) bool {
	switch kw {
	case "for":
		return !(hasWord(acc, "from") && (hasWord(acc, "through") || hasWord(acc, "to")))
	case "each":
		return !hasWord(acc, "in")
	case "if", "while", "return", "debug", "warn", "error", "extend":
		return afterKeywordEmpty(acc)
	case "else":
		// "@else if" needs a condition; a bare "@else" is complete.
		if hasWord(acc, "if") {
			return afterWord(acc, "if")
		}
		return false
	case "include", "mixin", "use", "forward":
		return afterKeywordEmpty(acc)
	case "function":
		// A function always has a parenthesised parameter list.
		return afterKeywordEmpty(acc) || !strings.Contains(acc, "(")
	}
	return false
}

// afterKeywordEmpty reports whether nothing but whitespace follows the leading
// at-keyword of acc.
func afterKeywordEmpty(acc string) bool {
	i := 1
	for i < len(acc) && (isIdentChar(acc[i]) || acc[i] == '-') {
		i++
	}
	return strings.TrimSpace(acc[i:]) == ""
}

// afterWord reports whether nothing but whitespace follows the last occurrence
// of whole word w in acc.
func afterWord(acc, w string) bool {
	idx := -1
	from := 0
	for {
		i := strings.Index(acc[from:], w)
		if i < 0 {
			break
		}
		i += from
		before := i == 0 || !isIdentChar(acc[i-1])
		after := i+len(w) >= len(acc) || !isIdentChar(acc[i+len(w)])
		if before && after {
			idx = i
		}
		from = i + 1
	}
	if idx < 0 {
		return false
	}
	return strings.TrimSpace(acc[idx+len(w):]) == ""
}

// headerIncomplete reports whether the accumulated header text is syntactically
// unfinished and should absorb the following physical line.
func headerIncomplete(acc string, kind int) bool {
	if depth, inStr := scanState(acc); depth > 0 || inStr {
		return true
	}
	seg := acc
	if i := strings.LastIndexByte(acc, '\n'); i >= 0 {
		seg = acc[i+1:]
	}
	expr := kind != kSelector
	at := kind == kAtRule
	if trailingContinues(seg, expr, at) {
		return true
	}
	switch kind {
	case kAtRule:
		trimmed := strings.TrimSpace(acc)
		return atRuleHeaderIncomplete(atKeyword(trimmed), trimmed)
	case kVariable:
		// A variable declaration needs a colon and a non-empty value; both may
		// appear on a following physical line.
		ci := topColon(acc)
		return ci < 0 || strings.TrimSpace(acc[ci+1:]) == ""
	}
	return false
}

// blockRequiring reports whether a childless statement of the given kind must
// still be written with an empty "{}" block in SCSS (selectors and block
// at-rules) rather than terminated with ";".
func blockRequiring(kind int, header string) bool {
	switch kind {
	case kSelector:
		return true
	case kAtRule:
		return blockAtRules[atKeyword(header)]
	}
	return false
}

type logicalLine struct {
	indent int
	text   string
}

// foldLogicalLines merges physical lines into logical statements, honouring
// header continuation.
func foldLogicalLines(phys []string) []logicalLine {
	var lls []logicalLine
	for i := 0; i < len(phys); {
		if strings.TrimSpace(phys[i]) == "" {
			i++
			continue
		}
		first := expandShorthand(phys[i])
		indent := indentOf(first)
		text := first
		i++
		kind := lineKind(strings.TrimSpace(first))
		for i < len(phys) && strings.TrimSpace(phys[i]) != "" && headerIncomplete(text, kind) {
			text += "\n" + phys[i]
			i++
		}
		lls = append(lls, logicalLine{indent: indent, text: text})
	}
	return lls
}

func convertIndented(src string) string {
	// The indented lexer treats CR, CRLF and form-feed as line breaks.
	src = strings.ReplaceAll(src, "\r\n", "\n")
	src = strings.ReplaceAll(src, "\r", "\n")
	src = strings.ReplaceAll(src, "\f", "\n")
	lls := foldLogicalLines(strings.Split(src, "\n"))
	var stack []int
	var out []string
	closeTo := func(indent int) {
		for len(stack) > 0 && indent <= stack[len(stack)-1] {
			stack = stack[:len(stack)-1]
			out = append(out, strings.Repeat("  ", len(stack))+"}")
		}
	}
	for idx, ll := range lls {
		content := strings.TrimSpace(ll.text)
		closeTo(ll.indent)
		pad := strings.Repeat("  ", len(stack))
		if strings.HasPrefix(content, "//") || strings.HasPrefix(content, "/*") {
			out = append(out, pad+content)
			continue
		}
		ni := -1
		if idx+1 < len(lls) {
			ni = lls[idx+1].indent
		}
		if ni > ll.indent {
			out = append(out, pad+content+" {")
			stack = append(stack, ll.indent)
		} else if blockRequiring(lineKind(content), content) {
			out = append(out, pad+content+" {}")
		} else {
			out = append(out, pad+content+";")
		}
	}
	for len(stack) > 0 {
		stack = stack[:len(stack)-1]
		out = append(out, strings.Repeat("  ", len(stack))+"}")
	}
	return strings.Join(out, "\n")
}
