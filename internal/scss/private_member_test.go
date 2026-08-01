// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import "testing"

// TestPrivateMembersNotExported checks that a module's private members (leading
// "-"/"_") are usable inside the defining module but are not exposed through
// "@use as *": a call to a private function from the consumer resolves to plain
// CSS, while a public sibling resolves normally.
func TestPrivateMembersNotExported(t *testing.T) {
	files := map[string]string{
		"_other.scss": "@function -priv() {@return secret}\n" +
			"@function pub() {@return public}\n" +
			"@mixin -pmix {inner: -priv()}\n",
	}
	src := "@use \"other\" as *;\n" +
		"a {\n  x: -priv();\n  y: pub();\n}\n"
	got := renderWith(t, src, files)
	want := "a {\n  x: -priv();\n  y: public;\n}\n"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

// TestPrivateMemberUsableWithinModule guards that filtering the exported tables
// does not disturb a module's own use of its private members.
func TestPrivateMemberUsableWithinModule(t *testing.T) {
	files := map[string]string{
		"_other.scss": "@function -priv() {@return secret}\n" +
			"@function pub() {@return -priv()}\n",
	}
	src := "@use \"other\";\na {b: other.pub()}\n"
	got := renderWith(t, src, files)
	want := "a {\n  b: secret;\n}\n"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

// TestPublicFiltersHelpers exercises publicFuncs/publicMixins directly, covering
// both the kept-public and dropped-private branches.
func TestPublicFiltersHelpers(t *testing.T) {
	fs := map[string]*funcEntry{"pub": {}, "-priv": {}}
	if got := publicFuncs(fs); len(got) != 1 || got["pub"] == nil || got["-priv"] != nil {
		t.Fatalf("publicFuncs kept wrong set: %v", got)
	}
	ms := map[string]*mixinEntry{"pub": {}, "-priv": {}}
	if got := publicMixins(ms); len(got) != 1 || got["pub"] == nil || got["-priv"] != nil {
		t.Fatalf("publicMixins kept wrong set: %v", got)
	}
}
