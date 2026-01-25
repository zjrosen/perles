// Package domain provides the pure domain layer for sessions with no infrastructure dependencies.
//
// This package follows Domain-Driven Design (DDD) principles:
//   - Contains only pure Go code with standard library imports (time package only)
//   - Defines the Session entity with encapsulated state and behavior
//   - Defines the SessionRepository interface for persistence abstraction
//   - Provides domain-specific error types
//
// The domain layer has no knowledge of infrastructure concerns (databases, file I/O, etc.).
package domain

import "time"

// SessionState represents the lifecycle state of a session.
type SessionState string

const (
	// SessionStatePending indicates the session is created but not yet started.
	SessionStatePending SessionState = "pending"

	// SessionStateRunning indicates the session is currently active.
	SessionStateRunning SessionState = "running"

	// SessionStatePaused indicates the session is temporarily suspended.
	SessionStatePaused SessionState = "paused"

	// SessionStateCompleted indicates the session completed successfully.
	SessionStateCompleted SessionState = "completed"

	// SessionStateFailed indicates the session failed due to an error.
	SessionStateFailed SessionState = "failed"

	// SessionStateTimedOut indicates the session was terminated due to timeout.
	SessionStateTimedOut SessionState = "timed_out"
)

// String returns the string representation of the session state.
func (s SessionState) String() string {
	return string(s)
}

// IsValid returns true if the state is a recognized session state.
func (s SessionState) IsValid() bool {
	switch s {
	case SessionStatePending, SessionStateRunning, SessionStatePaused, SessionStateCompleted, SessionStateFailed, SessionStateTimedOut:
		return true
	default:
		return false
	}
}

// Session represents a domain entity for orchestration sessions.
// All fields are unexported to enforce encapsulation; use the constructor
// and getter methods to access data.
type Session struct {
	id         int64
	guid       string
	project    string
	name       string
	state      SessionState
	templateID string
	epicID     string
	workDir    string
	labels     map[string]string

	// Worktree configuration (requested settings)
	worktreeEnabled    bool
	worktreeBaseBranch string
	worktreeBranchName string

	// Worktree state (actual created worktree)
	worktreePath   string
	worktreeBranch string

	// Session storage path (for file-based session logs in ~/.perles/sessions/)
	sessionDir string

	// Ownership for crash recovery
	ownerCreatedPID *int
	ownerCurrentPID *int

	// Metrics
	tokensUsed    int64
	activeWorkers int

	// Health tracking
	lastHeartbeatAt *time.Time
	lastProgressAt  *time.Time

	// Timestamps
	createdAt   time.Time
	startedAt   *time.Time
	pausedAt    *time.Time
	completedAt *time.Time
	updatedAt   time.Time
	archivedAt  *time.Time
	deletedAt   *time.Time
}

// NewSession creates a new Session with the given GUID, project, and state.
// The createdAt and updatedAt timestamps are set to the current time.
// The ID is left as zero; it will be assigned by the persistence layer.
func NewSession(guid, project string, state SessionState) *Session {
	now := time.Now()
	return &Session{
		id:                 0,
		guid:               guid,
		project:            project,
		name:               "",
		state:              state,
		templateID:         "",
		epicID:             "",
		workDir:            "",
		labels:             nil,
		worktreeEnabled:    false,
		worktreeBaseBranch: "",
		worktreeBranchName: "",
		worktreePath:       "",
		worktreeBranch:     "",
		sessionDir:         "",
		ownerCreatedPID:    nil,
		ownerCurrentPID:    nil,
		tokensUsed:         0,
		activeWorkers:      0,
		lastHeartbeatAt:    nil,
		lastProgressAt:     nil,
		createdAt:          now,
		startedAt:          nil,
		pausedAt:           nil,
		completedAt:        nil,
		updatedAt:          now,
		archivedAt:         nil,
		deletedAt:          nil,
	}
}

