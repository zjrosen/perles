package infrastructure

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	appbeads "github.com/zjrosen/perles/internal/beads/application"
	domain "github.com/zjrosen/perles/internal/beads/domain"
	"github.com/zjrosen/perles/internal/log"
)

// Compile-time check that BDExecutor implements IssueExecutor.
var _ appbeads.IssueExecutor = (*BDExecutor)(nil)

// BDExecutor implements IssueExecutor by executing actual BD CLI commands.
type BDExecutor struct {
	workDir  string
	beadsDir string
	// runFunc is an optional override for runBeads, used in tests.
	runFunc func(args ...string) (string, error)
}

// NewBDExecutor creates a new BDExecutor.
// workDir is the working directory for command execution.
// beadsDir is the path to the .beads directory (sets BEADS_DIR env var).
func NewBDExecutor(workDir, beadsDir string) *BDExecutor {
	return &BDExecutor{workDir: workDir, beadsDir: beadsDir}
}

// runBeads executes a bd command and returns stdout and any error.
func (e *BDExecutor) runBeads(args ...string) (string, error) {
	if e.runFunc != nil {
		return e.runFunc(args...)
	}
	//nolint:gosec // G204: args come from controlled sources
	cmd := exec.CommandContext(context.Background(), "bd", args...)
	if e.workDir != "" {
		cmd.Dir = e.workDir
	}
	if e.beadsDir != "" {
		cmd.Env = append(os.Environ(), "BEADS_DIR="+e.beadsDir)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return "", fmt.Errorf("bd %s failed: %s", args[0], strings.TrimSpace(stderr.String()))
		}
		return "", fmt.Errorf("bd %s failed: %w", args[0], err)
	}

	return strings.TrimSpace(stdout.String()), nil
}

// UpdateStatus changes an issue's status via bd CLI.
func (e *BDExecutor) UpdateStatus(issueID string, status domain.Status) error {
	start := time.Now()
	defer func() {
		log.Debug(log.CatBeads, "UpdateStatus completed", "issueID", issueID, "status", status, "duration", time.Since(start))
	}()

	if _, err := e.runBeads("update", issueID, "--status", string(status), "--json"); err != nil {
		log.Error(log.CatBeads, "UpdateStatus failed", "issueID", issueID, "error", err)
		return err
	}
	return nil
}

// UpdatePriority changes an issue's priority via bd CLI.
func (e *BDExecutor) UpdatePriority(issueID string, priority domain.Priority) error {
	start := time.Now()
	defer func() {
		log.Debug(log.CatBeads, "UpdatePriority completed", "issueID", issueID, "priority", priority, "duration", time.Since(start))
	}()

	if _, err := e.runBeads("update", issueID, "--priority", fmt.Sprintf("%d", priority), "--json"); err != nil {
		log.Error(log.CatBeads, "UpdatePriority failed", "issueID", issueID, "error", err)
		return err
	}
	return nil
}

// UpdateType changes an issue's type via bd CLI.
func (e *BDExecutor) UpdateType(issueID string, issueType domain.IssueType) error {
	start := time.Now()
	defer func() {
		log.Debug(log.CatBeads, "UpdateType completed", "issueID", issueID, "type", issueType, "duration", time.Since(start))
	}()

	if _, err := e.runBeads("update", issueID, "--type", string(issueType), "--json"); err != nil {
		log.Error(log.CatBeads, "UpdateType failed", "issueID", issueID, "error", err)
		return err
	}
	return nil
}

// UpdateTitle changes an issue's title via bd CLI.
func (e *BDExecutor) UpdateTitle(issueID, title string) error {
	start := time.Now()
	defer func() {
		log.Debug(log.CatBeads, "UpdateTitle completed", "issueID", issueID, "duration", time.Since(start))
	}()

	if _, err := e.runBeads("update", issueID, "--title", title, "--json"); err != nil {
		log.Error(log.CatBeads, "UpdateTitle failed", "issueID", issueID, "error", err)
		return err
	}
	return nil
}

// UpdateDescription changes an issue's description via bd CLI.
func (e *BDExecutor) UpdateDescription(issueID, description string) error {
	start := time.Now()
	defer func() {
		log.Debug(log.CatBeads, "UpdateDescription completed", "issueID", issueID, "duration", time.Since(start))
	}()

	if _, err := e.runBeads("update", issueID, "--description", description, "--json"); err != nil {
		log.Error(log.CatBeads, "UpdateDescription failed", "issueID", issueID, "error", err)
		return err
	}
	return nil
}

// UpdateNotes changes an issue's notes via bd CLI.
func (e *BDExecutor) UpdateNotes(issueID, notes string) error {
	start := time.Now()
	defer func() {
		log.Debug(log.CatBeads, "UpdateNotes completed", "issueID", issueID, "duration", time.Since(start))
	}()

	if _, err := e.runBeads("update", issueID, "--notes", notes, "--json"); err != nil {
		log.Error(log.CatBeads, "UpdateNotes failed", "issueID", issueID, "error", err)
		return err
	}
	return nil
}

