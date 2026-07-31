// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import (
	"math"
	"strings"
)

// SassCalculation is a first-class CSS calculation value (calc(), min(), max(),
// clamp() and the CSS math functions). It mirrors dart-sass's SassCalculation:
// each argument is a fully-simplified term — a *Number, a *SassCalculation, an
// unquoted *SassString, or a *CalcOp operation node.
type SassCalculation struct {
	Name string
	Args []calcTerm
}

// calcTerm is one of *Number, *SassString (unquoted), *SassCalculation or
// *CalcOp.
type calcTerm any

// CalcOp is a binary operation node inside a calculation's operation tree.
type CalcOp struct {
	Op          string // "+", "-", "*", "/"
	Left, Right calcTerm
}

func (c *SassCalculation) isTruthy() bool  { return true }
func (c *SassCalculation) sep() Separator  { return SepUndecided }
func (c *SassCalculation) asList() []Value { return []Value{c} }
func (c *SassCalculation) equals(o Value) bool {
	oc, ok := o.(*SassCalculation)
	if !ok || oc.Name != c.Name || len(oc.Args) != len(c.Args) {
		return false
	}
	for i := range c.Args {
		if !calcTermEquals(c.Args[i], oc.Args[i]) {
			return false
		}
	}
	return true
}

func calcTermEquals(a, b calcTerm) bool {
	switch av := a.(type) {
	case *Number:
		bv, ok := b.(*Number)
		return ok && av.equals(bv)
	case *SassString:
		bv, ok := b.(*SassString)
		return ok && av.Text == bv.Text
	case *SassCalculation:
		bv, ok := b.(*SassCalculation)
		return ok && av.equals(bv)
	case *CalcOp:
		bv, ok := b.(*CalcOp)
		return ok && av.Op == bv.Op && calcTermEquals(av.Left, bv.Left) && calcTermEquals(av.Right, bv.Right)
	}
	return false
}

// calcPrecedence returns operator precedence: additive 1, multiplicative 2.
func calcPrecedence(op string) int {
	if op == "*" || op == "/" {
		return 2
	}
	return 1
}

// --- Number helpers used by the calculation engine ---

func (n *Number) hasComplexUnits() bool { return len(n.Numer) > 1 || len(n.Denom) > 0 }

// hasCompatibleUnitsWith reports whether n can be converted to o's units without
// loss (unitless is compatible only with unitless).
func (n *Number) hasCompatibleUnitsWith(o *Number) bool {
	_, ok := convertUnits(n.Val, n.Numer, n.Denom, o.Numer, o.Denom)
	return ok
}

// isComparableTo reports whether n and o can be compared: a unitless number is
// comparable to anything, otherwise they must share convertible units.
func (n *Number) isComparableTo(o *Number) bool {
	if !n.hasUnits() || !o.hasUnits() {
		return true
	}
	return n.hasCompatibleUnitsWith(o)
}

// convertValueToMatch returns n's value expressed in o's units.
func (n *Number) convertValueToMatch(o *Number) float64 {
	if v, ok := convertUnits(n.Val, n.Numer, n.Denom, o.Numer, o.Denom); ok {
		return v
	}
	return n.Val
}

func cloneUnits(u []string) []string {
	if len(u) == 0 {
		return nil
	}
	return append([]string(nil), u...)
}

// coerceToMatch returns a number with n's value expressed in o's units and o's
// unit set.
func (n *Number) coerceToMatch(o *Number) *Number {
	if !n.hasUnits() {
		return &Number{Val: n.Val, Numer: cloneUnits(o.Numer), Denom: cloneUnits(o.Denom)}
	}
	v := n.convertValueToMatch(o)
	return &Number{Val: v, Numer: cloneUnits(o.Numer), Denom: cloneUnits(o.Denom)}
}

// matchUnits returns value carrying number's units.
func matchUnits(value float64, number *Number) *Number {
	return &Number{Val: value, Numer: cloneUnits(number.Numer), Denom: cloneUnits(number.Denom)}
}

