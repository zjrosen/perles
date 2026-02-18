package bql

import (
	"fmt"
	"strings"

	appbeads "github.com/zjrosen/perles/internal/beads/application"
)

// SQLBuilder converts a BQL AST to SQL.
type SQLBuilder struct {
	query   *Query
	params  []any
	dialect appbeads.SQLDialect
}

// NewSQLBuilder creates a builder for the query and dialect.
func NewSQLBuilder(query *Query, dialect appbeads.SQLDialect) *SQLBuilder {
	return &SQLBuilder{query: query, dialect: dialect}
}

// Build generates the SQL WHERE clause and ORDER BY.
func (b *SQLBuilder) Build() (whereClause string, orderBy string, params []any) {
	if b.query.Filter != nil {
		whereClause = b.buildExpr(b.query.Filter)
	}

	if len(b.query.OrderBy) > 0 {
		orderBy = b.buildOrderBy()
	}

	return whereClause, orderBy, b.params
}

// buildExpr recursively builds SQL for an expression.
func (b *SQLBuilder) buildExpr(expr Expr) string {
	switch e := expr.(type) {
	case *BinaryExpr:
		left := b.buildExpr(e.Left)
		right := b.buildExpr(e.Right)
		op := "AND"
		if e.Op == TokenOr {
			op = "OR"
		}
		return fmt.Sprintf("(%s %s %s)", left, op, right)

	case *NotExpr:
		return fmt.Sprintf("NOT (%s)", b.buildExpr(e.Expr))

	case *CompareExpr:
		return b.buildCompare(e)

	case *InExpr:
		return b.buildIn(e)
	}

	return ""
}

// buildCompare builds SQL for a comparison expression.
func (b *SQLBuilder) buildCompare(e *CompareExpr) string {
	// Handle special fields
	switch e.Field {
	case "blocked":
		return b.buildBlockedSQL(e.Value.Bool)

	case "ready":
		return b.buildReadySQL(e.Value.Bool)

	case "pinned", "is_template":
		// Nullable boolean columns (INTEGER in SQLite)
		// field = true -> column = 1
		// field = false -> column = 0 (won't match NULL)
		column := b.fieldToColumn(e.Field)
		if e.Value.Bool {
			return fmt.Sprintf("%s = 1", column)
		}
		return fmt.Sprintf("%s = 0", column)

	case "label":
		// Label check via labels table
		// Supports exact match (=, !=) and partial match (~, !~)
		switch e.Op {
		case TokenContains:
			b.params = append(b.params, "%"+e.Value.String+"%")
			return "i.id IN (SELECT issue_id FROM labels WHERE label LIKE ?)"
		case TokenNotContains:
			b.params = append(b.params, "%"+e.Value.String+"%")
			return "i.id NOT IN (SELECT issue_id FROM labels WHERE label LIKE ?)"
		case TokenNeq:
			b.params = append(b.params, e.Value.String)
			return "i.id NOT IN (SELECT issue_id FROM labels WHERE label = ?)"
		default: // TokenEq
			b.params = append(b.params, e.Value.String)
			return "i.id IN (SELECT issue_id FROM labels WHERE label = ?)"
		}
	}

	// Map BQL fields to SQL columns
	column := b.fieldToColumn(e.Field)

	// Handle priority comparisons
	if e.Field == "priority" {
		b.params = append(b.params, e.Value.Int)
		return fmt.Sprintf("%s %s ?", column, b.opToSQL(e.Op))
	}

	// Handle date comparisons
	if e.Value.Type == ValueDate {
		dateSQL := b.dateToSQL(e.Value.String)
		if b.dialect == appbeads.DialectMySQL {
			// MySQL/Dolt handles datetime types natively, no wrapping needed
			return fmt.Sprintf("%s %s %s", column, b.opToSQL(e.Op), dateSQL)
		}
		// SQLite: wrap column in datetime() to normalize ISO 8601 timestamps
		// with timezone to UTC format that matches datetime('now', ...) expressions
		return fmt.Sprintf("datetime(%s) %s %s", column, b.opToSQL(e.Op), dateSQL)
	}

	// Handle nullable string fields (use COALESCE so NULL matches empty string)
	if e.Field == "assignee" {
		switch e.Op {
		case TokenContains:
			b.params = append(b.params, "%"+e.Value.String+"%")
			return fmt.Sprintf("COALESCE(%s, '') LIKE ?", column)
		case TokenNotContains:
			b.params = append(b.params, "%"+e.Value.String+"%")
			return fmt.Sprintf("COALESCE(%s, '') NOT LIKE ?", column)
		default:
			b.params = append(b.params, e.Value.String)
			return fmt.Sprintf("COALESCE(%s, '') %s ?", column, b.opToSQL(e.Op))
		}
	}

	// Handle contains/not contains operators
	switch e.Op {
	case TokenContains:
		b.params = append(b.params, "%"+e.Value.String+"%")
		return fmt.Sprintf("%s LIKE ?", column)
	case TokenNotContains:
		b.params = append(b.params, "%"+e.Value.String+"%")
		return fmt.Sprintf("%s NOT LIKE ?", column)
	}

	// Standard comparison
	b.params = append(b.params, e.Value.String)
	return fmt.Sprintf("%s %s ?", column, b.opToSQL(e.Op))
}

