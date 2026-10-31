package infrastructure

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	domain "github.com/zjrosen/perles/internal/beads/domain"
)

// bdJSONPayload extracts the JSON value bd printed on stdout. It tolerates
// text before the value (a line-leading '[' or '{' starts it) and unwraps
// the BD_JSON_ENVELOPE shape {"schema_version":N,"data":X} to X, so both the
// bare and the enveloped output decode the same way.
func bdJSONPayload(output string) (json.RawMessage, error) {
	s := strings.TrimSpace(output)
	if s == "" {
		return nil, errors.New("empty output")
	}
	if s == "null" {
		// An empty (nil) result list, e.g. an issue with no comments.
		return json.RawMessage(s), nil
	}
	start := jsonValueStart(s)
	if start < 0 {
		return nil, fmt.Errorf("no JSON value in output %q", truncateRunes(s, 80))
	}
	dec := json.NewDecoder(strings.NewReader(s[start:]))
	var raw json.RawMessage
	if err := dec.Decode(&raw); err != nil {
		return nil, err
	}
	return unwrapBDEnvelope(raw), nil
}

// jsonValueStart returns the offset of the first line in s that starts
// (after indentation) with '[' or '{', or -1.
func jsonValueStart(s string) int {
	offset := 0
	for {
		line, rest, found := strings.Cut(s[offset:], "\n")
		trimmed := strings.TrimLeft(line, " \t\r")
		if strings.HasPrefix(trimmed, "[") || strings.HasPrefix(trimmed, "{") {
			return offset + len(line) - len(trimmed)
		}
		if !found {
			return -1
		}
		offset = len(s) - len(rest)
	}
}

// unwrapBDEnvelope returns the data member of a {"schema_version":N,"data":X}
// envelope, or raw unchanged. A bare object carries schema_version too (bd
// adds it to every object), but never a data member.
func unwrapBDEnvelope(raw json.RawMessage) json.RawMessage {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return raw
	}
	var env map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &env); err != nil {
		return raw
	}
	data, hasData := env["data"]
	_, hasVersion := env["schema_version"]
	if hasData && hasVersion {
		return data
	}
	return raw
}

// decodeBDList decodes a payload holding either an array of T or a single T.
func decodeBDList[T any](payload json.RawMessage) ([]T, error) {
	trimmed := bytes.TrimSpace(payload)
	if len(trimmed) > 0 && trimmed[0] == '{' {
		var one T
		if err := json.Unmarshal(trimmed, &one); err != nil {
			return nil, err
		}
		return []T{one}, nil
	}
	var many []T
	if err := json.Unmarshal(trimmed, &many); err != nil {
		return nil, err
	}
	return many, nil
}

// bdFlexibleID decodes an id that bd prints as a string (current) or a
// number (comment ids before beads v1.0).
type bdFlexibleID string

func (id *bdFlexibleID) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		*id = bdFlexibleID(s)
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(data, &n); err != nil {
		return fmt.Errorf("id is neither a string nor a number: %s", data)
	}
	*id = bdFlexibleID(n.String())
	return nil
}

// bdComment is a comment as `bd comments --json` and `bd show --json` print it.
type bdComment struct {
	ID        bdFlexibleID `json:"id"`
	Author    string       `json:"author"`
	Text      string       `json:"text"`
	CreatedAt time.Time    `json:"created_at"`
}

func (c bdComment) toDomain() domain.Comment {
	return domain.Comment{ID: string(c.ID), Author: c.Author, Text: c.Text, CreatedAt: c.CreatedAt}
}

// bdCreateResult is the issue `bd create --json` prints.
type bdCreateResult struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// bdShowDependency is one entry of bd show's dependencies/dependents: an
// issue with its dependency_type (IssueWithDependencyMetadata), or a raw
// dependency edge {issue_id, depends_on_id, type}.
type bdShowDependency struct {
	ID             string `json:"id"`
	DependencyType string `json:"dependency_type"`
	IssueID        string `json:"issue_id"`
	DependsOnID    string `json:"depends_on_id"`
	Type           string `json:"type"`
}

func (d bdShowDependency) depType() string {
	if d.DependencyType != "" {
		return d.DependencyType
	}
	return d.Type
}

// target is the issue a dependency of this issue points at.
func (d bdShowDependency) target() string {
	if d.DependsOnID != "" {
		return d.DependsOnID
	}
	return d.ID
}

// source is the issue that depends on this issue.
func (d bdShowDependency) source() string {
	if d.DependsOnID != "" && d.IssueID != "" {
		return d.IssueID
	}
	return d.ID
}

