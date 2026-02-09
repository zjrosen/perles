package sqlite

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	domain "github.com/zjrosen/perles/internal/orchestration/fabric/domain"
)

func TestFabricThreadModel_RoundTrip_AllFields(t *testing.T) {
	archivedAt := time.Date(2026, 2, 9, 12, 0, 0, 0, time.UTC)
	original := &domain.Thread{
		ID:           "msg-001",
		Type:         domain.ThreadMessage,
		CreatedAt:    time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC),
		CreatedBy:    "worker-1",
		Content:      "Hello, this is a test message",
		Kind:         "info",
		Slug:         "general",
		Title:        "Test Thread",
		Purpose:      "For testing round-trips",
		Name:         "test-artifact.txt",
		MediaType:    "text/plain",
		SizeBytes:    1024,
		StorageURI:   "file:///tmp/test.txt",
		Sha256:       "abc123def456",
		Mentions:     []string{"worker-2", "coordinator"},
		Participants: []string{"worker-1", "worker-2", "coordinator"},
		Meta:         map[string]string{"task_id": "bd-42", "priority": "high"},
		Seq:          7,
		ArchivedAt:   &archivedAt,
	}

	sessionID := "session-abc"
	model := toFabricThreadModel(sessionID, original)

	// Verify model fields
	require.Equal(t, sessionID, model.SessionID)
	require.Equal(t, original.ID, model.ID)
	require.Equal(t, string(original.Type), model.Type)
	require.Equal(t, original.CreatedAt.Unix(), model.CreatedAt)
	require.Equal(t, original.CreatedBy, model.CreatedBy)
	require.Equal(t, original.Seq, model.Seq)
	require.NotNil(t, model.ArchivedAt)
	require.Equal(t, archivedAt.Unix(), *model.ArchivedAt)

	// Round-trip back to domain
	result := model.toDomain()

	require.Equal(t, original.ID, result.ID)
	require.Equal(t, original.Type, result.Type)
	require.Equal(t, original.CreatedAt.Unix(), result.CreatedAt.Unix())
	require.Equal(t, original.CreatedBy, result.CreatedBy)
	require.Equal(t, original.Content, result.Content)
	require.Equal(t, original.Kind, result.Kind)
	require.Equal(t, original.Slug, result.Slug)
	require.Equal(t, original.Title, result.Title)
	require.Equal(t, original.Purpose, result.Purpose)
	require.Equal(t, original.Name, result.Name)
	require.Equal(t, original.MediaType, result.MediaType)
	require.Equal(t, original.SizeBytes, result.SizeBytes)
	require.Equal(t, original.StorageURI, result.StorageURI)
	require.Equal(t, original.Sha256, result.Sha256)
	require.Equal(t, original.Mentions, result.Mentions)
	require.Equal(t, original.Participants, result.Participants)
	require.Equal(t, original.Meta, result.Meta)
	require.Equal(t, original.Seq, result.Seq)
	require.NotNil(t, result.ArchivedAt)
	require.Equal(t, archivedAt.Unix(), result.ArchivedAt.Unix())
}

func TestFabricThreadModel_RoundTrip_MinimalFields(t *testing.T) {
	original := &domain.Thread{
		ID:        "msg-002",
		Type:      domain.ThreadChannel,
		CreatedAt: time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC),
		CreatedBy: "system",
		Seq:       1,
	}

	model := toFabricThreadModel("session-1", original)
	result := model.toDomain()

	require.Equal(t, original.ID, result.ID)
	require.Equal(t, original.Type, result.Type)
	require.Equal(t, original.CreatedAt.Unix(), result.CreatedAt.Unix())
	require.Equal(t, original.CreatedBy, result.CreatedBy)
	require.Equal(t, original.Seq, result.Seq)

	// Empty string fields should remain empty
	require.Empty(t, result.Content)
	require.Empty(t, result.Kind)
	require.Empty(t, result.Slug)
	require.Empty(t, result.Title)
	require.Empty(t, result.Purpose)
	require.Empty(t, result.Name)
	require.Empty(t, result.MediaType)
	require.Zero(t, result.SizeBytes)
	require.Empty(t, result.StorageURI)
	require.Empty(t, result.Sha256)

	// JSON fields should be empty slices/maps, not nil
	require.NotNil(t, result.Mentions)
	require.Empty(t, result.Mentions)
	require.NotNil(t, result.Participants)
	require.Empty(t, result.Participants)
	require.NotNil(t, result.Meta)
	require.Empty(t, result.Meta)

	require.Nil(t, result.ArchivedAt)
}

