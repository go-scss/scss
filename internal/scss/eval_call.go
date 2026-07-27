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

func (ci *callInfo) opt(i int, name string, def Value) Value {
	if v, ok := ci.get(i, name); ok {
		return v
	}
	return def
}

func (ci *callInfo) num(i int, name string) *Number {
	return ci.e.asNumber(ci.require(i, name))
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
		val := e.evalExpr(a.Value)
		if a.Spread {
			switch s := val.(type) {
			case *List:
				pos = append(pos, s.Elements...)
			case *Map:
				for i := range s.Keys {
					if ks, ok := s.Keys[i].(*SassString); ok {
						named[ks.Text] = s.Values[i]
					}
				}
			default:
				pos = append(pos, val)
			}
			continue
		}
		if a.Name != "" {
			named[a.Name] = val
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
	// user-defined function?
	if fn := e.lookupFunc(x.Namespace, x.Name); fn != nil {
		return e.callUserFunc(fn, x.Args)
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
		s := serializeValue(e.evalExpr(a.Value), false)
		if a.Name != "" {
			s = "$" + a.Name + ": " + s
		}
		if a.Spread {
			s += "..."
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
			rest := &List{Sep: SepComma}
			for ; pi < len(pos); pi++ {
				rest.Elements = append(rest.Elements, pos[pi])
			}
			// attach named args as a map on an arglist (simplified)
			e.env.defineVar(p.Name, rest)
			return
		}
		if pi < len(pos) {
			e.env.defineVar(p.Name, pos[pi])
			pi++
			continue
		}
		if v, ok := named[p.Name]; ok {
			e.env.defineVar(p.Name, v)
			continue
		}
		if p.Default != nil {
			e.env.defineVar(p.Name, e.evalExpr(p.Default))
			continue
		}
		e.fail("Missing argument $%s.", p.Name)
	}
}
