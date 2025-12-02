// Package bql implements the Beads Query Language parser and executor.
package bql

import "strings"

// TokenType represents the type of lexical token.
type TokenType int

const (
	TokenEOF TokenType = iota
	TokenIllegal

	// Literals
	TokenIdent  // field names, unquoted values
	TokenString // "quoted" or 'quoted'
	TokenNumber // integers

	// Delimiters
	TokenLParen // (
	TokenRParen // )
	TokenComma  // ,

	// Comparison operators
	TokenEq          // =
	TokenNeq         // !=
	TokenLt          // <
	TokenGt          // >
	TokenLte         // <=
	TokenGte         // >=
	TokenContains    // ~
	TokenNotContains // !~

	// Logical operators (keywords)
	TokenAnd // and
	TokenOr  // or
	TokenNot // not

	// Set operators
	TokenIn // in

	// Order clause
	TokenOrder // order
	TokenBy    // by
	TokenAsc   // asc
	TokenDesc  // desc

	// Boolean literals
	TokenTrue  // true
	TokenFalse // false
)

// String returns the string representation of the token type.
func (t TokenType) String() string {
	switch t {
	case TokenEOF:
		return "EOF"
	case TokenIllegal:
		return "ILLEGAL"
	case TokenIdent:
		return "IDENT"
	case TokenString:
		return "STRING"
	case TokenNumber:
		return "NUMBER"
	case TokenLParen:
		return "("
	case TokenRParen:
		return ")"
	case TokenComma:
		return ","
	case TokenEq:
		return "="
	case TokenNeq:
		return "!="
	case TokenLt:
		return "<"
	case TokenGt:
		return ">"
	case TokenLte:
		return "<="
	case TokenGte:
		return ">="
	case TokenContains:
		return "~"
	case TokenNotContains:
		return "!~"
	case TokenAnd:
		return "AND"
	case TokenOr:
		return "OR"
	case TokenNot:
		return "NOT"
	case TokenIn:
		return "IN"
	case TokenOrder:
		return "ORDER"
	case TokenBy:
		return "BY"
	case TokenAsc:
		return "ASC"
	case TokenDesc:
		return "DESC"
	case TokenTrue:
		return "TRUE"
	case TokenFalse:
		return "FALSE"
	default:
		return "UNKNOWN"
	}
}

// Token represents a lexical token.
type Token struct {
	Type    TokenType
	Literal string
	Pos     int // Position in input for error reporting
}

// keywords maps keyword strings to their token types.
var keywords = map[string]TokenType{
	"and":   TokenAnd,
	"or":    TokenOr,
	"not":   TokenNot,
	"in":    TokenIn,
	"order": TokenOrder,
	"by":    TokenBy,
	"asc":   TokenAsc,
	"desc":  TokenDesc,
	"true":  TokenTrue,
	"false": TokenFalse,
}

// LookupKeyword returns the token type for the given identifier.
// If the identifier is a keyword, returns the keyword token type.
// Otherwise, returns TokenIdent.
func LookupKeyword(ident string) TokenType {
	if tok, ok := keywords[strings.ToLower(ident)]; ok {
		return tok
	}
	return TokenIdent
}

// IsComparisonOp returns true if the token type is a comparison operator.
func (t TokenType) IsComparisonOp() bool {
	switch t {
	case TokenEq, TokenNeq, TokenLt, TokenGt, TokenLte, TokenGte, TokenContains, TokenNotContains:
		return true
	}
	return false
}

// IsLogicalOp returns true if the token type is a logical operator.
func (t TokenType) IsLogicalOp() bool {
	return t == TokenAnd || t == TokenOr
}