func (e *evaluator) assertNoUnits(n *Number, what string) {
	if n.hasUnits() {
		e.fail("Expected %s to have no units.", serializeValue(n, false))
	}
	_ = what
}

// moduloLikeSass implements Sass's floored-division modulo, matching dart-sass.
func moduloLikeSass(a, b float64) float64 {
	if math.IsInf(a, 0) {
		return math.NaN()
	}
	if math.IsInf(b, 0) {
		if signIncludingZero(a) == sign2(b) {
			return a
		}
		return math.NaN()
	}
	if b > 0 {
		r := math.Mod(a, b)
		if r < 0 {
			r += b
		}
		if r == 0 {
			// Floored modulo lands in [0, b) for a positive divisor, so a zero
			// result is +0 (dart-sass: math.div(1, mod(-7, 7)) == calc(infinity)).
			return 0
		}
		return r
	}
	if b == 0 {
		return math.NaN()
	}
	r := math.Mod(a, b)
	if r == 0 {
		// Floored modulo lands in (b, 0] for a negative divisor, so a zero result
		// carries the divisor's negative sign.
		return math.Copysign(0, -1)
	}
	if r > 0 {
		r += b
	}
	return r
}

func sign2(v float64) float64 {
	if v > 0 {
		return 1
	}
	if v < 0 {
		return -1
	}
	return v // 0 or NaN
}

func signIncludingZero(v float64) float64 {
	if math.Signbit(v) && v == 0 {
		return -1
	}
	if v == 0 {
		return 1
	}
	return sign2(v)
}

// --- calculation constructors (ports of SassCalculation static methods) ---

func (e *evaluator) calcCalc(arg calcTerm) Value {
	switch v := e.calcSimplify(arg).(type) {
	case *Number:
		return v
	case *SassCalculation:
		return v
	default:
		return &SassCalculation{Name: "calc", Args: []calcTerm{v}}
	}
}

func (e *evaluator) calcMinMax(name string, args []calcTerm) Value {
	args = e.simplifyArgs(args)
	var best *Number
	ok := true
	for _, a := range args {
		n, isNum := a.(*Number)
		if !isNum || (best != nil && !best.isComparableTo(n)) {
			ok = false
			break
		}
		if best == nil {
			best = n
			continue
		}
		bv := best.Val
		nv := n.convertValueToMatch(best)
		if name == "min" {
			if nv < bv && !fuzzyEquals(nv, bv) {
				best = n
			}
		} else {
			if nv > bv && !fuzzyEquals(nv, bv) {
				best = n
			}
		}
	}
	if ok && best != nil {
		return best
	}
	return &SassCalculation{Name: name, Args: args}
}

func (e *evaluator) calcClamp(args []calcTerm) Value {
	for i := range args {
		args[i] = e.calcSimplify(args[i])
	}
	if len(args) == 3 {
		lo, ok1 := args[0].(*Number)
		val, ok2 := args[1].(*Number)
		hi, ok3 := args[2].(*Number)
		if ok1 && ok2 && ok3 && lo.hasCompatibleUnitsWith(val) && lo.hasCompatibleUnitsWith(hi) {
			vv := val.Val
			// Compare using val's own units by converting lo and hi into them.
			loInVal := lo.convertValueToMatch(val)
			hiInVal := hi.convertValueToMatch(val)
			if vv <= loInVal || fuzzyEquals(vv, loInVal) {
				return lo
			}
			if vv >= hiInVal || fuzzyEquals(vv, hiInVal) {
				return hi
			}
			return val
		}
	}
	return &SassCalculation{Name: "clamp", Args: args}
}

