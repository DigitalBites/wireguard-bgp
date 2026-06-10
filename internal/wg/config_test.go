package wg

import "testing"

func TestParseConfigExtractsMetadataWithoutSecrets(t *testing.T) {
	input := `[Interface]
PrivateKey = private-value
Address = 10.0.15.7/32

[Peer]
PublicKey = abcdefghijklmnopqrstuvwxyz1234567890
PresharedKey = secret
Endpoint = 172.17.62.1:51820
AllowedIPs = 0.0.0.0/0, 10.0.0.0/8
PersistentKeepalive = 25
`
	meta := ParseConfig(input)
	if !meta.HasPrivateKey || !meta.HasPresharedKey {
		t.Fatalf("expected private and preshared keys to be detected")
	}
	if meta.InterfaceAddress != "10.0.15.7/32" {
		t.Fatalf("unexpected address: %q", meta.InterfaceAddress)
	}
	if meta.Endpoint != "172.17.62.1:51820" {
		t.Fatalf("unexpected endpoint: %q", meta.Endpoint)
	}
	if len(meta.AllowedIPs) != 2 || meta.AllowedIPs[1] != "10.0.0.0/8" {
		t.Fatalf("unexpected allowed IPs: %#v", meta.AllowedIPs)
	}
	if got := RedactKey(meta.PeerPublicKey); got != "abcdef...567890" {
		t.Fatalf("unexpected redaction: %q", got)
	}
}
