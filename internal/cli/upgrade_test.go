// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- pickSumForPlatform -------------------------------------------

func TestPickSumForPlatform(t *testing.T) {
	sums := platformSHAs{
		DarwinAmd64: "AA",
		DarwinArm64: "BB",
		LinuxAmd64:  "CC",
		LinuxArm64:  "DD",
	}
	cases := []struct {
		goos, goarch, want string
	}{
		{"darwin", "amd64", "AA"},
		{"darwin", "arm64", "BB"},
		{"linux", "amd64", "CC"},
		{"linux", "arm64", "DD"},
	}
	for _, c := range cases {
		got, err := pickSumForPlatform(sums, c.goos, c.goarch)
		if err != nil {
			t.Errorf("%s/%s: %v", c.goos, c.goarch, err)
		}
		if got != c.want {
			t.Errorf("%s/%s: got %q want %q", c.goos, c.goarch, got, c.want)
		}
	}
	if _, err := pickSumForPlatform(sums, "windows", "amd64"); err == nil {
		t.Errorf("windows must error for v0.x scope")
	}
}

// --- isBootstrapManaged -------------------------------------------

func TestIsBootstrapManaged(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"/Users/alice/.cache/envdoctor/versions/v0.1.0/darwin_arm64/envdoctor", true},
		{"/home/bob/.cache/envdoctor/versions/v0.2.0/linux_amd64/envdoctor", true},
		{"/usr/local/bin/envdoctor", false},
		{"/Users/alice/.local/bin/envdoctor", false},
		{"/tmp/envdoctor", false},
		{"", false},
	}
	for _, c := range cases {
		if got := isBootstrapManaged(c.path); got != c.want {
			t.Errorf("isBootstrapManaged(%q): got %v want %v", c.path, got, c.want)
		}
	}
}

// --- extractFileFromTarGz -----------------------------------------

func buildTestTarGz(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(body))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestExtractFileFromTarGz_HappyPath(t *testing.T) {
	archive := buildTestTarGz(t, map[string]string{
		"envdoctor": "binary body",
		"README.md": "# readme",
		"LICENSE":   "apache",
		"CHANGELOG": "0.1.0\n",
	})
	got, err := extractEnvdoctorBinary(archive)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "binary body" {
		t.Errorf("got %q, want %q", string(got), "binary body")
	}
}

func TestExtractFileFromTarGz_MissingEntryErrors(t *testing.T) {
	archive := buildTestTarGz(t, map[string]string{"OTHER": "x"})
	_, err := extractEnvdoctorBinary(archive)
	if err == nil {
		t.Fatal("expected error when entry not present")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should explain `not found`; got %q", err.Error())
	}
}

func TestExtractFileFromTarGz_NotAGzipErrors(t *testing.T) {
	_, err := extractEnvdoctorBinary([]byte("not a gzip stream"))
	if err == nil {
		t.Fatal("expected gzip error")
	}
}

// --- runUpgrade (mocked clients) ---------------------------------

// fakeUpgradeClients bundles the three injectable functions plus
// canned data, so each test can read them off a single struct.
type fakeUpgradeClients struct {
	latest        string
	resolveErr    error
	sums          platformSHAs
	sumsErr       error
	tarball       []byte
	tarballName   string
	downloadErr   error
	resolveCalls  int
	sumsCalls     int
	downloadCalls int
}

func (c *fakeUpgradeClients) resolver() tagResolver {
	return func(_ context.Context, _ string) (string, error) {
		c.resolveCalls++
		return c.latest, c.resolveErr
	}
}

func (c *fakeUpgradeClients) fetcher() sumFetcher {
	return func(_ context.Context, _, _ string) (platformSHAs, error) {
		c.sumsCalls++
		return c.sums, c.sumsErr
	}
}

func (c *fakeUpgradeClients) downloader() binaryDownloader {
	return func(_ context.Context, _, _, _, _ string) ([]byte, string, error) {
		c.downloadCalls++
		return c.tarball, c.tarballName, c.downloadErr
	}
}

func runUpgradeWith(t *testing.T, opts upgradeOpts) (stdout, stderr string, err error) {
	t.Helper()
	cmd := newUpgradeCmd()
	var outBuf, errBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetErr(&errBuf)
	cmd.SetContext(context.Background())
	err = runUpgrade(cmd, opts)
	return outBuf.String(), errBuf.String(), err
}

