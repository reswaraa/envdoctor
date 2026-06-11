// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/reswaraa/envdoctor/internal/output"
	"github.com/reswaraa/envdoctor/internal/probes"
	"github.com/reswaraa/envdoctor/internal/system"
)

// --- test probes ---

type okProbe struct {
	id       string
	emit     []output.Finding
	ranCount *int32
}

func (o *okProbe) ID() string                    { return o.id }
func (o *okProbe) AppliesTo(_ probes.Input) bool { return true }
func (o *okProbe) Run(_ context.Context, _ probes.Input) ([]output.Finding, error) {
	if o.ranCount != nil {
		atomic.AddInt32(o.ranCount, 1)
	}
	return o.emit, nil
}

type errProbe struct {
	id  string
	err error
}

func (e *errProbe) ID() string                    { return e.id }
func (e *errProbe) AppliesTo(_ probes.Input) bool { return true }
func (e *errProbe) Run(_ context.Context, _ probes.Input) ([]output.Finding, error) {
	return nil, e.err
}

type panicProbe struct{ id string }

func (p *panicProbe) ID() string                    { return p.id }
func (p *panicProbe) AppliesTo(_ probes.Input) bool { return true }
func (p *panicProbe) Run(_ context.Context, _ probes.Input) ([]output.Finding, error) {
	panic("boom")
}

type skipProbe struct {
	id       string
	ranCount *int32
}

func (s *skipProbe) ID() string                    { return s.id }
func (s *skipProbe) AppliesTo(_ probes.Input) bool { return false }
func (s *skipProbe) Run(_ context.Context, _ probes.Input) ([]output.Finding, error) {
	if s.ranCount != nil {
		atomic.AddInt32(s.ranCount, 1)
	}
	return nil, errors.New("skipProbe.Run must not be invoked")
}

type ctxProbe struct{ id string }

