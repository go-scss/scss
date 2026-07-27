// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import "math"

// ColorSpace identifies a CSS Color 4 color space by its canonical Sass name.
type ColorSpace string

const (
	spaceRGB          ColorSpace = "rgb"
	spaceHSL          ColorSpace = "hsl"
	spaceHWB          ColorSpace = "hwb"
	spaceSRGB         ColorSpace = "srgb"
	spaceSRGBLinear   ColorSpace = "srgb-linear"
	spaceDisplayP3    ColorSpace = "display-p3"
	spaceDisplayP3Lin ColorSpace = "display-p3-linear"
	spaceA98RGB       ColorSpace = "a98-rgb"
	spaceProphotoRGB  ColorSpace = "prophoto-rgb"
	spaceRec2020      ColorSpace = "rec2020"
	spaceXYZD65       ColorSpace = "xyz"
	spaceXYZD50       ColorSpace = "xyz-d50"
	spaceLab          ColorSpace = "lab"
	spaceLCH          ColorSpace = "lch"
	spaceOklab        ColorSpace = "oklab"
	spaceOklch        ColorSpace = "oklch"
	spaceLMS          ColorSpace = "lms"
)

// channelInfo is metadata about a single color channel.
type channelInfo struct {
	name            string
	polar           bool
	min, max        float64
	requiresPercent bool
	lowerClamped    bool
	upperClamped    bool
	assocUnit       string // "%", "deg", or ""
}

func linChan(name string, min, max float64, requiresPercent, lowerClamped, upperClamped, conventionallyPercent bool) channelInfo {
	assoc := ""
	if conventionallyPercent || (min == 0 && max == 100) {
		assoc = "%"
	}
	return channelInfo{name: name, min: min, max: max, requiresPercent: requiresPercent,
		lowerClamped: lowerClamped, upperClamped: upperClamped, assocUnit: assoc}
}

var hueChannelInfo = channelInfo{name: "hue", polar: true, assocUnit: "deg"}

var rgbUnitChannels = [3]channelInfo{
	linChan("red", 0, 1, false, false, false, false),
	linChan("green", 0, 1, false, false, false, false),
	linChan("blue", 0, 1, false, false, false, false),
}

var xyzUnitChannels = [3]channelInfo{
	linChan("x", 0, 1, false, false, false, false),
	linChan("y", 0, 1, false, false, false, false),
	linChan("z", 0, 1, false, false, false, false),
}

// spaceChannels returns the three channel definitions for a space.
func spaceChannels(s ColorSpace) [3]channelInfo {
	switch s {
	case spaceRGB:
		return [3]channelInfo{
			linChan("red", 0, 255, false, true, true, false),
			linChan("green", 0, 255, false, true, true, false),
			linChan("blue", 0, 255, false, true, true, false),
		}
	case spaceHSL:
		return [3]channelInfo{
			hueChannelInfo,
			linChan("saturation", 0, 100, true, true, false, false),
			linChan("lightness", 0, 100, true, false, false, false),
		}
	case spaceHWB:
		return [3]channelInfo{
			hueChannelInfo,
			linChan("whiteness", 0, 100, true, false, false, false),
			linChan("blackness", 0, 100, true, false, false, false),
		}
	case spaceLab:
		return [3]channelInfo{
			linChan("lightness", 0, 100, false, true, true, false),
			linChan("a", -125, 125, false, false, false, false),
			linChan("b", -125, 125, false, false, false, false),
		}
	case spaceLCH:
		return [3]channelInfo{
			linChan("lightness", 0, 100, false, true, true, false),
			linChan("chroma", 0, 150, false, true, false, false),
			hueChannelInfo,
		}
	case spaceOklab:
		return [3]channelInfo{
			linChan("lightness", 0, 1, false, true, true, true),
			linChan("a", -0.4, 0.4, false, false, false, false),
			linChan("b", -0.4, 0.4, false, false, false, false),
		}
	case spaceOklch:
		return [3]channelInfo{
			linChan("lightness", 0, 1, false, true, true, true),
			linChan("chroma", 0, 0.4, false, true, false, false),
			hueChannelInfo,
		}
	case spaceXYZD65, spaceXYZD50:
		return xyzUnitChannels
	case spaceLMS:
		return [3]channelInfo{
			linChan("long", 0, 1, false, false, false, false),
			linChan("medium", 0, 1, false, false, false, false),
			linChan("short", 0, 1, false, false, false, false),
		}
	default:
		// srgb, srgb-linear, display-p3, display-p3-linear, a98-rgb,
		// prophoto-rgb, rec2020
		return rgbUnitChannels
	}
}

func spaceIsLegacy(s ColorSpace) bool {
	return s == spaceRGB || s == spaceHSL || s == spaceHWB
}

func spaceIsPolar(s ColorSpace) bool {
	return s == spaceHSL || s == spaceHWB || s == spaceLCH || s == spaceOklch
}

