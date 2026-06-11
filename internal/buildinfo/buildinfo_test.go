package buildinfo

import (
	"testing"
	"time"
)

func TestResolveUsesProvidedVersion(t *testing.T) {
	if got := Resolve(" v0.1.0-arm64 ", time.Time{}); got != "v0.1.0-arm64" {
		t.Fatalf("unexpected version: %q", got)
	}
}

func TestResolveDefaultsToLocalTimestamp(t *testing.T) {
	now := time.Date(2026, 6, 11, 1, 2, 3, 0, time.UTC)
	if got := Resolve("", now); got != "local-2026-06-11T01:02:03Z" {
		t.Fatalf("unexpected local version: %q", got)
	}
}
