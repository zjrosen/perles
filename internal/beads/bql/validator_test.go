package bql

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidate_ValidQueries(t *testing.T) {
	validQueries := []string{
		"type = bug",
		"type = feature",
		"type = task",
		"type = epic",
		"type = chore",
		"type = milestone",
		"type = story",
		"type = spike",
		"type = decision",
		"type = message",
		"type = molecule",
		"type = gate",
		"type = event",
		"type = convoy",
		"type = my-custom-type",
		"type in (gate, decision)",
		"status = pinned",
		"status = hooked",
		"owner = alice",
		"owner ~ ali",
		"close_reason ~ duplicate",
		"acceptance_criteria ~ tests",
		"started > -7d",
		"closed >= -30d",
		"due < now",
		"type = bug order by due asc, started desc, closed",
		"priority = P0",
		"priority < P2",
		"priority >= P1",
		"priority = 0",
		"priority = 2",
		"priority < 3",
		"priority >= 1",
		"status = open",
		"status = in_progress",
		"status = closed",
		"status = awaiting_review",
		"status = in_review",
		"status in (open, awaiting_review)",
		"blocked = true",
		"blocked = false",
		"ready = true",
		"pinned = true",
		"pinned = false",
		"is_template = true",
		"is_template = false",
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
			require.NoError(t, err, "query should be valid: %s", query)
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
			require.Contains(t, err.Error(), "unknown field")
			require.Contains(t, err.Error(), tc.field)
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
			require.Contains(t, err.Error(), "not valid")
		})
	}
}

func TestValidate_InvalidValue(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{"type with number", "type = 5"},
		{"status with boolean", "status = true"},
		{"type with nil", "type = nil"},
		{"boolean field with string", "blocked = yes"},
		{"priority field with string", "priority = high"},
		{"priority integer too high", "priority = 5"},
		{"priority integer negative", "priority = -1"},
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
		{"in with numeric status values", "status in (1, 2)"},
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
	require.Contains(t, err.Error(), "unknown field in ORDER BY")
}

func TestValidate_BinaryExprWithInvalidChild(t *testing.T) {
	// Test that validation propagates through binary expressions
	parser := NewParser("type = bug and foo = bar")
	q, err := parser.Parse()
	require.NoError(t, err)

	err = Validate(q)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown field")
}

func TestValidate_NotExprWithInvalidChild(t *testing.T) {
	// Test that validation propagates through NOT expressions
	parser := NewParser("not foo = bar")
	q, err := parser.Parse()
	require.NoError(t, err)

	err = Validate(q)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown field")
}

func TestValidate_MetadataField(t *testing.T) {
	t.Run("valid metadata equals", func(t *testing.T) {
		parser := NewParser(`metadata.team = "backend"`)
		q, err := parser.Parse()
		require.NoError(t, err)
		err = Validate(q)
		require.NoError(t, err)
	})

	t.Run("valid metadata not equals", func(t *testing.T) {
		parser := NewParser(`metadata.team != "backend"`)
		q, err := parser.Parse()
		require.NoError(t, err)
		err = Validate(q)
		require.NoError(t, err)
	})

	t.Run("valid metadata contains", func(t *testing.T) {
		parser := NewParser(`metadata.team ~ auth`)
		q, err := parser.Parse()
		require.NoError(t, err)
		err = Validate(q)
		require.NoError(t, err)
	})

	t.Run("valid metadata key exists", func(t *testing.T) {
		parser := NewParser("metadata.team = nil")
		q, err := parser.Parse()
		require.NoError(t, err)
		err = Validate(q)
		require.NoError(t, err)
	})

	t.Run("valid metadata key not exists", func(t *testing.T) {
		parser := NewParser("metadata.team != nil")
		q, err := parser.Parse()
		require.NoError(t, err)
		err = Validate(q)
		require.NoError(t, err)
	})

	t.Run("valid nested metadata key", func(t *testing.T) {
		parser := NewParser(`metadata.jira.sprint = "Q1"`)
		q, err := parser.Parse()
		require.NoError(t, err)
		err = Validate(q)
		require.NoError(t, err)
	})

	t.Run("valid metadata combined with other fields", func(t *testing.T) {
		parser := NewParser(`type = task and metadata.team = "backend"`)
		q, err := parser.Parse()
		require.NoError(t, err)
		err = Validate(q)
		require.NoError(t, err)
	})

	t.Run("invalid metadata operator", func(t *testing.T) {
		parser := NewParser(`metadata.team < "value"`)
		q, err := parser.Parse()
		require.NoError(t, err)
		err = Validate(q)
		require.Error(t, err)
		require.Contains(t, err.Error(), "not valid for metadata field")
	})

	t.Run("invalid metadata value type", func(t *testing.T) {
		parser := NewParser("metadata.team = 123")
		q, err := parser.Parse()
		require.NoError(t, err)
		err = Validate(q)
		require.Error(t, err)
		require.Contains(t, err.Error(), "requires a string or nil value")
	})

	t.Run("invalid metadata key name", func(t *testing.T) {
		parser := NewParser(`metadata.123-invalid = "value"`)
		q, err := parser.Parse()
		require.NoError(t, err)
		err = Validate(q)
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid metadata key")
	})
}

func TestValidate_KnownValuesAndCustomNames(t *testing.T) {
	for _, v := range KnownTypeValues {
		q, err := NewParser("type = " + v).Parse()
		require.NoError(t, err)
		require.NoError(t, Validate(q), "known type %q", v)
	}
	for _, v := range KnownStatusValues {
		q, err := NewParser("status = " + v).Parse()
		require.NoError(t, err)
		require.NoError(t, Validate(q), "known status %q", v)
	}
	require.Subset(t, KnownTypeValues, []string{
		"bug", "feature", "task", "epic", "chore", "decision", "message",
		"molecule", "gate", "spike", "story", "milestone", "event",
	})
	require.Subset(t, KnownStatusValues, []string{"pinned", "hooked"})
}

func TestValidate_MetadataKeysFollowBd(t *testing.T) {
	for _, key := range []string{"team", "_private", "jira.sprint", "jira/sprint", "gc.routed_to", "a1/b2.c3"} {
		q, err := NewParser("metadata." + key + ` = "x"`).Parse()
		require.NoError(t, err, key)
		require.NoError(t, Validate(q), key)
	}
	for _, key := range []string{"1team", "story-points", "a:b", "/lead", ".lead"} {
		q, err := NewParser("metadata." + key + ` = "x"`).Parse()
		require.NoError(t, err, key)
		require.ErrorContains(t, Validate(q), "invalid metadata key", key)
	}
}
