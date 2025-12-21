package shared

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"perles/internal/beads"
	"perles/internal/bql"
	"perles/internal/ui/shared/modal"
	"perles/internal/ui/styles"
)

// GetAllDescendants returns all descendant issues for an epic using BQL expand.
// The returned slice includes the root issue as the first element.
func GetAllDescendants(loader bql.BQLExecutor, rootID string) []beads.Issue {
	query := fmt.Sprintf(`id = "%s" expand down depth *`, rootID)
	issues, err := loader.Execute(query)
	if err != nil {
		return nil
	}
	return issues
}

// CreateDeleteModal creates a confirmation modal for issue deletion.
// Returns the modal and a slice of all issue IDs to delete (including descendants for epics).
func CreateDeleteModal(issue *beads.Issue, loader bql.BQLExecutor) (modal.Model, []string) {
	// Check if this is an epic with child issues
	hasChildren := issue.Type == beads.TypeEpic && len(issue.Children) > 0

	if hasChildren {
		// Get ALL descendants using BQL expand (not just immediate children)
		allDescendants := GetAllDescendants(loader, issue.ID)

		// Build ID list and display list in one pass
		allIDs := make([]string, 0, len(allDescendants))
		var childList strings.Builder
		issueIdStyle := lipgloss.NewStyle().Foreground(styles.TextSecondaryColor)

		for _, desc := range allDescendants {
			allIDs = append(allIDs, desc.ID)
			if desc.ID == issue.ID {
				continue // Skip root in display
			}
			typeText := styles.GetTypeIndicator(desc.Type)
			typeStyle := styles.GetTypeStyle(desc.Type)
			priorityText := fmt.Sprintf("[P%d]", desc.Priority)
			priorityStyle := styles.GetPriorityStyle(desc.Priority)
			idText := fmt.Sprintf("[%s]", desc.ID)

			line := fmt.Sprintf("  %s%s%s %s\n",
				typeStyle.Render(typeText),
				priorityStyle.Render(priorityText),
				issueIdStyle.Render(idText),
				desc.TitleText)
			childList.WriteString(line)
		}

		// Handle edge case where expand returned nothing
		if len(allIDs) == 0 {
			allIDs = []string{issue.ID}
		}

		message := fmt.Sprintf("Delete epic \"%s: %s\"?\n\nThis will also delete %d descendant issue(s):\n%s\nThis action cannot be undone.",
			issue.ID, issue.TitleText, len(allIDs)-1, childList.String())

		return modal.New(modal.Config{
			Title:          "Delete Epic",
			Message:        message,
			ConfirmVariant: modal.ButtonDanger,
			MinWidth:       60,
		}), allIDs
	}

	// Regular issue deletion - return single-element slice with issue ID
	message := fmt.Sprintf("Delete \"%s: %s\"?\n\nThis action cannot be undone.", issue.ID, issue.TitleText)
	return modal.New(modal.Config{
		Title:          "Delete Issue",
		Message:        message,
		ConfirmVariant: modal.ButtonDanger,
	}), []string{issue.ID}
}
