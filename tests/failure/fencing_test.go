package failure_test

import (
	"errors"
	"testing"
	"time"

	"github.com/tyagiquamar/durablego/internal/execution"
)

func TestStaleWorkerProofScene(t *testing.T) {
	now := time.Unix(100, 0)
	engine := execution.New(2*time.Second, 3)
	engine.SetClock(func() time.Time { return now })
	workflow, _, err := engine.Start(execution.WorkflowDefinition{
		Namespace:  "production",
		Name:       "stale-worker-proof",
		Activities: []execution.ActivityDefinition{{Name: "inventory"}},
	}, "")
	if err != nil {
		t.Fatal(err)
	}

	stale, err := engine.Claim("worker-a", "")
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(3 * time.Second)
	engine.RunSchedulerPass()
	current, err := engine.Claim("worker-b", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Complete(stale.ActivityID, "worker-a", stale.FencingToken); !errors.Is(err, execution.ErrStaleLease) {
		t.Fatalf("stale completion err = %v", err)
	}
	if err := engine.Complete(current.ActivityID, "worker-b", current.FencingToken); err != nil {
		t.Fatal(err)
	}
	_, _, events, err := engine.Workflow(workflow.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, event := range events {
		if event.Type == "activity.stale_completion_rejected" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected stale completion rejection event")
	}
}
