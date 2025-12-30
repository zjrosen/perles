package vimtextarea

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// ============================================================================
// WordTextObject Tests (iw/aw)
// ============================================================================

func TestWordTextObject_FindBounds_CursorAtWordStart(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 0 // cursor at 'h'

	finder := &WordTextObject{bigWord: false}
	start, end, found := finder.FindBounds(m, true) // inner word

	assert.True(t, found)
	assert.Equal(t, Position{Row: 0, Col: 0}, start)
	assert.Equal(t, Position{Row: 0, Col: 4}, end) // "hello" is cols 0-4
}

func TestWordTextObject_FindBounds_CursorAtWordMiddle(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 2 // cursor at 'l'

	finder := &WordTextObject{bigWord: false}
	start, end, found := finder.FindBounds(m, true) // inner word

	assert.True(t, found)
	assert.Equal(t, Position{Row: 0, Col: 0}, start)
	assert.Equal(t, Position{Row: 0, Col: 4}, end) // "hello" is cols 0-4
}

func TestWordTextObject_FindBounds_CursorAtWordEnd(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 4 // cursor at 'o' (end of hello)

	finder := &WordTextObject{bigWord: false}
	start, end, found := finder.FindBounds(m, true) // inner word

	assert.True(t, found)
	assert.Equal(t, Position{Row: 0, Col: 0}, start)
	assert.Equal(t, Position{Row: 0, Col: 4}, end)
}

func TestWordTextObject_FindBounds_SingleWordOnLine(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.cursorCol = 2

	finder := &WordTextObject{bigWord: false}
	start, end, found := finder.FindBounds(m, true)

	assert.True(t, found)
	assert.Equal(t, Position{Row: 0, Col: 0}, start)
	assert.Equal(t, Position{Row: 0, Col: 4}, end)
}

func TestWordTextObject_FindBounds_MultipleWordsWithWhitespace(t *testing.T) {
	m := newTestModelWithContent("one two three")
	m.cursorCol = 5 // cursor at 'w' in "two"

	finder := &WordTextObject{bigWord: false}
	start, end, found := finder.FindBounds(m, true)

	assert.True(t, found)
	assert.Equal(t, Position{Row: 0, Col: 4}, start) // "two" starts at col 4
	assert.Equal(t, Position{Row: 0, Col: 6}, end)   // "two" ends at col 6
}

func TestWordTextObject_FindBounds_PunctuationAsSeparateWord(t *testing.T) {
	// In vim, punctuation is a separate "word"
	m := newTestModelWithContent("foo.bar")
	m.cursorCol = 3 // cursor at '.'

	finder := &WordTextObject{bigWord: false}
	start, end, found := finder.FindBounds(m, true)

	assert.True(t, found)
	assert.Equal(t, Position{Row: 0, Col: 3}, start) // '.' is its own word
	assert.Equal(t, Position{Row: 0, Col: 3}, end)
}

func TestWordTextObject_FindBounds_PunctuationSequence(t *testing.T) {
	m := newTestModelWithContent("foo...bar")
	m.cursorCol = 4 // cursor at middle '.'

	finder := &WordTextObject{bigWord: false}
	start, end, found := finder.FindBounds(m, true)

	assert.True(t, found)
	assert.Equal(t, Position{Row: 0, Col: 3}, start) // "..." starts at col 3
	assert.Equal(t, Position{Row: 0, Col: 5}, end)   // "..." ends at col 5
}

func TestWordTextObject_FindBounds_EmptyLineReturnsFalse(t *testing.T) {
	m := newTestModelWithContent("")
	m.cursorCol = 0

	finder := &WordTextObject{bigWord: false}
	_, _, found := finder.FindBounds(m, true)

	assert.False(t, found)
}

func TestWordTextObject_FindBounds_CursorBeyondLineLengthReturnsFalse(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.cursorCol = 10 // beyond line length

	finder := &WordTextObject{bigWord: false}
	_, _, found := finder.FindBounds(m, true)

	assert.False(t, found)
}

func TestWordTextObject_FindBounds_CursorOnWhitespaceReturnsFalse(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 5 // cursor on space

	finder := &WordTextObject{bigWord: false}
	_, _, found := finder.FindBounds(m, true)

	assert.False(t, found)
}

// ============================================================================
// WordTextObject AroundWord Tests (aw)
// ============================================================================

func TestWordTextObject_FindBounds_AroundWordIncludesTrailingWhitespace(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 2 // cursor in "hello"

	finder := &WordTextObject{bigWord: false}
	start, end, found := finder.FindBounds(m, false) // around word

	assert.True(t, found)
	assert.Equal(t, Position{Row: 0, Col: 0}, start)
	assert.Equal(t, Position{Row: 0, Col: 5}, end) // includes trailing space
}

func TestWordTextObject_FindBounds_AroundWordAtLineEndIncludesLeadingWhitespace(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 7 // cursor in "world"

	finder := &WordTextObject{bigWord: false}
	start, end, found := finder.FindBounds(m, false) // around word

	assert.True(t, found)
	// "world" has no trailing whitespace, so should include leading whitespace
	assert.Equal(t, Position{Row: 0, Col: 5}, start) // includes leading space
	assert.Equal(t, Position{Row: 0, Col: 10}, end)  // "world" ends at col 10
}

func TestWordTextObject_FindBounds_AroundWordNoWhitespace(t *testing.T) {
	// Single word with no whitespace around it
	m := newTestModelWithContent("hello")
	m.cursorCol = 2

	finder := &WordTextObject{bigWord: false}
	start, end, found := finder.FindBounds(m, false) // around word

	assert.True(t, found)
	assert.Equal(t, Position{Row: 0, Col: 0}, start)
	assert.Equal(t, Position{Row: 0, Col: 4}, end) // same as inner since no whitespace
}

func TestWordTextObject_FindBounds_AroundWordMultipleTabs(t *testing.T) {
	m := newTestModelWithContent("foo\t\tbar")
	m.cursorCol = 0 // cursor at 'f'

	finder := &WordTextObject{bigWord: false}
	start, end, found := finder.FindBounds(m, false) // around word

	assert.True(t, found)
	assert.Equal(t, Position{Row: 0, Col: 0}, start)
	assert.Equal(t, Position{Row: 0, Col: 4}, end) // includes tabs
}

// ============================================================================
// extractText Tests
// ============================================================================

func TestExtractText_SingleLine(t *testing.T) {
	content := []string{"hello world"}

	text := extractText(content, Position{Row: 0, Col: 0}, Position{Row: 0, Col: 4})
	assert.Equal(t, "hello", text)

	text = extractText(content, Position{Row: 0, Col: 6}, Position{Row: 0, Col: 10})
	assert.Equal(t, "world", text)
}

