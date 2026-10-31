package bql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	appbeads "github.com/zjrosen/perles/internal/beads/application"
	beads "github.com/zjrosen/perles/internal/beads/domain"
	"github.com/zjrosen/perles/internal/cachemanager"
	"github.com/zjrosen/perles/internal/log"
)

// BQLExecutor executes BQL queries and returns matching issues.
// This interface is implemented by *Executor and mocked in tests.
type BQLExecutor interface {
	Execute(query string) ([]beads.Issue, error)
}

// Verify Executor implements BQLExecutor at compile time.
var _ BQLExecutor = (*Executor)(nil)

// querier is the common interface between *sql.DB and *sql.Tx for running queries.
type querier interface {
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

// Executor runs BQL queries against the database.
type Executor struct {
	db            *sql.DB
	dialect       appbeads.SQLDialect
	cacheManager  cachemanager.CacheManager[string, []beads.Issue]
	depGraphCache cachemanager.CacheManager[string, *DependencyGraph]

	// schema is the probed database schema, cached after the first query and
	// reset when a query fails in a way that suggests the schema changed.
	schemaMu sync.Mutex
	schema   *Schema
}

// depGraphCacheKey is the static key for caching the dependency graph.
const depGraphCacheKey = "__dependency_graph__"

// NewExecutor creates a new query executor.
func NewExecutor(
	db *sql.DB,
	dialect appbeads.SQLDialect,
	cacheManager cachemanager.CacheManager[string, []beads.Issue],
	depGraphCache cachemanager.CacheManager[string, *DependencyGraph],
) *Executor {
	return &Executor{
		db:            db,
		dialect:       dialect,
		cacheManager:  cacheManager,
		depGraphCache: depGraphCache,
	}
}

// maxExpandIterations is the safety limit for unlimited depth expansion.
const maxExpandIterations = 100

// DependencyEdge represents an edge in the dependency graph.
type DependencyEdge struct {
	TargetID string
	Type     string // "parent-child", "blocks", "discovered-from"
}

// DependencyGraph represents the in-memory dependency graph with bidirectional edges.
// Forward edges go from issue_id -> target issue (e.g., child -> parent, blocked -> blocker)
// Reverse edges go from target issue -> issue_id (e.g., parent -> children, blocker -> blocked)
type DependencyGraph struct {
	Forward map[string][]DependencyEdge // issue_id -> target issue edges
	Reverse map[string][]DependencyEdge // target issue -> dependent issue edges
}

// Execute runs a BQL query and returns matching issues.
// Dolt transient error retry configuration.
// During rapid writes (e.g., bulk deletes), the Dolt server may return internal
// errors like "unable to find field with index N in row of 0 columns" as its
// MVCC state catches up. A brief retry with a fresh transaction resolves these.
const (
	doltRetryAttempts   = 6
	doltRetryBaseDelay  = 250 * time.Millisecond
	doltRetryMultiplier = 2 // exponential backoff: 250ms, 500ms, 1s, 2s, 4s
)

// isDoltTransientError returns true if the error is a transient Dolt server
// inconsistency that is likely to resolve on retry with a fresh snapshot.
func isDoltTransientError(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	// Dolt internal error during concurrent modifications
	if strings.Contains(errStr, "unable to find field with index") {
		return true
	}
	// MySQL driver transient errors
	if strings.Contains(errStr, "driver: bad connection") {
		return true
	}
	if strings.Contains(errStr, "invalid connection") {
		return true
	}
	if strings.Contains(errStr, "broken pipe") {
		return true
	}
	if strings.Contains(errStr, "connection reset") {
		return true
	}
	return false
}

func (e *Executor) Execute(input string) ([]beads.Issue, error) {
	start := time.Now()

	// Parse the query
	parser := NewParser(input)
	query, err := parser.Parse()
	if err != nil {
		log.ErrorErr(log.CatBQL, "Parse failed", err, "query", input)
		return nil, fmt.Errorf("parse error: %w", err)
	}

	// Validate the query
	if err := Validate(query); err != nil {
		log.ErrorErr(log.CatBQL, "Validation failed", err, "query", input)
		return nil, fmt.Errorf("validation error: %w", err)
	}

	// executeWithSnapshot runs all queries in a single transaction (Dolt) or
	// directly against the DB (SQLite). Each call gets a fresh MVCC snapshot.
	executeWithSnapshot := func() ([]beads.Issue, error) {
		var q querier = e.db
		if e.dialect == appbeads.DialectMySQL {
			tx, err := e.db.BeginTx(context.Background(), nil)
			if err != nil {
				return nil, fmt.Errorf("begin transaction: %w", err)
			}
			q = tx
			defer func() { _ = tx.Rollback() }()
		}

		issues, err := e.executeBaseQuery(query, q)
		if err != nil {
			return nil, err
		}

		if query.HasExpand() {
			issues, err = e.expandIssues(issues, query.Expand, q)
			if err != nil {
				return nil, err
			}
		}

		return issues, nil
	}

	// executeWithSchemaRefresh re-probes the schema and retries once when a
	// query fails in a way that suggests the cached schema is stale (bd
	// migrated the database while perles was running).
	executeWithSchemaRefresh := func() ([]beads.Issue, error) {
		issues, err := executeWithSnapshot()
		if err == nil || !isStaleSchemaError(err) {
			return issues, err
		}
		log.Debug(log.CatBQL, "Query failed on a possibly stale schema, re-probing", "error", err, "query", input)
		e.resetSchema()
		return executeWithSnapshot()
	}

	// Wrap with retry for transient Dolt errors. Each retry opens a fresh
	// transaction so the snapshot advances past the inconsistency.
	executeWithRetry := func() ([]beads.Issue, error) {
		attempts := 1
		if e.dialect == appbeads.DialectMySQL {
			attempts = doltRetryAttempts
		}
		var lastErr error
		for i := range attempts {
			issues, err := executeWithSchemaRefresh()
			if err == nil {
				return issues, nil
			}
			lastErr = err
			if !isDoltTransientError(err) || i == attempts-1 {
				return nil, lastErr
			}
			delay := doltRetryBaseDelay * time.Duration(1<<uint(i)) // 250ms, 500ms, 1s, 2s, 4s
			log.Debug(log.CatBQL, "Transient Dolt error, retrying",
				"attempt", i+1, "delay", delay, "error", err, "query", input)
			time.Sleep(delay)
		}
		return nil, lastErr
	}

	cache := cachemanager.NewReadThroughCache(
		e.cacheManager,
		func(ctx context.Context, _ *Query) ([]beads.Issue, error) {
			return executeWithRetry()
		},
		false,
	)
	issues, err := cache.GetWithRefresh(context.Background(), input, query, cachemanager.DefaultExpiration)
	if err != nil {
		log.ErrorErr(log.CatBQL, "failed to load issues", err, "query", input)
		return nil, err
	}

	log.Debug(log.CatBQL, "query complete", "duration", time.Since(start), "count", len(issues), "query", input)

	return issues, nil
}

// FieldUnavailableError reports a BQL field whose column the probed beads
// database does not have (its schema predates the column).
type FieldUnavailableError struct {
	Field string
}

func (e *FieldUnavailableError) Error() string {
	return fmt.Sprintf("field %q is not available in this beads database (upgrade beads)", e.Field)
}

// checkFieldsAvailable returns a *FieldUnavailableError for the first field
// in the query's filter or ORDER BY whose issues column the schema lacks.
func checkFieldsAvailable(query *Query, schema *Schema) error {
	check := func(field string) error {
		column, ok := issueColumnForField(field)
		if ok && !schema.HasIssueColumn(column) {
			return &FieldUnavailableError{Field: field}
		}
		return nil
	}

	var walk func(Expr) error
	walk = func(expr Expr) error {
		switch e := expr.(type) {
		case *BinaryExpr:
			if err := walk(e.Left); err != nil {
				return err
			}
			return walk(e.Right)
		case *NotExpr:
			return walk(e.Expr)
		case *CompareExpr:
			return check(e.Field)
		case *InExpr:
			return check(e.Field)
		}
		return nil
	}

	if query.Filter != nil {
		if err := walk(query.Filter); err != nil {
			return err
		}
	}
	for _, term := range query.OrderBy {
		if err := check(term.Field); err != nil {
			return err
		}
	}
	return nil
}

// resolveSchema returns the cached schema, probing the database on first use.
// The probe runs on q so it sees the same snapshot (and, for SQLite in-memory
// test databases, the same connection) as the query.
func (e *Executor) resolveSchema(q querier) (*Schema, error) {
	e.schemaMu.Lock()
	defer e.schemaMu.Unlock()
	if e.schema != nil {
		return e.schema, nil
	}

	schema, err := probeSchema(q)
	if err != nil {
		return nil, err
	}
	log.Debug(log.CatBQL, "Probed beads schema",
		"dependencyTarget", schema.DependencyTarget,
		"dependsOnExternal", schema.HasDependsOnExternal,
		"isBlocked", schema.HasIsBlocked,
		"customStatuses", schema.HasCustomStatuses,
		"customTypes", schema.HasCustomTypes)
	e.schema = schema
	return schema, nil
}

// resetSchema drops the cached schema so the next query probes again.
func (e *Executor) resetSchema() {
	e.schemaMu.Lock()
	defer e.schemaMu.Unlock()
	e.schema = nil
}

// probeSchema inspects the issues and dependencies columns and whether the
// custom_statuses and custom_types tables exist. It only uses
// SELECT * ... LIMIT 0, which works on both MySQL/Dolt and SQLite.
func probeSchema(q querier) (*Schema, error) {
	issueColumns, err := tableColumns(q, "issues")
	if err != nil {
		return nil, err
	}
	dependencyColumns, err := tableColumns(q, "dependencies")
	if err != nil {
		return nil, err
	}

	schema := &Schema{
		IssueColumns:         issueColumns,
		HasIsBlocked:         issueColumns["is_blocked"],
		HasDependsOnExternal: dependencyColumns[externalDependencyTargetColumn],
	}
	switch {
	case dependencyColumns[issueDependencyTargetColumn]:
		schema.DependencyTarget = issueDependencyTargetColumn
	case dependencyColumns[legacyDependencyTargetColumn]:
		schema.DependencyTarget = legacyDependencyTargetColumn
	default:
		return nil, fmt.Errorf("dependencies table has no supported target column (%s or %s)",
			issueDependencyTargetColumn, legacyDependencyTargetColumn)
	}

	if schema.HasCustomStatuses, err = tableExists(q, "custom_statuses"); err != nil {
		return nil, err
	}
	if schema.HasCustomTypes, err = tableExists(q, "custom_types"); err != nil {
		return nil, err
	}
	return schema, nil
}

// tableColumns returns the set of column names of a table.
func tableColumns(q querier, table string) (map[string]bool, error) {
	rows, err := q.Query("SELECT * FROM " + table + " LIMIT 0") //nolint:gosec // G202: table is a hardcoded name
	if err != nil {
		return nil, fmt.Errorf("inspect %s schema: %w", table, err)
	}
	defer func() { _ = rows.Close() }()

	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("read %s columns: %w", table, err)
	}
	set := make(map[string]bool, len(columns))
	for _, column := range columns {
		set[strings.ToLower(column)] = true
	}
	return set, nil
}

