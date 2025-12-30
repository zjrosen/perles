package vimtextarea

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test data covering all 6 Unicode edge case categories from the proposal:
// 1. ASCII baseline
// 2. Combining characters
// 3. Simple emoji
// 4. ZWJ sequences
// 5. Skin tone modifiers
// 6. Regional indicators (flags)
// 7. Variation selectors

var unicodeTestCases = []struct {
	name              string
	input             string
	expectedGraphemes int
	expectedBytes     int
	expectedDisplay   int
}{
	// Category 1: ASCII baseline
	{"ASCII hello", "hello", 5, 5, 5},
	{"ASCII numbers", "12345", 5, 5, 5},
	{"ASCII mixed", "hello world", 11, 11, 11},

	// Category 2: Combining characters
	// Note: "héllo" with e + combining acute (U+0301) = 5 graphemes
	// The combining acute accent combines with 'e' to form 1 grapheme
	{"combining accent", "he\u0301llo", 5, 6, 5},     // héllo with combining accent
	{"multiple combining", "e\u0301\u0327", 1, 4, 1}, // e + acute + cedilla = 1 grapheme

	// Category 3: Simple emoji
	{"simple emoji", "h😀llo", 5, 8, 6},  // 😀 is 4 bytes, runewidth reports as 2 columns but terminal-dependent
	{"emoji only", "😀", 1, 4, 2},        // Single emoji
	{"multiple emoji", "😀🎉🎊", 3, 12, 6}, // 3 emoji, each 4 bytes, 2 columns

	// Category 4: ZWJ sequences (Zero Width Joiner)
	// 👨‍👩‍👧‍👦 = Man + ZWJ + Woman + ZWJ + Girl + ZWJ + Boy = 1 grapheme, 25 bytes
	{"ZWJ family", "👨\u200d👩\u200d👧\u200d👦", 1, 25, 2},
	// 👨‍💻 = Man + ZWJ + Computer = 1 grapheme
	{"ZWJ technologist", "👨\u200d💻", 1, 11, 2},
	// Rainbow flag: 🏳️‍🌈
	// Note: runewidth reports this complex ZWJ as width=1 (terminal-dependent)
	{"ZWJ rainbow flag", "🏳\ufe0f\u200d🌈", 1, 14, 1},

	// Category 5: Skin tone modifiers
	// 👋🏽 = Waving hand + medium skin tone
	{"skin tone modifier", "👋🏽", 1, 8, 2},
	// 👩🏿 = Woman + dark skin tone
	{"skin tone woman", "👩🏿", 1, 8, 2},

	// Category 6: Regional indicators (flags)
	// 🇺🇸 = U+1F1FA (U) + U+1F1F8 (S) = 1 grapheme, 8 bytes
	// Note: runewidth reports flags as width=1 (terminal-dependent)
	{"flag US", "🇺🇸", 1, 8, 1},
	// 🇯🇵 = Japan flag
	{"flag Japan", "🇯🇵", 1, 8, 1},
	// Multiple flags
	{"multiple flags", "🇺🇸🇯🇵", 2, 16, 2},

	// Category 7: Variation selectors
	// ☀️ = Sun + variation selector-16 (emoji presentation)
	// Note: runewidth reports this as width=1 (terminal-dependent)
	{"variation selector emoji", "☀\ufe0f", 1, 6, 1},
	// ☀︎ = Sun + variation selector-15 (text presentation) - may render differently
	{"variation selector text", "☀\ufe0e", 1, 6, 1},

	// Mixed content
	{"mixed ASCII emoji", "café☕", 5, 8, 6},                            // 4 chars + coffee (2 cols)
	{"mixed content complex", "café☕🎉", 6, 12, 8},                      // café (5 bytes) + coffee (4) + party (4)
	{"text with ZWJ emoji", "Hello 👨\u200d👩\u200d👧\u200d👦!", 8, 32, 9}, // "Hello " (6) + family (2) + "!" (1)

	// Edge cases
	{"empty string", "", 0, 0, 0},
	{"single ASCII", "a", 1, 1, 1},
	{"single space", " ", 1, 1, 1},
	{"newline", "\n", 1, 1, 0},
	{"tab", "\t", 1, 1, 0},
}

func TestGraphemeCount(t *testing.T) {
	for _, tc := range unicodeTestCases {
		t.Run(tc.name, func(t *testing.T) {
			got := GraphemeCount(tc.input)
			assert.Equal(t, tc.expectedGraphemes, got, "GraphemeCount(%q)", tc.input)
		})
	}
}

func TestStringDisplayWidth(t *testing.T) {
	for _, tc := range unicodeTestCases {
		t.Run(tc.name, func(t *testing.T) {
			got := StringDisplayWidth(tc.input)
			assert.Equal(t, tc.expectedDisplay, got, "StringDisplayWidth(%q)", tc.input)
		})
	}
}

func TestGraphemeAt(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		idx      int
		expected string
	}{
		{"ASCII first", "hello", 0, "h"},
		{"ASCII middle", "hello", 2, "l"},
		{"ASCII last", "hello", 4, "o"},
		{"ASCII out of bounds", "hello", 5, ""},
		{"ASCII negative", "hello", -1, ""},

		{"emoji first", "😀hello", 0, "😀"},
		{"emoji after", "h😀llo", 1, "😀"},
		{"after emoji", "h😀llo", 2, "l"},

		{"ZWJ family", "👨\u200d👩\u200d👧\u200d👦", 0, "👨\u200d👩\u200d👧\u200d👦"},
		{"after ZWJ in text", "Hi👨\u200d👩\u200d👧\u200d👦!", 2, "👨\u200d👩\u200d👧\u200d👦"},
		{"exclaim after ZWJ", "Hi👨\u200d👩\u200d👧\u200d👦!", 3, "!"},

		{"flag", "🇺🇸", 0, "🇺🇸"},
		{"flag in text", "USA🇺🇸!", 3, "🇺🇸"},

		{"combining", "he\u0301llo", 1, "e\u0301"}, // é with combining accent
		{"after combining", "he\u0301llo", 2, "l"},

		{"empty", "", 0, ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := GraphemeAt(tc.input, tc.idx)
			assert.Equal(t, tc.expected, got, "GraphemeAt(%q, %d)", tc.input, tc.idx)
		})
	}
}

