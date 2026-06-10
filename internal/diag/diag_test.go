package diag

import (
	"context"
	"testing"
)

func TestCommandsAreAllowlisted(t *testing.T) {
	if _, ok := Commands["routes"]; !ok {
		t.Fatal("expected routes command")
	}
	if _, err := (Runner{}).Run(context.Background(), "rm -rf"); err == nil {
		t.Fatal("expected unknown command error")
	}
}