// ReconstituteSession creates a Session from existing data, typically when
// hydrating from the database. All fields are provided explicitly.
func ReconstituteSession(
	id int64,
	guid, project, name string,
	state SessionState,
	templateID, epicID, workDir string,
	labels map[string]string,
	worktreeEnabled bool,
	worktreeBaseBranch, worktreeBranchName string,
	worktreePath, worktreeBranch string,
	sessionDir string,
	ownerCreatedPID, ownerCurrentPID *int,
	tokensUsed int64,
	activeWorkers int,
	lastHeartbeatAt, lastProgressAt *time.Time,
	createdAt time.Time,
	startedAt, pausedAt, completedAt *time.Time,
	updatedAt time.Time,
	archivedAt, deletedAt *time.Time,
) *Session {
	return &Session{
		id:                 id,
		guid:               guid,
		project:            project,
		name:               name,
		state:              state,
		templateID:         templateID,
		epicID:             epicID,
		workDir:            workDir,
		labels:             labels,
		worktreeEnabled:    worktreeEnabled,
		worktreeBaseBranch: worktreeBaseBranch,
		worktreeBranchName: worktreeBranchName,
		worktreePath:       worktreePath,
		worktreeBranch:     worktreeBranch,
		sessionDir:         sessionDir,
		ownerCreatedPID:    ownerCreatedPID,
		ownerCurrentPID:    ownerCurrentPID,
		tokensUsed:         tokensUsed,
		activeWorkers:      activeWorkers,
		lastHeartbeatAt:    lastHeartbeatAt,
		lastProgressAt:     lastProgressAt,
		createdAt:          createdAt,
		startedAt:          startedAt,
		pausedAt:           pausedAt,
		completedAt:        completedAt,
		updatedAt:          updatedAt,
		archivedAt:         archivedAt,
		deletedAt:          deletedAt,
	}
}

// ID returns the database identifier for this session.
// Returns 0 for newly created sessions that haven't been persisted.
func (s *Session) ID() int64 {
	return s.id
}

// GUID returns the globally unique identifier for this session.
func (s *Session) GUID() string {
	return s.guid
}

// Project returns the project this session belongs to.
func (s *Session) Project() string {
	return s.project
}

// Name returns the human-readable name of this session.
func (s *Session) Name() string {
	return s.name
}

// State returns the current state of this session.
func (s *Session) State() SessionState {
	return s.state
}

// TemplateID returns the workflow template ID used to create this session.
func (s *Session) TemplateID() string {
	return s.templateID
}

// EpicID returns the epic ID associated with this session, if any.
func (s *Session) EpicID() string {
	return s.epicID
}

// Labels returns the labels associated with this session.
func (s *Session) Labels() map[string]string {
	return s.labels
}

// WorktreeEnabled returns whether a git worktree was requested for this session.
func (s *Session) WorktreeEnabled() bool {
	return s.worktreeEnabled
}

// WorktreeBaseBranch returns the branch the worktree was based on.
func (s *Session) WorktreeBaseBranch() string {
	return s.worktreeBaseBranch
}

// WorktreeBranchName returns the requested branch name for the worktree.
func (s *Session) WorktreeBranchName() string {
	return s.worktreeBranchName
}

// WorkDir returns the working directory for this session.
func (s *Session) WorkDir() string {
	return s.workDir
}

// WorktreePath returns the git worktree path for this session, if any.
func (s *Session) WorktreePath() string {
	return s.worktreePath
}

// WorktreeBranch returns the git worktree branch name for this session, if any.
func (s *Session) WorktreeBranch() string {
	return s.worktreeBranch
}

// SessionDir returns the path to the file-based session logs directory.
// This is where coordinator/worker logs, messages.jsonl, and metadata.json are stored.
func (s *Session) SessionDir() string {
	return s.sessionDir
}