func TestNthGrapheme(t *testing.T) {
	tests := []struct {
		name            string
		input           string
		n               int
		expectedCluster string
		expectedOffset  int
	}{
		{"ASCII first", "hello", 0, "h", 0},
		{"ASCII middle", "hello", 2, "l", 2},
		{"ASCII last", "hello", 4, "o", 4},
		{"ASCII out of bounds", "hello", 5, "", -1},
		{"ASCII negative", "hello", -1, "", -1},

		{"emoji first", "😀hello", 0, "😀", 0},
		{"after emoji", "😀hello", 1, "h", 4}, // emoji is 4 bytes
		{"emoji middle", "h😀llo", 1, "😀", 1},
		{"after middle emoji", "h😀llo", 2, "l", 5}, // 'h' (1) + emoji (4) = 5

		{"ZWJ family only", "👨\u200d👩\u200d👧\u200d👦", 0, "👨\u200d👩\u200d👧\u200d👦", 0},
		{"after ZWJ", "a👨\u200d👩\u200d👧\u200d👦b", 2, "b", 26}, // 'a' (1) + family (25) = 26

		{"flag", "🇺🇸🇯🇵", 1, "🇯🇵", 8}, // First flag is 8 bytes

		{"empty", "", 0, "", -1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cluster, offset := NthGrapheme(tc.input, tc.n)
			assert.Equal(t, tc.expectedCluster, cluster, "NthGrapheme(%q, %d) cluster", tc.input, tc.n)
			assert.Equal(t, tc.expectedOffset, offset, "NthGrapheme(%q, %d) offset", tc.input, tc.n)
		})
	}
}

func TestGraphemeToByteOffset(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		graphemeIdx int
		expected    int
	}{
		{"ASCII start", "hello", 0, 0},
		{"ASCII middle", "hello", 2, 2},
		{"ASCII end", "hello", 5, 5},
		{"ASCII beyond", "hello", 10, 5},
		{"ASCII negative", "hello", -1, 0},

		{"emoji start", "😀hello", 0, 0},
		{"after emoji", "😀hello", 1, 4}, // emoji is 4 bytes
		{"emoji at end", "hello😀", 5, 5},
		{"emoji beyond", "hello😀", 6, 9},

		{"ZWJ start", "👨\u200d👩\u200d👧\u200d👦", 0, 0},
		{"ZWJ to second", "👨\u200d👩\u200d👧\u200d👦a", 1, 25}, // Family is 25 bytes
		{"after ZWJ", "a👨\u200d👩\u200d👧\u200d👦b", 2, 26},

		{"flag start", "🇺🇸🇯🇵", 0, 0},
		{"flag second", "🇺🇸🇯🇵", 1, 8},
		{"flag end", "🇺🇸🇯🇵", 2, 16},

		{"combining", "he\u0301llo", 2, 4}, // 'h' (1) + 'é' (3) = 4

		{"empty", "", 0, 0},
		{"empty beyond", "", 1, 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := GraphemeToByteOffset(tc.input, tc.graphemeIdx)
			assert.Equal(t, tc.expected, got, "GraphemeToByteOffset(%q, %d)", tc.input, tc.graphemeIdx)
		})
	}
}

func TestByteToGraphemeOffset(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		byteOffset int
		expected   int
	}{
		{"ASCII start", "hello", 0, 0},
		{"ASCII middle", "hello", 2, 2},
		{"ASCII end", "hello", 5, 5},
		{"ASCII beyond", "hello", 10, 5},
		{"ASCII negative", "hello", -1, 0},

		{"emoji start", "😀hello", 0, 0},
		{"emoji middle byte", "😀hello", 2, 0}, // Middle of emoji = still at grapheme 0
		{"emoji after", "😀hello", 4, 1},       // After emoji
		{"after first char post emoji", "😀hello", 5, 2},

		{"ZWJ start", "👨\u200d👩\u200d👧\u200d👦", 0, 0},
		{"ZWJ middle byte", "👨\u200d👩\u200d👧\u200d👦", 10, 0}, // Still in family
		{"ZWJ end", "👨\u200d👩\u200d👧\u200d👦", 25, 1},
		{"after ZWJ in text", "a👨\u200d👩\u200d👧\u200d👦b", 26, 2},

		{"flag start", "🇺🇸🇯🇵", 0, 0},
		{"flag middle byte", "🇺🇸🇯🇵", 4, 0}, // Middle of first flag
		{"flag second", "🇺🇸🇯🇵", 8, 1},
		{"flag end", "🇺🇸🇯🇵", 16, 2},

		{"combining start", "he\u0301llo", 0, 0},
		{"combining on e", "he\u0301llo", 1, 1},      // At 'e'
		{"combining on accent", "he\u0301llo", 2, 1}, // Still at 'é' grapheme
		{"combining after", "he\u0301llo", 4, 2},     // After 'é'

		{"empty", "", 0, 0},
		{"empty beyond", "", 5, 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ByteToGraphemeOffset(tc.input, tc.byteOffset)
			assert.Equal(t, tc.expected, got, "ByteToGraphemeOffset(%q, %d)", tc.input, tc.byteOffset)
		})
	}
}

func TestSliceByGraphemes(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		start    int
		end      int
		expected string
	}{
		{"ASCII full", "hello", 0, 5, "hello"},
		{"ASCII middle", "hello", 1, 4, "ell"},
		{"ASCII start only", "hello", 0, 2, "he"},
		{"ASCII end only", "hello", 3, 5, "lo"},
		{"ASCII single", "hello", 2, 3, "l"},
		{"ASCII empty range", "hello", 2, 2, ""},
		{"ASCII invalid range", "hello", 4, 2, ""},
		{"ASCII negative start", "hello", -1, 3, "hel"},
		{"ASCII beyond end", "hello", 3, 10, "lo"},

		{"emoji full", "h😀llo", 0, 5, "h😀llo"},
		{"emoji only", "h😀llo", 1, 2, "😀"},
		{"after emoji", "h😀llo", 2, 4, "ll"},
		{"include emoji", "h😀llo", 0, 3, "h😀l"},

		{"ZWJ only", "a👨\u200d👩\u200d👧\u200d👦b", 1, 2, "👨\u200d👩\u200d👧\u200d👦"},
		{"around ZWJ", "a👨\u200d👩\u200d👧\u200d👦b", 0, 3, "a👨\u200d👩\u200d👧\u200d👦b"},

		{"flag only", "🇺🇸🇯🇵", 0, 1, "🇺🇸"},
		{"flag second", "🇺🇸🇯🇵", 1, 2, "🇯🇵"},
		{"flag both", "🇺🇸🇯🇵", 0, 2, "🇺🇸🇯🇵"},

		{"combining include", "he\u0301llo", 0, 2, "he\u0301"},
		{"combining only", "he\u0301llo", 1, 2, "e\u0301"},
		{"after combining", "he\u0301llo", 2, 5, "llo"},

		{"empty", "", 0, 0, ""},
		{"empty beyond", "", 0, 5, ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := SliceByGraphemes(tc.input, tc.start, tc.end)
			assert.Equal(t, tc.expected, got, "SliceByGraphemes(%q, %d, %d)", tc.input, tc.start, tc.end)
		})
	}
}

func TestGraphemeDisplayWidth(t *testing.T) {
	tests := []struct {
		name     string
		cluster  string
		expected int
	}{
		{"ASCII char", "a", 1},
		{"ASCII space", " ", 1},
		{"ASCII newline", "\n", 0},
		{"ASCII tab", "\t", 0},

		{"simple emoji", "😀", 2},
		{"ZWJ family", "👨\u200d👩\u200d👧\u200d👦", 2},
		{"skin tone", "👋🏽", 2},
		{"flag", "🇺🇸", 1}, // Note: runewidth reports flags as width=1 (terminal-dependent)

		{"combining", "e\u0301", 1}, // é with combining = 1 column

		{"empty", "", 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := GraphemeDisplayWidth(tc.cluster)
			assert.Equal(t, tc.expected, got, "GraphemeDisplayWidth(%q)", tc.cluster)
		})
	}
}

