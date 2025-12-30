package vimtextarea

import (
	"reflect"
	"regexp"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/zjrosen/perles/internal/ui/styles"
)

// mouseEscapePattern matches SGR mouse tracking sequences that weren't parsed by bubbletea.
// These look like "[<65;87;15M" or "<65;87;15M" (CSI < Pb ; Px ; Py M/m format).
var mouseEscapePattern = regexp.MustCompile(`^\[?<\d+;\d+;\d+[Mm]$`)

// isMouseEscapeSequence checks if runes represent an unparsed SGR mouse tracking sequence.
func isMouseEscapeSequence(runes []rune) bool {
	if len(runes) < 6 {
		return false
	}
	return mouseEscapePattern.MatchString(string(runes))
}

// YankHighlightDuration is the time the yank highlight is shown before fading.
const YankHighlightDuration = 200 * time.Millisecond

// YankHighlight represents a region to highlight after a yank operation.
type YankHighlight struct {
	Start    Position  // Start of highlighted region
	End      Position  // End of highlighted region (inclusive)
	Linewise bool      // Whether this is a line-wise highlight
	Expiry   time.Time // When the highlight should disappear
}

// yankHighlightTickMsg is sent to trigger a re-render after yank highlight expires.
type yankHighlightTickMsg struct{}

// cloneCommand creates a shallow copy of a command via reflection.
// Used for undoable commands to ensure each execution has independent state.
func cloneCommand(cmd Command) Command {
	v := reflect.ValueOf(cmd).Elem()
	clone := reflect.New(v.Type())
	clone.Elem().Set(v)
	return clone.Interface().(Command)
}

// Config defines vimtextarea configuration with optional callbacks.
type Config struct {
	// VimEnabled enables vim mode. When false, behaves as standard textarea (always Insert mode).
	VimEnabled bool

	// DefaultMode is the starting mode when vim is enabled. Ignored when VimEnabled is false.
	DefaultMode Mode

	// Placeholder is the text shown when the textarea is empty.
	Placeholder string

	// CharLimit is the maximum number of characters allowed. 0 means unlimited.
	CharLimit int

	// MaxHeight is the maximum display height in lines. 0 means unlimited.
	MaxHeight int

	// OnSubmit produces a custom message when content is submitted (Enter).
	// If nil, vimtextarea produces SubmitMsg{Content: content}.
	OnSubmit func(content string) tea.Msg

	// OnModeChange produces a custom message when vim mode changes.
	// If nil, no message is emitted on mode change.
	OnModeChange func(mode Mode, previous Mode) tea.Msg

	// OnChange produces a custom message when content changes.
	// If nil, no message is emitted on content change.
	OnChange func(content string) tea.Msg
}

// Position represents a cursor position in the textarea.
// Col is a grapheme index (not byte offset), representing the nth visible character.
type Position struct {
	Row int // Line number (0-indexed)
	Col int // Column as grapheme index (0-indexed, not byte offset)
}

// Model holds the vimtextarea state.
type Model struct {
	config Config

	// Content state
	content   []string // Lines of text
	cursorRow int      // Current line (0-indexed)
	cursorCol int      // Current column as grapheme index (0-indexed, not byte offset)

	// Vim state
	mode                Mode
	pendingBuilder      *PendingCommandBuilder // Structured pending command builder
	preferredCol        int                    // Preferred column for vertical movement (j/k)
	visualAnchor        Position               // Where visual selection started (anchor point)
	lastYankedText      string                 // Last yanked text (for paste command)
	lastYankWasLinewise bool                   // Whether the last yank was line-wise (affects paste behavior)

	// Yank highlight (brief flash on yanked text, like Vim's highlightedyank)
	yankHighlight *YankHighlight // Active yank highlight region (nil when inactive)

	// Syntax highlighting
	lexer SyntaxLexer // Lexer for syntax highlighting (nil = no highlighting)

	// Command-based undo/redo
	history *CommandHistory

	// Display state
	width   int
	height  int
	focused bool

	// Scrolling
	scrollOffset int // First visible line
}

// SubmitMsg is sent when the user submits content (Enter).
type SubmitMsg struct {
	Content string
}

// ModeChangeMsg is sent when vim mode changes (if OnModeChange callback is not set).
type ModeChangeMsg struct {
	Mode     Mode
	Previous Mode
}

