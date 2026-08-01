// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import (
	"strings"
	"testing"
)

// TestUseDefaultNamespaceStripsAllExtensions covers that the default @use
// namespace is the basename up to its first ".", so every file extension is
// discarded at once (dart-sass _defaultNamespace).
func TestUseDefaultNamespaceStripsAllExtensions(t *testing.T) {
	files := map[string]string{"other.foo.bar.baz.scss": "$variable: value;"}
	r, err := renderImp(t, `@use "other.foo.bar.baz.scss";
a {b: other.$variable}`, files)
	if err != nil {
		t.Fatalf("multi-extension namespace: %v", err)
	}
	if !strings.Contains(r.CSS, "b: value") {
		t.Fatalf("want b: value, got %q", r.CSS)
	}
}

// TestNestedImportForwardPrecedence covers that a @forward reached through a
// @import nested in a style rule makes the forwarded variable win over a
// same-scope variable set before the import.
func TestNestedImportForwardPrecedence(t *testing.T) {
	files := map[string]string{
		"midstream": `@forward "upstream";`,
		"upstream":  `$a: in-upstream;`,
	}
	r, err := renderImp(t, "b {\n  $a: in-input;\n  @import \"midstream\";\n  c: $a;\n}", files)
	if err != nil {
		t.Fatalf("nested import forward precedence: %v", err)
	}
	if !strings.Contains(r.CSS, "c: in-upstream") {
		t.Fatalf("want c: in-upstream, got %q", r.CSS)
	}
}

// TestNestedImportForwardWriteThrough guards that clearing the shadowing local
// does not break write-through: a write after the nested import reaches the
// source module's live slot, so the source's own function observes it.
func TestNestedImportForwardWriteThrough(t *testing.T) {
	files := map[string]string{
		"midstream": `@forward "upstream";`,
		"upstream":  "$b: old value;\n\n@function get-b() {@return $b}",
	}
	r, err := renderImp(t, "a {\n  @import \"midstream\";\n  $b: new value;\n  c: get-b();\n}", files)
	if err != nil {
		t.Fatalf("nested import forward write-through: %v", err)
	}
	if !strings.Contains(r.CSS, "c: new value") {
		t.Fatalf("want c: new value, got %q", r.CSS)
	}
}
