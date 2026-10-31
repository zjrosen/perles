package infrastructure

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	domain "github.com/zjrosen/perles/internal/beads/domain"
)

// showFixtureCurrent mirrors `bd show --json` on beads main (IssueDetails,
// count-only mode): issue_type, parent, dependencies with dependency_type,
// counts instead of dependents/comments.
const showFixtureCurrent = `[
  {
    "id": "PROJ-20",
    "title": "Fix login",
    "description": "Steps to reproduce",
    "notes": "Investigating",
    "status": "in_progress",
    "priority": 1,
    "issue_type": "bug",
    "assignee": "alice",
    "created_at": "2026-10-01T10:00:00Z",
    "created_by": "bob",
    "updated_at": "2026-10-02T11:30:00Z",
    "labels": ["backend", "urgent"],
    "dependencies": [
      {"id": "PROJ-1", "title": "Auth epic", "status": "open", "priority": 2, "issue_type": "epic",
       "created_at": "2026-09-01T00:00:00Z", "updated_at": "2026-09-01T00:00:00Z", "dependency_type": "parent-child"},
      {"id": "PROJ-5", "title": "Session store", "status": "open", "priority": 2, "issue_type": "task",
       "created_at": "2026-09-01T00:00:00Z", "updated_at": "2026-09-01T00:00:00Z", "dependency_type": "blocks"},
      {"id": "PROJ-6", "title": "Original report", "status": "closed", "priority": 3, "issue_type": "bug",
       "created_at": "2026-09-01T00:00:00Z", "updated_at": "2026-09-01T00:00:00Z", "dependency_type": "discovered-from"},
      {"id": "PROJ-7", "title": "Related", "status": "open", "priority": 3, "issue_type": "task",
       "created_at": "2026-09-01T00:00:00Z", "updated_at": "2026-09-01T00:00:00Z", "dependency_type": "related"}
    ],
    "parent": "PROJ-1",
    "dependent_count": 2,
    "dependency_count": 4,
    "comment_count": 3,
    "comments_omitted": true
  }
]`

func TestParseBDShow_CurrentShape(t *testing.T) {
	issues, err := parseBDShow(showFixtureCurrent)
	require.NoError(t, err)
	require.Len(t, issues, 1)

	issue := issues[0]
	require.Equal(t, "PROJ-20", issue.ID)
	require.Equal(t, "Fix login", issue.TitleText)
	require.Equal(t, "Steps to reproduce", issue.DescriptionText)
	require.Equal(t, "Investigating", issue.Notes)
	require.Equal(t, domain.StatusInProgress, issue.Status)
	require.Equal(t, domain.PriorityHigh, issue.Priority)
	require.Equal(t, domain.TypeBug, issue.Type, "type comes from issue_type")
	require.Equal(t, "alice", issue.Assignee)
	require.Equal(t, "bob", issue.CreatedBy)
	require.Equal(t, time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC), issue.CreatedAt.UTC())
	require.Equal(t, time.Date(2026, 10, 2, 11, 30, 0, 0, time.UTC), issue.UpdatedAt.UTC())
	require.Equal(t, []string{"backend", "urgent"}, issue.Labels)
	require.Equal(t, "PROJ-1", issue.ParentID, "parent comes from parent")
	require.Equal(t, []string{"PROJ-5"}, issue.BlockedBy)
	require.Equal(t, []string{"PROJ-6"}, issue.DiscoveredFrom)
	require.Empty(t, issue.Blocks, "dependents are not listed in count-only mode")
	require.Empty(t, issue.Children)
	require.Equal(t, 3, issue.CommentCount)
	require.True(t, issue.ClosedAt.IsZero())
}

