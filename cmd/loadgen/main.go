package main

import (
	"flag"
	"fmt"
	"time"

	"github.com/tyagiquamar/durablego/internal/execution"
)

func main() {
	count := flag.Int("workflows", 100, "number of workflows")
	workers := flag.Int("workers", 4, "worker count used in the report")
	flag.Parse()

	engine := execution.New(30*time.Second, 3)
	started := time.Now()
	for i := 0; i < *count; i++ {
		_, _, _ = engine.Start(execution.WorkflowDefinition{
			Namespace: "bench",
			Name:      "order-processing",
			Activities: []execution.ActivityDefinition{
				{Name: "validate"},
				{Name: "payment", DependsOn: []string{"validate"}},
				{Name: "inventory", DependsOn: []string{"validate"}},
			},
		}, fmt.Sprintf("bench-%d", i))
	}
	elapsed := time.Since(started)
	fmt.Printf("durablego_loadgen workflows=%d workers=%d elapsed_ms=%d workflows_per_sec=%.2f\n", *count, *workers, elapsed.Milliseconds(), float64(*count)/elapsed.Seconds())
}
