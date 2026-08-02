// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunExpanded(t *testing.T) {
	var out, errb bytes.Buffer
	code := run(nil, strings.NewReader(".a{x:1+1}"), &out, &errb)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	if out.String() != ".a {\n  x: 2;\n}\n" {
		t.Errorf("got %q", out.String())
	}
}

func TestRunCompressed(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"-style", "compressed"}, strings.NewReader(".a{x:1}"), &out, &errb); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if out.String() != ".a{x:1}\n" {
		t.Errorf("got %q", out.String())
	}
}

func TestRunIndented(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"-indented"}, strings.NewReader(".a\n  x: 1\n"), &out, &errb); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(out.String(), "x: 1") {
		t.Errorf("got %q", out.String())
	}
}

func TestRunError(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run(nil, strings.NewReader(".a{x:$undef}"), &out, &errb); code != 1 {
		t.Errorf("expected exit 1, got %d", code)
	}
	if !strings.Contains(errb.String(), "error:") {
		t.Errorf("stderr: %q", errb.String())
	}
}

func TestRunBadFlag(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"-nope"}, strings.NewReader(""), &out, &errb); code != 2 {
		t.Errorf("expected exit 2, got %d", code)
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestRunReadError(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run(nil, errReader{}, &out, &errb); code != 1 {
		t.Errorf("expected exit 1 on read error, got %d", code)
	}
	if !strings.Contains(errb.String(), "read error") {
		t.Errorf("stderr: %q", errb.String())
	}
}

func TestRunFileArg(t *testing.T) {
	dir := t.TempDir()
	partial := filepath.Join(dir, "_p.scss")
	if err := os.WriteFile(partial, []byte("$c: red;"), 0o644); err != nil {
		t.Fatal(err)
	}
	entry := filepath.Join(dir, "in.scss")
	if err := os.WriteFile(entry, []byte("@import 'p';\n.a{color:$c}"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	if code := run([]string{entry}, strings.NewReader(""), &out, &errb); code != 0 {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	if !strings.Contains(out.String(), "color: red") {
		t.Errorf("got %q", out.String())
	}
}

func TestRunFileArgMissing(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{filepath.Join(t.TempDir(), "nope.scss")}, strings.NewReader(""), &out, &errb); code != 1 {
		t.Errorf("expected exit 1 for missing file, got %d", code)
	}
	if !strings.Contains(errb.String(), "error:") {
		t.Errorf("stderr: %q", errb.String())
	}
}

func TestMainSeam(t *testing.T) {
	oldExit, oldArgs, oldStdin := osExit, os.Args, os.Stdin
	defer func() { osExit, os.Args, os.Stdin = oldExit, oldArgs, oldStdin }()
	r, w, _ := os.Pipe()
	_, _ = w.WriteString(".a{x:1}")
	_ = w.Close()
	os.Stdin = r
	os.Args = []string{"scssc"}
	var code int
	osExit = func(c int) { code = c }
	main()
	if code != 0 {
		t.Errorf("main exit %d", code)
	}
}
