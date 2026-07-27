// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import (
	"math"
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
	a, aok := n.compatConvert(on)
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
	if num == "" {
		num = "1"
	}
	den := strings.Join(n.Denom, "*")
	if len(n.Denom) > 1 {
		den = "(" + den + ")"
	}
	return num + "/" + den
}

// SassColor is an RGBA color. Original, when non-empty, is the verbatim source
// representation reproduced in expanded output (matching dart-sass semantics).
type SassColor struct {
	R, G, B  int
	A        float64
	Original string
}

func (c *SassColor) isTruthy() bool  { return true }
func (c *SassColor) sep() Separator  { return SepUndecided }
func (c *SassColor) asList() []Value { return []Value{c} }
func (c *SassColor) equals(o Value) bool {
	oc, ok := o.(*SassColor)
	if !ok {
		return false
	}
	return c.R == oc.R && c.G == oc.G && c.B == oc.B && c.A == oc.A
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

// fuzzyEquals compares two floats with Sass's tolerance (1e-11).
func fuzzyEquals(a, b float64) bool {
	return math.Abs(a-b) < 1e-11
}

// fuzzyRound rounds to the nearest integer with Sass tolerance handling.
func fuzzyRound(v float64) float64 {
	if v > 0 {
		return math.Floor(v + 0.5)
	}
	return math.Ceil(v - 0.5)
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
	s := strconv.FormatFloat(f, 'f', 10, 64)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(s, "0")
		s = strings.TrimRight(s, ".")
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
