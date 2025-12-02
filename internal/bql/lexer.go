package bql

// Lexer tokenizes BQL input.
type Lexer struct {
	input string
	pos   int  // current position in input
	ch    byte // current character under examination
}

// NewLexer creates a new lexer for the input string.
func NewLexer(input string) *Lexer {
	l := &Lexer{input: input}
	l.readChar()
	return l
}

// NextToken returns the next token from the input.
func (l *Lexer) NextToken() Token {
	l.skipWhitespace()

	tok := Token{Pos: l.pos}

	switch l.ch {
	case '(':
		tok.Type = TokenLParen
		tok.Literal = "("
	case ')':
		tok.Type = TokenRParen
		tok.Literal = ")"
	case ',':
		tok.Type = TokenComma
		tok.Literal = ","
	case '=':
		tok.Type = TokenEq
		tok.Literal = "="
	case '!':
		if l.peekChar() == '=' {
			l.readChar()
			tok.Type = TokenNeq
			tok.Literal = "!="
		} else if l.peekChar() == '~' {
			l.readChar()
			tok.Type = TokenNotContains
			tok.Literal = "!~"
		} else {
			tok.Type = TokenIllegal
			tok.Literal = string(l.ch)
		}
	case '<':
		if l.peekChar() == '=' {
			l.readChar()
			tok.Type = TokenLte
			tok.Literal = "<="
		} else {
			tok.Type = TokenLt
			tok.Literal = "<"
		}
	case '>':
		if l.peekChar() == '=' {
			l.readChar()
			tok.Type = TokenGte
			tok.Literal = ">="
		} else {
			tok.Type = TokenGt
			tok.Literal = ">"
		}
	case '~':
		tok.Type = TokenContains
		tok.Literal = "~"
	case '"', '\'':
		tok.Type = TokenString
		tok.Literal = l.readString(l.ch)
		return tok
	case 0:
		tok.Type = TokenEOF
		tok.Literal = ""
		return tok
	default:
		if isLetter(l.ch) {
			tok.Literal = l.readIdentifier()
			tok.Type = LookupKeyword(tok.Literal)
			return tok
		} else if isDigit(l.ch) || (l.ch == '-' && isDigit(l.peekChar())) {
			tok.Literal = l.readNumber()
			tok.Type = TokenNumber
			return tok
		} else {
			tok.Type = TokenIllegal
			tok.Literal = string(l.ch)
		}
	}

	l.readChar()
	return tok
}

// readChar reads the next character and advances position.
func (l *Lexer) readChar() {
	if l.pos >= len(l.input) {
		l.ch = 0
	} else {
		l.ch = l.input[l.pos]
	}
	l.pos++
}

// peekChar returns the next character without advancing.
func (l *Lexer) peekChar() byte {
	if l.pos >= len(l.input) {
		return 0
	}
	return l.input[l.pos]
}

// skipWhitespace advances past whitespace characters.
func (l *Lexer) skipWhitespace() {
	for l.ch == ' ' || l.ch == '\t' || l.ch == '\n' || l.ch == '\r' {
		l.readChar()
	}
}

// readIdentifier reads an identifier (letters, digits, underscores, hyphens).
func (l *Lexer) readIdentifier() string {
	start := l.pos - 1
	for isLetter(l.ch) || isDigit(l.ch) || l.ch == '_' || l.ch == '-' {
		l.readChar()
	}
	return l.input[start : l.pos-1]
}

// readString reads a quoted string (supports both " and ').
func (l *Lexer) readString(quote byte) string {
	l.readChar() // skip opening quote
	start := l.pos - 1
	for l.ch != quote && l.ch != 0 {
		l.readChar()
	}
	str := l.input[start : l.pos-1]
	if l.ch == quote {
		l.readChar() // skip closing quote
	}
	return str
}

// readNumber reads a number (including negative numbers and date/time offsets like -7d, -24h, -3m).
func (l *Lexer) readNumber() string {
	start := l.pos - 1
	if l.ch == '-' {
		l.readChar()
	}
	for isDigit(l.ch) {
		l.readChar()
	}
	// Support time offset formats: -7d (days), -24h (hours), -3m (months)
	if l.ch == 'd' || l.ch == 'D' || l.ch == 'h' || l.ch == 'H' || l.ch == 'm' || l.ch == 'M' {
		l.readChar()
	}
	return l.input[start : l.pos-1]
}

// isLetter returns true if c is a letter or underscore.
func isLetter(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_'
}

// isDigit returns true if c is a digit.
func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}
