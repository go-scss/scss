// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import "testing"

// TestSupportsConditions exercises the structured @supports condition parser and
// serializer (parser_supports.go / eval_supports.go). Every expected output is
// byte-for-byte what dart-sass 1.102 produces for the same input, so these cases
// pin the go-scss serialization to dart parity across the full condition grammar:
// declarations (plain / dynamic / custom-property / calc-unsimplified),
// operations (and/or, associativity, parenthesization), negation, functions,
// SupportsAnything raw fallbacks, and interpolation.
func TestSupportsConditions(t *testing.T) {
	cases := []struct{ src, want string }{
		{"@supports (a: b) {x{y:z}}", "@supports (a: b) {\n  x {\n    y: z;\n  }\n}\n"},
		{"@supports (a: \"b\") {x{y:z}}", "@supports (a: \"b\") {\n  x {\n    y: z;\n  }\n}\n"},
		{"@supports (a: 1 + 1) {x{y:z}}", "@supports (a: 2) {\n  x {\n    y: z;\n  }\n}\n"},
		{"@supports (1 + 1: b) {x{y:z}}", "@supports (2: b) {\n  x {\n    y: z;\n  }\n}\n"},
		{"@supports ((((a: b)))) {x{y:z}}", "@supports (a: b) {\n  x {\n    y: z;\n  }\n}\n"},
		{"@supports (--a: b) {x{y:z}}", "@supports (--a: b) {\n  x {\n    y: z;\n  }\n}\n"},
		{"@supports (--a:/**/x) {x{y:z}}", "@supports (--a:/**/x) {\n  x {\n    y: z;\n  }\n}\n"},
		{"@supports (--a: \"q\") {x{y:z}}", "@supports (--a: \"q\") {\n  x {\n    y: z;\n  }\n}\n"},
		{"@supports (--a: b//x\n) {x{y:z}}", "@supports (--a: b ) {\n  x {\n    y: z;\n  }\n}\n"},
		{"@supports (--a b) {x{y:z}}", "@supports (--a b) {\n  x {\n    y: z;\n  }\n}\n"},
		{"@supports (a: calc(1 + 2)) {x{y:z}}", "@supports (a: calc(1 + 2)) {\n  x {\n    y: z;\n  }\n}\n"},
		{"@supports (a: calc(1 + calc(2 + 3))) {x{y:z}}", "@supports (a: calc(1 + calc(2 + 3))) {\n  x {\n    y: z;\n  }\n}\n"},
		{"@supports (a: min(1px, 2px)) {x{y:z}}", "@supports (a: min(1px, 2px)) {\n  x {\n    y: z;\n  }\n}\n"},
		{"@supports (calc(0): a) {x{y:z}}", "@supports (calc(0): a) {\n  x {\n    y: z;\n  }\n}\n"},
		{"@supports (a: #{calc(1 + 2)}) {x{y:z}}", "@supports (a: 3) {\n  x {\n    y: z;\n  }\n}\n"},
		{"@supports (a: calc(#{1 + 2})) {x{y:z}}", "@supports (a: calc(3)) {\n  x {\n    y: z;\n  }\n}\n"},
		{"@supports (a b) {x{y:z}}", "@supports (a b) {\n  x {\n    y: z;\n  }\n}\n"},
		{"@supports (a /**/ b) {x{y:z}}", "@supports (a /**/ b) {\n  x {\n    y: z;\n  }\n}\n"},
		{"@supports (a b xyz) {x{y:z}}", "@supports (a b xyz) {\n  x {\n    y: z;\n  }\n}\n"},
		{"@supports not (a: b) {x{y:z}}", "@supports not (a: b) {\n  x {\n    y: z;\n  }\n}\n"},
		{"@supports not ((a: b) and (c: d)) {x{y:z}}", "@supports not ((a: b) and (c: d)) {\n  x {\n    y: z;\n  }\n}\n"},
		{"@supports (a: b) and (c: d) {x{y:z}}", "@supports (a: b) and (c: d) {\n  x {\n    y: z;\n  }\n}\n"},
		{"@supports (a: b) or (c: d) {x{y:z}}", "@supports (a: b) or (c: d) {\n  x {\n    y: z;\n  }\n}\n"},
		{"@supports (a: b) and (c: d) and (e: f) {x{y:z}}", "@supports (a: b) and (c: d) and (e: f) {\n  x {\n    y: z;\n  }\n}\n"},
		{"@supports (a: b) and ((c: d) or (e: f)) {x{y:z}}", "@supports (a: b) and ((c: d) or (e: f)) {\n  x {\n    y: z;\n  }\n}\n"},
		{"@supports (a: b) and (not (c: d)) {x{y:z}}", "@supports (a: b) and (not (c: d)) {\n  x {\n    y: z;\n  }\n}\n"},
		{"@supports a(b) {x{y:z}}", "@supports a(b) {\n  x {\n    y: z;\n  }\n}\n"},
		{"@supports a() {x{y:z}}", "@supports a() {\n  x {\n    y: z;\n  }\n}\n"},
		{"@supports a( ) {x{y:z}}", "@supports a( ) {\n  x {\n    y: z;\n  }\n}\n"},
		{"@supports a(b(c)) {x{y:z}}", "@supports a(b(c)) {\n  x {\n    y: z;\n  }\n}\n"},
		{"@supports a([x]) {x{y:z}}", "@supports a([x]) {\n  x {\n    y: z;\n  }\n}\n"},
		{"@supports a(b;c) {x{y:z}}", "@supports a(b;c) {\n  x {\n    y: z;\n  }\n}\n"},
		{"@supports a(/**/ b) {x{y:z}}", "@supports a(/**/ b) {\n  x {\n    y: z;\n  }\n}\n"},
		{"@supports -webkit-x(y) {x{y:z}}", "@supports -webkit-x(y) {\n  x {\n    y: z;\n  }\n}\n"},
		{"@supports a#{\"b\"}c(d) {x{y:z}}", "@supports abc(d) {\n  x {\n    y: z;\n  }\n}\n"},
		{"@supports a(#{1 + 1}) {x{y:z}}", "@supports a(2) {\n  x {\n    y: z;\n  }\n}\n"},
		{"@supports #{\"(a: b)\"} {x{y:z}}", "@supports (a: b) {\n  x {\n    y: z;\n  }\n}\n"},
		{"@supports #{\"(a: b)\"} and (c: 1 + 1) {x{y:z}}", "@supports (a: b) and (c: 2) {\n  x {\n    y: z;\n  }\n}\n"},
		{"@supports (#{\"(a: b)\"}) {x{y:z}}", "@supports ((a: b)) {\n  x {\n    y: z;\n  }\n}\n"},
		{"@supports (#{\"(a: b)\"} and (c: 1 + 1)) {x{y:z}}", "@supports (a: b) and (c: 2) {\n  x {\n    y: z;\n  }\n}\n"},
		{"@supports (#{\"(a: b)\"} or (c: d)) {x{y:z}}", "@supports (a: b) or (c: d) {\n  x {\n    y: z;\n  }\n}\n"},
		{"@supports (#{\"a\"} xyz) {x{y:z}}", "@supports (a xyz) {\n  x {\n    y: z;\n  }\n}\n"},
		{"@supports -#{\"webkit\"}-x(y) {x{y:z}}", "@supports -webkit-x(y) {\n  x {\n    y: z;\n  }\n}\n"},
		{"@supports --x(y) {x{y:z}}", "@supports --x(y) {\n  x {\n    y: z;\n  }\n}\n"},
		{"@supports a({y}) {x{y:z}}", "@supports a({y}) {\n  x {\n    y: z;\n  }\n}\n"},
		{"@supports a(+) {x{y:z}}", "@supports a(+) {\n  x {\n    y: z;\n  }\n}\n"},
		{"@supports a(\"s\") {x{y:z}}", "@supports a(\"s\") {\n  x {\n    y: z;\n  }\n}\n"},
		{"@supports (a #{1 + 1}) {x{y:z}}", "@supports (a 2) {\n  x {\n    y: z;\n  }\n}\n"},
		{"@supports (--a: x  y) {x{y:z}}", "@supports (--a: x y) {\n  x {\n    y: z;\n  }\n}\n"},
		{"@supports (--a: x\n y) {x{y:z}}", "@supports (--a: x y) {\n  x {\n    y: z;\n  }\n}\n"},
		{"@supports a(x\n y) {p{q:r}}", "@supports a(x\n y) {\n  p {\n    q: r;\n  }\n}\n"},
		{"@supports (a  b) {p{q:r}}", "@supports (a b) {\n  p {\n    q: r;\n  }\n}\n"},
		{"@supports a(b:c) {p{q:r}}", "@supports a(b:c) {\n  p {\n    q: r;\n  }\n}\n"},
		{"@supports (#{\"(a: b)\"} and (c: d) and (e: f)) {p{q:r}}", "@supports (a: b) and (c: d) and (e: f) {\n  p {\n    q: r;\n  }\n}\n"},
	}
	for _, c := range cases {
		expectEq(t, c.src, c.want)
	}
}

