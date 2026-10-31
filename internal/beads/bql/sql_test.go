package bql

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	appbeads "github.com/zjrosen/perles/internal/beads/application"
)

// readyExcludedTypesSQL is bd's ready-work type exclusion as a SQL list.
const readyExcludedTypesSQL = "'merge-request', 'gate', 'molecule', 'rig', 'agent', 'role', 'message'"

// sqliteReadyExpr is the ready predicate for the default (unprobed) SQLite
// schema: blocked_issues_cache and the legacy depends_on_id target.
const sqliteReadyExpr = "i.status = 'open'" +
	" AND (i.pinned = 0 OR i.pinned IS NULL)" +
	" AND (i.ephemeral = 0 OR i.ephemeral IS NULL)" +
	" AND i.issue_type NOT IN (" + readyExcludedTypesSQL + ")" +
	" AND (i.defer_until IS NULL OR datetime(i.defer_until) <= datetime('now'))" +
	" AND i.id NOT IN (SELECT issue_id FROM blocked_issues_cache)" +
	" AND NOT EXISTS (\n  SELECT 1 FROM dependencies dp\n  JOIN issues parent ON parent.id = dp.depends_on_id\n" +
	"  WHERE dp.issue_id = i.id AND dp.type = 'parent-child'\n" +
	"  AND parent.defer_until IS NOT NULL AND datetime(parent.defer_until) > datetime('now')\n)"

func TestSQLBuilder_SimpleComparison(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantWhere   string
		wantParams  []interface{}
		wantOrderBy string
	}{
		{
			name:       "equals string",
			input:      "type = task",
			wantWhere:  "i.issue_type = ?",
			wantParams: []interface{}{"task"},
		},
		{
			name:       "equals priority",
			input:      "priority = P0",
			wantWhere:  "i.priority = ?",
			wantParams: []interface{}{0},
		},
		{
			name:       "not equals",
			input:      "status != closed",
			wantWhere:  "i.status != ?",
			wantParams: []interface{}{"closed"},
		},
		{
			name:       "less than priority",
			input:      "priority < P2",
			wantWhere:  "i.priority < ?",
			wantParams: []interface{}{2},
		},
		{
			name:       "greater than priority",
			input:      "priority > P1",
			wantWhere:  "i.priority > ?",
			wantParams: []interface{}{1},
		},
		{
			name:       "contains",
			input:      "title ~ auth",
			wantWhere:  "i.title LIKE ?",
			wantParams: []interface{}{"%auth%"},
		},
		{
			name:       "not contains",
			input:      "title !~ test",
			wantWhere:  "i.title NOT LIKE ?",
			wantParams: []interface{}{"%test%"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewParser(tt.input)
			query, err := parser.Parse()
			require.NoError(t, err)

			builder := NewSQLBuilder(query, appbeads.DialectSQLite)
			where, orderBy, params := builder.Build()

			require.Equal(t, tt.wantWhere, where)
			require.Equal(t, tt.wantParams, params)
			require.Equal(t, tt.wantOrderBy, orderBy)
		})
	}
}

