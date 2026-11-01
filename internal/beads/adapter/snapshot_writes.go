package adapter

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"time"

	appbeads "github.com/zjrosen/perles/internal/beads/application"
	beads "github.com/zjrosen/perles/internal/beads/domain"
	"github.com/zjrosen/perles/internal/log"
)

// Compile-time check.
var _ appbeads.IssueExecutor = snapshotIssueExecutor{}

// commentOverlay is an issue's overlay entry: its comments as bd read them
// straight after perles added one or, when that read failed, synthesized
// from the comments served then plus a local copy of the new one. An entry
// stored over another is merged with it (see mergeOverlay).
type commentOverlay struct {
	// comments is what GetComments serves while the entry is live.
	comments []beads.Comment
	// seq is the writeSeq of the AddComment the comments were read after.
	// A snapshot whose coveredWriteSeq is at least seq was exported after
	// that write, so it has every comment of the entry that perles added.
	seq uint64
	// synthesized is set when the comments are not bd's: reading them back
	// after the AddComment failed, so they are what GetComments served then
	// plus a local copy of the new comment (see withLocalComment), or such
	// an entry merged with another (see mergeOverlay).
	synthesized bool
	// local lists the local copies among comments: comments that perles
	// synthesized in place of bd's copies, which the entry does not hold.
	local []localComment
}

// localComment is a local copy of a comment that perles added, made because
// reading the comments back after the AddComment failed.
type localComment struct {
	// comment has the ID local-<seq>, the author and text, and the time it
	// was synthesized.
	comment beads.Comment
	// since is when the AddComment's bd write started: bd's copy of the
	// comment was created no earlier.
	since time.Time
}

// snapshotIssueExecutor is the IssueExecutor under a SnapshotBackend's
// TaskExecutor. Every write goes through bd; after a successful one it bumps
// the backend's writeSeq and asks for a rebuild (see ownWrite). A failed
// write does neither. Writes never build or wait on a snapshot, and neither
// does ShowIssue, which always runs bd --readonly show, so it is always
// fresh.
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
	since := e.b.clock.Now()
	if err := e.writes.AddComment(issueID, author, text); err != nil {
		return err
	}
	seq := e.b.ownWrite()
	e.b.overlayComment(issueID, author, text, since, seq)
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

// ownWrite records a successful write made through perles and returns its
// writeSeq. It bumps writeSeq, then asks for a rebuild through the hash
// gate (T2, see requestRebuild): bd changed the manifest, so the gate
// passes and the scheduler rebuilds once pacing and the failure backoff
// allow. It runs on the writer's goroutine and never blocks: writes never
// wait on a rebuild. Until a build covers the write, queries answer from
// the installed snapshot without it, while comment reads use the overlay.
//
// A write that fails partway (UpdateIssue runs up to three bd commands) is
// not recorded; what it did write reaches the snapshot like any external
// change, through the manifest watcher.
func (b *SnapshotBackend) ownWrite() uint64 {
	seq := b.writeSeq.Add(1)
	b.requestRebuild(false)
	return seq
}

// overlayComment records the issue's overlay entry after perles added a
// comment as write seq, whose bd write started at since, so the next
// GetComments returns it before a snapshot covers the write. It reads the
// comments with bd --readonly comments. When that read fails, it falls back
// to the comments GetComments serves now plus a local copy of the new
// comment with the ID local-<seq> (see withLocalComment), and logs a
// warning. When those cannot be read either (no snapshot yet, the backend
// closed, a snapshot read error), it records no entry: the comment reads that
// follow then run bd (no snapshot yet) or fail like this read did.
func (b *SnapshotBackend) overlayComment(issueID, author, text string, since time.Time, seq uint64) {
	comments, err := b.bdReads.GetComments(issueID)
	if err == nil {
		b.storeOverlay(issueID, commentOverlay{comments: comments, seq: seq})
		return
	}
	local := localComment{
		comment: beads.Comment{
			ID:        fmt.Sprintf("local-%d", seq),
			Author:    author,
			Text:      text,
			CreatedAt: b.clock.Now().UTC(),
		},
		since: since,
	}
	entry, localErr := b.withLocalComment(issueID, local, seq)
	if localErr != nil {
		log.Warn(log.CatBeads, "Reading comments after AddComment failed; the new comment may be missing until the next snapshot",
			"issueID", issueID, "error", err, "snapshotError", localErr)
		return
	}
	if entry.isLocal(local.comment.ID) {
		log.Warn(log.CatBeads, "Reading comments after AddComment failed; serving the new comment from a local copy",
			"issueID", issueID, "commentID", local.comment.ID, "error", err)
	} else {
		log.Warn(log.CatBeads, "Reading comments after AddComment failed; the comments served already include the new comment",
			"issueID", issueID, "error", err)
	}
	b.storeOverlay(issueID, entry)
}

