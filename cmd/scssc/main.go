// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

// Command scssc compiles SCSS/Sass to CSS on stdout. Source is read from a file
// named as a positional argument (with @import/@use resolution relative to its
// directory, as `sass <file>` does) or, with no argument, from stdin. It doubles
// as the differential-testing harness against dart-sass.
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
	opts := &scss.Options{}
	if *style == "compressed" {
		opts.Style = scss.Compressed
	}
	if *indented {
		opts.Syntax = scss.SyntaxIndented
	}
	var res *scss.CompileResult
	var err error
	if files := fs.Args(); len(files) > 0 {
		// A positional argument names an input file, compiled with @import/@use
		// resolution relative to its directory — the mode used to compile real
		// frameworks such as Bootstrap.
		res, err = scss.Compile(files[0], opts)
	} else {
		data, rerr := io.ReadAll(stdin)
		if rerr != nil {
			fmt.Fprintln(stderr, "read error:", rerr)
			return 1
		}
		res, err = scss.CompileString(string(data), opts)
	}
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
