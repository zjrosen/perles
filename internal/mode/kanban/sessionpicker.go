// Package kanban implements the kanban board mode controller.
// sessionpicker.go provides the session picker UI for resuming orchestration sessions.
package kanban

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/zjrosen/perles/internal/mode"
	"github.com/zjrosen/perles/internal/mode/shared"
	"github.com/zjrosen/perles/internal/orchestration/session"
	"github.com/zjrosen/perles/internal/ui/commandpalette"
	"github.com/zjrosen/perles/internal/ui/shared/toaster"
	"github.com/zjrosen/perles/internal/ui/styles"
)

// buildSessionPickerItems converts session summaries to command palette items.
// Sessions are displayed with their application name, formatted start time, and
// descriptive metadata (relative time, worker count, status).
// Recent sessions (< 24h) are colored green, older sessions are muted.
func buildSessionPickerItems(sessions []session.SessionSummary, now time.Time) []commandpalette.Item {
	items := make([]commandpalette.Item, 0, len(sessions))

	for _, sess := range sessions {
		// Format display name as "appName - Jan 2 15:04"
		displayName := fmt.Sprintf("%s - %s", sess.ApplicationName, sess.StartTime.Format("Jan 2 15:04"))

		// Build description with relative time, worker count, status, and truncated session ID
		relTime := shared.FormatRelativeTimeFrom(sess.StartTime, now)
		shortID := sess.ID
		if len(shortID) > 8 {
			shortID = shortID[:8]
		}
		description := fmt.Sprintf("%s • %d workers • %s • %s", relTime, sess.WorkerCount, sess.Status, shortID)

		// Color recent sessions (< 24h) green, older sessions muted
		var color lipgloss.TerminalColor
		if now.Sub(sess.StartTime) < 24*time.Hour {
			color = styles.IssueFeatureColor // Green for recent
		} else {
			color = styles.TextMutedColor // Muted for older
		}

		items = append(items, commandpalette.Item{
			ID:          sess.SessionDir,
			Name:        displayName,
			Description: description,
			Color:       color,
		})
	}

	return items
}

// openSessionPicker creates and opens the session picker modal.
// It loads resumable sessions from the configured session storage,
// builds picker items, and creates a command palette for selection.
// Returns a tea.Cmd that shows a toast on error or empty sessions.
func (m Model) openSessionPicker() (Model, tea.Cmd) {
	// Build SessionPathBuilder from config
	sessionStorageCfg := m.services.Config.Orchestration.SessionStorage

	// Derive application name if not configured
	appName := sessionStorageCfg.ApplicationName
	if appName == "" {
		var gitExecutor session.GitRemoteGetter
		if m.services.GitExecutorFactory != nil {
			gitExecutor = m.services.GitExecutorFactory(m.services.WorkDir)
		}
		appName = session.DeriveApplicationName(m.services.WorkDir, gitExecutor)
	}

	pathBuilder := session.NewSessionPathBuilder(
		sessionStorageCfg.BaseDir,
		appName,
	)

	// Load resumable sessions
	sessions, err := session.ListResumableSessions(pathBuilder)
	if err != nil {
		return m, func() tea.Msg {
			return mode.ShowToastMsg{
				Message: fmt.Sprintf("Failed to load sessions: %v", err),
				Style:   toaster.StyleError,
			}
		}
	}

	// Check for empty sessions
	if len(sessions) == 0 {
		return m, func() tea.Msg {
			return mode.ShowToastMsg{
				Message: "No resumable sessions found",
				Style:   toaster.StyleInfo,
			}
		}
	}

	// Build picker items
	now := m.services.Clock.Now()
	items := buildSessionPickerItems(sessions, now)

	// Create the picker
	picker := commandpalette.New(commandpalette.Config{
		Title:       "Resume Session",
		Placeholder: "Search sessions...",
		Items:       items,
	})
	picker = picker.SetSize(m.width, m.height)
	m.sessionPicker = &picker
	m.showSessionPicker = true

	return m, nil
}
