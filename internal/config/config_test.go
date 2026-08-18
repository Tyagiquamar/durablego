package config

import "testing"

func TestLoadUsesDefaults(t *testing.T) {
	t.Setenv("DURABLEGO_ADDR", "")
	t.Setenv("DURABLEGO_LEASE_SECONDS", "")

	cfg := Load()
	if cfg.Addr != ":8080" {
		t.Fatalf("addr = %q, want default", cfg.Addr)
	}
	if cfg.LeaseTTL == 0 {
		t.Fatal("lease ttl should be set")
	}
}

func TestLoadUsesEnvironmentOverrides(t *testing.T) {
	t.Setenv("DURABLEGO_ADDR", ":9090")
	t.Setenv("DURABLEGO_LEASE_SECONDS", "5")

	cfg := Load()
	if cfg.Addr != ":9090" {
		t.Fatalf("addr = %q", cfg.Addr)
	}
	if cfg.LeaseTTL.Seconds() != 5 {
		t.Fatalf("lease ttl = %s", cfg.LeaseTTL)
	}
}
