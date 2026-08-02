// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import "testing"

// TestSpecialFunctionEscapedDelimiter covers CSS escapes inside a special
// function's raw argument text (sass-spec libsass-todo-issues/issue_1694/
// quoted-right-dbl-paren). An escaped delimiter such as `\)` must be kept
// literally rather than closing the balanced parenthesis, so
// `progid:...Alpha(opacity=20\))` round-trips unchanged. Each expected output is
// byte-verified against dart-sass 1.102.
func TestSpecialFunctionEscapedDelimiter(t *testing.T) {
	cases := []struct{ in, want string }{
		// The issue_1694 fixture: an escaped `\)` inside an IE progid filter.
		{"test {\n  filter: progid:DXImageTransform.Microsoft.Alpha(opacity=20\\));\n}\n",
			"test {\n  filter: progid:DXImageTransform.Microsoft.Alpha(opacity=20\\));\n}\n"},
		// The same escape inside a legacy expression() special function.
		{"a { b: expression(foo\\)); }", "a {\n  b: expression(foo\\));\n}\n"},
		// An escaped quote is content, not a string delimiter.
		{"a { b: progid:X(y=\\\"z\\\"); }", "a {\n  b: progid:X(y=\\\"z\\\");\n}\n"},
	}
	for _, c := range cases {
		res, err := Render(c.in, false, false, nil)
		if err != nil {
			t.Errorf("special function %q: unexpected error: %v", c.in, err)
			continue
		}
		if res.CSS != c.want {
			t.Errorf("special function %q:\n got %q\nwant %q", c.in, res.CSS, c.want)
		}
	}
}

// TestSpecialFunctionTrailingBackslash covers the end-of-input guard in
// parseCalc: a backslash that ends the source while scanning a special
// function's raw arguments is consumed without reading a following byte.
func TestSpecialFunctionTrailingBackslash(t *testing.T) {
	if _, err := Render("a { b: progid:X(\\", false, false, nil); err == nil {
		t.Fatal("expected an error for an unterminated special function")
	}
}