func TestExtractText_SingleCharacter(t *testing.T) {
	content := []string{"hello"}

	text := extractText(content, Position{Row: 0, Col: 2}, Position{Row: 0, Col: 2})
	assert.Equal(t, "l", text)
}

func TestExtractText_WholeLineSingleRow(t *testing.T) {
	content := []string{"hello"}

	text := extractText(content, Position{Row: 0, Col: 0}, Position{Row: 0, Col: 4})
	assert.Equal(t, "hello", text)
}

func TestExtractText_EmptyResult(t *testing.T) {
	content := []string{"hello"}

	// Start beyond line length
	text := extractText(content, Position{Row: 0, Col: 10}, Position{Row: 0, Col: 15})
	assert.Equal(t, "", text)
}

func TestExtractText_MultiLine(t *testing.T) {
	content := []string{"first line", "second line", "third line"}

	text := extractText(content, Position{Row: 0, Col: 6}, Position{Row: 2, Col: 4})
	assert.Equal(t, "line\nsecond line\nthird", text)
}

// ============================================================================
// TextObjectRegistry Tests
// ============================================================================

func TestTextObjectRegistry_WordRegistered(t *testing.T) {
	finder, ok := textObjectRegistry['w']

	assert.True(t, ok)
	assert.NotNil(t, finder)

	// Verify it's the correct type
	wordObj, isWord := finder.(*WordTextObject)
	assert.True(t, isWord)
	assert.False(t, wordObj.bigWord) // 'w' should be word, not WORD
}

// ============================================================================
// Edge Cases
// ============================================================================

func TestWordTextObject_FindBounds_UnderscoredWord(t *testing.T) {
	// Underscores are part of words in vim
	m := newTestModelWithContent("foo_bar_baz")
	m.cursorCol = 5 // cursor at 'a' in bar

	finder := &WordTextObject{bigWord: false}
	start, end, found := finder.FindBounds(m, true)

	assert.True(t, found)
	assert.Equal(t, Position{Row: 0, Col: 0}, start) // whole identifier
	assert.Equal(t, Position{Row: 0, Col: 10}, end)
}

func TestWordTextObject_FindBounds_NumbersInWord(t *testing.T) {
	m := newTestModelWithContent("var123 test")
	m.cursorCol = 3 // cursor at '1'

	finder := &WordTextObject{bigWord: false}
	start, end, found := finder.FindBounds(m, true)

	assert.True(t, found)
	assert.Equal(t, Position{Row: 0, Col: 0}, start)
	assert.Equal(t, Position{Row: 0, Col: 5}, end) // "var123"
}

func TestWordTextObject_FindBounds_WordOnSecondLine(t *testing.T) {
	m := newTestModelWithContent("first line", "second line")
	m.cursorRow = 1
	m.cursorCol = 0 // cursor at 's'

	finder := &WordTextObject{bigWord: false}
	start, end, found := finder.FindBounds(m, true)

	assert.True(t, found)
	assert.Equal(t, Position{Row: 1, Col: 0}, start)
	assert.Equal(t, Position{Row: 1, Col: 5}, end) // "second"
}

func TestWordTextObject_FindBounds_InvalidRow(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.cursorRow = 5 // invalid row

	finder := &WordTextObject{bigWord: false}
	_, _, found := finder.FindBounds(m, true)

	assert.False(t, found)
}

// ============================================================================
// WORD TextObject Tests (iW/aW) - bigWord=true
// ============================================================================

func TestWORDTextObject_FindBounds_EntireNonWhitespaceSequence(t *testing.T) {
	// WORD treats foo.bar as a single WORD (unlike word which sees 3 parts)
	m := newTestModelWithContent("foo.bar baz")
	m.cursorCol = 2 // cursor at 'o' in "foo.bar"

	finder := &WordTextObject{bigWord: true}
	start, end, found := finder.FindBounds(m, true) // inner WORD

	assert.True(t, found)
	assert.Equal(t, Position{Row: 0, Col: 0}, start)
	assert.Equal(t, Position{Row: 0, Col: 6}, end) // "foo.bar" is cols 0-6
}

func TestWORDTextObject_FindBounds_IncludesPunctuation(t *testing.T) {
	// "foo.bar(baz)" is one WORD
	m := newTestModelWithContent("foo.bar(baz) qux")
	m.cursorCol = 5 // cursor at 'a' in "bar"

	finder := &WordTextObject{bigWord: true}
	start, end, found := finder.FindBounds(m, true)

	assert.True(t, found)
	assert.Equal(t, Position{Row: 0, Col: 0}, start)
	assert.Equal(t, Position{Row: 0, Col: 11}, end) // "foo.bar(baz)" is cols 0-11
}

func TestWORDTextObject_FindBounds_CursorOnPunctuation(t *testing.T) {
	m := newTestModelWithContent("foo.bar baz")
	m.cursorCol = 3 // cursor at '.'

	finder := &WordTextObject{bigWord: true}
	start, end, found := finder.FindBounds(m, true)

	assert.True(t, found)
	assert.Equal(t, Position{Row: 0, Col: 0}, start)
	assert.Equal(t, Position{Row: 0, Col: 6}, end) // entire "foo.bar"
}

func TestWORDTextObject_FindBounds_AroundWordIncludesTrailingWhitespace(t *testing.T) {
	m := newTestModelWithContent("foo.bar baz")
	m.cursorCol = 2 // cursor in "foo.bar"

	finder := &WordTextObject{bigWord: true}
	start, end, found := finder.FindBounds(m, false) // around WORD

	assert.True(t, found)
	assert.Equal(t, Position{Row: 0, Col: 0}, start)
	assert.Equal(t, Position{Row: 0, Col: 7}, end) // includes trailing space
}

func TestWORDTextObject_FindBounds_AtLineEndIncludesLeadingWhitespace(t *testing.T) {
	m := newTestModelWithContent("foo bar.baz")
	m.cursorCol = 6 // cursor in "bar.baz"

	finder := &WordTextObject{bigWord: true}
	start, end, found := finder.FindBounds(m, false) // around WORD

	assert.True(t, found)
	// "bar.baz" has no trailing whitespace, so should include leading whitespace
	assert.Equal(t, Position{Row: 0, Col: 3}, start) // includes leading space
	assert.Equal(t, Position{Row: 0, Col: 10}, end)  // "bar.baz" ends at col 10
}

func TestWORDTextObject_FindBounds_WhitespaceOnlyLineReturnsFalse(t *testing.T) {
	m := newTestModelWithContent("   ")
	m.cursorCol = 1 // cursor on whitespace

	finder := &WordTextObject{bigWord: true}
	_, _, found := finder.FindBounds(m, true)

	assert.False(t, found)
}

