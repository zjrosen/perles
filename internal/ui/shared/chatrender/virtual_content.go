// Package chatrender provides shared chat message rendering for chat-based UIs.
// This file contains the virtual content types for virtual scrolling support.
package chatrender

import (
	"container/list"
	"sync"

	"github.com/charmbracelet/lipgloss"
)

// ChatLineType identifies the type of a virtual chat line.
type ChatLineType int

const (
	// LineTypeRole is a line containing a role label (e.g., "Coordinator", "You").
	LineTypeRole ChatLineType = iota
	// LineTypeContent is a line containing message content (text or word-wrapped continuation).
	LineTypeContent
	// LineTypeToolCall is a line containing a tool call entry.
	LineTypeToolCall
	// LineTypeBlank is an empty separator line between messages.
	LineTypeBlank
)

// String returns the string representation of the line type.
func (lt ChatLineType) String() string {
	switch lt {
	case LineTypeRole:
		return "Role"
	case LineTypeContent:
		return "Content"
	case LineTypeToolCall:
		return "ToolCall"
	case LineTypeBlank:
		return "Blank"
	default:
		return "Unknown"
	}
}

// ChatVirtualLine represents a single renderable line in virtual chat content.
// It stores the raw data needed to render the line on demand.
type ChatVirtualLine struct {
	// MessageIndex is the index into the source messages array.
	MessageIndex int
	// LineIndex is the line number within the message's rendered output.
	LineIndex int
	// LineType identifies whether this is a role line, content line, tool call, or blank.
	LineType ChatLineType
	// PlainText is the unstyled text content for selection extraction.
	PlainText string
}

// ChatVirtualContent manages virtual scrolling for chat messages.
// It pre-computes line metadata once and renders visible lines on demand.
type ChatVirtualContent struct {
	// lines holds the virtual line metadata for all chat content.
	lines []ChatVirtualLine
	// messages is the source message array (retained for rendering).
	messages []Message
	// plainLines holds unstyled text for each line (for text selection).
	plainLines []string
	// totalLines is the total number of lines across all messages.
	totalLines int
	// width is the current render width (for word wrapping).
	width int
	// cfg holds the render configuration (agent labels, colors).
	cfg RenderConfig
	// cache is the LRU cache for rendered line strings.
	cache *chatCache
}

// NewChatVirtualContent creates a new empty ChatVirtualContent.
// Use BuildLines to populate it with messages after creation.
func NewChatVirtualContent() *ChatVirtualContent {
	return &ChatVirtualContent{
		lines:      make([]ChatVirtualLine, 0),
		messages:   make([]Message, 0),
		plainLines: make([]string, 0),
		totalLines: 0,
		width:      0,
		cache:      newChatCache(DefaultCacheMaxItems, DefaultCacheMaxBytes),
	}
}

// NewChatVirtualContentWithMessages creates a ChatVirtualContent initialized with messages.
// This is the primary constructor for production use - it builds virtual lines immediately.
func NewChatVirtualContentWithMessages(messages []Message, width int, cfg RenderConfig) *ChatVirtualContent {
	vc := &ChatVirtualContent{
		lines:      make([]ChatVirtualLine, 0),
		messages:   messages,
		plainLines: make([]string, 0),
		totalLines: 0,
		width:      width,
		cfg:        cfg,
		cache:      newChatCache(DefaultCacheMaxItems, DefaultCacheMaxBytes),
	}
	vc.buildLines()
	return vc
}

// TotalLines returns the total number of virtual lines.
func (vc *ChatVirtualContent) TotalLines() int {
	return vc.totalLines
}

// Lines returns the virtual line array (for testing/debugging).
func (vc *ChatVirtualContent) Lines() []ChatVirtualLine {
	return vc.lines
}

// PlainLines returns the plain text lines array (for selection extraction).
func (vc *ChatVirtualContent) PlainLines() []string {
	return vc.plainLines
}

// Width returns the current render width.
func (vc *ChatVirtualContent) Width() int {
	return vc.width
}

// Cache returns the internal cache (for testing).
func (vc *ChatVirtualContent) Cache() *chatCache {
	return vc.cache
}

