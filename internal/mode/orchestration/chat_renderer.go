package orchestration

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// ChatRenderConfig configures how chat messages are rendered.
type ChatRenderConfig struct {
	AgentLabel              string                 // Label for agent messages (e.g., "Coordinator" or "Worker")
	AgentColor              lipgloss.AdaptiveColor // Color for the agent role label
	ShowCoordinatorInWorker bool                   // Show "Coordinator" role in worker panes for coordinator messages
}

// renderChatContent renders a slice of ChatMessages with tool call grouping.
// CRITICAL: Tool call sequence detection boundary conditions must be preserved:
// - Single tool call: Both first AND last (gets ╰╴ character)
// - First message is tool call: i == 0 check prevents index out of bounds
// - Last message is tool call: i == len-1 check prevents index out of bounds
// - Non-tool call surrounded by tool calls: Correctly breaks sequences
func renderChatContent(messages []ChatMessage, wrapWidth int, cfg ChatRenderConfig) string {
	var content strings.Builder

	for i, msg := range messages {
		// Tool call sequence detection - boundary checks are critical for off-by-one safety
		isFirstToolInSequence := msg.IsToolCall && (i == 0 || !messages[i-1].IsToolCall)
		isLastToolInSequence := msg.IsToolCall && (i == len(messages)-1 || !messages[i+1].IsToolCall)

		if msg.Role == "user" {
			roleLabel := roleStyle.Foreground(userMessageStyle.GetForeground()).Render("User")
			content.WriteString(roleLabel + "\n")
			content.WriteString(wordWrap(msg.Content, wrapWidth-4) + "\n\n")

		} else if msg.IsToolCall {
			if isFirstToolInSequence {
				roleLabel := roleStyle.Foreground(cfg.AgentColor).Render(cfg.AgentLabel)
				content.WriteString(roleLabel + "\n")
			}

			prefix := "├╴ "
			if isLastToolInSequence {
				prefix = "╰╴ "
			}

			toolName := strings.TrimPrefix(msg.Content, "🔧 ")
			content.WriteString(toolCallStyle.Render(prefix+toolName) + "\n")

			if isLastToolInSequence {
				content.WriteString("\n")
			}

		} else {
			// Regular text message - show role-specific label and color
			var roleLabel string
			switch {
			case msg.Role == "system":
				// System messages (enforcement reminders) get distinctive red styling
				roleLabel = roleStyle.Foreground(SystemColor).Render("System")
			case cfg.ShowCoordinatorInWorker && msg.Role == "coordinator":
				roleLabel = roleStyle.Foreground(CoordinatorColor).Render("Coordinator")
			default:
				roleLabel = roleStyle.Foreground(cfg.AgentColor).Render(cfg.AgentLabel)
			}
			content.WriteString(roleLabel + "\n")
			content.WriteString(wordWrap(msg.Content, wrapWidth-4) + "\n\n")
		}
	}

	return strings.TrimRight(content.String(), "\n")
}
