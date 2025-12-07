package bql

import (
	"database/sql"
	"fmt"
	"perles/internal/beads"
	"perles/internal/log"
	"strings"
)

// Executor runs BQL queries against the database.
type Executor struct {
	db *sql.DB
}

// NewExecutor creates a new query executor.
func NewExecutor(db *sql.DB) *Executor {
	return &Executor{db: db}
}

// maxExpandIterations is the safety limit for unlimited depth expansion.
const maxExpandIterations = 100

// Execute runs a BQL query and returns matching issues.
func (e *Executor) Execute(input string) ([]beads.Issue, error) {
	log.Debug(log.CatBQL, "Parsing query", "query", input)

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

	// Execute base query
	issues, err := e.executeBaseQuery(query)
	if err != nil {
		return nil, err
	}

	// Apply expansion if specified
	if query.HasExpand() {
		log.Debug(log.CatBQL, "Expanding results", "type", query.Expand.Type, "depth", query.Expand.Depth, "baseCount", len(issues))
		issues, err = e.expandIssues(issues, query.Expand)
		if err != nil {
			return nil, err
		}
	}

	log.Debug(log.CatBQL, "Query complete", "results", len(issues))
	return issues, nil
}

// executeBaseQuery runs the main BQL filter query.
func (e *Executor) executeBaseQuery(query *Query) ([]beads.Issue, error) {
	// Build SQL
	builder := NewSQLBuilder(query)
	whereClause, orderBy, params := builder.Build()

	// Construct full query
	// Added related_ids subquery to fetch all outgoing links (upstream dependencies)
	sqlQuery := `
		SELECT
			i.id, i.title, i.description, i.status,
			i.priority, i.issue_type, i.assignee, i.created_at, i.updated_at,
			COALESCE((
				SELECT d.depends_on_id
				FROM dependencies d
				WHERE d.issue_id = i.id AND d.type = 'parent-child'
				LIMIT 1
			), '') as parent_id,
			COALESCE((
				SELECT GROUP_CONCAT(d.depends_on_id)
				FROM dependencies d
				JOIN issues blocker ON d.depends_on_id = blocker.id
				WHERE d.issue_id = i.id
					AND d.type = 'blocks'
					AND blocker.status IN ('open', 'in_progress', 'blocked')
			), '') as blocker_ids,
			COALESCE((
				SELECT GROUP_CONCAT(d.issue_id)
				FROM dependencies d
				JOIN issues child ON d.issue_id = child.id
				WHERE d.depends_on_id = i.id
					AND d.type = 'blocks'
					AND child.status != 'deleted'
			), '') as blocks_ids,
			COALESCE((
				SELECT GROUP_CONCAT(d.issue_id)
				FROM dependencies d
				JOIN issues child ON d.issue_id = child.id
				WHERE d.depends_on_id = i.id
					AND d.type = 'parent-child'
					AND child.status != 'deleted'
			), '') as children_ids,
			COALESCE((
				SELECT GROUP_CONCAT(d.depends_on_id)
				FROM dependencies d
				JOIN issues up ON d.depends_on_id = up.id
				WHERE d.issue_id = i.id
					AND d.type NOT IN ('blocks', 'parent-child')
					AND up.status != 'deleted'
			), '') as related_ids,
			COALESCE((
				SELECT GROUP_CONCAT(l.label)
				FROM labels l
				WHERE l.issue_id = i.id
			), '') as labels
		FROM issues i
		WHERE i.status != 'deleted'
	`

	if whereClause != "" {
		sqlQuery += " AND " + whereClause
	}

	if orderBy != "" {
		sqlQuery += " ORDER BY " + orderBy
	} else {
		sqlQuery += " ORDER BY i.updated_at DESC"
	}

	// Execute query
	log.Debug(log.CatBQL, "Executing SQL", "argCount", len(params))
	rows, err := e.db.Query(sqlQuery, params...)
	if err != nil {
		log.ErrorErr(log.CatDB, "Query failed", err)
		return nil, fmt.Errorf("query error: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return e.scanIssues(rows)
}

// scanIssues reads issues from database rows.
func (e *Executor) scanIssues(rows *sql.Rows) ([]beads.Issue, error) {
	var issues []beads.Issue
	for rows.Next() {
		var issue beads.Issue
		var description sql.NullString
		var assignee sql.NullString
		var parentID sql.NullString
		var blockerIDs string
		var blocksIDs string
		var childrenIDs string
		var relatedIDs string
		var labelsStr string

		err := rows.Scan(
			&issue.ID, &issue.TitleText, &description,
			&issue.Status, &issue.Priority, &issue.Type,
			&assignee, &issue.CreatedAt, &issue.UpdatedAt,
			&parentID,
			&blockerIDs, &blocksIDs, &childrenIDs, &relatedIDs, &labelsStr,
		)
		if err != nil {
			log.ErrorErr(log.CatDB, "Scan failed", err)
			return nil, fmt.Errorf("scan error: %w", err)
		}

		if description.Valid {
			issue.DescriptionText = description.String
		}
		if assignee.Valid {
			issue.Assignee = assignee.String
		}
		if parentID.Valid {
			issue.ParentID = parentID.String
		}

		// Parse blocker IDs from comma-separated string (issues that block this one)
		if blockerIDs != "" {
			issue.BlockedBy = strings.Split(blockerIDs, ",")
		}

		// Parse blocks IDs from comma-separated string (issues this one blocks)
		if blocksIDs != "" {
			issue.Blocks = strings.Split(blocksIDs, ",")
		}

		// Parse children IDs
		if childrenIDs != "" {
			issue.Children = strings.Split(childrenIDs, ",")
		}

		// Parse related IDs (all upstream dependencies)
		if relatedIDs != "" {
			issue.Related = strings.Split(relatedIDs, ",")
		}

		// Parse labels from comma-separated string
		if labelsStr != "" {
			issue.Labels = strings.Split(labelsStr, ",")
		}

		issues = append(issues, issue)
	}

	return issues, rows.Err()
}

// expandIssues iteratively fetches related issues based on expansion configuration.
func (e *Executor) expandIssues(baseIssues []beads.Issue, expand *ExpandClause) ([]beads.Issue, error) {
	if len(baseIssues) == 0 {
		return baseIssues, nil
	}

	// Initialize tracking
	seenIDs := make(map[string]bool)
	for _, issue := range baseIssues {
		seenIDs[issue.ID] = true
	}

	// Start with base issues as the current frontier
	currentIDs := make([]string, len(baseIssues))
	for i, issue := range baseIssues {
		currentIDs[i] = issue.ID
	}

	allIssues := baseIssues

	// Determine max iterations
	maxIterations := int(expand.Depth)
	if expand.Depth == DepthUnlimited {
		maxIterations = maxExpandIterations
	}

	// Iterate up to depth
	for iteration := 0; iteration < maxIterations; iteration++ {
		// Query for related IDs
		relatedIDs, err := e.queryRelatedIDs(currentIDs, expand.Type)
		if err != nil {
			log.ErrorErr(log.CatBQL, "Expand query failed", err, "iteration", iteration)
			return nil, fmt.Errorf("expand query error: %w", err)
		}

		// Filter to genuinely new IDs
		var newIDs []string
		for _, id := range relatedIDs {
			if !seenIDs[id] {
				newIDs = append(newIDs, id)
				seenIDs[id] = true
			}
		}

		// If no new IDs found, we've reached the end of the graph
		if len(newIDs) == 0 {
			break
		}

		// Fetch the new issues
		newIssues, err := e.fetchIssuesByIDs(newIDs)
		if err != nil {
			log.ErrorErr(log.CatBQL, "Fetch expanded issues failed", err, "ids", len(newIDs))
			return nil, fmt.Errorf("fetch expanded issues error: %w", err)
		}

		// Add to results
		allIssues = append(allIssues, newIssues...)

		// Update frontier for next iteration
		currentIDs = newIDs
	}

	return allIssues, nil
}

// queryRelatedIDs queries the dependencies table for related issue IDs.
func (e *Executor) queryRelatedIDs(issueIDs []string, expandType ExpandType) ([]string, error) {
	if len(issueIDs) == 0 {
		return nil, nil
	}

	// Build placeholder string for IN clause
	placeholders := make([]string, len(issueIDs))
	params := make([]any, len(issueIDs))
	for i, id := range issueIDs {
		placeholders[i] = "?"
		params[i] = id
	}
	inClause := strings.Join(placeholders, ",")

	var queries []string

	switch expandType {
	case ExpandChildren:
		// Children: issues where depends_on_id is in our set (parent-child type)
		queries = append(queries, fmt.Sprintf(`
			SELECT d.issue_id FROM dependencies d
			JOIN issues i ON d.issue_id = i.id
			WHERE d.depends_on_id IN (%s)
				AND d.type = 'parent-child'
				AND i.status != 'deleted'
		`, inClause))

	case ExpandBlockers:
		// Blockers: issues that block the current set (depends_on_id in blocks type)
		queries = append(queries, fmt.Sprintf(`
			SELECT d.depends_on_id FROM dependencies d
			JOIN issues i ON d.depends_on_id = i.id
			WHERE d.issue_id IN (%s)
				AND d.type = 'blocks'
				AND i.status != 'deleted'
		`, inClause))

	case ExpandBlocks:
		// Blocks: issues blocked by the current set (issue_id in blocks type)
		queries = append(queries, fmt.Sprintf(`
			SELECT d.issue_id FROM dependencies d
			JOIN issues i ON d.issue_id = i.id
			WHERE d.depends_on_id IN (%s)
				AND d.type = 'blocks'
				AND i.status != 'deleted'
		`, inClause))

	case ExpandDeps:
		// Both directions for blocks type
		queries = append(queries, fmt.Sprintf(`
			SELECT d.depends_on_id FROM dependencies d
			JOIN issues i ON d.depends_on_id = i.id
			WHERE d.issue_id IN (%s)
				AND d.type = 'blocks'
				AND i.status != 'deleted'
		`, inClause))
		queries = append(queries, fmt.Sprintf(`
			SELECT d.issue_id FROM dependencies d
			JOIN issues i ON d.issue_id = i.id
			WHERE d.depends_on_id IN (%s)
				AND d.type = 'blocks'
				AND i.status != 'deleted'
		`, inClause))

	case ExpandAll:
		// All relationships in both directions
		queries = append(queries, fmt.Sprintf(`
			SELECT d.issue_id FROM dependencies d
			JOIN issues i ON d.issue_id = i.id
			WHERE d.depends_on_id IN (%s)
				AND i.status != 'deleted'
		`, inClause))
		queries = append(queries, fmt.Sprintf(`
			SELECT d.depends_on_id FROM dependencies d
			JOIN issues i ON d.depends_on_id = i.id
			WHERE d.issue_id IN (%s)
				AND i.status != 'deleted'
		`, inClause))

	case ExpandUpstream:
		// Upstream: all outgoing links (issue_id -> depends_on_id)
		queries = append(queries, fmt.Sprintf(`
			SELECT d.depends_on_id FROM dependencies d
			JOIN issues i ON d.depends_on_id = i.id
			WHERE d.issue_id IN (%s)
				AND i.status != 'deleted'
		`, inClause))

	case ExpandDownstream:
		// Downstream: all incoming links (depends_on_id <- issue_id)
		queries = append(queries, fmt.Sprintf(`
			SELECT d.issue_id FROM dependencies d
			JOIN issues i ON d.issue_id = i.id
			WHERE d.depends_on_id IN (%s)
				AND i.status != 'deleted'
		`, inClause))

	default:
		return nil, nil
	}

	// Execute all queries and collect unique IDs
	seenIDs := make(map[string]bool)
	var relatedIDs []string

	for _, q := range queries {
		rows, err := e.db.Query(q, params...)
		if err != nil {
			log.ErrorErr(log.CatDB, "Related IDs query failed", err)
			return nil, err
		}

		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				log.ErrorErr(log.CatDB, "Related ID scan failed", err)
				_ = rows.Close()
				return nil, err
			}
			if !seenIDs[id] {
				seenIDs[id] = true
				relatedIDs = append(relatedIDs, id)
			}
		}
		_ = rows.Close()

		if err := rows.Err(); err != nil {
			log.ErrorErr(log.CatDB, "Related IDs rows error", err)
			return nil, err
		}
	}

	return relatedIDs, nil
}

