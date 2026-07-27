// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import (
	"fmt"
	"math"
	"strings"
)

// namedColors maps CSS color keywords to their RGB triples.
var namedColors = map[string][3]int{
	"aliceblue": {240, 248, 255}, "antiquewhite": {250, 235, 215}, "aqua": {0, 255, 255},
	"aquamarine": {127, 255, 212}, "azure": {240, 255, 255}, "beige": {245, 245, 220},
	"bisque": {255, 228, 196}, "black": {0, 0, 0}, "blanchedalmond": {255, 235, 205},
	"blue": {0, 0, 255}, "blueviolet": {138, 43, 226}, "brown": {165, 42, 42},
	"burlywood": {222, 184, 135}, "cadetblue": {95, 158, 160}, "chartreuse": {127, 255, 0},
	"chocolate": {210, 105, 30}, "coral": {255, 127, 80}, "cornflowerblue": {100, 149, 237},
	"cornsilk": {255, 248, 220}, "crimson": {220, 20, 60}, "cyan": {0, 255, 255},
	"darkblue": {0, 0, 139}, "darkcyan": {0, 139, 139}, "darkgoldenrod": {184, 134, 11},
	"darkgray": {169, 169, 169}, "darkgreen": {0, 100, 0}, "darkgrey": {169, 169, 169},
	"darkkhaki": {189, 183, 107}, "darkmagenta": {139, 0, 139}, "darkolivegreen": {85, 107, 47},
	"darkorange": {255, 140, 0}, "darkorchid": {153, 50, 204}, "darkred": {139, 0, 0},
	"darksalmon": {233, 150, 122}, "darkseagreen": {143, 188, 143}, "darkslateblue": {72, 61, 139},
	"darkslategray": {47, 79, 79}, "darkslategrey": {47, 79, 79}, "darkturquoise": {0, 206, 209},
	"darkviolet": {148, 0, 211}, "deeppink": {255, 20, 147}, "deepskyblue": {0, 191, 255},
	"dimgray": {105, 105, 105}, "dimgrey": {105, 105, 105}, "dodgerblue": {30, 144, 255},
	"firebrick": {178, 34, 34}, "floralwhite": {255, 250, 240}, "forestgreen": {34, 139, 34},
	"fuchsia": {255, 0, 255}, "gainsboro": {220, 220, 220}, "ghostwhite": {248, 248, 255},
	"gold": {255, 215, 0}, "goldenrod": {218, 165, 32}, "gray": {128, 128, 128},
	"green": {0, 128, 0}, "greenyellow": {173, 255, 47}, "grey": {128, 128, 128},
	"honeydew": {240, 255, 240}, "hotpink": {255, 105, 180}, "indianred": {205, 92, 92},
	"indigo": {75, 0, 130}, "ivory": {255, 255, 240}, "khaki": {240, 230, 140},
	"lavender": {230, 230, 250}, "lavenderblush": {255, 240, 245}, "lawngreen": {124, 252, 0},
	"lemonchiffon": {255, 250, 205}, "lightblue": {173, 216, 230}, "lightcoral": {240, 128, 128},
	"lightcyan": {224, 255, 255}, "lightgoldenrodyellow": {250, 250, 210}, "lightgray": {211, 211, 211},
	"lightgreen": {144, 238, 144}, "lightgrey": {211, 211, 211}, "lightpink": {255, 182, 193},
	"lightsalmon": {255, 160, 122}, "lightseagreen": {32, 178, 170}, "lightskyblue": {135, 206, 250},
	"lightslategray": {119, 136, 153}, "lightslategrey": {119, 136, 153}, "lightsteelblue": {176, 196, 222},
	"lightyellow": {255, 255, 224}, "lime": {0, 255, 0}, "limegreen": {50, 205, 50},
	"linen": {250, 240, 230}, "magenta": {255, 0, 255}, "maroon": {128, 0, 0},
	"mediumaquamarine": {102, 205, 170}, "mediumblue": {0, 0, 205}, "mediumorchid": {186, 85, 211},
	"mediumpurple": {147, 112, 219}, "mediumseagreen": {60, 179, 113}, "mediumslateblue": {123, 104, 238},
	"mediumspringgreen": {0, 250, 154}, "mediumturquoise": {72, 209, 204}, "mediumvioletred": {199, 21, 133},
	"midnightblue": {25, 25, 112}, "mintcream": {245, 255, 250}, "mistyrose": {255, 228, 225},
	"moccasin": {255, 228, 181}, "navajowhite": {255, 222, 173}, "navy": {0, 0, 128},
	"oldlace": {253, 245, 230}, "olive": {128, 128, 0}, "olivedrab": {107, 142, 35},
	"orange": {255, 165, 0}, "orangered": {255, 69, 0}, "orchid": {218, 112, 214},
	"palegoldenrod": {238, 232, 170}, "palegreen": {152, 251, 152}, "paleturquoise": {175, 238, 238},
	"palevioletred": {219, 112, 147}, "papayawhip": {255, 239, 213}, "peachpuff": {255, 218, 185},
	"peru": {205, 133, 63}, "pink": {255, 192, 203}, "plum": {221, 160, 221},
	"powderblue": {176, 224, 230}, "purple": {128, 0, 128}, "rebeccapurple": {102, 51, 153},
	"red": {255, 0, 0}, "rosybrown": {188, 143, 143}, "royalblue": {65, 105, 225},
	"saddlebrown": {139, 69, 19}, "salmon": {250, 128, 114}, "sandybrown": {244, 164, 96},
	"seagreen": {46, 139, 87}, "seashell": {255, 245, 238}, "sienna": {160, 82, 45},
	"silver": {192, 192, 192}, "skyblue": {135, 206, 235}, "slateblue": {106, 90, 205},
	"slategray": {112, 128, 144}, "slategrey": {112, 128, 144}, "snow": {255, 250, 250},
	"springgreen": {0, 255, 127}, "steelblue": {70, 130, 180}, "tan": {210, 180, 140},
	"teal": {0, 128, 128}, "thistle": {216, 191, 216}, "tomato": {255, 99, 71},
	"turquoise": {64, 224, 208}, "violet": {238, 130, 238}, "wheat": {245, 222, 179},
	"white": {255, 255, 255}, "whitesmoke": {245, 245, 245}, "yellow": {255, 255, 0},
	"yellowgreen": {154, 205, 50},
}