// withLocalComment returns the synthesized overlay entry of write seq, whose
// comments could not be read back: the issue's comments as GetComments
// serves them now (see servedComments) followed by local, the new comment's
// local copy. When they already hold bd's copy, because a snapshot that
// read writeSeq before the write exported after it, local is left out (see
// withoutCopiedLocals).
//
// It builds on the live overlay entry, not on the snapshot's comments
// alone, so the entry keeps an earlier write's comment that the snapshot
// lacks even if a snapshot covering that write, but not this one, prunes
// the earlier entry before this one is stored.
func (b *SnapshotBackend) withLocalComment(issueID string, local localComment, seq uint64) (commentOverlay, error) {
	served, err := b.servedComments(issueID)
	if err != nil {
		return commentOverlay{}, err
	}
	return commentOverlay{
		comments:    append(served.comments, local.comment),
		seq:         seq,
		synthesized: true,
		local:       append(served.local, local),
	}.withoutCopiedLocals(), nil
}

// storeOverlay makes next the issue's overlay entry, merged with the entry
// already there (see mergeOverlay).
func (b *SnapshotBackend) storeOverlay(issueID string, next commentOverlay) {
	b.overlayMu.Lock()
	defer b.overlayMu.Unlock()
	if current, ok := b.overlay[issueID]; ok {
		next = mergeOverlay(current, next)
	}
	b.overlay[issueID] = next
}

// mergeOverlay returns the entry that replaces an issue's two overlay
// entries x and y. Comments that bd read after a write include every earlier
// write's comment, so when the later write's entry (the higher seq) holds
// bd's comments, it is the more complete one and replaces the other. A
// synthesized later entry, though, holds what GetComments served when it was
// made, which lacks the comment of an earlier write whose read was still
// running. The two are then merged into an entry of the later write: the
// earlier entry's comments, then the later one's that it lacks (by ID),
// without the local copies whose bd copy the merge holds.
func mergeOverlay(x, y commentOverlay) commentOverlay {
	later, earlier := x, y
	if earlier.seq > later.seq {
		later, earlier = earlier, later
	}
	if !later.synthesized {
		return later
	}
	merged := commentOverlay{seq: later.seq, synthesized: true}
	ids := map[string]bool{}
	for _, c := range slices.Concat(earlier.comments, later.comments) {
		if !ids[c.ID] {
			ids[c.ID] = true
			merged.comments = append(merged.comments, c)
		}
	}
	for _, l := range slices.Concat(earlier.local, later.local) {
		if !merged.isLocal(l.comment.ID) {
			merged.local = append(merged.local, l)
		}
	}
	return merged.withoutCopiedLocals()
}

// withoutCopiedLocals returns the entry without the local copies whose bd
// copy it holds (see hasBDCopy).
func (o commentOverlay) withoutCopiedLocals() commentOverlay {
	var copied []string
	for _, l := range o.local {
		if o.hasBDCopy(l) {
			copied = append(copied, l.comment.ID)
		}
	}
	if len(copied) == 0 {
		return o
	}
	o.comments = slices.DeleteFunc(slices.Clone(o.comments), func(c beads.Comment) bool {
		return slices.Contains(copied, c.ID)
	})
	o.local = slices.DeleteFunc(slices.Clone(o.local), func(l localComment) bool {
		return slices.Contains(copied, l.comment.ID)
	})
	return o
}

// hasBDCopy reports whether the entry holds bd's copy of its local copy l:
// a comment that is no local copy itself, by the same author with the same
// text, created no earlier than the second in which l's bd write started (bd
// stamps comments from the same clock, in whole seconds). bd stamps l's
// copy at some point during the write, which can be seconds later while bd
// waits on the database lock, so the rule cannot tell that copy from an
// identical comment by the same author made from that second on, by perles
// or another client: such a comment passes for it too.
func (o commentOverlay) hasBDCopy(l localComment) bool {
	since := l.since.Truncate(time.Second)
	return slices.ContainsFunc(o.comments, func(c beads.Comment) bool {
		return c.Author == l.comment.Author && c.Text == l.comment.Text &&
			!c.CreatedAt.Before(since) && !o.isLocal(c.ID)
	})
}

// isLocal reports whether the entry's comment with this ID is a local copy.
func (o commentOverlay) isLocal(id string) bool {
	return slices.ContainsFunc(o.local, func(l localComment) bool { return l.comment.ID == id })
}

// liveOverlay returns a copy of the issue's overlay entry when s does not
// cover its write. The caller holds s's read lock.
func (b *SnapshotBackend) liveOverlay(s *snapshot, issueID string) (commentOverlay, bool) {
	b.overlayMu.Lock()
	defer b.overlayMu.Unlock()
	entry, ok := b.overlay[issueID]
	if !ok || entry.seq <= s.coveredWriteSeq {
		return commentOverlay{}, false
	}
	entry.comments = slices.Clone(entry.comments)
	entry.local = slices.Clone(entry.local)
	return entry, true
}

// pruneOverlay drops the overlay entries whose write is covered by a
// snapshot with this coveredWriteSeq. install calls it after retiring the
// snapshot that one replaced. Installs run one at a time, in build order
// (every build holds the scheduler's buildMu), and covered write sequences
// only grow, so by then no reader holds a snapshot that lacks an entry it
// drops.
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
