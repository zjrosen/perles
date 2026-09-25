package beadsrust

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/zjrosen/perles/internal/log"
	"github.com/zjrosen/perles/internal/task"
)

// BRExecutor handles write operations by executing br CLI commands.
// Read operations (ShowIssue, GetComments) are handled by SQLiteReader.
type BRExecutor struct {
	workDir  string
	beadsDir string
	// runFunc is an optional override for runBR, used in tests.
	runFunc func(args ...string) (string, error)
}

// NewBRExecutor creates a new BRExecutor.
// workDir is the working directory for command execution.
// beadsDir is the path to the .beads directory.
func NewBRExecutor(workDir, beadsDir string) *BRExecutor {
	return &BRExecutor{workDir: workDir, beadsDir: beadsDir}
}

// runBR executes a br command and returns stdout and any error.
// br writes log lines to stderr, so only stdout is captured for JSON parsing.
func (e *BRExecutor) runBR(args ...string) (string, error) {
	if e.runFunc != nil {
		return e.runFunc(args...)
	}
	//nolint:gosec // G204: args come from controlled sources
	cmd := exec.CommandContext(context.Background(), "br", args...)
	if e.workDir != "" {
		cmd.Dir = e.workDir
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return "", fmt.Errorf("br %s failed: %s", args[0], strings.TrimSpace(stderr.String()))
		}
		return "", fmt.Errorf("br %s failed: %w", args[0], err)
	}

	return strings.TrimSpace(stdout.String()), nil
}

// UpdateStatus changes an issue's status via br CLI.
func (e *BRExecutor) UpdateStatus(issueID string, status task.Status) error {
	start := time.Now()
	defer func() {
		log.Debug(log.CatBeads, "br UpdateStatus completed", "issueID", issueID, "status", status, "duration", time.Since(start))
	}()

	if _, err := e.runBR("update", issueID, "--status", string(status), "--json"); err != nil {
		log.Error(log.CatBeads, "br UpdateStatus failed", "issueID", issueID, "error", err)
		return err
	}
	return nil
}

// UpdatePriority changes an issue's priority via br CLI.
func (e *BRExecutor) UpdatePriority(issueID string, priority task.Priority) error {
	start := time.Now()
	defer func() {
		log.Debug(log.CatBeads, "br UpdatePriority completed", "issueID", issueID, "priority", priority, "duration", time.Since(start))
	}()

	if _, err := e.runBR("update", issueID, "-p", fmt.Sprintf("%d", priority), "--json"); err != nil {
		log.Error(log.CatBeads, "br UpdatePriority failed", "issueID", issueID, "error", err)
		return err
	}
	return nil
}

// UpdateType changes an issue's type via br CLI.
func (e *BRExecutor) UpdateType(issueID string, issueType task.IssueType) error {
	start := time.Now()
	defer func() {
		log.Debug(log.CatBeads, "br UpdateType completed", "issueID", issueID, "type", issueType, "duration", time.Since(start))
	}()

	if _, err := e.runBR("update", issueID, "-t", string(issueType), "--json"); err != nil {
		log.Error(log.CatBeads, "br UpdateType failed", "issueID", issueID, "error", err)
		return err
	}
	return nil
}

// UpdateTitle changes an issue's title via br CLI.
func (e *BRExecutor) UpdateTitle(issueID, title string) error {
	start := time.Now()
	defer func() {
		log.Debug(log.CatBeads, "br UpdateTitle completed", "issueID", issueID, "duration", time.Since(start))
	}()

	if _, err := e.runBR("update", issueID, "--title", title, "--json"); err != nil {
		log.Error(log.CatBeads, "br UpdateTitle failed", "issueID", issueID, "error", err)
		return err
	}
	return nil
}

// UpdateDescription changes an issue's description via br CLI.
func (e *BRExecutor) UpdateDescription(issueID, description string) error {
	start := time.Now()
	defer func() {
		log.Debug(log.CatBeads, "br UpdateDescription completed", "issueID", issueID, "duration", time.Since(start))
	}()

	if _, err := e.runBR("update", issueID, "--description", description, "--json"); err != nil {
		log.Error(log.CatBeads, "br UpdateDescription failed", "issueID", issueID, "error", err)
		return err
	}
	return nil
}

// UpdateNotes changes an issue's notes via br CLI.
func (e *BRExecutor) UpdateNotes(issueID, notes string) error {
	start := time.Now()
	defer func() {
		log.Debug(log.CatBeads, "br UpdateNotes completed", "issueID", issueID, "duration", time.Since(start))
	}()

	if _, err := e.runBR("update", issueID, "--notes", notes, "--json"); err != nil {
		log.Error(log.CatBeads, "br UpdateNotes failed", "issueID", issueID, "error", err)
		return err
	}
	return nil
}

