// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import (
	"strings"
	"testing"
)

// TestQuotedInterpolationInSelector verifies that #{…} interpolation inside a
// quoted string within interpolated text (an attribute-selector value) is
// resolved, matching dart-sass, while quotes and escapes are preserved.
func TestQuotedInterpolationInSelector(t *testing.T) {
	cases := []struct{ in, out string }{
		// The parent reference resolves inside the quoted attribute value.
		{
			".foo {\n  [baz=\"#{&}\"] { x: y; }\n}\n",
			".foo [baz=\".foo\"] {\n  x: y;\n}\n",
		},
		// An arithmetic interpolation resolves inside the quoted value.
		{
			".a[data-x=\"#{1 + 1}\"] { color: red; }\n",
			".a[data-x=\"2\"] {\n  color: red;\n}\n",
		},
	}
	for _, c := range cases {
		if got := compile(t, c.in); got != c.out {
			t.Errorf("for %q:\n want: %q\n  got: %q", c.in, c.out, got)
		}
	}
}

// TestQuotedInterpValueContexts exercises quotedStringToInterp in a custom-
// property value: an escape is preserved verbatim and the #{…} interpolation
// inside the quoted string is resolved.
func TestQuotedInterpValueContexts(t *testing.T) {
	got := compile(t, ".a { --x: \"p\\\"#{1 + 1}q\"; }\n")
	if !strings.Contains(got, "\"p\\\"2q\"") {
		t.Errorf("quotedStringToInterp interp/escape: got %q", got)
	}
}