// New creates a new vimtextarea with the given configuration.
func New(cfg Config) Model {
	mode := cfg.DefaultMode
	if !cfg.VimEnabled {
		// When vim is disabled, always start in Insert mode
		mode = ModeInsert
	}

	return Model{
		config:         cfg,
		content:        []string{""},
		cursorRow:      0,
		cursorCol:      0,
		mode:           mode,
		pendingBuilder: NewPendingCommandBuilder(),
		history:        NewCommandHistory(),
		focused:        false,
	}
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		m, cmd := m.handleKeyMsg(msg)
		// Ensure cursor is visible after any key handling
		m.ensureCursorVisible()
		return m, cmd
	case yankHighlightTickMsg:
		// Tick arrived - the highlight will be cleared automatically in View if expired
		// Just trigger a re-render by returning
		return m, nil
	}
	return m, nil
}

// keyToString converts a tea.KeyMsg to a registry-compatible key string.
// Returns empty string for unhandled key types.
func keyToString(msg tea.KeyMsg) string {
	// Handle Alt+Enter (newline/split line)
	if msg.Alt && msg.Type == tea.KeyEnter {
		return "<alt+enter>"
	}

	// Handle Ctrl+J (alternative submit)
	if msg.String() == "ctrl+j" {
		return "<ctrl+j>"
	}

	switch msg.Type {
	case tea.KeyRunes:
		if len(msg.Runes) == 1 {
			return string(msg.Runes[0])
		}
		return "<runes>" // Multi-rune input (paste)
	case tea.KeyEscape:
		return "<escape>"
	case tea.KeyEnter:
		return "<enter>"
	case tea.KeyBackspace:
		return "<backspace>"
	case tea.KeyDelete:
		return "<delete>"
	case tea.KeySpace:
		return "<space>"
	case tea.KeyCtrlR:
		return "<ctrl+r>"
	case tea.KeyCtrlU:
		return "<ctrl+u>"
	case tea.KeyCtrlK:
		return "<ctrl+k>"
	case tea.KeyCtrlA:
		return "<ctrl+a>"
	case tea.KeyCtrlE:
		return "<ctrl+e>"
	case tea.KeyLeft:
		return "<left>"
	case tea.KeyRight:
		return "<right>"
	case tea.KeyUp:
		return "<up>"
	case tea.KeyDown:
		return "<down>"
	case tea.KeyCtrlF:
		return "<ctrl+f>"
	case tea.KeyCtrlB:
		return "<ctrl+b>"
	case tea.KeyCtrlC:
		return "<ctrl+c>"
	default:
		return ""
	}
}

// SubmitRequester is implemented by commands that may trigger submit.
type SubmitRequester interface {
	IsSubmit() bool
}

// YankHighlighter is implemented by commands that want to show a yank highlight.
// After execution, the command returns the region to highlight.
type YankHighlighter interface {
	// YankHighlightRegion returns the start/end positions and whether it's linewise.
	// Returns nil positions if no highlight should be shown.
	YankHighlightRegion() (start, end Position, linewise bool, show bool)
}

// handleKeyMsg processes keyboard input via pure registry dispatch.
// All key handling logic is encapsulated in Command implementations.
func (m Model) handleKeyMsg(msg tea.KeyMsg) (Model, tea.Cmd) {
	// Handle pending commands first (multi-key sequences like gg, dd, dw)
	if !m.pendingBuilder.IsEmpty() {
		return m.handlePendingCommand(msg)
	}

	// Convert key to registry string
	keyStr := keyToString(msg)
	if keyStr == "" {
		return m, nil
	}

	// Determine effective mode (vim disabled = always Insert mode)
	mode := m.mode
	if !m.config.VimEnabled {
		mode = ModeInsert
	}

	// Pure registry dispatch
	cmd, ok := DefaultRegistry.Get(mode, keyStr)
	if !ok {
		// Fallback: character input in Insert mode
		if mode == ModeInsert && msg.Type == tea.KeyRunes && len(msg.Runes) > 0 {
			return m.handleCharacterInput(msg.Runes)
		}
		// Fallback: character input in Replace mode
		if mode == ModeReplace && msg.Type == tea.KeyRunes && len(msg.Runes) > 0 {
			return m.handleReplaceModeInput(msg.Runes)
		}
		return m, nil // Key not handled
	}

	// Execute and respond
	return m.executeAndRespond(cmd, mode)
}