// tableExists reports whether a table exists. Errors other than a missing
// table are returned so a transient failure is not cached as "absent".
func tableExists(q querier, table string) (bool, error) {
	rows, err := q.Query("SELECT * FROM " + table + " LIMIT 0") //nolint:gosec // G202: table is a hardcoded name
	if err != nil {
		if isMissingTableError(err) {
			return false, nil
		}
		return false, fmt.Errorf("inspect %s table: %w", table, err)
	}
	_ = rows.Close()
	return true, nil
}

// isMissingTableError matches "table does not exist" errors from SQLite
// ("no such table"), MySQL (error 1146, "Table 'db.t' doesn't exist") and
// Dolt ("table not found: t").
func isMissingTableError(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no such table") ||
		strings.Contains(msg, "doesn't exist") ||
		strings.Contains(msg, "table not found")
}

// isStaleSchemaError reports errors that a schema change could explain: a
// field the cached schema lacks, or SQL that references a missing column or
// table.
func isStaleSchemaError(err error) bool {
	var unavailable *FieldUnavailableError
	if errors.As(err, &unavailable) {
		return true
	}
	if isMissingTableError(err) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no such column") ||
		strings.Contains(msg, "unknown column") ||
		(strings.Contains(msg, "column") && strings.Contains(msg, "could not be found"))
}

