package config

import (
	"path/filepath"
	"testing"
)

func TestLoadMissingReturnsDefaults(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "missing.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ListenAddr != ":8080" || cfg.ConfigDir != "/app-state" || cfg.BIRDConfigPath != "/app-state/bird/bird.conf" || cfg.WireGuard.Interface != "wg0" || cfg.WireGuard.MTU != 1320 {
		t.Fatalf("unexpected defaults: %#v", cfg)
	}
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.yaml")
	cfg := Default()
	cfg.BIRD.LocalASN = 65060
	cfg.BIRD.PeerASN = 65001
	cfg.BIRD.PeerIP = "192.168.50.1"
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.BIRD.LocalASN != 65060 || got.BIRD.PeerIP != "192.168.50.1" {
		t.Fatalf("unexpected loaded config: %#v", got)
	}
}

func TestValidateManagedPathsAllowsInternalState(t *testing.T) {
	cfg := Default()
	if err := ValidateManagedPaths(cfg); err != nil {
		t.Fatal(err)
	}
}

func TestValidateManagedPathsRejectsBirdConfigOutsideStateDir(t *testing.T) {
	cfg := Default()
	cfg.BIRDConfigPath = "/etc/bird.conf"
	if err := ValidateManagedPaths(cfg); err == nil {
		t.Fatal("expected path validation error")
	}
}

func TestValidateManagedPathsRejectsRelativeStateDir(t *testing.T) {
	cfg := Default()
	cfg.ConfigDir = "app-state"
	cfg.BIRDConfigPath = "app-state/bird/bird.conf"
	if err := ValidateManagedPaths(cfg); err == nil {
		t.Fatal("expected relative path validation error")
	}
}
