// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import (
	"math"
	"testing"
)

// allSpaceNames lists every CSS color-space name reachable through the public
// SCSS API (color.to-space / color()).
var allSpaceNames = []string{
	"srgb", "srgb-linear", "display-p3", "display-p3-linear",
	"a98-rgb", "prophoto-rgb", "rec2020", "xyz", "xyz-d50",
	"lab", "lch", "oklab", "oklch", "rgb", "hsl", "hwb",
}

// TestColorConversionMatrix converts a representative color between every
// ordered pair of color spaces. This exercises convertColor's whole src switch,
// convertLinear, srgbConvert, hslConvert, hwbConvert, labConvert, lchConvert,
// oklabConvert, oklchConvert, lmsConvert, xyzD50Convert and every gamma /
// transformationMatrix branch.
func TestColorConversionMatrix(t *testing.T) {
	for _, src := range allSpaceNames {
		for _, dst := range allSpaceNames {
			src, dst := src, dst
			okCompile(t, `@use "sass:color";
.a{ v: color.to-space(color.to-space(#3366cc, `+src+`), `+dst+`) }`)
		}
	}
}

// TestColorConversionEdgeChannels drives conversions with out-of-gamut,
// negative and tiny channel values to reach signOf's negative branch and the
// small-value branches of the gamma companding functions.
func TestColorConversionEdgeChannels(t *testing.T) {
	edges := []string{
		"color(prophoto-rgb 0.001 0.001 0.001)",
		"color(prophoto-rgb 0.0001 -0.0001 0.5)",
		"color(prophoto-rgb 0.9 0.9 0.9)",
		"color(display-p3 -0.5 1.5 0.2)",
		"color(a98-rgb -0.2 1.2 0.3)",
		"color(rec2020 -0.1 1.1 0.4)",
		"color(srgb -0.3 1.3 0.5)",
		"color(srgb-linear -0.2 1.2 0.6)",
		"color(xyz -0.1 0.5 1.2)",
		"color(xyz-d50 0.2 -0.3 0.9)",
		"lab(50% -60 -60)",
		"lch(50% 40 200)",
		"oklab(0.6 -0.2 -0.1)",
		"oklch(0.6 0.2 200)",
		"hsl(200 -50% 50%)",
		"hwb(200 -10% 110%)",
	}
	for _, src := range edges {
		for _, dst := range allSpaceNames {
			okCompile(t, `@use "sass:color";
.a{ v: color.to-space(`+src+`, `+dst+`) }`)
		}
	}
}

// TestColorNoneChannels converts colors carrying `none` channels through many
// spaces to reach the missing-channel branches of the converters.
func TestColorNoneChannels(t *testing.T) {
	nones := []string{
		"oklch(none 0.1 90)",
		"oklch(0.5 none 90)",
		"oklch(0.5 0.1 none)",
		"oklab(none 0.1 0.1)",
		"lab(none 20 20)",
		"lab(50% none 20)",
		"lab(50% 20 none)",
		"lch(none 20 90)",
		"lch(50% none 90)",
		"lch(50% 20 none)",
		"color(srgb none 0.5 0.5)",
		"color(srgb 0.5 none 0.5)",
		"color(srgb 0.5 0.5 none)",
		"color(xyz none none none)",
		"hsl(none 50% 50%)",
		"hwb(none 20% 30%)",
		"rgb(none 128 128)",
	}
	for _, src := range nones {
		for _, dst := range allSpaceNames {
			okCompile(t, `@use "sass:color";
.a{ v: color.to-space(`+src+`, `+dst+`) }`)
		}
	}
}

