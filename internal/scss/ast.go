// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

// Stmt is a Sass statement (a node in a stylesheet body).
type Stmt interface{ stmt() }

// Expr is a Sass expression.
type Expr interface{ expr() }

// --- Statements ---

// StyleRule is a selector with a body.
type StyleRule struct {
	Selector *Interp
	Body     []Stmt
}

// Declaration is a CSS property declaration, optionally with a nested body
// (e.g. `font: { size: 1px }`) and/or a custom-property raw value.
type Declaration struct {
	Name     *Interp
	Value    Expr // may be nil when only a nested body is present
	Body     []Stmt
	Custom   bool // custom property (--*): value is raw interpolated text
	RawValue *Interp
	NameCol  int // source column of the name (custom properties only), for re-indentation
}

// VarDecl assigns a variable.
type VarDecl struct {
	Name      string
	Namespace string // non-empty for `ns.$var: value` assignments to a module
	Value     Expr
	Default   bool
	Global    bool
}

// MixinDef defines an @mixin.
type MixinDef struct {
	Name   string
	Params *ParamList
	Body   []Stmt
}

// Include invokes an @include.
type Include struct {
	Namespace     string
	Name          string
	Args          *ArgList
	ContentParams *ParamList // `using (...)`
	Content       []Stmt     // block body (nil if none)
}

// FunctionDef defines an @function.
type FunctionDef struct {
	Name   string
	Params *ParamList
	Body   []Stmt
}

// Return is @return.
type Return struct{ Value Expr }

// IfClause is one condition/body pair.
type IfClause struct {
	Cond Expr
	Body []Stmt
}

// If is @if/@else if/@else.
type If struct {
	Clauses []IfClause
	Else    []Stmt // nil when no bare @else
	HasElse bool
}

// Each is @each.
type Each struct {
	Vars []string
	List Expr
	Body []Stmt
}

// For is @for.
type For struct {
	Var     string
	From    Expr
	To      Expr
	Through bool
	Body    []Stmt
}

// While is @while.
type While struct {
	Cond Expr
	Body []Stmt
}

// AtRoot is @at-root.
type AtRoot struct {
	Query *Interp // may be nil
	Body  []Stmt
}

// Media is @media.
type Media struct {
	Query *Interp
	Body  []Stmt
}

// Supports is @supports.
type Supports struct {
	Cond SupportsCond
	Body []Stmt
}

// SupportsCond is a parsed @supports condition tree. It mirrors dart-sass's
// SupportsCondition hierarchy so the condition can be re-serialized canonically
// (comments and redundant whitespace dropped, SassScript in declaration values
// evaluated) rather than passed through as raw text.
type SupportsCond interface{ supportsCond() }

// SupportsOperation is `left and right` or `left or right`. Op is "and"/"or".
type SupportsOperation struct {
	Left, Right SupportsCond
	Op          string
}

// SupportsNegation is `not <cond>`.
type SupportsNegation struct{ Cond SupportsCond }

// SupportsInterp is a lone `#{...}` interpolation used as a whole condition.
type SupportsInterp struct{ Expr Expr }

// SupportsDecl is a declaration condition `(name: value)`. For a custom
// property the raw value interpolation is kept in RawValue and Custom is set.
type SupportsDecl struct {
	Name     Expr
	Value    Expr    // non-custom: an evaluated expression
	RawValue *Interp // custom property: raw interpolated value (leading space kept)
	Custom   bool
}

// SupportsFunc is a function condition `name(args)`, both interpolated raw text.
type SupportsFunc struct {
	Name *Interp
	Args *Interp
}

// SupportsAnything is a parenthesized fallback `(contents)` whose contents are
// preserved as raw interpolated text (loud comments kept, silent comments and
// redundant whitespace dropped).
type SupportsAnything struct{ Contents *Interp }

func (*SupportsOperation) supportsCond() {}
func (*SupportsNegation) supportsCond()  {}
func (*SupportsInterp) supportsCond()    {}
func (*SupportsDecl) supportsCond()      {}
func (*SupportsFunc) supportsCond()      {}
func (*SupportsAnything) supportsCond()  {}

