// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import "strings"

// callInfo holds evaluated call arguments.
type callInfo struct {
	positional []Value
	named      map[string]Value
	e          *evaluator
	fn         string
}

func (ci *callInfo) get(i int, name string) (Value, bool) {
	if i >= 0 && i < len(ci.positional) {
		return ci.positional[i], true
	}
	if v, ok := ci.named[name]; ok {
		return v, true
	}
	return nil, false
}

func (ci *callInfo) require(i int, name string) Value {
	v, ok := ci.get(i, name)
	if !ok {
		ci.e.fail("Missing argument $%s.", name)
	}
	return v
}

func (ci *callInfo) num(i int, name string) *Number {
	return ci.e.asNumber(ci.require(i, name))
}

func (e *evaluator) asString(v Value) *SassString {
	if s, ok := v.(*SassString); ok {
		return s
	}
	return &SassString{Text: serializeValue(v, false)}
}

// callUserResolved calls a user-defined function with pre-resolved arguments.
func (e *evaluator) callUserResolved(fn *funcEntry, pos []Value, named map[string]Value) (result Value) {
	e.enter()
	defer e.leave()
	saved := e.env
	e.env = fn.env
	e.env.pushScope()
	e.bindResolved(fn.def.Params, pos, named)
	defer func() {
		e.env.popScope()
		e.env = saved
		if r := recover(); r != nil {
			if rs, ok := r.(returnSignal); ok {
				result = rs.value
				return
			}
			panic(r)
		}
	}()
	dummy := &frame{container: e.root, rootContainer: e.root, mediaParent: e.root, group: &groupInfo{}}
	for _, s := range fn.def.Body {
		e.evalStmt(s, dummy)
	}
	e.fail("Function %s did not return a value.", fn.def.Name)
	return sassNull
}

func (e *evaluator) asNumber(v Value) *Number {
	n, ok := v.(*Number)
	if !ok {
		e.fail("%s is not a number.", serializeValue(v, false))
	}
	return n
}

// evalArgs evaluates an ArgList into positional/named values, expanding spreads.
func (e *evaluator) evalArgs(args *ArgList) ([]Value, map[string]Value) {
	var pos []Value
	named := map[string]Value{}
	if args == nil {
		return pos, named
	}
	for _, a := range args.Args {
		// Binding an argument consumes it, so a bare slash number becomes its
		// quotient. Spreading a list makes each element its own argument, so the
		// elements are consumed individually too; an unspread list argument keeps
		// any slash its elements carry (withoutSlash is a no-op on the list).
		val := numWithoutSlash(e.evalExpr(a.Value))
		if a.Spread {
			switch s := val.(type) {
			case *List:
				for _, el := range s.Elements {
					pos = append(pos, numWithoutSlash(el))
				}
				// An argument list re-spreads its captured keyword arguments too.
				for k, v := range s.Keywords {
					named[k] = numWithoutSlash(v)
				}
			case *Map:
				for i := range s.Keys {
					if ks, ok := s.Keys[i].(*SassString); ok {
						named[normIdent(ks.Text)] = numWithoutSlash(s.Values[i])
					}
				}
			default:
				pos = append(pos, val)
			}
			continue
		}
		if a.Name != "" {
			// `-` and `_` are interchangeable in Sass identifiers, so a keyword
			// argument is matched against a parameter dash-normalised.
			named[normIdent(a.Name)] = val
		} else {
			pos = append(pos, val)
		}
	}
	return pos, named
}

func (e *evaluator) evalCall(x *FuncCall) Value {
	// if() is special: lazy evaluation of branches.
	if x.Namespace == "" && x.Name == "if" {
		return e.evalIfFunction(x)
	}
	// A function name beginning with "--" (e.g. `--a()`) is a custom-property-
	// style plain-CSS function: dart-sass never resolves it to a Sass function
	// (hyphen/underscore normalisation would otherwise alias it to a user
	// function like `__a`), so it always passes through verbatim.
	if x.Namespace == "" && strings.HasPrefix(x.Name, "--") {
		return e.plainCSSFunction(x)
	}
	// user-defined function?
	if fn := e.lookupFunc(x.Namespace, x.Name); fn != nil {
		return e.callUserFunc(fn, x.Args)
	}
	// CSS math functions (calc, min, max, clamp, and the math functions) are
	// evaluated as first-class calculations, not ordinary function calls.
	if x.Namespace == "" {
		if v, ok := e.tryCalculation(x); ok {
			return v
		}
	}
	// built-in?
	if bf, ok := e.lookupBuiltin(x.Namespace, x.Name); ok {
		pos, named := e.evalArgs(x.Args)
		return bf(&callInfo{positional: pos, named: named, e: e, fn: x.Name})
	}
	if x.Namespace != "" {
		e.fail("Undefined function %s.%s.", x.Namespace, x.Name)
	}
	// plain CSS function passthrough
	return e.plainCSSFunction(x)
}