// buildIn builds SQL for an IN expression.
func (b *SQLBuilder) buildIn(e *InExpr) string {
	// Handle label field specially
	if e.Field == "label" {
		placeholders := make([]string, len(e.Values))
		for i, v := range e.Values {
			placeholders[i] = "?"
			b.params = append(b.params, v.String)
		}
		subquery := fmt.Sprintf("i.id IN (SELECT issue_id FROM labels WHERE label IN (%s))",
			strings.Join(placeholders, ", "))
		if e.Not {
			return "NOT " + subquery
		}
		return subquery
	}

	column := b.fieldToColumn(e.Field)
	placeholders := make([]string, len(e.Values))

	for i, v := range e.Values {
		placeholders[i] = "?"
		if e.Field == "priority" {
			b.params = append(b.params, v.Int)
		} else {
			b.params = append(b.params, v.String)
		}
	}

	op := "IN"
	if e.Not {
		op = "NOT IN"
	}

	return fmt.Sprintf("%s %s (%s)", column, op, strings.Join(placeholders, ", "))
}

// fieldToColumn maps BQL field names to SQL column names.
func (b *SQLBuilder) fieldToColumn(field string) string {
	// Only map fields where BQL name differs from column name
	mapping := map[string]string{
		"type":          "i.issue_type",
		"created":       "i.created_at",
		"updated":       "i.updated_at",
		"last_activity": "i.last_activity",
	}
	if col, ok := mapping[field]; ok {
		return col
	}
	return "i." + field
}

// opToSQL converts a token operator to SQL.
func (b *SQLBuilder) opToSQL(op TokenType) string {
	switch op {
	case TokenEq:
		return "="
	case TokenNeq:
		return "!="
	case TokenLt:
		return "<"
	case TokenGt:
		return ">"
	case TokenLte:
		return "<="
	case TokenGte:
		return ">="
	default:
		return "="
	}
}

// dateToSQL converts a date value to a SQL expression based on the dialect.
func (b *SQLBuilder) dateToSQL(dateStr string) string {
	if b.dialect == appbeads.DialectMySQL {
		return b.dateToSQLMySQL(dateStr)
	}
	return b.dateToSQLSQLite(dateStr)
}

// dateToSQLSQLite generates SQLite date expressions.
func (b *SQLBuilder) dateToSQLSQLite(dateStr string) string {
	switch dateStr {
	case "today":
		return "date('now')"
	case "yesterday":
		return "date('now', '-1 day')"
	default:
		// Handle relative time formats: -Nd (days), -Nh (hours), -Nm (months)
		if len(dateStr) > 1 && dateStr[0] == '-' {
			suffix := dateStr[len(dateStr)-1]
			value := dateStr[1 : len(dateStr)-1] // strip - and suffix

			switch suffix {
			case 'd', 'D':
				return fmt.Sprintf("date('now', '-%s days')", value)
			case 'h', 'H':
				// Hours use datetime() for sub-day precision
				return fmt.Sprintf("datetime('now', '-%s hours')", value)
			case 'm', 'M':
				return fmt.Sprintf("date('now', '-%s months')", value)
			}
		}
		// Assume ISO date, pass through as string
		b.params = append(b.params, dateStr)
		return "?"
	}
}

