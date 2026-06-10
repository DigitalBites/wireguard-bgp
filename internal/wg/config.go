package wg

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"
)

type ConfigMeta struct {
	InterfaceAddress    string   `json:"interfaceAddress,omitempty"`
	Endpoint            string   `json:"endpoint,omitempty"`
	AllowedIPs          []string `json:"allowedIPs,omitempty"`
	PeerPublicKey       string   `json:"peerPublicKey,omitempty"`
	HasPrivateKey       bool     `json:"hasPrivateKey"`
	HasPresharedKey     bool     `json:"hasPresharedKey"`
	PersistentKeepalive string   `json:"persistentKeepalive,omitempty"`
}

func ParseConfig(input string) ConfigMeta {
	var meta ConfigMeta
	section := ""
	scanner := bufio.NewScanner(strings.NewReader(input))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.ToLower(strings.TrimSpace(line[1 : len(line)-1]))
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		switch section + "." + key {
		case "interface.privatekey":
			meta.HasPrivateKey = value != ""
		case "interface.address":
			meta.InterfaceAddress = value
		case "peer.publickey":
			meta.PeerPublicKey = value
		case "peer.presharedkey":
			meta.HasPresharedKey = value != ""
		case "peer.endpoint":
			meta.Endpoint = value
		case "peer.allowedips":
			meta.AllowedIPs = splitCSV(value)
		case "peer.persistentkeepalive":
			meta.PersistentKeepalive = value
		}
	}
	return meta
}

func ValidateConfig(input string) (ConfigMeta, error) {
	meta := ParseConfig(input)
	if !meta.HasPrivateKey || meta.PeerPublicKey == "" {
		return meta, fmt.Errorf("wireguard config must include interface private key and peer public key")
	}
	section := ""
	scanner := bufio.NewScanner(strings.NewReader(input))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.ToLower(strings.TrimSpace(line[1 : len(line)-1]))
			if section != "interface" && section != "peer" {
				return meta, fmt.Errorf("unsupported WireGuard section %q", section)
			}
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return meta, fmt.Errorf("invalid WireGuard config line %q", line)
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		if value == "" {
			return meta, fmt.Errorf("WireGuard %s.%s must not be empty", section, key)
		}
		if err := validateDirective(section, key, value); err != nil {
			return meta, err
		}
	}
	if err := scanner.Err(); err != nil {
		return meta, err
	}
	return meta, nil
}

func SetconfConfig(input string) string {
	var out []string
	section := ""
	scanner := bufio.NewScanner(strings.NewReader(input))
	for scanner.Scan() {
		raw := scanner.Text()
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.ToLower(strings.TrimSpace(line[1 : len(line)-1]))
			out = append(out, raw)
			continue
		}
		key, _, ok := strings.Cut(line, "=")
		if ok && section == "interface" && strings.EqualFold(strings.TrimSpace(key), "Address") {
			continue
		}
		out = append(out, raw)
	}
	return strings.TrimSpace(strings.Join(out, "\n")) + "\n"
}

func validateDirective(section, key, value string) error {
	switch section + "." + key {
	case "interface.privatekey", "peer.publickey", "peer.presharedkey":
		return validateKey(section+"."+key, value)
	case "interface.address":
		if strings.Contains(value, ",") {
			return fmt.Errorf("WireGuard interface address must contain exactly one prefix")
		}
		if _, err := netip.ParsePrefix(value); err != nil {
			return fmt.Errorf("WireGuard interface address is invalid: %w", err)
		}
	case "interface.listenport":
		return validatePort("WireGuard listen port", value)
	case "interface.fwmark":
		if value == "off" {
			return nil
		}
		if _, err := strconv.ParseUint(value, 0, 32); err != nil {
			return fmt.Errorf("WireGuard fwmark is invalid: %w", err)
		}
	case "peer.endpoint":
		return validateEndpoint(value)
	case "peer.allowedips":
		for _, prefix := range splitCSV(value) {
			if _, err := netip.ParsePrefix(prefix); err != nil {
				return fmt.Errorf("WireGuard allowed IP %q is invalid: %w", prefix, err)
			}
		}
	case "peer.persistentkeepalive":
		n, err := strconv.Atoi(value)
		if err != nil || n < 0 || n > 65535 {
			return fmt.Errorf("WireGuard persistent keepalive must be 0-65535")
		}
	default:
		return fmt.Errorf("unsupported WireGuard directive %s.%s", section, key)
	}
	return nil
}

func validateKey(label, value string) error {
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil || len(decoded) != 32 {
		return fmt.Errorf("%s must be a 32-byte base64 WireGuard key", label)
	}
	return nil
}

func validateEndpoint(value string) error {
	host, port, err := net.SplitHostPort(value)
	if err != nil {
		return fmt.Errorf("WireGuard endpoint must be host:port: %w", err)
	}
	if host == "" || strings.ContainsAny(host, " \t\r\n") {
		return fmt.Errorf("WireGuard endpoint host is invalid")
	}
	return validatePort("WireGuard endpoint port", port)
}

func validatePort(label, value string) error {
	n, err := strconv.Atoi(value)
	if err != nil || n < 1 || n > 65535 {
		return fmt.Errorf("%s must be 1-65535", label)
	}
	return nil
}

func RedactKey(key string) string {
	if key == "" {
		return ""
	}
	if len(key) <= 12 {
		return "redacted"
	}
	return key[:6] + "..." + key[len(key)-6:]
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
