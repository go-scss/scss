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
	if _, _, ok := imp("sass:math", "", false); ok {
		t.Error("fileImporter should not resolve sass: URLs")
	}

	cands := importCandidates("dir", "part.scss", false)
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

// TestImportCandidatesImportOnly covers importCandidates' forImport branches:
// an import-only file, and an import-only directory index, must precede the
// ordinary candidate of the same name — mirroring Dart Sass's resolveImportPath
// under forImport. Verified against dart-sass 1.102 behaviour.
func TestImportCandidatesImportOnly(t *testing.T) {
	// Extensionless legacy @import: x.import.scss / _x.import.scss lead, then the
	// ordinary x.scss/_x.scss, then index.import files, then plain index files.
	cands := importCandidates("dir", "other", true)
	first := filepath.Join("dir", "other.import.scss")
	if cands[0] != first {
		t.Fatalf("import-only lead: want %q got %q", first, cands[0])
	}
	idx := indexOf(cands, filepath.Join("dir", "other.import.scss"))
	plain := indexOf(cands, filepath.Join("dir", "other.scss"))
	iidx := indexOf(cands, filepath.Join("dir", "other", "index.import.scss"))
	pidx := indexOf(cands, filepath.Join("dir", "other", "index.scss"))
	if !(idx < plain && plain < iidx && iidx < pidx) {
		t.Errorf("precedence order wrong: import=%d plain=%d importIndex=%d plainIndex=%d", idx, plain, iidx, pidx)
	}

	// Explicit-extension legacy @import: other.import.scss precedes other.scss.
	ext := importCandidates("dir", "other.scss", true)
	if ext[0] != filepath.Join("dir", "other.import.scss") {
		t.Errorf("explicit-extension import-only lead: got %q", ext[0])
	}
	if indexOf(ext, filepath.Join("dir", "other.import.scss")) >= indexOf(ext, filepath.Join("dir", "other.scss")) {
		t.Errorf("explicit-extension precedence wrong: %v", ext)
	}

	// Without forImport, no .import candidates are generated.
	for _, c := range importCandidates("dir", "other", false) {
		if strings.Contains(c, ".import") {
			t.Errorf("forImport=false must not yield import-only candidate %q", c)
		}
	}
}

func indexOf(ss []string, want string) int {
	for i, s := range ss {
		if s == want {
			return i
		}
	}
	return -1
}
