// Package chatrender provides shared chat message rendering for chat-based UIs.
// Used by both orchestration mode and the lightweight chat panel.
package chatrender

import (
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// Agent colors - consistent colors for each agent type across all panes.
var (
	CoordinatorColor = lipgloss.AdaptiveColor{Light: "#179299", Dark: "#179299"}
	WorkerColor      = lipgloss.AdaptiveColor{Light: "#43BF6D", Dark: "#43BF6D"}
	ObserverColor    = lipgloss.AdaptiveColor{Light: "#A066D3", Dark: "#A066D3"} // Purple for observer
	UserColor        = lipgloss.AdaptiveColor{Light: "#FB923C", Dark: "#FB923C"}
	SystemColor      = lipgloss.AdaptiveColor{Light: "#FF6B6B", Dark: "#FF8787"}
	// AssistantColor is an alias for CoordinatorColor, used by the chat panel.
	AssistantColor = CoordinatorColor
)

// Chat rendering styles.
var (
	// RoleStyle applies bold formatting to role labels.
	RoleStyle = lipgloss.NewStyle().Bold(true)

	// UserMessageStyle is for user message content.
	UserMessageStyle = lipgloss.NewStyle().Foreground(UserColor)

	// ToolCallStyle is for tool call display (muted).
	ToolCallStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#999999", Dark: "#666666"})
)

// Message represents a single message in chat history.
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	IsToolCall bool       `json:"is_tool_call,omitempty"`
	Timestamp  *time.Time `json:"ts,omitempty"`
}

// RenderConfig configures how chat messages are rendered.
type RenderConfig struct {
	AgentLabel              string                 // Label for agent messages (e.g., "Coordinator", "Assistant")
	AgentColor              lipgloss.AdaptiveColor // Color for the agent role label
	UserLabel               string                 // Label for user messages (default: "You")
	ShowCoordinatorInWorker bool                   // Show "Coordinator" role in worker panes for coordinator messages
}

// RenderContent renders a slice of Messages with tool call grouping.
// CRITICAL: Tool call sequence detection boundary conditions must be preserved:
// - Single tool call: Both first AND last (gets ╰╴ character)
// - First message is tool call: i == 0 check prevents index out of bounds
// - Last message is tool call: i == len-1 check prevents index out of bounds
// - Non-tool call surrounded by tool calls: Correctly breaks sequences
func RenderContent(messages []Message, wrapWidth int, cfg RenderConfig) string {
	var content strings.Builder

	// Default user label if not specified
	userLabel := cfg.UserLabel
	if userLabel == "" {
		userLabel = "You"
	}

	for i, msg := range messages {
		// Tool call sequence detection - boundary checks are critical for off-by-one safety
		isFirstToolInSequence := msg.IsToolCall && (i == 0 || !messages[i-1].IsToolCall)
		isLastToolInSequence := msg.IsToolCall && (i == len(messages)-1 || !messages[i+1].IsToolCall)

		if msg.Role == "user" {
			roleLabel := RoleStyle.Foreground(UserMessageStyle.GetForeground()).Render(userLabel)
			content.WriteString(roleLabel + "\n")
			content.WriteString(WordWrap(msg.Content, wrapWidth-4) + "\n\n")

		} else if msg.IsToolCall {
			if isFirstToolInSequence {
				roleLabel := RoleStyle.Foreground(cfg.AgentColor).Render(cfg.AgentLabel)
				content.WriteString(roleLabel + "\n")
			}

			prefix := "├╴ "
			if isLastToolInSequence {
				prefix = "╰╴ "
			}

			toolName := strings.TrimPrefix(msg.Content, "🔧 ")
			content.WriteString(ToolCallStyle.Render(prefix+toolName) + "\n")

			if isLastToolInSequence {
				content.WriteString("\n")
			}

		} else {
			// Regular text message - show role-specific label and color
			var roleLabel string
			switch {
			case msg.Role == "system":
				// System messages (enforcement reminders) get distinctive red styling
				roleLabel = RoleStyle.Foreground(SystemColor).Render("System")
			case cfg.ShowCoordinatorInWorker && msg.Role == "coordinator":
				roleLabel = RoleStyle.Foreground(CoordinatorColor).Render("Coordinator")
			default:
				roleLabel = RoleStyle.Foreground(cfg.AgentColor).Render(cfg.AgentLabel)
			}
			content.WriteString(roleLabel + "\n")
			content.WriteString(WordWrap(msg.Content, wrapWidth-4) + "\n\n")
		}
	}

	return strings.TrimRight(content.String(), "\n")
}

// WordWrap wraps text at the given width, preserving explicit newlines.
func WordWrap(text string, width int) string {
	if width <= 0 {
		return text
	}

	// Split by newlines first to preserve explicit line breaks
	lines := strings.Split(text, "\n")
	var result strings.Builder

	for lineIdx, line := range lines {
		if lineIdx > 0 {
			result.WriteString("\n")
		}

		// Skip empty lines
		if strings.TrimSpace(line) == "" {
			continue
		}

		// Word wrap this line
		words := strings.Fields(line)
		var currentLine strings.Builder

		for i, word := range words {
			// Check if adding this word would exceed line width
			needsNewLine := currentLine.Len()+len(word)+1 > width && currentLine.Len() > 0

			if needsNewLine {
				result.WriteString(currentLine.String())
				result.WriteString("\n")
				currentLine.Reset()
			}

			if currentLine.Len() > 0 {
				currentLine.WriteString(" ")
			}
			currentLine.WriteString(word)

			// Write last word of this line
			if i == len(words)-1 && currentLine.Len() > 0 {
				result.WriteString(currentLine.String())
			}
		}
	}

	return result.String()
}
