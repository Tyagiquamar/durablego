package scheduler

import (
	"context"
	"log"
	"time"
)

type Backend interface {
	RunSchedulerPass(ctx context.Context) (int, error)
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
			changed, err := s.Backend.RunSchedulerPass(ctx)
			if err != nil {
				if s.Logger != nil {
					s.Logger.Printf("scheduler pass error: %v", err)
				}
				continue
			}
			if changed > 0 && s.Logger != nil {
				s.Logger.Printf("scheduler pass changed=%d", changed)
			}
		}
	}
}
