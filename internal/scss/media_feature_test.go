// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import "testing"

// TestMediaFeatureColonSpacing verifies dart-sass's canonical media-feature
// serialization: exactly one space follows the colon between a feature name and
// its value, regardless of the authored spacing. Range and boolean features
// (no colon) pass through unchanged. Byte-exact against dart-sass 1.102.
func TestMediaFeatureColonSpacing(t *testing.T) {
	cases := []struct{ in, out string }{
		{
			"@media screen and (orientation:landscape) {\n  a { x: 1; }\n}\n",
			"@media screen and (orientation: landscape) {\n  a {\n    x: 1;\n  }\n}\n",
		},
		{
			"@media (min-width:100px) and (max-width:200px) {\n  b { y: 2; }\n}\n",
			"@media (min-width: 100px) and (max-width: 200px) {\n  b {\n    y: 2;\n  }\n}\n",
		},
		// A boolean feature (no colon) is untouched.
		{
			"@media (color) {\n  c { z: 3; }\n}\n",
			"@media (color) {\n  c {\n    z: 3;\n  }\n}\n",
		},
	}
	for _, c := range cases {
		if got := compile(t, c.in); got != c.out {
			t.Errorf("for %q:\n want: %q\n  got: %q", c.in, c.out, got)
		}
	}
}