// Config returns the render configuration.
func (vc *ChatVirtualContent) Config() RenderConfig {
	return vc.cfg
}

// Messages returns the source messages array.
func (vc *ChatVirtualContent) Messages() []Message {
	return vc.messages
}

// buildLines converts the source messages into virtual lines.
// It processes each message to produce role lines, content lines, tool call lines, and blank separators.
// Tool call sequences are detected to apply correct tree prefixes (├╴ vs ╰╴).
func (vc *ChatVirtualContent) buildLines() {
	vc.lines = make([]ChatVirtualLine, 0)
	vc.plainLines = make([]string, 0)

	// Handle empty messages
	if len(vc.messages) == 0 {
		vc.totalLines = 0
		return
	}

	// Default user label if not specified
	userLabel := vc.cfg.UserLabel
	if userLabel == "" {
		userLabel = "You"
	}

	for i, msg := range vc.messages {
		// Tool call sequence detection - boundary checks are critical for off-by-one safety
		// This mirrors the logic in RenderContent()
		isFirstToolInSequence := msg.IsToolCall && (i == 0 || !vc.messages[i-1].IsToolCall)
		isLastToolInSequence := msg.IsToolCall && (i == len(vc.messages)-1 || !vc.messages[i+1].IsToolCall)

		lineIndexInMessage := 0

		if msg.Role == "user" {
			// Add role line
			vc.addLine(ChatVirtualLine{
				MessageIndex: i,
				LineIndex:    lineIndexInMessage,
				LineType:     LineTypeRole,
				PlainText:    userLabel,
			})
			lineIndexInMessage++

			// Word-wrap content and add content lines
			wrappedLines := vc.wrapContent(msg.Content)
			for _, wl := range wrappedLines {
				vc.addLine(ChatVirtualLine{
					MessageIndex: i,
					LineIndex:    lineIndexInMessage,
					LineType:     LineTypeContent,
					PlainText:    wl,
				})
				lineIndexInMessage++
			}

			// Add blank separator
			vc.addLine(ChatVirtualLine{
				MessageIndex: i,
				LineIndex:    lineIndexInMessage,
				LineType:     LineTypeBlank,
				PlainText:    "",
			})

		} else if msg.IsToolCall {
			// Only add role line for first tool in sequence
			if isFirstToolInSequence {
				vc.addLine(ChatVirtualLine{
					MessageIndex: i,
					LineIndex:    lineIndexInMessage,
					LineType:     LineTypeRole,
					PlainText:    vc.cfg.AgentLabel,
				})
				lineIndexInMessage++
			}

			// Determine prefix based on position in sequence
			prefix := "├╴ "
			if isLastToolInSequence {
				prefix = "╰╴ "
			}

			// Strip emoji prefix if present (matches RenderContent behavior)
			toolName := msg.Content
			if len(toolName) > 0 && toolName[0] == 0xF0 { // UTF-8 start of 4-byte emoji
				// Look for space after emoji
				for j := 0; j < len(toolName); j++ {
					if toolName[j] == ' ' {
						toolName = toolName[j+1:]
						break
					}
				}
			}
			// Also handle "🔧 " prefix explicitly
			if len(toolName) > 5 && toolName[:5] == "🔧 " {
				toolName = toolName[5:]
			}

			vc.addLine(ChatVirtualLine{
				MessageIndex: i,
				LineIndex:    lineIndexInMessage,
				LineType:     LineTypeToolCall,
				PlainText:    prefix + toolName,
			})
			lineIndexInMessage++

			// Add blank separator after last tool in sequence
			if isLastToolInSequence {
				vc.addLine(ChatVirtualLine{
					MessageIndex: i,
					LineIndex:    lineIndexInMessage,
					LineType:     LineTypeBlank,
					PlainText:    "",
				})
			}

		} else {
			// Regular text message - determine role label
			var roleLabel string
			switch {
			case msg.Role == "system":
				roleLabel = "System"
			case vc.cfg.ShowCoordinatorInWorker && msg.Role == "coordinator":
				roleLabel = "Coordinator"
			default:
				roleLabel = vc.cfg.AgentLabel
			}

			// Add role line
			vc.addLine(ChatVirtualLine{
				MessageIndex: i,
				LineIndex:    lineIndexInMessage,
				LineType:     LineTypeRole,
				PlainText:    roleLabel,
			})
			lineIndexInMessage++

			// Word-wrap content and add content lines
			wrappedLines := vc.wrapContent(msg.Content)
			for _, wl := range wrappedLines {
				vc.addLine(ChatVirtualLine{
					MessageIndex: i,
					LineIndex:    lineIndexInMessage,
					LineType:     LineTypeContent,
					PlainText:    wl,
				})
				lineIndexInMessage++
			}

			// Add blank separator
			vc.addLine(ChatVirtualLine{
				MessageIndex: i,
				LineIndex:    lineIndexInMessage,
				LineType:     LineTypeBlank,
				PlainText:    "",
			})
		}
	}

	vc.totalLines = len(vc.lines)
}

