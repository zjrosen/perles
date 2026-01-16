package orchestration

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/zjrosen/perles/internal/git"
	"github.com/zjrosen/perles/internal/ui/shared/chainart"
	"github.com/zjrosen/perles/internal/ui/styles"
)

// spinnerFrames defines the braille spinner animation sequence.
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// phaseLabels maps each InitPhase to its display text.
var phaseLabels = map[InitPhase]string{
	InitCreatingWorktree:     "Creating Worktree",
	InitCreatingWorkspace:    "Creating Workspace",
	InitSpawningCoordinator:  "Spawning Coordinator",
	InitAwaitingFirstMessage: "Coordinator Ready",
}

// phaseOrder defines the order in which phases are displayed.
var phaseOrder = []InitPhase{
	InitCreatingWorktree,
	InitCreatingWorkspace,
	InitSpawningCoordinator,
	InitAwaitingFirstMessage,
}

// Phase status indicator styles.
var (
	phaseCompletedStyle = lipgloss.NewStyle().
				Foreground(styles.StatusSuccessColor)

	phaseInProgressStyle = lipgloss.NewStyle().
				Foreground(styles.StatusWarningColor)

	phaseFailedStyle = lipgloss.NewStyle().
				Foreground(styles.StatusErrorColor)

	phasePendingStyle = lipgloss.NewStyle().
				Foreground(styles.TextMutedColor)

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(styles.TextPrimaryColor)

	errorTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(styles.StatusErrorColor)

	errorMessageStyle = lipgloss.NewStyle().
				Foreground(styles.StatusErrorColor)

	hintStyle = lipgloss.NewStyle().
			Foreground(styles.TextMutedColor)
)

// phaseToLinkIndex maps InitPhase to chain link index (0-4).
// The 5 chain links map to the 4 loading phases + final state:
//   - Link 0: InitCreatingWorktree
//   - Link 1: InitCreatingWorkspace
//   - Link 2: InitSpawningCoordinator
//   - Link 3: InitAwaitingFirstMessage
//   - Link 4: InitReady (all phases complete)
func phaseToLinkIndex(phase InitPhase) int {
	switch phase {
	case InitCreatingWorktree:
		return 0
	case InitCreatingWorkspace:
		return 1
	case InitSpawningCoordinator:
		return 2
	case InitAwaitingFirstMessage:
		return 3
	case InitReady:
		return 4 // All phases complete, show all links colored
	default:
		return 0
	}
}

