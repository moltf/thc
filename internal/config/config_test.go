package config

import "testing"

func TestDefault(t *testing.T) {
	cfg := Default()

	if cfg.Workers != 10 {
		t.Fatal("unexpected workers")
	}

	if cfg.Retries != 3 {
		t.Fatal("unexpected retries")
	}

	if cfg.Format != "txt" {
		t.Fatal("unexpected format")
	}
}
