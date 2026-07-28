// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import (
	"math"
	"strings"
)

// colorFns holds the sass:color module functions.
var colorFns = map[string]builtinFunc{
	"red":          fnRed,
	"green":        fnGreen,
	"blue":         fnBlue,
	"hue":          fnHue,
	"saturation":   fnSaturation,
	"lightness":    fnLightness,
	"whiteness":    fnWhiteness,
	"blackness":    fnBlackness,
	"alpha":        fnAlpha,
	"opacity":      fnOpacity,
	"mix":          fnMix,
	"grayscale":    fnGrayscale,
	"invert":       fnInvert,
	"complement":   fnComplement,
	"adjust":       fnColorAdjust,
	"scale":        fnColorScale,
	"change":       fnColorChange,
	"ie-hex-str":   fnIEHexStr,
	"hwb":          fnHWB,
	"space":        fnSpace,
	"to-space":     fnToSpace,
	"is-legacy":    fnIsLegacy,
	"is-missing":   fnIsMissing,
	"is-in-gamut":  fnIsInGamut,
	"to-gamut":     fnToGamut,
	"channel":      fnChannel,
	"same":         fnSame,
	"is-powerless": fnIsPowerless,
}

func (ci *callInfo) color(i int) *SassColor {
	return ci.colorNamed(i, "color")
}

func (ci *callInfo) colorNamed(i int, name string) *SassColor {
	v := ci.require(i, name)
	c, ok := v.(*SassColor)
	if !ok {
		ci.e.fail("$%s: %s is not a color.", name, serializeValue(v, false))
	}
	return c
}

// --- number → channel helpers ---

// percentageOrUnitless returns n's value; unitless passes through, "%" scales
// against max, and any other unit fails.
func (e *evaluator) percentageOrUnitless(n *Number, max float64, name string) float64 {
	if !n.hasUnits() {
		return n.Val
	}
	if n.hasUnit("%") {
		return max * n.Val / 100
	}
	e.fail("$%s: Expected %s to have unit \"%%\" or no units.", name, serializeValue(n, false))
	return 0
}

// angleValue returns n coerced to degrees (any angle unit or unitless).
func (e *evaluator) angleValue(n *Number) float64 {
	if n.compatibleWithUnit("deg") {
		return n.coerceValueToUnit("deg")
	}
	return n.Val
}

func forcePercent(n *Number) *Number {
	if n == nil {
		return nil
	}
	if n.hasUnit("%") {
		return n
	}
	return &Number{Val: n.Val, Numer: []string{"%"}}
}

// channelFromValue converts a channel number into a stored double per the
// channel's rules; nil in, nil out.
func (e *evaluator) channelFromValue(ch channelInfo, n *Number, clamp bool) *float64 {
	if n == nil {
		return nil
	}
	if ch.polar {
		return pf(dartMod(n.coerceValueToUnit("deg"), 360))
	}
	if ch.requiresPercent && !n.hasUnit("%") {
		e.fail("$%s: Expected %s to have unit \"%%\".", ch.name, serializeValue(n, false))
	}
	v := e.percentageOrUnitless(n, ch.max, ch.name)
	if (!ch.lowerClamped && !ch.upperClamped) || !clamp {
		return pf(v)
	}
	lo := math.Inf(-1)
	hi := math.Inf(1)
	if ch.lowerClamped {
		lo = ch.min
	}
	if ch.upperClamped {
		hi = ch.max
	}
	if math.IsNaN(v) {
		// A clamped channel collapses a NaN to its lower bound, matching dart:
		// rgb(0, 0, calc(NaN)) -> rgb(0, 0, 0), saturation/chroma -> 0. Every
		// channel that reaches here is lower-clamped (unclamped channels keep NaN,
		// serialized as calc(NaN), and return early above), so lo is finite.
		return pf(lo)
	}
	return pf(clampLikeCss(v, lo, hi))
}

// colorFromChannels builds a color for space from up-to-three channel numbers.
func (e *evaluator) colorFromChannels(space ColorSpace, c0, c1, c2 *Number, alpha *float64, clamp, fromRgb bool) *SassColor {
	chans := spaceChannels(space)
	switch space {
	case spaceHSL:
		var hue *float64
		if c0 != nil {
			hue = pf(e.angleValue(c0))
		}
		return newColor(spaceHSL, hue,
			e.channelFromValue(chans[1], forcePercent(c1), clamp),
			e.channelFromValue(chans[2], forcePercent(c2), clamp), alpha)
	case spaceHWB:
		var whiteness, blackness *float64
		if c1 != nil {
			whiteness = pf(c1.Val)
		}
		if c2 != nil {
			blackness = pf(c2.Val)
		}
		if whiteness != nil && blackness != nil && *whiteness+*blackness > 100 {
			ow := *whiteness
			whiteness = pf(*whiteness / (*whiteness + *blackness) * 100)
			blackness = pf(*blackness / (ow + *blackness) * 100)
		}
		var hue *float64
		if c0 != nil {
			hue = pf(e.angleValue(c0))
		}
		return newColor(spaceHWB, hue, whiteness, blackness, alpha)
	case spaceRGB:
		format := fmtNone
		if fromRgb {
			format = fmtRgbFunction
		}
		return sassColorRGB(
			e.channelFromValue(chans[0], c0, clamp),
			e.channelFromValue(chans[1], c1, clamp),
			e.channelFromValue(chans[2], c2, clamp), alpha, format)
	default:
		return newColor(space,
			e.channelFromValue(chans[0], c0, clamp),
			e.channelFromValue(chans[1], c1, clamp),
			e.channelFromValue(chans[2], c2, clamp), alpha)
	}
}

// --- channel accessors ---