// OwnerCreatedPID returns the PID of the process that created this session, if set.
func (s *Session) OwnerCreatedPID() *int {
	return s.ownerCreatedPID
}

// OwnerCurrentPID returns the PID of the process currently owning this session, if set.
func (s *Session) OwnerCurrentPID() *int {
	return s.ownerCurrentPID
}

// TokensUsed returns the total tokens used by this session.
func (s *Session) TokensUsed() int64 {
	return s.tokensUsed
}

// ActiveWorkers returns the number of active workers in this session.
func (s *Session) ActiveWorkers() int {
	return s.activeWorkers
}

// LastHeartbeatAt returns when the last heartbeat was received, or nil if never.
func (s *Session) LastHeartbeatAt() *time.Time {
	return s.lastHeartbeatAt
}

// LastProgressAt returns when the last progress was made, or nil if never.
func (s *Session) LastProgressAt() *time.Time {
	return s.lastProgressAt
}

// CreatedAt returns when this session was created.
func (s *Session) CreatedAt() time.Time {
	return s.createdAt
}

// StartedAt returns when this session started running, or nil if not yet started.
func (s *Session) StartedAt() *time.Time {
	return s.startedAt
}

// PausedAt returns when this session was paused, or nil if never paused.
func (s *Session) PausedAt() *time.Time {
	return s.pausedAt
}

// CompletedAt returns when this session was completed, or nil if not completed.
func (s *Session) CompletedAt() *time.Time {
	return s.completedAt
}

// UpdatedAt returns when this session was last updated.
func (s *Session) UpdatedAt() time.Time {
	return s.updatedAt
}

// ArchivedAt returns when this session was archived, or nil if not archived.
func (s *Session) ArchivedAt() *time.Time {
	return s.archivedAt
}

// DeletedAt returns when this session was soft-deleted, or nil if not deleted.
func (s *Session) DeletedAt() *time.Time {
	return s.deletedAt
}

// IsArchived returns true if this session has been archived.
func (s *Session) IsArchived() bool {
	return s.archivedAt != nil
}

// IsDeleted returns true if this session has been soft-deleted.
func (s *Session) IsDeleted() bool {
	return s.deletedAt != nil
}

// SetName sets the human-readable name of this session.
func (s *Session) SetName(name string) {
	s.name = name
	s.updatedAt = time.Now()
}

// SetEpicID sets the epic ID associated with this session.
func (s *Session) SetEpicID(epicID string) {
	s.epicID = epicID
	s.updatedAt = time.Now()
}

// SetWorkDir sets the working directory for this session.
func (s *Session) SetWorkDir(workDir string) {
	s.workDir = workDir
	s.updatedAt = time.Now()
}

// SetTemplateID sets the workflow template ID for this session.
func (s *Session) SetTemplateID(templateID string) {
	s.templateID = templateID
	s.updatedAt = time.Now()
}

// SetWorktreePath sets the git worktree path for this session.
func (s *Session) SetWorktreePath(path string) {
	s.worktreePath = path
	s.updatedAt = time.Now()
}

// SetWorktreeBranch sets the git worktree branch name for this session.
func (s *Session) SetWorktreeBranch(branch string) {
	s.worktreeBranch = branch
	s.updatedAt = time.Now()
}

// SetSessionDir sets the path to the file-based session logs directory.
func (s *Session) SetSessionDir(dir string) {
	s.sessionDir = dir
	s.updatedAt = time.Now()
}

// SetOwnerCreatedPID sets the PID of the process that created this session.
func (s *Session) SetOwnerCreatedPID(pid *int) {
	s.ownerCreatedPID = pid
	s.updatedAt = time.Now()
}

// SetOwnerCurrentPID sets the PID of the process currently owning this session.
func (s *Session) SetOwnerCurrentPID(pid *int) {
	s.ownerCurrentPID = pid
	s.updatedAt = time.Now()
}

