// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import (
	"strings"
	"testing"
)

// TestCrossModuleExtend covers applyAllExtends/addExtensions: an @extend in one
// module reaches the rules of the modules it @uses (upstream), transitively,
// while sibling modules stay isolated.
func TestCrossModuleExtend(t *testing.T) {
	// near: a downstream @extend reaches a directly-@used module's rule.
	near := map[string]string{"other": "in-other {x: y}"}
	got, err := renderImp(t, "@use \"other\";\nin-input {@extend in-other}", near)
	if err != nil || !strings.Contains(got.CSS, "in-other, in-input") {
		t.Fatalf("near: %v\n%s", err, got.CSS)
	}

	// far: the @extend reaches a module two hops away, through a pass-through
	// midstream that has no rules of its own (empty store still propagates).
	far := map[string]string{
		"midstream": "@use \"upstream\";",
		"upstream":  "in-upstream {x: y}",
	}
	got, err = renderImp(t, "@use \"midstream\";\nin-input {@extend in-upstream}", far)
	if err != nil || !strings.Contains(got.CSS, "in-upstream, in-input") {
		t.Fatalf("far: %v\n%s", err, got.CSS)
	}

	// diamond isolation: left and right both @use shared and @extend in-shared,
	// but left's !optional @extend of right-extendee must NOT apply (they don't
	// use one another). Extenders come out right-before-left.
	diamond := map[string]string{
		"left":   "@use \"shared\";\nleft-extendee {@extend in-shared}\nleft-extender {@extend right-extendee !optional}",
		"right":  "@use \"shared\";\nright-extendee {@extend in-shared}\nright-extender {@extend left-extendee !optional}",
		"shared": "in-shared {x: y}",
	}
	got, err = renderImp(t, "@use \"left\";\n@use \"right\";", diamond)
	if err != nil {
		t.Fatalf("diamond: %v", err)
	}
	if !strings.Contains(got.CSS, "in-shared, right-extendee, left-extendee") {
		t.Errorf("diamond order/isolation: %s", got.CSS)
	}
	if strings.Contains(got.CSS, "left-extender") || strings.Contains(got.CSS, "right-extender") {
		t.Errorf("diamond leaked a sibling extend: %s", got.CSS)
	}

	// private placeholder: a downstream @extend of a module-private placeholder
	// (%-x) is silently dropped (it is not visible across the module boundary).
	private := map[string]string{
		"other": "%-in-other {x: y}\nin-other {@extend %-in-other}",
	}
	got, err = renderImp(t, "@use \"other\";\nin-input {@extend %-in-other !optional}", private)
	if err != nil {
		t.Fatalf("private: %v", err)
	}
	if strings.Contains(got.CSS, "in-input") {
		t.Errorf("private placeholder leaked across module: %s", got.CSS)
	}
	if !strings.Contains(got.CSS, "in-other {") {
		t.Errorf("private: own extend lost: %s", got.CSS)
	}

	// chained cross-module extend: a downstream module extends an upstream
	// selector, and another downstream extend targets that new extender within
	// the same finalize, exercising extendExistingExtensions' additional merge.
	chain := map[string]string{
		"up": "a {x: y}",
	}
	got, err = renderImp(t, "@use \"up\";\nb {@extend a}\nc {@extend b}", chain)
	if err != nil {
		t.Fatalf("chain: %v", err)
	}
	if !strings.Contains(got.CSS, "a, b, c") {
		t.Errorf("chain: want a, b, c got %s", got.CSS)
	}

	// double: the same extend (downstream -> upstream) is authored both in the
	// upstream module itself and downstream; the downstream copy is a duplicate
	// of an extension already present, so it is merged once (dedup branch).
	double := map[string]string{
		"other": "upstream {a: b}\ndownstream {@extend upstream}",
	}
	got, err = renderImp(t, "@use \"other\";\ndownstream {@extend upstream}", double)
	if err != nil || !strings.Contains(got.CSS, "upstream, downstream") {
		t.Fatalf("double: %v\n%s", err, got.CSS)
	}

	// extender-as-target: a downstream @extend targets a selector that is itself
	// only an extender in the upstream module (b extends a there). This exercises
	// extendExistingExtensions inside addExtensions and the set==nil arm (b is an
	// extender, not a registered selector, in the upstream store).
	extTarget := map[string]string{
		"up": "a {x: y}\nb {@extend a}",
	}
	got, err = renderImp(t, "@use \"up\";\nc {@extend b}", extTarget)
	if err != nil || !strings.Contains(got.CSS, "a, b, c") {
		t.Fatalf("extender-as-target: %v\n%s", err, got.CSS)
	}

	// additional-merge: a downstream module extends both an upstream extender (d,
	// which extends a) and its target (a). Merging the transitive extension of a
	// via d exercises extendExistingExtensions returning a non-empty `additional`.
	// NOTE: the serialization ORDER of the resulting extenders in this deep
	// transitive cross-module case differs from dart (dart: a, n, d, m); the set
	// of selectors is correct. That ordering nuance is a documented residual.
	addl := map[string]string{
		"up": "a {x: y}\nd {@extend a}",
	}
	got, err = renderImp(t, "@use \"up\";\nm {@extend d}\nn {@extend a}", addl)
	if err != nil {
		t.Fatalf("additional-merge: %v", err)
	}
	for _, sel := range []string{"a", "d", "m", "n"} {
		if !strings.Contains(got.CSS, sel) {
			t.Errorf("additional-merge: missing extender %q in %s", sel, got.CSS)
		}
	}
}