// Extend is @extend.
type Extend struct {
	Selector *Interp
	Optional bool
}

// ContentStmt is @content.
type ContentStmt struct{ Args *ArgList }

// Import is legacy @import (possibly several URLs).
type Import struct{ Imports []ImportItem }

// ImportItem is one entry in @import; Plain means a CSS passthrough import.
type ImportItem struct {
	URL   string
	Plain bool
	// Raw is the exact source text of a quoted import URL, quotes included, so a
	// plain-CSS passthrough import round-trips with its original quote style
	// (dart-sass preserves `'foo.css'` as single-quoted rather than renormalising
	// to double quotes). Empty for the url(...) form and interpolated preludes.
	Raw string
	// URLInterp holds the plain-CSS import prelude (the `url(...)` wrapper or a
	// quoted URL) as an interpolation when it contains `#{...}`, so the
	// interpolation is evaluated at compile time. It is nil for the common
	// non-interpolated case, which keeps URL's verbatim round-trip untouched.
	URLInterp *Interp
	// Mods holds the parsed import modifiers (media/supports queries) following a
	// plain-CSS import URL, or nil when there are none. Its Parts may contain the
	// usual literal strings and *InterpExpr interpolations plus *supportsPart and
	// *mediaPart nodes, which serialize canonically at evaluation time.
	Mods *Interp
}

// supportsPart is an @supports condition embedded in a plain-CSS @import's
// modifier list (as in `@import "a" supports(b: c)`).
type supportsPart struct{ Cond SupportsCond }

// mediaPart is a media-query list embedded in a plain-CSS @import's modifier
// list (as in `@import "a" screen and (min-width: 100px)`). Its text is
// re-serialized through the media-query normalizer at evaluation time.
type mediaPart struct{ Query *Interp }

// Use is @use.
type Use struct {
	URL       string
	Namespace string // "" = derived, "*" = no namespace (as *)
	NoNS      bool
	Config    []ConfigVar
}

// Forward is @forward.
type Forward struct {
	URL     string
	Prefix  string
	Show    []string
	Hide    []string
	HasShow bool
	HasHide bool
	Config  []ConfigVar
}

// ConfigVar is a `with (...)` entry.
type ConfigVar struct {
	Name    string
	Value   Expr
	Default bool
}

// Warn/Debug/Error at-rules.
type Warn struct{ Value Expr }
type Debug struct{ Value Expr }
type ErrorStmt struct{ Value Expr }

// LoudComment is a preserved /* */ comment. Col is the 0-based source column of
// the opening `/*`, used at serialization time to re-indent multi-line comments
// exactly as dart-sass does (min of the comment body indentation and Col).
type LoudComment struct {
	Text *Interp
	Col  int
}

// AtRule is a generic unknown at-rule (@font-face, @keyframes, @page, ...).
type AtRule struct {
	Name string
	// NameInterp, when non-nil, is an interpolated at-rule name (`@#{expr}…`).
	// dart-sass treats any interpolated at-rule as unknown/generic (its special
	// parse-time behaviour, if any, is not triggered), except @keyframes whose
	// behaviour is applied at eval time once the name resolves. The resolved
	// string overrides Name during evaluation.
	NameInterp *Interp
	Value      *Interp // may be nil
	Body       []Stmt  // nil = no block
	NoBody     bool
}

func (*StyleRule) stmt()   {}
func (*Declaration) stmt() {}
func (*VarDecl) stmt()     {}
func (*MixinDef) stmt()    {}
func (*Include) stmt()     {}
func (*FunctionDef) stmt() {}
func (*Return) stmt()      {}
func (*If) stmt()          {}
func (*Each) stmt()        {}
func (*For) stmt()         {}
func (*While) stmt()       {}
func (*AtRoot) stmt()      {}
func (*Media) stmt()       {}
func (*Supports) stmt()    {}
func (*Extend) stmt()      {}
func (*ContentStmt) stmt() {}
func (*Import) stmt()      {}
func (*Use) stmt()         {}
func (*Forward) stmt()     {}
func (*Warn) stmt()        {}
func (*Debug) stmt()       {}
func (*ErrorStmt) stmt()   {}
func (*LoudComment) stmt() {}
func (*AtRule) stmt()      {}

