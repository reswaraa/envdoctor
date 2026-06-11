// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

package probes

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/reswaraa/envdoctor/internal/config"
	"github.com/reswaraa/envdoctor/internal/output"
	"github.com/reswaraa/envdoctor/internal/semver"
)

const (
	customProbeID = "custom"
	customDocURL  = "https://reswaraa.github.io/envdoctor/probes/custom"
)

// CustomChecks returns a Probe that interprets .envdoctor.yaml's
// `checks:` entries. Each entry produces a Finding when violated.
// Returns nil when cfg is nil or has no checks (the engine's
// AppliesTo will skip a nil Probe via the empty slice).
func CustomChecks(cfg *config.Config) Probe {
	return &customChecksProbe{cfg: cfg, detectVersion: realDetectToolVersion}
}

type customChecksProbe struct {
	cfg           *config.Config
	detectVersion func(ctx context.Context, tool string) (string, error)
}

func (p *customChecksProbe) ID() string { return customProbeID }

func (p *customChecksProbe) AppliesTo(_ Input) bool {
	return p.cfg != nil && len(p.cfg.Checks) > 0
}

func (p *customChecksProbe) Run(ctx context.Context, in Input) ([]output.Finding, error) {
	if p.cfg == nil {
		return nil, nil
	}
	var out []output.Finding
	for i, ck := range p.cfg.Checks {
		f, ok := p.evaluate(ctx, ck, i, in)
		if ok {
			out = append(out, f)
		}
	}
	return out, nil
}

func (p *customChecksProbe) evaluate(ctx context.Context, ck config.Check, idx int, in Input) (output.Finding, bool) {
	switch ck.Type {
	case config.CheckToolVersion:
		return p.checkToolVersion(ctx, ck, idx)
	case config.CheckPortFree:
		return p.checkPortFree(ck, idx)
	case config.CheckCommandPresent:
		return p.checkCommandPresent(ck, idx, in)
	case config.CheckEnvRequired:
		return p.checkEnvRequired(ck, idx, in.RepoRoot)
	}
	return output.Finding{}, false
}

// versionTokenRE extracts the first dotted-numeric token from a
// `<tool> --version` output. Handles "Python 3.11.5", "v20.10.0",
// "go version go1.21.5 darwin/arm64", "Docker version 24.0.7, build …".
var versionTokenRE = regexp.MustCompile(`\d+\.\d+(?:\.\d+)?`)

func (p *customChecksProbe) checkToolVersion(ctx context.Context, ck config.Check, idx int) (output.Finding, bool) {
	installed, err := p.detectVersion(ctx, ck.Tool)
	if err != nil || installed == "" {
		return p.findingFor(ck, idx,
			fmt.Sprintf("%s not found on PATH", ck.Tool),
			"(missing)", ck.Version), true
	}
	ok, sErr := semver.Satisfies(installed, ck.Version)
	if sErr == nil && ok {
		return output.Finding{}, false
	}
	return p.findingFor(ck, idx,
		fmt.Sprintf("%s %s does not satisfy %s", ck.Tool, installed, ck.Version),
		installed, ck.Version), true
}

func (p *customChecksProbe) checkPortFree(ck config.Check, idx int) (output.Finding, bool) {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", ck.Port))
	if err != nil {
		return p.findingFor(ck, idx,
			fmt.Sprintf("Port %d is in use", ck.Port),
			"in use", fmt.Sprintf("port %d free", ck.Port)), true
	}
	_ = ln.Close()
	return output.Finding{}, false
}

func (p *customChecksProbe) checkCommandPresent(ck config.Check, idx int, in Input) (output.Finding, bool) {
	if in.System.HasTool(ck.Command) {
		return output.Finding{}, false
	}
	return p.findingFor(ck, idx,
		fmt.Sprintf("Command %q not on PATH", ck.Command),
		"(missing)", fmt.Sprintf("%q on PATH", ck.Command)), true
}

func (p *customChecksProbe) checkEnvRequired(ck config.Check, idx int, repoRoot string) (output.Finding, bool) {
	present := map[string]bool{}
	for _, e := range os.Environ() {
		if i := strings.Index(e, "="); i > 0 {
			present[e[:i]] = true
		}
	}
	missing := []string{}
	for _, k := range ck.Keys {
		if !present[k] {
			missing = append(missing, k)
		}
	}
	if len(missing) == 0 {
		return output.Finding{}, false
	}
	_ = repoRoot
	return p.findingFor(ck, idx,
		fmt.Sprintf("Required env vars missing: %s", strings.Join(missing, ", ")),
		"(missing)", strings.Join(missing, ", ")), true
}

// findingFor constructs a Finding from a custom Check. The Category
// is always CategoryCustom; Severity is error. Reason from the YAML
// is appended to Evidence so the user knows *why* the maintainer
// asked for this check.
func (p *customChecksProbe) findingFor(ck config.Check, idx int, summary, observed, expected string) output.Finding {
	evidence := []string{fmt.Sprintf(".envdoctor.yaml#checks[%d].%s", idx, ck.Type)}
	if ck.Reason != "" {
		evidence = append(evidence, "reason: "+ck.Reason)
	}
	return output.Finding{
		Probe:    customProbeID,
		Category: output.CategoryCustom,
		Severity: output.SeverityError,
		Status:   output.StatusFail,
		Summary:  summary,
		Observed: observed,
		Expected: expected,
		Evidence: evidence,
		DocURL:   customDocURL,
	}
}

func realDetectToolVersion(ctx context.Context, tool string) (string, error) {
	for _, args := range [][]string{{"--version"}, {"-v"}, {"version"}} {
		cmd := exec.CommandContext(ctx, tool, args...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			continue
		}
		match := versionTokenRE.FindString(string(out))
		if match != "" {
			return match, nil
		}
	}
	return "", fmt.Errorf("could not detect version for %q", tool)
}
