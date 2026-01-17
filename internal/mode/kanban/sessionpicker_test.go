package kanban

import (
	"os"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/config"
	"github.com/zjrosen/perles/internal/flags"
	"github.com/zjrosen/perles/internal/git"
	"github.com/zjrosen/perles/internal/mocks"
	"github.com/zjrosen/perles/internal/mode"
	"github.com/zjrosen/perles/internal/orchestration/session"
	"github.com/zjrosen/perles/internal/ui/styles"
)

// TestBuildSessionPickerItems_EmptySessions verifies empty input returns empty output.
func TestBuildSessionPickerItems_EmptySessions(t *testing.T) {
	now := time.Date(2026, 1, 12, 15, 0, 0, 0, time.UTC)
	items := buildSessionPickerItems([]session.SessionSummary{}, now)

	require.Empty(t, items, "expected empty items for empty sessions")
}

// TestBuildSessionPickerItems_SingleSession verifies a single session is converted correctly.
func TestBuildSessionPickerItems_SingleSession(t *testing.T) {
	now := time.Date(2026, 1, 12, 15, 0, 0, 0, time.UTC)
	startTime := now.Add(-2 * time.Hour) // 2 hours ago (recent)

	sessions := []session.SessionSummary{
		{
			ID:              "abc123",
			ApplicationName: "perles",
			SessionDir:      "/home/user/.perles/sessions/perles/2026-01-12/abc123",
			StartTime:       startTime,
			Status:          session.StatusCompleted,
			WorkerCount:     3,
		},
	}

	items := buildSessionPickerItems(sessions, now)

	require.Len(t, items, 1)
	item := items[0]

	// Check ID is session directory
	require.Equal(t, "/home/user/.perles/sessions/perles/2026-01-12/abc123", item.ID)

	// Check name format: "appName - Jan 2 15:04"
	require.Equal(t, "perles - Jan 12 13:00", item.Name)

	// Check description includes relative time, worker count, status, and truncated session ID
	require.Contains(t, item.Description, "2h ago")
	require.Contains(t, item.Description, "3 workers")
	require.Contains(t, item.Description, "completed")
	require.Contains(t, item.Description, "abc123") // Session ID (< 8 chars, displayed as-is)

	// Check color is green for recent session
	require.Equal(t, styles.IssueFeatureColor, item.Color)
}

// TestBuildSessionPickerItems_MultipleSessions verifies multiple sessions are converted correctly.
func TestBuildSessionPickerItems_MultipleSessions(t *testing.T) {
	now := time.Date(2026, 1, 12, 15, 0, 0, 0, time.UTC)

	sessions := []session.SessionSummary{
		{
			ID:              "session1",
			ApplicationName: "app1",
			SessionDir:      "/sessions/app1/session1",
			StartTime:       now.Add(-1 * time.Hour),
			Status:          session.StatusRunning,
			WorkerCount:     2,
		},
		{
			ID:              "session2",
			ApplicationName: "app2",
			SessionDir:      "/sessions/app2/session2",
			StartTime:       now.Add(-5 * time.Hour),
			Status:          session.StatusCompleted,
			WorkerCount:     4,
		},
		{
			ID:              "session3",
			ApplicationName: "app3",
			SessionDir:      "/sessions/app3/session3",
			StartTime:       now.Add(-48 * time.Hour),
			Status:          session.StatusFailed,
			WorkerCount:     1,
		},
	}

	items := buildSessionPickerItems(sessions, now)

	require.Len(t, items, 3)

	// First session - 1 hour ago
	require.Equal(t, "/sessions/app1/session1", items[0].ID)
	require.Equal(t, "app1 - Jan 12 14:00", items[0].Name)
	require.Contains(t, items[0].Description, "1h ago")
	require.Contains(t, items[0].Description, "session1") // Session ID (< 8 chars)

	// Second session - 5 hours ago
	require.Equal(t, "/sessions/app2/session2", items[1].ID)
	require.Equal(t, "app2 - Jan 12 10:00", items[1].Name)
	require.Contains(t, items[1].Description, "5h ago")
	require.Contains(t, items[1].Description, "session2") // Session ID (< 8 chars)

	// Third session - 48 hours ago
	require.Equal(t, "/sessions/app3/session3", items[2].ID)
	require.Equal(t, "app3 - Jan 10 15:00", items[2].Name)
	require.Contains(t, items[2].Description, "2d ago")
	require.Contains(t, items[2].Description, "session3") // Session ID (< 8 chars)
}

