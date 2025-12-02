package bql

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidate_ValidQueries(t *testing.T) {
	validQueries := []string{
		"type = bug",
		"type = feature",
		"type = task",
		"type = epic",
		"type = chore",
		"priority = P0",
		"priority < P2",
		"priority >= P1",
		"status = open",
		"status = in_progress",
		"status = closed",
		"blocked = true",
		"blocked = false",
		"ready = true",
		"label = urgent",
		"title ~ auth",
		"title !~ test",
		"id = perles-123",
		"created > today",
		"created > yesterday",
		"created > -7d",
		"updated >= -30d",
		"type = bug and priority = P0",
		"type in (bug, task)",
		"status in (open, in_progress)",
		"priority in (P0, P1)",
		"label not in (backlog)",
		"not blocked = true",
		"(type = bug or type = task) and status = open",
		"type = bug order by created desc",
		"order by priority asc, updated desc",
	}

	for _, query := range validQueries {
		t.Run(query, func(t *testing.T) {
			parser := NewParser(query)
			q, err := parser.Parse()
			require.NoError(t, err)

			err = Validate(q)
			assert.NoError(t, err, "query should be valid: %s", query)
		})
	}
}

func TestValidate_InvalidField(t *testing.T) {
	invalidQueries := []struct {
		query string
		field string
	}{
		{"foo = bar", "foo"},
		{"unknown = value", "unknown"},
		{"xyz = 123", "xyz"},
	}

	for _, tc := range invalidQueries {
		t.Run(tc.query, func(t *testing.T) {
			parser := NewParser(tc.query)
			q, err := parser.Parse()
			require.NoError(t, err)

			err = Validate(q)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "unknown field")
			assert.Contains(t, err.Error(), tc.field)
		})
	}
}

func TestValidate_InvalidOperator(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{"boolean with less than", "blocked < true"},
		{"boolean with contains", "ready ~ something"},
		{"enum with less than", "status < open"},
		{"enum with contains", "type ~ bug"},
		{"date with contains", "created ~ today"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			parser := NewParser(tc.query)
			q, err := parser.Parse()
			require.NoError(t, err)

			err = Validate(q)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "not valid")
		})
	}
}

func TestValidate_InvalidValue(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{"invalid type value", "type = invalid"},
		{"invalid status value", "status = pending"},
		{"boolean field with string", "blocked = yes"},
		{"priority field with string", "priority = high"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			parser := NewParser(tc.query)
			q, err := parser.Parse()
			require.NoError(t, err)

			err = Validate(q)
			require.Error(t, err)
		})
	}
}

func TestValidate_InvalidIn(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{"in with boolean field", "blocked in (true, false)"},
		{"in with date field", "created in (today, yesterday)"},
		{"in with invalid type values", "type in (invalid, unknown)"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			parser := NewParser(tc.query)
			q, err := parser.Parse()
			require.NoError(t, err)

			err = Validate(q)
			require.Error(t, err)
		})
	}
}

func TestValidate_InvalidOrderBy(t *testing.T) {
	parser := NewParser("type = bug order by unknown")
	q, err := parser.Parse()
	require.NoError(t, err)

	err = Validate(q)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown field in ORDER BY")
}

func TestValidate_BinaryExprWithInvalidChild(t *testing.T) {
	// Test that validation propagates through binary expressions
	parser := NewParser("type = bug and foo = bar")
	q, err := parser.Parse()
	require.NoError(t, err)

	err = Validate(q)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown field")
}

func TestValidate_NotExprWithInvalidChild(t *testing.T) {
	// Test that validation propagates through NOT expressions
	parser := NewParser("not foo = bar")
	q, err := parser.Parse()
	require.NoError(t, err)

	err = Validate(q)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown field")
}