// --- Parameters and arguments ---

// Param is a single mixin/function parameter.
type Param struct {
	Name    string
	Default Expr
	Rest    bool
}

// ParamList is a declared parameter list.
type ParamList struct{ Params []Param }

// Arg is a single call argument.
type Arg struct {
	Name   string // "" for positional
	Value  Expr
	Spread bool
}

// ArgList is a call argument list.
type ArgList struct{ Args []Arg }

// --- Expressions ---

// NumberLit is a numeric literal with optional unit.
type NumberLit struct {
	Val  float64
	Unit string
}

// StringLit is a quoted or unquoted string possibly containing interpolation.
type StringLit struct {
	Parts  *Interp
	Quoted bool
}

// ColorLit is a hex or named color literal.
type ColorLit struct{ Color *SassColor }

// BoolLit / NullLit.
type BoolLit struct{ V bool }
type NullLit struct{}

// VarRef references a variable, optionally namespaced.
type VarRef struct {
	Namespace string
	Name      string
}

// Ident is a bare identifier used as an unquoted string value.
type Ident struct{ Name string }

// Parent is the `&` parent selector reference in an expression.
type Parent struct{}

// Binary is a binary operation.
type Binary struct {
	Op    string
	Left  Expr
	Right Expr
	// Slash marks a "/" whose literal operands should serialize as a slash
	// separator rather than being evaluated as division (legacy Sass behaviour).
	Slash bool
}

// Unary is a unary operation (-, +, /, not).
type Unary struct {
	Op   string
	Expr Expr
}

// FuncCall is a function call, optionally namespaced.
//
// When NameInterp is non-nil the callee's name embeds interpolation
// (`#{$f}(a)`, `foo#{1}bar(a)`); dart-sass never resolves such a call to a Sass
// function or built-in — it always passes through as a plain CSS function whose
// name is the evaluated interpolation.
type FuncCall struct {
	Namespace  string
	Name       string
	NameInterp *Interp
	Args       *ArgList
}

// ListExpr is a list literal.
type ListExpr struct {
	Elements  []Expr
	Sep       Separator
	Bracketed bool
}

// MapExpr is a map literal.
type MapExpr struct {
	Keys   []Expr
	Values []Expr
}

// Paren wraps a parenthesized expression.
type Paren struct{ Expr Expr }

// InterpExpr is a #{} interpolation used as an expression.
type InterpExpr struct{ Expr Expr }

func (*NumberLit) expr()  {}
func (*StringLit) expr()  {}
func (*ColorLit) expr()   {}
func (*BoolLit) expr()    {}
func (*NullLit) expr()    {}
func (*VarRef) expr()     {}
func (*Ident) expr()      {}
func (*Parent) expr()     {}
func (*Binary) expr()     {}
func (*Unary) expr()      {}
func (*FuncCall) expr()   {}
func (*ListExpr) expr()   {}
func (*MapExpr) expr()    {}
func (*Paren) expr()      {}
func (*InterpExpr) expr() {}

// Interp is interpolated text: a sequence of literal strings and expressions.
type Interp struct {
	// Parts alternates: string, Expr, string, Expr, ... Any element is either a
	// string (literal text) or an Expr (an interpolated #{} value).
	Parts []any
}

func literalInterp(s string) *Interp { return &Interp{Parts: []any{s}} }

// isPlain reports whether the interpolation has no dynamic parts.
func (i *Interp) isPlain() (string, bool) {
	var sb string
	for _, p := range i.Parts {
		s, ok := p.(string)
		if !ok {
			return "", false
		}
		sb += s
	}
	return sb, true
}