// TestBuildSessionPickerItems_ColorByRecency verifies sessions are colored by recency.
func TestBuildSessionPickerItems_ColorByRecency(t *testing.T) {
	now := time.Date(2026, 1, 12, 15, 0, 0, 0, time.UTC)

	sessions := []session.SessionSummary{
		{
			ID:              "recent",
			ApplicationName: "app",
			SessionDir:      "/sessions/recent",
			StartTime:       now.Add(-12 * time.Hour), // 12 hours ago (< 24h = green)
			Status:          session.StatusCompleted,
			WorkerCount:     1,
		},
		{
			ID:              "old",
			ApplicationName: "app",
			SessionDir:      "/sessions/old",
			StartTime:       now.Add(-48 * time.Hour), // 48 hours ago (> 24h = muted)
			Status:          session.StatusCompleted,
			WorkerCount:     1,
		},
	}

	items := buildSessionPickerItems(sessions, now)

	require.Len(t, items, 2)

	// Recent session gets green color
	require.Equal(t, styles.IssueFeatureColor, items[0].Color)

	// Old session gets muted color
	require.Equal(t, styles.TextMutedColor, items[1].Color)
}

// TestBuildSessionPickerItems_ExactBoundary24Hours verifies behavior at exactly 24 hours.
func TestBuildSessionPickerItems_ExactBoundary24Hours(t *testing.T) {
	now := time.Date(2026, 1, 12, 15, 0, 0, 0, time.UTC)

	sessions := []session.SessionSummary{
		{
			ID:              "exactly24h",
			ApplicationName: "app",
			SessionDir:      "/sessions/exactly24h",
			StartTime:       now.Add(-24 * time.Hour), // Exactly 24 hours ago (= 24h = muted)
			Status:          session.StatusCompleted,
			WorkerCount:     1,
		},
		{
			ID:              "justUnder24h",
			ApplicationName: "app",
			SessionDir:      "/sessions/justUnder24h",
			StartTime:       now.Add(-23*time.Hour - 59*time.Minute), // Just under 24 hours
			Status:          session.StatusCompleted,
			WorkerCount:     1,
		},
	}

	items := buildSessionPickerItems(sessions, now)

	require.Len(t, items, 2)

	// Exactly 24h gets muted (>= 24h)
	require.Equal(t, styles.TextMutedColor, items[0].Color)

	// Just under 24h gets green (< 24h)
	require.Equal(t, styles.IssueFeatureColor, items[1].Color)
}

// TestBuildSessionPickerItems_SessionIDTruncation verifies session IDs are truncated to 8 chars.
func TestBuildSessionPickerItems_SessionIDTruncation(t *testing.T) {
	now := time.Date(2026, 1, 12, 15, 0, 0, 0, time.UTC)

	sessions := []session.SessionSummary{
		{
			ID:              "abcdefghijklmnop", // 16 chars, should be truncated to "abcdefgh"
			ApplicationName: "app",
			SessionDir:      "/sessions/long",
			StartTime:       now.Add(-1 * time.Hour),
			Status:          session.StatusCompleted,
			WorkerCount:     1,
		},
		{
			ID:              "short", // 5 chars, should display as-is
			ApplicationName: "app",
			SessionDir:      "/sessions/short",
			StartTime:       now.Add(-2 * time.Hour),
			Status:          session.StatusCompleted,
			WorkerCount:     1,
		},
		{
			ID:              "exactly8", // Exactly 8 chars, should display as-is
			ApplicationName: "app",
			SessionDir:      "/sessions/exact",
			StartTime:       now.Add(-3 * time.Hour),
			Status:          session.StatusCompleted,
			WorkerCount:     1,
		},
	}

	items := buildSessionPickerItems(sessions, now)

	require.Len(t, items, 3)

	// Long ID (16 chars) truncated to first 8 chars
	require.Contains(t, items[0].Description, "abcdefgh")
	require.NotContains(t, items[0].Description, "abcdefghi") // Verify truncation

	// Short ID (5 chars) displayed as-is
	require.Contains(t, items[1].Description, "short")

	// Exactly 8 chars displayed as-is
	require.Contains(t, items[2].Description, "exactly8")
}

