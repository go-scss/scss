// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import (
	"math"
	"sort"
	"strings"
)

type builtinFunc func(ci *callInfo) Value

// lookupBuiltin resolves a namespaced or global built-in function.
func (e *evaluator) lookupBuiltin(ns, name string) (builtinFunc, bool) {
	name = normIdent(strings.ToLower(name))
	if ns != "" {
		alias, ok := e.env.builtinAliases[ns]
		if !ok {
			return nil, false
		}
		reg := moduleRegistry(alias)
		if reg == nil {
			return nil, false
		}
		f, ok := reg[name]
		return f, ok
	}
	// "as *" modules
	for _, m := range e.env.builtinGlobals {
		if f, ok := moduleRegistry(m)[name]; ok {
			return f, true
		}
	}
	if f, ok := globalFns[name]; ok {
		return f, true
	}
	return nil, false
}

func moduleRegistry(mod string) map[string]builtinFunc {
	switch mod {
	case "math":
		return mathFns
	case "color":
		return colorFns
	case "string":
		return stringFns
	case "list":
		return listFns
	case "map":
		return mapFns
	case "selector":
		return selectorFns
	case "meta":
		return metaFns
	}
	return nil
}

// number helpers -------------------------------------------------------------

func numOut(v float64, units ...string) *Number {
	return newNumber(v, units...)
}

func (ci *callInfo) str(i int, name string) *SassString {
	v := ci.require(i, name)
	s, ok := v.(*SassString)
	if !ok {
		ci.e.fail("%s is not a string.", serializeValue(v, false))
	}
	return s
}

// --- math module ---

var mathFns = map[string]builtinFunc{
	"div": func(ci *callInfo) Value {
		a := ci.require(0, "number1")
		b := ci.require(1, "number2")
		an, aok := a.(*Number)
		bn, bok := b.(*Number)
		if !aok || !bok {
			// Non-numeric operands fall back to the legacy "a/b" slash string,
			// matching dart-sass (which emits a deprecation warning here).
			return &SassString{Text: serializeValue(a, false) + "/" + serializeValue(b, false), Quoted: false}
		}
		return ci.e.numberArith("/", an, bn)
	},
	"percentage":  fnPercentage,
	"round":       unaryMath(fuzzyRound),
	"ceil":        unaryMath(math.Ceil),
	"floor":       unaryMath(math.Floor),
	"abs":         unaryMathUnit(math.Abs),
	"max":         fnMax,
	"min":         fnMin,
	"sqrt":        unaryMathUnitless(math.Sqrt),
	"sin":         trig(math.Sin),
	"cos":         trig(math.Cos),
	"tan":         trig(math.Tan),
	"asin":        invTrig(math.Asin),
	"acos":        invTrig(math.Acos),
	"atan":        invTrig(math.Atan),
	"atan2":       fnAtan2,
	"exp":         fnExp,
	"pow":         fnPow,
	"hypot":       fnHypot,
	"log":         fnLog,
	"unit":        fnUnit,
	"is-unitless": fnUnitless,
	"compatible":  fnComparable,
	"clamp":       fnClamp,
}

func unaryMath(f func(float64) float64) builtinFunc {
	return func(ci *callInfo) Value {
		n := ci.num(0, "number")
		return &Number{Val: f(n.Val), Numer: n.Numer, Denom: n.Denom}
	}
}

func unaryMathUnit(f func(float64) float64) builtinFunc {
	return func(ci *callInfo) Value {
		n := ci.num(0, "number")
		return &Number{Val: f(n.Val), Numer: n.Numer, Denom: n.Denom}
	}
}

func unaryMathUnitless(f func(float64) float64) builtinFunc {
	return func(ci *callInfo) Value {
		n := ci.num(0, "number")
		return numOut(f(n.Val))
	}
}

func trig(f func(float64) float64) builtinFunc {
	return func(ci *callInfo) Value {
		n := ci.num(0, "number")
		rad := n.Val
		if len(n.Numer) == 1 {
			switch n.Numer[0] {
			case "deg":
				rad = n.Val * math.Pi / 180
			case "grad":
				rad = n.Val * math.Pi / 200
			case "turn":
				rad = n.Val * 2 * math.Pi
			}
		}
		return numOut(f(rad))
	}
}

func fnPercentage(ci *callInfo) Value {
	n := ci.num(0, "number")
	return numOut(n.Val*100, "%")
}