// TestColorBuiltins exercises the sass:color module functions on a variety of
// colors and spaces.
func TestColorBuiltins(t *testing.T) {
	cases := []string{
		`.a{ v: color.space(oklch(0.5 0.1 90)) }`,
		`.a{ v: color.space(red) }`,
		`.a{ v: color.is-legacy(red) }`,
		`.a{ v: color.is-legacy(oklch(0.5 0.1 90)) }`,
		`.a{ v: color.is-missing(oklch(none 0.1 90), "lightness") }`,
		`.a{ v: color.is-missing(oklch(0.5 0.1 90), "hue") }`,
		`.a{ v: color.is-missing(red, "alpha") }`,
		`.a{ v: color.is-in-gamut(red) }`,
		`.a{ v: color.is-in-gamut(oklch(0.5 0.1 90)) }`,
		`.a{ v: color.is-in-gamut(color(srgb 2 0 0)) }`,
		`.a{ v: color.is-in-gamut(red, $space: display-p3) }`,
		`.a{ v: color.is-powerless(hsl(90 0% 50%), "hue") }`,
		`.a{ v: color.is-powerless(hsl(90 50% 50%), "hue") }`,
		`.a{ v: color.is-powerless(hwb(90 60% 60%), "hue") }`,
		`.a{ v: color.is-powerless(lch(50% 0 90), "hue") }`,
		`.a{ v: color.is-powerless(oklch(0.5 0 90), "hue") }`,
		`.a{ v: color.is-powerless(red, "alpha") }`,
		`.a{ v: color.is-powerless(red, "red", $space: srgb) }`,
		`.a{ v: color.channel(oklch(0.7 0.15 180), "lightness") }`,
		`.a{ v: color.channel(oklch(0.7 0.15 180), "chroma") }`,
		`.a{ v: color.channel(oklch(0.7 0.15 180), "hue") }`,
		`.a{ v: color.channel(red, "alpha") }`,
		`.a{ v: color.channel(red, "red") }`,
		`.a{ v: color.channel(hsl(90 50% 40%), "saturation") }`,
		`.a{ v: color.channel(red, "hue", $space: hsl) }`,
		`.a{ v: color.channel(lab(50% 20 20), "a") }`,
		`.a{ v: color.same(red, red) }`,
		`.a{ v: color.same(red, blue) }`,
		`.a{ v: color.same(red, color(srgb 1 0 0)) }`,
		`.a{ v: color.same(oklch(0.5 0.1 90), color(xyz 0.2 0.2 0.2)) }`,
		`.a{ v: color.same(color(xyz 0.2 0.2 0.2), color(xyz 0.2 0.2 0.2)) }`,
		`.a{ v: color.to-gamut(color(display-p3 1 0 0), $space: srgb, $method: clip) }`,
		`.a{ v: color.to-gamut(color(display-p3 1 0 0), $space: srgb, $method: local-minde) }`,
		`.a{ v: color.to-gamut(color(display-p3 0 1 0), $method: local-minde) }`,
		`.a{ v: color.to-gamut(oklch(1.5 0.4 30), $space: srgb, $method: local-minde) }`,
		`.a{ v: color.to-gamut(oklch(-0.2 0.4 30), $space: srgb, $method: local-minde) }`,
		`.a{ v: color.to-gamut(lab(50% 0 0), $method: clip) }`,
		`.a{ v: color.to-gamut(red, $method: clip) }`,
	}
	for _, c := range cases {
		okCompile(t, `@use "sass:color";`+"\n"+c)
	}
}

// TestColorChangeAdjustScale exercises change/adjust/scale across spaces and
// channels, including alpha handling and legacy-space sniffing.
func TestColorChangeAdjustScale(t *testing.T) {
	cases := []string{
		`.a{ v: color.change(red, $red: 10, $green: 20) }`,
		`.a{ v: color.change(red, $hue: 120) }`,
		`.a{ v: color.change(red, $saturation: 50%) }`,
		`.a{ v: color.change(red, $whiteness: 10%) }`,
		`.a{ v: color.change(red, $alpha: 0.5) }`,
		`.a{ v: color.change(red, $alpha: none) }`,
		`.a{ v: color.change(red, $alpha: 50%) }`,
		`.a{ v: color.change(oklch(0.5 0.1 90), $lightness: 0.8, $space: oklch) }`,
		`.a{ v: color.change(oklch(0.5 0.1 90), $chroma: 0.2) }`,
		`.a{ v: color.change(red, $red: 5, $space: srgb) }`,
		`.a{ v: color.change(oklch(none 0.1 90), $chroma: 0.2) }`,
		`.a{ v: color.adjust(red, $red: 10) }`,
		`.a{ v: color.adjust(red, $hue: 40deg) }`,
		`.a{ v: color.adjust(hsl(90 50% 50%), $saturation: 10) }`,
		`.a{ v: color.adjust(red, $alpha: -0.3) }`,
		`.a{ v: color.adjust(red, $alpha: 20%) }`,
		`.a{ v: color.adjust(oklch(0.5 0.1 90), $lightness: 0.2) }`,
		`.a{ v: color.adjust(lab(50% 20 20), $a: 200) }`,
		`.a{ v: color.adjust(lab(50% 20 20), $a: -300) }`,
		`.a{ v: color.adjust(rgb(250 0 0), $red: 30) }`,
		`.a{ v: color.adjust(rgb(5 0 0), $red: -30) }`,
		`.a{ v: color.scale(red, $red: 50%) }`,
		`.a{ v: color.scale(red, $red: -50%) }`,
		`.a{ v: color.scale(red, $red: 0%) }`,
		`.a{ v: color.scale(red, $alpha: -40%) }`,
		`.a{ v: color.scale(rgb(255 0 0), $red: 50%) }`,
		`.a{ v: color.scale(rgb(0 0 0), $red: -50%) }`,
		`.a{ v: color.scale(oklch(0.5 0.1 90), $lightness: 20%) }`,
	}
	for _, c := range cases {
		okCompile(t, `@use "sass:color";`+"\n"+c)
	}
}

