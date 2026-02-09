-- Drop fabric dependency indexes and table first (references fabric_threads via FK)
DROP INDEX IF EXISTS idx_fabric_deps_thread_relation;
DROP INDEX IF EXISTS idx_fabric_deps_depends_on_relation;
DROP INDEX IF EXISTS idx_fabric_deps_depends_on;
DROP INDEX IF EXISTS idx_fabric_deps_thread;
DROP TABLE IF EXISTS fabric_dependencies;

-- Drop fabric thread indexes and table
DROP INDEX IF EXISTS idx_fabric_threads_session_seq;
DROP INDEX IF EXISTS idx_fabric_threads_session_slug;
DROP TABLE IF EXISTS fabric_threads;
