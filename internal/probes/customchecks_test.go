// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

package probes

import (
	"context"
	"net"
	"os"
	"strings"
	"testing"

	"github.com/reswaraa/envdoctor/internal/config"
	"github.com/reswaraa/envdoctor/internal/output"
	"github.com/reswaraa/envdoctor/internal/system"
)

func TestCustomChecks_NilConfig(t *testing.T) {
	p := CustomChecks(nil)
	if p.AppliesTo(Input{}) {
		t.Error("nil config must not apply")
	}
	got, err := p.Run(context.Background(), Input{System: &system.Facts{}})
	if err != nil || got != nil {
		t.Errorf("nil config: got %v / %v", got, err)
	}
}

func TestCustomChecks_NoChecks(t *testing.T) {
	cfg := &config.Config{SchemaVersion: 1}
	p := CustomChecks(cfg)
	if p.AppliesTo(Input{}) {
		t.Error("empty checks must not apply")
	}
}

func TestCustomChecks_ToolVersion_Mismatch(t *testing.T) {
	p := &customChecksProbe{
		cfg: &config.Config{
			Checks: []config.Check{
				{Type: config.CheckToolVersion, Tool: "psql", Version: ">=14.0"},
			},
		},
		detectVersion: func(_ context.Context, _ string) (string, error) {
			return "13.2.0", nil
		},
	}
	got, err := p.Run(context.Background(), Input{System: &system.Facts{}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("findings: got %d, want 1", len(got))
	}
	if got[0].Category != output.CategoryCustom {
		t.Errorf("Category: got %q", got[0].Category)
	}
	if !strings.Contains(got[0].Summary, "13.2.0") {
		t.Errorf("Summary should mention installed version; got %q", got[0].Summary)
	}
}

func TestCustomChecks_ToolVersion_NotInstalled(t *testing.T) {
	p := &customChecksProbe{
		cfg: &config.Config{
			Checks: []config.Check{
				{Type: config.CheckToolVersion, Tool: "psql", Version: ">=14.0"},
			},
		},
		detectVersion: func(_ context.Context, _ string) (string, error) {
			return "", os.ErrNotExist
		},
	}
	got, _ := p.Run(context.Background(), Input{System: &system.Facts{}})
	if len(got) != 1 || got[0].Observed != "(missing)" {
		t.Errorf("not-installed: got %+v", got)
	}
}

func TestCustomChecks_ToolVersion_Satisfied(t *testing.T) {
	p := &customChecksProbe{
		cfg: &config.Config{
			Checks: []config.Check{
				{Type: config.CheckToolVersion, Tool: "psql", Version: ">=14.0"},
			},
		},
		detectVersion: func(_ context.Context, _ string) (string, error) {
			return "16.1.0", nil
		},
	}
	got, _ := p.Run(context.Background(), Input{System: &system.Facts{}})
	if got != nil {
		t.Errorf("satisfied tool should produce no finding; got %+v", got)
	}
}

func TestCustomChecks_PortFree_InUse(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	port := ln.Addr().(*net.TCPAddr).Port

	p := &customChecksProbe{
		cfg: &config.Config{
			Checks: []config.Check{{Type: config.CheckPortFree, Port: port}},
		},
	}
	got, _ := p.Run(context.Background(), Input{System: &system.Facts{}})
	if len(got) != 1 {
		t.Fatalf("findings: got %d, want 1", len(got))
	}
}

func TestCustomChecks_PortFree_Free(t *testing.T) {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	p := &customChecksProbe{
		cfg: &config.Config{
			Checks: []config.Check{{Type: config.CheckPortFree, Port: port}},
		},
	}
	got, _ := p.Run(context.Background(), Input{System: &system.Facts{}})
	if got != nil {
		t.Errorf("free port should produce no finding; got %+v", got)
	}
}

func TestCustomChecks_CommandPresent(t *testing.T) {
	// "go" must be present in any envdoctor dev/CI environment.
	p := &customChecksProbe{
		cfg: &config.Config{
			Checks: []config.Check{{Type: config.CheckCommandPresent, Command: "go"}},
		},
	}
	got, _ := p.Run(context.Background(), Input{System: &system.Facts{}})
	if got != nil {
		t.Errorf("present command should produce no finding; got %+v", got)
	}

	p.cfg.Checks[0].Command = "definitely-missing-binary-zzz"
	got, _ = p.Run(context.Background(), Input{System: &system.Facts{}})
	if len(got) != 1 || !strings.Contains(got[0].Summary, "definitely-missing-binary-zzz") {
		t.Errorf("absent command should produce a finding; got %+v", got)
	}
}

func TestCustomChecks_EnvRequired(t *testing.T) {
	t.Setenv("CUSTOM_ENVDOCTOR_TEST", "yes")
	p := &customChecksProbe{
		cfg: &config.Config{
			Checks: []config.Check{
				{Type: config.CheckEnvRequired, Keys: []string{"CUSTOM_ENVDOCTOR_TEST", "DEFINITELY_NOT_SET_ZZZ"}},
			},
		},
	}
	got, _ := p.Run(context.Background(), Input{System: &system.Facts{}})
	if len(got) != 1 {
		t.Fatalf("findings: got %d, want 1", len(got))
	}
	if !strings.Contains(got[0].Summary, "DEFINITELY_NOT_SET_ZZZ") {
		t.Errorf("Summary should list the missing key; got %q", got[0].Summary)
	}
	if strings.Contains(got[0].Summary, "CUSTOM_ENVDOCTOR_TEST") {
		t.Errorf("present key must not be listed; got %q", got[0].Summary)
	}
}

func TestCustomChecks_ReasonIncludedInEvidence(t *testing.T) {
	p := &customChecksProbe{
		cfg: &config.Config{
			Checks: []config.Check{{
				Type:    config.CheckCommandPresent,
				Command: "definitely-missing-binary-zzz",
				Reason:  "we use it in deploy scripts",
			}},
		},
	}
	got, _ := p.Run(context.Background(), Input{System: &system.Facts{}})
	if len(got) != 1 {
		t.Fatal("expected one finding")
	}
	joined := strings.Join(got[0].Evidence, "; ")
	if !strings.Contains(joined, "we use it in deploy scripts") {
		t.Errorf("evidence should include the reason; got %q", joined)
	}
}

func TestRealDetectToolVersion_ExtractsFromGo(t *testing.T) {
	v, err := realDetectToolVersion(context.Background(), "go")
	if err != nil {
		t.Skipf("go not available: %v", err)
	}
	if !strings.Contains(v, ".") {
		t.Errorf("expected dotted version; got %q", v)
	}
}
