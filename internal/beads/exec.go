package beads

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// UpdateStatus changes an issue's status via bd CLI.
func UpdateStatus(issueID string, status Status) error {
	cmd := exec.Command("bd", "update", issueID,
		"--status", string(status), "--json")

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return fmt.Errorf("bd update status failed: %s", stderr.String())
		}
		return fmt.Errorf("bd update status failed: %w", err)
	}
	return nil
}

// UpdatePriority changes an issue's priority via bd CLI.
func UpdatePriority(issueID string, priority Priority) error {
	cmd := exec.Command("bd", "update", issueID,
		"--priority", fmt.Sprintf("%d", priority), "--json")

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return fmt.Errorf("bd update priority failed: %s", stderr.String())
		}
		return fmt.Errorf("bd update priority failed: %w", err)
	}
	return nil
}

// UpdateType changes an issue's type via bd CLI.
func UpdateType(issueID string, issueType IssueType) error {
	cmd := exec.Command("bd", "update", issueID,
		"--type", string(issueType), "--json")

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return fmt.Errorf("bd update type failed: %s", stderr.String())
		}
		return fmt.Errorf("bd update type failed: %w", err)
	}
	return nil
}

// CloseIssue marks an issue as closed with a reason via bd CLI.
func CloseIssue(issueID, reason string) error {
	cmd := exec.Command("bd", "close", issueID,
		"--reason", reason, "--json")

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return fmt.Errorf("bd close failed: %s", stderr.String())
		}
		return fmt.Errorf("bd close failed: %w", err)
	}
	return nil
}

// ReopenIssue reopens a closed issue via bd CLI.
func ReopenIssue(issueID string) error {
	cmd := exec.Command("bd", "update", issueID,
		"--status", string(StatusOpen), "--json")

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return fmt.Errorf("bd reopen failed: %s", stderr.String())
		}
		return fmt.Errorf("bd reopen failed: %w", err)
	}
	return nil
}

// DeleteIssue deletes a single issue via bd CLI.
func DeleteIssue(issueID string) error {
	cmd := exec.Command("bd", "delete", issueID, "--force", "--json")

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return fmt.Errorf("bd delete failed: %s", stderr.String())
		}
		return fmt.Errorf("bd delete failed: %w", err)
	}
	return nil
}

// DeleteIssueCascade deletes an issue and all its dependents via bd CLI.
func DeleteIssueCascade(issueID string) error {
	cmd := exec.Command("bd", "delete", issueID, "--cascade", "--force", "--json")

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return fmt.Errorf("bd delete cascade failed: %s", stderr.String())
		}
		return fmt.Errorf("bd delete cascade failed: %w", err)
	}
	return nil
}

// SetLabels replaces all labels on an issue via bd CLI.
// Pass an empty slice to remove all labels.
func SetLabels(issueID string, labels []string) error {
	cmd := exec.Command("bd", "update", issueID,
		"--set-labels", strings.Join(labels, ","), "--json")

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return fmt.Errorf("bd update set-labels failed: %s", stderr.String())
		}
		return fmt.Errorf("bd update set-labels failed: %w", err)
	}
	return nil
}
