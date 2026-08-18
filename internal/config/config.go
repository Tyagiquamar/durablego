package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	Addr        string
	DatabaseURL string
	LeaseTTL    time.Duration
	MaxAttempts int
}

func Load() Config {
	return Config{
		Addr:        env("DURABLEGO_ADDR", ":8080"),
		DatabaseURL: env("DURABLEGO_DATABASE_URL", "postgres://durablego:durablego@localhost:5432/durablego?sslmode=disable"),
		LeaseTTL:    time.Duration(envInt("DURABLEGO_LEASE_SECONDS", 30)) * time.Second,
		MaxAttempts: envInt("DURABLEGO_MAX_ATTEMPTS", 3),
	}
}

func env(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func envInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}
