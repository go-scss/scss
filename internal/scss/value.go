// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import (
	"math"
	"math/big"
	"strconv"
	"strings"
)

// Separator identifies how the elements of a list are joined.
type Separator int

const (
	SepUndecided Separator = iota
	SepSpace
	SepComma
	SepSlash
)

// Value is any Sass runtime value.
type Value interface {
	// isTruthy reports whether the value is truthy (everything except false and null).
	isTruthy() bool
	// equals reports value equality per Sass ==.
	equals(Value) bool
	// sep returns the value's list separator (scalars are undecided/space).
	sep() Separator
	// asList returns the value coerced to a slice of elements.
	asList() []Value
}

// Number is a numeric value with optional units.
type Number struct {
	Val   float64
	Numer []string
	Denom []string
	// slashL/slashR carry the "as-slash" provenance dart-sass tracks for a
	// number produced by a literal "a/b" division of two number literals: the
	// value is the quotient, but the pair remembers the original operands so it
	// serializes back as "a/b" (recursively, so 1/2/3 stays flat). The
	// provenance is stripped by withoutSlash the moment the number is consumed
	// (arithmetic, a variable/argument/config binding, a function-call result,
	// or a parenthesized grouping), whereupon it serializes as the quotient.
	slashL *Number
	slashR *Number
}

// withoutSlash returns n stripped of any as-slash provenance. A number with no
// provenance is returned unchanged; otherwise a shallow copy with the provenance
// cleared is returned so the original (possibly shared) value is untouched.
func (n *Number) withoutSlash() *Number {
	if n.slashL == nil && n.slashR == nil {
		return n
	}
	m := *n
	m.slashL = nil
	m.slashR = nil
	return &m
}

// numWithoutSlash strips as-slash provenance from a value when it is a Number,
// mirroring dart-sass's Value.withoutSlash: only SassNumber overrides it, so
// lists (and their elements) pass through unchanged, which is how a slash inside
// a list survives being bound or returned while a bare slash number does not.
func numWithoutSlash(v Value) Value {
	if n, ok := v.(*Number); ok {
		return n.withoutSlash()
	}
	return v
}

func newNumber(v float64, units ...string) *Number {
	n := &Number{Val: v}
	if len(units) == 1 && units[0] != "" {
		n.Numer = []string{units[0]}
	}
	return n
}

func (n *Number) isTruthy() bool  { return true }
func (n *Number) sep() Separator  { return SepUndecided }
func (n *Number) asList() []Value { return []Value{n} }
func (n *Number) equals(o Value) bool {
	on, ok := o.(*Number)
	if !ok {
		return false
	}
	// Equality is unit-strict, matching dart-sass's SassNumber ==: a unitless
	// number equals only another unitless number, and a number with units
	// equals another only when their units are mutually convertible. This is
	// stricter than the arithmetic/comparison path (compatConvert), where a
	// unitless operand is treated as compatible with any unit.
	if n.hasUnits() != on.hasUnits() {
		return false
	}
	a, aok := convertUnits(n.Val, n.Numer, n.Denom, on.Numer, on.Denom)
	if !aok {
		return false
	}
	return fuzzyEquals(a, on.Val)
}

func (n *Number) hasUnits() bool { return len(n.Numer) > 0 || len(n.Denom) > 0 }

func (n *Number) unitString() string {
	if len(n.Numer) == 0 && len(n.Denom) == 0 {
		return ""
	}
	num := strings.Join(n.Numer, "*")
	if len(n.Denom) == 0 {
		return num
	}
	den := strings.Join(n.Denom, "*")
	if len(n.Denom) > 1 {
		den = "(" + den + ")"
	}
	// A pure-denominator unit (no numerators) serialises with a negative
	// exponent, matching dart-sass: "px^-1", "(px*em*rad)^-1".
	if num == "" {
		return den + "^-1"
	}
	return num + "/" + den
}

// SassString is a text value, quoted or unquoted.
type SassString struct {
	Text   string
	Quoted bool
}

func (s *SassString) isTruthy() bool  { return true }
func (s *SassString) sep() Separator  { return SepUndecided }
func (s *SassString) asList() []Value { return []Value{s} }
func (s *SassString) equals(o Value) bool {
	os, ok := o.(*SassString)
	if !ok {
		return false
	}
	return s.Text == os.Text
}