// makeFakeRelease builds a fake release tarball whose internal
// `envdoctor` file body is `body`. Returns the tarball bytes and
// the matching platformSHAs (real digest for the running platform,
// zeros for the others — same fail-secure pattern as the bootstrap
// tests).
func makeFakeRelease(t *testing.T, body string, goos, goarch string) ([]byte, platformSHAs, string) {
	t.Helper()
	tarball := buildTestTarGz(t, map[string]string{"envdoctor": body})
	sum := sha256.Sum256(tarball)
	hex := hex.EncodeToString(sum[:])
	bogus := strings.Repeat("0", 64)
	sums := platformSHAs{bogus, bogus, bogus, bogus}
	switch goos + "/" + goarch {
	case "darwin/amd64":
		sums.DarwinAmd64 = hex
	case "darwin/arm64":
		sums.DarwinArm64 = hex
	case "linux/amd64":
		sums.LinuxAmd64 = hex
	case "linux/arm64":
		sums.LinuxArm64 = hex
	}
	name := fmt.Sprintf("envdoctor_0.2.0_%s_%s.tar.gz", goos, goarch)
	return tarball, sums, name
}

func TestRunUpgrade_RefusesOnDevBuild(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "envdoctor")
	if err := os.WriteFile(dest, []byte("# pretend old"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, _, err := runUpgradeWith(t, upgradeOpts{
		currentPath:    dest,
		currentVersion: "dev",
		repo:           "test/repo",
	})
	if err == nil {
		t.Fatal("expected refusal on dev build")
	}
	if !strings.Contains(err.Error(), "released envdoctor") {
		t.Errorf("error should explain dev refusal; got %q", err.Error())
	}
}

func TestRunUpgrade_RefusesBootstrapManagedCopy(t *testing.T) {
	dest := filepath.Join(t.TempDir(), ".cache", "envdoctor", "versions", "v0.1.0", "darwin_arm64", "envdoctor")
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest, []byte("# bootstrap copy"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, _, err := runUpgradeWith(t, upgradeOpts{
		currentPath:    dest,
		currentVersion: "0.1.0",
		repo:           "test/repo",
	})
	if err == nil {
		t.Fatal("expected refusal for bootstrap-managed binary")
	}
	if !strings.Contains(err.Error(), "bootstrap-managed") {
		t.Errorf("error should explain bootstrap-managed; got %q", err.Error())
	}
}

func TestRunUpgrade_NoOpWhenAlreadyOnTarget(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "envdoctor")
	if err := os.WriteFile(dest, []byte("# v0.2.0"), 0o755); err != nil {
		t.Fatal(err)
	}
	c := &fakeUpgradeClients{latest: "v0.2.0"}
	_, stderr, err := runUpgradeWith(t, upgradeOpts{
		currentPath:    dest,
		currentVersion: "0.2.0",
		repo:           "test/repo",
		resolver:       c.resolver(),
		fetcher:        c.fetcher(),
		downloader:     c.downloader(),
		goos:           "darwin",
		goarch:         "arm64",
	})
	if err != nil {
		t.Fatalf("expected nil err for no-op; got %v", err)
	}
	if !strings.Contains(stderr, "already on v0.2.0") {
		t.Errorf("stderr should say `already on`; got %q", stderr)
	}
	// Must NOT have downloaded or even fetched sums.
	if c.sumsCalls != 0 || c.downloadCalls != 0 {
		t.Errorf("no-op must not fetch/download; got sumsCalls=%d downloadCalls=%d", c.sumsCalls, c.downloadCalls)
	}
}

func TestRunUpgrade_DryRunDoesNotReplace(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "envdoctor")
	original := []byte("# original v0.1.0")
	if err := os.WriteFile(dest, original, 0o755); err != nil {
		t.Fatal(err)
	}
	c := &fakeUpgradeClients{latest: "v0.2.0"}
	stdout, stderr, err := runUpgradeWith(t, upgradeOpts{
		currentPath:    dest,
		currentVersion: "0.1.0",
		repo:           "test/repo",
		resolver:       c.resolver(),
		fetcher:        c.fetcher(),
		downloader:     c.downloader(),
		dryRun:         true,
	})
	if err != nil {
		t.Fatalf("dry-run unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "would upgrade v0.1.0 → v0.2.0") {
		t.Errorf("stdout missing dry-run plan line; got %q", stdout)
	}
	got, _ := os.ReadFile(dest)
	if !bytes.Equal(got, original) {
		t.Errorf("dry-run replaced the binary! got %q", string(got))
	}
	// And must not have hit the network for sums/download.
	if c.sumsCalls != 0 || c.downloadCalls != 0 {
		t.Errorf("dry-run must skip downloads; got sumsCalls=%d downloadCalls=%d stderr=%s", c.sumsCalls, c.downloadCalls, stderr)
	}
}

func TestRunUpgrade_HappyPath_ReplacesAtomically(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "envdoctor")
	if err := os.WriteFile(dest, []byte("# old"), 0o755); err != nil {
		t.Fatal(err)
	}
	const newBody = "#!/bin/sh\necho upgraded-v0.2.0\n"
	tarball, sums, name := makeFakeRelease(t, newBody, "darwin", "arm64")
	c := &fakeUpgradeClients{
		latest:      "v0.2.0",
		sums:        sums,
		tarball:     tarball,
		tarballName: name,
	}
	_, stderr, err := runUpgradeWith(t, upgradeOpts{
		currentPath:    dest,
		currentVersion: "0.1.0",
		repo:           "test/repo",
		resolver:       c.resolver(),
		fetcher:        c.fetcher(),
		downloader:     c.downloader(),
		goos:           "darwin",
		goarch:         "arm64",
	})
	if err != nil {
		t.Fatalf("runUpgrade: %v\nstderr:\n%s", err, stderr)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != newBody {
		t.Errorf("binary not replaced; got %q", string(got))
	}
	info, _ := os.Stat(dest)
	if info.Mode().Perm()&0o111 == 0 {
		t.Errorf("replaced binary should be executable; got %s", info.Mode())
	}
	for _, want := range []string{"current v0.1.0", "target v0.2.0", "SHA-256 verified", "upgraded"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr missing %q; got:\n%s", want, stderr)
		}
	}
}