// TestColorMixInvertComplement covers mix/invert/complement/grayscale/hwb and
// interpolation across polar and non-polar spaces with every hue method.
func TestColorMixInvertComplement(t *testing.T) {
	cases := []string{
		`.a{ v: color.mix(red, blue) }`,
		`.a{ v: color.mix(red, blue, 25%) }`,
		`.a{ v: color.mix(rgba(255,0,0,0.5), blue, 25%) }`,
		`.a{ v: color.mix(red, blue, $method: hsl) }`,
		`.a{ v: color.mix(red, blue, $method: hsl longer hue) }`,
		`.a{ v: color.mix(red, blue, $method: hsl increasing hue) }`,
		`.a{ v: color.mix(red, blue, $method: hsl decreasing hue) }`,
		`.a{ v: color.mix(red, blue, $method: hsl shorter hue) }`,
		`.a{ v: color.mix(red, blue, $method: hwb) }`,
		`.a{ v: color.mix(oklch(0.5 0.1 20), oklch(0.7 0.2 300), $method: oklch) }`,
		`.a{ v: color.mix(oklch(0.5 0.1 20), oklch(0.7 0.2 300), $method: oklch longer hue) }`,
		`.a{ v: color.mix(lch(50% 30 20), lch(70% 40 300), $method: lch) }`,
		`.a{ v: color.mix(oklab(0.5 0.1 0.1), oklab(0.7 -0.1 -0.1), $method: oklab) }`,
		`.a{ v: color.mix(color(srgb 1 0 0), color(srgb 0 0 1), $method: srgb) }`,
		`.a{ v: color.mix(oklch(none 0.1 20), oklch(0.7 0.2 300), $method: oklch) }`,
		`.a{ v: color.mix(oklch(0.5 0.1 20), oklch(none 0.2 300), $method: oklch) }`,
		`.a{ v: color.mix(hsl(none 50% 50%), hsl(200 60% 40%), $method: hsl) }`,
		`.a{ v: color.mix(red, red, $method: oklch) }`,
		`.a{ v: color.mix(red, blue, 0%, $method: oklch) }`,
		`.a{ v: color.mix(red, blue, 100%, $method: oklch) }`,
		`.a{ v: color.invert(red) }`,
		`.a{ v: color.invert(red, 30%) }`,
		`.a{ v: color.invert(red, $space: oklch) }`,
		`.a{ v: color.invert(red, 0%, $space: oklch) }`,
		`.a{ v: color.invert(red, 100%, $space: oklch) }`,
		`.a{ v: color.invert(red, 50%, $space: oklch) }`,
		`.a{ v: color.invert(red, $space: hwb) }`,
		`.a{ v: color.invert(red, $space: hsl) }`,
		`.a{ v: color.invert(red, $space: lch) }`,
		`.a{ v: color.invert(color(srgb 0.2 0.4 0.6), $space: srgb) }`,
		`.a{ v: color.invert(color(display-p3 0.2 0.4 0.6), 40%, $space: display-p3) }`,
		`.a{ v: color.complement(red) }`,
		`.a{ v: color.complement(oklch(0.5 0.1 90), $space: oklch) }`,
		`.a{ v: color.complement(red, $space: hsl) }`,
		`.a{ v: color.complement(lch(50% 30 90), $space: lch) }`,
		`.a{ v: color.grayscale(red) }`,
		`.a{ v: color.grayscale(oklch(0.5 0.1 90)) }`,
		`.a{ v: color.grayscale(5) }`,
		`.a{ v: color.hwb(90 20% 30%) }`,
		`.a{ v: color.hwb(90 20% 30% / 0.5) }`,
		`.a{ v: color.hwb(90deg 20% 30%) }`,
		`.a{ v: color.ie-hex-str(red) }`,
		`.a{ v: color.ie-hex-str(rgba(1,2,3,0.5)) }`,
	}
	for _, c := range cases {
		okCompile(t, `@use "sass:color";`+"\n"+c)
	}
}

