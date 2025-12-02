package bql

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

			builder := NewSQLBuilder(query)
			where, orderBy, params := builder.Build()

			assert.Equal(t, tt.wantWhere, where)
			assert.Equal(t, tt.wantParams, params)
			assert.Equal(t, tt.wantOrderBy, orderBy)
		})
	}
}

func TestSQLBuilder_SpecialFields(t *testing.T) {
	t.Run("blocked true", func(t *testing.T) {
		parser := NewParser("blocked = true")
		query, err := parser.Parse()
		require.NoError(t, err)

		builder := NewSQLBuilder(query)
		where, _, params := builder.Build()

		assert.Equal(t, "i.id IN (SELECT issue_id FROM blocked_issues_cache)", where)
		assert.Empty(t, params)
	})

	t.Run("blocked false", func(t *testing.T) {
		parser := NewParser("blocked = false")
		query, err := parser.Parse()
		require.NoError(t, err)

		builder := NewSQLBuilder(query)
		where, _, _ := builder.Build()

		assert.Equal(t, "i.id NOT IN (SELECT issue_id FROM blocked_issues_cache)", where)
	})

	t.Run("ready true", func(t *testing.T) {
		parser := NewParser("ready = true")
		query, err := parser.Parse()
		require.NoError(t, err)

		builder := NewSQLBuilder(query)
		where, _, _ := builder.Build()

		assert.Equal(t, "i.id IN (SELECT id FROM ready_issues)", where)
	})

	t.Run("single label", func(t *testing.T) {
		parser := NewParser("label = urgent")
		query, err := parser.Parse()
		require.NoError(t, err)

		builder := NewSQLBuilder(query)
		where, _, params := builder.Build()

		assert.Equal(t, "i.id IN (SELECT issue_id FROM labels WHERE label = ?)", where)
		assert.Equal(t, []interface{}{"urgent"}, params)
	})
}

func TestSQLBuilder_InExpression(t *testing.T) {
	t.Run("type in list", func(t *testing.T) {
		parser := NewParser("type in (bug, task)")
		query, err := parser.Parse()
		require.NoError(t, err)

		builder := NewSQLBuilder(query)
		where, _, params := builder.Build()

		assert.Equal(t, "i.issue_type IN (?, ?)", where)
		assert.Equal(t, []interface{}{"bug", "task"}, params)
	})

	t.Run("type not in list", func(t *testing.T) {
		parser := NewParser("type not in (epic, chore)")
		query, err := parser.Parse()
		require.NoError(t, err)

		builder := NewSQLBuilder(query)
		where, _, params := builder.Build()

		assert.Equal(t, "i.issue_type NOT IN (?, ?)", where)
		assert.Equal(t, []interface{}{"epic", "chore"}, params)
	})

	t.Run("priority in list", func(t *testing.T) {
		parser := NewParser("priority in (P0, P1)")
		query, err := parser.Parse()
		require.NoError(t, err)

		builder := NewSQLBuilder(query)
		where, _, params := builder.Build()

		assert.Equal(t, "i.priority IN (?, ?)", where)
		assert.Equal(t, []interface{}{0, 1}, params)
	})

	t.Run("label in list", func(t *testing.T) {
		parser := NewParser("label in (urgent, critical)")
		query, err := parser.Parse()
		require.NoError(t, err)

		builder := NewSQLBuilder(query)
		where, _, params := builder.Build()

		assert.Equal(t, "i.id IN (SELECT issue_id FROM labels WHERE label IN (?, ?))", where)
		assert.Equal(t, []interface{}{"urgent", "critical"}, params)
	})
}

