// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import (
	"strings"
	"testing"
)

// TestLoadCSSConfigCaching covers meta.load-css's single-module-registry
// semantics: a module loaded (and configured) once via load-css is reused, with
// that configuration intact, by a later @use/@forward of the same URL. The CSS
// is nonetheless spliced independently at every load-css site.
func TestLoadCSSConfigCaching(t *testing.T) {
	// load-css configures "up"; a later @use (through "mid") sees the config.
	files := map[string]string{
		"up":  "$a: original !default;",
		"mid": `@use "up"; b { c: up.$a }`,
	}
	src := `@use "sass:meta";
@include meta.load-css("up", $with: (a: configured));
@include meta.load-css("mid");`
	r, err := renderImp(t, src, files)
	if err != nil {
		t.Fatalf("load-css config caching: %v", err)
	}
	if !strings.Contains(r.CSS, "c: configured") {
		t.Fatalf("want configured member, got %q", r.CSS)
	}

	// An empty $with map counts as no configuration: the second load reuses the
	// already-configured "up" without erroring.
	src2 := `@use "sass:meta";
@include meta.load-css("up", $with: (a: configured));
@include meta.load-css("mid", $with: ());`
	r2, err := renderImp(t, src2, files)
	if err != nil {
		t.Fatalf("empty-config second load: %v", err)
	}
	if !strings.Contains(r2.CSS, "c: configured") {
		t.Fatalf("want configured member, got %q", r2.CSS)
	}
}

// TestLoadCSSReconfigureError covers the error branch: configuring a module that
// another load site already loaded is rejected, exactly as `@use ... with`.
func TestLoadCSSReconfigureError(t *testing.T) {
	files := map[string]string{"up": "$a: original !default; x { y: $a }"}
	// load-css configured, then load-css the same module configured again.
	mustErrImp(t, `@use "sass:meta";
@include meta.load-css("up", $with: (a: c1));
@include meta.load-css("up", $with: (a: c2));`, files)
	// @use first, then load-css the same module with a configuration.
	mustErrImp(t, `@use "up";
@use "sass:meta";
@include meta.load-css("up", $with: (a: c));`, files)
}

// TestLoadCSSIndependentCopies covers that each load-css site emits its own CSS
// copy with its own extend scope even when the module is already registered:
// re-nesting and extends applied at one site never leak into another.
func TestLoadCSSIndependentCopies(t *testing.T) {
	files := map[string]string{"other": "c { d: e }"}
	// Different nesting: two load-css sites nest "other" under distinct selectors.
	r, err := renderImp(t, `@use "sass:meta";
a { @include meta.load-css("other") }
b { @include meta.load-css("other") }`, files)
	if err != nil {
		t.Fatalf("different nesting: %v", err)
	}
	if !strings.Contains(r.CSS, "a c") || !strings.Contains(r.CSS, "b c") {
		t.Fatalf("want independent a c / b c nesting, got %q", r.CSS)
	}

	// Different extend: an @extend at one load site must not reach the other copy.
	ef := map[string]string{
		"other": "a { b: c }",
		"left":  `@use "sass:meta"; @include meta.load-css("other"); left { @extend a }`,
		"right": `@use "sass:meta"; @include meta.load-css("other"); right { @extend a }`,
	}
	r2, err := renderImp(t, `@use "left"; @use "right";`, ef)
	if err != nil {
		t.Fatalf("different extend: %v", err)
	}
	if strings.Contains(r2.CSS, "left, right") || strings.Contains(r2.CSS, "right, left") {
		t.Fatalf("extends leaked across load-css copies: %q", r2.CSS)
	}
	if !strings.Contains(r2.CSS, "left") || !strings.Contains(r2.CSS, "right") {
		t.Fatalf("want both extends applied to own copy, got %q", r2.CSS)
	}
}