func TestGraphemeType(t *testing.T) {
	tests := []struct {
		name     string
		cluster  string
		expected int
	}{
		// Whitespace
		{"space", " ", graphemeWhitespace},
		{"tab", "\t", graphemeWhitespace},
		{"newline", "\n", graphemeWhitespace},
		{"carriage return", "\r", graphemeWhitespace},

		// Word characters
		{"lowercase", "a", graphemeWord},
		{"uppercase", "Z", graphemeWord},
		{"digit", "5", graphemeWord},
		{"underscore", "_", graphemeWord},
		{"combining letter", "e\u0301", graphemeWord}, // é is a letter

		// Punctuation (including emoji)
		{"period", ".", graphemePunctuation},
		{"comma", ",", graphemePunctuation},
		{"emoji", "😀", graphemePunctuation},
		{"ZWJ family", "👨\u200d👩\u200d👧\u200d👦", graphemePunctuation},
		{"flag", "🇺🇸", graphemePunctuation},
		{"bracket", "[", graphemePunctuation},
		{"at sign", "@", graphemePunctuation},

		// Empty
		{"empty", "", graphemeWhitespace},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := graphemeType(tc.cluster)
			assert.Equal(t, tc.expected, got, "graphemeType(%q)", tc.cluster)
		})
	}
}

func TestGraphemeIterator(t *testing.T) {
	t.Run("ASCII", func(t *testing.T) {
		iter := NewGraphemeIterator("hello")
		var clusters []string
		var positions []int
		var indices []int

		for iter.Next() {
			clusters = append(clusters, iter.Cluster())
			positions = append(positions, iter.BytePos())
			indices = append(indices, iter.Index())
		}

		assert.Equal(t, []string{"h", "e", "l", "l", "o"}, clusters)
		assert.Equal(t, []int{0, 1, 2, 3, 4}, positions)
		assert.Equal(t, []int{0, 1, 2, 3, 4}, indices)
	})

	t.Run("with emoji", func(t *testing.T) {
		iter := NewGraphemeIterator("h😀llo")
		var clusters []string
		var positions []int

		for iter.Next() {
			clusters = append(clusters, iter.Cluster())
			positions = append(positions, iter.BytePos())
		}

		assert.Equal(t, []string{"h", "😀", "l", "l", "o"}, clusters)
		assert.Equal(t, []int{0, 1, 5, 6, 7}, positions)
	})

	t.Run("ZWJ family", func(t *testing.T) {
		family := "👨\u200d👩\u200d👧\u200d👦"
		iter := NewGraphemeIterator("a" + family + "b")
		var clusters []string
		var positions []int

		for iter.Next() {
			clusters = append(clusters, iter.Cluster())
			positions = append(positions, iter.BytePos())
		}

		assert.Equal(t, []string{"a", family, "b"}, clusters)
		assert.Equal(t, []int{0, 1, 26}, positions)
	})

	t.Run("empty string", func(t *testing.T) {
		iter := NewGraphemeIterator("")
		assert.False(t, iter.Next())
	})

	t.Run("Reset", func(t *testing.T) {
		iter := NewGraphemeIterator("ab")

		require.True(t, iter.Next())
		assert.Equal(t, "a", iter.Cluster())

		iter.Reset()

		require.True(t, iter.Next())
		assert.Equal(t, "a", iter.Cluster())
		assert.Equal(t, 0, iter.Index())
	})

	t.Run("Index before Next", func(t *testing.T) {
		iter := NewGraphemeIterator("test")
		assert.Equal(t, -1, iter.Index())
	})
}

func TestReverseGraphemeIterator(t *testing.T) {
	t.Run("ASCII", func(t *testing.T) {
		iter := NewReverseGraphemeIterator("hello")
		var clusters []string
		var indices []int

		for iter.Next() {
			clusters = append(clusters, iter.Cluster())
			indices = append(indices, iter.Index())
		}

		assert.Equal(t, []string{"o", "l", "l", "e", "h"}, clusters)
		assert.Equal(t, []int{4, 3, 2, 1, 0}, indices)
	})

	t.Run("with emoji", func(t *testing.T) {
		iter := NewReverseGraphemeIterator("h😀llo")
		var clusters []string
		var positions []int

		for iter.Next() {
			clusters = append(clusters, iter.Cluster())
			positions = append(positions, iter.BytePos())
		}

		assert.Equal(t, []string{"o", "l", "l", "😀", "h"}, clusters)
		assert.Equal(t, []int{7, 6, 5, 1, 0}, positions)
	})

	t.Run("ZWJ family", func(t *testing.T) {
		family := "👨\u200d👩\u200d👧\u200d👦"
		iter := NewReverseGraphemeIterator("a" + family + "b")
		var clusters []string

		for iter.Next() {
			clusters = append(clusters, iter.Cluster())
		}

		assert.Equal(t, []string{"b", family, "a"}, clusters)
	})

	t.Run("empty string", func(t *testing.T) {
		iter := NewReverseGraphemeIterator("")
		assert.False(t, iter.Next())
	})
}

func TestGraphemesInRange(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		start    int
		end      int
		expected []string
	}{
		{"ASCII full", "hello", 0, 5, []string{"h", "e", "l", "l", "o"}},
		{"ASCII middle", "hello", 1, 4, []string{"e", "l", "l"}},
		{"ASCII empty", "hello", 2, 2, nil},
		{"ASCII invalid", "hello", 4, 2, nil},

		{"with emoji", "h😀llo", 0, 3, []string{"h", "😀", "l"}},
		{"emoji only", "h😀llo", 1, 2, []string{"😀"}},

		{"ZWJ", "a👨\u200d👩\u200d👧\u200d👦b", 0, 3, []string{"a", "👨\u200d👩\u200d👧\u200d👦", "b"}},

		{"empty", "", 0, 5, nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := GraphemesInRange(tc.input, tc.start, tc.end)
			assert.Equal(t, tc.expected, got, "GraphemesInRange(%q, %d, %d)", tc.input, tc.start, tc.end)
		})
	}
}

func TestInsertAtGrapheme(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		graphemeIdx int
		insert      string
		expected    string
	}{
		{"ASCII start", "hello", 0, "X", "Xhello"},
		{"ASCII middle", "hello", 2, "X", "heXllo"},
		{"ASCII end", "hello", 5, "X", "helloX"},

		{"emoji start", "h😀llo", 0, "X", "Xh😀llo"},
		{"before emoji", "h😀llo", 1, "X", "hX😀llo"},
		{"after emoji", "h😀llo", 2, "X", "h😀Xllo"},

		{"insert emoji", "hello", 2, "😀", "he😀llo"},
		{"insert ZWJ", "ab", 1, "👨\u200d👩\u200d👧\u200d👦", "a👨\u200d👩\u200d👧\u200d👦b"},

		{"empty input", "", 0, "X", "X"},
		{"empty insert", "hello", 2, "", "hello"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := InsertAtGrapheme(tc.input, tc.graphemeIdx, tc.insert)
			assert.Equal(t, tc.expected, got, "InsertAtGrapheme(%q, %d, %q)", tc.input, tc.graphemeIdx, tc.insert)
		})
	}
}