func (e *evaluator) calcHypot(args []calcTerm) Value {
	args = e.simplifyArgs(args)
	first, ok := args[0].(*Number)
	if !ok || first.hasUnit("%") {
		return &SassCalculation{Name: "hypot", Args: args}
	}
	subtotal := 0.0
	for _, a := range args {
		n, isNum := a.(*Number)
		if !isNum || !n.hasCompatibleUnitsWith(first) {
			return &SassCalculation{Name: "hypot", Args: args}
		}
		v := n.convertValueToMatch(first)
		subtotal += v * v
	}
	return &Number{Val: math.Sqrt(subtotal), Numer: cloneUnits(first.Numer), Denom: cloneUnits(first.Denom)}
}

// singleArg implements the single-argument math functions.
func (e *evaluator) calcSingle(name string, arg calcTerm, fn func(*Number) *Number, forbidUnits bool) Value {
	arg = e.calcSimplify(arg)
	n, ok := arg.(*Number)
	if !ok {
		return &SassCalculation{Name: name, Args: []calcTerm{arg}}
	}
	if forbidUnits {
		e.assertNoUnits(n, "number")
	}
	return fn(n)
}

func (e *evaluator) calcAbs(arg calcTerm) Value {
	arg = e.calcSimplify(arg)
	n, ok := arg.(*Number)
	if !ok {
		return &SassCalculation{Name: "abs", Args: []calcTerm{arg}}
	}
	return (&Number{Val: math.Abs(n.Val)}).coerceToMatch(n)
}

func (e *evaluator) calcExp(arg calcTerm) Value {
	arg = e.calcSimplify(arg)
	n, ok := arg.(*Number)
	if !ok {
		return &SassCalculation{Name: "exp", Args: []calcTerm{arg}}
	}
	e.assertNoUnits(n, "number")
	return &Number{Val: math.Pow(math.E, n.Val)}
}

func (e *evaluator) calcSign(arg calcTerm) Value {
	arg = e.calcSimplify(arg)
	n, ok := arg.(*Number)
	if !ok {
		return &SassCalculation{Name: "sign", Args: []calcTerm{arg}}
	}
	if math.IsNaN(n.Val) || n.Val == 0 {
		return n
	}
	if !n.hasUnit("%") {
		return (&Number{Val: sign2(n.Val)}).coerceToMatch(n)
	}
	return &SassCalculation{Name: "sign", Args: []calcTerm{arg}}
}

func (e *evaluator) calcPow(base, exp calcTerm) Value {
	base = e.calcSimplify(base)
	exp = e.calcSimplify(exp)
	bn, ok1 := base.(*Number)
	xn, ok2 := exp.(*Number)
	if !ok1 || !ok2 {
		return &SassCalculation{Name: "pow", Args: []calcTerm{base, exp}}
	}
	e.assertNoUnits(bn, "base")
	e.assertNoUnits(xn, "exponent")
	return &Number{Val: math.Pow(bn.Val, xn.Val)}
}

func (e *evaluator) calcLog(number calcTerm, base calcTerm) Value {
	number = e.calcSimplify(number)
	var args []calcTerm
	nn, ok := number.(*Number)
	if base == nil {
		if !ok {
			return &SassCalculation{Name: "log", Args: []calcTerm{number}}
		}
		e.assertNoUnits(nn, "number")
		return &Number{Val: math.Log(nn.Val)}
	}
	base = e.calcSimplify(base)
	bn, bok := base.(*Number)
	args = []calcTerm{number, base}
	if !ok || !bok {
		return &SassCalculation{Name: "log", Args: args}
	}
	e.assertNoUnits(nn, "number")
	e.assertNoUnits(bn, "base")
	return &Number{Val: math.Log(nn.Val) / math.Log(bn.Val)}
}

func (e *evaluator) calcAtan2(y, x calcTerm) Value {
	y = e.calcSimplify(y)
	x = e.calcSimplify(x)
	args := []calcTerm{y, x}
	yn, ok1 := y.(*Number)
	xn, ok2 := x.(*Number)
	if !ok1 || !ok2 || yn.hasUnit("%") || xn.hasUnit("%") || !yn.hasCompatibleUnitsWith(xn) {
		return &SassCalculation{Name: "atan2", Args: args}
	}
	return radiansToDegrees(math.Atan2(yn.Val, xn.convertValueToMatch(yn)))
}

