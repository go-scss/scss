// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import "testing"

// TestSlashProvenanceStripping covers where dart-sass clears a scalar slash
// number's "as-slash" provenance: on binding an @each variable, and on every
// binary operand other than "/". A list operand is unaffected, so its inner
// slashes survive. A grouping also resets the surrounding arithmetic context so
// a "/" inside it keeps its slash even as an operand. Outputs are byte-verified
// against dart-sass 1.102.
func TestSlashProvenanceStripping(t *testing.T) {
	cases := []struct{ in, out string }{
		// @each binding strips a scalar slash element but keeps a nested list's.
		{
			"a {\n  @each $x in a 3/4 b { b: $x; }\n}\n",
			"a {\n  b: a;\n  b: 0.75;\n  b: b;\n}\n",
		},
		// A whole list keeps its inner slashes when serialized directly.
		{"$x: a 3/4 b;\na { b: $x; }\n", "a {\n  b: a 3/4 b;\n}\n"},
		// A "/" inside a grouping keeps its slash even as an operand of "+".
		{"a { b: x + (5/6 7/8); }\n", "a {\n  b: x5/6 7/8;\n}\n"},
		{"a { b: (1/2 3/4) + (5/6 7/8); }\n", "a {\n  b: 1/2 3/45/6 7/8;\n}\n"},
		// A scalar slash operand of "+"/"-" loses its provenance.
		{"a { b: 3/4 + (4/5 6/7); }\n", "a {\n  b: 0.754/5 6/7;\n}\n"},
		// A bare division literal (no consuming operation) keeps its slash.
		{"a { b: 3/4; }\n", "a {\n  b: 3/4;\n}\n"},
	}
	for _, c := range cases {
		if got := compile(t, c.in); got != c.out {
			t.Errorf("for %q:\n want: %q\n  got: %q", c.in, c.out, got)
		}
	}
}
