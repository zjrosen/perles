package bql

import "fmt"

// Parser parses BQL tokens into an AST.
type Parser struct {
	lexer   *Lexer
	current Token
	peek    Token
}

// NewParser creates a parser for the input.
func NewParser(input string) *Parser {
	p := &Parser{lexer: NewLexer(input)}
	// Prime the parser with two tokens
	p.nextToken()
	p.nextToken()
	return p
}

// Parse parses the input and returns the Query AST.
func (p *Parser) Parse() (*Query, error) {
	query := &Query{}

	// Parse filter expression (optional - might just be EXPAND or ORDER BY)
	if p.current.Type != TokenExpand && p.current.Type != TokenOrder && p.current.Type != TokenEOF {
		expr, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		query.Filter = expr
	}

	// Parse EXPAND clause (optional)
	if p.current.Type == TokenExpand {
		expand, err := p.parseExpand()
		if err != nil {
			return nil, err
		}
		query.Expand = expand
	}

	// Parse ORDER BY clause (optional)
	if p.current.Type == TokenOrder {
		orderBy, err := p.parseOrderBy()
		if err != nil {
			return nil, err
		}
		query.OrderBy = orderBy
	}

	// Should be at EOF now
	if p.current.Type != TokenEOF {
		return nil, fmt.Errorf("unexpected token %q at position %d", p.current.Literal, p.current.Pos)
	}

	return query, nil
}

// nextToken advances to the next token.
func (p *Parser) nextToken() {
	p.current = p.peek
	p.peek = p.lexer.NextToken()
}

// parseExpression parses OR-separated terms.
// expression = term { "or" term }
func (p *Parser) parseExpression() (Expr, error) {
	left, err := p.parseTerm()
	if err != nil {
		return nil, err
	}

	for p.current.Type == TokenOr {
		p.nextToken() // consume OR
		right, err := p.parseTerm()
		if err != nil {
			return nil, err
		}
		left = &BinaryExpr{Left: left, Op: TokenOr, Right: right}
	}

	return left, nil
}

// parseTerm parses AND-separated factors.
// term = factor { "and" factor }
func (p *Parser) parseTerm() (Expr, error) {
	left, err := p.parseFactor()
	if err != nil {
		return nil, err
	}

	for p.current.Type == TokenAnd {
		p.nextToken() // consume AND
		right, err := p.parseFactor()
		if err != nil {
			return nil, err
		}
		left = &BinaryExpr{Left: left, Op: TokenAnd, Right: right}
	}

	return left, nil
}

// parseFactor parses NOT, parenthesized expressions, or comparisons.
// factor = "not" factor | "(" expression ")" | comparison
func (p *Parser) parseFactor() (Expr, error) {
	switch p.current.Type {
	case TokenNot:
		p.nextToken() // consume NOT
		expr, err := p.parseFactor()
		if err != nil {
			return nil, err
		}
		return &NotExpr{Expr: expr}, nil

	case TokenLParen:
		p.nextToken() // consume (
		expr, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		if p.current.Type != TokenRParen {
			return nil, fmt.Errorf("expected ')' at position %d, got %q", p.current.Pos, p.current.Literal)
		}
		p.nextToken() // consume )
		return expr, nil

	default:
		return p.parseComparison()
	}
}

// parseComparison parses field comparisons.
// comparison = field op value | field "in" "(" values ")" | field "not" "in" "(" values ")"
func (p *Parser) parseComparison() (Expr, error) {
	// Expect field name
	if p.current.Type != TokenIdent {
		return nil, fmt.Errorf("expected field name at position %d, got %q", p.current.Pos, p.current.Literal)
	}
	field := p.current.Literal
	p.nextToken()

	// Check for "not in"
	if p.current.Type == TokenNot && p.peek.Type == TokenIn {
		p.nextToken() // consume NOT
		p.nextToken() // consume IN
		return p.parseInExpr(field, true)
	}

	// Check for "in"
	if p.current.Type == TokenIn {
		p.nextToken() // consume IN
		return p.parseInExpr(field, false)
	}

	// Must be comparison operator
	if !p.current.Type.IsComparisonOp() {
		return nil, fmt.Errorf("expected operator at position %d, got %q", p.current.Pos, p.current.Literal)
	}
	op := p.current.Type
	p.nextToken()

	// Parse value
	value, err := p.parseValue()
	if err != nil {
		return nil, err
	}

	return &CompareExpr{Field: field, Op: op, Value: value}, nil
}

