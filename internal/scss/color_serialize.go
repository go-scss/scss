// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import (
	"math"
	"strings"
)

// serializeColor renders a color to CSS, matching dart-sass's visitColor.
func serializeColor(c *SassColor, compressed, inspect bool) string {
	var sb strings.Builder
	writeColor(&sb, c, compressed, inspect)
	return sb.String()
}

func writeColor(sb *strings.Builder, c *SassColor, compressed, inspect bool) {
	switch c.space {
	case spaceRGB, spaceHSL, spaceHWB:
		if !c.m0 && !c.m1 && !c.m2 && !c.am {
			writeLegacyColor(sb, c, compressed, inspect)
			return
		}
		if c.space == spaceRGB {
			sb.WriteString("rgb(")
			writeChannel(sb, chanP(c, 0), "", compressed, inspect)
			sb.WriteByte(' ')
			writeChannel(sb, chanP(c, 1), "", compressed, inspect)
			sb.WriteByte(' ')
			writeChannel(sb, chanP(c, 2), "", compressed, inspect)
			maybeWriteSlashAlpha(sb, c, compressed, inspect)
			sb.WriteByte(')')
			return
		}
		sb.WriteString(string(c.space))
		sb.WriteByte('(')
		hueUnit := "deg"
		if compressed {
			hueUnit = ""
		}
		writeChannel(sb, chanP(c, 0), hueUnit, compressed, inspect)
		sb.WriteByte(' ')
		writeChannel(sb, chanP(c, 1), "%", compressed, inspect)
		sb.WriteByte(' ')
		writeChannel(sb, chanP(c, 2), "%", compressed, inspect)
		maybeWriteSlashAlpha(sb, c, compressed, inspect)
		sb.WriteByte(')')
		return
	}

	if !inspect && isColorMixCase(c) {
		sb.WriteString("color-mix(in ")
		sb.WriteString(string(c.space))
		sb.WriteString(commaSep(compressed))
		writeColorFunction(sb, c.toSpace(spaceXYZD65, true), compressed, inspect)
		if !compressed {
			sb.WriteByte(' ')
		}
		sb.WriteString("100%")
		sb.WriteString(commaSep(compressed))
		if compressed {
			sb.WriteString("red")
		} else {
			sb.WriteString("black")
		}
		sb.WriteByte(')')
		return
	}

	switch c.space {
	case spaceLab, spaceOklab, spaceLCH, spaceOklch:
		writeLabLike(sb, c, compressed, inspect)
	default:
		writeColorFunction(sb, c, compressed, inspect)
	}
}

// isColorMixCase reports whether an out-of-gamut lab/lch/oklab/oklch color must
// be serialized via color-mix() rather than its own function.
func isColorMixCase(c *SassColor) bool {
	switch c.space {
	case spaceLab, spaceLCH:
		if !fuzzyInRange(c.c0, 0, 100) && !c.m1 && !c.m2 {
			return true
		}
	case spaceOklab, spaceOklch:
		if !fuzzyInRange(c.c0, 0, 1) && !c.m1 && !c.m2 {
			return true
		}
	}
	switch c.space {
	case spaceLCH, spaceOklch:
		if fuzzyLessThan(c.c1, 0) && !c.m0 && !c.m1 {
			return true
		}
	}
	return false
}

func writeLabLike(sb *strings.Builder, c *SassColor, compressed, inspect bool) {
	sb.WriteString(string(c.space))
	sb.WriteByte('(')
	chans := spaceChannels(c.space)
	polar := chans[2].polar

	if !inspect && (!fuzzyInRange(c.c0, 0, 100) || (polar && fuzzyLessThan(c.c1, 0))) {
		sb.WriteString("from ")
		if compressed {
			sb.WriteString("red")
		} else {
			sb.WriteString("black")
		}
		sb.WriteByte(' ')
	}

	if !compressed && !c.m0 {
		max := chans[0].max
		writeNum(sb, c.c0*100/max, compressed, inspect)
		sb.WriteByte('%')
	} else {
		writeChannel(sb, chanP(c, 0), "", compressed, inspect)
	}
	sb.WriteByte(' ')
	writeChannel(sb, chanP(c, 1), "", compressed, inspect)
	sb.WriteByte(' ')
	unit := ""
	if polar && !compressed {
		unit = "deg"
	}
	writeChannel(sb, chanP(c, 2), unit, compressed, inspect)
	maybeWriteSlashAlpha(sb, c, compressed, inspect)
	sb.WriteByte(')')
}

