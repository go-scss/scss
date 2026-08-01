// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import (
	"strings"
	"testing"
)

// TestForwardBuiltinModule covers @forward of a built-in module (sass:math):
// a downstream @use of the forwarding stylesheet reaches the built-in's members
// through the forwarding chain, honouring an as-prefix and show/hide filters.
func TestForwardBuiltinModule(t *testing.T) {
	cases := []struct {
		name, other, call, want string
	}{
		{"bare", `@forward "sass:math";`, "other.round(0.7)", "b: 1"},
		{"show", `@forward "sass:math" show round;`, "other.round(0.7)", "b: 1"},
		{"hide", `@forward "sass:math" hide ceil;`, "other.round(0.7)", "b: 1"},
		{"as", `@forward "sass:math" as s-*;`, "other.s-round(0.7)", "b: 1"},
		{"var", `@forward "sass:math";`, "other.$pi", "b: 3.1415926536"},
	}
	for _, c := range cases {
		files := map[string]string{"other": c.other}
		r, err := renderImp(t, `@use "other";
a {b: `+c.call+`}`, files)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if !strings.Contains(r.CSS, c.want) {
			t.Fatalf("%s: want %q in %q", c.name, c.want, r.CSS)
		}
	}
}

// TestForwardBuiltinFiltered covers that show/hide actually withhold a member:
// a hidden function is undefined downstream.
func TestForwardBuiltinFiltered(t *testing.T) {
	// show ceil hides round.
	mustErrImp(t, `@use "other";
a {b: other.round(0.7)}`, map[string]string{"other": `@forward "sass:math" show ceil;`})
	// hide round hides round.
	mustErrImp(t, `@use "other";
a {b: other.round(0.7)}`, map[string]string{"other": `@forward "sass:math" hide round;`})
}

// TestForwardBuiltinErrors covers the eval-time error branches of @forward of a
// built-in: an unknown module name and an illegal configuration.
func TestForwardBuiltinErrors(t *testing.T) {
	mustErrImp(t, `@forward "sass:nope";`, map[string]string{})
	mustErrImp(t, `@forward "sass:math" with ($x: 1);`, map[string]string{})
}

// TestForwardBuiltinGetFunctionCall covers reaching a forwarded built-in through
// a first-class function reference: meta.get-function resolves it via the
// forwarding namespace and meta.call dispatches to the native implementation
// (the callUserResolved built-in path).
func TestForwardBuiltinGetFunctionCall(t *testing.T) {
	files := map[string]string{"other": `@forward "sass:math";`}
	r, err := renderImp(t, `@use "other";
@use "sass:meta";
a {b: meta.call(meta.get-function("round", $module: "other"), 0.7)}`, files)
	if err != nil {
		t.Fatalf("get-function via forward: %v", err)
	}
	if !strings.Contains(r.CSS, "b: 1") {
		t.Fatalf("want b: 1, got %q", r.CSS)
	}
}

// TestForwardBuiltinThroughImport covers the legacy @import path: a stylesheet
// that @forwards a built-in module, when @imported, inlines those members so the
// importing scope can call them directly.
func TestForwardBuiltinThroughImport(t *testing.T) {
	files := map[string]string{"other": `@forward "sass:math";`}
	r, err := renderImp(t, `@import "other";
a {b: round(0.7)}`, files)
	if err != nil {
		t.Fatalf("import forward builtin: %v", err)
	}
	if !strings.Contains(r.CSS, "b: 1") {
		t.Fatalf("want b: 1, got %q", r.CSS)
	}
}
