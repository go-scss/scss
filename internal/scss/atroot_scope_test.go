// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import (
	"strings"
	"testing"
)

// TestAtRootVariableScoping locks down dart-sass @at-root variable scoping: an
// @at-root body is a nested, opaque scope (dart's Environment.scope without
// semiGlobal). An implicit (non-!global) assignment to a variable that exists
// only at the global scope creates a scope-local shadow rather than mutating the
// global; a variable/mixin/function declared inside the body is local to it and
// does not survive the body. All expectations are byte-exact against dart-sass
// 1.102.
func TestAtRootVariableScoping(t *testing.T) {
	cases := []struct{ in, out string }{
		// Implicit assignment to a global-only variable inside @at-root does NOT
		// mutate the global — a scope-local shadow is created and discarded.
		{
			"$g: outer;\n@at-root {\n  $g: inner;\n  $loc: made;\n}\na { g: $g; }\n",
			"a {\n  g: outer;\n}\n",
		},
		// An explicit !global assignment inside @at-root DOES mutate the global.
		{
			"$g: outer;\n@at-root {\n  $g: inner !global;\n}\na { g: $g; }\n",
			"a {\n  g: inner;\n}\n",
		},
		// A variable declared inside @at-root is visible within the body but gone
		// once it closes.
		{
			"@at-root {\n  $loc: made;\n  a { x: $loc; }\n}\nb { @if variable-exists(loc) { present: 1; } @else { present: 0; } }\n",
			"a {\n  x: made;\n}\n\nb {\n  present: 0;\n}\n",
		},
		// A nested @at-root updates the binding in the enclosing @at-root scope,
		// not the global; the global stays untouched throughout.
		{
			"$g: root;\n@at-root {\n  $g: middle;\n  @at-root {\n    $g: deep;\n  }\n}\na { g: $g; }\n",
			"a {\n  g: root;\n}\n",
		},
	}
	for _, c := range cases {
		if got := compile(t, c.in); got != c.out {
			t.Errorf("for %q:\n want: %q\n  got: %q", c.in, c.out, got)
		}
	}
}

// TestAtRootMixinScoping confirms a mixin declared inside @at-root is local to
// the body: dart-sass reports an undefined mixin when it is referenced outside.
func TestAtRootMixinScoping(t *testing.T) {
	_, err := Render("@at-root {\n  @mixin m { x: 1; }\n}\na { @include m; }\n", false, false, nil)
	if err == nil {
		t.Fatal("want undefined-mixin error for a mixin declared inside @at-root and used after it")
	}
	if !strings.Contains(err.Error(), "Undefined mixin") {
		t.Fatalf("want Undefined mixin error, got %v", err)
	}
}