// TestParseBDShow_DependentsAndClosedFields covers the bd v1.0-style output
// that still lists dependents, plus closed/pinned/defer fields.
func TestParseBDShow_DependentsAndClosedFields(t *testing.T) {
	out := `[{
	  "id": "PROJ-30", "title": "Epic", "status": "closed", "priority": 0, "issue_type": "epic",
	  "created_at": "2026-10-01T00:00:00Z", "updated_at": "2026-10-03T00:00:00Z",
	  "closed_at": "2026-10-03T00:00:00Z", "close_reason": "done",
	  "defer_until": "2026-11-01T09:00:00Z", "pinned": true, "mol_type": "swarm",
	  "dependencies": [{"id": "PROJ-29", "issue_type": "epic", "dependency_type": "parent-child"}],
	  "dependents": [
	    {"id": "PROJ-31", "issue_type": "task", "dependency_type": "parent-child"},
	    {"id": "PROJ-32", "issue_type": "task", "dependency_type": "parent-child"},
	    {"id": "PROJ-33", "issue_type": "task", "dependency_type": "blocks"},
	    {"id": "PROJ-34", "issue_type": "bug", "dependency_type": "discovered-from"}
	  ],
	  "comments": [{"id": "c1", "issue_id": "PROJ-30", "author": "alice", "text": "hi", "created_at": "2026-10-02T00:00:00Z"}]
	}]`

	issues, err := parseBDShow(out)
	require.NoError(t, err)
	require.Len(t, issues, 1)
	issue := issues[0]

	require.Equal(t, domain.TypeEpic, issue.Type)
	require.Equal(t, domain.StatusClosed, issue.Status)
	require.Equal(t, domain.PriorityCritical, issue.Priority)
	require.Equal(t, "PROJ-29", issue.ParentID, "parent falls back to the parent-child dependency")
	require.Equal(t, []string{"PROJ-31", "PROJ-32"}, issue.Children)
	require.Equal(t, []string{"PROJ-33"}, issue.Blocks)
	require.Equal(t, []string{"PROJ-34"}, issue.Discovered)
	require.Equal(t, time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC), issue.ClosedAt.UTC())
	require.Equal(t, time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC), issue.DeferUntil.UTC())
	require.Equal(t, "done", issue.CloseReason)
	require.NotNil(t, issue.Pinned)
	require.True(t, *issue.Pinned)
	require.Nil(t, issue.IsTemplate)
	require.Equal(t, "swarm", issue.MolType)
	require.Len(t, issue.Comments, 1)
	require.Equal(t, "c1", issue.Comments[0].ID)
	require.Equal(t, "alice", issue.Comments[0].Author)
	require.Equal(t, 1, issue.CommentCount)
}

// TestParseBDShow_LegacyKeys covers output that used the domain.Issue tags
// (type, parent_id, blocked_by, ...), so older consumers keep working.
func TestParseBDShow_LegacyKeys(t *testing.T) {
	out := `[{"id":"PROJ-40","title":"T","type":"feature","parent_id":"PROJ-39",
	  "blocked_by":["PROJ-38"],"blocks":["PROJ-41"],"children":["PROJ-42"],
	  "discovered_from":["PROJ-37"],"discovered":["PROJ-43"]}]`

	issues, err := parseBDShow(out)
	require.NoError(t, err)
	issue := issues[0]
	require.Equal(t, domain.TypeFeature, issue.Type)
	require.Equal(t, "PROJ-39", issue.ParentID)
	require.Equal(t, []string{"PROJ-38"}, issue.BlockedBy)
	require.Equal(t, []string{"PROJ-41"}, issue.Blocks)
	require.Equal(t, []string{"PROJ-42"}, issue.Children)
	require.Equal(t, []string{"PROJ-37"}, issue.DiscoveredFrom)
	require.Equal(t, []string{"PROJ-43"}, issue.Discovered)
}

