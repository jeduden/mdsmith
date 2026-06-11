package syntax

// ast.go is the in-house syntax tree the cuelite frontend produces and the
// in-house evaluators walk. It replaces cuelang.org/go/cue/ast (plan 240):
// the node set is exactly the CUE subset the three consumers — compile.go's
// schema walk, eval.go's scoped evaluator, and evalrow.go's row evaluator —
// reach, with the SAME node names and field names those walks already use, so
// the consumers change only their import, not their structure.
//
// The tree carries no source positions: the evaluators never read a position
// (errors quote source text reconstructed from the node, not a line/column),
// so a position-free tree keeps the node structs small and the parser simple.
//
// The one deliberate divergence from CUE's tree is Interpolation: CUE's
// Interpolation.Elts interleaves RAW scanner-token string fragments (carrying
// the opening quote dialect and the `\(`→`(` collapse) that a consumer must
// re-decode through literal.ParseQuotes. The in-house parser decodes the
// fragments while scanning, so Interpolation.Elts carries already-decoded
// string fragments (as *BasicLit with the decoded text in Value and a
// dedicated kInterpFrag marker) interleaved with the embedded expressions.
// evalRowInterpolation reads the decoded fragments directly.

// Node is any syntax-tree node. It exists so Walk can descend a heterogeneous
// tree and so freeRefs can range over children uniformly.
type Node interface{ aNode() }

// Expr is any expression node. The evaluators type-switch on the concrete
// Expr to build a value.
type Expr interface {
	Node
	aExpr()
}

// Decl is any declaration node — a struct/file element: a Field, an
// EmbedDecl, an Ellipsis, or a Comprehension.
type Decl interface {
	Node
	aDecl()
}

// Label is a field or selector label: a bare Ident or a quoted-string
// BasicLit. It names the struct key or the selected member.
type Label interface {
	Node
	aLabel()
}

// Clause is a comprehension clause: an IfClause or a ForClause.
type Clause interface {
	Node
	aClause()
}

// File is the parsed top-level source: a sequence of declarations. A file
// whose single declaration is an EmbedDecl is an embedded expression (the
// `{...}` / `close({...})` form); otherwise the declarations are fields
// forming an implicit struct.
type File struct {
	Decls []Decl
}

// Ident is a bare identifier: a type keyword (string, int, …), a bool/null
// literal name, or a field reference.
type Ident struct {
	Name string
}

// BasicLit is a scalar literal token. Kind selects the literal class and
// Value carries its decoded payload for a kInterpFrag and its raw source text
// otherwise (compileBasicLit re-decodes a STRING through cueUnquote-equivalent
// logic, parses an INT/FLOAT through strconv).
type BasicLit struct {
	Kind  Token
	Value string
}

// Interpolation is a string interpolation (`"a\(x)b"`). Elts interleaves
// decoded string fragments (*BasicLit with Kind kInterpFrag, even indices) and
// embedded expressions (odd indices); the first and last elements are always
// fragments, so Elts has odd length ≥ 3.
type Interpolation struct {
	Elts []Expr
	// IsBytes reports whether the interpolation used the single-quote (bytes)
	// dialect (`'a\(x)b'`). The row evaluator rejects a bytes interpolation as
	// out-of-subset; CUE's tree carried this in the raw fragment dialect that
	// literal.ParseQuotes decoded, which the in-house decoded-fragment tree
	// replaces with this explicit flag.
	IsBytes bool
}

// UnaryExpr is a prefix-operator expression: a bound (>=0), a numeric sign
// (-1, +1), the disjunction default mark (*x), or boolean negation (!c).
type UnaryExpr struct {
	Op Token
	X  Expr
}

// BinaryExpr is an infix-operator expression: a disjunction (|), a meet (&),
// a comparison (==, !=, >, >=, <, <=, =~, !~), or arithmetic (+, *).
type BinaryExpr struct {
	X  Expr
	Op Token
	Y  Expr
}

// ParenExpr is a parenthesized expression. It is significant: explicit parens
// mark a sub-disjunction boundary the default-mode flatten respects.
type ParenExpr struct {
	X Expr
}

// SelectorExpr is a field selection (`fm.id`, `m."my-key"`). X is the base and
// Sel is the member label (an Ident or a quoted-string BasicLit).
type SelectorExpr struct {
	X   Expr
	Sel Label
}

// IndexExpr is an index expression (`xs[0]`, `fm["k"]`). X is the base and
// Index the index expression.
type IndexExpr struct {
	X     Expr
	Index Expr
}

// CallExpr is a function call (`close(s)`, `strings.Join(xs, ",")`, `len(x)`).
// Fun is the call target and Args the argument list.
type CallExpr struct {
	Fun  Expr
	Args []Expr
}

// StructLit is a brace-delimited struct literal. Elts are its declarations:
// fields, embedded expressions, ellipses, and comprehensions.
type StructLit struct {
	Elts []Decl
}

// ListLit is a bracket-delimited list literal. Elts are its elements:
// expressions, an Ellipsis tail, and comprehensions.
type ListLit struct {
	Elts []Expr
}

// Field is a struct field declaration: a Label, a Value, and a Constraint
// token (OPTION for a `?` optional key, else NoToken).
type Field struct {
	Label      Label
	Value      Expr
	Constraint Token
}