func fnRed(ci *callInfo) Value {
	return numOut(math.Round(ci.color(0).toSpace(spaceRGB, true).c0))
}
func fnGreen(ci *callInfo) Value {
	return numOut(math.Round(ci.color(0).toSpace(spaceRGB, true).c1))
}
func fnBlue(ci *callInfo) Value {
	return numOut(math.Round(ci.color(0).toSpace(spaceRGB, true).c2))
}
func fnHue(ci *callInfo) Value {
	return numOut(ci.color(0).toSpace(spaceHSL, true).c0, "deg")
}
func fnSaturation(ci *callInfo) Value {
	return numOut(ci.color(0).toSpace(spaceHSL, true).c1, "%")
}
func fnLightness(ci *callInfo) Value {
	return numOut(ci.color(0).toSpace(spaceHSL, true).c2, "%")
}
func fnWhiteness(ci *callInfo) Value {
	return numOut(ci.color(0).toSpace(spaceHWB, true).c1, "%")
}
func fnBlackness(ci *callInfo) Value {
	return numOut(ci.color(0).toSpace(spaceHWB, true).c2, "%")
}
func fnAlpha(ci *callInfo) Value {
	// Microsoft-filter overload: alpha(c=d) / alpha(c=d, e=f, g=h). Every argument
	// is an unquoted "x=y" string; the call is preserved verbatim as plain CSS.
	if len(ci.positional) > 0 && len(ci.named) == 0 {
		allFilter := true
		for _, a := range ci.positional {
			if s, ok := a.(*SassString); !ok || s.Quoted || !strings.Contains(s.Text, "=") {
				allFilter = false
				break
			}
		}
		if allFilter {
			return &SassString{Text: functionString("alpha", ci.positional), Quoted: false}
		}
	}
	return numOut(ci.color(0).alpha)
}

// fnOpacity implements the opacity()/color.opacity() overload. Unlike alpha(),
// opacity() doubles as the CSS `opacity()` filter function, so a numeric or
// special-value argument is preserved as a plain CSS function call.
func fnOpacity(ci *callInfo) Value {
	if len(ci.positional) == 1 && len(ci.named) == 0 {
		if a := ci.positional[0]; isCssFilterArg(a) {
			return &SassString{Text: functionString("opacity", []Value{a}), Quoted: false}
		}
	}
	return numOut(ci.color(0).alpha)
}

// --- rgb()/hsl()/color()/oklch()/... construction ---

func alphaFromValue(e *evaluator, v Value) *float64 {
	if isNone(v) {
		return nil
	}
	n := e.asNumber(v)
	a := e.percentageOrUnitless(n, 1, "alpha")
	if math.IsNaN(a) {
		// dart-sass carries a non-finite alpha through clamping to [0, 1]; a NaN
		// alpha collapses to 0 (the lower bound), unlike a NaN *color* channel,
		// which is preserved as calc(NaN).
		return pf(0)
	}
	return pf(clampLikeCss(a, 0, 1))
}

func isNone(v Value) bool {
	s, ok := v.(*SassString)
	return ok && !s.Quoted && strings.EqualFold(s.Text, "none")
}

// isCssFilterArg reports whether a single-argument colour filter overload
// (grayscale/invert/opacity/saturate) should be emitted as a plain CSS function
// rather than treated as a colour operation. Dart-sass does this when the sole
// argument is a plain number (e.g. opacity(10%)) or a special CSS value such as
// var(--c) or an unquoted calc string.
func isCssFilterArg(v Value) bool {
	if _, ok := v.(*Number); ok {
		return true
	}
	return isSpecialValue(v)
}

// isSpecialValue reports whether v is an unquoted CSS math/reference function
// (calc(), var(), env(), min(), max(), clamp(), attr()) that a color function
// must preserve verbatim rather than evaluate.
func isSpecialValue(v Value) bool {
	if _, ok := v.(*SassCalculation); ok {
		return true
	}
	s, ok := v.(*SassString)
	if !ok || s.Quoted {
		return false
	}
	t := strings.ToLower(strings.TrimLeft(s.Text, " \t"))
	for _, p := range []string{"calc(", "var(", "env(", "min(", "max(", "clamp(", "attr(", "if("} {
		if strings.HasPrefix(t, p) {
			return true
		}
	}
	return false
}

// legacyColorFunction reports whether name is a legacy comma-separated color
// function whose special-value passthrough uses comma syntax.
func legacyColorFunction(name string) bool {
	switch strings.ToLower(name) {
	case "rgb", "rgba", "hsl", "hsla":
		return true
	}
	return false
}

func anySpecialValue(vs ...Value) bool {
	for _, v := range vs {
		if isSpecialValue(v) {
			return true
		}
	}
	return false
}

func numOrNil(e *evaluator, v Value) *Number {
	if isNone(v) {
		return nil
	}
	if n, ok := v.(*Number); ok {
		return n
	}
	e.fail("Expected channel to be a number, was %s.", serializeValue(v, false))
	return nil
}

func fnRGB(ci *callInfo) Value {
	return rgbLike(ci, spaceRGB, [3]string{"red", "green", "blue"})
}

func fnHSL(ci *callInfo) Value {
	return rgbLike(ci, spaceHSL, [3]string{"hue", "saturation", "lightness"})
}