func TestSQLBuilder_SpecialFields(t *testing.T) {
	t.Run("blocked true", func(t *testing.T) {
		parser := NewParser("blocked = true")
		query, err := parser.Parse()
		require.NoError(t, err)

		builder := NewSQLBuilder(query, appbeads.DialectSQLite)
		where, _, params := builder.Build()

		require.Equal(t, "i.id IN (SELECT issue_id FROM blocked_issues_cache)", where)
		require.Empty(t, params)
	})

	t.Run("blocked false", func(t *testing.T) {
		parser := NewParser("blocked = false")
		query, err := parser.Parse()
		require.NoError(t, err)

		builder := NewSQLBuilder(query, appbeads.DialectSQLite)
		where, _, _ := builder.Build()

		require.Equal(t, "i.id NOT IN (SELECT issue_id FROM blocked_issues_cache)", where)
	})

	t.Run("ready true", func(t *testing.T) {
		parser := NewParser("ready = true")
		query, err := parser.Parse()
		require.NoError(t, err)

		builder := NewSQLBuilder(query, appbeads.DialectSQLite)
		where, _, _ := builder.Build()

		require.Equal(t, "("+sqliteReadyExpr+")", where)
	})

	t.Run("ready false", func(t *testing.T) {
		parser := NewParser("ready = false")
		query, err := parser.Parse()
		require.NoError(t, err)

		builder := NewSQLBuilder(query, appbeads.DialectSQLite)
		where, _, _ := builder.Build()

		require.Equal(t, "NOT ("+sqliteReadyExpr+")", where)
	})

	t.Run("pinned true", func(t *testing.T) {
		parser := NewParser("pinned = true")
		query, err := parser.Parse()
		require.NoError(t, err)

		builder := NewSQLBuilder(query, appbeads.DialectSQLite)
		where, _, params := builder.Build()

		require.Equal(t, "i.pinned = 1", where)
		require.Empty(t, params)
	})

	t.Run("pinned false", func(t *testing.T) {
		parser := NewParser("pinned = false")
		query, err := parser.Parse()
		require.NoError(t, err)

		builder := NewSQLBuilder(query, appbeads.DialectSQLite)
		where, _, params := builder.Build()

		require.Equal(t, "i.pinned = 0", where)
		require.Empty(t, params)
	})

	t.Run("is_template true", func(t *testing.T) {
		parser := NewParser("is_template = true")
		query, err := parser.Parse()
		require.NoError(t, err)

		builder := NewSQLBuilder(query, appbeads.DialectSQLite)
		where, _, params := builder.Build()

		require.Equal(t, "i.is_template = 1", where)
		require.Empty(t, params)
	})

	t.Run("is_template false", func(t *testing.T) {
		parser := NewParser("is_template = false")
		query, err := parser.Parse()
		require.NoError(t, err)

		builder := NewSQLBuilder(query, appbeads.DialectSQLite)
		where, _, params := builder.Build()

		require.Equal(t, "i.is_template = 0", where)
		require.Empty(t, params)
	})

	t.Run("assignee equals", func(t *testing.T) {
		parser := NewParser("assignee = alice")
		query, err := parser.Parse()
		require.NoError(t, err)

		builder := NewSQLBuilder(query, appbeads.DialectSQLite)
		where, _, params := builder.Build()

		require.Equal(t, "COALESCE(i.assignee, '') = ?", where)
		require.Equal(t, []interface{}{"alice"}, params)
	})

	t.Run("assignee empty", func(t *testing.T) {
		parser := NewParser(`assignee = ""`)
		query, err := parser.Parse()
		require.NoError(t, err)

		builder := NewSQLBuilder(query, appbeads.DialectSQLite)
		where, _, params := builder.Build()

		require.Equal(t, "COALESCE(i.assignee, '') = ?", where)
		require.Equal(t, []interface{}{""}, params)
	})

	t.Run("assignee contains", func(t *testing.T) {
		parser := NewParser("assignee ~ bob")
		query, err := parser.Parse()
		require.NoError(t, err)

		builder := NewSQLBuilder(query, appbeads.DialectSQLite)
		where, _, params := builder.Build()

		require.Equal(t, "COALESCE(i.assignee, '') LIKE ?", where)
		require.Equal(t, []interface{}{"%bob%"}, params)
	})

	t.Run("single label", func(t *testing.T) {
		parser := NewParser("label = urgent")
		query, err := parser.Parse()
		require.NoError(t, err)

		builder := NewSQLBuilder(query, appbeads.DialectSQLite)
		where, _, params := builder.Build()

		require.Equal(t, "i.id IN (SELECT issue_id FROM labels WHERE label = ?)", where)
		require.Equal(t, []interface{}{"urgent"}, params)
	})

	t.Run("label not equals", func(t *testing.T) {
		parser := NewParser("label != urgent")
		query, err := parser.Parse()
		require.NoError(t, err)

		builder := NewSQLBuilder(query, appbeads.DialectSQLite)
		where, _, params := builder.Build()

		require.Equal(t, "i.id NOT IN (SELECT issue_id FROM labels WHERE label = ?)", where)
		require.Equal(t, []interface{}{"urgent"}, params)
	})

	t.Run("label contains", func(t *testing.T) {
		parser := NewParser("label ~ spec")
		query, err := parser.Parse()
		require.NoError(t, err)

		builder := NewSQLBuilder(query, appbeads.DialectSQLite)
		where, _, params := builder.Build()

		require.Equal(t, "i.id IN (SELECT issue_id FROM labels WHERE label LIKE ?)", where)
		require.Equal(t, []interface{}{"%spec%"}, params)
	})

	t.Run("label not contains", func(t *testing.T) {
		parser := NewParser("label !~ backlog")
		query, err := parser.Parse()
		require.NoError(t, err)

		builder := NewSQLBuilder(query, appbeads.DialectSQLite)
		where, _, params := builder.Build()

		require.Equal(t, "i.id NOT IN (SELECT issue_id FROM labels WHERE label LIKE ?)", where)
		require.Equal(t, []interface{}{"%backlog%"}, params)
	})
}

func TestSQLBuilder_InExpression(t *testing.T) {
	t.Run("type in list", func(t *testing.T) {
		parser := NewParser("type in (bug, task)")
		query, err := parser.Parse()
		require.NoError(t, err)

		builder := NewSQLBuilder(query, appbeads.DialectSQLite)
		where, _, params := builder.Build()

		require.Equal(t, "i.issue_type IN (?, ?)", where)
		require.Equal(t, []interface{}{"bug", "task"}, params)
	})

	t.Run("type not in list", func(t *testing.T) {
		parser := NewParser("type not in (epic, chore)")
		query, err := parser.Parse()
		require.NoError(t, err)

		builder := NewSQLBuilder(query, appbeads.DialectSQLite)
		where, _, params := builder.Build()

		require.Equal(t, "i.issue_type NOT IN (?, ?)", where)
		require.Equal(t, []interface{}{"epic", "chore"}, params)
	})

	t.Run("priority in list", func(t *testing.T) {
		parser := NewParser("priority in (P0, P1)")
		query, err := parser.Parse()
		require.NoError(t, err)

		builder := NewSQLBuilder(query, appbeads.DialectSQLite)
		where, _, params := builder.Build()

		require.Equal(t, "i.priority IN (?, ?)", where)
		require.Equal(t, []interface{}{0, 1}, params)
	})

	t.Run("label in list", func(t *testing.T) {
		parser := NewParser("label in (urgent, critical)")
		query, err := parser.Parse()
		require.NoError(t, err)

		builder := NewSQLBuilder(query, appbeads.DialectSQLite)
		where, _, params := builder.Build()

		require.Equal(t, "i.id IN (SELECT issue_id FROM labels WHERE label IN (?, ?))", where)
		require.Equal(t, []interface{}{"urgent", "critical"}, params)
	})
}

