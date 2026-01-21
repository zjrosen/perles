package beads

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/zjrosen/perles/internal/log"
)

// Compile-time check that RealExecutor implements BeadsExecutor.
var _ BeadsExecutor = (*RealExecutor)(nil)

// RealExecutor implements BeadsExecutor by executing actual BD commands.
type RealExecutor struct {
	workDir  string // Working directory for command execution
	beadsDir string // Path to .beads directory for BEADS_DIR env var
}

// NewRealExecutor creates a new RealExecutor.
// workDir is the working directory for command execution.
// beadsDir is the path to the .beads directory (sets BEADS_DIR env var).
func NewRealExecutor(workDir, beadsDir string) *RealExecutor {
	return &RealExecutor{workDir: workDir, beadsDir: beadsDir}
}

// runBeads executes a bd command and returns stdout and any error.
func (e *RealExecutor) runBeads(args ...string) (string, error) {
	//nolint:gosec // G204: args come from controlled sources
	cmd := exec.Command("bd", args...)
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
func (e *RealExecutor) UpdateStatus(issueID string, status Status) error {
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
func (e *RealExecutor) UpdatePriority(issueID string, priority Priority) error {
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
func (e *RealExecutor) UpdateType(issueID string, issueType IssueType) error {
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

// CloseIssue marks an issue as closed with a reason via bd CLI.
func (e *RealExecutor) CloseIssue(issueID, reason string) error {
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
func (e *RealExecutor) ReopenIssue(issueID string) error {
	start := time.Now()
	defer func() {
		log.Debug(log.CatBeads, "ReopenIssue completed", "issueID", issueID, "duration", time.Since(start))
	}()

	if _, err := e.runBeads("update", issueID, "--status", string(StatusOpen), "--json"); err != nil {
		log.Error(log.CatBeads, "ReopenIssue failed", "issueID", issueID, "error", err)
		return err
	}
	return nil
}

// DeleteIssues deletes one or more issues in a single bd CLI call.
func (e *RealExecutor) DeleteIssues(issueIDs []string) error {
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
// Pass an empty slice to remove all labels.
func (e *RealExecutor) SetLabels(issueID string, labels []string) error {
	start := time.Now()
	defer func() {
		log.Debug(log.CatBeads, "SetLabels completed", "issueID", issueID, "labels", strings.Join(labels, ","), "duration", time.Since(start))
	}()

	if _, err := e.runBeads("update", issueID, "--set-labels", strings.Join(labels, ","), "--json"); err != nil {
		log.Error(log.CatBeads, "SetLabels failed", "issueID", issueID, "error", err)
		return err
	}
	return nil
}

// ShowIssue executes 'bd show <id> --json' and parses the JSON array output.
func (e *RealExecutor) ShowIssue(issueID string) (*Issue, error) {
	start := time.Now()
	defer func() {
		log.Debug(log.CatBeads, "ShowIssue completed", "issueID", issueID, "duration", time.Since(start))
	}()

	output, err := e.runBeads("show", issueID, "--json")
	if err != nil {
		log.Error(log.CatBeads, "ShowIssue failed", "issueID", issueID, "error", err)
		return nil, err
	}

	// bd show returns a JSON array
	var issues []Issue
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

// AddComment executes 'bd comment <id> --author <author> -- <text>'.
func (e *RealExecutor) AddComment(issueID, author, text string) error {
	start := time.Now()
	defer func() {
		log.Debug(log.CatBeads, "AddComment completed", "issueID", issueID, "author", author, "duration", time.Since(start))
	}()

	if _, err := e.runBeads("comment", issueID, "--author", author, "--", text); err != nil {
		log.Error(log.CatBeads, "AddComment failed", "issueID", issueID, "error", err)
		return err
	}
	return nil
}

// CreateEpic creates a new epic via bd CLI.
func (e *RealExecutor) CreateEpic(title, description string, labels []string) (CreateResult, error) {
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
		return CreateResult{}, err
	}

	var result CreateResult
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		err = fmt.Errorf("failed to parse bd create output: %w", err)
		log.Error(log.CatBeads, "CreateEpic parse failed", "error", err)
		return CreateResult{}, err
	}

	return result, nil
}

// CreateTask creates a new task as a child of an epic via bd CLI.
func (e *RealExecutor) CreateTask(title, description, parentID, assignee string, labels []string) (CreateResult, error) {
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
		return CreateResult{}, err
	}

	var result CreateResult
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		err = fmt.Errorf("failed to parse bd create output: %w", err)
		log.Error(log.CatBeads, "CreateTask parse failed", "error", err)
		return CreateResult{}, err
	}

	return result, nil
}

// AddDependency adds a dependency between two tasks via bd CLI.
func (e *RealExecutor) AddDependency(taskID, dependsOnID string) error {
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
