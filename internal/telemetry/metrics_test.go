package telemetry

import (
	"strings"
	"testing"

	"github.com/tyagiquamar/durablego/internal/execution"
)

func TestWorkflowMetricsIncludesStatuses(t *testing.T) {
	body := WorkflowMetrics([]execution.Workflow{{Status: execution.WorkflowRunning}})
	if !strings.Contains(body, `durablego_workflows_total{status="running"} 1`) {
		t.Fatalf("metrics missing running count:\n%s", body)
	}
	if !strings.Contains(body, `durablego_workflows_total{status="failed"} 0`) {
		t.Fatalf("metrics missing failed count:\n%s", body)
	}
}