// handleCharacterInput handles printable character input in Insert mode.
func (m Model) handleCharacterInput(runes []rune) (Model, tea.Cmd) {
	if len(runes) == 0 {
		return m, nil
	}

	// Filter out SGR mouse tracking escape sequences that weren't parsed by bubbletea.
	// These look like "[<65;87;15M" or "<65;87;15M" (scroll events, button events, etc.)
	// The CSI sequence is ESC [ < Pb ; Px ; Py M/m where Pb is button, Px/Py are coords.
	if isMouseEscapeSequence(runes) {
		return m, nil
	}

	cmd := &InsertTextCommand{
		row:  m.cursorRow,
		col:  m.cursorCol,
		text: string(runes),
	}
	_, result, teaCmd := m.executeCommand(cmd)
	if result == Skipped {
		return m, nil
	}
	return m, teaCmd
}

// handleReplaceModeInput handles printable character input in Replace mode.
// Characters overwrite existing text or append at end of line.
func (m Model) handleReplaceModeInput(runes []rune) (Model, tea.Cmd) {
	if len(runes) == 0 {
		return m, nil
	}

	// Filter out SGR mouse tracking escape sequences
	if isMouseEscapeSequence(runes) {
		return m, nil
	}

	// Process each rune as a separate replace command for proper undo granularity
	var lastCmd tea.Cmd
	for _, r := range runes {
		cmd := &ReplaceModeCharCommand{
			newChar: r,
		}
		_, result, teaCmd := m.executeCommand(cmd)
		if result == Skipped {
			continue
		}
		lastCmd = teaCmd
	}
	return m, lastCmd
}

// executeAndRespond executes a command and produces the appropriate response.
// Handles ExecuteResult, mode changes, submit requests, and yank highlights.
func (m Model) executeAndRespond(cmd Command, previousMode Mode) (Model, tea.Cmd) {
	cmd, result, teaCmd := m.executeCommand(cmd)

	switch result {
	case PassThrough:
		// Command chose not to handle - return nil to let parent handle
		return m, nil
	case Skipped:
		// Pre-conditions not met - key consumed but no effect
		return m, nil
	case Executed:
		// Command executed successfully
	}

	// Check if command wants to trigger submit
	if submitter, ok := cmd.(SubmitRequester); ok && submitter.IsSubmit() {
		return m.submitContent()
	}

	// Check if command wants to show yank highlight
	if yanker, ok := cmd.(YankHighlighter); ok {
		if start, end, linewise, show := yanker.YankHighlightRegion(); show {
			highlightCmd := m.SetYankHighlight(start, end, linewise)
			teaCmd = tea.Batch(teaCmd, highlightCmd)
		}
	}

	// Check if command changed mode
	if cmd.IsModeChange() {
		return m, tea.Batch(teaCmd, m.modeChangeCmd(previousMode))
	}

	return m, teaCmd
}

// totalCharCount returns the total number of grapheme clusters (visible characters) in the content.
// Newlines are counted as one character each.
// This ensures CharLimit counts visible characters, not bytes
// (e.g., "280 characters" means 280 graphemes including emoji).
func (m Model) totalCharCount() int {
	count := 0
	for i, line := range m.content {
		count += GraphemeCount(line)
		if i < len(m.content)-1 {
			count++ // Count newline between lines
		}
	}
	return count
}

// clampCursorCol ensures cursor column is within valid bounds for current line.
// In Normal mode, cursor can be at most at the last character (grapheme count - 1), not after.
// Note: cursorCol represents grapheme index, not byte offset.
func (m *Model) clampCursorCol() {
	line := m.content[m.cursorRow]
	maxCol := max(GraphemeCount(line)-1, 0)
	m.cursorCol = max(min(m.cursorCol, maxCol), 0)
}

// ============================================================================
// Delete Operations (x, dd, D, dw, d$)
// ============================================================================

// onChangeCmd returns a command that emits a change message if OnChange is configured.
func (m Model) onChangeCmd() tea.Cmd {
	if m.config.OnChange != nil {
		content := m.Value()
		return func() tea.Msg {
			return m.config.OnChange(content)
		}
	}
	return nil
}

