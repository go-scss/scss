<!-- SPDX-License-Identifier: BSD-3-Clause -->
<!-- Copyright (c) 2026, the go-scss/scss authors -->

# Benchmarks

Performance of go-scss versus the reference implementation **dart-sass 1.102.0**,
measured on a representative corpus. The goal of the benchmark suite is to keep
go-scss compiling **at least as fast as dart-sass**, and to catch regressions in
CI via Go `Benchmark*` on the hot paths.

Correctness is the precondition for every number here: go-scss produces
**byte-identical** output to dart-sass on the whole corpus, and the full
sass-spec differential (11220/11406 dart-applicable cases) stays byte-identical.
A speed win that changes a single output byte is not a win.

## Methodology

- **Hardware / toolchain.** Apple M4 Max, macOS 26.5.1, Go 1.26.4,
  dart-sass 1.102.0 (native **AOT** binary from the dart-sass GitHub release —
  the modern performance reference; the `npm i -g sass` build is the slower
  dart2js JavaScript build and is not used here).
- **What is timed.** go-scss is timed **in-process** around `scss.Compile`, so
  its figure is pure compile time (parse + eval + serialize, including reading
  imported partials from disk). dart-sass is timed around its subprocess.
- **Startup separation.** The dart-sass subprocess figure includes VM startup.
  The harness measures dart-sass startup once on an empty input (~22 ms here)
  and reports a startup-subtracted **compile-only** column, which is the
  apples-to-apples compiler comparison. It also reports the raw end-to-end
  wall-clock a user sees. go-scss's own process startup is a few milliseconds
  (a static Go binary); the `scssc` CLI end-to-end wall-clock is reported
  separately below.
- **Sampling.** Each cell is the **median** (and **min**) of 21 timed runs after
  5 discarded warm-up runs (which also warm the OS file cache). Output style is
  measured both **expanded** and **compressed**.
- **Reproduce.** From the repo root:

  ```sh
  # (re)generate the self-contained synthetic corpus (already committed)
  go run bench/gen.go > bench/corpus/generated.scss

  # cross-implementation wall-clock table
  go run bench/harness.go -runs 21 -warmup 5 \
      -sass /path/to/dart-sass/sass \
      -bootstrap /path/to/bootstrap/scss/bootstrap.scss

  # Go micro/whole-pipeline benchmarks (also the CI regression guard)
  go test -run '^$' -bench . -benchmem .
  ```

## Corpus

| input | what it stresses | source |
|---|---|---|
| `generated` | one large single file (~140 KB): deep nesting, mixins with `@content`, `@extend` chains, placeholder selectors, color functions, maps + `@each`, `@for`, math, media queries | committed at `bench/corpus/generated.scss` (deterministic, from `bench/gen.go`) |
| `bootstrap` | a real framework split across ~40 partials: heavy maps/functions, loops, many `@import`s | Bootstrap 5 `scss/bootstrap.scss` (supply the path; not vendored) |

`generated` deliberately exercises the *large single stylesheet* class (compiled
design-token bundles, concatenated output); `bootstrap` exercises the
*many-small-partials* class typical of real frameworks.

## Baseline (main @ 5b03f0b, before the perf campaign)

go-scss compile time (in-process median) vs dart-sass 1.102 AOT (compile-only):

| corpus | style | go-scss median | dart-sass compile-only | ratio (dart / go) |
|---|---|---:|---:|---:|
| generated | expanded | 120.7 ms | 39.7 ms | 0.33x — go-scss **slower** |
| generated | compressed | 121.1 ms | 40.4 ms | 0.33x — go-scss **slower** |
| bootstrap | expanded | 65.5 ms | 123.8 ms | 1.89x — go-scss faster |
| bootstrap | compressed | 65.6 ms | 122.8 ms | 1.87x — go-scss faster |

At baseline go-scss already beat dart-sass on the many-partials framework
(Bootstrap) but **lost by ~3x on the large single file** (`generated`). Profiling
identified the cause; see the optimization log once it lands. This document is
updated with the post-optimization numbers as each change merges.
