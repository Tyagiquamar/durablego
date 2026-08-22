package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tyagiquamar/durablego/internal/execution"
	"github.com/tyagiquamar/durablego/internal/persistence"
	"github.com/tyagiquamar/durablego/internal/testdb"
)

func TestMain(m *testing.M) {
	code := m.Run()
	testdb.Shutdown()
	os.Exit(code)
}

func newStore(t *testing.T, leaseTTL time.Duration) *persistence.Postgres {
	t.Helper()
	store, err := persistence.NewPostgres(context.Background(), testdb.Shared(t), leaseTTL, 3)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	return store
}

func uniqueNamespace(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("%s_%d", strings.ReplaceAll(t.Name(), "/", "_"), time.Now().UnixNano())
}

func eventTypes(events []execution.Event) []string {
	types := make([]string, 0, len(events))
	for _, event := range events {
		types = append(types, event.Type)
	}
	return types
}

func TestConcurrentDuplicateStartReturnsSingleWorkflow(t *testing.T) {
	store := newStore(t, 30*time.Second)
	def := execution.WorkflowDefinition{
		Namespace: uniqueNamespace(t),
		Name:      "order-processing",
		Activities: []execution.ActivityDefinition{
			{Name: "validate"},
			{Name: "charge", DependsOn: []string{"validate"}},
		},
	}
	const goroutines = 16
	ids := make([]string, goroutines)
	duplicates := make([]bool, goroutines)
	errs := make([]error, goroutines)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			workflow, duplicate, err := store.Start(def, "order-129331")
			if workflow != nil {
				ids[i] = workflow.ID
			}
			duplicates[i] = duplicate
			errs[i] = err
		}(i)
	}
	close(start)
	wg.Wait()

	fresh := 0
	for i := 0; i < goroutines; i++ {
		if errs[i] != nil {
			t.Fatalf("goroutine %d start error = %v", i, errs[i])
		}
		if ids[i] == "" {
			t.Fatalf("goroutine %d received no workflow id", i)
		}
		if ids[i] != ids[0] {
			t.Fatalf("goroutine %d id = %s, want %s", i, ids[i], ids[0])
		}
		if !duplicates[i] {
			fresh++
		}
	}
	if fresh != 1 {
		t.Fatalf("fresh starts = %d, want exactly 1", fresh)
	}
	workflows, err := store.ListWorkflows()
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, workflow := range workflows {
		if workflow.Namespace == def.Namespace {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("workflows in namespace = %d, want 1", count)
	}
}

func TestClaimSerializationKeepsTokensStrictlyMonotonic(t *testing.T) {
	store := newStore(t, time.Second)
	queue := uniqueNamespace(t) + "_queue"
	workflow, _, err := store.Start(execution.WorkflowDefinition{
		Namespace: uniqueNamespace(t),
		Name:      "claim-race",
		Activities: []execution.ActivityDefinition{
			{Name: "charge", TaskQueue: queue, MaxAttempts: 10},
		},
	}, "")
	if err != nil {
		t.Fatal(err)
	}

	const rounds = 4
	const racers = 8
	tokens := make([]int64, 0, rounds)
	var lastWinner execution.Claim
	var lastWorker string
	for round := 0; round < rounds; round++ {
		results := make([]execution.Claim, racers)
		errs := make([]error, racers)
		barrier := make(chan struct{})
		var wg sync.WaitGroup
		for i := 0; i < racers; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-barrier
				results[i], errs[i] = store.Claim(fmt.Sprintf("worker-%d", i), queue)
			}(i)
		}
		close(barrier)
		wg.Wait()

		winners := 0
		for i := 0; i < racers; i++ {
			switch {
			case errs[i] == nil:
				winners++
				tokens = append(tokens, results[i].FencingToken)
				lastWinner = results[i]
				lastWorker = fmt.Sprintf("worker-%d", i)
			case !errors.Is(errs[i], execution.ErrNoRunnableActivity):
				t.Fatalf("racer %d claim error = %v", i, errs[i])
			}
		}
		if winners != 1 {
			t.Fatalf("round %d winners = %d, want exactly 1", round, winners)
		}
		if round < rounds-1 {
			time.Sleep(1200 * time.Millisecond)
			changed, err := store.RunSchedulerPass(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if changed != 1 {
				t.Fatalf("round %d scheduler changed = %d, want 1", round, changed)
			}
		}
	}
	for i := 1; i < len(tokens); i++ {
		if tokens[i] <= tokens[i-1] {
			t.Fatalf("fencing tokens not strictly monotonic: %v", tokens)
		}
	}
	if err := store.Complete(lastWinner.ActivityID, lastWorker, lastWinner.FencingToken); err != nil {
		t.Fatal(err)
	}
	got, activities, _, err := store.Workflow(workflow.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != execution.WorkflowCompleted {
		t.Fatalf("status = %s, want completed after final claim", got.Status)
	}
	if len(activities) != 1 || activities[0].Attempt != rounds {
		t.Fatalf("attempt = %+v, want %d claims recorded", activities, rounds)
	}
}

