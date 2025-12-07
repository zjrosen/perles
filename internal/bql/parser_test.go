package bql

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParser_SimpleComparison(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		field    string
		op       TokenType
		value    string
		valType  ValueType
		intValue int
	}{
		{"equals string", "type = task", "type", TokenEq, "task", ValueString, 0},
		{"equals priority", "priority = P0", "priority", TokenEq, "P0", ValuePriority, 0},
		{"not equals", "status != closed", "status", TokenNeq, "closed", ValueString, 0},
		{"less than", "priority < P2", "priority", TokenLt, "P2", ValuePriority, 2},
		{"greater than", "priority > P1", "priority", TokenGt, "P1", ValuePriority, 1},
		{"less or equal", "priority <= P1", "priority", TokenLte, "P1", ValuePriority, 1},
		{"greater or equal", "priority >= P3", "priority", TokenGte, "P3", ValuePriority, 3},
		{"contains", "title ~ auth", "title", TokenContains, "auth", ValueString, 0},
		{"not contains", "title !~ test", "title", TokenNotContains, "test", ValueString, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewParser(tt.input)
			query, err := parser.Parse()
			require.NoError(t, err)
			require.NotNil(t, query.Filter)

			cmp, ok := query.Filter.(*CompareExpr)
			require.True(t, ok, "expected CompareExpr")
			require.Equal(t, tt.field, cmp.Field)
			require.Equal(t, tt.op, cmp.Op)
			require.Equal(t, tt.value, cmp.Value.String)
			require.Equal(t, tt.valType, cmp.Value.Type)
			if tt.valType == ValuePriority {
				require.Equal(t, tt.intValue, cmp.Value.Int)
			}
		})
	}
}

func TestParser_BooleanComparison(t *testing.T) {
	tests := []struct {
		input string
		field string
		value bool
	}{
		{"blocked = true", "blocked", true},
		{"blocked = false", "blocked", false},
		{"ready = true", "ready", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			parser := NewParser(tt.input)
			query, err := parser.Parse()
			require.NoError(t, err)

			cmp, ok := query.Filter.(*CompareExpr)
			require.True(t, ok)
			require.Equal(t, tt.field, cmp.Field)
			require.Equal(t, ValueBool, cmp.Value.Type)
			require.Equal(t, tt.value, cmp.Value.Bool)
		})
	}
}

func TestParser_InExpression(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		field  string
		not    bool
		values []string
	}{
		{"in two values", "type in (bug, task)", "type", false, []string{"bug", "task"}},
		{"in three values", "status in (open, in_progress, blocked)", "status", false, []string{"open", "in_progress", "blocked"}},
		{"not in", "label not in (backlog, deferred)", "label", true, []string{"backlog", "deferred"}},
		{"in single value", "type in (bug)", "type", false, []string{"bug"}},
		{"in values with colons", "label in (spec:010-output-schema-support)", "label", false, []string{"spec:010-output-schema-support"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewParser(tt.input)
			query, err := parser.Parse()
			require.NoError(t, err)

			in, ok := query.Filter.(*InExpr)
			require.True(t, ok, "expected InExpr")
			require.Equal(t, tt.field, in.Field)
			require.Equal(t, tt.not, in.Not)
			require.Len(t, in.Values, len(tt.values))
			for i, v := range tt.values {
				require.Equal(t, v, in.Values[i].String)
			}
		})
	}
}

func TestParser_BinaryExpressions(t *testing.T) {
	t.Run("and expression", func(t *testing.T) {
		parser := NewParser("type = bug and priority = P0")
		query, err := parser.Parse()
		require.NoError(t, err)

		bin, ok := query.Filter.(*BinaryExpr)
		require.True(t, ok, "expected BinaryExpr")
		require.Equal(t, TokenAnd, bin.Op)

		left, ok := bin.Left.(*CompareExpr)
		require.True(t, ok)
		require.Equal(t, "type", left.Field)

		right, ok := bin.Right.(*CompareExpr)
		require.True(t, ok)
		require.Equal(t, "priority", right.Field)
	})

	t.Run("or expression", func(t *testing.T) {
		parser := NewParser("type = bug or type = task")
		query, err := parser.Parse()
		require.NoError(t, err)

		bin, ok := query.Filter.(*BinaryExpr)
		require.True(t, ok)
		require.Equal(t, TokenOr, bin.Op)
	})

	t.Run("and/or precedence", func(t *testing.T) {
		// "A and B or C" should parse as "(A and B) or C"
		parser := NewParser("type = bug and priority = P0 or status = open")
		query, err := parser.Parse()
		require.NoError(t, err)

		// Top level should be OR
		bin, ok := query.Filter.(*BinaryExpr)
		require.True(t, ok)
		require.Equal(t, TokenOr, bin.Op)

		// Left side should be AND
		leftBin, ok := bin.Left.(*BinaryExpr)
		require.True(t, ok)
		require.Equal(t, TokenAnd, leftBin.Op)
	})

	t.Run("multiple and", func(t *testing.T) {
		parser := NewParser("type = bug and priority = P0 and status = open")
		query, err := parser.Parse()
		require.NoError(t, err)

		// Should be (bug AND P0) AND open
		bin, ok := query.Filter.(*BinaryExpr)
		require.True(t, ok)
		require.Equal(t, TokenAnd, bin.Op)
	})
}