func rgbLike(ci *callInfo, space ColorSpace, names [3]string) Value {
	e := ci.e
	// Named single $channels form: rgb($channels: 0 255 127).
	if ch, ok := ci.named["channels"]; ok && len(ci.positional) == 0 && len(ci.named) == 1 {
		return e.parseChannels(string(space), ch, &space)
	}
	// Two-argument $color/$alpha form, positional or named:
	// rgb($color, $alpha) / rgb($color: #123, $alpha: 0.5).
	cv, okColor := ci.named["color"]
	if !okColor && len(ci.positional) == 2 {
		cv, okColor = ci.positional[0], true
	}
	if okColor {
		av, hasAlpha := ci.get(1, "alpha")
		if c, isColor := cv.(*SassColor); isColor {
			// A special-value alpha (var(--foo), an unquoted calc string) can't be
			// resolved to a number. For rgb() dart expands the colour into its
			// legacy channels and appends the special alpha; for hsl() it leaves
			// the whole call verbatim.
			if hasAlpha && isSpecialValue(av) {
				if space == spaceRGB {
					rgb := c.toSpace(spaceRGB, true)
					args := []Value{
						numOut(math.Round(rgb.c0)), numOut(math.Round(rgb.c1)),
						numOut(math.Round(rgb.c2)), av,
					}
					return &SassString{Text: functionString(ci.fn, args), Quoted: false}
				}
				return &SassString{Text: functionString(ci.fn, []Value{cv, av}), Quoted: false}
			}
			a := pf(1.0)
			if hasAlpha {
				a = alphaFromValue(e, av)
			}
			return c.toSpace(spaceRGB, true).changeAlpha(a)
		}
		// A special CSS value ($color or $alpha) makes the call unresolvable, so
		// it is emitted as a plain-CSS function verbatim.
		if isSpecialValue(cv) || (hasAlpha && isSpecialValue(av)) {
			args := []Value{cv}
			if hasAlpha {
				args = append(args, av)
			}
			return &SassString{Text: functionString(ci.fn, args), Quoted: false}
		}
	}
	// Single $channels argument.
	if len(ci.positional) == 1 && len(ci.named) == 0 {
		return e.parseChannels(string(space), ci.positional[0], &space)
	}
	// Three/four positional-or-named arguments.
	v0, ok0 := ci.get(0, names[0])
	v1, ok1 := ci.get(1, names[1])
	v2, ok2 := ci.get(2, names[2])
	if ok0 && ok1 && ok2 {
		av, hasAlpha := ci.get(3, "alpha")
		if anySpecialValue(v0, v1, v2) || (hasAlpha && isSpecialValue(av)) {
			return &SassString{Text: functionString(ci.fn, ci.positional), Quoted: false}
		}
		c0 := numOrNil(e, v0)
		c1 := numOrNil(e, v1)
		c2 := numOrNil(e, v2)
		alpha := pf(1.0)
		if hasAlpha {
			alpha = alphaFromValue(e, av)
		}
		return e.colorFromChannels(space, c0, c1, c2, alpha, true, space == spaceRGB)
	}
	e.fail("Missing element $channels.")
	return nil
}

func fnHWB(ci *callInfo) Value {
	e := ci.e
	// color.hwb($hue, $whiteness, $blackness, $alpha) 3-4 argument form, positional
	// or named (color.hwb($hue: 0, $whiteness: 30%, $blackness: 40%)).
	h, okH := ci.get(0, "hue")
	w, okW := ci.get(1, "whiteness")
	bl, okB := ci.get(2, "blackness")
	if okH && okW && okB {
		inner := &List{Elements: []Value{h, w, bl}, Sep: SepSpace}
		var input Value = inner
		if av, ok := ci.get(3, "alpha"); ok {
			input = &List{Elements: []Value{inner, av}, Sep: SepSlash}
		}
		sp := spaceHWB
		return e.parseChannels("hwb", input, &sp)
	}
	// Single $channels argument (space-separated list), e.g. color.hwb(0 30% 40%).
	sp := spaceHWB
	return e.parseChannels("hwb", ci.require(0, "channels"), &sp)
}

func fnColorFunc(ci *callInfo) Value {
	return ci.e.parseChannels("color", ci.require(0, "description"), nil)
}

func spaceFuncMaker(space ColorSpace, fname string) builtinFunc {
	return func(ci *callInfo) Value {
		sp := space
		return ci.e.parseChannels(fname, ci.require(0, "channels"), &sp)
	}
}

// parseChannels parses a color()/oklch()/lab()/... channel argument.
func (e *evaluator) parseChannels(functionName string, input Value, space *ColorSpace) Value {
	components, alphaValue := parseSlashChannels(e, input)
	elems := commonListElements(components)
	if len(elems) == 0 {
		e.fail("Color component list may not be empty.")
	}
	if s, ok := elems[0].(*SassString); ok && !s.Quoted && strings.EqualFold(s.Text, "from") {
		return &SassString{Text: functionString(functionName, []Value{input}), Quoted: false}
	}

	var channels []Value
	sp := space
	if sp == nil {
		nameStr, ok := elems[0].(*SassString)
		if !ok {
			e.fail("Expected color space to be a string.")
		}
		found, ok := colorSpaceFromName(nameStr.Text)
		if !ok {
			e.fail("Unknown color space \"%s\".", nameStr.Text)
		}
		switch found {
		case spaceRGB, spaceHSL, spaceHWB, spaceLab, spaceLCH, spaceOklab, spaceOklch:
			e.fail("The color() function doesn't support the color space %s. Use the %s() function instead.", found, found)
		}
		sp = &found
		channels = elems[1:]
	} else {
		channels = elems
	}

	if anySpecialValue(channels...) || (alphaValue != nil && isSpecialValue(alphaValue)) {
		// Legacy comma-separated functions (rgb/hsl) reconstruct the comma form
		// only when the channel count is exactly 3; a var() that might expand to a
		// different number of channels is preserved verbatim in space form.
		if legacyColorFunction(functionName) && len(channels) == 3 {
			args := append([]Value(nil), channels...)
			if alphaValue != nil {
				args = append(args, alphaValue)
			}
			return &SassString{Text: functionString(functionName, args), Quoted: false}
		}
		return &SassString{Text: functionString(functionName, []Value{input}), Quoted: false}
	}

	var alpha *float64
	if alphaValue == nil {
		alpha = pf(1)
	} else {
		alpha = alphaFromValue(e, alphaValue)
	}

	if len(channels) != 3 {
		e.fail("The %s color space has 3 channels but %s has %d.", *sp, serializeValue(input, false), len(channels))
	}

	return e.colorFromChannels(*sp,
		numOrNil(e, channels[0]),
		numOrNil(e, channels[1]),
		numOrNil(e, channels[2]),
		alpha, true, *sp == spaceRGB)
}

