// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import (
	"strings"
	"testing"
)

// TestCustomPropertyValueTokenStream covers the custom-property (`--*`) value
// scanner: the value is a verbatim token stream (SassScript only inside #{}),
// `//` is literal, `/* */` loud comments and every bracket kind are preserved,
// whitespace is folded (a run collapses to its last character; a trailing run
// with a newline becomes a single space), and multiline values are re-indented
// to the current nesting at serialization. Byte-exact against dart-sass 1.102.
func TestCustomPropertyValueTokenStream(t *testing.T) {
	cases := []struct{ in, out string }{
		// A lone variable-looking expression stays literal; only #{} is evaluated.
		{".a{--c: 1 + 2}", ".a {\n  --c: 1 + 2;\n}\n"},
		{".a{--i: #{1 + 2}}", ".a {\n  --i: 3;\n}\n"},
		// `//` is literal text, not a silent comment.
		{".a{--s: c // d}", ".a {\n  --s: c // d;\n}\n"},
		// Loud comments are preserved verbatim.
		{".a{--l: c /* d */ e}", ".a {\n  --l: c /* d */ e;\n}\n"},
		// The single space after the colon is a token and is preserved.
		{".a{--e: ;}", ".a {\n  --e: ;\n}\n"},
		// Every bracket kind is tracked so `;` inside does not terminate.
		{".a{--p: (x; y)}", ".a {\n  --p: (x; y);\n}\n"},
		{".a{--sq: [x; y]}", ".a {\n  --sq: [x; y];\n}\n"},
		{".a{--cu: {x; y}}", ".a {\n  --cu: {x; y};\n}\n"},
		// A quoted string is tokenised as a string (its content kept verbatim).
		{".a{--q: \"f'o\" 'b\"r'}", ".a {\n  --q: \"f'o\" 'b\"r';\n}\n"},
		// A run of whitespace collapses to its last character (tab kept as tab).
		{".a{--t: a\tb}", ".a {\n  --t: a\tb;\n}\n"},
		{".a{--w: a  b}", ".a {\n  --w: a b;\n}\n"},
		// An escape is copied verbatim so `\;` does not terminate the value.
		{".a{--x: a\\;b}", ".a {\n  --x: a\\;b;\n}\n"},
		// A trailing newline collapses to a single space.
		{".a {\n  --n: c\n}", ".a {\n  --n: c ;\n}\n"},
		// A multiline value is re-indented to min(continuation indent, name column).
		{".a {\n  --m: y\n      z;\n}", ".a {\n  --m: y\n      z;\n}\n"},
		// A deeply-indented source declaration re-indents by its own column.
		{".a {\n         --deep: y\n           z;\n}", ".a {\n  --deep: y\n    z;\n}\n"},
		// A fully-interpolated name is a normal declaration (value evaluated);
		// a name whose plain prefix is `--` is a custom property (value literal).
		{".a {\n  #{--y}: 1 + 2;\n  --#{z}: 1 + 2;\n}", ".a {\n  --y: 3;\n  --z: 1 + 2;\n}\n"},
		// An empty value is a single empty part.
		{".a {\n  --e:;\n}", ".a {\n  --e:;\n}\n"},
		// Trailing whitespace (spaces after a newline) collapses to one space.
		{".a {\n  --x: c\n   }", ".a {\n  --x: c ;\n}\n"},
		// Several continuation lines are re-indented against the shallowest.
		{".a {\n  --x: p\n    q\n      r;\n}", ".a {\n  --x: p\n    q\n      r;\n}\n"},
	}
	for _, c := range cases {
		if got := compile(t, c.in); got != c.out {
			t.Errorf("for %q:\n want: %q\n  got: %q", c.in, c.out, got)
		}
	}
	// Compressed output writes the raw value with no trailing semicolon.
	if got := compileC(t, ".a{--x: 1 + 2}"); !strings.Contains(got, ".a{--x: 1 + 2}") || strings.Contains(got, "2;") {
		t.Errorf("compressed custom property: got %q", got)
	}
}

