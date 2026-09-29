package session

import (
	"fmt"
	"strings"
	"time"
)

// Factory creates Session instances with consistent path building and configuration.
// It is shared between mode/orchestration.Initializer and controlplane.Supervisor
// to ensure sessions are created with the same directory structure and options.
type Factory struct {
	baseDir         string
	applicationName string
	gitExecutor     GitRemoteGetter
}

// FactoryConfig holds configuration for creating a Factory.
type FactoryConfig struct {
	// BaseDir is the root directory for session storage.
	// If empty, DefaultBaseDir() (~/.perles/sessions) is used.
	BaseDir string

	// ApplicationName overrides the derived application name for every session
	// created by this factory (orchestration.session_storage.application_name).
	// Surrounding whitespace is trimmed. If empty, the name is derived from the
	// git remote or the work directory basename.
	ApplicationName string

	// GitExecutor is used to derive the application name from git remote.
	// If nil, the application name falls back to the work directory basename.
	GitExecutor GitRemoteGetter
}

// NewFactory creates a new session Factory with the given configuration.
func NewFactory(cfg FactoryConfig) *Factory {
	baseDir := cfg.BaseDir
	if baseDir == "" {
		baseDir = DefaultBaseDir()
	}
	return &Factory{
		baseDir:         baseDir,
		applicationName: strings.TrimSpace(cfg.ApplicationName),
		gitExecutor:     cfg.GitExecutor,
	}
}

// ResolveApplicationName returns the application name used for sessions rooted at workDir.
// Resolution order:
//  1. FactoryConfig.ApplicationName (the configured override), if non-empty
//  2. Git remote "origin" repository name (when a GitExecutor is configured)
//  3. Basename of workDir
func (f *Factory) ResolveApplicationName(workDir string) string {
	if f.applicationName != "" {
		return f.applicationName
	}
	return DeriveApplicationName(workDir, f.gitExecutor)
}

// CreateOptions holds options for creating a new session.
type CreateOptions struct {
	// SessionID is the unique identifier for the session (typically a UUID).
	SessionID string

	// WorkDir is the project working directory.
	// Used for deriving application name if not explicitly set.
	WorkDir string

	// ApplicationName overrides the application name for this session only.
	// If empty, the factory's ResolveApplicationName is used (configured
	// override, then git remote, then WorkDir basename).
	ApplicationName string

	// WorkflowID is the ID of the workflow this session belongs to.
	// Enables frontend to route API calls to the correct active workflow.
	WorkflowID string
}

// Create creates a new session with the given options.
// It derives the application name, builds the session path, creates the directory
// structure, and returns an initialized Session ready for writing.
func (f *Factory) Create(opts CreateOptions) (*Session, error) {
	if opts.SessionID == "" {
		return nil, fmt.Errorf("SessionID is required")
	}
	if opts.WorkDir == "" {
		return nil, fmt.Errorf("WorkDir is required")
	}

	// Resolve application name (per-call override, then factory override, then derived)
	appName := opts.ApplicationName
	if appName == "" {
		appName = f.ResolveApplicationName(opts.WorkDir)
	}

	// Build session path
	pathBuilder := NewSessionPathBuilder(f.baseDir, appName)
	now := time.Now()
	sessionDir := pathBuilder.SessionDir(opts.SessionID, now)
	datePartition := now.Format("2006-01-02")

	// Create session with all options
	sess, err := New(opts.SessionID, sessionDir,
		WithWorkDir(opts.WorkDir),
		WithApplicationName(appName),
		WithDatePartition(datePartition),
		WithPathBuilder(pathBuilder),
		WithWorkflowID(opts.WorkflowID),
	)
	if err != nil {
		return nil, fmt.Errorf("creating session: %w", err)
	}

	return sess, nil
}

// Reopen reopens an existing session for continued writing (session resumption).
// This is used when restoring from a previous session.
func (f *Factory) Reopen(opts CreateOptions, existingDir string) (*Session, error) {
	if opts.SessionID == "" {
		return nil, fmt.Errorf("SessionID is required")
	}
	if existingDir == "" {
		return nil, fmt.Errorf("existingDir is required")
	}

	// Resolve application name (per-call override, then factory override, then derived)
	appName := opts.ApplicationName
	if appName == "" {
		appName = f.ResolveApplicationName(opts.WorkDir)
	}

	// Build path builder for consistency (needed for index updates)
	pathBuilder := NewSessionPathBuilder(f.baseDir, appName)

	// Extract date partition from existing directory if possible
	// Format: {base}/{app}/{date}/{id} - we need the date component
	datePartition := time.Now().Format("2006-01-02") // fallback

	// Reopen existing session in append mode
	sess, err := Reopen(opts.SessionID, existingDir,
		WithWorkDir(opts.WorkDir),
		WithApplicationName(appName),
		WithDatePartition(datePartition),
		WithPathBuilder(pathBuilder),
	)
	if err != nil {
		return nil, fmt.Errorf("reopening session: %w", err)
	}

	return sess, nil
}

// BaseDir returns the configured base directory.
func (f *Factory) BaseDir() string {
	return f.baseDir
}
