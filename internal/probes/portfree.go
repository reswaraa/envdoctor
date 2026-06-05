// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

package probes

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"time"

	"github.com/reswaraa/envdoctor/internal/inference"
	"github.com/reswaraa/envdoctor/internal/output"
	"github.com/reswaraa/envdoctor/internal/recipes"
	"github.com/reswaraa/envdoctor/internal/system"
)

const (
	portProbeID  = "port-free"
	portRecipeID = "port-collision"
	portDocURL   = "https://envdoctor.dev/probes/port-free"
)

// PortFree returns the Probe that checks each port declared in
// docker-compose.yml#services[].ports is bindable locally. Each
// colliding port emits its own Finding.
func PortFree(lib *recipes.Library) Probe {
	return &portFreeProbe{
		lib:       lib,
		checkPort: realCheckPort,
		holderFor: realHolderFor,
	}
}

type portFreeProbe struct {
	lib       *recipes.Library
	checkPort func(ctx context.Context, port int) bool
	holderFor func(ctx context.Context, port int) string
}

func (p *portFreeProbe) ID() string { return portProbeID }

func (p *portFreeProbe) AppliesTo(in Input) bool {
	reqs, err := inference.InferPorts(in.RepoRoot)
	if err != nil {
		return true
	}
	return len(reqs) > 0
}

func (p *portFreeProbe) Run(ctx context.Context, in Input) ([]output.Finding, error) {
	reqs, err := inference.InferPorts(in.RepoRoot)
	if err != nil {
		return nil, err
	}
	if len(reqs) == 0 {
		return nil, nil
	}
	holders := map[int]string{}
	for _, r := range reqs {
		if p.checkPort(ctx, r.Port) {
			continue
		}
		holders[r.Port] = p.holderFor(ctx, r.Port)
	}
	return p.evaluate(reqs, holders, in.System), nil
}

// evaluate is the pure-function core. holders maps a port number to a
// human-readable description of what's holding it (empty string if
// detection failed). Ports not in holders are considered free.
func (p *portFreeProbe) evaluate(reqs []inference.PortRequirement, holders map[int]string, facts *system.Facts) []output.Finding {
	var out []output.Finding
	for _, r := range reqs {
		holder, used := holders[r.Port]
		if !used {
			continue
		}
		out = append(out, p.applyRecipe(p.findingFor(r, holder), r.Port, holder, facts))
	}
	return out
}

func (p *portFreeProbe) findingFor(r inference.PortRequirement, holder string) output.Finding {
	summary := fmt.Sprintf("Port %d in use", r.Port)
	observed := "in use"
	if holder != "" {
		summary += " by " + holder
		observed = "in use by " + holder
	}
	return output.Finding{
		Probe:    portProbeID,
		Category: output.CategoryPorts,
		Severity: output.SeverityWarning,
		Status:   output.StatusFail,
		Summary:  summary,
		Observed: observed,
		Expected: fmt.Sprintf("port %d free", r.Port),
		Evidence: []string{r.Source},
		DocURL:   portDocURL,
	}
}

func (p *portFreeProbe) applyRecipe(f output.Finding, port int, holder string, facts *system.Facts) output.Finding {
	if p.lib == nil {
		return f
	}
	rec, ok := p.lib.Lookup(portRecipeID)
	if !ok {
		return f
	}
	params := map[string]any{
		"Port":   fmt.Sprintf("%d", port),
		"Holder": holder,
	}
	fix, cmd, err := recipes.SelectFix(rec, facts, params)
	if err != nil || cmd == "" {
		return f
	}
	f.RecipeID = fix.ID
	f.RecipeCommand = cmd
	return f
}

// realCheckPort attempts to bind :port and returns true if the bind
// succeeded (port is free) or false if anything went wrong (port is
// almost certainly in use; we don't distinguish permission errors).
func realCheckPort(_ context.Context, port int) bool {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}

// realHolderFor runs `lsof -i :PORT -t -P -n` then `ps -p PID -o comm=`
// to identify the process holding the port. Best-effort; returns "" if
// either command fails or lsof is not installed.
func realHolderFor(parentCtx context.Context, port int) string {
	ctx, cancel := context.WithTimeout(parentCtx, 1500*time.Millisecond)
	defer cancel()

	out, err := exec.CommandContext(ctx, "lsof", "-i", fmt.Sprintf(":%d", port), "-t", "-P", "-n").Output()
	if err != nil {
		return ""
	}
	pid := strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
	if pid == "" {
		return ""
	}
	out, err = exec.CommandContext(ctx, "ps", "-p", pid, "-o", "comm=").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
