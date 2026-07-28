// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import "testing"

// TestInterpolatedIdentifierValues covers parsing an identifier that embeds
// interpolation in value position (continueInterpIdent, and the branches of
// parseIdentValue / parseHashValue that hand off to it), plus the quoted-string
// space-list adjacency they rely on.
func TestInterpolatedIdentifierValues(t *testing.T) {
	cases := []struct{ in, out string }{
		{".a{b: foo#{1}}", ".a {\n  b: foo1;\n}\n"},              // ident then interpolation
		{".a{b: #{1}bar}", ".a {\n  b: 1bar;\n}\n"},              // interpolation then ident
		{".a{b: #{1}#{2}}", ".a {\n  b: 12;\n}\n"},               // interpolation then interpolation
		{".a{b: a#{1}\\.b}", ".a {\n  b: a1\\.b;\n}\n"},          // escape inside interpolated ident
		{`.a{b: "x"foo"y"}`, ".a {\n  b: \"x\" foo \"y\";\n}\n"}, // adjacency at quote boundaries
		{".a{b: literal$c}", ".a {\n  b: literal literal;\n}\n"}, // adjacency at a variable boundary
		{".a{b: $c#{$c}}", ".a {\n  b: literal literal;\n}\n"},   // variable then interpolation
	}
	for _, c := range cases {
		src := "$c: literal;\n" + c.in
		if got := compile(t, src); got != c.out {
			t.Errorf("for %q:\n want: %q\n  got: %q", c.in, c.out, got)
		}
	}
}

// TestCanonicalEscapeSerialization covers the escape-canonicalisation branches
// of scanEscape / canonicalEscape: bare name-char escapes, single- and
// double-hex-digit control escapes, a leading escaped digit, and the
// backslash-plus-literal fallback.
func TestCanonicalEscapeSerialization(t *testing.T) {
	cases := []struct{ in, out string }{
		{`.a{b: \.}`, ".a {\n  b: \\.;\n}\n"},                 // fallback: backslash + literal
		{`.a{b: \1 x}`, ".a {\n  b: \\1 x;\n}\n"},             // control escape, single hex nibble
		{`.a{b: \7f x}`, ".a {\n  b: \\7f x;\n}\n"},           // control escape, two hex nibbles
		{`.a{b: \31 foo}`, ".a {\n  b: \\31 foo;\n}\n"},       // leading escaped digit
		{`.a{b: l\\ite\ral}`, ".a {\n  b: l\\\\iteral;\n}\n"}, // \\ kept, \r -> r
	}
	for _, c := range cases {
		if got := compile(t, c.in); got != c.out {
			t.Errorf("for %q:\n want: %q\n  got: %q", c.in, c.out, got)
		}
	}
}

// TestUnquotedSerialization covers serializeUnquoted's final-mode branches:
// Private Use Area escaping (with and without a disambiguating trailing space),
// newline collapsing, and the compressed pass-through that leaves PUA literal.
func TestUnquotedSerialization(t *testing.T) {
	expectEq(t, ".a{b: #{\"\ue600\"}}", ".a {\n  b: \\e600;\n}\n")           // PUA escaped, no trailing space
	expectEq(t, ".a{b: #{\"\ue600\" + \"a\"}}", ".a {\n  b: \\e600 a;\n}\n") // PUA escaped, trailing space before hex
	expectEq(t, "$x: \"a\\a b\";\n.a{b: #{$x}}", ".a {\n  b: a b;\n}\n")     // newline collapses, following space swallowed
	expectEq(t, "$x: \"p q\\a r\";\n.a{b: #{$x}}", ".a {\n  b: p q r;\n}\n") // literal space kept when not after a newline
	expectEq(t, "$x: \"p\\a  q\";\n.a{b: #{$x}}", ".a {\n  b: p q;\n}\n")    // space right after a newline is swallowed
	if got := compileC(t, ".a{b: \ue600}"); got != "\ufeff.a{b:\ue600}\n" {
		t.Errorf("compressed PUA: got %q", got)
	}
}

// TestPlusUnaryNonNumber covers evalUnary's unary-plus branch for a value that
// is not a number, where dart-sass keeps the sign as literal text.
func TestPlusUnaryNonNumber(t *testing.T) {
	expectEq(t, ".a{b: +foo(12px)}", ".a {\n  b: +foo(12px);\n}\n")
	expectEq(t, ".a{b: +5}", ".a {\n  b: 5;\n}\n")
}

// TestBangFlagWhitespace covers the whitespace-tolerant `!` flag path.
func TestBangFlagWhitespace(t *testing.T) {
	expectEq(t, "div{a: red ! important}", "div {\n  a: red !important;\n}\n")
}

// TestQuotedTabLiteral covers serializeQuoted leaving a tab literal (the one
// control character dart-sass does not escape inside quoted strings) and the
// disambiguating space written after a control-character escape whose following
// character could be read as part of it.
func TestQuotedTabLiteral(t *testing.T) {
	expectEq(t, ".a{b: \"x\ty\"}", ".a {\n  b: \"x\ty\";\n}\n")
	expectEq(t, `.a{b: "\1 5"}`, ".a {\n  b: \"\\1 5\";\n}\n")
}

// TestBlankListDeclarationOmitted covers isBlankValue dropping a declaration
// whose list value serializes to nothing.
func TestBlankListDeclarationOmitted(t *testing.T) {
	expectEq(t, ".a{b: (null null null); c: 1}", ".a {\n  c: 1;\n}\n")
}

// TestEscapeSequenceAtNewlineErrors covers scanEscape's error branch when a
// backslash is not followed by an escape sequence.
func TestEscapeSequenceAtNewlineErrors(t *testing.T) {
	mustErr(t, ".a{b: x\\\n}")
}

// TestUnclosedInterpolatedIdentifier covers continueInterpIdent's error branch
// when a trailing interpolation is left unclosed.
func TestUnclosedInterpolatedIdentifier(t *testing.T) {
	mustErr(t, ".a{b: #{1}#{2")
}
