package worker

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/tyagiquamar/durablego/internal/execution"
)

type Handler func(context.Context, execution.Claim) error

type Backend interface {
	Claim(workerID, taskQueue string) (execution.Claim, error)
	Complete(activityID, workerID string, token int64) error
	Fail(activityID, workerID string, token int64, retryable bool, message string) error
}

type Runtime struct {
	Backend      Backend
	WorkerID     string
	TaskQueue    string
	Concurrency  int
	PollInterval time.Duration
	Handlers     map[string]Handler
	Logger       *log.Logger
}

func (r *Runtime) Run(ctx context.Context) error {
	if r.Concurrency <= 0 {
		r.Concurrency = 1
	}
	if r.PollInterval <= 0 {
		r.PollInterval = 250 * time.Millisecond
	}
	sem := make(chan struct{}, r.Concurrency)
	var wg sync.WaitGroup
	defer wg.Wait()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case sem <- struct{}{}:
			claim, err := r.Backend.Claim(r.WorkerID, r.TaskQueue)
			if errors.Is(err, execution.ErrNoRunnableActivity) {
				<-sem
				time.Sleep(r.PollInterval)
				continue
			}
			if err != nil {
				<-sem
				return err
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer func() { <-sem }()
				r.execute(ctx, claim)
			}()
		}
	}
}

func (r *Runtime) execute(ctx context.Context, claim execution.Claim) {
	handler := r.Handlers[claim.Name]
	if handler == nil {
		_ = r.Backend.Fail(claim.ActivityID, r.WorkerID, claim.FencingToken, false, "no handler registered")
		return
	}
	if err := handler(ctx, claim); err != nil {
		_ = r.Backend.Fail(claim.ActivityID, r.WorkerID, claim.FencingToken, true, err.Error())
		return
	}
	if err := r.Backend.Complete(claim.ActivityID, r.WorkerID, claim.FencingToken); err != nil && r.Logger != nil {
		r.Logger.Printf("complete rejected activity=%s err=%v", claim.ActivityID, err)
	}
}