// createTestServices creates mode.Services for session picker testing.
func createTestServices(t *testing.T, cfg *config.Config, clock *mocks.MockClock, gitExecutor *mocks.MockGitExecutor) mode.Services {
	mockExecutor := mocks.NewMockBQLExecutor(t)
	clipboard := mocks.NewMockClipboard(t)
	clipboard.EXPECT().Copy(mock.Anything).Return(nil).Maybe()

	services := mode.Services{
		Config:    cfg,
		Clipboard: clipboard,
		Executor:  mockExecutor,
		Clock:     clock,
		WorkDir:   "/test/workdir",
	}

	if gitExecutor != nil {
		services.GitExecutorFactory = func(path string) git.GitExecutor {
			return gitExecutor
		}
	}

	return services
}

// TestOpenSessionPicker_NoSessions verifies toast shown when no sessions available.
func TestOpenSessionPicker_NoSessions(t *testing.T) {
	// Create temp directory for session storage
	tempDir := t.TempDir()

	clock := mocks.NewMockClock(t)
	clock.EXPECT().Now().Return(time.Date(2026, 1, 12, 15, 0, 0, 0, time.UTC)).Maybe()

	cfg := config.Defaults()
	cfg.Orchestration.SessionStorage = config.SessionStorageConfig{
		BaseDir:         tempDir,
		ApplicationName: "testapp",
	}

	services := createTestServices(t, &cfg, clock, nil)

	m := Model{
		services: services,
		width:    100,
		height:   40,
		view:     ViewBoard,
	}

	// Call openSessionPicker
	m, cmd := m.openSessionPicker()

	// Should not show picker
	require.False(t, m.showSessionPicker)
	require.Nil(t, m.sessionPicker)

	// Should return a command that produces a toast
	require.NotNil(t, cmd)
	msg := cmd()
	toastMsg, ok := msg.(mode.ShowToastMsg)
	require.True(t, ok, "expected ShowToastMsg")
	require.Equal(t, "No resumable sessions found", toastMsg.Message)
}

// createResumableTestSession creates a resumable session in the test directory.
// This is a helper that mirrors the pattern used in session/discovery_test.go.
func createResumableTestSession(t *testing.T, pathBuilder *session.SessionPathBuilder, id string, startTime time.Time, workerCount int) string {
	t.Helper()

	sessionDir := pathBuilder.SessionDir(id, startTime)
	entry := session.SessionIndexEntry{
		ID:              id,
		StartTime:       startTime,
		EndTime:         startTime.Add(time.Hour),
		Status:          session.StatusCompleted,
		SessionDir:      sessionDir,
		WorkerCount:     workerCount,
		ApplicationName: pathBuilder.ApplicationName(),
		WorkDir:         "/test/project",
	}

	metadata := &session.Metadata{
		SessionID:             id,
		StartTime:             startTime,
		EndTime:               startTime.Add(time.Hour),
		Status:                session.StatusCompleted,
		SessionDir:            sessionDir,
		Resumable:             true,
		CoordinatorSessionRef: "coord-ref-" + id,
		ApplicationName:       pathBuilder.ApplicationName(),
		WorkDir:               "/test/project",
	}

	// Create session directory
	require.NoError(t, os.MkdirAll(sessionDir, 0750))

	// Save metadata
	require.NoError(t, metadata.Save(sessionDir))

	// Load or create application index and add entry
	indexPath := pathBuilder.ApplicationIndexPath()
	appIndex, err := session.LoadApplicationIndex(indexPath)
	require.NoError(t, err)

	appIndex.Sessions = append(appIndex.Sessions, entry)
	appIndex.ApplicationName = pathBuilder.ApplicationName()

	require.NoError(t, session.SaveApplicationIndex(indexPath, appIndex))

	return sessionDir
}

