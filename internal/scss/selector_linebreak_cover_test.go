// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import "testing"

// TestSelectorLineBreaks pins go-scss to dart-sass 1.102's line-break handling
// in comma-separated selector lists: a complex selector preceded by a newline in
// the source is re-emitted after `,\n` (with continuation lines indented to the
// rule's depth), the break propagates through nesting, and compressed output
// drops the break entirely. Every expected output is byte-for-byte dart-sass.
func TestSelectorLineBreaks(t *testing.T) {
	cases := []struct{ src, want string }{
		{"a,\nb {\n  c: d\n}\n", "a,\nb {\n  c: d;\n}\n"},
		{"a, b,\nc {x: y}\n", "a, b,\nc {\n  x: y;\n}\n"},
		{"foo,\nbar {\n  baz,\n  bang {a: b}}\n",
			"foo baz,\nfoo bang,\nbar baz,\nbar bang {\n  a: b;\n}\n"},
		{"@media x {\n  a,\n  b { c: d }\n}\n",
			"@media x {\n  a,\n  b {\n    c: d;\n  }\n}\n"},
	}
	for _, c := range cases {
		if got := compile(t, c.src); got != c.want {
			t.Errorf("for %q:\n want: %q\n  got: %q", c.src, c.want, got)
		}
	}
}

// TestSelectorLineBreaksCompressed confirms compressed output collapses a
// multi-line selector list to a comma with no space or newline.
func TestSelectorLineBreaksCompressed(t *testing.T) {
	const src = "a,\nb {x:y}\n"
	const want = "a,b{x:y}\n"
	if got := compileC(t, src); got != want {
		t.Errorf("for %q:\n want: %q\n  got: %q", src, want, got)
	}
}
