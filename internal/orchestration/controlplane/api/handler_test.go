package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/orchestration/controlplane"
	"github.com/zjrosen/perles/internal/orchestration/controlplane/mocks"
	appreg "github.com/zjrosen/perles/internal/registry/application"
)

// === Tests ===

func TestHandler_Create(t *testing.T) {
	mockCP := mocks.NewMockControlPlane(t)
	mockCP.EXPECT().
		Create(mock.Anything, mock.MatchedBy(func(spec controlplane.WorkflowSpec) bool {
			// Arguments are now rendered into epic template via {{.Args.key}},
			// so they don't appear in InitialPrompt. Without RegistryService, InitialPrompt is empty.
			return spec.TemplateID == "cook" && spec.InitialPrompt == ""
		})).
		Return(controlplane.WorkflowID("wf-123"), nil).
		Once()

	h := NewHandler(mockCP)

	body := `{"template_id": "cook", "args": {"goal": "Build feature X"}}`
	req := httptest.NewRequest(http.MethodPost, "/workflows", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	h.Routes().ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)

	var resp CreateWorkflowResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "wf-123", resp.ID)
}

func TestHandler_Create_InvalidJSON(t *testing.T) {
	mockCP := mocks.NewMockControlPlane(t)

	h := NewHandler(mockCP)

	req := httptest.NewRequest(http.MethodPost, "/workflows", bytes.NewBufferString("not json"))
	w := httptest.NewRecorder()

	h.Routes().ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)

	var resp ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "invalid_json", resp.Code)
}

func TestHandler_Get(t *testing.T) {
	mockCP := mocks.NewMockControlPlane(t)
	mockCP.EXPECT().
		Get(mock.Anything, controlplane.WorkflowID("wf-123")).
		Return(&controlplane.WorkflowInstance{
			ID:            "wf-123",
			TemplateID:    "cook",
			State:         controlplane.WorkflowRunning,
			InitialPrompt: "Build feature",
			MCPPort:       19001,
		}, nil).
		Once()
	mockCP.EXPECT().
		GetHealthStatus(controlplane.WorkflowID("wf-123")).
		Return(controlplane.HealthStatus{
			WorkflowID: "wf-123",
			IsHealthy:  true,
		}, true).
		Once()

	h := NewHandler(mockCP)

	req := httptest.NewRequest(http.MethodGet, "/workflows/wf-123", nil)
	w := httptest.NewRecorder()

	h.Routes().ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp WorkflowResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "wf-123", resp.ID)
	assert.Equal(t, "running", resp.State)
	assert.Equal(t, 19001, resp.Port)
	assert.True(t, resp.IsHealthy)
}

func TestHandler_Get_NotFound(t *testing.T) {
	mockCP := mocks.NewMockControlPlane(t)
	mockCP.EXPECT().
		Get(mock.Anything, controlplane.WorkflowID("unknown")).
		Return(nil, controlplane.ErrWorkflowNotFound).
		Once()

	h := NewHandler(mockCP)

	req := httptest.NewRequest(http.MethodGet, "/workflows/unknown", nil)
	w := httptest.NewRecorder()

	h.Routes().ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandler_Start(t *testing.T) {
	mockCP := mocks.NewMockControlPlane(t)
	mockCP.EXPECT().
		Start(mock.Anything, controlplane.WorkflowID("wf-123")).
		Return(nil).
		Once()

	h := NewHandler(mockCP)

	req := httptest.NewRequest(http.MethodPost, "/workflows/wf-123/start", nil)
	w := httptest.NewRecorder()

	h.Routes().ServeHTTP(w, req)

	require.Equal(t, http.StatusNoContent, w.Code)
}

func TestHandler_Pause(t *testing.T) {
	mockCP := mocks.NewMockControlPlane(t)
	mockCP.EXPECT().
		Pause(mock.Anything, controlplane.WorkflowID("wf-123")).
		Return(nil).
		Once()

	h := NewHandler(mockCP)

	req := httptest.NewRequest(http.MethodPost, "/workflows/wf-123/pause", nil)
	w := httptest.NewRecorder()

	h.Routes().ServeHTTP(w, req)

	require.Equal(t, http.StatusNoContent, w.Code)
}

func TestHandler_Pause_NotFound(t *testing.T) {
	mockCP := mocks.NewMockControlPlane(t)
	mockCP.EXPECT().
		Pause(mock.Anything, controlplane.WorkflowID("unknown")).
		Return(controlplane.ErrWorkflowNotFound).
		Once()

	h := NewHandler(mockCP)

	req := httptest.NewRequest(http.MethodPost, "/workflows/unknown/pause", nil)
	w := httptest.NewRecorder()

	h.Routes().ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code)

	var resp ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "not_found", resp.Code)
}

