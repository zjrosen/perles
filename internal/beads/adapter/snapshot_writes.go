package adapter

import (
	"errors"
	"fmt"
	"maps"
	"slices"

	appbeads "github.com/zjrosen/perles/internal/beads/application"
	beads "github.com/zjrosen/perles/internal/beads/domain"
	"github.com/zjrosen/perles/internal/log"
)

// Compile-time check.
var _ appbeads.IssueExecutor = snapshotIssueExecutor{}

// commentOverlay is an issue's comments read straight after perles added
// one.
type commentOverlay struct {
	comments []beads.Comment
	// seq is the writeSeq of the AddComment the comments were read after.
	// A snapshot whose coveredWriteSeq is at least seq was exported after
	// that write, so it has every comment of the entry that perles added.
	seq uint64
}

// snapshotIssueExecutor is the IssueExecutor under a SnapshotBackend's
// TaskExecutor. Every write goes through bd; after a successful one it bumps
// the backend's writeSeq and calls onOwnWrite. A failed write does neither.
// Writes never build or wait on a snapshot, and neither does ShowIssue,
// which always runs bd --readonly show, so it is always fresh.
//
// AddComment also reads the issue's comments back into the comment overlay,
// so a GetComments straight after it (as the TUI's comment editor runs it)
// returns the new comment before a snapshot covers the write.
type snapshotIssueExecutor struct {
	b      *SnapshotBackend
	writes appbeads.IssueWriter
}

// wrote records a successful write when err is nil, and returns err.
func (e snapshotIssueExecutor) wrote(err error) error {
	if err == nil {
		e.b.ownWrite()
	}
	return err
}

func (e snapshotIssueExecutor) ShowIssue(issueID string) (*beads.Issue, error) {
	return e.b.bdReads.ShowIssue(issueID)
}

func (e snapshotIssueExecutor) UpdateStatus(issueID string, status beads.Status) error {
	return e.wrote(e.writes.UpdateStatus(issueID, status))
}

func (e snapshotIssueExecutor) UpdatePriority(issueID string, priority beads.Priority) error {
	return e.wrote(e.writes.UpdatePriority(issueID, priority))
}

func (e snapshotIssueExecutor) UpdateType(issueID string, issueType beads.IssueType) error {
	return e.wrote(e.writes.UpdateType(issueID, issueType))
}

func (e snapshotIssueExecutor) UpdateTitle(issueID, title string) error {
	return e.wrote(e.writes.UpdateTitle(issueID, title))
}

func (e snapshotIssueExecutor) UpdateDescription(issueID, description string) error {
	return e.wrote(e.writes.UpdateDescription(issueID, description))
}

func (e snapshotIssueExecutor) UpdateNotes(issueID, notes string) error {
	return e.wrote(e.writes.UpdateNotes(issueID, notes))
}

func (e snapshotIssueExecutor) CloseIssue(issueID, reason string) error {
	return e.wrote(e.writes.CloseIssue(issueID, reason))
}

func (e snapshotIssueExecutor) ReopenIssue(issueID string) error {
	return e.wrote(e.writes.ReopenIssue(issueID))
}

func (e snapshotIssueExecutor) SetLabels(issueID string, labels []string) error {
	return e.wrote(e.writes.SetLabels(issueID, labels))
}

// AddComment adds the comment through bd, then, on the same goroutine,
// reads the issue's comments back into the overlay (see
// SnapshotBackend.overlayComment). Only the write's error is returned: once
// bd added the comment, AddComment succeeds.
func (e snapshotIssueExecutor) AddComment(issueID, author, text string) error {
	if err := e.writes.AddComment(issueID, author, text); err != nil {
		return err
	}
	seq := e.b.ownWrite()
	e.b.overlayComment(issueID, author, text, seq)
	return nil
}

func (e snapshotIssueExecutor) CreateEpic(title, description string, labels []string) (beads.CreateResult, error) {
	result, err := e.writes.CreateEpic(title, description, labels)
	return result, e.wrote(err)
}

func (e snapshotIssueExecutor) CreateTask(title, description, parentID, assignee string, labels []string) (beads.CreateResult, error) {
	result, err := e.writes.CreateTask(title, description, parentID, assignee, labels)
	return result, e.wrote(err)
}

func (e snapshotIssueExecutor) DeleteIssues(issueIDs []string) error {
	return e.wrote(e.writes.DeleteIssues(issueIDs))
}

func (e snapshotIssueExecutor) AddDependency(taskID, dependsOnID string) error {
	return e.wrote(e.writes.AddDependency(taskID, dependsOnID))
}

func (e snapshotIssueExecutor) UpdateIssue(issueID string, opts beads.UpdateIssueOptions) error {
	return e.wrote(e.writes.UpdateIssue(issueID, opts))
}

