// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import "testing"

func TestConvertIndentedContinuation(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"bracket_open", "@function a(\n  $b, $c)", "@function a(\n  $b, $c) {}"},
		{"string_open", "a\n  b: 'x \\\n  y'", "a {\n  b: 'x \\\n  y';\n}"},
		{"trailing_binary", "$a: b +\nc\nd\n  e: $a", "$a: b +\nc;\nd {\n  e: $a;\n}"},
		{"trailing_comma_selector", "a,\nb\n  c: d", "a,\nb {\n  c: d;\n}"},
		{"leading_operator_not_joined", "$a: b\n+ c", "$a: b;\n+ c {}"},
		{"unary_keyword", "$a: not\nb\nc\n  d: $a", "$a: not\nb;\nc {\n  d: $a;\n}"},
		{"for_full", "@for\n  $i from 1 through 10", "@for\n  $i from 1 through 10 {}"},
		{"for_partial", "@for $i from 1\n  through 10", "@for $i from 1\n  through 10 {}"},
		{"each_in", "@each $a in\n  b, c\n  x\n    d: $a", "@each $a in\n  b, c {\n  x {\n    d: $a;\n  }\n}"},
		{"each_destructure", "@each $a,\n  $b in (c: d)\n  x\n    e: $b", "@each $a,\n  $b in (c: d) {\n  x {\n    e: $b;\n  }\n}"},
		{"if_empty", "a\n  @if\n    true\n    e: f", "a {\n  @if\n    true {\n    e: f;\n  }\n}"},
		{"else_if", "@if true\n  a: b\n@else if\n  false\n  c: d", "@if true {\n  a: b;\n}\n@else if\n  false {\n  c: d;\n}"},
		{"variable_no_colon", "$a\n  : b", "$a\n  : b;"},
		{"variable_empty_value", "$a:\n  b", "$a:\n  b;"},
		{"function_no_paren", "@function a\n  ()", "@function a\n  () {}"},
		{"bang_important", "a\n  b: c !\n    important", "a {\n  b: c !\n    important;\n}"},
		{"include_shorthand", "=m\n  x: 1\nd\n  +m", "@mixin m {\n  x: 1;\n}\nd {\n  @include m;\n}"},
		{"mixin_shorthand_newline", "=\n  m\n    x: 1", "@mixin \n  m {\n  x: 1;\n}"},
		{"plus_space_selector", "d\n  + a\n    x: 1", "d {\n  + a {\n    x: 1;\n  }\n}"},
		{"childless_selector", "a\nb\n  c: d", "a {}\nb {\n  c: d;\n}"},
		{"childless_media", "@media screen", "@media screen {}"},
		{"unknown_atrule_stmt", "@value x 1", "@value x 1;"},
		{"loud_comment", "/* c */", "/* c */"},
		{"silent_comment", "// c\na\n  b: d", "a {\n  b: d;\n}"},
		{"silent_comment_spans_deeper", "// c\n  deeper\na\n  b: d", "a {\n  b: d;\n}"},
		{"trailing_silent", "a\n  b: c // note\n  d: e", "a {\n  b: c;\n  d: e;\n}"},
		{"trailing_loud", "a\n  b: c; /* f */\n  d: e", "a {\n  b: c;;\n  d: e;\n}"},
		{"inline_loud_whitespace", "a\n  b: 1 /* f */ 2", "a {\n  b: 1   2;\n}"},
		{"trailing_silent_directive", "@for $i from 1 through 3 //\n  a\n    b: c", "@for $i from 1 through 3 {\n  a {\n    b: c;\n  }\n}"},
		{"silent_in_folded_header", "@for $i from //\n  1 through 3\n  a\n    b: c", "@for $i from \n  1 through 3 {\n  a {\n    b: c;\n  }\n}"},
		{"custom_prop_keeps_loud", "a\n  --b: c /* f */", "a {\n  --b: c /* f */;\n}"},
		{"custom_prop_keeps_silent", "a\n  --b: c // f", "a {\n  --b: c // f;\n}"},
		{"url_scheme_slashes_kept", "a\n  b: url(http://ex.com/x) // c", "a {\n  b: url(http://ex.com/x);\n}"},
		{"comment_in_string_kept", "a\n  b: \"x /* y */ z\"", "a {\n  b: \"x /* y */ z\";\n}"},
		{"comment_in_interp_kept", "a\n  b: #{1 + 2}px", "a {\n  b: #{1 + 2}px;\n}"},
		{"crlf", "a\r\n  b: c", "a {\n  b: c;\n}"},
		{"formfeed", "a\f  b: c", "a {\n  b: c;\n}"},
		{"blank_lines", "a\n\n  b: c\n", "a {\n  b: c;\n}"},
		{"forward_keyword", "@forward \"x\" show\n  $a", "@forward \"x\" show\n  $a;"},
		{"extend_empty", "d\n  @extend\n    a\nb\n  e: f", "d {\n  @extend\n    a;\n}\nb {\n  e: f;\n}"},
		{"tab_indent", "a\n\tb: c", "a {\n  b: c;\n}"},
	}
	for _, c := range cases {
		if got := convertIndented(c.in); got != c.want {
			t.Errorf("%s:\n got: %q\nwant: %q", c.name, got, c.want)
		}
	}
}

