//go:build ignore

// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

// Command harness is the cross-implementation wall-clock benchmark for go-scss.
// It compiles each corpus stylesheet with go-scss (in-process) and, when a
// dart-sass binary is available, with dart-sass (subprocess), in both expanded
// and compressed output styles, and prints a Markdown table of median/min
// wall-clock plus throughput.
//
// Methodology. Each measurement is the median (and min) of -runs timed
// iterations after -warmup discarded iterations (which also warm the OS file
// cache). go-scss is timed in-process around scss.Compile, so its number is
// pure compile time. dart-sass is timed around the subprocess; because that
// includes VM startup, the harness also measures dart-sass startup on an empty
// file and reports a startup-subtracted "compile-only" figure, plus the raw
// end-to-end wall-clock (what a user sees). go-scss's own process startup is a
// few milliseconds and is reported separately for the CLI story.
//
// Usage:
//
//	go run bench/harness.go [-runs 15] [-warmup 3] [-sass /path/to/sass] \
//	    [-bootstrap /path/to/bootstrap.scss] [-foundation /path/to/foundation.scss]
//
// The self-contained bench/corpus/generated.scss is always included. Bootstrap
// and Foundation are included when their entry files are supplied.
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

func main() {
	runs := flag.Int("runs", 15, "timed iterations per measurement")
	warmup := flag.Int("warmup", 3, "discarded warm-up iterations")
	sassBin := flag.String("sass", "sass", "dart-sass binary (empty to skip)")
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
	if *bootstrap != "" {
		inputs = append(inputs, input{"bootstrap", *bootstrap})
	}
	if *foundation != "" {
		inputs = append(inputs, input{"foundation", *foundation})
	}

	dart := *sassBin
	if dart != "" {
		if _, err := exec.LookPath(dart); err != nil {
			fmt.Fprintf(os.Stderr, "dart-sass %q not found: comparing go-scss only\n", dart)
			dart = ""
		}
	}

	var dartStartup time.Duration
	if dart != "" {
		empty := filepath.Join(os.TempDir(), "harness-empty.scss")
		_ = os.WriteFile(empty, []byte("a{b:c}"), 0o644)
		dartStartup = median(timeCmd(dart, empty, false, *runs, *warmup))
		fmt.Printf("dart-sass startup (empty input, subtracted below): %s\n\n", ms(dartStartup))
	}

	fmt.Println("| corpus | style | go-scss median | go-scss min | dart wall median | dart compile-only | go-scss out throughput | ratio (dart/​go) |")
	fmt.Println("|---|---|---:|---:|---:|---:|---:|---:|")
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
			row := fmt.Sprintf("| %s | %s | %s | %s |", in.name, styleName, ms(goMed), ms(goMin))
			if dart != "" {
				dartSamples := timeCmd(dart, in.path, style == scss.Compressed, *runs, *warmup)
				dartMed := median(dartSamples)
				dartCompile := dartMed - dartStartup
				if dartCompile < 0 {
					dartCompile = 0
				}
				ratio := float64(dartCompile) / float64(goMed)
				row += fmt.Sprintf(" %s | %s | %s | %.2fx |", ms(dartMed), ms(dartCompile), tput, ratio)
			} else {
				row += fmt.Sprintf(" n/a | n/a | %s | n/a |", tput)
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

func timeCmd(bin, path string, compressed bool, runs, warmup int) []time.Duration {
	args := []string{"--no-source-map"}
	if compressed {
		args = append(args, "--style", "compressed")
	}
	args = append(args, path)
	out := make([]time.Duration, 0, runs)
	for i := 0; i < warmup+runs; i++ {
		cmd := exec.Command(bin, args...)
		cmd.Stdout = nil
		t := time.Now()
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "%s error on %s: %v\n", bin, path, err)
			os.Exit(1)
		}
		if i >= warmup {
			out = append(out, time.Since(t))
		}
	}
	return out
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