func (e *evaluator) calcRem(dividend, modulus calcTerm) Value {
	dividend = e.calcSimplify(dividend)
	modulus = e.calcSimplify(modulus)
	args := []calcTerm{dividend, modulus}
	dn, ok1 := dividend.(*Number)
	mn, ok2 := modulus.(*Number)
	if !ok1 || !ok2 || !dn.hasCompatibleUnitsWith(mn) {
		return &SassCalculation{Name: "rem", Args: args}
	}
	mv := mn.convertValueToMatch(dn)
	res := matchUnits(moduloLikeSass(dn.Val, mv), dn)
	if signIncludingZero(mv) != signIncludingZero(dn.Val) {
		if math.IsInf(mv, 0) {
			return dn
		}
		if res.Val == 0 {
			return &Number{Val: -res.Val, Numer: cloneUnits(res.Numer), Denom: cloneUnits(res.Denom)}
		}
		return &Number{Val: res.Val - mv, Numer: cloneUnits(res.Numer), Denom: cloneUnits(res.Denom)}
	}
	return res
}

func (e *evaluator) calcMod(dividend, modulus calcTerm) Value {
	dividend = e.calcSimplify(dividend)
	modulus = e.calcSimplify(modulus)
	args := []calcTerm{dividend, modulus}
	dn, ok1 := dividend.(*Number)
	mn, ok2 := modulus.(*Number)
	if !ok1 || !ok2 || !dn.hasCompatibleUnitsWith(mn) {
		return &SassCalculation{Name: "mod", Args: args}
	}
	mv := mn.convertValueToMatch(dn)
	return matchUnits(moduloLikeSass(dn.Val, mv), dn)
}

func (e *evaluator) calcSize(basis, value calcTerm) Value {
	basis = e.calcSimplify(basis)
	if value != nil {
		value = e.calcSimplify(value)
		return &SassCalculation{Name: "calc-size", Args: []calcTerm{basis, value}}
	}
	return &SassCalculation{Name: "calc-size", Args: []calcTerm{basis}}
}

func radiansToDegrees(rad float64) *Number {
	return &Number{Val: rad * (180 / math.Pi), Numer: []string{"deg"}}
}

// --- round ---

var roundStrategies = map[string]bool{"nearest": true, "up": true, "down": true, "to-zero": true}

func (e *evaluator) calcRound(a, b, c calcTerm, legacy bool) Value {
	s0 := e.calcSimplify(a)
	var s1, s2 calcTerm
	if b != nil {
		s1 = e.calcSimplify(b)
	}
	if c != nil {
		s2 = e.calcSimplify(c)
	}

	n0, isNum0 := s0.(*Number)
	str0, isStr0 := s0.(*SassString)

	// (number, null, null)
	if b == nil && c == nil {
		if isNum0 {
			if !n0.hasUnits() {
				return &Number{Val: math.Round(n0.Val)}
			}
			if legacy {
				return matchUnits(math.Round(n0.Val), n0)
			}
		}
		return &SassCalculation{Name: "round", Args: []calcTerm{s0}}
	}

	// (number, step, null)
	if b != nil && c == nil {
		n1, isNum1 := s1.(*Number)
		if isNum0 && isNum1 {
			if !n0.hasCompatibleUnitsWith(n1) {
				return &SassCalculation{Name: "round", Args: []calcTerm{s0, s1}}
			}
			return e.roundWithStep("nearest", n0, n1)
		}
		if isStr0 && roundStrategies[str0.Text] {
			// strategy + an unquoted string second value (e.g. a var()) stays a
			// symbolic round() call; a strategy with any other kind of value is an
			// error because a numeric step is required.
			if _, isS1Str := s1.(*SassString); isS1Str {
				return &SassCalculation{Name: "round", Args: []calcTerm{s0, s1}}
			}
			e.fail("If strategy is not null, step is required.")
		}
		return &SassCalculation{Name: "round", Args: []calcTerm{s0, s1}}
	}

	// three arguments: (strategy, number, step)
	n1, isNum1 := s1.(*Number)
	n2, isNum2 := s2.(*Number)
	if isStr0 && roundStrategies[str0.Text] && isNum1 && isNum2 {
		if !n1.hasCompatibleUnitsWith(n2) {
			return &SassCalculation{Name: "round", Args: []calcTerm{s0, s1, s2}}
		}
		return e.roundWithStep(str0.Text, n1, n2)
	}
	if isStr0 && (roundStrategies[str0.Text] || isSpecialVariableString(str0)) {
		return &SassCalculation{Name: "round", Args: []calcTerm{s0, s1, s2}}
	}
	e.fail("%s must be either nearest, up, down or to-zero.", serializeValue(valueOfTerm(s0), false))
	return nil
}

