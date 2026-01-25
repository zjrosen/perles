-- Sessions table for orchestration session tracking
-- Multi-tenant via project column, single database at ~/.perles/perles.db
CREATE TABLE sessions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    guid TEXT NOT NULL UNIQUE,
    project TEXT NOT NULL,
    name TEXT,
    state TEXT NOT NULL CHECK(state IN ('pending', 'running', 'paused', 'completed', 'failed', 'timed_out')),
    template_id TEXT,
    epic_id TEXT,
    work_dir TEXT,
    labels TEXT,  -- JSON encoded map[string]string

    -- Worktree configuration (requested settings)
    worktree_enabled INTEGER NOT NULL DEFAULT 0,
    worktree_base_branch TEXT,
    worktree_branch_name TEXT,

    -- Worktree state (actual created worktree)
    worktree_path TEXT,
    worktree_branch TEXT,

    -- Session storage path (for file-based session logs in ~/.perles/sessions/)
    session_dir TEXT,

    -- Ownership for crash recovery
    owner_created_pid INTEGER,
    owner_current_pid INTEGER,

    -- Metrics
    tokens_used INTEGER NOT NULL DEFAULT 0,
    active_workers INTEGER NOT NULL DEFAULT 0,

    -- Health tracking
    last_heartbeat_at INTEGER,
    last_progress_at INTEGER,

    -- Timestamps
    created_at INTEGER NOT NULL,
    started_at INTEGER,
    paused_at INTEGER,
    completed_at INTEGER,
    updated_at INTEGER NOT NULL,
    archived_at INTEGER,
    deleted_at INTEGER
);

-- Indexes for common query patterns
CREATE INDEX idx_sessions_project ON sessions(project);
CREATE INDEX idx_sessions_guid ON sessions(guid);
CREATE INDEX idx_sessions_deleted_at ON sessions(deleted_at);
CREATE INDEX idx_sessions_archived_at ON sessions(archived_at);
CREATE INDEX idx_sessions_project_state ON sessions(project, state) WHERE deleted_at IS NULL;
