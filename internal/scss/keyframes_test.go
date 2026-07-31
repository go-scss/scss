// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import (
	"strings"
	"testing"
)

// TestKeyframes covers @keyframes (and vendor-prefixed) parsing/serialization:
// keyframe selectors (from/to/percentages, incl. scientific notation), selector
// lists, interpolation, stray declarations, nested at-rules and bubbling out of
// a style rule. Byte-exact against dart-sass 1.102.
func TestKeyframes(t *testing.T) {
	cases := []struct{ in, out string }{
		// from/to + percentage list; scientific-notation exponent lower-cased.
		{
			"@keyframes a {\n  from, 15%, to { c: d; }\n  13E+1% { e: f; }\n}\n",
			"@keyframes a {\n  from, 15%, to {\n    c: d;\n  }\n  13e+1% {\n    e: f;\n  }\n}\n",
		},
		// Vendor prefix, integer and double percentages.
		{
			"@-webkit-keyframes x {\n  0% { o: 0; }\n  10.3% { o: 0.5; }\n  100% { o: 1; }\n}\n",
			"@-webkit-keyframes x {\n  0% {\n    o: 0;\n  }\n  10.3% {\n    o: 0.5;\n  }\n  100% {\n    o: 1;\n  }\n}\n",
		},
		// Interpolated keyframe name and interpolated keyframe selector.
		{
			"$n: a;\n$b: 10%;\n@keyframes #{$n} {\n  #{$b} { c: d; }\n}\n",
			"@keyframes a {\n  10% {\n    c: d;\n  }\n}\n",
		},
		// A variable-like name is kept literal (not evaluated).
		{
			"$a: b;\n@keyframes $a {\n  to { c: d; }\n}\n",
			"@keyframes $a {\n  to {\n    c: d;\n  }\n}\n",
		},
		// Unknown at-rule inside a keyframe block.
		{
			"@keyframes a {\n  to {@b}\n}\n",
			"@keyframes a {\n  to {\n    @b;\n  }\n}\n",
		},
		// Known at-rule (@media) stays nested inside the keyframe block.
		{
			"@keyframes a {\n  to {@media screen {b: c}}\n}\n",
			"@keyframes a {\n  to {\n    @media screen {\n      b: c;\n    }\n  }\n}\n",
		},
		// A stray declaration in the @keyframes body is emitted verbatim.
		{
			"@keyframes a {\n  blah: blee;\n}\n",
			"@keyframes a {\n  blah: blee;\n}\n",
		},
		// @keyframes bubbles out of an enclosing style rule.
		{
			"a {\n  b: c;\n  @keyframes d {\n    to { e: f; }\n  }\n}\n",
			"a {\n  b: c;\n}\n@keyframes d {\n  to {\n    e: f;\n  }\n}\n",
		},
	}
	for _, c := range cases {
		if got := compile(t, c.in); got != c.out {
			t.Errorf("for %q:\n want: %q\n  got: %q", c.in, c.out, got)
		}
	}
}

// TestKeyframesStyleRuleError verifies that a nested style rule inside a keyframe
// block is rejected exactly as dart-sass rejects it.
func TestKeyframesStyleRuleError(t *testing.T) {
	_, err := Render("@keyframes a {\n  to { to { c: d; } }\n}\n", false, false, nil)
	if err == nil || !strings.Contains(err.Error(), "Style rules may not be used within keyframe blocks.") {
		t.Fatalf("expected keyframe-block style-rule error, got %v", err)
	}
}