// rgbToName maps an RGB triple to the shortest CSS keyword, matching dart-sass's
// choice for compressed output.
var rgbToName = buildRGBToName()

func buildRGBToName() map[[3]int]string {
	m := make(map[[3]int]string)
	// Iterate deterministically over a preferred-order list so the shortest name
	// wins ties the way dart-sass does (e.g. aqua over cyan, magenta over fuchsia).
	for name, rgb := range namedColors {
		if cur, ok := m[rgb]; !ok || len(name) < len(cur) || (len(name) == len(cur) && name < cur) {
			m[rgb] = name
		}
	}
	return m
}

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
	// A hex literal serializes verbatim in expanded output only when opaque;
	// an alpha component forces the canonical rgba()/hex form dart-sass uses.
	orig := hex
	if a != 1 {
		orig = ""
	}
	return &SassColor{Rf: float64(r), Gf: float64(g), Bf: float64(b), A: a, Original: orig}
}

func hexPair(s string) int {
	v := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		var d int
		switch {
		case c >= '0' && c <= '9':
			d = int(c - '0')
		case c >= 'a' && c <= 'f':
			d = int(c-'a') + 10
		default:
			d = int(c-'A') + 10
		}
		v = v*16 + d
	}
	return v
}

func clampAlpha(a float64) float64 {
	if a < 0 {
		return 0
	}
	if a > 1 {
		return 1
	}
	return a
}

// clampChannelF clamps a float channel to the 0..255 range without rounding.
func clampChannelF(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return v
}

// fuzzyIsInt reports whether v is (within dart-sass tolerance) a whole number.
func fuzzyIsInt(v float64) bool {
	return math.Abs(v-math.Round(v)) < 1e-11
}

// rgbColor builds a color from evaluated rgb()/rgba() arguments. It serializes in
// rgb() form (never keyword-substituted) in expanded output.
func rgbColor(r, g, b, a float64) *SassColor {
	return &SassColor{Rf: clampChannelF(r), Gf: clampChannelF(g), Bf: clampChannelF(b), A: clampAlpha(a), rgbFmt: true}
}

// computedColor builds a color with no source format, so it serialises as a CSS
// keyword or hex when its channels are integers, or rgb()/rgba() percentages when
// any channel is fractional — matching modern dart-sass.
func computedColor(r, g, b, a float64) *SassColor {
	return &SassColor{Rf: clampChannelF(r), Gf: clampChannelF(g), Bf: clampChannelF(b), A: clampAlpha(a)}
}

func (c *SassColor) allInt() bool {
	return fuzzyIsInt(c.Rf) && fuzzyIsInt(c.Gf) && fuzzyIsInt(c.Bf)
}

func hexOf(r, g, b int) string {
	if r%17 == 0 && g%17 == 0 && b%17 == 0 {
		return fmt.Sprintf("#%x%x%x", r/17, g/17, b/17)
	}
	return fmt.Sprintf("#%02x%02x%02x", r, g, b)
}

func pctChannel(v float64, compressed bool) string {
	return formatFloat(v/255*100, compressed) + "%"
}

