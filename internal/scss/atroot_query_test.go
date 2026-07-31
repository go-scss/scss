// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import (
	"strings"
	"testing"
)

// TestAtRootQuerySemantics exercises @at-root `(with: …)` / `(without: …)` query
// semantics: which enclosing frames the body escapes or retains. Every expected
// output is byte-exact against dart-sass 1.102.
func TestAtRootQuerySemantics(t *testing.T) {
	cases := []struct{ in, out string }{
		// `without: media` drops the @media frame but keeps the enclosing style
		// rule; a bare declaration lands in that rule at the document root.
		{
			"@media print {\n  a {\n    @at-root (without: media) {\n      b: c;\n    }\n  }\n}\n",
			"a {\n  b: c;\n}\n",
		},
		// `without: media` keeping the rule, with a nested selector: the parent
		// selector is re-materialised at the root and the child nests under it.
		{
			"@media x {.a{@at-root (without: media) {.b{y:1}}}}",
			".a .b {\n  y: 1;\n}\n",
		},
		// `with: media` keeps the @media frame and drops the style rule, so the
		// body escapes the rule but stays wrapped in the media.
		{
			"@media x {.a{@at-root (with: media) {.b{y:1}}}}",
			"@media x {\n  .b {\n    y: 1;\n  }\n}\n",
		},
		// `without: supports` drops a @supports frame while keeping the rule.
		{
			"@supports (a: b) {.a{@at-root (without: supports) {.b{y:1}}}}",
			".a .b {\n  y: 1;\n}\n",
		},
		// `without: all` escapes every frame, including a @keyframes; the now-empty
		// @keyframes is still emitted (dart keeps empty non-media/supports rules).
		{
			"@keyframes a {\n  @at-root (without: all) {\n    b {c: d}\n  }\n}\n",
			"@keyframes a {}\nb {\n  c: d;\n}\n",
		},
		// A silent comment inside the query is treated as whitespace: the body is
		// empty, so nothing is emitted.
		{
			"@at-root (//\n  without: media) {}",
			"",
		},
		{
			"@at-root (without //\n  : media) {}",
			"",
		},
		// A loud comment inside the query is likewise whitespace for parsing.
		{
			"@media x {.a{@at-root (with: /* keep */ media) {.b{y:1}}}}",
			"@media x {\n  .b {\n    y: 1;\n  }\n}\n",
		},
	}
	for _, c := range cases {
		if got := compile(t, c.in); got != c.out {
			t.Errorf("for %q:\n want: %q\n  got: %q", c.in, c.out, got)
		}
	}
}

// TestAtRootQueryMalformed checks that a query without a `with:`/`without:`
// keyword, or with an empty name list, is a compile error — matching dart, which
// raises `Expected "with" or "without".`
func TestAtRootQueryMalformed(t *testing.T) {
	for _, src := range []string{
		"@media x {.a{@at-root (foo) {.b{y:1}}}}",      // no keyword
		"@media x {.a{@at-root (without:) {.b{y:1}}}}", // empty name list
		"@media x {.a{@at-root (sideways: media) {}}}", // wrong keyword
		"@media x {.a{@at-root (/* c */) {.b{y:1}}}}",  // comment-only, no colon
	} {
		if _, err := Render(src, false, false, nil); err == nil {
			t.Errorf("expected error for %q", src)
		}
	}
}

// TestEmptyAtRuleEmission verifies dart's empty-at-rule rules: an empty
// @media/@supports is dropped, but an empty @keyframes, @font-face or unknown
// at-rule is still serialised as `@name {}` (expanded) or `@name{}`
// (compressed). Byte-exact against dart-sass 1.102.
func TestEmptyAtRuleEmission(t *testing.T) {
	cases := []struct{ in, out string }{
		{"@media screen {}", ""},
		{"@supports (a: b) {}", ""},
		{"@flooblehoof {}", "@flooblehoof {}\n"},
		{"@keyframes a {}", "@keyframes a {}\n"},
		{"@font-face {}", "@font-face {}\n"},
		// An empty unknown at-rule nested in @media keeps the media non-empty, so
		// the media is retained too.
		{
			"@media screen {@flooblehoof {}}",
			"@media screen {\n  @flooblehoof {}\n}\n",
		},
	}
	for _, c := range cases {
		if got := compile(t, c.in); got != c.out {
			t.Errorf("expanded %q:\n want: %q\n  got: %q", c.in, c.out, got)
		}
	}
	// Compressed empty at-rule collapses to `@name{}`.
	if got := compileC(t, "@keyframes a {}"); got != "@keyframes a{}\n" {
		t.Errorf("compressed keyframes: got %q", got)
	}
	if got := compileC(t, "@media screen {}"); got != "" {
		t.Errorf("compressed empty media: got %q", got)
	}
}

// TestExtendInBubblingFrame checks that @extend nested in a @media directly
// inside a style rule applies to the enclosing rule (re-materialised in the
// media context), rather than raising the rule-less error. Byte-exact vs dart.
func TestExtendInBubblingFrame(t *testing.T) {
	got := compile(t, ".foo {\n  @media screen {\n    @extend %bar;\n  }\n}\n\n@media screen {\n  %bar {\n    a: b;\n  }\n}\n")
	want := "@media screen {\n  .foo {\n    a: b;\n  }\n}\n"
	if got != want {
		t.Errorf("extend in media:\n want: %q\n  got: %q", want, got)
	}
	// A truly rule-less @extend (top level, or inside a top-level @media) is an
	// error.
	for _, src := range []string{
		"@extend .foo;",
		"@media screen { @extend .foo; }",
	} {
		if _, err := Render(src, false, false, nil); err == nil || !strings.Contains(err.Error(), "@extend may only be used within style rules") {
			t.Errorf("expected rule-less @extend error for %q, got %v", src, err)
		}
	}
}

// TestStripCSSComments exercises stripCSSComments directly, including the
// defensive unterminated-comment paths that the parser normally rejects before a
// query reaches the stripper.
func TestStripCSSComments(t *testing.T) {
	cases := []struct{ in, out string }{
		{"without: media", "without: media"},
		{"with: /* keep */ media", "with:   media"},
		{"a //x\n b", "a  \n b"}, // // terminated by newline (newline preserved)
		{"a /* b */ c", "a   c"},
		{"a /* unterminated", "a  "}, // no closing */: consume to end
		{"a // unterminated", "a  "}, // no newline: consume to end
	}
	for _, c := range cases {
		if got := stripCSSComments(c.in); got != c.out {
			t.Errorf("stripCSSComments(%q) = %q, want %q", c.in, got, c.out)
		}
	}
}

// TestGlobalSelectorFunctions covers the global (un-namespaced) selector
// function aliases dart still exposes. Byte-exact vs dart-sass 1.102.
func TestGlobalSelectorFunctions(t *testing.T) {
	cases := []struct{ in, out string }{
		{"a {b: selector-extend(c, c, d)}", "a {\n  b: c, d;\n}\n"},
		{"a {b: selector-replace(c, c, d)}", "a {\n  b: d;\n}\n"},
		{"a {b: is-superselector(a, a b)}", "a {\n  b: false;\n}\n"},
		{"a {b: simple-selectors('a.b')}", "a {\n  b: a, .b;\n}\n"},
		{"a {b: selector-parse('a b')}", "a {\n  b: a b;\n}\n"},
	}
	for _, c := range cases {
		if got := compile(t, c.in); got != c.out {
			t.Errorf("for %q:\n want: %q\n  got: %q", c.in, c.out, got)
		}
	}
}