// TestCssCustomFunction covers the plain-CSS custom-function at-rule
// (`@function --a()`), which dart-sass never resolves to a Sass function: the
// keyword case is preserved, the prelude is a verbatim token stream, a plain
// `result:` (any case) declaration takes a token-stream value, and other
// declarations evaluate normally. Byte-exact against dart-sass 1.102.
func TestCssCustomFunction(t *testing.T) {
	cases := []struct{ in, out string }{
		{"@function --a() {\n  result: $b;\n}", "@function --a() {\n  result: $b;\n}\n"},
		{"@FUNCTION --a() {\n  RESULT: $b;\n}", "@FUNCTION --a() {\n  RESULT: $b;\n}\n"},
		{"@function --a() {\n  result: #{1 + 1};\n}", "@function --a() {\n  result: 2;\n}\n"},
		// A non-result declaration evaluates as usual.
		{"@function --a() {\n  d: 1 + 1;\n}", "@function --a() {\n  d: 2;\n}\n"},
		// An interpolated declaration name is not the special `result:`.
		{"@function --a() {\n  #{result}: 1 + 1;\n}", "@function --a() {\n  result: 2;\n}\n"},
		// A parameterised prelude is preserved verbatim.
		{"@function --a(--b <color>) {result: c}", "@function --a(--b <color>) {\n  result: c;\n}\n"},
		// The childless form.
		{"@FUNCTION --a() x;", "@FUNCTION --a() x;\n"},
		// A real Sass @function is still a definition (produces no CSS).
		{"@function real() {@return 1}\n.a {b: real()}", ".a {\n  b: 1;\n}\n"},
		// A `--`-prefixed function call is a plain-CSS function, never a Sass one
		// (normalisation would otherwise alias `--a` to `__a`).
		{"@function __a() {@return 1}\n.b {c: --a()}", ".b {\n  c: --a();\n}\n"},
	}
	for _, c := range cases {
		if got := compile(t, c.in); got != c.out {
			t.Errorf("for %q:\n want: %q\n  got: %q", c.in, c.out, got)
		}
	}
}

// TestPercentLiteralToken covers a bare "%" in value position: it is a literal
// token when it has no operands, and the modulo operator between two operands.
func TestPercentLiteralToken(t *testing.T) {
	cases := []struct{ in, out string }{
		{"a {b: %}", "a {\n  b: %;\n}\n"},
		{"a {b: % c}", "a {\n  b: % c;\n}\n"},
		{"a {b: c %}", "a {\n  b: c %;\n}\n"},
		{"a {b: c(%)}", "a {\n  b: c(%);\n}\n"},
		{"a {b: c(d %)}", "a {\n  b: c(d %);\n}\n"},
		{"a {b: 3 % 2}", "a {\n  b: 1;\n}\n"},
	}
	for _, c := range cases {
		if got := compile(t, c.in); got != c.out {
			t.Errorf("for %q:\n want: %q\n  got: %q", c.in, c.out, got)
		}
	}
	// Modulo between incompatible operands is a runtime error.
	if _, err := Render("a {b: c % d}", false, false, nil); err == nil || !strings.Contains(err.Error(), "Undefined operation") {
		t.Errorf("expected undefined-operation error, got %v", err)
	}
}

// TestUnicodeRange covers the `U+…` unicode-range token: single/range/wildcard
// forms, case preservation, and the token boundary that lets a following
// identifier start a fresh space-list element without whitespace.
func TestUnicodeRange(t *testing.T) {
	cases := []struct{ in, out string }{
		{"a {b: U+1}", "a {\n  b: U+1;\n}\n"},
		{"a {b: u+1a2b}", "a {\n  b: u+1a2b;\n}\n"},
		{"a {b: U+1A2B-F9E8}", "a {\n  b: U+1A2B-F9E8;\n}\n"},
		{"a {b: U+000001-7}", "a {\n  b: U+000001-7;\n}\n"},
		{"a {b: U+????}", "a {\n  b: U+????;\n}\n"},
		{"a {b: U+123???}", "a {\n  b: U+123???;\n}\n"},
		{"a {b: U+A?BCDE}", "a {\n  b: U+A? BCDE;\n}\n"},
		{"a {b: U+A?-BCDE}", "a {\n  b: U+A? -BCDE;\n}\n"},
		{"a {b: U+A?-1234}", "a {\n  b: U+A?-1234;\n}\n"},
		// A "u" not followed by "+hex/?" is an ordinary identifier; a spaced
		// "U + 1" is string concatenation, not a unicode range.
		{"a {b: url(x)}", "a {\n  b: url(x);\n}\n"},
		{"a {b: U + 1}", "a {\n  b: U1;\n}\n"},
	}
	for _, c := range cases {
		if got := compile(t, c.in); got != c.out {
			t.Errorf("for %q:\n want: %q\n  got: %q", c.in, c.out, got)
		}
	}
}