func valueOfTerm(t calcTerm) Value {
	if v, ok := t.(Value); ok {
		return v
	}
	return &SassString{Text: serializeCalcTerm(t, false)}
}

func (e *evaluator) roundWithStep(strategy string, number, step *Number) *Number {
	nv, sv := number.Val, step.Val
	if (math.IsInf(nv, 0) && math.IsInf(sv, 0)) || sv == 0 || math.IsNaN(nv) || math.IsNaN(sv) {
		return matchUnits(math.NaN(), number)
	}
	if math.IsInf(nv, 0) {
		return number
	}
	if math.IsInf(sv, 0) {
		switch {
		case nv == 0:
			return number
		case (strategy == "nearest" || strategy == "to-zero") && nv > 0:
			return matchUnits(0.0, number)
		case strategy == "nearest" || strategy == "to-zero":
			return matchUnits(math.Copysign(0, -1), number)
		case strategy == "up" && nv > 0:
			return matchUnits(math.Inf(1), number)
		case strategy == "up":
			return matchUnits(math.Copysign(0, -1), number)
		case strategy == "down" && nv < 0:
			return matchUnits(math.Inf(-1), number)
		default:
			return matchUnits(0, number)
		}
	}
	stepUnit := step.convertValueToMatch(number)
	q := nv / stepUnit
	switch strategy {
	case "nearest":
		return matchUnits(math.Round(q)*stepUnit, number)
	case "up":
		if stepUnit < 0 {
			return matchUnits(math.Floor(q)*stepUnit, number)
		}
		return matchUnits(math.Ceil(q)*stepUnit, number)
	case "down":
		if stepUnit < 0 {
			return matchUnits(math.Ceil(q)*stepUnit, number)
		}
		return matchUnits(math.Floor(q)*stepUnit, number)
	default: // "to-zero" (the only remaining validated strategy)
		if nv < 0 {
			return matchUnits(math.Ceil(q)*stepUnit, number)
		}
		return matchUnits(math.Floor(q)*stepUnit, number)
	}
}

func isSpecialVariableString(s *SassString) bool {
	return isSpecialNumberString(s.Text)
}

// isSpecialNumberString reports whether an unquoted string is a CSS function
// that produces a number and must be preserved verbatim (mirrors dart-sass's
// SassString.isSpecialNumber): calc(), var(), env(), min(), max(), clamp(),
// attr() and if().
func isSpecialNumberString(text string) bool {
	for _, p := range []string{"calc(", "var(", "env(", "min(", "max(", "clamp(", "attr(", "if("} {
		if len(text) >= len(p) && strings.EqualFold(text[:len(p)], p) {
			return true
		}
	}
	return false
}

// --- operate ---

