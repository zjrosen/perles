package beadsrust

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	appbeads "github.com/zjrosen/perles/internal/beads/application"
	"github.com/zjrosen/perles/internal/beads/bql"
	"github.com/zjrosen/perles/internal/cachemanager"
	"github.com/zjrosen/perles/internal/log"
	"github.com/zjrosen/perles/internal/task"
)

// brValidFields defines the valid BQL fields for the beads_rust backend.
// Compared to the default beads ValidFields this:
//   - Adds "owner" (beads_rust-specific field).
//   - Supports "ready" via inline subquery (no ready_issues view needed).
var brValidFields = map[string]bql.FieldType{
	"type":        bql.FieldEnum,
	"priority":    bql.FieldPriority,
	"status":      bql.FieldEnum,
	"blocked":     bql.FieldBool,
	"ready":       bql.FieldBool,
	"pinned":      bql.FieldBool,
	"is_template": bql.FieldBool,
	"label":       bql.FieldString,
	"title":       bql.FieldString,
	"id":          bql.FieldString,
	"assignee":    bql.FieldString,
	"sender":      bql.FieldString,
	"owner":       bql.FieldString,
	"description": bql.FieldString,
	"design":      bql.FieldString,
	"notes":       bql.FieldString,
	"created_by":  bql.FieldString,
	"created":     bql.FieldDate,
	"updated":     bql.FieldDate,
	"defer_until": bql.FieldDate,
}

// brIssueColumns is the SELECT column list for the beads_rust issues table.
// This matches the columns scanned by scanBRIssueFromRows.
const brIssueColumns = `
	i.id, i.title, i.description, i.design, i.acceptance_criteria, i.notes,
	i.status, i.priority, i.issue_type, i.assignee, i.owner,
	i.sender, i.ephemeral, i.pinned, i.is_template,
	i.created_at, i.created_by, i.updated_at, i.closed_at, i.close_reason,
	i.defer_until, i.source_repo, i.external_ref
`

// maxExpandIterations is the safety limit for unlimited depth expansion.
const maxExpandIterations = 100

// BQLExecutor executes BQL queries against the beads_rust SQLite database.
// It reuses the BQL parser and SQL builder from the beads BQL package,
// with beads_rust-specific column list, scan functions, and field validation.
type BQLExecutor struct {
	db            *sql.DB
	cacheManager  cachemanager.CacheManager[string, []task.Issue]
	depGraphCache cachemanager.CacheManager[string, *bql.DependencyGraph]
}

// NewBQLExecutor creates a new BQL executor for beads_rust.
func NewBQLExecutor(
	db *sql.DB,
	cacheManager cachemanager.CacheManager[string, []task.Issue],
	depGraphCache cachemanager.CacheManager[string, *bql.DependencyGraph],
) *BQLExecutor {
	return &BQLExecutor{
		db:            db,
		cacheManager:  cacheManager,
		depGraphCache: depGraphCache,
	}
}

// Execute runs a BQL query and returns matching issues as task DTOs.
func (e *BQLExecutor) Execute(input string) ([]task.Issue, error) {
	start := time.Now()

	// Parse the query.
	parser := bql.NewParser(input)
	query, err := parser.Parse()
	if err != nil {
		log.ErrorErr(log.CatBQL, "beads_rust: parse failed", err, "query", input)
		return nil, fmt.Errorf("parse error: %w", err)
	}

	// Validate against beads_rust fields.
	if err := bql.ValidateWithFields(query, brValidFields); err != nil {
		log.ErrorErr(log.CatBQL, "beads_rust: validation failed", err, "query", input)
		return nil, fmt.Errorf("validation error: %w", err)
	}

	// Execute (with read-through cache).
	executeQuery := func() ([]task.Issue, error) {
		issues, err := e.executeBaseQuery(query)
		if err != nil {
			return nil, err
		}

		if query.HasExpand() {
			issues, err = e.expandIssues(issues, query.Expand)
			if err != nil {
				return nil, err
			}
		}

		return issues, nil
	}

	cache := cachemanager.NewReadThroughCache(
		e.cacheManager,
		func(_ context.Context, _ *bql.Query) ([]task.Issue, error) {
			return executeQuery()
		},
		false,
	)
	issues, err := cache.GetWithRefresh(context.Background(), input, query, cachemanager.DefaultExpiration)
	if err != nil {
		log.ErrorErr(log.CatBQL, "beads_rust: failed to load issues", err, "query", input)
		return nil, err
	}

	log.Debug(log.CatBQL, "beads_rust: query complete",
		"duration", time.Since(start), "count", len(issues), "query", input)

	return issues, nil
}

