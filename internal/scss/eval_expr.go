// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import (
	"math"
	"strings"
)

func (e *evaluator) evalExpr(expr Expr) Value {
	switch x := expr.(type) {
	case *NumberLit:
		return &Number{Val: x.Val, Numer: unitSlice(x.Unit)}
	case *StringLit:
		return e.evalString(x)
	case *ColorLit:
		return x.Color
	case *BoolLit:
		return boolean(x.V)
	case *NullLit:
		return sassNull
	case *VarRef:
		return e.evalVarRef(x)
	case *Ident:
		return e.evalIdent(x)
	case *Parent:
		if e.currentParent.isEmpty() {
			return sassNull
		}
		return &SassString{Text: e.currentParent.serialize(false)}
	case *Binary:
		return e.evalBinary(x)
	case *Unary:
		return e.evalUnary(x)
	case *FuncCall:
		return e.evalCall(x)
	case *IfCssExpr:
		return e.evalIfCss(x)
	case *preEvaluated:
		return x.v
	case *ListExpr:
		return e.evalList(x)
	case *MapExpr:
		return e.evalMap(x)
	case *Paren:
		return e.evalExpr(x.Expr)
	case *InterpExpr:
		return &SassString{Text: e.evalInterpPart(x.Expr)}
	}
	e.fail("Unsupported expression.")
	return nil
}

func unitSlice(u string) []string {
	if u == "" {
		return nil
	}
	return []string{u}
}

// evalInterpPart evaluates a `#{...}` interpolation expression to its unquoted
// interpolation text. Interpolation is a fresh evaluation context, so any
// enclosing @supports-declaration state is cleared: calculations inside `#{}`
// are fully simplified (dart-sass resets _inSupportsDeclaration for interpolated
// content).
func (e *evaluator) evalInterpPart(expr Expr) string {
	old := e.inSupportsDecl
	e.inSupportsDecl = false
	v := e.evalExpr(expr)
	e.inSupportsDecl = old
	return e.stringifyInterp(v)
}

func (e *evaluator) evalString(x *StringLit) Value {
	var sb strings.Builder
	for _, part := range x.Parts.Parts {
		switch p := part.(type) {
		case string:
			sb.WriteString(p)
		case *InterpExpr:
			sb.WriteString(e.evalInterpPart(p.Expr))
		}
	}
	return &SassString{Text: sb.String(), Quoted: x.Quoted}
}

func (e *evaluator) evalVarRef(x *VarRef) Value {
	if x.Namespace != "" {
		if mod, ok := e.env.modules[x.Namespace]; ok {
			if v, ok := mod.vars[x.Name]; ok {
				return v
			}
		}
		if real, ok := e.env.builtinAliases[x.Namespace]; ok {
			if v, ok := builtinModuleVar(real, x.Name); ok {
				return v
			}
		}
		e.fail("Undefined variable.")
	}
	// A bare variable may resolve to a built-in module exposed via `as *`.
	if v, ok := e.env.getVar(x.Name); ok {
		return v
	}
	for _, m := range e.env.builtinGlobals {
		if v, ok := builtinModuleVar(m, x.Name); ok {
			return v
		}
	}
	e.fail("Undefined variable.")
	return nil
}

func (e *evaluator) evalIdent(x *Ident) Value {
	lower := strings.ToLower(x.Name)
	if lower == "transparent" {
		return namedColorTransparent(x.Name)
	}
	if c, ok := lookupNamedColor(x.Name); ok {
		return c
	}
	return &SassString{Text: x.Name, Quoted: false}
}

func (e *evaluator) evalList(x *ListExpr) Value {
	elems := make([]Value, len(x.Elements))
	for i, el := range x.Elements {
		elems[i] = e.evalExpr(el)
	}
	return &List{Elements: elems, Sep: x.Sep, Bracketed: x.Bracketed}
}

func (e *evaluator) evalMap(x *MapExpr) Value {
	m := &Map{}
	for i := range x.Keys {
		k := e.evalExpr(x.Keys[i])
		v := e.evalExpr(x.Values[i])
		if _, dup := m.get(k); dup {
			e.fail("Duplicate key in map.")
		}
		m.set(k, v)
	}
	return m
}

func (e *evaluator) evalNumber(expr Expr) *Number {
	v := e.evalExpr(expr)
	n, ok := v.(*Number)
	if !ok {
		e.fail("%s is not a number.", serializeValue(v, false))
	}
	return n
}

func (e *evaluator) evalUnary(x *Unary) Value {
	if x.Op == "not" {
		return boolean(!e.evalExpr(x.Expr).isTruthy())
	}
	v := e.evalExpr(x.Expr)
	if x.Op == "+" {
		// Unary plus is a no-op on a number, but on any other value dart-sass
		// keeps the sign as literal text: `+foo(12px)` stays `+foo(12px)`.
		if _, ok := v.(*Number); ok {
			return v
		}
		return &SassString{Text: "+" + serializeValue(v, false), Quoted: false}
	}
	// unary minus
	if n, ok := v.(*Number); ok {
		return &Number{Val: -n.Val, Numer: n.Numer, Denom: n.Denom}
	}
	return &SassString{Text: "-" + serializeValue(v, false), Quoted: false}
}

