package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tyagiquamar/durablego/internal/execution"
)

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
