// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

// Package installtest exercises scripts/install.sh end-to-end
// against a local httptest.Server impersonating GitHub Releases.
//
// The script's contract is unforgiving — checksum-mismatched
// downloads must abort, atomic installs must not leave half-
// written binaries, ENVDOCTOR_VERSION must skip the latest-tag
// redirect, ENVDOCTOR_INSTALL_DIR must override the default
// location. Going through real shell process plumbing instead
// of mocking is the only way to catch the subtle bugs.
package installtest

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const (
	fakeVersion = "9.9.9"
	fakeTag     = "v" + fakeVersion
	fakeRepo    = "fake/envdoctor"
	// The fake binary is a tiny shell script that prints a fixed
	// banner so the test can verify "this is the bytes install.sh
	// just placed" without needing a real Go cross-compile.
	fakeBinaryBody = "#!/bin/sh\necho envdoctor-fake-installed\n"
)

// fakeRelease builds the tarball + sums file install.sh expects
// at /<repo>/releases/download/<tag>/. Returns the tarball bytes
// and the sums-file body.
func fakeRelease(t *testing.T) (tarballName string, tarball []byte, sums []byte) {
	t.Helper()
	os := runtime.GOOS
	arch := runtime.GOARCH
	tarballName = fmt.Sprintf("envdoctor_%s_%s_%s.tar.gz", fakeVersion, os, arch)

	var buf strings.Builder
	gz := gzip.NewWriter(stringWriter{&buf})
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{
		Name: "envdoctor",
		Mode: 0o755,
		Size: int64(len(fakeBinaryBody)),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte(fakeBinaryBody)); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	tarball = []byte(buf.String())

	sum := sha256.Sum256(tarball)
	sums = []byte(fmt.Sprintf("%s  %s\n", hex.EncodeToString(sum[:]), tarballName))
	return tarballName, tarball, sums
}

// stringWriter adapts a strings.Builder to io.Writer (Builder
// satisfies io.Writer already but the linter prefers an explicit
// adapter when we're chaining through gzip).
type stringWriter struct{ b *strings.Builder }

func (s stringWriter) Write(p []byte) (int, error) { return s.b.Write(p) }

// newFakeReleaseServer wires the three URLs install.sh hits:
//   - /<repo>/releases/latest      → 302 to /releases/tag/<tag>
//   - /<repo>/releases/download/<tag>/<tarball>  → tarball bytes
//   - /<repo>/releases/download/<tag>/sha256sums.txt → sums body
//
// The tamper hook lets a test rewrite the sums body on the wire
// to simulate a checksum mismatch without touching the tarball.
func newFakeReleaseServer(t *testing.T, tamper func(name string, body []byte) []byte) *httptest.Server {
	t.Helper()
	name, tarball, sums := fakeRelease(t)
	if tamper == nil {
		tamper = func(_ string, b []byte) []byte { return b }
	}

	mux := http.NewServeMux()
	mux.HandleFunc(fmt.Sprintf("/%s/releases/latest", fakeRepo), func(w http.ResponseWriter, _ *http.Request) {
		// The real GitHub /releases/latest endpoint 302s to
		// /releases/tag/<tag> with the resolved tag in the path.
		w.Header().Set("Location", fmt.Sprintf("/%s/releases/tag/%s", fakeRepo, fakeTag))
		w.WriteHeader(http.StatusFound)
	})
	tarballPath := fmt.Sprintf("/%s/releases/download/%s/%s", fakeRepo, fakeTag, name)
	mux.HandleFunc(tarballPath, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(tamper("tarball", tarball))
	})
	sumsPath := fmt.Sprintf("/%s/releases/download/%s/sha256sums.txt", fakeRepo, fakeTag)
	mux.HandleFunc(sumsPath, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(tamper("sums", sums))
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

// runInstaller invokes scripts/install.sh in a clean subshell with
// the given env overrides. Returns stdout, stderr, exit code.
func runInstaller(t *testing.T, env map[string]string) (stdout, stderr string, exitCode int) {
	t.Helper()
	// Locate scripts/install.sh relative to the test source. We
	// walk up from the test binary's CWD until we find go.mod.
	scriptPath := findRepoFile(t, filepath.Join("scripts", "install.sh"))

	cmd := exec.Command("/bin/sh", scriptPath)
	// Pass a minimal-but-real PATH so curl/sha256sum can be found.
	baseEnv := []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + os.Getenv("HOME"),
		"TMPDIR=" + os.Getenv("TMPDIR"),
	}
	for k, v := range env {
		baseEnv = append(baseEnv, k+"="+v)
	}
	cmd.Env = baseEnv

	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()

	exitCode = 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		} else {
			t.Fatalf("install.sh failed to start: %v", err)
		}
	}
	return outBuf.String(), errBuf.String(), exitCode
}