func TestSQLBuilder_BinaryExpressions(t *testing.T) {
	t.Run("and expression", func(t *testing.T) {
		parser := NewParser("type = bug and priority = P0")
		query, err := parser.Parse()
		require.NoError(t, err)

		builder := NewSQLBuilder(query, appbeads.DialectSQLite)
		where, _, params := builder.Build()

		require.Equal(t, "(i.issue_type = ? AND i.priority = ?)", where)
		require.Equal(t, []interface{}{"bug", 0}, params)
	})

	t.Run("or expression", func(t *testing.T) {
		parser := NewParser("type = bug or type = task")
		query, err := parser.Parse()
		require.NoError(t, err)

		builder := NewSQLBuilder(query, appbeads.DialectSQLite)
		where, _, params := builder.Build()

		require.Equal(t, "(i.issue_type = ? OR i.issue_type = ?)", where)
		require.Equal(t, []interface{}{"bug", "task"}, params)
	})

	t.Run("complex and/or", func(t *testing.T) {
		parser := NewParser("type = bug and priority = P0 or status = open")
		query, err := parser.Parse()
		require.NoError(t, err)

		builder := NewSQLBuilder(query, appbeads.DialectSQLite)
		where, _, params := builder.Build()

		// Should be ((bug AND P0) OR open) due to precedence
		require.Equal(t, "((i.issue_type = ? AND i.priority = ?) OR i.status = ?)", where)
		require.Equal(t, []interface{}{"bug", 0, "open"}, params)
	})
}

func TestSQLBuilder_NotExpression(t *testing.T) {
	parser := NewParser("not blocked = true")
	query, err := parser.Parse()
	require.NoError(t, err)

	builder := NewSQLBuilder(query, appbeads.DialectSQLite)
	where, _, _ := builder.Build()

	require.Equal(t, "NOT (i.id IN (SELECT issue_id FROM blocked_issues_cache))", where)
}

func TestSQLBuilder_OrderBy(t *testing.T) {
	t.Run("single field", func(t *testing.T) {
		parser := NewParser("type = bug order by created")
		query, err := parser.Parse()
		require.NoError(t, err)

		builder := NewSQLBuilder(query, appbeads.DialectSQLite)
		_, orderBy, _ := builder.Build()

		require.Equal(t, "i.created_at ASC", orderBy)
	})

	t.Run("single field desc", func(t *testing.T) {
		parser := NewParser("type = bug order by created desc")
		query, err := parser.Parse()
		require.NoError(t, err)

		builder := NewSQLBuilder(query, appbeads.DialectSQLite)
		_, orderBy, _ := builder.Build()

		require.Equal(t, "i.created_at DESC", orderBy)
	})

	t.Run("multiple fields", func(t *testing.T) {
		parser := NewParser("status = open order by priority asc, created desc")
		query, err := parser.Parse()
		require.NoError(t, err)

		builder := NewSQLBuilder(query, appbeads.DialectSQLite)
		_, orderBy, _ := builder.Build()

		require.Equal(t, "i.priority ASC, i.created_at DESC", orderBy)
	})

	t.Run("order by only", func(t *testing.T) {
		parser := NewParser("order by updated desc")
		query, err := parser.Parse()
		require.NoError(t, err)

		builder := NewSQLBuilder(query, appbeads.DialectSQLite)
		where, orderBy, params := builder.Build()

		require.Empty(t, where)
		require.Equal(t, "i.updated_at DESC", orderBy)
		require.Empty(t, params)
	})
}

func TestSQLBuilder_DateComparisons(t *testing.T) {
	tests := []struct {
		input     string
		wantWhere string
	}{
		// Column wrapped in datetime() to normalize ISO 8601 with timezone to UTC
		{"created > today", "datetime(i.created_at) > date('now')"},
		{"created > yesterday", "datetime(i.created_at) > date('now', '-1 day')"},
		{"created > -7d", "datetime(i.created_at) > date('now', '-7 days')"},
		{"updated >= -30d", "datetime(i.updated_at) >= date('now', '-30 days')"},
		// Hour offsets use datetime() for sub-day precision
		{"created > -24h", "datetime(i.created_at) > datetime('now', '-24 hours')"},
		{"updated >= -1h", "datetime(i.updated_at) >= datetime('now', '-1 hours')"},
		// Month offsets
		{"created > -3m", "datetime(i.created_at) > date('now', '-3 months')"},
		{"updated >= -1m", "datetime(i.updated_at) >= date('now', '-1 months')"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			parser := NewParser(tt.input)
			query, err := parser.Parse()
			require.NoError(t, err)

			builder := NewSQLBuilder(query, appbeads.DialectSQLite)
			where, _, _ := builder.Build()

			require.Equal(t, tt.wantWhere, where)
		})
	}
}

func TestSQLBuilder_ComplexQuery(t *testing.T) {
	input := "(type = bug or type = task) and blocked = false order by priority asc, created desc"

	parser := NewParser(input)
	query, err := parser.Parse()
	require.NoError(t, err)

	builder := NewSQLBuilder(query, appbeads.DialectSQLite)
	where, orderBy, params := builder.Build()

	require.Equal(t, "((i.issue_type = ? OR i.issue_type = ?) AND i.id NOT IN (SELECT issue_id FROM blocked_issues_cache))", where)
	require.Equal(t, "i.priority ASC, i.created_at DESC", orderBy)
	require.Equal(t, []interface{}{"bug", "task"}, params)
}

