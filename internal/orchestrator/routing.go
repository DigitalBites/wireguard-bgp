package orchestrator

import (
	"context"
	"fmt"
	"strings"

	"peplink-wg-bgp/internal/supervisor"
)

type ActionClient interface {
	Call(ctx context.Context, action string) (supervisor.Response, error)
}

type Routing struct {
	Client ActionClient
}

type Step struct {
	Action string `json:"action"`
	OK     bool   `json:"ok"`
	Output string `json:"output,omitempty"`
	Error  string `json:"error,omitempty"`
}

type Result struct {
	OK    bool   `json:"ok"`
	Steps []Step `json:"steps"`
}

func (r Routing) Apply(ctx context.Context) Result {
	return r.run(ctx, []string{
		supervisor.ActionWGRestart,
		supervisor.ActionBIRDStart,
		supervisor.ActionBIRDReload,
	})
}

func (r Routing) Start(ctx context.Context) Result {
	return r.run(ctx, []string{
		supervisor.ActionWGStart,
		supervisor.ActionBIRDStart,
		supervisor.ActionBIRDReload,
	})
}

func (r Routing) Stop(ctx context.Context) Result {
	return r.run(ctx, []string{
		supervisor.ActionBIRDStop,
		supervisor.ActionWGStop,
	})
}

func (r Routing) Restart(ctx context.Context) Result {
	return r.run(ctx, []string{
		supervisor.ActionBIRDStop,
		supervisor.ActionWGRestart,
		supervisor.ActionBIRDStart,
		supervisor.ActionBIRDReload,
	})
}

func (r Routing) run(ctx context.Context, actions []string) Result {
	result := Result{OK: true, Steps: make([]Step, 0, len(actions))}
	for _, action := range actions {
		resp, err := r.Client.Call(ctx, action)
		step := Step{
			Action: action,
			OK:     err == nil && resp.OK,
			Output: resp.Output,
		}
		if err != nil {
			step.Error = err.Error()
		} else {
			step.Error = resp.Error
		}
		result.Steps = append(result.Steps, step)
		if !step.OK {
			result.OK = false
			break
		}
	}
	return result
}

func ActionSummary(result Result) string {
	parts := make([]string, 0, len(result.Steps))
	for _, step := range result.Steps {
		if step.OK {
			parts = append(parts, step.Action+":ok")
			continue
		}
		parts = append(parts, fmt.Sprintf("%s:error", step.Action))
	}
	return strings.Join(parts, ",")
}
