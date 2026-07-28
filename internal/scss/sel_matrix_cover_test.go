// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import (
	"fmt"
	"testing"
)

// selMatrix is a curated set of selectors spanning every simple-selector kind
// and combinator so the selector-algorithm matrix below reaches the deep arms
// of unify/weave/superselector/extend that individual hand cases miss.
var selMatrix = []string{
	"a", "b", "c", ".x", ".y", "#i", "#j", "a.x",
	"a:hover", ":not(a)", ":not(.x)", ":is(a, b)", ":where(a)",
	"a b", ".x .y", "a > b", "#i a", "#j #i", "::slotted(a)",
	"*", "*|a", "a|b", "[a]", "[a=b]", "[a~=b i]", "%p", "a + b", "a ~ b",
}

// TestSelectorMatrixNoPanic asserts that selector.extend/replace/unify over the
// full selector matrix never panics and is deterministic. Not panicking on
// well-formed selectors is a hard correctness property of the engine; running
// the whole matrix drives the unify/weave/superselector branches that no single
// case reaches.
func TestSelectorMatrixNoPanic(t *testing.T) {
	targets := []string{"a", ".x", "#i", "b"}
	render := func(src string) (string, bool) {
		var out string
		ok := true
		func() {
			defer func() {
				if r := recover(); r != nil {
					ok = false
					t.Errorf("panic on %q: %v", src, r)
				}
			}()
			s, err := Render(src, false, false, nil)
			if err == nil {
				out = s.CSS
			}
		}()
		return out, ok
	}
	for _, a := range selMatrix {
		for _, b := range selMatrix {
			for _, tgt := range targets {
				for _, fn := range []string{"extend", "replace"} {
					src := fmt.Sprintf(`@use "sass:selector"; .r{v: selector.%s(%q, %q, %q)}`, fn, a, tgt, b)
					o1, ok := render(src)
					if !ok {
						continue
					}
					if o2, _ := render(src); o1 != o2 {
						t.Errorf("non-deterministic %q: %q vs %q", src, o1, o2)
					}
				}
			}
			for _, fn := range []string{"unify", "is-superselector", "append", "nest"} {
				src := fmt.Sprintf(`@use "sass:selector"; .r{v: selector.%s(%q, %q)}`, fn, a, b)
				o1, ok := render(src)
				if !ok {
					continue
				}
				if o2, _ := render(src); o1 != o2 {
					t.Errorf("non-deterministic %q: %q vs %q", src, o1, o2)
				}
			}
		}
	}
}

// TestExtendGraphMatrix drives @extend across a matrix of base selectors,
// building single, transitive and multi-target extension graphs. It reaches the
// extension-store arms (existing-extension merge, selector rewrite, transitive
// closure) that only fire when real rules and extends interact.
func TestExtendGraphMatrix(t *testing.T) {
	base := []string{"a", "b", ".x", ".y", "#i", "a b", ".x .y", "#i a", "a:hover", ":not(a)", "a.x", "#i #j"}
	firstSimple := func(s string) string {
		switch s {
		case "a b":
			return "b"
		case ".x .y":
			return ".y"
		case "#i a":
			return "a"
		case "a:hover":
			return "a"
		case ":not(a)":
			return "a"
		case "a.x":
			return "a"
		case "#i #j":
			return "#j"
		}
		return s
	}
	render := func(src string) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("panic on %q: %v", src, r)
			}
		}()
		_, _ = Render(src, false, false, nil)
	}
	for _, s1 := range base {
		for _, s2 := range base {
			for _, s3 := range base {
				render(fmt.Sprintf("%s {p: q} %s {@extend %s} %s {@extend %s}",
					s1, s2, firstSimple(s1), s3, firstSimple(s2)))
			}
		}
	}
}
