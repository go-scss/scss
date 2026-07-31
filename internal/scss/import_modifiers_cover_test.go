// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import (
	"strings"
	"testing"
)

// TestImportModifiers exercises the plain-CSS @import modifier grammar
// (parser_atrule.go tryImportModifiers / parseImportSupportsQuery and
// eval_supports.go resolveImportMods / normalizeImportMedia). Every expected
// output is byte-for-byte what dart-sass 1.102 produces for the same input, so
// these cases pin the go-scss serialization to dart parity across the whole
// modifier grammar: media-query lists (with normalization), @supports
// declarations / conditions / functions / negations, unknown modifier
// functions and identifiers, and interpolation.
func TestImportModifiers(t *testing.T) {
	cases := []struct{ src, want string }{
		{`@import "a.css";`, "@import \"a.css\";\n"},
		{`@import url("a.css") print;`, "@import url(\"a.css\") print;\n"},
		{`@import "a.css" supports(a: b);`, "@import \"a.css\" supports(a: b);\n"},
		{`@import "a.css" supports(--a: b);`, "@import \"a.css\" supports(--a: b);\n"},
		{`@import "a.css" supports(--a: );`, "@import \"a.css\" supports(--a: );\n"},
		{`@import "a.css" supports((a: b));`, "@import \"a.css\" supports(a: b);\n"},
		{`@import "a.css" supports((a: b) and (c: d));`, "@import \"a.css\" supports((a: b) and (c: d));\n"},
		{`@import "a.css" supports(not (a: b));`, "@import \"a.css\" supports(not (a: b));\n"},
		{`@import "a.css" supports(a(b));`, "@import \"a.css\" supports(a(b));\n"},
		{`@import "a.css" supports(calc(1));`, "@import \"a.css\" supports(calc(1));\n"},
		{`@import "a" b;`, "@import \"a\" b;\n"},
		{`@import "a" b();`, "@import \"a\" b();\n"},
		{`@import "a" b(c);`, "@import \"a\" b(c);\n"},
		{`@import "a" b c d(e) supports(f: g) h i j(k) l m (n: o), (p: q);`,
			"@import \"a\" b c d(e) supports(f: g) h i j(k) l m (n: o), (p: q);\n"},
		{`@import "a" b, (c: d) and (e: f), g;`, "@import \"a\" b, (c: d) and (e: f), g;\n"},
		{`@import "a" (b: c), (d: e) and (f: g), h;`, "@import \"a\" (b: c), (d: e) and (f: g), h;\n"},
		{`@import "a" b and(c: d), e;`, "@import \"a\" b and (c: d), e;\n"},
		{`@import "a" supports(b: c) (d: e);`, "@import \"a\" supports(b: c) (d: e);\n"},
		{`@import "a" supports(b: c), "d.css";`, "@import \"a\" supports(b: c);\n@import \"d.css\";\n"},
		{`@import "a" b(c), "e.css";`, "@import \"a\" b(c);\n@import \"e.css\";\n"},
		{`$x: "z"; @import "a" c(#{$x});`, "@import \"a\" c(z);\n"},
		{`$x: "z"; @import "a" c#{$x}d;`, "@import \"a\" czd;\n"},
		{`$x: "z"; @import "a" #{$x};`, "@import \"a\" z;\n"},
		// Unknown modifier function arguments preserve their newlines verbatim.
		{"@import \"a.css\" b(\n  c);", "@import \"a.css\" b(\n  c);\n"},
		{"@import \"a.css\" b(c\n  );", "@import \"a.css\" b(c\n  );\n"},
	}
	for _, c := range cases {
		if got := compile(t, c.src); got != c.want {
			t.Errorf("for %q:\n want: %q\n  got: %q", c.src, c.want, got)
		}
	}
}

// TestImportModifiersIndented covers the @supports function/condition modifiers
// in indented (.sass) syntax, where a supports function value collapses its
// internal whitespace to a single space (dart-sass re-serializes it).
func TestImportModifiersIndented(t *testing.T) {
	cases := []struct{ src, want string }{
		{"@import \"a.css\" supports(a(\n  b))", "@import \"a.css\" supports(a( b));\n"},
		{"@import \"a.css\" supports(a(b\n  ))", "@import \"a.css\" supports(a(b ));\n"},
		{"@import \"a.css\" supports(\n  a: b)", "@import \"a.css\" supports(a: b);\n"},
		{"@import \"a.css\" supports((a: b) \n  and (c: d))", "@import \"a.css\" supports((a: b) and (c: d));\n"},
		{"@import \"a.css\" supports(\n  not (a: b))", "@import \"a.css\" supports(not (a: b));\n"},
	}
	for _, c := range cases {
		res, err := Render(c.src, true, false, nil)
		if err != nil {
			t.Fatalf("compile error for %q: %v", c.src, err)
		}
		if res.CSS != c.want {
			t.Errorf("for %q:\n want: %q\n  got: %q", c.src, c.want, res.CSS)
		}
	}
}

// TestImportModifiersErrors covers the parser error branches of the @import
// modifier grammar: a supports declaration missing its colon, and an unknown /
// supports modifier function missing its closing parenthesis.
func TestImportModifiersErrors(t *testing.T) {
	for _, src := range []string{
		`@import "a" supports(b c);`,
		`@import "a" supports(a(b);`,
		`@import "a" supports(a(b`,
		`@import "a" b(c;`,
	} {
		if _, err := Render(src, false, false, nil); err == nil {
			t.Errorf("expected error for %q, got none", src)
		}
	}
}

// TestImportMediaFallback covers normalizeImportMedia's best-effort fallback:
// when an interpolated media modifier resolves to text that is not a valid
// media-query list, its resolved text is emitted trimmed rather than erroring.
func TestImportMediaFallback(t *testing.T) {
	const src = `$x: "!bogus"; @import "a" b, #{$x};`
	got := compile(t, src)
	if !strings.Contains(got, "!bogus") {
		t.Errorf("fallback media text missing: %q", got)
	}
}
