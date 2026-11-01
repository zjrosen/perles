package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	appbeads "github.com/zjrosen/perles/internal/beads/application"
	domain "github.com/zjrosen/perles/internal/beads/domain"
	"github.com/zjrosen/perles/internal/log"
)

// Compile-time check that BDExecutor implements IssueExecutor.
var _ appbeads.IssueExecutor = (*BDExecutor)(nil)

// BDExecutor implements IssueExecutor by executing actual BD CLI commands.
//
// User-supplied text is always passed as a single --flag=value argument (or
// after "--" for positional text), so a value starting with '-' is never
// parsed as a flag.
type BDExecutor struct {
	workDir  string
	beadsDir string
	// timeout bounds each bd invocation; zero means defaultBDTimeout.
	timeout time.Duration
	// runner, when set, runs every bd command with runCtx as its parent
	// context instead of the real bd (NewBDExecutorWithRunner).
	runner BDRunner
	runCtx context.Context
	// runFunc is an optional override for running bd, used in tests.
	runFunc func(args ...string) bdOutput
}

// NewBDExecutor creates a new BDExecutor.
// workDir is the working directory for command execution.
// beadsDir is the path to the .beads directory (sets BEADS_DIR env var). A
// relative beadsDir is made absolute against perles' working directory, since
// bd runs in workDir.
func NewBDExecutor(workDir, beadsDir string) *BDExecutor {
	return &BDExecutor{workDir: workDir, beadsDir: absBeadsDir(beadsDir)}
}

// NewBDExecutorWithRunner returns a BDExecutor that runs every bd command
// through run, with ctx as the parent context of each one: when ctx ends, the
// command running and every later one fail with run's error. run decides
// where bd runs and how long it may take (NewBDRunner: workDir, beadsDir and
// defaultBDTimeout), and its errors are returned unchanged.
func NewBDExecutorWithRunner(ctx context.Context, run BDRunner) *BDExecutor {
	return &BDExecutor{runner: run, runCtx: ctx}
}

// runBeads executes a bd command and returns its trimmed stdout. On failure
// the error carries bd's own explanation (see newBDError).
func (e *BDExecutor) runBeads(args ...string) (string, error) {
	if e.runner != nil {
		out, err := e.runner(e.runCtx, BDRequest{Args: args})
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(out), nil
	}
	var out bdOutput
	if e.runFunc != nil {
		out = e.runFunc(args...)
	} else {
		out = e.execBD(args)
	}
	if out.failed() {
		return "", newBDError(args, out, e.effectiveTimeout())
	}
	return strings.TrimSpace(out.Stdout), nil
}

