// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package main

import (
	"bytes"
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
