// Package syntax is the in-house CUE-subset frontend for cue/cuelite (plan
// 240). It lexes and parses the exact CUE subset mdsmith uses — schema
// constraint expressions, query/where filters, and catalog row expressions —
// into a small in-house syntax tree (the node set in ast.go), replacing
// cuelang.org/go/cue/parser, cue/ast, cue/token, and cue/literal. It depends
// only on the standard library.
//
// The package is module-internal to cue/cuelite: the public cuelite API
// (Compile, CompileRow, …) is the supported surface, and the syntax tree is an
// implementation detail the cuelite evaluators walk.
package syntax

// ParseFile parses a complete CUE-subset source string into a File, the
// in-house replacement for cuelang.org/go/cue/parser.ParseFile. A syntactic
// error, an unsupported construct the lexer cannot tokenize, or trailing
// tokens after the declarations is returned as an error.
func ParseFile(src string) (*File, error) {
	return parseFile(src)
}

// Unquote decodes a complete (non-interpolation) string-literal's raw quoted
// source — including its delimiters — into its value, the in-house replacement
// for cuelang.org/go/cue/literal.Unquote across the three CUE string dialects
// (plain, raw, multiline). A bytes literal (`'…'`) decodes too; callers reject
// it as out-of-subset via IsBytesLiteral before calling Unquote when bytes are
// disallowed.
func Unquote(raw string) (string, error) {
	return unquoteCUEString(raw)
}

// IsBytesLiteral reports whether a raw string literal uses the single-quote
// (bytes) dialect, so a caller can reject a bytes value as out-of-subset
// before decoding it.
func IsBytesLiteral(raw string) bool {
	return isBytesLiteral(raw)
}

// WalkChildren calls fn on each direct child node of n. It is the in-house
// replacement for cuelang.org/go/cue/ast.Walk's child descent that
// cuelite.freeRefs uses to collect references across an expression.
func WalkChildren(n Node, fn func(Node)) {
	walkChildren(n, fn)
}