// renderInitScreen renders the initialization loading screen.
// This displays the chain art header, initialization phases with status indicators,
// and action hints at the bottom.
func (m Model) renderInitScreen() string {
	var lines []string

	// Get current phase from initializer
	currentPhase := m.getInitPhase()

	// Determine if we're in an error/timeout state
	isError := currentPhase == InitFailed || currentPhase == InitTimedOut

	// Calculate completed phases and failed phase for chain art
	completedPhases := phaseToLinkIndex(currentPhase)
	failedPhase := -1 // No failure by default

	if currentPhase == InitFailed || currentPhase == InitTimedOut {
		// Use getFailedPhase to determine which phase failed/timed out
		failedPhaseValue := m.getFailedPhase()
		failedPhase = phaseToLinkIndex(failedPhaseValue)
		// Completed phases are those before the failed phase
		completedPhases = failedPhase
	}

	// Chain art header - progressive coloring based on phase progress
	chainArt := chainart.BuildProgressChainArt(completedPhases, failedPhase)

	// Get chain art width for centering text elements
	chainArtWidth := lipgloss.Width(chainArt)

	// Style for centering text within the chain art width
	centerTextStyle := lipgloss.NewStyle().
		Width(chainArtWidth).
		Align(lipgloss.Center)

	lines = append(lines, chainArt)
	lines = append(lines, "") // Spacing

	// Title
	var title string
	switch currentPhase {
	case InitFailed:
		title = errorTitleStyle.Render("Initialization Failed")
	case InitTimedOut:
		title = errorTitleStyle.Render("Initialization Timed Out")
	default:
		title = titleStyle.Render("Initializing Orchestration")
	}
	lines = append(lines, centerTextStyle.Render(title))
	lines = append(lines, "") // Spacing

	// Build phase lines
	var phaseLines []string
	for _, phase := range phaseOrder {
		// Skip worktree phase if worktree is not enabled
		if phase == InitCreatingWorktree && !m.worktreeEnabled {
			continue
		}

		label := phaseLabels[phase]
		indicator, style := m.getPhaseIndicatorAndStyle(phase, currentPhase)

		// Show worktree path during CreatingWorktree phase
		if phase == InitCreatingWorktree && currentPhase == InitCreatingWorktree {
			if worktreePath := m.getWorktreePath(); worktreePath != "" {
				label = fmt.Sprintf("%s: %s", label, worktreePath)
			}
		}

		// Only color the indicator icon, not the label text
		// For completed, in-progress, and failed phases: use default text color for label
		// For pending phases: use muted style for label (no visual indicator to highlight)
		// Pending phases have whitespace-only indicators (after stripping ANSI codes)
		isPending := strings.TrimSpace(ansi.Strip(indicator)) == ""
		var line string
		if isPending {
			// Pending: use muted style for label
			line = indicator + " " + style.Render(label)
		} else {
			// Completed, in-progress, or failed: icon is already styled, label uses default color
			line = indicator + " " + label
		}
		phaseLines = append(phaseLines, line)
	}
	// Join phase lines and center the block
	phaseBlock := strings.Join(phaseLines, "\n")
	lines = append(lines, centerTextStyle.Render(phaseBlock))

	// Error message (if failed)
	actualFailedPhase := m.getFailedPhase()
	if currentPhase == InitFailed {
		if initErr := m.getInitError(); initErr != nil {
			lines = append(lines, "") // Spacing
			// Use worktree-specific error message if the failure was during worktree creation
			var errMsg string
			if actualFailedPhase == InitCreatingWorktree {
				errMsg = errorMessageStyle.Render("Error: " + worktreeErrorMessage(initErr))
			} else {
				errMsg = errorMessageStyle.Render("Error: " + initErr.Error())
			}
			lines = append(lines, centerTextStyle.Render(errMsg))
		}
	}

	// Timeout message
	if currentPhase == InitTimedOut {
		lines = append(lines, "") // Spacing
		// Use phase-specific timeout message with duration
		timeoutDuration := m.getTimeoutDuration(actualFailedPhase)
		timeoutMsg := errorMessageStyle.Render(timeoutErrorMessage(actualFailedPhase, timeoutDuration))
		lines = append(lines, centerTextStyle.Render(timeoutMsg))
	}

	lines = append(lines, "") // Spacing
	lines = append(lines, "") // Extra spacing before hints

	// Action hints
	if isError {
		hints := hintStyle.Render("[R] Retry     [ESC] Exit to Kanban")
		// Worktree-specific hints include Skip option
		if actualFailedPhase == InitCreatingWorktree {
			hints = hintStyle.Render("[R] Retry     [S] Skip (use current dir)     [ESC] Exit")
		}
		lines = append(lines, centerTextStyle.Render(hints))
	} else {
		hints := hintStyle.Render("[ESC] Cancel")
		lines = append(lines, centerTextStyle.Render(hints))
	}

	// Join all lines
	content := strings.Join(lines, "\n")

	// Center the content horizontally and vertically
	contentWidth := lipgloss.Width(content)
	contentHeight := lipgloss.Height(content)

	// Calculate horizontal padding
	leftPad := 0
	if m.width > contentWidth {
		leftPad = (m.width - contentWidth) / 2
	}

	// Calculate vertical padding
	topPad := 0
	if m.height > contentHeight {
		topPad = (m.height - contentHeight) / 2
	}

	// Apply centering
	centeredStyle := lipgloss.NewStyle().
		Width(m.width).
		Height(m.height).
		PaddingLeft(leftPad).
		PaddingTop(topPad)

	return centeredStyle.Render(content)
}

// getPhaseIndicatorAndStyle returns the styled status indicator and label style for a phase.
func (m Model) getPhaseIndicatorAndStyle(phase InitPhase, currentPhase InitPhase) (string, lipgloss.Style) {
	// Handle special states
	if currentPhase == InitReady {
		// All phases complete
		return phaseCompletedStyle.Render("✓"), phaseCompletedStyle
	}

	if currentPhase == InitFailed {
		// Find which phase failed
		failedPhase := m.getFailedPhase()
		if phase < failedPhase {
			return phaseCompletedStyle.Render("✓"), phaseCompletedStyle
		} else if phase == failedPhase {
			return phaseFailedStyle.Render("✗"), phaseFailedStyle
		}
		return phasePendingStyle.Render(" "), phasePendingStyle
	}

	if currentPhase == InitTimedOut {
		// Find which phase timed out
		failedPhase := m.getFailedPhase()
		if phase < failedPhase {
			return phaseCompletedStyle.Render("✓"), phaseCompletedStyle
		} else if phase == failedPhase {
			return phaseFailedStyle.Render("✗"), phaseFailedStyle
		}
		return phasePendingStyle.Render(" "), phasePendingStyle
	}

	// Normal loading state
	if phase < currentPhase {
		return phaseCompletedStyle.Render("✓"), phaseCompletedStyle
	} else if phase == currentPhase {
		// Spinner for current phase
		frame := spinnerFrames[m.spinnerFrame%len(spinnerFrames)]
		return phaseInProgressStyle.Render(frame), phaseInProgressStyle
	}

	// Pending phase
	return phasePendingStyle.Render(" "), phasePendingStyle
}

