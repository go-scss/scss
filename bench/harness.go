//go:build ignore

// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

// Command harness is the cross-implementation wall-clock benchmark for go-scss.
// It compiles each corpus stylesheet with go-scss (in-process) and, when their
// binaries are available, with dart-sass and libsass/sassc (each a subprocess),
// in both expanded and compressed output styles, and prints a Markdown table of
// median/min wall-clock plus throughput.
//
// Methodology. Each measurement is the median (and min) of -runs timed
// iterations after -warmup discarded iterations (which also warm the OS file
// cache). go-scss is timed in-process around scss.Compile, so its number is
// pure compile time. Each external compiler is timed around its subprocess;
// because that includes process/VM startup, the harness also measures each
// compiler's startup on an empty file and reports a startup-subtracted
// "compile-only" figure, plus the raw end-to-end wall-clock (what a user sees).
// go-scss's own process startup is a few milliseconds and is reported
// separately for the CLI story.
//
// libsass/sassc caveat. libsass is feature-frozen: it has no modern module
// system (@use/@forward) and no CSS Color Level 4 / sass:color / sass:math
// functions. When an external compiler cannot compile an input at all, the
// harness records "cannot compile" for that cell instead of a time — that is a
// correctness gap, not a speed result. Even where libsass does compile a legacy
// @import stylesheet, its output is not byte-identical to dart-sass (go-scss is);
// the libsass column is therefore a compile-speed datapoint only.
//
// Usage:
//
//	go run bench/harness.go [-runs 15] [-warmup 3] \
//	    [-sass /path/to/sass] [-sassc /path/to/sassc] \
//	    [-bootstrap /path/to/bootstrap.scss] [-foundation /path/to/foundation.scss]
//
// The self-contained bench/corpus/generated.scss is always included. Bootstrap
// and Foundation are included when their entry files are supplied. Pass an empty
// value to -sass or -sassc to skip that compiler.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"time"

	"github.com/go-scss/scss"
)

type input struct {
	name string
	path string
}

// tool is an external (subprocess) SCSS compiler compared against go-scss.
type tool struct {
	name    string // column label, e.g. "dart" or "sassc"
	bin     string // resolved binary path ("" = skip)
	args    func(path string, compressed bool) []string
	startup time.Duration
}

func main() {
	runs := flag.Int("runs", 15, "timed iterations per measurement")
	warmup := flag.Int("warmup", 3, "discarded warm-up iterations")
	sassBin := flag.String("sass", "sass", "dart-sass binary (empty to skip)")
	sasscBin := flag.String("sassc", "sassc", "libsass/sassc binary (empty to skip)")
	bootstrap := flag.String("bootstrap", "", "path to bootstrap.scss entry (optional)")
	foundation := flag.String("foundation", "", "path to foundation entry (optional)")
	flag.Parse()

	// The harness is meant to be run from the repo root (go run bench/harness.go).
	gen := filepath.Join("bench", "corpus", "generated.scss")
	if _, err := os.Stat(gen); err != nil {
		fmt.Fprintf(os.Stderr, "run from the repo root: %s not found\n", gen)
		os.Exit(1)
	}

	inputs := []input{{"generated", gen}}
	// modern.scss uses @use/@forward and the sass:color / sass:math modules, so
	// dart-sass and go-scss compile it but libsass cannot — it makes the feature
	// gap show up as a "cannot compile" cell rather than a missing note.
	if mod := filepath.Join("bench", "corpus", "modern.scss"); statOK(mod) {
		inputs = append(inputs, input{"modern", mod})
	}
	if *bootstrap != "" {
		inputs = append(inputs, input{"bootstrap", *bootstrap})
	}
	if *foundation != "" {
		inputs = append(inputs, input{"foundation", *foundation})
	}

	// dart-sass: expanded is the default style; --no-source-map keeps the
	// subprocess honest. sassc defaults to the legacy "nested" style, so it must
	// be told "expanded" explicitly for an apples-to-apples serializer; sassc
	// writes no source map unless asked.
	tools := []*tool{
		{
			name: "dart",
			bin:  *sassBin,
			args: func(path string, compressed bool) []string {
				a := []string{"--no-source-map"}
				if compressed {
					a = append(a, "--style", "compressed")
				}
				return append(a, path)
			},
		},
		{
			name: "sassc",
			bin:  *sasscBin,
			args: func(path string, compressed bool) []string {
				style := "expanded"
				if compressed {
					style = "compressed"
				}
				return []string{"-t", style, path}
			},
		},
	}

	// Resolve binaries and measure each compiler's startup on an empty input.
	empty := filepath.Join(os.TempDir(), "harness-empty.scss")
	_ = os.WriteFile(empty, []byte("a{b:c}"), 0o644)
	var active []*tool
	for _, t := range tools {
		if t.bin == "" {
			continue
		}
		if _, err := exec.LookPath(t.bin); err != nil {
			fmt.Fprintf(os.Stderr, "%s %q not found: skipping that column\n", t.name, t.bin)
			continue
		}
		s, ok := timeCmd(t, empty, false, *runs, *warmup)
		if !ok {
			fmt.Fprintf(os.Stderr, "%s failed on the empty startup probe: skipping\n", t.name)
			continue
		}
		t.startup = median(s)
		fmt.Printf("%s startup (empty input, subtracted below): %s\n", t.name, ms(t.startup))
		active = append(active, t)
	}
	fmt.Println()

	// Header: fixed go-scss columns, then a (wall | compile-only | ratio) triple
	// per active external compiler.
	hdr := "| corpus | style | go-scss median | go-scss min | go-scss out throughput |"
	sep := "|---|---|---:|---:|---:|"
	for _, t := range active {
		hdr += fmt.Sprintf(" %s wall median | %s compile-only | ratio (%s/​go) |", t.name, t.name, t.name)
		sep += "---:|---:|---:|"
	}
	fmt.Println(hdr)
	fmt.Println(sep)

	for _, in := range inputs {
		for _, style := range []scss.OutputStyle{scss.Expanded, scss.Compressed} {
			styleName := "expanded"
			if style == scss.Compressed {
				styleName = "compressed"
			}
			goSamples, outSize := timeGo(in.path, style, *runs, *warmup)
			goMed, goMin := median(goSamples), min(goSamples)
			// Throughput is generated-CSS bytes per second, which is well defined
			// for multi-file inputs (Bootstrap) too, unlike entry-file size.
			tput := throughput(outSize, goMed)
			row := fmt.Sprintf("| %s | %s | %s | %s | %s |", in.name, styleName, ms(goMed), ms(goMin), tput)
			for _, t := range active {
				samples, ok := timeCmd(t, in.path, style == scss.Compressed, *runs, *warmup)
				if !ok {
					// A hard feature gap (e.g. libsass on @use/Color-4): a
					// correctness gap, not a speed result.
					row += " cannot compile | cannot compile | n/a |"
					continue
				}
				wall := median(samples)
				compile := wall - t.startup
				if compile < 0 {
					compile = 0
				}
				ratio := float64(compile) / float64(goMed)
				row += fmt.Sprintf(" %s | %s | %.2fx |", ms(wall), ms(compile), ratio)
			}
			fmt.Println(row)
		}
	}
}