// IssueDeps holds all dependency data for an issue, grouped by type.
type IssueDeps struct {
	ParentID       string   // Single parent (parent-child where this is child)
	BlockedBy      []string // Issues that block this one
	Blocks         []string // Issues this one blocks
	Children       []string // Child issues (parent-child where this is parent)
	DiscoveredFrom []string // Issues this was discovered from
	Discovered     []string // Issues discovered from this one
}

// softDeleteFilter returns the SQL WHERE clause fragment that excludes soft-deleted issues.
// For SQLite, this includes both status and deleted_at checks (tombstone support).
// For Dolt/MySQL, deleted_at column does not exist; hard deletes remove rows entirely.
func (e *Executor) softDeleteFilter(alias string) string {
	if e.dialect == appbeads.DialectMySQL {
		// Dolt uses hard deletes; no tombstone columns exist.
		// Status filter kept as defense-in-depth.
		return alias + ".status NOT IN ('deleted', 'tombstone')"
	}
	// SQLite: belt-and-suspenders with both status and deleted_at checks.
	return alias + ".status NOT IN ('deleted', 'tombstone') AND " + alias + ".deleted_at IS NULL"
}

// executeBaseQuery runs the main BQL filter query with batch-loaded dependencies.
// This uses 3 total queries instead of 8N correlated subqueries:
// 1. Main query (no dependency subqueries)
// 2. Batch load dependencies for all result IDs
// 3. Batch load labels for all result IDs
// 4. Batch load comment counts for all result IDs
func (e *Executor) executeBaseQuery(query *Query, q querier) ([]beads.Issue, error) {
	// Build SQL for the probed schema
	schema, err := e.resolveSchema(q)
	if err != nil {
		return nil, err
	}
	if err := checkFieldsAvailable(query, schema); err != nil {
		return nil, err
	}
	builder := NewSQLBuilder(query, e.dialect, WithSchema(schema))
	whereClause, orderBy, params := builder.Build()

	// Construct main query WITHOUT dependency subqueries
	//nolint:gosec // G201: softDeleteFilter returns hardcoded SQL fragments with table alias, not user input
	sqlQuery := fmt.Sprintf(`
		SELECT
			i.id,
			i.title,
			i.description,
			i.design,
			i.acceptance_criteria,
			i.notes,
			i.status,
			i.priority,
			i.issue_type,
			i.assignee,
			i.sender,
			i.ephemeral,
			i.pinned,
			i.is_template,
			i.created_at,
			i.created_by,
			i.updated_at,
			i.closed_at,
			i.defer_until,
			i.close_reason,
			i.mol_type
		FROM issues i
		WHERE %s
	`, e.softDeleteFilter("i"))

	if whereClause != "" {
		sqlQuery += " AND " + whereClause //nolint:gosec // whereClause is built from validated BQL fields, not raw user input
	}

	if orderBy != "" {
		sqlQuery += " ORDER BY " + orderBy //nolint:gosec // orderBy is built from validated BQL field names
	} else {
		sqlQuery += " ORDER BY i.updated_at DESC"
	}

	// Execute main query
	rows, err := q.Query(sqlQuery, params...)
	if err != nil {
		log.ErrorErr(log.CatDB, "Query failed", err)
		return nil, fmt.Errorf("query error: %w", err)
	}
	defer func() { _ = rows.Close() }()

	// Scan base issue data (without dependencies)
	issues, err := e.scanIssuesBase(rows)
	if err != nil {
		return nil, err
	}

	if len(issues) == 0 {
		return issues, nil
	}

	// Collect issue IDs for batch loading
	ids := make([]string, len(issues))
	for i, issue := range issues {
		ids[i] = issue.ID
	}

	// Batch load dependencies (1 query)
	deps, err := e.loadDependenciesForIssues(ids, q)
	if err != nil {
		return nil, fmt.Errorf("load dependencies: %w", err)
	}

	// Batch load labels (1 query)
	labels, err := e.loadLabelsForIssues(ids, q)
	if err != nil {
		return nil, fmt.Errorf("load labels: %w", err)
	}

	// Batch load comment counts (1 query)
	commentCounts, err := e.loadCommentCountsForIssues(ids, q)
	if err != nil {
		return nil, fmt.Errorf("load comment counts: %w", err)
	}

	// Attach batch-loaded data to issues
	for i := range issues {
		id := issues[i].ID
		if d, ok := deps[id]; ok {
			issues[i].ParentID = d.ParentID
			issues[i].BlockedBy = d.BlockedBy
			issues[i].Blocks = d.Blocks
			issues[i].Children = d.Children
			issues[i].DiscoveredFrom = d.DiscoveredFrom
			issues[i].Discovered = d.Discovered
		}
		if l, ok := labels[id]; ok {
			issues[i].Labels = l
		}
		if c, ok := commentCounts[id]; ok {
			issues[i].CommentCount = c
		}
	}

	return issues, nil
}

