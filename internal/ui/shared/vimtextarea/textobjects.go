package vimtextarea

// TextObjectFinder locates text object bounds around the cursor position.
// Text objects are semantic units of text (words, quoted strings, bracketed expressions)
// that can be operated on as a whole regardless of cursor position within them.
type TextObjectFinder interface {
	// FindBounds returns the start and end positions of the text object.
	// inner=true excludes delimiters/whitespace, inner=false includes them.
	// Returns found=false if no valid text object exists at cursor position.
	FindBounds(m *Model, inner bool) (start, end Position, found bool)
}

// textObjectRegistry maps object characters to their finders.
// For example, 'w' maps to WordTextObject for word-based text objects.
var textObjectRegistry = map[rune]TextObjectFinder{
	'w':  &WordTextObject{bigWord: false},
	'W':  &WordTextObject{bigWord: true},
	'"':  &PairedDelimiterTextObject{openChar: '"', closeChar: '"'},
	'\'': &PairedDelimiterTextObject{openChar: '\'', closeChar: '\''},
	// Bracket objects - both open and close characters map to the same finder
	'(': &PairedDelimiterTextObject{openChar: '(', closeChar: ')'},
	')': &PairedDelimiterTextObject{openChar: '(', closeChar: ')'},
	'[': &PairedDelimiterTextObject{openChar: '[', closeChar: ']'},
	']': &PairedDelimiterTextObject{openChar: '[', closeChar: ']'},
	'{': &PairedDelimiterTextObject{openChar: '{', closeChar: '}'},
	'}': &PairedDelimiterTextObject{openChar: '{', closeChar: '}'},
	// 'b' for any bracket type - finds innermost of (), [], or {}
	'b': &BracketTextObject{},
}

// WordTextObject handles 'w' (word) and 'W' (WORD) text objects.
// A word is a sequence of alphanumeric characters and underscores, or a sequence of
// punctuation characters. A WORD is any sequence of non-whitespace characters.
type WordTextObject struct {
	bigWord bool // true for 'W' (WORD), false for 'w' (word)
}

// FindBounds locates the word boundaries around the cursor.
// For inner=true (iw): returns just the word without surrounding whitespace.
// For inner=false (aw): includes trailing whitespace, or leading whitespace if at line end.
// Note: All column positions are grapheme indices, not byte offsets.
func (w *WordTextObject) FindBounds(m *Model, inner bool) (start, end Position, found bool) {
	if m.cursorRow < 0 || m.cursorRow >= len(m.content) {
		return Position{}, Position{}, false
	}

	line := m.content[m.cursorRow]
	col := m.cursorCol

	// Build array of graphemes for efficient access
	graphemeCount := GraphemeCount(line)

	// Handle empty line
	if graphemeCount == 0 {
		return Position{}, Position{}, false
	}

	// Handle cursor beyond line length
	if col >= graphemeCount {
		return Position{}, Position{}, false
	}

	// Get all graphemes for this line
	graphemes := GraphemesInRange(line, 0, graphemeCount)

	var startCol, endCol int

	if w.bigWord {
		// WORD: any non-whitespace sequence

		// If on whitespace, no word found
		if graphemeType(graphemes[col]) == graphemeWhitespace {
			return Position{}, Position{}, false
		}

		// Scan backward while not whitespace
		startCol = col
		for startCol > 0 && graphemeType(graphemes[startCol-1]) != graphemeWhitespace {
			startCol--
		}

		// Scan forward while not whitespace
		endCol = col
		for endCol < graphemeCount-1 && graphemeType(graphemes[endCol+1]) != graphemeWhitespace {
			endCol++
		}
	} else {
		// word: use graphemeType() for word/punct/whitespace classification
		curType := graphemeType(graphemes[col])

		// If on whitespace, no word found
		if curType == graphemeWhitespace {
			return Position{}, Position{}, false
		}

		// Scan backward while same character type
		startCol = col
		for startCol > 0 && graphemeType(graphemes[startCol-1]) == curType {
			startCol--
		}

		// Scan forward while same character type
		endCol = col
		for endCol < graphemeCount-1 && graphemeType(graphemes[endCol+1]) == curType {
			endCol++
		}
	}

	// For "around word" (aw), include surrounding whitespace
	if !inner {
		// Try to include trailing whitespace first
		trailingEnd := endCol
		for trailingEnd < graphemeCount-1 && graphemeType(graphemes[trailingEnd+1]) == graphemeWhitespace {
			trailingEnd++
		}

		if trailingEnd > endCol {
			// Found trailing whitespace, include it
			endCol = trailingEnd
		} else {
			// No trailing whitespace, try leading whitespace (word at line end)
			for startCol > 0 && graphemeType(graphemes[startCol-1]) == graphemeWhitespace {
				startCol--
			}
		}
	}

	return Position{Row: m.cursorRow, Col: startCol},
		Position{Row: m.cursorRow, Col: endCol},
		true
}