// TestColorGlobalConstruction covers the global color-constructing functions
// including none channels, slash alpha, percent alpha and special values.
func TestColorGlobalConstruction(t *testing.T) {
	cases := []string{
		`.a{ v: oklch(0.7 0.15 180) }`,
		`.a{ v: oklch(0.7 0.15 180 / 0.5) }`,
		`.a{ v: oklab(0.7 0.1 -0.1) }`,
		`.a{ v: lab(50% 40 30) }`,
		`.a{ v: lch(50% 40 30) }`,
		`.a{ v: hwb(90 20% 30%) }`,
		`.a{ v: color(srgb 1 0 0) }`,
		`.a{ v: color(srgb 1 0 0 / 0.5) }`,
		`.a{ v: color(xyz 0.2 0.3 0.4) }`,
		`.a{ v: rgb(1 2 3) }`,
		`.a{ v: rgb(1 2 3 / 0.5) }`,
		`.a{ v: rgb(1, 2, 3) }`,
		`.a{ v: rgba(1, 2, 3, 0.5) }`,
		`.a{ v: rgb(50% 20% 30%) }`,
		`.a{ v: hsl(90 50% 40%) }`,
		`.a{ v: hsl(90 50% 40% / 0.5) }`,
		`.a{ v: hsla(90, 50%, 40%, 0.5) }`,
		`.a{ v: oklch(none 0.15 180) }`,
		`.a{ v: oklch(0.7 none 180) }`,
		`.a{ v: oklch(0.7 0.15 none) }`,
		`.a{ v: oklch(0.7 0.15 180 / none) }`,
		`.a{ v: rgb(1 2 3 / calc(0.2 + 0.3)) }`,
		`.a{ v: rgb(none 2 3) }`,
		`.a{ v: rgba(1, 2, 3, none) }`,
		`.a{ v: hsl(none 50% 40%) }`,
		`.a{ v: rgb(red, 0.5) }`,
		`.a{ v: hsl(red, 0.5) }`,
		`.a{ v: rgb(10% 20% 30%) }`,
		`.a{ v: oklch(calc(0.5 + 0.2) 0.15 180) }`,
		`.a{ v: oklch(var(--l) 0.15 180) }`,
		`.a{ v: oklch(env(--l) 0.15 180) }`,
		`.a{ v: oklch(min(0.5, 0.7) 0.15 180) }`,
		`.a{ v: oklch(max(0.5, 0.7) 0.15 180) }`,
		`.a{ v: oklch(clamp(0, 0.5, 1) 0.15 180) }`,
		`.a{ v: oklch(attr(data-l) 0.15 180) }`,
		`.a{ v: rgb(calc(1 + 1) 2 3) }`,
		`.a{ v: rgba(1, 2, 3, calc(0.2 + 0.3)) }`,
		`.a{ v: color(srgb calc(0.2 + 0.3) 0 0) }`,
		`.a{ v: hwb(calc(90deg) 20% 30%) }`,
		`.a{ v: oklch(from red l c h) }`,
		`.a{ v: color(from red srgb r g b) }`,
	}
	for _, c := range cases {
		okCompile(t, `@use "sass:color";`+"\n"+c)
	}
}

// TestColorSerialization covers the color serializer's branches in both
// expanded and compressed modes.
func TestColorSerialization(t *testing.T) {
	cases := []string{
		`.a{ v: red }`,
		`.a{ v: #abc }`,
		`.a{ v: #aabbcc }`,
		`.a{ v: #123456 }`,
		`.a{ v: rgb(1 2 3) }`,
		`.a{ v: rgba(1, 2, 3, 0.5) }`,
		`.a{ v: hsl(90 50% 40%) }`,
		`.a{ v: hsla(90, 50%, 40%, 0.5) }`,
		`.a{ v: hwb(90 20% 30%) }`,
		`.a{ v: hsl(0 200% 50%) }`,
		`.a{ v: rgb(none 128 128) }`,
		`.a{ v: hsl(none 50% 40%) }`,
		`.a{ v: hwb(none 20% 30%) }`,
		`.a{ v: rgba(10.5, 20, 30, 0.5) }`,
		`.a{ v: hsla(5, 100%, 50%, 0.5) }`,
		`.a{ v: transparent }`,
		`.a{ v: rgba(255, 0, 0, 0) }`,
		`.a{ v: oklch(0.7 0.15 180) }`,
		`.a{ v: oklab(0.7 0.1 -0.1) }`,
		`.a{ v: lab(50% 40 30) }`,
		`.a{ v: lch(50% 40 30) }`,
		`.a{ v: color(srgb 1 0 0) }`,
		`.a{ v: color(display-p3 1 0 0) }`,
		`.a{ v: color(srgb 1 0 0 / 0.5) }`,
		`.a{ v: color(srgb 1 0 0 / none) }`,
		`.a{ v: oklch(0.5 0.1 180 / none) }`,
		`.a{ v: oklch(2 0.5 180) }`,
		`.a{ v: lab(200% 40 30) }`,
		`.a{ v: lch(200% 40 30) }`,
		`.a{ v: oklab(2 0.1 0.1) }`,
		`.a{ v: lab(200% none 30) }`,
		`.a{ v: oklch(15000% 0.5 none) }`,
		`.a{ v: oklch(0.5 0.1 180 / 0.5) }`,
	}
	for _, c := range cases {
		okCompile(t, `@use "sass:color";`+"\n"+c)
		compileC(t, `@use "sass:color";`+"\n"+c)
	}
}

// TestColorInspect covers inspect-only serialization branches (writeHwb and
// none/precision handling).
func TestColorInspect(t *testing.T) {
	cases := []string{
		`@use "sass:meta"; .a{ v: meta.inspect(hwb(90 20% 30%)) }`,
		`@use "sass:meta"; .a{ v: meta.inspect(hwb(90 20% 30% / 0.5)) }`,
		`@use "sass:meta"; .a{ v: meta.inspect(oklch(0.7 0.15 180)) }`,
		`@use "sass:meta"; .a{ v: meta.inspect(oklch(2 0.5 180)) }`,
		`@use "sass:meta"; .a{ v: meta.inspect(lab(200% 40 30)) }`,
		`@use "sass:meta"; .a{ v: meta.inspect(rgb(none 1 2)) }`,
		`@use "sass:meta"; .a{ v: meta.inspect(color(srgb 1 0 0)) }`,
		`@use "sass:meta"; .a{ v: meta.inspect(red) }`,
	}
	for _, c := range cases {
		okCompile(t, c)
	}
}

