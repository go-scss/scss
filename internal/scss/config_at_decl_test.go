// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import "testing"

// TestConfigAppliedAtDeclaration verifies dart-sass's configuration timing: a
// `@use ... with (...)` value is applied at the module's `!default` declaration,
// so meta.variable-exists is false before that line and the configured value
// overrides the default there.
func TestConfigAppliedAtDeclaration(t *testing.T) {
	files := map[string]string{
		"_other.scss": "@use \"sass:meta\";\n" +
			"$before: meta.variable-exists(a);\n" +
			"$a: original !default;\n" +
			"b {\n  before: $before;\n  after: meta.variable-exists(a);\n  value: $a;\n}\n",
	}
	got := renderWith(t, "@use \"other\" with ($a: configured);\n", files)
	want := "b {\n  before: false;\n  after: true;\n  value: configured;\n}\n"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

// TestConfigNullFallsThroughToDefault verifies that a null configured value is
// consumed but leaves the variable unset, so the `!default` value applies.
func TestConfigNullFallsThroughToDefault(t *testing.T) {
	files := map[string]string{
		"_other.scss": "$a: original !default;\nb {c: $a}\n",
	}
	got := renderWith(t, "@use \"other\" with ($a: null);\n", files)
	want := "b {\n  c: original;\n}\n"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

// TestConfigForwardedThroughForward verifies the configured value still reaches
// an upstream module through a @forward ... with, guarding the propagation path
// after removing the global-scope pre-seed.
func TestConfigForwardedThroughForward(t *testing.T) {
	files := map[string]string{
		"_midstream.scss": "@forward \"upstream\" with ($a: configured);\n",
		"_upstream.scss":  "$a: original !default;\nb {c: $a}\n",
	}
	got := renderWith(t, "@use \"midstream\";\n", files)
	want := "b {\n  c: configured;\n}\n"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
