// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import "strings"

// This file ports Dart Sass's selector AST (lib/src/ast/selector/*) to Go: the
// simple/compound/complex selector model plus specificity and serialization.
// The algorithms in sel_functions.go and sel_extend.go operate over these types
// and mirror dart-sass 1.102.0 to produce byte-identical selector output.

// combinator is the relationship between two compound selectors.
type combinator int

const (
	combChild            combinator = iota // ">"
	combNextSibling                        // "+"
	combFollowingSibling                   // "~"
)

func (c combinator) String() string {
	switch c {
	case combChild:
		return ">"
	case combNextSibling:
		return "+"
	default:
		return "~"
	}
}

// qname is a qualified name: an identifier with an optional namespace. A nil
// namespace means the default namespace; "" means no namespace; "*" means any.
type qname struct {
	name string
	ns   *string // nil = default namespace
}

func (q qname) String() string {
	if q.ns == nil {
		return q.name
	}
	return *q.ns + "|" + q.name
}

func (q qname) equal(o qname) bool {
	if (q.ns == nil) != (o.ns == nil) {
		return false
	}
	if q.ns != nil && *q.ns != *o.ns {
		return false
	}
	return q.name == o.name
}

// simpleSel is a simple selector: type, universal, id, class, attribute,
// pseudo, placeholder or parent.
type simpleSel interface {
	// write appends this selector's serialization to sb.
	write(sb *strings.Builder, compressed bool)
	// specificity returns the base-1000 specificity contribution.
	specificity() int
	// hasComplicated reports whether super-/sub-selector reasoning is non-local
	// (pseudo-elements and selector-argument pseudo-classes).
	hasComplicated() bool
	// isSuper reports whether this is a superselector of other.
	isSuper(other simpleSel) bool
	// unify returns the components of a compound selector matching both this and
	// compound, or (nil,false) if unification is impossible.
	unify(compound []simpleSel) ([]simpleSel, bool)
	// addSuffix returns this selector with suffix appended, or an error.
	addSuffix(suffix string) (simpleSel, error)
	// equal reports structural equality.
	equal(other simpleSel) bool
}

// --- universal ---

type universalSel struct{ ns *string }

func (u *universalSel) specificity() int     { return 0 }
func (u *universalSel) hasComplicated() bool { return false }

func (u *universalSel) write(sb *strings.Builder, _ bool) {
	if u.ns != nil {
		sb.WriteString(*u.ns)
		sb.WriteByte('|')
	}
	sb.WriteByte('*')
}

func (u *universalSel) equal(o simpleSel) bool {
	x, ok := o.(*universalSel)
	if !ok {
		return false
	}
	return nsEqual(u.ns, x.ns)
}

func (u *universalSel) addSuffix(string) (simpleSel, error) {
	return nil, selErr("Selector \"" + selSimpleString(u) + "\" can't have a suffix")
}

func (u *universalSel) isSuper(other simpleSel) bool {
	if u.ns != nil && *u.ns == "*" {
		return true
	}
	switch o := other.(type) {
	case *typeSel:
		return nsEqual(u.ns, o.name.ns)
	case *universalSel:
		return nsEqual(u.ns, o.ns)
	}
	return u.ns == nil || defaultIsSuper(u, other)
}

func (u *universalSel) unify(compound []simpleSel) ([]simpleSel, bool) {
	if len(compound) == 0 {
		return []simpleSel{u}, true
	}
	switch first := compound[0].(type) {
	case *universalSel, *typeSel:
		unified, ok := unifyUniversalAndElement(u, compound[0])
		if !ok {
			return nil, false
		}
		return append([]simpleSel{unified}, compound[1:]...), true
	case *pseudoSel:
		if len(compound) == 1 && (first.isHost() || first.isHostContext()) {
			return nil, false
		}
	}
	if u.ns == nil || *u.ns == "*" {
		return compound, true
	}
	return append([]simpleSel{u}, compound...), true
}

// --- type ---

type typeSel struct{ name qname }

func (t *typeSel) specificity() int     { return 1 }
func (t *typeSel) hasComplicated() bool { return false }

func (t *typeSel) write(sb *strings.Builder, _ bool) { sb.WriteString(t.name.String()) }

func (t *typeSel) equal(o simpleSel) bool {
	x, ok := o.(*typeSel)
	return ok && t.name.equal(x.name)
}

