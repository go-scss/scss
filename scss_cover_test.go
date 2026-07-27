// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCompileCSSFile covers the .css suffix -> SyntaxCSS branch in Compile.
func TestCompileCSSFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plain.css")
	if err := os.WriteFile(path, []byte(".a { color: red }"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Compile(path, nil)
	if err != nil {
		t.Fatalf("compile .css: %v", err)
	}
	if !strings.Contains(res.CSS, "color: red") {
		t.Errorf("got %q", res.CSS)
	}
}

// TestCompileFileError covers Compile's propagation of a CompileString error.
func TestCompileFileError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "broken.scss")
	if err := os.WriteFile(path, []byte(".a { x: $undefined }"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Compile(path, nil); err == nil {
		t.Error("expected error from broken file")
	}
}

// TestFileImporterInternals covers the fileImporter sass: guard and the
// importCandidates explicit-extension (hasSassExt) branch directly.
func TestFileImporterInternals(t *testing.T) {
	imp := fileImporter("somewhere", nil)
	if _, _, ok := imp("sass:math"); ok {
		t.Error("fileImporter should not resolve sass: URLs")
	}

	cands := importCandidates("dir", "part.scss")
	if len(cands) != 2 {
		t.Fatalf("hasSassExt candidates: want 2 got %v", cands)
	}
	if cands[0] != filepath.Join("dir", "part.scss") {
		t.Errorf("first candidate: %q", cands[0])
	}
	if !strings.Contains(cands[1], "_part.scss") {
		t.Errorf("partial candidate: %q", cands[1])
	}
}