// TestOpenSessionPicker_Success verifies picker is created with session items.
func TestOpenSessionPicker_Success(t *testing.T) {
	tempDir := t.TempDir()
	appName := "testapp"

	// Create test sessions using the session package helpers
	pathBuilder := session.NewSessionPathBuilder(tempDir, appName)
	now := time.Date(2026, 1, 12, 15, 0, 0, 0, time.UTC)

	// Create two resumable sessions at different times
	sessionDir1 := createResumableTestSession(t, pathBuilder, "session-1", now.Add(-2*time.Hour), 2)
	sessionDir2 := createResumableTestSession(t, pathBuilder, "session-2", now.Add(-1*time.Hour), 3)

	clock := mocks.NewMockClock(t)
	clock.EXPECT().Now().Return(now).Maybe()

	cfg := config.Defaults()
	cfg.Orchestration.SessionStorage = config.SessionStorageConfig{
		BaseDir:         tempDir,
		ApplicationName: appName,
	}

	services := createTestServices(t, &cfg, clock, nil)

	m := Model{
		services: services,
		width:    100,
		height:   40,
		view:     ViewBoard,
	}

	// Call openSessionPicker
	m, cmd := m.openSessionPicker()

	// Should show picker
	require.True(t, m.showSessionPicker, "expected showSessionPicker to be true")
	require.NotNil(t, m.sessionPicker, "expected sessionPicker to not be nil")

	// Should return nil command (no error, no empty toast)
	require.Nil(t, cmd, "expected nil command when sessions exist")

	// Verify picker contains expected items
	// The picker should have 2 items (both sessions are resumable)
	items := m.sessionPicker.FilteredItems()
	require.Len(t, items, 2, "expected 2 items in picker")

	// Sessions are sorted by recency (most recent first)
	// session-2 is more recent (1 hour ago) than session-1 (2 hours ago)
	require.Equal(t, sessionDir2, items[0].ID, "expected most recent session first")
	require.Equal(t, sessionDir1, items[1].ID, "expected older session second")

	// Verify item names contain app name and formatted time
	require.Contains(t, items[0].Name, appName)
	require.Contains(t, items[1].Name, appName)

	// Verify descriptions contain worker counts
	require.Contains(t, items[0].Description, "3 workers")
	require.Contains(t, items[1].Description, "2 workers")
}

// TestOpenSessionPicker_DeriveApplicationName verifies app name is derived when not configured.
func TestOpenSessionPicker_DeriveApplicationName(t *testing.T) {
	tempDir := t.TempDir()

	clock := mocks.NewMockClock(t)
	clock.EXPECT().Now().Return(time.Date(2026, 1, 12, 15, 0, 0, 0, time.UTC)).Maybe()

	// Create mock git executor that returns a remote URL
	gitExecutor := mocks.NewMockGitExecutor(t)
	gitExecutor.EXPECT().GetRemoteURL("origin").Return("git@github.com:user/myrepo.git", nil).Maybe()

	cfg := config.Defaults()
	cfg.Orchestration.SessionStorage = config.SessionStorageConfig{
		BaseDir:         tempDir,
		ApplicationName: "", // Empty to trigger derivation
	}

	mockBQLExecutor := mocks.NewMockBQLExecutor(t)
	clipboard := mocks.NewMockClipboard(t)
	clipboard.EXPECT().Copy(mock.Anything).Return(nil).Maybe()

	services := mode.Services{
		Config:    &cfg,
		Clipboard: clipboard,
		Executor:  mockBQLExecutor,
		Clock:     clock,
		WorkDir:   "/test/workdir",
		GitExecutorFactory: func(path string) git.GitExecutor {
			return gitExecutor
		},
	}

	m := Model{
		services: services,
		width:    100,
		height:   40,
		view:     ViewBoard,
	}

	// openSessionPicker should use derived app name (no sessions, but shouldn't panic)
	m, cmd := m.openSessionPicker()

	// With no sessions, should show toast
	require.NotNil(t, cmd)
	require.False(t, m.showSessionPicker)
}

