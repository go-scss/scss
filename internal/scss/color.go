// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import (
	"math"
	"strings"
)

// colorFormat records how a color literal was written, for expanded-mode
// serialization (only meaningful for legacy rgb-space colors).
type colorFormat int

const (
	fmtNone colorFormat = iota
	fmtRgbFunction
	fmtSpan // original holds the verbatim source text (hex/keyword/hsl literal)
)

// SassColor is a CSS Color 4 color: a color space, three channels (with
// optional "missing"/none flags), and an alpha channel. Legacy rgb/hsl/hwb
// colors are represented in their own spaces just like modern ones; the
// serializer collapses them to the shortest legacy form.
type SassColor struct {
	space      ColorSpace
	c0, c1, c2 float64
	m0, m1, m2 bool // channel is "missing" (none)
	alpha      float64
	am         bool // alpha is missing
	format     colorFormat
	original   string
}

// --- nullable-channel helpers (mirror dart-sass's double?) ---

// noFMA rounds x, acting as a barrier that stops the Go compiler from fusing a
// multiply and an add into one FMA instruction (which some arches, e.g. arm64,
// do). dart-sass never fuses, so this keeps every conversion bit-identical to
// dart-sass and identical across all target architectures.
func noFMA(x float64) float64 { return float64(x) }

// dot3 computes a·b + c·d + e·f with each product rounded separately (no FMA).
func dot3(a0, b0, a1, b1, a2, b2 float64) float64 {
	return noFMA(a0*b0) + noFMA(a1*b1) + noFMA(a2*b2)
}

func pf(v float64) *float64 { return &v }

func orZero(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}

func mapP(p *float64, f func(float64) float64) *float64 {
	if p == nil {
		return nil
	}
	return pf(f(*p))
}

func divP(p *float64, d float64) *float64 {
	if p == nil {
		return nil
	}
	return pf(*p / d)
}

func mulP(p *float64, m float64) *float64 {
	if p == nil {
		return nil
	}
	return pf(*p * m)
}

// --- fuzzy helpers (dart-sass semantics) ---

func fuzzyLessThan(a, b float64) bool    { return a < b && !fuzzyEquals(a, b) }
func fuzzyGreaterThan(a, b float64) bool { return a > b && !fuzzyEquals(a, b) }
func fuzzyLessThanOrEquals(a, b float64) bool {
	return a < b || fuzzyEquals(a, b)
}
func fuzzyGreaterThanOrEquals(a, b float64) bool {
	return a > b || fuzzyEquals(a, b)
}
func fuzzyInRange(v, min, max float64) bool {
	return fuzzyGreaterThanOrEquals(v, min) && fuzzyLessThanOrEquals(v, max)
}
func fuzzyIsIntC(v float64) bool { return fuzzyEquals(v, math.Round(v)) }

