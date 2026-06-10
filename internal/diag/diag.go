package diag

import (
	"context"
	"fmt"
	"os/exec"
	"time"
)

type Command struct {
	Name string   `json:"name"`
	Args []string `json:"args"`
}

var Commands = map[string]Command{
	"routes":      {Name: "ip", Args: []string{"route", "show", "table", "all"}},
	"addresses":   {Name: "ip", Args: []string{"addr", "show"}},
	"rules":       {Name: "ip", Args: []string{"rule", "show"}},
	"links":       {Name: "ip", Args: []string{"link", "show"}},
	"neighbors":   {Name: "ip", Args: []string{"neigh", "show"}},
	"wg":          {Name: "wg", Args: []string{"show"}},
	"bird":        {Name: "birdc", Args: []string{"show", "protocols", "all"}},
	"bird-routes": {Name: "birdc", Args: []string{"show", "route", "all"}},
}

type Runner struct {
	Timeout time.Duration
}

func (r Runner) Run(ctx context.Context, name string) (string, error) {
	spec, ok := Commands[name]
	if !ok {
		return "", fmt.Errorf("unknown diagnostic command %q", name)
	}
	timeout := r.Timeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, spec.Name, spec.Args...).CombinedOutput()
	if ctx.Err() != nil {
		return string(out), ctx.Err()
	}
	if err != nil {
		return string(out), fmt.Errorf("%s failed: %w", name, err)
	}
	return string(out), nil
}
