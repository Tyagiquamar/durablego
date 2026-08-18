package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/tyagiquamar/durablego/internal/execution"
)

type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

type StartWorkflowRequest struct {
	Namespace      string                         `json:"namespace"`
	Name           string                         `json:"name"`
	IdempotencyKey string                         `json:"idempotency_key"`
	Activities     []execution.ActivityDefinition `json:"activities"`
}

type StartWorkflowResponse struct {
	Workflow  execution.Workflow `json:"workflow"`
	Duplicate bool               `json:"duplicate"`
}

func (c Client) StartWorkflow(ctx context.Context, req StartWorkflowRequest) (StartWorkflowResponse, error) {
	var out StartWorkflowResponse
	if err := c.do(ctx, http.MethodPost, "/v1/workflows", req, &out); err != nil {
		return StartWorkflowResponse{}, err
	}
	return out, nil
}

func (c Client) do(ctx context.Context, method, path string, in, out any) error {
	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	var body bytes.Buffer
	if in != nil {
		if err := json.NewEncoder(&body).Encode(in); err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.BaseURL, "/")+path, &body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		return fmt.Errorf("durablego: %s", res.Status)
	}
	return json.NewDecoder(res.Body).Decode(out)
}