// TestColorErrors covers the e.fail / panic error branches.
func TestColorErrors(t *testing.T) {
	errs := []string{
		`.a{ v: color(unknownspace 1 2 3) }`,
		`.a{ v: color(rgb 1 2 3) }`,
		`.a{ v: color(srgb 1 2) }`,
		`.a{ v: color(srgb 1 2 red) }`,
		`.a{ v: color(srgb 1 2 3 / 4 / 5) }`,
		`.a{ v: color.channel(red, "bogus") }`,
		`.a{ v: color.is-missing(red, "bogus") }`,
		`.a{ v: color.is-powerless(red, "bogus") }`,
		`.a{ v: color.to-gamut(red) }`,
		`.a{ v: color.to-gamut(red, $method: bogus) }`,
		`.a{ v: color.to-gamut(red, $method: 5) }`,
		`.a{ v: color.scale(oklch(0.5 0.1 90), $hue: 10%) }`,
		`.a{ v: color.adjust(oklch(none 0.1 90), $lightness: 10%) }`,
		`.a{ v: color.scale(oklch(none 0.1 90), $lightness: 10%) }`,
		`.a{ v: color.adjust(hwb(0 0% 0%), $whiteness: 10) }`,
		`.a{ v: color.space(5) }`,
		`.a{ v: color.to-space(5, srgb) }`,
		`.a{ v: color.to-space(red, 5) }`,
		`.a{ v: color.to-space(red, unknownxyz) }`,
		`.a{ v: color.channel(red, red) }`,
		`.a{ v: color.change(red, $bogus: 5) }`,
		`.a{ v: color.complement(hsl(none 50% 50%)) }`,
		`.a{ v: color.complement(red, $space: srgb) }`,
		`.a{ v: color.invert(oklch(none 0.1 90), $space: oklch) }`,
		`.a{ v: rgb(1 2 3 / 5px) }`,
		`.a{ v: oklch("x" 0.1 90) }`,
		`.a{ v: color.mix(red, oklch(0.5 0.1 90)) }`,
		`.a{ v: color.mix(oklch(0.5 0.1 90), red) }`,
	}
	for _, s := range errs {
		mustErr(t, `@use "sass:color";`+"\n"+s)
	}
}