// executeCommand runs a command and handles history + onChange based on undoability.
// Returns (error, tea.Cmd) - the tea.Cmd is onChangeCmd() for content mutations.
// This wrapper ensures that:
// 1. The command is executed
// 2. If the command is undoable, it's pushed to history
// 3. The onChange callback is returned for content-mutating commands
//
// How undoability is determined:
// - Motion commands and mode changes are NOT added to history
// - Undo/Redo commands are NOT added to history (they manipulate history)
// - Content-mutating commands (delete, insert, etc.) ARE added to history
//
// How onChange is determined:
// - Any command that changes content triggers onChange (including undo/redo)
// - Motion and mode commands do NOT trigger onChange
func (m *Model) executeCommand(cmd Command) (Command, ExecuteResult, tea.Cmd) {
	// Clone undoable commands before execution so each has independent state
	if isUndoable(cmd) {
		cmd = cloneCommand(cmd)
	}

	result := cmd.Execute(m)
	if result != Executed {
		return cmd, result, nil
	}

	// Add to history if undoable (content-mutating)
	if isUndoable(cmd) {
		m.history.Push(cmd)
	}

	// Return onChange for commands that change content
	if changesContent(cmd) {
		return cmd, Executed, m.onChangeCmd()
	}
	return cmd, Executed, nil
}

// isUndoable delegates to the Command interface method.
// Commands that modify content are undoable; motion and mode commands are not.
func isUndoable(cmd Command) bool {
	return cmd.IsUndoable()
}

// changesContent delegates to the Command interface method.
// Used to determine whether to trigger onChange callbacks.
func changesContent(cmd Command) bool {
	return cmd.ChangesContent()
}

// ============================================================================
// Yank Highlight API
// ============================================================================

// SetYankHighlight sets the yank highlight region and returns a command to trigger re-render after expiry.
func (m *Model) SetYankHighlight(start, end Position, linewise bool) tea.Cmd {
	m.yankHighlight = &YankHighlight{
		Start:    start,
		End:      end,
		Linewise: linewise,
		Expiry:   time.Now().Add(YankHighlightDuration),
	}
	return tea.Tick(YankHighlightDuration, func(time.Time) tea.Msg {
		return yankHighlightTickMsg{}
	})
}

// ClearYankHighlight clears the yank highlight immediately.
func (m *Model) ClearYankHighlight() {
	m.yankHighlight = nil
}

// YankHighlightRegion returns the current yank highlight region, or nil if none.
func (m Model) YankHighlightRegion() *YankHighlight {
	return m.yankHighlight
}

// ============================================================================
// Syntax Highlighting API
// ============================================================================

// SetLexer configures syntax highlighting for the textarea.
// Pass nil to disable syntax highlighting (plain text rendering).
func (m *Model) SetLexer(l SyntaxLexer) {
	m.lexer = l
}

// Lexer returns the currently configured syntax lexer, or nil if none.
func (m Model) Lexer() SyntaxLexer {
	return m.lexer
}

// ============================================================================
// Undo/Redo Public API
// ============================================================================

// CanUndo returns true if there are commands in the undo history.
func (m Model) CanUndo() bool {
	return m.history.CanUndo()
}

// CanRedo returns true if there are commands in the redo history.
func (m Model) CanRedo() bool {
	return m.history.CanRedo()
}

// ============================================================================
// Pending Command Handler
// ============================================================================

// handlePendingCommand processes keys when there's a pending command.
// Uses PendingCommandRegistry for pure dispatch - all command logic is in Command implementations.
// Supports multi-key sequences like 'dgg', 'cgg', 'diw', etc. by buffering keys.
// Special case: 'r' operator takes a single character as replacement (not a motion).
func (m Model) handlePendingCommand(msg tea.KeyMsg) (Model, tea.Cmd) {
	operator := m.pendingBuilder.Operator()

	// Special case: 'r' operator takes a single character as replacement
	if operator == 'r' {
		return m.handleReplaceCharPending(msg)
	}

	// Convert key to string for registry lookup
	var key string
	if msg.Type == tea.KeyRunes && len(msg.Runes) == 1 {
		key = string(msg.Runes[0])
	} else {
		// Non-rune keys (escape, etc.) - clear pending and ignore
		m.pendingBuilder.Clear()
		return m, nil
	}

	// Build the full key sequence by appending to buffer
	m.pendingBuilder.AppendKey(key)
	keySequence := m.pendingBuilder.KeyBuffer()

	// First check for exact match
	if cmd, ok := DefaultPendingRegistry.Get(operator, keySequence); ok {
		// Found exact match - execute command
		m.pendingBuilder.Clear()
		cmd, _, teaCmd := m.executeCommand(cmd)

		// Check if command wants to show yank highlight
		if yanker, ok := cmd.(YankHighlighter); ok {
			if start, end, linewise, show := yanker.YankHighlightRegion(); show {
				highlightCmd := m.SetYankHighlight(start, end, linewise)
				teaCmd = tea.Batch(teaCmd, highlightCmd)
			}
		}

		return m, teaCmd
	}

	// Check if there are commands with this prefix (need more keys)
	if DefaultPendingRegistry.HasPrefix(operator, keySequence) {
		// Keep buffering - more keys expected
		return m, nil
	}

	// No match and no prefix - handle fallback cases
	m.pendingBuilder.Clear()

	// Special case: 'v' operator fallback - enter visual mode and replay key as motion
	// This allows sequences like 'vj' to work as expected (enter visual mode, then move down)
	if operator == 'v' {
		return m.handleVisualOperatorFallback(msg)
	}

	return m, nil
}

