// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import (
	"strings"
	"testing"
)

// TestForUnitBounds covers evalFor's unit handling: the loop variable inherits
// `from`'s units and `to` is coerced into them.
func TestForUnitBounds(t *testing.T) {
	cases := map[string]string{
		// from has a unit; to shares it — values keep the unit.
		"a {@for $i from 1px through 3px {b: $i}}": "b: 1px",
		// from has a unit; to is unitless — coerced, unit kept.
		"a {@for $i from 1px through 3 {b: $i}}": "b: 3px",
		// from unitless; to has a unit — result is unitless.
		"a {@for $i from 1 through 3px {b: $i}}": "b: 3;",
		// compatible units — to converted into from's unit (1cm == 10mm).
		"a {@for $i from 8mm through 1cm {b: $i}}": "b: 10mm",
		// exclusive, descending range keeps units too.
		"a {@for $i from 3px to 1px {b: $i}}": "b: 2px",
	}
	for src, want := range cases {
		if got := compile(t, src); !strings.Contains(got, want) {
			t.Errorf("%q => want substr %q, got %q", src, want, got)
		}
	}
}

// TestNamespacedVarAssignment covers evalVarDecl's namespaced branch and
// assignNamespacedVar (success, !default, and both error paths), plus the
// parser's atNamespacedVarDecl detection.
func TestNamespacedVarAssignment(t *testing.T) {
	files := map[string]string{
		"other":   "$member: value;\n@function get() {@return $member}",
		"hasnull": "$n: null;",
	}
	// Writing a namespaced variable is visible to the module's own functions.
	got, err := renderImp(t, `@use "other";
other.$member: new value;
a {b: other.get()}`, files)
	if err != nil {
		t.Fatalf("namespaced assign: %v", err)
	}
	if !strings.Contains(got.CSS, "b: new value") {
		t.Errorf("namespaced assign: got %q", got.CSS)
	}

	// A guarded assignment keeps an already-configured (non-null) value.
	got, err = renderImp(t, `@use "other";
other.$member: kept !default;
a {b: other.$member}`, files)
	if err != nil || !strings.Contains(got.CSS, "b: value") {
		t.Errorf("namespaced !default keep: %v %q", err, got.CSS)
	}

	// A guarded assignment fills a null variable.
	got, err = renderImp(t, `@use "hasnull";
hasnull.$n: filled !default;
a {b: hasnull.$n}`, files)
	if err != nil || !strings.Contains(got.CSS, "b: filled") {
		t.Errorf("namespaced !default fill null: %v %q", err, got.CSS)
	}

	// Error: no module with that namespace.
	if _, err := renderImp(t, `@use "other";
nope.$member: x;`, files); err == nil {
		t.Error("assign to unknown namespace: want error")
	}
	// Error: the module does not define that variable.
	if _, err := renderImp(t, `@use "other";
other.$missing: x;`, files); err == nil {
		t.Error("assign to undefined module var: want error")
	}

	// atNamespacedVarDecl false branches: an ordinary declaration whose value
	// contains a dot but no `.$`, and a plain identifier declaration.
	okCompile(t, "a.b {width: 1px}") // selector containing a dot, not ns.$var
	okCompile(t, "a {prop-x: 1}")    // ident with no dot
}