func fnMax(ci *callInfo) Value {
	best := ci.num(0, "value")
	for i := 1; i < len(ci.positional); i++ {
		n := ci.e.asNumber(ci.positional[i])
		cv, _ := n.compatConvert(best)
		if cv > best.Val {
			best = n
		}
	}
	return best
}

func fnMin(ci *callInfo) Value {
	best := ci.num(0, "value")
	for i := 1; i < len(ci.positional); i++ {
		n := ci.e.asNumber(ci.positional[i])
		cv, _ := n.compatConvert(best)
		if cv < best.Val {
			best = n
		}
	}
	return best
}

func fnPow(ci *callInfo) Value {
	b := ci.num(0, "base")
	x := ci.num(1, "exponent")
	return numOut(math.Pow(b.Val, x.Val))
}

// invTrig implements the inverse trig functions (asin/acos/atan) that take a
// unitless number and return degrees.
func invTrig(f func(float64) float64) builtinFunc {
	return func(ci *callInfo) Value {
		n := ci.num(0, "number")
		ci.e.assertNoUnits(n, "number")
		return radiansToDegrees(f(n.Val))
	}
}

func fnAtan2(ci *callInfo) Value {
	y := ci.num(0, "y")
	x := ci.num(1, "x")
	return radiansToDegrees(math.Atan2(y.Val, x.convertValueToMatch(y)))
}

func fnExp(ci *callInfo) Value {
	n := ci.num(0, "number")
	ci.e.assertNoUnits(n, "number")
	return numOut(math.Pow(math.E, n.Val))
}

func fnHypot(ci *callInfo) Value {
	sum := 0.0
	for _, v := range ci.positional {
		n := ci.e.asNumber(v)
		sum += n.Val * n.Val
	}
	return numOut(math.Sqrt(sum))
}

func fnLog(ci *callInfo) Value {
	n := ci.num(0, "number")
	if b, ok := ci.get(1, "base"); ok {
		// A null base means the natural logarithm.
		if _, isNull := b.(*Null); !isNull {
			return numOut(math.Log(n.Val) / math.Log(ci.e.asNumber(b).Val))
		}
	}
	return numOut(math.Log(n.Val))
}

// numGE reports whether a >= b once b is converted into a's units, with Sass's
// fuzzy tolerance. Incompatible units are an error.
func (e *evaluator) numGE(a, b *Number) bool {
	bv, ok := b.compatConvert(a)
	if !ok {
		e.fail("%s and %s have incompatible units.", serializeValue(a, false), serializeValue(b, false))
	}
	return a.Val > bv || fuzzyEquals(a.Val, bv)
}

func fnClamp(ci *callInfo) Value {
	min := ci.num(0, "min")
	n := ci.num(1, "number")
	max := ci.num(2, "max")
	// dart-sass's clamp: an inverted range collapses to min, and each branch
	// returns the winning argument with its own units preserved.
	switch {
	case ci.e.numGE(min, max):
		return min
	case ci.e.numGE(min, n):
		return min
	case ci.e.numGE(n, max):
		return max
	default:
		return n
	}
}

func fnUnit(ci *callInfo) Value {
	n := ci.num(0, "number")
	return &SassString{Text: n.unitString(), Quoted: true}
}

func fnUnitless(ci *callInfo) Value {
	n := ci.num(0, "number")
	return boolean(!n.hasUnits())
}

func fnComparable(ci *callInfo) Value {
	a := ci.num(0, "number1")
	b := ci.num(1, "number2")
	_, ok := b.compatConvert(a)
	if !a.hasUnits() || !b.hasUnits() {
		ok = true
	}
	return boolean(ok)
}

// --- list module ---

var listFns = map[string]builtinFunc{
	"length":       fnLength,
	"nth":          fnNth,
	"join":         fnJoin,
	"append":       fnAppend,
	"index":        fnIndex,
	"separator":    fnSeparator,
	"is-bracketed": fnIsBracketed,
	"set-nth":      fnSetNth,
	"zip":          fnZip,
	"slash":        fnSlashList,
}

func asListVal(v Value) *List {
	if l, ok := v.(*List); ok {
		return l
	}
	if m, ok := v.(*Map); ok {
		return &List{Elements: m.asList(), Sep: SepComma}
	}
	return &List{Elements: []Value{v}, Sep: SepSpace}
}

func fnLength(ci *callInfo) Value {
	v := ci.require(0, "list")
	if m, ok := v.(*Map); ok {
		return numOut(float64(len(m.Keys)))
	}
	return numOut(float64(len(asListVal(v).Elements)))
}