// extractText extracts the text between start and end positions (inclusive).
// For single-line extraction, this returns the substring from start.Col to end.Col inclusive.
// For multi-line extraction, it joins the lines with newlines.
// Note: Position.Col values are grapheme indices, not byte offsets.
func extractText(content []string, start, end Position) string {
	if start.Row == end.Row {
		// Single line extraction using grapheme-aware slicing
		line := content[start.Row]
		graphemeCount := GraphemeCount(line)
		if start.Col > graphemeCount {
			return ""
		}
		endCol := min(end.Col+1, graphemeCount) // Make end inclusive
		return SliceByGraphemes(line, start.Col, endCol)
	}

	// Multi-line extraction (not currently used for word objects, but provided for completeness)
	var result string
	for row := start.Row; row <= end.Row; row++ {
		line := content[row]
		graphemeCount := GraphemeCount(line)
		switch row {
		case start.Row:
			result = SliceByGraphemes(line, start.Col, graphemeCount)
		case end.Row:
			endCol := min(end.Col+1, graphemeCount)
			result += "\n" + SliceByGraphemes(line, 0, endCol)
		default:
			result += "\n" + line
		}
	}
	return result
}

// PairedDelimiterTextObject handles paired delimiter text objects like quotes.
// For symmetric delimiters (like quotes), openChar and closeChar are the same.
// For asymmetric delimiters (like parens), they differ.
type PairedDelimiterTextObject struct {
	openChar  rune // Opening delimiter character
	closeChar rune // Closing delimiter character
}

// FindBounds locates the delimiter boundaries around the cursor.
// For inner=true (i"): returns just the content without delimiter characters.
// For inner=false (a"): includes the delimiter characters.
// Note: Positions are returned as grapheme indices, not byte offsets.
func (p *PairedDelimiterTextObject) FindBounds(m *Model, inner bool) (start, end Position, found bool) {
	if m.cursorRow < 0 || m.cursorRow >= len(m.content) {
		return Position{}, Position{}, false
	}

	line := m.content[m.cursorRow]
	col := m.cursorCol

	// Handle empty line or cursor beyond line length (using grapheme count)
	graphemeCount := GraphemeCount(line)
	if graphemeCount == 0 || col >= graphemeCount {
		return Position{}, Position{}, false
	}

	// For symmetric delimiters (quotes), we need to find the enclosing pair
	// by searching outward from the cursor position.
	// Note: findEnclosingPair returns grapheme indices
	openPos, closePos, ok := p.findEnclosingPair(line, col)
	if !ok {
		return Position{}, Position{}, false
	}

	if inner {
		// Inner: exclude delimiters
		// If the delimiters are adjacent (empty content), return false
		if closePos == openPos+1 {
			// Empty content case: "i" on empty quotes `""` should select nothing
			// but still be a valid operation - cursor stays at same position
			return Position{Row: m.cursorRow, Col: openPos + 1},
				Position{Row: m.cursorRow, Col: openPos},
				true
		}
		return Position{Row: m.cursorRow, Col: openPos + 1},
			Position{Row: m.cursorRow, Col: closePos - 1},
			true
	}

	// Around: include delimiters
	return Position{Row: m.cursorRow, Col: openPos},
		Position{Row: m.cursorRow, Col: closePos},
		true
}