// addLine appends a virtual line and its plain text to the content.
func (vc *ChatVirtualContent) addLine(line ChatVirtualLine) {
	vc.lines = append(vc.lines, line)
	vc.plainLines = append(vc.plainLines, line.PlainText)
}

// wrapContent word-wraps content to fit within the configured width.
// Returns a slice of wrapped lines. Empty/whitespace-only content produces a single empty line.
func (vc *ChatVirtualContent) wrapContent(content string) []string {
	// Handle edge cases
	if content == "" || isOnlyWhitespace(content) {
		return []string{""}
	}

	// Use 4-char indent like RenderContent (wrapWidth-4)
	wrapWidth := vc.width - 4
	if wrapWidth <= 0 {
		wrapWidth = 1
	}

	wrapped := WordWrap(content, wrapWidth)
	if wrapped == "" {
		return []string{""}
	}

	return splitLines(wrapped)
}

// isOnlyWhitespace checks if a string contains only whitespace characters (spaces, tabs, newlines).
func isOnlyWhitespace(s string) bool {
	for _, r := range s {
		if r != ' ' && r != '\t' && r != '\n' && r != '\r' {
			return false
		}
	}
	return true
}

// splitLines splits a string by newlines, preserving empty lines.
func splitLines(s string) []string {
	if s == "" {
		return []string{""}
	}

	var result []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			result = append(result, s[start:i])
			start = i + 1
		}
	}
	// Add remaining content after last newline
	if start <= len(s) {
		result = append(result, s[start:])
	}

	// Ensure at least one line
	if len(result) == 0 {
		return []string{""}
	}

	return result
}

// chatCacheKey uniquely identifies a cached rendered line.
// Selection is NOT part of the key - selection highlight is applied as post-render overlay.
type chatCacheKey struct {
	// messageIndex identifies the source message.
	messageIndex int
	// lineIndex identifies the line within the message's rendered output.
	lineIndex int
	// width is the render width at which the line was cached.
	width int
}

// DefaultCacheMaxItems is the default maximum number of cached entries.
const DefaultCacheMaxItems = 1000

// DefaultCacheMaxBytes is the default memory limit for the cache (~10MB).
const DefaultCacheMaxBytes = 10 * 1024 * 1024

// chatCache is an LRU cache for rendered chat line strings.
// It has both item count and memory size limits for bounded resource usage.
// Eviction occurs when either limit is exceeded (whichever is hit first).
type chatCache struct {
	// capacity is the maximum number of entries.
	capacity int
	// maxBytes is the maximum total bytes.
	maxBytes int64
	// currentSize is the current total size in bytes.
	currentSize int64
	// cache maps keys to LRU list elements.
	cache map[chatCacheKey]*list.Element
	// lru is the LRU list for eviction ordering.
	lru *list.List
	// mu provides thread safety.
	mu sync.Mutex
}

// chatCacheEntry holds a cached rendered line with size tracking.
type chatCacheEntry struct {
	key   chatCacheKey
	value string
	size  int64 // Approximate memory size of this entry
}