// TestHandleBoardKey_OrchestrateResume_FlagDisabled verifies ctrl+r does nothing when flag is disabled.
func TestHandleBoardKey_OrchestrateResume_FlagDisabled(t *testing.T) {
	tests := []struct {
		name  string
		flags *flags.Registry
	}{
		{
			name:  "nil flags registry returns nil",
			flags: nil,
		},
		{
			name:  "flag explicitly false returns nil",
			flags: flags.New(map[string]bool{flags.FlagSessionResume: false}),
		},
		{
			name:  "unrelated flags only returns nil",
			flags: flags.New(map[string]bool{"other-flag": true}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.Defaults()
			clipboard := mocks.NewMockClipboard(t)
			clipboard.EXPECT().Copy(mock.Anything).Return(nil).Maybe()
			mockExecutor := mocks.NewMockBQLExecutor(t)

			services := mode.Services{
				Config:    &cfg,
				Clipboard: clipboard,
				Executor:  mockExecutor,
				Flags:     tt.flags,
			}

			m := Model{
				services: services,
				width:    100,
				height:   40,
				view:     ViewBoard,
			}

			// Simulate ctrl+r key press
			msg := tea.KeyMsg{Type: tea.KeyCtrlR}
			_, cmd := m.handleBoardKey(msg)

			// Should return nil (no-op) when flag is disabled
			require.Nil(t, cmd, "expected nil command when flag disabled")
		})
	}
}

// TestHandleBoardKey_OrchestrateResume_FlagEnabled verifies ctrl+r opens session picker when flag is enabled.
func TestHandleBoardKey_OrchestrateResume_FlagEnabled(t *testing.T) {
	tempDir := t.TempDir()

	clock := mocks.NewMockClock(t)
	clock.EXPECT().Now().Return(time.Date(2026, 1, 12, 15, 0, 0, 0, time.UTC)).Maybe()

	cfg := config.Defaults()
	cfg.Orchestration.SessionStorage = config.SessionStorageConfig{
		BaseDir:         tempDir,
		ApplicationName: "testapp",
	}

	clipboard := mocks.NewMockClipboard(t)
	clipboard.EXPECT().Copy(mock.Anything).Return(nil).Maybe()
	mockExecutor := mocks.NewMockBQLExecutor(t)

	// Enable the session-resume flag
	flagRegistry := flags.New(map[string]bool{flags.FlagSessionResume: true})

	services := mode.Services{
		Config:    &cfg,
		Clipboard: clipboard,
		Executor:  mockExecutor,
		Clock:     clock,
		Flags:     flagRegistry,
		WorkDir:   "/test/workdir",
	}

	m := Model{
		services: services,
		width:    100,
		height:   40,
		view:     ViewBoard,
	}

	// Simulate ctrl+r key press
	msg := tea.KeyMsg{Type: tea.KeyCtrlR}
	m, cmd := m.handleBoardKey(msg)

	// With flag enabled but no sessions, should show "No resumable sessions found" toast
	// (verifies the keybinding proceeded to openSessionPicker, not blocked by flag check)
	require.NotNil(t, cmd, "expected command to be returned")
	result := cmd()
	toastMsg, ok := result.(mode.ShowToastMsg)
	require.True(t, ok, "expected ShowToastMsg, got %T", result)
	require.Equal(t, "No resumable sessions found", toastMsg.Message, "expected 'no sessions' message when flag enabled")
	require.False(t, m.showSessionPicker, "expected picker not shown when no sessions")
}
