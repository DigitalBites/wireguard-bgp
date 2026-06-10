package supervisor

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type recordingRunner struct {
	name   string
	args   []string
	output string
	err    error
}

func (r *recordingRunner) Run(ctx context.Context, name string, args ...string) (string, error) {
	r.name = name
	r.args = append([]string(nil), args...)
	return r.output, r.err
}

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

func TestSupervisorBIRDReloadRunsFixedCommand(t *testing.T) {
	runner := &recordingRunner{output: "configured\n"}
	resp := (Server{Runner: runner}).dispatch(Request{Action: ActionBIRDReload})
	if !resp.OK {
		t.Fatalf("unexpected response: %#v", resp)
	}
	if runner.name != "birdc" || strings.Join(runner.args, " ") != "-s /run/bird/bird.ctl configure" {
		t.Fatalf("unexpected command: %s %#v", runner.name, runner.args)
	}
	if resp.Output != "configured\n" {
		t.Fatalf("unexpected output: %q", resp.Output)
	}
}

func TestSupervisorBIRDStartRunsFixedCommand(t *testing.T) {
	runner := &recordingRunner{output: "started\n"}
	resp := (Server{Runner: runner}).dispatch(Request{Action: ActionBIRDStart})
	if !resp.OK {
		t.Fatalf("unexpected response: %#v", resp)
	}
	if runner.name != "bird" || strings.Join(runner.args, " ") != "-c /app-state/bird/bird.conf -s /run/bird/bird.ctl" {
		t.Fatalf("unexpected command: %s %#v", runner.name, runner.args)
	}
}

func TestSupervisorBIRDStatusRunsFixedCommand(t *testing.T) {
	runner := &recordingRunner{output: "peplink BGP up\n"}
	resp := (Server{Runner: runner}).dispatch(Request{Action: ActionBIRDStatus})
	if !resp.OK {
		t.Fatalf("unexpected response: %#v", resp)
	}
	if runner.name != "birdc" || strings.Join(runner.args, " ") != "-s /run/bird/bird.ctl show protocols" {
		t.Fatalf("unexpected command: %s %#v", runner.name, runner.args)
	}
}

func TestSupervisorWGStatusRunsFixedCommand(t *testing.T) {
	runner := &recordingRunner{output: "interface: wg0\n"}
	resp := (Server{Runner: runner}).dispatch(Request{Action: ActionWGStatus})
	if !resp.OK {
		t.Fatalf("unexpected response: %#v", resp)
	}
	if runner.name != "wg" || strings.Join(runner.args, " ") != "show" {
		t.Fatalf("unexpected command: %s %#v", runner.name, runner.args)
	}
}

func TestSupervisorBIRDReloadReportsCommandFailure(t *testing.T) {
	runner := &recordingRunner{output: "socket failed\n", err: errors.New("exit status 1")}
	resp := (Server{Runner: runner}).dispatch(Request{Action: ActionBIRDReload})
	if resp.OK || resp.Output != "socket failed\n" || resp.Error == "" {
		t.Fatalf("unexpected response: %#v", resp)
	}
}
