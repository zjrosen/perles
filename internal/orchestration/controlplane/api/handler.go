// Package api provides an HTTP API for the ControlPlane.
// It exposes REST endpoints for workflow management and SSE for event streaming.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/zjrosen/perles/internal/log"
	"github.com/zjrosen/perles/internal/orchestration/controlplane"
	appreg "github.com/zjrosen/perles/internal/registry/application"
)

// Handler provides HTTP endpoints for ControlPlane operations.
type Handler struct {
	cp              controlplane.ControlPlane
	workflowCreator *appreg.WorkflowCreator
	registryService *appreg.RegistryService
}

// HandlerConfig configures the API handler.
type HandlerConfig struct {
	// ControlPlane manages workflow lifecycle (required).
	ControlPlane controlplane.ControlPlane
	// WorkflowCreator creates epics and tasks in beads (optional).
	// If nil, epic creation is skipped and goal is used directly.
	WorkflowCreator *appreg.WorkflowCreator
	// RegistryService provides access to workflow templates (optional).
	// Required for building coordinator prompts with instructions.
	RegistryService *appreg.RegistryService
}

// NewHandler creates a new API handler wrapping the given ControlPlane.
func NewHandler(cp controlplane.ControlPlane) *Handler {
	return &Handler{cp: cp}
}

// NewHandlerWithConfig creates a new API handler with full configuration.
func NewHandlerWithConfig(cfg HandlerConfig) *Handler {
	return &Handler{
		cp:              cfg.ControlPlane,
		workflowCreator: cfg.WorkflowCreator,
		registryService: cfg.RegistryService,
	}
}

// Routes returns an http.Handler with all API routes registered.
func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()

	// Templates
	mux.HandleFunc("GET /templates", h.ListTemplates)

	// Workflow CRUD
	mux.HandleFunc("POST /workflows", h.Create)
	mux.HandleFunc("GET /workflows", h.List)
	mux.HandleFunc("GET /workflows/{id}", h.Get)
	mux.HandleFunc("POST /workflows/{id}/start", h.Start)
	mux.HandleFunc("POST /workflows/{id}/stop", h.Stop)

	// Event streaming
	mux.HandleFunc("GET /workflows/{id}/events", h.StreamWorkflowEvents)
	mux.HandleFunc("GET /events", h.StreamAllEvents)

	// Health check
	mux.HandleFunc("GET /health", h.Health)

	return mux
}

// === Request/Response Types ===

// CreateWorkflowRequest is the request body for creating a workflow.
type CreateWorkflowRequest struct {
	// TemplateID is the workflow template to use (required).
	TemplateID string `json:"template_id"`
	// Name is the display name for the workflow (optional, defaults to template name).
	Name string `json:"name,omitempty"`
	// Args are template-defined argument values (e.g., {"goal": "Implement X"}).
	// Required arguments are validated against the template's defined arguments.
	Args map[string]string `json:"args,omitempty"`
	// Labels are arbitrary key-value pairs for filtering (optional).
	Labels map[string]string `json:"labels,omitempty"`
	// WorktreeEnabled indicates whether to create a git worktree (optional).
	WorktreeEnabled bool `json:"worktree_enabled,omitempty"`
	// WorktreeBaseBranch is the branch to base the worktree on (required if worktree_enabled).
	WorktreeBaseBranch string `json:"worktree_base_branch,omitempty"`
	// BranchName is an optional custom branch name for the worktree.
	BranchName string `json:"branch_name,omitempty"`
}

// CreateWorkflowResponse is the response body for creating a workflow.
type CreateWorkflowResponse struct {
	ID string `json:"id"`
}