func TestSQLBuilder_MySQLDialect_ContainsIsCaseInsensitive(t *testing.T) {
	// Dolt's default utf8mb4_0900_bin collation makes LIKE case-sensitive,
	// so ~ and !~ must lowercase both sides to match SQLite behavior.
	tests := []struct {
		input      string
		wantWhere  string
		wantParams []interface{}
	}{
		{
			input:      `title ~ "Test Issue"`,
			wantWhere:  "LOWER(i.title) LIKE ?",
			wantParams: []interface{}{"%test issue%"},
		},
		{
			input:      `title !~ "WIP"`,
			wantWhere:  "LOWER(i.title) NOT LIKE ?",
			wantParams: []interface{}{"%wip%"},
		},
		{
			input:      "assignee ~ Bob",
			wantWhere:  "LOWER(COALESCE(i.assignee, '')) LIKE ?",
			wantParams: []interface{}{"%bob%"},
		},
		{
			input:      "label ~ Spec",
			wantWhere:  "i.id IN (SELECT issue_id FROM labels WHERE LOWER(label) LIKE ?)",
			wantParams: []interface{}{"%spec%"},
		},
		{
			input:      "label !~ Backlog",
			wantWhere:  "i.id NOT IN (SELECT issue_id FROM labels WHERE LOWER(label) LIKE ?)",
			wantParams: []interface{}{"%backlog%"},
		},
		{
			input:      "metadata.team ~ Backend",
			wantWhere:  "LOWER(JSON_UNQUOTE(JSON_EXTRACT(i.metadata, ?))) LIKE ?",
			wantParams: []interface{}{`$."team"`, "%backend%"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			query, err := NewParser(tt.input).Parse()
			require.NoError(t, err)

			where, _, params := NewSQLBuilder(query, appbeads.DialectMySQL).Build()

			require.Equal(t, tt.wantWhere, where)
			require.Equal(t, tt.wantParams, params)
		})
	}
}

func TestSQLBuilder_MySQLDialect_DateToday(t *testing.T) {
	parser := NewParser("created > today")
	query, err := parser.Parse()
	require.NoError(t, err)

	builder := NewSQLBuilder(query, appbeads.DialectMySQL)
	where, _, _ := builder.Build()

	// MySQL: no datetime() wrapper; dates are UTC like bd and SQLite
	require.Equal(t, "i.created_at > DATE(UTC_TIMESTAMP())", where)
}

func TestSQLBuilder_MySQLDialect_DateYesterday(t *testing.T) {
	parser := NewParser("updated >= yesterday")
	query, err := parser.Parse()
	require.NoError(t, err)

	builder := NewSQLBuilder(query, appbeads.DialectMySQL)
	where, _, _ := builder.Build()

	require.Equal(t, "i.updated_at >= DATE_SUB(DATE(UTC_TIMESTAMP()), INTERVAL 1 DAY)", where)
}

func TestSQLBuilder_MySQLDialect_RelativeDays(t *testing.T) {
	parser := NewParser("created > -7d")
	query, err := parser.Parse()
	require.NoError(t, err)

	builder := NewSQLBuilder(query, appbeads.DialectMySQL)
	where, _, _ := builder.Build()

	require.Equal(t, "i.created_at > DATE_SUB(DATE(UTC_TIMESTAMP()), INTERVAL 7 DAY)", where)
}

func TestSQLBuilder_MySQLDialect_RelativeHours(t *testing.T) {
	parser := NewParser("updated > -5h")
	query, err := parser.Parse()
	require.NoError(t, err)

	builder := NewSQLBuilder(query, appbeads.DialectMySQL)
	where, _, _ := builder.Build()

	require.Equal(t, "i.updated_at > DATE_SUB(UTC_TIMESTAMP(), INTERVAL 5 HOUR)", where)
}

func TestSQLBuilder_MySQLDialect_RelativeMonths(t *testing.T) {
	parser := NewParser("created > -3m")
	query, err := parser.Parse()
	require.NoError(t, err)

	builder := NewSQLBuilder(query, appbeads.DialectMySQL)
	where, _, _ := builder.Build()

	require.Equal(t, "i.created_at > DATE_SUB(DATE(UTC_TIMESTAMP()), INTERVAL 3 MONTH)", where)
}

func TestSQLBuilder_MySQLDialect_ISODate(t *testing.T) {
	parser := NewParser(`created > "2024-01-15"`)
	query, err := parser.Parse()
	require.NoError(t, err)

	builder := NewSQLBuilder(query, appbeads.DialectMySQL)
	where, _, params := builder.Build()

	// MySQL: no datetime() wrapper, ISO date passed as parameter
	require.Equal(t, "i.created_at > ?", where)
	require.Equal(t, []any{"2024-01-15"}, params)
}

func TestSQLBuilder_MySQLDialect_NonDateQuery(t *testing.T) {
	// Non-date queries should produce identical SQL regardless of dialect
	parser := NewParser("type = bug and status = open")
	query, err := parser.Parse()
	require.NoError(t, err)

	sqliteBuilder := NewSQLBuilder(query, appbeads.DialectSQLite)
	sqliteWhere, _, sqliteParams := sqliteBuilder.Build()

	mysqlBuilder := NewSQLBuilder(query, appbeads.DialectMySQL)
	mysqlWhere, _, mysqlParams := mysqlBuilder.Build()

	require.Equal(t, sqliteWhere, mysqlWhere)
	require.Equal(t, sqliteParams, mysqlParams)
}

func TestSQLBuilder_MySQLDialect_BlockedTrue(t *testing.T) {
	parser := NewParser("blocked = true")
	query, err := parser.Parse()
	require.NoError(t, err)

	builder := NewSQLBuilder(query, appbeads.DialectMySQL)
	where, _, params := builder.Build()

	// Dolt inlines the blocked_issues view SQL to bypass a Dolt server bug with views
	require.Contains(t, where, "i.id IN (SELECT bi.id FROM issues bi")
	require.Contains(t, where, "d.type = 'blocks'")
	require.Contains(t, where, "blocker.id = d.depends_on_issue_id")
	require.NotContains(t, where, "d.depends_on_id")
	require.Empty(t, params)
}

