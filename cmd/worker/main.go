package main

import (
	"context"
	"log"
	"time"

	"github.com/tyagiquamar/durablego/internal/config"
	"github.com/tyagiquamar/durablego/internal/execution"
	"github.com/tyagiquamar/durablego/internal/persistence"
	"github.com/tyagiquamar/durablego/pkg/worker"
)

func main() {
	cfg := config.Load()
	store, err := persistence.NewPostgres(context.Background(), cfg.DatabaseURL, cfg.LeaseTTL, cfg.MaxAttempts)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()
	log.Println("durablego demo worker started")
	runtime := worker.Runtime{
		Backend:     store,
		WorkerID:    cfg.WorkerID,
		Concurrency: 2,
		Handlers: map[string]worker.Handler{
			"validate": func(context.Context, execution.Claim) error { return nil },
			"payment": func(context.Context, execution.Claim) error {
				time.Sleep(100 * time.Millisecond)
				return nil
			},
			"inventory": func(context.Context, execution.Claim) error { return nil },
			"email":     func(context.Context, execution.Claim) error { return nil },
			"analytics": func(context.Context, execution.Claim) error { return nil },
		},
		Logger: log.Default(),
	}
	if err := runtime.Run(context.Background()); err != nil {
		log.Fatal(err)
	}
}