func TestDeleteGraphemeRange(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		start    int
		end      int
		expected string
	}{
		{"ASCII middle", "hello", 1, 4, "ho"},
		{"ASCII start", "hello", 0, 2, "llo"},
		{"ASCII end", "hello", 3, 5, "hel"},
		{"ASCII all", "hello", 0, 5, ""},
		{"ASCII single", "hello", 2, 3, "helo"},

		{"delete emoji", "h😀llo", 1, 2, "hllo"},
		{"around emoji", "h😀llo", 0, 3, "lo"},

		{"delete ZWJ", "a👨\u200d👩\u200d👧\u200d👦b", 1, 2, "ab"},
		{"delete with ZWJ", "a👨\u200d👩\u200d👧\u200d👦b", 0, 2, "b"},

		{"empty range", "hello", 2, 2, "hello"},
		{"empty input", "", 0, 0, ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := DeleteGraphemeRange(tc.input, tc.start, tc.end)
			assert.Equal(t, tc.expected, got, "DeleteGraphemeRange(%q, %d, %d)", tc.input, tc.start, tc.end)
		})
	}
}

func TestTruncateToDisplayWidth(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxWidth int
		expected string
	}{
		{"ASCII fits", "hello", 10, "hello"},
		{"ASCII exact", "hello", 5, "hello"},
		{"ASCII truncate", "hello", 3, "hel"},
		{"ASCII zero", "hello", 0, ""},

		{"emoji fits", "h😀llo", 10, "h😀llo"},
		{"emoji truncate partial", "h😀llo", 3, "h😀"}, // emoji is 2 columns, so h(1) + 😀(2) = 3
		{"emoji no room", "h😀llo", 2, "h"},           // Not enough room for emoji
		{"emoji exact", "😀", 2, "😀"},
		{"emoji too wide", "😀", 1, ""}, // Emoji is 2 columns, can't fit in 1

		{"ZWJ fits", "👨\u200d👩\u200d👧\u200d👦", 2, "👨\u200d👩\u200d👧\u200d👦"},
		{"ZWJ no fit", "👨\u200d👩\u200d👧\u200d👦", 1, ""}, // ZWJ family is 2 columns

		{"mixed truncate", "hi😀bye", 5, "hi😀b"}, // h(1) + i(1) + 😀(2) + b(1) = 5

		{"empty", "", 5, ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := TruncateToDisplayWidth(tc.input, tc.maxWidth)
			assert.Equal(t, tc.expected, got, "TruncateToDisplayWidth(%q, %d)", tc.input, tc.maxWidth)
		})
	}
}

// Round-trip tests to verify bidirectional conversions work correctly
func TestRoundTripConversions(t *testing.T) {
	testStrings := []string{
		"hello",
		"h😀llo",
		"👨\u200d👩\u200d👧\u200d👦",
		"a👨\u200d👩\u200d👧\u200d👦b",
		"🇺🇸🇯🇵",
		"he\u0301llo",
		"café☕🎉",
		"",
	}

	for _, s := range testStrings {
		t.Run(s, func(t *testing.T) {
			graphemeCount := GraphemeCount(s)

			// For each grapheme index, verify round-trip
			for i := 0; i < graphemeCount; i++ {
				// grapheme -> byte -> grapheme
				byteOffset := GraphemeToByteOffset(s, i)
				backToGrapheme := ByteToGraphemeOffset(s, byteOffset)
				assert.Equal(t, i, backToGrapheme, "Round-trip grapheme %d in %q", i, s)
			}

			// Verify slicing preserves content
			if graphemeCount > 0 {
				// Full slice should equal original
				fullSlice := SliceByGraphemes(s, 0, graphemeCount)
				assert.Equal(t, s, fullSlice, "Full slice of %q", s)

				// Concatenating halves should equal original
				if graphemeCount > 1 {
					mid := graphemeCount / 2
					firstHalf := SliceByGraphemes(s, 0, mid)
					secondHalf := SliceByGraphemes(s, mid, graphemeCount)
					assert.Equal(t, s, firstHalf+secondHalf, "Concatenated halves of %q", s)
				}
			}
		})
	}
}

// Verify that NthGrapheme and GraphemeAt return consistent results
func TestConsistencyBetweenFunctions(t *testing.T) {
	testStrings := []string{
		"hello",
		"h😀llo",
		"👨\u200d👩\u200d👧\u200d👦",
		"🇺🇸test🇯🇵",
		"he\u0301llo",
	}

	for _, s := range testStrings {
		t.Run(s, func(t *testing.T) {
			graphemeCount := GraphemeCount(s)

			for i := 0; i < graphemeCount; i++ {
				// GraphemeAt and NthGrapheme should return the same cluster
				atResult := GraphemeAt(s, i)
				nthCluster, nthOffset := NthGrapheme(s, i)

				assert.Equal(t, atResult, nthCluster,
					"GraphemeAt and NthGrapheme cluster mismatch at index %d in %q", i, s)

				// The byte offset from NthGrapheme should match GraphemeToByteOffset
				expectedOffset := GraphemeToByteOffset(s, i)
				assert.Equal(t, expectedOffset, nthOffset,
					"NthGrapheme offset doesn't match GraphemeToByteOffset at index %d in %q", i, s)

				// SliceByGraphemes(s, i, i+1) should equal the grapheme
				sliced := SliceByGraphemes(s, i, i+1)
				assert.Equal(t, atResult, sliced,
					"SliceByGraphemes single grapheme mismatch at index %d in %q", i, s)
			}
		})
	}
}

// Test that the iterator produces the same results as the index-based functions
func TestIteratorConsistency(t *testing.T) {
	testStrings := []string{
		"hello",
		"h😀llo",
		"👨\u200d👩\u200d👧\u200d👦",
		"🇺🇸test🇯🇵",
		"",
	}

	for _, s := range testStrings {
		t.Run(s, func(t *testing.T) {
			iter := NewGraphemeIterator(s)
			idx := 0

			for iter.Next() {
				expectedCluster := GraphemeAt(s, idx)
				expectedBytePos := GraphemeToByteOffset(s, idx)

				assert.Equal(t, expectedCluster, iter.Cluster(),
					"Iterator cluster mismatch at index %d in %q", idx, s)
				assert.Equal(t, expectedBytePos, iter.BytePos(),
					"Iterator bytePos mismatch at index %d in %q", idx, s)
				assert.Equal(t, idx, iter.Index(),
					"Iterator index mismatch at index %d in %q", idx, s)

				idx++
			}

			assert.Equal(t, GraphemeCount(s), idx, "Iterator count mismatch for %q", s)
		})
	}
}