func fnNth(ci *callInfo) Value {
	l := asListVal(ci.require(0, "list"))
	n := int(ci.num(1, "n").Val)
	idx := n
	if idx < 0 {
		idx = len(l.Elements) + idx + 1
	}
	if idx < 1 || idx > len(l.Elements) {
		ci.e.fail("$n: Invalid index %d for a list with %d elements.", n, len(l.Elements))
	}
	return l.Elements[idx-1]
}

func fnSetNth(ci *callInfo) Value {
	l := asListVal(ci.require(0, "list"))
	n := int(ci.num(1, "n").Val)
	val := ci.require(2, "value")
	idx := n
	if idx < 0 {
		idx = len(l.Elements) + idx + 1
	}
	if idx < 1 || idx > len(l.Elements) {
		ci.e.fail("$n: Invalid index.")
	}
	out := append([]Value(nil), l.Elements...)
	out[idx-1] = val
	return &List{Elements: out, Sep: l.Sep, Bracketed: l.Bracketed}
}

func fnJoin(ci *callInfo) Value {
	l1 := asListVal(ci.require(0, "list1"))
	l2 := asListVal(ci.require(1, "list2"))
	sep := l1.Sep
	if sepArg, ok := ci.get(2, "separator"); ok {
		sep = parseSeparatorArg(sepArg, sep)
	} else if sep == SepUndecided {
		sep = l2.Sep
	}
	if sep == SepUndecided {
		sep = SepSpace
	}
	elems := append(append([]Value(nil), l1.Elements...), l2.Elements...)
	bracketed := l1.Bracketed
	if b, ok := ci.get(3, "bracketed"); ok {
		bracketed = b.isTruthy()
	}
	return &List{Elements: elems, Sep: sep, Bracketed: bracketed}
}

func fnAppend(ci *callInfo) Value {
	l := asListVal(ci.require(0, "list"))
	val := ci.require(1, "val")
	sep := l.Sep
	if sepArg, ok := ci.get(2, "separator"); ok {
		sep = parseSeparatorArg(sepArg, sep)
	}
	if sep == SepUndecided {
		sep = SepSpace
	}
	elems := append(append([]Value(nil), l.Elements...), val)
	return &List{Elements: elems, Sep: sep, Bracketed: l.Bracketed}
}

func parseSeparatorArg(v Value, def Separator) Separator {
	if s, ok := v.(*SassString); ok {
		switch s.Text {
		case "comma":
			return SepComma
		case "space":
			return SepSpace
		case "slash":
			return SepSlash
		case "auto":
			return def
		}
	}
	return def
}

func fnIndex(ci *callInfo) Value {
	l := asListVal(ci.require(0, "list"))
	val := ci.require(1, "value")
	for i, e := range l.Elements {
		if e.equals(val) {
			return numOut(float64(i + 1))
		}
	}
	return sassNull
}

func fnSeparator(ci *callInfo) Value {
	l := asListVal(ci.require(0, "list"))
	switch l.Sep {
	case SepComma:
		return &SassString{Text: "comma"}
	case SepSlash:
		return &SassString{Text: "slash"}
	default:
		return &SassString{Text: "space"}
	}
}

func fnIsBracketed(ci *callInfo) Value {
	l := asListVal(ci.require(0, "list"))
	return boolean(l.Bracketed)
}

func fnZip(ci *callInfo) Value {
	lists := make([]*List, len(ci.positional))
	minLen := -1
	for i, v := range ci.positional {
		lists[i] = asListVal(v)
		if minLen < 0 || len(lists[i].Elements) < minLen {
			minLen = len(lists[i].Elements)
		}
	}
	var out []Value
	for i := 0; i < minLen; i++ {
		row := make([]Value, len(lists))
		for j := range lists {
			row[j] = lists[j].Elements[i]
		}
		out = append(out, &List{Elements: row, Sep: SepSpace})
	}
	return &List{Elements: out, Sep: SepComma}
}

func fnSlashList(ci *callInfo) Value {
	return &List{Elements: append([]Value(nil), ci.positional...), Sep: SepSlash}
}

// --- map module ---

var mapFns = map[string]builtinFunc{
	"get":         fnMapGet,
	"has-key":     fnMapHasKey,
	"keys":        fnMapKeys,
	"values":      fnMapValues,
	"merge":       fnMapMerge,
	"remove":      fnMapRemove,
	"set":         fnMapSet,
	"deep-merge":  fnMapDeepMerge,
	"deep-remove": fnMapDeepRemove,
}