func TestFabricThreadModel_NilMentionsParticipantsMeta(t *testing.T) {
	original := &domain.Thread{
		ID:           "msg-003",
		Type:         domain.ThreadMessage,
		CreatedAt:    time.Now(),
		CreatedBy:    "worker-1",
		Mentions:     nil,
		Participants: nil,
		Meta:         nil,
	}

	model := toFabricThreadModel("session-1", original)

	// nil slices/maps should produce nil JSON fields
	require.Nil(t, model.MentionsJSON)
	require.Nil(t, model.ParticipantsJSON)
	require.Nil(t, model.MetaJSON)

	// Round-trip should return empty slices/maps, not nil
	result := model.toDomain()
	require.NotNil(t, result.Mentions)
	require.Empty(t, result.Mentions)
	require.NotNil(t, result.Participants)
	require.Empty(t, result.Participants)
	require.NotNil(t, result.Meta)
	require.Empty(t, result.Meta)
}

func TestFabricThreadModel_EmptySlicesAndMaps(t *testing.T) {
	original := &domain.Thread{
		ID:           "msg-004",
		Type:         domain.ThreadMessage,
		CreatedAt:    time.Now(),
		CreatedBy:    "worker-1",
		Mentions:     []string{},
		Participants: []string{},
		Meta:         map[string]string{},
	}

	model := toFabricThreadModel("session-1", original)

	// Empty slices/maps should produce nil JSON (len==0 check)
	require.Nil(t, model.MentionsJSON)
	require.Nil(t, model.ParticipantsJSON)
	require.Nil(t, model.MetaJSON)

	result := model.toDomain()
	require.NotNil(t, result.Mentions)
	require.Empty(t, result.Mentions)
	require.NotNil(t, result.Participants)
	require.Empty(t, result.Participants)
	require.NotNil(t, result.Meta)
	require.Empty(t, result.Meta)
}

func TestFabricThreadModel_ArchivedAt_NilAndNonNil(t *testing.T) {
	t.Run("nil ArchivedAt", func(t *testing.T) {
		original := &domain.Thread{
			ID:        "msg-005",
			Type:      domain.ThreadMessage,
			CreatedAt: time.Now(),
			CreatedBy: "worker-1",
		}

		model := toFabricThreadModel("session-1", original)
		require.Nil(t, model.ArchivedAt)

		result := model.toDomain()
		require.Nil(t, result.ArchivedAt)
	})

	t.Run("non-nil ArchivedAt", func(t *testing.T) {
		archivedAt := time.Date(2026, 6, 15, 14, 30, 0, 0, time.UTC)
		original := &domain.Thread{
			ID:         "msg-006",
			Type:       domain.ThreadMessage,
			CreatedAt:  time.Now(),
			CreatedBy:  "worker-1",
			ArchivedAt: &archivedAt,
		}

		model := toFabricThreadModel("session-1", original)
		require.NotNil(t, model.ArchivedAt)
		require.Equal(t, archivedAt.Unix(), *model.ArchivedAt)

		result := model.toDomain()
		require.NotNil(t, result.ArchivedAt)
		require.Equal(t, archivedAt.Unix(), result.ArchivedAt.Unix())
	})
}

func TestFabricThreadModel_ComplexMeta(t *testing.T) {
	original := &domain.Thread{
		ID:        "msg-007",
		Type:      domain.ThreadMessage,
		CreatedAt: time.Now(),
		CreatedBy: "worker-1",
		Meta: map[string]string{
			"task_id":     "bd-42",
			"priority":    "high",
			"description": "A complex description with special chars: <>&\"'",
			"empty_value": "",
			"unicode_key": "value with émøjï",
		},
	}

	model := toFabricThreadModel("session-1", original)
	require.NotNil(t, model.MetaJSON)

	result := model.toDomain()
	require.Equal(t, original.Meta, result.Meta)
}

func TestFabricThreadModel_ZeroValueTime(t *testing.T) {
	original := &domain.Thread{
		ID:        "msg-008",
		Type:      domain.ThreadMessage,
		CreatedAt: time.Time{}, // zero value
		CreatedBy: "worker-1",
	}

	model := toFabricThreadModel("session-1", original)
	result := model.toDomain()

	// Zero time round-trips through Unix timestamps
	// time.Time{}.Unix() gives a negative number, but the round-trip should be consistent
	require.Equal(t, original.CreatedAt.Unix(), result.CreatedAt.Unix())
}

func TestFabricThreadModel_UnicodeContent(t *testing.T) {
	original := &domain.Thread{
		ID:        "msg-009",
		Type:      domain.ThreadMessage,
		CreatedAt: time.Now(),
		CreatedBy: "worker-1",
		Content:   "Hello 🌍! Élève 你好 🚀🌟",
		Title:     "📝 Unicode Title ✅",
	}

	model := toFabricThreadModel("session-1", original)
	result := model.toDomain()

	require.Equal(t, original.Content, result.Content)
	require.Equal(t, original.Title, result.Title)
}

func TestFabricThreadModel_VeryLongContent(t *testing.T) {
	longContent := ""
	for i := 0; i < 10000; i++ {
		longContent += "abcdefghij"
	}

	original := &domain.Thread{
		ID:        "msg-010",
		Type:      domain.ThreadMessage,
		CreatedAt: time.Now(),
		CreatedBy: "worker-1",
		Content:   longContent,
	}

	model := toFabricThreadModel("session-1", original)
	result := model.toDomain()

	require.Equal(t, original.Content, result.Content)
	require.Len(t, result.Content, 100000)
}