// TestColorCoverageGaps closes the remaining branch gaps identified from the
// coverage profile.
func TestColorCoverageGaps(t *testing.T) {
	ok := []string{
		// angleValue non-deg unit path.
		`.a{ v: color.adjust(hsl(90 50% 50%), $hue: 1grad) }`,
		`.a{ v: color.adjust(hsl(90 50% 50%), $hue: 40px) }`,
		// forcePercent: nil (none) and unitless-wrap paths.
		`.a{ v: color.change(hsl(90 50% 50%), $saturation: 60) }`,
		`.a{ v: color.change(hsl(90 50% 50%), $saturation: none) }`,
		// fnWhiteness / fnBlackness.
		`.a{ v: color.whiteness(red) }`,
		`.a{ v: color.blackness(red) }`,
		// fnHWB 3-positional and 4-positional (alpha) forms.
		`.a{ v: color.hwb(90, 20%, 30%) }`,
		`.a{ v: color.hwb(90, 20%, 30%, 0.5) }`,
		// rgbLike special-value passthrough (three-arg form).
		`.a{ v: rgb(var(--r), 2, 3) }`,
		`.a{ v: rgb(1, 2, 3, var(--a)) }`,
		// channelForChange: none arg.
		`.a{ v: color.change(red, $red: none) }`,
		// scaleChannel: unitless factor path.
		`.a{ v: color.scale(red, $green: 40) }`,
		// adjustChannel: clamp with old value already beyond bound.
		`.a{ v: color.adjust(color.change(red, $red: 400), $red: 10) }`,
		`.a{ v: color.adjust(color.change(red, $red: -50), $red: -10) }`,
		// fnInvert number passthrough.
		`.a{ v: color.invert(5) }`,
		// invertChannel min<0 (lab/oklab a-channel) path.
		`.a{ v: color.invert(oklab(0.5 0.1 0.1), $space: oklab) }`,
		`.a{ v: color.invert(lab(50% 20 20), $space: lab) }`,
		// fnSaturate 1-arg CSS passthrough.
		`.a{ v: saturate(50%) }`,
		// interpolate: both colors missing alpha / both missing a channel.
		`.a{ v: color.mix(color.change(red, $alpha: none), color.change(blue, $alpha: none), $method: oklch) }`,
		`.a{ v: color.mix(oklch(none 0.1 90), oklch(none 0.2 200), $method: oklch) }`,
		`.a{ v: color.mix(oklch(0.5 0.1 none), oklch(0.7 0.2 none), $method: oklch) }`,
		`.a{ v: color.mix(hsl(none 50% 50%), hsl(none 60% 40%), $method: hsl) }`,
		// isAnalogousChannelMissing: analogous missing channel across spaces.
		`.a{ v: color.mix(oklch(0.5 none 90), red, $method: hsl) }`,
		`.a{ v: color.mix(oklch(none 0.1 90), red, $method: lch) }`,
		// gamutClip: polar channel passthrough and missing channel.
		`.a{ v: color.to-gamut(hsl(400 200% 50%), $method: clip) }`,
		`.a{ v: color.to-gamut(color(srgb none 2 0.5), $space: srgb, $method: clip) }`,
		// gamutLocalMinde: legacy lightness>=1, slight out-of-gamut early return.
		`.a{ v: color.to-gamut(color.change(red, $red: 400, $green: 400, $blue: 400), $method: local-minde) }`,
		`.a{ v: color.to-gamut(color(srgb 1.001 0 0), $space: srgb, $method: local-minde) }`,
		`.a{ v: color.to-gamut(color(display-p3 1 1 0), $space: srgb, $method: local-minde) }`,
		`.a{ v: color.to-gamut(oklch(0.7 0.4 30), $space: srgb, $method: local-minde) }`,
		`.a{ v: color.to-gamut(color(prophoto-rgb 0 0 1), $space: srgb, $method: local-minde) }`,
		`.a{ v: color.to-gamut(color(prophoto-rgb 1 0 0), $space: srgb, $method: local-minde) }`,
		`.a{ v: color.to-gamut(color(rec2020 0 1 0), $space: srgb, $method: local-minde) }`,
		`.a{ v: color.to-gamut(lab(50% 120 -120), $space: srgb, $method: local-minde) }`,
		`.a{ v: color.to-gamut(oklch(0.5 0.37 0), $space: srgb, $method: local-minde) }`,
		// Binary-search fall-through (loop exits by width, returns clipped).
		`.a{ v: color.to-gamut(oklch(0.3 0.2 0), $space: srgb, $method: local-minde) }`,
		// updateComponents: legacy hwb sniff + space keyword.
		`.a{ v: color.change(red, $whiteness: 10%, $blackness: 20%) }`,
		`.a{ v: color.change(red, $lightness: 40%, $space: hsl) }`,
	}
	for _, c := range ok {
		okCompile(t, `@use "sass:color";`+"\n"+c)
	}

	// equals (==) branches.
	eqs := []string{
		`.a{ v: red == oklch(0.5 0.1 90) }`,
		`.a{ v: red == hsl(0 100% 50%) }`,
		`.a{ v: oklch(0.5 0.1 90) == lab(50% 0 0) }`,
		`.a{ v: oklch(none 0.1 90) == oklch(none 0.1 90) }`,
		`.a{ v: oklch(0.5 0.1 90) == oklch(0.5 0.1 90) }`,
		`.a{ v: red == blue }`,
		`.a{ v: red == 5 }`,
	}
	for _, c := range eqs {
		okCompile(t, `@use "sass:color";`+"\n"+c)
	}

	errs := []string{
		`.a{ v: rgb(1, 2) }`,
		`.a{ v: color(1 2 3) }`,
		`.a{ v: color(()) }`,
	}
	for _, c := range errs {
		mustErr(t, `@use "sass:color";`+"\n"+c)
	}

	// Slash list with >2 elements as a whole color input.
	mustErr(t, `@use "sass:color"; @use "sass:list";
.a{ v: oklch(list.slash(0.5, 0.1, 90)) }`)
}