func TestSQLBuilder_MySQLDialect_BlockedTrueLegacyDependencyTargetColumn(t *testing.T) {
	parser := NewParser("blocked = true")
	query, err := parser.Parse()
	require.NoError(t, err)

	builder := NewSQLBuilder(query, appbeads.DialectMySQL, WithDependencyTargetColumn(legacyDependencyTargetColumn))
	where, _, params := builder.Build()

	require.Contains(t, where, "blocker.id = d.depends_on_id")
	require.NotContains(t, where, "d.depends_on_issue_id")
	require.Empty(t, params)
}

func TestSQLBuilder_MySQLDialect_BlockedFalse(t *testing.T) {
	parser := NewParser("blocked = false")
	query, err := parser.Parse()
	require.NoError(t, err)

	builder := NewSQLBuilder(query, appbeads.DialectMySQL)
	where, _, _ := builder.Build()

	require.Contains(t, where, "i.id NOT IN (SELECT bi.id FROM issues bi")
}

func TestSQLBuilder_MySQLDialect_NotBlocked(t *testing.T) {
	parser := NewParser("not blocked = true")
	query, err := parser.Parse()
	require.NoError(t, err)

	builder := NewSQLBuilder(query, appbeads.DialectMySQL)
	where, _, _ := builder.Build()

	require.Contains(t, where, "NOT (i.id IN (SELECT bi.id FROM issues bi")
}

func TestSQLBuilder_MySQLDialect_ComplexWithBlocked(t *testing.T) {
	input := "(type = bug or type = task) and blocked = false order by priority asc, created desc"

	parser := NewParser(input)
	query, err := parser.Parse()
	require.NoError(t, err)

	builder := NewSQLBuilder(query, appbeads.DialectMySQL)
	where, orderBy, params := builder.Build()

	require.Contains(t, where, "i.id NOT IN (SELECT bi.id FROM issues bi")
	require.Contains(t, where, "(i.issue_type = ? OR i.issue_type = ?)")
	require.Equal(t, "i.priority ASC, i.created_at DESC", orderBy)
	require.Equal(t, []interface{}{"bug", "task"}, params)
}

func TestSQLBuilder_MySQLDialect_ReadyField(t *testing.T) {
	parser := NewParser("ready = true")
	query, err := parser.Parse()
	require.NoError(t, err)

	// SQLite inlines readiness so defer_until is always honored.
	sqliteBuilder := NewSQLBuilder(query, appbeads.DialectSQLite)
	sqliteWhere, _, _ := sqliteBuilder.Build()
	require.Equal(t, "("+sqliteReadyExpr+")", sqliteWhere)

	// Dolt also inlines readiness to bypass views and honor defer_until,
	// comparing against UTC_TIMESTAMP() as bd ready does.
	mysqlBuilder := NewSQLBuilder(query, appbeads.DialectMySQL)
	mysqlWhere, _, params := mysqlBuilder.Build()
	require.Equal(t, "("+mysqlFallbackReadyExpr+")", mysqlWhere)
	require.Empty(t, params)
}

// mysqlFallbackBlockedSubquery is the inlined blocked_issues view for a Dolt
// schema without is_blocked, depends_on_external or custom_statuses.
const mysqlFallbackBlockedSubquery = `SELECT bi.id FROM issues bi
WHERE bi.status NOT IN ('closed', 'pinned')
AND EXISTS (
  SELECT 1 FROM dependencies d
  WHERE d.issue_id = bi.id AND d.type = 'blocks'
  AND EXISTS (
    SELECT 1 FROM issues blocker
    WHERE blocker.id = d.depends_on_issue_id
    AND blocker.status NOT IN ('closed', 'pinned')
  )
)`

// mysqlFallbackReadyExpr is the ready predicate for the same schema.
const mysqlFallbackReadyExpr = "i.status = 'open'" +
	" AND (i.pinned = 0 OR i.pinned IS NULL)" +
	" AND (i.ephemeral = 0 OR i.ephemeral IS NULL)" +
	" AND i.issue_type NOT IN (" + readyExcludedTypesSQL + ")" +
	" AND (i.defer_until IS NULL OR i.defer_until <= UTC_TIMESTAMP())" +
	" AND i.id NOT IN (" + mysqlFallbackBlockedSubquery + ")" +
	" AND NOT EXISTS (\n  SELECT 1 FROM dependencies dp\n  JOIN issues parent ON parent.id = dp.depends_on_issue_id\n" +
	"  WHERE dp.issue_id = i.id AND dp.type = 'parent-child'\n" +
	"  AND parent.defer_until IS NOT NULL AND parent.defer_until > UTC_TIMESTAMP()\n)"