// makeReadySQL returns a SQL fragment generator for the ready field that uses
// a pre-loaded set of blocked IDs instead of a subquery. This avoids
// triggering SQLite corruption errors on databases with malformed autoindices
// (e.g. sqlite_autoindex_issues_1), which cause any subquery or join to fail.
func makeReadySQL(blockedIDs []string) func(bool) string {
	return func(isReady bool) string {
		readyExpr := "i.status = 'open' AND (i.defer_until IS NULL OR datetime(i.defer_until) <= datetime('now'))"
		if len(blockedIDs) == 0 {
			if isReady {
				return readyExpr
			}
			return "NOT (" + readyExpr + ")"
		}
		inList := quotedIDList(blockedIDs)
		readyExpr = "(" + readyExpr + " AND i.id NOT IN (" + inList + "))"
		if isReady {
			return readyExpr
		}
		return "NOT " + readyExpr
	}
}

// makeBlockedSQL returns a SQL fragment generator for the blocked field that
// uses a pre-loaded set of blocked IDs instead of a subquery.
func makeBlockedSQL(blockedIDs []string) func(bool) string {
	return func(isBlocked bool) string {
		if len(blockedIDs) == 0 {
			if isBlocked {
				return "1=0" // nothing is blocked
			}
			return "1=1" // everything is not-blocked
		}
		inList := quotedIDList(blockedIDs)
		if isBlocked {
			return "i.id IN (" + inList + ")"
		}
		return "i.id NOT IN (" + inList + ")"
	}
}

// quotedIDList returns a comma-separated, single-quoted list of IDs for SQL IN clauses.
// IDs are constrained identifiers (alphanumeric + hyphens) so quoting is safe.
func quotedIDList(ids []string) string {
	quoted := make([]string, len(ids))
	for i, id := range ids {
		// Escape single quotes in IDs (defensive, IDs shouldn't contain them).
		escaped := strings.ReplaceAll(id, "'", "''")
		quoted[i] = "'" + escaped + "'"
	}
	return strings.Join(quoted, ",")
}

// loadBlockedIDs computes the set of blocked issue IDs from first principles,
// using the dependencies and issues tables directly. An issue is blocked when
// it has a "blocks" dependency whose blocker is still in an open state.
//
// This mirrors the Dolt inline-subquery approach but executes as two flat
// queries joined in Go, which avoids SQL subqueries/joins that can trigger
// failures on databases with corrupt autoindices (sqlite_autoindex_issues_1).
func (e *BQLExecutor) loadBlockedIDs() []string {
	// 1. Load all blocking relationships (flat scan of dependencies table).
	// NOT INDEXED: bypass corrupt sqlite_autoindex_dependencies_1.
	depRows, err := e.db.QueryContext(context.Background(),
		"SELECT issue_id, depends_on_id FROM dependencies NOT INDEXED WHERE type = 'blocks'",
	)
	if err != nil {
		log.ErrorErr(log.CatDB, "beads_rust: cannot load blocking dependencies", err)
		return nil
	}
	defer func() { _ = depRows.Close() }()

	// blockerOf maps blocker_id -> []blocked_issue_id.
	blockerOf := make(map[string][]string)
	for depRows.Next() {
		var issueID, blockerID string
		if err := depRows.Scan(&issueID, &blockerID); err != nil {
			log.ErrorErr(log.CatDB, "beads_rust: scan blocking dependency", err)
			return nil
		}
		blockerOf[blockerID] = append(blockerOf[blockerID], issueID)
	}
	if err := depRows.Err(); err != nil {
		log.ErrorErr(log.CatDB, "beads_rust: iterate blocking dependencies", err)
		return nil
	}

	if len(blockerOf) == 0 {
		return nil // no blocking deps at all
	}

	// 2. Load open issue IDs (flat table scan).
	// NOT INDEXED: bypass corrupt sqlite_autoindex_issues_1.
	issueRows, err := e.db.QueryContext(context.Background(),
		"SELECT id FROM issues NOT INDEXED WHERE status IN ('open','in_progress','blocked','deferred') AND deleted_at IS NULL",
	)
	if err != nil {
		log.ErrorErr(log.CatDB, "beads_rust: cannot load open issues for blocked check", err)
		return nil
	}
	defer func() { _ = issueRows.Close() }()

	openIDs := make(map[string]bool)
	for issueRows.Next() {
		var id string
		if err := issueRows.Scan(&id); err != nil {
			log.ErrorErr(log.CatDB, "beads_rust: scan open issue id", err)
			return nil
		}
		openIDs[id] = true
	}
	if err := issueRows.Err(); err != nil {
		log.ErrorErr(log.CatDB, "beads_rust: iterate open issues", err)
		return nil
	}

	// 3. Compute: issue is blocked if any of its blockers are still open.
	blocked := make(map[string]bool)
	for blockerID, dependents := range blockerOf {
		if openIDs[blockerID] {
			for _, depID := range dependents {
				blocked[depID] = true
			}
		}
	}

	ids := make([]string, 0, len(blocked))
	for id := range blocked {
		ids = append(ids, id)
	}
	return ids
}