// TestColorPureHelpers calls internal helpers directly to reach branches that
// the SCSS surface cannot (or cannot reliably) hit.
func TestColorPureHelpers(t *testing.T) {
	// dot3
	if got := dot3(1, 2, 3, 4, 5, 6); got != 2+12+30 {
		t.Errorf("dot3 = %v", got)
	}
	// signOf: positive, negative, zero
	if signOf(2) != 1 || signOf(-2) != -1 || signOf(0) != 0 {
		t.Errorf("signOf wrong")
	}
	// fuzzyGreaterThan: greater, equal, less
	if !fuzzyGreaterThan(2, 1) || fuzzyGreaterThan(1, 1) || fuzzyGreaterThan(0, 1) {
		t.Errorf("fuzzyGreaterThan wrong")
	}
	// mapP/divP/mulP nil and non-nil
	if mapP(nil, math.Abs) != nil || divP(nil, 2) != nil || mulP(nil, 2) != nil {
		t.Errorf("nullable helpers should pass nil through")
	}
	if *mapP(pf(-3), math.Abs) != 3 || *divP(pf(6), 2) != 3 || *mulP(pf(3), 2) != 6 {
		t.Errorf("nullable helpers value wrong")
	}
	// normalizeHueP nil + non-nil
	if normalizeHueP(nil, false) != nil {
		t.Errorf("normalizeHueP(nil) should be nil")
	}
	if v := normalizeHueP(pf(-90), true); v == nil {
		t.Errorf("normalizeHueP non-nil")
	}
	// alphaP missing + present
	miss := newColorRaw(spaceSRGB, pf(0), pf(0), pf(0), nil, fmtNone, "")
	if miss.alphaP() != nil {
		t.Errorf("alphaP should be nil when missing")
	}
	pres := newColorRaw(spaceSRGB, pf(0), pf(0), pf(0), pf(0.5), fmtNone, "")
	if pres.alphaP() == nil || *pres.alphaP() != 0.5 {
		t.Errorf("alphaP present wrong")
	}
	// chanVal 0,1,2
	c := newColorRaw(spaceSRGB, pf(1), pf(2), pf(3), pf(1), fmtNone, "")
	if chanVal(c, 0) != 1 || chanVal(c, 1) != 2 || chanVal(c, 2) != 3 {
		t.Errorf("chanVal wrong")
	}
	// channelsAnalogous: matching and non-matching groups
	rgb := spaceChannels(spaceRGB)
	xyz := spaceChannels(spaceXYZD65)
	if !channelsAnalogous(rgb[0], xyz[0]) { // red <-> x
		t.Errorf("red/x should be analogous")
	}
	if channelsAnalogous(rgb[0], rgb[1]) { // red vs green
		t.Errorf("red/green should not be analogous")
	}
	hslCh := spaceChannels(spaceHSL)
	lchCh := spaceChannels(spaceLCH)
	if !channelsAnalogous(hslCh[1], lchCh[1]) { // saturation <-> chroma
		t.Errorf("saturation/chroma should be analogous")
	}
	if channelsAnalogous(hueChannelInfo, rgb[0]) {
		t.Errorf("hue/red should not be analogous")
	}
	if !channelsAnalogous(hueChannelInfo, hueChannelInfo) {
		t.Errorf("hue/hue should be analogous")
	}
	lab := spaceChannels(spaceLab)
	if !channelsAnalogous(lab[0], lchCh[0]) { // lightness <-> lightness
		t.Errorf("lightness/lightness should be analogous")
	}
	if channelsAnalogous(lab[1], lab[2]) { // a vs b -> group ""
		t.Errorf("a/b should not be analogous")
	}

	// interpolateHues: all four methods with wrap-around cases
	interpolateHues(10, 300, "longer", 0.5)     // d in (0,180)? d=290 not; else branch
	interpolateHues(10, 100, "longer", 0.5)     // d in (0,180)
	interpolateHues(100, 50, "longer", 0.5)     // d in (-180,0]
	interpolateHues(300, 10, "longer", 0.5)     // d negative large
	interpolateHues(10, 300, "increasing", 0.5) // hue2>hue1
	interpolateHues(300, 10, "increasing", 0.5) // hue2<hue1 -> +360
	interpolateHues(10, 300, "decreasing", 0.5) // hue1<hue2 -> +360
	interpolateHues(300, 10, "decreasing", 0.5) // hue1>hue2
	interpolateHues(10, 300, "shorter", 0.5)    // d>180
	interpolateHues(300, 10, "shorter", 0.5)    // d<-180
	interpolateHues(10, 100, "shorter", 0.5)    // small d

	// toXyzNoMissing: xyz-d65 fast path and general path
	xyzColor := newColorRaw(spaceXYZD65, pf(0.2), pf(0.2), pf(0.2), pf(1), fmtNone, "")
	if got := toXyzNoMissing(xyzColor); got.space != spaceXYZD65 {
		t.Errorf("toXyzNoMissing xyz fast path")
	}
	srgbColor := newColorRaw(spaceSRGB, pf(0.5), pf(0.5), pf(0.5), pf(1), fmtNone, "")
	if got := toXyzNoMissing(srgbColor); got.space != spaceXYZD65 {
		t.Errorf("toXyzNoMissing general path")
	}

	// deltaEOK
	_ = deltaEOK(srgbColor, xyzColor)

	// spaceToLinear / spaceFromLinear: every case including rgb & default
	toLinearSpaces := []ColorSpace{spaceRGB, spaceSRGB, spaceDisplayP3, spaceA98RGB, spaceRec2020, spaceProphotoRGB, spaceSRGBLinear, spaceXYZD65, spaceLMS}
	for _, s := range toLinearSpaces {
		_ = spaceToLinear(s, 0.5)
		_ = spaceToLinear(s, -0.5)
		_ = spaceFromLinear(s, 0.5)
		_ = spaceFromLinear(s, -0.5)
	}
	// prophoto small-value branches explicitly
	_ = prophotoToLinear(0.001)
	_ = prophotoToLinear(-0.001)
	_ = prophotoFromLinear(0.001)
	_ = prophotoFromLinear(-0.001)
	_ = srgbToLinear(0.001)
	_ = srgbFromLinear(0.001)

	// colorSpaceFromName: every name plus xyz-d65 alias and unknown
	names := []string{"rgb", "hwb", "hsl", "srgb", "srgb-linear", "display-p3",
		"display-p3-linear", "a98-rgb", "prophoto-rgb", "rec2020", "xyz", "xyz-d65",
		"xyz-d50", "lab", "lch", "oklab", "oklch", "OKLCH", "nope"}
	for _, n := range names {
		_, _ = colorSpaceFromName(n)
	}

	// spaceChannels: every space including lms
	for _, s := range []ColorSpace{spaceRGB, spaceHSL, spaceHWB, spaceLab, spaceLCH,
		spaceOklab, spaceOklch, spaceXYZD65, spaceXYZD50, spaceLMS, spaceSRGB} {
		_ = spaceChannels(s)
	}
	// spaceIsBounded both branches
	if spaceIsBounded(spaceLab) || !spaceIsBounded(spaceSRGB) {
		t.Errorf("spaceIsBounded wrong")
	}
}

