// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import (
	"sort"
	"strings"
)

// unitFactors maps a unit to (canonical unit, factor-to-canonical) within its
// dimension. Two units are compatible iff they share a canonical unit.
var unitFactors = map[string]struct {
	canonical string
	factor    float64
}{
	// length (canonical: px)
	"px": {"px", 1},
	"cm": {"px", 96.0 / 2.54},
	"mm": {"px", 96.0 / 25.4},
	"q":  {"px", 96.0 / 101.6},
	"in": {"px", 96},
	"pt": {"px", 96.0 / 72.0},
	"pc": {"px", 16},
	// angle (canonical: deg)
	"deg":  {"deg", 1},
	"grad": {"deg", 360.0 / 400.0},
	"rad":  {"deg", 180.0 / 3.141592653589793},
	"turn": {"deg", 360},
	// time (canonical: s)
	"s":  {"s", 1},
	"ms": {"s", 1.0 / 1000.0},
	// frequency (canonical: hz)
	"hz":  {"hz", 1},
	"khz": {"hz", 1000},
	// resolution (canonical: dppx)
	"dpi":  {"dppx", 1.0 / 96.0},
	"dpcm": {"dppx", 2.54 / 96.0},
	"dppx": {"dppx", 1},
}

func canonicalOf(unit string) (string, float64, bool) {
	f, ok := unitFactors[strings.ToLower(unit)]
	if !ok {
		return "", 0, false
	}
	return f.canonical, f.factor, true
}

// compatConvert returns n.Val expressed in o's units when the two numbers are
// unit-compatible. The bool is false when they cannot be converted.
func (n *Number) compatConvert(o *Number) (float64, bool) {
	if unitsEqual(n.Numer, o.Numer) && unitsEqual(n.Denom, o.Denom) {
		return n.Val, true
	}
	if !n.hasUnits() || !o.hasUnits() {
		// A unitless number is compatible with any single-unit number for ==.
		return n.Val, true
	}
	v, ok := convertUnits(n.Val, n.Numer, n.Denom, o.Numer, o.Denom)
	return v, ok
}

func unitsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	as := append([]string(nil), a...)
	bs := append([]string(nil), b...)
	sort.Strings(as)
	sort.Strings(bs)
	for i := range as {
		if as[i] != bs[i] {
			return false
		}
	}
	return true
}

// convertUnits converts value with (numer/denom) units into (toNumer/toDenom).
func convertUnits(value float64, numer, denom, toNumer, toDenom []string) (float64, bool) {
	v, ok := convertUnitList(value, numer, toNumer, true)
	if !ok {
		return 0, false
	}
	return convertUnitList(v, denom, toDenom, false)
}

func convertUnitList(value float64, from, to []string, multiply bool) (float64, bool) {
	if len(from) != len(to) {
		return 0, false
	}
	remaining := append([]string(nil), to...)
	for _, u := range from {
		canon, factor, ok := canonicalOf(u)
		if !ok {
			// Unknown unit: must match exactly.
			idx := indexOf(remaining, u)
			if idx < 0 {
				return 0, false
			}
			remaining = removeAt(remaining, idx)
			continue
		}
		matched := -1
		for i, t := range remaining {
			tc, tf, tok := canonicalOf(t)
			if tok && tc == canon {
				matched = i
				if multiply {
					value = value * factor / tf
				} else {
					value = value / factor * tf
				}
				break
			}
		}
		if matched < 0 {
			return 0, false
		}
		remaining = removeAt(remaining, matched)
	}
	return value, true
}

func indexOf(s []string, v string) int {
	for i := range s {
		if s[i] == v {
			return i
		}
	}
	return -1
}

func removeAt(s []string, i int) []string {
	out := append([]string(nil), s[:i]...)
	return append(out, s[i+1:]...)
}

// simplifyUnits cancels matching numerator/denominator units, converting
// compatible pairs. Returns the (possibly rescaled) value plus reduced units.
func simplifyUnits(value float64, numer, denom []string) (float64, []string, []string) {
	num := append([]string(nil), numer...)
	den := append([]string(nil), denom...)
	for i := 0; i < len(num); i++ {
		for j := 0; j < len(den); j++ {
			if num[i] == den[j] {
				num = removeAt(num, i)
				den = removeAt(den, j)
				i--
				break
			}
			nc, nf, nok := canonicalOf(num[i])
			dc, df, dok := canonicalOf(den[j])
			if nok && dok && nc == dc {
				value = value * nf / df
				num = removeAt(num, i)
				den = removeAt(den, j)
				i--
				break
			}
		}
	}
	return value, num, den
}