func TestFabricThreadModel_EmptyStringContent(t *testing.T) {
	original := &domain.Thread{
		ID:        "msg-011",
		Type:      domain.ThreadMessage,
		CreatedAt: time.Now(),
		CreatedBy: "worker-1",
		Content:   "",
	}

	model := toFabricThreadModel("session-1", original)
	// Empty string should produce nil (not stored)
	require.Nil(t, model.Content)

	result := model.toDomain()
	require.Empty(t, result.Content)
}

func TestFabricThreadModel_AllThreadTypes(t *testing.T) {
	types := []domain.ThreadType{
		domain.ThreadChannel,
		domain.ThreadMessage,
		domain.ThreadArtifact,
	}

	for _, tt := range types {
		t.Run(string(tt), func(t *testing.T) {
			original := &domain.Thread{
				ID:        "thread-" + string(tt),
				Type:      tt,
				CreatedAt: time.Now(),
				CreatedBy: "system",
			}

			model := toFabricThreadModel("session-1", original)
			require.Equal(t, string(tt), model.Type)

			result := model.toDomain()
			require.Equal(t, tt, result.Type)
		})
	}
}

func TestFabricDependencyModel_RoundTrip(t *testing.T) {
	original := domain.Dependency{
		ThreadID:    "msg-001",
		DependsOnID: "channel-general",
		Relation:    domain.RelationChildOf,
		CreatedAt:   time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC),
	}

	sessionID := "session-abc"
	model := toFabricDependencyModel(sessionID, original)

	// Verify model fields
	require.Equal(t, sessionID, model.SessionID)
	require.Equal(t, original.ThreadID, model.ThreadID)
	require.Equal(t, original.DependsOnID, model.DependsOnID)
	require.Equal(t, string(original.Relation), model.Relation)
	require.Equal(t, original.CreatedAt.Unix(), model.CreatedAt)

	// Round-trip
	result := model.toDomain()

	require.Equal(t, original.ThreadID, result.ThreadID)
	require.Equal(t, original.DependsOnID, result.DependsOnID)
	require.Equal(t, original.Relation, result.Relation)
	require.Equal(t, original.CreatedAt.Unix(), result.CreatedAt.Unix())
}

func TestFabricDependencyModel_AllRelationTypes(t *testing.T) {
	relations := []domain.RelationType{
		domain.RelationChildOf,
		domain.RelationReplyTo,
		domain.RelationReferences,
	}

	for _, rel := range relations {
		t.Run(string(rel), func(t *testing.T) {
			original := domain.Dependency{
				ThreadID:    "msg-001",
				DependsOnID: "msg-002",
				Relation:    rel,
				CreatedAt:   time.Now(),
			}

			model := toFabricDependencyModel("session-1", original)
			require.Equal(t, string(rel), model.Relation)

			result := model.toDomain()
			require.Equal(t, rel, result.Relation)
		})
	}
}

func TestFabricDependencyModel_ZeroValueTime(t *testing.T) {
	original := domain.Dependency{
		ThreadID:    "msg-001",
		DependsOnID: "msg-002",
		Relation:    domain.RelationReplyTo,
		CreatedAt:   time.Time{},
	}

	model := toFabricDependencyModel("session-1", original)
	result := model.toDomain()

	require.Equal(t, original.CreatedAt.Unix(), result.CreatedAt.Unix())
}

func TestFabricThreadModel_MentionsWithSingleEntry(t *testing.T) {
	original := &domain.Thread{
		ID:        "msg-012",
		Type:      domain.ThreadMessage,
		CreatedAt: time.Now(),
		CreatedBy: "worker-1",
		Mentions:  []string{"here"},
	}

	model := toFabricThreadModel("session-1", original)
	require.NotNil(t, model.MentionsJSON)
	require.Equal(t, `["here"]`, *model.MentionsJSON)

	result := model.toDomain()
	require.Equal(t, []string{"here"}, result.Mentions)
}

func TestFabricThreadModel_SessionIDPreserved(t *testing.T) {
	thread := &domain.Thread{
		ID:        "msg-001",
		Type:      domain.ThreadMessage,
		CreatedAt: time.Now(),
		CreatedBy: "worker-1",
	}

	model := toFabricThreadModel("session-xyz-123", thread)
	require.Equal(t, "session-xyz-123", model.SessionID)
}

func TestFabricDependencyModel_SessionIDPreserved(t *testing.T) {
	dep := domain.Dependency{
		ThreadID:    "msg-001",
		DependsOnID: "msg-002",
		Relation:    domain.RelationChildOf,
		CreatedAt:   time.Now(),
	}

	model := toFabricDependencyModel("session-xyz-123", dep)
	require.Equal(t, "session-xyz-123", model.SessionID)
}