// TestTransformationMatrixAll exercises every source/dest pair of
// transformationMatrix and its panic fallback.
func TestTransformationMatrixAll(t *testing.T) {
	srcs := []ColorSpace{spaceSRGB, spaceDisplayP3, spaceA98RGB, spaceRec2020,
		spaceProphotoRGB, spaceXYZD65, spaceXYZD50, spaceLMS}
	dests := []ColorSpace{spaceSRGBLinear, spaceSRGB, spaceRGB, spaceDisplayP3,
		spaceDisplayP3Lin, spaceA98RGB, spaceProphotoRGB, spaceRec2020,
		spaceXYZD65, spaceXYZD50, spaceLMS}
	for _, s := range srcs {
		for _, d := range dests {
			if s == d {
				continue
			}
			func() {
				defer func() { _ = recover() }()
				_ = transformationMatrix(s, d)
			}()
		}
	}
	// panic fallback: no matrix from lab to rgb.
	func() {
		defer func() {
			if recover() == nil {
				t.Errorf("expected panic for missing transformation matrix")
			}
		}()
		_ = transformationMatrix(spaceLab, spaceRGB)
	}()
}

// TestColorSerializeDirect exercises serializer branches that need hand-built
// colors (non-finite channels, negative chroma color-mix form).
func TestColorSerializeDirect(t *testing.T) {
	// Infinite / NaN channels -> numberCalcRepr, unitNumer nil and non-nil.
	infSrgb := newColorRaw(spaceSRGB, pf(math.Inf(1)), pf(0), pf(0), pf(1), fmtNone, "")
	_ = serializeColor(infSrgb, false, false)
	nanSrgb := newColorRaw(spaceSRGB, pf(math.NaN()), pf(0), pf(0), pf(1), fmtNone, "")
	_ = serializeColor(nanSrgb, false, false)
	// HSL with a missing channel and an infinite hue -> unitNumer("deg").
	infHsl := newColorRaw(spaceHSL, pf(math.Inf(1)), pf(50), pf(50), nil, fmtNone, "")
	_ = serializeColor(infHsl, false, false)
	// Negative-chroma oklch (constructed raw) -> isColorMixCase c1<0 return true.
	negChroma := newColorRaw(spaceOklch, pf(0.5), pf(-0.1), pf(180), pf(1), fmtNone, "")
	_ = serializeColor(negChroma, false, false)
	_ = serializeColor(negChroma, true, false)
	negChromaLch := newColorRaw(spaceLCH, pf(50), pf(-10), pf(180), pf(1), fmtNone, "")
	_ = serializeColor(negChromaLch, false, false)
	// Missing-alpha serialization -> maybeWriteSlashAlpha writes "none".
	noneAlpha := newColorRaw(spaceOklch, pf(0.5), pf(0.1), pf(180), nil, fmtNone, "")
	_ = serializeColor(noneAlpha, false, false)
	_ = serializeColor(noneAlpha, true, false)
	noneAlphaColorFn := newColorRaw(spaceSRGB, pf(0.5), pf(0.1), pf(0.2), nil, fmtNone, "")
	_ = serializeColor(noneAlphaColorFn, false, false)
	// writeHwb via inspect on an hwb color (opaque and translucent).
	hwbC := newColorRaw(spaceHWB, pf(120), pf(10), pf(20), pf(1), fmtNone, "")
	_ = serializeColor(hwbC, false, true)
	hwbA := newColorRaw(spaceHWB, pf(120), pf(10), pf(20), pf(0.5), fmtNone, "")
	_ = serializeColor(hwbA, false, true)
	// Lab-like color with out-of-range lightness and a missing channel: not the
	// color-mix case (m1 set), so writeLabLike takes its "from black/red"
	// relative-syntax branch in both expanded and compressed modes.
	labFrom := newColorRaw(spaceLab, pf(200), nil, pf(30), pf(1), fmtNone, "")
	_ = serializeColor(labFrom, false, false)
	_ = serializeColor(labFrom, true, false)
}

// TestColorConvertDirect reaches converter branches unreachable from SCSS
// because the LMS space and raw out-of-normalization values are internal-only.
func TestColorConvertDirect(t *testing.T) {
	// convertColor with an LMS source.
	for _, dst := range []ColorSpace{spaceSRGB, spaceOklab, spaceOklch, spaceXYZD65} {
		_ = convertColor(spaceLMS, [3]*float64{pf(0.5), pf(0.4), pf(0.3)}, pf(1), dst)
	}
	// hwbConvert with whiteness+blackness > 1 (normalization path).
	_ = hwbConvert(spaceSRGB, pf(90), pf(80), pf(80), pf(1))
	// hwbConvert with missing hue.
	_ = hwbConvert(spaceSRGB, nil, pf(20), pf(30), pf(1))
}
