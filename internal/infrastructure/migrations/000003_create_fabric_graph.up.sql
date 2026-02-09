-- Fabric graph tables for durable thread and dependency storage.
-- Session-scoped to prevent cross-workflow data contamination.

CREATE TABLE fabric_threads (
    session_id TEXT NOT NULL,
    id TEXT NOT NULL,
    type TEXT NOT NULL CHECK(type IN ('channel', 'message', 'artifact')),
    created_at INTEGER NOT NULL,
    created_by TEXT NOT NULL,
    content TEXT,
    kind TEXT,
    slug TEXT,
    title TEXT,
    purpose TEXT,
    name TEXT,
    media_type TEXT,
    size_bytes INTEGER,
    storage_uri TEXT,
    sha256 TEXT,
    mentions_json TEXT,       -- JSON []string
    participants_json TEXT,   -- JSON []string
    meta_json TEXT,           -- JSON map[string]string
    seq INTEGER NOT NULL,
    archived_at INTEGER,
    PRIMARY KEY (session_id, id)
);

-- Unique partial index on slug within a session (only for non-null slugs)
CREATE UNIQUE INDEX idx_fabric_threads_session_slug
    ON fabric_threads(session_id, slug) WHERE slug IS NOT NULL;

-- Deterministic list ordering by sequence number within a session
CREATE INDEX idx_fabric_threads_session_seq
    ON fabric_threads(session_id, seq);

CREATE TABLE fabric_dependencies (
    session_id TEXT NOT NULL,
    thread_id TEXT NOT NULL,
    depends_on_id TEXT NOT NULL,
    relation TEXT NOT NULL CHECK(relation IN ('child_of', 'reply_to', 'references')),
    created_at INTEGER NOT NULL,
    PRIMARY KEY (session_id, thread_id, depends_on_id, relation),
    FOREIGN KEY (session_id, thread_id) REFERENCES fabric_threads(session_id, id),
    FOREIGN KEY (session_id, depends_on_id) REFERENCES fabric_threads(session_id, id)
);

-- Parent traversal: find all parents of a given thread
CREATE INDEX idx_fabric_deps_thread
    ON fabric_dependencies(session_id, thread_id);

-- Child traversal: find all children of a given thread
CREATE INDEX idx_fabric_deps_depends_on
    ON fabric_dependencies(session_id, depends_on_id);

-- GetChildren queries filtered by relation
CREATE INDEX idx_fabric_deps_depends_on_relation
    ON fabric_dependencies(session_id, depends_on_id, relation);

-- GetParents queries filtered by relation
CREATE INDEX idx_fabric_deps_thread_relation
    ON fabric_dependencies(session_id, thread_id, relation);