func TestStripIndentedComments(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"plain", "plain", "plain"},
		{"inline_loud_space", "a /* c */ b", "a   b"},
		{"trailing_silent", "a // c", "a"},
		{"silent_keeps_continuation", "a // c\n  b", "a \n  b"},
		{"silent_no_newline", "a //", "a"},
		{"loud_in_single_string", "'x // y'", "'x // y'"},
		{"loud_in_double_string", "\"x /* y */ z\"", "\"x /* y */ z\""},
		{"escaped_quote_in_string", "\"a\\\"b\"", "\"a\\\"b\""},
		{"unterminated_string", "'abc", "'abc"},
		{"interp_kept", "#{a}b", "#{a}b"},
		{"interp_nested_braces", "#{a{b}}x", "#{a{b}}x"},
		{"unterminated_interp", "#{ab", "#{ab"},
		{"url_scheme_slashes", "url(http://x//y)", "url(http://x//y)"},
		{"url_uppercase", "URL(a)", "URL(a)"},
		{"url_nested_parens", "url(a(b)c)", "url(a(b)c)"},
		{"url_quote_inside", "url('a//b')", "url('a//b')"},
		{"url_escaped_quote", "url(\"a\\\"b\")", "url(\"a\\\"b\")"},
		{"unterminated_url", "url(abc", "url(abc"},
		{"not_url_preceded_by_ident", "myurl(x) // c", "myurl(x)"},
		{"unterminated_loud", "a /* bc", "a"},
	}
	for _, c := range cases {
		if got := stripIndentedComments(c.in); got != c.want {
			t.Errorf("%s: stripIndentedComments(%q)=%q want %q", c.name, c.in, got, c.want)
		}
	}
}

func TestMatchesURLOpen(t *testing.T) {
	if !matchesURLOpen("url(x)", 0) {
		t.Error("url at start")
	}
	if !matchesURLOpen("a url(x)", 2) {
		t.Error("url after space")
	}
	if matchesURLOpen("xurl(x)", 1) {
		t.Error("url preceded by ident is not a url token")
	}
	if matchesURLOpen("ur", 0) {
		t.Error("too short")
	}
	if matchesURLOpen("abc(", 0) {
		t.Error("not url")
	}
}

func TestScanState(t *testing.T) {
	if d, s := scanState("a(b[c]"); d != 1 || s {
		t.Errorf("nested brackets: %d %v", d, s)
	}
	if d, s := scanState("a '\\'' b"); d != 0 || s {
		t.Errorf("escaped quote closed: %d %v", d, s)
	}
	if _, s := scanState("x /* open"); s {
		t.Errorf("block comment open is not a string: %v", s)
	}
}

