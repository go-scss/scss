// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import "testing"

// TestCallableScopeChain locks down dart-sass lexical scoping of @function and
// @mixin definitions. A callable declared inside a style rule, another function,
// or a mixin is visible only within that scope and shadows an outer definition
// of the same name for the lifetime of the scope; once the scope closes the
// outer definition is restored and the nested one is no longer resolvable. This
// mirrors dart-sass storing callables in its Environment scope chain rather than
// a single flat table. All expectations are byte-exact against dart-sass 1.102.
func TestCallableScopeChain(t *testing.T) {
	cases := []struct{ in, out string }{
		// A @mixin redefined inside a rule shadows the global one only there; a
		// later rule sees the original global definition again (sass-spec
		// non_conformant/scss-tests/132_test_nested_mixin_shadow).
		{
			"@mixin bar {a: b}\n\nfoo {\n  @mixin bar {c: d}\n  @include bar;\n}\n\nbaz {@include bar}\n",
			"foo {\n  c: d;\n}\n\nbaz {\n  a: b;\n}\n",
		},
		// A @function defined inside a rule is not visible outside it: the outer
		// call stays an unresolved plain-CSS function (133_test_nested_function_def).
		{
			"foo {\n  @function foo() {@return 1}\n  a: foo(); }\n\nbar {b: foo()}\n",
			"foo {\n  a: 1;\n}\n\nbar {\n  b: foo();\n}\n",
		},
		// A nested @function shadows a global one of the same name only inside the
		// rule; a later rule resolves the global (134_test_nested_function_shadow).
		{
			"@function foo() {@return 1}\n\nfoo {\n  @function foo() {@return 2}\n  a: foo();\n}\n\nbaz {b: foo()}\n",
			"foo {\n  a: 2;\n}\n\nbaz {\n  b: 1;\n}\n",
		},
		// A callable declared inside another rule's descendant is visible to still
		// deeper descendants but not to siblings of the declaring rule.
		{
			"a {\n  @function f() {@return 1}\n  b { x: f(); }\n}\nc { y: f(); }\n",
			"a b {\n  x: 1;\n}\n\nc {\n  y: f();\n}\n",
		},
		// function-exists / mixin-exists honour the scope chain: true inside the
		// declaring scope, false once it has closed.
		{
			"@use \"sass:meta\";\na {\n  @function nf() {@return 1}\n  @mixin nm {q: 1}\n  in-fn: meta.function-exists(nf);\n  in-mx: meta.mixin-exists(nm);\n}\nb {\n  out-fn: meta.function-exists(nf);\n  out-mx: meta.mixin-exists(nm);\n}\n",
			"a {\n  in-fn: true;\n  in-mx: true;\n}\n\nb {\n  out-fn: false;\n  out-mx: false;\n}\n",
		},
	}
	for _, c := range cases {
		if got := compile(t, c.in); got != c.out {
			t.Errorf("for %q:\n want: %q\n  got: %q", c.in, c.out, got)
		}
	}
}
