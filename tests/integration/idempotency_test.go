package integration_test

import (
	"sync"
	"testing"
	"time"

	"github.com/tyagiquamar/durablego/internal/execution"
)

func TestConcurrentDuplicateStartReturnsSameWorkflow(t *testing.T) {
	engine := execution.New(time.Second, 3)
	def := execution.WorkflowDefinition{
		Namespace: "production",
		Name:      "order-processing",
		Activities: []execution.ActivityDefinition{
			{Name: "validate", TaskQueue: "default"},
			{Name: "payment", TaskQueue: "payments", DependsOn: []string{"validate"}},
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
			workflow, duplicate, err := engine.Start(def, "order-129331")
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
	workflows, err := engine.ListWorkflows()
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, workflow := range workflows {
		if workflow.Namespace == def.Namespace && workflow.IdempotencyKey == "order-129331" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("persisted workflows for key = %d, want 1", count)
	}
}