// cacheEntrySize estimates the memory usage of a cache entry.
func cacheEntrySize(_ chatCacheKey, value string) int64 {
	// String content
	size := int64(len(value))
	// Key fields: three ints (~24 bytes)
	size += 24
	// Struct overhead
	size += 16
	return size
}

// newChatCache creates a new LRU cache with the given limits.
func newChatCache(capacity int, maxBytes int64) *chatCache {
	return &chatCache{
		capacity:    capacity,
		maxBytes:    maxBytes,
		currentSize: 0,
		cache:       make(map[chatCacheKey]*list.Element),
		lru:         list.New(),
	}
}

// Get retrieves a cached rendered line, returning ("", false) if not found.
func (c *chatCache) Get(key chatCacheKey) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.cache[key]; ok {
		c.lru.MoveToFront(elem)
		return elem.Value.(*chatCacheEntry).value, true
	}
	return "", false
}

// Put stores a rendered line in the cache.
// Evicts entries when either item count or memory limit is exceeded.
func (c *chatCache) Put(key chatCacheKey, value string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	size := cacheEntrySize(key, value)

	// Update existing entry
	if elem, ok := c.cache[key]; ok {
		c.lru.MoveToFront(elem)
		oldEntry := elem.Value.(*chatCacheEntry)
		c.currentSize -= oldEntry.size
		oldEntry.value = value
		oldEntry.size = size
		c.currentSize += size
		return
	}

	// Evict until we have room for both count and size limits
	for c.lru.Len() >= c.capacity || (c.maxBytes > 0 && c.currentSize+size > c.maxBytes) {
		oldest := c.lru.Back()
		if oldest == nil {
			break
		}
		entry := oldest.Value.(*chatCacheEntry)
		delete(c.cache, entry.key)
		c.lru.Remove(oldest)
		c.currentSize -= entry.size
	}

	entry := &chatCacheEntry{key: key, value: value, size: size}
	elem := c.lru.PushFront(entry)
	c.cache[key] = elem
	c.currentSize += size
}

// Clear empties the cache.
func (c *chatCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cache = make(map[chatCacheKey]*list.Element)
	c.lru.Init()
	c.currentSize = 0
}

// Size returns the current number of cached entries.
func (c *chatCache) Size() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lru.Len()
}

// ByteSize returns the current estimated memory usage in bytes.
func (c *chatCache) ByteSize() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.currentSize
}

// Capacity returns the maximum number of entries.
func (c *chatCache) Capacity() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.capacity
}

// MaxBytes returns the maximum memory limit.
func (c *chatCache) MaxBytes() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.maxBytes
}

