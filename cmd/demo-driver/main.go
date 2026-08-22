// Demo driver: submits realistic order-processing workflows through the live
// API so the hosted dashboard shows genuine executions instead of fixtures.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"
)

func main() {
	apiURL := envOr("DURABLEGO_API_URL", "http://127.0.0.1:8080")
	interval := envIntOr("DEMO_INTERVAL_SECS", 40)

	client := &http.Client{Timeout: 10 * time.Second}
	log.Printf("demo-driver started: api=%s interval=%ds", apiURL, interval)

	time.Sleep(15 * time.Second) // let sibling services finish booting

	for i := 0; ; i++ {
		if err := startWorkflow(client, apiURL, i); err != nil {
			log.Printf("workflow #%d failed: %v", i, err)
			time.Sleep(10 * time.Second)
			continue
		}
		log.Printf("workflow #%d submitted", i)
		time.Sleep(time.Duration(interval) * time.Second)
	}
}

// startWorkflow mirrors a small e-commerce pipeline: validate -> charge ->
// ship -> notify. Workers complete each activity through their handlers.
func startWorkflow(client *http.Client, apiURL string, seq int) error {
	body := map[string]any{
		"namespace":       "demo",
		"name":            "order-processing",
		"idempotency_key": fmt.Sprintf("demo-%d-%d", time.Now().Unix(), seq),
		"activities": []map[string]any{
			{"name": "validate"},
			{"name": "charge", "depends_on": []string{"validate"}},
			{"name": "ship", "depends_on": []string{"charge"}},
			{"name": "notify", "depends_on": []string{"ship"}},
		},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	res, err := client.Post(apiURL+"/v1/workflows", "application/json", bytes.NewReader(raw))
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		return fmt.Errorf("start workflow: %s", res.Status)
	}
	return nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envIntOr(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}
