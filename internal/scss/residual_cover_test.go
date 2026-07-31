// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import (
	"strings"
	"testing"
)

// TestResidualEvalBranches covers scattered evaluator/parser/color branches that
// the conformance corpus does not reach: named if() arguments, empty-parent &,
// interpolation-as-expression, unit alignment/simplification, media `or`, hex
// case handling and the integer-RGB serialization fallback.
func TestResidualEvalBranches(t *testing.T) {
	oks := []string{
		// if() with named arguments (condition/if-true/if-false arms).
		`.a{ v: if($condition: true, $if-true: 1, $if-false: 2) }`,
		`.a{ v: if($if-false: 2, $if-true: 1, $condition: false) }`,
		// & where the current parent is empty -> null.
		`$x: &; .a{ v: inspect($x) }`,
		// interpolation used directly as an expression / on the left of a binary.
		`.a{ v: #{1} }`,
		`.a{ v: #{1} + 2 }`,
		// short-circuit `and` returning the falsy left operand.
		`.a{ v: inspect(false and true) }`,
		// alignForAdd: unitful + unitless.
		`.a{ v: 1px + 2 }`,
		`.a{ v: 2 + 1px }`,
		// media `or` condition arm (needs nested-paren grouping to route
		// through mediaCondition).
		`@media ((min-width: 1px) or (max-width: 999px)) { a{b:c} }`,
		`@media (not (min-width: 1px)) { a{b:c} }`,
		// inspect of a quoted string (serializeQuoted arm).
		`@use "sass:meta"; .a{ v: meta.inspect("foo") }`,
		// space-separated list whose next element starts with `-` (canStartValue).
		`.a{ v: 1px -2px }`,
		// paren list with a trailing comma.
		`@use "sass:meta"; .a{ v: meta.inspect((1, 2,)) }`,
		// protocol-relative @import stays a plain CSS @import.
		`@import "//cdn.example.com/x";`,
		// @extend nested inside a property-declaration block has no selector to
		// extend from (defensive continue).
		`.b{x:y} .a{ font: 1px { @extend .b } }`,
		// uppercase hex color (hexPair default arm).
		`.a{ color: #ABCDEF }`,
		// nested property block containing a non-declaration statement.
		`.a{ font: 10px { @if true { weight: bold } } }`,
		// loud comment inside a style rule (fr.block append path).
		`.a{ /* hi */ color: red }`,
		// global-variable-exists on an existing global.
		`$g: 1; .a{ v: global-variable-exists(g) }`,
		// inspect of an unquoted string returns the raw text.
		`@use "sass:meta"; .a{ v: meta.inspect(foo) }`,
		// alpha() builtin.
		`.a{ v: alpha(rgba(1,2,3,.5)) }`,
		// integer-RGB serialization fallback: a non-integer blue channel.
		`.a{ color: rgb(1, 2, 3.5) }`,
	}
	for _, src := range oks {
		if _, err := Render(src, false, false, nil); err != nil {
			t.Errorf("case failed: %q: %v", src, err)
		}
	}
}

// TestResidualUnitAndCalc covers compound-unit conversion/simplification and the
// calc() interpolation/nesting parser arms.
func TestResidualUnitAndCalc(t *testing.T) {
	oks := []string{
		// simplifyUnits: canonical cancel (px over in) with rescale.
		`@use "sass:math"; .a{ v: math.div(96px, 1in) }`,
		// convertUnitList: unknown-unit exact match across compound units when
		// aligning two products with the same units in different order.
		`.a{ v: (1px * 1foo) + (1foo * 1px) }`,
		// calc() with interpolation and a nested parenthesized sub-expression.
		`.a{ v: calc(#{1px} + (2 * 3px)) }`,
		`.a{ v: calc((1px + 2px) * 3) }`,
		// map literal with a trailing comma.
		`@use "sass:meta"; .a{ v: meta.inspect((x: 1, y: 2,)) }`,
	}
	for _, src := range oks {
		if _, err := Render(src, false, false, nil); err != nil {
			t.Errorf("unit/calc case failed: %q: %v", src, err)
		}
	}
	// convertUnitList mismatch: aligning products with incompatible unknown
	// units fails (idx < 0 arm).
	mustErr(t, `.a{ v: (1px * 1foo) + (1px * 1bar) }`)
}

