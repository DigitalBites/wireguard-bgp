package bird

import (
	"bytes"
	"fmt"
	"net/netip"
	"strings"
	"text/template"
)

type Config struct {
	RouterID         string   `json:"routerId" yaml:"routerId"`
	LocalASN         uint32   `json:"localAsn" yaml:"localAsn"`
	PeerASN          uint32   `json:"peerAsn" yaml:"peerAsn"`
	PeerIP           string   `json:"peerIp" yaml:"peerIp"`
	Interface        string   `json:"interface" yaml:"interface"`
	AdvertisedRoutes []string `json:"advertisedRoutes" yaml:"advertisedRoutes"`
}

func (c Config) WithDefaults() Config {
	if c.Interface == "" {
		c.Interface = "wg0"
	}
	return c
}

func Generate(c Config) (string, error) {
	c = c.WithDefaults()
	if c.LocalASN == 0 {
		return "", fmt.Errorf("local ASN is required")
	}
	if c.PeerASN == 0 {
		return "", fmt.Errorf("peer ASN is required")
	}
	if _, err := netip.ParseAddr(c.PeerIP); err != nil {
		return "", fmt.Errorf("peer IP is invalid: %w", err)
	}
	if c.RouterID != "" {
		if _, err := netip.ParseAddr(c.RouterID); err != nil {
			return "", fmt.Errorf("router ID is invalid: %w", err)
		}
	}
	if len(c.AdvertisedRoutes) == 0 {
		return "", fmt.Errorf("at least one advertised route is required")
	}
	for _, route := range c.AdvertisedRoutes {
		if _, err := netip.ParsePrefix(route); err != nil {
			return "", fmt.Errorf("advertised route %q is invalid: %w", route, err)
		}
	}

	var out bytes.Buffer
	if err := birdTemplate.Execute(&out, c); err != nil {
		return "", err
	}
	return strings.TrimSpace(out.String()) + "\n", nil
}

var birdTemplate = template.Must(template.New("bird").Parse(`
log stderr all;
{{- if .RouterID }}
router id {{ .RouterID }};
{{- end }}

protocol device {
}

protocol direct {
    ipv4;
    interface "{{ .Interface }}";
}

protocol static advertised_routes {
    ipv4;
{{- range .AdvertisedRoutes }}
    route {{ . }} via "{{ $.Interface }}";
{{- end }}
}

protocol bgp peplink {
    local as {{ .LocalASN }};
    neighbor {{ .PeerIP }} as {{ .PeerASN }};
    ipv4 {
        import none;
        export where proto = "advertised_routes";
    };
}
`))