// TestSQLBuilder_SchemaProbeStates checks blocked and ready SQL for each
// combination of optional schema features the executor probes for.
func TestSQLBuilder_SchemaProbeStates(t *testing.T) {
	const isBlockedExpr = "(i.is_blocked = 1 AND i.status NOT IN ('closed', 'pinned'))"

	// Dolt with custom_statuses and depends_on_external but no is_blocked
	// (beads migrations 0041-0045): the blocked_issues view from 0045 with
	// cross-prefix targets.
	const customExternalBlockedSubquery = `SELECT bi.id FROM issues bi
WHERE bi.status NOT IN ('closed', 'pinned') AND bi.status NOT IN (SELECT name FROM custom_statuses WHERE category IN ('done', 'frozen'))
AND EXISTS (
  SELECT 1 FROM dependencies d
  WHERE d.issue_id = bi.id AND d.type = 'blocks'
  AND EXISTS (
    SELECT 1 FROM issues blocker
    WHERE blocker.id = COALESCE(d.depends_on_issue_id, d.depends_on_external)
    AND blocker.status NOT IN ('closed', 'pinned') AND blocker.status NOT IN (SELECT name FROM custom_statuses WHERE category IN ('done', 'frozen'))
  )
)`
	const externalDeferredParent = " AND NOT EXISTS (\n  SELECT 1 FROM dependencies dp\n" +
		"  JOIN issues parent ON parent.id = COALESCE(dp.depends_on_issue_id, dp.depends_on_external)\n" +
		"  WHERE dp.issue_id = i.id AND dp.type = 'parent-child'\n" +
		"  AND parent.defer_until IS NOT NULL AND parent.defer_until > UTC_TIMESTAMP()\n)"
	const customOpenStatus = "(i.status = 'open' OR i.status IN (SELECT name FROM custom_statuses WHERE category = 'active'))"
	const mysqlCommonReady = " AND (i.pinned = 0 OR i.pinned IS NULL)" +
		" AND (i.ephemeral = 0 OR i.ephemeral IS NULL)" +
		" AND i.issue_type NOT IN (" + readyExcludedTypesSQL + ")" +
		" AND (i.defer_until IS NULL OR i.defer_until <= UTC_TIMESTAMP())"

	tests := []struct {
		name        string
		dialect     appbeads.SQLDialect
		schema      *Schema
		wantBlocked string
		wantReady   string
	}{
		{
			name:        "mysql unprobed falls back to inlined blocked_issues view",
			dialect:     appbeads.DialectMySQL,
			schema:      nil,
			wantBlocked: "i.id IN (" + mysqlFallbackBlockedSubquery + ")",
			wantReady:   "(" + mysqlFallbackReadyExpr + ")",
		},
		{
			name:    "mysql with custom statuses and depends_on_external, no is_blocked",
			dialect: appbeads.DialectMySQL,
			schema: &Schema{
				DependencyTarget:     issueDependencyTargetColumn,
				HasDependsOnExternal: true,
				HasCustomStatuses:    true,
			},
			wantBlocked: "i.id IN (" + customExternalBlockedSubquery + ")",
			wantReady: "(" + customOpenStatus + mysqlCommonReady +
				" AND i.id NOT IN (" + customExternalBlockedSubquery + ")" + externalDeferredParent + ")",
		},
		{
			name:    "mysql current beads schema uses is_blocked",
			dialect: appbeads.DialectMySQL,
			schema: &Schema{
				DependencyTarget:     issueDependencyTargetColumn,
				HasDependsOnExternal: true,
				HasIsBlocked:         true,
				HasCustomStatuses:    true,
				HasCustomTypes:       true,
			},
			wantBlocked: isBlockedExpr,
			wantReady:   "(" + customOpenStatus + mysqlCommonReady + " AND i.is_blocked = 0" + externalDeferredParent + ")",
		},
		{
			name:    "mysql is_blocked without custom statuses or external",
			dialect: appbeads.DialectMySQL,
			schema: &Schema{
				DependencyTarget: issueDependencyTargetColumn,
				HasIsBlocked:     true,
			},
			wantBlocked: isBlockedExpr,
			wantReady: "(i.status = 'open'" + mysqlCommonReady + " AND i.is_blocked = 0" +
				" AND NOT EXISTS (\n  SELECT 1 FROM dependencies dp\n  JOIN issues parent ON parent.id = dp.depends_on_issue_id\n" +
				"  WHERE dp.issue_id = i.id AND dp.type = 'parent-child'\n" +
				"  AND parent.defer_until IS NOT NULL AND parent.defer_until > UTC_TIMESTAMP()\n))",
		},
		{
			name:        "sqlite unprobed uses blocked_issues_cache",
			dialect:     appbeads.DialectSQLite,
			schema:      nil,
			wantBlocked: "i.id IN (SELECT issue_id FROM blocked_issues_cache)",
			wantReady:   "(" + sqliteReadyExpr + ")",
		},
		{
			name:    "sqlite with is_blocked and custom statuses",
			dialect: appbeads.DialectSQLite,
			schema: &Schema{
				DependencyTarget:  legacyDependencyTargetColumn,
				HasIsBlocked:      true,
				HasCustomStatuses: true,
			},
			wantBlocked: isBlockedExpr,
			wantReady: "(" + customOpenStatus +
				" AND (i.pinned = 0 OR i.pinned IS NULL)" +
				" AND (i.ephemeral = 0 OR i.ephemeral IS NULL)" +
				" AND i.issue_type NOT IN (" + readyExcludedTypesSQL + ")" +
				" AND (i.defer_until IS NULL OR datetime(i.defer_until) <= datetime('now'))" +
				" AND i.is_blocked = 0" +
				" AND NOT EXISTS (\n  SELECT 1 FROM dependencies dp\n  JOIN issues parent ON parent.id = dp.depends_on_id\n" +
				"  WHERE dp.issue_id = i.id AND dp.type = 'parent-child'\n" +
				"  AND parent.defer_until IS NOT NULL AND datetime(parent.defer_until) > datetime('now')\n))",
		},
		{
			name:    "sqlite without pinned and ephemeral columns skips those predicates",
			dialect: appbeads.DialectSQLite,
			schema: &Schema{
				DependencyTarget: legacyDependencyTargetColumn,
				IssueColumns:     map[string]bool{"id": true, "status": true, "issue_type": true, "defer_until": true},
			},
			wantBlocked: "i.id IN (SELECT issue_id FROM blocked_issues_cache)",
			wantReady: "(i.status = 'open'" +
				" AND i.issue_type NOT IN (" + readyExcludedTypesSQL + ")" +
				" AND (i.defer_until IS NULL OR datetime(i.defer_until) <= datetime('now'))" +
				" AND i.id NOT IN (SELECT issue_id FROM blocked_issues_cache)" +
				" AND NOT EXISTS (\n  SELECT 1 FROM dependencies dp\n  JOIN issues parent ON parent.id = dp.depends_on_id\n" +
				"  WHERE dp.issue_id = i.id AND dp.type = 'parent-child'\n" +
				"  AND parent.defer_until IS NOT NULL AND datetime(parent.defer_until) > datetime('now')\n))",
		},
	}

	build := func(t *testing.T, input string, dialect appbeads.SQLDialect, schema *Schema) string {
		t.Helper()
		query, err := NewParser(input).Parse()
		require.NoError(t, err)
		where, _, params := NewSQLBuilder(query, dialect, WithSchema(schema)).Build()
		require.Empty(t, params, "blocked/ready SQL must not bind params")
		return where
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.wantBlocked, build(t, "blocked = true", tt.dialect, tt.schema))
			require.Equal(t, tt.wantReady, build(t, "ready = true", tt.dialect, tt.schema))

			// false is the exact negation of true.
			wantNotBlocked := "NOT " + tt.wantBlocked
			if strings.HasPrefix(tt.wantBlocked, "i.id IN (") {
				wantNotBlocked = "i.id NOT IN (" + strings.TrimPrefix(tt.wantBlocked, "i.id IN (")
			}
			require.Equal(t, wantNotBlocked, build(t, "blocked = false", tt.dialect, tt.schema))
			require.Equal(t, "NOT "+tt.wantReady, build(t, "ready = false", tt.dialect, tt.schema))
		})
	}
}