func TestHandler_Pause_InvalidState(t *testing.T) {
	mockCP := mocks.NewMockControlPlane(t)
	mockCP.EXPECT().
		Pause(mock.Anything, controlplane.WorkflowID("wf-123")).
		Return(controlplane.ErrInvalidState).
		Once()

	h := NewHandler(mockCP)

	req := httptest.NewRequest(http.MethodPost, "/workflows/wf-123/pause", nil)
	w := httptest.NewRecorder()

	h.Routes().ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)

	var resp ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "invalid_state", resp.Code)
}

func TestHandler_Resume(t *testing.T) {
	mockCP := mocks.NewMockControlPlane(t)
	mockCP.EXPECT().
		Resume(mock.Anything, controlplane.WorkflowID("wf-123")).
		Return(nil).
		Once()

	h := NewHandler(mockCP)

	req := httptest.NewRequest(http.MethodPost, "/workflows/wf-123/resume", nil)
	w := httptest.NewRecorder()

	h.Routes().ServeHTTP(w, req)

	require.Equal(t, http.StatusNoContent, w.Code)
}

func TestHandler_Resume_NotFound(t *testing.T) {
	mockCP := mocks.NewMockControlPlane(t)
	mockCP.EXPECT().
		Resume(mock.Anything, controlplane.WorkflowID("unknown")).
		Return(controlplane.ErrWorkflowNotFound).
		Once()

	h := NewHandler(mockCP)

	req := httptest.NewRequest(http.MethodPost, "/workflows/unknown/resume", nil)
	w := httptest.NewRecorder()

	h.Routes().ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code)

	var resp ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "not_found", resp.Code)
}

func TestHandler_Resume_InvalidState(t *testing.T) {
	mockCP := mocks.NewMockControlPlane(t)
	mockCP.EXPECT().
		Resume(mock.Anything, controlplane.WorkflowID("wf-123")).
		Return(controlplane.ErrInvalidState).
		Once()

	h := NewHandler(mockCP)

	req := httptest.NewRequest(http.MethodPost, "/workflows/wf-123/resume", nil)
	w := httptest.NewRecorder()

	h.Routes().ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)

	var resp ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "invalid_state", resp.Code)
}

func TestHandler_Stop_EndpointRemoved(t *testing.T) {
	// The /stop endpoint should return 404 (not found) since it's been removed
	mockCP := mocks.NewMockControlPlane(t)

	h := NewHandler(mockCP)

	req := httptest.NewRequest(http.MethodPost, "/workflows/wf-123/stop", nil)
	w := httptest.NewRecorder()

	h.Routes().ServeHTTP(w, req)

	// http.ServeMux returns 404 for unregistered routes
	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandler_List(t *testing.T) {
	mockCP := mocks.NewMockControlPlane(t)
	mockCP.EXPECT().
		List(mock.Anything, mock.Anything).
		Return([]*controlplane.WorkflowInstance{
			{ID: "wf-1", TemplateID: "cook", State: controlplane.WorkflowRunning},
			{ID: "wf-2", TemplateID: "plan", State: controlplane.WorkflowPending},
		}, nil).
		Once()
	mockCP.EXPECT().
		GetHealthStatus(controlplane.WorkflowID("wf-1")).
		Return(controlplane.HealthStatus{WorkflowID: "wf-1", IsHealthy: true}, true).
		Once()
	mockCP.EXPECT().
		GetHealthStatus(controlplane.WorkflowID("wf-2")).
		Return(controlplane.HealthStatus{WorkflowID: "wf-2", IsHealthy: false}, true).
		Once()

	h := NewHandler(mockCP)

	req := httptest.NewRequest(http.MethodGet, "/workflows", nil)
	w := httptest.NewRecorder()

	h.Routes().ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp ListWorkflowsResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, 2, resp.Total)
	assert.Len(t, resp.Workflows, 2)
}