// TestCustomPropertyIdentifiers covers `--`, `---` and `--1` parsed as
// custom-property-style identifiers (e.g. inside var()).
func TestCustomPropertyIdentifiers(t *testing.T) {
	cases := []struct{ in, out string }{
		{"a {b: var(--1)}", "a {\n  b: var(--1);\n}\n"},
		{"a {b: var(--)}", "a {\n  b: var(--);\n}\n"},
		{"a {b: var(---)}", "a {\n  b: var(---);\n}\n"},
	}
	for _, c := range cases {
		if got := compile(t, c.in); got != c.out {
			t.Errorf("for %q:\n want: %q\n  got: %q", c.in, c.out, got)
		}
	}
}

// TestAttributeSelectorQuoting covers attribute-value quoting: a quoted source
// value is unquoted only when it is a plain identifier needing no escapes, so a
// backslash, a digit start or a dash-only value stays quoted; an unquoted source
// value is kept verbatim (escapes preserved).
func TestAttributeSelectorQuoting(t *testing.T) {
	cases := []struct{ in, out string }{
		{`[a="\\"] {c: d}`, "[a=\"\\\\\"] {\n  c: d;\n}\n"},
		{`[a="foo"] {c: d}`, "[a=foo] {\n  c: d;\n}\n"},
		{`[a="1"] {c: d}`, "[a=\"1\"] {\n  c: d;\n}\n"},
		{`[a="-"] {c: d}`, "[a=\"-\"] {\n  c: d;\n}\n"},
		{`[a="a b"] {c: d}`, "[a=\"a b\"] {\n  c: d;\n}\n"},
		{`[a=\31] {c: d}`, "[a=\\31 ] {\n  c: d;\n}\n"},
		// A hex escape inside a quoted value is decoded then re-serialised; a
		// trailing space after the hex digits is consumed as part of the escape.
		{`[a="\41"] {c: d}`, "[a=A] {\n  c: d;\n}\n"},
		{`[a="\41 b"] {c: d}`, "[a=Ab] {\n  c: d;\n}\n"},
		// A zero / out-of-range / surrogate codepoint escape decodes to U+FFFD.
		{`[a="\0 z"] {c: d}`, "@charset \"UTF-8\";\n[a=�z] {\n  c: d;\n}\n"},
	}
	for _, c := range cases {
		if got := compile(t, c.in); got != c.out {
			t.Errorf("for %q:\n want: %q\n  got: %q", c.in, c.out, got)
		}
	}
	// A backslash at end-of-input inside a quoted attribute value is tolerated
	// (the scanner stops) and surfaces as a parse error at the missing "]".
	// The interpolation resolves the selector to `[a="x\`, driving the selector
	// parser's attribute-value scanner to end-of-input on the backslash.
	if _, err := Render(`#{'[a="x' + '\\'} {c: d}`, false, false, nil); err == nil {
		t.Error("expected error for unterminated attribute value")
	}
}

// TestUnknownDirectiveEdges covers unknown at-rule preludes (leading comments
// discarded, inner/trailing loud comments preserved) and childless at-rules
// interleaving with declarations inside a style rule.
func TestUnknownDirectiveEdges(t *testing.T) {
	cases := []struct{ in, out string }{
		{"@a /**/ b", "@a b;\n"},
		{"@a b /**/", "@a b /**/;\n"},
		{"@a /**/", "@a;\n"},
		{"@asdf foo /* bar */ baz;", "@asdf foo /* bar */ baz;\n"},
		// A childless at-rule stays inside the enclosing style rule.
		{"a {\n  @b c;\n}", "a {\n  @b c;\n}\n"},
		// It interleaves with declarations after a nested rule.
		{"a {\n  b {c: d}\n  @e f;\n}", "a b {\n  c: d;\n}\na {\n  @e f;\n}\n"},
		// A prelude may embed strings, brackets and interpolation.
		{"@x (p) [q] \"s\" #{1 + 1};", "@x (p) [q] \"s\" 2;\n"},
		// @-moz-document is dart-special: loud comments are ordinary whitespace.
		{"@-moz-document url-prefix(a) /**/ {}", "@-moz-document url-prefix(a) {}\n"},
		// An empty prelude.
		{"@x;", "@x;\n"},
		// A silent comment inside a prelude is stripped to end-of-line.
		{"@x a //\n b;", "@x a \n b;\n"},
	}
	for _, c := range cases {
		if got := compile(t, c.in); got != c.out {
			t.Errorf("for %q:\n want: %q\n  got: %q", c.in, c.out, got)
		}
	}
	// An unterminated interpolation in a prelude is an error.
	if _, err := Render("@x #{1", false, false, nil); err == nil {
		t.Error("expected error for unterminated interpolation in at-rule prelude")
	}
	// An unterminated interpolation in a custom-property value is an error.
	if _, err := Render(".a{--x: #{1", false, false, nil); err == nil {
		t.Error("expected error for unterminated interpolation in custom property")
	}
}