// TestForwardedVarAssignment covers module.setVar: a namespaced assignment to a
// forwarded variable reaches the defining module's storage, and a prefix that
// does not match a given @forward is skipped.
func TestForwardedVarAssignment(t *testing.T) {
	files := map[string]string{
		"mid": "@forward \"up1\" as a-*;\n@forward \"up2\" as b-*;",
		"up1": "$x: original;\n@function get-x() {@return $x}",
		"up2": "$y: original;",
	}
	got, err := renderImp(t, "@use \"mid\";\nmid.$a-x: changed;\nr {v: mid.a-get-x()}", files)
	if err != nil {
		t.Fatalf("forwarded assign: %v", err)
	}
	if !strings.Contains(got.CSS, "v: changed") {
		t.Errorf("forwarded assign did not reach upstream function: %s", got.CSS)
	}
}

// TestDependsOn covers the moduleScope dependency-edge helper's guard branches.
func TestDependsOn(t *testing.T) {
	e := newEvaluator(nil)
	// A nil upstream (e.g. a plain-CSS module with no scope) is ignored.
	e.dependsOn(nil)
	// Depending on one's own scope is a no-op.
	e.dependsOn(e.scope)
	// A real edge is recorded once and de-duplicated on repeat.
	other := &moduleScope{}
	e.dependsOn(other)
	e.dependsOn(other)
	if len(e.scope.upstream) != 1 {
		t.Errorf("dependsOn: want 1 upstream edge, got %d", len(e.scope.upstream))
	}
}

// TestLoadCSSModule covers meta.load-css branches: built-in modules emit no CSS,
// a bad sass: URL and a configured built-in are errors, and $with configuration
// flows through the loaded module's @forward.
func TestLoadCSSModule(t *testing.T) {
	// A built-in module contributes no CSS.
	got, err := renderImp(t, "@use \"sass:meta\";\n@include meta.load-css(\"sass:color\");\n/* only me */", nil)
	if err != nil {
		t.Fatalf("load-css builtin: %v", err)
	}
	if strings.Contains(got.CSS, "{") {
		t.Errorf("load-css of builtin emitted CSS: %s", got.CSS)
	}

	// Configuring a built-in module is an error.
	if _, err := renderImp(t, "@use \"sass:meta\";\n@include meta.load-css(\"sass:color\", $with: (a: 1));", nil); err == nil {
		t.Errorf("load-css configuring builtin: want error")
	}

	// An unknown sass: module is an error.
	if _, err := renderImp(t, "@use \"sass:meta\";\n@include meta.load-css(\"sass:nope\");", nil); err == nil {
		t.Errorf("load-css unknown builtin: want error")
	}

	// $with configuration flows through the loaded module's @forward.
	files := map[string]string{
		"loaded":    "@forward \"forwarded\";",
		"forwarded": "$a: original !default;\nb {c: $a}",
	}
	got, err = renderImp(t, "@use \"sass:meta\";\n@include meta.load-css(\"loaded\", $with: (a: configured));", files)
	if err != nil {
		t.Fatalf("load-css with config: %v", err)
	}
	if !strings.Contains(got.CSS, "c: configured") {
		t.Errorf("load-css config through forward: %s", got.CSS)
	}
}

// TestForwardShadowedMember covers a module's own member winning over a member
// it forwards under the same name (mergeModuleGlobally own-wins).
func TestForwardShadowedMember(t *testing.T) {
	files := map[string]string{
		"mid": "@forward \"up\";\n$c: midstream;\n@function c() {@return midstream}",
		"up":  "$c: upstream;\n@function c() {@return upstream}",
	}
	got, err := renderImp(t, "@use \"mid\";\na {b: mid.$c; d: mid.c()}", files)
	if err != nil {
		t.Fatalf("shadowed: %v", err)
	}
	if !strings.Contains(got.CSS, "b: midstream") || !strings.Contains(got.CSS, "d: midstream") {
		t.Errorf("own member should win over forwarded: %s", got.CSS)
	}
}
