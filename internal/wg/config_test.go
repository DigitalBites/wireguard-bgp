package wg

import (
	"strings"
	"testing"
)

func TestParseConfigExtractsMetadataWithoutSecrets(t *testing.T) {
	input := `[Interface]
	PrivateKey = AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=
Address = 10.0.15.7/32

[Peer]
	PublicKey = AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=
	PresharedKey = AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=
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
	if got := RedactKey(meta.PeerPublicKey); got != "AAAAAA...AAAAA=" {
		t.Fatalf("unexpected redaction: %q", got)
	}
}

func TestValidateConfigRejectsInvalidAddress(t *testing.T) {
	_, err := ValidateConfig(`[Interface]
PrivateKey = AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=
Address = not-a-prefix

[Peer]
PublicKey = AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=
`)
	if err == nil {
		t.Fatal("expected invalid address error")
	}
}

func TestValidateConfigRejectsUnsupportedDirective(t *testing.T) {
	_, err := ValidateConfig(`[Interface]
PrivateKey = AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=
PostUp = rm -rf /

[Peer]
PublicKey = AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=
`)
	if err == nil {
		t.Fatal("expected unsupported directive error")
	}
}

func TestSetconfConfigStripsAddress(t *testing.T) {
	got := SetconfConfig(`[Interface]
PrivateKey = AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=
Address = 10.0.15.7/32

[Peer]
PublicKey = AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=
`)
	if strings.Contains(got, "Address") {
		t.Fatalf("setconf config still contains Address:\n%s", got)
	}
	if !strings.Contains(got, "PrivateKey") || !strings.Contains(got, "[Peer]") {
		t.Fatalf("setconf config dropped required values:\n%s", got)
	}
}
