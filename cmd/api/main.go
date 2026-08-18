package main

import (
	"context"
	"log"
	"net/http"

	"github.com/tyagiquamar/durablego/internal/api"
	"github.com/tyagiquamar/durablego/internal/config"
	"github.com/tyagiquamar/durablego/internal/persistence"
)

func main() {
	cfg := config.Load()
	store, err := persistence.NewPostgres(context.Background(), cfg.DatabaseURL, cfg.LeaseTTL, cfg.MaxAttempts)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()
	log.Printf("durablego api listening on %s", cfg.Addr)
	if err := http.ListenAndServe(cfg.Addr, api.New(store)); err != nil {
		log.Fatal(err)
	}
}