// parseSlashChannels splits input into its component list and optional alpha.
func parseSlashChannels(e *evaluator, input Value) (Value, Value) {
	l, ok := input.(*List)
	if !ok {
		return input, nil
	}
	if l.Sep == SepSlash {
		if len(l.Elements) == 2 {
			return l.Elements[0], l.Elements[1]
		}
		e.fail("Only 2 slash-separated elements allowed, but %d were passed.", len(l.Elements))
	}
	if l.Sep == SepSpace && len(l.Elements) >= 1 {
		last := l.Elements[len(l.Elements)-1]
		if sl, ok := last.(*List); ok && sl.Sep == SepSlash && len(sl.Elements) == 2 {
			newElems := append(append([]Value(nil), l.Elements[:len(l.Elements)-1]...), sl.Elements[0])
			return &List{Elements: newElems, Sep: SepSpace}, sl.Elements[1]
		}
	}
	return input, nil
}

func commonListElements(v Value) []Value {
	if l, ok := v.(*List); ok {
		return l.Elements
	}
	return []Value{v}
}

func functionString(name string, args []Value) string {
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = serializeValue(a, false)
	}
	return name + "(" + strings.Join(parts, ", ") + ")"
}

// --- color spaces module ---

func fnSpace(ci *callInfo) Value {
	return &SassString{Text: string(ci.color(0).space), Quoted: false}
}

func fnIsLegacy(ci *callInfo) Value {
	return boolean(ci.color(0).isLegacy())
}

func (e *evaluator) colorInSpace(colorV, spaceV Value, legacyMissing bool) *SassColor {
	c, ok := colorV.(*SassColor)
	if !ok {
		e.fail("$color: %s is not a color.", serializeValue(colorV, false))
	}
	if _, isNull := spaceV.(*Null); isNull || spaceV == nil {
		return c
	}
	sp := e.spaceArg(spaceV, "space")
	return c.toSpace(sp, legacyMissing)
}

func (e *evaluator) spaceArg(v Value, name string) ColorSpace {
	s, ok := v.(*SassString)
	if !ok {
		e.fail("$%s: %s is not a string.", name, serializeValue(v, false))
	}
	sp, ok := colorSpaceFromName(s.Text)
	if !ok {
		e.fail("$%s: Unknown color space \"%s\".", name, s.Text)
	}
	return sp
}

func channelNameArg(e *evaluator, v Value) string {
	s, ok := v.(*SassString)
	if !ok || !s.Quoted {
		e.fail("$channel: Expected a quoted string.")
	}
	return s.Text
}

func fnToSpace(ci *callInfo) Value {
	return ci.e.colorInSpace(ci.require(0, "color"), ci.require(1, "space"), false)
}

func fnIsMissing(ci *callInfo) Value {
	c := ci.color(0)
	name := channelNameArg(ci.e, ci.require(1, "channel"))
	miss, ok := c.channelMissingByName(name)
	if !ok {
		ci.e.fail("Color %s doesn't have a channel named \"%s\".", serializeValue(c, false), name)
	}
	return boolean(miss)
}

func fnIsInGamut(ci *callInfo) Value {
	var spaceV Value = sassNull
	if v, ok := ci.get(1, "space"); ok {
		spaceV = v
	}
	return boolean(ci.e.colorInSpace(ci.require(0, "color"), spaceV, true).isInGamut())
}

func fnChannel(ci *callInfo) Value {
	var spaceV Value = sassNull
	if v, ok := ci.get(2, "space"); ok {
		spaceV = v
	}
	c := ci.e.colorInSpace(ci.require(0, "color"), spaceV, true)
	name := channelNameArg(ci.e, ci.require(1, "channel"))
	if name == "alpha" {
		return numOut(c.alpha)
	}
	chans := spaceChannels(c.space)
	vals := [3]float64{c.c0, c.c1, c.c2}
	for i := range chans {
		if chans[i].name == name {
			v := vals[i]
			unit := chans[i].assocUnit
			if unit == "%" {
				v = v * 100 / chans[i].max
			}
			if unit == "" {
				return numOut(v)
			}
			return numOut(v, unit)
		}
	}
	ci.e.fail("Color %s has no channel named %s.", serializeValue(c, false), name)
	return nil
}

func fnIsPowerless(ci *callInfo) Value {
	var spaceV Value = sassNull
	if v, ok := ci.get(2, "space"); ok {
		spaceV = v
	}
	c := ci.e.colorInSpace(ci.require(0, "color"), spaceV, true)
	name := channelNameArg(ci.e, ci.require(1, "channel"))
	pw, ok := c.channelPowerlessByName(name)
	if !ok {
		ci.e.fail("Color %s doesn't have a channel named \"%s\".", serializeValue(c, false), name)
	}
	return boolean(pw)
}

func fnToGamut(ci *callInfo) Value {
	e := ci.e
	color := ci.color(0)
	var spaceV Value = sassNull
	if v, ok := ci.get(1, "space"); ok {
		spaceV = v
	}
	space := color.space
	if _, isNull := spaceV.(*Null); !isNull {
		space = e.spaceArg(spaceV, "space")
	}
	methodV, ok := ci.get(2, "method")
	if !ok || methodV == sassNull {
		e.fail("color.to-gamut() requires a $method argument.")
	}
	ms, ok := methodV.(*SassString)
	if !ok {
		e.fail("$method: %s is not a string.", serializeValue(methodV, false))
	}
	method := ms.Text
	if method != "clip" && method != "local-minde" {
		e.fail("$method: Unknown gamut map method \"%s\".", method)
	}
	if !spaceIsBounded(space) {
		return color
	}
	return color.toSpace(space, true).toGamut(method).toSpace(color.space, false)
}

func fnSame(ci *callInfo) Value {
	c1 := ci.colorNamed(0, "color1")
	c2 := ci.colorNamed(1, "color2")
	if c1.space == c2.space {
		return boolean(fuzzyEquals(c1.c0, c2.c0) && fuzzyEquals(c1.c1, c2.c1) &&
			fuzzyEquals(c1.c2, c2.c2) && fuzzyEquals(c1.alpha, c2.alpha))
	}
	return boolean(toXyzNoMissing(c1).equals(toXyzNoMissing(c2)))
}

