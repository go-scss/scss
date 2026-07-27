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

Correctness is defined as **matching dart-sass byte-for-byte**. A corpus of representative `.scss`/`.sass` files (covering every feature above) is compiled with both this compiler and dart-sass and diffed. The expected CSS is frozen as golden `testdata`, so CI runs without dart-sass; a skip-gated live test re-verifies against a real `sass` binary when present.

Current corpus: **all files byte-match dart-sass 1.102 in both expanded and compressed** output.

```
go test ./...          # golden gate (no dart-sass needed)
sass --version && go test ./...   # also runs the live differential
```

## Honest residuals

Dart Sass output is the source of truth; where this compiler intentionally diverges it says so:

- **CSS Color Level 4 serialization** — dart-sass 1.102 serializes some computed colors from `mix`/`scale`/`hsl` as percentage channels (e.g. `rgb(25%, 75%, 25%)`) in the new color model. This compiler serializes computed colors as hex/`rgba()`; hex-literal, named, and `rgb()/rgba()` colors match exactly.
- **Source maps** — not yet emitted (`CompileResult.SourceMap` is empty). Named residual, not silently dropped.
- **`@extend`** — covers class/placeholder extension with dart-compatible ordering; advanced selector unification (`@extend a b`, complex `:not()` merges) is partial.
- **Media-query conflict pruning** — dart drops impossible merges (e.g. `screen and print`); this compiler emits the merged query.
- **Exotic `sass:selector` functions** and full `sass:meta` reflection (`get-function`, `call`, `module-variables`) are not yet implemented.
- **Coverage** is **79.3%** of statements; the fleet target is 100% and is being raised (the differential gate and public API are the priority).

## License

BSD-3-Clause. Copyright (c) 2026, the go-scss/scss authors.
