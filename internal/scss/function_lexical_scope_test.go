// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import "testing"

// TestFunctionLexicalScope locks down that a user @function runs in its lexical
// (definition) scope, not the dynamic caller's. A variable local to the caller
// is invisible inside the callee, and a variable local to one function is not
// visible to another function it calls — dart-sass runs a callable in its
// captured closure. Expectations are byte-exact against dart-sass 1.102.
func TestFunctionLexicalScope(t *testing.T) {
	cases := []struct{ in, out string }{
		// A caller-local $x is invisible to a global function's own
		// variable-exists, though it is visible to variable-exists at the call site.
		{
			"@use \"sass:meta\";\n@function ex($n) { @return meta.variable-exists($n); }\n.a { $x: 1; foo: ex(x); bar: meta.variable-exists(x); }\n",
			".a {\n  foo: false;\n  bar: true;\n}\n",
		},
		// A local declared in f() is not visible to g() that f() calls.
		{
			"@use \"sass:meta\";\n@function f() { $foo: hi; @return g(); }\n@function g() { @return meta.variable-exists(foo); }\n.a { v: f(); }\n",
			".a {\n  v: false;\n}\n",
		},
		// A function still sees a global declared after it (the global scope is the
		// live map, shared into every closure).
		{
			"@function f() { @return $g; }\n$g: 7;\n.a { v: f(); }\n",
			".a {\n  v: 7;\n}\n",
		},
	}
	for _, c := range cases {
		if got := compile(t, c.in); got != c.out {
			t.Errorf("for %q:\n want: %q\n  got: %q", c.in, c.out, got)
		}
	}
}