func TestParser_NotExpression(t *testing.T) {
	parser := NewParser("not blocked = true")
	query, err := parser.Parse()
	require.NoError(t, err)

	not, ok := query.Filter.(*NotExpr)
	require.True(t, ok, "expected NotExpr")

	cmp, ok := not.Expr.(*CompareExpr)
	require.True(t, ok)
	require.Equal(t, "blocked", cmp.Field)
}

func TestParser_ParenthesesGrouping(t *testing.T) {
	t.Run("simple grouping", func(t *testing.T) {
		parser := NewParser("(type = bug or type = task) and priority = P0")
		query, err := parser.Parse()
		require.NoError(t, err)

		// Top level should be AND
		bin, ok := query.Filter.(*BinaryExpr)
		require.True(t, ok)
		require.Equal(t, TokenAnd, bin.Op)

		// Left side should be OR (grouped)
		leftBin, ok := bin.Left.(*BinaryExpr)
		require.True(t, ok)
		require.Equal(t, TokenOr, leftBin.Op)
	})

	t.Run("nested parentheses", func(t *testing.T) {
		parser := NewParser("((type = bug))")
		query, err := parser.Parse()
		require.NoError(t, err)

		cmp, ok := query.Filter.(*CompareExpr)
		require.True(t, ok)
		require.Equal(t, "type", cmp.Field)
	})
}

func TestParser_OrderBy(t *testing.T) {
	t.Run("order by single field", func(t *testing.T) {
		parser := NewParser("type = bug order by created")
		query, err := parser.Parse()
		require.NoError(t, err)
		require.Len(t, query.OrderBy, 1)
		require.Equal(t, "created", query.OrderBy[0].Field)
		require.False(t, query.OrderBy[0].Desc)
	})

	t.Run("order by desc", func(t *testing.T) {
		parser := NewParser("type = bug order by created desc")
		query, err := parser.Parse()
		require.NoError(t, err)
		require.Len(t, query.OrderBy, 1)
		require.Equal(t, "created", query.OrderBy[0].Field)
		require.True(t, query.OrderBy[0].Desc)
	})

	t.Run("order by asc explicit", func(t *testing.T) {
		parser := NewParser("type = bug order by priority asc")
		query, err := parser.Parse()
		require.NoError(t, err)
		require.Len(t, query.OrderBy, 1)
		require.Equal(t, "priority", query.OrderBy[0].Field)
		require.False(t, query.OrderBy[0].Desc)
	})

	t.Run("order by multiple fields", func(t *testing.T) {
		parser := NewParser("status = open order by priority asc, created desc")
		query, err := parser.Parse()
		require.NoError(t, err)
		require.Len(t, query.OrderBy, 2)
		require.Equal(t, "priority", query.OrderBy[0].Field)
		require.False(t, query.OrderBy[0].Desc)
		require.Equal(t, "created", query.OrderBy[1].Field)
		require.True(t, query.OrderBy[1].Desc)
	})

	t.Run("order by only (no filter)", func(t *testing.T) {
		parser := NewParser("order by updated desc")
		query, err := parser.Parse()
		require.NoError(t, err)
		require.Nil(t, query.Filter)
		require.Len(t, query.OrderBy, 1)
		require.Equal(t, "updated", query.OrderBy[0].Field)
	})
}

func TestParser_DateValues(t *testing.T) {
	tests := []struct {
		input   string
		dateStr string
	}{
		{"created > today", "today"},
		{"created > yesterday", "yesterday"},
		{"created > -7d", "-7d"},
		{"updated >= -30d", "-30d"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			parser := NewParser(tt.input)
			query, err := parser.Parse()
			require.NoError(t, err)

			cmp, ok := query.Filter.(*CompareExpr)
			require.True(t, ok)
			require.Equal(t, ValueDate, cmp.Value.Type)
			require.Equal(t, tt.dateStr, cmp.Value.String)
		})
	}
}

func TestParser_QuotedStrings(t *testing.T) {
	t.Run("double quotes", func(t *testing.T) {
		parser := NewParser(`title ~ "hello world"`)
		query, err := parser.Parse()
		require.NoError(t, err)

		cmp, ok := query.Filter.(*CompareExpr)
		require.True(t, ok)
		require.Equal(t, "hello world", cmp.Value.String)
	})

	t.Run("single quotes", func(t *testing.T) {
		parser := NewParser(`title ~ 'hello world'`)
		query, err := parser.Parse()
		require.NoError(t, err)

		cmp, ok := query.Filter.(*CompareExpr)
		require.True(t, ok)
		require.Equal(t, "hello world", cmp.Value.String)
	})
}