func asMapVal(ci *callInfo, v Value) *Map {
	if m, ok := v.(*Map); ok {
		return m
	}
	if l, ok := v.(*List); ok && len(l.Elements) == 0 {
		return &Map{}
	}
	ci.e.fail("%s is not a map.", serializeValue(v, false))
	return nil
}

func fnMapGet(ci *callInfo) Value {
	m := asMapVal(ci, ci.require(0, "map"))
	key := ci.require(1, "key")
	if len(ci.positional) > 2 {
		// nested keys
		cur := Value(m)
		for i := 1; i < len(ci.positional); i++ {
			mm, ok := cur.(*Map)
			if !ok {
				return sassNull
			}
			v, found := mm.get(ci.positional[i])
			if !found {
				return sassNull
			}
			cur = v
		}
		return cur
	}
	if v, ok := m.get(key); ok {
		return v
	}
	return sassNull
}

func fnMapHasKey(ci *callInfo) Value {
	m, keys := mapKeyArgs(ci)
	if len(keys) == 0 {
		ci.e.fail("Missing argument $key.")
	}
	return boolean(mapHasKeyPath(m, keys))
}

func fnMapKeys(ci *callInfo) Value {
	m := asMapVal(ci, ci.require(0, "map"))
	return &List{Elements: append([]Value(nil), m.Keys...), Sep: SepComma}
}

func fnMapValues(ci *callInfo) Value {
	m := asMapVal(ci, ci.require(0, "map"))
	return &List{Elements: append([]Value(nil), m.Values...), Sep: SepComma}
}

func fnMapMerge(ci *callInfo) Value {
	m1 := asMapVal(ci, ci.require(0, "map1"))
	// Simple form: map.merge($map1, $map2).
	if m2v, ok := ci.named["map2"]; ok {
		return mapMergeShallow(m1, asMapVal(ci, m2v))
	}
	// Nested form: map.merge($map1, $keys..., $map2).
	n := len(ci.positional)
	if n < 2 {
		ci.e.fail("Missing argument $map2.")
	}
	m2 := asMapVal(ci, ci.positional[n-1])
	keys := ci.positional[1 : n-1]
	return mapMergePath(m1, keys, m2)
}

func fnMapSet(ci *callInfo) Value {
	m := asMapVal(ci, ci.require(0, "map"))
	// Simple named form: map.set($map, $key, $value).
	if val, ok := ci.named["value"]; ok {
		return mapSetPath(m, []Value{ci.require(1, "key")}, val)
	}
	// Positional form: map.set($map, $key, $keys..., $value).
	n := len(ci.positional)
	if n < 3 {
		ci.e.fail("Missing argument $value.")
	}
	keys := ci.positional[1 : n-1]
	return mapSetPath(m, keys, ci.positional[n-1])
}

func fnMapRemove(ci *callInfo) Value {
	m, keys := mapKeyArgs(ci)
	out := &Map{}
	for i := range m.Keys {
		remove := false
		for _, k := range keys {
			if m.Keys[i].equals(k) {
				remove = true
				break
			}
		}
		if !remove {
			out.set(m.Keys[i], m.Values[i])
		}
	}
	return out
}

func fnMapDeepMerge(ci *callInfo) Value {
	m1 := asMapVal(ci, ci.require(0, "map1"))
	m2 := asMapVal(ci, ci.require(1, "map2"))
	return mapDeepMerge(m1, m2)
}

func fnMapDeepRemove(ci *callInfo) Value {
	m, keys := mapKeyArgs(ci)
	if len(keys) == 0 {
		ci.e.fail("Missing argument $key.")
	}
	return mapDeepRemove(m, keys)
}

// --- sass:selector module ---

var selectorFns = map[string]builtinFunc{
	"nest":             fnSelectorNest,
	"append":           fnSelectorAppend,
	"unify":            fnSelectorUnify,
	"extend":           fnSelectorExtend,
	"replace":          fnSelectorReplace,
	"is-superselector": fnSelectorIsSuperselector,
	"simple-selectors": fnSelectorSimpleSelectors,
	"parse":            fnSelectorParse,
}

func fnSelectorNest(ci *callInfo) Value {
	selectors := ci.positional
	if len(selectors) == 0 {
		selPanic("$selectors: At least one selector must be passed.")
	}
	var parent *selList
	for _, v := range selectors {
		child := valueToSelectorList(v, true)
		child = child.nestWithin(parent, true, false)
		parent = child
	}
	return selListToSassList(parent)
}

