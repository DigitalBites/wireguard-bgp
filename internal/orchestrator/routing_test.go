package orchestrator

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"peplink-wg-bgp/internal/supervisor"
)

type recordingClient struct {
	calls []string
	fail  string
}

func (c *recordingClient) Call(ctx context.Context, action string) (supervisor.Response, error) {
	c.calls = append(c.calls, action)
	if action == c.fail {
		return supervisor.Response{OK: false, Action: action, Error: "failed"}, errors.New("failed")
	}
	if action == supervisor.ActionWGStatus {
		return supervisor.Response{OK: true, Action: action, Output: "interface: wg0\n"}, nil
	}
	return supervisor.Response{OK: true, Action: action, Output: action + "\n"}, nil
}

type downStatusClient struct {
	recordingClient
}

func (c *downStatusClient) Call(ctx context.Context, action string) (supervisor.Response, error) {
	c.calls = append(c.calls, action)
	if action == supervisor.ActionWGStatus {
		return supervisor.Response{OK: true, Action: action, Output: ""}, nil
	}
	return supervisor.Response{OK: true, Action: action, Output: action + "\n"}, nil
}

func TestRoutingApplyRestartsWhenWireGuardIsUp(t *testing.T) {
	client := &recordingClient{}
	result := (Routing{Client: client}).Apply(context.Background())
	want := []string{supervisor.ActionWGStatus, supervisor.ActionWGRestart, supervisor.ActionRoutesApply, supervisor.ActionBIRDStart, supervisor.ActionBIRDReload}
	if !result.OK || !reflect.DeepEqual(client.calls, want) {
		t.Fatalf("calls=%#v result=%#v", client.calls, result)
	}
}

func TestRoutingApplyStartsWhenWireGuardIsDown(t *testing.T) {
	client := &downStatusClient{}
	result := (Routing{Client: client}).Apply(context.Background())
	want := []string{supervisor.ActionWGStatus, supervisor.ActionWGStart, supervisor.ActionRoutesApply, supervisor.ActionBIRDStart, supervisor.ActionBIRDReload}
	if !result.OK || !reflect.DeepEqual(client.calls, want) {
		t.Fatalf("calls=%#v result=%#v", client.calls, result)
	}
}

func TestRoutingStartOrder(t *testing.T) {
	client := &recordingClient{}
	result := (Routing{Client: client}).Start(context.Background())
	want := []string{supervisor.ActionWGStart, supervisor.ActionRoutesApply, supervisor.ActionBIRDStart, supervisor.ActionBIRDReload}
	if !result.OK || !reflect.DeepEqual(client.calls, want) {
		t.Fatalf("calls=%#v result=%#v", client.calls, result)
	}
}

func TestRoutingStopOrder(t *testing.T) {
	client := &recordingClient{}
	result := (Routing{Client: client}).Stop(context.Background())
	want := []string{supervisor.ActionBIRDStop, supervisor.ActionWGStop}
	if !result.OK || !reflect.DeepEqual(client.calls, want) {
		t.Fatalf("calls=%#v result=%#v", client.calls, result)
	}
}

func TestRoutingRestartOrder(t *testing.T) {
	client := &recordingClient{}
	result := (Routing{Client: client}).Restart(context.Background())
	want := []string{supervisor.ActionBIRDStop, supervisor.ActionWGRestart, supervisor.ActionRoutesApply, supervisor.ActionBIRDStart, supervisor.ActionBIRDReload}
	if !result.OK || !reflect.DeepEqual(client.calls, want) {
		t.Fatalf("calls=%#v result=%#v", client.calls, result)
	}
}

func TestRoutingStopsOnFailure(t *testing.T) {
	client := &recordingClient{fail: supervisor.ActionRoutesApply}
	result := (Routing{Client: client}).Start(context.Background())
	want := []string{supervisor.ActionWGStart, supervisor.ActionRoutesApply}
	if result.OK || !reflect.DeepEqual(client.calls, want) {
		t.Fatalf("calls=%#v result=%#v", client.calls, result)
	}
	if got := ActionSummary(result); got != "wg.start:ok,routes.apply:error" {
		t.Fatalf("summary = %q", got)
	}
}