// dateToSQLMySQL generates MySQL/Dolt date expressions.
func (b *SQLBuilder) dateToSQLMySQL(dateStr string) string {
	switch dateStr {
	case "today":
		return "CURDATE()"
	case "yesterday":
		return "DATE_SUB(CURDATE(), INTERVAL 1 DAY)"
	default:
		// Handle relative time formats: -Nd (days), -Nh (hours), -Nm (months)
		if len(dateStr) > 1 && dateStr[0] == '-' {
			suffix := dateStr[len(dateStr)-1]
			value := dateStr[1 : len(dateStr)-1] // strip - and suffix

			switch suffix {
			case 'd', 'D':
				return fmt.Sprintf("DATE_SUB(CURDATE(), INTERVAL %s DAY)", value)
			case 'h', 'H':
				return fmt.Sprintf("DATE_SUB(NOW(), INTERVAL %s HOUR)", value)
			case 'm', 'M':
				return fmt.Sprintf("DATE_SUB(CURDATE(), INTERVAL %s MONTH)", value)
			}
		}
		// Assume ISO date, pass through as string
		b.params = append(b.params, dateStr)
		return "?"
	}
}

// doltBlockedSubquery is the inlined SQL for finding blocked issues in Dolt.
// This bypasses the blocked_issues view to work around a Dolt server bug where
// views return stale field index errors after client reconnection.
const doltBlockedSubquery = `SELECT bi.id FROM issues bi
WHERE bi.status IN ('open', 'in_progress', 'blocked', 'deferred', 'hooked')
AND EXISTS (
  SELECT 1 FROM dependencies d
  WHERE d.issue_id = bi.id AND d.type = 'blocks'
  AND EXISTS (
    SELECT 1 FROM issues blocker
    WHERE blocker.id = d.depends_on_id
    AND blocker.status IN ('open', 'in_progress', 'blocked', 'deferred', 'hooked')
  )
)`

// buildBlockedSQL returns the SQL fragment for the blocked field.
// SQLite uses the blocked_issues_cache table; Dolt inlines the view SQL directly.
func (b *SQLBuilder) buildBlockedSQL(isBlocked bool) string {
	if b.dialect == appbeads.DialectMySQL {
		if isBlocked {
			return "i.id IN (" + doltBlockedSubquery + ")"
		}
		return "i.id NOT IN (" + doltBlockedSubquery + ")"
	}
	// SQLite: blocked_issues_cache is a table with issue_id column
	if isBlocked {
		return "i.id IN (SELECT issue_id FROM blocked_issues_cache)"
	}
	return "i.id NOT IN (SELECT issue_id FROM blocked_issues_cache)"
}

// buildReadySQL returns the SQL fragment for the ready field.
// SQLite uses the ready_issues view; Dolt inlines the SQL to bypass views.
func (b *SQLBuilder) buildReadySQL(isReady bool) string {
	if b.dialect == appbeads.DialectMySQL {
		readySubquery := `SELECT ri.id FROM issues ri
WHERE ri.status = 'open'
AND ri.id NOT IN (` + doltBlockedSubquery + `)`
		if isReady {
			return "i.id IN (" + readySubquery + ")"
		}
		return "i.id NOT IN (" + readySubquery + ")"
	}
	// SQLite: ready_issues view works fine
	if isReady {
		return "i.id IN (SELECT id FROM ready_issues)"
	}
	return "i.id NOT IN (SELECT id FROM ready_issues)"
}

// buildOrderBy builds the ORDER BY clause.
func (b *SQLBuilder) buildOrderBy() string {
	var parts []string
	for _, term := range b.query.OrderBy {
		col := b.fieldToColumn(term.Field)
		dir := "ASC"
		if term.Desc {
			dir = "DESC"
		}
		parts = append(parts, fmt.Sprintf("%s %s", col, dir))
	}
	return strings.Join(parts, ", ")
}