// fetchIssuesByIDs fetches full issue details for a list of IDs.
func (e *Executor) fetchIssuesByIDs(ids []string) ([]beads.Issue, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	// Build placeholder string for IN clause
	placeholders := make([]string, len(ids))
	params := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		params[i] = id
	}
	inClause := strings.Join(placeholders, ",")

	//nolint:gosec // G201 - inClause contains only safe ? placeholders, not user input
	sqlQuery := fmt.Sprintf(`
		SELECT
			i.id, i.title, i.description, i.status,
			i.priority, i.issue_type, i.assignee, i.created_at, i.updated_at,
			COALESCE((
				SELECT d.depends_on_id
				FROM dependencies d
				WHERE d.issue_id = i.id AND d.type = 'parent-child'
				LIMIT 1
			), '') as parent_id,
			COALESCE((
				SELECT GROUP_CONCAT(d.depends_on_id)
				FROM dependencies d
				JOIN issues blocker ON d.depends_on_id = blocker.id
				WHERE d.issue_id = i.id
					AND d.type = 'blocks'
					AND blocker.status IN ('open', 'in_progress', 'blocked')
			), '') as blocker_ids,
			COALESCE((
				SELECT GROUP_CONCAT(d.issue_id)
				FROM dependencies d
				JOIN issues child ON d.issue_id = child.id
				WHERE d.depends_on_id = i.id
					AND d.type = 'blocks'
					AND child.status != 'deleted'
			), '') as blocks_ids,
			COALESCE((
				SELECT GROUP_CONCAT(d.issue_id)
				FROM dependencies d
				JOIN issues child ON d.issue_id = child.id
				WHERE d.depends_on_id = i.id
					AND d.type = 'parent-child'
					AND child.status != 'deleted'
			), '') as children_ids,
			COALESCE((
				SELECT GROUP_CONCAT(d.depends_on_id)
				FROM dependencies d
				JOIN issues up ON d.depends_on_id = up.id
				WHERE d.issue_id = i.id
					AND d.type NOT IN ('blocks', 'parent-child')
					AND up.status != 'deleted'
			), '') as related_ids,
			COALESCE((
				SELECT GROUP_CONCAT(l.label)
				FROM labels l
				WHERE l.issue_id = i.id
			), '') as labels
		FROM issues i
		WHERE i.id IN (%s)
			AND i.status != 'deleted'
	`, inClause)

	rows, err := e.db.Query(sqlQuery, params...)
	if err != nil {
		log.ErrorErr(log.CatDB, "Fetch by IDs failed", err, "idCount", len(ids))
		return nil, fmt.Errorf("fetch by IDs error: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return e.scanIssues(rows)
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
