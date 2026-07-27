// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

// Command scssc compiles SCSS/Sass from stdin to CSS on stdout. It doubles as
// the differential-testing harness against dart-sass.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/go-scss/scss"
)

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("scssc", flag.ContinueOnError)
	fs.SetOutput(stderr)
	style := fs.String("style", "expanded", "expanded|compressed")
	indented := fs.Bool("indented", false, "indented .sass syntax")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	data, err := io.ReadAll(stdin)
	if err != nil {
		fmt.Fprintln(stderr, "read error:", err)
		return 1
	}
	opts := &scss.Options{}
	if *style == "compressed" {
		opts.Style = scss.Compressed
	}
	if *indented {
		opts.Syntax = scss.SyntaxIndented
	}
	res, err := scss.CompileString(string(data), opts)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	fmt.Fprint(stdout, res.CSS)
	return 0
}

// osExit is a seam so main() is testable.
var osExit = os.Exit

func main() {
	osExit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