func TestWORDTextObject_FindBounds_TabsAreWhitespace(t *testing.T) {
	m := newTestModelWithContent("foo\tbar")
	m.cursorCol = 0 // cursor at 'f'

	finder := &WordTextObject{bigWord: true}
	start, end, found := finder.FindBounds(m, true)

	assert.True(t, found)
	assert.Equal(t, Position{Row: 0, Col: 0}, start)
	assert.Equal(t, Position{Row: 0, Col: 2}, end) // "foo" only, tab is separator
}

func TestWORDTextObject_FindBounds_SingleCharacterWORD(t *testing.T) {
	m := newTestModelWithContent("a b c")
	m.cursorCol = 2 // cursor at 'b'

	finder := &WordTextObject{bigWord: true}
	start, end, found := finder.FindBounds(m, true)

	assert.True(t, found)
	assert.Equal(t, Position{Row: 0, Col: 2}, start)
	assert.Equal(t, Position{Row: 0, Col: 2}, end) // just 'b'
}

// ============================================================================
// WORD TextObject Registry Tests
// ============================================================================

func TestTextObjectRegistry_WORDRegistered(t *testing.T) {
	finder, ok := textObjectRegistry['W']

	assert.True(t, ok)
	assert.NotNil(t, finder)

	// Verify it's the correct type
	wordObj, isWord := finder.(*WordTextObject)
	assert.True(t, isWord)
	assert.True(t, wordObj.bigWord) // 'W' should be WORD, not word
}

// ============================================================================
// PairedDelimiterTextObject Tests (i"/a", i'/a')
// ============================================================================

func TestPairedDelimiterTextObject_FindBounds_DoubleQuote_CursorInside(t *testing.T) {
	m := newTestModelWithContent(`say "hello world" now`)
	m.cursorCol = 7 // cursor at 'l' inside quotes

	finder := &PairedDelimiterTextObject{openChar: '"', closeChar: '"'}
	start, end, found := finder.FindBounds(m, true) // inner

	assert.True(t, found)
	assert.Equal(t, Position{Row: 0, Col: 5}, start) // after opening "
	assert.Equal(t, Position{Row: 0, Col: 15}, end)  // before closing "
	assert.Equal(t, "hello world", extractText(m.content, start, end))
}

func TestPairedDelimiterTextObject_FindBounds_DoubleQuote_AroundIncludesQuotes(t *testing.T) {
	m := newTestModelWithContent(`say "hello world" now`)
	m.cursorCol = 7 // cursor at 'l' inside quotes

	finder := &PairedDelimiterTextObject{openChar: '"', closeChar: '"'}
	start, end, found := finder.FindBounds(m, false) // around

	assert.True(t, found)
	assert.Equal(t, Position{Row: 0, Col: 4}, start) // opening "
	assert.Equal(t, Position{Row: 0, Col: 16}, end)  // closing "
	assert.Equal(t, `"hello world"`, extractText(m.content, start, end))
}

func TestPairedDelimiterTextObject_FindBounds_DoubleQuote_CursorOnOpeningQuote(t *testing.T) {
	m := newTestModelWithContent(`say "hello" now`)
	m.cursorCol = 4 // cursor on opening "

	finder := &PairedDelimiterTextObject{openChar: '"', closeChar: '"'}
	start, end, found := finder.FindBounds(m, true) // inner

	assert.True(t, found)
	assert.Equal(t, Position{Row: 0, Col: 5}, start)
	assert.Equal(t, Position{Row: 0, Col: 9}, end)
	assert.Equal(t, "hello", extractText(m.content, start, end))
}

func TestPairedDelimiterTextObject_FindBounds_DoubleQuote_CursorOnClosingQuote(t *testing.T) {
	m := newTestModelWithContent(`say "hello" now`)
	m.cursorCol = 10 // cursor on closing "

	finder := &PairedDelimiterTextObject{openChar: '"', closeChar: '"'}
	start, end, found := finder.FindBounds(m, true) // inner

	assert.True(t, found)
	assert.Equal(t, Position{Row: 0, Col: 5}, start)
	assert.Equal(t, Position{Row: 0, Col: 9}, end)
	assert.Equal(t, "hello", extractText(m.content, start, end))
}

func TestPairedDelimiterTextObject_FindBounds_DoubleQuote_EscapedQuoteNotDelimiter(t *testing.T) {
	// The \" in the middle should not be treated as a delimiter
	m := newTestModelWithContent(`say "hello \"world\" end" now`)
	m.cursorCol = 7 // cursor at 'l' inside quotes

	finder := &PairedDelimiterTextObject{openChar: '"', closeChar: '"'}
	start, end, found := finder.FindBounds(m, true) // inner

	assert.True(t, found)
	assert.Equal(t, Position{Row: 0, Col: 5}, start)
	assert.Equal(t, Position{Row: 0, Col: 23}, end)
	assert.Equal(t, `hello \"world\" end`, extractText(m.content, start, end))
}

func TestPairedDelimiterTextObject_FindBounds_DoubleQuote_ConsecutiveEscapes(t *testing.T) {
	// "\\" is an escaped backslash, followed by " which IS a delimiter
	m := newTestModelWithContent(`say "foo\\" now`)
	m.cursorCol = 6 // cursor at 'o' inside quotes

	finder := &PairedDelimiterTextObject{openChar: '"', closeChar: '"'}
	start, end, found := finder.FindBounds(m, true) // inner

	assert.True(t, found)
	assert.Equal(t, Position{Row: 0, Col: 5}, start)
	assert.Equal(t, Position{Row: 0, Col: 9}, end)
	assert.Equal(t, `foo\\`, extractText(m.content, start, end))
}

func TestPairedDelimiterTextObject_FindBounds_DoubleQuote_TripleBackslashEscapedQuote(t *testing.T) {
	// "\\\"" is escaped backslash + escaped quote (3 backslashes = odd, so quote is escaped)
	m := newTestModelWithContent(`say "foo\\\" bar" now`)
	m.cursorCol = 6 // cursor at 'o' inside quotes

	finder := &PairedDelimiterTextObject{openChar: '"', closeChar: '"'}
	start, end, found := finder.FindBounds(m, true) // inner

	assert.True(t, found)
	assert.Equal(t, Position{Row: 0, Col: 5}, start)
	assert.Equal(t, Position{Row: 0, Col: 15}, end)
	assert.Equal(t, `foo\\\" bar`, extractText(m.content, start, end))
}

func TestPairedDelimiterTextObject_FindBounds_DoubleQuote_UnclosedQuoteReturnsFalse(t *testing.T) {
	m := newTestModelWithContent(`say "hello world`)
	m.cursorCol = 7 // cursor at 'l' - inside unclosed quote

	finder := &PairedDelimiterTextObject{openChar: '"', closeChar: '"'}
	_, _, found := finder.FindBounds(m, true)

	assert.False(t, found)
}