func TestSchema_DependencyTargetExpr(t *testing.T) {
	require.Equal(t, "d.depends_on_issue_id",
		(&Schema{DependencyTarget: issueDependencyTargetColumn}).DependencyTargetExpr("d"))
	require.Equal(t, "d.depends_on_id",
		(&Schema{DependencyTarget: legacyDependencyTargetColumn}).DependencyTargetExpr("d"))
	require.Equal(t, "COALESCE(dp.depends_on_issue_id, dp.depends_on_external)",
		(&Schema{DependencyTarget: issueDependencyTargetColumn, HasDependsOnExternal: true}).DependencyTargetExpr("dp"))
}

func TestSchema_HasIssueColumn(t *testing.T) {
	var unknown *Schema
	require.True(t, unknown.HasIssueColumn("started_at"), "nil schema assumes every column exists")
	require.True(t, (&Schema{}).HasIssueColumn("started_at"), "nil column set assumes every column exists")

	s := &Schema{IssueColumns: map[string]bool{"closed_at": true}}
	require.True(t, s.HasIssueColumn("closed_at"))
	require.False(t, s.HasIssueColumn("started_at"))
}

func TestSQLBuilder_NewFields(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		dialect     appbeads.SQLDialect
		wantWhere   string
		wantParams  []interface{}
		wantOrderBy string
	}{
		{
			name:       "owner equals uses COALESCE",
			input:      "owner = alice",
			dialect:    appbeads.DialectSQLite,
			wantWhere:  "COALESCE(i.owner, '') = ?",
			wantParams: []interface{}{"alice"},
		},
		{
			name:       "owner contains on mysql lowercases",
			input:      "owner ~ Alice",
			dialect:    appbeads.DialectMySQL,
			wantWhere:  "LOWER(COALESCE(i.owner, '')) LIKE ?",
			wantParams: []interface{}{"%alice%"},
		},
		{
			name:       "close_reason contains",
			input:      `close_reason ~ "duplicate"`,
			dialect:    appbeads.DialectSQLite,
			wantWhere:  "COALESCE(i.close_reason, '') LIKE ?",
			wantParams: []interface{}{"%duplicate%"},
		},
		{
			name:       "acceptance_criteria not contains",
			input:      `acceptance_criteria !~ "tests"`,
			dialect:    appbeads.DialectSQLite,
			wantWhere:  "COALESCE(i.acceptance_criteria, '') NOT LIKE ?",
			wantParams: []interface{}{"%tests%"},
		},
		{
			name:      "started maps to started_at on sqlite",
			input:     "started >= -7d",
			dialect:   appbeads.DialectSQLite,
			wantWhere: "datetime(i.started_at) >= date('now', '-7 days')",
		},
		{
			name:      "closed maps to closed_at on mysql",
			input:     "closed > -24h",
			dialect:   appbeads.DialectMySQL,
			wantWhere: "i.closed_at > DATE_SUB(UTC_TIMESTAMP(), INTERVAL 24 HOUR)",
		},
		{
			name:      "due maps to due_at",
			input:     "due < now",
			dialect:   appbeads.DialectMySQL,
			wantWhere: "i.due_at < UTC_TIMESTAMP()",
		},
		{
			name:        "order by new date fields",
			input:       "type = task order by due asc, started desc, closed",
			dialect:     appbeads.DialectSQLite,
			wantWhere:   "i.issue_type = ?",
			wantParams:  []interface{}{"task"},
			wantOrderBy: "i.due_at ASC, i.started_at DESC, i.closed_at ASC",
		},
		{
			name:       "custom type values are accepted",
			input:      "type in (decision, gate, convoy)",
			dialect:    appbeads.DialectSQLite,
			wantWhere:  "i.issue_type IN (?, ?, ?)",
			wantParams: []interface{}{"decision", "gate", "convoy"},
		},
		{
			name:       "metadata slash key is quoted",
			input:      `metadata.jira/sprint = "Q1"`,
			dialect:    appbeads.DialectMySQL,
			wantWhere:  "JSON_UNQUOTE(JSON_EXTRACT(i.metadata, ?)) = ?",
			wantParams: []interface{}{`$."jira/sprint"`, "Q1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query, err := NewParser(tt.input).Parse()
			require.NoError(t, err)
			require.NoError(t, Validate(query))

			where, orderBy, params := NewSQLBuilder(query, tt.dialect).Build()
			require.Equal(t, tt.wantWhere, where)
			require.Equal(t, tt.wantParams, params)
			require.Equal(t, tt.wantOrderBy, orderBy)
		})
	}
}