// TestSupportsConditionErrors covers the parser's error and backtracking
// branches. dart-sass rejects each of these inputs too.
func TestSupportsConditionErrors(t *testing.T) {
	for _, src := range []string{
		`@supports (a: b) and not (c: d) {x{y:z}}`,              // "not" as an operand
		`@supports not not (a: b) {x{y:z}}`,                     // "not" after "not"
		`@supports abc {x{y:z}}`,                                // bare identifier, no "("
		`@supports 123 {x{y:z}}`,                                // not an identifier, not "("
		`@supports a(b {x{y:z}}`,                                // unclosed function args
		`@supports (not (a: b) {x{y:z}}`,                        // unclosed "not (...)"
		`@supports ((a: b) {x{y:z}}`,                            // unclosed nested "("
		`@supports (--a: b]) {x{y:z}}`,                          // custom-property bad close
		`@supports (a: b {x{y:z}}`,                              // declaration unclosed
		`@supports (a: b) and (c: d) or (e: f) {x{y:z}}`,        // mixed operators
		`@supports (#{"(a: b)"} and (c: d) or (e: f)) {x{y:z}}`, // mixed operators (interp)
		`@supports a(b]) {x{y:z}}`,                              // bracket mismatch
		`@supports (- : a) {x{y:z}}`,                            // lone "-" is not an identifier
		`@supports (* b) {x{y:z}}`,                              // "*" is not an identifier
		`@supports (a! : b) {x{y:z}}`,                           // raw value runs into a colon
		`@supports (--a: b;) {x{y:z}}`,                          // custom value stops at ";"
		`@supports a(/*x) {x{y:z}}`,                             // unterminated loud comment
		`@supports (a b]) {x{y:z}}`,                             // SupportsAnything bad close
		`@supports foo#{bar {x{y:z}}`,                           // unclosed interpolation in identifier
		`@supports a(#{bar) {x{y:z}}`,                           // unclosed interpolation in raw value
		`@supports a([b) {x{y:z}}`,                              // mismatched bracket in raw value
		`@supports (#{"a"} and (b: c);) {x{y:z}}`,               // interp operation not closed by ")"
	} {
		mustErr(t, src)
	}
}

// TestSupportsConditionEscape documents that go-scss preserves CSS escapes
// verbatim in raw condition text (a serialization concern independent of the
// @supports grammar). It also covers the escape branches of interpolatedIdentifier
// and lookingAtInterpolatedIdentifier.
func TestSupportsConditionEscape(t *testing.T) {
	expectEq(t, `@supports \61(b) {x{y:z}}`, "@supports \\61(b) {\n  x {\n    y: z;\n  }\n}\n")
	expectEq(t, `@supports a(\62) {x{y:z}}`, "@supports a(\\62) {\n  x {\n    y: z;\n  }\n}\n")
}