// executeBaseQuery runs the main BQL filter query with batch-loaded related data.
func (e *BQLExecutor) executeBaseQuery(query *bql.Query) ([]task.Issue, error) {
	// Pre-load blocked IDs to avoid subqueries in the WHERE clause.
	// This works around SQLite corruption (malformed autoindices) that causes
	// subqueries to fail with "database disk image is malformed".
	blockedIDs := e.loadBlockedIDs()

	// Build WHERE + ORDER BY from the AST.
	// Use pre-loaded blocked IDs for ready/blocked SQL fragments.
	builder := bql.NewSQLBuilder(query, appbeads.DialectSQLite,
		bql.WithReadySQL(makeReadySQL(blockedIDs)),
		bql.WithBlockedSQL(makeBlockedSQL(blockedIDs)),
	)
	whereClause, orderBy, params := builder.Build()

	// Construct the full SELECT.
	// NOT INDEXED: bypass corrupt sqlite_autoindex_issues_1 created by frankensqlite.
	// BQL queries like "id = X" or "id in (...)" would use the broken autoindex.
	//nolint:gosec // G201: softDeleteFilter is a hardcoded constant, not user input
	sqlQuery := fmt.Sprintf("SELECT%sFROM issues i NOT INDEXED\n\t\tWHERE %s", brIssueColumns, softDeleteFilter)

	if whereClause != "" {
		sqlQuery += " AND " + whereClause //nolint:gosec // whereClause is built from validated BQL fields
	}

	if orderBy != "" {
		sqlQuery += " ORDER BY " + orderBy //nolint:gosec // orderBy is built from validated BQL field names
	} else {
		sqlQuery += " ORDER BY i.updated_at DESC"
	}

	// Execute.
	rows, err := e.db.QueryContext(context.Background(), sqlQuery, params...)
	if err != nil {
		log.ErrorErr(log.CatDB, "beads_rust: BQL query failed", err)
		return nil, fmt.Errorf("query error: %w", err)
	}
	defer func() { _ = rows.Close() }()

	// Scan base issue data.
	var issues []task.Issue
	for rows.Next() {
		issue, err := scanBRIssueFromRows(rows)
		if err != nil {
			return nil, err
		}
		issues = append(issues, *issue)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate issues: %w", err)
	}

	if len(issues) == 0 {
		return issues, nil
	}

	// Collect IDs for batch loading.
	ids := make([]string, len(issues))
	issueMap := make(map[string]*task.Issue, len(issues))
	for i := range issues {
		ids[i] = issues[i].ID
		issueMap[issues[i].ID] = &issues[i]
	}

	// Batch load related data.
	if err := e.loadLabels(ids, issueMap); err != nil {
		return nil, fmt.Errorf("load labels: %w", err)
	}
	if err := e.loadDependencies(ids, issueMap); err != nil {
		return nil, fmt.Errorf("load dependencies: %w", err)
	}
	if err := e.loadCommentCounts(ids, issueMap); err != nil {
		return nil, fmt.Errorf("load comment counts: %w", err)
	}

	return issues, nil
}