func spaceIsBounded(s ColorSpace) bool {
	switch s {
	case spaceLab, spaceLCH, spaceOklab, spaceOklch, spaceXYZD65, spaceXYZD50, spaceLMS:
		return false
	default:
		return true
	}
}

// colorSpaceFromName resolves a CSS color space name (case-insensitive).
func colorSpaceFromName(name string) (ColorSpace, bool) {
	switch toLowerASCII(name) {
	case "rgb":
		return spaceRGB, true
	case "hwb":
		return spaceHWB, true
	case "hsl":
		return spaceHSL, true
	case "srgb":
		return spaceSRGB, true
	case "srgb-linear":
		return spaceSRGBLinear, true
	case "display-p3":
		return spaceDisplayP3, true
	case "display-p3-linear":
		return spaceDisplayP3Lin, true
	case "a98-rgb":
		return spaceA98RGB, true
	case "prophoto-rgb":
		return spaceProphotoRGB, true
	case "rec2020":
		return spaceRec2020, true
	case "xyz", "xyz-d65":
		return spaceXYZD65, true
	case "xyz-d50":
		return spaceXYZD50, true
	case "lab":
		return spaceLab, true
	case "lch":
		return spaceLCH, true
	case "oklab":
		return spaceOklab, true
	case "oklch":
		return spaceOklch, true
	default:
		return "", false
	}
}

// --- gamma companding (per RGB space) ---

func srgbToLinear(channel float64) float64 {
	abs := math.Abs(channel)
	if abs <= 0.04045 {
		return channel / 12.92
	}
	return signOf(channel) * math.Pow((abs+0.055)/1.055, 2.4)
}

func srgbFromLinear(channel float64) float64 {
	abs := math.Abs(channel)
	if abs <= 0.0031308 {
		return channel * 12.92
	}
	return signOf(channel) * (noFMA(1.055*math.Pow(abs, 1/2.4)) - 0.055)
}

func a98ToLinear(channel float64) float64 {
	return signOf(channel) * math.Pow(math.Abs(channel), 563.0/256.0)
}
func a98FromLinear(channel float64) float64 {
	return signOf(channel) * math.Pow(math.Abs(channel), 256.0/563.0)
}

func rec2020ToLinear(channel float64) float64 {
	return signOf(channel) * math.Pow(math.Abs(channel), 2.40)
}
func rec2020FromLinear(channel float64) float64 {
	return signOf(channel) * math.Pow(math.Abs(channel), 1/2.40)
}

func prophotoToLinear(channel float64) float64 {
	abs := math.Abs(channel)
	if abs <= 16.0/512.0 {
		return channel / 16
	}
	return signOf(channel) * math.Pow(abs, 1.8)
}
func prophotoFromLinear(channel float64) float64 {
	abs := math.Abs(channel)
	if abs >= 1.0/512.0 {
		return signOf(channel) * math.Pow(abs, 1/1.8)
	}
	return 16 * channel
}

// spaceToLinear converts a single channel of space s to its linear-light form.
func spaceToLinear(s ColorSpace, channel float64) float64 {
	switch s {
	case spaceRGB:
		return srgbToLinear(channel / 255)
	case spaceSRGB, spaceDisplayP3:
		return srgbToLinear(channel)
	case spaceA98RGB:
		return a98ToLinear(channel)
	case spaceRec2020:
		return rec2020ToLinear(channel)
	case spaceProphotoRGB:
		return prophotoToLinear(channel)
	default:
		// srgb-linear, display-p3-linear, xyz-d65, xyz-d50, lms: identity
		return channel
	}
}

// spaceFromLinear converts a single linear-light channel back to space s.
func spaceFromLinear(s ColorSpace, channel float64) float64 {
	switch s {
	case spaceRGB:
		return srgbFromLinear(channel) * 255
	case spaceSRGB, spaceDisplayP3:
		return srgbFromLinear(channel)
	case spaceA98RGB:
		return a98FromLinear(channel)
	case spaceRec2020:
		return rec2020FromLinear(channel)
	case spaceProphotoRGB:
		return prophotoFromLinear(channel)
	default:
		return channel
	}
}

func signOf(v float64) float64 {
	if v > 0 {
		return 1
	}
	if v < 0 {
		return -1
	}
	return v // preserves 0 and -0 like Dart's num.sign
}