func TestPairedDelimiterTextObject_FindBounds_DoubleQuote_CursorBeforeQuotesReturnsFalse(t *testing.T) {
	m := newTestModelWithContent(`say "hello" now`)
	m.cursorCol = 0 // cursor at 's' - before all quotes

	finder := &PairedDelimiterTextObject{openChar: '"', closeChar: '"'}
	_, _, found := finder.FindBounds(m, true)

	assert.False(t, found)
}

func TestPairedDelimiterTextObject_FindBounds_DoubleQuote_CursorAfterAllQuotesReturnsFalse(t *testing.T) {
	m := newTestModelWithContent(`say "hello" now`)
	m.cursorCol = 12 // cursor at 'n' in 'now' - after all quotes

	finder := &PairedDelimiterTextObject{openChar: '"', closeChar: '"'}
	_, _, found := finder.FindBounds(m, true)

	assert.False(t, found)
}

func TestPairedDelimiterTextObject_FindBounds_DoubleQuote_UnpairedQuoteBeforePair(t *testing.T) {
	// Bug fix: 'some" "words"' - unpaired quote before a valid pair
	// Cursor on 'r' in 'words' - should find "words"
	m := newTestModelWithContent(`some" "words"`)
	m.cursorCol = 8 // cursor at 'r' in 'words'

	finder := &PairedDelimiterTextObject{openChar: '"', closeChar: '"'}
	start, end, found := finder.FindBounds(m, true)

	assert.True(t, found)
	assert.Equal(t, Position{Row: 0, Col: 7}, start)
	assert.Equal(t, Position{Row: 0, Col: 11}, end)
	assert.Equal(t, "words", extractText(m.content, start, end))
}

func TestPairedDelimiterTextObject_FindBounds_DoubleQuote_NestedSingleQuotes(t *testing.T) {
	// Double quotes containing single quotes - di" should get the whole thing
	m := newTestModelWithContent(`say "foo 'bar' baz" now`)
	m.cursorCol = 10 // cursor at 'b' in 'bar'

	finder := &PairedDelimiterTextObject{openChar: '"', closeChar: '"'}
	start, end, found := finder.FindBounds(m, true) // inner

	assert.True(t, found)
	assert.Equal(t, Position{Row: 0, Col: 5}, start)
	assert.Equal(t, Position{Row: 0, Col: 17}, end)
	assert.Equal(t, "foo 'bar' baz", extractText(m.content, start, end))
}

func TestPairedDelimiterTextObject_FindBounds_SingleQuote_CursorInside(t *testing.T) {
	m := newTestModelWithContent(`say 'hello world' now`)
	m.cursorCol = 7 // cursor at 'l' inside quotes

	finder := &PairedDelimiterTextObject{openChar: '\'', closeChar: '\''}
	start, end, found := finder.FindBounds(m, true) // inner

	assert.True(t, found)
	assert.Equal(t, Position{Row: 0, Col: 5}, start)
	assert.Equal(t, Position{Row: 0, Col: 15}, end)
	assert.Equal(t, "hello world", extractText(m.content, start, end))
}

func TestPairedDelimiterTextObject_FindBounds_SingleQuote_AroundIncludesQuotes(t *testing.T) {
	m := newTestModelWithContent(`say 'hello' now`)
	m.cursorCol = 7 // cursor at 'l' inside quotes

	finder := &PairedDelimiterTextObject{openChar: '\'', closeChar: '\''}
	start, end, found := finder.FindBounds(m, false) // around

	assert.True(t, found)
	assert.Equal(t, Position{Row: 0, Col: 4}, start)
	assert.Equal(t, Position{Row: 0, Col: 10}, end)
	assert.Equal(t, "'hello'", extractText(m.content, start, end))
}

func TestPairedDelimiterTextObject_FindBounds_SingleQuote_NestedInDoubleQuotes(t *testing.T) {
	// Cursor inside single quotes that are inside double quotes - ci' should change 'bar'
	m := newTestModelWithContent(`say "foo 'bar' baz" now`)
	m.cursorCol = 11 // cursor at 'a' in 'bar'

	finder := &PairedDelimiterTextObject{openChar: '\'', closeChar: '\''}
	start, end, found := finder.FindBounds(m, true) // inner

	assert.True(t, found)
	assert.Equal(t, Position{Row: 0, Col: 10}, start)
	assert.Equal(t, Position{Row: 0, Col: 12}, end)
	assert.Equal(t, "bar", extractText(m.content, start, end))
}

func TestPairedDelimiterTextObject_FindBounds_EmptyQuotes(t *testing.T) {
	m := newTestModelWithContent(`say "" now`)
	m.cursorCol = 4 // cursor on opening "

	finder := &PairedDelimiterTextObject{openChar: '"', closeChar: '"'}
	start, end, found := finder.FindBounds(m, true) // inner

	// For empty quotes, inner should return found=true but with end < start
	// indicating empty content
	assert.True(t, found)
	// Start is after opening quote, end is before closing quote
	// For empty quotes "", start=5 and end=4 (empty range)
	assert.Equal(t, Position{Row: 0, Col: 5}, start)
	assert.Equal(t, Position{Row: 0, Col: 4}, end)
}

func TestPairedDelimiterTextObject_FindBounds_EmptyQuotes_Around(t *testing.T) {
	m := newTestModelWithContent(`say "" now`)
	m.cursorCol = 4 // cursor on opening "

	finder := &PairedDelimiterTextObject{openChar: '"', closeChar: '"'}
	start, end, found := finder.FindBounds(m, false) // around

	assert.True(t, found)
	assert.Equal(t, Position{Row: 0, Col: 4}, start)
	assert.Equal(t, Position{Row: 0, Col: 5}, end)
	assert.Equal(t, `""`, extractText(m.content, start, end))
}

func TestPairedDelimiterTextObject_FindBounds_EmptyLine(t *testing.T) {
	m := newTestModelWithContent("")
	m.cursorCol = 0

	finder := &PairedDelimiterTextObject{openChar: '"', closeChar: '"'}
	_, _, found := finder.FindBounds(m, true)

	assert.False(t, found)
}

func TestPairedDelimiterTextObject_FindBounds_MultipleQuotePairs(t *testing.T) {
	// Multiple quote pairs on same line - cursor in second pair
	m := newTestModelWithContent(`"first" and "second"`)
	m.cursorCol = 15 // cursor at 'c' in "second"

	finder := &PairedDelimiterTextObject{openChar: '"', closeChar: '"'}
	start, end, found := finder.FindBounds(m, true) // inner

	assert.True(t, found)
	assert.Equal(t, Position{Row: 0, Col: 13}, start)
	assert.Equal(t, Position{Row: 0, Col: 18}, end)
	assert.Equal(t, "second", extractText(m.content, start, end))
}