// UpdateIssue applies field updates to an issue via br CLI.
// Only non-nil fields in opts are included. Labels are handled as a separate
// br update call because --set-labels cannot be combined with other flags.
func (e *BRExecutor) UpdateIssue(issueID string, opts task.UpdateOptions) error {
	start := time.Now()
	defer func() {
		log.Debug(log.CatBeads, "br UpdateIssue completed", "issueID", issueID, "duration", time.Since(start))
	}()

	// Build args for non-label fields.
	args := []string{"update", issueID}

	if opts.Title != nil {
		args = append(args, "--title", *opts.Title)
	}
	if opts.Description != nil {
		args = append(args, "--description", *opts.Description)
	}
	if opts.Notes != nil {
		args = append(args, "--notes", *opts.Notes)
	}
	if opts.Priority != nil {
		args = append(args, "-p", fmt.Sprintf("%d", *opts.Priority))
	}
	if opts.Status != nil {
		args = append(args, "--status", string(*opts.Status))
	}
	if opts.Assignee != nil {
		args = append(args, "--assignee", *opts.Assignee)
	}
	if opts.Type != nil {
		args = append(args, "-t", string(*opts.Type))
	}
	if opts.ParentID != nil {
		args = append(args, "--parent", *opts.ParentID)
	}

	// Execute non-label update if any fields were set.
	if len(args) > 2 {
		args = append(args, "--json")
		if _, err := e.runBR(args...); err != nil {
			log.Error(log.CatBeads, "br UpdateIssue failed", "issueID", issueID, "error", err)
			return fmt.Errorf("saving issue %s: %w", issueID, err)
		}
	}

	// Labels require a separate br update call.
	if opts.Labels != nil {
		if err := e.SetLabels(issueID, *opts.Labels); err != nil {
			return fmt.Errorf("saving issue %s labels: %w", issueID, err)
		}
	}

	return nil
}

// CloseIssue marks an issue as closed with a reason via br CLI.
func (e *BRExecutor) CloseIssue(issueID, reason string) error {
	start := time.Now()
	defer func() {
		log.Debug(log.CatBeads, "br CloseIssue completed", "issueID", issueID, "duration", time.Since(start))
	}()

	args := []string{"close", issueID, "--force", "--json"}
	if reason != "" {
		args = append(args, "--reason", reason)
	}
	if _, err := e.runBR(args...); err != nil {
		log.Error(log.CatBeads, "br CloseIssue failed", "issueID", issueID, "error", err)
		return err
	}
	return nil
}

// ReopenIssue reopens a closed issue via br CLI.
func (e *BRExecutor) ReopenIssue(issueID string) error {
	start := time.Now()
	defer func() {
		log.Debug(log.CatBeads, "br ReopenIssue completed", "issueID", issueID, "duration", time.Since(start))
	}()

	if _, err := e.runBR("reopen", issueID, "--json"); err != nil {
		log.Error(log.CatBeads, "br ReopenIssue failed", "issueID", issueID, "error", err)
		return err
	}
	return nil
}

// DeleteIssues deletes one or more issues via br CLI.
func (e *BRExecutor) DeleteIssues(issueIDs []string) error {
	if len(issueIDs) == 0 {
		return nil
	}

	start := time.Now()
	defer func() {
		log.Debug(log.CatBeads, "br DeleteIssues completed", "count", len(issueIDs), "duration", time.Since(start))
	}()

	args := append([]string{"delete"}, issueIDs...)
	args = append(args, "--force", "--json")

	if _, err := e.runBR(args...); err != nil {
		log.Error(log.CatBeads, "br DeleteIssues failed", "count", len(issueIDs), "error", err)
		return err
	}
	return nil
}

