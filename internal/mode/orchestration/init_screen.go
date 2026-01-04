package orchestration

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

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
	InitAwaitingFirstMessage: "Coordinator Loaded",
	InitSpawningWorkers:      "Spawning Workers",
	InitWorkersReady:         "Workers Loaded",
}

// phaseOrder defines the order in which phases are displayed.
var phaseOrder = []InitPhase{
	InitCreatingWorktree,
	InitCreatingWorkspace,
	InitSpawningCoordinator,
	InitAwaitingFirstMessage,
	InitSpawningWorkers,
	InitWorkersReady,
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

// phaseToLinkIndex maps InitPhase to chain link index (0-5).
// The 6 chain links map to the 6 loading phases:
//   - Link 0: InitCreatingWorktree
//   - Link 1: InitCreatingWorkspace
//   - Link 2: InitSpawningCoordinator
//   - Link 3: InitAwaitingFirstMessage
//   - Link 4: InitSpawningWorkers
//   - Link 5: InitWorkersReady
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
	case InitSpawningWorkers:
		return 4
	case InitWorkersReady:
		return 5
	case InitReady:
		return 6 // All phases complete, show all links colored
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

		// Append "(timed out)" suffix for the timeout case
		if currentPhase == InitTimedOut && phase == InitWorkersReady {
			label = label + " (timed out)"
		}

		// Show worktree path during CreatingWorktree phase
		if phase == InitCreatingWorktree && currentPhase == InitCreatingWorktree {
			if worktreePath := m.getWorktreePath(); worktreePath != "" {
				label = fmt.Sprintf("%s: %s", label, worktreePath)
			}
		}

		// Show worker loading progress for WorkersReady phase
		if phase == InitWorkersReady && currentPhase >= InitSpawningWorkers {
			_, _, expected, confirmed := m.getSpinnerData()
			confirmedCount := len(confirmed)
			if expected > 0 && confirmedCount > 0 && confirmedCount < expected {
				label = fmt.Sprintf("%s (%d/%d)", label, confirmedCount, expected)
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
		timeoutMsg := errorMessageStyle.Render("The workers did not load in time.")
		lines = append(lines, centerTextStyle.Render(timeoutMsg))
		lines = append(lines, centerTextStyle.Render(errorMessageStyle.Render("This may indicate an API or network issue.")))
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

// getSpinnerData returns spinner data from the Initializer.
// Returns defaults if initializer is nil.
func (m Model) getSpinnerData() (phase InitPhase, workersSpawned, expectedWorkers int, confirmedWorkers map[string]bool) {
	if m.initializer != nil {
		return m.initializer.SpinnerData()
	}
	return InitNotStarted, 0, 4, make(map[string]bool)
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

// worktreeErrorMessage returns a user-friendly error message for worktree-specific errors.
// It parses the error to identify common git worktree failure modes.
func worktreeErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	errStr := err.Error()
	switch {
	case strings.Contains(errStr, "already checked out"):
		return "Branch is already checked out in another worktree."
	case strings.Contains(errStr, "already exists"):
		return "Worktree path already exists."
	case strings.Contains(errStr, "not a git repository"):
		return "Not a git repository. Worktree feature unavailable."
	default:
		return fmt.Sprintf("Worktree creation failed: %v", err)
	}
}
