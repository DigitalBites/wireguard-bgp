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
	if cfg.Runtime.AutoStart || cfg.Runtime.PinDashboardClientRoute {
		t.Fatalf("runtime options should default off: %#v", cfg.Runtime)
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
	if got.BIRD.LocalASN != 65060 || got.BIRD.PeerIP != "192.168.50.1" || got.Runtime.AutoStart {
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

func TestValidateManagedPathsRejectsUnsafeInterface(t *testing.T) {
	cfg := Default()
	cfg.BIRD.Interface = "wg0\";\ninjected"
	if err := ValidateManagedPaths(cfg); err == nil {
		t.Fatal("expected unsafe interface validation error")
	}
}

func TestValidateManagedPathsRejectsUnsafeMTU(t *testing.T) {
	cfg := Default()
	cfg.WireGuard.MTU = 1
	if err := ValidateManagedPaths(cfg); err == nil {
		t.Fatal("expected unsafe MTU validation error")
	}
}

func TestNormalizePinnedClientRoutes(t *testing.T) {
	got, err := NormalizePinnedClientRoutes([]string{"192.168.64.1", "192.168.64.1/32"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "192.168.64.1/32" {
		t.Fatalf("unexpected routes: %#v", got)
	}
}

func TestValidateRuntimeRejectsBroadPinnedClientRoute(t *testing.T) {
	err := ValidateRuntime(Runtime{PinnedClientRoutes: []string{"192.168.64.0/24"}})
	if err == nil {
		t.Fatal("expected broad pinned route error")
	}
}