func (e *evaluator) evalBinary(x *Binary) Value {
	switch x.Op {
	case "and":
		l := e.evalExpr(x.Left)
		if !l.isTruthy() {
			return l
		}
		return e.evalExpr(x.Right)
	case "or":
		l := e.evalExpr(x.Left)
		if l.isTruthy() {
			return l
		}
		return e.evalExpr(x.Right)
	}
	if x.Op == "/" {
		l := e.evalExpr(x.Left)
		r := e.evalExpr(x.Right)
		_, lnum := l.(*Number)
		_, rnum := r.(*Number)
		if lnum && rnum {
			if x.Slash {
				return &List{Elements: []Value{l, r}, Sep: SepSlash, SlashLit: true}
			}
			return e.arith(x.Op, l, r)
		}
		// "/" between operands that aren't both numbers is a slash separator, not
		// division (e.g. the "/ none" or "/ calc(…)" alpha in a color function).
		return &List{Elements: []Value{l, r}, Sep: SepSlash, SlashLit: true}
	}
	l := e.evalExpr(x.Left)
	r := e.evalExpr(x.Right)
	switch x.Op {
	case "==":
		return boolean(l.equals(r))
	case "!=":
		return boolean(!l.equals(r))
	case "<", ">", "<=", ">=":
		return e.compare(x.Op, l, r)
	default:
		return e.arith(x.Op, l, r)
	}
}

func (e *evaluator) compare(op string, l, r Value) Value {
	ln, lok := l.(*Number)
	rn, rok := r.(*Number)
	if !lok || !rok {
		e.fail("Undefined operation \"%s %s %s\".", serializeValue(l, false), op, serializeValue(r, false))
	}
	rv, ok := rn.compatConvert(ln)
	if !ok {
		e.fail("Incompatible units.")
	}
	a := ln.Val
	b := rv
	switch op {
	case "<":
		return boolean(a < b)
	case ">":
		return boolean(a > b)
	case "<=":
		return boolean(a <= b)
	default:
		return boolean(a >= b)
	}
}

func (e *evaluator) arith(op string, l, r Value) Value {
	ln, lok := l.(*Number)
	rn, rok := r.(*Number)
	if lok && rok {
		return e.numberArith(op, ln, rn)
	}
	// string operations
	switch op {
	case "+":
		return e.stringOp(l, r, "")
	case "-":
		return e.stringOp(l, r, "-")
	case "*":
		e.fail("Undefined operation \"%s * %s\".", serializeValue(l, false), serializeValue(r, false))
	case "%":
		e.fail("Undefined operation \"%s %% %s\".", serializeValue(l, false), serializeValue(r, false))
	}
	return nil
}

func (e *evaluator) stringOp(l, r Value, sep string) Value {
	// The result's quotedness follows the left operand when it is a string; when
	// the left operand is not a string (e.g. a number), it follows the right
	// operand instead. So `3 + "hello"` yields the quoted `"3hello"`, matching
	// dart-sass.
	quoted := false
	if s, ok := l.(*SassString); ok {
		quoted = s.Quoted
	} else if s, ok := r.(*SassString); ok {
		quoted = s.Quoted
	}
	ls := unquotedInner(l)
	rs := unquotedInner(r)
	return &SassString{Text: ls + sep + rs, Quoted: quoted}
}

func unquotedInner(v Value) string {
	if s, ok := v.(*SassString); ok {
		return s.Text
	}
	return serializeValue(v, false)
}

func (e *evaluator) numberArith(op string, a, b *Number) Value {
	switch op {
	case "+", "-":
		bv, num, den := e.alignForAdd(a, b)
		val := a.Val
		if op == "+" {
			val += bv
		} else {
			val -= bv
		}
		return &Number{Val: val, Numer: num, Denom: den}
	case "*":
		val := a.Val * b.Val
		num := append(append([]string(nil), a.Numer...), b.Numer...)
		den := append(append([]string(nil), a.Denom...), b.Denom...)
		val, num, den = simplifyUnits(val, num, den)
		return &Number{Val: val, Numer: num, Denom: den}
	case "/":
		val := a.Val / b.Val
		num := append(append([]string(nil), a.Numer...), b.Denom...)
		den := append(append([]string(nil), a.Denom...), b.Numer...)
		val, num, den = simplifyUnits(val, num, den)
		return &Number{Val: val, Numer: num, Denom: den}
	case "%":
		bv, num, _ := e.alignForAdd(a, b)
		val := math.Mod(a.Val, bv)
		if val != 0 && (val < 0) != (bv < 0) {
			val += bv
		}
		return &Number{Val: val, Numer: num}
	}
	return nil
}

// alignForAdd converts b into a's units for addition/subtraction, returning the
// converted value and the result units.
func (e *evaluator) alignForAdd(a, b *Number) (float64, []string, []string) {
	if !a.hasUnits() {
		return b.Val, b.Numer, b.Denom
	}
	if !b.hasUnits() {
		return b.Val, a.Numer, a.Denom
	}
	v, ok := convertUnits(b.Val, b.Numer, b.Denom, a.Numer, a.Denom)
	if !ok {
		e.fail("%s and %s have incompatible units.", serializeValue(a, false), serializeValue(b, false))
	}
	return v, a.Numer, a.Denom
}
