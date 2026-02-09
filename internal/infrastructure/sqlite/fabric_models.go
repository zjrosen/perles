package sqlite

import (
	"encoding/json"
	"time"

	domain "github.com/zjrosen/perles/internal/orchestration/fabric/domain"
)

// fabricThreadModel represents the database row for the fabric_threads table.
// Fields map directly to SQL columns with Unix timestamps for time values
// and JSON-encoded TEXT columns for slice/map fields.
type fabricThreadModel struct {
	SessionID string
	ID        string
	Type      string
	CreatedAt int64
	CreatedBy string

	Content *string // nullable
	Kind    *string // nullable

	Slug    *string // nullable
	Title   *string // nullable
	Purpose *string // nullable

	Name       *string // nullable
	MediaType  *string // nullable
	SizeBytes  *int64  // nullable
	StorageURI *string // nullable
	Sha256     *string // nullable

	MentionsJSON     *string // nullable, JSON encoded []string
	ParticipantsJSON *string // nullable, JSON encoded []string
	MetaJSON         *string // nullable, JSON encoded map[string]string

	Seq        int64
	ArchivedAt *int64 // nullable, Unix timestamp
}

// toFabricThreadModel converts a domain Thread to a database row model.
func toFabricThreadModel(sessionID string, t *domain.Thread) *fabricThreadModel {
	m := &fabricThreadModel{
		SessionID: sessionID,
		ID:        t.ID,
		Type:      string(t.Type),
		CreatedAt: t.CreatedAt.Unix(),
		CreatedBy: t.CreatedBy,
		Seq:       t.Seq,
	}

	if t.Content != "" {
		m.Content = &t.Content
	}
	if t.Kind != "" {
		m.Kind = &t.Kind
	}
	if t.Slug != "" {
		m.Slug = &t.Slug
	}
	if t.Title != "" {
		m.Title = &t.Title
	}
	if t.Purpose != "" {
		m.Purpose = &t.Purpose
	}
	if t.Name != "" {
		m.Name = &t.Name
	}
	if t.MediaType != "" {
		m.MediaType = &t.MediaType
	}
	if t.SizeBytes != 0 {
		m.SizeBytes = &t.SizeBytes
	}
	if t.StorageURI != "" {
		m.StorageURI = &t.StorageURI
	}
	if t.Sha256 != "" {
		m.Sha256 = &t.Sha256
	}

	if len(t.Mentions) > 0 {
		data, err := json.Marshal(t.Mentions)
		if err == nil {
			s := string(data)
			m.MentionsJSON = &s
		}
	}
	if len(t.Participants) > 0 {
		data, err := json.Marshal(t.Participants)
		if err == nil {
			s := string(data)
			m.ParticipantsJSON = &s
		}
	}
	if len(t.Meta) > 0 {
		data, err := json.Marshal(t.Meta)
		if err == nil {
			s := string(data)
			m.MetaJSON = &s
		}
	}

	if t.ArchivedAt != nil {
		archivedAt := t.ArchivedAt.Unix()
		m.ArchivedAt = &archivedAt
	}

	return m
}

// toDomain converts a database row model back to a domain Thread.
func (m *fabricThreadModel) toDomain() *domain.Thread {
	t := &domain.Thread{
		ID:        m.ID,
		Type:      domain.ThreadType(m.Type),
		CreatedAt: time.Unix(m.CreatedAt, 0),
		CreatedBy: m.CreatedBy,
		Seq:       m.Seq,
	}

	if m.Content != nil {
		t.Content = *m.Content
	}
	if m.Kind != nil {
		t.Kind = *m.Kind
	}
	if m.Slug != nil {
		t.Slug = *m.Slug
	}
	if m.Title != nil {
		t.Title = *m.Title
	}
	if m.Purpose != nil {
		t.Purpose = *m.Purpose
	}
	if m.Name != nil {
		t.Name = *m.Name
	}
	if m.MediaType != nil {
		t.MediaType = *m.MediaType
	}
	if m.SizeBytes != nil {
		t.SizeBytes = *m.SizeBytes
	}
	if m.StorageURI != nil {
		t.StorageURI = *m.StorageURI
	}
	if m.Sha256 != nil {
		t.Sha256 = *m.Sha256
	}

	// JSON fields: always return empty slices/maps, not nil
	t.Mentions = []string{}
	if m.MentionsJSON != nil {
		_ = json.Unmarshal([]byte(*m.MentionsJSON), &t.Mentions)
		if t.Mentions == nil {
			t.Mentions = []string{}
		}
	}

	t.Participants = []string{}
	if m.ParticipantsJSON != nil {
		_ = json.Unmarshal([]byte(*m.ParticipantsJSON), &t.Participants)
		if t.Participants == nil {
			t.Participants = []string{}
		}
	}

	t.Meta = map[string]string{}
	if m.MetaJSON != nil {
		_ = json.Unmarshal([]byte(*m.MetaJSON), &t.Meta)
		if t.Meta == nil {
			t.Meta = map[string]string{}
		}
	}

	if m.ArchivedAt != nil {
		archivedAt := time.Unix(*m.ArchivedAt, 0)
		t.ArchivedAt = &archivedAt
	}

	return t
}

// fabricDependencyModel represents the database row for the fabric_dependencies table.
type fabricDependencyModel struct {
	SessionID   string
	ThreadID    string
	DependsOnID string
	Relation    string
	CreatedAt   int64
}

// toFabricDependencyModel converts a domain Dependency to a database row model.
func toFabricDependencyModel(sessionID string, d domain.Dependency) *fabricDependencyModel {
	return &fabricDependencyModel{
		SessionID:   sessionID,
		ThreadID:    d.ThreadID,
		DependsOnID: d.DependsOnID,
		Relation:    string(d.Relation),
		CreatedAt:   d.CreatedAt.Unix(),
	}
}

// toDomain converts a database row model back to a domain Dependency.
func (m *fabricDependencyModel) toDomain() domain.Dependency {
	return domain.Dependency{
		ThreadID:    m.ThreadID,
		DependsOnID: m.DependsOnID,
		Relation:    domain.RelationType(m.Relation),
		CreatedAt:   time.Unix(m.CreatedAt, 0),
	}
}