// findEnclosingPair finds the positions of the opening and closing delimiters
// that enclose the cursor position. Returns (openPos, closePos, found).
// Note: cursorCol and return values are grapheme indices.
func (p *PairedDelimiterTextObject) findEnclosingPair(line string, cursorCol int) (int, int, bool) {
	if p.openChar == p.closeChar {
		// Symmetric delimiter (quotes): use quote-pairing strategy
		return p.findSymmetricPair(line, cursorCol)
	}
	// Asymmetric delimiter (brackets): use nesting-aware strategy
	return p.findAsymmetricPair(line, cursorCol)
}

// findSymmetricPair handles symmetric delimiters like quotes.
// Strategy: Find quotes surrounding the cursor position contextually.
// This handles cases like 'some" "words"' where left-to-right pairing would
// incorrectly pair the first two quotes, leaving "words" unreachable.
// Note: cursorCol and return values are grapheme indices.
func (p *PairedDelimiterTextObject) findSymmetricPair(line string, cursorCol int) (int, int, bool) {
	// Collect all unescaped quote positions (as grapheme indices)
	var quotePositions []int
	graphemes := GraphemesInRange(line, 0, GraphemeCount(line))

	for i, g := range graphemes {
		// Check if this grapheme is the quote character
		// (single-rune grapheme matching the delimiter)
		if len(g) == len(string(p.openChar)) && []rune(g)[0] == p.openChar && !p.isEscapedGrapheme(graphemes, i) {
			quotePositions = append(quotePositions, i)
		}
	}

	if len(quotePositions) < 2 {
		return -1, -1, false
	}

	// First, try left-to-right pairing and check if cursor is inside any pair
	var pairs [][2]int
	for i := 0; i+1 < len(quotePositions); i += 2 {
		pairs = append(pairs, [2]int{quotePositions[i], quotePositions[i+1]})
	}

	// Check if cursor is inside any existing pair
	for _, pair := range pairs {
		if cursorCol >= pair[0] && cursorCol <= pair[1] {
			return pair[0], pair[1], true
		}
	}

	// Fallback: contextual pairing for edge cases like 'some" "words"'
	// where left-to-right pairing creates wrong pairs, leaving cursor
	// between an unpaired left quote and a right quote.
	// Only applies when cursor is strictly between two quotes.
	leftQuote := -1
	rightQuote := -1

	for _, pos := range quotePositions {
		if pos < cursorCol {
			leftQuote = pos
		}
	}

	for _, pos := range quotePositions {
		if pos > cursorCol {
			rightQuote = pos
			break
		}
	}

	if leftQuote >= 0 && rightQuote >= 0 {
		return leftQuote, rightQuote, true
	}

	return -1, -1, false
}

// findAsymmetricPair handles asymmetric delimiters like brackets with nesting support.
// Strategy: Find the innermost pair that contains the cursor.
// Uses depth counting to handle nested delimiters.
// Note: cursorCol and return values are grapheme indices.
func (p *PairedDelimiterTextObject) findAsymmetricPair(line string, cursorCol int) (int, int, bool) {
	graphemes := GraphemesInRange(line, 0, GraphemeCount(line))

	// Track all opening bracket positions as a stack (grapheme indices)
	var openStack []int

	// Track pairs as we find them (open, close)
	var pairs [][2]int

	for i, g := range graphemes {
		// Check if grapheme is a single rune matching our delimiters
		if len([]rune(g)) == 1 {
			ch := []rune(g)[0]

			if ch == p.openChar && !p.isEscapedGrapheme(graphemes, i) {
				// Push opening position onto stack
				openStack = append(openStack, i)
			} else if ch == p.closeChar && !p.isEscapedGrapheme(graphemes, i) {
				// Pop from stack if we have an opener
				if len(openStack) > 0 {
					openPos := openStack[len(openStack)-1]
					openStack = openStack[:len(openStack)-1]
					pairs = append(pairs, [2]int{openPos, i})
				}
			}
		}
	}

	// Find the innermost pair that contains the cursor
	// Since pairs are discovered when we see the closer, they're stored in inside-out order.
	// We want the smallest (innermost) pair containing cursor.
	var bestPair [2]int
	found := false

	for _, pair := range pairs {
		if cursorCol >= pair[0] && cursorCol <= pair[1] {
			if !found {
				bestPair = pair
				found = true
			} else {
				// Check if this pair is smaller (more inner) than current best
				pairSize := pair[1] - pair[0]
				bestSize := bestPair[1] - bestPair[0]
				if pairSize < bestSize {
					bestPair = pair
				}
			}
		}
	}

	if found {
		return bestPair[0], bestPair[1], true
	}

	return -1, -1, false
}

