// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import "math"

var colorFns = map[string]builtinFunc{
	"red":            func(ci *callInfo) Value { return numOut(float64(ci.color(0).R)) },
	"green":          func(ci *callInfo) Value { return numOut(float64(ci.color(0).G)) },
	"blue":           func(ci *callInfo) Value { return numOut(float64(ci.color(0).B)) },
	"alpha":          fnAlpha,
	"opacity":        fnAlpha,
	"hue":            fnHue,
	"saturation":     fnSaturation,
	"lightness":      fnLightness,
	"mix":            fnMix,
	"grayscale":      fnGrayscale,
	"invert":         fnInvert,
	"complement":     fnComplement,
	"adjust":         fnColorAdjust,
	"scale":          fnColorScale,
	"change":         fnColorChange,
	"lighten":        fnLighten,
	"darken":         fnDarken,
	"saturate":       fnSaturate,
	"desaturate":     fnDesaturate,
	"adjust-hue":     fnAdjustHue,
	"opacify":        fnOpacify,
	"fade-in":        fnOpacify,
	"transparentize": fnTransparentize,
	"fade-out":       fnTransparentize,
	"ie-hex-str":     fnIEHexStr,
}

func (ci *callInfo) color(i int) *SassColor {
	v := ci.require(i, "color")
	c, ok := v.(*SassColor)
	if !ok {
		ci.e.fail("%s is not a color.", serializeValue(v, false))
	}
	return c
}

func channelValue(n *Number) int {
	if len(n.Numer) == 1 && n.Numer[0] == "%" {
		return clampChannel(n.Val / 100 * 255)
	}
	return clampChannel(n.Val)
}

func fnRGB(ci *callInfo) Value {
	// rgb(color, alpha) form
	if len(ci.positional) == 2 {
		if c, ok := ci.positional[0].(*SassColor); ok {
			a := ci.e.asNumber(ci.positional[1])
			return rgbColor(c.R, c.G, c.B, alphaValue(a))
		}
	}
	if len(ci.positional) == 1 {
		if l, ok := ci.positional[0].(*List); ok && len(l.Elements) >= 3 {
			r := channelValue(ci.e.asNumber(l.Elements[0]))
			g := channelValue(ci.e.asNumber(l.Elements[1]))
			b := channelValue(ci.e.asNumber(l.Elements[2]))
			a := 1.0
			if len(l.Elements) == 4 {
				a = alphaValue(ci.e.asNumber(l.Elements[3]))
			}
			return rgbColor(r, g, b, a)
		}
	}
	r := channelValue(ci.num(0, "red"))
	g := channelValue(ci.num(1, "green"))
	b := channelValue(ci.num(2, "blue"))
	a := 1.0
	if av, ok := ci.get(3, "alpha"); ok {
		a = alphaValue(ci.e.asNumber(av))
	}
	return rgbColor(r, g, b, a)
}

func alphaValue(n *Number) float64 {
	if len(n.Numer) == 1 && n.Numer[0] == "%" {
		return clampAlpha(n.Val / 100)
	}
	return clampAlpha(n.Val)
}

func fnHSL(ci *callInfo) Value {
	if len(ci.positional) == 1 {
		if l, ok := ci.positional[0].(*List); ok && len(l.Elements) >= 3 {
			h := ci.e.asNumber(l.Elements[0]).Val
			s := ci.e.asNumber(l.Elements[1]).Val
			ll := ci.e.asNumber(l.Elements[2]).Val
			a := 1.0
			if len(l.Elements) == 4 {
				a = alphaValue(ci.e.asNumber(l.Elements[3]))
			}
			return hslColor(h, s, ll, a)
		}
	}
	h := ci.num(0, "hue").Val
	s := ci.num(1, "saturation").Val
	l := ci.num(2, "lightness").Val
	a := 1.0
	if av, ok := ci.get(3, "alpha"); ok {
		a = alphaValue(ci.e.asNumber(av))
	}
	return hslColor(h, s, l, a)
}

func fnAlpha(ci *callInfo) Value {
	return numOut(ci.color(0).A)
}

func fnHue(ci *callInfo) Value {
	c := ci.color(0)
	h, _, _ := rgbToHSL(c.R, c.G, c.B)
	return numOut(normalizeHue(h), "deg")
}

