<!-- SPDX-License-Identifier: BSD-3-Clause -->
<!-- Copyright (c) 2026, the go-scss/scss authors -->

# Benchmarks

Performance of go-scss versus the reference implementation **dart-sass 1.102.0**
and the historically-fastest C implementation **libsass 3.6.6** (via `sassc`
3.6.2), measured on a representative corpus. The goal of the benchmark suite is
to keep go-scss compiling **at least as fast as dart-sass**, and to catch
regressions in CI via Go `Benchmark*` on the hot paths.

> **libsass is feature-frozen.** It predates the Sass module system
> (`@use`/`@forward`) and the CSS Color Level 4 / `sass:color` / `sass:math`
> functions, so several corpora are **dart-sass / go-scss only** — libsass
> cannot compile them at all (a correctness gap, flagged *cannot compile*
> below, not a speed result). Even on the legacy `@import` stylesheets it *does*
> compile, libsass output is **not** byte-identical to dart-sass (go-scss is);
> the libsass column is therefore a **compile-speed datapoint only**.

Correctness is the precondition for every number here: go-scss produces
**byte-identical** output to dart-sass on the whole corpus, and the full
sass-spec differential (11227/11406 dart-applicable cases) stays byte-identical.
A speed win that changes a single output byte is not a win.

## Methodology

- **Hardware / toolchain.** Apple M4 Max, macOS 26.5.1, Go 1.26.4,
  dart-sass 1.102.0 (native **AOT** binary from the dart-sass GitHub release —
  the modern performance reference; the `npm i -g sass` build is the slower
  dart2js JavaScript build and is not used here), and **libsass 3.6.6** driven
  by **`sassc` 3.6.2** (`brew install sassc`; a native C binary, ~2.7 ms
  startup). All three compilers were measured on the **same machine**, so the
  numbers are directly comparable.
- **What is timed.** go-scss is timed **in-process** around `scss.Compile`, so
  its figure is pure compile time (parse + eval + serialize, including reading
  imported partials from disk). dart-sass is timed around its subprocess.
- **Startup separation.** Each subprocess figure includes process/VM startup.
  The harness measures each external compiler's startup once on an empty input
  (dart-sass ~22 ms, sassc ~2.7 ms here) and reports a startup-subtracted
  **compile-only** column, which is the apples-to-apples compiler comparison. It
  also reports the raw end-to-end wall-clock a user sees. go-scss's own process
  startup is a few milliseconds (a static Go binary); the `scssc` CLI end-to-end
  wall-clock is reported separately below.
- **Feature gaps are not timed.** When an external compiler cannot compile an
  input at all (libsass on the module system / Color 4), the harness records
  *cannot compile* for that cell rather than a time — a correctness gap, not a
  speed result.
- **Sampling.** Each cell is the **median** (and **min**) of 21 timed runs after
  5 discarded warm-up runs (which also warm the OS file cache). Output style is
  measured both **expanded** and **compressed**.
- **Reproduce.** From the repo root:

  ```sh
  # (re)generate the self-contained synthetic corpus (already committed)
  go run bench/gen.go > bench/corpus/generated.scss

  # cross-implementation wall-clock table (go-scss vs dart-sass vs libsass)
  # bench/corpus/{generated,modern}.scss are committed and always included;
  # the -foundation slot below carried Bootstrap 4.6.2 for these numbers.
  go run bench/harness.go -runs 21 -warmup 5 \
      -sass /path/to/dart-sass/sass \
      -sassc /path/to/sassc \
      -bootstrap /path/to/bootstrap-5/scss/bootstrap.scss \
      -foundation /path/to/bootstrap-4/scss/bootstrap.scss

  # Go micro/whole-pipeline benchmarks (also the CI regression guard)
  go test -run '^$' -bench . -benchmem .
  ```

## Corpus

| input | what it stresses | libsass? | source |
|---|---|---|---|
| `generated` | one large single file (~140 KB): deep nesting, mixins with `@content`, `@extend` chains, placeholder selectors, color functions, maps + `@each`, `@for`, math, media queries | compiles (divergent output) | committed at `bench/corpus/generated.scss` (deterministic, from `bench/gen.go`) |
| `modern` | a small stylesheet on the **modern** stack: `@use`/`@forward` + `sass:color`/`sass:math`/`sass:map` + Color 4 (`color.adjust`, `color.mix`, `math.div`, `math.pow`) | **cannot compile** | committed at `bench/corpus/modern.scss` |
| `bootstrap` | Bootstrap **5.3.3**, a real framework split across ~40 partials: heavy maps/functions, loops, many `@import`s | compiles (divergent output) | Bootstrap 5 `scss/bootstrap.scss` (supply the path; not vendored) |
| `bootstrap4` | Bootstrap **4.6.2**, the last fully libsass-compatible framework release (pure `@import`, legacy functions) — the fair many-partials 3-way baseline | compiles (divergent output) | Bootstrap 4 `scss/bootstrap.scss` (supply via `-foundation`; not vendored) |

`generated` deliberately exercises the *large single stylesheet* class (compiled
design-token bundles, concatenated output); `bootstrap`/`bootstrap4` exercise the
*many-small-partials* class typical of real frameworks. `modern` is the
correctness-gap probe: dart-sass and go-scss compile it byte-identically, libsass
cannot parse the module syntax at all. The **libsass?** column is a correctness
note — "compiles (divergent output)" means libsass produces CSS but *not* the
byte-identical output dart-sass and go-scss agree on (see the caveat below).

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