func toXyzNoMissing(c *SassColor) *SassColor {
	if c.space == spaceXYZD65 {
		return newColorRaw(spaceXYZD65, pf(c.c0), pf(c.c1), pf(c.c2), pf(c.alpha), fmtNone, "")
	}
	return convertColor(c.space, [3]*float64{pf(c.c0), pf(c.c1), pf(c.c2)}, pf(c.alpha), spaceXYZD65)
}

// --- change/adjust/scale ---

func fnColorChange(ci *callInfo) Value { return updateComponents(ci, "change") }
func fnColorAdjust(ci *callInfo) Value { return updateComponents(ci, "adjust") }
func fnColorScale(ci *callInfo) Value  { return updateComponents(ci, "scale") }

func updateComponents(ci *callInfo, mode string) Value {
	e := ci.e
	original := ci.color(0)
	keywords := map[string]Value{}
	for k, v := range ci.named {
		if k == "color" {
			continue // bound to the first parameter, not a channel keyword
		}
		keywords[k] = v
	}
	var spaceKeyword Value
	if v, ok := keywords["space"]; ok {
		spaceKeyword = v
		delete(keywords, "space")
	}
	var alphaArg Value
	if v, ok := keywords["alpha"]; ok {
		alphaArg = v
		delete(keywords, "alpha")
	}

	var color *SassColor
	if spaceKeyword == nil && original.isLegacy() && len(keywords) > 0 {
		if sp, ok := sniffLegacyColorSpace(keywords); ok {
			color = original.toSpace(sp, false)
		} else {
			color = original
		}
	} else {
		var spaceV Value = sassNull
		if spaceKeyword != nil {
			spaceV = spaceKeyword
		}
		color = e.colorInSpace(original, spaceV, true)
	}

	chans := spaceChannels(color.space)
	var channelArgs [3]Value
	for name, v := range keywords {
		idx := -1
		for i := range chans {
			if chans[i].name == name {
				idx = i
				break
			}
		}
		if idx == -1 {
			e.fail("Color space %s doesn't have a channel with this name.", color.space)
		}
		channelArgs[idx] = v
	}

	var result *SassColor
	switch mode {
	case "change":
		result = e.changeColor(color, channelArgs, alphaArg)
	case "scale":
		result = e.scaleColor(color, channelArgs, alphaArg)
	default:
		result = e.adjustColor(color, channelArgs, alphaArg)
	}
	return result.toSpace(original.space, false)
}

func sniffLegacyColorSpace(keywords map[string]Value) (ColorSpace, bool) {
	for k := range keywords {
		switch k {
		case "red", "green", "blue":
			return spaceRGB, true
		case "saturation", "lightness":
			return spaceHSL, true
		case "whiteness", "blackness":
			return spaceHWB, true
		}
	}
	if _, ok := keywords["hue"]; ok {
		return spaceHSL, true
	}
	return "", false
}

func (e *evaluator) changeColor(color *SassColor, channelArgs [3]Value, alphaArg Value) *SassColor {
	chans := spaceChannels(color.space)
	vals := color.channels()
	nums := [3]*Number{}
	for i := 0; i < 3; i++ {
		nums[i] = e.channelForChange(channelArgs[i], color, vals[i], i, chans)
	}
	var alpha *float64
	switch {
	case alphaArg == nil:
		alpha = pf(color.alpha)
	case isNone(alphaArg):
		alpha = nil
	default:
		n := e.asNumber(alphaArg)
		if n.hasUnit("%") {
			alpha = pf(e.percentageOrUnitless(n, 1, "alpha"))
		} else {
			alpha = pf(n.Val)
		}
	}
	return e.colorFromChannels(color.space, nums[0], nums[1], nums[2], alpha, false, false)
}

func (e *evaluator) channelForChange(arg Value, color *SassColor, cur *float64, index int, chans [3]channelInfo) *Number {
	if arg == nil {
		if cur == nil {
			return nil
		}
		unit := ""
		if (color.space == spaceHSL || color.space == spaceHWB) && index > 0 {
			unit = "%"
		}
		n := &Number{Val: *cur}
		if unit != "" {
			n.Numer = []string{unit}
		}
		return n
	}
	if isNone(arg) {
		return nil
	}
	return e.asNumber(arg)
}

func (e *evaluator) scaleColor(color *SassColor, channelArgs [3]Value, alphaArg Value) *SassColor {
	chans := spaceChannels(color.space)
	vals := color.channels()
	nc := [3]*float64{}
	for i := 0; i < 3; i++ {
		nc[i] = e.scaleChannel(color, chans[i], vals[i], channelArgs[i])
	}
	alpha := color.alphaP()
	if alphaArg != nil {
		alpha = e.scaleChannel(color, channelInfo{name: "alpha", min: 0, max: 1}, color.alphaP(), alphaArg)
	}
	return newColor(color.space, nc[0], nc[1], nc[2], alpha)
}

func (e *evaluator) scaleChannel(color *SassColor, ch channelInfo, oldValue *float64, factorArg Value) *float64 {
	if factorArg == nil {
		return oldValue
	}
	if ch.polar {
		e.fail("Channel isn't scalable.")
	}
	if oldValue == nil {
		e.fail("Because the CSS working group is still deciding on the best behavior, Sass doesn't currently support modifying missing channels.")
	}
	fn := e.asNumber(factorArg)
	factor := e.percentageOrUnitless(fn, 100, ch.name)
	if !fn.hasUnit("%") {
		factor = fn.Val
	}
	factor = clampLikeCss(factor, -100, 100) / 100
	ov := *oldValue
	switch {
	case factor == 0:
		return pf(ov)
	case factor > 0:
		if ov >= ch.max {
			return pf(ov)
		}
		return pf(ov + noFMA((ch.max-ov)*factor))
	default:
		if ov <= ch.min {
			return pf(ov)
		}
		return pf(ov + noFMA((ov-ch.min)*factor))
	}
}