func fnSaturation(ci *callInfo) Value {
	c := ci.color(0)
	_, s, _ := rgbToHSL(c.R, c.G, c.B)
	return numOut(s, "%")
}

func fnLightness(ci *callInfo) Value {
	c := ci.color(0)
	_, _, l := rgbToHSL(c.R, c.G, c.B)
	return numOut(l, "%")
}

func fnMix(ci *callInfo) Value {
	c1 := ci.color(0)
	c2 := ci.color(1)
	weight := 50.0
	if w, ok := ci.get(2, "weight"); ok {
		weight = ci.e.asNumber(w).Val
	}
	p := weight / 100
	w := p*2 - 1
	a := c1.A - c2.A
	var w1 float64
	if w*a == -1 {
		w1 = w
	} else {
		w1 = (w + a) / (1 + w*a)
	}
	w1 = (w1 + 1) / 2
	w2 := 1 - w1
	r := clampChannel(float64(c1.R)*w1 + float64(c2.R)*w2)
	g := clampChannel(float64(c1.G)*w1 + float64(c2.G)*w2)
	b := clampChannel(float64(c1.B)*w1 + float64(c2.B)*w2)
	alpha := c1.A*p + c2.A*(1-p)
	return computedColor(r, g, b, alpha)
}

func fnGrayscale(ci *callInfo) Value {
	c := ci.color(0)
	_, _, l := rgbToHSL(c.R, c.G, c.B)
	r, g, b := hslToRGB(0, 0, l)
	return computedColor(r, g, b, c.A)
}

func fnInvert(ci *callInfo) Value {
	c := ci.color(0)
	weight := 100.0
	if w, ok := ci.get(1, "weight"); ok {
		weight = ci.e.asNumber(w).Val
	}
	p := weight / 100
	r := clampChannel(float64(255-c.R)*p + float64(c.R)*(1-p))
	g := clampChannel(float64(255-c.G)*p + float64(c.G)*(1-p))
	b := clampChannel(float64(255-c.B)*p + float64(c.B)*(1-p))
	return computedColor(r, g, b, c.A)
}

func fnComplement(ci *callInfo) Value {
	c := ci.color(0)
	h, s, l := rgbToHSL(c.R, c.G, c.B)
	r, g, b := hslToRGB(h+180, s, l)
	return computedColor(r, g, b, c.A)
}

func fnLighten(ci *callInfo) Value {
	c := ci.color(0)
	amt := ci.num(1, "amount").Val
	h, s, l := rgbToHSL(c.R, c.G, c.B)
	r, g, b := hslToRGB(h, s, clampPct(l+amt))
	return computedColor(r, g, b, c.A)
}

func fnDarken(ci *callInfo) Value {
	c := ci.color(0)
	amt := ci.num(1, "amount").Val
	h, s, l := rgbToHSL(c.R, c.G, c.B)
	r, g, b := hslToRGB(h, s, clampPct(l-amt))
	return computedColor(r, g, b, c.A)
}

func fnSaturate(ci *callInfo) Value {
	c := ci.color(0)
	amt := ci.num(1, "amount").Val
	h, s, l := rgbToHSL(c.R, c.G, c.B)
	r, g, b := hslToRGB(h, clampPct(s+amt), l)
	return computedColor(r, g, b, c.A)
}

func fnDesaturate(ci *callInfo) Value {
	c := ci.color(0)
	amt := ci.num(1, "amount").Val
	h, s, l := rgbToHSL(c.R, c.G, c.B)
	r, g, b := hslToRGB(h, clampPct(s-amt), l)
	return computedColor(r, g, b, c.A)
}

func fnAdjustHue(ci *callInfo) Value {
	c := ci.color(0)
	amt := ci.num(1, "degrees").Val
	h, s, l := rgbToHSL(c.R, c.G, c.B)
	r, g, b := hslToRGB(h+amt, s, l)
	return computedColor(r, g, b, c.A)
}

func fnOpacify(ci *callInfo) Value {
	c := ci.color(0)
	amt := ci.num(1, "amount").Val
	return computedColor(c.R, c.G, c.B, clampAlpha(c.A+amt))
}