// Benchmark tests
func BenchmarkGraphemeCount(b *testing.B) {
	testCases := []struct {
		name  string
		input string
	}{
		{"ASCII short", "hello"},
		{"ASCII long", "Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua."},
		{"emoji mixed", "Hello 😀 World 🎉 Test 👨\u200d👩\u200d👧\u200d👦 End"},
		{"emoji heavy", "😀🎉🎊👋🏽🇺🇸👨\u200d👩\u200d👧\u200d👦😀🎉🎊👋🏽🇺🇸👨\u200d👩\u200d👧\u200d👦"},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				GraphemeCount(tc.input)
			}
		})
	}
}

func BenchmarkGraphemeIterator(b *testing.B) {
	input := "Hello 😀 World 🎉 Test 👨\u200d👩\u200d👧\u200d👦 End"

	b.Run("forward", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			iter := NewGraphemeIterator(input)
			for iter.Next() {
				_ = iter.Cluster()
			}
		}
	})

	b.Run("reverse", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			iter := NewReverseGraphemeIterator(input)
			for iter.Next() {
				_ = iter.Cluster()
			}
		}
	})
}

func BenchmarkSliceByGraphemes(b *testing.B) {
	input := "Hello 😀 World 🎉 Test 👨\u200d👩\u200d👧\u200d👦 End"
	count := GraphemeCount(input)

	for i := 0; i < b.N; i++ {
		SliceByGraphemes(input, 2, count-2)
	}
}

// =============================================================================
// Phase 2: Core Model Tests - clampCursorCol, totalCharCount, selection bounds
// =============================================================================

// TestClampCursorCol_WithEmoji verifies cursor clamping uses grapheme count
func TestClampCursorCol_WithEmoji(t *testing.T) {
	tests := []struct {
		name          string
		content       string
		initialCol    int
		expectedCol   int
		graphemeCount int // for documentation
	}{
		// ASCII baseline - behavior unchanged
		{"ASCII fits", "hello", 2, 2, 5},
		{"ASCII clamps", "hello", 10, 4, 5},
		{"ASCII at end", "hello", 4, 4, 5},
		{"ASCII empty", "", 5, 0, 0},

		// Simple emoji - cursor should clamp to grapheme index
		{"emoji line", "h😀llo", 2, 2, 5},    // 5 graphemes: h, 😀, l, l, o
		{"emoji clamps", "h😀llo", 10, 4, 5}, // max is 4 (5-1)
		{"emoji at max", "h😀llo", 4, 4, 5},
		{"emoji only", "😀", 0, 0, 1},
		{"emoji only clamps", "😀", 5, 0, 1}, // max is 0 (1-1)

		// ZWJ sequences - complex emoji as single grapheme
		{"ZWJ family", "👨\u200d👩\u200d👧\u200d👦", 0, 0, 1}, // single grapheme
		{"ZWJ family clamps", "👨\u200d👩\u200d👧\u200d👦", 5, 0, 1},
		{"text with ZWJ", "Hi👨\u200d👩\u200d👧\u200d👦!", 3, 3, 4}, // H, i, family, !

		// Flags
		{"flags", "🇺🇸🇯🇵", 1, 1, 2},        // two graphemes
		{"flags clamps", "🇺🇸🇯🇵", 5, 1, 2}, // max is 1

		// Combining characters
		{"combining", "he\u0301llo", 2, 2, 5}, // héllo = 5 graphemes
		{"combining clamps", "he\u0301llo", 10, 4, 5},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
			m.content = []string{tc.content}
			m.cursorRow = 0
			m.cursorCol = tc.initialCol

			m.clampCursorCol()

			assert.Equal(t, tc.expectedCol, m.cursorCol, "clampCursorCol with content %q, initial col %d", tc.content, tc.initialCol)

			// Verify grapheme count for documentation
			actualCount := GraphemeCount(tc.content)
			assert.Equal(t, tc.graphemeCount, actualCount, "expected grapheme count for %q", tc.content)
		})
	}
}

// TestTotalCharCount_WithEmoji verifies char counting uses grapheme count
func TestTotalCharCount_WithEmoji(t *testing.T) {
	tests := []struct {
		name     string
		content  []string
		expected int
	}{
		// ASCII baseline
		{"ASCII single line", []string{"hello"}, 5},
		{"ASCII multi line", []string{"hello", "world"}, 11}, // 5 + 1 (newline) + 5
		{"ASCII empty", []string{""}, 0},

		// Simple emoji - should count as 1 grapheme each
		{"emoji single", []string{"h😀llo"}, 5},      // h, 😀, l, l, o
		{"emoji multi", []string{"hi😀", "bye🎉"}, 8}, // 3 + 1 + 4
		{"emoji only", []string{"😀🎉🎊"}, 3},

		// ZWJ family - entire sequence is 1 grapheme
		{"ZWJ family", []string{"👨\u200d👩\u200d👧\u200d👦"}, 1},
		{"ZWJ in text", []string{"Hi👨\u200d👩\u200d👧\u200d👦!"}, 4},           // H, i, family, !
		{"ZWJ multi line", []string{"Hi👨\u200d👩\u200d👧\u200d👦", "Bye!"}, 8}, // H,i,family (3) + newline (1) + B,y,e,! (4) = 8

		// Flags - pair of regional indicators = 1 grapheme
		{"flags", []string{"🇺🇸🇯🇵"}, 2},
		{"flags in text", []string{"USA🇺🇸"}, 4}, // U, S, A, flag

		// Combining characters - base + combiner = 1 grapheme
		{"combining", []string{"café"}, 4},                // Note: precomposed café = 4 graphemes
		{"combining accents", []string{"he\u0301llo"}, 5}, // h, é (combining), l, l, o

		// Mixed complex content
		// "Hello 😀" = H,e,l,l,o, ,😀 = 7 graphemes
		// + newline = 1
		// "👨‍👩‍👧‍👦" = 1 grapheme
		// Total = 9
		{"mixed complex", []string{"Hello 😀", "👨\u200d👩\u200d👧\u200d👦"}, 9},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := New(Config{VimEnabled: true})
			m.content = tc.content

			got := m.totalCharCount()

			assert.Equal(t, tc.expected, got, "totalCharCount for content %v", tc.content)
		})
	}
}

// TestClampCursor_WithEmoji verifies general cursor clamping uses grapheme count
func TestClampCursor_WithEmoji(t *testing.T) {
	tests := []struct {
		name        string
		content     []string
		initialRow  int
		initialCol  int
		expectedRow int
		expectedCol int
	}{
		// Row clamping unchanged
		{"row clamps down", []string{"hello"}, 5, 0, 0, 0},
		{"row clamps up", []string{"hello"}, -1, 0, 0, 0},

		// Column clamping now uses grapheme count
		{"col clamps emoji", []string{"h😀llo"}, 0, 10, 0, 5}, // grapheme count = 5
		{"col fits emoji", []string{"h😀llo"}, 0, 3, 0, 3},
		{"col clamps ZWJ", []string{"👨\u200d👩\u200d👧\u200d👦"}, 0, 5, 0, 1}, // 1 grapheme
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := New(Config{VimEnabled: true})
			m.content = tc.content
			m.cursorRow = tc.initialRow
			m.cursorCol = tc.initialCol

			m.clampCursor()

			assert.Equal(t, tc.expectedRow, m.cursorRow, "clampCursor row")
			assert.Equal(t, tc.expectedCol, m.cursorCol, "clampCursor col")
		})
	}
}