// transformationMatrix returns the linear-space matrix from src to dest, using
// dart-sass's precomputed direct matrices. dest is always a linear/xyz/lms space.
func transformationMatrix(src, dest ColorSpace) [9]float64 {
	switch src {
	case spaceSRGB, spaceSRGBLinear, spaceRGB:
		switch dest {
		case spaceDisplayP3, spaceDisplayP3Lin:
			return linearSrgbToLinearDisplayP3
		case spaceA98RGB:
			return linearSrgbToLinearA98Rgb
		case spaceProphotoRGB:
			return linearSrgbToLinearProphotoRgb
		case spaceRec2020:
			return linearSrgbToLinearRec2020
		case spaceXYZD65:
			return linearSrgbToXyzD65
		case spaceXYZD50:
			return linearSrgbToXyzD50
		case spaceLMS:
			return linearSrgbToLms
		}
	case spaceDisplayP3, spaceDisplayP3Lin:
		switch dest {
		case spaceSRGBLinear, spaceSRGB, spaceRGB:
			return linearDisplayP3ToLinearSrgb
		case spaceA98RGB:
			return linearDisplayP3ToLinearA98Rgb
		case spaceProphotoRGB:
			return linearDisplayP3ToLinearProphotoRgb
		case spaceRec2020:
			return linearDisplayP3ToLinearRec2020
		case spaceXYZD65:
			return linearDisplayP3ToXyzD65
		case spaceXYZD50:
			return linearDisplayP3ToXyzD50
		case spaceLMS:
			return linearDisplayP3ToLms
		}
	case spaceA98RGB:
		switch dest {
		case spaceSRGBLinear, spaceSRGB, spaceRGB:
			return linearA98RgbToLinearSrgb
		case spaceDisplayP3, spaceDisplayP3Lin:
			return linearA98RgbToLinearDisplayP3
		case spaceProphotoRGB:
			return linearA98RgbToLinearProphotoRgb
		case spaceRec2020:
			return linearA98RgbToLinearRec2020
		case spaceXYZD65:
			return linearA98RgbToXyzD65
		case spaceXYZD50:
			return linearA98RgbToXyzD50
		case spaceLMS:
			return linearA98RgbToLms
		}
	case spaceRec2020:
		switch dest {
		case spaceSRGBLinear, spaceSRGB, spaceRGB:
			return linearRec2020ToLinearSrgb
		case spaceA98RGB:
			return linearRec2020ToLinearA98Rgb
		case spaceDisplayP3, spaceDisplayP3Lin:
			return linearRec2020ToLinearDisplayP3
		case spaceProphotoRGB:
			return linearRec2020ToLinearProphotoRgb
		case spaceXYZD65:
			return linearRec2020ToXyzD65
		case spaceXYZD50:
			return linearRec2020ToXyzD50
		case spaceLMS:
			return linearRec2020ToLms
		}
	case spaceProphotoRGB:
		switch dest {
		case spaceSRGBLinear, spaceSRGB, spaceRGB:
			return linearProphotoRgbToLinearSrgb
		case spaceA98RGB:
			return linearProphotoRgbToLinearA98Rgb
		case spaceDisplayP3, spaceDisplayP3Lin:
			return linearProphotoRgbToLinearDisplayP3
		case spaceRec2020:
			return linearProphotoRgbToLinearRec2020
		case spaceXYZD65:
			return linearProphotoRgbToXyzD65
		case spaceXYZD50:
			return linearProphotoRgbToXyzD50
		case spaceLMS:
			return linearProphotoRgbToLms
		}
	case spaceXYZD65:
		switch dest {
		case spaceSRGBLinear, spaceSRGB, spaceRGB:
			return xyzD65ToLinearSrgb
		case spaceA98RGB:
			return xyzD65ToLinearA98Rgb
		case spaceProphotoRGB:
			return xyzD65ToLinearProphotoRgb
		case spaceDisplayP3, spaceDisplayP3Lin:
			return xyzD65ToLinearDisplayP3
		case spaceRec2020:
			return xyzD65ToLinearRec2020
		case spaceXYZD50:
			return xyzD65ToXyzD50
		case spaceLMS:
			return xyzD65ToLms
		}
	case spaceXYZD50:
		switch dest {
		case spaceSRGBLinear, spaceSRGB, spaceRGB:
			return xyzD50ToLinearSrgb
		case spaceA98RGB:
			return xyzD50ToLinearA98Rgb
		case spaceProphotoRGB:
			return xyzD50ToLinearProphotoRgb
		case spaceDisplayP3, spaceDisplayP3Lin:
			return xyzD50ToLinearDisplayP3
		case spaceRec2020:
			return xyzD50ToLinearRec2020
		case spaceXYZD65:
			return xyzD50ToXyzD65
		case spaceLMS:
			return xyzD50ToLms
		}
	case spaceLMS:
		switch dest {
		case spaceSRGBLinear, spaceSRGB, spaceRGB:
			return lmsToLinearSrgb
		case spaceA98RGB:
			return lmsToLinearA98Rgb
		case spaceProphotoRGB:
			return lmsToLinearProphotoRgb
		case spaceDisplayP3, spaceDisplayP3Lin:
			return lmsToLinearDisplayP3
		case spaceRec2020:
			return lmsToLinearRec2020
		case spaceXYZD65:
			return lmsToXyzD65
		case spaceXYZD50:
			return lmsToXyzD50
		}
	}
	panic("no transformation matrix from " + string(src) + " to " + string(dest))
}
