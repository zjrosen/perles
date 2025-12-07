package shared

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"perles/internal/beads"
	"perles/internal/ui/board"
	"perles/internal/ui/shared/modal"
	"perles/internal/ui/styles"
)

// IssueLoader provides the ability to fetch issues by their IDs.
type IssueLoader interface {
	ListIssuesByIds(ids []string) ([]beads.Issue, error)
}

// isNilInterface checks if an interface value is nil or contains a nil pointer.
func isNilInterface(i any) bool {
	if i == nil {
		return true
	}
	v := reflect.ValueOf(i)
	switch v.Kind() {
	case reflect.Ptr, reflect.Map, reflect.Chan, reflect.Slice, reflect.Func, reflect.Interface:
		return v.IsNil()
	}
	return false
}

// GetIssuesByIds fetches multiple issues in a single query and returns them as a map.
func GetIssuesByIds(loader IssueLoader, ids []string) map[string]*beads.Issue {
	result := make(map[string]*beads.Issue)
	if len(ids) == 0 || isNilInterface(loader) {
		return result
	}
	issues, err := loader.ListIssuesByIds(ids)
	if err != nil {
		return result
	}
	for i := range issues {
		result[issues[i].ID] = &issues[i]
	}
	return result
}

// CreateDeleteModal creates a confirmation modal for issue deletion.
// Returns the modal and a boolean indicating if this is a cascade delete (epic with children).
func CreateDeleteModal(issue *beads.Issue, loader IssueLoader) (modal.Model, bool) {
	// Check if this is an epic with child issues (checking both Children and Blocks for backward compatibility)
	allChildrenIDs := append([]string{}, issue.Children...)
	allChildrenIDs = append(allChildrenIDs, issue.Blocks...)
	hasChildren := issue.Type == beads.TypeEpic && len(allChildrenIDs) > 0

	if hasChildren {
		// Build list of child issues for the modal message
		childIssues := GetIssuesByIds(loader, allChildrenIDs)
		var childList strings.Builder
		issueIdStyle := lipgloss.NewStyle().Foreground(styles.TextSecondaryColor)
		for _, childID := range allChildrenIDs {
			if child, ok := childIssues[childID]; ok {
				typeText := board.GetTypeIndicator(child.Type)
				typeStyle := board.GetTypeStyle(child.Type)
				priorityText := fmt.Sprintf("[P%d]", child.Priority)
				priorityStyle := board.GetPriorityStyle(child.Priority)
				idText := fmt.Sprintf("[%s]", childID)

				line := fmt.Sprintf("  %s%s%s %s\n",
					typeStyle.Render(typeText),
					priorityStyle.Render(priorityText),
					issueIdStyle.Render(idText),
					child.TitleText)
				childList.WriteString(line)
			} else {
				childList.WriteString(fmt.Sprintf("  - %s\n", childID))
			}
		}

		message := fmt.Sprintf("Delete epic \"%s: %s\"?\n\nThis will also delete %d child issue(s):\n%s\nThis action cannot be undone.",
			issue.ID, issue.TitleText, len(allChildrenIDs), childList.String())

		return modal.New(modal.Config{
			Title:          "Delete Epic",
			Message:        message,
			ConfirmVariant: modal.ButtonDanger,
			MinWidth:       60,
		}), true
	}

	// Regular issue deletion
	message := fmt.Sprintf("Delete \"%s: %s\"?\n\nThis action cannot be undone.", issue.ID, issue.TitleText)
	return modal.New(modal.Config{
		Title:          "Delete Issue",
		Message:        message,
		ConfirmVariant: modal.ButtonDanger,
	}), false
}