func (t *typeSel) addSuffix(suffix string) (simpleSel, error) {
	return &typeSel{name: qname{name: t.name.name + suffix, ns: t.name.ns}}, nil
}

func (t *typeSel) isSuper(other simpleSel) bool {
	if defaultIsSuper(t, other) {
		return true
	}
	o, ok := other.(*typeSel)
	if !ok {
		return false
	}
	return t.name.name == o.name.name &&
		((t.name.ns != nil && *t.name.ns == "*") || nsEqual(t.name.ns, o.name.ns))
}

func (t *typeSel) unify(compound []simpleSel) ([]simpleSel, bool) {
	if len(compound) > 0 {
		switch compound[0].(type) {
		case *universalSel, *typeSel:
			unified, ok := unifyUniversalAndElement(t, compound[0])
			if !ok {
				return nil, false
			}
			return append([]simpleSel{unified}, compound[1:]...), true
		}
	}
	return append([]simpleSel{t}, compound...), true
}

// --- id ---

type idSel struct{ name string }

func (i *idSel) specificity() int     { return 1000 * 1000 }
func (i *idSel) hasComplicated() bool { return false }

func (i *idSel) write(sb *strings.Builder, _ bool) {
	sb.WriteByte('#')
	sb.WriteString(i.name)
}

func (i *idSel) equal(o simpleSel) bool {
	x, ok := o.(*idSel)
	return ok && i.name == x.name
}

func (i *idSel) addSuffix(suffix string) (simpleSel, error) {
	return &idSel{name: i.name + suffix}, nil
}

func (i *idSel) isSuper(other simpleSel) bool { return defaultIsSuper(i, other) }

func (i *idSel) unify(compound []simpleSel) ([]simpleSel, bool) {
	for _, s := range compound {
		if o, ok := s.(*idSel); ok && o.name != i.name {
			return nil, false
		}
	}
	return defaultUnify(i, compound)
}

// --- class ---

type classSel struct{ name string }

func (c *classSel) specificity() int     { return 1000 }
func (c *classSel) hasComplicated() bool { return false }

func (c *classSel) write(sb *strings.Builder, _ bool) {
	sb.WriteByte('.')
	sb.WriteString(c.name)
}

func (c *classSel) equal(o simpleSel) bool {
	x, ok := o.(*classSel)
	return ok && c.name == x.name
}

func (c *classSel) addSuffix(suffix string) (simpleSel, error) {
	return &classSel{name: c.name + suffix}, nil
}

func (c *classSel) isSuper(other simpleSel) bool { return defaultIsSuper(c, other) }
func (c *classSel) unify(compound []simpleSel) ([]simpleSel, bool) {
	return defaultUnify(c, compound)
}

// --- placeholder ---

type placeholderSel struct{ name string }

func (p *placeholderSel) specificity() int     { return 1000 }
func (p *placeholderSel) hasComplicated() bool { return false }

func (p *placeholderSel) isPrivate() bool {
	return len(p.name) > 0 && (p.name[0] == '-' || p.name[0] == '_')
}

func (p *placeholderSel) write(sb *strings.Builder, _ bool) {
	sb.WriteByte('%')
	sb.WriteString(p.name)
}

func (p *placeholderSel) equal(o simpleSel) bool {
	x, ok := o.(*placeholderSel)
	return ok && p.name == x.name
}

func (p *placeholderSel) addSuffix(suffix string) (simpleSel, error) {
	return &placeholderSel{name: p.name + suffix}, nil
}

func (p *placeholderSel) isSuper(other simpleSel) bool { return defaultIsSuper(p, other) }
func (p *placeholderSel) unify(compound []simpleSel) ([]simpleSel, bool) {
	return defaultUnify(p, compound)
}

// --- parent ---

type parentSel struct{ suffix *string }

func (p *parentSel) specificity() int     { return 1000 }
func (p *parentSel) hasComplicated() bool { return false }

func (p *parentSel) write(sb *strings.Builder, _ bool) {
	sb.WriteByte('&')
	if p.suffix != nil {
		sb.WriteString(*p.suffix)
	}
}

func (p *parentSel) equal(o simpleSel) bool {
	x, ok := o.(*parentSel)
	if !ok {
		return false
	}
	return (p.suffix == nil) == (x.suffix == nil) && (p.suffix == nil || *p.suffix == *x.suffix)
}