func (c *ctxProbe) ID() string                    { return c.id }
func (c *ctxProbe) AppliesTo(_ probes.Input) bool { return true }
func (c *ctxProbe) Run(ctx context.Context, _ probes.Input) ([]output.Finding, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

// --- helpers ---

func newInput() probes.Input {
	return probes.Input{
		RepoRoot: ".",
		System:   &system.Facts{},
	}
}

// --- tests ---

func TestEngine_NoProbesReturnsEmpty(t *testing.T) {
	e := New(nil)
	findings, stats := e.Run(context.Background(), newInput())
	if len(findings) != 0 {
		t.Errorf("findings: got %d, want 0", len(findings))
	}
	if stats.Total != 0 || stats.Applicable != 0 || stats.Ran != 0 {
		t.Errorf("stats: %+v", stats)
	}
}

func TestEngine_OrderingByProbeID(t *testing.T) {
	mk := func(id string) probes.Probe {
		return &okProbe{
			id: id,
			emit: []output.Finding{{
				Probe:    id,
				Category: "test",
				Severity: output.SeverityWarning,
				Status:   output.StatusFail,
				Summary:  "fixture " + id,
				DocURL:   "https://reswaraa.github.io/envdoctor/probes/" + id,
			}},
		}
	}
	// Insert out of order to prove the engine sorts.
	e := New([]probes.Probe{mk("c"), mk("a"), mk("b")})
	findings, _ := e.Run(context.Background(), newInput())
	if len(findings) != 3 {
		t.Fatalf("findings: got %d, want 3", len(findings))
	}
	want := []string{"a", "b", "c"}
	for i, f := range findings {
		if f.Probe != want[i] {
			t.Errorf("findings[%d].Probe: got %q, want %q", i, f.Probe, want[i])
		}
	}
}

func TestEngine_AssignsFindingID(t *testing.T) {
	p := &okProbe{
		id: "node-version",
		emit: []output.Finding{
			{Probe: "node-version", DocURL: "x"},
			{Probe: "node-version", DocURL: "x", ID: "explicit-id"},
		},
	}
	findings, _ := New([]probes.Probe{p}).Run(context.Background(), newInput())
	if len(findings) != 2 {
		t.Fatalf("findings: got %d, want 2", len(findings))
	}
	if findings[0].ID != "node-version-1" {
		t.Errorf("findings[0].ID: got %q, want %q", findings[0].ID, "node-version-1")
	}
	if findings[1].ID != "explicit-id" {
		t.Errorf("findings[1].ID: got %q, want %q (engine must not overwrite an explicit ID)", findings[1].ID, "explicit-id")
	}
}

func TestEngine_PanicBecomesProbeFailed(t *testing.T) {
	e := New([]probes.Probe{&panicProbe{id: "boom"}})
	findings, stats := e.Run(context.Background(), newInput())
	if len(findings) != 1 {
		t.Fatalf("findings: got %d, want 1", len(findings))
	}
	if findings[0].Status != output.StatusProbeFailed {
		t.Errorf("status: got %q, want %q", findings[0].Status, output.StatusProbeFailed)
	}
	if findings[0].Severity != output.SeverityError {
		t.Errorf("severity: got %q, want %q", findings[0].Severity, output.SeverityError)
	}
	if stats.Failed != 1 || stats.Ran != 1 {
		t.Errorf("stats: %+v; want Failed=1 Ran=1", stats)
	}
}

func TestEngine_ErrorBecomesProbeFailed(t *testing.T) {
	e := New([]probes.Probe{&errProbe{id: "broken", err: errors.New("nope")}})
	findings, stats := e.Run(context.Background(), newInput())
	if len(findings) != 1 {
		t.Fatalf("findings: got %d, want 1", len(findings))
	}
	if findings[0].Status != output.StatusProbeFailed {
		t.Errorf("status: got %q, want %q", findings[0].Status, output.StatusProbeFailed)
	}
	if stats.Failed != 1 {
		t.Errorf("Failed: got %d, want 1", stats.Failed)
	}
}

func TestEngine_AppliesToFalseIsNotRun(t *testing.T) {
	var skipCount, okCount int32
	probesList := []probes.Probe{
		&skipProbe{id: "skipped", ranCount: &skipCount},
		&okProbe{id: "ran", emit: nil, ranCount: &okCount},
	}
	_, stats := New(probesList).Run(context.Background(), newInput())
	if atomic.LoadInt32(&skipCount) != 0 {
		t.Errorf("skipProbe.Run was invoked %d times; want 0", skipCount)
	}
	if atomic.LoadInt32(&okCount) != 1 {
		t.Errorf("okProbe.Run was invoked %d times; want 1", okCount)
	}
	if stats.Total != 2 || stats.Applicable != 1 || stats.Ran != 1 {
		t.Errorf("stats: %+v; want Total=2 Applicable=1 Ran=1", stats)
	}
}

func TestEngine_ContextCancellationPropagates(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	e := New([]probes.Probe{&ctxProbe{id: "waiter"}})

	done := make(chan struct{})
	var (
		findings []output.Finding
		stats    Stats
	)
	go func() {
		findings, stats = e.Run(ctx, newInput())
		close(done)
	}()

	// Give the goroutine time to enter Run, then cancel.
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("engine did not return after ctx cancellation")
	}
	if len(findings) != 1 {
		t.Fatalf("findings: got %d, want 1", len(findings))
	}
	if findings[0].Status != output.StatusProbeFailed {
		t.Errorf("status: got %q, want %q", findings[0].Status, output.StatusProbeFailed)
	}
	if stats.Failed != 1 {
		t.Errorf("Failed: got %d, want 1", stats.Failed)
	}
}

func TestEngine_RunsInParallel(t *testing.T) {
	// 8 probes that each take ~50ms. Serial would be ~400ms; with default
	// concurrency it should finish well under 200ms. Loose bound avoids
	// flakiness on slow CI.
	const n = 8
	const each = 50 * time.Millisecond
	list := make([]probes.Probe, n)
	for i := 0; i < n; i++ {
		list[i] = &sleepProbe{id: fmt.Sprintf("p%02d", i), dur: each}
	}
	start := time.Now()
	_, _ = New(list).Run(context.Background(), newInput())
	dur := time.Since(start)
	if dur > 300*time.Millisecond {
		t.Errorf("8 x 50ms probes took %v; expected concurrency to keep it under 300ms", dur)
	}
}

type sleepProbe struct {
	id  string
	dur time.Duration
}

func (s *sleepProbe) ID() string                    { return s.id }
func (s *sleepProbe) AppliesTo(_ probes.Input) bool { return true }
func (s *sleepProbe) Run(ctx context.Context, _ probes.Input) ([]output.Finding, error) {
	select {
	case <-time.After(s.dur):
		return nil, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