func TestPairedDelimiterTextObject_FindBounds_MultipleQuotePairs_First(t *testing.T) {
	// Multiple quote pairs on same line - cursor in first pair
	m := newTestModelWithContent(`"first" and "second"`)
	m.cursorCol = 3 // cursor at 'r' in "first"

	finder := &PairedDelimiterTextObject{openChar: '"', closeChar: '"'}
	start, end, found := finder.FindBounds(m, true) // inner

	assert.True(t, found)
	assert.Equal(t, Position{Row: 0, Col: 1}, start)
	assert.Equal(t, Position{Row: 0, Col: 5}, end)
	assert.Equal(t, "first", extractText(m.content, start, end))
}

// ============================================================================
// Quote TextObject Registry Tests
// ============================================================================

func TestTextObjectRegistry_DoubleQuoteRegistered(t *testing.T) {
	finder, ok := textObjectRegistry['"']

	assert.True(t, ok)
	assert.NotNil(t, finder)

	// Verify it's the correct type
	quoteObj, isQuote := finder.(*PairedDelimiterTextObject)
	assert.True(t, isQuote)
	assert.Equal(t, '"', quoteObj.openChar)
	assert.Equal(t, '"', quoteObj.closeChar)
}

func TestTextObjectRegistry_SingleQuoteRegistered(t *testing.T) {
	finder, ok := textObjectRegistry['\'']

	assert.True(t, ok)
	assert.NotNil(t, finder)

	// Verify it's the correct type
	quoteObj, isQuote := finder.(*PairedDelimiterTextObject)
	assert.True(t, isQuote)
	assert.Equal(t, '\'', quoteObj.openChar)
	assert.Equal(t, '\'', quoteObj.closeChar)
}

// ============================================================================
// isEscaped Helper Tests
// ============================================================================