// scanIssuesBase reads base issue data from database rows (without dependency fields).
func (e *Executor) scanIssuesBase(rows *sql.Rows) ([]beads.Issue, error) {
	var issues []beads.Issue
	for rows.Next() {
		var (
			issue              beads.Issue
			description        sql.NullString
			design             sql.NullString
			acceptanceCriteria sql.NullString
			notes              sql.NullString
			assignee           sql.NullString
			sender             sql.NullString
			ephemeral          sql.NullBool
			pinned             sql.NullBool
			isTemplate         sql.NullBool
			createdBy          sql.NullString
			closedAt           sql.NullTime
			deferUntil         sql.NullTime
			closeReason        sql.NullString
			molType            sql.NullString
		)

		err := rows.Scan(
			&issue.ID,
			&issue.TitleText,
			&description,
			&design,
			&acceptanceCriteria,
			&notes,
			&issue.Status,
			&issue.Priority,
			&issue.Type,
			&assignee,
			&sender,
			&ephemeral,
			&pinned,
			&isTemplate,
			&issue.CreatedAt,
			&createdBy,
			&issue.UpdatedAt,
			&closedAt,
			&deferUntil,
			&closeReason,
			&molType,
		)
		if err != nil {
			log.ErrorErr(log.CatDB, "Scan failed", err)
			return nil, fmt.Errorf("scan error: %w", err)
		}

		if description.Valid {
			issue.DescriptionText = description.String
		}
		if design.Valid {
			issue.Design = design.String
		}
		if acceptanceCriteria.Valid {
			issue.AcceptanceCriteria = acceptanceCriteria.String
		}
		if notes.Valid {
			issue.Notes = notes.String
		}
		if assignee.Valid {
			issue.Assignee = assignee.String
		}
		if sender.Valid {
			issue.Sender = sender.String
		}
		if ephemeral.Valid && ephemeral.Bool {
			issue.Ephemeral = true
		}
		if pinned.Valid {
			issue.Pinned = &pinned.Bool
		}
		if isTemplate.Valid {
			issue.IsTemplate = &isTemplate.Bool
		}
		if createdBy.Valid {
			issue.CreatedBy = createdBy.String
		}
		if closedAt.Valid {
			issue.ClosedAt = closedAt.Time
		}
		if deferUntil.Valid {
			issue.DeferUntil = deferUntil.Time
		}
		if closeReason.Valid {
			issue.CloseReason = closeReason.String
		}
		if molType.Valid {
			issue.MolType = molType.String
		}

		issues = append(issues, issue)
	}

	return issues, rows.Err()
}

