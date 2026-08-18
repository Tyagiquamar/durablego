package execution

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func testEngine(now time.Time) *Engine {
	e := New(2*time.Second, 3)
	e.SetClock(func() time.Time { return now })
	return e
}

func sequentialDef() WorkflowDefinition {
	return WorkflowDefinition{
		Namespace: "production",
		Name:      "order-processing",
		Activities: []ActivityDefinition{
			{Name: "validate", TaskQueue: "default"},
			{Name: "payment", TaskQueue: "payments", DependsOn: []string{"validate"}},
		},
	}
}

func TestIdempotentStartReturnsExistingWorkflow(t *testing.T) {
	e := testEngine(time.Unix(100, 0))

	first, duplicate, err := e.Start(sequentialDef(), "order-129331")
	if err != nil {
		t.Fatal(err)
	}
	if duplicate {
		t.Fatal("first start should not be duplicate")
	}
	second, duplicate, err := e.Start(sequentialDef(), "order-129331")
	if err != nil {
		t.Fatal(err)
	}
	if !duplicate {
		t.Fatal("second start should be duplicate")
	}
	if first.ID != second.ID {
		t.Fatalf("workflow id = %s, want %s", second.ID, first.ID)
	}
}

func TestDependenciesUnlockDownstreamActivity(t *testing.T) {
	e := testEngine(time.Unix(100, 0))
	_, _, err := e.Start(sequentialDef(), "")
	if err != nil {
		t.Fatal(err)
	}
	claim, err := e.Claim("worker-a", "default")
	if err != nil {
		t.Fatal(err)
	}
	if claim.Name != "validate" {
		t.Fatalf("claimed %s", claim.Name)
	}
	if err := e.Complete(claim.ActivityID, "worker-a", claim.FencingToken); err != nil {
		t.Fatal(err)
	}
	next, err := e.Claim("worker-b", "payments")
	if err != nil {
		t.Fatal(err)
	}
	if next.Name != "payment" {
		t.Fatalf("claimed %s", next.Name)
	}
}

func TestConcurrentClaimOnlyGrantsOneWorker(t *testing.T) {
	e := testEngine(time.Unix(100, 0))
	_, _, err := e.Start(WorkflowDefinition{
		Namespace:  "production",
		Name:       "single",
		Activities: []ActivityDefinition{{Name: "ship"}},
	}, "")
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, worker := range []string{"worker-a", "worker-b"} {
		wg.Add(1)
		go func(worker string) {
			defer wg.Done()
			_, err := e.Claim(worker, "")
			results <- err
		}(worker)
	}
	wg.Wait()
	close(results)

	claims := 0
	for err := range results {
		if err == nil {
			claims++
		}
	}
	if claims != 1 {
		t.Fatalf("claims = %d, want 1", claims)
	}
}

func TestStaleCompletionRejectedAfterLeaseReclaim(t *testing.T) {
	now := time.Unix(100, 0)
	e := New(2*time.Second, 3)
	e.SetClock(func() time.Time { return now })
	_, _, err := e.Start(WorkflowDefinition{
		Namespace:  "production",
		Name:       "single",
		Activities: []ActivityDefinition{{Name: "inventory"}},
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	oldClaim, err := e.Claim("worker-a", "")
	if err != nil {
		t.Fatal(err)
	}

	now = now.Add(3 * time.Second)
	if changed := e.RunSchedulerPass(); changed != 1 {
		t.Fatalf("changed = %d, want 1", changed)
	}
	newClaim, err := e.Claim("worker-b", "")
	if err != nil {
		t.Fatal(err)
	}
	if newClaim.FencingToken <= oldClaim.FencingToken {
		t.Fatalf("token did not advance: old=%d new=%d", oldClaim.FencingToken, newClaim.FencingToken)
	}
	err = e.Complete(oldClaim.ActivityID, "worker-a", oldClaim.FencingToken)
	if !errors.Is(err, ErrStaleLease) {
		t.Fatalf("err = %v, want stale lease", err)
	}
	if err := e.Complete(newClaim.ActivityID, "worker-b", newClaim.FencingToken); err != nil {
		t.Fatal(err)
	}
}

func TestRetryExhaustionFailsWorkflow(t *testing.T) {
	e := testEngine(time.Unix(100, 0))
	workflow, _, err := e.Start(WorkflowDefinition{
		Namespace: "production",
		Name:      "retry",
		Activities: []ActivityDefinition{{
			Name:        "charge",
			MaxAttempts: 1,
		}},
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	claim, err := e.Claim("worker-a", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Fail(claim.ActivityID, "worker-a", claim.FencingToken, true, "gateway unavailable"); err != nil {
		t.Fatal(err)
	}
	got, _, _, err := e.Workflow(workflow.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != WorkflowFailed {
		t.Fatalf("status = %s", got.Status)
	}
}