// TestSelectionBounds_VisualLineMode_WithEmoji verifies line-wise selection uses grapheme count
func TestSelectionBounds_VisualLineMode_WithEmoji(t *testing.T) {
	tests := []struct {
		name             string
		content          []string
		anchorRow        int
		anchorCol        int
		cursorRow        int
		cursorCol        int
		expectedStartCol int
		expectedEndCol   int // grapheme count for the end row
	}{
		// ASCII baseline
		{"ASCII line", []string{"hello"}, 0, 0, 0, 2, 0, 5},

		// Emoji line - end col should be grapheme count
		{"emoji line", []string{"h😀llo"}, 0, 0, 0, 2, 0, 5},                   // 5 graphemes
		{"ZWJ line", []string{"Hi👨\u200d👩\u200d👧\u200d👦!"}, 0, 0, 0, 2, 0, 4}, // 4 graphemes

		// Multi-line with emoji
		{"multi emoji", []string{"hello", "h😀llo"}, 0, 2, 1, 1, 0, 5}, // end line has 5 graphemes
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := New(Config{VimEnabled: true})
			m.content = tc.content
			m.mode = ModeVisualLine
			m.visualAnchor = Position{Row: tc.anchorRow, Col: tc.anchorCol}
			m.cursorRow = tc.cursorRow
			m.cursorCol = tc.cursorCol

			start, end := m.SelectionBounds()

			assert.Equal(t, tc.expectedStartCol, start.Col, "SelectionBounds start.Col")
			assert.Equal(t, tc.expectedEndCol, end.Col, "SelectionBounds end.Col")
		})
	}
}

// TestSelectedText_WithEmoji verifies selected text extraction uses grapheme slicing
func TestSelectedText_WithEmoji(t *testing.T) {
	tests := []struct {
		name      string
		content   []string
		anchorRow int
		anchorCol int
		cursorRow int
		cursorCol int
		mode      Mode
		expected  string
	}{
		// ASCII baseline
		{"ASCII single", []string{"hello"}, 0, 1, 0, 3, ModeVisual, "ell"},

		// Emoji selection - should extract complete graphemes
		{"select emoji", []string{"h😀llo"}, 0, 1, 0, 1, ModeVisual, "😀"},
		{"around emoji", []string{"h😀llo"}, 0, 0, 0, 2, ModeVisual, "h😀l"},
		{"after emoji", []string{"h😀llo"}, 0, 2, 0, 4, ModeVisual, "llo"},

		// ZWJ family selection
		{"select ZWJ", []string{"Hi👨\u200d👩\u200d👧\u200d👦!"}, 0, 2, 0, 2, ModeVisual, "👨\u200d👩\u200d👧\u200d👦"},
		{"around ZWJ", []string{"Hi👨\u200d👩\u200d👧\u200d👦!"}, 0, 1, 0, 3, ModeVisual, "i👨\u200d👩\u200d👧\u200d👦!"},

		// Flag selection
		{"select flag", []string{"USA🇺🇸JP"}, 0, 3, 0, 3, ModeVisual, "🇺🇸"},
		{"flags", []string{"🇺🇸🇯🇵"}, 0, 0, 0, 1, ModeVisual, "🇺🇸🇯🇵"},

		// Combining characters
		{"combining", []string{"he\u0301llo"}, 0, 1, 0, 1, ModeVisual, "e\u0301"}, // é as combining

		// Multi-line with emoji
		{"multi line emoji", []string{"hi😀", "bye🎉"}, 0, 2, 1, 2, ModeVisual, "😀\nbye"},

		// Line-wise mode
		{"line-wise emoji", []string{"h😀llo"}, 0, 0, 0, 2, ModeVisualLine, "h😀llo"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := New(Config{VimEnabled: true})
			m.content = tc.content
			m.mode = tc.mode
			m.visualAnchor = Position{Row: tc.anchorRow, Col: tc.anchorCol}
			m.cursorRow = tc.cursorRow
			m.cursorCol = tc.cursorCol

			got := m.SelectedText()

			assert.Equal(t, tc.expected, got, "SelectedText for content %v", tc.content)
		})
	}
}

// TestGetSelectionRangeForRow_WithEmoji verifies row selection range uses grapheme count
func TestGetSelectionRangeForRow_WithEmoji(t *testing.T) {
	tests := []struct {
		name          string
		content       []string
		mode          Mode
		anchorRow     int
		anchorCol     int
		cursorRow     int
		cursorCol     int
		queryRow      int
		expectedStart int
		expectedEnd   int
		expectedIn    bool
	}{
		// ASCII baseline
		{"ASCII char-wise", []string{"hello"}, ModeVisual, 0, 1, 0, 3, 0, 1, 4, true},
		{"ASCII line-wise", []string{"hello"}, ModeVisualLine, 0, 0, 0, 2, 0, 0, 5, true},

		// Emoji char-wise
		{"emoji char", []string{"h😀llo"}, ModeVisual, 0, 1, 0, 3, 0, 1, 4, true},

		// Emoji line-wise - end should be grapheme count
		{"emoji line", []string{"h😀llo"}, ModeVisualLine, 0, 0, 0, 2, 0, 0, 5, true},
		{"ZWJ line", []string{"Hi👨\u200d👩\u200d👧\u200d👦!"}, ModeVisualLine, 0, 0, 0, 1, 0, 0, 4, true},

		// Row not in selection
		{"not in selection", []string{"hello", "world"}, ModeVisual, 0, 1, 0, 3, 1, 0, 0, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := New(Config{VimEnabled: true})
			m.content = tc.content
			m.mode = tc.mode
			m.visualAnchor = Position{Row: tc.anchorRow, Col: tc.anchorCol}
			m.cursorRow = tc.cursorRow
			m.cursorCol = tc.cursorCol

			startCol, endCol, inSelection := m.getSelectionRangeForRow(tc.queryRow)

			assert.Equal(t, tc.expectedIn, inSelection, "getSelectionRangeForRow inSelection")
			if inSelection {
				assert.Equal(t, tc.expectedStart, startCol, "getSelectionRangeForRow startCol")
				assert.Equal(t, tc.expectedEnd, endCol, "getSelectionRangeForRow endCol")
			}
		})
	}
}

