// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import "testing"

// TestLeadingSlashValue exercises a value that begins with "/" (a slash
// separator with an empty left-hand side), including the degenerate slash chain
// that nests such expressions on the right.
func TestLeadingSlashValue(t *testing.T) {
	cases := []struct{ src, want string }{
		{"a{b: /bar}", "a {\n  b: /bar;\n}\n"},
		{"a{b: 1/ / /bar}", "a {\n  b: 1///bar;\n}\n"},
		{"a{b: 1/ /bar}", "a {\n  b: 1//bar;\n}\n"},
	}
	for _, c := range cases {
		if got := compile(t, c.src); got != c.want {
			t.Errorf("for %q:\n want: %q\n  got: %q", c.src, c.want, got)
		}
	}
}

// TestMinusTokenBoundary exercises dart-sass's disambiguation of "-": an
// interpolated identifier after a non-absorbing token (here a quoted string)
// starts a fresh space-list element even without intervening whitespace, while
// a number or quote after the "-" keeps it a binary subtraction.
func TestMinusTokenBoundary(t *testing.T) {
	cases := []struct{ src, want string }{
		{"a{b: \"x\"-#{y}}", "a {\n  b: \"x\" -y;\n}\n"},     // -#{...} interp ident -> list
		{"a{b: \"x\"-y}", "a {\n  b: \"x\" -y;\n}\n"},        // -ident -> list
		{"a{b: \"x\"-2}", "a {\n  b: \"x\"-2;\n}\n"},         // -number, no space -> binary
		{"a{b: \"x\"-\"y\"}", "a {\n  b: \"x\"-\"y\";\n}\n"}, // -quote -> binary
		{"a{b: c -(d)}", "a {\n  b: c-d;\n}\n"},              // -( -> binary
		{"a{b: 1 -2}", "a {\n  b: 1 -2;\n}\n"},               // space + number -> list
	}
	for _, c := range cases {
		if got := compile(t, c.src); got != c.want {
			t.Errorf("for %q:\n want: %q\n  got: %q", c.src, c.want, got)
		}
	}
}

// TestHashIDToken exercises the "#"-value paths: a hex colour reached through
// the identifier scan (`#abcdef`), an ID token that is not a valid colour
// (`#axc`), and an ID token that embeds interpolation (`#x#{y}`).
func TestHashIDToken(t *testing.T) {
	cases := []struct{ src, want string }{
		{"a{b: #abcdef}", "a {\n  b: #abcdef;\n}\n"},
		{"a{b: #ABCDEF}", "a {\n  b: #ABCDEF;\n}\n"},
		{"a{b: #axc}", "a {\n  b: #axc;\n}\n"},
		{"a{b: #x#{y}}", "a {\n  b: #xy;\n}\n"},
	}
	for _, c := range cases {
		if got := compile(t, c.src); got != c.want {
			t.Errorf("for %q:\n want: %q\n  got: %q", c.src, c.want, got)
		}
	}
}

// TestSelectorEscapeCanonicalEOF drives escapeCanonical directly with a lone
// backslash at end-of-input, the defensive arm reached when a selector or
// media-query identifier ends in an incomplete escape.
func TestSelectorEscapeCanonicalEOF(t *testing.T) {
	if got := newSelParser("\\", true, false).escapeCanonical(false); got == "" {
		t.Error("escapeCanonical at EOF (body) should return a replacement escape")
	}
	if got := newSelParser("\\", true, false).escapeCanonical(true); got == "" {
		t.Error("escapeCanonical at EOF (start) should return a replacement escape")
	}
}
