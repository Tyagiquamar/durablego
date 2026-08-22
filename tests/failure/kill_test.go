package failure_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tyagiquamar/durablego/internal/api"
	"github.com/tyagiquamar/durablego/internal/execution"
	"github.com/tyagiquamar/durablego/internal/persistence"
	"github.com/tyagiquamar/durablego/internal/testdb"
)

// TestWorkerProcessKillRecovery kills REAL worker processes mid-claim and
// proves the full fencing arc against PostgreSQL through the live API:
//
//	doomed claims + holds  -> SIGKILL -> lease expires -> reaper recovers
//	heir1 claims + holds   -> doomed's late completion is rejected (409)
//	heir1 killed           -> recovered again -> heir2 completes cleanly
//
// Each run gets a unique task queue so parallel/leftover state can never be
// claimed across runs.
func TestWorkerProcessKillRecovery(t *testing.T) {
	if testing.Short() {
		t.Skip("process-kill failure scene requires PostgreSQL; run make test-pg")
	}

	ctx := context.Background()
	pgStore, err := persistence.NewPostgres(ctx, testdb.Shared(t), 3*time.Second, 3)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pgStore.Close)

	server := httptest.NewServer(api.New(pgStore))
	t.Cleanup(server.Close)

	queue := fmt.Sprintf("kill-scene-%d", time.Now().UnixNano())
	workflow, _, err := pgStore.Start(execution.WorkflowDefinition{
		Namespace: strings.ReplaceAll(queue, "-", "_"),
		Name:      "order-processing",
		Activities: []execution.ActivityDefinition{
			{Name: "long_task", TaskQueue: queue},
		},
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	// Never leave hold-loop workers orphaned on a failed assertion.
	t.Cleanup(func() { _ = pgStore.Cancel(workflow.ID) })

	bin := buildTestWorker(t)

	spawn := func(workerID string, hold bool) *exec.Cmd {
		t.Helper()
		args := []string{
			"-api-url", server.URL,
			"-worker-id", workerID,
			"-task-queue", queue,
			"-heartbeat-interval", "500ms",
		}
		if hold {
			args = append(args, "-hold")
		}
		cmd := exec.Command(bin, args...)
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out
		if err := cmd.Start(); err != nil {
			t.Fatalf("start worker %s: %v", workerID, err)
		}
		t.Cleanup(func() {
			_ = cmd.Process.Kill()
			// Wait (not Process.Wait) joins the io-copy goroutines writing
			// into out; reading a bytes.Buffer while exec still copies is
			// exactly the race the detector flags here.
			_ = cmd.Wait()
			if out.Len() > 0 && t.Failed() {
				t.Logf("worker %s output:\n%s", workerID, out.String())
			}
		})
		return cmd
	}

	reap := func() {
		t.Helper()
		time.Sleep(4 * time.Second) // lease TTL is 3s; let it lapse for real
		changed, err := pgStore.RunSchedulerPass(context.Background())
		if err != nil {
			t.Fatalf("scheduler pass: %v", err)
		}
		if changed < 1 {
			t.Fatalf("scheduler pass recovered %d activities, want >=1", changed)
		}
	}

	// 1. Doomed worker claims and holds.
	doomed := spawn("doomed", true)
	doomedClaim := waitForClaim(t, pgStore, workflow.ID, "doomed", 15*time.Second)

	// 2. SIGKILL mid-hold; no cleanup, no final heartbeat.
	if err := doomed.Process.Kill(); err != nil {
		t.Fatalf("kill doomed: %v", err)
	}
	// cmd.Wait (not cmd.Process.Wait): joins exec's stdout-copy goroutine so
	// the captured buffer is safe to read afterwards.
	_ = doomed.Wait()
	reap()

	// 3. Heir1 claims the recovered activity with a higher token.
	heir1 := spawn("heir1", true)
	heir1Claim := waitForClaim(t, pgStore, workflow.ID, "heir1", 15*time.Second)
	if heir1Claim.FencingToken <= doomedClaim.FencingToken {
		t.Fatalf("heir token %d must exceed doomed token %d", heir1Claim.FencingToken, doomedClaim.FencingToken)
	}

	// 4. The dead worker reports completion with its old token while heir1
	// holds a newer live lease: the API must reject with 409.
	res, err := http.Post(server.URL+"/v1/worker/complete", "application/json",
		bytes.NewReader(mustJSONBytes(leaseRequest{
			ActivityID:   doomedClaim.ActivityID,
			WorkerID:     "doomed",
			FencingToken: doomedClaim.FencingToken,
		})))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("stale completion status = %d body=%s, want 409", res.StatusCode, body)
	}
	assertEvent(t, pgStore, workflow.ID, "activity.stale_completion_rejected")

	// 5. Kill heir1 too; recover once more and let heir2 complete cleanly.
	if err := heir1.Process.Kill(); err != nil {
		t.Fatalf("kill heir1: %v", err)
	}
	_ = heir1.Wait()
	reap()

	heir2 := spawn("heir2", false)
	if err := heir2.Wait(); err != nil {
		t.Fatalf("heir2 run: %v", err)
	}

	// 6. Final state: completed by heir2 despite two mid-flight crashes.
	_, activities, events, err := pgStore.Workflow(workflow.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(activities) != 1 || !strings.Contains(strings.ToLower(fmt.Sprint(activities[0].Status)), "completed") {
		t.Fatalf("final activity status=%v attempt=%d, want completed", activityStatuses(activities), activities[0].Attempt)
	}
	expiries := 0
	for _, e := range events {
		if e.Type == "activity.lease_expired" {
			expiries++
		}
	}
	if expiries < 2 {
		t.Fatalf("lease_expired events = %d, want >=2 (two crash-recovery rounds)", expiries)
	}
}

type leaseRequest struct {
	ActivityID   string `json:"activity_id"`
	WorkerID     string `json:"worker_id"`
	FencingToken int64  `json:"fencing_token"`
}

func mustJSONBytes(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

func buildTestWorker(t *testing.T) string {
	t.Helper()
	// Not t.TempDir(): Windows keeps a killed child's executable image locked
	// for a while, which would make TempDir's strict RemoveAll fail the test.
	dir, err := os.MkdirTemp("", "durablego-failure-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) }) // best effort only
	bin := filepath.Join(dir, "durablego-testworker.exe")
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/testworker")
	cmd.Dir = repoRoot(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build testworker: %v\n%s", err, out)
	}
	return bin
}

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(wd, "..", "..")
}

func waitForClaim(t *testing.T, store *persistence.Postgres, workflowID, owner string, timeout time.Duration) execution.Claim {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		_, activities, _, err := store.Workflow(workflowID)
		if err != nil {
			t.Fatal(err)
		}
		for _, a := range activities {
			if strings.EqualFold(fmt.Sprint(a.Status), "running") && a.LeaseOwner == owner {
				return execution.Claim{ActivityID: a.ID, FencingToken: a.FencingToken}
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("worker %q never claimed within %s", owner, timeout)
	return execution.Claim{}
}

func assertEvent(t *testing.T, store *persistence.Postgres, workflowID, eventType string) {
	t.Helper()
	_, _, events, err := store.Workflow(workflowID)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range events {
		if e.Type == eventType {
			return
		}
	}
	types := make([]string, 0, len(events))
	for _, e := range events {
		types = append(types, e.Type)
	}
	t.Fatalf("event %q missing from arc %v", eventType, types)
}

func activityStatuses(activities []execution.Activity) []string {
	out := make([]string, 0, len(activities))
	for _, a := range activities {
		out = append(out, fmt.Sprint(a.Status))
	}
	return out
}