// handleReplaceCharPending handles the 'r' operator pending state.
// The next single character typed becomes the replacement character.
// <Escape> cancels the pending state.
// <Space> replaces with a space character.
func (m Model) handleReplaceCharPending(msg tea.KeyMsg) (Model, tea.Cmd) {
	// Handle <Escape> - cancel pending
	if msg.Type == tea.KeyEscape {
		m.pendingBuilder.Clear()
		return m, nil
	}

	// Handle <Space> - replace with space character
	// Note: Bubble Tea sends space as KeySpace, not KeyRunes
	if msg.Type == tea.KeySpace {
		cmd := &ReplaceCharCommand{
			newChar: ' ',
		}
		m.pendingBuilder.Clear()
		_, _, teaCmd := m.executeCommand(cmd)
		return m, teaCmd
	}

	// Accept single rune input as the replacement character
	if msg.Type == tea.KeyRunes && len(msg.Runes) == 1 {
		// Create and execute the replace command
		cmd := &ReplaceCharCommand{
			newChar: msg.Runes[0],
		}
		m.pendingBuilder.Clear()
		_, _, teaCmd := m.executeCommand(cmd)
		return m, teaCmd
	}

	// Ignore other keys (e.g., <Enter> is deferred per proposal)
	// Clear pending and return
	m.pendingBuilder.Clear()
	return m, nil
}

// handleVisualOperatorFallback handles 'v' operator fallback when no text object match is found.
// This enables sequences like 'vj' (enter visual mode, then move down) to work correctly.
// When 'v' is followed by a key that's not a text object prefix (like 'i' or 'a'),
// we enter visual mode and replay the key as a visual mode motion.
func (m Model) handleVisualOperatorFallback(msg tea.KeyMsg) (Model, tea.Cmd) {
	// First, enter visual mode at current cursor position
	m.visualAnchor = Position{Row: m.cursorRow, Col: m.cursorCol}
	m.mode = ModeVisual
	previousMode := ModeNormal

	// Convert key to registry string
	keyStr := keyToString(msg)
	if keyStr == "" {
		// Mode change with empty key (shouldn't happen but handle gracefully)
		return m, m.modeChangeCmd(previousMode)
	}

	// Try to dispatch the key as a visual mode motion
	if cmd, ok := DefaultRegistry.Get(ModeVisual, keyStr); ok {
		_, _, teaCmd := m.executeCommand(cmd)
		// Batch mode change notification with command result
		return m, tea.Batch(m.modeChangeCmd(previousMode), teaCmd)
	}

	// Key not recognized in visual mode - just enter visual mode
	return m, m.modeChangeCmd(previousMode)
}

// modeChangeCmd returns a command that emits a mode change message if configured.
func (m Model) modeChangeCmd(previous Mode) tea.Cmd {
	if m.config.OnModeChange != nil {
		return func() tea.Msg {
			return m.config.OnModeChange(m.mode, previous)
		}
	}
	// If no callback, emit the default ModeChangeMsg
	return func() tea.Msg {
		return ModeChangeMsg{Mode: m.mode, Previous: previous}
	}
}

// View is implemented in render.go

// SetSize sets the viewport dimensions.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
	m.ensureCursorVisible()
}

// Focus focuses the textarea.
func (m *Model) Focus() {
	m.focused = true
}

// Blur removes focus from the textarea and clears any pending command.
func (m *Model) Blur() {
	m.focused = false
	m.pendingBuilder.Clear()
}

// Focused returns whether the textarea is focused.
func (m Model) Focused() bool {
	return m.focused
}

// Value returns the full content as a single string with newlines.
func (m Model) Value() string {
	return strings.Join(m.content, "\n")
}