func (e *evaluator) evalIfFunction(x *FuncCall) Value {
	// A spread argument ($args...) can only be resolved by evaluating it, so the
	// lazy branch selection below is unavailable: fall back to fully binding the
	// arguments and picking the taken branch from the resolved values.
	if x.Args != nil {
		for _, a := range x.Args.Args {
			if a.Spread {
				pos, named := e.evalArgs(x.Args)
				pick := func(name string, idx int) (Value, bool) {
					if v, ok := named[name]; ok {
						return v, true
					}
					if idx < len(pos) {
						return pos[idx], true
					}
					return nil, false
				}
				cond, ok1 := pick("condition", 0)
				tv, ok2 := pick("if-true", 1)
				fv, ok3 := pick("if-false", 2)
				if !ok1 || !ok2 || !ok3 {
					e.fail("if() requires three arguments.")
				}
				if cond.isTruthy() {
					return tv
				}
				return fv
			}
		}
	}
	if x.Args == nil || len(x.Args.Args) < 3 {
		e.fail("if() requires three arguments.")
	}
	var cond, t, f Expr
	for _, a := range x.Args.Args {
		switch a.Name {
		case "condition":
			cond = a.Value
		case "if-true":
			t = a.Value
		case "if-false":
			f = a.Value
		}
	}
	if cond == nil {
		cond = x.Args.Args[0].Value
		t = x.Args.Args[1].Value
		f = x.Args.Args[2].Value
	}
	if e.evalExpr(cond).isTruthy() {
		return e.evalExpr(t)
	}
	return e.evalExpr(f)
}

func (e *evaluator) plainCSSFunction(x *FuncCall) Value {
	var parts []string
	for _, a := range x.Args.Args {
		// A spread argument (`$list...`) is expanded in place: dart-sass serialises
		// the underlying value with its own separator (so a comma list becomes
		// `a, b` and a space list `a b`) rather than keeping a literal `...`.
		s := serializeValue(e.evalExpr(a.Value), false)
		if !a.Spread && a.Name != "" {
			s = "$" + a.Name + ": " + s
		}
		parts = append(parts, s)
	}
	return &SassString{Text: x.Name + "(" + strings.Join(parts, ", ") + ")", Quoted: false}
}

func (e *evaluator) lookupFunc(ns, name string) *funcEntry {
	name = normIdent(name)
	if ns != "" {
		if mod, ok := e.env.modules[ns]; ok {
			if f, ok := mod.funcs[name]; ok {
				return f
			}
		}
		return nil
	}
	if f, ok := e.env.funcs[name]; ok {
		return f
	}
	return nil
}

func (e *evaluator) callUserFunc(fn *funcEntry, args *ArgList) (result Value) {
	e.enter()
	defer e.leave()
	saved := e.env
	e.env = fn.env
	e.env.pushScope()
	e.bindArgs(fn.def.Params, args, saved)
	defer func() {
		e.env.popScope()
		e.env = saved
		if r := recover(); r != nil {
			if rs, ok := r.(returnSignal); ok {
				result = rs.value
				return
			}
			panic(r)
		}
	}()
	dummy := &frame{container: e.root, rootContainer: e.root, mediaParent: e.root, group: &groupInfo{}}
	for _, s := range fn.def.Body {
		e.evalStmt(s, dummy)
	}
	e.fail("Function %s did not return a value.", fn.def.Name)
	return sassNull
}

// bindArgs binds call arguments to parameters in the current (already pushed) scope.
func (e *evaluator) bindArgs(params *ParamList, args *ArgList, callEnv *environment) {
	savedEnv := e.env
	e.env = callEnv
	pos, named := e.evalArgs(args)
	e.env = savedEnv
	e.bindResolved(params, pos, named)
}

func (e *evaluator) bindResolved(params *ParamList, pos []Value, named map[string]Value) {
	if params == nil {
		return
	}
	pi := 0
	for _, p := range params.Params {
		if p.Rest {
			rest := &List{Sep: SepComma, IsArgList: true}
			for ; pi < len(pos); pi++ {
				rest.Elements = append(rest.Elements, pos[pi])
			}
			// Capture the call's trailing keyword arguments, keyed dash-normalised
			// without the leading "$", so meta.keywords and re-spreading work.
			if len(named) > 0 {
				rest.Keywords = map[string]Value{}
				for k, v := range named {
					rest.Keywords[normIdent(k)] = v
				}
			}
			e.env.defineVar(p.Name, rest)
			return
		}
		if pi < len(pos) {
			e.env.defineVar(p.Name, pos[pi])
			pi++
			continue
		}
		if v, ok := named[normIdent(p.Name)]; ok {
			e.env.defineVar(p.Name, v)
			continue
		}
		if p.Default != nil {
			e.env.defineVar(p.Name, numWithoutSlash(e.evalExpr(p.Default)))
			continue
		}
		e.fail("Missing argument $%s.", p.Name)
	}
}