// TestParseBDShow_RawDependencyEdges covers dependencies printed as raw
// edges {issue_id, depends_on_id, type} (the Issue.Dependencies shape).
func TestParseBDShow_RawDependencyEdges(t *testing.T) {
	out := `[{"id":"PROJ-50","title":"T","issue_type":"task",
	  "dependencies":[
	    {"id":"6f1c9a2e-uuid","issue_id":"PROJ-50","depends_on_id":"PROJ-51","type":"blocks"},
	    {"issue_id":"PROJ-50","depends_on_id":"PROJ-49","type":"parent-child"},
	    {"issue_id":"PROJ-50","depends_on_id":"PROJ-51","type":"blocks"}
	  ],
	  "dependents":[{"issue_id":"PROJ-52","depends_on_id":"PROJ-50","type":"blocks"}]}]`

	issues, err := parseBDShow(out)
	require.NoError(t, err)
	issue := issues[0]
	require.Equal(t, "PROJ-49", issue.ParentID)
	require.Equal(t, []string{"PROJ-51"}, issue.BlockedBy, "duplicates collapse; the edge row id is not a target")
	require.Equal(t, []string{"PROJ-52"}, issue.Blocks)
}

func TestParseBDShow_EnvelopeAndSingleObject(t *testing.T) {
	enveloped := `{"schema_version":1,"data":[{"id":"PROJ-60","title":"Wrapped","issue_type":"chore"}]}`
	issues, err := parseBDShow(enveloped)
	require.NoError(t, err)
	require.Len(t, issues, 1)
	require.Equal(t, "PROJ-60", issues[0].ID)
	require.Equal(t, domain.TypeChore, issues[0].Type)

	single := `{"id":"PROJ-61","title":"Bare object","issue_type":"task","schema_version":1}`
	issues, err = parseBDShow(single)
	require.NoError(t, err)
	require.Len(t, issues, 1)
	require.Equal(t, "PROJ-61", issues[0].ID)
}

func TestParseBDShow_LeadingNoise(t *testing.T) {
	out := "Warning: something unrelated\n[{\"id\":\"PROJ-70\",\"title\":\"T\",\"issue_type\":\"task\"}]\n"
	issues, err := parseBDShow(out)
	require.NoError(t, err)
	require.Equal(t, "PROJ-70", issues[0].ID)
}

func TestParseBDShow_Invalid(t *testing.T) {
	_, err := parseBDShow("")
	require.Error(t, err)
	_, err = parseBDShow("no json here")
	require.ErrorContains(t, err, "no JSON value")
	_, err = parseBDShow(`[{"id": 5}]`)
	require.Error(t, err)
}

func TestBDJSONPayload_Envelope(t *testing.T) {
	cases := map[string]struct {
		in   string
		want string
	}{
		"bare array":            {in: `[1,2]`, want: `[1,2]`},
		"bare object":           {in: `{"id":"x","schema_version":1}`, want: `{"id":"x","schema_version":1}`},
		"envelope object":       {in: `{"schema_version":1,"data":{"id":"x"}}`, want: `{"id":"x"}`},
		"envelope array":        {in: `{"schema_version":1,"data":[{"id":"x"}],"pagination":{"returned":1}}`, want: `[{"id":"x"}]`},
		"data without version":  {in: `{"data":{"id":"x"}}`, want: `{"data":{"id":"x"}}`},
		"indented with newline": {in: "{\n  \"schema_version\": 1,\n  \"data\": []\n}\n", want: `[]`},
		"null":                  {in: "null\n", want: `null`},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := bdJSONPayload(tc.in)
			require.NoError(t, err)
			require.JSONEq(t, tc.want, string(got))
		})
	}
}

func TestParseBDCreate(t *testing.T) {
	for name, out := range map[string]string{
		"bare":     `{"id":"PROJ-80","title":"New","status":"open","schema_version":1}`,
		"envelope": `{"schema_version":1,"data":{"id":"PROJ-80","title":"New"}}`,
		"array":    `[{"id":"PROJ-80","title":"New"}]`,
	} {
		t.Run(name, func(t *testing.T) {
			result, err := parseBDCreate(out)
			require.NoError(t, err)
			require.Equal(t, domain.CreateResult{ID: "PROJ-80", Title: "New"}, result)
		})
	}
}
