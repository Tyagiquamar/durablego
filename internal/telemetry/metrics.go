package telemetry

import (
	"fmt"
	"strings"

	"github.com/tyagiquamar/durablego/internal/execution"
)

func WorkflowMetrics(workflows []execution.Workflow) string {
	counts := map[execution.WorkflowStatus]int{}
	for _, workflow := range workflows {
		counts[workflow.Status]++
	}
	lines := []string{
		"# HELP durablego_workflows_total Workflows by status.",
		"# TYPE durablego_workflows_total gauge",
	}
	for _, status := range []execution.WorkflowStatus{execution.WorkflowRunning, execution.WorkflowCompleted, execution.WorkflowFailed, execution.WorkflowCancelled} {
		lines = append(lines, fmt.Sprintf("durablego_workflows_total{status=%q} %d", status, counts[status]))
	}
	return strings.Join(lines, "\n") + "\n"
}
