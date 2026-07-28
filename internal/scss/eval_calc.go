// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import (
	"math"
	"strings"
)

// calcArity maps a calculation function to its maximum positional argument count
// (0 means unbounded, e.g. min/max/hypot).
var calcArity = map[string]int{
	"calc": 1, "sqrt": 1, "sin": 1, "cos": 1, "tan": 1,
	"asin": 1, "acos": 1, "atan": 1, "abs": 1, "exp": 1, "sign": 1,
	"min": 0, "max": 0, "hypot": 0,
	"pow": 2, "atan2": 2, "log": 2, "mod": 2, "rem": 2, "calc-size": 2,
	"round": 3, "clamp": 3,
}

// calcLegacyGated are the functions that only act as calculations when all their
// arguments are calculation-safe; otherwise they fall back to the legacy numeric
// global built-ins.
var calcLegacyGated = map[string]bool{"min": true, "max": true, "round": true, "abs": true}

// tryCalculation handles a global (non-namespaced) call whose name is a CSS math
// function. It returns (value, true) when the call is evaluated as a
// calculation, or (nil, false) to let a legacy numeric built-in handle it.
func (e *evaluator) tryCalculation(x *FuncCall) (Value, bool) {
	name := strings.ToLower(normIdent(x.Name))
	maxArgs, ok := calcArity[name]
	if !ok {
		return nil, false
	}
	if calcLegacyGated[name] && !e.argsCalcSafe(x.Args) {
		return nil, false
	}
	return e.evalCalculation(name, x, maxArgs), true
}

func (e *evaluator) argsCalcSafe(args *ArgList) bool {
	if args == nil {
		return false
	}
	for _, a := range args.Args {
		if a.Name != "" || a.Spread {
			return false
		}
		if !e.exprCalcSafe(a.Value) {
			return false
		}
	}
	return true
}

func (e *evaluator) evalCalculation(name string, x *FuncCall, maxArgs int) Value {
	var argExprs []Expr
	if x.Args != nil {
		for _, a := range x.Args.Args {
			if a.Name != "" {
				e.fail("Keyword arguments can't be used with calculations.")
			}
			if a.Spread {
				e.fail("Rest arguments can't be used with calculations.")
			}
			argExprs = append(argExprs, a.Value)
		}
	}
	if len(argExprs) == 0 {
		e.fail("Missing argument.")
	}
	if maxArgs != 0 && len(argExprs) > maxArgs {
		e.fail("Only %d argument(s) allowed, but %d were passed.", maxArgs, len(argExprs))
	}

	legacy := ""
	if calcLegacyGated[name] {
		legacy = name
	}
	args := make([]calcTerm, len(argExprs))
	for i, ex := range argExprs {
		args[i] = e.visitCalcExpr(ex, legacy)
	}
	// name is guaranteed to be in calcDispatch: tryCalculation only reaches here
	// for names present in calcArity, which has the same keys.
	return calcDispatch[name](e, args)
}

