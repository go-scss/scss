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
		a := ci.num(0, "number1")
		b := ci.num(1, "number2")
		return ci.e.numberArith("/", a, b)
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
		return numOut(math.Log(n.Val) / math.Log(ci.e.asNumber(b).Val))
	}
	return numOut(math.Log(n.Val))
}

func fnClamp(ci *callInfo) Value {
	min := ci.num(0, "min")
	n := ci.num(1, "number")
	max := ci.num(2, "max")
	v := n.Val
	if v < min.Val {
		v = min.Val
	}
	if v > max.Val {
		v = max.Val
	}
	return &Number{Val: v, Numer: n.Numer, Denom: n.Denom}
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
	"get":     fnMapGet,
	"has-key": fnMapHasKey,
	"keys":    fnMapKeys,
	"values":  fnMapValues,
	"merge":   fnMapMerge,
	"remove":  fnMapRemove,
	"set":     fnMapSet,
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
	m := asMapVal(ci, ci.require(0, "map"))
	_, ok := m.get(ci.require(1, "key"))
	return boolean(ok)
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
	m2 := asMapVal(ci, ci.require(1, "map2"))
	out := &Map{Keys: append([]Value(nil), m1.Keys...), Values: append([]Value(nil), m1.Values...)}
	for i := range m2.Keys {
		out.set(m2.Keys[i], m2.Values[i])
	}
	return out
}

func fnMapSet(ci *callInfo) Value {
	m := asMapVal(ci, ci.require(0, "map"))
	key := ci.require(1, "key")
	val := ci.require(2, "value")
	out := &Map{Keys: append([]Value(nil), m.Keys...), Values: append([]Value(nil), m.Values...)}
	out.set(key, val)
	return out
}

func fnMapRemove(ci *callInfo) Value {
	m := asMapVal(ci, ci.require(0, "map"))
	out := &Map{}
	for i := range m.Keys {
		remove := false
		for j := 1; j < len(ci.positional); j++ {
			if m.Keys[i].equals(ci.positional[j]) {
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

// --- selector module (minimal) ---

var selectorFns = map[string]builtinFunc{
	"nest":   fnSelectorNest,
	"append": fnSelectorAppend,
	"unify":  fnSelectorUnify,
}

func fnSelectorNest(ci *callInfo) Value {
	cur := selectorList{}
	for i, v := range ci.positional {
		child := parseSelectorList(selectorText(v))
		if i == 0 {
			cur = child
		} else {
			cur = resolveNesting(child, cur)
		}
	}
	return selectorToValue(cur)
}

func fnSelectorAppend(ci *callInfo) Value {
	var parts []string
	for _, v := range ci.positional {
		parts = append(parts, selectorText(v))
	}
	return &SassString{Text: strings.Join(parts, ""), Quoted: false}
}

func fnSelectorUnify(ci *callInfo) Value {
	a := selectorText(ci.require(0, "selector1"))
	b := selectorText(ci.require(1, "selector2"))
	return &SassString{Text: a + b, Quoted: false}
}

func selectorText(v Value) string {
	switch x := v.(type) {
	case *SassString:
		return x.Text
	case *List:
		parts := make([]string, len(x.Elements))
		for i, e := range x.Elements {
			parts[i] = selectorText(e)
		}
		if x.Sep == SepComma {
			return strings.Join(parts, ", ")
		}
		return strings.Join(parts, " ")
	}
	return serializeValue(v, false)
}

func selectorToValue(sl selectorList) Value {
	elems := make([]Value, len(sl.complexes))
	for i, cx := range sl.complexes {
		elems[i] = &SassString{Text: cx.serialize(false), Quoted: false}
	}
	return &List{Elements: elems, Sep: SepComma}
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
	"global-variable-exists": func(ci *callInfo) Value {
		_, ok := ci.e.env.scopes[0][normIdent(ci.str(0, "name").Text)]
		return boolean(ok)
	},
	"function-exists": func(ci *callInfo) Value {
		name := normIdent(ci.str(0, "name").Text)
		_, ok := ci.e.env.funcs[name]
		if !ok {
			_, ok = globalFns[normIdent(strings.ToLower(name))]
		}
		return boolean(ok)
	},
	"mixin-exists": func(ci *callInfo) Value {
		_, ok := ci.e.env.mixins[normIdent(ci.str(0, "name").Text)]
		return boolean(ok)
	},
	"content-exists": func(ci *callInfo) Value {
		return boolean(ci.e.env.content != nil)
	},
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
		_ = x
		return "list"
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
		if len(x.Elements) == 0 {
			return "()"
		}
		parts := make([]string, len(x.Elements))
		for i, e := range x.Elements {
			parts[i] = inspect(e)
		}
		sep := " "
		if x.Sep == SepComma {
			sep = ", "
		} else if x.Sep == SepSlash {
			sep = " / "
		}
		body := strings.Join(parts, sep)
		if x.Bracketed {
			return "[" + body + "]"
		}
		return body
	case *Map:
		parts := make([]string, len(x.Keys))
		for i := range x.Keys {
			parts[i] = inspect(x.Keys[i]) + ": " + inspect(x.Values[i])
		}
		return "(" + strings.Join(parts, ", ") + ")"
	case *Null:
		return "null"
	}
	return serializeValue(v, false)
}

func fnKeywords(ci *callInfo) Value {
	l := ci.require(0, "args")
	if m, ok := l.(*Map); ok {
		return m
	}
	return &Map{}
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

func fnUpper(ci *callInfo) Value {
	s := ci.str(0, "string")
	return &SassString{Text: strings.ToUpper(s.Text), Quoted: s.Quoted}
}

func fnLower(ci *callInfo) Value {
	s := ci.str(0, "string")
	return &SassString{Text: strings.ToLower(s.Text), Quoted: s.Quoted}
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

func fnStrSlice(ci *callInfo) Value {
	s := ci.str(0, "string")
	runes := []rune(s.Text)
	start := normalizeStrIndex(int(ci.num(1, "start-at").Val), len(runes))
	end := len(runes)
	if v, ok := ci.get(2, "end-at"); ok {
		end = normalizeStrIndex(int(ci.e.asNumber(v).Val), len(runes)) + 1
	}
	if start < 1 {
		start = 1
	}
	if end > len(runes) {
		end = len(runes)
	}
	if start > end {
		return &SassString{Text: "", Quoted: s.Quoted}
	}
	return &SassString{Text: string(runes[start-1 : end]), Quoted: s.Quoted}
}

func fnStrSplit(ci *callInfo) Value {
	s := ci.str(0, "string")
	sep := ci.str(1, "separator")
	var parts []string
	if sep.Text == "" {
		parts = []string{s.Text}
	} else {
		parts = strings.Split(s.Text, sep.Text)
	}
	elems := make([]Value, len(parts))
	for i, p := range parts {
		elems[i] = &SassString{Text: p, Quoted: true}
	}
	return &List{Elements: elems, Sep: SepComma, Bracketed: true}
}

func normalizeStrIndex(idx, length int) int {
	if idx < 0 {
		return length + idx + 1
	}
	return idx
}

// sort helper for deterministic iteration where needed.
var _ = sort.Strings