func fnSelectorAppend(ci *callInfo) Value {
	selectors := ci.positional
	if len(selectors) == 0 {
		selPanic("$selectors: At least one selector must be passed.")
	}
	parent := valueToSelectorList(selectors[0], false)
	for _, v := range selectors[1:] {
		child := valueToSelectorList(v, false)
		var complexes []*selComplex
		for _, complex := range child.components {
			if len(complex.leadingCombinators) != 0 {
				selPanic("Can't append " + complex.String() + " to " + parent.String() + ".")
			}
			component := complex.components[0]
			newCompound := prependParentCompound(component.selector)
			if newCompound == nil {
				selPanic("Can't append " + complex.String() + " to " + parent.String() + ".")
			}
			comps := []complexComponent{{selector: newCompound, combinators: component.combinators}}
			comps = append(comps, complex.components[1:]...)
			complexes = append(complexes, &selComplex{components: comps})
		}
		parent = (&selList{components: complexes}).nestWithin(parent, true, false)
	}
	return selListToSassList(parent)
}

func fnSelectorUnify(ci *callInfo) Value {
	a := valueToSelectorList(ci.require(0, "selector1"), false)
	b := valueToSelectorList(ci.require(1, "selector2"), false)
	u := a.unify(b)
	if u == nil {
		return sassNull
	}
	return selListToSassList(u)
}

func fnSelectorExtend(ci *callInfo) Value {
	selector := valueToSelectorList(ci.require(0, "selector"), false)
	target := valueToSelectorList(ci.require(1, "extendee"), false)
	source := valueToSelectorList(ci.require(2, "extender"), false)
	return selListToSassList(extendOrReplace(selector, source, target, extendAllTargets))
}

func fnSelectorReplace(ci *callInfo) Value {
	selector := valueToSelectorList(ci.require(0, "selector"), false)
	target := valueToSelectorList(ci.require(1, "original"), false)
	source := valueToSelectorList(ci.require(2, "replacement"), false)
	return selListToSassList(extendOrReplace(selector, source, target, extendReplace))
}

func fnSelectorIsSuperselector(ci *callInfo) Value {
	a := valueToSelectorList(ci.require(0, "super"), false)
	b := valueToSelectorList(ci.require(1, "sub"), false)
	return boolean(a.isSuperList(b))
}

func fnSelectorSimpleSelectors(ci *callInfo) Value {
	compound := valueToCompoundSelector(ci.require(0, "selector"))
	elems := make([]Value, len(compound.components))
	for i, simple := range compound.components {
		elems[i] = &SassString{Text: selSimpleString(simple), Quoted: false}
	}
	return &List{Elements: elems, Sep: SepComma}
}

func fnSelectorParse(ci *callInfo) Value {
	return selListToSassList(valueToSelectorList(ci.require(0, "selector"), false))
}

// prependParentCompound adds a ParentSelector to the front of a compound, per
// Dart Sass's _prependParent; returns nil if that wouldn't be valid.
func prependParentCompound(compound *compoundSel) *compoundSel {
	switch first := compound.components[0].(type) {
	case *universalSel:
		return nil
	case *typeSel:
		if first.name.ns != nil {
			return nil
		}
		rest := append([]simpleSel{&parentSel{suffix: strptr(first.name.name)}}, compound.components[1:]...)
		return &compoundSel{components: rest}
	default:
		comps := append([]simpleSel{&parentSel{}}, compound.components...)
		return &compoundSel{components: comps}
	}
}

// valueToSelectorList converts a selector-parse()-style value to a *selList.
func valueToSelectorList(v Value, allowParent bool) *selList {
	str, ok := selectorStringOrNil(v)
	if !ok {
		selPanic(serializeValue(v, false) + " is not a valid selector: it must be a string,\n" +
			"a list of strings, or a list of lists of strings.")
	}
	list, err := parseSelectorListStrErr(str, allowParent, false)
	if err != nil {
		panic(err)
	}
	return list
}

func valueToCompoundSelector(v Value) *compoundSel {
	str, ok := selectorStringOrNil(v)
	if !ok {
		selPanic(serializeValue(v, false) + " is not a valid selector: it must be a string,\n" +
			"a list of strings, or a list of lists of strings.")
	}
	return parseCompoundSelectorStr(str, false)
}