// Boolean is a Sass true/false.
type Boolean struct{ V bool }

func (b *Boolean) isTruthy() bool  { return b.V }
func (b *Boolean) sep() Separator  { return SepUndecided }
func (b *Boolean) asList() []Value { return []Value{b} }
func (b *Boolean) equals(o Value) bool {
	ob, ok := o.(*Boolean)
	return ok && b.V == ob.V
}

var (
	sassTrue  = &Boolean{V: true}
	sassFalse = &Boolean{V: false}
)

func boolean(v bool) *Boolean {
	if v {
		return sassTrue
	}
	return sassFalse
}

// Null is the Sass null value.
type Null struct{}

var sassNull = &Null{}

func (n *Null) isTruthy() bool      { return false }
func (n *Null) sep() Separator      { return SepUndecided }
func (n *Null) asList() []Value     { return []Value{n} }
func (n *Null) equals(o Value) bool { _, ok := o.(*Null); return ok }

// List is an ordered sequence of values with a separator.
type List struct {
	Elements  []Value
	Sep       Separator
	Bracketed bool
	// SlashLit marks a slash-separated list that came from literal "a/b"
	// division syntax, which serializes without spaces ("a/b"); constructed
	// slash lists (list.slash) serialize with spaces ("a / b").
	SlashLit bool
	// IsArgList marks a list produced by binding a rest parameter ($args...);
	// such a list also carries the call's trailing keyword arguments and reports
	// its type as "arglist".
	IsArgList bool
	// Keywords holds the keyword (named) arguments captured by a rest parameter,
	// keyed by their dash-normalised name (without the leading "$").
	Keywords map[string]Value
}

func (l *List) isTruthy() bool  { return true }
func (l *List) sep() Separator  { return l.Sep }
func (l *List) asList() []Value { return l.Elements }
func (l *List) equals(o Value) bool {
	ol, ok := o.(*List)
	if !ok {
		// A single-element bare list may equal a scalar in Sass, but we keep it strict.
		if len(l.Elements) == 1 {
			return l.Elements[0].equals(o)
		}
		return false
	}
	if len(l.Elements) != len(ol.Elements) || l.Sep != ol.Sep || l.Bracketed != ol.Bracketed {
		return false
	}
	for i := range l.Elements {
		if !l.Elements[i].equals(ol.Elements[i]) {
			return false
		}
	}
	return true
}

// Map is an ordered key/value collection.
type Map struct {
	Keys   []Value
	Values []Value
}

func (m *Map) isTruthy() bool { return true }
func (m *Map) sep() Separator { return SepComma }
func (m *Map) asList() []Value {
	out := make([]Value, len(m.Keys))
	for i := range m.Keys {
		out[i] = &List{Elements: []Value{m.Keys[i], m.Values[i]}, Sep: SepSpace}
	}
	return out
}
func (m *Map) equals(o Value) bool {
	om, ok := o.(*Map)
	if !ok {
		if len(m.Keys) == 0 {
			if ol, ok := o.(*List); ok {
				return len(ol.Elements) == 0
			}
		}
		return false
	}
	if len(m.Keys) != len(om.Keys) {
		return false
	}
	for i := range m.Keys {
		v, found := om.get(m.Keys[i])
		if !found || !m.Values[i].equals(v) {
			return false
		}
	}
	return true
}

func (m *Map) get(key Value) (Value, bool) {
	for i := range m.Keys {
		if m.Keys[i].equals(key) {
			return m.Values[i], true
		}
	}
	return nil, false
}

func (m *Map) set(key, val Value) {
	for i := range m.Keys {
		if m.Keys[i].equals(key) {
			m.Values[i] = val
			return
		}
	}
	m.Keys = append(m.Keys, key)
	m.Values = append(m.Values, val)
}

// fuzzyEquals compares two floats with Sass's tolerance (1e-11). Non-finite
// magnitudes compare exactly (Infinity equals Infinity, NaN equals nothing), so
// an infinite value works as a map key rather than being lost to a NaN diff.
func fuzzyEquals(a, b float64) bool {
	if math.IsInf(a, 0) || math.IsInf(b, 0) || math.IsNaN(a) || math.IsNaN(b) {
		return a == b
	}
	return math.Abs(a-b) < 1e-11
}