func writeColorFunction(sb *strings.Builder, c *SassColor, compressed, inspect bool) {
	sb.WriteString("color(")
	sb.WriteString(string(c.space))
	sb.WriteByte(' ')
	writeChannel(sb, chanP(c, 0), "", compressed, inspect)
	sb.WriteByte(' ')
	writeChannel(sb, chanP(c, 1), "", compressed, inspect)
	sb.WriteByte(' ')
	writeChannel(sb, chanP(c, 2), "", compressed, inspect)
	maybeWriteSlashAlpha(sb, c, compressed, inspect)
	sb.WriteByte(')')
}

func chanP(c *SassColor, i int) *float64 {
	return c.channels()[i]
}

func writeChannel(sb *strings.Builder, channel *float64, unit string, compressed, inspect bool) {
	if channel == nil {
		sb.WriteString("none")
		return
	}
	if math.IsInf(*channel, 0) || math.IsNaN(*channel) {
		sb.WriteString(numberCalcRepr(&Number{Val: *channel, Numer: unitNumer(unit)}))
		return
	}
	writeNum(sb, *channel, compressed, inspect)
	sb.WriteString(unit)
}

func unitNumer(unit string) []string {
	if unit == "" {
		return nil
	}
	return []string{unit}
}

func writeNum(sb *strings.Builder, v float64, compressed, inspect bool) {
	sb.WriteString(formatFloat(v, compressed))
}

func maybeWriteSlashAlpha(sb *strings.Builder, c *SassColor, compressed, inspect bool) {
	if !c.am && fuzzyEquals(c.alpha, 1) {
		return
	}
	if !compressed {
		sb.WriteByte(' ')
	}
	sb.WriteByte('/')
	if !compressed {
		sb.WriteByte(' ')
	}
	writeChannel(sb, c.alphaP(), "", compressed, inspect)
}

func commaSep(compressed bool) string {
	if compressed {
		return ","
	}
	return ", "
}

// --- legacy color serialization ---

func writeLegacyColor(sb *strings.Builder, c *SassColor, compressed, inspect bool) {
	opaque := fuzzyEquals(c.alpha, 1)

	if !c.isInGamut() && !inspect {
		writeHsl(sb, c, compressed)
		return
	}

	if compressed {
		rgb := c.toSpace(spaceRGB, true)
		if opaque && tryHexOrNamedRgb(sb, rgb) {
			return
		}
		var rgbBuf, hslBuf strings.Builder
		writeRgb(&rgbBuf, rgb, compressed)
		writeHsl(&hslBuf, rgb.toSpace(spaceHSL, true), compressed)
		if rgbBuf.Len() <= hslBuf.Len()+2 {
			sb.WriteString(rgbBuf.String())
		} else {
			sb.WriteString(hslBuf.String())
		}
		return
	}

	if c.space == spaceHSL {
		writeHsl(sb, c, compressed)
		return
	}
	if inspect && c.space == spaceHWB {
		writeHwb(sb, c)
		return
	}

	switch c.format {
	case fmtRgbFunction:
		writeRgb(sb, c, compressed)
		return
	case fmtSpan:
		sb.WriteString(c.original)
		return
	}

	if opaque {
		rgb := c.toSpace(spaceRGB, true)
		if name, ok := namesByColor(rgb); ok {
			sb.WriteString(name)
			return
		}
		if canUseHex(rgb) {
			sb.WriteByte('#')
			writeHexComponent(sb, int(math.Round(rgb.c0)))
			writeHexComponent(sb, int(math.Round(rgb.c1)))
			writeHexComponent(sb, int(math.Round(rgb.c2)))
			return
		}
	}

	if c.space == spaceHWB {
		writeHsl(sb, c, compressed)
	} else {
		writeRgb(sb, c, compressed)
	}
}

func tryHexOrNamedRgb(sb *strings.Builder, rgb *SassColor) bool {
	if !canUseHex(rgb) {
		return false
	}
	redInt := int(math.Round(rgb.c0))
	greenInt := int(math.Round(rgb.c1))
	blueInt := int(math.Round(rgb.c2))
	shortHex := canUseShortHex(redInt, greenInt, blueInt)
	nameLen := 7
	if shortHex {
		nameLen = 4
	}
	if name, ok := namesByColor(rgb); ok && len(name) <= nameLen {
		sb.WriteString(name)
	} else if shortHex {
		sb.WriteByte('#')
		sb.WriteByte(hexCharFor(redInt & 0xF))
		sb.WriteByte(hexCharFor(greenInt & 0xF))
		sb.WriteByte(hexCharFor(blueInt & 0xF))
	} else {
		sb.WriteByte('#')
		writeHexComponent(sb, redInt)
		writeHexComponent(sb, greenInt)
		writeHexComponent(sb, blueInt)
	}
	return true
}