// selectorStringOrNil implements Dart Sass's Value._selectorStringOrNull.
func selectorStringOrNil(v Value) (string, bool) {
	switch x := v.(type) {
	case *SassString:
		return x.Text, true
	case *List:
		if len(x.Elements) == 0 {
			return "", false
		}
		var parts []string
		switch x.Sep {
		case SepComma:
			for _, complex := range x.Elements {
				switch c := complex.(type) {
				case *SassString:
					parts = append(parts, c.Text)
				case *List:
					if c.Sep != SepSpace {
						return "", false
					}
					s, ok := selectorStringOrNil(c)
					if !ok {
						return "", false
					}
					parts = append(parts, s)
				default:
					return "", false
				}
			}
			return strings.Join(parts, ", "), true
		case SepSlash:
			return "", false
		default:
			for _, compound := range x.Elements {
				s, ok := compound.(*SassString)
				if !ok {
					return "", false
				}
				parts = append(parts, s.Text)
			}
			return strings.Join(parts, " "), true
		}
	}
	return "", false
}

func selPanic(msg string) { panic(&SassError{Msg: msg}) }

// selListToSassList renders a selector list as a comma list of space lists of
// strings, matching Dart Sass's SelectorList.asSassList.
func selListToSassList(sl *selList) Value {
	complexes := make([]Value, len(sl.components))
	for i, complex := range sl.components {
		var items []Value
		for _, comb := range complex.leadingCombinators {
			items = append(items, &SassString{Text: comb.String(), Quoted: false})
		}
		for _, component := range complex.components {
			items = append(items, &SassString{Text: compoundString(component.selector), Quoted: false})
			for _, comb := range component.combinators {
				items = append(items, &SassString{Text: comb.String(), Quoted: false})
			}
		}
		complexes[i] = &List{Elements: items, Sep: SepSpace}
	}
	return &List{Elements: complexes, Sep: SepComma}
}

func compoundString(c *compoundSel) string {
	var sb strings.Builder
	c.write(&sb, false)
	return sb.String()
}

// --- meta module (subset) ---

// knownFeatures are the feature strings meta.feature-exists() reports as
// supported, matching dart-sass.
var knownFeatures = map[string]bool{
	"global-builtin":              true,
	"extend-selector-pseudoclass": true,
	"units-level-3":               true,
	"at-error":                    true,
	"custom-property":             true,
	"global-variable-shadowing":   true,
}

var metaFns = map[string]builtinFunc{
	"type-of":  fnTypeOf,
	"inspect":  fnInspect,
	"keywords": fnKeywords,
	"feature-exists": func(ci *callInfo) Value {
		return boolean(knownFeatures[ci.str(0, "feature").Text])
	},
	"variable-exists": func(ci *callInfo) Value {
		return boolean(ci.e.env.hasVar(ci.str(0, "name").Text))
	},
	"content-exists": func(ci *callInfo) Value {
		return boolean(ci.e.env.content != nil)
	},
	"calc-args": fnCalcArgs,
	// First-class function/mixin and reflection built-ins are registered in an
	// init() (see meta.go) to avoid an initialisation cycle: they reference
	// moduleRegistry, which references this map.
}

// fnCalcArgs implements meta.calc-args: the arguments of a calculation as a
// comma-separated list. Operation nodes are returned as unquoted strings.
func fnCalcArgs(ci *callInfo) Value {
	v := ci.require(0, "calc")
	c, ok := v.(*SassCalculation)
	if !ok {
		ci.e.fail("$calc: %s is not a calculation.", serializeValue(v, false))
	}
	elems := make([]Value, len(c.Args))
	for i, a := range c.Args {
		switch t := a.(type) {
		case *Number:
			elems[i] = t
		case *SassString:
			elems[i] = t
		case *SassCalculation:
			elems[i] = t
		default:
			elems[i] = &SassString{Text: serializeCalcTerm(a, false)}
		}
	}
	return &List{Elements: elems, Sep: SepComma}
}

func fnTypeOf(ci *callInfo) Value {
	return &SassString{Text: typeName(ci.require(0, "value")), Quoted: false}
}

func typeName(v Value) string {
	switch x := v.(type) {
	case *Number:
		return "number"
	case *SassColor:
		return "color"
	case *SassString:
		return "string"
	case *Boolean:
		return "bool"
	case *Null:
		return "null"
	case *Map:
		return "map"
	case *List:
		if x.IsArgList {
			return "arglist"
		}
		return "list"
	case *SassCalculation:
		return "calculation"
	case *SassFunction:
		return "function"
	case *SassMixin:
		return "mixin"
	}
	return "unknown"
}

func fnInspect(ci *callInfo) Value {
	return &SassString{Text: inspect(ci.require(0, "value")), Quoted: false}
}