// fuzzyRound rounds to the nearest integer with Sass tolerance handling.
func fuzzyRound(v float64) float64 {
	if v > 0 {
		return math.Floor(v + 0.5)
	}
	return math.Ceil(v - 0.5)
}

// roundDecimalString rounds a shortest decimal string to at most 10 fractional
// digits, half-up, mirroring dart-sass's _writeRounded.
func roundDecimalString(text string) string {
	neg := strings.HasPrefix(text, "-")
	body := text
	if neg {
		body = text[1:]
	}
	dot := strings.IndexByte(body, '.')
	if dot < 0 {
		return text
	}
	frac := body[dot+1:]
	if len(frac) <= 10 {
		return text
	}
	intpart := body[:dot]
	digits := []byte(intpart + frac[:10])
	ilen := len(intpart)
	if frac[10] >= '5' {
		i := len(digits) - 1
		for {
			if digits[i] == '9' {
				digits[i] = '0'
				i--
				if i < 0 {
					digits = append([]byte{'1'}, digits...)
					ilen++
					break
				}
			} else {
				digits[i]++
				break
			}
		}
	}
	ni := string(digits[:ilen])
	nf := strings.TrimRight(string(digits[ilen:]), "0")
	res := ni
	if nf != "" {
		res += "." + nf
	}
	if neg {
		res = "-" + res
	}
	return res
}

// bigTenPow10 is 10^SassNumber.precision, the fixed-point scale dart-sass rounds
// numbers to (10 fractional digits).
var bigTenPow10 = new(big.Int).Exp(big.NewInt(10), big.NewInt(10), nil)

// exactIntegerString renders the exact integer value of a whole-valued finite
// float64. dart-sass serializes whole doubles via Dart's double.toString(),
// which for integers prints their exact value rather than the shortest round-
// trippable decimal — so 76837657717023024.0 prints as "76837657717023024",
// not Go strconv's shorter "76837657717023020". Non-integer doubles keep the
// shortest-decimal path, matching Dart's toString for fractional numbers (e.g.
// 2120983611678430.75 prints as "2120983611678430.8").
func exactIntegerString(f float64) string {
	r := new(big.Rat).SetFloat64(f) // exact; f is a finite whole number here
	neg := r.Sign() < 0
	if neg {
		r.Abs(r)
	}
	res := new(big.Int).Quo(r.Num(), r.Denom()).String()
	if neg && res != "0" {
		res = "-" + res
	}
	return res
}

// formatFloat renders a float the way dart-sass does: up to 10 fractional
// digits, trailing zeros stripped, "-0" normalised to "0", and (when
// compressed) a leading zero dropped for magnitudes below 1.
func formatFloat(f float64, compressed bool) string {
	if math.IsInf(f, 0) || math.IsNaN(f) {
		if math.IsNaN(f) {
			return "NaN"
		}
		if f > 0 {
			return "Infinity"
		}
		return "-Infinity"
	}
	// dart-sass serializes via Dart's double.toString(). For a whole-valued
	// double whose magnitude is below 1e21, Dart prints its exact integer value
	// (76837657717023024, not Go strconv's shorter 76837657717023020). At 1e21
	// and above, Dart switches to exponential notation with the shortest
	// mantissa, which dart-sass's _removeExponent expands to the shortest decimal
	// with trailing zeros — matching Go strconv's fixed shortest form. Fractional
	// doubles print the shortest round-trippable decimal rounded to at most
	// SassNumber.precision (10) fractional digits.
	var s string
	if f == math.Trunc(f) && math.Abs(f) < 1e21 {
		s = exactIntegerString(f)
	} else {
		s = roundDecimalString(strconv.FormatFloat(f, 'f', -1, 64))
	}
	if s == "-0" || s == "" {
		s = "0"
	}
	if compressed {
		if strings.HasPrefix(s, "0.") {
			s = s[1:]
		} else if strings.HasPrefix(s, "-0.") {
			s = "-" + s[2:]
		}
	}
	return s
}
