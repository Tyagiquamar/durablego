package config

import "testing"

func TestLoadUsesDefaults(t *testing.T) {
	t.Setenv("DURABLEGO_ADDR", "")
	t.Setenv("PORT", "")
	t.Setenv("DURABLEGO_LEASE_SECONDS", "")

	cfg := Load()
	if cfg.Addr != ":8080" {
		t.Fatalf("addr = %q, want default", cfg.Addr)
	}
	if cfg.LeaseTTL == 0 {
		t.Fatal("lease ttl should be set")
	}
	if cfg.WorkerID != "demo-worker" {
		t.Fatalf("worker id = %q, want default", cfg.WorkerID)
	}
}

func TestLoadUsesEnvironmentOverrides(t *testing.T) {
	t.Setenv("DURABLEGO_ADDR", ":9090")
	t.Setenv("PORT", "8088")
	t.Setenv("DURABLEGO_LEASE_SECONDS", "5")
	t.Setenv("DURABLEGO_WORKER_ID", "worker-a")

	cfg := Load()
	if cfg.Addr != ":9090" {
		t.Fatalf("addr = %q", cfg.Addr)
	}
	if cfg.LeaseTTL.Seconds() != 5 {
		t.Fatalf("lease ttl = %s", cfg.LeaseTTL)
	}
	if cfg.WorkerID != "worker-a" {
		t.Fatalf("worker id = %q", cfg.WorkerID)
	}
}

func TestLoadUsesPlatformPortWhenAddressIsUnset(t *testing.T) {
	t.Setenv("DURABLEGO_ADDR", "")
	t.Setenv("PORT", "8088")

	if got := Load().Addr; got != ":8088" {
		t.Fatalf("addr = %q, want platform port", got)
	}
}
