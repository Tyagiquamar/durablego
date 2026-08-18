package persistence

import (
	"context"

	"github.com/tyagiquamar/durablego/internal/execution"
)

type WorkflowStore interface {
	StartWorkflow(ctx context.Context, def execution.WorkflowDefinition, idempotencyKey string) (*execution.Workflow, bool, error)
	Workflow(ctx context.Context, id string) (*execution.Workflow, []execution.Activity, []execution.Event, error)
	ListWorkflows(ctx context.Context) ([]execution.Workflow, error)
	CancelWorkflow(ctx context.Context, id string) error
}

type WorkerStore interface {
	Claim(ctx context.Context, workerID, taskQueue string) (execution.Claim, error)
	Heartbeat(ctx context.Context, activityID, workerID string, token int64) error
	Complete(ctx context.Context, activityID, workerID string, token int64) error
	Fail(ctx context.Context, activityID, workerID string, token int64, retryable bool, message string) error
}

type SchedulerStore interface {
	RunSchedulerPass(ctx context.Context) (int, error)
}