func (p *parentSel) addSuffix(string) (simpleSel, error) {
	return nil, selErr("Selector \"&\" can't have a suffix")
}
func (p *parentSel) isSuper(simpleSel) bool { return false }
func (p *parentSel) unify([]simpleSel) ([]simpleSel, bool) {
	panic(selErr("& doesn't support unification."))
}

// --- attribute ---

type attrSel struct {
	name     qname
	op       string // "" means no operator
	value    string
	modifier *string
}

func (a *attrSel) specificity() int     { return 1000 }
func (a *attrSel) hasComplicated() bool { return false }

func (a *attrSel) write(sb *strings.Builder, _ bool) {
	sb.WriteByte('[')
	sb.WriteString(a.name.String())
	if a.op != "" {
		sb.WriteString(a.op)
		if isIdentifierString(a.value) && !strings.HasPrefix(a.value, "--") {
			sb.WriteString(a.value)
			if a.modifier != nil {
				sb.WriteByte(' ')
			}
		} else {
			sb.WriteString(serializeQuoted(a.value))
			if a.modifier != nil {
				sb.WriteByte(' ')
			}
		}
		if a.modifier != nil {
			sb.WriteString(*a.modifier)
		}
	}
	sb.WriteByte(']')
}

func (a *attrSel) equal(o simpleSel) bool {
	x, ok := o.(*attrSel)
	if !ok {
		return false
	}
	if !a.name.equal(x.name) || a.op != x.op || a.value != x.value {
		return false
	}
	return (a.modifier == nil) == (x.modifier == nil) && (a.modifier == nil || *a.modifier == *x.modifier)
}

func (a *attrSel) addSuffix(string) (simpleSel, error) {
	return nil, selErr("Selector \"" + selSimpleString(a) + "\" can't have a suffix")
}
func (a *attrSel) isSuper(other simpleSel) bool { return defaultIsSuper(a, other) }
func (a *attrSel) unify(compound []simpleSel) ([]simpleSel, bool) {
	return defaultUnify(a, compound)
}

// --- pseudo ---

type pseudoSel struct {
	name             string
	normalizedName   string
	isClass          bool
	isSyntacticClass bool
	argument         *string
	selector         *selList
}

func newPseudo(name string, element bool, argument *string, selector *selList) *pseudoSel {
	return &pseudoSel{
		name:             name,
		normalizedName:   unvendor(name),
		isClass:          !element && !isFakePseudoElement(name),
		isSyntacticClass: !element,
		argument:         argument,
		selector:         selector,
	}
}

func (p *pseudoSel) isElement() bool          { return !p.isClass }
func (p *pseudoSel) isSyntacticElement() bool { return !p.isSyntacticClass }
func (p *pseudoSel) isHost() bool             { return p.isClass && p.name == "host" }
func (p *pseudoSel) isHostContext() bool {
	return p.isClass && p.name == "host-context" && p.selector != nil
}

func (p *pseudoSel) hasComplicated() bool { return p.isElement() || p.selector != nil }

func (p *pseudoSel) specificity() int {
	if p.isElement() {
		return 1
	}
	if p.selector == nil {
		return 1000
	}
	switch p.normalizedName {
	case "where":
		return 0
	case "is", "not", "has", "matches":
		return maxComplexSpecificity(p.selector.components)
	case "nth-child", "nth-last-child":
		return 1000 + maxComplexSpecificity(p.selector.components)
	default:
		return 1000
	}
}

func (p *pseudoSel) withSelector(sel *selList) *pseudoSel {
	np := *p
	np.selector = sel
	return &np
}

func (p *pseudoSel) write(sb *strings.Builder, compressed bool) {
	// `:not(<invisible>)` is semantically identical to `*` and is omitted.
	if p.name == "not" && p.selector != nil && p.selector.isInvisible() {
		return
	}
	sb.WriteByte(':')
	if p.isSyntacticElement() {
		sb.WriteByte(':')
	}
	sb.WriteString(p.name)
	if p.argument == nil && p.selector == nil {
		return
	}
	sb.WriteByte('(')
	if p.argument != nil {
		sb.WriteString(*p.argument)
		if p.selector != nil {
			sb.WriteByte(' ')
		}
	}
	if p.selector != nil {
		p.selector.write(sb, compressed)
	}
	sb.WriteByte(')')
}