func TestParser_ComplexQueries(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"full example 1", "type = bug and priority = P0 order by created desc"},
		{"full example 2", "type in (bug, feature) or status = open"},
		{"full example 3", "ready = true and priority <= P1 order by priority asc"},
		{"full example 4", "label not in (backlog) and status = open order by updated desc"},
		{"full example 5", "(type = bug or type = task) and blocked = false order by priority asc, created desc"},
		{"with hyphenated id", "id = perles-123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewParser(tt.input)
			query, err := parser.Parse()
			require.NoError(t, err, "failed to parse: %s", tt.input)
			require.NotNil(t, query)
		})
	}
}

func TestParser_Errors(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"missing value", "type = "},
		{"missing field", "= task"},
		{"missing operator", "type task"},
		{"unclosed paren", "(type = bug"},
		{"missing in values", "type in ()"},
		{"missing in paren", "type in bug, task"},
		{"invalid token", "type = bug @@@ priority = P0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewParser(tt.input)
			_, err := parser.Parse()
			require.Error(t, err, "expected error for: %s", tt.input)
		})
	}
}

func TestParser_ExpandClause(t *testing.T) {
	tests := []struct {
		input     string
		wantType  ExpandType
		wantDepth ExpandDepth
		hasFilter bool
	}{
		{"type = epic expand children", ExpandChildren, DepthDefault, true},
		{"id = x expand blockers", ExpandBlockers, DepthDefault, true},
		{"priority = P0 expand blocks", ExpandBlocks, DepthDefault, true},
		{"status = open expand deps", ExpandDeps, DepthDefault, true},
		{"type = epic expand all", ExpandAll, DepthDefault, true},
		{"expand children order by priority", ExpandChildren, DepthDefault, false},
		{"expand deps", ExpandDeps, DepthDefault, false},
		{"type = epic expand children depth 1", ExpandChildren, 1, true},
		{"type = epic expand children depth 2", ExpandChildren, 2, true},
		{"type = epic expand children depth 10", ExpandChildren, 10, true},
		{"type = epic expand all depth *", ExpandAll, DepthUnlimited, true},
		{"expand blockers depth *", ExpandBlockers, DepthUnlimited, false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			p := NewParser(tt.input)
			query, err := p.Parse()
			require.NoError(t, err)
			require.NotNil(t, query.Expand)
			require.Equal(t, tt.wantType, query.Expand.Type)
			require.Equal(t, tt.wantDepth, query.Expand.Depth)
			if tt.hasFilter {
				require.NotNil(t, query.Filter)
			} else {
				require.Nil(t, query.Filter)
			}
		})
	}
}

func TestParser_ExpandWithOrderBy(t *testing.T) {
	p := NewParser("type = epic expand children depth 2 order by priority asc")
	query, err := p.Parse()
	require.NoError(t, err)
	require.NotNil(t, query.Filter)
	require.NotNil(t, query.Expand)
	require.Equal(t, ExpandChildren, query.Expand.Type)
	require.Equal(t, ExpandDepth(2), query.Expand.Depth)
	require.Len(t, query.OrderBy, 1)
	require.Equal(t, "priority", query.OrderBy[0].Field)
}

func TestParser_HasExpand(t *testing.T) {
	t.Run("query without expand", func(t *testing.T) {
		p := NewParser("type = epic")
		query, err := p.Parse()
		require.NoError(t, err)
		require.False(t, query.HasExpand())
	})

	t.Run("query with expand", func(t *testing.T) {
		p := NewParser("type = epic expand children")
		query, err := p.Parse()
		require.NoError(t, err)
		require.True(t, query.HasExpand())
	})
}

func TestParser_InvalidExpandType(t *testing.T) {
	p := NewParser("type = epic expand invalid")
	_, err := p.Parse()
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown expansion type")
}

func TestParser_InvalidDepthValue(t *testing.T) {
	tests := []struct {
		input   string
		wantErr string
	}{
		{"expand children depth 0", "depth must be at least 1"},
		{"expand children depth 11", "depth cannot exceed 10"},
		{"expand children depth foo", "expected depth value"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			p := NewParser(tt.input)
			_, err := p.Parse()
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestParser_ExpandTypeCaseInsensitive(t *testing.T) {
	types := []struct {
		input    string
		wantType ExpandType
	}{
		{"expand children", ExpandChildren},
		{"expand Children", ExpandChildren},
		{"expand CHILDREN", ExpandChildren},
		{"expand blockers", ExpandBlockers},
		{"expand Blockers", ExpandBlockers},
		{"expand BLOCKERS", ExpandBlockers},
		{"expand blocks", ExpandBlocks},
		{"expand deps", ExpandDeps},
		{"expand all", ExpandAll},
		{"expand ALL", ExpandAll},
	}

	for _, tt := range types {
		t.Run(tt.input, func(t *testing.T) {
			p := NewParser(tt.input)
			query, err := p.Parse()
			require.NoError(t, err)
			require.NotNil(t, query.Expand)
			require.Equal(t, tt.wantType, query.Expand.Type)
		})
	}
}
