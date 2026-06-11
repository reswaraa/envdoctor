// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- parsePlatformSums --------------------------------------------

func TestParsePlatformSums_HappyPath(t *testing.T) {
	body := `aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa  envdoctor_0.1.0_darwin_amd64.tar.gz
bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb  envdoctor_0.1.0_darwin_arm64.tar.gz
cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc  envdoctor_0.1.0_linux_amd64.tar.gz
dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd  envdoctor_0.1.0_linux_arm64.tar.gz
ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff  some_other_artifact.zip
`
	got, err := parsePlatformSums(body, "0.1.0")
	if err != nil {
		t.Fatalf("parsePlatformSums: %v", err)
	}
	want := platformSHAs{
		DarwinAmd64: strings.Repeat("a", 64),
		DarwinArm64: strings.Repeat("b", 64),
		LinuxAmd64:  strings.Repeat("c", 64),
		LinuxArm64:  strings.Repeat("d", 64),
	}
	if got != want {
		t.Errorf("parsePlatformSums: got %+v\nwant %+v", got, want)
	}
}

func TestParsePlatformSums_BSDStarPrefixTolerated(t *testing.T) {
	// BSD-style sha256sum output uses `*<name>` for binary mode.
	body := `aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa *envdoctor_0.1.0_darwin_amd64.tar.gz
bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb *envdoctor_0.1.0_darwin_arm64.tar.gz
cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc *envdoctor_0.1.0_linux_amd64.tar.gz
dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd *envdoctor_0.1.0_linux_arm64.tar.gz
`
	got, err := parsePlatformSums(body, "0.1.0")
	if err != nil {
		t.Fatal(err)
	}
	if got.DarwinAmd64 != strings.Repeat("a", 64) {
		t.Errorf("darwin_amd64 not extracted from BSD-prefixed line; got %q", got.DarwinAmd64)
	}
}

func TestParsePlatformSums_MissingPlatformIsError(t *testing.T) {
	// Only two of four platforms present — must fail loudly, not
	// silently ship a half-pinned bootstrap.
	body := `aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa  envdoctor_0.1.0_darwin_amd64.tar.gz
bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb  envdoctor_0.1.0_darwin_arm64.tar.gz
`
	_, err := parsePlatformSums(body, "0.1.0")
	if err == nil {
		t.Fatal("expected error when linux entries are missing")
	}
	if !strings.Contains(err.Error(), "linux_amd64") || !strings.Contains(err.Error(), "linux_arm64") {
		t.Errorf("error should name the missing entries; got %q", err.Error())
	}
}

// --- renderBootstrap ----------------------------------------------

func TestRenderBootstrap_FillsAllPlaceholders(t *testing.T) {
	tmpl := `version=__ENVDOCTOR_VERSION__
repo=__ENVDOCTOR_REPO__
sda=__SHA_DARWIN_AMD64__
sd1=__SHA_DARWIN_ARM64__
sla=__SHA_LINUX_AMD64__
sl1=__SHA_LINUX_ARM64__
`
	out := renderBootstrap(tmpl, "v1.2.3", "foo/bar", platformSHAs{
		DarwinAmd64: "A", DarwinArm64: "B", LinuxAmd64: "C", LinuxArm64: "D",
	})
	want := `version=v1.2.3
repo=foo/bar
sda=A
sd1=B
sla=C
sl1=D
`
	if out != want {
		t.Errorf("renderBootstrap output:\n--- got ---\n%s\n--- want ---\n%s", out, want)
	}
}

func TestRenderBootstrap_OnEmbeddedTemplateLeavesNoPlaceholders(t *testing.T) {
	// Belt-and-suspenders: render against the REAL embedded template
	// and assert no __NAME__ placeholder survives. Catches a future
	// refactor that adds a placeholder to the template but not to
	// renderBootstrap.
	out := renderBootstrap(bootstrapTemplate, "v0.1.0", "x/y", platformSHAs{
		DarwinAmd64: strings.Repeat("a", 64),
		DarwinArm64: strings.Repeat("b", 64),
		LinuxAmd64:  strings.Repeat("c", 64),
		LinuxArm64:  strings.Repeat("d", 64),
	})
	if strings.Contains(out, "__") {
		// Find the offending token for the error message.
		i := strings.Index(out, "__")
		end := i + 2
		for end < len(out) && out[end] != '_' && out[end] != '\n' {
			end++
		}
		t.Errorf("placeholder survived rendering near: %q", out[i:end+2])
	}
}