// WorkflowResponse is the response body for a single workflow.
type WorkflowResponse struct {
	ID          string            `json:"id"`
	TemplateID  string            `json:"template_id"`
	Name        string            `json:"name"`
	State       string            `json:"state"`
	InitialGoal string            `json:"initial_goal"`
	Labels      map[string]string `json:"labels,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	StartedAt   *time.Time        `json:"started_at,omitempty"`
	EndedAt     *time.Time        `json:"ended_at,omitempty"`
	Port        int               `json:"port,omitempty"`
	// Worktree fields
	WorktreeEnabled bool   `json:"worktree_enabled,omitempty"`
	WorktreePath    string `json:"worktree_path,omitempty"`
	// Health fields
	IsHealthy       bool       `json:"is_healthy"`
	LastHeartbeatAt *time.Time `json:"last_heartbeat_at,omitempty"`
	LastProgressAt  *time.Time `json:"last_progress_at,omitempty"`
	RecoveryCount   int        `json:"recovery_count,omitempty"`
}

// ListWorkflowsResponse is the response body for listing workflows.
type ListWorkflowsResponse struct {
	Workflows []WorkflowResponse `json:"workflows"`
	Total     int                `json:"total"`
}

// StopWorkflowRequest is the request body for stopping a workflow.
type StopWorkflowRequest struct {
	Reason string `json:"reason,omitempty"`
	Force  bool   `json:"force,omitempty"`
}

// ErrorResponse is the response body for errors.
type ErrorResponse struct {
	Error   string `json:"error"`
	Code    string `json:"code,omitempty"`
	Details string `json:"details,omitempty"`
}

// TemplateResponse is the response body for a single template.
type TemplateResponse struct {
	Key         string             `json:"key"`
	Name        string             `json:"name"`
	Description string             `json:"description,omitempty"`
	Version     string             `json:"version"`
	Source      string             `json:"source"` // "built-in" or "user"
	Labels      []string           `json:"labels,omitempty"`
	Arguments   []ArgumentResponse `json:"arguments,omitempty"`
}

// ArgumentResponse is the response body for a template argument.
type ArgumentResponse struct {
	Key          string `json:"key"`
	Label        string `json:"label"`
	Description  string `json:"description,omitempty"`
	Type         string `json:"type"` // "text", "number", "textarea"
	Required     bool   `json:"required"`
	DefaultValue string `json:"default_value,omitempty"`
}

// ListTemplatesResponse is the response body for listing templates.
type ListTemplatesResponse struct {
	Templates []TemplateResponse `json:"templates"`
	Total     int                `json:"total"`
}

// === Handlers ===

// Create creates a new workflow in Pending state.
// POST /workflows
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var req CreateWorkflowRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body", err.Error())
		return
	}

	// Validate required fields
	if req.TemplateID == "" {
		h.writeError(w, http.StatusBadRequest, "validation_error", "template_id is required", "")
		return
	}

	// Validate required template arguments if registry service is available
	if h.registryService != nil {
		if err := h.validateTemplateArgs(req.TemplateID, req.Args); err != nil {
			h.writeError(w, http.StatusBadRequest, "validation_error", err.Error(), "")
			return
		}
	}

	var epicID string
	var initialPrompt string

	// If WorkflowCreator is available, create epic + tasks in beads first
	if h.workflowCreator != nil {
		// Use name as feature slug, or derive from templateID if empty
		feature := req.Name
		if feature == "" {
			feature = req.TemplateID
		}

		result, err := h.workflowCreator.CreateWithArgs(feature, req.TemplateID, req.Args)
		if err != nil {
			h.writeError(w, http.StatusInternalServerError, "epic_creation_failed", "Failed to create epic", err.Error())
			return
		}

		epicID = result.Epic.ID
	}

	// Build coordinator prompt: instructions template + epic ID section + args
	initialPrompt = h.buildCoordinatorPrompt(req.TemplateID, epicID, req.Args)

	spec := controlplane.WorkflowSpec{
		TemplateID:         req.TemplateID,
		Name:               req.Name,
		InitialGoal:        initialPrompt,
		Labels:             req.Labels,
		WorktreeEnabled:    req.WorktreeEnabled,
		WorktreeBaseBranch: req.WorktreeBaseBranch,
		WorktreeBranchName: req.BranchName,
		EpicID:             epicID,
	}

	id, err := h.cp.Create(r.Context(), spec)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "create_failed", "Failed to create workflow", err.Error())
		return
	}

	h.writeJSON(w, http.StatusCreated, CreateWorkflowResponse{ID: string(id)})
}

// ListTemplates returns all available workflow templates.
// GET /templates
func (h *Handler) ListTemplates(w http.ResponseWriter, r *http.Request) {
	if h.registryService == nil {
		h.writeJSON(w, http.StatusOK, ListTemplatesResponse{
			Templates: []TemplateResponse{},
			Total:     0,
		})
		return
	}

	registrations := h.registryService.GetByNamespace("workflow")
	templates := make([]TemplateResponse, 0, len(registrations))

	for _, reg := range registrations {
		tmpl := TemplateResponse{
			Key:         reg.Key(),
			Name:        reg.Name(),
			Description: reg.Description(),
			Version:     reg.Version(),
			Source:      reg.Source().String(),
			Labels:      reg.Labels(),
		}

		// Convert arguments
		for _, arg := range reg.Arguments() {
			tmpl.Arguments = append(tmpl.Arguments, ArgumentResponse{
				Key:          arg.Key(),
				Label:        arg.Label(),
				Description:  arg.Description(),
				Type:         string(arg.Type()),
				Required:     arg.Required(),
				DefaultValue: arg.DefaultValue(),
			})
		}

		templates = append(templates, tmpl)
	}

	h.writeJSON(w, http.StatusOK, ListTemplatesResponse{
		Templates: templates,
		Total:     len(templates),
	})
}

// validateTemplateArgs validates that required template arguments are present and non-empty.
func (h *Handler) validateTemplateArgs(templateID string, args map[string]string) error {
	reg, err := h.registryService.GetByKey("workflow", templateID)
	if err != nil {
		return fmt.Errorf("template not found: %s", templateID)
	}

	for _, arg := range reg.Arguments() {
		if arg.Required() {
			val := ""
			if args != nil {
				val = strings.TrimSpace(args[arg.Key()])
			}
			if val == "" {
				return fmt.Errorf("%s is required", arg.Label())
			}
		}
	}

	return nil
}

// buildCoordinatorPrompt assembles the coordinator prompt from:
// 1. Instructions template content (from registration's instructions field)
// 2. Epic ID section (so coordinator can read detailed instructions via bd show)
// 3. Argument values (formatted as a section)
func (h *Handler) buildCoordinatorPrompt(templateID, epicID string, args map[string]string) string {
	// Load instructions template if registry service is available
	var instructionsContent string
	if h.registryService != nil {
		// Get the registration for this template
		reg, err := h.registryService.GetByKey("workflow", templateID)
		if err == nil {
			content, err := h.registryService.GetInstructionsTemplate(reg)
			if err == nil {
				instructionsContent = content
			}
		}
		// If error loading template, continue without it
	}

	// Build the full prompt
	var parts []string

	if instructionsContent != "" {
		parts = append(parts, instructionsContent)
	}

	if epicID != "" {
		epicSection := fmt.Sprintf("# Epic\n\nThe epic for this workflow is `%s`. Run `bd show %s` to read the detailed work breakdown and instructions.", epicID, epicID)
		parts = append(parts, epicSection)
	}

	// Build arguments section if any args are present
	if len(args) > 0 {
		var sb strings.Builder
		sb.WriteString("# Arguments\n\n")
		for key, val := range args {
			sb.WriteString(fmt.Sprintf("- **%s**: %s\n", key, val))
		}
		parts = append(parts, sb.String())
	}

	return strings.Join(parts, "\n\n")
}

// List returns all workflows matching optional filters.
// GET /workflows?state=running&template_id=cook
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	query := controlplane.ListQuery{}

	// Parse state filter
	if stateStr := r.URL.Query().Get("state"); stateStr != "" {
		query.States = []controlplane.WorkflowState{controlplane.WorkflowState(stateStr)}
	}

	// Parse template filter
	if templateID := r.URL.Query().Get("template_id"); templateID != "" {
		query.TemplateID = templateID
	}

	workflows, err := h.cp.List(r.Context(), query)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "list_failed", "Failed to list workflows", err.Error())
		return
	}

	resp := ListWorkflowsResponse{
		Workflows: make([]WorkflowResponse, 0, len(workflows)),
		Total:     len(workflows),
	}

	for _, wf := range workflows {
		resp.Workflows = append(resp.Workflows, h.workflowToResponse(wf))
	}

	h.writeJSON(w, http.StatusOK, resp)
}

// Get returns a single workflow by ID.
// GET /workflows/{id}
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id := controlplane.WorkflowID(r.PathValue("id"))

	wf, err := h.cp.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, controlplane.ErrWorkflowNotFound) {
			h.writeError(w, http.StatusNotFound, "not_found", "Workflow not found", "")
			return
		}
		h.writeError(w, http.StatusInternalServerError, "get_failed", "Failed to get workflow", err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, h.workflowToResponse(wf))
}

// Start transitions a workflow from Pending to Running.
// POST /workflows/{id}/start
func (h *Handler) Start(w http.ResponseWriter, r *http.Request) {
	id := controlplane.WorkflowID(r.PathValue("id"))

	if err := h.cp.Start(r.Context(), id); err != nil {
		if errors.Is(err, controlplane.ErrWorkflowNotFound) {
			h.writeError(w, http.StatusNotFound, "not_found", "Workflow not found", "")
			return
		}
		h.writeError(w, http.StatusBadRequest, "start_failed", "Failed to start workflow", err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Stop terminates a workflow.
// POST /workflows/{id}/stop
func (h *Handler) Stop(w http.ResponseWriter, r *http.Request) {
	id := controlplane.WorkflowID(r.PathValue("id"))

	var req StopWorkflowRequest
	if r.Body != nil && r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			h.writeError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body", err.Error())
			return
		}
	}

	opts := controlplane.StopOptions{
		Reason: req.Reason,
		Force:  req.Force,
	}

	if err := h.cp.Stop(r.Context(), id, opts); err != nil {
		if errors.Is(err, controlplane.ErrWorkflowNotFound) {
			h.writeError(w, http.StatusNotFound, "not_found", "Workflow not found", "")
			return
		}
		h.writeError(w, http.StatusBadRequest, "stop_failed", "Failed to stop workflow", err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// StreamWorkflowEvents streams events for a specific workflow via SSE.
// GET /workflows/{id}/events
func (h *Handler) StreamWorkflowEvents(w http.ResponseWriter, r *http.Request) {
	id := controlplane.WorkflowID(r.PathValue("id"))

	// Verify workflow exists
	if _, err := h.cp.Get(r.Context(), id); err != nil {
		if errors.Is(err, controlplane.ErrWorkflowNotFound) {
			h.writeError(w, http.StatusNotFound, "not_found", "Workflow not found", "")
			return
		}
		h.writeError(w, http.StatusInternalServerError, "get_failed", "Failed to get workflow", err.Error())
		return
	}

	events, unsub := h.cp.SubscribeWorkflow(r.Context(), id)
	defer unsub()

	h.streamEvents(w, r, events)
}

// StreamAllEvents streams all control plane events via SSE.
// GET /events
func (h *Handler) StreamAllEvents(w http.ResponseWriter, r *http.Request) {
	events, unsub := h.cp.Subscribe(r.Context())
	defer unsub()

	h.streamEvents(w, r, events)
}

// HealthResponse is the response body for the health endpoint.
type HealthResponse struct {
	Status    string                   `json:"status"`
	Workflows []WorkflowHealthResponse `json:"workflows,omitempty"`
}

// WorkflowHealthResponse is the health status for a single workflow.
type WorkflowHealthResponse struct {
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	State           string     `json:"state"`
	IsHealthy       bool       `json:"is_healthy"`
	LastHeartbeatAt *time.Time `json:"last_heartbeat_at,omitempty"`
	LastProgressAt  *time.Time `json:"last_progress_at,omitempty"`
	RecoveryCount   int        `json:"recovery_count,omitempty"`
}

// Health returns the daemon health status including all workflow health.
// GET /health
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	resp := HealthResponse{Status: "ok"}

	// List all active workflows
	workflows, err := h.cp.List(r.Context(), controlplane.ListQuery{})
	if err != nil {
		// If we can't list workflows, daemon is unhealthy
		h.writeJSON(w, http.StatusServiceUnavailable, HealthResponse{Status: "unhealthy"})
		return
	}

	// Get health status for each workflow
	for _, wf := range workflows {
		wfHealth := WorkflowHealthResponse{
			ID:    string(wf.ID),
			Name:  wf.Name,
			State: string(wf.State),
		}

		// Get detailed health status if available
		if status, ok := h.cp.GetHealthStatus(wf.ID); ok {
			wfHealth.IsHealthy = status.IsHealthy
			wfHealth.RecoveryCount = status.RecoveryCount
			if !status.LastHeartbeatAt.IsZero() {
				wfHealth.LastHeartbeatAt = &status.LastHeartbeatAt
			}
			if !status.LastProgressAt.IsZero() {
				wfHealth.LastProgressAt = &status.LastProgressAt
			}
		} else {
			// No health tracking yet - assume healthy if running
			wfHealth.IsHealthy = wf.State == controlplane.WorkflowRunning
		}

		resp.Workflows = append(resp.Workflows, wfHealth)
	}

	h.writeJSON(w, http.StatusOK, resp)
}

// === Helpers ===

func (h *Handler) streamEvents(w http.ResponseWriter, r *http.Request, events <-chan controlplane.ControlPlaneEvent) {
	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // Disable nginx buffering

	flusher, ok := w.(http.Flusher)
	if !ok {
		h.writeError(w, http.StatusInternalServerError, "streaming_unsupported", "Streaming not supported", "")
		return
	}

	// Send initial connection event
	_, _ = fmt.Fprintf(w, "event: connected\ndata: {}\n\n")
	flusher.Flush()

	// Heartbeat ticker to keep connection alive
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Send heartbeat comment (not a real event, just keeps connection alive)
			_, _ = fmt.Fprintf(w, ": heartbeat\n\n")
			flusher.Flush()
		case event, ok := <-events:
			if !ok {
				return
			}

			data, err := json.Marshal(h.eventToJSON(event))
			if err != nil {
				log.Error(log.CatOrch, "Failed to marshal event", "error", err)
				continue
			}

			_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, data)
			flusher.Flush()
		}
	}
}

func (h *Handler) workflowToResponse(wf *controlplane.WorkflowInstance) WorkflowResponse {
	resp := WorkflowResponse{
		ID:              string(wf.ID),
		TemplateID:      wf.TemplateID,
		Name:            wf.Name,
		State:           string(wf.State),
		InitialGoal:     wf.InitialGoal,
		Labels:          wf.Labels,
		CreatedAt:       wf.CreatedAt,
		Port:            wf.MCPPort,
		WorktreeEnabled: wf.WorktreeEnabled,
		WorktreePath:    wf.WorktreePath,
	}

	if wf.StartedAt != nil {
		resp.StartedAt = wf.StartedAt
	}

	// Add health status if available
	if status, ok := h.cp.GetHealthStatus(wf.ID); ok {
		resp.IsHealthy = status.IsHealthy
		resp.RecoveryCount = status.RecoveryCount
		if !status.LastHeartbeatAt.IsZero() {
			resp.LastHeartbeatAt = &status.LastHeartbeatAt
		}
		if !status.LastProgressAt.IsZero() {
			resp.LastProgressAt = &status.LastProgressAt
		}
	} else {
		// No health tracking yet - assume healthy if running
		resp.IsHealthy = wf.State == controlplane.WorkflowRunning
	}

	return resp
}

func (h *Handler) eventToJSON(event controlplane.ControlPlaneEvent) map[string]any {
	result := map[string]any{
		"type":          string(event.Type),
		"workflow_id":   string(event.WorkflowID),
		"workflow_name": event.WorkflowName,
		"template_id":   event.TemplateID,
		"state":         string(event.State),
		"timestamp":     event.Timestamp,
	}

	if event.ProcessID != "" {
		result["process_id"] = event.ProcessID
	}
	if event.TaskID != "" {
		result["task_id"] = event.TaskID
	}

	// Try to serialize payload - if it fails, convert to string
	if event.Payload != nil {
		if payloadBytes, err := json.Marshal(event.Payload); err == nil {
			var payloadMap any
			if json.Unmarshal(payloadBytes, &payloadMap) == nil {
				result["payload"] = payloadMap
			} else {
				result["payload"] = string(payloadBytes)
			}
		} else {
			result["payload"] = fmt.Sprintf("%v", event.Payload)
		}
	}

	return result
}

func (h *Handler) writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Error(log.CatOrch, "Failed to encode JSON response", "error", err)
	}
}

func (h *Handler) writeError(w http.ResponseWriter, status int, code, message, details string) {
	h.writeJSON(w, status, ErrorResponse{
		Error:   message,
		Code:    code,
		Details: details,
	})
}

// Server wraps the Handler with an http.Server for lifecycle management.
type Server struct {
	handler  *Handler
	server   *http.Server
	listener net.Listener
	addr     string
	port     int // Actual port after binding (useful when using :0)
}

// ServerConfig configures the API server.
type ServerConfig struct {
	// Addr is the address to listen on (e.g., ":19999" or "/var/run/perles.sock").
	Addr string
	// ControlPlane is the control plane to expose via HTTP.
	ControlPlane controlplane.ControlPlane
	// WorkflowCreator creates epics and tasks in beads (optional).
	WorkflowCreator *appreg.WorkflowCreator
	// RegistryService provides access to workflow templates (optional).
	RegistryService *appreg.RegistryService
	// ReadTimeout is the maximum duration for reading the entire request.
	ReadTimeout time.Duration
	// WriteTimeout is the maximum duration before timing out writes of the response.
	WriteTimeout time.Duration
}

// NewServer creates a new API server.
// If Addr uses port 0 (e.g., "localhost:0" or ":0"), the OS will assign an available port.
// Use Port() after Start() to get the actual port.
func NewServer(cfg ServerConfig) (*Server, error) {
	handler := NewHandlerWithConfig(HandlerConfig{
		ControlPlane:    cfg.ControlPlane,
		WorkflowCreator: cfg.WorkflowCreator,
		RegistryService: cfg.RegistryService,
	})

	readTimeout := cfg.ReadTimeout
	if readTimeout == 0 {
		readTimeout = 30 * time.Second
	}

	writeTimeout := cfg.WriteTimeout
	if writeTimeout == 0 {
		writeTimeout = 0 // No timeout for SSE
	}

	// Create listener first to get the actual port (important for :0)
	listener, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on %s: %w", cfg.Addr, err)
	}

	// Extract actual port from listener address
	port := 0
	if tcpAddr, ok := listener.Addr().(*net.TCPAddr); ok {
		port = tcpAddr.Port
	}

	return &Server{
		handler:  handler,
		addr:     cfg.Addr,
		port:     port,
		listener: listener,
		server: &http.Server{
			Handler:           handler.Routes(),
			ReadTimeout:       readTimeout,
			ReadHeaderTimeout: 10 * time.Second,
			WriteTimeout:      writeTimeout,
		},
	}, nil
}

// Start starts the HTTP server. It blocks until the server is stopped or fails.
func (s *Server) Start() error {
	log.Info(log.CatOrch, "Starting API server", "addr", s.listener.Addr().String(), "port", s.port)
	return s.server.Serve(s.listener)
}

// Stop gracefully shuts down the server.
func (s *Server) Stop(ctx context.Context) error {
	log.Info(log.CatOrch, "Stopping API server")
	return s.server.Shutdown(ctx)
}

// Port returns the actual port the server is listening on.
// This is useful when the server was configured with port 0 for auto-assignment.
func (s *Server) Port() int {
	return s.port
}
