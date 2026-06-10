package config

import (
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"peplink-wg-bgp/internal/bird"
)

type App struct {
	ListenAddr     string      `json:"listenAddr" yaml:"listenAddr"`
	ConfigDir      string      `json:"configDir" yaml:"configDir"`
	BIRDConfigPath string      `json:"birdConfigPath" yaml:"birdConfigPath"`
	Runtime        Runtime     `json:"runtime" yaml:"runtime"`
	WireGuard      WireGuard   `json:"wireGuard" yaml:"wireGuard"`
	BIRD           bird.Config `json:"bird" yaml:"bird"`
}

type Runtime struct {
	AutoStart               bool     `json:"autoStart" yaml:"autoStart"`
	PinDashboardClientRoute bool     `json:"pinDashboardClientRoute" yaml:"pinDashboardClientRoute"`
	PinnedClientRoutes      []string `json:"pinnedClientRoutes,omitempty" yaml:"pinnedClientRoutes,omitempty"`
}

type WireGuard struct {
	Interface              string `json:"interface" yaml:"interface"`
	MTU                    int    `json:"mtu" yaml:"mtu"`
	EndpointRouteInterface string `json:"endpointRouteInterface,omitempty" yaml:"endpointRouteInterface,omitempty"`
	EndpointRouteVia       string `json:"endpointRouteVia,omitempty" yaml:"endpointRouteVia,omitempty"`
}

func Default() App {
	return App{
		ListenAddr:     ":8080",
		ConfigDir:      "/app-state",
		BIRDConfigPath: "/app-state/bird/bird.conf",
		WireGuard: WireGuard{
			Interface:              "wg0",
			MTU:                    1320,
			EndpointRouteInterface: "eth0",
		},
		BIRD: bird.Config{
			Interface:        "wg0",
			AdvertisedRoutes: []string{"0.0.0.0/1", "128.0.0.0/1"},
		},
	}
}

func Load(path string) (App, error) {
	cfg := Default()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return App{}, err
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return App{}, err
	}
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = ":8080"
	}
	if cfg.ConfigDir == "" {
		cfg.ConfigDir = "/app-state"
	}
	if cfg.BIRDConfigPath == "" {
		cfg.BIRDConfigPath = filepath.Join(cfg.ConfigDir, "bird", "bird.conf")
	}
	if cfg.WireGuard.Interface == "" {
		cfg.WireGuard.Interface = "wg0"
	}
	if cfg.WireGuard.MTU == 0 {
		cfg.WireGuard.MTU = 1320
	}
	if cfg.WireGuard.EndpointRouteInterface == "" {
		cfg.WireGuard.EndpointRouteInterface = "eth0"
	}
	cfg.BIRD = cfg.BIRD.WithDefaults()
	return cfg, nil
}

func ValidateManagedPaths(cfg App) error {
	if cfg.ConfigDir == "" {
		return fmt.Errorf("config dir is required")
	}
	if !filepath.IsAbs(cfg.ConfigDir) {
		return fmt.Errorf("config dir must be absolute")
	}
	if err := requirePathInside(cfg.ConfigDir, filepath.Join(cfg.ConfigDir, "app.yaml"), "app config path"); err != nil {
		return err
	}
	if err := requirePathInside(cfg.ConfigDir, filepath.Join(cfg.ConfigDir, "wireguard", "wg0.conf"), "wireguard config path"); err != nil {
		return err
	}
	if err := requirePathInside(cfg.ConfigDir, cfg.BIRDConfigPath, "BIRD config path"); err != nil {
		return err
	}
	if err := bird.ValidateInterfaceName(cfg.WireGuard.Interface); err != nil {
		return fmt.Errorf("wireguard interface is invalid: %w", err)
	}
	if err := bird.ValidateInterfaceName(cfg.WireGuard.EndpointRouteInterface); err != nil {
		return fmt.Errorf("wireguard endpoint route interface is invalid: %w", err)
	}
	if cfg.WireGuard.MTU < 576 || cfg.WireGuard.MTU > 9000 {
		return fmt.Errorf("wireguard MTU must be 576-9000")
	}
	if err := ValidateRuntime(cfg.Runtime); err != nil {
		return err
	}
	if err := bird.ValidateInterfaceName(cfg.BIRD.WithDefaults().Interface); err != nil {
		return fmt.Errorf("BIRD interface is invalid: %w", err)
	}
	return nil
}

func ValidateRuntime(runtime Runtime) error {
	for _, route := range runtime.PinnedClientRoutes {
		if _, err := NormalizePinnedClientRoute(route); err != nil {
			return err
		}
	}
	return nil
}

func NormalizePinnedClientRoutes(routes []string) ([]string, error) {
	normalized := make([]string, 0, len(routes))
	seen := map[string]struct{}{}
	for _, route := range routes {
		prefix, err := NormalizePinnedClientRoute(route)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[prefix]; ok {
			continue
		}
		seen[prefix] = struct{}{}
		normalized = append(normalized, prefix)
	}
	return normalized, nil
}

func NormalizePinnedClientRoute(route string) (string, error) {
	route = strings.TrimSpace(route)
	if route == "" {
		return "", fmt.Errorf("pinned client route must not be empty")
	}
	if addr, err := netip.ParseAddr(route); err == nil {
		if !addr.Is4() {
			return "", fmt.Errorf("pinned client route %q must be IPv4", route)
		}
		return addr.String() + "/32", nil
	}
	prefix, err := netip.ParsePrefix(route)
	if err != nil {
		return "", fmt.Errorf("pinned client route %q is invalid: %w", route, err)
	}
	if !prefix.Addr().Is4() {
		return "", fmt.Errorf("pinned client route %q must be IPv4", route)
	}
	if prefix.Bits() != 32 {
		return "", fmt.Errorf("pinned client route %q must be a /32 host route", route)
	}
	return prefix.Masked().String(), nil
}

func Save(path string, cfg App) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func requirePathInside(root, path, label string) error {
	cleanRoot, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return fmt.Errorf("%s root is invalid: %w", label, err)
	}
	cleanPath, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return fmt.Errorf("%s is invalid: %w", label, err)
	}
	rel, err := filepath.Rel(cleanRoot, cleanPath)
	if err != nil {
		return fmt.Errorf("%s cannot be compared to state dir: %w", label, err)
	}
	if rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." || filepath.IsAbs(rel) {
		return fmt.Errorf("%s %q must stay under %q", label, cleanPath, cleanRoot)
	}
	return nil
}