func TestIndentOf(t *testing.T) {
	if indentOf("  x") != 2 {
		t.Error("spaces")
	}
	if indentOf("\tx") != 8 {
		t.Error("tab")
	}
	if indentOf("x") != 0 {
		t.Error("none")
	}
	if indentOf("") != 0 {
		t.Error("empty")
	}
}

func TestExpandShorthand(t *testing.T) {
	if got := expandShorthand("  =a"); got != "  @mixin a" {
		t.Errorf("mixin: %q", got)
	}
	if got := expandShorthand("+a(1)"); got != "@include a(1)" {
		t.Errorf("include: %q", got)
	}
	if got := expandShorthand("+ a"); got != "+ a" {
		t.Errorf("plus-space kept: %q", got)
	}
	if got := expandShorthand("+"); got != "+" {
		t.Errorf("bare plus kept: %q", got)
	}
	if got := expandShorthand(".x"); got != ".x" {
		t.Errorf("selector kept: %q", got)
	}
}

func TestSassNameStart(t *testing.T) {
	for _, c := range []byte{'_', '-', '#', 'a', 'Z'} {
		if !sassNameStart(c) {
			t.Errorf("expected name start: %c", c)
		}
	}
	for _, c := range []byte{' ', '1', '('} {
		if sassNameStart(c) {
			t.Errorf("unexpected name start: %c", c)
		}
	}
}

func TestTopColonAndProperty(t *testing.T) {
	if topColon("a:b") != 1 {
		t.Error("simple colon")
	}
	if topColon("a::before") != -1 {
		t.Error("double colon skipped")
	}
	if topColon("url(a:b)") != -1 {
		t.Error("colon in paren skipped")
	}
	if topColon(`"a:b"`) != -1 {
		t.Error("colon in string skipped")
	}
	if topColon(`"a\"b":c`) != 6 {
		t.Error("colon after escaped-quote string")
	}
	if topColon("abc") != -1 {
		t.Error("no colon")
	}
	if topColon("]a:b") != 2 {
		t.Error("stray close bracket clamps")
	}
	if !isPropertyDecl("a: b") {
		t.Error("space after colon -> property")
	}
	if isPropertyDecl("a:b") {
		t.Error("no space after colon -> selector")
	}
	if !isPropertyDecl("a:") {
		t.Error("colon at end -> property")
	}
	if !isPropertyDecl("--x:y") {
		t.Error("custom property")
	}
	if isPropertyDecl("--x") {
		t.Error("custom property without colon")
	}
	if isPropertyDecl("a") {
		t.Error("no colon -> not property")
	}
}

func TestLineKind(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"", kSelector},
		{"$a: b", kVariable},
		{"@media x", kAtRule},
		{"a: b", kProperty},
		{".sel", kSelector},
	}
	for _, c := range cases {
		if got := lineKind(c.in); got != c.want {
			t.Errorf("lineKind(%q)=%d want %d", c.in, got, c.want)
		}
	}
}

func TestAtKeyword(t *testing.T) {
	if atKeyword("@each $a") != "each" {
		t.Error("each")
	}
	if atKeyword("@-webkit-keyframes x") != "-webkit-keyframes" {
		t.Error("vendor")
	}
	if atKeyword("selector") != "" {
		t.Error("not at-rule")
	}
	if atKeyword("") != "" {
		t.Error("empty")
	}
}

func TestHasWordAndLastToken(t *testing.T) {
	if !hasWord("@for $i from 1", "from") {
		t.Error("from present")
	}
	if hasWord("performance", "from") {
		t.Error("from not a whole word")
	}
	if hasWord("xfromx", "from") {
		t.Error("from embedded")
	}
	if hasWord("abc", "from") {
		t.Error("no from")
	}
	if lastToken("a b cd") != "cd" {
		t.Error("trailing word")
	}
	if lastToken("a +") != "" {
		t.Error("trailing operator has no word")
	}
}

