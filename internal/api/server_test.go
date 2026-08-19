package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tyagiquamar/durablego/internal/execution"
)

type failingListBackend struct {
	Backend
}

func (failingListBackend) ListWorkflows() ([]execution.Workflow, error) {
	return nil, errors.New("database unavailable")
}

type emptyListBackend struct {
	Backend
}

func (emptyListBackend) ListWorkflows() ([]execution.Workflow, error) {
	return nil, nil
}

func TestStartWorkflowAndHistoryEndpoint(t *testing.T) {
	engine := execution.New(time.Second, 3)
	server := New(engine)

	body := []byte(`{"namespace":"production","name":"order","idempotency_key":"order-1","activities":[{"Name":"validate"}]}`)
	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/workflows", bytes.NewReader(body))
	server.ServeHTTP(res, req)

	if res.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", res.Code, res.Body.String())
	}
	var started struct {
		Workflow execution.Workflow `json:"workflow"`
	}
	if err := json.NewDecoder(res.Body).Decode(&started); err != nil {
		t.Fatal(err)
	}

	res = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/workflows/"+started.Workflow.ID, nil)
	server.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", res.Code, res.Body.String())
	}
	var detail struct {
		Events []execution.Event `json:"events"`
	}
	if err := json.NewDecoder(res.Body).Decode(&detail); err != nil {
		t.Fatal(err)
	}
	if len(detail.Events) == 0 {
		t.Fatal("expected event history")
	}
}

func TestMutationRoutesRequireConfiguredAPIKey(t *testing.T) {
	engine := execution.New(time.Second, 3)
	server := New(engine, "production-key")

	request := httptest.NewRequest(http.MethodPost, "/v1/workflows", bytes.NewBufferString(`{"namespace":"production","name":"order"}`))
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d body=%s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodPost, "/v1/workflows", bytes.NewBufferString(`{"namespace":"production","name":"order","activities":[{"Name":"validate"}]}`))
	request.Header.Set("Authorization", "Bearer production-key")
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("authorized status = %d body=%s", response.Code, response.Body.String())
	}
}

func TestListWorkflowsReturnsEmptyArrayForEmptyEngine(t *testing.T) {
	response := httptest.NewRecorder()
	New(execution.New(time.Second, 3)).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/workflows", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
	var body struct {
		Workflows []execution.Workflow `json:"workflows"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Workflows == nil || len(body.Workflows) != 0 {
		t.Fatalf("workflows = %#v, want an empty array", body.Workflows)
	}
}

func TestListWorkflowsReturnsServerErrorWhenBackendFails(t *testing.T) {
	response := httptest.NewRecorder()
	New(failingListBackend{}).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/workflows", nil))

	if response.Code < http.StatusInternalServerError {
		t.Fatalf("status = %d body=%s, want server error", response.Code, response.Body.String())
	}
}

func TestListWorkflowsNormalizesNilSuccessToEmptyArray(t *testing.T) {
	response := httptest.NewRecorder()
	New(emptyListBackend{}).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/workflows", nil))

	var body struct {
		Workflows []execution.Workflow `json:"workflows"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Workflows == nil {
		t.Fatalf("workflows = nil, want an empty array")
	}
}