// AppendMessage appends a single message to the virtual content.
// It adds new virtual lines for the message to the end of lines[],
// appends corresponding entries to plainLines[], and updates totalLines.
// Complexity: O(lines in message), NOT O(total lines) - existing cache entries are preserved.
// Returns the line index where the new message starts.
func (vc *ChatVirtualContent) AppendMessage(msg Message) int {
	startIndex := vc.totalLines
	msgIndex := len(vc.messages)

	// Append to messages array
	vc.messages = append(vc.messages, msg)

	// Default user label if not specified
	userLabel := vc.cfg.UserLabel
	if userLabel == "" {
		userLabel = "You"
	}

	// Tool call sequence detection for appended message
	// Check if previous message was a tool call to determine if we're continuing a sequence
	prevIsToolCall := msgIndex > 0 && vc.messages[msgIndex-1].IsToolCall
	isFirstToolInSequence := msg.IsToolCall && !prevIsToolCall
	// For appended messages, they're always the last in any sequence (for now)
	// A subsequent append may update the prefix if another tool call follows
	isLastToolInSequence := msg.IsToolCall // Always last until another is appended

	lineIndexInMessage := 0

	if msg.Role == "user" {
		// Add role line
		vc.addLine(ChatVirtualLine{
			MessageIndex: msgIndex,
			LineIndex:    lineIndexInMessage,
			LineType:     LineTypeRole,
			PlainText:    userLabel,
		})
		lineIndexInMessage++

		// Word-wrap content and add content lines
		wrappedLines := vc.wrapContent(msg.Content)
		for _, wl := range wrappedLines {
			vc.addLine(ChatVirtualLine{
				MessageIndex: msgIndex,
				LineIndex:    lineIndexInMessage,
				LineType:     LineTypeContent,
				PlainText:    wl,
			})
			lineIndexInMessage++
		}

		// Add blank separator
		vc.addLine(ChatVirtualLine{
			MessageIndex: msgIndex,
			LineIndex:    lineIndexInMessage,
			LineType:     LineTypeBlank,
			PlainText:    "",
		})

	} else if msg.IsToolCall {
		// If this is the first tool in a new sequence, add role line
		if isFirstToolInSequence {
			vc.addLine(ChatVirtualLine{
				MessageIndex: msgIndex,
				LineIndex:    lineIndexInMessage,
				LineType:     LineTypeRole,
				PlainText:    vc.cfg.AgentLabel,
			})
			lineIndexInMessage++
		}

		// If we're continuing a tool call sequence, update the previous tool's prefix
		// from ╰╴ to ├╴ (since it's no longer the last in sequence)
		if prevIsToolCall && vc.totalLines > 0 {
			vc.updatePreviousToolCallPrefix()
		}

		// Determine prefix based on position in sequence
		prefix := "├╴ "
		if isLastToolInSequence {
			prefix = "╰╴ "
		}

		// Strip emoji prefix if present (matches RenderContent behavior)
		toolName := msg.Content
		if len(toolName) > 0 && toolName[0] == 0xF0 { // UTF-8 start of 4-byte emoji
			// Look for space after emoji
			for j := 0; j < len(toolName); j++ {
				if toolName[j] == ' ' {
					toolName = toolName[j+1:]
					break
				}
			}
		}
		// Also handle "🔧 " prefix explicitly
		if len(toolName) > 5 && toolName[:5] == "🔧 " {
			toolName = toolName[5:]
		}

		vc.addLine(ChatVirtualLine{
			MessageIndex: msgIndex,
			LineIndex:    lineIndexInMessage,
			LineType:     LineTypeToolCall,
			PlainText:    prefix + toolName,
		})
		lineIndexInMessage++

		// Add blank separator after last tool in sequence
		if isLastToolInSequence {
			vc.addLine(ChatVirtualLine{
				MessageIndex: msgIndex,
				LineIndex:    lineIndexInMessage,
				LineType:     LineTypeBlank,
				PlainText:    "",
			})
		}

	} else {
		// Regular text message - determine role label
		var roleLabel string
		switch {
		case msg.Role == "system":
			roleLabel = "System"
		case vc.cfg.ShowCoordinatorInWorker && msg.Role == "coordinator":
			roleLabel = "Coordinator"
		default:
			roleLabel = vc.cfg.AgentLabel
		}

		// Add role line
		vc.addLine(ChatVirtualLine{
			MessageIndex: msgIndex,
			LineIndex:    lineIndexInMessage,
			LineType:     LineTypeRole,
			PlainText:    roleLabel,
		})
		lineIndexInMessage++

		// Word-wrap content and add content lines
		wrappedLines := vc.wrapContent(msg.Content)
		for _, wl := range wrappedLines {
			vc.addLine(ChatVirtualLine{
				MessageIndex: msgIndex,
				LineIndex:    lineIndexInMessage,
				LineType:     LineTypeContent,
				PlainText:    wl,
			})
			lineIndexInMessage++
		}

		// Add blank separator
		vc.addLine(ChatVirtualLine{
			MessageIndex: msgIndex,
			LineIndex:    lineIndexInMessage,
			LineType:     LineTypeBlank,
			PlainText:    "",
		})
	}

	vc.totalLines = len(vc.lines)
	return startIndex
}

// Tool call prefix constants (used for sequence detection)
const (
	toolPrefixLast    = "╰╴ " // Last tool in sequence
	toolPrefixMiddle  = "├╴ " // First/middle tool in sequence
	toolPrefixByteLen = 7     // Both prefixes are 7 bytes in UTF-8
)

