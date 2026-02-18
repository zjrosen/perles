package bql

import (
	"testing"

	"github.com/stretchr/testify/require"
	appbeads "github.com/zjrosen/perles/internal/beads/application"
)

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

		require.Equal(t, "i.id IN (SELECT id FROM ready_issues)", where)
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

func TestSQLBuilder_MySQLDialect_DateToday(t *testing.T) {
	parser := NewParser("created > today")
	query, err := parser.Parse()
	require.NoError(t, err)

	builder := NewSQLBuilder(query, appbeads.DialectMySQL)
	where, _, _ := builder.Build()

	// MySQL: no datetime() wrapper, uses CURDATE()
	require.Equal(t, "i.created_at > CURDATE()", where)
}

func TestSQLBuilder_MySQLDialect_DateYesterday(t *testing.T) {
	parser := NewParser("updated >= yesterday")
	query, err := parser.Parse()
	require.NoError(t, err)

	builder := NewSQLBuilder(query, appbeads.DialectMySQL)
	where, _, _ := builder.Build()

	require.Equal(t, "i.updated_at >= DATE_SUB(CURDATE(), INTERVAL 1 DAY)", where)
}

func TestSQLBuilder_MySQLDialect_RelativeDays(t *testing.T) {
	parser := NewParser("created > -7d")
	query, err := parser.Parse()
	require.NoError(t, err)

	builder := NewSQLBuilder(query, appbeads.DialectMySQL)
	where, _, _ := builder.Build()

	require.Equal(t, "i.created_at > DATE_SUB(CURDATE(), INTERVAL 7 DAY)", where)
}

func TestSQLBuilder_MySQLDialect_RelativeHours(t *testing.T) {
	parser := NewParser("updated > -5h")
	query, err := parser.Parse()
	require.NoError(t, err)

	builder := NewSQLBuilder(query, appbeads.DialectMySQL)
	where, _, _ := builder.Build()

	require.Equal(t, "i.updated_at > DATE_SUB(NOW(), INTERVAL 5 HOUR)", where)
}

func TestSQLBuilder_MySQLDialect_RelativeMonths(t *testing.T) {
	parser := NewParser("created > -3m")
	query, err := parser.Parse()
	require.NoError(t, err)

	builder := NewSQLBuilder(query, appbeads.DialectMySQL)
	where, _, _ := builder.Build()

	require.Equal(t, "i.created_at > DATE_SUB(CURDATE(), INTERVAL 3 MONTH)", where)
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

	// SQLite uses the ready_issues view
	sqliteBuilder := NewSQLBuilder(query, appbeads.DialectSQLite)
	sqliteWhere, _, _ := sqliteBuilder.Build()
	require.Equal(t, "i.id IN (SELECT id FROM ready_issues)", sqliteWhere)

	// Dolt inlines the SQL to bypass a server bug with views
	mysqlBuilder := NewSQLBuilder(query, appbeads.DialectMySQL)
	mysqlWhere, _, _ := mysqlBuilder.Build()
	require.Contains(t, mysqlWhere, "i.id IN (SELECT ri.id FROM issues ri")
	require.Contains(t, mysqlWhere, "ri.status = 'open'")
	require.Contains(t, mysqlWhere, "NOT IN (SELECT bi.id FROM issues bi")
}