// parseInExpr parses the IN expression values list.
func (p *Parser) parseInExpr(field string, not bool) (Expr, error) {
	// Expect (
	if p.current.Type != TokenLParen {
		return nil, fmt.Errorf("expected '(' at position %d, got %q", p.current.Pos, p.current.Literal)
	}
	p.nextToken()

	// Parse values
	var values []Value
	for {
		value, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		values = append(values, value)

		if p.current.Type == TokenComma {
			p.nextToken() // consume comma
			continue
		}
		break
	}

	// Expect )
	if p.current.Type != TokenRParen {
		return nil, fmt.Errorf("expected ')' at position %d, got %q", p.current.Pos, p.current.Literal)
	}
	p.nextToken()

	return &InExpr{Field: field, Values: values, Not: not}, nil
}

// parseValue parses a literal value.
func (p *Parser) parseValue() (Value, error) {
	var v Value

	switch p.current.Type {
	case TokenString:
		v = Value{Type: ValueString, Raw: p.current.Literal, String: p.current.Literal}
	case TokenNumber:
		v = parseNumberValue(p.current.Literal)
	case TokenTrue:
		v = Value{Type: ValueBool, Raw: p.current.Literal, Bool: true}
	case TokenFalse:
		v = Value{Type: ValueBool, Raw: p.current.Literal, Bool: false}
	case TokenIdent:
		v = parseIdentValue(p.current.Literal)
	default:
		return v, fmt.Errorf("expected value at position %d, got %q", p.current.Pos, p.current.Literal)
	}

	p.nextToken()
	return v, nil
}

// parseNumberValue parses a number literal (including date/time offsets).
func parseNumberValue(literal string) Value {
	// Check for date/time offset formats (-7d days, -24h hours, -3m months)
	if len(literal) > 1 {
		suffix := literal[len(literal)-1]
		if suffix == 'd' || suffix == 'D' || suffix == 'h' || suffix == 'H' || suffix == 'm' || suffix == 'M' {
			return Value{Type: ValueDate, Raw: literal, String: literal}
		}
	}

	// Parse as integer
	var n int
	_, _ = fmt.Sscanf(literal, "%d", &n)
	return Value{Type: ValueInt, Raw: literal, Int: n}
}

// parseIdentValue parses an identifier as a value.
func parseIdentValue(literal string) Value {
	// Check for priority format (P0-P4)
	if len(literal) == 2 && (literal[0] == 'P' || literal[0] == 'p') {
		if literal[1] >= '0' && literal[1] <= '4' {
			return Value{
				Type:   ValuePriority,
				Raw:    literal,
				String: literal,
				Int:    int(literal[1] - '0'),
			}
		}
	}

	// Check for special date values
	switch literal {
	case "today", "Today", "TODAY":
		return Value{Type: ValueDate, Raw: literal, String: "today"}
	case "yesterday", "Yesterday", "YESTERDAY":
		return Value{Type: ValueDate, Raw: literal, String: "yesterday"}
	}

	// Plain string value
	return Value{Type: ValueString, Raw: literal, String: literal}
}

