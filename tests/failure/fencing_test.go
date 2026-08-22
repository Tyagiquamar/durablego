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

// An expired lease must be unusable even before the scheduler reaps it.
func TestExpiredLeaseRejectsMutationsBeforeSchedulerPass(t *testing.T) {
	now := time.Unix(100, 0)
	engine := execution.New(2*time.Second, 3)
	engine.SetClock(func() time.Time { return now })
	workflow, _, err := engine.Start(execution.WorkflowDefinition{
		Namespace: "production",
		Name:      "expired-lease-guard",
		Activities: []execution.ActivityDefinition{
			{Name: "heartbeat"},
			{Name: "complete"},
			{Name: "fail"},
		},
	}, "")
	if err != nil {
		t.Fatal(err)
	}

	stale := make([]execution.Claim, 0, 3)
	for range 3 {
		claim, err := engine.Claim("worker-a", "")
		if err != nil {
			t.Fatal(err)
		}
		stale = append(stale, claim)
	}
	now = now.Add(3 * time.Second) // leases expired; RunSchedulerPass deliberately not called

	if err := engine.Heartbeat(stale[0].ActivityID, "worker-a", stale[0].FencingToken); !errors.Is(err, execution.ErrStaleLease) {
		t.Fatalf("expired heartbeat err = %v, want ErrStaleLease", err)
	}
	if err := engine.Complete(stale[1].ActivityID, "worker-a", stale[1].FencingToken); !errors.Is(err, execution.ErrStaleLease) {
		t.Fatalf("expired completion err = %v, want ErrStaleLease", err)
	}
	if err := engine.Fail(stale[2].ActivityID, "worker-a", stale[2].FencingToken, true, "late failure"); !errors.Is(err, execution.ErrStaleLease) {
		t.Fatalf("expired failure err = %v, want ErrStaleLease", err)
	}

	got, activities, events, err := engine.Workflow(workflow.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != execution.WorkflowRunning {
		t.Fatalf("workflow status = %s, want running", got.Status)
	}
	for _, activity := range activities {
		if activity.Status != execution.ActivityRunning || activity.Attempt != 1 || activity.LeaseOwner != "worker-a" {
			t.Fatalf("activity %s mutated by expired worker: %+v", activity.Name, activity)
		}
	}
	found := map[string]bool{}
	for _, event := range events {
		found[event.Type] = true
	}
	for _, expected := range []string{
		"activity.stale_heartbeat_rejected",
		"activity.stale_completion_rejected",
		"activity.stale_failure_rejected",
	} {
		if !found[expected] {
			t.Fatalf("missing %q in events %v", expected, found)
		}
	}

	// The scheduler can still reclaim the expired work afterwards.
	if changed := engine.RunSchedulerPass(); changed != 3 {
		t.Fatalf("changed = %d, want 3", changed)
	}
	for i := range 3 {
		current, err := engine.Claim("worker-b", "")
		if err != nil {
			t.Fatal(err)
		}
		if current.FencingToken <= stale[i].FencingToken {
			t.Fatalf("token did not advance for %s: old=%d new=%d", current.Name, stale[i].FencingToken, current.FencingToken)
		}
		if err := engine.Complete(current.ActivityID, "worker-b", current.FencingToken); err != nil {
			t.Fatal(err)
		}
	}
}