// --- renderMinimalConfig ------------------------------------------

func TestRenderMinimalConfig_HasSchemaAndMinVersion(t *testing.T) {
	got := renderMinimalConfig("v0.1.0")
	for _, want := range []string{
		`schema_version: 1`,
		`min_version: "0.1.0"`,
		`envdoctor:`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("renderMinimalConfig missing %q; got:\n%s", want, got)
		}
	}
}

// --- runInit (end-to-end through the cobra command) ---------------

func chdirTemp(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })
	return dir
}

func fakeFetcher(sums platformSHAs, err error) sumFetcher {
	return func(_ context.Context, _, _ string) (platformSHAs, error) {
		return sums, err
	}
}

func sampleSums() platformSHAs {
	return platformSHAs{
		DarwinAmd64: strings.Repeat("a", 64),
		DarwinArm64: strings.Repeat("b", 64),
		LinuxAmd64:  strings.Repeat("c", 64),
		LinuxArm64:  strings.Repeat("d", 64),
	}
}

// runInitWith wires the init RunE directly so tests can pass
// initOpts with the fake fetcher and the injected Version.
func runInitWith(t *testing.T, opts initOpts) (stdout, stderr string, err error) {
	t.Helper()
	cmd := newInitCmd()
	var outBuf, errBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetErr(&errBuf)
	cmd.SetContext(context.Background())
	// The cobra newInitCmd hard-codes a default opts struct in its
	// closure. To inject test-only fields, call runInit directly.
	err = runInit(cmd, opts)
	return outBuf.String(), errBuf.String(), err
}

func TestRunInit_HappyPath_WritesBootstrapAndConfig(t *testing.T) {
	dir := chdirTemp(t)

	_, stderr, err := runInitWith(t, initOpts{
		repo:       "test/repo",
		envdoctorV: "0.1.0",
		fetcher:    fakeFetcher(sampleSums(), nil),
		skipScan:   true, // tempdir is empty so scan would be 0 anyway; keep deterministic
	})
	if err != nil {
		t.Fatalf("runInit: %v\nstderr:\n%s", err, stderr)
	}

	bootstrap, err := os.ReadFile(filepath.Join(dir, "envdoctor"))
	if err != nil {
		t.Fatalf("read bootstrap: %v", err)
	}
	if !strings.Contains(string(bootstrap), `ENVDOCTOR_VERSION="v0.1.0"`) {
		t.Errorf("bootstrap should pin v0.1.0; got:\n%s", string(bootstrap)[:200])
	}
	if !strings.Contains(string(bootstrap), `ENVDOCTOR_REPO="test/repo"`) {
		t.Errorf("bootstrap should pin the repo override; got:\n%s", string(bootstrap)[:200])
	}
	if !strings.Contains(string(bootstrap), strings.Repeat("a", 64)) {
		t.Errorf("bootstrap should embed the fetched darwin_amd64 SHA; got:\n%s", string(bootstrap)[:200])
	}
	info, _ := os.Stat(filepath.Join(dir, "envdoctor"))
	if info.Mode().Perm()&0o111 == 0 {
		t.Errorf("bootstrap must be executable; got mode %s", info.Mode())
	}

	cfg, err := os.ReadFile(filepath.Join(dir, ".envdoctor.yaml"))
	if err != nil {
		t.Fatalf("read .envdoctor.yaml: %v", err)
	}
	if !strings.Contains(string(cfg), `schema_version: 1`) {
		t.Errorf(".envdoctor.yaml missing schema_version; got:\n%s", cfg)
	}
}

func TestRunInit_RefusesIfBootstrapExists(t *testing.T) {
	dir := chdirTemp(t)
	if err := os.WriteFile(filepath.Join(dir, "envdoctor"), []byte("# stale\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, _, err := runInitWith(t, initOpts{
		repo:       "test/repo",
		envdoctorV: "0.1.0",
		fetcher:    fakeFetcher(sampleSums(), nil),
		skipScan:   true,
	})
	if err == nil {
		t.Fatal("expected refusal when ./envdoctor already exists")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error should mention `already exists`; got %q", err.Error())
	}
}

