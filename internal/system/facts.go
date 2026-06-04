// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

// Package system collects facts about the user's machine at scan time.
//
// Collect() is called once per scan by the engine and the resulting *Facts
// is passed to every probe. HasTool caches results so repeated lookups
// across probes are O(1).
package system

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"

	"github.com/reswaraa/envdoctor/internal/output"
)

// Facts holds the collected system facts plus a tool-availability cache.
// Always used by pointer; the mutex protects the cache from probes that
// the engine runs in parallel.
type Facts struct {
	OS     string
	Arch   string
	Distro string
	Kernel string
	Shell  string
	WSL    bool

	mu        sync.Mutex
	toolCache map[string]bool
}

// Collect probes the local system and returns a fresh Facts.
//
// On Linux it parses /etc/os-release for the distro ID and reads
// /proc/sys/kernel/osrelease for WSL detection. On macOS those fields
// stay empty. Kernel is best-effort via `uname -r`.
func Collect() *Facts {
	f := &Facts{
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
		Shell:     os.Getenv("SHELL"),
		Kernel:    detectKernel(),
		toolCache: make(map[string]bool),
	}
	if f.OS == "linux" {
		f.Distro = detectDistro()
		f.WSL = detectWSL()
	}
	return f
}

// HasTool reports whether the named executable is on PATH. The result is
// cached per Facts; repeated lookups across probes are O(1).
func (f *Facts) HasTool(name string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if v, ok := f.toolCache[name]; ok {
		return v
	}
	_, err := exec.LookPath(name)
	v := err == nil
	f.toolCache[name] = v
	return v
}

// AsSystem returns the JSON-emittable subset of the facts. Used by the
// engine when constructing an output.Report.
func (f *Facts) AsSystem() output.System {
	return output.System{
		OS:     f.OS,
		Arch:   f.Arch,
		Distro: f.Distro,
		Kernel: f.Kernel,
		Shell:  f.Shell,
		WSL:    f.WSL,
	}
}

func detectDistro() string {
	b, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return ""
	}
	return parseOSReleaseID(string(b))
}

// parseOSReleaseID extracts the value of ID= from an os-release content
// string. Returns "" if no ID line is present. Handles quoted and
// unquoted values per the os-release(5) spec.
func parseOSReleaseID(content string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "ID=") {
			continue
		}
		v := strings.TrimPrefix(line, "ID=")
		v = strings.Trim(v, `"`)
		v = strings.Trim(v, `'`)
		return v
	}
	return ""
}

func detectWSL() bool {
	b, err := os.ReadFile("/proc/sys/kernel/osrelease")
	if err != nil {
		return false
	}
	return isWSLOSRelease(string(b))
}

// isWSLOSRelease reports whether a /proc/sys/kernel/osrelease string
// indicates WSL. The kernel string contains "microsoft" (WSL1, WSL2 on
// older Windows) or "WSL" (newer WSL2 builds).
func isWSLOSRelease(s string) bool {
	s = strings.ToLower(s)
	return strings.Contains(s, "microsoft") || strings.Contains(s, "wsl")
}

func detectKernel() string {
	out, err := exec.Command("uname", "-r").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