// UpdateIssue applies field updates to an issue via bd CLI.
// Only non-nil fields in opts are included. Labels are handled as a separate
// bd update call because --set-labels cannot be combined with other flags.
// Returns nil without invoking bd if no fields are set.
func (e *BDExecutor) UpdateIssue(issueID string, opts domain.UpdateIssueOptions) error {
	start := time.Now()
	defer func() {
		log.Debug(log.CatBeads, "UpdateIssue completed", "issueID", issueID, "duration", time.Since(start))
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
		args = append(args, "--priority", fmt.Sprintf("%d", *opts.Priority))
	}
	if opts.Status != nil {
		args = append(args, "--status", string(*opts.Status))
	}
	if opts.Assignee != nil {
		args = append(args, "--assignee", *opts.Assignee)
	}
	if opts.Type != nil {
		args = append(args, "--type", string(*opts.Type))
	}
	if opts.ParentID != nil {
		args = append(args, "--parent", *opts.ParentID)
	}

	// Execute non-label update if any fields were set.
	if len(args) > 2 {
		args = append(args, "--json")
		if _, err := e.runBeads(args...); err != nil {
			log.Error(log.CatBeads, "UpdateIssue failed", "issueID", issueID, "error", err)
			return fmt.Errorf("saving issue %s: %w", issueID, err)
		}
	}

	// Labels require a separate bd update call; --set-labels cannot be
	// combined with other flags in a single invocation.
	if opts.Labels != nil {
		if err := e.SetLabels(issueID, *opts.Labels); err != nil {
			return fmt.Errorf("saving issue %s labels: %w", issueID, err)
		}
	}

	return nil
}

// CloseIssue marks an issue as closed with a reason via bd CLI.
func (e *BDExecutor) CloseIssue(issueID, reason string) error {
	start := time.Now()
	defer func() {
		log.Debug(log.CatBeads, "CloseIssue completed", "issueID", issueID, "duration", time.Since(start))
	}()

	if _, err := e.runBeads("close", issueID, "--reason", reason, "--json"); err != nil {
		log.Error(log.CatBeads, "CloseIssue failed", "issueID", issueID, "error", err)
		return err
	}
	return nil
}

// ReopenIssue reopens a closed issue via bd CLI.
func (e *BDExecutor) ReopenIssue(issueID string) error {
	start := time.Now()
	defer func() {
		log.Debug(log.CatBeads, "ReopenIssue completed", "issueID", issueID, "duration", time.Since(start))
	}()

	if _, err := e.runBeads("update", issueID, "--status", string(domain.StatusOpen), "--json"); err != nil {
		log.Error(log.CatBeads, "ReopenIssue failed", "issueID", issueID, "error", err)
		return err
	}
	return nil
}

// DeleteIssues deletes one or more issues in a single bd CLI call.
func (e *BDExecutor) DeleteIssues(issueIDs []string) error {
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

	if _, err := e.runBeads(args...); err != nil {
		log.Error(log.CatBeads, "DeleteIssues failed", "count", len(issueIDs), "error", err)
		return err
	}
	return nil
}

// SetLabels replaces all labels on an issue via bd CLI.
// Pass an empty slice (or nil) to remove all labels.
//
// bd's --set-labels flag is a repeatable "strings" type that ignores empty
// values, so clearing all labels requires fetching the current labels and
// calling --remove-label for each one.
func (e *BDExecutor) SetLabels(issueID string, labels []string) error {
	start := time.Now()
	defer func() {
		log.Debug(log.CatBeads, "SetLabels completed", "issueID", issueID, "labels", strings.Join(labels, ","), "duration", time.Since(start))
	}()

	if len(labels) > 0 {
		if _, err := e.runBeads("update", issueID, "--set-labels", strings.Join(labels, ","), "--json"); err != nil {
			log.Error(log.CatBeads, "SetLabels failed", "issueID", issueID, "error", err)
			return err
		}
		return nil
	}

	// Empty labels: bd ignores --set-labels "", so we must fetch current
	// labels and remove each one individually.
	current, err := e.ShowIssue(issueID)
	if err != nil {
		return fmt.Errorf("fetching issue %s for label clearing: %w", issueID, err)
	}
	if len(current.Labels) == 0 {
		return nil // already clear
	}
	args := []string{"update", issueID}
	for _, l := range current.Labels {
		args = append(args, "--remove-label", l)
	}
	args = append(args, "--json")
	if _, err := e.runBeads(args...); err != nil {
		log.Error(log.CatBeads, "SetLabels (clear) failed", "issueID", issueID, "error", err)
		return err
	}
	return nil
}

