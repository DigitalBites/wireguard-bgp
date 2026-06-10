package bird

import (
	"strings"
	"testing"
)

func TestGenerateBirdConfig(t *testing.T) {
	got, err := Generate(Config{
		RouterID:         "192.168.50.67",
		LocalASN:         65060,
		PeerASN:          65001,
		PeerIP:           "192.168.50.1",
		AdvertisedRoutes: []string{"0.0.0.0/1", "128.0.0.0/1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"router id 192.168.50.67;",
		"local as 65060;",
		"neighbor 192.168.50.1 as 65001;",
		`route 0.0.0.0/1 via "wg0";`,
		`route 128.0.0.0/1 via "wg0";`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("generated config missing %q:\n%s", want, got)
		}
	}
}

func TestGenerateRejectsInvalidRoute(t *testing.T) {
	_, err := Generate(Config{
		LocalASN:         65060,
		PeerASN:          65001,
		PeerIP:           "192.168.50.1",
		AdvertisedRoutes: []string{"not-a-route"},
	})
	if err == nil {
		t.Fatal("expected invalid route error")
	}
}
