// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import "testing"

// TestEscapedElse covers the `@else` clause recognised through the identifier
// scanner so a CSS-escaped spelling (`@\65lse`, `@\65lseif`) and the deprecated
// one-word `@elseif` are all honoured, while an unrelated at-rule following the
// `@if` (e.g. `@media`, `@-webkit-keyframes`) is left untouched. Each expected
// output is byte-verified against dart-sass 1.102.
func TestEscapedElse(t *testing.T) {
	cases := []struct{ in, want string }{
		// Escaped `@else` after an empty `@if` (sass/dart-sass#1011).
		{"@if false {}\n@\\65lse {a {b: c}}\n", "a {\n  b: c;\n}\n"},
		// Escaped one-word `@\65lseif` behaves as `@else if`.
		{"@if false {}\n@\\65lseif true {a{b:c}}\n", "a {\n  b: c;\n}\n"},
		// Deprecated plain `@elseif` behaves as `@else if`.
		{"@if false {}\n@elseif true {a{b:c}}\n", "a {\n  b: c;\n}\n"},
		// A comment between `@else` and `if` is skipped.
		{"@if false {}\n@else /* c */ if true {a{b:c}}\n", "a {\n  b: c;\n}\n"},
		// A bare escaped `@else if` chain falls through to the else block.
		{"@if false {}\n@else if false {}\n@else {a{b:c}}\n", "a {\n  b: c;\n}\n"},
		// An unrelated `@media` after the `@if` is not consumed as an else clause
		// (the identifier scanner reads "media", the loop resets and stops).
		{"@if true {a{b:c}}\n@media x {y{z:1}}\n",
			"a {\n  b: c;\n}\n\n@media x {\n  y {\n    z: 1;\n  }\n}\n"},
		// A following `@-webkit-keyframes` starts with `-`, which is not an
		// identifier start, so the pre-scan guard rejects it without failing.
		{"@if true {a{b:c}}\n@-webkit-keyframes x {from{a:b}}\n",
			"a {\n  b: c;\n}\n\n@-webkit-keyframes x {\n  from {\n    a: b;\n  }\n}\n"},
		// A `@if` at end of input has no following `@`, so the else loop stops on
		// the first (non-`@`) check.
		{"@if true {a{b:c}}\n", "a {\n  b: c;\n}\n"},
	}
	for _, c := range cases {
		res, err := Render(c.in, false, false, nil)
		if err != nil {
			t.Errorf("%q: unexpected error: %v", c.in, err)
			continue
		}
		if res.CSS != c.want {
			t.Errorf("escaped else %q:\n got %q\nwant %q", c.in, res.CSS, c.want)
		}
	}
}
