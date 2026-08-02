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
(`go test -cpuprofile`) showed a single dominant cause — see the optimization log.

## Results (after the perf campaign)

go-scss compile time (in-process median) vs dart-sass 1.102 AOT (compile-only),
same methodology as the baseline:

| corpus | style | go-scss median | go-scss min | dart-sass compile-only | throughput | ratio (dart / go) |
|---|---|---:|---:|---:|---:|---:|
| generated | expanded | **13.2 ms** | 12.7 ms | 39.7 ms | 18.9 MB/s | **3.01x faster** |
| generated | compressed | **14.1 ms** | 13.5 ms | 40.4 ms | 14.6 MB/s | **2.86x faster** |
| bootstrap | expanded | **59.5 ms** | 58.3 ms | 123.8 ms | 4.4 MB/s | **2.08x faster** |
| bootstrap | compressed | **59.4 ms** | 58.1 ms | 122.8 ms | 3.8 MB/s | **2.07x faster** |

go-scss is **faster than dart-sass 1.102 AOT on every corpus/style combination**,
by 2.07x–3.01x on pure compile time. Including startup, the end-to-end
wall-clock gap is wider still: the static `scssc` binary compiles Bootstrap in
**~70 ms end-to-end** (process start + file I/O + compile) versus dart-sass's
~145 ms — a ~2x end-to-end win — and dart-sass's slower `npm` (dart2js
JavaScript) build is several times slower again.

Throughput is generated-CSS bytes per second (well defined across the
multi-file Bootstrap input, unlike entry-file size).

## Optimization log

Each change was landed only after verifying byte-identical output on the whole
corpus, an unchanged sass-spec differential (11220/11406, identical failure
set), and 100% coverage.

1. **Parser line-number lookup: O(n²) → O(n log n)** (`internal/scss/parser.go`).
   `lineAt` counted newlines from the start of the source on every call, and it
   is called per statement/block, so line-number resolution was quadratic in the
   file length. This was **70% of CPU** on `generated`. Fixed by indexing the
   newline offsets once and binary-searching them.
   Effect (identical harness methodology): `generated` **120.7 → 13.2 ms
   (9.1x)** expanded, 121.1 → 14.1 ms compressed. Bootstrap (many small
   partials, where the per-file quadratic never bit) is unaffected. This alone
   turned the single-large-file loss into a win.

2. **Lazy per-scope tables** (`internal/scss/env.go`). Opening a scope
   eagerly allocated three maps (variables, mixins, functions); the vast
   majority of scopes — a style rule with only property declarations, a loop
   body — populate none of them. Allocate each map on first write instead, with
   `closureAt` materialising any captured-but-empty scope so lexical-closure
   sharing stays identical to the eager version. Effect: **−7.6% allocations and
   −7% peak memory on Bootstrap** (1.06M → 0.98M allocs/op; 55.5 → 51.6 MB/op),
   −2.6% allocations on `generated`. Wall-clock is essentially flat at these
   input sizes (GC was not on the critical path), so this is a memory/GC-pressure
   win rather than a latency one.

## Honest gaps

- The large single-file (`generated`) result rests on optimization #1; that
  input class is real (compiled design-token bundles, concatenated output) but
  is synthetic here.
- After the O(n²) fix, both corpora are dominated by allocation/GC rather than a
  single algorithmic hot spot; the remaining allocation cost is spread thinly
  across the evaluator. Further wins are available but would be many small,
  individually-verified changes with diminishing returns — go-scss already
  clears the dart-sass-parity bar on every measured input, so they are deferred
  over risking the correctness/coverage guarantees.
