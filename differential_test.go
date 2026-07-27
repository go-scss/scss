// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/go-scss/scss"
)

// corpusEntry is one input file plus its two frozen golden outputs.
type corpusEntry struct {
	input      string
	expanded   string
	compressed string
	indented   bool
}

func loadCorpus(t *testing.T) []corpusEntry {
	t.Helper()
	dir := filepath.Join("testdata", "corpus")
	matches, err := filepath.Glob(filepath.Join(dir, "*.scss"))
	if err != nil {
		t.Fatal(err)
	}
	sassFiles, _ := filepath.Glob(filepath.Join(dir, "*.sass"))
	matches = append(matches, sassFiles...)
	sort.Strings(matches)
	var entries []corpusEntry
	for _, in := range matches {
		base := strings.TrimSuffix(in, filepath.Ext(in))
		exp, err := os.ReadFile(base + ".expanded.css")
		if err != nil {
			t.Fatalf("missing golden for %s: %v", in, err)
		}
		cmp, err := os.ReadFile(base + ".compressed.css")
		if err != nil {
			t.Fatalf("missing golden for %s: %v", in, err)
		}
		entries = append(entries, corpusEntry{
			input:      in,
			expanded:   string(exp),
			compressed: string(cmp),
			indented:   strings.HasSuffix(in, ".sass"),
		})
	}
	if len(entries) == 0 {
		t.Fatal("no corpus files found")
	}
	return entries
}

// TestGoldenExpanded checks the compiler against frozen dart-sass output
// (expanded). Runs in CI without dart-sass installed.
func TestGoldenExpanded(t *testing.T) {
	for _, e := range loadCorpus(t) {
		e := e
		t.Run(filepath.Base(e.input), func(t *testing.T) {
			src, err := os.ReadFile(e.input)
			if err != nil {
				t.Fatal(err)
			}
			opts := &scss.Options{BaseDir: filepath.Dir(e.input)}
			if e.indented {
				opts.Syntax = scss.SyntaxIndented
			}
			res, err := scss.CompileString(string(src), opts)
			if err != nil {
				t.Fatalf("compile error: %v", err)
			}
			if res.CSS != e.expanded {
				t.Errorf("expanded mismatch for %s:\n--- want (dart-sass) ---\n%s\n--- got ---\n%s", e.input, e.expanded, res.CSS)
			}
		})
	}
}

// TestGoldenCompressed checks the compiler against frozen dart-sass output
// (compressed).
func TestGoldenCompressed(t *testing.T) {
	for _, e := range loadCorpus(t) {
		e := e
		t.Run(filepath.Base(e.input), func(t *testing.T) {
			src, err := os.ReadFile(e.input)
			if err != nil {
				t.Fatal(err)
			}
			opts := &scss.Options{BaseDir: filepath.Dir(e.input), Style: scss.Compressed}
			if e.indented {
				opts.Syntax = scss.SyntaxIndented
			}
			res, err := scss.CompileString(string(src), opts)
			if err != nil {
				t.Fatalf("compile error: %v", err)
			}
			if res.CSS != e.compressed {
				t.Errorf("compressed mismatch for %s:\n--- want (dart-sass) ---\n%s\n--- got ---\n%s", e.input, e.compressed, res.CSS)
			}
		})
	}
}

// TestDifferentialLive re-verifies the corpus against a live dart-sass binary
// when available (skipped otherwise), guarding against golden drift. This is the
// oracle gate: dart-sass output is the source of truth.
func TestDifferentialLive(t *testing.T) {
	sassBin, err := exec.LookPath("sass")
	if err != nil {
		t.Skip("dart-sass (sass) not installed; skipping live differential")
	}
	for _, e := range loadCorpus(t) {
		e := e
		t.Run(filepath.Base(e.input), func(t *testing.T) {
			for _, style := range []string{"expanded", "compressed"} {
				args := []string{"--style=" + style}
				if e.indented {
					args = append(args, "--indented")
				}
				args = append(args, e.input)
				out, err := exec.Command(sassBin, args...).Output()
				if err != nil {
					t.Fatalf("dart-sass failed on %s: %v", e.input, err)
				}
				opts := &scss.Options{BaseDir: filepath.Dir(e.input)}
				if style == "compressed" {
					opts.Style = scss.Compressed
				}
				if e.indented {
					opts.Syntax = scss.SyntaxIndented
				}
				src, _ := os.ReadFile(e.input)
				res, cerr := scss.CompileString(string(src), opts)
				if cerr != nil {
					t.Fatalf("compile error: %v", cerr)
				}
				if res.CSS != string(out) {
					t.Errorf("live differential mismatch %s (%s):\n--- dart ---\n%s\n--- go ---\n%s", e.input, style, out, res.CSS)
				}
			}
		})
	}
}
