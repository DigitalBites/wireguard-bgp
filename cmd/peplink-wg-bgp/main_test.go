package main

import (
	"os"
	"path/filepath"
	"testing"

	"peplink-wg-bgp/internal/config"
)

func TestMissingAutoStartConfigRequiresWireGuardAndBIRDConfigs(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.ConfigDir = dir
	cfg.BIRDConfigPath = filepath.Join(dir, "bird", "bird.conf")
	missing, err := missingAutoStartConfig(cfg)
	if err == nil || missing != filepath.Join(dir, "wireguard", "wg0.conf") {
		t.Fatalf("expected missing WireGuard config, got missing=%q err=%v", missing, err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "wireguard"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "wireguard", "wg0.conf"), []byte("wg"), 0o600); err != nil {
		t.Fatal(err)
	}
	missing, err = missingAutoStartConfig(cfg)
	if err == nil || missing != cfg.BIRDConfigPath {
		t.Fatalf("expected missing BIRD config, got missing=%q err=%v", missing, err)
	}
	if err := os.MkdirAll(filepath.Dir(cfg.BIRDConfigPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg.BIRDConfigPath, []byte("bird"), 0o600); err != nil {
		t.Fatal(err)
	}
	missing, err = missingAutoStartConfig(cfg)
	if err != nil || missing != "" {
		t.Fatalf("expected configs ready, got missing=%q err=%v", missing, err)
	}
}
