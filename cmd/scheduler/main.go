package main

import (
	"context"
	"log"
	"os"

	"github.com/tyagiquamar/durablego/internal/config"
	"github.com/tyagiquamar/durablego/internal/persistence"
	"github.com/tyagiquamar/durablego/internal/scheduler"
)

func main() {
	cfg := config.Load()
	store, err := persistence.NewPostgres(context.Background(), cfg.DatabaseURL, cfg.LeaseTTL, cfg.MaxAttempts)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()
	log.Println("durablego scheduler started")
	if err := (scheduler.Scheduler{Backend: store, Logger: log.New(os.Stdout, "", log.LstdFlags)}).Run(context.Background()); err != nil {
		log.Fatal(err)
	}
}
