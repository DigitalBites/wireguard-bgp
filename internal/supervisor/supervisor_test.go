package supervisor

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestSupervisorPing(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "supervisor.sock")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		errCh <- (Server{SocketPath: socketPath}).Serve(ctx)
	}()
	client := Client{SocketPath: socketPath, Timeout: time.Second}
	var resp Response
	var err error
	for i := 0; i < 50; i++ {
		resp, err = client.Call(context.Background(), ActionPing)
		if err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	if !resp.OK || resp.Output != "pong" {
		t.Fatalf("unexpected response: %#v", resp)
	}
	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("supervisor did not stop")
	}
}

func TestSupervisorRejectsUnknownAction(t *testing.T) {
	resp := (Server{}).dispatch(Request{Action: "shell"})
	if resp.OK || resp.Error == "" {
		t.Fatalf("expected rejected action, got %#v", resp)
	}
}