func findRepoFile(t *testing.T, relPath string) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		candidate := filepath.Join(dir, "go.mod")
		if _, err := os.Stat(candidate); err == nil {
			return filepath.Join(dir, relPath)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatalf("could not find go.mod above %s", relPath)
	return ""
}

// --- the tests ----------------------------------------------------

func TestInstall_HappyPath_PinnedVersion(t *testing.T) {
	server := newFakeReleaseServer(t, nil)
	installDir := t.TempDir()

	stdout, stderr, code := runInstaller(t, map[string]string{
		"ENVDOCTOR_BASE_URL":    server.URL,
		"ENVDOCTOR_REPO":        fakeRepo,
		"ENVDOCTOR_VERSION":     fakeTag,
		"ENVDOCTOR_INSTALL_DIR": installDir,
	})
	if code != 0 {
		t.Fatalf("installer exit %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}

	dest := filepath.Join(installDir, "envdoctor")
	info, err := os.Stat(dest)
	if err != nil {
		t.Fatalf("expected %s installed; %v", dest, err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Errorf("installed binary should be executable; got %s", info.Mode())
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != fakeBinaryBody {
		t.Errorf("installed bytes don't match the release; got %q", string(got))
	}
	// The script invokes `dest version` at the end; the fake binary
	// is our shell script that prints the fixed banner.
	if !strings.Contains(stdout, "envdoctor-fake-installed") {
		t.Errorf("installer should exec the installed binary; stdout:\n%s", stdout)
	}
	if !strings.Contains(stderr, "verifying SHA-256") {
		t.Errorf("installer should mention SHA-256 verification; stderr:\n%s", stderr)
	}
}

func TestInstall_LatestRedirectResolvesTag(t *testing.T) {
	server := newFakeReleaseServer(t, nil)
	installDir := t.TempDir()

	// No ENVDOCTOR_VERSION → installer must hit /releases/latest and
	// read the redirect to discover the tag.
	stdout, stderr, code := runInstaller(t, map[string]string{
		"ENVDOCTOR_BASE_URL":    server.URL,
		"ENVDOCTOR_REPO":        fakeRepo,
		"ENVDOCTOR_INSTALL_DIR": installDir,
	})
	if code != 0 {
		t.Fatalf("installer exit %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	dest := filepath.Join(installDir, "envdoctor")
	if _, err := os.Stat(dest); err != nil {
		t.Fatalf("expected %s installed via latest-tag resolution; %v", dest, err)
	}
}

func TestInstall_ChecksumMismatchAborts(t *testing.T) {
	server := newFakeReleaseServer(t, func(kind string, body []byte) []byte {
		if kind != "sums" {
			return body
		}
		// Flip one hex char in the sum so verification fails.
		bad := make([]byte, len(body))
		copy(bad, body)
		bad[0] = 'f' // The first byte of a sha256 hex digest.
		if body[0] == 'f' {
			bad[0] = '0'
		}
		return bad
	})
	installDir := t.TempDir()

	_, stderr, code := runInstaller(t, map[string]string{
		"ENVDOCTOR_BASE_URL":    server.URL,
		"ENVDOCTOR_REPO":        fakeRepo,
		"ENVDOCTOR_VERSION":     fakeTag,
		"ENVDOCTOR_INSTALL_DIR": installDir,
	})
	if code == 0 {
		t.Fatalf("expected non-zero exit on checksum mismatch; got 0\nstderr:\n%s", stderr)
	}
	if !strings.Contains(stderr, "checksum mismatch") {
		t.Errorf("stderr should explain checksum mismatch; got:\n%s", stderr)
	}
	// And no binary should have landed in the install dir.
	if _, err := os.Stat(filepath.Join(installDir, "envdoctor")); err == nil {
		t.Errorf("checksum failure must abort BEFORE the binary is installed")
	}
}

func TestInstall_MissingFromSumsAborts(t *testing.T) {
	server := newFakeReleaseServer(t, func(kind string, body []byte) []byte {
		if kind != "sums" {
			return body
		}
		return []byte("0000000000000000000000000000000000000000000000000000000000000000  some-other-file.tar.gz\n")
	})
	installDir := t.TempDir()

	_, stderr, code := runInstaller(t, map[string]string{
		"ENVDOCTOR_BASE_URL":    server.URL,
		"ENVDOCTOR_REPO":        fakeRepo,
		"ENVDOCTOR_VERSION":     fakeTag,
		"ENVDOCTOR_INSTALL_DIR": installDir,
	})
	if code == 0 {
		t.Fatalf("expected non-zero exit when tarball isn't in sums; got 0\nstderr:\n%s", stderr)
	}
	if !strings.Contains(stderr, "not listed in sha256sums.txt") {
		t.Errorf("stderr should explain the missing-entry condition; got:\n%s", stderr)
	}
}