// EmbedDecl is an embedded value in a struct or file (`{>=1 & <=10}`, `{X}`):
// an expression standing where a field would, unified into the enclosing
// struct.
type EmbedDecl struct {
	Expr Expr
}

// Ellipsis is an open-list or open-struct tail (`[...T]`, `{...}`). Type, when
// non-nil, is the element type T of an open list.
type Ellipsis struct {
	Type Expr
}

// Comprehension is a list or struct comprehension: one or more Clauses and a
// struct Value (the body). The subset uses a single clause.
type Comprehension struct {
	Clauses []Clause
	Value   Expr
}

// IfClause is a comprehension `if cond` clause.
type IfClause struct {
	Condition Expr
}

// LetClause is a comprehension `let x = expr` clause. The subset's evaluator
// rejects a multi-clause comprehension as unsupported; the node exists so the
// parser can build a multi-clause comprehension for that rejection to fire,
// rather than erroring at parse time.
type LetClause struct {
	Ident *Ident
	Expr  Expr
}

// ForClause is a comprehension `for x in src` (or `for k, x in src`) clause.
// Key is the optional first variable (the two-variable form), Value the
// element variable, and Source the iterated expression.
type ForClause struct {
	Key    *Ident
	Value  *Ident
	Source Expr
}

func (*File) aNode()          {}
func (*Ident) aNode()         {}
func (*BasicLit) aNode()      {}
func (*Interpolation) aNode() {}
func (*UnaryExpr) aNode()     {}
func (*BinaryExpr) aNode()    {}
func (*ParenExpr) aNode()     {}
func (*SelectorExpr) aNode()  {}
func (*IndexExpr) aNode()     {}
func (*CallExpr) aNode()      {}
func (*StructLit) aNode()     {}
func (*ListLit) aNode()       {}
func (*Field) aNode()         {}
func (*EmbedDecl) aNode()     {}
func (*Ellipsis) aNode()      {}
func (*Comprehension) aNode() {}
func (*IfClause) aNode()      {}
func (*ForClause) aNode()     {}
func (*LetClause) aNode()     {}

func (*Ident) aExpr()         {}
func (*BasicLit) aExpr()      {}
func (*Interpolation) aExpr() {}
func (*UnaryExpr) aExpr()     {}
func (*BinaryExpr) aExpr()    {}
func (*ParenExpr) aExpr()     {}
func (*SelectorExpr) aExpr()  {}
func (*IndexExpr) aExpr()     {}
func (*CallExpr) aExpr()      {}
func (*StructLit) aExpr()     {}
func (*ListLit) aExpr()       {}

// Ellipsis and Comprehension are list elements as well as struct declarations,
// so they are both Decl and Expr — mirroring cuelang's tree, where ListLit.Elts
// is []Expr and carries an ellipsis tail and comprehension elements.
func (*Ellipsis) aExpr()      {}
func (*Comprehension) aExpr() {}

func (*Field) aDecl()         {}
func (*EmbedDecl) aDecl()     {}
func (*Ellipsis) aDecl()      {}
func (*Comprehension) aDecl() {}

func (*Ident) aLabel()    {}
func (*BasicLit) aLabel() {}

func (*IfClause) aClause()  {}
func (*ForClause) aClause() {}
func (*LetClause) aClause() {}

// walkChildren calls fn on each direct child node of n. It is the descent
// freeRefs uses to collect references across an arbitrary expression. A node
// with no children (a leaf Ident or BasicLit) calls fn for none.
func walkChildren(n Node, fn func(Node)) {
	switch node := n.(type) {
	case *File:
		for _, d := range node.Decls {
			fn(d)
		}
	case *StructLit:
		for _, d := range node.Elts {
			fn(d)
		}
	case *ListLit:
		for _, e := range node.Elts {
			fn(e)
		}
	case *Field:
		fn(node.Label)
		fn(node.Value)
	case *EmbedDecl:
		fn(node.Expr)
	case *Ellipsis:
		if node.Type != nil {
			fn(node.Type)
		}
	case *Comprehension:
		for _, c := range node.Clauses {
			fn(c)
		}
		fn(node.Value)
	default:
		walkExprChildren(n, fn)
	}
}

// walkExprChildren calls fn on each direct child of an expression or clause
// node. It covers Interpolation, the operator expressions (UnaryExpr,
// BinaryExpr, ParenExpr), the composite expressions (SelectorExpr, IndexExpr,
// CallExpr), and the three clause types (IfClause, LetClause, ForClause).
// Leaf nodes (Ident, BasicLit) have no children and are silently ignored.
func walkExprChildren(n Node, fn func(Node)) {
	switch node := n.(type) {
	case *Interpolation:
		for _, e := range node.Elts {
			fn(e)
		}
	case *UnaryExpr:
		fn(node.X)
	case *BinaryExpr:
		fn(node.X)
		fn(node.Y)
	case *ParenExpr:
		fn(node.X)
	case *SelectorExpr:
		fn(node.X)
		fn(node.Sel)
	case *IndexExpr:
		fn(node.X)
		fn(node.Index)
	case *CallExpr:
		fn(node.Fun)
		for _, a := range node.Args {
			fn(a)
		}
	case *IfClause:
		fn(node.Condition)
	case *LetClause:
		fn(node.Ident)
		fn(node.Expr)
	case *ForClause:
		if node.Key != nil {
			fn(node.Key)
		}
		fn(node.Value)
		fn(node.Source)
	}
}