// isEscapedGrapheme returns true if the grapheme at position pos is escaped.
// A grapheme is escaped if it's preceded by an odd number of backslash graphemes.
func (p *PairedDelimiterTextObject) isEscapedGrapheme(graphemes []string, pos int) bool {
	if pos == 0 {
		return false
	}

	// Count consecutive backslash graphemes before this position
	backslashCount := 0
	for i := pos - 1; i >= 0; i-- {
		if graphemes[i] == "\\" {
			backslashCount++
		} else {
			break
		}
	}

	// Odd number of backslashes = escaped
	return backslashCount%2 == 1
}

// BracketTextObject handles the 'b' text object, which finds the innermost
// bracket pair of any type (parentheses, square brackets, or curly braces)
// containing the cursor.
type BracketTextObject struct{}

// bracketTypes defines the bracket pairs to try for 'b' text object.
var bracketTypes = []struct {
	open  rune
	close rune
}{
	{'(', ')'},
	{'[', ']'},
	{'{', '}'},
}

// FindBounds locates the innermost bracket pair containing the cursor.
// It tries all three bracket types and returns the smallest (innermost) match.
// For inner=true (ib): returns content without delimiter characters.
// For inner=false (ab): includes the delimiter characters.
// Note: Position.Col values are grapheme indices, not byte offsets.
func (b *BracketTextObject) FindBounds(m *Model, inner bool) (start, end Position, found bool) {
	if m.cursorRow < 0 || m.cursorRow >= len(m.content) {
		return Position{}, Position{}, false
	}

	line := m.content[m.cursorRow]
	col := m.cursorCol

	// Handle empty line or cursor beyond line length (using grapheme count)
	graphemeCount := GraphemeCount(line)
	if graphemeCount == 0 || col >= graphemeCount {
		return Position{}, Position{}, false
	}

	// Try each bracket type and find the innermost match
	var bestStart, bestEnd Position
	bestFound := false
	bestSize := -1

	for _, bt := range bracketTypes {
		finder := &PairedDelimiterTextObject{openChar: bt.open, closeChar: bt.close}
		// findAsymmetricPair returns grapheme indices
		openPos, closePos, ok := finder.findAsymmetricPair(line, col)
		if !ok {
			continue
		}

		// Calculate size of this bracket pair
		size := closePos - openPos

		// Keep the smallest (innermost) match
		if !bestFound || size < bestSize {
			bestFound = true
			bestSize = size

			if inner {
				// Inner: exclude delimiters
				if closePos == openPos+1 {
					// Empty content case
					bestStart = Position{Row: m.cursorRow, Col: openPos + 1}
					bestEnd = Position{Row: m.cursorRow, Col: openPos}
				} else {
					bestStart = Position{Row: m.cursorRow, Col: openPos + 1}
					bestEnd = Position{Row: m.cursorRow, Col: closePos - 1}
				}
			} else {
				// Around: include delimiters
				bestStart = Position{Row: m.cursorRow, Col: openPos}
				bestEnd = Position{Row: m.cursorRow, Col: closePos}
			}
		}
	}

	return bestStart, bestEnd, bestFound
}
