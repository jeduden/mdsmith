package syntax

import "fmt"

// parse_struct.go parses struct and list literals (plan 240). A struct literal
// `{ … }` holds field, embed, ellipsis, and comprehension declarations; a list
// literal `[ … ]` holds element expressions, comprehensions, and an optional
// `...T` open tail.

// parseStructLit parses a `{ … }` struct literal. Its callers (parsePrimary's
// tLBrace case and parseComprehension after its own tLBrace check) only invoke
// it when the current token is `{`, so it consumes that `{` directly.
func (p *parser) parseStructLit() (*StructLit, error) {
	p.take() // the opening `{` the caller guaranteed
	decls, err := p.parseDecls(tRBrace)
	if err != nil {
		return nil, err
	}
	if p.peekKind() != tRBrace {
		return nil, fmt.Errorf("cuelite: expected '}' to close struct")
	}
	p.take()
	return &StructLit{Elts: decls}, nil
}

// parseListLit parses a `[ … ]` list literal: comma-separated elements, each
// an expression, a comprehension, or the `...T` open tail. Its only caller
// (parsePrimary's tLBrack case) invokes it when the current token is `[`, so it
// consumes that `[` directly.
func (p *parser) parseListLit() (*ListLit, error) {
	p.take() // the opening `[` the caller guaranteed
	var elts []Expr
	for p.peekKind() != tRBrack && p.peekKind() != tEOF {
		el, err := p.parseListElem()
		if err != nil {
			return nil, err
		}
		elts = append(elts, el)
		if p.peekKind() == tComma {
			p.take()
			continue
		}
		break
	}
	if p.peekKind() != tRBrack {
		return nil, fmt.Errorf("cuelite: expected ']' to close list")
	}
	p.take()
	return &ListLit{Elts: elts}, nil
}

// parseListElem parses one list element. A `...` is the open tail (carried as
// an Ellipsis expression); an `if`/`for` is a comprehension; anything else is
// an expression. The Ellipsis and Comprehension are Decls but list elements
// are typed Expr in the cuelang tree, so they implement aExpr too via the
// wrappers below.
func (p *parser) parseListElem() (Expr, error) {
	switch p.peekKind() {
	case tEllipsis:
		p.take()
		el := &Ellipsis{}
		if p.startsExpr() {
			t, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			el.Type = t
		}
		return el, nil
	case tIdent:
		if p.peekTok().text == "if" || p.peekTok().text == "for" {
			d, err := p.parseComprehension()
			if err != nil {
				return nil, err
			}
			return d.(Expr), nil
		}
	}
	return p.parseExpr()
}
