package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/tyagiquamar/durablego/internal/execution"
)

type pollRequest struct {
	WorkerID  string `json:"worker_id"`
	TaskQueue string `json:"task_queue"`
}

type leaseRequest struct {
	ActivityID   string `json:"activity_id"`
	WorkerID     string `json:"worker_id"`
	FencingToken int64  `json:"fencing_token"`
}

func main() {
	apiURL := flag.String("api-url", "http://127.0.0.1:8080", "durablego api base url")
	workerID := flag.String("worker-id", "test-worker", "worker identity")
	taskQueue := flag.String("task-queue", "", "task queue to poll")
	heartbeatInterval := flag.Duration("heartbeat-interval", 10*time.Second, "interval between heartbeats")
	hold := flag.Bool("hold", false, "claim and hold the lease without completing")
	flag.Parse()

	client := &http.Client{Timeout: 5 * time.Second}
	claim, ok := poll(client, *apiURL, *workerID, *taskQueue)
	if !ok {
		fmt.Println("NO_WORK")
		os.Exit(1)
	}
	if err := post(client, *apiURL+"/v1/worker/heartbeat", leaseRequest{
		ActivityID:   claim.ActivityID,
		WorkerID:     *workerID,
		FencingToken: claim.FencingToken,
	}); err != nil {
		fmt.Printf("HEARTBEAT_ERROR %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("CLAIMED {\"activity_id\":%q,\"fencing_token\":%d}\n", claim.ActivityID, claim.FencingToken)

	if !*hold {
		if err := post(client, *apiURL+"/v1/worker/complete", leaseRequest{
			ActivityID:   claim.ActivityID,
			WorkerID:     *workerID,
			FencingToken: claim.FencingToken,
		}); err != nil {
			fmt.Printf("COMPLETE_ERROR %v\n", err)
			os.Exit(1)
		}
		return
	}
	for {
		time.Sleep(*heartbeatInterval)
		if err := post(client, *apiURL+"/v1/worker/heartbeat", leaseRequest{
			ActivityID:   claim.ActivityID,
			WorkerID:     *workerID,
			FencingToken: claim.FencingToken,
		}); err != nil {
			os.Exit(1)
		}
	}
}

func poll(client *http.Client, apiURL, workerID, taskQueue string) (execution.Claim, bool) {
	res, err := client.Post(apiURL+"/v1/worker/poll", "application/json", mustJSON(pollRequest{
		WorkerID:  workerID,
		TaskQueue: taskQueue,
	}))
	if err != nil {
		fmt.Printf("POLL_ERROR %v\n", err)
		os.Exit(1)
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNoContent {
		return execution.Claim{}, false
	}
	var claim execution.Claim
	if err := json.NewDecoder(res.Body).Decode(&claim); err != nil {
		fmt.Printf("POLL_DECODE_ERROR %v\n", err)
		os.Exit(1)
	}
	return claim, true
}

func post(client *http.Client, url string, body any) error {
	res, err := client.Post(url, "application/json", mustJSON(body))
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		return fmt.Errorf("%s", res.Status)
	}
	return nil
}

func mustJSON(value any) *bytes.Reader {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return bytes.NewReader(data)
}