func TestRunUpgrade_PinnedVersionSkipsResolver(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "envdoctor")
	if err := os.WriteFile(dest, []byte("# old"), 0o755); err != nil {
		t.Fatal(err)
	}
	tarball, sums, name := makeFakeRelease(t, "# new", "linux", "amd64")
	c := &fakeUpgradeClients{
		// resolver returns a different version on purpose; the
		// --version flag must beat it.
		latest:      "v9.9.9",
		sums:        sums,
		tarball:     tarball,
		tarballName: name,
	}
	_, _, err := runUpgradeWith(t, upgradeOpts{
		version:        "v0.2.0",
		currentPath:    dest,
		currentVersion: "0.1.0",
		repo:           "test/repo",
		resolver:       c.resolver(),
		fetcher:        c.fetcher(),
		downloader:     c.downloader(),
		goos:           "linux",
		goarch:         "amd64",
	})
	if err != nil {
		t.Fatal(err)
	}
	if c.resolveCalls != 0 {
		t.Errorf("--version should skip the latest-tag resolver; calls=%d", c.resolveCalls)
	}
}

func TestRunUpgrade_SHAMismatchAborts(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "envdoctor")
	original := []byte("# original")
	if err := os.WriteFile(dest, original, 0o755); err != nil {
		t.Fatal(err)
	}
	tarball, _, name := makeFakeRelease(t, "# new", "darwin", "arm64")
	bogus := strings.Repeat("0", 64)
	c := &fakeUpgradeClients{
		latest:      "v0.2.0",
		sums:        platformSHAs{bogus, bogus, bogus, bogus},
		tarball:     tarball,
		tarballName: name,
	}
	_, _, err := runUpgradeWith(t, upgradeOpts{
		currentPath:    dest,
		currentVersion: "0.1.0",
		repo:           "test/repo",
		resolver:       c.resolver(),
		fetcher:        c.fetcher(),
		downloader:     c.downloader(),
		goos:           "darwin",
		goarch:         "arm64",
	})
	if err == nil {
		t.Fatal("expected SHA mismatch error")
	}
	if !strings.Contains(err.Error(), "SHA-256 mismatch") {
		t.Errorf("error should mention SHA-256 mismatch; got %q", err.Error())
	}
	// And the binary at dest must be untouched.
	got, _ := os.ReadFile(dest)
	if !bytes.Equal(got, original) {
		t.Errorf("SHA mismatch must leave the original binary in place")
	}
}

func TestRunUpgrade_ResolverErrorBubbles(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "envdoctor")
	if err := os.WriteFile(dest, []byte("# old"), 0o755); err != nil {
		t.Fatal(err)
	}
	c := &fakeUpgradeClients{resolveErr: errors.New("dns timeout")}
	_, _, err := runUpgradeWith(t, upgradeOpts{
		currentPath:    dest,
		currentVersion: "0.1.0",
		repo:           "test/repo",
		resolver:       c.resolver(),
		fetcher:        c.fetcher(),
		downloader:     c.downloader(),
	})
	if err == nil {
		t.Fatal("expected resolver error to bubble")
	}
	if !strings.Contains(err.Error(), "resolve latest tag") {
		t.Errorf("error should explain `resolve latest tag`; got %q", err.Error())
	}
}