// loadDependenciesForIssues batch-loads all dependency data for the given issue IDs.
// Returns a map of issue ID -> IssueDeps with grouped dependencies by type.
// This replaces 6 correlated subqueries with a single IN-clause query.
func (e *Executor) loadDependenciesForIssues(ids []string, q querier) (map[string]IssueDeps, error) {
	if len(ids) == 0 {
		return make(map[string]IssueDeps), nil
	}

	// Build placeholders for IN clause
	placeholders := make([]string, len(ids))
	params := make([]any, 0, len(ids)*2)
	for i, id := range ids {
		placeholders[i] = "?"
		params = append(params, id)
	}
	inClause := strings.Join(placeholders, ",")

	// Duplicate params for the second IN clause
	for _, id := range ids {
		params = append(params, id)
	}

	// Single query to get all dependencies for all issues (both directions)
	// Filter out deleted issues on the related side
	softDelete := e.softDeleteFilter("i")
	schema, err := e.resolveSchema(q)
	if err != nil {
		return nil, err
	}
	// The target expression includes depends_on_external when it exists, so
	// cross-prefix edges to local issues load too; the JOIN to issues keeps
	// only targets that are local issues.
	dependencyTarget := schema.DependencyTargetExpr("d")
	//nolint:gosec // G201: inClause contains only "?" placeholders and dependencyTarget is built from probed column names.
	query := fmt.Sprintf(`
		SELECT d.issue_id, %s, d.type
		FROM dependencies d
		JOIN issues i ON %s = i.id
		WHERE d.issue_id IN (%s)
		  AND %s
		UNION
		SELECT d.issue_id, %s, d.type
		FROM dependencies d
		JOIN issues i ON d.issue_id = i.id
		WHERE %s IN (%s)
		  AND %s
	`, dependencyTarget, dependencyTarget, inClause, softDelete, dependencyTarget, dependencyTarget, inClause, softDelete)

	rows, err := q.Query(query, params...)
	if err != nil {
		log.ErrorErr(log.CatDB, "Failed to batch load dependencies", err)
		return nil, fmt.Errorf("batch load dependencies: %w", err)
	}
	defer func() { _ = rows.Close() }()

	// Create a set of target IDs for quick lookup
	targetSet := make(map[string]bool)
	for _, id := range ids {
		targetSet[id] = true
	}

	// Group dependencies by issue ID and type
	result := make(map[string]IssueDeps)
	for rows.Next() {
		var issueID, dependsOnID, depType string
		if err := rows.Scan(&issueID, &dependsOnID, &depType); err != nil {
			log.ErrorErr(log.CatDB, "Failed to scan dependency row", err)
			return nil, fmt.Errorf("scan dependency: %w", err)
		}

		// Process based on dependency type and direction
		switch depType {
		case "parent-child":
			// issueID is child, dependsOnID is parent
			if targetSet[issueID] {
				deps := result[issueID]
				deps.ParentID = dependsOnID
				result[issueID] = deps
			}
			// dependsOnID is parent, issueID is child
			if targetSet[dependsOnID] {
				deps := result[dependsOnID]
				deps.Children = append(deps.Children, issueID)
				result[dependsOnID] = deps
			}
		case "blocks":
			// issueID is blocked by dependsOnID
			if targetSet[issueID] {
				deps := result[issueID]
				deps.BlockedBy = append(deps.BlockedBy, dependsOnID)
				result[issueID] = deps
			}
			// dependsOnID blocks issueID
			if targetSet[dependsOnID] {
				deps := result[dependsOnID]
				deps.Blocks = append(deps.Blocks, issueID)
				result[dependsOnID] = deps
			}
		case "discovered-from":
			// issueID was discovered from dependsOnID
			if targetSet[issueID] {
				deps := result[issueID]
				deps.DiscoveredFrom = append(deps.DiscoveredFrom, dependsOnID)
				result[issueID] = deps
			}
			// dependsOnID has issueID as discovered
			if targetSet[dependsOnID] {
				deps := result[dependsOnID]
				deps.Discovered = append(deps.Discovered, issueID)
				result[dependsOnID] = deps
			}
		}
	}

	if err := rows.Err(); err != nil {
		log.ErrorErr(log.CatDB, "Dependency rows error", err)
		return nil, fmt.Errorf("dependency rows: %w", err)
	}

	return result, nil
}