// updatePreviousToolCallPrefix updates the previous tool call's prefix from ╰╴ to ├╴
// when a new tool call is appended (making the previous one no longer the last in sequence).
func (vc *ChatVirtualContent) updatePreviousToolCallPrefix() {
	// Search backwards for the previous tool call line
	for i := len(vc.lines) - 1; i >= 0; i-- {
		if vc.lines[i].LineType == LineTypeToolCall {
			// Update prefix from ╰╴ to ├╴
			if len(vc.lines[i].PlainText) >= toolPrefixByteLen && vc.lines[i].PlainText[:toolPrefixByteLen] == toolPrefixLast {
				vc.lines[i].PlainText = toolPrefixMiddle + vc.lines[i].PlainText[toolPrefixByteLen:]
				vc.plainLines[i] = vc.lines[i].PlainText
				// Cache will be updated on next RenderLine call for this index
				// (LRU handles stale entries automatically)
			}
			// Also need to remove the blank line that followed it if present
			if i+1 < len(vc.lines) && vc.lines[i+1].LineType == LineTypeBlank {
				// Remove the blank line
				vc.lines = append(vc.lines[:i+1], vc.lines[i+2:]...)
				vc.plainLines = append(vc.plainLines[:i+1], vc.plainLines[i+2:]...)
			}
			break
		}
	}
}

// RenderLine renders a single line at the given index.
// It checks the cache first using key {messageIndex, lineIndex, width}.
// On cache hit, returns the cached string.
// On cache miss, renders based on LineType and stores in cache.
func (vc *ChatVirtualContent) RenderLine(index int) string {
	if index < 0 || index >= len(vc.lines) {
		return ""
	}

	line := vc.lines[index]

	// Build cache key
	key := chatCacheKey{
		messageIndex: line.MessageIndex,
		lineIndex:    line.LineIndex,
		width:        vc.width,
	}

	// Check cache
	if cached, ok := vc.cache.Get(key); ok {
		return cached
	}

	// Render based on line type
	var rendered string
	switch line.LineType {
	case LineTypeRole:
		rendered = vc.renderRoleLine(line)
	case LineTypeContent:
		rendered = vc.renderContentLine(line)
	case LineTypeToolCall:
		rendered = vc.renderToolCallLine(line)
	case LineTypeBlank:
		rendered = ""
	default:
		rendered = line.PlainText
	}

	// Store in cache
	vc.cache.Put(key, rendered)

	return rendered
}

// renderRoleLine renders a role line with appropriate styling.
func (vc *ChatVirtualContent) renderRoleLine(line ChatVirtualLine) string {
	// Determine color based on role label
	var style lipgloss.Style

	switch line.PlainText {
	case "You", vc.cfg.UserLabel:
		style = RoleStyle.Foreground(UserColor)
	case "System":
		style = RoleStyle.Foreground(SystemColor)
	case "Coordinator":
		style = RoleStyle.Foreground(CoordinatorColor)
	default:
		// Use agent color from config
		style = RoleStyle.Foreground(vc.cfg.AgentColor)
	}

	return style.Render(line.PlainText)
}

// renderContentLine renders a content line.
func (vc *ChatVirtualContent) renderContentLine(line ChatVirtualLine) string {
	// Content lines are rendered as plain text
	// The message context determines any styling applied
	if line.MessageIndex < len(vc.messages) {
		msg := vc.messages[line.MessageIndex]
		if msg.Role == "user" {
			// User messages could have user styling if desired
			// For now, just return plain text
			return line.PlainText
		}
	}
	return line.PlainText
}

// renderToolCallLine renders a tool call line with muted styling and prefix.
func (vc *ChatVirtualContent) renderToolCallLine(line ChatVirtualLine) string {
	return ToolCallStyle.Render(line.PlainText)
}

// SetWidth updates the render width and clears the cache if width changed.
// This is necessary because word wrapping depends on width.
func (vc *ChatVirtualContent) SetWidth(width int) {
	if vc.width == width {
		return
	}

	vc.width = width
	vc.cache.Clear()

	// Rebuild all lines with new width
	vc.buildLines()
}