func canUseHex(rgb *SassColor) bool {
	return canUseHexForChannel(rgb.c0) && canUseHexForChannel(rgb.c1) && canUseHexForChannel(rgb.c2)
}

func canUseHexForChannel(channel float64) bool {
	return fuzzyIsIntC(channel) && fuzzyGreaterThanOrEquals(channel, 0) && fuzzyLessThan(channel, 256)
}

func writeRgb(sb *strings.Builder, c *SassColor, compressed bool) {
	opaque := fuzzyEquals(c.alpha, 1)
	rgb := c.toSpace(spaceRGB, true)
	if opaque {
		sb.WriteString("rgb(")
	} else {
		sb.WriteString("rgba(")
	}
	if !tryIntegerRgbChannels(sb, rgb, compressed) {
		writeChannel(sb, pf(rgb.c0*100/255), "%", compressed, false)
		sb.WriteString(commaSep(compressed))
		writeChannel(sb, pf(rgb.c1*100/255), "%", compressed, false)
		sb.WriteString(commaSep(compressed))
		writeChannel(sb, pf(rgb.c2*100/255), "%", compressed, false)
	}
	if !opaque {
		sb.WriteString(commaSep(compressed))
		writeNum(sb, c.alpha, compressed, false)
	}
	sb.WriteByte(')')
}

func tryIntegerRgbChannels(sb *strings.Builder, rgb *SassColor, compressed bool) bool {
	r, ok := asExactInt(rgb.c0)
	if !ok {
		return false
	}
	g, ok := asExactInt(rgb.c1)
	if !ok {
		return false
	}
	b, ok := asExactInt(rgb.c2)
	if !ok {
		return false
	}
	writeIntStr(sb, r)
	sb.WriteString(commaSep(compressed))
	writeIntStr(sb, g)
	sb.WriteString(commaSep(compressed))
	writeIntStr(sb, b)
	return true
}

func writeHsl(sb *strings.Builder, c *SassColor, compressed bool) {
	opaque := fuzzyEquals(c.alpha, 1)
	hsl := c.toSpace(spaceHSL, true)
	if opaque {
		sb.WriteString("hsl(")
	} else {
		sb.WriteString("hsla(")
	}
	writeChannel(sb, pf(hsl.c0), "", compressed, false)
	sb.WriteString(commaSep(compressed))
	writeChannel(sb, pf(hsl.c1), "%", compressed, false)
	sb.WriteString(commaSep(compressed))
	writeChannel(sb, pf(hsl.c2), "%", compressed, false)
	if !opaque {
		sb.WriteString(commaSep(compressed))
		writeNum(sb, c.alpha, compressed, false)
	}
	sb.WriteByte(')')
}

func writeHwb(sb *strings.Builder, c *SassColor) {
	sb.WriteString("hwb(")
	hwb := c.toSpace(spaceHWB, true)
	writeNum(sb, hwb.c0, false, true)
	sb.WriteByte(' ')
	writeNum(sb, hwb.c1, false, true)
	sb.WriteByte('%')
	sb.WriteByte(' ')
	writeNum(sb, hwb.c2, false, true)
	sb.WriteByte('%')
	if !fuzzyEquals(c.alpha, 1) {
		sb.WriteString(" / ")
		writeNum(sb, c.alpha, false, true)
	}
	sb.WriteByte(')')
}

func namesByColor(rgb *SassColor) (string, bool) {
	if !fuzzyIsIntC(rgb.c0) || !fuzzyIsIntC(rgb.c1) || !fuzzyIsIntC(rgb.c2) {
		return "", false
	}
	key := [3]int{int(math.Round(rgb.c0)), int(math.Round(rgb.c1)), int(math.Round(rgb.c2))}
	name, ok := rgbToName[key]
	return name, ok
}

func isSymmetricalHex(color int) bool { return color&0xF == color>>4 }

func canUseShortHex(r, g, b int) bool {
	return isSymmetricalHex(r) && isSymmetricalHex(g) && isSymmetricalHex(b)
}

func writeHexComponent(sb *strings.Builder, color int) {
	sb.WriteByte(hexCharFor(color >> 4))
	sb.WriteByte(hexCharFor(color & 0xF))
}

func hexCharFor(n int) byte {
	if n < 10 {
		return byte('0' + n)
	}
	return byte('a' + n - 10)
}

// asExactInt returns v as an int only if v is exactly an integer.
func asExactInt(v float64) (int, bool) {
	r := math.Round(v)
	if r == v && !math.IsInf(v, 0) {
		return int(r), true
	}
	return 0, false
}

func writeIntStr(sb *strings.Builder, v int) {
	sb.WriteString(formatFloat(float64(v), false))
}