// loadLabelsForIssues batch-loads all labels for the given issue IDs.
// Returns a map of issue ID -> label slice.
func (e *Executor) loadLabelsForIssues(ids []string, q querier) (map[string][]string, error) {
	if len(ids) == 0 {
		return make(map[string][]string), nil
	}

	// Build placeholders for IN clause
	placeholders := make([]string, len(ids))
	params := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		params[i] = id
	}
	inClause := strings.Join(placeholders, ",")

	//nolint:gosec // G201: inClause contains only "?" placeholders, not user input
	query := fmt.Sprintf(`
		SELECT issue_id, label
		FROM labels
		WHERE issue_id IN (%s)
	`, inClause)

	rows, err := q.Query(query, params...)
	if err != nil {
		log.ErrorErr(log.CatDB, "Failed to batch load labels", err)
		return nil, fmt.Errorf("batch load labels: %w", err)
	}
	defer func() { _ = rows.Close() }()

	result := make(map[string][]string)
	for rows.Next() {
		var issueID, label string
		if err := rows.Scan(&issueID, &label); err != nil {
			log.ErrorErr(log.CatDB, "Failed to scan label row", err)
			return nil, fmt.Errorf("scan label: %w", err)
		}
		result[issueID] = append(result[issueID], label)
	}

	if err := rows.Err(); err != nil {
		log.ErrorErr(log.CatDB, "Label rows error", err)
		return nil, fmt.Errorf("label rows: %w", err)
	}

	return result, nil
}

// loadCommentCountsForIssues batch-loads comment counts for the given issue IDs.
// Returns a map of issue ID -> comment count.
func (e *Executor) loadCommentCountsForIssues(ids []string, q querier) (map[string]int, error) {
	if len(ids) == 0 {
		return make(map[string]int), nil
	}

	// Build placeholders for IN clause
	placeholders := make([]string, len(ids))
	params := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		params[i] = id
	}
	inClause := strings.Join(placeholders, ",")

	//nolint:gosec // G201: inClause contains only "?" placeholders, not user input
	query := fmt.Sprintf(`
		SELECT issue_id, COUNT(*)
		FROM comments
		WHERE issue_id IN (%s)
		GROUP BY issue_id
	`, inClause)

	rows, err := q.Query(query, params...)
	if err != nil {
		log.ErrorErr(log.CatDB, "Failed to batch load comment counts", err)
		return nil, fmt.Errorf("batch load comment counts: %w", err)
	}
	defer func() { _ = rows.Close() }()

	result := make(map[string]int)
	for rows.Next() {
		var issueID string
		var count int
		if err := rows.Scan(&issueID, &count); err != nil {
			log.ErrorErr(log.CatDB, "Failed to scan comment count row", err)
			return nil, fmt.Errorf("scan comment count: %w", err)
		}
		result[issueID] = count
	}

	if err := rows.Err(); err != nil {
		log.ErrorErr(log.CatDB, "Comment count rows error", err)
		return nil, fmt.Errorf("comment count rows: %w", err)
	}

	return result, nil
}

