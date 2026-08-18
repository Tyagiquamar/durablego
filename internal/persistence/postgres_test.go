package persistence

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/tyagiquamar/durablego/internal/execution"
)

func TestPostgresWorkflowClaimAndFencing(t *testing.T) {
	databaseURL := os.Getenv("DURABLEGO_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set DURABLEGO_TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	store, err := NewPostgres(context.Background(), databaseURL, time.Second, 3)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	namespace := "test_" + time.Now().Format("20060102150405.000000000")
	defaultQueue := namespace + "_default"
	paymentsQueue := namespace + "_payments"
	workflow, duplicate, err := store.Start(execution.WorkflowDefinition{
		Namespace: namespace,
		Name:      "postgres-proof",
		Activities: []execution.ActivityDefinition{
			{Name: "validate", TaskQueue: defaultQueue},
			{Name: "payment", TaskQueue: paymentsQueue, DependsOn: []string{"validate"}},
		},
	}, "order-1")
	if err != nil {
		t.Fatal(err)
	}
	if duplicate {
		t.Fatal("first start should not be duplicate")
	}
	again, duplicate, err := store.Start(execution.WorkflowDefinition{
		Namespace: namespace,
		Name:      "postgres-proof",
		Activities: []execution.ActivityDefinition{
			{Name: "validate", TaskQueue: defaultQueue},
			{Name: "payment", TaskQueue: paymentsQueue, DependsOn: []string{"validate"}},
		},
	}, "order-1")
	if err != nil {
		t.Fatal(err)
	}
	if !duplicate || again.ID != workflow.ID {
		t.Fatalf("duplicate start returned duplicate=%v id=%s want %s", duplicate, again.ID, workflow.ID)
	}

	first, err := store.Claim("worker-a", defaultQueue)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(1100 * time.Millisecond)
	if changed := store.RunSchedulerPass(); changed == 0 {
		t.Fatal("expected scheduler to expire the lease")
	}
	second, err := store.Claim("worker-b", defaultQueue)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Complete(first.ActivityID, "worker-a", first.FencingToken); !errors.Is(err, execution.ErrStaleLease) {
		t.Fatalf("stale completion err = %v", err)
	}
	if err := store.Complete(second.ActivityID, "worker-b", second.FencingToken); err != nil {
		t.Fatal(err)
	}
	payment, err := store.Claim("worker-c", paymentsQueue)
	if err != nil {
		t.Fatal(err)
	}
	if payment.Name != "payment" {
		t.Fatalf("claimed %s", payment.Name)
	}
	if err := store.Complete(payment.ActivityID, "worker-c", payment.FencingToken); err != nil {
		t.Fatal(err)
	}
	got, _, events, err := store.Workflow(workflow.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != execution.WorkflowCompleted {
		t.Fatalf("status = %s", got.Status)
	}
	foundStale := false
	for _, event := range events {
		if event.Type == "activity.stale_completion_rejected" {
			foundStale = true
		}
	}
	if !foundStale {
		t.Fatal("expected stale completion event")
	}
}
