package supervisor

import (
	"context"
	"fmt"
	"os/exec"
	"time"
)

type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) (string, error)
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, name string, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("%s failed: %w", name, err)
	}
	return string(out), nil
}

func (s Server) runCommand(action, name string, args ...string) Response {
	timeout := s.CommandTimeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	runner := s.Runner
	if runner == nil {
		runner = ExecRunner{}
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	out, err := runner.Run(ctx, name, args...)
	if err != nil {
		return Response{OK: false, Action: action, Output: out, Error: err.Error()}
	}
	return Response{OK: true, Action: action, Output: out}
}