// expandIssues uses graph-based traversal to expand issues based on the expansion configuration.
// This approach loads the full dependency graph once (1 SQL query) and traverses in-memory,
// then batch-fetches all discovered issues (1 SQL query). This replaces the previous O(D×N)
// iterative SQL approach with O(2) queries + O(V+E) in-memory traversal.
func (e *Executor) expandIssues(baseIssues []beads.Issue, expand *ExpandClause, q querier) ([]beads.Issue, error) {
	if len(baseIssues) == 0 {
		return baseIssues, nil
	}

	// Step 1: Load the full dependency graph (ONE SQL query)
	graph, err := e.loadDependencyGraph(q)
	if err != nil {
		return nil, fmt.Errorf("load dependency graph: %w", err)
	}

	// Step 2: Extract starting IDs from base issues
	startIDs := make([]string, len(baseIssues))
	for i, issue := range baseIssues {
		startIDs[i] = issue.ID
	}

	// Step 3: BFS/DFS traversal in Go (no SQL)
	allIDs := e.traverseGraph(graph, startIDs, expand.Type, int(expand.Depth))

	// Step 4: Identify IDs we need to fetch (exclude base issues we already have)
	baseIDSet := make(map[string]bool)
	for _, issue := range baseIssues {
		baseIDSet[issue.ID] = true
	}

	var newIDs []string
	for _, id := range allIDs {
		if !baseIDSet[id] {
			newIDs = append(newIDs, id)
		}
	}

	// If no new IDs to fetch, just return base issues
	if len(newIDs) == 0 {
		return baseIssues, nil
	}

	// Step 5: Batch fetch all new issues (ONE SQL query)
	newIssues, err := e.fetchIssuesByIDs(newIDs, q)
	if err != nil {
		return nil, fmt.Errorf("fetch expanded issues: %w", err)
	}

	// Combine base issues with expanded issues
	allIssues := make([]beads.Issue, 0, len(baseIssues)+len(newIssues))
	allIssues = append(allIssues, baseIssues...)
	allIssues = append(allIssues, newIssues...)

	log.Debug(log.CatBQL, "Graph-based expand complete",
		"baseCount", len(baseIssues),
		"expandedCount", len(newIssues),
		"totalCount", len(allIssues))

	return allIssues, nil
}

// BuildIDQuery constructs a BQL query to fetch issues by their IDs.
// Returns empty string if ids is empty.
func BuildIDQuery(ids []string) string {
	if len(ids) == 0 {
		return ""
	}
	if len(ids) == 1 {
		return fmt.Sprintf(`id = %q`, ids[0])
	}

	quoted := make([]string, len(ids))
	for i, id := range ids {
		quoted[i] = fmt.Sprintf("%q", id)
	}
	return fmt.Sprintf("id in (%s)", strings.Join(quoted, ", "))
}

// IsBQLQuery returns true if the input looks like a BQL query.
// This is used to determine whether to use BQL or simple text search.
func IsBQLQuery(input string) bool {
	// If it contains any BQL operators, treat as BQL
	bqlIndicators := []string{
		" = ", " != ", " < ", " > ", " <= ", " >= ",
		" ~ ", " !~ ",
		" and ", " AND ", " And ",
		" or ", " OR ", " Or ",
		" in ", " IN ", " In ",
		" not ", " NOT ", " Not ",
		"order by", "ORDER BY", "Order By",
		" expand ", " EXPAND ", " Expand ",
		" depth ", " DEPTH ", " Depth ",
	}

	for _, indicator := range bqlIndicators {
		if strings.Contains(input, indicator) {
			return true
		}
	}

	// Also check for expand at the start of the query (no leading space needed)
	lowerInput := strings.ToLower(strings.TrimSpace(input))
	return strings.HasPrefix(lowerInput, "expand ")
}

// loadDependencyGraph returns the cached dependency graph, loading from DB if not cached.
// Uses read-through cache pattern - invalidated automatically when cache is flushed on DB change.
// This enables O(1) SQL queries + O(V+E) in-memory traversal instead of O(D×N) iterative queries.
func (e *Executor) loadDependencyGraph(q querier) (*DependencyGraph, error) {
	cache := cachemanager.NewReadThroughCache(
		e.depGraphCache,
		func(ctx context.Context, _ struct{}) (*DependencyGraph, error) {
			return e.loadDependencyGraphFromDB(q)
		},
		false,
	)
	return cache.GetWithRefresh(context.Background(), depGraphCacheKey, struct{}{}, cachemanager.DefaultExpiration)
}