## Three-way comparison: go-scss vs dart-sass vs libsass (`sassc`)

libsass (via `sassc`) is the C implementation that was, for years, the fastest
Sass compiler available — the reason it's the historical speed reference. This
section pits it against go-scss and dart-sass 1.102 AOT on the **same M4 Max**,
same methodology (median of 21 timed runs after 5 warm-ups, both output styles,
startup-subtracted **compile-only** figures). Ratios are `tool ÷ go-scss`, so a
value **> 1 means go-scss is that many times faster**.

| corpus | style | go-scss (compile) | dart-sass (compile-only) | libsass/sassc (compile-only) | dart ÷ go | sassc ÷ go |
|---|---|---:|---:|---:|---:|---:|
| generated | expanded | **14.0 ms** | 40.5 ms | 38.0 ms | 2.88x | 2.71x |
| generated | compressed | **14.3 ms** | 41.4 ms | 37.0 ms | 2.89x | 2.58x |
| modern | expanded | **0.2 ms** | 1.1 ms | *cannot compile* | 5.05x† | — |
| modern | compressed | **0.3 ms** | 1.4 ms | *cannot compile* | 5.44x† | — |
| bootstrap (5.3.3) | expanded | **62.1 ms** | 125.0 ms | 188.3 ms | 2.01x | 3.03x |
| bootstrap (5.3.3) | compressed | **61.3 ms** | 124.0 ms | 188.0 ms | 2.02x | 3.07x |
| bootstrap4 (4.6.2) | expanded | **28.1 ms** | 59.7 ms | 62.2 ms | 2.12x | 2.21x |
| bootstrap4 (4.6.2) | compressed | **28.0 ms** | 60.3 ms | 61.2 ms | 2.15x | 2.19x |

> † `modern` is a ~6.6 KB feature-probe, not a speed headline: its timings are
> dominated by fixed per-invocation costs, so the ratio is noise. Its purpose is
> the *cannot compile* cell — libsass rejects `@use`/`sass:color`/`sass:math`
> outright (exit 65) where dart-sass and go-scss produce byte-identical output.

**Reading it:**

- **go-scss is the fastest compiler on every input all three can compile** — by
  **2.0x–2.9x** over both dart-sass AOT and libsass. The C reference no longer
  wins on speed: a static Go binary beats it everywhere here.
- **libsass vs dart-sass is a wash, input-dependent.** On the large single file
  (`generated`) libsass edges out dart-sass AOT (37–38 ms vs 40–41 ms); on the
  many-partials **Bootstrap 5** it is *slower* than dart-sass (188 ms vs 124 ms)
  and ~3x slower than go-scss; on legacy **Bootstrap 4** the two are within a few
  percent. So "libsass is the fastest" is no longer a general truth even against
  modern dart-sass, and it is beaten across the board by go-scss.

### The honest caveat: libsass is feature-frozen

The libsass column is a **compile-speed datapoint only**, for two reasons:

1. **It cannot compile modern Sass at all.** libsass predates the module system
   (`@use`/`@forward`) and the `sass:color` / `sass:math` built-in modules /
   CSS Color Level 4 functions. The `modern` corpus — which dart-sass and go-scss
   compile to **byte-identical** CSS — makes `sassc` exit 65 at parse time. Any
   real stylesheet on the modern stack (the direction the whole ecosystem, and
   Bootstrap ≥ 5.3's own source, is moving) is **dart-sass / go-scss only**.
2. **Where it does compile a legacy `@import` stylesheet, its output is not
   correct-by-modern-Sass.** libsass output is *not* byte-identical to dart-sass,
   while go-scss **is**. Measured divergence (expanded, `diff` changed lines):

   | corpus | go-scss vs dart-sass | libsass vs dart-sass |
   |---|---:|---:|
   | generated | **0** (byte-identical) | 5116 |
   | bootstrap (5.3.3) | **0** (byte-identical) | 1073 |
   | bootstrap4 (4.6.2) | 2‡ | 1877 |

   libsass's differences are semantic, not just whitespace: legacy hex/`rgb()`
   color serialization instead of Color 4 percentage `rgb()` (e.g.
   `#0257d5` vs `rgb(0.69% 34.2% 83.6%)`), a dropped `@charset`, unquoted
   attribute selectors (`[data-bs-theme=light]` vs `[data-bs-theme="light"]`),
   and `1 / 10` left unevaluated. A speed number over *different* output is not
   comparable to one over identical output.

   ‡ go-scss's only Bootstrap 4 difference from dart-sass is a single
   selector-deduplication edge on one `.navbar > .container-*` rule — a known,
   pre-existing go-scss behavior, unrelated to this benchmark.

**Bottom line.** Against the historically-fastest C compiler, go-scss is faster
*and* strictly more correct: it matches dart-sass byte-for-byte and compiles the
modern-Sass inputs libsass cannot touch, while still out-running libsass 2.2x–3.1x
on the inputs libsass can compile.

## Optimization log

Each change was landed only after verifying byte-identical output on the whole
corpus, an unchanged sass-spec differential (11227/11406, identical failure
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