func fnTransparentize(ci *callInfo) Value {
	c := ci.color(0)
	amt := ci.num(1, "amount").Val
	return computedColor(c.R, c.G, c.B, clampAlpha(c.A-amt))
}

func clampPct(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

func fnColorAdjust(ci *callInfo) Value {
	c := ci.color(0)
	r, g, b, a := c.R, c.G, c.B, c.A
	if v, ok := ci.named["red"]; ok {
		r = clampChannel(float64(r) + ci.e.asNumber(v).Val)
	}
	if v, ok := ci.named["green"]; ok {
		g = clampChannel(float64(g) + ci.e.asNumber(v).Val)
	}
	if v, ok := ci.named["blue"]; ok {
		b = clampChannel(float64(b) + ci.e.asNumber(v).Val)
	}
	if v, ok := ci.named["alpha"]; ok {
		a = clampAlpha(a + ci.e.asNumber(v).Val)
	}
	h, s, l := rgbToHSL(r, g, b)
	changed := false
	if v, ok := ci.named["hue"]; ok {
		h += ci.e.asNumber(v).Val
		changed = true
	}
	if v, ok := ci.named["saturation"]; ok {
		s = clampPct(s + ci.e.asNumber(v).Val)
		changed = true
	}
	if v, ok := ci.named["lightness"]; ok {
		l = clampPct(l + ci.e.asNumber(v).Val)
		changed = true
	}
	if changed {
		r, g, b = hslToRGB(h, s, l)
	}
	return computedColor(r, g, b, a)
}

func fnColorChange(ci *callInfo) Value {
	c := ci.color(0)
	r, g, b, a := c.R, c.G, c.B, c.A
	if v, ok := ci.named["red"]; ok {
		r = channelValue(ci.e.asNumber(v))
	}
	if v, ok := ci.named["green"]; ok {
		g = channelValue(ci.e.asNumber(v))
	}
	if v, ok := ci.named["blue"]; ok {
		b = channelValue(ci.e.asNumber(v))
	}
	if v, ok := ci.named["alpha"]; ok {
		a = alphaValue(ci.e.asNumber(v))
	}
	h, s, l := rgbToHSL(r, g, b)
	changed := false
	if v, ok := ci.named["hue"]; ok {
		h = ci.e.asNumber(v).Val
		changed = true
	}
	if v, ok := ci.named["saturation"]; ok {
		s = ci.e.asNumber(v).Val
		changed = true
	}
	if v, ok := ci.named["lightness"]; ok {
		l = ci.e.asNumber(v).Val
		changed = true
	}
	if changed {
		r, g, b = hslToRGB(h, s, l)
	}
	return computedColor(r, g, b, a)
}

func fnColorScale(ci *callInfo) Value {
	c := ci.color(0)
	r, g, b, a := float64(c.R), float64(c.G), float64(c.B), c.A
	scale := func(cur, target, pct float64) float64 {
		return cur + (target-cur)*(pct/100)
	}
	if v, ok := ci.named["red"]; ok {
		p := ci.e.asNumber(v).Val
		r = scale(r, tern(p > 0, 255, 0), p)
	}
	if v, ok := ci.named["green"]; ok {
		p := ci.e.asNumber(v).Val
		g = scale(g, tern(p > 0, 255, 0), p)
	}
	if v, ok := ci.named["blue"]; ok {
		p := ci.e.asNumber(v).Val
		b = scale(b, tern(p > 0, 255, 0), p)
	}
	if v, ok := ci.named["alpha"]; ok {
		p := ci.e.asNumber(v).Val
		a = scale(a, tern(p > 0, 1, 0), p)
	}
	return computedColor(clampChannel(r), clampChannel(g), clampChannel(b), clampAlpha(a))
}

func tern(cond bool, a, b float64) float64 {
	if cond {
		return a
	}
	return b
}

func fnIEHexStr(ci *callInfo) Value {
	c := ci.color(0)
	alpha := int(math.Round(c.A * 255))
	return &SassString{Text: "#" + hex2(alpha) + hex2(c.R) + hex2(c.G) + hex2(c.B), Quoted: false}
}

func hex2(v int) string {
	const digits = "0123456789ABCDEF"
	return string([]byte{digits[(v/16)&0xf], digits[v&0xf]})
}
