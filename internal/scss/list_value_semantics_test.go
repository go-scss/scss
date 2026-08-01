// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import "testing"

// TestListEqualsStrict verifies that a list is never equal to a scalar — not
// even a one-element list to that element — while an empty list equals an empty
// map, matching dart-sass's SassList equality.
func TestListEqualsStrict(t *testing.T) {
	one := &List{Elements: []Value{newNumber(1, "")}, Sep: SepComma}
	if one.equals(newNumber(1, "")) {
		t.Error("(1,) must not equal the number 1")
	}
	empty := &List{}
	if !empty.equals(&Map{}) {
		t.Error("() must equal the empty map")
	}
	if empty.equals(&Map{Keys: []Value{newNumber(1, "")}, Values: []Value{newNumber(2, "")}}) {
		t.Error("() must not equal a non-empty map")
	}
	if one.equals(&Map{}) {
		t.Error("(1,) must not equal the empty map")
	}
	// Whole-program check: `&` (a one-element list) compared to the string form
	// of its selector is false (dart), so the @else branch is taken.
	got := compile(t, "@mixin m($s) {\n  @if (& == $s) { a: yes; } @else { a: no; }\n}\n.bee { @include m(\".bee\"); }\n")
	if got != ".bee {\n  a: no;\n}\n" {
		t.Errorf("& == \".bee\" should be false: got %q", got)
	}
}

// TestListBlankElementsDropped verifies that dart-sass omits "blank" elements
// (sassNull and blank unbracketed lists such as the empty list `()`) from CSS
// list output, while a bracketed empty list `[]` is kept. Byte-exact against
// dart-sass 1.102.
func TestListBlankElementsDropped(t *testing.T) {
	cases := []struct{ in, out string }{
		// The empty list `()` is blank and dropped, so no double space appears.
		{"div { content: 1 2 () 3; }\n", "div {\n  content: 1 2 3;\n}\n"},
		// A bracketed empty list is NOT blank and is preserved.
		{"div { content: 1 [] 2; }\n", "div {\n  content: 1 [] 2;\n}\n"},
		// A plain list keeps all its (non-blank) elements.
		{"div { content: 1 2 3; }\n", "div {\n  content: 1 2 3;\n}\n"},
	}
	for _, c := range cases {
		if got := compile(t, c.in); got != c.out {
			t.Errorf("for %q:\n want: %q\n  got: %q", c.in, c.out, got)
		}
	}
}
