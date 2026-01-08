package orchestration

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/zjrosen/perles/internal/orchestration/v2/command"
	"github.com/zjrosen/perles/internal/ui/shared/panes"
	"github.com/zjrosen/perles/internal/ui/styles"
)

// maxCommandLogEntries is the maximum number of entries to keep in the command log.
// When exceeded, oldest entries are removed (FIFO eviction).
const maxCommandLogEntries = 2000

// maxErrorDisplayLength is the maximum length for error messages before truncation.
const maxErrorDisplayLength = 200

// commandIDDisplayLength is the number of characters to show from CommandID (last N chars).
const commandIDDisplayLength = 8

// traceIDDisplayLength is the number of characters to show from TraceID (first N chars).
const traceIDDisplayLength = 8

// Command pane styles
var (
	commandTimestampStyle = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "#666666", Dark: "#696969"})

	commandSourceStyle = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "#8888FF", Dark: "#9999FF"})

	commandTypeStyle = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "#CCCCCC", Dark: "#AAAAAA"})

	commandSuccessStyle = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "#43BF6D", Dark: "#43BF6D"})

	commandFailStyle = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "#FF6B6B", Dark: "#FF8787"})

	commandDurationStyle = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "#888888", Dark: "#777777"})

	commandIDStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#555555", Dark: "#666666"})

	// commandTraceIDStyle uses muted/secondary color for trace ID (per research doc)
	commandTraceIDStyle = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "#777777", Dark: "#555555"})
)

// CommandLogEntry represents a single command in the log.
type CommandLogEntry struct {
	Timestamp   time.Time
	CommandType command.CommandType
	CommandID   string
	Source      command.CommandSource
	Success     bool
	Error       string // Converted from error for display
	Duration    time.Duration
	TraceID     string // Distributed trace ID for correlation (empty if tracing disabled)
}

// CommandPane shows the command processing log.
type CommandPane struct {
	entries       []CommandLogEntry
	viewports     map[string]viewport.Model // Use map for reference semantics
	contentDirty  bool
	hasNewContent bool // True when new content arrived while scrolled up
}

// newCommandPane creates a new CommandPane with initialized state.
func newCommandPane() CommandPane {
	vps := make(map[string]viewport.Model)
	vps[viewportKey] = viewport.New(0, 0)
	return CommandPane{
		entries:      make([]CommandLogEntry, 0),
		viewports:    vps,
		contentDirty: true, // Start dirty to trigger initial render
	}
}

// renderCommandPane renders the command log pane.
// This function is designed to be called from Model after commandPane is added.
// The pane pointer must be passed to allow viewport state updates.
func renderCommandPane(pane *CommandPane, width, height int) string {
	// Get viewport from map (will be modified by helper via pointer)
	vp := pane.viewports[viewportKey]

	// Use panes.ScrollablePane helper for viewport setup, padding, and auto-scroll
	result := panes.ScrollablePane(width, height, panes.ScrollableConfig{
		Viewport:       &vp,
		ContentDirty:   pane.contentDirty,
		HasNewContent:  pane.hasNewContent,
		MetricsDisplay: "", // No metrics for command pane
		LeftTitle:      "COMMAND LOG",
		TitleColor:     styles.TextSecondaryColor,
		BorderColor:    styles.BorderDefaultColor,
	}, func(wrapWidth int) string {
		return renderCommandContent(pane.entries, wrapWidth)
	})

	// Store updated viewport back to map (helper modified via pointer)
	pane.viewports[viewportKey] = vp

	return result
}

// renderCommandContent builds the pre-wrapped content string for the viewport.
// Format: "15:04:05 [source] command_type (id) ✓/✗ [traceID] (duration)"
// Trace ID is only shown when present (tracing enabled).
func renderCommandContent(entries []CommandLogEntry, wrapWidth int) string {
	if len(entries) == 0 {
		return ""
	}

	var content strings.Builder

	for _, entry := range entries {
		// Format timestamp
		timestamp := commandTimestampStyle.Render(entry.Timestamp.Format("15:04:05"))

		// Format source in brackets
		source := commandSourceStyle.Render("[" + string(entry.Source) + "]")

		// Format command type
		cmdType := commandTypeStyle.Render(string(entry.CommandType))

		// Format shortened command ID (last 8 chars)
		shortID := entry.CommandID
		if len(shortID) > commandIDDisplayLength {
			shortID = shortID[len(shortID)-commandIDDisplayLength:]
		}
		cmdID := commandIDStyle.Render("(" + shortID + ")")

		// Format status (success/failure)
		var status string
		if entry.Success {
			status = commandSuccessStyle.Render("✓")
		} else {
			// Truncate error message if too long
			errMsg := entry.Error
			if len(errMsg) > maxErrorDisplayLength {
				errMsg = errMsg[:maxErrorDisplayLength] + "..."
			}
			if errMsg != "" {
				status = commandFailStyle.Render("✗ " + errMsg)
			} else {
				status = commandFailStyle.Render("✗")
			}
		}

		// Format trace ID (abbreviated to first 8 chars, only show when present)
		var traceIDDisplay string
		if entry.TraceID != "" {
			shortTraceID := entry.TraceID
			if len(shortTraceID) > traceIDDisplayLength {
				shortTraceID = shortTraceID[:traceIDDisplayLength]
			}
			traceIDDisplay = " " + commandTraceIDStyle.Render("["+shortTraceID+"]")
		}

		// Format duration
		duration := commandDurationStyle.Render(fmt.Sprintf("(%s)", formatDuration(entry.Duration)))

		// Build the line
		line := fmt.Sprintf("%s %s %s %s %s%s %s", timestamp, source, cmdType, cmdID, status, traceIDDisplay, duration)

		// Apply ANSI-aware truncation if line exceeds viewport width
		if ansi.StringWidth(line) > wrapWidth {
			line = ansi.Truncate(line, wrapWidth-3, "...")
		}

		content.WriteString(line)
		content.WriteString("\n")
	}

	return strings.TrimRight(content.String(), "\n")
}

// formatDuration formats a duration for display in the command log.
// Uses milliseconds for short durations, seconds for longer ones.
func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}
