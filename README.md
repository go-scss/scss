# go-scss/scss

[![CI](https://github.com/go-scss/scss/actions/workflows/ci.yml/badge.svg)](https://github.com/go-scss/scss/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/go-scss/scss.svg)](https://pkg.go.dev/github.com/go-scss/scss)
[![License: BSD-3-Clause](https://img.shields.io/badge/License-BSD--3--Clause-blue.svg)](LICENSE)

A **pure-Go (CGO-free) Sass/SCSS compiler** whose output tracks **[Dart Sass](https://sass-lang.com)**, the canonical reference implementation. It compiles both the **SCSS** and the indented **`.sass`** syntaxes to CSS in the **`expanded`** and **`compressed`** output styles, matching dart-sass byte-for-byte on the common real-world surface.

No cgo, no libsass, no Node — a single static Go dependency for static-site generators, asset pipelines, and language bindings.

```go
import "github.com/go-scss/scss"

res, err := scss.CompileString(`
  $accent: #3498db;
  .btn { color: $accent; &:hover { color: darken($accent, 10%); } }
`, nil)
fmt.Println(res.CSS)
```

## API

Modeled on the modern `sass-embedded` shape:

```go
func CompileString(source string, opts *Options) (*CompileResult, error)
func Compile(path string, opts *Options) (*CompileResult, error)

type Options struct {
    Syntax    Syntax        // SyntaxSCSS | SyntaxIndented | SyntaxCSS
    Style     OutputStyle   // Expanded | Compressed
    LoadPaths []string
    Importer  Importer      // custom URL resolver (defaults to filesystem)
}
type CompileResult struct { CSS string; LoadedURLs []string; SourceMap string }
```

## Language surface

| Feature | Status |
| --- | --- |
| Variables (`$x`, `!default`, `!global`) | ✅ |
| Nesting, parent selector `&` | ✅ |
| Interpolation `#{}` | ✅ |
| `@mixin` / `@include` (defaults, keyword & rest args, `@content`, `using`) | ✅ |
| `@function` / `@return`, recursion | ✅ |
| `@if` / `@else if` / `@else`, `@each`, `@for`, `@while` | ✅ |
| Placeholders `%x` + `@extend` (incl. `!optional`) | ✅ |
| `@use` / `@forward` (namespacing, `as`, `as *`, prefix, config `with`) | ✅ |
| Legacy `@import` (Sass partials + plain-CSS passthrough) | ✅ |
| `@media` / `@supports` (nesting, bubbling, media-query merging) | ✅ |
| `@at-root` | ✅ |
| Operators: numeric+units, string, comparison, `and`/`or`/`not` | ✅ |
| Maps + lists (space/comma/slash/bracketed) | ✅ |
| `calc()` / `clamp()` / `env()` preservation | ✅ |
| Output styles `expanded` + `compressed` (byte-matched) | ✅ |
| Comments: `//` silent, `/* */` preserved | ✅ |
| Built-in modules `sass:math`, `sass:color`, `sass:string`, `sass:list`, `sass:map`, `sass:selector`, `sass:meta` (common functions) + global aliases | ✅ |

### Built-in functions covered

- **math**: `div`, `percentage`, `round`, `ceil`, `floor`, `abs`, `min`, `max`, `sqrt`, `sin`, `cos`, `tan`, `pow`, `hypot`, `log`, `clamp`, `unit`, `is-unitless`, `compatible`
- **color**: `rgb(a)`, `hsl(a)`, `red`, `green`, `blue`, `hue`, `saturation`, `lightness`, `alpha`, `mix`, `lighten`, `darken`, `saturate`, `desaturate`, `adjust-hue`, `grayscale`, `invert`, `complement`, `opacify`, `transparentize`, `adjust`, `scale`, `change`, `ie-hex-str`
- **string**: `quote`, `unquote`, `length`, `insert`, `index`, `slice`, `to-upper-case`, `to-lower-case`, `split`, `unique-id`
- **list**: `length`, `nth`, `set-nth`, `join`, `append`, `zip`, `index`, `separator`, `is-bracketed`, `slash`
- **map**: `get`, `set`, `merge`, `remove`, `has-key`, `keys`, `values`
- **selector**: `nest`, `append`, `unify`
- **meta**: `type-of`, `inspect`, `keywords`, `*-exists`
- plus the global (un-namespaced) aliases (`map-get`, `str-length`, `nth`, …).

## Differential correctness (the compat gate)

Correctness is defined as **matching dart-sass byte-for-byte**. Two oracles enforce it:

1. **sass-spec** — the canonical conformance suite ([github.com/sass/sass-spec](https://github.com/sass/sass-spec)), whose HRX archives pair each `input.scss` with the reference `output.css`. The harness scores exactly the way the official runner scores dart-sass: it honours the `options.yml` annotation system (`:ignore_for:`/`:todo:` for dart-sass are excluded, not failed), the per-impl expected-output overrides (`output-dart-sass.css`), and the `--load-path` import resolution — so the denominator is the **dart-applicable** success set. See [sass-spec conformance](#sass-spec-conformance) below for the exact audited figures. A representative, self-contained, all-passing subset of **1575 cases** is frozen under `testdata/spec` as the in-repo conformance gate, so CI needs no sass-spec checkout; the full suite (and the dart-sass ceiling oracle via `SASS_SPEC_ORACLE=…`) runs skip-gated (`SASS_SPEC_PATH=…`) and as a non-blocking CI job.
2. **Live dart-sass** — a hand-curated corpus of representative `.scss`/`.sass` files is compiled with both engines and diffed byte-for-byte (**all byte-match dart-sass 1.102** in expanded and compressed). Frozen as golden `testdata`; a skip-gated live test re-verifies against a real `sass` binary when present.

```
go test ./...                                   # golden + frozen sass-spec gate (no dart-sass needed)
sass --version && go test ./...                 # also runs the live differential
SASS_SPEC_PATH=/path/to/sass-spec go test -run TestSassSpecFull -v ./   # full suite pass rate
```

The **CSS Color 4 color-space module** is now implemented (see residuals for the named exotic cases that remain).

### sass-spec conformance

Audited 2026-08-02 against a full sass/sass-spec checkout, with **dart-sass 1.102.0** run through the *same* annotation-aware harness as the ceiling oracle (`GOWORK=off`, `SASS_SPEC_ORACLE=sass`). Over the **11406** dart-applicable success-output cases:

| | passes | of denominator |
|---|---|---|
| **go-scss** | **11220 / 11406** | **98.37%** |
| dart-sass 1.102 (achievable ceiling) | 11341 / 11406 | 99.43% |
| **go-scss as a share of the ceiling** | **11220 / 11341** | **98.93%** |

go-scss is byte-exact against dart-sass 1.102 across the entire real-world language. It even **matches the vendored fixture where current dart-sass does not on 16 cases** — stale fixtures that dart-sass 1.102 itself now fails (its last-ULP behaviour drifted; go-scss still matches the frozen expectation).

The remaining **186** go-scss misses are honestly, oracle-bucketed into three groups:

| bucket | count | why it is where it is |
|---|---:|---|
| **libm-ULP** (color/math last-bit) | ~127 | Far-out-of-gamut `color.to-space` conversions and math asymptotes (`math.tan`, `math.pow`) that differ from dart in the last 1–2 ULPs. Irreducible without CGO or breaking cross-arch determinism: pure-Go math vs dart's platform `libm`, where products are rounded separately so results stay identical across all six arches. |
| **stale-vendored** | ~49 | The vendored fixture is stale; **dart-sass 1.102 itself fails these** (oracle-proven), so they lie outside the achievable ceiling and are not closeable by go-scss. On 16 neighbouring stale cases go-scss is in fact *ahead* of dart — it still matches the frozen expectation that dart 1.102 has drifted away from. |
| **architectural** | ~10 | A small, named set of structural gaps. The largest is the per-import-clone `@extend`-store cluster (5–6 cases: `use/extend/scope/*`, `meta.load-css` `extend::shared_cssless_midstream`) — the `ExtensionStore.clone()` foundation is landed, but closing it needs combine-level clone-node separation plus ordered per-clone finalize, a documented multi-layer follow-up. The rest are `issue_2055` (single-pass extend composition-ordering) and two high-blast-radius core reworks, `issue_1786` (value-interpolation) and `blead-global` (env-scoping). |

Scored via the annotation-aware harness that reproduces the official runner's `options.yml`/per-impl-override logic; the full per-case bucket assignment is reproducible with `TestSassSpecFull` + `SASS_SPEC_ORACLE`.

## Honest residuals

Dart Sass output is the source of truth; where this compiler intentionally diverges it says so. Divergences are measured against the sass-spec suite (spec case named in parentheses):

**Closed** (now match dart-sass 1.102):

- **Legacy color serialization** — computed colors now carry floating-point channels and serialize exactly as dart does: integer-channel colors as their CSS keyword or hex (e.g. `invert(#fff)` → `black`), fractional-channel colors as `rgb()`/`rgba()` percentages (e.g. `mix(red, blue)` → `rgb(50%, 0%, 50%)`, `color.adjust(plum, $saturation: -200%)` → `rgb(74.7058823529%, …)`).
- **Non-finite numbers** — `1/0`, `math.cos(∞)` serialize as `calc(infinity)` / `calc(-infinity)` / `calc(NaN)`.
- **Hyphen/underscore-insensitive identifiers** — variables, functions, and mixins fold `_`↔`-` on both definition and lookup (`function_exists` ≡ `function-exists`).
- **`meta.feature-exists`** returns the correct booleans for known features.
- **CSS Color Level 4 color-space module** — all Color 4 spaces (`srgb`, `srgb-linear`, `display-p3`, `a98-rgb`, `prophoto-rgb`, `rec2020`, `xyz`/`xyz-d65`, `xyz-d50`, `lab`, `lch`, `oklab`, `oklch`) plus the `color()`, `lab()`/`lch()`/`oklab()`/`oklch()`/`hwb()` constructors and the `sass:color` module (`space`, `to-space`, `channel`, `is-legacy`, `is-missing`, `is-in-gamut`, `is-powerless`, `same`, `to-gamut` with `clip`/`local-minde`, plus `change`/`adjust`/`scale`/`mix`/`invert`/`complement` extended with `$space` and space-specific channels). Conversion matrices, gamma companding, Bradford D65↔D50 adaptation, OkLab/OkLCH, missing/`none`-channel carrying and powerless-channel rules match dart-sass 1.102 byte-for-byte (all products are rounded separately to keep results identical to dart across every architecture — dart never fuses multiply-add). This closed ~4400 sass-spec cases.
- **Special-number passthrough in color functions** — `calc()`/`var()`/`env()`/`attr()` and `min()`/`max()`/`clamp()` channel arguments now serialize as dart does (all `core_functions/color/**/special_functions/*` pass).
- **`sass:selector` + `@extend` unification** — `selector.extend`, `selector.is-superselector`, `selector.parse` and complex/compound selector unification are implemented; every dart-applicable `core_functions/selector/**` and `@extend` case passes except the per-import-clone scope residual noted below.
- **Modern media-query syntax** — range (`width < 100px`) and `and`/`or`/`not` logic merging/pruning match dart for every dart-applicable case; the remaining `css/media/**` fixtures are stale-vendored (dart-sass 1.102 fails them too).

**Still divergent** (named, not hidden — see the [sass-spec conformance](#sass-spec-conformance) table for exact per-bucket counts):

- **Extreme out-of-gamut colors and math asymptotes** (libm-ULP bucket, ~127) — far-out-of-gamut `color.to-space` conversions (`core_functions/color/to_space/*/out_of_range/far`) and math asymptotes (`math.tan`, `math.pow`) differ from dart in the last 1–2 ULPs: the magnitudes amplify unavoidable floating-point rounding-order differences between pure-Go math and dart's platform `libm`. Irreducible without CGO or breaking cross-arch determinism.
- **Stale-vendored fixtures** (stale-vendored bucket, ~49) — the vendored `output.css` is out of date; dart-sass 1.102 itself fails these, so they lie outside the achievable ceiling. On 16 neighbouring cases go-scss is ahead of dart 1.102.
- **Per-import-clone `@extend` scope** (architectural bucket, 5–6) — `@extend` interacting with `@use`/`@import` module boundaries (`use/extend/scope/*`, `meta.load-css` `extend::shared_cssless_midstream`). The `ExtensionStore.clone()` foundation is landed; closing the cluster needs combine-level clone-node separation plus ordered per-clone finalize — a documented multi-layer follow-up.
- **`issue_2055` extend composition-ordering** (architectural bucket, 1) — single-pass `@extend` composition applies extensions in an order that diverges from dart's staged resolution.
- **`issue_1786` / `blead-global`** (architectural bucket, 2) — two high-blast-radius core reworks: value-interpolation semantics (`issue_1786`) and global-variable env-scoping (`blead-global`).
- **Source maps** — not emitted (`CompileResult.SourceMap` is empty).
- **Coverage** is **100.0%** of statements (up from 79.3%); every parser/eval error-recovery and defensive branch is exercised, either through malformed-SCSS tests or via direct white-box drives of the defensive seams. The CI floor is **100%**.

## License

BSD-3-Clause. Copyright (c) 2026, the go-scss/scss authors.
