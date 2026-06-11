package syntax

import "fmt"

// parse_expr.go is the expression grammar of the in-house parser (plan 240):
// precedence-climbing binary operators over a unary/primary core, with
// selectors, indexing, and calls as postfix operators. The precedence matches
// CUE (token.Precedence): OR=1, AND=2, comparison=5, ADD/SUB=6, MUL=7, all
// left-associative.

// startsExpr reports whether the next token can begin an expression, used to
// decide whether a `...` ellipsis carries an element type.
func (p *parser) startsExpr() bool {
	switch p.peekKind() {
	case tIdent, tInt, tFloat, tString, tInterpStart,
		tLParen, tLBrace, tLBrack:
		return true
	case tOp:
		// A prefix operator (`*`, `-`, `+`, `!`, or a bound `>=`) starts an
		// expression.
		switch p.peekTok().op {
		case MUL, SUB, ADD, NOT, GEQ, LEQ, GTR, LSS, NEQ, MAT, NMAT:
			return true
		}
	}
	return false
}

// parseExpr parses a full expression at the lowest binary precedence.
func (p *parser) parseExpr() (Expr, error) {
	return p.parseBinary(1)
}

// parseBinary parses a binary expression at minimum precedence minPrec using
// precedence climbing. Each operator is left-associative.
func (p *parser) parseBinary(minPrec int) (Expr, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for {
		t := p.peekTok()
		if t.kind != tOp {
			return left, nil
		}
		prec := binaryPrec(t.op)
		if prec < minPrec {
			return left, nil
		}
		p.take() // consume the operator
		right, err := p.parseBinary(prec + 1)
		if err != nil {
			return nil, err
		}
		left = &BinaryExpr{X: left, Op: t.op, Y: right}
	}
}

// binaryPrec returns the precedence of a binary operator, or 0 for a token
// that is not a binary operator (a prefix-only `!`, or a bound that only
// appears prefixed). CUE's bound operators (>=, =~, …) are NOT binary here:
// they appear only as the prefix of a constraint (`>=0`), so they return 0 and
// parseBinary stops, leaving them to parseUnary.
func binaryPrec(op Token) int {
	switch op {
	case OR:
		return 1
	case AND:
		return 2
	case EQL, NEQ, LSS, LEQ, GTR, GEQ, MAT, NMAT:
		return 5
	case ADD, SUB:
		return 6
	case MUL:
		return 7
	}
	return 0
}

// parseUnary parses a prefix-operator expression: a `*` default mark, a `-`/`+`
// numeric sign, a `!` negation, or a bound operator (`>=0`, `=~"p"`). A bound
// or default applies to a unary operand. Without a prefix operator it falls
// through to the postfix/primary parser.
func (p *parser) parseUnary() (Expr, error) {
	t := p.peekTok()
	if t.kind == tOp {
		switch t.op {
		case MUL, SUB, ADD, NOT, GEQ, LEQ, GTR, LSS, NEQ, MAT, NMAT:
			p.take() // consume the prefix operator
			operand, err := p.parseUnary()
			if err != nil {
				return nil, err
			}
			return &UnaryExpr{Op: t.op, X: operand}, nil
		}
	}
	return p.parsePostfix()
}

// parsePostfix parses a primary expression followed by any run of postfix
// operators: a `.member` selector, a `[index]` subscript, or a `(args)` call.
func (p *parser) parsePostfix() (Expr, error) {
	e, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}
	for {
		switch p.peekKind() {
		case tDot:
			p.take()
			sel, err := p.parseSelectorLabel()
			if err != nil {
				return nil, err
			}
			e = &SelectorExpr{X: e, Sel: sel}
		case tLBrack:
			p.take()
			idx, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			if p.peekKind() != tRBrack {
				return nil, fmt.Errorf("cuelite: expected ']' after index")
			}
			p.take()
			e = &IndexExpr{X: e, Index: idx}
		case tLParen:
			p.take()
			args, err := p.parseArgs()
			if err != nil {
				return nil, err
			}
			e = &CallExpr{Fun: e, Args: args}
		default:
			return e, nil
		}
	}
}