// ownWrite records a successful write made through perles: it bumps
// writeSeq, calls onOwnWrite with the new value and returns it. A write that
// fails partway (UpdateIssue runs up to three bd commands) is not recorded;
// what it did write is a database change like any other.
func (b *SnapshotBackend) ownWrite() uint64 {
	seq := b.writeSeq.Add(1)
	b.onOwnWrite(seq)
	return seq
}

// logOwnWrite is the default onOwnWrite. Until a later build covers the
// write, queries answer from the installed snapshot without it, while
// comment reads use the overlay.
func (b *SnapshotBackend) logOwnWrite(seq uint64) {
	log.Debug(log.CatBeads, "Own write is not in the snapshot yet", "writeSeq", seq)
}

// overlayComment fills the issue's overlay entry after perles added a
// comment as write seq, so the next GetComments returns it before a snapshot
// covers the write. It reads the comments with bd --readonly comments. When
// that read fails, it falls back to the issue's comments as of the current
// snapshot (overlay included) plus the new comment synthesized locally with
// the ID local-<seq>, and logs a warning. When those cannot be read either
// (no snapshot yet, the backend closed, a snapshot read error), it records no
// entry: the comment reads that follow then run bd (no snapshot yet) or fail
// like this read did.
func (b *SnapshotBackend) overlayComment(issueID, author, text string, seq uint64) {
	comments, err := b.bdReads.GetComments(issueID)
	if err != nil {
		local, localErr := b.withLocalComment(issueID, author, text, seq)
		if localErr != nil {
			log.Warn(log.CatBeads, "Reading comments after AddComment failed; the new comment may be missing until the next snapshot",
				"issueID", issueID, "error", err, "snapshotError", localErr)
			return
		}
		log.Warn(log.CatBeads, "Reading comments after AddComment failed; serving the new comment from a local copy",
			"issueID", issueID, "commentID", local[len(local)-1].ID, "error", err)
		comments = local
	}
	b.storeOverlay(issueID, comments, seq)
}

// withLocalComment returns the issue's comments as of the current snapshot
// followed by the comment perles added as write seq, synthesized with the
// ID local-<seq> and the current time.
func (b *SnapshotBackend) withLocalComment(issueID, author, text string, seq uint64) ([]beads.Comment, error) {
	comments, err := b.snapshotComments(issueID)
	if err != nil {
		return nil, err
	}
	return append(comments, beads.Comment{
		ID:        fmt.Sprintf("local-%d", seq),
		Author:    author,
		Text:      text,
		CreatedAt: b.clock.Now().UTC(),
	}), nil
}

// storeOverlay makes comments the issue's overlay entry for write seq,
// unless the entry already belongs to a later write. Comments read after a
// later write include every earlier write's comment, so the later entry is
// the more complete one.
func (b *SnapshotBackend) storeOverlay(issueID string, comments []beads.Comment, seq uint64) {
	b.overlayMu.Lock()
	defer b.overlayMu.Unlock()
	if current, ok := b.overlay[issueID]; ok && current.seq > seq {
		return
	}
	b.overlay[issueID] = commentOverlay{comments: comments, seq: seq}
}

// liveOverlay returns a copy of the issue's overlay entry when s does not
// cover its write. The caller holds s's read lock.
func (b *SnapshotBackend) liveOverlay(s *snapshot, issueID string) ([]beads.Comment, bool) {
	b.overlayMu.Lock()
	defer b.overlayMu.Unlock()
	entry, ok := b.overlay[issueID]
	if !ok || entry.seq <= s.coveredWriteSeq {
		return nil, false
	}
	return slices.Clone(entry.comments), true
}

// pruneOverlay drops the overlay entries whose write is covered by a
// snapshot with this coveredWriteSeq. install calls it after retiring the
// snapshot that one replaced. Installs run one at a time (the lazy first
// build is shared, later builds are not concurrent) and covered write
// sequences only grow, so by then no reader holds a snapshot that lacks an
// entry it drops.
func (b *SnapshotBackend) pruneOverlay(coveredWriteSeq uint64) {
	b.overlayMu.Lock()
	defer b.overlayMu.Unlock()
	maps.DeleteFunc(b.overlay, func(_ string, entry commentOverlay) bool {
		return entry.seq <= coveredWriteSeq
	})
}

// getComments returns an issue's comments, oldest first. Once a snapshot is
// installed it reads memory only (see snapshotComments) and returns a
// snapshot read error as is: it never runs bd, so it never blocks the UI
// goroutine. Before the first snapshot it runs bd --readonly comments
// instead of building one, since one cheap read beats a full export (a
// daemon that reads one issue's comments never exports). The TUI builds the
// first snapshot before the UI starts, so that read never runs on the UI
// goroutine. After Close it fails with errSnapshotBackendClosed.
func (b *SnapshotBackend) getComments(issueID string) ([]beads.Comment, error) {
	comments, err := b.snapshotComments(issueID)
	if errors.Is(err, errNoSnapshot) {
		return b.bdReads.GetComments(issueID)
	}
	return comments, err
}