func (e *evaluator) adjustColor(color *SassColor, channelArgs [3]Value, alphaArg Value) *SassColor {
	chans := spaceChannels(color.space)
	vals := color.channels()
	nc := [3]*float64{}
	for i := 0; i < 3; i++ {
		nc[i] = e.adjustChannel(color, chans[i], vals[i], channelArgs[i])
	}
	alpha := color.alphaP()
	if alphaArg != nil {
		a := e.adjustChannel(color, channelInfo{name: "alpha", min: 0, max: 1}, color.alphaP(), alphaArg)
		if a != nil {
			alpha = pf(clampLikeCss(*a, 0, 1))
		}
	}
	return newColor(color.space, nc[0], nc[1], nc[2], alpha)
}

func (e *evaluator) adjustChannel(color *SassColor, ch channelInfo, oldValue *float64, arg Value) *float64 {
	if arg == nil {
		return oldValue
	}
	if oldValue == nil {
		e.fail("Because the CSS working group is still deciding on the best behavior, Sass doesn't currently support modifying missing channels.")
	}
	adj := e.asNumber(arg)
	if (color.space == spaceHSL || color.space == spaceHWB) && ch.polar {
		adj = &Number{Val: e.angleValue(adj)}
	} else if color.space == spaceHSL && (ch.name == "saturation" || ch.name == "lightness") {
		adj = &Number{Val: adj.Val, Numer: []string{"%"}}
	} else if ch.name == "alpha" && adj.hasUnits() {
		adj = &Number{Val: adj.Val}
	}
	delta := e.channelFromValue(ch, adj, false)
	result := *oldValue + orZero(delta)
	if ch.lowerClamped && result < ch.min {
		if *oldValue < ch.min {
			return pf(math.Max(*oldValue, result))
		}
		return pf(ch.min)
	}
	if ch.upperClamped && result > ch.max {
		if *oldValue > ch.max {
			return pf(math.Min(*oldValue, result))
		}
		return pf(ch.max)
	}
	return pf(result)
}

// --- mix / invert / complement / grayscale ---

func fnMix(ci *callInfo) Value {
	e := ci.e
	c1 := ci.colorNamed(0, "color1")
	c2 := ci.colorNamed(1, "color2")
	weight := &Number{Val: 50}
	if w, ok := ci.get(2, "weight"); ok {
		weight = e.asNumber(w)
	}
	if methodV, ok := ci.get(3, "method"); ok && methodV != sassNull {
		method := parseInterpolationMethod(e, methodV)
		w := e.percentageOrUnitless(weight, 100, "weight") / 100
		return c1.interpolate(c2, method, w, false)
	}
	if !c1.isLegacy() {
		e.fail("To use color.mix() with non-legacy color %s, you must provide a $method.", serializeValue(c1, false))
	}
	if !c2.isLegacy() {
		e.fail("To use color.mix() with non-legacy color %s, you must provide a $method.", serializeValue(c2, false))
	}
	return mixLegacy(c1, c2, weight.Val)
}

func mixLegacy(c1, c2 *SassColor, weightPct float64) *SassColor {
	rgb1 := c1.toSpace(spaceRGB, true)
	rgb2 := c2.toSpace(spaceRGB, true)
	weightScale := weightPct / 100
	normalizedWeight := weightScale*2 - 1
	alphaDistance := c1.alpha - c2.alpha
	var combined float64
	if normalizedWeight*alphaDistance == -1 {
		combined = normalizedWeight
	} else {
		combined = (normalizedWeight + alphaDistance) / (1 + normalizedWeight*alphaDistance)
	}
	weight1 := (combined + 1) / 2
	weight2 := 1 - weight1
	return sassColorRGB(
		pf(noFMA(rgb1.c0*weight1)+noFMA(rgb2.c0*weight2)),
		pf(noFMA(rgb1.c1*weight1)+noFMA(rgb2.c1*weight2)),
		pf(noFMA(rgb1.c2*weight1)+noFMA(rgb2.c2*weight2)),
		pf(noFMA(rgb1.alpha*weightScale)+noFMA(rgb2.alpha*(1-weightScale))), fmtNone)
}

func fnGrayscale(ci *callInfo) Value {
	arg := ci.require(0, "color")
	if isCssFilterArg(arg) {
		return &SassString{Text: functionString("grayscale", []Value{arg}), Quoted: false}
	}
	color := ci.color(0)
	if color.isLegacy() {
		hsl := color.toSpace(spaceHSL, true)
		return newColor(spaceHSL, hsl.channels()[0], pf(0), hsl.channels()[2], hsl.alphaP()).toSpace(color.space, false)
	}
	oklch := color.toSpace(spaceOklch, true)
	return newColor(spaceOklch, oklch.channels()[0], pf(0), oklch.channels()[2], oklch.alphaP()).toSpace(color.space, true)
}

func fnComplement(ci *callInfo) Value {
	e := ci.e
	color := ci.color(0)
	var spaceV Value = sassNull
	if v, ok := ci.get(1, "space"); ok {
		spaceV = v
	}
	var space ColorSpace
	if color.isLegacy() && spaceV == sassNull {
		space = spaceHSL
	} else {
		space = e.spaceArg(spaceV, "space")
	}
	if !spaceIsPolar(space) {
		e.fail("Color space %s doesn't have a hue channel.", space)
	}
	colorInSpace := color.toSpace(space, spaceV != sassNull)
	chans := spaceChannels(space)
	var result *SassColor
	if spaceIsLegacy(space) {
		result = newColor(space,
			e.adjustHueChannel(colorInSpace, chans[0], colorInSpace.channels()[0]),
			colorInSpace.channels()[1], colorInSpace.channels()[2], colorInSpace.alphaP())
	} else {
		result = newColor(space, colorInSpace.channels()[0], colorInSpace.channels()[1],
			e.adjustHueChannel(colorInSpace, chans[2], colorInSpace.channels()[2]), colorInSpace.alphaP())
	}
	return result.toSpace(color.space, false)
}

func (e *evaluator) adjustHueChannel(color *SassColor, ch channelInfo, value *float64) *float64 {
	if value == nil {
		e.fail("Because the CSS working group is still deciding on the best behavior, Sass doesn't currently support modifying missing channels.")
	}
	return pf(*value + 180)
}

