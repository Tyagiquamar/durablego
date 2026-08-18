package scheduler

import (
	"context"
	"log"
	"time"
)

type Backend interface {
	RunSchedulerPass() int
}

type Scheduler struct {
	Backend  Backend
	Interval time.Duration
	Logger   *log.Logger
}

func (s Scheduler) Run(ctx context.Context) error {
	interval := s.Interval
	if interval <= 0 {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			changed := s.Backend.RunSchedulerPass()
			if changed > 0 && s.Logger != nil {
				s.Logger.Printf("scheduler pass changed=%d", changed)
			}
		}
	}
}
