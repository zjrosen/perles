package bql

import (
	"database/sql"
	"fmt"
	"perles/internal/beads"
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

// Execute runs a BQL query and returns matching issues.
func (e *Executor) Execute(input string) ([]beads.Issue, error) {
	// Parse the query
	parser := NewParser(input)
	query, err := parser.Parse()
	if err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}

	// Validate the query
	if err := Validate(query); err != nil {
		return nil, fmt.Errorf("validation error: %w", err)
	}

	// Build SQL
	builder := NewSQLBuilder(query)
	whereClause, orderBy, params := builder.Build()

	// Construct full query
	sqlQuery := `
		SELECT
			i.id, i.title, i.description, i.status,
			i.priority, i.issue_type, i.created_at, i.updated_at,
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
					AND d.type IN ('blocks', 'parent-child')
					AND child.status != 'deleted'
			), '') as blocks_ids,
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
	rows, err := e.db.Query(sqlQuery, params...)
	if err != nil {
		return nil, fmt.Errorf("query error: %w", err)
	}
	defer func() { _ = rows.Close() }()

	// Scan results
	var issues []beads.Issue
	for rows.Next() {
		var issue beads.Issue
		var description sql.NullString
		var blockerIDs string
		var blocksIDs string
		var labelsStr string

		err := rows.Scan(
			&issue.ID, &issue.TitleText, &description,
			&issue.Status, &issue.Priority, &issue.Type,
			&issue.CreatedAt, &issue.UpdatedAt,
			&blockerIDs, &blocksIDs, &labelsStr,
		)
		if err != nil {
			return nil, fmt.Errorf("scan error: %w", err)
		}

		if description.Valid {
			issue.DescriptionText = description.String
		}

		// Parse blocker IDs from comma-separated string (issues that block this one)
		if blockerIDs != "" {
			issue.BlockedBy = strings.Split(blockerIDs, ",")
		}

		// Parse blocks IDs from comma-separated string (issues this one blocks)
		if blocksIDs != "" {
			issue.Blocks = strings.Split(blocksIDs, ",")
		}

		// Parse labels from comma-separated string
		if labelsStr != "" {
			issue.Labels = strings.Split(labelsStr, ",")
		}

		issues = append(issues, issue)
	}

	return issues, rows.Err()
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
	}

	for _, indicator := range bqlIndicators {
		if strings.Contains(input, indicator) {
			return true
		}
	}

	return false
}