func fnInvert(ci *callInfo) Value {
	e := ci.e
	weightNumber := &Number{Val: 100, Numer: []string{"%"}}
	if w, ok := ci.get(1, "weight"); ok {
		weightNumber = e.asNumber(w)
	}
	if arg := ci.require(0, "color"); isCssFilterArg(arg) {
		return &SassString{Text: functionString("invert", []Value{arg}), Quoted: false}
	}
	color := ci.color(0)
	var spaceV Value = sassNull
	if v, ok := ci.get(2, "space"); ok {
		spaceV = v
	}
	if spaceV == sassNull {
		rgb := color.toSpace(spaceRGB, true)
		chans := spaceChannels(spaceRGB)
		inverted := sassColorRGB(
			e.invertChannel(rgb, chans[0], rgb.channels()[0]),
			e.invertChannel(rgb, chans[1], rgb.channels()[1]),
			e.invertChannel(rgb, chans[2], rgb.channels()[2]),
			rgb.alphaP(), fmtNone)
		return mixLegacy(inverted, color, weightNumber.Val).toSpace(color.space, true)
	}
	space := e.spaceArg(spaceV, "space")
	weight := e.percentageOrUnitless(weightNumber, 100, "weight") / 100
	if fuzzyEquals(weight, 0) {
		return color
	}
	inSpace := color.toSpace(space, true)
	chans := spaceChannels(space)
	var inverted *SassColor
	switch space {
	case spaceHWB:
		inverted = newColor(spaceHWB,
			e.invertChannel(inSpace, chans[0], inSpace.channels()[0]),
			inSpace.channels()[2], inSpace.channels()[1], inSpace.alphaP())
	case spaceHSL, spaceLCH, spaceOklch:
		inverted = newColor(space,
			e.invertChannel(inSpace, chans[0], inSpace.channels()[0]),
			inSpace.channels()[1],
			e.invertChannel(inSpace, chans[2], inSpace.channels()[2]), inSpace.alphaP())
	default:
		inverted = newColor(space,
			e.invertChannel(inSpace, chans[0], inSpace.channels()[0]),
			e.invertChannel(inSpace, chans[1], inSpace.channels()[1]),
			e.invertChannel(inSpace, chans[2], inSpace.channels()[2]), inSpace.alphaP())
	}
	if fuzzyEquals(weight, 1) {
		return inverted.toSpace(color.space, false)
	}
	return color.interpolate(inverted, interpolationMethod{space: space}, 1-weight, false)
}

func (e *evaluator) invertChannel(color *SassColor, ch channelInfo, value *float64) *float64 {
	if value == nil {
		e.fail("Because the CSS working group is still deciding on the best behavior, Sass doesn't currently support modifying missing channels.")
	}
	if ch.polar {
		return pf(dartMod(*value+180, 360))
	}
	if ch.min < 0 {
		return pf(-*value)
	}
	return pf(ch.max - *value)
}

// --- legacy modification functions ---

func (c *SassColor) changeAlpha(alpha *float64) *SassColor {
	return newColor(c.space, c.channels()[0], c.channels()[1], c.channels()[2], alpha)
}

func (c *SassColor) changeHsl(idx int, value float64) *SassColor {
	hsl := c.toSpace(spaceHSL, true)
	ch := hsl.channels()
	ch[idx] = pf(value)
	return newColor(spaceHSL, ch[0], ch[1], ch[2], hsl.alphaP()).toSpace(c.space, true)
}

func fnAdjustHue(ci *callInfo) Value {
	c := ci.color(0)
	deg := ci.e.angleValue(ci.num(1, "degrees"))
	hsl := c.toSpace(spaceHSL, true)
	return c.changeHsl(0, hsl.c0+deg)
}

func fnLighten(ci *callInfo) Value {
	c := ci.color(0)
	amt := ci.num(1, "amount").Val
	hsl := c.toSpace(spaceHSL, true)
	return c.changeHsl(2, clampLikeCss(hsl.c2+amt, 0, 100))
}

func fnDarken(ci *callInfo) Value {
	c := ci.color(0)
	amt := ci.num(1, "amount").Val
	hsl := c.toSpace(spaceHSL, true)
	return c.changeHsl(2, clampLikeCss(hsl.c2-amt, 0, 100))
}

func fnSaturate(ci *callInfo) Value {
	// Single-argument CSS filter overload: saturate(<number|special>), including
	// the named form saturate($amount: 50%). The two-argument form is the legacy
	// colour modifier, so this only applies when exactly one argument is passed.
	if len(ci.positional)+len(ci.named) == 1 {
		var a Value
		if len(ci.positional) == 1 {
			a = ci.positional[0]
		} else {
			for _, v := range ci.named {
				a = v
			}
		}
		if isCssFilterArg(a) {
			return &SassString{Text: functionString("saturate", []Value{a}), Quoted: false}
		}
	}
	c := ci.color(0)
	amt := ci.num(1, "amount").Val
	hsl := c.toSpace(spaceHSL, true)
	return c.changeHsl(1, clampLikeCss(hsl.c1+amt, 0, 100))
}

func fnDesaturate(ci *callInfo) Value {
	c := ci.color(0)
	amt := ci.num(1, "amount").Val
	hsl := c.toSpace(spaceHSL, true)
	return c.changeHsl(1, clampLikeCss(hsl.c1-amt, 0, 100))
}

func fnOpacify(ci *callInfo) Value {
	c := ci.color(0)
	amt := ci.num(1, "amount").Val
	return c.changeAlpha(pf(clampLikeCss(c.alpha+amt, 0, 1)))
}

func fnTransparentize(ci *callInfo) Value {
	c := ci.color(0)
	amt := ci.num(1, "amount").Val
	return c.changeAlpha(pf(clampLikeCss(c.alpha-amt, 0, 1)))
}

// --- interpolation ---

type interpolationMethod struct {
	space ColorSpace
	hue   string // "shorter"|"longer"|"increasing"|"decreasing"; "" means shorter
}

