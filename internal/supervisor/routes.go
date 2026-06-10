package supervisor

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/netip"
	"os"
	"strings"

	"peplink-wg-bgp/internal/bird"
	"peplink-wg-bgp/internal/config"
	"peplink-wg-bgp/internal/wg"
)

const DefaultEndpointRouteInterface = "eth0"

type RouteManager interface {
	Apply(ctx context.Context) (string, error)
	PinClient(ctx context.Context, clientIP string) (string, error)
}

type RouteApplyManager struct {
	AppConfigPath string
	WGConfigPath  string
	Runner        CommandRunner
	Resolver      HostResolver
}

type HostResolver interface {
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
}

func (s Server) applyRoutes() Response {
	ctx, cancel := s.commandContext()
	defer cancel()
	out, err := s.routeManager().Apply(ctx)
	return routeResponse(ActionRoutesApply, out, err)
}

func (s Server) pinClientRoute(req Request) Response {
	ctx, cancel := s.commandContext()
	defer cancel()
	out, err := s.routeManager().PinClient(ctx, req.Params["clientIP"])
	return routeResponse(ActionRoutesPinClient, out, err)
}

func (s Server) routeManager() RouteManager {
	if s.RouteManager != nil {
		return s.RouteManager
	}
	return &RouteApplyManager{
		AppConfigPath: s.appConfigPath(),
		WGConfigPath:  DefaultWGConfigPath,
		Runner:        s.Runner,
	}
}

func (s Server) appConfigPath() string {
	if s.AppConfigPath == "" {
		return DefaultAppConfigPath
	}
	return s.AppConfigPath
}

func (m *RouteApplyManager) Apply(ctx context.Context) (string, error) {
	cfg, err := config.Load(m.appConfigPath())
	if err != nil {
		return "", fmt.Errorf("load app config: %w", err)
	}
	if err := config.ValidateManagedPaths(cfg); err != nil {
		return "", err
	}
	if _, err := bird.Generate(cfg.BIRD); err != nil {
		return "", err
	}
	wgData, err := os.ReadFile(m.wgConfigPath())
	if err != nil {
		return "", fmt.Errorf("read WireGuard config: %w", err)
	}
	wgMeta, err := wg.ValidateConfig(string(wgData))
	if err != nil {
		return "", err
	}
	endpointIPs, err := m.endpointAddrs(ctx, wgMeta.Endpoint)
	if err != nil {
		return "", err
	}

	var out bytes.Buffer
	for _, endpointIP := range endpointIPs {
		if err := m.applyHostRoute(ctx, &out, cfg, endpointIP); err != nil {
			return out.String(), err
		}
	}
	for _, route := range cfg.Runtime.PinnedClientRoutes {
		prefix, err := netip.ParsePrefix(route)
		if err != nil {
			return out.String(), fmt.Errorf("pinned client route %q is invalid: %w", route, err)
		}
		if !prefix.Addr().Is4() || prefix.Bits() != 32 {
			return out.String(), fmt.Errorf("pinned client route %q must be an IPv4 /32", route)
		}
		if err := m.applyHostRoute(ctx, &out, cfg, prefix.Addr()); err != nil {
			return out.String(), err
		}
	}
	for _, route := range cfg.BIRD.AdvertisedRoutes {
		prefix, err := netip.ParsePrefix(route)
		if err != nil {
			return out.String(), fmt.Errorf("advertised route %q is invalid: %w", route, err)
		}
		if err := m.runStep(ctx, &out, "ip", "route", "replace", prefix.String(), "dev", cfg.BIRD.WithDefaults().Interface); err != nil {
			return out.String(), err
		}
	}
	return out.String(), nil
}

func (m *RouteApplyManager) PinClient(ctx context.Context, clientIP string) (string, error) {
	addr, err := netip.ParseAddr(clientIP)
	if err != nil {
		return "", fmt.Errorf("client IP is invalid: %w", err)
	}
	if !addr.Is4() {
		return "", fmt.Errorf("client route pinning currently requires an IPv4 client")
	}
	cfg, err := config.Load(m.appConfigPath())
	if err != nil {
		return "", fmt.Errorf("load app config: %w", err)
	}
	var out bytes.Buffer
	if err := m.applyHostRoute(ctx, &out, cfg, addr); err != nil {
		return out.String(), err
	}
	return out.String(), nil
}