// SetLabels sets the labels associated with this session.
func (s *Session) SetLabels(labels map[string]string) {
	s.labels = labels
	s.updatedAt = time.Now()
}

// SetWorktreeEnabled sets whether a git worktree was requested for this session.
func (s *Session) SetWorktreeEnabled(enabled bool) {
	s.worktreeEnabled = enabled
	s.updatedAt = time.Now()
}

// SetWorktreeBaseBranch sets the branch the worktree was based on.
func (s *Session) SetWorktreeBaseBranch(branch string) {
	s.worktreeBaseBranch = branch
	s.updatedAt = time.Now()
}

// SetWorktreeBranchName sets the requested branch name for the worktree.
func (s *Session) SetWorktreeBranchName(name string) {
	s.worktreeBranchName = name
	s.updatedAt = time.Now()
}

// SetTokensUsed sets the total tokens used by this session.
func (s *Session) SetTokensUsed(tokens int64) {
	s.tokensUsed = tokens
	s.updatedAt = time.Now()
}

// AddTokens adds tokens to the usage counter.
func (s *Session) AddTokens(tokens int64) {
	s.tokensUsed += tokens
	s.updatedAt = time.Now()
}

// SetActiveWorkers sets the number of active workers in this session.
func (s *Session) SetActiveWorkers(count int) {
	s.activeWorkers = count
	s.updatedAt = time.Now()
}

// RecordHeartbeat updates the last heartbeat timestamp.
func (s *Session) RecordHeartbeat() {
	now := time.Now()
	s.lastHeartbeatAt = &now
	s.updatedAt = now
}

// RecordProgress updates the last progress timestamp.
// This also updates the heartbeat timestamp.
func (s *Session) RecordProgress() {
	now := time.Now()
	s.lastProgressAt = &now
	s.lastHeartbeatAt = &now
	s.updatedAt = now
}

// Start transitions the session to the running state and sets startedAt.
// The startedAt and updatedAt timestamps are set to the current time.
func (s *Session) Start() {
	now := time.Now()
	s.state = SessionStateRunning
	s.startedAt = &now
	s.updatedAt = now
}

// Pause transitions the session to the paused state.
// The pausedAt and updatedAt timestamps are set to the current time.
func (s *Session) Pause() {
	now := time.Now()
	s.state = SessionStatePaused
	s.pausedAt = &now
	s.updatedAt = now
}

// Resume transitions the session from paused back to running.
// The updatedAt timestamp is set to the current time.
func (s *Session) Resume() {
	s.state = SessionStateRunning
	s.updatedAt = time.Now()
}

// MarkCompleted transitions the session to the completed state.
// Both completedAt and updatedAt timestamps are set to the current time.
func (s *Session) MarkCompleted() {
	now := time.Now()
	s.state = SessionStateCompleted
	s.completedAt = &now
	s.updatedAt = now
}

// MarkFailed transitions the session to the failed state.
// The updatedAt timestamp is set to the current time.
func (s *Session) MarkFailed() {
	s.state = SessionStateFailed
	s.updatedAt = time.Now()
}

// MarkTimedOut transitions the session to the timed_out state.
// The updatedAt timestamp is set to the current time.
func (s *Session) MarkTimedOut() {
	s.state = SessionStateTimedOut
	s.updatedAt = time.Now()
}

// Archive marks the session as archived.
// Both archivedAt and updatedAt timestamps are set to the current time.
func (s *Session) Archive() {
	now := time.Now()
	s.archivedAt = &now
	s.updatedAt = now
}

// SoftDelete marks the session as deleted without removing it from storage.
// Both deletedAt and updatedAt timestamps are set to the current time.
func (s *Session) SoftDelete() {
	now := time.Now()
	s.deletedAt = &now
	s.updatedAt = now
}

// SetID sets the database identifier for this session.
// This is typically called by the persistence layer after inserting a new session.
func (s *Session) SetID(id int64) {
	s.id = id
}