func TestStaleHeartbeatCompleteFailRejectedWithEvents(t *testing.T) {
	store := newStore(t, time.Second)
	namespace := uniqueNamespace(t)
	queue := namespace + "_queue"
	workflow, _, err := store.Start(execution.WorkflowDefinition{
		Namespace: namespace,
		Name:      "stale-proof",
		Activities: []execution.ActivityDefinition{
			{Name: "heartbeat-target", TaskQueue: queue},
			{Name: "complete-target", TaskQueue: queue},
			{Name: "fail-target", TaskQueue: queue},
		},
	}, "")
	if err != nil {
		t.Fatal(err)
	}

	stale := make([]execution.Claim, 0, 3)
	for {
		claim, err := store.Claim("worker-a", queue)
		if errors.Is(err, execution.ErrNoRunnableActivity) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		stale = append(stale, claim)
	}
	if len(stale) != 3 {
		t.Fatalf("claims = %d, want 3", len(stale))
	}
	time.Sleep(1200 * time.Millisecond)
	changed, err := store.RunSchedulerPass(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if changed != 3 {
		t.Fatalf("scheduler changed = %d, want 3", changed)
	}
	current := make([]execution.Claim, 0, 3)
	for {
		claim, err := store.Claim("worker-b", queue)
		if errors.Is(err, execution.ErrNoRunnableActivity) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		current = append(current, claim)
	}
	if len(current) != 3 {
		t.Fatalf("reclaims = %d, want 3", len(current))
	}

	if err := store.Heartbeat(stale[0].ActivityID, "worker-a", stale[0].FencingToken); !errors.Is(err, execution.ErrStaleLease) {
		t.Fatalf("stale heartbeat err = %v", err)
	}
	if err := store.Complete(stale[1].ActivityID, "worker-a", stale[1].FencingToken); !errors.Is(err, execution.ErrStaleLease) {
		t.Fatalf("stale complete err = %v", err)
	}
	if err := store.Fail(stale[2].ActivityID, "worker-a", stale[2].FencingToken, true, "late failure"); !errors.Is(err, execution.ErrStaleLease) {
		t.Fatalf("stale fail err = %v", err)
	}
	if err := store.Complete(current[1].ActivityID, "worker-b", current[1].FencingToken); err != nil {
		t.Fatal(err)
	}
	if err := store.Complete(current[0].ActivityID, "worker-b", current[0].FencingToken); err != nil {
		t.Fatal(err)
	}
	if err := store.Complete(current[2].ActivityID, "worker-b", current[2].FencingToken); err != nil {
		t.Fatal(err)
	}

	_, _, events, err := store.Workflow(workflow.ID)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, event := range events {
		seen[event.Type] = true
	}
	for _, expected := range []string{
		"activity.stale_heartbeat_rejected",
		"activity.stale_completion_rejected",
		"activity.stale_failure_rejected",
	} {
		if !seen[expected] {
			t.Fatalf("missing %q in events %v", expected, eventTypes(events))
		}
	}
}

// An expired lease must be rejected even while it still looks claimable:
// status running, owner and fencing token unchanged, scheduler not yet run.
func TestExpiredLeaseRejectsMutationsBeforeReclaim(t *testing.T) {
	store := newStore(t, time.Second)
	namespace := uniqueNamespace(t)
	queue := namespace + "_queue"
	workflow, _, err := store.Start(execution.WorkflowDefinition{
		Namespace: namespace,
		Name:      "expired-lease-guard",
		Activities: []execution.ActivityDefinition{
			{Name: "heartbeat-target", TaskQueue: queue},
			{Name: "complete-target", TaskQueue: queue},
			{Name: "fail-target", TaskQueue: queue},
		},
	}, "")
	if err != nil {
		t.Fatal(err)
	}

	stale := make([]execution.Claim, 0, 3)
	for {
		claim, err := store.Claim("worker-a", queue)
		if errors.Is(err, execution.ErrNoRunnableActivity) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		stale = append(stale, claim)
	}
	if len(stale) != 3 {
		t.Fatalf("claims = %d, want 3", len(stale))
	}
	time.Sleep(1200 * time.Millisecond)

	// Deliberately no RunSchedulerPass: the lease is expired by database time
	// but not yet reclaimed, and must already be unusable.
	if err := store.Heartbeat(stale[0].ActivityID, "worker-a", stale[0].FencingToken); !errors.Is(err, execution.ErrStaleLease) {
		t.Fatalf("expired heartbeat err = %v, want ErrStaleLease", err)
	}
	if err := store.Complete(stale[1].ActivityID, "worker-a", stale[1].FencingToken); !errors.Is(err, execution.ErrStaleLease) {
		t.Fatalf("expired completion err = %v, want ErrStaleLease", err)
	}
	if err := store.Fail(stale[2].ActivityID, "worker-a", stale[2].FencingToken, true, "late failure"); !errors.Is(err, execution.ErrStaleLease) {
		t.Fatalf("expired failure err = %v, want ErrStaleLease", err)
	}

	got, activities, events, err := store.Workflow(workflow.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != execution.WorkflowRunning {
		t.Fatalf("workflow status = %s, want running", got.Status)
	}
	claimedExpiry := map[string]time.Time{}
	for _, claim := range stale {
		claimedExpiry[claim.ActivityID] = claim.LeaseExpiresAt
	}
	for _, activity := range activities {
		if activity.Status != execution.ActivityRunning || activity.Attempt != 1 || activity.LeaseOwner != "worker-a" {
			t.Fatalf("activity %s mutated by expired worker: %+v", activity.Name, activity)
		}
		if !activity.LeaseExpiresAt.Equal(claimedExpiry[activity.ID]) {
			t.Fatalf("activity %s lease changed by an expired worker: got %v want %v", activity.Name, activity.LeaseExpiresAt, claimedExpiry[activity.ID])
		}
	}
	seen := map[string]bool{}
	for _, event := range events {
		seen[event.Type] = true
	}
	for _, expected := range []string{
		"activity.stale_heartbeat_rejected",
		"activity.stale_completion_rejected",
		"activity.stale_failure_rejected",
	} {
		if !seen[expected] {
			t.Fatalf("missing %q in events %v", expected, eventTypes(events))
		}
	}

	// The scheduler can still reclaim the expired work afterwards.
	changed, err := store.RunSchedulerPass(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if changed != 3 {
		t.Fatalf("scheduler changed = %d, want 3", changed)
	}
	current := make([]execution.Claim, 0, 3)
	for {
		claim, err := store.Claim("worker-b", queue)
		if errors.Is(err, execution.ErrNoRunnableActivity) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		current = append(current, claim)
	}
	if len(current) != 3 {
		t.Fatalf("reclaims = %d, want 3", len(current))
	}
	for i, claim := range current {
		if claim.FencingToken <= stale[i].FencingToken {
			t.Fatalf("token did not advance for %s: old=%d new=%d", claim.Name, stale[i].FencingToken, claim.FencingToken)
		}
		if err := store.Complete(claim.ActivityID, "worker-b", claim.FencingToken); err != nil {
			t.Fatal(err)
		}
	}
	got, _, _, err = store.Workflow(workflow.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != execution.WorkflowCompleted {
		t.Fatalf("status = %s, want completed after reclaim", got.Status)
	}
}

func TestLeaseExpiryReclaimsActivity(t *testing.T) {
	store := newStore(t, time.Second)
	queue := uniqueNamespace(t) + "_queue"
	workflow, _, err := store.Start(execution.WorkflowDefinition{
		Namespace: uniqueNamespace(t),
		Name:      "lease-recovery",
		Activities: []execution.ActivityDefinition{{Name: "ship", TaskQueue: queue}},
	}, "")
	if err != nil {
		t.Fatal(err)
	}

	first, err := store.Claim("worker-a", queue)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(1200 * time.Millisecond)
	changed, err := store.RunSchedulerPass(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if changed != 1 {
		t.Fatalf("scheduler changed = %d, want 1", changed)
	}
	second, err := store.Claim("worker-b", queue)
	if err != nil {
		t.Fatal(err)
	}
	if second.FencingToken <= first.FencingToken {
		t.Fatalf("token did not advance: old=%d new=%d", first.FencingToken, second.FencingToken)
	}
	if err := store.Complete(second.ActivityID, "worker-b", second.FencingToken); err != nil {
		t.Fatal(err)
	}

	got, _, events, err := store.Workflow(workflow.ID)
	if err != nil {
		t.Fatal(err)
	}
	expected := []string{
		"workflow.started",
		"activity.scheduled",
		"activity.claimed",
		"activity.lease_expired",
		"activity.claimed",
		"activity.completed",
		"workflow.completed",
	}
	if got.Status != execution.WorkflowCompleted {
		t.Fatalf("status = %s, want completed", got.Status)
	}
	if !reflect.DeepEqual(eventTypes(events), expected) {
		t.Fatalf("event arc = %v, want %v", eventTypes(events), expected)
	}
}

func TestRetryExhaustionArc(t *testing.T) {
	store := newStore(t, 30*time.Second)
	queue := uniqueNamespace(t) + "_queue"
	workflow, _, err := store.Start(execution.WorkflowDefinition{
		Namespace: uniqueNamespace(t),
		Name:      "retry-exhaustion",
		Activities: []execution.ActivityDefinition{{
			Name:         "charge",
			TaskQueue:    queue,
			MaxAttempts:  2,
			RetryBackoff: time.Millisecond,
		}},
	}, "")
	if err != nil {
		t.Fatal(err)
	}

	first, err := store.Claim("worker-a", queue)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Fail(first.ActivityID, "worker-a", first.FencingToken, true, "gateway unavailable"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	changed, err := store.RunSchedulerPass(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if changed != 1 {
		t.Fatalf("scheduler changed = %d, want 1 retry release", changed)
	}
	second, err := store.Claim("worker-b", queue)
	if err != nil {
		t.Fatal(err)
	}
	if second.Attempt != 2 {
		t.Fatalf("attempt = %d, want 2", second.Attempt)
	}
	if err := store.Fail(second.ActivityID, "worker-b", second.FencingToken, true, "gateway unavailable again"); err != nil {
		t.Fatal(err)
	}

	got, activities, events, err := store.Workflow(workflow.ID)
	if err != nil {
		t.Fatal(err)
	}
	expected := []string{
		"workflow.started",
		"activity.scheduled",
		"activity.claimed",
		"activity.retry_scheduled",
		"activity.ready",
		"activity.claimed",
		"activity.failed",
		"workflow.failed",
	}
	if got.Status != execution.WorkflowFailed {
		t.Fatalf("status = %s, want failed", got.Status)
	}
	if len(activities) != 1 || activities[0].Status != execution.ActivityFailed || activities[0].Attempt != 2 {
		t.Fatalf("activities = %+v, want exhausted failed activity", activities)
	}
	if !reflect.DeepEqual(eventTypes(events), expected) {
		t.Fatalf("event arc = %v, want %v", eventTypes(events), expected)
	}
}
