package wg

import (
	"bufio"
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
