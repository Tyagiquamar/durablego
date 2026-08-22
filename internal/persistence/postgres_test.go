package persistence

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/tyagiquamar/durablego/internal/execution"
	"github.com/tyagiquamar/durablego/internal/testdb"
)

func TestMain(m *testing.M) {
	code := m.Run()
	testdb.Shutdown()
	os.Exit(code)
}

func startDatabase(t *testing.T) string {
	t.Helper()
	return testdb.Shared(t)
}

func uniqueNamespace(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("%s_%d", t.Name(), time.Now().UnixNano())
}

func TestPostgresWorkflowClaimAndFencing(t *testing.T) {
	url := startDatabase(t)
	store, err := NewPostgres(context.Background(), url, time.Second, 3)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	namespace := uniqueNamespace(t)
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
	changed, err := store.RunSchedulerPass(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if changed == 0 {
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

func TestCrashBeforeCommitReplayLeavesNoPartialRows(t *testing.T) {
	url := startDatabase(t)
	store, err := NewPostgres(context.Background(), url, 30*time.Second, 3)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	def := execution.WorkflowDefinition{
		Namespace: uniqueNamespace(t),
		Name:      "crash-replay",
		Activities: []execution.ActivityDefinition{
			{Name: "validate"},
			{Name: "charge", DependsOn: []string{"validate"}},
		},
	}
	crashed := false
	store.crashBeforeCommit = func() error {
		crashed = true
		return errors.New("simulated crash before commit")
	}
	if _, _, err := store.Start(def, "crash-1"); err == nil || !crashed {
		t.Fatalf("start err = %v crashed = %v, want simulated crash", err, crashed)
	}
	store.crashBeforeCommit = nil

	workflow, duplicate, err := store.Start(def, "crash-1")
	if err != nil {
		t.Fatal(err)
	}
	if duplicate {
		t.Fatal("replay after aborted transaction should create a fresh workflow")
	}
	workflows, err := store.ListWorkflows()
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, row := range workflows {
		if row.Namespace == def.Namespace {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("workflows in namespace = %d, want 1 with no partial rows", count)
	}
	got, activities, _, err := store.Workflow(workflow.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(activities) != len(def.Activities) {
		t.Fatalf("activities = %d, want %d", len(activities), len(def.Activities))
	}
	if got.Status != execution.WorkflowRunning {
		t.Fatalf("status = %s", got.Status)
	}
}

func TestCrashAfterCommitReplayReturnsOriginalWorkflow(t *testing.T) {
	url := startDatabase(t)
	store, err := NewPostgres(context.Background(), url, 30*time.Second, 3)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	def := execution.WorkflowDefinition{
		Namespace: uniqueNamespace(t),
		Name:      "ambiguous-replay",
		Activities: []execution.ActivityDefinition{{Name: "validate"}},
	}
	committed := false
	store.crashAfterCommit = func() {
		committed = true
		panic("worker process died after commit")
	}
	func() {
		defer func() { _ = recover() }()
		_, _, _ = store.Start(def, "replay-1")
	}()
	if !committed {
		t.Fatal("expected post-commit crash hook to fire")
	}
	store.crashAfterCommit = nil

	replay, duplicate, err := store.Start(def, "replay-1")
	if err != nil {
		t.Fatal(err)
	}
	if !duplicate {
		t.Fatal("replay after committed crash should return the original workflow")
	}
	workflows, err := store.ListWorkflows()
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, row := range workflows {
		if row.Namespace == def.Namespace {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("workflows in namespace = %d, want exactly the original", count)
	}
	if replay.ID == "" {
		t.Fatal("replay returned empty workflow id")
	}
}