// clampLikeCss clamps v to [min,max], snapping fuzzily-equal bounds.
func clampLikeCss(v, min, max float64) float64 {
	if fuzzyEquals(v, min) {
		return min
	}
	if fuzzyEquals(v, max) {
		return max
	}
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

// dartMod returns a % b with the sign of b (Dart's % operator).
func dartMod(a, b float64) float64 {
	r := math.Mod(a, b)
	if r != 0 && (r < 0) != (b < 0) {
		r += b
	}
	return r
}

func toLowerASCII(s string) string { return strings.ToLower(s) }

// --- constructors ---

// newColorRaw builds a color with no channel pre-processing (dart's _forSpace).
func newColorRaw(space ColorSpace, c0, c1, c2, alpha *float64, format colorFormat, original string) *SassColor {
	c := &SassColor{space: space, format: format, original: original}
	setChan(&c.c0, &c.m0, c0)
	setChan(&c.c1, &c.m1, c1)
	setChan(&c.c2, &c.m2, c2)
	if alpha == nil {
		c.am = true
	} else {
		c.alpha = clampLikeCss(*alpha, 0, 1)
	}
	return c
}

func setChan(v *float64, miss *bool, p *float64) {
	if p == nil {
		*miss = true
		*v = 0
	} else {
		*v = *p
	}
}

// newColor builds a color applying dart's forSpaceInternal normalization
// (hue wrapping and chroma/saturation absolute value for polar spaces).
func newColor(space ColorSpace, c0, c1, c2, alpha *float64) *SassColor {
	switch space {
	case spaceHSL:
		invert := c1 != nil && fuzzyLessThan(*c1, 0)
		return newColorRaw(space, normalizeHueP(c0, invert), mapP(c1, math.Abs), c2, alpha, fmtNone, "")
	case spaceHWB:
		return newColorRaw(space, normalizeHueP(c0, false), c1, c2, alpha, fmtNone, "")
	case spaceLCH, spaceOklch:
		invert := c1 != nil && fuzzyLessThan(*c1, 0)
		return newColorRaw(space, c0, mapP(c1, math.Abs), normalizeHueP(c2, invert), alpha, fmtNone, "")
	default:
		return newColorRaw(space, c0, c1, c2, alpha, fmtNone, "")
	}
}

func normalizeHueP(hue *float64, invert bool) *float64 {
	if hue == nil {
		return nil
	}
	off := 0.0
	if invert {
		off = 180
	}
	return pf(dartMod(dartMod(*hue, 360)+360+off, 360))
}

// sassColorRGB builds a legacy rgb-space color with an explicit format.
func sassColorRGB(r, g, b, alpha *float64, format colorFormat) *SassColor {
	return newColorRaw(spaceRGB, r, g, b, alpha, format, "")
}

// --- accessors ---

func (c *SassColor) channels() [3]*float64 {
	var out [3]*float64
	if !c.m0 {
		out[0] = pf(c.c0)
	}
	if !c.m1 {
		out[1] = pf(c.c1)
	}
	if !c.m2 {
		out[2] = pf(c.c2)
	}
	return out
}

func (c *SassColor) alphaP() *float64 {
	if c.am {
		return nil
	}
	return pf(c.alpha)
}

func (c *SassColor) hasMissingChannel() bool {
	return c.m0 || c.m1 || c.m2 || c.am
}

func (c *SassColor) isLegacy() bool { return spaceIsLegacy(c.space) }

func (c *SassColor) channelMissingByName(name string) (bool, bool) {
	chans := spaceChannels(c.space)
	miss := [3]bool{c.m0, c.m1, c.m2}
	for i := range chans {
		if chans[i].name == name {
			return miss[i], true
		}
	}
	if name == "alpha" {
		return c.am, true
	}
	return false, false
}

func (c *SassColor) channelPowerlessByName(name string) (bool, bool) {
	chans := spaceChannels(c.space)
	for i := range chans {
		if chans[i].name == name {
			return c.isChannelPowerless(i), true
		}
	}
	if name == "alpha" {
		return false, true
	}
	return false, false
}

func (c *SassColor) isChannelPowerless(index int) bool {
	switch index {
	case 0:
		switch c.space {
		case spaceHSL:
			return fuzzyEquals(c.c1, 0)
		case spaceHWB:
			return fuzzyGreaterThanOrEquals(c.c1+c.c2, 100)
		}
	case 2:
		switch c.space {
		case spaceLCH, spaceOklch:
			return fuzzyEquals(c.c1, 0)
		}
	}
	return false
}

func (c *SassColor) isInGamut() bool {
	if !spaceIsBounded(c.space) {
		return true
	}
	chans := spaceChannels(c.space)
	vals := [3]float64{c.c0, c.c1, c.c2}
	for i := range chans {
		if chans[i].polar {
			continue
		}
		if !fuzzyInRange(vals[i], chans[i].min, chans[i].max) {
			return false
		}
	}
	return true
}

// --- conversion ---

// toSpace converts this color to dest. If legacyMissing is false, missing
// channels in a resulting legacy color are replaced by zero.
func (c *SassColor) toSpace(dest ColorSpace, legacyMissing bool) *SassColor {
	if c.space == dest {
		return c
	}
	conv := convertColor(c.space, c.channels(), c.alphaP(), dest)
	if !legacyMissing && conv.isLegacy() && conv.hasMissingChannel() {
		return newColorRaw(conv.space, pf(conv.c0), pf(conv.c1), pf(conv.c2), pf(conv.alpha), fmtNone, "")
	}
	return conv
}

func convertColor(src ColorSpace, ch [3]*float64, alpha *float64, dest ColorSpace) *SassColor {
	c0, c1, c2 := ch[0], ch[1], ch[2]
	switch src {
	case spaceRGB:
		return srgbConvert(dest, divP(c0, 255), divP(c1, 255), divP(c2, 255), alpha, false, false, false)
	case spaceSRGB:
		return srgbConvert(dest, c0, c1, c2, alpha, false, false, false)
	case spaceSRGBLinear:
		switch dest {
		case spaceRGB, spaceHSL, spaceHWB, spaceSRGB:
			return srgbConvert(dest, mapP(c0, srgbFromLinear), mapP(c1, srgbFromLinear), mapP(c2, srgbFromLinear), alpha, false, false, false)
		default:
			return convertLinear(src, dest, c0, c1, c2, alpha, false, false, false, false, false)
		}
	case spaceHSL:
		return hslConvert(dest, c0, c1, c2, alpha)
	case spaceHWB:
		return hwbConvert(dest, c0, c1, c2, alpha)
	case spaceLab:
		return labConvert(dest, c0, c1, c2, alpha, false, false)
	case spaceLCH:
		return lchConvert(dest, c0, c1, c2, alpha)
	case spaceOklab:
		return oklabConvert(dest, c0, c1, c2, alpha, false, false)
	case spaceOklch:
		return oklchConvert(dest, c0, c1, c2, alpha)
	case spaceLMS:
		return lmsConvert(dest, c0, c1, c2, alpha, false, false, false, false, false)
	case spaceXYZD50:
		return xyzD50Convert(dest, c0, c1, c2, alpha, false, false, false, false, false)
	case spaceDisplayP3:
		if dest == spaceDisplayP3Lin {
			return newColorRaw(dest, mapP(c0, srgbToLinear), mapP(c1, srgbToLinear), mapP(c2, srgbToLinear), alpha, fmtNone, "")
		}
		return convertLinear(src, dest, c0, c1, c2, alpha, false, false, false, false, false)
	case spaceDisplayP3Lin:
		if dest == spaceDisplayP3 {
			return newColorRaw(dest, mapP(c0, srgbFromLinear), mapP(c1, srgbFromLinear), mapP(c2, srgbFromLinear), alpha, fmtNone, "")
		}
		return convertLinear(src, dest, c0, c1, c2, alpha, false, false, false, false, false)
	default:
		// a98-rgb, prophoto-rgb, rec2020, xyz-d65
		return convertLinear(src, dest, c0, c1, c2, alpha, false, false, false, false, false)
	}
}

func convertLinear(src, dest ColorSpace, red, green, blue, alpha *float64, mL, mC, mH, mA, mB bool) *SassColor {
	linearDest := dest
	switch dest {
	case spaceHSL, spaceHWB:
		linearDest = spaceSRGB
	case spaceLab, spaceLCH:
		linearDest = spaceXYZD50
	case spaceOklab, spaceOklch:
		linearDest = spaceLMS
	}

	var tr, tg, tb *float64
	if linearDest == src {
		tr, tg, tb = red, green, blue
	} else {
		lr := spaceToLinear(src, orZero(red))
		lg := spaceToLinear(src, orZero(green))
		lb := spaceToLinear(src, orZero(blue))
		m := transformationMatrix(src, linearDest)
		tr = pf(spaceFromLinear(linearDest, dot3(m[0], lr, m[1], lg, m[2], lb)))
		tg = pf(spaceFromLinear(linearDest, dot3(m[3], lr, m[4], lg, m[5], lb)))
		tb = pf(spaceFromLinear(linearDest, dot3(m[6], lr, m[7], lg, m[8], lb)))
	}

	switch dest {
	case spaceHSL, spaceHWB:
		return srgbConvert(dest, tr, tg, tb, alpha, mL, mC, mH)
	case spaceLab, spaceLCH:
		return xyzD50Convert(dest, tr, tg, tb, alpha, mL, mC, mH, mA, mB)
	case spaceOklab, spaceOklch:
		return lmsConvert(dest, tr, tg, tb, alpha, mL, mC, mH, mA, mB)
	default:
		var o0, o1, o2 *float64
		if red != nil {
			o0 = tr
		}
		if green != nil {
			o1 = tg
		}
		if blue != nil {
			o2 = tb
		}
		return newColorRaw(dest, o0, o1, o2, alpha, fmtNone, "")
	}
}

func srgbConvert(dest ColorSpace, red, green, blue, alpha *float64, mL, mC, mH bool) *SassColor {
	switch dest {
	case spaceHSL, spaceHWB:
		r, g, b := orZero(red), orZero(green), orZero(blue)
		max := math.Max(math.Max(r, g), b)
		min := math.Min(math.Min(r, g), b)
		delta := max - min
		var hue float64
		switch {
		case max == min:
			hue = 0
		case max == r:
			hue = noFMA(60*(g-b)/delta) + 360
		case max == g:
			hue = noFMA(60*(b-r)/delta) + 120
		default:
			hue = noFMA(60*(r-g)/delta) + 240
		}
		if dest == spaceHSL {
			lightness := (min + max) / 2
			var saturation float64
			if lightness == 0 || lightness == 1 {
				saturation = 0
			} else {
				saturation = noFMA(100*(max-lightness)) / math.Min(lightness, 1-lightness)
			}
			if saturation < 0 {
				hue += 180
				saturation = math.Abs(saturation)
			}
			var hp, sp, lp *float64
			if !(mH || fuzzyEquals(saturation, 0)) {
				hp = pf(dartMod(hue, 360))
			}
			if !mC {
				sp = pf(saturation)
			}
			if !mL {
				lp = pf(lightness * 100)
			}
			return newColor(spaceHSL, hp, sp, lp, alpha)
		}
		whiteness := min * 100
		blackness := 100 - noFMA(max*100)
		var hp *float64
		if !(mH || fuzzyGreaterThanOrEquals(whiteness+blackness, 100)) {
			hp = pf(dartMod(hue, 360))
		}
		return newColor(spaceHWB, hp, pf(whiteness), pf(blackness), alpha)
	case spaceRGB:
		return sassColorRGB(mulP(red, 255), mulP(green, 255), mulP(blue, 255), alpha, fmtNone)
	case spaceSRGBLinear:
		return newColorRaw(dest, mapP(red, srgbToLinear), mapP(green, srgbToLinear), mapP(blue, srgbToLinear), alpha, fmtNone, "")
	default:
		return convertLinear(spaceSRGB, dest, red, green, blue, alpha, mL, mC, mH, false, false)
	}
}

func hslConvert(dest ColorSpace, hue, sat, light, alpha *float64) *SassColor {
	scaledHue := dartMod(orZero(hue)/360, 1)
	scaledSat := orZero(sat) / 100
	scaledLight := orZero(light) / 100
	var m2 float64
	if scaledLight <= 0.5 {
		m2 = scaledLight * (scaledSat + 1)
	} else {
		m2 = scaledLight + scaledSat - noFMA(scaledLight*scaledSat)
	}
	m1 := scaledLight*2 - m2
	return srgbConvert(dest,
		pf(hueToRgb(m1, m2, scaledHue+1.0/3)),
		pf(hueToRgb(m1, m2, scaledHue)),
		pf(hueToRgb(m1, m2, scaledHue-1.0/3)),
		alpha, light == nil, sat == nil, hue == nil)
}

func hwbConvert(dest ColorSpace, hue, white, black, alpha *float64) *SassColor {
	scaledHue := dartMod(orZero(hue), 360) / 360
	scaledWhite := orZero(white) / 100
	scaledBlack := orZero(black) / 100
	sum := scaledWhite + scaledBlack
	if sum > 1 {
		scaledWhite /= sum
		scaledBlack /= sum
	}
	factor := 1 - scaledWhite - scaledBlack
	toRgb := func(h float64) float64 { return noFMA(hueToRgb(0, 1, h)*factor) + scaledWhite }
	return srgbConvert(dest,
		pf(toRgb(scaledHue+1.0/3)),
		pf(toRgb(scaledHue)),
		pf(toRgb(scaledHue-1.0/3)),
		alpha, false, false, hue == nil)
}

func hueToRgb(m1, m2, hue float64) float64 {
	if hue < 0 {
		hue += 1
	}
	if hue > 1 {
		hue -= 1
	}
	switch {
	case hue < 1.0/6:
		return m1 + noFMA((m2-m1)*hue*6)
	case hue < 1.0/2:
		return m2
	case hue < 2.0/3:
		return m1 + noFMA((m2-m1)*(2.0/3-hue)*6)
	default:
		return m1
	}
}

const (
	labKappa   = 24389.0 / 27.0
	labEpsilon = 216.0 / 24389.0
)

func labConvert(dest ColorSpace, light, a, b, alpha *float64, mC, mH bool) *SassColor {
	switch dest {
	case spaceLab:
		powerlessAB := light == nil || fuzzyEquals(orZero(light), 0)
		var ap, bp *float64
		if !(a == nil || powerlessAB) {
			ap = a
		}
		if !(b == nil || powerlessAB) {
			bp = b
		}
		return newColor(spaceLab, light, ap, bp, alpha)
	case spaceLCH:
		return labToLch(spaceLCH, light, a, b, alpha, false, false)
	default:
		mLight := light == nil
		L := orZero(light)
		f1 := (L + 16) / 116
		x := convertFToXorZ(orZero(a)/500+f1) * d50[0]
		var yv float64
		if L > labKappa*labEpsilon {
			yv = math.Pow((L+16)/116, 3)
		} else {
			yv = L / labKappa
		}
		yv *= d50[1]
		z := convertFToXorZ(f1-orZero(b)/200) * d50[2]
		return xyzD50Convert(dest, pf(x), pf(yv), pf(z), alpha, mLight, mC, mH, a == nil, b == nil)
	}
}

func convertFToXorZ(component float64) float64 {
	cubed := math.Pow(component, 3)
	if cubed > labEpsilon {
		return cubed
	}
	return (float64(116*component) - 16) / labKappa
}

func lchConvert(dest ColorSpace, light, chroma, hue, alpha *float64) *SassColor {
	hueRad := orZero(hue) * math.Pi / 180
	return labConvert(dest, light, pf(orZero(chroma)*math.Cos(hueRad)), pf(orZero(chroma)*math.Sin(hueRad)), alpha, chroma == nil, hue == nil)
}

func oklabConvert(dest ColorSpace, light, a, b, alpha *float64, mC, mH bool) *SassColor {
	if dest == spaceOklch {
		return labToLch(spaceOklch, light, a, b, alpha, mC, mH)
	}
	mLight := light == nil
	mA := a == nil
	mB := b == nil
	L, av, bv := orZero(light), orZero(a), orZero(b)
	l0 := math.Pow(dot3(oklabToLms[0], L, oklabToLms[1], av, oklabToLms[2], bv), 3)
	l1 := math.Pow(dot3(oklabToLms[3], L, oklabToLms[4], av, oklabToLms[5], bv), 3)
	l2 := math.Pow(dot3(oklabToLms[6], L, oklabToLms[7], av, oklabToLms[8], bv), 3)
	return lmsConvert(dest, pf(l0), pf(l1), pf(l2), alpha, mLight, mC, mH, mA, mB)
}

func oklchConvert(dest ColorSpace, light, chroma, hue, alpha *float64) *SassColor {
	hueRad := orZero(hue) * math.Pi / 180
	return oklabConvert(dest, light, pf(orZero(chroma)*math.Cos(hueRad)), pf(orZero(chroma)*math.Sin(hueRad)), alpha, chroma == nil, hue == nil)
}

func lmsConvert(dest ColorSpace, long, medium, short, alpha *float64, mL, mC, mH, mA, mB bool) *SassColor {
	switch dest {
	case spaceOklab:
		ls := cubeRootSign(orZero(long))
		ms := cubeRootSign(orZero(medium))
		ss := cubeRootSign(orZero(short))
		var lp, ap, bp *float64
		if !mL {
			lp = pf(dot3(lmsToOklab[0], ls, lmsToOklab[1], ms, lmsToOklab[2], ss))
		}
		if !mA {
			ap = pf(dot3(lmsToOklab[3], ls, lmsToOklab[4], ms, lmsToOklab[5], ss))
		}
		if !mB {
			bp = pf(dot3(lmsToOklab[6], ls, lmsToOklab[7], ms, lmsToOklab[8], ss))
		}
		return newColor(spaceOklab, lp, ap, bp, alpha)
	case spaceOklch:
		ls := cubeRootSign(orZero(long))
		ms := cubeRootSign(orZero(medium))
		ss := cubeRootSign(orZero(short))
		var lp *float64
		if !mL {
			lp = pf(dot3(lmsToOklab[0], ls, lmsToOklab[1], ms, lmsToOklab[2], ss))
		}
		return labToLch(spaceOklch, lp,
			pf(dot3(lmsToOklab[3], ls, lmsToOklab[4], ms, lmsToOklab[5], ss)),
			pf(dot3(lmsToOklab[6], ls, lmsToOklab[7], ms, lmsToOklab[8], ss)),
			alpha, mC, mH)
	default:
		return convertLinear(spaceLMS, dest, long, medium, short, alpha, mL, mC, mH, mA, mB)
	}
}

func cubeRootSign(n float64) float64 {
	return math.Pow(math.Abs(n), 1.0/3) * signOf(n)
}

func xyzD50Convert(dest ColorSpace, x, y, z, alpha *float64, mL, mC, mH, mA, mB bool) *SassColor {
	switch dest {
	case spaceLab, spaceLCH:
		f0 := convertComponentToLabF(orZero(x) / d50[0])
		f1 := convertComponentToLabF(orZero(y) / d50[1])
		f2 := convertComponentToLabF(orZero(z) / d50[2])
		var L *float64
		if !mL {
			L = pf(116*f1 - 16)
		}
		a := 500 * (f0 - f1)
		b := 200 * (f1 - f2)
		if dest == spaceLab {
			var ap, bp *float64
			if !mA {
				ap = pf(a)
			}
			if !mB {
				bp = pf(b)
			}
			return newColor(spaceLab, L, ap, bp, alpha)
		}
		return labToLch(spaceLCH, L, pf(a), pf(b), alpha, mC, mH)
	default:
		return convertLinear(spaceXYZD50, dest, x, y, z, alpha, mL, mC, mH, mA, mB)
	}
}

func convertComponentToLabF(component float64) float64 {
	if component > labEpsilon {
		return math.Pow(component, 1.0/3)
	}
	return (noFMA(labKappa*component) + 16) / 116
}

func labToLch(dest ColorSpace, light, a, b, alpha *float64, mC, mH bool) *SassColor {
	chroma := math.Sqrt(math.Pow(orZero(a), 2) + math.Pow(orZero(b), 2))
	var hue *float64
	if !(mH || fuzzyEquals(chroma, 0)) {
		h := math.Atan2(orZero(b), orZero(a)) * 180 / math.Pi
		if h < 0 {
			h += 360
		}
		hue = pf(h)
	}
	var cp *float64
	if !mC {
		cp = pf(chroma)
	}
	return newColor(dest, light, cp, hue, alpha)
}

// --- gamut mapping ---

func (c *SassColor) toGamut(method string) *SassColor {
	if c.isInGamut() {
		return c
	}
	if method == "clip" {
		return c.gamutClip()
	}
	return c.gamutLocalMinde()
}

func (c *SassColor) gamutClip() *SassColor {
	chans := spaceChannels(c.space)
	vals := c.channels()
	clamp := func(p *float64, ch channelInfo) *float64 {
		if p == nil {
			return nil
		}
		if ch.polar {
			return p
		}
		return pf(clampLikeCss(*p, ch.min, ch.max))
	}
	return newColorRaw(c.space, clamp(vals[0], chans[0]), clamp(vals[1], chans[1]), clamp(vals[2], chans[2]), c.alphaP(), fmtNone, "")
}

func (c *SassColor) gamutLocalMinde() *SassColor {
	const jnd = 0.02
	const epsilon = 0.0001
	origin := c.toSpace(spaceOklch, true)
	lightness := origin.alphaAwareChannel(0)
	hue := origin.alphaAwareChannel(2)
	alpha := origin.alphaP()

	if fuzzyGreaterThanOrEquals(orZero(lightness), 1) {
		if c.isLegacy() {
			return sassColorRGB(pf(255), pf(255), pf(255), c.alphaP(), fmtNone).toSpace(c.space, true)
		}
		return newColorRaw(c.space, pf(1), pf(1), pf(1), c.alphaP(), fmtNone, "")
	} else if fuzzyLessThanOrEquals(orZero(lightness), 0) {
		return sassColorRGB(pf(0), pf(0), pf(0), c.alphaP(), fmtNone).toSpace(c.space, true)
	}

	clipped := c.toGamut("clip")
	if deltaEOK(clipped, c) < jnd {
		return clipped
	}

	min := 0.0
	max := origin.c1
	minInGamut := true
	for max-min > epsilon {
		chroma := (min + max) / 2
		current := convertColor(spaceOklch, [3]*float64{lightness, pf(chroma), hue}, alpha, c.space)
		if minInGamut && current.isInGamut() {
			min = chroma
			continue
		}
		clipped = current.toGamut("clip")
		e := deltaEOK(clipped, current)
		if e < jnd {
			if jnd-e < epsilon {
				return clipped
			}
			minInGamut = false
			min = chroma
		} else {
			max = chroma
		}
	}
	return clipped
}

// alphaAwareChannel returns channel i as a nullable pointer (nil if missing).
func (c *SassColor) alphaAwareChannel(i int) *float64 {
	ch := c.channels()
	return ch[i]
}

func deltaEOK(c1, c2 *SassColor) float64 {
	lab1 := c1.toSpace(spaceOklab, true)
	lab2 := c2.toSpace(spaceOklab, true)
	return math.Sqrt(math.Pow(lab1.c0-lab2.c0, 2) + math.Pow(lab1.c1-lab2.c1, 2) + math.Pow(lab1.c2-lab2.c2, 2))
}

// --- equality ---

func (c *SassColor) equals(o Value) bool {
	oc, ok := o.(*SassColor)
	if !ok {
		return false
	}
	if c.isLegacy() {
		if !oc.isLegacy() {
			return false
		}
		if !fuzzyEqualsNullable(c.alphaP(), oc.alphaP()) {
			return false
		}
		if c.space == oc.space {
			cc := c.channels()
			ocx := oc.channels()
			return fuzzyEqualsNullable(cc[0], ocx[0]) && fuzzyEqualsNullable(cc[1], ocx[1]) && fuzzyEqualsNullable(cc[2], ocx[2])
		}
		return c.toSpace(spaceRGB, true).equals(oc.toSpace(spaceRGB, true))
	}
	if c.space != oc.space {
		return false
	}
	cc := c.channels()
	ocx := oc.channels()
	return fuzzyEqualsNullable(cc[0], ocx[0]) && fuzzyEqualsNullable(cc[1], ocx[1]) &&
		fuzzyEqualsNullable(cc[2], ocx[2]) && fuzzyEqualsNullable(c.alphaP(), oc.alphaP())
}

func fuzzyEqualsNullable(a, b *float64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return fuzzyEquals(*a, *b)
}

func (c *SassColor) isTruthy() bool  { return true }
func (c *SassColor) sep() Separator  { return SepUndecided }
func (c *SassColor) asList() []Value { return []Value{c} }

// --- hex/named literal parsing ---

func newHexColor(hex string) *SassColor {
	body := hex[1:]
	var r, g, b int
	a := 1.0
	switch len(body) {
	case 3:
		r = hexPair(body[0:1] + body[0:1])
		g = hexPair(body[1:2] + body[1:2])
		b = hexPair(body[2:3] + body[2:3])
	case 4:
		r = hexPair(body[0:1] + body[0:1])
		g = hexPair(body[1:2] + body[1:2])
		b = hexPair(body[2:3] + body[2:3])
		a = float64(hexPair(body[3:4]+body[3:4])) / 255.0
	case 6:
		r = hexPair(body[0:2])
		g = hexPair(body[2:4])
		b = hexPair(body[4:6])
	case 8:
		r = hexPair(body[0:2])
		g = hexPair(body[2:4])
		b = hexPair(body[4:6])
		a = float64(hexPair(body[6:8])) / 255.0
	}
	c := sassColorRGB(pf(float64(r)), pf(float64(g)), pf(float64(b)), pf(a), fmtNone)
	// A hex literal serializes verbatim in expanded output only when opaque.
	if a == 1 {
		c.format = fmtSpan
		c.original = hex
	}
	return c
}

func isHexColor(s string) bool {
	if len(s) < 4 || s[0] != '#' {
		return false
	}
	body := s[1:]
	if len(body) != 3 && len(body) != 4 && len(body) != 6 && len(body) != 8 {
		return false
	}
	for i := 0; i < len(body); i++ {
		c := body[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

func lookupNamedColor(name string) (*SassColor, bool) {
	rgb, ok := namedColors[strings.ToLower(name)]
	if !ok {
		return nil, false
	}
	c := sassColorRGB(pf(float64(rgb[0])), pf(float64(rgb[1])), pf(float64(rgb[2])), pf(1), fmtNone)
	c.format = fmtSpan
	c.original = name
	return c, true
}

// namedColorTransparent builds the `transparent` keyword color.
func namedColorTransparent(name string) *SassColor {
	c := sassColorRGB(pf(0), pf(0), pf(0), pf(0), fmtNone)
	c.format = fmtSpan
	c.original = name
	return c
}
