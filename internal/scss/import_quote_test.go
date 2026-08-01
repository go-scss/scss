// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import "testing"

// TestPlainImportQuotePreserved locks down that a plain-CSS passthrough @import
// round-trips its original quote style: dart-sass keeps `'foo.css'` single-quoted
// rather than renormalising it to double quotes, and preserves each URL in a
// comma list independently. Expectations are byte-exact against dart-sass 1.102.
func TestPlainImportQuotePreserved(t *testing.T) {
	cases := []struct{ in, out string }{
		{"@import 'a.css';\n", "@import 'a.css';\n"},
		{"@import \"b.css\";\n", "@import \"b.css\";\n"},
		{"@import 'c.css', \"d.css\";\n", "@import 'c.css';\n@import \"d.css\";\n"},
	}
	for _, c := range cases {
		if got := compile(t, c.in); got != c.out {
			t.Errorf("for %q:\n want: %q\n  got: %q", c.in, c.out, got)
		}
	}
}