// TestCharLimit_WithEmoji verifies CharLimit counts graphemes not bytes
func TestCharLimit_WithEmoji(t *testing.T) {
	// CharLimit is enforced through totalCharCount, so we test that relationship
	tests := []struct {
		name         string
		content      []string
		charLimit    int
		shouldAccept bool
	}{
		// ASCII baseline
		{"ASCII under limit", []string{"hello"}, 10, true},
		{"ASCII at limit", []string{"hello"}, 5, true},
		{"ASCII over limit", []string{"hello"}, 4, false},

		// Emoji - should count as 1 character each, not by bytes
		// "h😀llo" = 5 graphemes (h, 😀, l, l, o) but 8 bytes
		{"emoji under limit", []string{"h😀llo"}, 10, true},
		{"emoji at limit", []string{"h😀llo"}, 5, true},
		{"emoji over limit", []string{"h😀llo"}, 4, false},

		// ZWJ family - 1 grapheme but 25 bytes
		{"ZWJ single grapheme", []string{"👨\u200d👩\u200d👧\u200d👦"}, 1, true},
		{"ZWJ at limit 1", []string{"👨\u200d👩\u200d👧\u200d👦a"}, 2, true},     // family + 'a' = 2 graphemes
		{"ZWJ over limit 2", []string{"👨\u200d👩\u200d👧\u200d👦ab"}, 2, false}, // family + 'a' + 'b' = 3 graphemes, limit 2

		// This is the key test: "280 characters" should allow 280 visible characters
		// including emoji, not be limited by byte count
		{"twitter-like limit", []string{"Hello 😀"}, 7, true}, // 7 graphemes
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := New(Config{VimEnabled: true, CharLimit: tc.charLimit})
			m.content = tc.content

			charCount := m.totalCharCount()
			withinLimit := tc.charLimit == 0 || charCount <= tc.charLimit

			assert.Equal(t, tc.shouldAccept, withinLimit, "CharLimit enforcement: content %v with limit %d, count %d", tc.content, tc.charLimit, charCount)
		})
	}
}

// TestEmptyLineHandling_WithGraphemes verifies edge cases with empty lines
func TestEmptyLineHandling_WithGraphemes(t *testing.T) {
	t.Run("clampCursorCol on empty line", func(t *testing.T) {
		m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
		m.content = []string{""}
		m.cursorRow = 0
		m.cursorCol = 5

		m.clampCursorCol()

		assert.Equal(t, 0, m.cursorCol, "cursor should clamp to 0 on empty line")
	})

	t.Run("totalCharCount with empty lines", func(t *testing.T) {
		m := New(Config{VimEnabled: true})
		m.content = []string{"", "", ""}

		count := m.totalCharCount()

		// 3 empty lines with 2 newlines between them = 2
		assert.Equal(t, 2, count, "totalCharCount with empty lines")
	})

	t.Run("selection on empty line", func(t *testing.T) {
		m := New(Config{VimEnabled: true})
		m.content = []string{""}
		m.mode = ModeVisualLine
		m.visualAnchor = Position{Row: 0, Col: 0}
		m.cursorRow = 0
		m.cursorCol = 0

		start, end := m.SelectionBounds()

		assert.Equal(t, 0, start.Col, "start.Col on empty line")
		assert.Equal(t, 0, end.Col, "end.Col on empty line (grapheme count = 0)")
	})
}

// TestAsciiRegression verifies ASCII behavior is unchanged
func TestAsciiRegression(t *testing.T) {
	t.Run("clampCursorCol ASCII unchanged", func(t *testing.T) {
		m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
		m.content = []string{"hello world"}
		m.cursorRow = 0
		m.cursorCol = 15

		m.clampCursorCol()

		// "hello world" = 11 characters, max col = 10
		assert.Equal(t, 10, m.cursorCol, "ASCII clamp unchanged")
	})

	t.Run("totalCharCount ASCII unchanged", func(t *testing.T) {
		m := New(Config{VimEnabled: true})
		m.content = []string{"hello", "world"}

		count := m.totalCharCount()

		// 5 + 1 (newline) + 5 = 11
		assert.Equal(t, 11, count, "ASCII totalCharCount unchanged")
	})

	t.Run("SelectedText ASCII unchanged", func(t *testing.T) {
		m := New(Config{VimEnabled: true})
		m.content = []string{"hello world"}
		m.mode = ModeVisual
		m.visualAnchor = Position{Row: 0, Col: 0}
		m.cursorRow = 0
		m.cursorCol = 4

		text := m.SelectedText()

		assert.Equal(t, "hello", text, "ASCII selection unchanged")
	})
}

// ============================================================================
// Phase 5: Rendering Tests - wrapLine, cursor positioning, selection rendering
// ============================================================================

// TestWrapLine_WithEmoji verifies that wrapLine handles emoji correctly by grapheme
func TestWrapLine_WithEmoji(t *testing.T) {
	tests := []struct {
		name          string
		content       string
		width         int
		expectedLines int
		// For checking first segment doesn't split emoji
		checkFirstNotSplit bool
	}{
		{
			name:          "emoji fits in one line",
			content:       "hi😀👋",
			width:         10, // 2 + 2 + 2 = 6 display columns
			expectedLines: 1,
		},
		{
			name:          "emoji causes wrap",
			content:       "hi😀👋test",
			width:         6, // "hi😀" fits (2+2=4), "👋t" needs new line
			expectedLines: 2,
		},
		{
			name:               "ZWJ family doesn't split",
			content:            "a👨\u200d👩\u200d👧\u200d👦b", // family emoji is 2 columns
			width:              3,                          // "a" (1) + family (2) = 3 fits, "b" wraps
			expectedLines:      2,
			checkFirstNotSplit: true,
		},
		{
			name:          "flag emoji handles correctly",
			content:       "hello🇺🇸world",
			width:         10,
			expectedLines: 2, // "hello🇺🇸" is 5+2=7, "world" needs new line
		},
		{
			name:          "wide emoji at boundary",
			content:       "ab😀cd",
			width:         3, // "ab" fits (2), "😀" needs new line (2 columns), "cd" needs another
			expectedLines: 3,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := New(Config{})
			m.SetValue(tc.content)
			m.width = tc.width

			wrapped, _ := m.wrapLineWithInfo(tc.content)

			assert.Equal(t, tc.expectedLines, len(wrapped), "number of wrapped lines for %q with width %d", tc.content, tc.width)

			// Verify that joined wrapped lines equal original content
			joined := ""
			for _, line := range wrapped {
				joined += line
			}
			assert.Equal(t, tc.content, joined, "joined wrapped lines should equal original")

			if tc.checkFirstNotSplit {
				// Verify the ZWJ family emoji wasn't split
				assert.Contains(t, wrapped[0], "👨\u200d👩\u200d👧\u200d👦", "ZWJ sequence should not be split")
			}
		})
	}
}

// TestDisplayLinesForLine_WithEmoji verifies display line calculation with emoji
func TestDisplayLinesForLine_WithEmoji(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		width    int
		expected int
	}{
		{
			name:     "ASCII simple",
			content:  "hello",
			width:    10,
			expected: 1, // 5 columns < 10
		},
		{
			name:     "ASCII wraps",
			content:  "hello world",
			width:    5,
			expected: 3, // 11 columns, 5 per line = 3 lines
		},
		{
			name:     "emoji simple",
			content:  "hi😀", // 2 + 2 = 4 columns
			width:    10,
			expected: 1,
		},
		{
			name:     "emoji causes extra line",
			content:  "hi😀", // 2 + 2 = 4 columns
			width:    3,
			expected: 2, // "hi" (2), "😀" (2) = 2 lines
		},
		{
			name:     "ZWJ family",
			content:  "👨\u200d👩\u200d👧\u200d👦", // 2 columns despite 25 bytes
			width:    10,
			expected: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := New(Config{})
			m.SetValue(tc.content)
			m.width = tc.width

			result := m.displayLinesForLine(tc.content)

			assert.Equal(t, tc.expected, result, "displayLinesForLine(%q, width=%d)", tc.content, tc.width)
		})
	}
}