func (e *evaluator) calcOperate(op string, left, right calcTerm, legacy bool) calcTerm {
	// Inside a @supports declaration the operation is kept unsimplified: no
	// numeric reduction, no operand simplification (dart-sass simplify:false).
	if e.inSupportsDecl {
		return &CalcOp{Op: op, Left: left, Right: right}
	}
	left = e.calcSimplify(left)
	right = e.calcSimplify(right)

	if op == "+" || op == "-" {
		ln, lok := left.(*Number)
		rn, rok := right.(*Number)
		if lok && rok {
			compatible := ln.hasCompatibleUnitsWith(rn)
			if !compatible && legacy && ln.isComparableTo(rn) {
				compatible = true
			}
			if compatible {
				return e.numberArith(op, ln, rn)
			}
		}
		if rok && rn.Val < 0 && !fuzzyEquals(rn.Val, 0) {
			neg := &Number{Val: -rn.Val, Numer: cloneUnits(rn.Numer), Denom: cloneUnits(rn.Denom)}
			if op == "+" {
				op = "-"
			} else {
				op = "+"
			}
			return &CalcOp{Op: op, Left: left, Right: neg}
		}
		return &CalcOp{Op: op, Left: left, Right: right}
	}
	// times / dividedBy
	ln, lok := left.(*Number)
	rn, rok := right.(*Number)
	if lok && rok {
		return e.numberArith(op, ln, rn)
	}
	return &CalcOp{Op: op, Left: left, Right: right}
}

// --- simplify ---

func (e *evaluator) simplifyArgs(args []calcTerm) []calcTerm {
	out := make([]calcTerm, len(args))
	for i, a := range args {
		out[i] = e.calcSimplify(a)
	}
	return out
}

func (e *evaluator) calcSimplify(arg calcTerm) calcTerm {
	switch v := arg.(type) {
	case *Number:
		return v
	case *CalcOp:
		return v
	case *SassString:
		if v.Quoted {
			e.fail("Quoted string %s can't be used in a calculation.", serializeValue(v, false))
		}
		return v
	case *SassCalculation:
		if v.Name == "calc" && len(v.Args) == 1 {
			if s, ok := v.Args[0].(*SassString); ok && !s.Quoted {
				if needsParentheses(s.Text) {
					return &SassString{Text: "(" + s.Text + ")"}
				}
			}
			return v.Args[0]
		}
		return v
	}
	e.fail("Value can't be used in a calculation.")
	return nil
}

// needsParentheses reports whether an unquoted calc() string argument needs
// wrapping when embedded in another calculation.
func needsParentheses(text string) bool {
	if text == "" {
		return false
	}
	charNeeds := func(c byte) bool {
		return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == '/' || c == '*'
	}
	eqi := func(c, low byte) bool {
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		return c == low
	}
	first := text[0]
	if charNeeds(first) {
		return true
	}
	couldBeVar := len(text) >= 4 && eqi(first, 'v')
	if len(text) < 2 {
		return false
	}
	second := text[1]
	if charNeeds(second) {
		return true
	}
	couldBeVar = couldBeVar && eqi(second, 'a')
	if len(text) < 3 {
		return false
	}
	third := text[2]
	if charNeeds(third) {
		return true
	}
	couldBeVar = couldBeVar && eqi(third, 'r')
	if len(text) < 4 {
		return false
	}
	fourth := text[3]
	if couldBeVar && fourth == '(' {
		return true
	}
	if charNeeds(fourth) {
		return true
	}
	for i := 4; i < len(text); i++ {
		if charNeeds(text[i]) {
			return true
		}
	}
	return false
}

// --- serialization ---

func serializeCalculation(c *SassCalculation, compressed bool) string {
	var sb strings.Builder
	writeCalcCall(&sb, c, compressed)
	return sb.String()
}

func writeCalcCall(sb *strings.Builder, c *SassCalculation, compressed bool) {
	sb.WriteString(c.Name)
	sb.WriteByte('(')
	sep := ", "
	if compressed {
		sep = ","
	}
	for i, a := range c.Args {
		if i > 0 {
			sb.WriteString(sep)
		}
		writeCalcTerm(sb, a, compressed)
	}
	sb.WriteByte(')')
}

