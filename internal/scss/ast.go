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
	Condition *Interp
	Body      []Stmt
}

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
	URL     string
	Plain   bool
	RawText string // for plain imports, the full media/text after url
}

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

// LoudComment is a preserved /* */ comment.
type LoudComment struct{ Text *Interp }

// AtRule is a generic unknown at-rule (@font-face, @keyframes, @page, ...).
type AtRule struct {
	Name   string
	Value  *Interp // may be nil
	Body   []Stmt  // nil = no block
	NoBody bool
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
type FuncCall struct {
	Namespace string
	Name      string
	Args      *ArgList
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