func (p *pseudoSel) equal(o simpleSel) bool {
	x, ok := o.(*pseudoSel)
	if !ok {
		return false
	}
	if p.name != x.name || p.isClass != x.isClass {
		return false
	}
	if (p.argument == nil) != (x.argument == nil) || (p.argument != nil && *p.argument != *x.argument) {
		return false
	}
	if (p.selector == nil) != (x.selector == nil) {
		return false
	}
	if p.selector != nil && !p.selector.equal(x.selector) {
		return false
	}
	return true
}

func (p *pseudoSel) addSuffix(suffix string) (simpleSel, error) {
	if p.argument != nil || p.selector != nil {
		return nil, selErr("Selector \"" + selSimpleString(p) + "\" can't have a suffix")
	}
	return newPseudo(p.name+suffix, p.isElement(), nil, nil), nil
}

func (p *pseudoSel) unify(compound []simpleSel) ([]simpleSel, bool) {
	if p.name == "host" || p.name == "host-context" {
		for _, s := range compound {
			ps, ok := s.(*pseudoSel)
			if !ok || !(ps.isHost() || ps.selector != nil) {
				return nil, false
			}
		}
	} else if len(compound) == 1 {
		if other, ok := compound[0].(*universalSel); ok {
			return other.unify([]simpleSel{p})
		}
		if other, ok := compound[0].(*pseudoSel); ok && (other.isHost() || other.isHostContext()) {
			return other.unify([]simpleSel{p})
		}
	}

	if containsSimple(compound, p) {
		return compound, true
	}

	var result []simpleSel
	addedThis := false
	for _, s := range compound {
		if ps, ok := s.(*pseudoSel); ok && ps.isElement() {
			if p.isElement() {
				return nil, false
			}
			result = append(result, p)
			addedThis = true
		}
		result = append(result, s)
	}
	if !addedThis {
		result = append(result, p)
	}
	return result, true
}

func (p *pseudoSel) isSuper(other simpleSel) bool {
	if defaultIsSuper(p, other) {
		return true
	}
	if p.selector == nil {
		return p.equal(other)
	}
	if o, ok := other.(*pseudoSel); ok && p.isElement() && o.isElement() &&
		p.normalizedName == "slotted" && o.name == p.name {
		if o.selector == nil {
			return false
		}
		return p.selector.isSuperList(o.selector)
	}
	return compoundIsSuperselector(
		&compoundSel{components: []simpleSel{p}},
		&compoundSel{components: []simpleSel{other}}, nil)
}

// --- compound / complex / list ---

type compoundSel struct{ components []simpleSel }

func (c *compoundSel) specificity() int {
	sum := 0
	for _, s := range c.components {
		sum += s.specificity()
	}
	return sum
}

func (c *compoundSel) hasComplicated() bool {
	for _, s := range c.components {
		if s.hasComplicated() {
			return true
		}
	}
	return false
}

func (c *compoundSel) singleSimple() simpleSel {
	if len(c.components) == 1 {
		return c.components[0]
	}
	return nil
}

func (c *compoundSel) write(sb *strings.Builder, compressed bool) {
	start := sb.Len()
	for _, s := range c.components {
		s.write(sb, compressed)
	}
	if sb.Len() == start {
		sb.WriteByte('*')
	}
}

func (c *compoundSel) equal(o *compoundSel) bool {
	if len(c.components) != len(o.components) {
		return false
	}
	for i := range c.components {
		if !c.components[i].equal(o.components[i]) {
			return false
		}
	}
	return true
}

func (c *compoundSel) isSuper(o *compoundSel) bool {
	return compoundIsSuperselector(c, o, nil)
}

// complexComponent is a compound selector plus its trailing combinators.
type complexComponent struct {
	selector    *compoundSel
	combinators []combinator
}

func (cc *complexComponent) withAdditionalCombinators(combs []combinator) *complexComponent {
	if len(combs) == 0 {
		return cc
	}
	return &complexComponent{
		selector:    cc.selector,
		combinators: append(append([]combinator{}, cc.combinators...), combs...),
	}
}

func (cc *complexComponent) equal(o *complexComponent) bool {
	if !cc.selector.equal(o.selector) {
		return false
	}
	return combSliceEqual(cc.combinators, o.combinators)
}