// SetLabels replaces all labels on an issue via br CLI.
// Pass an empty slice (or nil) to remove all labels.
func (e *BRExecutor) SetLabels(issueID string, labels []string) error {
	start := time.Now()
	defer func() {
		log.Debug(log.CatBeads, "br SetLabels completed", "issueID", issueID, "labels", strings.Join(labels, ","), "duration", time.Since(start))
	}()

	if len(labels) > 0 {
		args := []string{"update", issueID}
		for _, l := range labels {
			args = append(args, "--set-labels", l)
		}
		args = append(args, "--json")
		if _, err := e.runBR(args...); err != nil {
			log.Error(log.CatBeads, "br SetLabels failed", "issueID", issueID, "error", err)
			return err
		}
		return nil
	}

	// Empty labels: fetch current labels via br show and remove each.
	output, err := e.runBR("show", issueID, "--json")
	if err != nil {
		return fmt.Errorf("fetching issue %s for label clearing: %w", issueID, err)
	}
	var issues []Issue
	if err := json.Unmarshal([]byte(output), &issues); err != nil {
		return fmt.Errorf("parsing issue %s: %w", issueID, err)
	}
	if len(issues) == 0 || len(issues[0].Labels) == 0 {
		return nil // already clear
	}
	args := []string{"update", issueID}
	for _, l := range issues[0].Labels {
		args = append(args, "--remove-label", l)
	}
	args = append(args, "--json")
	if _, err := e.runBR(args...); err != nil {
		log.Error(log.CatBeads, "br SetLabels (clear) failed", "issueID", issueID, "error", err)
		return err
	}
	return nil
}

// AddComment adds a comment to an issue via br CLI.
func (e *BRExecutor) AddComment(issueID, author, text string) error {
	start := time.Now()
	defer func() {
		log.Debug(log.CatBeads, "br AddComment completed", "issueID", issueID, "author", author, "duration", time.Since(start))
	}()

	args := []string{"comments", "add", issueID, text, "--json"}
	if author != "" {
		args = append(args, "--actor", author)
	}
	if _, err := e.runBR(args...); err != nil {
		log.Error(log.CatBeads, "br AddComment failed", "issueID", issueID, "error", err)
		return err
	}
	return nil
}

// CreateEpic creates a new epic via br CLI.
func (e *BRExecutor) CreateEpic(title, description string, labels []string) (task.CreateResult, error) {
	start := time.Now()
	defer func() {
		log.Debug(log.CatBeads, "br CreateEpic completed", "title", title, "duration", time.Since(start))
	}()

	args := []string{"create", title, "-t", "epic", "--json"}
	if description != "" {
		args = append(args, "--description", description)
	}
	if len(labels) > 0 {
		args = append(args, "-l", strings.Join(labels, ","))
	}

	output, err := e.runBR(args...)
	if err != nil {
		log.Error(log.CatBeads, "br CreateEpic failed", "title", title, "error", err)
		return task.CreateResult{}, err
	}

	var result CreateResult
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		err = fmt.Errorf("failed to parse br create output: %w", err)
		log.Error(log.CatBeads, "br CreateEpic parse failed", "error", err)
		return task.CreateResult{}, err
	}

	return task.CreateResult{ID: result.ID, Title: result.Title}, nil
}

// CreateTask creates a new task as a child of an epic via br CLI.
func (e *BRExecutor) CreateTask(title, description, parentID, assignee string, labels []string) (task.CreateResult, error) {
	start := time.Now()
	defer func() {
		log.Debug(log.CatBeads, "br CreateTask completed", "title", title, "parentID", parentID, "assignee", assignee, "duration", time.Since(start))
	}()

	args := []string{"create", title, "-t", "task", "--json"}
	if parentID != "" {
		args = append(args, "--parent", parentID)
	}
	if description != "" {
		args = append(args, "--description", description)
	}
	if assignee != "" {
		args = append(args, "-a", assignee)
	}
	if len(labels) > 0 {
		args = append(args, "-l", strings.Join(labels, ","))
	}

	output, err := e.runBR(args...)
	if err != nil {
		log.Error(log.CatBeads, "br CreateTask failed", "title", title, "error", err)
		return task.CreateResult{}, err
	}

	var result CreateResult
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		err = fmt.Errorf("failed to parse br create output: %w", err)
		log.Error(log.CatBeads, "br CreateTask parse failed", "error", err)
		return task.CreateResult{}, err
	}

	return task.CreateResult{ID: result.ID, Title: result.Title}, nil
}

// AddDependency adds a dependency between two tasks via br CLI.
func (e *BRExecutor) AddDependency(taskID, dependsOnID string) error {
	start := time.Now()
	defer func() {
		log.Debug(log.CatBeads, "br AddDependency completed", "taskID", taskID, "dependsOnID", dependsOnID, "duration", time.Since(start))
	}()

	if _, err := e.runBR("dep", "add", taskID, dependsOnID, "--json"); err != nil {
		log.Error(log.CatBeads, "br AddDependency failed", "taskID", taskID, "dependsOnID", dependsOnID, "error", err)
		return err
	}
	return nil
}