func TestPairedDelimiterTextObject_isEscaped(t *testing.T) {
	finder := &PairedDelimiterTextObject{openChar: '"', closeChar: '"'}

	tests := []struct {
		name     string
		line     string
		pos      int
		expected bool
	}{
		{"no backslash", `"hello"`, 6, false},             // closing quote
		{"single backslash", `"hello\"world"`, 7, true},   // escaped quote
		{"double backslash", `"hello\\"`, 8, false},       // not escaped (even backslashes)
		{"triple backslash", `"hello\\\"world"`, 9, true}, // escaped (odd backslashes)
		{"position 0", `"hello"`, 0, false},               // first char can't be escaped
		{"backslash at start", `\"hello"`, 1, true},       // escaped quote after one backslash
		{"four backslashes", `"hello\\\\"`, 10, false},    // not escaped (4 backslashes = even)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			graphemes := GraphemesInRange(tt.line, 0, GraphemeCount(tt.line))
			result := finder.isEscapedGrapheme(graphemes, tt.pos)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ============================================================================
// Bracket TextObject Tests (parentheses, brackets, braces)
// ============================================================================

func TestBracketTextObject_FindBounds_Parentheses_CursorInside(t *testing.T) {
	m := newTestModelWithContent("foo (bar) baz")
	m.cursorCol = 6 // cursor at 'a' inside parens

	finder := &PairedDelimiterTextObject{openChar: '(', closeChar: ')'}
	start, end, found := finder.FindBounds(m, true) // inner

	assert.True(t, found)
	assert.Equal(t, Position{Row: 0, Col: 5}, start) // after (
	assert.Equal(t, Position{Row: 0, Col: 7}, end)   // before )
	assert.Equal(t, "bar", extractText(m.content, start, end))
}

func TestBracketTextObject_FindBounds_Parentheses_AroundIncludesDelimiters(t *testing.T) {
	m := newTestModelWithContent("foo (bar) baz")
	m.cursorCol = 6 // cursor at 'a' inside parens

	finder := &PairedDelimiterTextObject{openChar: '(', closeChar: ')'}
	start, end, found := finder.FindBounds(m, false) // around

	assert.True(t, found)
	assert.Equal(t, Position{Row: 0, Col: 4}, start) // opening (
	assert.Equal(t, Position{Row: 0, Col: 8}, end)   // closing )
	assert.Equal(t, "(bar)", extractText(m.content, start, end))
}

func TestBracketTextObject_FindBounds_Parentheses_CursorOnOpeningDelimiter(t *testing.T) {
	m := newTestModelWithContent("foo (bar) baz")
	m.cursorCol = 4 // cursor on (

	finder := &PairedDelimiterTextObject{openChar: '(', closeChar: ')'}
	start, end, found := finder.FindBounds(m, true) // inner

	assert.True(t, found)
	assert.Equal(t, Position{Row: 0, Col: 5}, start)
	assert.Equal(t, Position{Row: 0, Col: 7}, end)
	assert.Equal(t, "bar", extractText(m.content, start, end))
}

func TestBracketTextObject_FindBounds_Parentheses_CursorOnClosingDelimiter(t *testing.T) {
	m := newTestModelWithContent("foo (bar) baz")
	m.cursorCol = 8 // cursor on )

	finder := &PairedDelimiterTextObject{openChar: '(', closeChar: ')'}
	start, end, found := finder.FindBounds(m, true) // inner

	assert.True(t, found)
	assert.Equal(t, Position{Row: 0, Col: 5}, start)
	assert.Equal(t, Position{Row: 0, Col: 7}, end)
	assert.Equal(t, "bar", extractText(m.content, start, end))
}

func TestBracketTextObject_FindBounds_Parentheses_ClosingCharEquivalentToOpening(t *testing.T) {
	// di) should work the same as di(
	m := newTestModelWithContent("foo (bar) baz")
	m.cursorCol = 6

	finderOpen := &PairedDelimiterTextObject{openChar: '(', closeChar: ')'}
	finderClose := &PairedDelimiterTextObject{openChar: '(', closeChar: ')'}

	startO, endO, foundO := finderOpen.FindBounds(m, true)
	startC, endC, foundC := finderClose.FindBounds(m, true)

	assert.True(t, foundO)
	assert.True(t, foundC)
	assert.Equal(t, startO, startC)
	assert.Equal(t, endO, endC)
}

func TestBracketTextObject_FindBounds_SquareBrackets_CursorInside(t *testing.T) {
	m := newTestModelWithContent("arr[index]")
	m.cursorCol = 5 // cursor at 'n' inside brackets

	finder := &PairedDelimiterTextObject{openChar: '[', closeChar: ']'}
	start, end, found := finder.FindBounds(m, true) // inner

	assert.True(t, found)
	assert.Equal(t, Position{Row: 0, Col: 4}, start)
	assert.Equal(t, Position{Row: 0, Col: 8}, end)
	assert.Equal(t, "index", extractText(m.content, start, end))
}

func TestBracketTextObject_FindBounds_SquareBrackets_Around(t *testing.T) {
	m := newTestModelWithContent("arr[index]")
	m.cursorCol = 5

	finder := &PairedDelimiterTextObject{openChar: '[', closeChar: ']'}
	start, end, found := finder.FindBounds(m, false) // around

	assert.True(t, found)
	assert.Equal(t, Position{Row: 0, Col: 3}, start)
	assert.Equal(t, Position{Row: 0, Col: 9}, end)
	assert.Equal(t, "[index]", extractText(m.content, start, end))
}

func TestBracketTextObject_FindBounds_CurlyBraces_CursorInside(t *testing.T) {
	m := newTestModelWithContent("obj{value}")
	m.cursorCol = 5 // cursor at 'a'

	finder := &PairedDelimiterTextObject{openChar: '{', closeChar: '}'}
	start, end, found := finder.FindBounds(m, true)

	assert.True(t, found)
	assert.Equal(t, Position{Row: 0, Col: 4}, start)
	assert.Equal(t, Position{Row: 0, Col: 8}, end)
	assert.Equal(t, "value", extractText(m.content, start, end))
}

func TestBracketTextObject_FindBounds_CurlyBraces_Around(t *testing.T) {
	m := newTestModelWithContent("obj{value}")
	m.cursorCol = 5

	finder := &PairedDelimiterTextObject{openChar: '{', closeChar: '}'}
	start, end, found := finder.FindBounds(m, false)

	assert.True(t, found)
	assert.Equal(t, Position{Row: 0, Col: 3}, start)
	assert.Equal(t, Position{Row: 0, Col: 9}, end)
	assert.Equal(t, "{value}", extractText(m.content, start, end))
}

// ============================================================================
// Nested Bracket Tests
// ============================================================================

func TestBracketTextObject_FindBounds_NestedParentheses_CursorInInner(t *testing.T) {
	// Nested: (foo (bar) baz)
	//              ^^^ cursor in inner parens
	m := newTestModelWithContent("(foo (bar) baz)")
	m.cursorCol = 7 // cursor at 'a' in inner (bar)

	finder := &PairedDelimiterTextObject{openChar: '(', closeChar: ')'}
	start, end, found := finder.FindBounds(m, true)

	assert.True(t, found)
	assert.Equal(t, Position{Row: 0, Col: 6}, start)
	assert.Equal(t, Position{Row: 0, Col: 8}, end)
	assert.Equal(t, "bar", extractText(m.content, start, end))
}

func TestBracketTextObject_FindBounds_NestedParentheses_CursorInOuter(t *testing.T) {
	// Nested: (foo (bar) baz)
	//           ^ cursor in outer but outside inner
	m := newTestModelWithContent("(foo (bar) baz)")
	m.cursorCol = 2 // cursor at 'o' in "foo"

	finder := &PairedDelimiterTextObject{openChar: '(', closeChar: ')'}
	start, end, found := finder.FindBounds(m, true)

	assert.True(t, found)
	assert.Equal(t, Position{Row: 0, Col: 1}, start)
	assert.Equal(t, Position{Row: 0, Col: 13}, end)
	assert.Equal(t, "foo (bar) baz", extractText(m.content, start, end))
}

func TestBracketTextObject_FindBounds_NestedParentheses_CursorBetweenInnerAndOuter(t *testing.T) {
	// Nested: (foo (bar) baz)
	//                   ^ cursor at space after inner close but still in outer
	m := newTestModelWithContent("(foo (bar) baz)")
	m.cursorCol = 10 // cursor at space after )

	finder := &PairedDelimiterTextObject{openChar: '(', closeChar: ')'}
	start, end, found := finder.FindBounds(m, true)

	assert.True(t, found)
	assert.Equal(t, Position{Row: 0, Col: 1}, start)
	assert.Equal(t, Position{Row: 0, Col: 13}, end)
	assert.Equal(t, "foo (bar) baz", extractText(m.content, start, end))
}

func TestBracketTextObject_FindBounds_AdjacentNestedBrackets(t *testing.T) {
	// ((foo))
	//   ^ cursor at 'f' should select innermost
	m := newTestModelWithContent("((foo))")
	m.cursorCol = 2 // cursor at 'f'

	finder := &PairedDelimiterTextObject{openChar: '(', closeChar: ')'}
	start, end, found := finder.FindBounds(m, true)

	assert.True(t, found)
	assert.Equal(t, Position{Row: 0, Col: 2}, start)
	assert.Equal(t, Position{Row: 0, Col: 4}, end)
	assert.Equal(t, "foo", extractText(m.content, start, end))
}

func TestBracketTextObject_FindBounds_MixedDelimiterTypes(t *testing.T) {
	// ([foo]) - cursor inside brackets, brackets are inside parens
	m := newTestModelWithContent("([foo])")
	m.cursorCol = 3 // cursor at 'o' in foo

	// di[ should select just foo (inside brackets)
	finderBracket := &PairedDelimiterTextObject{openChar: '[', closeChar: ']'}
	start, end, found := finderBracket.FindBounds(m, true)

	assert.True(t, found)
	assert.Equal(t, Position{Row: 0, Col: 2}, start)
	assert.Equal(t, Position{Row: 0, Col: 4}, end)
	assert.Equal(t, "foo", extractText(m.content, start, end))

	// di( should select [foo] (inside parens)
	finderParen := &PairedDelimiterTextObject{openChar: '(', closeChar: ')'}
	start2, end2, found2 := finderParen.FindBounds(m, true)

	assert.True(t, found2)
	assert.Equal(t, Position{Row: 0, Col: 1}, start2)
	assert.Equal(t, Position{Row: 0, Col: 5}, end2)
	assert.Equal(t, "[foo]", extractText(m.content, start2, end2))
}

// ============================================================================
// Edge Cases for Brackets
// ============================================================================

func TestBracketTextObject_FindBounds_EmptyDelimiters(t *testing.T) {
	// Empty brackets ()
	m := newTestModelWithContent("foo() bar")
	m.cursorCol = 3 // cursor on (

	finder := &PairedDelimiterTextObject{openChar: '(', closeChar: ')'}
	start, end, found := finder.FindBounds(m, true)

	// For empty content, inner should return valid but empty range
	assert.True(t, found)
	assert.Equal(t, Position{Row: 0, Col: 4}, start)
	assert.Equal(t, Position{Row: 0, Col: 3}, end) // end < start indicates empty
}

func TestBracketTextObject_FindBounds_EmptyDelimiters_Around(t *testing.T) {
	m := newTestModelWithContent("foo() bar")
	m.cursorCol = 3 // cursor on (

	finder := &PairedDelimiterTextObject{openChar: '(', closeChar: ')'}
	start, end, found := finder.FindBounds(m, false)

	assert.True(t, found)
	assert.Equal(t, Position{Row: 0, Col: 3}, start)
	assert.Equal(t, Position{Row: 0, Col: 4}, end)
	assert.Equal(t, "()", extractText(m.content, start, end))
}

func TestBracketTextObject_FindBounds_UnclosedDelimiter_ReturnsFalse(t *testing.T) {
	m := newTestModelWithContent("foo (bar")
	m.cursorCol = 5

	finder := &PairedDelimiterTextObject{openChar: '(', closeChar: ')'}
	_, _, found := finder.FindBounds(m, true)

	assert.False(t, found)
}

func TestBracketTextObject_FindBounds_UnopenedDelimiter_ReturnsFalse(t *testing.T) {
	m := newTestModelWithContent("foo bar)")
	m.cursorCol = 5

	finder := &PairedDelimiterTextObject{openChar: '(', closeChar: ')'}
	_, _, found := finder.FindBounds(m, true)

	assert.False(t, found)
}

func TestBracketTextObject_FindBounds_CursorOutsideBrackets_ReturnsFalse(t *testing.T) {
	m := newTestModelWithContent("foo (bar) baz")
	m.cursorCol = 1 // cursor at 'o' - outside brackets

	finder := &PairedDelimiterTextObject{openChar: '(', closeChar: ')'}
	_, _, found := finder.FindBounds(m, true)

	assert.False(t, found)
}

func TestBracketTextObject_FindBounds_UnbalancedNesting(t *testing.T) {
	// Unbalanced: ((foo)
	m := newTestModelWithContent("((foo)")
	m.cursorCol = 3 // cursor at 'o'

	finder := &PairedDelimiterTextObject{openChar: '(', closeChar: ')'}
	start, end, found := finder.FindBounds(m, true)

	// Should find the one complete pair (1-5)
	assert.True(t, found)
	assert.Equal(t, Position{Row: 0, Col: 2}, start)
	assert.Equal(t, Position{Row: 0, Col: 4}, end)
	assert.Equal(t, "foo", extractText(m.content, start, end))
}

// ============================================================================
// Registry Tests for Brackets
// ============================================================================

func TestTextObjectRegistry_ParenthesesRegistered(t *testing.T) {
	// Test that both ( and ) are registered
	finderOpen, okOpen := textObjectRegistry['(']
	finderClose, okClose := textObjectRegistry[')']

	assert.True(t, okOpen)
	assert.True(t, okClose)

	// Both should be PairedDelimiterTextObject
	parenOpen, isParen := finderOpen.(*PairedDelimiterTextObject)
	assert.True(t, isParen)
	assert.Equal(t, '(', parenOpen.openChar)
	assert.Equal(t, ')', parenOpen.closeChar)

	parenClose, isParen := finderClose.(*PairedDelimiterTextObject)
	assert.True(t, isParen)
	assert.Equal(t, '(', parenClose.openChar)
	assert.Equal(t, ')', parenClose.closeChar)
}

func TestTextObjectRegistry_SquareBracketsRegistered(t *testing.T) {
	finderOpen, okOpen := textObjectRegistry['[']
	finderClose, okClose := textObjectRegistry[']']

	assert.True(t, okOpen)
	assert.True(t, okClose)

	bracketOpen, isBracket := finderOpen.(*PairedDelimiterTextObject)
	assert.True(t, isBracket)
	assert.Equal(t, '[', bracketOpen.openChar)
	assert.Equal(t, ']', bracketOpen.closeChar)

	bracketClose, isBracket := finderClose.(*PairedDelimiterTextObject)
	assert.True(t, isBracket)
	assert.Equal(t, '[', bracketClose.openChar)
	assert.Equal(t, ']', bracketClose.closeChar)
}

func TestTextObjectRegistry_CurlyBracesRegistered(t *testing.T) {
	finderOpen, okOpen := textObjectRegistry['{']
	finderClose, okClose := textObjectRegistry['}']

	assert.True(t, okOpen)
	assert.True(t, okClose)

	braceOpen, isBrace := finderOpen.(*PairedDelimiterTextObject)
	assert.True(t, isBrace)
	assert.Equal(t, '{', braceOpen.openChar)
	assert.Equal(t, '}', braceOpen.closeChar)

	braceClose, isBrace := finderClose.(*PairedDelimiterTextObject)
	assert.True(t, isBrace)
	assert.Equal(t, '{', braceClose.openChar)
	assert.Equal(t, '}', braceClose.closeChar)
}

// ============================================================================
// BracketTextObject Tests (ib/ab) - 'b' for any bracket type
// ============================================================================

func TestBracketTextObject_FindBounds_Parentheses(t *testing.T) {
	m := newTestModelWithContent("foo (bar) baz")
	m.cursorCol = 6 // cursor at 'a' inside parens

	finder := &BracketTextObject{}
	start, end, found := finder.FindBounds(m, true) // inner

	assert.True(t, found)
	assert.Equal(t, Position{Row: 0, Col: 5}, start)
	assert.Equal(t, Position{Row: 0, Col: 7}, end)
	assert.Equal(t, "bar", extractText(m.content, start, end))
}

func TestBracketTextObject_FindBounds_SquareBrackets(t *testing.T) {
	m := newTestModelWithContent("arr[index]")
	m.cursorCol = 5 // cursor at 'n' inside brackets

	finder := &BracketTextObject{}
	start, end, found := finder.FindBounds(m, true) // inner

	assert.True(t, found)
	assert.Equal(t, Position{Row: 0, Col: 4}, start)
	assert.Equal(t, Position{Row: 0, Col: 8}, end)
	assert.Equal(t, "index", extractText(m.content, start, end))
}

func TestBracketTextObject_FindBounds_CurlyBraces(t *testing.T) {
	m := newTestModelWithContent("obj{value}")
	m.cursorCol = 5 // cursor at 'a'

	finder := &BracketTextObject{}
	start, end, found := finder.FindBounds(m, true) // inner

	assert.True(t, found)
	assert.Equal(t, Position{Row: 0, Col: 4}, start)
	assert.Equal(t, Position{Row: 0, Col: 8}, end)
	assert.Equal(t, "value", extractText(m.content, start, end))
}

func TestBracketTextObject_FindBounds_NestedDifferentTypes(t *testing.T) {
	// {[foo]} - cursor on 'foo' should select [] (innermost)
	m := newTestModelWithContent("{[foo]}")
	m.cursorCol = 3 // cursor at 'o' in foo

	finder := &BracketTextObject{}
	start, end, found := finder.FindBounds(m, true) // inner

	assert.True(t, found)
	assert.Equal(t, Position{Row: 0, Col: 2}, start)
	assert.Equal(t, Position{Row: 0, Col: 4}, end)
	assert.Equal(t, "foo", extractText(m.content, start, end))
}

func TestBracketTextObject_FindBounds_NestedSameType(t *testing.T) {
	// ((foo)) - cursor on 'foo' should select inner ()
	m := newTestModelWithContent("((foo))")
	m.cursorCol = 3 // cursor at 'o' in foo

	finder := &BracketTextObject{}
	start, end, found := finder.FindBounds(m, true) // inner

	assert.True(t, found)
	assert.Equal(t, Position{Row: 0, Col: 2}, start)
	assert.Equal(t, Position{Row: 0, Col: 4}, end)
	assert.Equal(t, "foo", extractText(m.content, start, end))
}

func TestBracketTextObject_FindBounds_NoBrackets(t *testing.T) {
	m := newTestModelWithContent("foo bar")
	m.cursorCol = 2

	finder := &BracketTextObject{}
	_, _, found := finder.FindBounds(m, true)

	assert.False(t, found)
}

func TestBracketTextObject_FindBounds_CursorOnDelimiter(t *testing.T) {
	// Test cursor on opening bracket
	m := newTestModelWithContent("foo (bar) baz")
	m.cursorCol = 4 // cursor on (

	finder := &BracketTextObject{}
	start, end, found := finder.FindBounds(m, true) // inner

	assert.True(t, found)
	assert.Equal(t, Position{Row: 0, Col: 5}, start)
	assert.Equal(t, Position{Row: 0, Col: 7}, end)
	assert.Equal(t, "bar", extractText(m.content, start, end))

	// Test cursor on closing bracket
	m.cursorCol = 8 // cursor on )
	start, end, found = finder.FindBounds(m, true)

	assert.True(t, found)
	assert.Equal(t, Position{Row: 0, Col: 5}, start)
	assert.Equal(t, Position{Row: 0, Col: 7}, end)
	assert.Equal(t, "bar", extractText(m.content, start, end))
}

func TestBracketTextObject_FindBounds_Inner_ExcludesDelimiters(t *testing.T) {
	m := newTestModelWithContent("[hello]")
	m.cursorCol = 3

	finder := &BracketTextObject{}
	start, end, found := finder.FindBounds(m, true) // inner

	assert.True(t, found)
	assert.Equal(t, Position{Row: 0, Col: 1}, start) // after [
	assert.Equal(t, Position{Row: 0, Col: 5}, end)   // before ]
	assert.Equal(t, "hello", extractText(m.content, start, end))
}

func TestBracketTextObject_FindBounds_Around_IncludesDelimiters(t *testing.T) {
	m := newTestModelWithContent("[hello]")
	m.cursorCol = 3

	finder := &BracketTextObject{}
	start, end, found := finder.FindBounds(m, false) // around

	assert.True(t, found)
	assert.Equal(t, Position{Row: 0, Col: 0}, start) // [
	assert.Equal(t, Position{Row: 0, Col: 6}, end)   // ]
	assert.Equal(t, "[hello]", extractText(m.content, start, end))
}

func TestBracketTextObject_FindBounds_EmptyBrackets(t *testing.T) {
	m := newTestModelWithContent("foo() bar")
	m.cursorCol = 3 // cursor on (

	finder := &BracketTextObject{}
	start, end, found := finder.FindBounds(m, true) // inner

	// For empty content, inner should return valid but empty range
	assert.True(t, found)
	assert.Equal(t, Position{Row: 0, Col: 4}, start)
	assert.Equal(t, Position{Row: 0, Col: 3}, end) // end < start indicates empty
}

func TestBracketTextObject_FindBounds_EmptyBrackets_Around(t *testing.T) {
	m := newTestModelWithContent("foo() bar")
	m.cursorCol = 3 // cursor on (

	finder := &BracketTextObject{}
	start, end, found := finder.FindBounds(m, false) // around

	assert.True(t, found)
	assert.Equal(t, Position{Row: 0, Col: 3}, start)
	assert.Equal(t, Position{Row: 0, Col: 4}, end)
	assert.Equal(t, "()", extractText(m.content, start, end))
}

func TestBracketTextObject_FindBounds_MultipleNested(t *testing.T) {
	// ([{foo}]) - cursor on 'foo' should select {} (innermost)
	m := newTestModelWithContent("([{foo}])")
	m.cursorCol = 4 // cursor at 'o' in foo

	finder := &BracketTextObject{}
	start, end, found := finder.FindBounds(m, true) // inner

	assert.True(t, found)
	assert.Equal(t, Position{Row: 0, Col: 3}, start)
	assert.Equal(t, Position{Row: 0, Col: 5}, end)
	assert.Equal(t, "foo", extractText(m.content, start, end))
}

func TestBracketTextObject_FindBounds_CursorOutsideBrackets(t *testing.T) {
	m := newTestModelWithContent("foo (bar) baz")
	m.cursorCol = 1 // cursor at 'o' - outside brackets

	finder := &BracketTextObject{}
	_, _, found := finder.FindBounds(m, true)

	assert.False(t, found)
}

func TestBracketTextObject_FindBounds_EmptyLine(t *testing.T) {
	m := newTestModelWithContent("")
	m.cursorCol = 0

	finder := &BracketTextObject{}
	_, _, found := finder.FindBounds(m, true)

	assert.False(t, found)
}

func TestBracketTextObject_FindBounds_CursorBeyondLineLength(t *testing.T) {
	m := newTestModelWithContent("(foo)")
	m.cursorCol = 10 // beyond line length

	finder := &BracketTextObject{}
	_, _, found := finder.FindBounds(m, true)

	assert.False(t, found)
}

func TestBracketTextObject_FindBounds_InvalidRow(t *testing.T) {
	m := newTestModelWithContent("(foo)")
	m.cursorRow = 5 // invalid row

	finder := &BracketTextObject{}
	_, _, found := finder.FindBounds(m, true)

	assert.False(t, found)
}

// ============================================================================
// BracketTextObject Registry Tests
// ============================================================================

func TestTextObjectRegistry_BracketRegistered(t *testing.T) {
	finder, ok := textObjectRegistry['b']

	assert.True(t, ok)
	assert.NotNil(t, finder)

	// Verify it's the correct type
	_, isBracket := finder.(*BracketTextObject)
	assert.True(t, isBracket)
}
