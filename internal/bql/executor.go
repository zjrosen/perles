package bql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/zjrosen/perles/internal/beads"
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

// Executor runs BQL queries against the database.
type Executor struct {
	db            *sql.DB
	cacheManager  cachemanager.CacheManager[string, []beads.Issue]
	depGraphCache cachemanager.CacheManager[string, *DependencyGraph]
}

// depGraphCacheKey is the static key for caching the dependency graph.
const depGraphCacheKey = "__dependency_graph__"

// NewExecutor creates a new query executor.
func NewExecutor(
	db *sql.DB,
	cacheManager cachemanager.CacheManager[string, []beads.Issue],
	depGraphCache cachemanager.CacheManager[string, *DependencyGraph],
) *Executor {
	return &Executor{
		db:            db,
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
// Forward edges go from issue_id -> depends_on_id (e.g., child -> parent, blocked -> blocker)
// Reverse edges go from depends_on_id -> issue_id (e.g., parent -> children, blocker -> blocked)
type DependencyGraph struct {
	Forward map[string][]DependencyEdge // issue_id -> edges pointing to depends_on_id
	Reverse map[string][]DependencyEdge // depends_on_id -> edges pointing from issue_id
}

// Execute runs a BQL query and returns matching issues.
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

	// Execute query, using cache if available
	executeQuery := func() ([]beads.Issue, error) {
		issues, err := e.executeBaseQuery(query)
		if err != nil {
			return nil, err
		}

		// Apply expansion if specified
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
		func(ctx context.Context, q *Query) ([]beads.Issue, error) {
			return executeQuery()
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

// IssueDeps holds all dependency data for an issue, grouped by type.
type IssueDeps struct {
	ParentID       string   // Single parent (parent-child where this is child)
	BlockedBy      []string // Issues that block this one
	Blocks         []string // Issues this one blocks
	Children       []string // Child issues (parent-child where this is parent)
	DiscoveredFrom []string // Issues this was discovered from
	Discovered     []string // Issues discovered from this one
}

// executeBaseQuery runs the main BQL filter query with batch-loaded dependencies.
// This uses 3 total queries instead of 8N correlated subqueries:
// 1. Main query (no dependency subqueries)
// 2. Batch load dependencies for all result IDs
// 3. Batch load labels for all result IDs
// 4. Batch load comment counts for all result IDs
func (e *Executor) executeBaseQuery(query *Query) ([]beads.Issue, error) {
	// Build SQL
	builder := NewSQLBuilder(query)
	whereClause, orderBy, params := builder.Build()

	// Construct main query WITHOUT dependency subqueries
	sqlQuery := `
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
			i.hook_bead,
			i.role_bead,
			i.agent_state,
			i.last_activity,
			i.role_type,
			i.rig,
			i.mol_type
		FROM issues i
		WHERE i.status not in ('deleted', 'tombstone')
	  AND i.deleted_at is null
	`

	if whereClause != "" {
		sqlQuery += " AND " + whereClause
	}

	if orderBy != "" {
		sqlQuery += " ORDER BY " + orderBy
	} else {
		sqlQuery += " ORDER BY i.updated_at DESC"
	}

	// Execute main query
	rows, err := e.db.Query(sqlQuery, params...)
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
	deps, err := e.loadDependenciesForIssues(ids)
	if err != nil {
		return nil, fmt.Errorf("load dependencies: %w", err)
	}

	// Batch load labels (1 query)
	labels, err := e.loadLabelsForIssues(ids)
	if err != nil {
		return nil, fmt.Errorf("load labels: %w", err)
	}

	// Batch load comment counts (1 query)
	commentCounts, err := e.loadCommentCountsForIssues(ids)
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
			hookBead           sql.NullString
			roleBead           sql.NullString
			agentState         sql.NullString
			lastActivity       sql.NullTime
			roleType           sql.NullString
			rig                sql.NullString
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
			&hookBead,
			&roleBead,
			&agentState,
			&lastActivity,
			&roleType,
			&rig,
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
		if hookBead.Valid {
			issue.HookBead = hookBead.String
		}
		if roleBead.Valid {
			issue.RoleBead = roleBead.String
		}
		if agentState.Valid {
			issue.AgentState = agentState.String
		}
		if lastActivity.Valid {
			issue.LastActivity = lastActivity.Time
		}
		if roleType.Valid {
			issue.RoleType = roleType.String
		}
		if rig.Valid {
			issue.Rig = rig.String
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
func (e *Executor) loadDependenciesForIssues(ids []string) (map[string]IssueDeps, error) {
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
	//nolint:gosec // G201: inClause contains only "?" placeholders, not user input
	query := fmt.Sprintf(`
		SELECT d.issue_id, d.depends_on_id, d.type
		FROM dependencies d
		JOIN issues i ON d.depends_on_id = i.id
		WHERE d.issue_id IN (%s)
		  AND i.status NOT IN ('deleted', 'tombstone')
		  AND i.deleted_at IS NULL
		UNION ALL
		SELECT d.issue_id, d.depends_on_id, d.type
		FROM dependencies d
		JOIN issues i ON d.issue_id = i.id
		WHERE d.depends_on_id IN (%s)
		  AND i.status NOT IN ('deleted', 'tombstone')
		  AND i.deleted_at IS NULL
	`, inClause, inClause)

	rows, err := e.db.Query(query, params...)
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
func (e *Executor) loadLabelsForIssues(ids []string) (map[string][]string, error) {
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

	rows, err := e.db.Query(query, params...)
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
func (e *Executor) loadCommentCountsForIssues(ids []string) (map[string]int, error) {
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

	rows, err := e.db.Query(query, params...)
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
func (e *Executor) expandIssues(baseIssues []beads.Issue, expand *ExpandClause) ([]beads.Issue, error) {
	if len(baseIssues) == 0 {
		return baseIssues, nil
	}

	// Step 1: Load the full dependency graph (ONE SQL query)
	graph, err := e.loadDependencyGraph()
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
	newIssues, err := e.fetchIssuesByIDs(newIDs)
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
func (e *Executor) loadDependencyGraph() (*DependencyGraph, error) {
	cache := cachemanager.NewReadThroughCache(
		e.depGraphCache,
		func(ctx context.Context, _ struct{}) (*DependencyGraph, error) {
			return e.loadDependencyGraphFromDB()
		},
		false,
	)
	return cache.GetWithRefresh(context.Background(), depGraphCacheKey, struct{}{}, cachemanager.DefaultExpiration)
}

// loadDependencyGraphFromDB loads the full dependency graph from the database in a single query.
func (e *Executor) loadDependencyGraphFromDB() (*DependencyGraph, error) {
	log.Debug(log.CatBQL, "Loading dependency graph from database")

	query := `
		SELECT d.issue_id, d.depends_on_id, d.type
		FROM dependencies d
		JOIN issues i1 ON d.issue_id = i1.id
		JOIN issues i2 ON d.depends_on_id = i2.id
		WHERE i1.status NOT IN ('deleted', 'tombstone')
		  AND i2.status NOT IN ('deleted', 'tombstone')
		  AND i1.deleted_at IS NULL
		  AND i2.deleted_at IS NULL
	`

	rows, err := e.db.Query(query)
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

		// Forward: issue_id -> depends_on_id
		graph.Forward[issueID] = append(graph.Forward[issueID], DependencyEdge{
			TargetID: dependsOnID,
			Type:     depType,
		})

		// Reverse: depends_on_id -> issue_id
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
func (e *Executor) fetchIssuesByIDs(ids []string) ([]beads.Issue, error) {
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
	return e.executeBaseQuery(query)
}