// scanBRIssueFromRows scans a single issue from *sql.Rows using the brIssueColumns order.
func scanBRIssueFromRows(rows *sql.Rows) (*task.Issue, error) {
	var issue task.Issue
	var (
		status, issueType                   string
		priority, ephemeral, pinned, isTmpl int
		createdAt, updatedAt                string
		description, design, acceptCriteria sql.NullString
		notes                               sql.NullString
		assigneeN, ownerN, senderN          sql.NullString
		createdByN, closeReasonN            sql.NullString
		sourceRepoN, externalRefN           sql.NullString
		closedAtN, deferUntilN              sql.NullString
	)

	err := rows.Scan(
		&issue.ID, &issue.TitleText, &description, &design, &acceptCriteria, &notes,
		&status, &priority, &issueType, &assigneeN, &ownerN,
		&senderN, &ephemeral, &pinned, &isTmpl,
		&createdAt, &createdByN, &updatedAt, &closedAtN, &closeReasonN,
		&deferUntilN, &sourceRepoN, &externalRefN,
	)
	if err != nil {
		return nil, fmt.Errorf("scan issue: %w", err)
	}

	issue.DescriptionText = nullStr(description)
	issue.Design = nullStr(design)
	issue.AcceptanceCriteria = nullStr(acceptCriteria)
	issue.Notes = nullStr(notes)
	issue.Status = task.Status(status)
	issue.Priority = task.Priority(priority)
	issue.Type = task.IssueType(issueType)
	issue.Assignee = nullStr(assigneeN)
	issue.CloseReason = nullStr(closeReasonN)
	issue.CreatedAt = parseTime(createdAt)
	issue.UpdatedAt = parseTime(updatedAt)
	if s := nullStr(closedAtN); s != "" {
		issue.ClosedAt = parseTime(s)
	}
	if s := nullStr(deferUntilN); s != "" {
		issue.DeferUntil = parseTime(s)
	}

	// Extensions for beads_rust-specific fields.
	ext := make(map[string]any)
	if v := nullStr(ownerN); v != "" {
		ext["owner"] = v
	}
	if v := nullStr(senderN); v != "" {
		ext["sender"] = v
	}
	if v := nullStr(createdByN); v != "" {
		ext["created_by"] = v
	}
	if v := nullStr(sourceRepoN); v != "" && v != "." {
		ext["source_repo"] = v
	}
	if v := nullStr(externalRefN); v != "" {
		ext["external_ref"] = v
	}
	if pinned != 0 {
		ext["pinned"] = true
	}
	if ephemeral != 0 {
		ext["ephemeral"] = true
	}
	if isTmpl != 0 {
		ext["is_template"] = true
	}
	if len(ext) > 0 {
		issue.Extensions = ext
	}

	return &issue, nil
}

// --- Batch loaders (return-map style, following beads BQL executor pattern) ---