// rgbFuncRepr serializes the color in rgb()/rgba() form: integer channels as
// integers, fractional channels as percentages.
func (c *SassColor) rgbFuncRepr(compressed bool) string {
	sp := " "
	if compressed {
		sp = ""
	}
	var rs, gs, bs string
	if c.allInt() {
		rs = fmt.Sprintf("%d", c.Ri())
		gs = fmt.Sprintf("%d", c.Gi())
		bs = fmt.Sprintf("%d", c.Bi())
	} else {
		rs = pctChannel(c.Rf, compressed)
		gs = pctChannel(c.Gf, compressed)
		bs = pctChannel(c.Bf, compressed)
	}
	if c.A == 1 {
		return fmt.Sprintf("rgb(%s,%s%s,%s%s)", rs, sp, gs, sp, bs)
	}
	return fmt.Sprintf("rgba(%s,%s%s,%s%s,%s%s)", rs, sp, gs, sp, bs, sp, formatFloat(c.A, compressed))
}

// expandedRepr returns the expanded-mode serialization.
func (c *SassColor) expandedRepr() string {
	if c.Original != "" {
		return c.Original
	}
	if c.rgbFmt {
		return c.rgbFuncRepr(false)
	}
	// computed color
	if c.allInt() {
		rgb := [3]int{c.Ri(), c.Gi(), c.Bi()}
		if c.A == 1 {
			if name, ok := rgbToName[rgb]; ok {
				return name
			}
			return hexOf(rgb[0], rgb[1], rgb[2])
		}
	}
	return c.rgbFuncRepr(false)
}

// compressedRepr returns the shortest valid serialization, independent of the
// source format (dart-sass collapses every color to its canonical shortest form).
func (c *SassColor) compressedRepr() string {
	if c.allInt() && c.A == 1 {
		hex := hexOf(c.Ri(), c.Gi(), c.Bi())
		if name, ok := rgbToName[[3]int{c.Ri(), c.Gi(), c.Bi()}]; ok && len(name) <= len(hex) {
			return name
		}
		return hex
	}
	return c.rgbFuncRepr(true)
}

// hsl conversion helpers (used by hsl()/color functions).

func rgbToHSL(r, g, b float64) (h, s, l float64) {
	rf := r / 255.0
	gf := g / 255.0
	bf := b / 255.0
	max := math.Max(rf, math.Max(gf, bf))
	min := math.Min(rf, math.Min(gf, bf))
	l = (max + min) / 2
	if max == min {
		return 0, 0, l * 100
	}
	d := max - min
	if l > 0.5 {
		s = d / (2 - max - min)
	} else {
		s = d / (max + min)
	}
	switch max {
	case rf:
		h = (gf - bf) / d
		if gf < bf {
			h += 6
		}
	case gf:
		h = (bf-rf)/d + 2
	default:
		h = (rf-gf)/d + 4
	}
	h *= 60
	return h, s * 100, l * 100
}

func hslToRGB(h, s, l float64) (float64, float64, float64) {
	h = math.Mod(h, 360)
	if h < 0 {
		h += 360
	}
	s /= 100
	l /= 100
	c := (1 - math.Abs(2*l-1)) * s
	x := c * (1 - math.Abs(math.Mod(h/60, 2)-1))
	m := l - c/2
	var rf, gf, bf float64
	switch {
	case h < 60:
		rf, gf, bf = c, x, 0
	case h < 120:
		rf, gf, bf = x, c, 0
	case h < 180:
		rf, gf, bf = 0, c, x
	case h < 240:
		rf, gf, bf = 0, x, c
	case h < 300:
		rf, gf, bf = x, 0, c
	default:
		rf, gf, bf = c, 0, x
	}
	return clampChannelF((rf + m) * 255), clampChannelF((gf + m) * 255), clampChannelF((bf + m) * 255)
}

func hslColor(h, s, l, a float64) *SassColor {
	r, g, b := hslToRGB(h, s, l)
	c := &SassColor{Rf: r, Gf: g, Bf: b, A: clampAlpha(a)}
	sp := " "
	if a == 1 {
		c.Original = fmt.Sprintf("hsl(%s,%s%s%%,%s%s%%)", formatFloat(normalizeHue(h), false), sp, formatFloat(s, false), sp, formatFloat(l, false))
	} else {
		c.Original = fmt.Sprintf("hsla(%s,%s%s%%,%s%s%%,%s%s)", formatFloat(normalizeHue(h), false), sp, formatFloat(s, false), sp, formatFloat(l, false), sp, formatFloat(a, false))
	}
	return c
}

func normalizeHue(h float64) float64 {
	h = math.Mod(h, 360)
	if h < 0 {
		h += 360
	}
	return h
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
	return &SassColor{Rf: float64(rgb[0]), Gf: float64(rgb[1]), Bf: float64(rgb[2]), A: 1, Original: name}, true
}