func TestSQLBuilder_BinaryExpressions(t *testing.T) {
	t.Run("and expression", func(t *testing.T) {
		parser := NewParser("type = bug and priority = P0")
		query, err := parser.Parse()
		require.NoError(t, err)

		builder := NewSQLBuilder(query)
		where, _, params := builder.Build()

		assert.Equal(t, "(i.issue_type = ? AND i.priority = ?)", where)
		assert.Equal(t, []interface{}{"bug", 0}, params)
	})

	t.Run("or expression", func(t *testing.T) {
		parser := NewParser("type = bug or type = task")
		query, err := parser.Parse()
		require.NoError(t, err)

		builder := NewSQLBuilder(query)
		where, _, params := builder.Build()

		assert.Equal(t, "(i.issue_type = ? OR i.issue_type = ?)", where)
		assert.Equal(t, []interface{}{"bug", "task"}, params)
	})

	t.Run("complex and/or", func(t *testing.T) {
		parser := NewParser("type = bug and priority = P0 or status = open")
		query, err := parser.Parse()
		require.NoError(t, err)

		builder := NewSQLBuilder(query)
		where, _, params := builder.Build()

		// Should be ((bug AND P0) OR open) due to precedence
		assert.Equal(t, "((i.issue_type = ? AND i.priority = ?) OR i.status = ?)", where)
		assert.Equal(t, []interface{}{"bug", 0, "open"}, params)
	})
}

func TestSQLBuilder_NotExpression(t *testing.T) {
	parser := NewParser("not blocked = true")
	query, err := parser.Parse()
	require.NoError(t, err)

	builder := NewSQLBuilder(query)
	where, _, _ := builder.Build()

	assert.Equal(t, "NOT (i.id IN (SELECT issue_id FROM blocked_issues_cache))", where)
}

func TestSQLBuilder_OrderBy(t *testing.T) {
	t.Run("single field", func(t *testing.T) {
		parser := NewParser("type = bug order by created")
		query, err := parser.Parse()
		require.NoError(t, err)

		builder := NewSQLBuilder(query)
		_, orderBy, _ := builder.Build()

		assert.Equal(t, "i.created_at ASC", orderBy)
	})

	t.Run("single field desc", func(t *testing.T) {
		parser := NewParser("type = bug order by created desc")
		query, err := parser.Parse()
		require.NoError(t, err)

		builder := NewSQLBuilder(query)
		_, orderBy, _ := builder.Build()

		assert.Equal(t, "i.created_at DESC", orderBy)
	})

	t.Run("multiple fields", func(t *testing.T) {
		parser := NewParser("status = open order by priority asc, created desc")
		query, err := parser.Parse()
		require.NoError(t, err)

		builder := NewSQLBuilder(query)
		_, orderBy, _ := builder.Build()

		assert.Equal(t, "i.priority ASC, i.created_at DESC", orderBy)
	})

	t.Run("order by only", func(t *testing.T) {
		parser := NewParser("order by updated desc")
		query, err := parser.Parse()
		require.NoError(t, err)

		builder := NewSQLBuilder(query)
		where, orderBy, params := builder.Build()

		assert.Empty(t, where)
		assert.Equal(t, "i.updated_at DESC", orderBy)
		assert.Empty(t, params)
	})
}

func TestSQLBuilder_DateComparisons(t *testing.T) {
	tests := []struct {
		input     string
		wantWhere string
	}{
		{"created > today", "i.created_at > date('now')"},
		{"created > yesterday", "i.created_at > date('now', '-1 day')"},
		{"created > -7d", "i.created_at > date('now', '-7 days')"},
		{"updated >= -30d", "i.updated_at >= date('now', '-30 days')"},
		// Hour offsets use datetime() for sub-day precision
		{"created > -24h", "i.created_at > datetime('now', '-24 hours')"},
		{"updated >= -1h", "i.updated_at >= datetime('now', '-1 hours')"},
		// Month offsets
		{"created > -3m", "i.created_at > date('now', '-3 months')"},
		{"updated >= -1m", "i.updated_at >= date('now', '-1 months')"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			parser := NewParser(tt.input)
			query, err := parser.Parse()
			require.NoError(t, err)

			builder := NewSQLBuilder(query)
			where, _, _ := builder.Build()

			assert.Equal(t, tt.wantWhere, where)
		})
	}
}

func TestSQLBuilder_ComplexQuery(t *testing.T) {
	input := "(type = bug or type = task) and blocked = false order by priority asc, created desc"

	parser := NewParser(input)
	query, err := parser.Parse()
	require.NoError(t, err)

	builder := NewSQLBuilder(query)
	where, orderBy, params := builder.Build()

	assert.Equal(t, "((i.issue_type = ? OR i.issue_type = ?) AND i.id NOT IN (SELECT issue_id FROM blocked_issues_cache))", where)
	assert.Equal(t, "i.priority ASC, i.created_at DESC", orderBy)
	assert.Equal(t, []interface{}{"bug", "task"}, params)
}