// TestResidualPreludeComments covers comment handling inside at-rule preludes,
// which route through parseInterpolatedText.
func TestResidualPreludeComments(t *testing.T) {
	oks := []string{
		`@media screen /* block */ and (min-width: 1px) { a{b:c} }`,
		`@supports (display: grid) /* c */ { a{b:c} }`,
		"@media screen // line\n  and (min-width: 1px) { a{b:c} }",
		`@foo bar ("x") [y] { a{b:c} }`,
	}
	for _, src := range oks {
		if _, err := Render(src, false, false, nil); err != nil {
			t.Errorf("prelude case failed: %q: %v", src, err)
		}
	}
}

// TestResidualModuleBranches covers namespaced function/mixin resolution, global
// merge of module functions, forwarded-member propagation, plain @import
// evaluation and @forward show/hide member lists.
func TestResidualModuleBranches(t *testing.T) {
	files := map[string]string{
		"cssmod": `.m { x: y }`,
		"fnmod":  `@function f() { @return 1 } @mixin mm { a: 1 }`,
		"inner2": `@function fi() { @return 7 } @mixin mi { b: 2 }`,
		"outer2": `@forward "inner2";`,
		"plain":  `.imported { x: y }`,
		"showm":  `$v: 1; @function bar() { @return 1 } @mixin baz { c: 3 }`,
	}
	oks := []string{
		// @use'd module that emits CSS is spliced into the output.
		`@use "cssmod"; .a{ z: 1 }`,
		// namespaced function found.
		`@use "fnmod" as a; .a{ v: a.f() }`,
		// namespaced mixin found.
		`@use "fnmod" as a; .a{ @include a.mm }`,
		// @use as * merges module functions globally.
		`@use "fnmod" as *; .a{ v: f() }`,
		// forwarded function + mixin propagate through a re-forwarding module.
		`@use "outer2" as o; .a{ v: o.fi(); @include o.mi }`,
		// successful plain @import evaluates the module body inline.
		`@import "plain"; .a{ z: 1 }`,
		// @forward show/hide with plain (non-$) member identifiers and a $var.
		`@forward "showm" show bar, baz;`,
		`@forward "showm" show $v, bar;`,
		`@forward "showm" hide bar;`,
	}
	for _, src := range oks {
		if _, err := renderImp(t, src, files); err != nil {
			t.Errorf("module case failed: %q: %v", src, err)
		}
	}
	// unresolved @import falls back to a passthrough @import rule.
	res, err := Render(`@import "no-such-module-xyz";`, false, false, nil)
	if err != nil {
		t.Fatalf("passthrough import: %v", err)
	}
	if !strings.Contains(res.CSS, "@import") {
		t.Errorf("passthrough import missing: %q", res.CSS)
	}
}

// TestResidualDefensiveArms covers guarded branches that normal source cannot
// reach: canStartValue's sign arm and applyExtends' nil-box defense (an @extend
// whose enclosing rule never registered a selector).
func TestResidualDefensiveArms(t *testing.T) {
	// canStartValue accepts a leading sign.
	if !newParser("-2px").canStartValue() {
		t.Error("canStartValue('-'): want true")
	}
	if !newParser("+2px").canStartValue() {
		t.Error("canStartValue('+'): want true")
	}
	if !newParser("#fff").canStartValue() {
		t.Error("canStartValue('#'): want true")
	}

	// The extend finalizer skips an @extend whose enclosing rule has no box (its
	// selector was never registered), rather than dereferencing a nil box.
	e := newEvaluator(nil)
	rule := &cssStyleRule{} // never added via addSelector -> box stays nil
	e.extendEvents = []extendEvent{{ext: &pendingExtend{
		rule:    rule,
		targets: parseSelectorList(".b").list,
	}}}
	e.applyAllExtends() // must not panic; the nil-box arm continues.
}