func writeCalcTerm(sb *strings.Builder, t calcTerm, compressed bool) {
	switch v := t.(type) {
	case *Number:
		if math.IsInf(v.Val, 0) || math.IsNaN(v.Val) {
			switch {
			case math.IsNaN(v.Val):
				sb.WriteString("NaN")
			case v.Val > 0:
				sb.WriteString("infinity")
			default:
				sb.WriteString("-infinity")
			}
			writeCalcUnits(sb, v.Numer, v.Denom, compressed)
			return
		}
		if v.hasComplexUnits() {
			sb.WriteString(formatFloat(v.Val, compressed))
			// The value glues to the first numerator unit ("1px"); any further
			// numerators and all denominators are written as " * 1u" / " / 1u".
			// A denominator-only number (no numerators, e.g. 1 / 1px / 1rad) has
			// nothing to glue, so its whole unit list goes through writeCalcUnits.
			if len(v.Numer) > 0 {
				sb.WriteString(v.Numer[0])
				writeCalcUnits(sb, v.Numer[1:], v.Denom, compressed)
			} else {
				writeCalcUnits(sb, nil, v.Denom, compressed)
			}
			return
		}
		sb.WriteString(serializeValue(v, compressed))
	case *SassString:
		sb.WriteString(v.Text)
	case *SassCalculation:
		writeCalcCall(sb, v, compressed)
	case *CalcOp:
		writeCalcOp(sb, v, compressed)
	default:
		if val, ok := t.(Value); ok {
			sb.WriteString(serializeValue(val, compressed))
		}
	}
}

func writeCalcOp(sb *strings.Builder, op *CalcOp, compressed bool) {
	parenLeft := false
	if lo, ok := op.Left.(*CalcOp); ok && calcPrecedence(lo.Op) < calcPrecedence(op.Op) {
		parenLeft = true
	}
	if parenLeft {
		sb.WriteByte('(')
	}
	writeCalcTerm(sb, op.Left, compressed)
	if parenLeft {
		sb.WriteByte(')')
	}

	opWs := !compressed || calcPrecedence(op.Op) == 1
	if opWs {
		sb.WriteByte(' ')
	}
	sb.WriteString(op.Op)
	if opWs {
		sb.WriteByte(' ')
	}

	parenRight := false
	if ro, ok := op.Right.(*CalcOp); ok && parenthesizeCalcRhs(op.Op, ro.Op) {
		parenRight = true
	} else if op.Op == "/" {
		if rn, ok := op.Right.(*Number); ok {
			if math.IsInf(rn.Val, 0) || math.IsNaN(rn.Val) {
				parenRight = rn.hasUnits()
			} else {
				parenRight = rn.hasComplexUnits()
			}
		}
	}
	if parenRight {
		sb.WriteByte('(')
	}
	writeCalcTerm(sb, op.Right, compressed)
	if parenRight {
		sb.WriteByte(')')
	}
}

func parenthesizeCalcRhs(outer, right string) bool {
	switch outer {
	case "/":
		return true
	case "+":
		return false
	default:
		return right == "+" || right == "-"
	}
}

func writeCalcUnits(sb *strings.Builder, numer, denom []string, compressed bool) {
	space := " "
	if compressed {
		space = ""
	}
	for _, u := range numer {
		sb.WriteString(space)
		sb.WriteByte('*')
		sb.WriteString(space)
		sb.WriteString("1")
		sb.WriteString(u)
	}
	for _, u := range denom {
		sb.WriteString(space)
		sb.WriteByte('/')
		sb.WriteString(space)
		sb.WriteString("1")
		sb.WriteString(u)
	}
}

func serializeCalcTerm(t calcTerm, compressed bool) string {
	var sb strings.Builder
	writeCalcTerm(&sb, t, compressed)
	return sb.String()
}