func parseInterpolationMethod(e *evaluator, v Value) interpolationMethod {
	var tokens []string
	collect := func(val Value) {
		if s, ok := val.(*SassString); ok {
			tokens = append(tokens, s.Text)
		}
	}
	if l, ok := v.(*List); ok {
		for _, el := range l.Elements {
			collect(el)
		}
	} else {
		collect(v)
	}
	m := interpolationMethod{hue: "shorter"}
	for _, t := range tokens {
		lt := strings.ToLower(t)
		switch lt {
		case "in", "hue":
			continue
		case "shorter", "longer", "increasing", "decreasing":
			m.hue = lt
		default:
			if sp, ok := colorSpaceFromName(t); ok {
				m.space = sp
			}
		}
	}
	return m
}

func (c *SassColor) interpolate(other *SassColor, method interpolationMethod, weight float64, legacyMissing bool) *SassColor {
	if fuzzyEquals(weight, 0) {
		return other
	}
	if fuzzyEquals(weight, 1) {
		return c
	}
	color1 := c.toSpace(method.space, true)
	color2 := other.toSpace(method.space, true)

	missing1 := [3]bool{
		isAnalogousChannelMissing(c, color1, 0),
		isAnalogousChannelMissing(c, color1, 1),
		isAnalogousChannelMissing(c, color1, 2),
	}
	missing2 := [3]bool{
		isAnalogousChannelMissing(other, color2, 0),
		isAnalogousChannelMissing(other, color2, 1),
		isAnalogousChannelMissing(other, color2, 2),
	}
	pick := func(m bool, a, b *SassColor, i int) float64 {
		if m {
			return chanVal(b, i)
		}
		return chanVal(a, i)
	}
	c1 := [3]float64{pick(missing1[0], color1, color2, 0), pick(missing1[1], color1, color2, 1), pick(missing1[2], color1, color2, 2)}
	c2 := [3]float64{pick(missing2[0], color2, color1, 0), pick(missing2[1], color2, color1, 1), pick(missing2[2], color2, color1, 2)}

	alpha1 := c.alpha
	if c.am {
		alpha1 = other.alpha
	}
	alpha2 := other.alpha
	if other.am {
		alpha2 = c.alpha
	}
	thisMul := 1.0
	if !c.am {
		thisMul = c.alpha
	}
	thisMul *= weight
	otherMul := 1.0
	if !other.am {
		otherMul = other.alpha
	}
	otherMul *= 1 - weight

	var mixedAlpha *float64
	if !(c.am && other.am) {
		mixedAlpha = pf(noFMA(alpha1*weight) + noFMA(alpha2*(1-weight)))
	}
	denom := 1.0
	if mixedAlpha != nil {
		denom = *mixedAlpha
	}
	mixChan := func(i int) *float64 {
		if missing1[i] && missing2[i] {
			return nil
		}
		return pf((noFMA(c1[i]*thisMul) + noFMA(c2[i]*otherMul)) / denom)
	}
	m0, m1, m2 := mixChan(0), mixChan(1), mixChan(2)

	var result *SassColor
	switch method.space {
	case spaceHSL, spaceHWB:
		var h *float64
		if !(missing1[0] && missing2[0]) {
			h = pf(interpolateHues(c1[0], c2[0], method.hue, weight))
		}
		result = newColor(method.space, h, m1, m2, mixedAlpha)
	case spaceLCH, spaceOklch:
		var h *float64
		if !(missing1[2] && missing2[2]) {
			h = pf(interpolateHues(c1[2], c2[2], method.hue, weight))
		}
		result = newColor(method.space, m0, m1, h, mixedAlpha)
	default:
		result = newColor(method.space, m0, m1, m2, mixedAlpha)
	}
	return result.toSpace(c.space, legacyMissing)
}

func chanVal(c *SassColor, i int) float64 {
	switch i {
	case 0:
		return c.c0
	case 1:
		return c.c1
	default:
		return c.c2
	}
}

func isAnalogousChannelMissing(original, output *SassColor, index int) bool {
	if output.channels()[index] == nil {
		return true
	}
	if original == output {
		return false
	}
	outCh := spaceChannels(output.space)[index]
	origChans := spaceChannels(original.space)
	for i := range origChans {
		if channelsAnalogous(outCh, origChans[i]) {
			miss := [3]bool{original.m0, original.m1, original.m2}
			return miss[i]
		}
	}
	return false
}

func channelsAnalogous(a, b channelInfo) bool {
	group := func(name string) string {
		switch name {
		case "red", "x":
			return "red"
		case "green", "y":
			return "green"
		case "blue", "z":
			return "blue"
		case "chroma", "saturation":
			return "chroma"
		case "lightness":
			return "lightness"
		case "hue":
			return "hue"
		}
		return ""
	}
	ga := group(a.name)
	return ga != "" && ga == group(b.name)
}

func interpolateHues(hue1, hue2 float64, method string, weight float64) float64 {
	switch method {
	case "longer":
		d := hue2 - hue1
		if d > 0 && d < 180 {
			hue2 += 360
		} else if d > -180 && d <= 0 {
			hue1 += 360
		}
	case "increasing":
		if hue2 < hue1 {
			hue2 += 360
		}
	case "decreasing":
		if hue1 < hue2 {
			hue1 += 360
		}
	default: // shorter
		d := hue2 - hue1
		if d > 180 {
			hue1 += 360
		} else if d < -180 {
			hue2 += 360
		}
	}
	return noFMA(hue1*weight) + noFMA(hue2*(1-weight))
}

// --- ie-hex-str ---

func fnIEHexStr(ci *callInfo) Value {
	c := ci.color(0).toSpace(spaceRGB, true).toGamut("local-minde")
	hexString := func(component float64) string {
		v := int(fuzzyRound(component))
		return strings.ToUpper(twoHex(v))
	}
	return &SassString{Text: "#" + hexString(c.alpha*255) + hexString(c.c0) + hexString(c.c1) + hexString(c.c2), Quoted: false}
}

func twoHex(v int) string {
	const digits = "0123456789abcdef"
	return string([]byte{digits[(v/16)&0xf], digits[v&0xf]})
}