// bdShowIssue is one element of `bd show --json` (beads IssueDetails). It
// differs from domain.Issue's tags: the type is issue_type, the parent is
// parent, and relationships come as dependencies/dependents entries rather
// than blocked_by/blocks/children. The older keys are still read so a bd
// that printed them keeps working.
type bdShowIssue struct {
	ID                 string             `json:"id"`
	Title              string             `json:"title"`
	Description        string             `json:"description"`
	Design             string             `json:"design"`
	AcceptanceCriteria string             `json:"acceptance_criteria"`
	Notes              string             `json:"notes"`
	Status             string             `json:"status"`
	Priority           int                `json:"priority"`
	IssueType          string             `json:"issue_type"`
	Type               string             `json:"type"`
	Assignee           string             `json:"assignee"`
	Sender             string             `json:"sender"`
	Ephemeral          bool               `json:"ephemeral"`
	Pinned             *bool              `json:"pinned"`
	IsTemplate         *bool              `json:"is_template"`
	Labels             []string           `json:"labels"`
	CreatedAt          time.Time          `json:"created_at"`
	CreatedBy          string             `json:"created_by"`
	UpdatedAt          time.Time          `json:"updated_at"`
	ClosedAt           *time.Time         `json:"closed_at"`
	DeferUntil         *time.Time         `json:"defer_until"`
	CloseReason        string             `json:"close_reason"`
	MolType            string             `json:"mol_type"`
	Parent             *string            `json:"parent"`
	ParentID           string             `json:"parent_id"`
	Dependencies       []bdShowDependency `json:"dependencies"`
	Dependents         []bdShowDependency `json:"dependents"`
	Comments           []bdComment        `json:"comments"`
	CommentCount       *int               `json:"comment_count"`
	LegacyBlockedBy    []string           `json:"blocked_by"`
	LegacyBlocks       []string           `json:"blocks"`
	LegacyChildren     []string           `json:"children"`
	LegacyDiscoveredFr []string           `json:"discovered_from"`
	LegacyDiscovered   []string           `json:"discovered"`
}

// toDomain maps bd's show shape onto domain.Issue.
func (b bdShowIssue) toDomain() domain.Issue {
	issue := domain.Issue{
		ID:                 b.ID,
		TitleText:          b.Title,
		DescriptionText:    b.Description,
		Design:             b.Design,
		AcceptanceCriteria: b.AcceptanceCriteria,
		Notes:              b.Notes,
		Status:             domain.Status(b.Status),
		Priority:           domain.Priority(b.Priority),
		Type:               domain.IssueType(firstNonEmpty(b.IssueType, b.Type)),
		Assignee:           b.Assignee,
		Sender:             b.Sender,
		Ephemeral:          b.Ephemeral,
		Pinned:             b.Pinned,
		IsTemplate:         b.IsTemplate,
		Labels:             b.Labels,
		CreatedAt:          b.CreatedAt,
		CreatedBy:          b.CreatedBy,
		UpdatedAt:          b.UpdatedAt,
		CloseReason:        b.CloseReason,
		MolType:            b.MolType,
		BlockedBy:          b.LegacyBlockedBy,
		Blocks:             b.LegacyBlocks,
		Children:           b.LegacyChildren,
		DiscoveredFrom:     b.LegacyDiscoveredFr,
		Discovered:         b.LegacyDiscovered,
		ParentID:           b.ParentID,
	}
	if b.Parent != nil && *b.Parent != "" {
		issue.ParentID = *b.Parent
	}
	if b.ClosedAt != nil {
		issue.ClosedAt = *b.ClosedAt
	}
	if b.DeferUntil != nil {
		issue.DeferUntil = *b.DeferUntil
	}

	for _, d := range b.Dependencies {
		target := d.target()
		if target == "" {
			continue
		}
		switch d.depType() {
		case "parent-child":
			if issue.ParentID == "" {
				issue.ParentID = target
			}
		case "blocks":
			issue.BlockedBy = appendUnique(issue.BlockedBy, target)
		case "discovered-from":
			issue.DiscoveredFrom = appendUnique(issue.DiscoveredFrom, target)
		}
	}
	for _, d := range b.Dependents {
		source := d.source()
		if source == "" {
			continue
		}
		switch d.depType() {
		case "parent-child":
			issue.Children = appendUnique(issue.Children, source)
		case "blocks":
			issue.Blocks = appendUnique(issue.Blocks, source)
		case "discovered-from":
			issue.Discovered = appendUnique(issue.Discovered, source)
		}
	}

	if len(b.Comments) > 0 {
		issue.Comments = make([]domain.Comment, len(b.Comments))
		for i, c := range b.Comments {
			issue.Comments[i] = c.toDomain()
		}
	}
	switch {
	case b.CommentCount != nil:
		issue.CommentCount = *b.CommentCount
	case len(b.Comments) > 0:
		issue.CommentCount = len(b.Comments)
	}
	return issue
}

// parseBDShow decodes `bd show --json` output into domain issues.
func parseBDShow(output string) ([]domain.Issue, error) {
	payload, err := bdJSONPayload(output)
	if err != nil {
		return nil, err
	}
	raw, err := decodeBDList[bdShowIssue](payload)
	if err != nil {
		return nil, err
	}
	issues := make([]domain.Issue, len(raw))
	for i, r := range raw {
		issues[i] = r.toDomain()
	}
	return issues, nil
}

// parseBDCreate decodes `bd create --json` output.
func parseBDCreate(output string) (domain.CreateResult, error) {
	payload, err := bdJSONPayload(output)
	if err != nil {
		return domain.CreateResult{}, err
	}
	results, err := decodeBDList[bdCreateResult](payload)
	if err != nil {
		return domain.CreateResult{}, err
	}
	if len(results) == 0 || results[0].ID == "" {
		return domain.CreateResult{}, errors.New("no issue id in output")
	}
	return domain.CreateResult{ID: results[0].ID, Title: results[0].Title}, nil
}

// parseBDComments decodes `bd comments --json` output.
func parseBDComments(output string) ([]domain.Comment, error) {
	payload, err := bdJSONPayload(output)
	if err != nil {
		return nil, err
	}
	raw, err := decodeBDList[bdComment](payload)
	if err != nil {
		return nil, err
	}
	comments := make([]domain.Comment, len(raw))
	for i, c := range raw {
		comments[i] = c.toDomain()
	}
	return comments, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func appendUnique(list []string, value string) []string {
	if slices.Contains(list, value) {
		return list
	}
	return append(list, value)
}
