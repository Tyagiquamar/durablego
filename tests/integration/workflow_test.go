package integration_test

import (
	"testing"
	"time"

	"github.com/tyagiquamar/durablego/internal/execution"
)

func TestWorkflowCompletesThroughPublicExecutionFlow(t *testing.T) {
	engine := execution.New(time.Second, 3)
	workflow, _, err := engine.Start(execution.WorkflowDefinition{
		Namespace: "production",
		Name:      "order-processing",
		Activities: []execution.ActivityDefinition{
			{Name: "validate", TaskQueue: "default"},
			{Name: "payment", TaskQueue: "payments", DependsOn: []string{"validate"}},
			{Name: "inventory", TaskQueue: "inventory", DependsOn: []string{"validate"}},
		},
	}, "order-42")
	if err != nil {
		t.Fatal(err)
	}

	validate, err := engine.Claim("worker-default", "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Complete(validate.ActivityID, "worker-default", validate.FencingToken); err != nil {
		t.Fatal(err)
	}
	for _, queue := range []string{"payments", "inventory"} {
		claim, err := engine.Claim("worker-"+queue, queue)
		if err != nil {
			t.Fatal(err)
		}
		if err := engine.Complete(claim.ActivityID, "worker-"+queue, claim.FencingToken); err != nil {
			t.Fatal(err)
		}
	}
	got, _, events, err := engine.Workflow(workflow.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != execution.WorkflowCompleted {
		t.Fatalf("status = %s", got.Status)
	}
	if events[len(events)-1].Type != "workflow.completed" {
		t.Fatalf("last event = %s", events[len(events)-1].Type)
	}
}
