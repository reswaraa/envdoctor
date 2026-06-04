// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

package system

import (
	"runtime"
	"testing"

	"github.com/reswaraa/envdoctor/internal/output"
)

func TestParseOSReleaseID(t *testing.T) {
	cases := []struct {
		name, content, want string
	}{
		{
			name: "ubuntu",
			content: `NAME="Ubuntu"
VERSION="22.04.3 LTS (Jammy Jellyfish)"
ID=ubuntu
ID_LIKE=debian`,
			want: "ubuntu",
		},
		{
			name: "debian-quoted",
			content: `PRETTY_NAME="Debian GNU/Linux 12 (bookworm)"
NAME="Debian GNU/Linux"
ID="debian"`,
			want: "debian",
		},
		{
			name: "alpine",
			content: `NAME="Alpine Linux"
ID=alpine
VERSION_ID=3.18.4`,
			want: "alpine",
		},
		{
			name: "no-id",
			content: `NAME="Mystery"
VERSION_ID=1.0`,
			want: "",
		},
		{
			name:    "empty",
			content: "",
			want:    "",
		},
		{
			name: "id-not-first",
			content: `NAME="Fedora Linux"
VERSION="39 (Workstation Edition)"
ID=fedora
ID_LIKE=`,
			want: "fedora",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := parseOSReleaseID(c.content); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestIsWSLOSRelease(t *testing.T) {
	cases := []struct {
		name, content string
		want          bool
	}{
		{"wsl2-microsoft", "5.15.146.1-microsoft-standard-WSL2", true},
		{"wsl-uppercase", "5.10.WSL+", true},
		{"native-linux", "6.5.0-14-generic", false},
		{"empty", "", false},
		{"microsoft-mixed-case", "5.15.0-Microsoft-standard", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isWSLOSRelease(c.content); got != c.want {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}

// TestFacts_HasToolCachesPresent asserts that a known-present tool ("go",
// which must exist in any envdoctor dev or CI environment) is returned as
// true and is cached on subsequent lookups.
func TestFacts_HasToolCachesPresent(t *testing.T) {
	f := &Facts{toolCache: map[string]bool{}}
	if !f.HasTool("go") {
		t.Fatal("expected 'go' on PATH in the dev/CI environment")
	}
	if v, ok := f.toolCache["go"]; !ok || !v {
		t.Fatalf("cache: ok=%v v=%v; want present and true", ok, v)
	}
}

func TestFacts_HasToolCachesAbsent(t *testing.T) {
	f := &Facts{toolCache: map[string]bool{}}
	const name = "definitely-not-a-real-binary-envdoctor-zzz"
	if f.HasTool(name) {
		t.Fatalf("expected %q absent", name)
	}
	if v, ok := f.toolCache[name]; !ok || v {
		t.Fatalf("cache: ok=%v v=%v; want present and false", ok, v)
	}
}

func TestFacts_AsSystemMapsAllFields(t *testing.T) {
	f := &Facts{
		OS:     "linux",
		Arch:   "amd64",
		Distro: "ubuntu",
		Kernel: "5.15.0-generic",
		Shell:  "/bin/bash",
		WSL:    true,
	}
	want := output.System{
		OS:     "linux",
		Arch:   "amd64",
		Distro: "ubuntu",
		Kernel: "5.15.0-generic",
		Shell:  "/bin/bash",
		WSL:    true,
	}
	if got := f.AsSystem(); got != want {
		t.Errorf("AsSystem: got %+v, want %+v", got, want)
	}
}

// TestCollect_PopulatesRuntimeFields is a thin smoke test for Collect.
// It cannot assert specific OS / Distro / WSL values because those depend
// on the host; it does assert that OS and Arch match runtime constants.
func TestCollect_PopulatesRuntimeFields(t *testing.T) {
	f := Collect()
	if f.OS != runtime.GOOS {
		t.Errorf("OS: got %q, want %q", f.OS, runtime.GOOS)
	}
	if f.Arch != runtime.GOARCH {
		t.Errorf("Arch: got %q, want %q", f.Arch, runtime.GOARCH)
	}
	if f.toolCache == nil {
		t.Error("toolCache must be initialized for HasTool to be safe")
	}
}
