// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import "testing"

// TestIndentedMultiLineComments covers the indented preprocessor's handling of
// loud comments that span physical lines (compile.go inOpenLoudComment): a
// `/* ... */` opened on one line is folded into the same logical statement
// until it closes, so a value-embedded multi-line comment is stripped as
// whitespace exactly as dart-sass 1.102 does. It also exercises the scanner's
// string and `//` guards, which must not treat a `/*` inside a string or after
// a silent comment as opening a loud comment.
func TestIndentedMultiLineComments(t *testing.T) {
	cases := []struct{ src, want string }{
		// Loud comment spanning lines inside a declaration value -> stripped.
		{"a\n  b: c /* \n    d */ e\n", "a {\n  b: c e;\n}\n"},
		{"a\n  b: c /* d\n          e */ f\n", "a {\n  b: c f;\n}\n"},
		// Single-line loud comment closes within the same line (no fold).
		{"a\n  b: c /* d */ e\n", "a {\n  b: c e;\n}\n"},
		// A `/*` inside a string is not a comment (quote/escape guards).
		{"a\n  b: \"/* x\"\n", "a {\n  b: \"/* x\";\n}\n"},
		{"a\n  b: \"x\\\"y\"\n", "a {\n  b: 'x\"y';\n}\n"},
		// A `//` silent comment hides a following `/*` on the same line.
		{"a\n  b: c\n  d: 1 // note\n", "a {\n  b: c;\n  d: 1;\n}\n"},
	}
	for _, c := range cases {
		res, err := Render(c.src, true, false, nil)
		if err != nil {
			t.Fatalf("compile error for %q: %v", c.src, err)
		}
		if res.CSS != c.want {
			t.Errorf("for %q:\n want: %q\n  got: %q", c.src, c.want, res.CSS)
		}
	}
}

// TestInOpenLoudComment directly pins the loud-comment open-state scanner across
// every guarded construct: a comment that opens and closes on the line, one left
// open, a `/*` neutralized inside a single- or double-quoted string (with an
// escaped quote), and a `/*` neutralized after a `//` silent comment. These are
// exercised directly because a trailing `//` in an indented value is a separate
// unsupported construct that cannot reach the scanner through a passing compile.
func TestInOpenLoudComment(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"b: c", false},
		{"b: c /* d */ e", false},
		{"b: c /* d", true},
		{"b: c /* d * e", true},
		{`b: "/* x"`, false},
		{`b: '/* x'`, false},
		{`b: "a\"b" /* c`, true},
		{"b: c // note /* d", false},
	}
	for _, c := range cases {
		if got := inOpenLoudComment(c.in); got != c.want {
			t.Errorf("inOpenLoudComment(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