// calcDispatch maps each calculation function to its constructor. Its keys match
// calcArity exactly.
var calcDispatch = map[string]func(*evaluator, []calcTerm) Value{
	"calc":  func(e *evaluator, a []calcTerm) Value { return e.calcCalc(a[0]) },
	"min":   func(e *evaluator, a []calcTerm) Value { return e.calcMinMax("min", a) },
	"max":   func(e *evaluator, a []calcTerm) Value { return e.calcMinMax("max", a) },
	"clamp": func(e *evaluator, a []calcTerm) Value { return e.calcClamp(a) },
	"hypot": func(e *evaluator, a []calcTerm) Value { return e.calcHypot(a) },
	"sqrt": func(e *evaluator, a []calcTerm) Value {
		return e.calcSingle("sqrt", a[0], func(n *Number) *Number { return &Number{Val: math.Sqrt(n.Val)} }, true)
	},
	"sin": func(e *evaluator, a []calcTerm) Value {
		return e.calcSingle("sin", a[0], func(n *Number) *Number { return &Number{Val: math.Sin(n.coerceValueToUnit("rad"))} }, false)
	},
	"cos": func(e *evaluator, a []calcTerm) Value {
		return e.calcSingle("cos", a[0], func(n *Number) *Number { return &Number{Val: math.Cos(n.coerceValueToUnit("rad"))} }, false)
	},
	"tan": func(e *evaluator, a []calcTerm) Value {
		return e.calcSingle("tan", a[0], func(n *Number) *Number { return &Number{Val: math.Tan(n.coerceValueToUnit("rad"))} }, false)
	},
	"asin": func(e *evaluator, a []calcTerm) Value {
		return e.calcSingle("asin", a[0], func(n *Number) *Number { return radiansToDegrees(math.Asin(n.Val)) }, true)
	},
	"acos": func(e *evaluator, a []calcTerm) Value {
		return e.calcSingle("acos", a[0], func(n *Number) *Number { return radiansToDegrees(math.Acos(n.Val)) }, true)
	},
	"atan": func(e *evaluator, a []calcTerm) Value {
		return e.calcSingle("atan", a[0], func(n *Number) *Number { return radiansToDegrees(math.Atan(n.Val)) }, true)
	},
	"abs":       func(e *evaluator, a []calcTerm) Value { return e.calcAbs(a[0]) },
	"exp":       func(e *evaluator, a []calcTerm) Value { return e.calcExp(a[0]) },
	"sign":      func(e *evaluator, a []calcTerm) Value { return e.calcSign(a[0]) },
	"pow":       func(e *evaluator, a []calcTerm) Value { return e.calcPow(a[0], second(a)) },
	"atan2":     func(e *evaluator, a []calcTerm) Value { return e.calcAtan2(a[0], second(a)) },
	"log":       func(e *evaluator, a []calcTerm) Value { return e.calcLog(a[0], second(a)) },
	"mod":       func(e *evaluator, a []calcTerm) Value { return e.calcMod(a[0], second(a)) },
	"rem":       func(e *evaluator, a []calcTerm) Value { return e.calcRem(a[0], second(a)) },
	"calc-size": func(e *evaluator, a []calcTerm) Value { return e.calcSize(a[0], second(a)) },
	"round":     func(e *evaluator, a []calcTerm) Value { return e.calcRound(a[0], second(a), third(a), true) },
}

func second(args []calcTerm) calcTerm {
	if len(args) >= 2 {
		return args[1]
	}
	return nil
}

func third(args []calcTerm) calcTerm {
	if len(args) >= 3 {
		return args[2]
	}
	return nil
}

// visitCalcExpr evaluates an expression node in calculation context, producing a
// calcTerm (*Number, *SassString, *SassCalculation, or *CalcOp).
func (e *evaluator) visitCalcExpr(expr Expr, legacy string) calcTerm {
	switch x := expr.(type) {
	case *Paren:
		r := e.visitCalcExpr(x.Expr, legacy)
		if s, ok := r.(*SassString); ok {
			return &SassString{Text: "(" + s.Text + ")"}
		}
		return r
	case *Ident:
		if n, ok := calcConstant(x.Name); ok {
			return n
		}
		return &SassString{Text: x.Name}
	case *StringLit:
		// A plain unquoted identifier reaches the *Ident case above; an unquoted
		// StringLit here always has interpolation, so it never names a constant.
		// A quoted string is rejected later by calcSimplify.
		return e.evalString(x)
	case *InterpExpr:
		return &SassString{Text: e.stringifyInterp(e.evalExpr(x.Expr))}
	case *Binary:
		switch x.Op {
		case "+", "-", "*", "/":
			left := e.visitCalcExpr(x.Left, legacy)
			right := e.visitCalcExpr(x.Right, legacy)
			return e.calcOperate(x.Op, left, right, legacy != "")
		}
		e.fail("This operation can't be used in a calculation.")
	case *Unary:
		if x.Op == "-" || x.Op == "+" {
			inner := e.visitCalcExpr(x.Expr, legacy)
			if n, ok := inner.(*Number); ok {
				if x.Op == "-" {
					return &Number{Val: -n.Val, Numer: cloneUnits(n.Numer), Denom: cloneUnits(n.Denom)}
				}
				return n
			}
		}
		e.fail("This expression can't be used in a calculation.")
	case *NumberLit, *VarRef, *FuncCall:
		return e.valueToCalcTerm(e.evalExpr(expr))
	case *ListExpr:
		if x.Sep == SepSpace && !x.Bracketed && len(x.Elements) > 1 {
			parts := make([]string, len(x.Elements))
			for i, el := range x.Elements {
				t := e.visitCalcExpr(el, legacy)
				if op, ok := t.(*CalcOp); ok {
					if _, isParen := x.Elements[i].(*Paren); isParen {
						parts[i] = "(" + serializeCalcTerm(op, false) + ")"
						continue
					}
				}
				parts[i] = serializeCalcTerm(t, false)
			}
			return &SassString{Text: strings.Join(parts, " ")}
		}
		e.fail("This expression can't be used in a calculation.")
	}
	e.fail("This expression can't be used in a calculation.")
	return nil
}