func TestTrailingContinues(t *testing.T) {
	if !trailingContinues("a,", false, false) {
		t.Error("comma")
	}
	if !trailingContinues("a \\", false, false) {
		t.Error("backslash")
	}
	if !trailingContinues("a !", true, false) {
		t.Error("bang expr")
	}
	if trailingContinues("a !", false, false) {
		t.Error("bang non-expr")
	}
	if !trailingContinues("a +", true, false) {
		t.Error("spaced plus")
	}
	if trailingContinues("10%", true, false) {
		t.Error("unit percent not operator")
	}
	if !trailingContinues("a and", true, false) {
		t.Error("expr keyword")
	}
	if !trailingContinues("x show", false, true) {
		t.Error("at keyword")
	}
	if trailingContinues("x show", false, false) {
		t.Error("at keyword needs at context")
	}
	if trailingContinues("", true, true) {
		t.Error("empty")
	}
	if trailingContinues("plain", true, true) {
		t.Error("no trailing")
	}
	if trailingContinues("a // + ", true, false) {
		t.Error("operator in comment stripped")
	}
}

func TestStripLineComment(t *testing.T) {
	if stripLineComment("a // b") != "a " {
		t.Error("strip")
	}
	if stripLineComment(`"//" x`) != `"//" x` {
		t.Error("slashes in string kept")
	}
	if stripLineComment("a b") != "a b" {
		t.Error("no comment")
	}
	if got := stripLineComment(`"a\"b" // c`); got != `"a\"b" ` {
		t.Errorf("escaped quote in string: %q", got)
	}
}

func TestAtRuleHeaderIncomplete(t *testing.T) {
	cases := []struct {
		kw, acc string
		want    bool
	}{
		{"for", "@for $i", true},
		{"for", "@for $i from 1 through 3", false},
		{"for", "@for $i from 1 to 3", false},
		{"each", "@each $a", true},
		{"each", "@each $a in b", false},
		{"if", "@if", true},
		{"if", "@if true", false},
		{"while", "@while x", false},
		{"return", "@return", true},
		{"else", "@else", false},
		{"else", "@else if", true},
		{"else", "@else if x", false},
		{"include", "@include", true},
		{"mixin", "@mixin m", false},
		{"function", "@function a", true},
		{"function", "@function a()", false},
		{"use", "@use", true},
		{"forward", "@forward \"x\"", false},
		{"extend", "@extend", true},
		{"media", "@media screen", false},
	}
	for _, c := range cases {
		if got := atRuleHeaderIncomplete(c.kw, c.acc); got != c.want {
			t.Errorf("atRuleHeaderIncomplete(%q,%q)=%v want %v", c.kw, c.acc, got, c.want)
		}
	}
}

func TestAfterWordNoMatch(t *testing.T) {
	if afterWord("no keyword here", "if") {
		t.Error("missing word -> false")
	}
	if !afterWord("a if", "if") {
		t.Error("trailing word -> empty after")
	}
	if afterWord("a if x", "if") {
		t.Error("content after word")
	}
}

func TestHeaderIncomplete(t *testing.T) {
	if !headerIncomplete("a(", kSelector) {
		t.Error("open bracket")
	}
	if !headerIncomplete("$a", kVariable) {
		t.Error("variable no colon")
	}
	if headerIncomplete("$a: b", kVariable) {
		t.Error("variable complete")
	}
	if !headerIncomplete("@each $a", kAtRule) {
		t.Error("atrule incomplete")
	}
	if headerIncomplete("a: b", kProperty) {
		t.Error("property complete")
	}
	if headerIncomplete("sel", kSelector) {
		t.Error("selector complete")
	}
}

func TestBlockRequiring(t *testing.T) {
	if !blockRequiring(kSelector, "a") {
		t.Error("selector")
	}
	if !blockRequiring(kAtRule, "@media x") {
		t.Error("block atrule")
	}
	if blockRequiring(kAtRule, "@include m") {
		t.Error("stmt atrule")
	}
	if blockRequiring(kProperty, "a: b") {
		t.Error("property")
	}
	if blockRequiring(kVariable, "$a: b") {
		t.Error("variable")
	}
}
