package beads

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/zjrosen/perles/internal/log"
)

// Compile-time check that RealExecutor implements BeadsExecutor.
var _ BeadsExecutor = (*RealExecutor)(nil)

// RealExecutor implements BeadsExecutor by executing actual BD commands.
type RealExecutor struct {
	workDir string
}

// NewRealExecutor creates a new RealExecutor.
func NewRealExecutor(workDir string) *RealExecutor {
	return &RealExecutor{workDir: workDir}
}

// UpdateStatus delegates to the package-level UpdateStatus function.
func (e *RealExecutor) UpdateStatus(issueID string, status Status) error {
	return UpdateStatus(issueID, status)
}

// UpdatePriority delegates to the package-level UpdatePriority function.
func (e *RealExecutor) UpdatePriority(issueID string, priority Priority) error {
	return UpdatePriority(issueID, priority)
}

// UpdateType delegates to the package-level UpdateType function.
func (e *RealExecutor) UpdateType(issueID string, issueType IssueType) error {
	return UpdateType(issueID, issueType)
}

// CloseIssue delegates to the package-level CloseIssue function.
func (e *RealExecutor) CloseIssue(issueID, reason string) error {
	return CloseIssue(issueID, reason)
}

// ReopenIssue delegates to the package-level ReopenIssue function.
func (e *RealExecutor) ReopenIssue(issueID string) error {
	return ReopenIssue(issueID)
}

// DeleteIssues delegates to the package-level DeleteIssues function.
func (e *RealExecutor) DeleteIssues(issueIDs []string) error {
	return DeleteIssues(issueIDs)
}

// SetLabels delegates to the package-level SetLabels function.
func (e *RealExecutor) SetLabels(issueID string, labels []string) error {
	return SetLabels(issueID, labels)
}

// ShowIssue executes 'bd show <id> --json' and parses the JSON array output.
func (e *RealExecutor) ShowIssue(issueID string) (*Issue, error) {
	start := time.Now()
	defer func() {
		log.Debug(log.CatBeads, "ShowIssue completed", "issueID", issueID, "duration", time.Since(start))
	}()

	//nolint:gosec // G204: issueID comes from bd database, not user input
	cmd := exec.Command("bd", "show", issueID, "--json")
	if e.workDir != "" {
		cmd.Dir = e.workDir
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			err = fmt.Errorf("bd show failed: %s", stderr.String())
			log.Error(log.CatBeads, "ShowIssue failed", "issueID", issueID, "error", err)
			return nil, err
		}
		err = fmt.Errorf("bd show failed: %w", err)
		log.Error(log.CatBeads, "ShowIssue failed", "issueID", issueID, "error", err)
		return nil, err
	}

	// bd show returns a JSON array
	var issues []Issue
	if err := json.Unmarshal(stdout.Bytes(), &issues); err != nil {
		err = fmt.Errorf("failed to parse bd show output: %w", err)
		log.Error(log.CatBeads, "ShowIssue parse failed", "issueID", issueID, "error", err)
		return nil, err
	}

	if len(issues) == 0 {
		err := fmt.Errorf("issue not found: %s", issueID)
		log.Error(log.CatBeads, "ShowIssue not found", "issueID", issueID)
		return nil, err
	}

	return &issues[0], nil
}

// AddComment executes 'bd comment <id> --author <author> -- <text>'.
func (e *RealExecutor) AddComment(issueID, author, text string) error {
	start := time.Now()
	defer func() {
		log.Debug(log.CatBeads, "AddComment completed", "issueID", issueID, "author", author, "duration", time.Since(start))
	}()

	//nolint:gosec // G204: issueID/author/text come from controlled sources
	cmd := exec.Command("bd", "comment", issueID, "--author", author, "--", text)
	if e.workDir != "" {
		cmd.Dir = e.workDir
	}

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			err = fmt.Errorf("bd comment failed: %s", stderr.String())
			log.Error(log.CatBeads, "AddComment failed", "issueID", issueID, "error", err)
			return err
		}
		err = fmt.Errorf("bd comment failed: %w", err)
		log.Error(log.CatBeads, "AddComment failed", "issueID", issueID, "error", err)
		return err
	}
	return nil
}

// UpdateStatus changes an issue's status via bd CLI.
func UpdateStatus(issueID string, status Status) error {
	start := time.Now()
	defer func() {
		log.Debug(log.CatBeads, "UpdateStatus completed", "issueID", issueID, "status", status, "duration", time.Since(start))
	}()

	//nolint:gosec // G204: issueID comes from bd database, not user input
	cmd := exec.Command("bd", "update", issueID,
		"--status", string(status), "--json")

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			err = fmt.Errorf("bd update status failed: %s", stderr.String())
			log.Error(log.CatBeads, "UpdateStatus failed", "issueID", issueID, "error", err)
			return err
		}
		err = fmt.Errorf("bd update status failed: %w", err)
		log.Error(log.CatBeads, "UpdateStatus failed", "issueID", issueID, "error", err)
		return err
	}
	return nil
}