// loadDependencyGraphFromDB loads the full dependency graph from the database in a single query.
func (e *Executor) loadDependencyGraphFromDB(q querier) (*DependencyGraph, error) {
	log.Debug(log.CatBQL, "Loading dependency graph from database")

	schema, err := e.resolveSchema(q)
	if err != nil {
		return nil, err
	}
	// Includes depends_on_external when present; the JOIN keeps local targets only.
	dependencyTarget := schema.DependencyTargetExpr("d")
	//nolint:gosec // G201: dependencyTarget and softDeleteFilter return hardcoded SQL fragments.
	query := fmt.Sprintf(`
		SELECT d.issue_id, %s, d.type
		FROM dependencies d
		JOIN issues i1 ON d.issue_id = i1.id
		JOIN issues i2 ON %s = i2.id
		WHERE %s
		  AND %s
	`, dependencyTarget, dependencyTarget, e.softDeleteFilter("i1"), e.softDeleteFilter("i2"))

	rows, err := q.Query(query)
	if err != nil {
		log.ErrorErr(log.CatDB, "Failed to load dependency graph", err)
		return nil, fmt.Errorf("load dependency graph: %w", err)
	}
	defer func() { _ = rows.Close() }()

	graph := &DependencyGraph{
		Forward: make(map[string][]DependencyEdge),
		Reverse: make(map[string][]DependencyEdge),
	}

	for rows.Next() {
		var issueID, dependsOnID, depType string
		if err := rows.Scan(&issueID, &dependsOnID, &depType); err != nil {
			log.ErrorErr(log.CatDB, "Failed to scan dependency row", err)
			return nil, fmt.Errorf("scan dependency: %w", err)
		}

		// Forward: issue_id -> target issue
		graph.Forward[issueID] = append(graph.Forward[issueID], DependencyEdge{
			TargetID: dependsOnID,
			Type:     depType,
		})

		// Reverse: target issue -> issue_id
		graph.Reverse[dependsOnID] = append(graph.Reverse[dependsOnID], DependencyEdge{
			TargetID: issueID,
			Type:     depType,
		})
	}

	if err := rows.Err(); err != nil {
		log.ErrorErr(log.CatDB, "Dependency graph rows error", err)
		return nil, fmt.Errorf("dependency graph rows: %w", err)
	}

	return graph, nil
}

// traverseGraph performs BFS traversal from starting IDs, following edges based on expand type.
// Returns all reachable issue IDs (including the starting IDs).
func (e *Executor) traverseGraph(graph *DependencyGraph, startIDs []string, expandType ExpandType, depth int) []string {
	if len(startIDs) == 0 {
		return nil
	}

	// Track visited nodes to avoid duplicates and cycles
	visited := make(map[string]bool)
	for _, id := range startIDs {
		visited[id] = true
	}

	// BFS using levels to track depth
	currentLevel := startIDs
	maxDepth := depth
	if depth == int(DepthUnlimited) {
		maxDepth = maxExpandIterations
	}

	for level := 0; level < maxDepth && len(currentLevel) > 0; level++ {
		var nextLevel []string

		for _, id := range currentLevel {
			neighbors := e.getNeighbors(graph, id, expandType)
			for _, neighborID := range neighbors {
				if !visited[neighborID] {
					visited[neighborID] = true
					nextLevel = append(nextLevel, neighborID)
				}
			}
		}

		currentLevel = nextLevel
	}

	// Convert visited map to slice
	result := make([]string, 0, len(visited))
	for id := range visited {
		result = append(result, id)
	}

	return result
}

// getNeighbors returns the IDs of nodes connected to the given node based on expand type.
func (e *Executor) getNeighbors(graph *DependencyGraph, id string, expandType ExpandType) []string {
	var neighbors []string
	seen := make(map[string]bool)

	addNeighbor := func(targetID string) {
		if !seen[targetID] {
			seen[targetID] = true
			neighbors = append(neighbors, targetID)
		}
	}

	switch expandType {
	case ExpandUp:
		// Up: traverse toward dependencies (parent, blockers, discovered-from origins)
		// Forward edges: issue -> depends_on (parent, blocker, discovered-from)
		for _, edge := range graph.Forward[id] {
			addNeighbor(edge.TargetID)
		}

	case ExpandDown:
		// Down: traverse toward dependents (children, blocks, discovered issues)
		// Reverse edges: depends_on <- issue
		for _, edge := range graph.Reverse[id] {
			addNeighbor(edge.TargetID)
		}

	case ExpandAll:
		// All: both directions
		for _, edge := range graph.Forward[id] {
			addNeighbor(edge.TargetID)
		}
		for _, edge := range graph.Reverse[id] {
			addNeighbor(edge.TargetID)
		}

	default:
		// Unknown expand type - return empty
	}

	return neighbors
}

// fetchIssuesByIDs fetches issues by their IDs by delegating to executeBaseQuery.
func (e *Executor) fetchIssuesByIDs(ids []string, q querier) ([]beads.Issue, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	log.Debug(log.CatBQL, "Fetching issues by IDs", "count", len(ids))

	values := make([]Value, len(ids))
	for i, id := range ids {
		values[i] = Value{Type: ValueString, Raw: id, String: id}
	}

	query := &Query{
		Filter: &InExpr{
			Field:  "id",
			Values: values,
		},
	}
	return e.executeBaseQuery(query, q)
}