func inspect(v Value) string {
	switch x := v.(type) {
	case *SassString:
		if x.Quoted {
			return serializeQuoted(x.Text)
		}
		return x.Text
	case *List:
		return inspectList(x)
	case *Map:
		parts := make([]string, len(x.Keys))
		for i := range x.Keys {
			parts[i] = inspectMapElement(x.Keys[i]) + ": " + inspectMapElement(x.Values[i])
		}
		return "(" + strings.Join(parts, ", ") + ")"
	case *Null:
		return "null"
	case *SassFunction:
		return "get-function(" + serializeQuoted(x.name) + ")"
	case *SassMixin:
		return "get-mixin(" + serializeQuoted(x.name) + ")"
	}
	return serializeValue(v, false)
}

// inspectList renders a list the way dart-sass's inspect serializer does,
// including single-element comma/slash trailing separators and the parenthesis
// rules for nested lists.
func inspectList(x *List) string {
	if len(x.Elements) == 0 {
		if x.Bracketed {
			return "[]"
		}
		return "()"
	}
	sepStr := " "
	switch x.Sep {
	case SepComma:
		sepStr = ", "
	case SepSlash:
		sepStr = " / "
	}
	parts := make([]string, len(x.Elements))
	for i, e := range x.Elements {
		s := inspect(e)
		if listElementNeedsParens(x.Sep, e) {
			s = "(" + s + ")"
		}
		parts[i] = s
	}
	var sb strings.Builder
	if x.Bracketed {
		sb.WriteByte('[')
	}
	singleton := len(x.Elements) == 1 && (x.Sep == SepComma || x.Sep == SepSlash)
	if singleton && !x.Bracketed {
		sb.WriteByte('(')
	}
	sb.WriteString(strings.Join(parts, sepStr))
	if singleton {
		if x.Sep == SepSlash {
			sb.WriteByte('/')
		} else {
			sb.WriteByte(',')
		}
		if !x.Bracketed {
			sb.WriteByte(')')
		}
	}
	if x.Bracketed {
		sb.WriteByte(']')
	}
	return sb.String()
}

// listElementNeedsParens reports whether a nested list element must be
// parenthesised when inspected inside a parent list of the given separator.
func listElementNeedsParens(parent Separator, v Value) bool {
	l, ok := v.(*List)
	if !ok || len(l.Elements) < 2 || l.Bracketed {
		return false
	}
	switch parent {
	case SepComma:
		return l.Sep == SepComma
	case SepSlash:
		return l.Sep == SepComma || l.Sep == SepSlash
	default:
		return l.Sep != SepUndecided
	}
}

// inspectMapElement renders a map key or value, parenthesising an unbracketed
// comma-separated list as dart-sass does.
func inspectMapElement(v Value) string {
	if l, ok := v.(*List); ok && l.Sep == SepComma && !l.Bracketed {
		return "(" + inspect(v) + ")"
	}
	return inspect(v)
}

func fnKeywords(ci *callInfo) Value {
	v := ci.require(0, "args")
	l, ok := v.(*List)
	if !ok || !l.IsArgList {
		ci.e.fail("$args: %s is not an argument list.", serializeValue(v, false))
	}
	out := &Map{}
	for _, k := range sortedKeys(l.Keywords) {
		out.set(&SassString{Text: k, Quoted: false}, l.Keywords[k])
	}
	return out
}

// --- string module ---

var stringFns = map[string]builtinFunc{
	"quote":         fnQuote,
	"unquote":       fnUnquote,
	"length":        fnStrLength,
	"insert":        fnStrInsert,
	"index":         fnStrIndex,
	"slice":         fnStrSlice,
	"to-upper-case": fnUpper,
	"to-lower-case": fnLower,
	"unique-id":     func(ci *callInfo) Value { return &SassString{Text: "u" + "id00000", Quoted: false} },
	"split":         fnStrSplit,
}

func fnQuote(ci *callInfo) Value {
	s := ci.require(0, "string")
	if str, ok := s.(*SassString); ok {
		return &SassString{Text: str.Text, Quoted: true}
	}
	return &SassString{Text: serializeValue(s, false), Quoted: true}
}

func fnUnquote(ci *callInfo) Value {
	s := ci.str(0, "string")
	return &SassString{Text: s.Text, Quoted: false}
}

func fnStrLength(ci *callInfo) Value {
	return numOut(float64(len([]rune(ci.str(0, "string").Text))))
}

// asciiMap applies f to the ASCII letters of s only; Sass case conversion is
// defined over US-ASCII and leaves all other code points untouched.
func asciiMap(s string, f func(byte) byte) string {
	b := []byte(s)
	for i := 0; i < len(b); i++ {
		if b[i] < 0x80 {
			b[i] = f(b[i])
		}
	}
	return string(b)
}