func timeGo(path string, style scss.OutputStyle, runs, warmup int) ([]time.Duration, int64) {
	opts := &scss.Options{Style: style}
	out := make([]time.Duration, 0, runs)
	var outSize int64
	for i := 0; i < warmup+runs; i++ {
		t := time.Now()
		res, err := scss.Compile(path, opts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "go-scss error on %s: %v\n", path, err)
			os.Exit(1)
		}
		outSize = int64(len(res.CSS))
		if i >= warmup {
			out = append(out, time.Since(t))
		}
	}
	return out, outSize
}

// timeCmd times a subprocess compiler over the given input. It returns the timed
// samples and ok=true on success; if the compiler cannot compile the input (a
// feature gap such as libsass on @use), it returns ok=false rather than aborting
// the whole run, so one compiler's correctness gap does not hide the others'
// numbers.
func timeCmd(t *tool, path string, compressed bool, runs, warmup int) ([]time.Duration, bool) {
	args := t.args(path, compressed)
	out := make([]time.Duration, 0, runs)
	for i := 0; i < warmup+runs; i++ {
		cmd := exec.Command(t.bin, args...)
		cmd.Stdout = nil
		start := time.Now()
		if err := cmd.Run(); err != nil {
			if i == 0 {
				fmt.Fprintf(os.Stderr, "%s cannot compile %s: %v\n", t.name, path, err)
				return nil, false
			}
			// A transient failure after the first run should not be silently
			// averaged in; treat it as fatal for that compiler.
			fmt.Fprintf(os.Stderr, "%s error on %s (run %d): %v\n", t.name, path, i, err)
			return nil, false
		}
		if i >= warmup {
			out = append(out, time.Since(start))
		}
	}
	return out, true
}

func statOK(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func median(d []time.Duration) time.Duration {
	s := append([]time.Duration(nil), d...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	return s[len(s)/2]
}

func min(d []time.Duration) time.Duration {
	m := d[0]
	for _, x := range d[1:] {
		if x < m {
			m = x
		}
	}
	return m
}

func ms(d time.Duration) string { return fmt.Sprintf("%.1f ms", float64(d)/float64(time.Millisecond)) }

func throughput(size int64, d time.Duration) string {
	if d == 0 {
		return "n/a"
	}
	mbps := float64(size) / (1024 * 1024) / d.Seconds()
	return fmt.Sprintf("%.1f MB/s", mbps)
}