// TestConfigThroughForward covers buildConfig's merge rules: a downstream
// mandatory config overriding an upstream `!default` forward config, a null
// downstream config letting the upstream default win, propagation of unrelated
// config, and a plain (unconfigured) forward.
func TestConfigThroughForward(t *testing.T) {
	files := map[string]string{
		// bare @forward propagates the importer's configuration downstream.
		"bare_down": `@forward "bare_up";`,
		"bare_up":   "$a: original !default;\nb {c: $a}",
		// downstream mandatory beats midstream !default.
		"def_down": `@forward "def_mid" with ($a: from-downstream);`,
		"def_mid":  `@forward "def_up" with ($a: from-midstream !default);`,
		"def_up":   "$a: from-upstream !default;\nb {c: $a}",
		// downstream null lets midstream's !default win.
		"null_down": `@forward "null_mid" with ($a: null);`,
		"null_mid":  `@forward "null_up" with ($a: from-midstream !default);`,
		"null_up":   "$a: from-upstream !default;\nb {c: $a}",
		// unrelated incoming var propagates alongside the forward's own config.
		"unrel_down": `@forward "unrel_mid" with ($a: from-downstream);`,
		"unrel_mid":  `@forward "unrel_up" with ($b: from-midstream !default);`,
		"unrel_up":   "$a: from-upstream !default;\n$b: from-upstream !default;\nc {a: $a; b: $b}",
	}
	check := func(src, want string) {
		got, err := renderImp(t, src, files)
		if err != nil {
			t.Fatalf("%q: %v", src, err)
		}
		if !strings.Contains(got.CSS, want) {
			t.Errorf("%q => want %q, got %q", src, want, got.CSS)
		}
	}
	check(`@use "bare_down" with ($a: configured);`, "c: configured")
	check(`@use "def_down";`, "c: from-downstream")
	check(`@use "null_down";`, "c: from-midstream")
	check(`@use "unrel_down";`, "b: from-midstream")
}

// TestModuleCSSChunking covers emitModuleCSS: a module whose only members are
// variables (no visible CSS), a chunk whose first visible node is a comment,
// and a chunk whose first visible node is an at-rule (setBlankBefore's comment
// and at-rule arms).
func TestModuleCSSChunking(t *testing.T) {
	files := map[string]string{
		"varsonly":  "$x: 1;",
		"leadcomm":  "/* lead */\nb {c: d}",
		"leadmedia": "@media screen {e {f: g}}",
		// An empty rule produces an invisible node the chunk must skip before it
		// finds the first visible node.
		"leadempty": "empty {}\nb {c: d}",
	}
	// A variable-only module contributes no CSS: the importing rule stands alone.
	got, err := renderImp(t, `a {b: c}
@use "varsonly";`, files)
	if err != nil || strings.Count(got.CSS, "{") != 1 {
		t.Errorf("vars-only module emitted CSS: %v %q", err, got.CSS)
	}
	// A module chunk led by a comment, after a style rule: blank line + comment.
	got, err = renderImp(t, `a {b: c}
@use "leadcomm";`, files)
	if err != nil || !strings.Contains(got.CSS, "/* lead */") {
		t.Errorf("lead comment chunk: %v %q", err, got.CSS)
	}
	// A module chunk led by an at-rule, after a style rule.
	got, err = renderImp(t, `a {b: c}
@use "leadmedia";`, files)
	if err != nil || !strings.Contains(got.CSS, "@media screen") {
		t.Errorf("lead media chunk: %v %q", err, got.CSS)
	}
	// A module leading with an invisible (empty) rule: the chunk skips it and
	// still emits the following visible rule.
	got, err = renderImp(t, `a {b: c}
@use "leadempty";`, files)
	if err != nil || !strings.Contains(got.CSS, "c: d") || strings.Contains(got.CSS, "empty") {
		t.Errorf("lead empty chunk: %v %q", err, got.CSS)
	}
}

// TestDiamondDedup covers loadModule's shared-registry hit: a module reached
// through two paths is evaluated and emitted exactly once.
func TestDiamondDedup(t *testing.T) {
	files := map[string]string{
		"left":   `@use "shared";` + "\na {file: left}",
		"right":  `@use "shared";` + "\na {file: right}",
		"shared": "a {file: shared}",
	}
	got, err := renderImp(t, `@use "left";
@use "right";
a {file: input}`, files)
	if err != nil {
		t.Fatalf("diamond: %v", err)
	}
	if n := strings.Count(got.CSS, "file: shared"); n != 1 {
		t.Errorf("shared module emitted %d times, want 1: %q", n, got.CSS)
	}
}