// TestCursorWrapLine_WithEmoji verifies cursor wrap line calculation with emoji
func TestCursorWrapLine_WithEmoji(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		width     int
		cursorCol int // grapheme index
		expected  int // expected wrap line (0-indexed)
	}{
		{
			name:      "cursor before emoji",
			content:   "hi😀world", // graphemes: h, i, 😀, w, o, r, l, d (8 graphemes)
			width:     5,
			cursorCol: 0, // on 'h'
			expected:  0,
		},
		{
			name:      "cursor on emoji",
			content:   "hi😀world",
			width:     5, // "hi😀" = 2+2 = 4 columns fits on line 0, "world" on line 1
			cursorCol: 2, // on 😀 (grapheme index 2)
			expected:  0, // 😀 at column 2 (display column 2), still in line 0
		},
		{
			name:      "cursor after emoji on next wrap",
			content:   "hi😀world",
			width:     5, // "hi😀" = 4 cols, "w" starts at col 4
			cursorCol: 3, // on 'w'
			expected:  0, // 'w' is at display col 4, 4/5 = 0
		},
		{
			name:      "cursor in second line with emoji",
			content:   "hi😀world",
			width:     5,
			cursorCol: 5, // on 'r', display col = 2+2+1+1 = 6, 6/5 = 1
			expected:  1,
		},
		{
			name:      "ZWJ family cursor position",
			content:   "a👨\u200d👩\u200d👧\u200d👦bc", // a (1), family (2), b, c = graphemes 0, 1, 2, 3
			width:     3,                           // "a" + family = 1 + 2 = 3, fits; "bc" wraps
			cursorCol: 2,                           // on 'b', display col = 1+2 = 3, 3/3 = 1
			expected:  1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := New(Config{})
			m.content = []string{tc.content}
			m.width = tc.width
			m.cursorRow = 0
			m.cursorCol = tc.cursorCol

			result := m.cursorWrapLine()

			assert.Equal(t, tc.expected, result, "cursorWrapLine() for cursor at grapheme %d", tc.cursorCol)
		})
	}
}

// TestRenderLineWithCursor_Emoji verifies cursor renders on correct grapheme
func TestRenderLineWithCursor_Emoji(t *testing.T) {
	tests := []struct {
		name           string
		content        string
		cursorCol      int
		containsCursor string // the grapheme that should have cursor
	}{
		{
			name:           "cursor on emoji",
			content:        "hi😀world",
			cursorCol:      2, // on 😀
			containsCursor: "😀",
		},
		{
			name:           "cursor before emoji",
			content:        "hi😀world",
			cursorCol:      1, // on 'i'
			containsCursor: "i",
		},
		{
			name:           "cursor after emoji",
			content:        "hi😀world",
			cursorCol:      3, // on 'w'
			containsCursor: "w",
		},
		{
			name:           "cursor on ZWJ family",
			content:        "a👨\u200d👩\u200d👧\u200d👦b",
			cursorCol:      1, // on family emoji
			containsCursor: "👨\u200d👩\u200d👧\u200d👦",
		},
		{
			name:           "cursor on flag emoji",
			content:        "hi🇺🇸bye",
			cursorCol:      2, // on flag
			containsCursor: "🇺🇸",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := New(Config{})
			m.content = []string{tc.content}

			result := m.renderLineWithCursor(tc.content, tc.cursorCol)

			// Check that cursor is on the expected grapheme
			assert.Contains(t, result, cursorOn+tc.containsCursor+cursorOff,
				"cursor should be on %q", tc.containsCursor)
		})
	}
}

// TestRenderWrappedLineWithSelection_Emoji verifies selection renders correctly with emoji
func TestRenderWrappedLineWithSelection_Emoji(t *testing.T) {
	tests := []struct {
		name                string
		content             string
		selectionStart      int  // grapheme index
		selectionEnd        int  // grapheme index (cursor position - cursorCol in visual mode)
		cursorCol           int  // where cursor is
		expectCursor        bool // expect cursor markers in output
		expectSelection     bool // expect selection markers in output
		expectedCursorOn    string
		expectedSelectionOn string
	}{
		{
			name:             "cursor on single emoji selection",
			content:          "hi😀bye",
			selectionStart:   2, // start on 😀
			selectionEnd:     2, // cursor on 😀
			cursorCol:        2,
			expectCursor:     true,
			expectSelection:  false, // cursor takes precedence over selection
			expectedCursorOn: "😀",
		},
		{
			name:                "select text before emoji",
			content:             "hi😀bye",
			selectionStart:      0,
			selectionEnd:        1, // cursor on 'i'
			cursorCol:           1,
			expectCursor:        true,
			expectSelection:     true, // 'h' should be selected, 'i' has cursor
			expectedSelectionOn: "h",
			expectedCursorOn:    "i",
		},
		{
			name:                "select text including emoji",
			content:             "hi😀bye",
			selectionStart:      0,
			selectionEnd:        3, // cursor on 'b' (after emoji)
			cursorCol:           3,
			expectCursor:        true,
			expectSelection:     true, // "hi😀" should be selected, 'b' has cursor
			expectedSelectionOn: "hi😀",
			expectedCursorOn:    "b",
		},
		{
			name:             "cursor on ZWJ family",
			content:          "a👨\u200d👩\u200d👧\u200d👦b",
			selectionStart:   1,
			selectionEnd:     1, // cursor on family emoji
			cursorCol:        1,
			expectCursor:     true,
			expectSelection:  false, // single selection with cursor = cursor only
			expectedCursorOn: "👨\u200d👩\u200d👧\u200d👦",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := New(Config{VimEnabled: true})
			m.content = []string{tc.content}
			m.width = 100 // Wide enough for no wrapping
			m.mode = ModeVisual
			m.visualAnchor = Position{Row: 0, Col: tc.selectionStart}
			m.cursorRow = 0
			m.cursorCol = tc.cursorCol
			m.Focus()

			result := m.renderWrappedLineWithSelection(tc.content, 0, 0, tc.cursorCol, true, 0)

			// Check cursor markers
			if tc.expectCursor {
				assert.Contains(t, result, cursorOn+tc.expectedCursorOn+cursorOff,
					"cursor should be on %q", tc.expectedCursorOn)
			}

			// Check selection markers
			if tc.expectSelection {
				assert.Contains(t, result, selectionOn, "should have selection on")
				assert.Contains(t, result, tc.expectedSelectionOn, "should contain selected text %q", tc.expectedSelectionOn)
			}
		})
	}
}
