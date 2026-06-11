// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

// Package engine runs a set of probes in parallel, recovers panics and
// errors, and returns Findings in stable order.
//
// The engine is the only place that knows about probe scheduling, panic
// containment, and Finding ID assignment. Probes themselves are pure
// (input -> findings) and never touch concurrency primitives.
package engine

import (
	"context"
	"fmt"
	"runtime"
	"sort"
	"sync"
	"time"

	"github.com/reswaraa/envdoctor/internal/output"
	"github.com/reswaraa/envdoctor/internal/probes"
)

// Engine runs a set of probes. Concurrency bounds the number of probes
// running simultaneously; zero (the default) means runtime.GOMAXPROCS(0).
type Engine struct {
	Probes      []probes.Probe
	Concurrency int
}

// New returns an Engine wrapping the given probes.
func New(ps []probes.Probe) *Engine {
	return &Engine{Probes: ps}
}

// Stats summarizes a scan's mechanical outcome (not the per-finding
// severity). Used by the TTY renderer to print "Ran N probes in Xms".
type Stats struct {
	Total      int
	Applicable int
	Ran        int
	Failed     int
	Duration   time.Duration
}

// Run executes all applicable probes against in. Returns the collected
// findings in stable order (by probe ID, then by their position in the
// probe's returned slice) and a Stats summary.
//
// A probe that panics or returns an error is turned into a single
// Finding with Status = StatusProbeFailed; Run itself never returns an
// error for per-probe failures.
//
// ctx cancellation propagates to probes via the context argument to
// Probe.Run; the engine waits for all goroutines to exit before returning.
func (e *Engine) Run(ctx context.Context, in probes.Input) ([]output.Finding, Stats) {
	start := time.Now()
	stats := Stats{Total: len(e.Probes)}

	applicable := make([]probes.Probe, 0, len(e.Probes))
	for _, p := range e.Probes {
		if p.AppliesTo(in) {
			applicable = append(applicable, p)
		}
	}
	stats.Applicable = len(applicable)

	if len(applicable) == 0 {
		stats.Duration = time.Since(start)
		return []output.Finding{}, stats
	}

	n := e.Concurrency
	if n <= 0 {
		n = runtime.GOMAXPROCS(0)
	}
	if n > len(applicable) {
		n = len(applicable)
	}

	type slot struct {
		probeID  string
		findings []output.Finding
		failed   bool
	}
	slots := make([]slot, len(applicable))

	sem := make(chan struct{}, n)
	var wg sync.WaitGroup
	for i, p := range applicable {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, p probes.Probe) {
			defer wg.Done()
			defer func() { <-sem }()
			findings, failed := runProbe(ctx, p, in)
			slots[i] = slot{probeID: p.ID(), findings: findings, failed: failed}
		}(i, p)
	}
	wg.Wait()

	sort.SliceStable(slots, func(i, j int) bool {
		return slots[i].probeID < slots[j].probeID
	})

	findings := make([]output.Finding, 0)
	for _, s := range slots {
		stats.Ran++
		if s.failed {
			stats.Failed++
		}
		for j, f := range s.findings {
			if f.ID == "" {
				f.ID = fmt.Sprintf("%s-%d", s.probeID, j+1)
			}
			findings = append(findings, f)
		}
	}

	stats.Duration = time.Since(start)
	return findings, stats
}

// runProbe invokes p.Run with panic recovery. Returns the probe's findings
// on success or a synthetic StatusProbeFailed finding on panic or error.
// The bool reports whether the probe failed (panicked or returned error).
func runProbe(ctx context.Context, p probes.Probe, in probes.Input) ([]output.Finding, bool) {
	var (
		findings []output.Finding
		runErr   error
	)
	func() {
		defer func() {
			if r := recover(); r != nil {
				runErr = fmt.Errorf("panic: %v", r)
			}
		}()
		findings, runErr = p.Run(ctx, in)
	}()
	if runErr != nil {
		return []output.Finding{{
			Probe:    p.ID(),
			Category: "internal",
			Severity: output.SeverityError,
			Status:   output.StatusProbeFailed,
			Summary:  fmt.Sprintf("Probe %q failed: %v", p.ID(), runErr),
			DocURL:   "https://reswaraa.github.io/envdoctor/probes/" + p.ID(),
		}}, true
	}
	return findings, false
}
