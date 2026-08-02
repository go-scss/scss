// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import "testing"

// TestInterpListNewline locks down how a newline inside a string is serialized
// through interpolation, matching dart-sass 1.102. dart writes a directly
// interpolated SassString's text verbatim (a newline survives to be re-escaped
// as `\a` by an enclosing quoted string), but runs a string reached *through* a
// list or map element through its full serializer, which collapses the newline
// to a space and swallows the following whitespace. The distinction is the
// element boundary, not whether the enclosing interpolation is quoted.
func TestInterpListNewline(t *testing.T) {
	cases := []struct{ in, out string }{
		// A string element inside a list interpolated into a quoted string:
		// newline collapses to a space.
		{
			"$s: \"x\\ay\";\nr { v: \"#{a $s}\"; }\n",
			"r {\n  v: \"a x y\";\n}\n",
		},
		// A single-element list is still a list: the element collapses.
		{
			"$s: \"x\\ay\";\nr { v: \"#{($s,)}\"; }\n",
			"r {\n  v: \"x y\";\n}\n",
		},
		// A string interpolated DIRECTLY (not through a list) keeps its raw
		// newline, which the enclosing quoted string then re-escapes as `\a`.
		{
			"$s: \"x\\ay\";\nr { v: \"#{$s}\"; }\n",
			"r {\n  v: \"x\\ay\";\n}\n",
		},
		// The same list in an UNQUOTED interpolation also collapses the newline
		// (go already did this; the fix must not disturb it).
		{
			"$s: \"x\\ay\";\nr { v: #{a $s}; }\n",
			"r {\n  v: a x y;\n}\n",
		},
		// dart swallows the whitespace that follows a collapsed newline inside a
		// list element.
		{
			"$s: \"x\\a  y\";\nr { v: \"#{($s, z)}\"; }\n",
			"r {\n  v: \"x y, z\";\n}\n",
		},
		// The libsass-closed issue_1786 case itself: a NUL becomes U+FFFD and each
		// newline in the list-element string collapses to a space (the @charset
		// header appears because the output carries a non-ASCII byte).
		{
			"$input: \"\\0_\\a_\\A\";\ntest { bug2: \"#{a $input}\"; }\n",
			"@charset \"UTF-8\";\ntest {\n  bug2: \"a \ufffd_ _ \";\n}\n",
		},
	}
	for _, c := range cases {
		if got := compile(t, c.in); got != c.out {
			t.Errorf("for %q:\n want: %q\n  got: %q", c.in, c.out, got)
		}
	}
}
