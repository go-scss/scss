// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import "testing"

// TestInterpolatedFunctionName covers dart-sass's rule that an interpolated
// callee name (`#{$f}(a)`, `foo#{1}bar(a)`) is never resolved to a Sass
// function or built-in: it always passes through as a plain CSS function whose
// name is the evaluated interpolation, with its arguments evaluated. Every
// expected output below is byte-verified against dart-sass 1.102.
func TestInterpolatedFunctionName(t *testing.T) {
	cases := []struct{ in, out string }{
		// A bare `#{…}(…)` names a plain CSS function.
		{
			"$f: foo;\ndiv { color: #{$f}(a, 1+2, c); }\n",
			"div {\n  color: foo(a, 3, c);\n}\n",
		},
		// Interpolation embedded mid-identifier, at the start, and at the end.
		{
			".x {\n  start: #{1 + 1}foo(arg);\n  mid: foo#{1 + 1}bar(arg);\n  end: foo#{1 + 1}(arg);\n  full: #{foo}(arg);\n}\n",
			".x {\n  start: 2foo(arg);\n  mid: foo2bar(arg);\n  end: foo2(arg);\n  full: foo(arg);\n}\n",
		},
		// An interpolated name that spells a built-in (`unquote`) or user
		// function (`identity`) is NOT called — it stays a plain CSS function.
		{
			"@function identity($arg) {@return $arg}\ndiv {\n  a: un#{quo}te(\"hello\");\n  b: id#{enti}ty(arg);\n}\n",
			"div {\n  a: unquote(\"hello\");\n  b: identity(arg);\n}\n",
		},
		// Arguments are still fully evaluated, and a spread expands in place.
		{
			"div {\n  $list: 1, 2, 3;\n  a: foo#{1 + 1}bar(2 + 2);\n  b: foo#{1 + 1}bar($list...);\n}\n",
			"div {\n  a: foo2bar(4);\n  b: foo2bar(1, 2, 3);\n}\n",
		},
	}
	for _, c := range cases {
		if got := compile(t, c.in); got != c.out {
			t.Errorf("for %q:\n want: %q\n  got: %q", c.in, c.out, got)
		}
	}
}

// TestSpaceListCloseParenAdjacency covers dart-sass separating a value that
// immediately follows a close paren/bracket into a fresh space-list element
// even with no intervening whitespace. Expected outputs are byte-verified
// against dart-sass 1.102.
func TestSpaceListCloseParenAdjacency(t *testing.T) {
	cases := []struct{ in, out string }{
		{"a { b: (1 + 2)px; }\n", "a {\n  b: 3 px;\n}\n"},
		{"a { b: (\"hello\")un#{quo}te; }\n", "a {\n  b: \"hello\" unquote;\n}\n"},
		{"a { b: [1 2]foo; }\n", "a {\n  b: [1 2] foo;\n}\n"},
		{"a { b: url(x)no-repeat; }\n", "a {\n  b: url(x) no-repeat;\n}\n"},
	}
	for _, c := range cases {
		if got := compile(t, c.in); got != c.out {
			t.Errorf("for %q:\n want: %q\n  got: %q", c.in, c.out, got)
		}
	}
}