// selComplex is a complex selector: leading combinators plus components.
type selComplex struct {
	leadingCombinators []combinator
	components         []complexComponent
	lineBreak          bool
}

func (cx *selComplex) specificity() int {
	sum := 0
	for _, c := range cx.components {
		sum += c.selector.specificity()
	}
	return sum
}

func (cx *selComplex) singleCompound() *compoundSel {
	if len(cx.leadingCombinators) != 0 {
		return nil
	}
	if len(cx.components) == 1 && len(cx.components[0].combinators) == 0 {
		return cx.components[0].selector
	}
	return nil
}

func (cx *selComplex) equal(o *selComplex) bool {
	if !combSliceEqual(cx.leadingCombinators, o.leadingCombinators) {
		return false
	}
	if len(cx.components) != len(o.components) {
		return false
	}
	for i := range cx.components {
		if !cx.components[i].equal(&o.components[i]) {
			return false
		}
	}
	return true
}

func (cx *selComplex) isSuper(o *selComplex) bool {
	return len(cx.leadingCombinators) == 0 && len(o.leadingCombinators) == 0 &&
		complexIsSuperselector(cx.components, o.components)
}

// withAdditionalCombinators appends combs to the final component.
func (cx *selComplex) withAdditionalCombinators(combs []combinator, forceLineBreak bool) *selComplex {
	if len(combs) == 0 && !forceLineBreak {
		return cx
	}
	if len(combs) == 0 {
		return &selComplex{cx.leadingCombinators, cx.components, cx.lineBreak || forceLineBreak}
	}
	if len(cx.components) == 0 {
		return &selComplex{
			leadingCombinators: append(append([]combinator{}, cx.leadingCombinators...), combs...),
			lineBreak:          cx.lineBreak || forceLineBreak,
		}
	}
	comps := append([]complexComponent{}, cx.components...)
	last := comps[len(comps)-1]
	comps[len(comps)-1] = *last.withAdditionalCombinators(combs)
	return &selComplex{cx.leadingCombinators, comps, cx.lineBreak || forceLineBreak}
}

func (cx *selComplex) withAdditionalComponent(comp complexComponent, forceLineBreak bool) *selComplex {
	return &selComplex{
		leadingCombinators: cx.leadingCombinators,
		components:         append(append([]complexComponent{}, cx.components...), comp),
		lineBreak:          cx.lineBreak || forceLineBreak,
	}
}

// concatenate appends child's combinators/components to the end.
func (cx *selComplex) concatenate(child *selComplex, forceLineBreak bool) *selComplex {
	lb := cx.lineBreak || child.lineBreak || forceLineBreak
	if len(child.leadingCombinators) == 0 {
		return &selComplex{
			leadingCombinators: cx.leadingCombinators,
			components:         append(append([]complexComponent{}, cx.components...), child.components...),
			lineBreak:          lb,
		}
	}
	if len(cx.components) > 0 {
		comps := append([]complexComponent{}, cx.components...)
		last := comps[len(comps)-1]
		comps[len(comps)-1] = *last.withAdditionalCombinators(child.leadingCombinators)
		comps = append(comps, child.components...)
		return &selComplex{cx.leadingCombinators, comps, lb}
	}
	return &selComplex{
		leadingCombinators: append(append([]combinator{}, cx.leadingCombinators...), child.leadingCombinators...),
		components:         child.components,
		lineBreak:          lb,
	}
}

func (cx *selComplex) write(sb *strings.Builder, compressed bool) {
	writeCombinators(sb, cx.leadingCombinators, compressed)
	if len(cx.leadingCombinators) > 0 && len(cx.components) > 0 && !compressed {
		sb.WriteByte(' ')
	}
	for i := range cx.components {
		comp := &cx.components[i]
		comp.selector.write(sb, compressed)
		if len(comp.combinators) > 0 && !compressed {
			sb.WriteByte(' ')
		}
		writeCombinators(sb, comp.combinators, compressed)
		if i != len(cx.components)-1 && (!compressed || len(comp.combinators) == 0) {
			sb.WriteByte(' ')
		}
	}
}

func writeCombinators(sb *strings.Builder, combs []combinator, compressed bool) {
	for i, c := range combs {
		if i > 0 && !compressed {
			sb.WriteByte(' ')
		}
		sb.WriteString(c.String())
	}
}

// selList is a comma-separated list of complex selectors.
type selList struct{ components []*selComplex }