func TestJSONMetadataPath(t *testing.T) {
	require.Equal(t, `$."team"`, jsonMetadataPath("team"))
	require.Equal(t, `$."gc.routed_to"`, jsonMetadataPath("gc.routed_to"))
	require.Equal(t, `$."jira/sprint"`, jsonMetadataPath("jira/sprint"))
	// Defense in depth: quotes and backslashes are escaped.
	require.Equal(t, `$."a\"b\\c"`, jsonMetadataPath(`a"b\c`))
}

func TestSQLBuilder_MetadataField(t *testing.T) {
	// SQLite has no JSON_UNQUOTE, so value comparisons unquote text values
	// via json_extract and render everything else as JSON text via ->.
	const sqliteValue = "CASE JSON_TYPE(i.metadata, ?) WHEN 'text' THEN JSON_EXTRACT(i.metadata, ?) ELSE i.metadata -> ? END"
	const mysqlValue = "JSON_UNQUOTE(JSON_EXTRACT(i.metadata, ?))"

	tests := []struct {
		name        string
		input       string
		sqliteWhere string
		sqliteArgs  []interface{}
		mysqlWhere  string
		mysqlArgs   []interface{}
	}{
		{
			name:        "metadata equals string",
			input:       `metadata.team = "backend"`,
			sqliteWhere: sqliteValue + " = ?",
			sqliteArgs:  []interface{}{`$."team"`, `$."team"`, `$."team"`, "backend"},
			mysqlWhere:  mysqlValue + " = ?",
			mysqlArgs:   []interface{}{`$."team"`, "backend"},
		},
		{
			name:        "metadata not equals string",
			input:       `metadata.team != "backend"`,
			sqliteWhere: sqliteValue + " != ?",
			sqliteArgs:  []interface{}{`$."team"`, `$."team"`, `$."team"`, "backend"},
			mysqlWhere:  mysqlValue + " != ?",
			mysqlArgs:   []interface{}{`$."team"`, "backend"},
		},
		{
			name:        "metadata contains pattern",
			input:       `metadata.team ~ Auth`,
			sqliteWhere: sqliteValue + " LIKE ?",
			sqliteArgs:  []interface{}{`$."team"`, `$."team"`, `$."team"`, "%Auth%"},
			mysqlWhere:  "LOWER(" + mysqlValue + ") LIKE ?",
			mysqlArgs:   []interface{}{`$."team"`, "%auth%"},
		},
		{
			name:        "metadata not contains pattern",
			input:       `metadata.team !~ legacy`,
			sqliteWhere: sqliteValue + " NOT LIKE ?",
			sqliteArgs:  []interface{}{`$."team"`, `$."team"`, `$."team"`, "%legacy%"},
			mysqlWhere:  "LOWER(" + mysqlValue + ") NOT LIKE ?",
			mysqlArgs:   []interface{}{`$."team"`, "%legacy%"},
		},
		{
			name:        "metadata key exists",
			input:       "metadata.team != nil",
			sqliteWhere: "JSON_TYPE(i.metadata, ?) IS NOT NULL",
			sqliteArgs:  []interface{}{`$."team"`},
			mysqlWhere:  "JSON_EXTRACT(i.metadata, ?) IS NOT NULL",
			mysqlArgs:   []interface{}{`$."team"`},
		},
		{
			name:        "metadata key not exists",
			input:       "metadata.team = nil",
			sqliteWhere: "JSON_TYPE(i.metadata, ?) IS NULL",
			sqliteArgs:  []interface{}{`$."team"`},
			mysqlWhere:  "JSON_EXTRACT(i.metadata, ?) IS NULL",
			mysqlArgs:   []interface{}{`$."team"`},
		},
		{
			name:        "metadata dotted key is a literal key",
			input:       `metadata.jira.sprint = "Q1-2026"`,
			sqliteWhere: sqliteValue + " = ?",
			sqliteArgs:  []interface{}{`$."jira.sprint"`, `$."jira.sprint"`, `$."jira.sprint"`, "Q1-2026"},
			mysqlWhere:  mysqlValue + " = ?",
			mysqlArgs:   []interface{}{`$."jira.sprint"`, "Q1-2026"},
		},
		{
			name:        "metadata combined with other filters",
			input:       `type = task and metadata.team = "backend"`,
			sqliteWhere: "(i.issue_type = ? AND " + sqliteValue + " = ?)",
			sqliteArgs:  []interface{}{"task", `$."team"`, `$."team"`, `$."team"`, "backend"},
			mysqlWhere:  "(i.issue_type = ? AND " + mysqlValue + " = ?)",
			mysqlArgs:   []interface{}{"task", `$."team"`, "backend"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query, err := NewParser(tt.input).Parse()
			require.NoError(t, err)

			where, _, params := NewSQLBuilder(query, appbeads.DialectSQLite).Build()
			require.Equal(t, tt.sqliteWhere, where)
			require.Equal(t, tt.sqliteArgs, params)
			require.NotContains(t, where, "JSON_UNQUOTE", "SQLite has no JSON_UNQUOTE function")

			where, _, params = NewSQLBuilder(query, appbeads.DialectMySQL).Build()
			require.Equal(t, tt.mysqlWhere, where)
			require.Equal(t, tt.mysqlArgs, params)
		})
	}
}
