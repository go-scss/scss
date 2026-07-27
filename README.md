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

1. **sass-spec** — the canonical conformance suite ([github.com/sass/sass-spec](https://github.com/sass/sass-spec)), whose HRX archives pair each `input.scss` with the reference `output.css`. A differential harness runs every success case and reports the pass rate. Current rate: **7520 / 11406 success cases (65.9%)**. A representative, self-contained, all-passing subset of **551 cases** is frozen under `testdata/spec` as the in-repo conformance gate, so CI needs no sass-spec checkout; the full suite runs skip-gated (`SASS_SPEC_PATH=…`) and as a non-blocking CI job.
2. **Live dart-sass** — a hand-curated corpus of representative `.scss`/`.sass` files is compiled with both engines and diffed byte-for-byte (**all byte-match dart-sass 1.102** in expanded and compressed). Frozen as golden `testdata`; a skip-gated live test re-verifies against a real `sass` binary when present.

```
go test ./...                                   # golden + frozen sass-spec gate (no dart-sass needed)
sass --version && go test ./...                 # also runs the live differential
SASS_SPEC_PATH=/path/to/sass-spec go test -run TestSassSpecFull -v ./   # full suite pass rate
```

The **CSS Color 4 color-space module** is now implemented (see residuals for the named exotic cases that remain).

## Honest residuals

Dart Sass output is the source of truth; where this compiler intentionally diverges it says so. Divergences are measured against the sass-spec suite (spec case named in parentheses):

**Closed** (now match dart-sass 1.102):

- **Legacy color serialization** — computed colors now carry floating-point channels and serialize exactly as dart does: integer-channel colors as their CSS keyword or hex (e.g. `invert(#fff)` → `black`), fractional-channel colors as `rgb()`/`rgba()` percentages (e.g. `mix(red, blue)` → `rgb(50%, 0%, 50%)`, `color.adjust(plum, $saturation: -200%)` → `rgb(74.7058823529%, …)`).
- **Non-finite numbers** — `1/0`, `math.cos(∞)` serialize as `calc(infinity)` / `calc(-infinity)` / `calc(NaN)`.
- **Hyphen/underscore-insensitive identifiers** — variables, functions, and mixins fold `_`↔`-` on both definition and lookup (`function_exists` ≡ `function-exists`).
- **`meta.feature-exists`** returns the correct booleans for known features.
- **CSS Color Level 4 color-space module** — all Color 4 spaces (`srgb`, `srgb-linear`, `display-p3`, `a98-rgb`, `prophoto-rgb`, `rec2020`, `xyz`/`xyz-d65`, `xyz-d50`, `lab`, `lch`, `oklab`, `oklch`) plus the `color()`, `lab()`/`lch()`/`oklab()`/`oklch()`/`hwb()` constructors and the `sass:color` module (`space`, `to-space`, `channel`, `is-legacy`, `is-missing`, `is-in-gamut`, `is-powerless`, `same`, `to-gamut` with `clip`/`local-minde`, plus `change`/`adjust`/`scale`/`mix`/`invert`/`complement` extended with `$space` and space-specific channels). Conversion matrices, gamma companding, Bradford D65↔D50 adaptation, OkLab/OkLCH, missing/`none`-channel carrying and powerless-channel rules match dart-sass 1.102 byte-for-byte (all products are rounded separately to keep results identical to dart across every architecture — dart never fuses multiply-add). This closed ~4400 sass-spec cases.

**Still divergent** (named, not hidden):

- **Special-number passthrough in color functions** — `calc()`/`var()`/`env()`/`attr()` channel arguments are preserved verbatim, but `min()`/`max()`/`clamp()` (and the `attr()` parse) still evaluate as Sass functions rather than CSS calculations, so a subset of `core_functions/color/**/special_functions/*` (min/max/clamp/attr) diverges. Needs the CSS calculation value type.
- **Extreme out-of-gamut colors** — conversions of colors whose channels are far outside their gamut (e.g. `core_functions/color/to_space/*/out_of_range/far`) can differ from dart in the last 1–2 ULPs because the magnitudes amplify unavoidable floating-point rounding order differences; and a handful of achromatic extremes (`lab`/`oklab` white via HSL) can differ by one ULP.
- **Source maps** — not emitted (`CompileResult.SourceMap` is empty).
- **`@extend` / `sass:selector` unification** — class/placeholder extension with dart-compatible ordering works; `selector.extend`, `selector.is-superselector`, `selector.parse`, and complex/compound selector unification are not implemented (`core_functions/selector/extend/*`).
- **Modern media-query syntax** — range (`width < 100px`) and `and`/`or`/`not` logic merging/pruning are partial (`css/media/logic/*`, `css/media/range/*`).
- **Parser edge cases** — interpolation with an adjacent literal suffix in a declaration value (`prop: val-#{$x}`) and a top-level `@return` are not yet handled gracefully.
- **Coverage** is **100.0%** of statements (up from 79.3%); every parser/eval error-recovery and defensive branch is exercised, either through malformed-SCSS tests or via direct white-box drives of the defensive seams. The CI floor is **100%**.

## License

BSD-3-Clause. Copyright (c) 2026, the go-scss/scss authors.
