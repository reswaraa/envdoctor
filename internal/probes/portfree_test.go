// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

package probes

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/reswaraa/envdoctor/internal/inference"
	"github.com/reswaraa/envdoctor/internal/output"
	"github.com/reswaraa/envdoctor/internal/system"
)

const testPortRecipe = `
id: port-collision
probe: port-free
fixes:
  - id: kill-port-holder
    class: destructive
    label: "Kill process holding port {{.Port}}"
    command: "kill $(lsof -ti :{{.Port}})"
`

func TestPortFree_ID(t *testing.T) {
	if got := PortFree(nil).ID(); got != "port-free" {
		t.Errorf("got %q, want port-free", got)
	}
}

func TestPortFree_AppliesTo(t *testing.T) {
	p := PortFree(nil)
	if p.AppliesTo(Input{RepoRoot: t.TempDir()}) {
		t.Error("AppliesTo must be false for a repo without compose files")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(`services:
  db:
    ports: ["5432:5432"]
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if !p.AppliesTo(Input{RepoRoot: dir}) {
		t.Error("AppliesTo must be true once compose declares a host port")
	}
}

func TestPortFree_Evaluate_NoRequirements(t *testing.T) {
	p := &portFreeProbe{lib: nil}
	if got := p.evaluate(nil, nil, &system.Facts{}); got != nil {
		t.Errorf("expected nil; got %+v", got)
	}
}

func TestPortFree_Evaluate_AllFree(t *testing.T) {
	p := &portFreeProbe{lib: mkLibrary(t, testPortRecipe)}
	reqs := []inference.PortRequirement{
		{Source: "docker-compose.yml#services.db.ports", Port: 5432},
	}
	if got := p.evaluate(reqs, map[int]string{}, &system.Facts{}); got != nil {
		t.Errorf("expected nil; got %+v", got)
	}
}

func TestPortFree_Evaluate_OnePerColliding(t *testing.T) {
	p := &portFreeProbe{lib: mkLibrary(t, testPortRecipe)}
	reqs := []inference.PortRequirement{
		{Source: "docker-compose.yml#services.db.ports", Port: 5432},
		{Source: "docker-compose.yml#services.web.ports", Port: 8080},
	}
	holders := map[int]string{5432: "postgres", 8080: ""}
	got := p.evaluate(reqs, holders, &system.Facts{})
	if len(got) != 2 {
		t.Fatalf("findings: got %d, want 2", len(got))
	}
	if got[0].Probe != "port-free" || got[0].Category != output.CategoryPorts {
		t.Errorf("Probe/Category: %+v", got[0])
	}
	if !strings.Contains(got[0].Summary, "5432") || !strings.Contains(got[0].Summary, "postgres") {
		t.Errorf("Summary should name port and holder; got %q", got[0].Summary)
	}
	if !strings.Contains(got[1].Summary, "8080") || strings.Contains(got[1].Summary, "by ") {
		t.Errorf("port without known holder should not include 'by '; got %q", got[1].Summary)
	}
	if got[0].RecipeID != "kill-port-holder" {
		t.Errorf("RecipeID: got %q", got[0].RecipeID)
	}
	if !strings.Contains(got[0].RecipeCommand, "lsof -ti :5432") {
		t.Errorf("RecipeCommand should template Port; got %q", got[0].RecipeCommand)
	}
}

func TestPortFree_Run_WithRealNetListen(t *testing.T) {
	// Bind a port for the duration of the test, then run the probe
	// against a compose file declaring that port. This is real (not
	// mocked) — we use the actual net.Listen path the probe uses.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }()
	port := ln.Addr().(*net.TCPAddr).Port

	dir := t.TempDir()
	compose := fmt.Sprintf("services:\n  app:\n    ports: [\"%d:%d\"]\n", port, port)
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(compose), 0o644); err != nil {
		t.Fatal(err)
	}

	p := &portFreeProbe{
		lib:       nil,
		checkPort: realCheckPort, // exercises the real bind logic
		holderFor: func(_ context.Context, _ int) string { return "test-process" },
	}
	got, err := p.Run(context.Background(), Input{RepoRoot: dir, System: &system.Facts{}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("findings: got %d, want 1; output: %+v", len(got), got)
	}
	if !strings.Contains(got[0].Summary, fmt.Sprintf("%d", port)) {
		t.Errorf("Summary should mention port %d; got %q", port, got[0].Summary)
	}
}

func TestPortFree_Run_FreePortEmitsNothing(t *testing.T) {
	// Bind, then close so the port becomes free, then run probe.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	dir := t.TempDir()
	compose := fmt.Sprintf("services:\n  app:\n    ports: [\"%d:%d\"]\n", port, port)
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(compose), 0o644); err != nil {
		t.Fatal(err)
	}

	p := &portFreeProbe{
		lib:       nil,
		checkPort: realCheckPort,
		holderFor: func(_ context.Context, _ int) string { return "" },
	}
	got, err := p.Run(context.Background(), Input{RepoRoot: dir, System: &system.Facts{}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected no findings for free port; got %+v", got)
	}
}