// Lines returns the content as a slice of lines.
func (m Model) Lines() []string {
	return m.content
}

// SetValue sets the content from a string, splitting on newlines.
// If in visual mode, clears visual mode since content change invalidates anchor.
func (m *Model) SetValue(s string) {
	// Clear visual mode if active - content change invalidates anchor
	if m.InVisualMode() {
		m.mode = ModeNormal
		m.visualAnchor = Position{}
	}

	if s == "" {
		m.content = []string{""}
	} else {
		m.content = strings.Split(s, "\n")
	}
	// Clamp cursor to valid position
	m.clampCursor()
}

// Mode returns the current vim mode.
func (m Model) Mode() Mode {
	return m.mode
}

// ModeIndicator returns a styled mode indicator string (e.g., "[NORMAL]" or "[INSERT]")
// suitable for display in a UI. Each mode has a distinct color:
// - Normal: blue
// - Insert: green
// - Visual/VisualLine: purple
// Returns empty string if vim mode is disabled.
func (m Model) ModeIndicator() string {
	if !m.config.VimEnabled {
		return ""
	}

	var color lipgloss.AdaptiveColor
	switch m.mode {
	case ModeNormal:
		color = styles.VimNormalModeColor
	case ModeInsert:
		color = styles.VimInsertModeColor
	case ModeVisual, ModeVisualLine:
		color = styles.VimVisualModeColor
	case ModeReplace:
		color = styles.VimReplaceModeColor
	default:
		color = styles.TextMutedColor
	}

	style := lipgloss.NewStyle().Foreground(color)
	return style.Render("[" + m.mode.String() + "]")
}

// SetMode sets the vim mode.
func (m *Model) SetMode(mode Mode) {
	m.mode = mode
}

// CursorPosition returns the current cursor position.
func (m Model) CursorPosition() Position {
	return Position{Row: m.cursorRow, Col: m.cursorCol}
}

// SetVimEnabled enables or disables vim mode.
func (m *Model) SetVimEnabled(enabled bool) {
	m.config.VimEnabled = enabled
	if !enabled {
		// When disabling vim, switch to Insert mode
		m.mode = ModeInsert
	}
}

// VimEnabled returns whether vim mode is enabled.
func (m Model) VimEnabled() bool {
	return m.config.VimEnabled
}

// SetPlaceholder sets the placeholder text.
func (m *Model) SetPlaceholder(placeholder string) {
	m.config.Placeholder = placeholder
}

// Reset clears the content and resets the cursor position.
func (m *Model) Reset() {
	m.content = []string{""}
	m.cursorRow = 0
	m.cursorCol = 0
	m.history.Clear()
	m.pendingBuilder.Clear()
}

// ClearPendingCommand clears any pending multi-key command.
func (m *Model) ClearPendingCommand() {
	m.pendingBuilder.Clear()
}

// InVisualMode returns true if currently in any visual mode.
func (m Model) InVisualMode() bool {
	return m.mode == ModeVisual || m.mode == ModeVisualLine
}

// InNormalMode returns true if currently in normal mode.
func (m Model) InNormalMode() bool {
	return m.mode == ModeNormal
}

// InInsertMode returns true if currently in insert mode.
func (m Model) InInsertMode() bool {
	return m.mode == ModeInsert
}

// clampCursor ensures the cursor is within valid bounds.
// Note: cursorCol represents grapheme index, not byte offset.
func (m *Model) clampCursor() {
	if len(m.content) == 0 {
		m.content = []string{""}
	}
	if m.cursorRow >= len(m.content) {
		m.cursorRow = len(m.content) - 1
	}
	if m.cursorRow < 0 {
		m.cursorRow = 0
	}
	// Use grapheme count for column bounds
	lineGraphemeCount := GraphemeCount(m.content[m.cursorRow])
	if m.cursorCol > lineGraphemeCount {
		m.cursorCol = lineGraphemeCount
	}
	if m.cursorCol < 0 {
		m.cursorCol = 0
	}
}

// ============================================================================
// Visual Mode Helpers
// ============================================================================