// parseSelectorLabel parses the member label after a `.`: a bare identifier or
// a quoted string (`fm."my-key"`).
func (p *parser) parseSelectorLabel() (Label, error) {
	t := p.peekTok()
	switch t.kind {
	case tIdent:
		p.take()
		return &Ident{Name: t.text}, nil
	case tString:
		p.take()
		return &BasicLit{Kind: STRING, Value: t.text}, nil
	default:
		return nil, fmt.Errorf("cuelite: expected selector member after '.'")
	}
}

// parseArgs parses a call's comma-separated argument list up to the closing
// `)`. The opening `(` has already been consumed.
func (p *parser) parseArgs() ([]Expr, error) {
	var args []Expr
	for p.peekKind() != tRParen {
		a, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		args = append(args, a)
		if p.peekKind() == tComma {
			p.take()
			continue
		}
		break
	}
	if p.peekKind() != tRParen {
		return nil, fmt.Errorf("cuelite: expected ')' to close call")
	}
	p.take()
	return args, nil
}

// parsePrimary parses an atom: a literal, an identifier, a parenthesized
// expression, a struct literal, a list literal, or an interpolation.
func (p *parser) parsePrimary() (Expr, error) {
	t := p.peekTok()
	switch t.kind {
	case tIdent:
		p.take()
		return identOrKeyword(t.text), nil
	case tInt:
		p.take()
		return &BasicLit{Kind: INT, Value: t.text}, nil
	case tFloat:
		p.take()
		return &BasicLit{Kind: FLOAT, Value: t.text}, nil
	case tString:
		p.take()
		return &BasicLit{Kind: STRING, Value: t.text}, nil
	case tInterpStart:
		return p.parseInterpolation()
	case tLParen:
		p.take()
		inner, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		if p.peekKind() != tRParen {
			return nil, fmt.Errorf("cuelite: expected ')'")
		}
		p.take()
		return &ParenExpr{X: inner}, nil
	case tLBrace:
		return p.parseStructLit()
	case tLBrack:
		return p.parseListLit()
	default:
		return nil, fmt.Errorf("cuelite: unexpected token in expression")
	}
}

// identOrKeyword maps an identifier to its literal node when it names a
// bool/null keyword (the parser emits those as BasicLit so compileBasicLit
// builds them and evalRowIdent never sees them), else an Ident.
func identOrKeyword(name string) Expr {
	switch name {
	case "true":
		return &BasicLit{Kind: TRUE, Value: "true"}
	case "false":
		return &BasicLit{Kind: FALSE, Value: "false"}
	case "null":
		return &BasicLit{Kind: NULL, Value: "null"}
	}
	return &Ident{Name: name}
}

// parseInterpolation parses a string interpolation: the scanner has produced a
// tInterpStart (the decoded first fragment); the parser then alternates
// embedded expressions (parsed from the resumed token stream) and resumed
// fragments until the closing tString fragment. The Elts interleave decoded
// fragment BasicLits (Kind kInterpFrag) and the embedded expressions.
func (p *parser) parseInterpolation() (Expr, error) {
	first := p.peekTok()
	// take() returns the tInterpStart fragment AND advances the scanner, so cur
	// now holds the first embedded-expression token — no extra advance.
	p.take()
	isBytes := first.bytes
	elts := []Expr{&BasicLit{Kind: kInterpFrag, Value: first.text}}
	for {
		expr, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		elts = append(elts, expr)
		// The expression parser stopped at the `)` that closes this interpolation.
		if p.peekKind() != tRParen {
			return nil, fmt.Errorf("cuelite: expected ')' to close interpolation")
		}
		// Resume the string fragment after the `)`. resumeInterp returns a
		// tInterpStart (another fragment follows), a tString (the final fragment),
		// or a tEOF — and a tEOF always carries a recorded scanner error (scanString
		// body calls s.fail before returning it), so a non-fragment kind here is
		// exactly that error.
		p.resume()
		frag := p.cur
		if frag.kind != tInterpStart && frag.kind != tString {
			return nil, p.sc.err
		}
		elts = append(elts, &BasicLit{Kind: kInterpFrag, Value: frag.text})
		if frag.kind == tString {
			// Final fragment: advance past it and finish.
			p.advance()
			return &Interpolation{Elts: elts, IsBytes: isBytes}, nil
		}
		// Another embedded expression follows.
		p.advance()
	}
}