func (sl *selList) equal(o *selList) bool {
	if sl == nil || o == nil {
		return sl == nil && o == nil
	}
	if len(sl.components) != len(o.components) {
		return false
	}
	for i := range sl.components {
		if !sl.components[i].equal(o.components[i]) {
			return false
		}
	}
	return true
}

func (sl *selList) isSuperList(o *selList) bool {
	return listIsSuperselector(sl.components, o.components)
}

// write serializes the list, filtering invisible complexes.
func (sl *selList) write(sb *strings.Builder, compressed bool) {
	first := true
	for _, cx := range sl.components {
		if cx.isInvisible() {
			continue
		}
		if first {
			first = false
		} else {
			sb.WriteByte(',')
			if cx.lineBreak {
				sb.WriteByte('\n')
			} else if !compressed {
				sb.WriteByte(' ')
			}
		}
		cx.write(sb, compressed)
	}
}

func (sl *selList) String() string {
	var sb strings.Builder
	sl.write(&sb, false)
	return sb.String()
}

func (sl *selList) withAdditionalCombinators(combs []combinator) *selList {
	if len(combs) == 0 {
		return sl
	}
	out := make([]*selComplex, len(sl.components))
	for i, cx := range sl.components {
		out[i] = cx.withAdditionalCombinators(combs, false)
	}
	return &selList{components: out}
}

// --- helpers ---

func nsEqual(a, b *string) bool {
	if (a == nil) != (b == nil) {
		return false
	}
	return a == nil || *a == *b
}

func combSliceEqual(a, b []combinator) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func maxComplexSpecificity(cs []*selComplex) int {
	m := 0
	for i, c := range cs {
		s := c.specificity()
		if i == 0 || s > m {
			m = s
		}
	}
	return m
}

func selSimpleString(s simpleSel) string {
	var sb strings.Builder
	s.write(&sb, false)
	return sb.String()
}

func containsSimple(compound []simpleSel, s simpleSel) bool {
	for _, x := range compound {
		if x.equal(s) {
			return true
		}
	}
	return false
}

// defaultUnify implements SimpleSelector.unify's default behavior.
func defaultUnify(this simpleSel, compound []simpleSel) ([]simpleSel, bool) {
	if len(compound) == 1 {
		if other, ok := compound[0].(*universalSel); ok {
			return other.unify([]simpleSel{this})
		}
		if other, ok := compound[0].(*pseudoSel); ok && (other.isHost() || other.isHostContext()) {
			return other.unify([]simpleSel{this})
		}
	}
	if containsSimple(compound, this) {
		return compound, true
	}
	var result []simpleSel
	addedThis := false
	for _, s := range compound {
		if _, ok := s.(*pseudoSel); ok && !addedThis {
			result = append(result, this)
			addedThis = true
		}
		result = append(result, s)
	}
	if !addedThis {
		result = append(result, this)
	}
	return result, true
}

// defaultIsSuper implements SimpleSelector.isSuperselector's default behavior.
func defaultIsSuper(this, other simpleSel) bool {
	if this.equal(other) {
		return true
	}
	o, ok := other.(*pseudoSel)
	if !ok || !o.isClass {
		return false
	}
	list := o.selector
	if list == nil || !subselectorPseudos[o.normalizedName] {
		return false
	}
	for _, complex := range list.components {
		last := complex.components[len(complex.components)-1].selector
		matched := false
		for _, simple := range last.components {
			if this.isSuper(simple) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

var subselectorPseudos = map[string]bool{
	"is": true, "matches": true, "where": true, "any": true,
	"nth-child": true, "nth-last-child": true,
}

func isFakePseudoElement(name string) bool {
	if name == "" {
		return false
	}
	switch name[0] {
	case 'a', 'A':
		return strings.EqualFold(name, "after")
	case 'b', 'B':
		return strings.EqualFold(name, "before")
	case 'f', 'F':
		return strings.EqualFold(name, "first-line") || strings.EqualFold(name, "first-letter")
	}
	return false
}

func unvendor(name string) string {
	if len(name) < 2 || name[0] != '-' || name[1] == '-' {
		return name
	}
	for i := 2; i < len(name); i++ {
		if name[i] == '-' {
			return name[i+1:]
		}
	}
	return name
}

func strptr(s string) *string { return &s }
