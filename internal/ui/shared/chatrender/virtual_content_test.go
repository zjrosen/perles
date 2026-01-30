package chatrender

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChatLineType_String(t *testing.T) {
	tests := []struct {
		name     string
		lineType ChatLineType
		expected string
	}{
		{
			name:     "LineTypeRole",
			lineType: LineTypeRole,
			expected: "Role",
		},
		{
			name:     "LineTypeContent",
			lineType: LineTypeContent,
			expected: "Content",
		},
		{
			name:     "LineTypeToolCall",
			lineType: LineTypeToolCall,
			expected: "ToolCall",
		},
		{
			name:     "LineTypeBlank",
			lineType: LineTypeBlank,
			expected: "Blank",
		},
		{
			name:     "Unknown line type",
			lineType: ChatLineType(999),
			expected: "Unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.lineType.String()
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestChatVirtualLine_Fields(t *testing.T) {
	tests := []struct {
		name         string
		messageIndex int
		lineIndex    int
		lineType     ChatLineType
		plainText    string
	}{
		{
			name:         "Role line",
			messageIndex: 0,
			lineIndex:    0,
			lineType:     LineTypeRole,
			plainText:    "Coordinator",
		},
		{
			name:         "Content line",
			messageIndex: 1,
			lineIndex:    2,
			lineType:     LineTypeContent,
			plainText:    "This is message content.",
		},
		{
			name:         "Tool call line",
			messageIndex: 2,
			lineIndex:    0,
			lineType:     LineTypeToolCall,
			plainText:    "├╴ read_file",
		},
		{
			name:         "Blank line",
			messageIndex: 3,
			lineIndex:    0,
			lineType:     LineTypeBlank,
			plainText:    "",
		},
		{
			name:         "High indices",
			messageIndex: 10000,
			lineIndex:    500,
			lineType:     LineTypeContent,
			plainText:    "Deep content",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			line := ChatVirtualLine{
				MessageIndex: tt.messageIndex,
				LineIndex:    tt.lineIndex,
				LineType:     tt.lineType,
				PlainText:    tt.plainText,
			}

			require.Equal(t, tt.messageIndex, line.MessageIndex, "MessageIndex should match")
			require.Equal(t, tt.lineIndex, line.LineIndex, "LineIndex should match")
			require.Equal(t, tt.lineType, line.LineType, "LineType should match")
			require.Equal(t, tt.plainText, line.PlainText, "PlainText should match")
		})
	}
}

func TestChatVirtualContent_NewEmpty(t *testing.T) {
	t.Run("creates empty content with defaults", func(t *testing.T) {
		vc := NewChatVirtualContent()

		require.NotNil(t, vc, "NewChatVirtualContent should return non-nil")
		require.Equal(t, 0, vc.TotalLines(), "TotalLines should be 0 for empty content")
		require.Empty(t, vc.Lines(), "Lines should be empty")
		require.Empty(t, vc.PlainLines(), "PlainLines should be empty")
		require.Equal(t, 0, vc.Width(), "Width should be 0 initially")
		require.NotNil(t, vc.Cache(), "Cache should be initialized")
	})

	t.Run("cache has correct default limits", func(t *testing.T) {
		vc := NewChatVirtualContent()
		cache := vc.Cache()

		require.Equal(t, DefaultCacheMaxItems, cache.Capacity(), "Cache should have default capacity")
		require.Equal(t, int64(DefaultCacheMaxBytes), cache.MaxBytes(), "Cache should have default max bytes")
		require.Equal(t, 0, cache.Size(), "Cache should start empty")
		require.Equal(t, int64(0), cache.ByteSize(), "Cache byte size should be 0")
	})
}

func TestChatCacheKey_Equality(t *testing.T) {
	t.Run("equal keys are equal", func(t *testing.T) {
		key1 := chatCacheKey{messageIndex: 1, lineIndex: 2, width: 80}
		key2 := chatCacheKey{messageIndex: 1, lineIndex: 2, width: 80}

		require.Equal(t, key1, key2, "Keys with same values should be equal")
	})

	t.Run("different messageIndex makes keys unequal", func(t *testing.T) {
		key1 := chatCacheKey{messageIndex: 1, lineIndex: 2, width: 80}
		key2 := chatCacheKey{messageIndex: 2, lineIndex: 2, width: 80}

		require.NotEqual(t, key1, key2, "Keys with different messageIndex should not be equal")
	})

	t.Run("different lineIndex makes keys unequal", func(t *testing.T) {
		key1 := chatCacheKey{messageIndex: 1, lineIndex: 2, width: 80}
		key2 := chatCacheKey{messageIndex: 1, lineIndex: 3, width: 80}

		require.NotEqual(t, key1, key2, "Keys with different lineIndex should not be equal")
	})

	t.Run("different width makes keys unequal", func(t *testing.T) {
		key1 := chatCacheKey{messageIndex: 1, lineIndex: 2, width: 80}
		key2 := chatCacheKey{messageIndex: 1, lineIndex: 2, width: 100}

		require.NotEqual(t, key1, key2, "Keys with different width should not be equal")
	})

	t.Run("keys work as map keys", func(t *testing.T) {
		m := make(map[chatCacheKey]string)
		key1 := chatCacheKey{messageIndex: 1, lineIndex: 2, width: 80}
		key2 := chatCacheKey{messageIndex: 1, lineIndex: 2, width: 80}

		m[key1] = "value1"
		val, ok := m[key2]

		require.True(t, ok, "Equal key should find value in map")
		require.Equal(t, "value1", val, "Should retrieve correct value")
	})
}

func TestChatCache_MaxItems(t *testing.T) {
	t.Run("evicts oldest when capacity exceeded", func(t *testing.T) {
		// Create cache with small capacity for testing
		cache := newChatCache(3, DefaultCacheMaxBytes)

		// Add 3 items (fills capacity)
		cache.Put(chatCacheKey{messageIndex: 0, lineIndex: 0, width: 80}, "line0")
		cache.Put(chatCacheKey{messageIndex: 0, lineIndex: 1, width: 80}, "line1")
		cache.Put(chatCacheKey{messageIndex: 0, lineIndex: 2, width: 80}, "line2")

		require.Equal(t, 3, cache.Size(), "Cache should have 3 items")

		// Add 4th item - should evict line0 (oldest)
		cache.Put(chatCacheKey{messageIndex: 0, lineIndex: 3, width: 80}, "line3")

		require.Equal(t, 3, cache.Size(), "Cache should still have 3 items after eviction")

		// line0 should be evicted
		_, ok := cache.Get(chatCacheKey{messageIndex: 0, lineIndex: 0, width: 80})
		require.False(t, ok, "line0 should be evicted")

		// line1, line2, line3 should still be present
		val, ok := cache.Get(chatCacheKey{messageIndex: 0, lineIndex: 1, width: 80})
		require.True(t, ok, "line1 should still be present")
		require.Equal(t, "line1", val)

		val, ok = cache.Get(chatCacheKey{messageIndex: 0, lineIndex: 2, width: 80})
		require.True(t, ok, "line2 should still be present")
		require.Equal(t, "line2", val)

		val, ok = cache.Get(chatCacheKey{messageIndex: 0, lineIndex: 3, width: 80})
		require.True(t, ok, "line3 should still be present")
		require.Equal(t, "line3", val)
	})

	t.Run("LRU ordering preserved - access promotes item", func(t *testing.T) {
		cache := newChatCache(3, DefaultCacheMaxBytes)

		// Add 3 items: 0, 1, 2
		cache.Put(chatCacheKey{messageIndex: 0, lineIndex: 0, width: 80}, "line0")
		cache.Put(chatCacheKey{messageIndex: 0, lineIndex: 1, width: 80}, "line1")
		cache.Put(chatCacheKey{messageIndex: 0, lineIndex: 2, width: 80}, "line2")

		// Access line0 - promotes it to front (most recently used)
		_, _ = cache.Get(chatCacheKey{messageIndex: 0, lineIndex: 0, width: 80})

		// Add new item - should evict line1 (now oldest)
		cache.Put(chatCacheKey{messageIndex: 0, lineIndex: 3, width: 80}, "line3")

		// line1 should be evicted (was oldest after line0 was accessed)
		_, ok := cache.Get(chatCacheKey{messageIndex: 0, lineIndex: 1, width: 80})
		require.False(t, ok, "line1 should be evicted after line0 was promoted")

		// line0 should still be present (was promoted)
		_, ok = cache.Get(chatCacheKey{messageIndex: 0, lineIndex: 0, width: 80})
		require.True(t, ok, "line0 should still be present after promotion")
	})

	t.Run("update existing entry does not change count", func(t *testing.T) {
		cache := newChatCache(3, DefaultCacheMaxBytes)

		key := chatCacheKey{messageIndex: 0, lineIndex: 0, width: 80}
		cache.Put(key, "original")
		require.Equal(t, 1, cache.Size())

		cache.Put(key, "updated")
		require.Equal(t, 1, cache.Size(), "Size should remain 1 after update")

		val, ok := cache.Get(key)
		require.True(t, ok)
		require.Equal(t, "updated", val, "Value should be updated")
	})
}

func TestChatCache_MaxBytes(t *testing.T) {
	t.Run("evicts when byte limit exceeded", func(t *testing.T) {
		// Create cache with small byte limit
		// Each entry is ~40 bytes (24 for key fields + 16 overhead + string length)
		// Use 150 bytes max to allow ~3 entries of ~40 bytes each
		cache := newChatCache(1000, 150)

		// Add entries with small content
		cache.Put(chatCacheKey{messageIndex: 0, lineIndex: 0, width: 80}, "a") // ~41 bytes
		cache.Put(chatCacheKey{messageIndex: 0, lineIndex: 1, width: 80}, "b") // ~41 bytes
		cache.Put(chatCacheKey{messageIndex: 0, lineIndex: 2, width: 80}, "c") // ~41 bytes

		// Total should be ~123 bytes, under 150 limit
		require.Equal(t, 3, cache.Size())

		// Add a 4th entry - should evict oldest to stay under limit
		cache.Put(chatCacheKey{messageIndex: 0, lineIndex: 3, width: 80}, "d")

		// Should have evicted at least one entry
		require.LessOrEqual(t, cache.Size(), 3, "Should evict to stay under byte limit")

		// The newest entry should be present
		_, ok := cache.Get(chatCacheKey{messageIndex: 0, lineIndex: 3, width: 80})
		require.True(t, ok, "Newest entry should be present")
	})

	t.Run("large entry triggers multiple evictions", func(t *testing.T) {
		// Use a larger byte limit to ensure we can fit multiple small entries
		// Each small entry is ~41 bytes (24 key + 16 overhead + 1 char)
		// 500 bytes allows ~12 small entries
		cache := newChatCache(1000, 500)

		// Add several small entries
		for i := 0; i < 10; i++ {
			cache.Put(chatCacheKey{messageIndex: 0, lineIndex: i, width: 80}, "x")
		}

		initialSize := cache.Size()
		require.Equal(t, 10, initialSize)

		// Add one large entry that exceeds remaining space
		// ~300 bytes total entry size (24 + 16 + 250)
		largeContent := strings.Repeat("x", 250)
		cache.Put(chatCacheKey{messageIndex: 0, lineIndex: 100, width: 80}, largeContent)

		// Should have evicted multiple small entries to make room
		require.Less(t, cache.Size(), initialSize, "Should have evicted entries")

		// Large entry should be present
		val, ok := cache.Get(chatCacheKey{messageIndex: 0, lineIndex: 100, width: 80})
		require.True(t, ok, "Large entry should be present")
		require.Equal(t, largeContent, val)
	})

	t.Run("ByteSize tracks memory correctly", func(t *testing.T) {
		cache := newChatCache(1000, DefaultCacheMaxBytes)

		require.Equal(t, int64(0), cache.ByteSize(), "Empty cache should have 0 bytes")

		cache.Put(chatCacheKey{messageIndex: 0, lineIndex: 0, width: 80}, "test")
		firstSize := cache.ByteSize()
		require.Greater(t, firstSize, int64(0), "Cache should track byte size")

		cache.Put(chatCacheKey{messageIndex: 0, lineIndex: 1, width: 80}, "test2")
		require.Greater(t, cache.ByteSize(), firstSize, "Byte size should increase")

		cache.Clear()
		require.Equal(t, int64(0), cache.ByteSize(), "Cleared cache should have 0 bytes")
	})
}

func TestChatCache_Clear(t *testing.T) {
	cache := newChatCache(100, DefaultCacheMaxBytes)

	// Add some entries
	for i := 0; i < 10; i++ {
		cache.Put(chatCacheKey{messageIndex: 0, lineIndex: i, width: 80}, "content")
	}

	require.Equal(t, 10, cache.Size())
	require.Greater(t, cache.ByteSize(), int64(0))

	cache.Clear()

	require.Equal(t, 0, cache.Size(), "Cache should be empty after clear")
	require.Equal(t, int64(0), cache.ByteSize(), "Byte size should be 0 after clear")

	// Should be able to add new entries after clear
	cache.Put(chatCacheKey{messageIndex: 0, lineIndex: 0, width: 80}, "new")
	require.Equal(t, 1, cache.Size())
}

func TestChatCache_GetMiss(t *testing.T) {
	cache := newChatCache(100, DefaultCacheMaxBytes)

	// Get on empty cache
	val, ok := cache.Get(chatCacheKey{messageIndex: 0, lineIndex: 0, width: 80})
	require.False(t, ok, "Get should return false for missing key")
	require.Equal(t, "", val, "Get should return empty string for missing key")

	// Add one entry and miss on different key
	cache.Put(chatCacheKey{messageIndex: 0, lineIndex: 0, width: 80}, "exists")

	val, ok = cache.Get(chatCacheKey{messageIndex: 0, lineIndex: 1, width: 80})
	require.False(t, ok, "Get should return false for different lineIndex")

	val, ok = cache.Get(chatCacheKey{messageIndex: 1, lineIndex: 0, width: 80})
	require.False(t, ok, "Get should return false for different messageIndex")

	val, ok = cache.Get(chatCacheKey{messageIndex: 0, lineIndex: 0, width: 100})
	require.False(t, ok, "Get should return false for different width")
}

// ============================================================================
// BuildLines Tests - Task 2 of Virtual Scrolling Epic
// ============================================================================

func TestNewChatVirtualContent_EmptyMessages(t *testing.T) {
	t.Run("empty messages array produces zero lines", func(t *testing.T) {
		cfg := RenderConfig{
			AgentLabel: "Coordinator",
		}
		vc := NewChatVirtualContentWithMessages([]Message{}, 80, cfg)

		require.Equal(t, 0, vc.TotalLines(), "TotalLines should be 0 for empty messages")
		require.Empty(t, vc.Lines(), "Lines should be empty")
		require.Empty(t, vc.PlainLines(), "PlainLines should be empty")
		require.Equal(t, 80, vc.Width(), "Width should be set")
	})

	t.Run("nil messages array produces zero lines", func(t *testing.T) {
		cfg := RenderConfig{
			AgentLabel: "Coordinator",
		}
		vc := NewChatVirtualContentWithMessages(nil, 80, cfg)

		require.Equal(t, 0, vc.TotalLines(), "TotalLines should be 0 for nil messages")
		require.Empty(t, vc.Lines(), "Lines should be empty")
		require.Empty(t, vc.PlainLines(), "PlainLines should be empty")
	})
}

func TestBuildLines_SingleMessage(t *testing.T) {
	t.Run("user message produces role + content + blank", func(t *testing.T) {
		cfg := RenderConfig{
			AgentLabel: "Coordinator",
		}
		messages := []Message{
			{Role: "user", Content: "Hello world"},
		}
		vc := NewChatVirtualContentWithMessages(messages, 80, cfg)

		lines := vc.Lines()
		require.Len(t, lines, 3, "Should have 3 lines: role + content + blank")

		// Role line
		require.Equal(t, LineTypeRole, lines[0].LineType)
		require.Equal(t, "You", lines[0].PlainText)
		require.Equal(t, 0, lines[0].MessageIndex)
		require.Equal(t, 0, lines[0].LineIndex)

		// Content line
		require.Equal(t, LineTypeContent, lines[1].LineType)
		require.Equal(t, "Hello world", lines[1].PlainText)
		require.Equal(t, 0, lines[1].MessageIndex)
		require.Equal(t, 1, lines[1].LineIndex)

		// Blank line
		require.Equal(t, LineTypeBlank, lines[2].LineType)
		require.Equal(t, "", lines[2].PlainText)
		require.Equal(t, 0, lines[2].MessageIndex)
		require.Equal(t, 2, lines[2].LineIndex)
	})

	t.Run("assistant message uses AgentLabel", func(t *testing.T) {
		cfg := RenderConfig{
			AgentLabel: "TestAssistant",
		}
		messages := []Message{
			{Role: "assistant", Content: "I can help with that."},
		}
		vc := NewChatVirtualContentWithMessages(messages, 80, cfg)

		lines := vc.Lines()
		require.Len(t, lines, 3, "Should have 3 lines: role + content + blank")

		// Role line should use AgentLabel
		require.Equal(t, LineTypeRole, lines[0].LineType)
		require.Equal(t, "TestAssistant", lines[0].PlainText)

		// Content line
		require.Equal(t, LineTypeContent, lines[1].LineType)
		require.Equal(t, "I can help with that.", lines[1].PlainText)
	})

	t.Run("system message produces System role label", func(t *testing.T) {
		cfg := RenderConfig{
			AgentLabel: "Coordinator",
		}
		messages := []Message{
			{Role: "system", Content: "System notice"},
		}
		vc := NewChatVirtualContentWithMessages(messages, 80, cfg)

		lines := vc.Lines()
		require.Equal(t, LineTypeRole, lines[0].LineType)
		require.Equal(t, "System", lines[0].PlainText)
	})

	t.Run("coordinator message in worker pane shows Coordinator label", func(t *testing.T) {
		cfg := RenderConfig{
			AgentLabel:              "Worker",
			ShowCoordinatorInWorker: true,
		}
		messages := []Message{
			{Role: "coordinator", Content: "Task assigned"},
		}
		vc := NewChatVirtualContentWithMessages(messages, 80, cfg)

		lines := vc.Lines()
		require.Equal(t, LineTypeRole, lines[0].LineType)
		require.Equal(t, "Coordinator", lines[0].PlainText)
	})

	t.Run("custom UserLabel is used", func(t *testing.T) {
		cfg := RenderConfig{
			AgentLabel: "Coordinator",
			UserLabel:  "Developer",
		}
		messages := []Message{
			{Role: "user", Content: "Test"},
		}
		vc := NewChatVirtualContentWithMessages(messages, 80, cfg)

		lines := vc.Lines()
		require.Equal(t, LineTypeRole, lines[0].LineType)
		require.Equal(t, "Developer", lines[0].PlainText)
	})
}

func TestBuildLines_MultipleMessages(t *testing.T) {
	t.Run("multiple messages produce correct line sequence", func(t *testing.T) {
		cfg := RenderConfig{
			AgentLabel: "Coordinator",
		}
		messages := []Message{
			{Role: "user", Content: "Hello"},
			{Role: "assistant", Content: "Hi there"},
			{Role: "user", Content: "Thanks"},
		}
		vc := NewChatVirtualContentWithMessages(messages, 80, cfg)

		lines := vc.Lines()
		// Each message: role + content + blank = 3 lines
		// 3 messages * 3 lines = 9 lines
		require.Len(t, lines, 9, "Should have 9 lines (3 messages * 3 lines each)")

		// Verify message indices
		require.Equal(t, 0, lines[0].MessageIndex, "First 3 lines belong to message 0")
		require.Equal(t, 0, lines[1].MessageIndex)
		require.Equal(t, 0, lines[2].MessageIndex)

		require.Equal(t, 1, lines[3].MessageIndex, "Next 3 lines belong to message 1")
		require.Equal(t, 1, lines[4].MessageIndex)
		require.Equal(t, 1, lines[5].MessageIndex)

		require.Equal(t, 2, lines[6].MessageIndex, "Last 3 lines belong to message 2")
		require.Equal(t, 2, lines[7].MessageIndex)
		require.Equal(t, 2, lines[8].MessageIndex)

		// Verify roles
		require.Equal(t, "You", lines[0].PlainText)
		require.Equal(t, "Coordinator", lines[3].PlainText)
		require.Equal(t, "You", lines[6].PlainText)

		// Verify contents
		require.Equal(t, "Hello", lines[1].PlainText)
		require.Equal(t, "Hi there", lines[4].PlainText)
		require.Equal(t, "Thanks", lines[7].PlainText)
	})

	t.Run("TotalLines matches actual line count", func(t *testing.T) {
		cfg := RenderConfig{AgentLabel: "Coordinator"}
		messages := []Message{
			{Role: "user", Content: "One"},
			{Role: "assistant", Content: "Two"},
		}
		vc := NewChatVirtualContentWithMessages(messages, 80, cfg)

		require.Equal(t, len(vc.Lines()), vc.TotalLines())
	})
}

func TestBuildLines_WordWrap(t *testing.T) {
	t.Run("long content wraps at width boundary", func(t *testing.T) {
		cfg := RenderConfig{AgentLabel: "Coordinator"}
		// Content that exceeds narrow width
		longContent := "This is a message that should definitely wrap when the width is narrow enough to require it"
		messages := []Message{
			{Role: "user", Content: longContent},
		}

		// Very narrow width forces wrapping
		vc := NewChatVirtualContentWithMessages(messages, 30, cfg)

		lines := vc.Lines()
		// Should have: role + multiple wrapped lines + blank
		require.Greater(t, len(lines), 3, "Long content should produce multiple content lines")

		// First line is role
		require.Equal(t, LineTypeRole, lines[0].LineType)
		require.Equal(t, "You", lines[0].PlainText)

		// Middle lines are content (wrapped)
		for i := 1; i < len(lines)-1; i++ {
			require.Equal(t, LineTypeContent, lines[i].LineType, "Line %d should be content", i)
		}

		// Last line is blank
		require.Equal(t, LineTypeBlank, lines[len(lines)-1].LineType)
	})

	t.Run("content exactly at width doesn't wrap unnecessarily", func(t *testing.T) {
		cfg := RenderConfig{AgentLabel: "Coordinator"}
		// Content that fits in one line
		shortContent := "Short"
		messages := []Message{
			{Role: "user", Content: shortContent},
		}

		vc := NewChatVirtualContentWithMessages(messages, 80, cfg)

		lines := vc.Lines()
		require.Len(t, lines, 3, "Short content should produce exactly 3 lines")
		require.Equal(t, "Short", lines[1].PlainText)
	})

	t.Run("multiline content with explicit newlines", func(t *testing.T) {
		cfg := RenderConfig{AgentLabel: "Coordinator"}
		multilineContent := "Line one\nLine two\nLine three"
		messages := []Message{
			{Role: "user", Content: multilineContent},
		}

		vc := NewChatVirtualContentWithMessages(messages, 80, cfg)

		lines := vc.Lines()
		// role + 3 content lines + blank = 5 lines
		require.Len(t, lines, 5, "Should have 5 lines for 3-line content")

		require.Equal(t, "Line one", lines[1].PlainText)
		require.Equal(t, "Line two", lines[2].PlainText)
		require.Equal(t, "Line three", lines[3].PlainText)
	})
}

func TestBuildLines_ToolCallSequence(t *testing.T) {
	t.Run("single tool call uses ╰╴ prefix (first AND last)", func(t *testing.T) {
		cfg := RenderConfig{AgentLabel: "Coordinator"}
		messages := []Message{
			{Role: "assistant", Content: "read_file", IsToolCall: true},
		}

		vc := NewChatVirtualContentWithMessages(messages, 80, cfg)

		lines := vc.Lines()
		// role + tool call + blank = 3 lines
		require.Len(t, lines, 3)

		require.Equal(t, LineTypeRole, lines[0].LineType)
		require.Equal(t, "Coordinator", lines[0].PlainText)

		require.Equal(t, LineTypeToolCall, lines[1].LineType)
		require.Equal(t, "╰╴ read_file", lines[1].PlainText, "Single tool should use ╰╴")

		require.Equal(t, LineTypeBlank, lines[2].LineType)
	})

	t.Run("two tool calls use ├╴ then ╰╴ prefixes", func(t *testing.T) {
		cfg := RenderConfig{AgentLabel: "Coordinator"}
		messages := []Message{
			{Role: "assistant", Content: "read_file", IsToolCall: true},
			{Role: "assistant", Content: "write_file", IsToolCall: true},
		}

		vc := NewChatVirtualContentWithMessages(messages, 80, cfg)

		lines := vc.Lines()
		// role + tool1 + tool2 + blank = 4 lines
		// (second tool doesn't add role because it continues sequence)
		require.Len(t, lines, 4)

		require.Equal(t, LineTypeRole, lines[0].LineType)
		require.Equal(t, "Coordinator", lines[0].PlainText)

		require.Equal(t, LineTypeToolCall, lines[1].LineType)
		require.Equal(t, "├╴ read_file", lines[1].PlainText, "First in sequence uses ├╴")

		require.Equal(t, LineTypeToolCall, lines[2].LineType)
		require.Equal(t, "╰╴ write_file", lines[2].PlainText, "Last in sequence uses ╰╴")

		require.Equal(t, LineTypeBlank, lines[3].LineType)
	})

	t.Run("three tool calls: ├╴ ├╴ ╰╴ prefixes", func(t *testing.T) {
		cfg := RenderConfig{AgentLabel: "Coordinator"}
		messages := []Message{
			{Role: "assistant", Content: "tool1", IsToolCall: true},
			{Role: "assistant", Content: "tool2", IsToolCall: true},
			{Role: "assistant", Content: "tool3", IsToolCall: true},
		}

		vc := NewChatVirtualContentWithMessages(messages, 80, cfg)

		lines := vc.Lines()
		require.Len(t, lines, 5) // role + 3 tools + blank

		require.Equal(t, "├╴ tool1", lines[1].PlainText)
		require.Equal(t, "├╴ tool2", lines[2].PlainText)
		require.Equal(t, "╰╴ tool3", lines[3].PlainText)
	})

	t.Run("tool call sequence interrupted by text message", func(t *testing.T) {
		cfg := RenderConfig{AgentLabel: "Coordinator"}
		messages := []Message{
			{Role: "assistant", Content: "tool1", IsToolCall: true},
			{Role: "assistant", Content: "Regular text"},
			{Role: "assistant", Content: "tool2", IsToolCall: true},
		}

		vc := NewChatVirtualContentWithMessages(messages, 80, cfg)

		lines := vc.Lines()
		// First sequence: role + tool1 (╰╴ because it's also last) + blank
		// Text message: role + content + blank
		// Second sequence: role + tool2 (╰╴) + blank
		// Total: 3 + 3 + 3 = 9 lines

		// Find tool call lines
		var toolLines []ChatVirtualLine
		for _, l := range lines {
			if l.LineType == LineTypeToolCall {
				toolLines = append(toolLines, l)
			}
		}

		require.Len(t, toolLines, 2)
		require.Equal(t, "╰╴ tool1", toolLines[0].PlainText, "First tool is alone so uses ╰╴")
		require.Equal(t, "╰╴ tool2", toolLines[1].PlainText, "Second tool is alone so uses ╰╴")
	})

	t.Run("tool call with emoji prefix is stripped", func(t *testing.T) {
		cfg := RenderConfig{AgentLabel: "Coordinator"}
		messages := []Message{
			{Role: "assistant", Content: "🔧 read_file", IsToolCall: true},
		}

		vc := NewChatVirtualContentWithMessages(messages, 80, cfg)

		lines := vc.Lines()
		require.Equal(t, "╰╴ read_file", lines[1].PlainText, "Emoji prefix should be stripped")
	})
}

func TestBuildLines_PlainLinesSync(t *testing.T) {
	t.Run("plainLines matches lines 1:1", func(t *testing.T) {
		cfg := RenderConfig{AgentLabel: "Coordinator"}
		messages := []Message{
			{Role: "user", Content: "Hello"},
			{Role: "assistant", Content: "Hi", IsToolCall: false},
			{Role: "assistant", Content: "tool1", IsToolCall: true},
		}

		vc := NewChatVirtualContentWithMessages(messages, 80, cfg)

		lines := vc.Lines()
		plainLines := vc.PlainLines()

		require.Equal(t, len(lines), len(plainLines), "lines and plainLines should have same length")

		// Each plainLine should match the corresponding line's PlainText
		for i, line := range lines {
			require.Equal(t, line.PlainText, plainLines[i], "plainLines[%d] should match lines[%d].PlainText", i, i)
		}
	})

	t.Run("plainLines preserved after multiple messages", func(t *testing.T) {
		cfg := RenderConfig{AgentLabel: "Coordinator"}
		messages := []Message{
			{Role: "user", Content: "Line one\nLine two"},
			{Role: "assistant", Content: "Response"},
		}

		vc := NewChatVirtualContentWithMessages(messages, 80, cfg)

		plainLines := vc.PlainLines()

		// Verify content is correct
		require.Contains(t, plainLines, "You")
		require.Contains(t, plainLines, "Line one")
		require.Contains(t, plainLines, "Line two")
		require.Contains(t, plainLines, "Coordinator")
		require.Contains(t, plainLines, "Response")
	})
}

func TestBuildLines_MessageWithNewlinesOnly(t *testing.T) {
	t.Run("content with only newlines produces empty content line", func(t *testing.T) {
		cfg := RenderConfig{AgentLabel: "Coordinator"}
		messages := []Message{
			{Role: "user", Content: "\n\n\n"},
		}

		vc := NewChatVirtualContentWithMessages(messages, 80, cfg)

		lines := vc.Lines()
		// role + content (empty) + blank
		require.GreaterOrEqual(t, len(lines), 3)

		require.Equal(t, LineTypeRole, lines[0].LineType)
		require.Equal(t, "You", lines[0].PlainText)

		// Content line should exist (may be empty)
		require.Equal(t, LineTypeContent, lines[1].LineType)

		// Last line is blank
		require.Equal(t, LineTypeBlank, lines[len(lines)-1].LineType)
	})

	t.Run("content with only spaces produces empty content line", func(t *testing.T) {
		cfg := RenderConfig{AgentLabel: "Coordinator"}
		messages := []Message{
			{Role: "user", Content: "     "},
		}

		vc := NewChatVirtualContentWithMessages(messages, 80, cfg)

		lines := vc.Lines()
		require.GreaterOrEqual(t, len(lines), 3)

		// Content line exists
		require.Equal(t, LineTypeContent, lines[1].LineType)
	})

	t.Run("empty content produces empty content line", func(t *testing.T) {
		cfg := RenderConfig{AgentLabel: "Coordinator"}
		messages := []Message{
			{Role: "user", Content: ""},
		}

		vc := NewChatVirtualContentWithMessages(messages, 80, cfg)

		lines := vc.Lines()
		// role + empty content + blank
		require.Len(t, lines, 3)

		require.Equal(t, LineTypeRole, lines[0].LineType)
		require.Equal(t, LineTypeContent, lines[1].LineType)
		require.Equal(t, "", lines[1].PlainText)
		require.Equal(t, LineTypeBlank, lines[2].LineType)
	})
}

func TestBuildLines_VeryLongLine(t *testing.T) {
	t.Run("extremely long single line wraps correctly", func(t *testing.T) {
		cfg := RenderConfig{AgentLabel: "Coordinator"}
		// Create a very long word that exceeds width
		longWord := strings.Repeat("x", 200)
		messages := []Message{
			{Role: "user", Content: longWord},
		}

		vc := NewChatVirtualContentWithMessages(messages, 80, cfg)

		lines := vc.Lines()
		// Should still produce valid output
		require.Greater(t, len(lines), 2, "Should produce multiple lines")
		require.Equal(t, LineTypeRole, lines[0].LineType)
		require.Equal(t, LineTypeBlank, lines[len(lines)-1].LineType)
	})

	t.Run("mixed very long and short content", func(t *testing.T) {
		cfg := RenderConfig{AgentLabel: "Coordinator"}
		longContent := strings.Repeat("word ", 100) // 500 chars of words
		messages := []Message{
			{Role: "user", Content: "Short"},
			{Role: "assistant", Content: longContent},
			{Role: "user", Content: "Another short"},
		}

		vc := NewChatVirtualContentWithMessages(messages, 60, cfg)

		lines := vc.Lines()
		require.Greater(t, len(lines), 9, "Long content should produce many wrapped lines")

		// First message should be normal
		require.Equal(t, "You", lines[0].PlainText)
		require.Equal(t, "Short", lines[1].PlainText)

		// Verify all lines have valid structure
		for i, line := range lines {
			require.True(t, line.LineType >= LineTypeRole && line.LineType <= LineTypeBlank,
				"Line %d should have valid LineType", i)
		}
	})

	t.Run("width of 1 still produces valid output", func(t *testing.T) {
		cfg := RenderConfig{AgentLabel: "Coordinator"}
		messages := []Message{
			{Role: "user", Content: "Hello world"},
		}

		vc := NewChatVirtualContentWithMessages(messages, 1, cfg)

		lines := vc.Lines()
		require.Greater(t, len(lines), 0, "Should produce output even with width=1")
		require.Equal(t, LineTypeRole, lines[0].LineType)
	})

	t.Run("zero width handled gracefully", func(t *testing.T) {
		cfg := RenderConfig{AgentLabel: "Coordinator"}
		messages := []Message{
			{Role: "user", Content: "Test content"},
		}

		vc := NewChatVirtualContentWithMessages(messages, 0, cfg)

		lines := vc.Lines()
		require.Greater(t, len(lines), 0, "Should produce output even with width=0")
	})
}

// ============================================================================
// AppendMessage Tests - Task 3 of Virtual Scrolling Epic
// ============================================================================

func TestAppendMessage_AddsToEnd(t *testing.T) {
	t.Run("appending to empty content", func(t *testing.T) {
		cfg := RenderConfig{AgentLabel: "Coordinator"}
		vc := NewChatVirtualContent()
		vc.cfg = cfg
		vc.width = 80

		startIdx := vc.AppendMessage(Message{Role: "user", Content: "Hello"})

		require.Equal(t, 0, startIdx, "First message should start at index 0")
		require.Equal(t, 3, vc.TotalLines(), "Should have 3 lines: role + content + blank")
		require.Len(t, vc.Messages(), 1, "Should have 1 message")
	})

	t.Run("appending adds to end of existing content", func(t *testing.T) {
		cfg := RenderConfig{AgentLabel: "Coordinator"}
		messages := []Message{
			{Role: "user", Content: "First message"},
		}
		vc := NewChatVirtualContentWithMessages(messages, 80, cfg)

		initialLines := vc.TotalLines()

		startIdx := vc.AppendMessage(Message{Role: "assistant", Content: "Second message"})

		require.Equal(t, initialLines, startIdx, "New message should start at previous total")
		require.Greater(t, vc.TotalLines(), initialLines, "Total lines should increase")
		require.Len(t, vc.Messages(), 2, "Should have 2 messages")
	})

	t.Run("multiple appends accumulate correctly", func(t *testing.T) {
		cfg := RenderConfig{AgentLabel: "Coordinator"}
		vc := NewChatVirtualContent()
		vc.cfg = cfg
		vc.width = 80

		startIdx1 := vc.AppendMessage(Message{Role: "user", Content: "One"})
		startIdx2 := vc.AppendMessage(Message{Role: "assistant", Content: "Two"})
		startIdx3 := vc.AppendMessage(Message{Role: "user", Content: "Three"})

		require.Equal(t, 0, startIdx1, "First message at 0")
		require.Equal(t, 3, startIdx2, "Second message at 3")
		require.Equal(t, 6, startIdx3, "Third message at 6")
		require.Equal(t, 9, vc.TotalLines(), "Total should be 9 lines")
		require.Len(t, vc.Messages(), 3, "Should have 3 messages")
	})

	t.Run("returns correct start index for wrapped content", func(t *testing.T) {
		cfg := RenderConfig{AgentLabel: "Coordinator"}
		vc := NewChatVirtualContent()
		vc.cfg = cfg
		vc.width = 30 // Narrow width forces wrapping

		// First message - short
		vc.AppendMessage(Message{Role: "user", Content: "Short"})
		linesBefore := vc.TotalLines()

		// Second message - long, will wrap
		longContent := "This is a much longer message that should wrap to multiple lines"
		startIdx := vc.AppendMessage(Message{Role: "assistant", Content: longContent})

		require.Equal(t, linesBefore, startIdx, "Start index should be previous total")
		require.Greater(t, vc.TotalLines(), linesBefore+3, "Long message should produce more than 3 lines")
	})
}

func TestAppendMessage_PreservesCache(t *testing.T) {
	t.Run("existing cache entries not invalidated", func(t *testing.T) {
		cfg := RenderConfig{AgentLabel: "Coordinator"}
		messages := []Message{
			{Role: "user", Content: "First"},
		}
		vc := NewChatVirtualContentWithMessages(messages, 80, cfg)

		// Render first line to populate cache
		rendered1 := vc.RenderLine(0)
		require.NotEmpty(t, rendered1)

		// Check cache has entry
		initialCacheSize := vc.Cache().Size()
		require.Greater(t, initialCacheSize, 0, "Cache should have entries after render")

		// Append new message
		vc.AppendMessage(Message{Role: "assistant", Content: "Second"})

		// Original cache entry should still be valid (cache not cleared)
		// Re-render should hit cache
		rendered1Again := vc.RenderLine(0)
		require.Equal(t, rendered1, rendered1Again, "Cached render should be identical")
	})

	t.Run("new message lines can be rendered without affecting old cache", func(t *testing.T) {
		cfg := RenderConfig{AgentLabel: "Coordinator"}
		messages := []Message{
			{Role: "user", Content: "First"},
		}
		vc := NewChatVirtualContentWithMessages(messages, 80, cfg)

		// Render all initial lines
		for i := 0; i < vc.TotalLines(); i++ {
			vc.RenderLine(i)
		}
		cacheAfterFirst := vc.Cache().Size()

		// Append and render new message
		vc.AppendMessage(Message{Role: "assistant", Content: "Second"})
		for i := 3; i < vc.TotalLines(); i++ {
			vc.RenderLine(i)
		}

		// Cache should have more entries
		require.Greater(t, vc.Cache().Size(), cacheAfterFirst, "Cache should grow with new lines")
	})
}

func TestAppendMessage_O1Complexity(t *testing.T) {
	t.Run("append does not rebuild all lines", func(t *testing.T) {
		cfg := RenderConfig{AgentLabel: "Coordinator"}

		// Create content with many messages
		var messages []Message
		for i := 0; i < 100; i++ {
			messages = append(messages, Message{Role: "user", Content: "Message content"})
		}
		vc := NewChatVirtualContentWithMessages(messages, 80, cfg)

		initialTotal := vc.TotalLines()
		initialLineCount := len(vc.Lines())

		// Append one message
		vc.AppendMessage(Message{Role: "assistant", Content: "New message"})

		// Verify only new lines were added (not all rebuilt)
		// 3 lines per message: role + content + blank
		expectedNewLines := 3
		actualNewLines := vc.TotalLines() - initialTotal

		require.Equal(t, expectedNewLines, actualNewLines,
			"Should only add 3 lines, not rebuild all %d lines", initialLineCount)

		// Verify message indices are sequential
		lastLine := vc.Lines()[len(vc.Lines())-1]
		require.Equal(t, 100, lastLine.MessageIndex, "New message should have index 100")
	})
}

func TestAppendMessage_ToolCallSequence(t *testing.T) {
	t.Run("appending tool call continues sequence", func(t *testing.T) {
		cfg := RenderConfig{AgentLabel: "Coordinator"}
		vc := NewChatVirtualContent()
		vc.cfg = cfg
		vc.width = 80

		// Append first tool call
		vc.AppendMessage(Message{Role: "assistant", Content: "tool1", IsToolCall: true})

		// At this point, tool1 should have ╰╴ prefix (it's the only/last tool)
		toolLine1 := findToolCallLine(vc.Lines(), 0)
		require.Equal(t, "╰╴ tool1", toolLine1.PlainText, "Single tool should have ╰╴")

		// Append second tool call
		vc.AppendMessage(Message{Role: "assistant", Content: "tool2", IsToolCall: true})

		// Now tool1 should have ├╴ prefix (no longer last)
		toolLine1Updated := findToolCallLine(vc.Lines(), 0)
		require.Equal(t, "├╴ tool1", toolLine1Updated.PlainText, "First tool should now have ├╴")

		// tool2 should have ╰╴ prefix (it's now last)
		toolLine2 := findToolCallLine(vc.Lines(), 1)
		require.Equal(t, "╰╴ tool2", toolLine2.PlainText, "Last tool should have ╰╴")
	})

	t.Run("appending non-tool after tool starts new sequence", func(t *testing.T) {
		cfg := RenderConfig{AgentLabel: "Coordinator"}
		vc := NewChatVirtualContent()
		vc.cfg = cfg
		vc.width = 80

		vc.AppendMessage(Message{Role: "assistant", Content: "tool1", IsToolCall: true})
		vc.AppendMessage(Message{Role: "assistant", Content: "Regular text"})
		vc.AppendMessage(Message{Role: "assistant", Content: "tool2", IsToolCall: true})

		// tool1 should have ╰╴ (only one in its sequence)
		toolLine1 := findToolCallLine(vc.Lines(), 0)
		require.Equal(t, "╰╴ tool1", toolLine1.PlainText)

		// tool2 should have ╰╴ (only one in its new sequence)
		toolLine2 := findToolCallLine(vc.Lines(), 1)
		require.Equal(t, "╰╴ tool2", toolLine2.PlainText)
	})
}

// Helper to find nth tool call line
func findToolCallLine(lines []ChatVirtualLine, n int) ChatVirtualLine {
	count := 0
	for _, line := range lines {
		if line.LineType == LineTypeToolCall {
			if count == n {
				return line
			}
			count++
		}
	}
	return ChatVirtualLine{}
}

// ============================================================================
// RenderLine Tests - Task 3 of Virtual Scrolling Epic
// ============================================================================

func TestRenderLine_CacheHit(t *testing.T) {
	t.Run("second call returns cached value", func(t *testing.T) {
		cfg := RenderConfig{AgentLabel: "Coordinator"}
		messages := []Message{
			{Role: "user", Content: "Hello"},
		}
		vc := NewChatVirtualContentWithMessages(messages, 80, cfg)

		// First render - cache miss
		rendered1 := vc.RenderLine(0)
		require.NotEmpty(t, rendered1)

		cacheSize := vc.Cache().Size()
		require.Greater(t, cacheSize, 0, "Cache should have entry after first render")

		// Second render - should hit cache
		rendered2 := vc.RenderLine(0)
		require.Equal(t, rendered1, rendered2, "Second render should match first")

		// Cache size shouldn't have changed (hit, not new entry)
		require.Equal(t, cacheSize, vc.Cache().Size(), "Cache size unchanged on hit")
	})
}

func TestRenderLine_CacheMiss(t *testing.T) {
	t.Run("first render populates cache", func(t *testing.T) {
		cfg := RenderConfig{AgentLabel: "Coordinator"}
		messages := []Message{
			{Role: "user", Content: "Hello"},
		}
		vc := NewChatVirtualContentWithMessages(messages, 80, cfg)

		require.Equal(t, 0, vc.Cache().Size(), "Cache should be empty initially")

		vc.RenderLine(0)
		require.Greater(t, vc.Cache().Size(), 0, "Cache should have entry after render")
	})

	t.Run("different lines create separate cache entries", func(t *testing.T) {
		cfg := RenderConfig{AgentLabel: "Coordinator"}
		messages := []Message{
			{Role: "user", Content: "Hello"},
		}
		vc := NewChatVirtualContentWithMessages(messages, 80, cfg)

		vc.RenderLine(0) // Role line
		vc.RenderLine(1) // Content line
		vc.RenderLine(2) // Blank line

		require.Equal(t, 3, vc.Cache().Size(), "Should have 3 separate cache entries")
	})
}

func TestRenderLine_RoleType(t *testing.T) {
	t.Run("user role has correct styling", func(t *testing.T) {
		cfg := RenderConfig{AgentLabel: "Coordinator"}
		messages := []Message{
			{Role: "user", Content: "Hello"},
		}
		vc := NewChatVirtualContentWithMessages(messages, 80, cfg)

		rendered := vc.RenderLine(0)
		// Should contain "You" - ANSI codes may or may not be present depending on terminal detection
		require.Contains(t, rendered, "You", "Should render 'You' for user role")
	})

	t.Run("agent role uses AgentLabel", func(t *testing.T) {
		cfg := RenderConfig{AgentLabel: "TestAgent"}
		messages := []Message{
			{Role: "assistant", Content: "Hello"},
		}
		vc := NewChatVirtualContentWithMessages(messages, 80, cfg)

		rendered := vc.RenderLine(0)
		require.Contains(t, rendered, "TestAgent", "Should render AgentLabel for assistant role")
	})

	t.Run("system role renders System", func(t *testing.T) {
		cfg := RenderConfig{AgentLabel: "Coordinator"}
		messages := []Message{
			{Role: "system", Content: "Notice"},
		}
		vc := NewChatVirtualContentWithMessages(messages, 80, cfg)

		rendered := vc.RenderLine(0)
		require.Contains(t, rendered, "System", "Should render 'System' for system role")
	})

	t.Run("coordinator role in worker pane", func(t *testing.T) {
		cfg := RenderConfig{
			AgentLabel:              "Worker",
			ShowCoordinatorInWorker: true,
		}
		messages := []Message{
			{Role: "coordinator", Content: "Task"},
		}
		vc := NewChatVirtualContentWithMessages(messages, 80, cfg)

		rendered := vc.RenderLine(0)
		require.Contains(t, rendered, "Coordinator", "Should render 'Coordinator' when ShowCoordinatorInWorker")
	})
}

func TestRenderLine_ContentType(t *testing.T) {
	t.Run("content line renders plain text", func(t *testing.T) {
		cfg := RenderConfig{AgentLabel: "Coordinator"}
		messages := []Message{
			{Role: "user", Content: "Hello world"},
		}
		vc := NewChatVirtualContentWithMessages(messages, 80, cfg)

		// Line 1 is content
		rendered := vc.RenderLine(1)
		require.Equal(t, "Hello world", rendered, "Content line should render plain text")
	})
}

func TestRenderLine_ToolCallType(t *testing.T) {
	t.Run("tool call renders with prefix", func(t *testing.T) {
		cfg := RenderConfig{AgentLabel: "Coordinator"}
		messages := []Message{
			{Role: "assistant", Content: "read_file", IsToolCall: true},
		}
		vc := NewChatVirtualContentWithMessages(messages, 80, cfg)

		// Find the tool call line (usually line 1 after role)
		toolLineIdx := -1
		for i, line := range vc.Lines() {
			if line.LineType == LineTypeToolCall {
				toolLineIdx = i
				break
			}
		}
		require.NotEqual(t, -1, toolLineIdx, "Should have a tool call line")

		rendered := vc.RenderLine(toolLineIdx)
		require.Contains(t, rendered, "read_file", "Should contain tool name")
		// ANSI codes may or may not be present depending on terminal detection
	})
}

func TestRenderLine_BlankType(t *testing.T) {
	t.Run("blank line renders empty string", func(t *testing.T) {
		cfg := RenderConfig{AgentLabel: "Coordinator"}
		messages := []Message{
			{Role: "user", Content: "Hello"},
		}
		vc := NewChatVirtualContentWithMessages(messages, 80, cfg)

		// Line 2 is blank
		rendered := vc.RenderLine(2)
		require.Equal(t, "", rendered, "Blank line should render empty string")
	})
}

func TestRenderLine_OutOfBounds(t *testing.T) {
	t.Run("negative index returns empty", func(t *testing.T) {
		cfg := RenderConfig{AgentLabel: "Coordinator"}
		messages := []Message{
			{Role: "user", Content: "Hello"},
		}
		vc := NewChatVirtualContentWithMessages(messages, 80, cfg)

		rendered := vc.RenderLine(-1)
		require.Equal(t, "", rendered, "Negative index should return empty")
	})

	t.Run("index past end returns empty", func(t *testing.T) {
		cfg := RenderConfig{AgentLabel: "Coordinator"}
		messages := []Message{
			{Role: "user", Content: "Hello"},
		}
		vc := NewChatVirtualContentWithMessages(messages, 80, cfg)

		rendered := vc.RenderLine(100)
		require.Equal(t, "", rendered, "Index past end should return empty")
	})

	t.Run("empty content returns empty for any index", func(t *testing.T) {
		vc := NewChatVirtualContent()

		rendered := vc.RenderLine(0)
		require.Equal(t, "", rendered, "Empty content should return empty")
	})
}

// ============================================================================
// SetWidth Tests - Task 3 of Virtual Scrolling Epic
// ============================================================================

func TestSetWidth_ClearsCache(t *testing.T) {
	t.Run("width change clears cache", func(t *testing.T) {
		cfg := RenderConfig{AgentLabel: "Coordinator"}
		messages := []Message{
			{Role: "user", Content: "Hello world"},
		}
		vc := NewChatVirtualContentWithMessages(messages, 80, cfg)

		// Populate cache
		for i := 0; i < vc.TotalLines(); i++ {
			vc.RenderLine(i)
		}
		require.Greater(t, vc.Cache().Size(), 0, "Cache should have entries")

		// Change width
		vc.SetWidth(100)

		require.Equal(t, 0, vc.Cache().Size(), "Cache should be cleared on width change")
		require.Equal(t, 100, vc.Width(), "Width should be updated")
	})

	t.Run("same width does not clear cache", func(t *testing.T) {
		cfg := RenderConfig{AgentLabel: "Coordinator"}
		messages := []Message{
			{Role: "user", Content: "Hello world"},
		}
		vc := NewChatVirtualContentWithMessages(messages, 80, cfg)

		// Populate cache
		for i := 0; i < vc.TotalLines(); i++ {
			vc.RenderLine(i)
		}
		cacheSize := vc.Cache().Size()
		require.Greater(t, cacheSize, 0, "Cache should have entries")

		// Set same width
		vc.SetWidth(80)

		require.Equal(t, cacheSize, vc.Cache().Size(), "Cache should not be cleared for same width")
	})
}

func TestSetWidth_RebuildsLines(t *testing.T) {
	t.Run("width change rebuilds lines with new wrapping", func(t *testing.T) {
		cfg := RenderConfig{AgentLabel: "Coordinator"}
		longContent := "This is a long message that will wrap differently at different widths"
		messages := []Message{
			{Role: "user", Content: longContent},
		}
		vc := NewChatVirtualContentWithMessages(messages, 80, cfg)

		linesAtWidth80 := vc.TotalLines()

		// Change to narrow width - should produce more lines
		vc.SetWidth(30)

		require.Greater(t, vc.TotalLines(), linesAtWidth80,
			"Narrower width should produce more lines due to wrapping")
	})

	t.Run("wider width produces fewer wrapped lines", func(t *testing.T) {
		cfg := RenderConfig{AgentLabel: "Coordinator"}
		longContent := strings.Repeat("word ", 50) // 250 chars
		messages := []Message{
			{Role: "user", Content: longContent},
		}
		vc := NewChatVirtualContentWithMessages(messages, 40, cfg)

		linesAtWidth40 := vc.TotalLines()

		// Change to wide width
		vc.SetWidth(200)

		require.Less(t, vc.TotalLines(), linesAtWidth40,
			"Wider width should produce fewer lines")
	})

	t.Run("plainLines stays synchronized after width change", func(t *testing.T) {
		cfg := RenderConfig{AgentLabel: "Coordinator"}
		messages := []Message{
			{Role: "user", Content: "Hello world test content"},
		}
		vc := NewChatVirtualContentWithMessages(messages, 80, cfg)

		// Change width
		vc.SetWidth(50)

		lines := vc.Lines()
		plainLines := vc.PlainLines()

		require.Equal(t, len(lines), len(plainLines), "lines and plainLines should stay synchronized")

		for i, line := range lines {
			require.Equal(t, line.PlainText, plainLines[i],
				"plainLines[%d] should match lines[%d].PlainText after width change", i, i)
		}
	})
}

func TestSetWidth_MessagesPreserved(t *testing.T) {
	t.Run("messages array unchanged after width change", func(t *testing.T) {
		cfg := RenderConfig{AgentLabel: "Coordinator"}
		messages := []Message{
			{Role: "user", Content: "First"},
			{Role: "assistant", Content: "Second"},
		}
		vc := NewChatVirtualContentWithMessages(messages, 80, cfg)

		require.Len(t, vc.Messages(), 2)

		vc.SetWidth(50)

		require.Len(t, vc.Messages(), 2, "Messages should be preserved after width change")
		require.Equal(t, "First", vc.Messages()[0].Content)
		require.Equal(t, "Second", vc.Messages()[1].Content)
	})
}