// parseOrderBy parses the ORDER BY clause.
func (p *Parser) parseOrderBy() ([]OrderTerm, error) {
	// Expect ORDER
	if p.current.Type != TokenOrder {
		return nil, fmt.Errorf("expected 'order' at position %d", p.current.Pos)
	}
	p.nextToken()

	// Expect BY
	if p.current.Type != TokenBy {
		return nil, fmt.Errorf("expected 'by' at position %d, got %q", p.current.Pos, p.current.Literal)
	}
	p.nextToken()

	var terms []OrderTerm
	for {
		// Expect field name
		if p.current.Type != TokenIdent {
			return nil, fmt.Errorf("expected field name at position %d, got %q", p.current.Pos, p.current.Literal)
		}
		term := OrderTerm{Field: p.current.Literal}
		p.nextToken()

		// Optional ASC/DESC
		switch p.current.Type {
		case TokenAsc:
			term.Desc = false
			p.nextToken()
		case TokenDesc:
			term.Desc = true
			p.nextToken()
		}

		terms = append(terms, term)

		// Check for more terms
		if p.current.Type == TokenComma {
			p.nextToken()
			continue
		}
		break
	}

	return terms, nil
}

// parseExpand parses the EXPAND clause: "expand <type> [depth <n>|*]"
func (p *Parser) parseExpand() (*ExpandClause, error) {
	p.nextToken() // consume EXPAND

	if p.current.Type != TokenIdent {
		return nil, fmt.Errorf(
			"expected expansion type at position %d, got %q (valid: children, blockers, blocks, deps, all)",
			p.current.Pos, p.current.Literal)
	}

	expandType, err := parseExpandType(p.current.Literal)
	if err != nil {
		return nil, fmt.Errorf("%w at position %d", err, p.current.Pos)
	}
	p.nextToken()

	// Default depth is 1
	clause := &ExpandClause{
		Type:  expandType,
		Depth: DepthDefault,
	}

	// Parse optional DEPTH clause
	if p.current.Type == TokenDepth {
		p.nextToken() // consume DEPTH

		depth, err := p.parseDepthValue()
		if err != nil {
			return nil, err
		}
		clause.Depth = depth
	}

	return clause, nil
}

// parseDepthValue parses the depth value: a number or "*" for unlimited.
func (p *Parser) parseDepthValue() (ExpandDepth, error) {
	switch p.current.Type {
	case TokenStar:
		p.nextToken()
		return DepthUnlimited, nil

	case TokenNumber:
		var n int
		_, err := fmt.Sscanf(p.current.Literal, "%d", &n)
		if err != nil {
			return 0, fmt.Errorf("invalid depth value %q at position %d",
				p.current.Literal, p.current.Pos)
		}
		if n < 1 {
			return 0, fmt.Errorf("depth must be at least 1, got %d at position %d",
				n, p.current.Pos)
		}
		if n > int(DepthMax) {
			return 0, fmt.Errorf("depth cannot exceed %d, got %d at position %d",
				DepthMax, n, p.current.Pos)
		}
		p.nextToken()
		return ExpandDepth(n), nil

	default:
		return 0, fmt.Errorf(
			"expected depth value (number or *) at position %d, got %q",
			p.current.Pos, p.current.Literal)
	}
}

// parseExpandType converts a string to an ExpandType.
func parseExpandType(s string) (ExpandType, error) {
	switch s {
	case "children", "Children", "CHILDREN":
		return ExpandChildren, nil
	case "blockers", "Blockers", "BLOCKERS":
		return ExpandBlockers, nil
	case "blocks", "Blocks", "BLOCKS":
		return ExpandBlocks, nil
	case "deps", "Deps", "DEPS":
		return ExpandDeps, nil
	case "all", "All", "ALL":
		return ExpandAll, nil
	case "upstream", "Upstream", "UPSTREAM":
		return ExpandUpstream, nil
	case "downstream", "Downstream", "DOWNSTREAM":
		return ExpandDownstream, nil
	default:
		return ExpandNone, fmt.Errorf(
			"unknown expansion type %q (valid: children, blockers, blocks, deps, all, upstream, downstream)", s)
	}
}