// UpdateStatus changes an issue's status via bd CLI.
//
// Moving an issue into closed is refused by bd >= 1.3 while it has open
// children or a live blocker; perles does not pass --force, so that refusal
// is returned with bd's reason.
func (e *BDExecutor) UpdateStatus(issueID string, status domain.Status) error {
	start := time.Now()
	defer func() {
		log.Debug(log.CatBeads, "UpdateStatus completed", "issueID", issueID, "status", status, "duration", time.Since(start))
	}()

	if _, err := e.runBeads("update", issueID, "--status="+string(status), "--json"); err != nil {
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

	if _, err := e.runBeads("update", issueID, fmt.Sprintf("--priority=%d", priority), "--json"); err != nil {
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

	if _, err := e.runBeads("update", issueID, "--type="+string(issueType), "--json"); err != nil {
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

	if _, err := e.runBeads("update", issueID, "--title="+title, "--json"); err != nil {
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

	if _, err := e.runBeads("update", issueID, "--description="+description, "--json"); err != nil {
		log.Error(log.CatBeads, "UpdateDescription failed", "issueID", issueID, "error", err)
		return err
	}
	return nil
}

// UpdateNotes replaces an issue's notes via bd CLI (see setNotes).
func (e *BDExecutor) UpdateNotes(issueID, notes string) error {
	start := time.Now()
	defer func() {
		log.Debug(log.CatBeads, "UpdateNotes completed", "issueID", issueID, "duration", time.Since(start))
	}()

	if err := e.setNotes(issueID, notes, nil); err != nil {
		log.Error(log.CatBeads, "UpdateNotes failed", "issueID", issueID, "error", err)
		return err
	}
	return nil
}

// setNotes replaces an issue's notes in a bd update call of its own, so the
// --force it may need never reaches other fields (on bd, --force also
// overrides the close policy and claim fences).
//
// Non-empty notes are sent as --notes=<notes>; bd >= 1.3.2 refuses to
// replace existing non-empty notes without --force. When expected (the notes
// the caller last saw) is set, that refusal is retried with --force only if
// bd's current notes still equal expected; otherwise someone else changed
// them since, and ErrNotesChanged is returned instead of clobbering their
// edit. With expected nil (a plain "replace notes" call) the refusal is
// retried with --force. Empty notes clear the field with --clear-notes
// (bd >= 1.3.2 refuses --notes ""), falling back to --notes= on a bd
// without that flag.
func (e *BDExecutor) setNotes(issueID, notes string, expected *string) error {
	if notes == "" {
		_, err := e.runBeads("update", issueID, "--clear-notes", "--json")
		if err != nil && isUnknownFlag(err, "clear-notes") {
			log.Debug(log.CatBeads, "bd has no --clear-notes; clearing notes with --notes=", "issueID", issueID)
			_, err = e.runBeads("update", issueID, "--notes=", "--json")
		}
		return err
	}

	_, err := e.runBeads("update", issueID, "--notes="+notes, "--json")
	if err != nil && isNotesOverwriteRefusal(err) {
		if expected != nil {
			current, showErr := e.ShowIssue(issueID)
			if showErr != nil {
				return fmt.Errorf("%w (checking current notes: %w)", err, showErr)
			}
			if current.Notes != *expected {
				log.Debug(log.CatBeads, "notes changed since they were read; not forcing", "issueID", issueID)
				return fmt.Errorf("%w: %s", ErrNotesChanged, issueID)
			}
		}
		log.Debug(log.CatBeads, "bd refused to overwrite notes; retrying with --force", "issueID", issueID)
		_, err = e.runBeads("update", issueID, "--notes="+notes, "--force", "--json")
	}
	return err
}

// ErrNotesChanged reports that an issue's notes changed (for example an agent
// appended to them) after the caller read them, so perles did not overwrite
// them.
var ErrNotesChanged = errors.New("notes were changed by someone else since this issue was opened; reopen it to see the latest notes, then save again")

// UpdateIssue applies field updates to an issue via bd CLI.
// Only non-nil fields in opts are included. Up to three bd update calls are
// made, in order: the other fields, then notes (see setNotes), then labels
// (--set-labels cannot be combined with other flags). Returns nil without
// invoking bd if no fields are set.
func (e *BDExecutor) UpdateIssue(issueID string, opts domain.UpdateIssueOptions) error {
	start := time.Now()
	defer func() {
		log.Debug(log.CatBeads, "UpdateIssue completed", "issueID", issueID, "duration", time.Since(start))
	}()

	// Build args for the fields other than notes and labels.
	args := []string{"update", issueID}

	if opts.Title != nil {
		args = append(args, "--title="+*opts.Title)
	}
	if opts.Description != nil {
		args = append(args, "--description="+*opts.Description)
	}
	if opts.Priority != nil {
		args = append(args, fmt.Sprintf("--priority=%d", *opts.Priority))
	}
	if opts.Status != nil {
		args = append(args, "--status="+string(*opts.Status))
	}
	if opts.Assignee != nil {
		args = append(args, "--assignee="+*opts.Assignee)
	}
	if opts.Type != nil {
		args = append(args, "--type="+string(*opts.Type))
	}
	if opts.ParentID != nil {
		args = append(args, "--parent="+*opts.ParentID)
	}

	if len(args) > 2 {
		args = append(args, "--json")
		if _, err := e.runBeads(args...); err != nil {
			log.Error(log.CatBeads, "UpdateIssue failed", "issueID", issueID, "error", err)
			return fmt.Errorf("saving issue %s: %w", issueID, err)
		}
	}

	if opts.Notes != nil {
		if err := e.setNotes(issueID, *opts.Notes, opts.ExpectedNotes); err != nil {
			log.Error(log.CatBeads, "UpdateIssue notes failed", "issueID", issueID, "error", err)
			return fmt.Errorf("saving issue %s notes: %w", issueID, err)
		}
	}

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

	if _, err := e.runBeads("close", issueID, "--reason="+reason, "--json"); err != nil {
		log.Error(log.CatBeads, "CloseIssue failed", "issueID", issueID, "error", err)
		return err
	}
	return nil
}

// ReopenIssue reopens a closed issue with `bd reopen`, which records a
// Reopened event. A bd without that command gets `bd update --status=open`.
func (e *BDExecutor) ReopenIssue(issueID string) error {
	start := time.Now()
	defer func() {
		log.Debug(log.CatBeads, "ReopenIssue completed", "issueID", issueID, "duration", time.Since(start))
	}()

	_, err := e.runBeads("reopen", issueID, "--json")
	if err != nil && isUnknownCommand(err, "reopen") {
		log.Debug(log.CatBeads, "bd has no reopen command; reopening with update --status", "issueID", issueID)
		_, err = e.runBeads("update", issueID, "--status="+string(domain.StatusOpen), "--json")
	}
	if err != nil {
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
		if _, err := e.runBeads("update", issueID, "--set-labels="+strings.Join(labels, ","), "--json"); err != nil {
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
		args = append(args, "--remove-label="+l)
	}
	args = append(args, "--json")
	if _, err := e.runBeads(args...); err != nil {
		log.Error(log.CatBeads, "SetLabels (clear) failed", "issueID", issueID, "error", err)
		return err
	}
	return nil
}

// ShowIssue executes 'bd --readonly show <id> --json' and maps bd's issue
// shape (issue_type, parent, dependencies) onto domain.Issue.
//
// --readonly is a bd global flag, so it goes before the subcommand. Plain
// 'show --json' rewrites .beads/last-touched, which perles watches, so each
// read would trigger a needless refresh. Read-only bd also skips migrations
// and gate files.
func (e *BDExecutor) ShowIssue(issueID string) (*domain.Issue, error) {
	start := time.Now()
	defer func() {
		log.Debug(log.CatBeads, "ShowIssue completed", "issueID", issueID, "duration", time.Since(start))
	}()

	output, err := e.runBeads("--readonly", "show", issueID, "--json")
	if err != nil {
		log.Error(log.CatBeads, "ShowIssue failed", "issueID", issueID, "error", err)
		return nil, err
	}

	issues, err := parseBDShow(output)
	if err != nil {
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

// GetComments executes 'bd --readonly comments <id> --json' and parses the
// JSON output. Like ShowIssue, it passes --readonly (before the subcommand)
// so the read does not touch .beads/last-touched, run migrations or create
// gate files.
func (e *BDExecutor) GetComments(issueID string) ([]domain.Comment, error) {
	start := time.Now()
	defer func() {
		log.Debug(log.CatBeads, "GetComments completed", "issueID", issueID, "duration", time.Since(start))
	}()

	output, err := e.runBeads("--readonly", "comments", issueID, "--json")
	if err != nil {
		log.Error(log.CatBeads, "GetComments failed", "issueID", issueID, "error", err)
		return nil, err
	}

	comments, err := parseBDComments(output)
	if err != nil {
		err = fmt.Errorf("failed to parse bd comments output: %w", err)
		log.Error(log.CatBeads, "GetComments parse failed", "issueID", issueID, "error", err)
		return nil, err
	}
	return comments, nil
}

// AddComment executes 'bd comments add <id> -- <text>'. The text follows
// "--" so a comment starting with '-' is not parsed as a flag.
func (e *BDExecutor) AddComment(issueID, author, text string) error {
	start := time.Now()
	defer func() {
		log.Debug(log.CatBeads, "AddComment completed", "issueID", issueID, "author", author, "duration", time.Since(start))
	}()

	args := []string{"comments", "add", issueID, "--json"}
	if author != "" {
		args = append(args, "--actor="+author)
	}
	args = append(args, "--", text)
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

	result, err := e.create(createArgs(title, description, domain.TypeEpic, "", "", labels))
	if err != nil {
		log.Error(log.CatBeads, "CreateEpic failed", "title", title, "error", err)
		return domain.CreateResult{}, err
	}
	return result, nil
}

// CreateTask creates a new task (a child of parentID when it is set) via bd CLI.
func (e *BDExecutor) CreateTask(title, description, parentID, assignee string, labels []string) (domain.CreateResult, error) {
	start := time.Now()
	defer func() {
		log.Debug(log.CatBeads, "CreateTask completed", "title", title, "parentID", parentID, "assignee", assignee, "duration", time.Since(start))
	}()

	result, err := e.create(createArgs(title, description, domain.TypeTask, parentID, assignee, labels))
	if err != nil {
		log.Error(log.CatBeads, "CreateTask failed", "title", title, "error", err)
		return domain.CreateResult{}, err
	}
	return result, nil
}

// createArgs builds `bd create` args. The title is passed as --title=<t>
// (available since beads v1.0) rather than positionally.
func createArgs(title, description string, issueType domain.IssueType, parentID, assignee string, labels []string) []string {
	args := []string{"create", "--title=" + title, "--type=" + string(issueType)}
	if parentID != "" {
		args = append(args, "--parent="+parentID)
	}
	if description != "" {
		args = append(args, "--description="+description)
	}
	if assignee != "" {
		args = append(args, "--assignee="+assignee)
	}
	for _, l := range labels {
		args = append(args, "--label="+l)
	}
	return append(args, "--json")
}

func (e *BDExecutor) create(args []string) (domain.CreateResult, error) {
	output, err := e.runBeads(args...)
	if err != nil {
		return domain.CreateResult{}, err
	}
	result, err := parseBDCreate(output)
	if err != nil {
		return domain.CreateResult{}, fmt.Errorf("failed to parse bd create output: %w", err)
	}
	return result, nil
}

// AddDependency adds a dependency between two tasks via bd CLI.
func (e *BDExecutor) AddDependency(taskID, dependsOnID string) error {
	start := time.Now()
	defer func() {
		log.Debug(log.CatBeads, "AddDependency completed", "taskID", taskID, "dependsOnID", dependsOnID, "duration", time.Since(start))
	}()

	if _, err := e.runBeads("dep", "add", taskID, dependsOnID, "--type=blocks"); err != nil {
		log.Error(log.CatBeads, "AddDependency failed", "taskID", taskID, "dependsOnID", dependsOnID, "error", err)
		return err
	}
	return nil
}