func fnUpper(ci *callInfo) Value {
	s := ci.str(0, "string")
	return &SassString{Text: asciiMap(s.Text, func(c byte) byte {
		if c >= 'a' && c <= 'z' {
			return c - 32
		}
		return c
	}), Quoted: s.Quoted}
}

func fnLower(ci *callInfo) Value {
	s := ci.str(0, "string")
	return &SassString{Text: asciiMap(s.Text, func(c byte) byte {
		if c >= 'A' && c <= 'Z' {
			return c + 32
		}
		return c
	}), Quoted: s.Quoted}
}

func fnStrInsert(ci *callInfo) Value {
	s := ci.str(0, "string")
	insert := ci.str(1, "insert")
	idx := int(ci.num(2, "index").Val)
	runes := []rune(s.Text)
	// str-insert uses a 1-based index for the character the insertion precedes.
	pos := idx
	if pos < 0 {
		pos = len(runes) + pos + 2
	}
	pos-- // to 0-based
	if pos < 0 {
		pos = 0
	}
	if pos > len(runes) {
		pos = len(runes)
	}
	res := string(runes[:pos]) + insert.Text + string(runes[pos:])
	return &SassString{Text: res, Quoted: s.Quoted}
}

func fnStrIndex(ci *callInfo) Value {
	s := ci.str(0, "string")
	sub := ci.str(1, "substring")
	idx := strings.Index(s.Text, sub.Text)
	if idx < 0 {
		return sassNull
	}
	return numOut(float64(len([]rune(s.Text[:idx])) + 1))
}

// codepointForIndex maps a 1-based Sass string index (which may be negative,
// counting from the end) to a 0-based code-point offset, mirroring dart-sass's
// _codepointForIndex.
func codepointForIndex(index, length int, allowNegative bool) int {
	if index == 0 {
		return 0
	}
	if index > 0 {
		if index-1 < length {
			return index - 1
		}
		return length
	}
	result := length + index
	if result < 0 && !allowNegative {
		return 0
	}
	return result
}

func fnStrSlice(ci *callInfo) Value {
	s := ci.str(0, "string")
	runes := []rune(s.Text)
	n := len(runes)
	start := int(ci.num(1, "start-at").Val)
	end := n
	if v, ok := ci.get(2, "end-at"); ok {
		end = int(ci.e.asNumber(v).Val)
	}
	// An end index of 0 always yields the empty string, regardless of start.
	if end == 0 {
		return &SassString{Text: "", Quoted: s.Quoted}
	}
	startIdx := codepointForIndex(start, n, false)
	endIdx := codepointForIndex(end, n, true)
	if endIdx == n {
		endIdx--
	}
	// codepointForIndex clamps a non-negative request to [0, n], so startIdx is
	// always in range here; an out-of-range or inverted span yields "".
	if endIdx < startIdx || startIdx >= n {
		return &SassString{Text: "", Quoted: s.Quoted}
	}
	return &SassString{Text: string(runes[startIdx : endIdx+1]), Quoted: s.Quoted}
}

func fnStrSplit(ci *callInfo) Value {
	s := ci.str(0, "string")
	sep := ci.str(1, "separator")
	limit := -1
	if v, ok := ci.get(2, "limit"); ok {
		if _, isNull := v.(*Null); !isNull {
			limit = int(ci.e.asNumber(v).Val)
			if limit < 1 {
				ci.e.fail("$limit: Must be 1 or greater, was %d.", limit)
			}
		}
	}
	// An empty string always yields an empty (comma-separated, bracketed) list.
	if s.Text == "" {
		return &List{Sep: SepComma, Bracketed: true}
	}
	var parts []string
	if sep.Text == "" {
		// An empty separator splits into individual code points.
		for _, r := range s.Text {
			parts = append(parts, string(r))
		}
	} else {
		lastEnd := 0
		for {
			if limit >= 0 && len(parts) == limit {
				break
			}
			idx := strings.Index(s.Text[lastEnd:], sep.Text)
			if idx < 0 {
				break
			}
			parts = append(parts, s.Text[lastEnd:lastEnd+idx])
			lastEnd += idx + len(sep.Text)
		}
		parts = append(parts, s.Text[lastEnd:])
	}
	elems := make([]Value, len(parts))
	for i, p := range parts {
		elems[i] = &SassString{Text: p, Quoted: s.Quoted}
	}
	return &List{Elements: elems, Sep: SepComma, Bracketed: true}
}

// sort helper for deterministic iteration where needed.
var _ = sort.Strings