// getInitPhase returns the current initialization phase from the Initializer.
// Returns InitNotStarted if initializer is nil.
func (m Model) getInitPhase() InitPhase {
	if m.initializer != nil {
		return m.initializer.Phase()
	}
	return InitNotStarted
}

// getInitError returns the initialization error from the Initializer.
// Returns nil if initializer is nil.
func (m Model) getInitError() error {
	if m.initializer != nil {
		return m.initializer.Error()
	}
	return nil
}

// getFailedPhase determines which phase failed based on the current state.
// This is called when the phase is InitFailed or InitTimedOut.
func (m Model) getFailedPhase() InitPhase {
	if m.initializer != nil {
		return m.initializer.FailedAtPhase()
	}
	// Initializer should always be present; default to first phase if somehow nil
	return InitCreatingWorkspace
}

// getWorktreePath returns the worktree path from the Initializer.
// Returns empty string if initializer is nil or worktree is not enabled.
func (m Model) getWorktreePath() string {
	if m.initializer != nil {
		return m.initializer.WorktreePath()
	}
	return ""
}

// getTimeoutDuration returns the configured timeout duration for the given phase.
// Returns a default duration if initializer is nil.
func (m Model) getTimeoutDuration(phase InitPhase) time.Duration {
	if m.initializer != nil {
		timeouts := m.initializer.Timeouts()
		switch phase {
		case InitCreatingWorktree:
			return timeouts.WorktreeCreation
		case InitCreatingWorkspace:
			return timeouts.WorkspaceSetup
		case InitSpawningCoordinator, InitAwaitingFirstMessage:
			return timeouts.CoordinatorStart
		}
	}
	// Default fallback
	return 30 * time.Second
}

// worktreeErrorMessage returns a user-friendly error message for worktree-specific errors.
// It parses the error to identify common git worktree failure modes.
func worktreeErrorMessage(err error) string {
	if err == nil {
		return ""
	}

	// Check for sentinel errors first using errors.Is
	if errors.Is(err, git.ErrInvalidBranchName) {
		return "Invalid branch name. Branch names cannot contain spaces, special characters (~^:?*[), or start with a dot."
	}

	// Check for timeout errors (context deadline exceeded)
	if errors.Is(err, context.DeadlineExceeded) {
		return "Worktree creation timed out. Git may be slow due to large repository size, SSH key prompts, or NFS issues. Check for .git/index.lock files. Consider increasing orchestration.timeouts.worktree_creation in config."
	}

	errStr := err.Error()
	switch {
	case strings.Contains(errStr, "already checked out"):
		return "Branch is already checked out in another worktree."
	case strings.Contains(errStr, "already exists"):
		return "Worktree path already exists."
	case strings.Contains(errStr, "not a git repository"):
		return "Not a git repository. Worktree feature unavailable."
	case strings.Contains(errStr, "is not a valid branch name"):
		return "Invalid branch name. Branch names cannot contain spaces, special characters (~^:?*[), or start with a dot."
	default:
		return fmt.Sprintf("Worktree creation failed: %v", err)
	}
}

// timeoutErrorMessage returns a phase-specific timeout error message with actionable guidance.
// The duration parameter indicates the timeout that was exceeded.
func timeoutErrorMessage(phase InitPhase, duration time.Duration) string {
	switch phase {
	case InitCreatingWorktree:
		return fmt.Sprintf("Worktree creation timed out after %v. Git may be slow due to large repository size, SSH key prompts, or NFS issues. Consider increasing orchestration.timeouts.worktree_creation in config.", duration)
	case InitCreatingWorkspace:
		return fmt.Sprintf("Workspace setup timed out after %v. MCP server or session initialization failed. Consider increasing orchestration.timeouts.workspace_setup in config.", duration)
	case InitSpawningCoordinator, InitAwaitingFirstMessage:
		return fmt.Sprintf("Coordinator did not respond within %v. The AI service may be overloaded. Consider increasing orchestration.timeouts.coordinator_start in config.", duration)
	default:
		return fmt.Sprintf("Initialization timed out after %v.", duration)
	}
}