// UpdatePriority changes an issue's priority via bd CLI.
func UpdatePriority(issueID string, priority Priority) error {
	start := time.Now()
	defer func() {
		log.Debug(log.CatBeads, "UpdatePriority completed", "issueID", issueID, "priority", priority, "duration", time.Since(start))
	}()

	//nolint:gosec // G204: issueID comes from bd database, not user input
	cmd := exec.Command("bd", "update", issueID,
		"--priority", fmt.Sprintf("%d", priority), "--json")

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			err = fmt.Errorf("bd update priority failed: %s", stderr.String())
			log.Error(log.CatBeads, "UpdatePriority failed", "issueID", issueID, "error", err)
			return err
		}
		err = fmt.Errorf("bd update priority failed: %w", err)
		log.Error(log.CatBeads, "UpdatePriority failed", "issueID", issueID, "error", err)
		return err
	}
	return nil
}

// UpdateType changes an issue's type via bd CLI.
func UpdateType(issueID string, issueType IssueType) error {
	start := time.Now()
	defer func() {
		log.Debug(log.CatBeads, "UpdateType completed", "issueID", issueID, "type", issueType, "duration", time.Since(start))
	}()

	//nolint:gosec // G204: issueID comes from bd database, not user input
	cmd := exec.Command("bd", "update", issueID,
		"--type", string(issueType), "--json")

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			err = fmt.Errorf("bd update type failed: %s", stderr.String())
			log.Error(log.CatBeads, "UpdateType failed", "issueID", issueID, "error", err)
			return err
		}
		err = fmt.Errorf("bd update type failed: %w", err)
		log.Error(log.CatBeads, "UpdateType failed", "issueID", issueID, "error", err)
		return err
	}
	return nil
}

// CloseIssue marks an issue as closed with a reason via bd CLI.
func CloseIssue(issueID, reason string) error {
	start := time.Now()
	defer func() {
		log.Debug(log.CatBeads, "CloseIssue completed", "issueID", issueID, "duration", time.Since(start))
	}()

	cmd := exec.Command("bd", "close", issueID,
		"--reason", reason, "--json")

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			err = fmt.Errorf("bd close failed: %s", stderr.String())
			log.Error(log.CatBeads, "CloseIssue failed", "issueID", issueID, "error", err)
			return err
		}
		err = fmt.Errorf("bd close failed: %w", err)
		log.Error(log.CatBeads, "CloseIssue failed", "issueID", issueID, "error", err)
		return err
	}
	return nil
}

// ReopenIssue reopens a closed issue via bd CLI.
func ReopenIssue(issueID string) error {
	start := time.Now()
	defer func() {
		log.Debug(log.CatBeads, "ReopenIssue completed", "issueID", issueID, "duration", time.Since(start))
	}()

	//nolint:gosec // G204: issueID comes from bd database, not user input
	cmd := exec.Command("bd", "update", issueID,
		"--status", string(StatusOpen), "--json")

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			err = fmt.Errorf("bd reopen failed: %s", stderr.String())
			log.Error(log.CatBeads, "ReopenIssue failed", "issueID", issueID, "error", err)
			return err
		}
		err = fmt.Errorf("bd reopen failed: %w", err)
		log.Error(log.CatBeads, "ReopenIssue failed", "issueID", issueID, "error", err)
		return err
	}
	return nil
}

// DeleteIssues deletes one or more issues in a single bd CLI call.
func DeleteIssues(issueIDs []string) error {
	if len(issueIDs) == 0 {
		return nil
	}

	start := time.Now()
	defer func() {
		log.Debug(log.CatBeads, "DeleteIssues completed",
			"count", len(issueIDs),
			"duration", time.Since(start))
	}()

	args := append([]string{"delete"}, issueIDs...)
	args = append(args, "--force", "--json")

	//nolint:gosec // G204: issueIDs come from bd database, not user input
	cmd := exec.Command("bd", args...)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			err = fmt.Errorf("bd delete failed: %s", stderr.String())
			log.Error(log.CatBeads, "DeleteIssues failed", "count", len(issueIDs), "error", err)
			return err
		}
		err = fmt.Errorf("bd delete failed: %w", err)
		log.Error(log.CatBeads, "DeleteIssues failed", "count", len(issueIDs), "error", err)
		return err
	}
	return nil
}

// SetLabels replaces all labels on an issue via bd CLI.
// Pass an empty slice to remove all labels.
func SetLabels(issueID string, labels []string) error {
	start := time.Now()
	defer func() {
		log.Debug(log.CatBeads, "SetLabels completed", "issueID", issueID, "labels", strings.Join(labels, ","), "duration", time.Since(start))
	}()

	//nolint:gosec // G204: issueID and labels come from bd database, not user input
	cmd := exec.Command("bd", "update", issueID,
		"--set-labels", strings.Join(labels, ","), "--json")

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			err = fmt.Errorf("bd update set-labels failed: %s", stderr.String())
			log.Error(log.CatBeads, "SetLabels failed", "issueID", issueID, "error", err)
			return err
		}
		err = fmt.Errorf("bd update set-labels failed: %w", err)
		log.Error(log.CatBeads, "SetLabels failed", "issueID", issueID, "error", err)
		return err
	}
	return nil
}