func (m *RouteApplyManager) applyHostRoute(ctx context.Context, out *bytes.Buffer, cfg config.App, addr netip.Addr) error {
	if !addr.Is4() {
		return fmt.Errorf("host route %s must be IPv4", addr)
	}
	iface := cfg.WireGuard.EndpointRouteInterface
	if iface == "" {
		iface = DefaultEndpointRouteInterface
	}
	route := addr.String() + "/32"
	via := strings.TrimSpace(cfg.WireGuard.EndpointRouteVia)
	if via == "" {
		defaultOut, err := m.runner().Run(ctx, "ip", "route", "show", "default", "dev", iface)
		if defaultOut != "" {
			out.WriteString(defaultOut)
			if !strings.HasSuffix(defaultOut, "\n") {
				out.WriteByte('\n')
			}
		}
		if err != nil {
			return err
		}
		via = defaultRouteGateway(defaultOut)
	}
	args := []string{"route", "replace", route}
	if via != "" {
		args = append(args, "via", via)
	}
	args = append(args, "dev", iface)
	return m.runStep(ctx, out, "ip", args...)
}

func (m *RouteApplyManager) endpointAddrs(ctx context.Context, endpoint string) ([]netip.Addr, error) {
	host, _, err := net.SplitHostPort(endpoint)
	if err != nil {
		return nil, fmt.Errorf("WireGuard endpoint must be host:port: %w", err)
	}
	addr, err := netip.ParseAddr(host)
	if err == nil {
		if !addr.Is4() {
			return nil, fmt.Errorf("WireGuard endpoint route pinning currently requires an IPv4 endpoint")
		}
		return []netip.Addr{addr}, nil
	}
	addrs, err := m.resolver().LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("resolve WireGuard endpoint host %q: %w", host, err)
	}
	resolved := make([]netip.Addr, 0, len(addrs))
	seen := map[netip.Addr]struct{}{}
	for _, candidate := range addrs {
		if candidate.IP == nil {
			continue
		}
		addr, ok := netip.AddrFromSlice(candidate.IP)
		if !ok {
			continue
		}
		addr = addr.Unmap()
		if !addr.Is4() {
			continue
		}
		if _, ok := seen[addr]; ok {
			continue
		}
		seen[addr] = struct{}{}
		resolved = append(resolved, addr)
	}
	if len(resolved) == 0 {
		return nil, fmt.Errorf("WireGuard endpoint host %q did not resolve to an IPv4 address", host)
	}
	return resolved, nil
}

func defaultRouteGateway(output string) string {
	fields := strings.Fields(output)
	for i, field := range fields {
		if field == "via" && i+1 < len(fields) {
			return fields[i+1]
		}
	}
	return ""
}

func (m *RouteApplyManager) runStep(ctx context.Context, out *bytes.Buffer, name string, args ...string) error {
	stepOut, err := m.runner().Run(ctx, name, args...)
	if stepOut != "" {
		out.WriteString(stepOut)
		if !strings.HasSuffix(stepOut, "\n") {
			out.WriteByte('\n')
		}
	}
	return err
}

func (m *RouteApplyManager) appConfigPath() string {
	if m.AppConfigPath == "" {
		return DefaultAppConfigPath
	}
	return m.AppConfigPath
}

func (m *RouteApplyManager) wgConfigPath() string {
	if m.WGConfigPath == "" {
		return DefaultWGConfigPath
	}
	return m.WGConfigPath
}

func (m *RouteApplyManager) runner() CommandRunner {
	if m.Runner == nil {
		return ExecRunner{}
	}
	return m.Runner
}

func (m *RouteApplyManager) resolver() HostResolver {
	if m.Resolver == nil {
		return net.DefaultResolver
	}
	return m.Resolver
}

func routeResponse(action string, out string, err error) Response {
	if err != nil {
		return Response{OK: false, Action: action, Output: out, Error: err.Error()}
	}
	return Response{OK: true, Action: action, Output: out}
}
