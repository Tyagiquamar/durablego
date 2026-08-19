package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/tyagiquamar/durablego/internal/execution"
	"github.com/tyagiquamar/durablego/internal/telemetry"
)

type Backend interface {
	Start(def execution.WorkflowDefinition, idempotencyKey string) (*execution.Workflow, bool, error)
	ListWorkflows() ([]execution.Workflow, error)
	Workflow(id string) (*execution.Workflow, []execution.Activity, []execution.Event, error)
	Cancel(workflowID string) error
	Claim(workerID, taskQueue string) (execution.Claim, error)
	Heartbeat(activityID, workerID string, token int64) error
	Complete(activityID, workerID string, token int64) error
	Fail(activityID, workerID string, token int64, retryable bool, message string) error
}

type Server struct {
	backend Backend
	mux     *http.ServeMux
	apiKey  string
}

func New(backend Backend, apiKeys ...string) *Server {
	apiKey := ""
	if len(apiKeys) > 0 {
		apiKey = apiKeys[0]
	}
	s := &Server{backend: backend, mux: http.NewServeMux(), apiKey: apiKey}
	s.routes()
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "live"})
	})
	s.mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})
	s.mux.HandleFunc("POST /v1/workflows", s.authorizeMutation(s.startWorkflow))
	s.mux.HandleFunc("GET /v1/workflows", s.listWorkflows)
	s.mux.HandleFunc("GET /v1/workflows/{id}", s.getWorkflow)
	s.mux.HandleFunc("POST /v1/workflows/{id}/cancel", s.authorizeMutation(s.cancelWorkflow))
	s.mux.HandleFunc("POST /v1/worker/poll", s.authorizeMutation(s.poll))
	s.mux.HandleFunc("POST /v1/worker/heartbeat", s.authorizeMutation(s.heartbeat))
	s.mux.HandleFunc("POST /v1/worker/complete", s.authorizeMutation(s.complete))
	s.mux.HandleFunc("POST /v1/worker/fail", s.authorizeMutation(s.fail))
	s.mux.HandleFunc("GET /metrics", s.metrics)
}

func (s *Server) authorizeMutation(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.apiKey != "" && r.Header.Get("Authorization") != "Bearer "+s.apiKey {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid api key"})
			return
		}
		next(w, r)
	}
}

type startWorkflowRequest struct {
	Namespace      string                         `json:"namespace"`
	Name           string                         `json:"name"`
	IdempotencyKey string                         `json:"idempotency_key"`
	Activities     []execution.ActivityDefinition `json:"activities"`
}

func (s *Server) startWorkflow(w http.ResponseWriter, r *http.Request) {
	var req startWorkflowRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	workflow, duplicate, err := s.backend.Start(execution.WorkflowDefinition{
		Namespace:  req.Namespace,
		Name:       req.Name,
		Activities: req.Activities,
	}, req.IdempotencyKey)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	status := http.StatusCreated
	if duplicate {
		status = http.StatusOK
	}
	writeJSON(w, status, map[string]any{"workflow": workflow, "duplicate": duplicate})
}

func (s *Server) listWorkflows(w http.ResponseWriter, r *http.Request) {
	workflows, err := s.backend.ListWorkflows()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if workflows == nil {
		workflows = []execution.Workflow{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"workflows": workflows})
}

func (s *Server) getWorkflow(w http.ResponseWriter, r *http.Request) {
	workflow, activities, events, err := s.backend.Workflow(r.PathValue("id"))
	if err != nil {
		writeEngineError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"workflow":   workflow,
		"activities": activities,
		"events":     events,
	})
}

func (s *Server) cancelWorkflow(w http.ResponseWriter, r *http.Request) {
	if err := s.backend.Cancel(r.PathValue("id")); err != nil {
		writeEngineError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) poll(w http.ResponseWriter, r *http.Request) {
	var req struct {
		WorkerID  string `json:"worker_id"`
		TaskQueue string `json:"task_queue"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	claim, err := s.backend.Claim(req.WorkerID, req.TaskQueue)
	if err != nil {
		writeEngineError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, claim)
}

func (s *Server) heartbeat(w http.ResponseWriter, r *http.Request) {
	var req leaseRequest
	if !decodeLease(w, r, &req) {
		return
	}
	if err := s.backend.Heartbeat(req.ActivityID, req.WorkerID, req.FencingToken); err != nil {
		writeEngineError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) complete(w http.ResponseWriter, r *http.Request) {
	var req leaseRequest
	if !decodeLease(w, r, &req) {
		return
	}
	if err := s.backend.Complete(req.ActivityID, req.WorkerID, req.FencingToken); err != nil {
		writeEngineError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) fail(w http.ResponseWriter, r *http.Request) {
	var req struct {
		leaseRequest
		Retryable bool   `json:"retryable"`
		Message   string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.backend.Fail(req.ActivityID, req.WorkerID, req.FencingToken, req.Retryable, req.Message); err != nil {
		writeEngineError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) metrics(w http.ResponseWriter, r *http.Request) {
	workflows, err := s.backend.ListWorkflows()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	_, _ = w.Write([]byte(telemetry.WorkflowMetrics(workflows)))
}

type leaseRequest struct {
	ActivityID   string `json:"activity_id"`
	WorkerID     string `json:"worker_id"`
	FencingToken int64  `json:"fencing_token"`
}

func decodeLease(w http.ResponseWriter, r *http.Request, req *leaseRequest) bool {
	if err := json.NewDecoder(r.Body).Decode(req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return false
	}
	return true
}

func writeEngineError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, execution.ErrNotFound):
		writeError(w, http.StatusNotFound, err)
	case errors.Is(err, execution.ErrNoRunnableActivity):
		writeError(w, http.StatusNoContent, err)
	case errors.Is(err, execution.ErrStaleLease):
		writeError(w, http.StatusConflict, err)
	default:
		writeError(w, http.StatusBadRequest, err)
	}
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if status != http.StatusNoContent {
		_ = json.NewEncoder(w).Encode(value)
	}
}