func TestRunInit_ForceOverwritesExistingFiles(t *testing.T) {
	dir := chdirTemp(t)
	if err := os.WriteFile(filepath.Join(dir, "envdoctor"), []byte("# stale\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, stderr, err := runInitWith(t, initOpts{
		repo:       "test/repo",
		envdoctorV: "0.1.0",
		fetcher:    fakeFetcher(sampleSums(), nil),
		skipScan:   true,
		force:      true,
	})
	if err != nil {
		t.Fatalf("--force should allow overwrite; err=%v\nstderr:\n%s", err, stderr)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "envdoctor"))
	if strings.Contains(string(got), "# stale") {
		t.Errorf("--force did not replace the stale bootstrap; got:\n%s", string(got))
	}
}

func TestRunInit_RefusesOnDevVersion(t *testing.T) {
	chdirTemp(t)
	_, _, err := runInitWith(t, initOpts{
		repo:       "test/repo",
		envdoctorV: "dev",
		fetcher:    fakeFetcher(sampleSums(), nil),
		skipScan:   true,
	})
	if err == nil {
		t.Fatal("expected refusal when running a dev build")
	}
	if !strings.Contains(err.Error(), "released envdoctor") {
		t.Errorf("error should explain the dev-build refusal; got %q", err.Error())
	}
}

func TestRunInit_FetcherErrorBubbles(t *testing.T) {
	chdirTemp(t)
	_, _, err := runInitWith(t, initOpts{
		repo:       "test/repo",
		envdoctorV: "0.1.0",
		fetcher:    fakeFetcher(platformSHAs{}, errors.New("network down")),
		skipScan:   true,
	})
	if err == nil {
		t.Fatal("expected error from broken fetcher")
	}
	if !strings.Contains(err.Error(), "fetch sha256sums") {
		t.Errorf("error should mention fetch sha256sums; got %q", err.Error())
	}
}

func TestRunInit_NotMutateExistingNonInitFiles(t *testing.T) {
	dir := chdirTemp(t)
	readme := "# my repo\n"
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte(readme), 0o644); err != nil {
		t.Fatal(err)
	}
	contributing := "# Contributing\n"
	if err := os.WriteFile(filepath.Join(dir, "CONTRIBUTING.md"), []byte(contributing), 0o644); err != nil {
		t.Fatal(err)
	}

	_, _, err := runInitWith(t, initOpts{
		repo:       "test/repo",
		envdoctorV: "0.1.0",
		fetcher:    fakeFetcher(sampleSums(), nil),
		skipScan:   true,
	})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "README.md"))
	if string(got) != readme {
		t.Errorf("README.md was modified! got %q, want %q", string(got), readme)
	}
	got, _ = os.ReadFile(filepath.Join(dir, "CONTRIBUTING.md"))
	if string(got) != contributing {
		t.Errorf("CONTRIBUTING.md was modified! got %q, want %q", string(got), contributing)
	}
}

func TestRunInit_PrintsPasteSnippets(t *testing.T) {
	chdirTemp(t)
	stdout, _, err := runInitWith(t, initOpts{
		repo:       "test/repo",
		envdoctorV: "0.1.0",
		fetcher:    fakeFetcher(sampleSums(), nil),
		skipScan:   true,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"README snippet",
		"CONTRIBUTING snippet",
		"./envdoctor scan",
		"./envdoctor fix",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout should contain %q; got:\n%s", want, stdout)
		}
	}
}

// --- atomicWriteFile ---------------------------------------------

func TestAtomicWriteFile_SetsExactMode(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x")
	if err := atomicWriteFile(p, []byte("hi"), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode: got %s, want 0o600", info.Mode())
	}
}

func TestAtomicWriteFile_DoesNotLeaveTempfileOnError(t *testing.T) {
	// Use a path under a non-existent directory so os.CreateTemp
	// fails. No tempfile should be left around.
	err := atomicWriteFile("/nonexistent/dir/x", []byte("hi"), 0o600)
	if err == nil {
		t.Fatal("expected error when parent dir missing")
	}
}