// loadLabels batch-loads labels for the given issue IDs into the issueMap.
func (e *BQLExecutor) loadLabels(ids []string, issueMap map[string]*task.Issue) error {
	if len(ids) == 0 {
		return nil
	}
	placeholders, args := inClause(ids)
	// NOT INDEXED: bypass corrupt sqlite_autoindex_labels_1 created by frankensqlite.
	//nolint:gosec // G202: placeholders contains only "?" markers from inClause, not user input
	rows, err := e.db.QueryContext(context.Background(),
		"SELECT issue_id, label FROM labels NOT INDEXED WHERE issue_id IN ("+placeholders+") ORDER BY issue_id, label",
		args...,
	)
	if err != nil {
		return fmt.Errorf("batch load labels: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var issueID, label string
		if err := rows.Scan(&issueID, &label); err != nil {
			return fmt.Errorf("scan label: %w", err)
		}
		if issue, ok := issueMap[issueID]; ok {
			issue.Labels = append(issue.Labels, label)
		}
	}
	return rows.Err()
}

// loadDependencies batch-loads dependencies for the given issue IDs into the issueMap.
func (e *BQLExecutor) loadDependencies(ids []string, issueMap map[string]*task.Issue) error {
	if len(ids) == 0 {
		return nil
	}
	placeholders, args := inClause(ids)

	// Double the args for both halves of the UNION.
	allArgs := make([]any, 0, len(args)*2)
	allArgs = append(allArgs, args...)
	allArgs = append(allArgs, args...)

	// NOT INDEXED: bypass corrupt sqlite_autoindex_dependencies_1 from frankensqlite.
	//nolint:gosec // G202: placeholders contains only "?" markers from inClause, not user input
	query := `
		SELECT d.issue_id, d.depends_on_id, d.type, 'forward' AS dir
		FROM dependencies d NOT INDEXED
		WHERE d.issue_id IN (` + placeholders + `)
		UNION ALL
		SELECT d.issue_id, d.depends_on_id, d.type, 'reverse' AS dir
		FROM dependencies d NOT INDEXED
		WHERE d.depends_on_id IN (` + placeholders + `)
		ORDER BY issue_id, depends_on_id`

	rows, err := e.db.QueryContext(context.Background(), query, allArgs...)
	if err != nil {
		return fmt.Errorf("batch load dependencies: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var issueID, dependsOnID, depType, dir string
		if err := rows.Scan(&issueID, &dependsOnID, &depType, &dir); err != nil {
			return fmt.Errorf("scan dependency: %w", err)
		}

		if dir == "forward" {
			issue, ok := issueMap[issueID]
			if !ok {
				continue
			}
			switch depType {
			case "blocks", "conditional-blocks", "waits-for":
				issue.BlockedBy = append(issue.BlockedBy, dependsOnID)
			case "parent-child":
				issue.ParentID = dependsOnID
			case "discovered-from":
				issue.DiscoveredFrom = append(issue.DiscoveredFrom, dependsOnID)
			}
		} else {
			issue, ok := issueMap[dependsOnID]
			if !ok {
				continue
			}
			switch depType {
			case "blocks", "conditional-blocks", "waits-for":
				issue.Blocks = append(issue.Blocks, issueID)
			case "parent-child":
				issue.Children = append(issue.Children, issueID)
			case "discovered-from":
				issue.Discovered = append(issue.Discovered, issueID)
			}
		}
	}
	return rows.Err()
}

// loadCommentCounts batch-loads comment counts for the given issue IDs into the issueMap.
func (e *BQLExecutor) loadCommentCounts(ids []string, issueMap map[string]*task.Issue) error {
	if len(ids) == 0 {
		return nil
	}
	placeholders, args := inClause(ids)
	//nolint:gosec // G202: placeholders contains only "?" markers from inClause, not user input
	rows, err := e.db.QueryContext(context.Background(),
		"SELECT issue_id, COUNT(*) FROM comments WHERE issue_id IN ("+placeholders+") GROUP BY issue_id",
		args...,
	)
	if err != nil {
		return fmt.Errorf("batch load comment counts: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var issueID string
		var count int
		if err := rows.Scan(&issueID, &count); err != nil {
			return fmt.Errorf("scan comment count: %w", err)
		}
		if issue, ok := issueMap[issueID]; ok {
			issue.CommentCount = count
		}
	}
	return rows.Err()
}

// --- EXPAND support ---

// expandIssues traverses the dependency graph to find related issues.
func (e *BQLExecutor) expandIssues(baseIssues []task.Issue, expand *bql.ExpandClause) ([]task.Issue, error) {
	if len(baseIssues) == 0 {
		return baseIssues, nil
	}

	// Load the full dependency graph.
	graph, err := e.loadDependencyGraph()
	if err != nil {
		return nil, fmt.Errorf("load dependency graph: %w", err)
	}

	// BFS traversal.
	startIDs := make([]string, len(baseIssues))
	for i, issue := range baseIssues {
		startIDs[i] = issue.ID
	}

	allIDs := traverseGraph(graph, startIDs, expand.Type, int(expand.Depth))

	// Find IDs not already in base set.
	baseIDSet := make(map[string]bool, len(baseIssues))
	for _, issue := range baseIssues {
		baseIDSet[issue.ID] = true
	}

	var newIDs []string
	for _, id := range allIDs {
		if !baseIDSet[id] {
			newIDs = append(newIDs, id)
		}
	}

	if len(newIDs) == 0 {
		return baseIssues, nil
	}

	// Fetch new issues via BQL ID query.
	newIssues, err := e.fetchIssuesByIDs(newIDs)
	if err != nil {
		return nil, fmt.Errorf("fetch expanded issues: %w", err)
	}

	allIssues := make([]task.Issue, 0, len(baseIssues)+len(newIssues))
	allIssues = append(allIssues, baseIssues...)
	allIssues = append(allIssues, newIssues...)

	log.Debug(log.CatBQL, "beads_rust: expand complete",
		"baseCount", len(baseIssues), "expandedCount", len(newIssues), "totalCount", len(allIssues))

	return allIssues, nil
}

// loadDependencyGraph returns the cached dependency graph.
func (e *BQLExecutor) loadDependencyGraph() (*bql.DependencyGraph, error) {
	cache := cachemanager.NewReadThroughCache(
		e.depGraphCache,
		func(_ context.Context, _ struct{}) (*bql.DependencyGraph, error) {
			return e.loadDependencyGraphFromDB()
		},
		false,
	)
	return cache.GetWithRefresh(context.Background(), "__dependency_graph__", struct{}{}, cachemanager.DefaultExpiration)
}

// loadDependencyGraphFromDB loads the full dependency graph from the database.
func (e *BQLExecutor) loadDependencyGraphFromDB() (*bql.DependencyGraph, error) {
	log.Debug(log.CatBQL, "beads_rust: loading dependency graph from database")

	// NOT INDEXED on all tables: bypass corrupt autoindices from frankensqlite.
	// dependencies has sqlite_autoindex_dependencies_1, issues has sqlite_autoindex_issues_1.
	rows, err := e.db.QueryContext(context.Background(), `
		SELECT d.issue_id, d.depends_on_id, d.type
		FROM dependencies d NOT INDEXED
		JOIN issues i1 NOT INDEXED ON d.issue_id = i1.id
		JOIN issues i2 NOT INDEXED ON d.depends_on_id = i2.id
		WHERE i1.status NOT IN ('deleted', 'tombstone') AND i1.deleted_at IS NULL
		  AND i2.status NOT IN ('deleted', 'tombstone') AND i2.deleted_at IS NULL
	`)
	if err != nil {
		return nil, fmt.Errorf("load dependency graph: %w", err)
	}
	defer func() { _ = rows.Close() }()

	graph := &bql.DependencyGraph{
		Forward: make(map[string][]bql.DependencyEdge),
		Reverse: make(map[string][]bql.DependencyEdge),
	}

	for rows.Next() {
		var issueID, dependsOnID, depType string
		if err := rows.Scan(&issueID, &dependsOnID, &depType); err != nil {
			return nil, fmt.Errorf("scan dependency: %w", err)
		}

		graph.Forward[issueID] = append(graph.Forward[issueID], bql.DependencyEdge{
			TargetID: dependsOnID,
			Type:     depType,
		})
		graph.Reverse[dependsOnID] = append(graph.Reverse[dependsOnID], bql.DependencyEdge{
			TargetID: issueID,
			Type:     depType,
		})
	}

	return graph, rows.Err()
}

// fetchIssuesByIDs fetches issues by their IDs using the BQL executor.
func (e *BQLExecutor) fetchIssuesByIDs(ids []string) ([]task.Issue, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	values := make([]bql.Value, len(ids))
	for i, id := range ids {
		values[i] = bql.Value{Type: bql.ValueString, Raw: id, String: id}
	}

	query := &bql.Query{
		Filter: &bql.InExpr{
			Field:  "id",
			Values: values,
		},
	}
	return e.executeBaseQuery(query)
}

// traverseGraph performs BFS from starting IDs following edges based on expand type.
func traverseGraph(graph *bql.DependencyGraph, startIDs []string, expandType bql.ExpandType, depth int) []string {
	if len(startIDs) == 0 {
		return nil
	}

	visited := make(map[string]bool, len(startIDs))
	for _, id := range startIDs {
		visited[id] = true
	}

	currentLevel := startIDs
	maxDepth := depth
	if depth == int(bql.DepthUnlimited) {
		maxDepth = maxExpandIterations
	}

	for level := 0; level < maxDepth && len(currentLevel) > 0; level++ {
		var nextLevel []string

		for _, id := range currentLevel {
			neighbors := getNeighbors(graph, id, expandType)
			for _, neighborID := range neighbors {
				if !visited[neighborID] {
					visited[neighborID] = true
					nextLevel = append(nextLevel, neighborID)
				}
			}
		}

		currentLevel = nextLevel
	}

	result := make([]string, 0, len(visited))
	for id := range visited {
		result = append(result, id)
	}
	return result
}

// getNeighbors returns connected node IDs based on expand direction.
func getNeighbors(graph *bql.DependencyGraph, id string, expandType bql.ExpandType) []string {
	var neighbors []string
	seen := make(map[string]bool)

	addNeighbor := func(targetID string) {
		if !seen[targetID] {
			seen[targetID] = true
			neighbors = append(neighbors, targetID)
		}
	}

	switch expandType {
	case bql.ExpandUp:
		for _, edge := range graph.Forward[id] {
			addNeighbor(edge.TargetID)
		}
	case bql.ExpandDown:
		for _, edge := range graph.Reverse[id] {
			addNeighbor(edge.TargetID)
		}
	case bql.ExpandAll:
		for _, edge := range graph.Forward[id] {
			addNeighbor(edge.TargetID)
		}
		for _, edge := range graph.Reverse[id] {
			addNeighbor(edge.TargetID)
		}
	}

	return neighbors
}
