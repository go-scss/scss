// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import "testing"

// TestColorCoverageResiduals covers the last few color-subsystem branches that
// the broader color tests don't reach: the gamut-mapping binary-search
// fall-through, the relative-color ("from black") out-of-range serialization,
// the compressed rgb-vs-hsl length pick, and the number-unit helper edges.
func TestColorCoverageResiduals(t *testing.T) {
	// gamutLocalMinde: a highly-saturated out-of-gamut color whose chroma
	// binary-search runs to completion (falls through to `return clipped`).
	expectEq(t,
		"@use \"sass:color\";\na{b: color.to-gamut(oklch(0.5 0.4 30), $space: srgb, $method: local-minde)}",
		"a {\n  b: oklch(51.3785733485% 0.210833465 29.2338802796deg);\n}\n")

	// writeLabLike relative-color syntax: an out-of-[0,100] lightness together
	// with a missing channel (so color-mix() can't represent it). color.change
	// applies channels without clamping, which is how lightness escapes range.
	expectEq(t,
		"@use \"sass:color\";\na{b: color.change(lab(50% 40 30), $lightness: 200%, $b: none)}",
		"a {\n  b: lab(from black 200% 40 none);\n}\n")

	// Compressed legacy serialization where the hsl() form is shorter than the
	// rgb() form (fractional channels), exercising the length-comparison pick.
	if got := compileC(t, ".a{v: hsl(200, 5%, 47%)}"); got != ".a{v:hsl(200,5%,47%)}\n" {
		t.Errorf("compressed hsl pick: got %q", got)
	}

	// eval_call.require: a missing required argument.
	mustErr(t, "@use \"sass:color\";\n.a{v: color.space()}")

	// number.compatibleWithUnit: a compound-unit number is not compatible with a
	// single unit (reaches the len != 1 guard). number.coerceValueToUnit: an
	// incompatible unit falls back to the raw value (reaches the !ok guard).
	okCompile(t, "@use \"sass:color\";\n.a{v: color.adjust(red, $hue: 1px*1px)}")
	okCompile(t, ".a{v: oklch(0.5 0.1 5px)}")

	// Direct white-box checks of the two number helpers for good measure.
	if (&Number{Val: 1, Numer: []string{"px", "px"}}).compatibleWithUnit("deg") {
		t.Error("px*px should not be compatible with deg")
	}
	if v := (&Number{Val: 5, Numer: []string{"px"}}).coerceValueToUnit("deg"); v != 5 {
		t.Errorf("incompatible coerce should return raw value, got %v", v)
	}
}