// SelectionBounds returns normalized selection bounds (start always <= end).
// For line-wise mode, columns span the full line.
// Returns (Position{}, Position{}) if not in visual mode.
// Note: Position.Col represents grapheme index, not byte offset.
func (m Model) SelectionBounds() (start, end Position) {
	if !m.InVisualMode() {
		return Position{}, Position{}
	}

	anchor := m.visualAnchor
	cursor := Position{Row: m.cursorRow, Col: m.cursorCol}

	// Normalize: start should be before end lexicographically
	if anchor.Row < cursor.Row || (anchor.Row == cursor.Row && anchor.Col <= cursor.Col) {
		start, end = anchor, cursor
	} else {
		start, end = cursor, anchor
	}

	// For line-wise mode, extend to full lines
	if m.mode == ModeVisualLine {
		start.Col = 0
		if end.Row < len(m.content) {
			// Use grapheme count for end column (grapheme index semantics)
			end.Col = GraphemeCount(m.content[end.Row])
		}
	}

	return start, end
}

// SelectedText returns the selected text as a single string.
// Returns empty string if not in visual mode.
// Note: Selection bounds use grapheme indices, so this function uses SliceByGraphemes.
func (m Model) SelectedText() string {
	if !m.InVisualMode() {
		return ""
	}

	start, end := m.SelectionBounds()

	// Handle invalid bounds (shouldn't happen, but defensive)
	if start.Row >= len(m.content) || end.Row >= len(m.content) {
		return ""
	}

	if start.Row == end.Row {
		// Single line selection - use grapheme-aware slicing
		line := m.content[start.Row]
		lineGraphemeCount := GraphemeCount(line)
		endCol := min(end.Col+1, lineGraphemeCount)
		if start.Col >= lineGraphemeCount {
			return ""
		}
		return SliceByGraphemes(line, start.Col, endCol)
	}

	// Multi-line selection
	var lines []string

	// First line: from start.Col to end of line (grapheme-aware)
	firstLine := m.content[start.Row]
	firstLineGraphemeCount := GraphemeCount(firstLine)
	if start.Col < firstLineGraphemeCount {
		lines = append(lines, SliceByGraphemes(firstLine, start.Col, firstLineGraphemeCount))
	} else {
		lines = append(lines, "")
	}

	// Middle lines: full lines
	for row := start.Row + 1; row < end.Row; row++ {
		lines = append(lines, m.content[row])
	}

	// Last line: from start to end.Col (grapheme-aware)
	lastLine := m.content[end.Row]
	lastLineGraphemeCount := GraphemeCount(lastLine)
	endCol := min(end.Col+1, lastLineGraphemeCount)
	lines = append(lines, SliceByGraphemes(lastLine, 0, endCol))

	return strings.Join(lines, "\n")
}

// getSelectionRangeForRow returns the column range selected on a given row.
// Returns (startCol, endCol, inSelection) where endCol is exclusive.
// For rows outside the selection, returns (0, 0, false).
// Note: Column values are grapheme indices, not byte offsets.
func (m Model) getSelectionRangeForRow(row int) (startCol, endCol int, inSelection bool) {
	if !m.InVisualMode() {
		return 0, 0, false
	}

	start, end := m.SelectionBounds()

	// Check if row is in selection range
	if row < start.Row || row > end.Row {
		return 0, 0, false
	}

	// Handle empty content case
	if row >= len(m.content) {
		return 0, 0, false
	}

	line := m.content[row]
	lineGraphemeCount := GraphemeCount(line)

	if m.mode == ModeVisualLine {
		// Line-wise: entire line is selected (grapheme count)
		return 0, lineGraphemeCount, true
	}

	// Character-wise selection (grapheme indices)
	if row == start.Row && row == end.Row {
		// Single line: start.Col to end.Col+1 (exclusive)
		return start.Col, min(end.Col+1, lineGraphemeCount), true
	} else if row == start.Row {
		// First line of multi-line: start.Col to end of line
		return start.Col, lineGraphemeCount, true
	} else if row == end.Row {
		// Last line of multi-line: start of line to end.Col+1 (exclusive)
		return 0, min(end.Col+1, lineGraphemeCount), true
	} else {
		// Middle line: entire line
		return 0, lineGraphemeCount, true
	}
}