func (e *evaluator) valueToCalcTerm(v Value) calcTerm {
	switch t := v.(type) {
	case *Number:
		return t
	case *SassCalculation:
		return t
	case *SassString:
		if !t.Quoted {
			return t
		}
	}
	e.fail("Value %s can't be used in a calculation.", serializeValue(v, false))
	return nil
}

func calcConstant(text string) (*Number, bool) {
	switch strings.ToLower(text) {
	case "pi":
		return &Number{Val: math.Pi}, true
	case "e":
		return &Number{Val: math.E}, true
	case "infinity":
		return &Number{Val: math.Inf(1)}, true
	case "-infinity":
		return &Number{Val: math.Inf(-1)}, true
	case "nan":
		return &Number{Val: math.NaN()}, true
	}
	return nil, false
}

// exprCalcSafe reports whether expr can appear in a calculation context (mirrors
// dart-sass's IsCalculationSafeVisitor).
func (e *evaluator) exprCalcSafe(expr Expr) bool {
	// Types that are never calculation-safe (BoolLit, ColorLit, MapExpr, NullLit,
	// Parent) and any unhandled type fall through to the final `return false`.
	switch x := expr.(type) {
	case *Binary:
		switch x.Op {
		case "+", "-", "*", "/":
			return e.exprCalcSafe(x.Left) && e.exprCalcSafe(x.Right)
		}
	case *FuncCall:
		return true
	case *ListExpr:
		if x.Sep != SepSpace || x.Bracketed || len(x.Elements) < 2 {
			return false
		}
		for _, el := range x.Elements {
			if !e.exprCalcSafe(el) {
				return false
			}
		}
		return true
	case *NumberLit:
		return true
	case *Paren:
		return e.exprCalcSafe(x.Expr)
	case *InterpExpr:
		return true
	case *VarRef:
		return true
	case *Unary:
		// A leading -/+ on a numeric literal is a signed number literal in
		// dart-sass (a calculation-safe NumberExpression), not a unary operation.
		if x.Op == "-" || x.Op == "+" {
			if _, ok := x.Expr.(*NumberLit); ok {
				return true
			}
		}
	case *Ident:
		return plainCalcSafe(x.Name)
	case *StringLit:
		if !x.Quoted {
			text, _ := x.Parts.isPlain()
			return plainCalcSafe(text)
		}
	}
	return false
}

func plainCalcSafe(text string) bool {
	if strings.HasPrefix(text, "!") || strings.HasPrefix(text, "#") {
		return false
	}
	if len(text) > 1 && text[1] == '+' {
		return false
	}
	if len(text) > 3 && text[3] == '(' {
		return false
	}
	return true
}