// ShowIssue executes 'bd show <id> --json' and parses the JSON array output.
func (e *BDExecutor) ShowIssue(issueID string) (*domain.Issue, error) {
	start := time.Now()
	defer func() {
		log.Debug(log.CatBeads, "ShowIssue completed", "issueID", issueID, "duration", time.Since(start))
	}()

	output, err := e.runBeads("show", issueID, "--json")
	if err != nil {
		log.Error(log.CatBeads, "ShowIssue failed", "issueID", issueID, "error", err)
		return nil, err
	}

	var issues []domain.Issue
	if err := json.Unmarshal([]byte(output), &issues); err != nil {
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

// GetComments executes 'bd comments <id> --json' and parses the JSON output.
func (e *BDExecutor) GetComments(issueID string) ([]domain.Comment, error) {
	start := time.Now()
	defer func() {
		log.Debug(log.CatBeads, "GetComments completed", "issueID", issueID, "duration", time.Since(start))
	}()

	output, err := e.runBeads("comments", issueID, "--json")
	if err != nil {
		log.Error(log.CatBeads, "GetComments failed", "issueID", issueID, "error", err)
		return nil, err
	}

	var raw []struct {
		Author    string    `json:"author"`
		Text      string    `json:"text"`
		CreatedAt time.Time `json:"created_at"`
	}
	if err := json.Unmarshal([]byte(output), &raw); err != nil {
		err = fmt.Errorf("failed to parse bd comments output: %w", err)
		log.Error(log.CatBeads, "GetComments parse failed", "issueID", issueID, "error", err)
		return nil, err
	}

	comments := make([]domain.Comment, len(raw))
	for i, c := range raw {
		comments[i] = domain.Comment{
			Author:    c.Author,
			Text:      c.Text,
			CreatedAt: c.CreatedAt,
		}
	}
	return comments, nil
}

// AddComment executes 'bd comments add <id> <text>'.
func (e *BDExecutor) AddComment(issueID, author, text string) error {
	start := time.Now()
	defer func() {
		log.Debug(log.CatBeads, "AddComment completed", "issueID", issueID, "author", author, "duration", time.Since(start))
	}()

	args := []string{"comments", "add", issueID, text, "--json"}
	if author != "" {
		args = append(args, "--actor", author)
	}
	if _, err := e.runBeads(args...); err != nil {
		log.Error(log.CatBeads, "AddComment failed", "issueID", issueID, "error", err)
		return err
	}
	return nil
}

// CreateEpic creates a new epic via bd CLI.
func (e *BDExecutor) CreateEpic(title, description string, labels []string) (domain.CreateResult, error) {
	start := time.Now()
	defer func() {
		log.Debug(log.CatBeads, "CreateEpic completed", "title", title, "duration", time.Since(start))
	}()

	args := []string{"create", title, "-t", "epic", "-d", description, "--json"}
	for _, l := range labels {
		args = append(args, "--label", l)
	}

	output, err := e.runBeads(args...)
	if err != nil {
		log.Error(log.CatBeads, "CreateEpic failed", "title", title, "error", err)
		return domain.CreateResult{}, err
	}

	var result domain.CreateResult
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		err = fmt.Errorf("failed to parse bd create output: %w", err)
		log.Error(log.CatBeads, "CreateEpic parse failed", "error", err)
		return domain.CreateResult{}, err
	}

	return result, nil
}

// CreateTask creates a new task as a child of an epic via bd CLI.
func (e *BDExecutor) CreateTask(title, description, parentID, assignee string, labels []string) (domain.CreateResult, error) {
	start := time.Now()
	defer func() {
		log.Debug(log.CatBeads, "CreateTask completed", "title", title, "parentID", parentID, "assignee", assignee, "duration", time.Since(start))
	}()

	args := []string{"create", title, "--parent", parentID, "-t", "task", "-d", description, "--json"}
	if assignee != "" {
		args = append(args, "--assignee", assignee)
	}
	for _, l := range labels {
		args = append(args, "--label", l)
	}

	output, err := e.runBeads(args...)
	if err != nil {
		log.Error(log.CatBeads, "CreateTask failed", "title", title, "error", err)
		return domain.CreateResult{}, err
	}

	var result domain.CreateResult
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		err = fmt.Errorf("failed to parse bd create output: %w", err)
		log.Error(log.CatBeads, "CreateTask parse failed", "error", err)
		return domain.CreateResult{}, err
	}

	return result, nil
}

// AddDependency adds a dependency between two tasks via bd CLI.
func (e *BDExecutor) AddDependency(taskID, dependsOnID string) error {
	start := time.Now()
	defer func() {
		log.Debug(log.CatBeads, "AddDependency completed", "taskID", taskID, "dependsOnID", dependsOnID, "duration", time.Since(start))
	}()

	if _, err := e.runBeads("dep", "add", taskID, dependsOnID, "-t", "blocks"); err != nil {
		log.Error(log.CatBeads, "AddDependency failed", "taskID", taskID, "dependsOnID", dependsOnID, "error", err)
		return err
	}
	return nil
}