// deleteSelection deletes the text between start and end positions (inclusive).
// For line-wise mode (wasLinewise=true), deletes entire lines.
// Returns the deleted content as a slice of strings.
// After deletion, cursor is positioned at the start of the deleted region.
// Note: start.Col and end.Col are grapheme indices, not byte offsets.
func (m *Model) deleteSelection(start, end Position, wasLinewise bool) []string {
	// Capture deleted content
	var deletedContent []string

	if wasLinewise {
		// Line-wise deletion: delete entire lines from start.Row to end.Row
		for row := start.Row; row <= end.Row && row < len(m.content); row++ {
			deletedContent = append(deletedContent, m.content[row])
		}

		if len(m.content) <= end.Row-start.Row+1 {
			// Deleting all lines - leave one empty line
			m.content = []string{""}
			m.cursorRow = 0
			m.cursorCol = 0
		} else {
			// Remove lines from start.Row to end.Row
			newContent := make([]string, 0, len(m.content)-(end.Row-start.Row+1))
			newContent = append(newContent, m.content[:start.Row]...)
			newContent = append(newContent, m.content[end.Row+1:]...)
			m.content = newContent

			// Position cursor
			if start.Row >= len(m.content) {
				m.cursorRow = len(m.content) - 1
			} else {
				m.cursorRow = start.Row
			}
			// Move to first non-blank or column 0
			m.cursorCol = 0
		}
	} else {
		// Character-wise deletion using grapheme-aware slicing
		if start.Row == end.Row {
			// Single line deletion
			line := m.content[start.Row]
			lineGraphemeCount := GraphemeCount(line)
			endCol := min(end.Col+1, lineGraphemeCount)
			// Capture deleted content (grapheme slice)
			deletedContent = append(deletedContent, SliceByGraphemes(line, start.Col, endCol))
			// Rebuild line without deleted portion
			m.content[start.Row] = SliceByGraphemes(line, 0, start.Col) + SliceByGraphemes(line, endCol, lineGraphemeCount)
		} else {
			// Multi-line deletion
			// First line: capture from start.Col to end (grapheme-aware)
			firstLine := m.content[start.Row]
			firstLineGraphemeCount := GraphemeCount(firstLine)
			if start.Col < firstLineGraphemeCount {
				deletedContent = append(deletedContent, SliceByGraphemes(firstLine, start.Col, firstLineGraphemeCount))
			} else {
				deletedContent = append(deletedContent, "")
			}

			// Middle lines: capture full lines
			for row := start.Row + 1; row < end.Row && row < len(m.content); row++ {
				deletedContent = append(deletedContent, m.content[row])
			}

			// Last line: capture from start to end.Col (grapheme-aware)
			if end.Row < len(m.content) {
				lastLine := m.content[end.Row]
				lastLineGraphemeCount := GraphemeCount(lastLine)
				endCol := min(end.Col+1, lastLineGraphemeCount)
				deletedContent = append(deletedContent, SliceByGraphemes(lastLine, 0, endCol))
			}

			// Join first line prefix with last line suffix (grapheme-aware)
			firstLinePrefix := SliceByGraphemes(m.content[start.Row], 0, start.Col)

			lastLineSuffix := ""
			if end.Row < len(m.content) {
				lastLine := m.content[end.Row]
				lastLineGraphemeCount := GraphemeCount(lastLine)
				endCol := min(end.Col+1, lastLineGraphemeCount)
				if endCol < lastLineGraphemeCount {
					lastLineSuffix = SliceByGraphemes(lastLine, endCol, lastLineGraphemeCount)
				}
			}

			// Create new content
			newContent := make([]string, 0, len(m.content)-(end.Row-start.Row))
			newContent = append(newContent, m.content[:start.Row]...)
			newContent = append(newContent, firstLinePrefix+lastLineSuffix)
			if end.Row+1 < len(m.content) {
				newContent = append(newContent, m.content[end.Row+1:]...)
			}
			m.content = newContent
		}

		// Position cursor at start of deletion (grapheme index)
		m.cursorRow = start.Row
		m.cursorCol = start.Col

		// Ensure we have at least one line
		if len(m.content) == 0 {
			m.content = []string{""}
		}
	}

	// Clamp cursor to valid position using grapheme count
	if m.cursorRow >= len(m.content) {
		m.cursorRow = len(m.content) - 1
	}
	lineGraphemeCount := GraphemeCount(m.content[m.cursorRow])
	if m.cursorCol > 0 && m.cursorCol >= lineGraphemeCount {
		m.cursorCol = max(lineGraphemeCount-1, 0)
	}

	return deletedContent
}

// submitContent triggers the OnSubmit callback with the current content.
// This is the Enter key behavior.
func (m Model) submitContent() (Model, tea.Cmd) {
	content := m.Value()

	if m.config.OnSubmit != nil {
		return m, func() tea.Msg {
			return m.config.OnSubmit(content)
		}
	}

	// Default: emit SubmitMsg
	return m, func() tea.Msg {
		return SubmitMsg{Content: content}
	}
}