func TestHandler_Health(t *testing.T) {
	mockCP := mocks.NewMockControlPlane(t)
	mockCP.EXPECT().
		List(mock.Anything, mock.Anything).
		Return([]*controlplane.WorkflowInstance{
			{ID: "wf-1", Name: "Test Workflow", State: controlplane.WorkflowRunning},
		}, nil).
		Once()
	mockCP.EXPECT().
		GetHealthStatus(controlplane.WorkflowID("wf-1")).
		Return(controlplane.HealthStatus{
			WorkflowID: "wf-1",
			IsHealthy:  true,
		}, true).
		Once()

	h := NewHandler(mockCP)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	h.Routes().ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp HealthResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "ok", resp.Status)
	require.Len(t, resp.Workflows, 1)
	assert.Equal(t, "wf-1", resp.Workflows[0].ID)
	assert.True(t, resp.Workflows[0].IsHealthy)
}

func TestHandler_ListTemplates(t *testing.T) {
	// Create a test filesystem with a workflow template
	testFS := fstest.MapFS{
		"workflows/test-workflow/template.yaml": &fstest.MapFile{
			Data: []byte(`registry:
  - namespace: "workflow"
    key: "test-workflow"
    version: "v1"
    name: "Test Workflow"
    description: "A test workflow"
    labels:
      - "category:test"
    arguments:
      - key: "goal"
        label: "Goal"
        description: "What do you want to achieve?"
        type: "textarea"
        required: true
      - key: "priority"
        label: "Priority"
        description: "Task priority"
        type: "text"
        required: false
        default: "medium"
    nodes:
      - key: "task1"
        name: "Task 1"
        template: "task1.md"
`),
		},
		"workflows/test-workflow/task1.md": &fstest.MapFile{Data: []byte("# Task 1")},
	}

	registryService, err := appreg.NewRegistryService(testFS, nil, "")
	require.NoError(t, err)

	mockCP := mocks.NewMockControlPlane(t)
	h := NewHandlerWithConfig(HandlerConfig{
		ControlPlane:    mockCP,
		RegistryService: registryService,
	})

	req := httptest.NewRequest(http.MethodGet, "/templates", nil)
	w := httptest.NewRecorder()

	h.Routes().ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp ListTemplatesResponse
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	require.Equal(t, 1, resp.Total)
	require.Len(t, resp.Templates, 1)

	tmpl := resp.Templates[0]
	assert.Equal(t, "test-workflow", tmpl.Key)
	assert.Equal(t, "Test Workflow", tmpl.Name)
	assert.Equal(t, "A test workflow", tmpl.Description)
	assert.Equal(t, "v1", tmpl.Version)
	assert.Equal(t, "built-in", tmpl.Source)
	assert.Contains(t, tmpl.Labels, "category:test")

	// Verify arguments
	require.Len(t, tmpl.Arguments, 2)

	goalArg := tmpl.Arguments[0]
	assert.Equal(t, "goal", goalArg.Key)
	assert.Equal(t, "Goal", goalArg.Label)
	assert.Equal(t, "textarea", goalArg.Type)
	assert.True(t, goalArg.Required)

	priorityArg := tmpl.Arguments[1]
	assert.Equal(t, "priority", priorityArg.Key)
	assert.Equal(t, "Priority", priorityArg.Label)
	assert.Equal(t, "text", priorityArg.Type)
	assert.False(t, priorityArg.Required)
	assert.Equal(t, "medium", priorityArg.DefaultValue)
}

func TestHandler_ListTemplates_NoRegistryService(t *testing.T) {
	mockCP := mocks.NewMockControlPlane(t)
	h := NewHandler(mockCP) // No registry service

	req := httptest.NewRequest(http.MethodGet, "/templates", nil)
	w := httptest.NewRecorder()

	h.Routes().ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp ListTemplatesResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Equal(t, 0, resp.Total)
	assert.Empty(t, resp.Templates)
}

func TestNewServer_FrontendSessionsUseSessionBaseDir(t *testing.T) {
	baseDir := t.TempDir()
	frontendFS := fstest.MapFS{"dist/index.html": &fstest.MapFile{Data: []byte("<html></html>")}}

	server, err := NewServer(ServerConfig{
		Addr:           "localhost:0",
		ControlPlane:   mocks.NewMockControlPlane(t),
		FrontendFS:     frontendFS,
		SessionBaseDir: baseDir,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = server.listener.Close() })

	req := httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
	rec := httptest.NewRecorder()
	server.server.Handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		BasePath string `json:"basePath"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, baseDir, resp.BasePath)
}
