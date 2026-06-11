// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

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

// bootstrapVersion is the fake version pinned into the bootstrap
// template during these tests. Different from fakeVersion so a
// stale cache from install.sh tests can't accidentally satisfy a
// bootstrap test.
const bootstrapVersion = "v8.8.8"

// renderedBootstrap reads scripts/bootstrap.template.sh, fills
// the placeholders with the supplied values, and writes the
// result to a tempfile chmod +x. Returns the path. Version is
// always bootstrapVersion today; the parameter is kept so a
// future test can render multiple versions side-by-side.
func renderedBootstrap(t *testing.T, baseURL, sumDarwinArm, sumDarwinAmd, sumLinuxArm, sumLinuxAmd string) string {
	t.Helper()
	tmpl := findRepoFile(t, filepath.Join("internal", "cli", "bootstrap_template.sh"))
	raw, err := os.ReadFile(tmpl)
	if err != nil {
		t.Fatalf("read template: %v", err)
	}
	rep := strings.NewReplacer(
		"__ENVDOCTOR_VERSION__", bootstrapVersion,
		"__ENVDOCTOR_REPO__", fakeRepo,
		"__SHA_DARWIN_AMD64__", sumDarwinAmd,
		"__SHA_DARWIN_ARM64__", sumDarwinArm,
		"__SHA_LINUX_AMD64__", sumLinuxAmd,
		"__SHA_LINUX_ARM64__", sumLinuxArm,
	)
	out := rep.Replace(string(raw))
	// Set ENVDOCTOR_BASE_URL from the test by writing an override
	// near the top — easier than threading env through exec since
	// it's already a documented test seam in the template.
	out = strings.Replace(out,
		`ENVDOCTOR_BASE_URL="${ENVDOCTOR_BASE_URL:-https://github.com}"`,
		fmt.Sprintf(`ENVDOCTOR_BASE_URL="%s"`, baseURL),
		1)

	dest := filepath.Join(t.TempDir(), "envdoctor")
	if err := os.WriteFile(dest, []byte(out), 0o755); err != nil {
		t.Fatal(err)
	}
	return dest
}

// fakeReleaseForBootstrap is the bootstrap-flavored equivalent of
// fakeRelease: same tarball shape, named for `bootstrapVersion`.
// Returns (tarballName, tarballBytes, sha256-hex).
func fakeReleaseForBootstrap(t *testing.T) (string, []byte, string) {
	return fakeReleaseForBootstrapWithBody(t, fakeBinaryBody)
}

// bootstrapTestServer wires only the one URL the bootstrap hits:
//
//	/<repo>/releases/download/<tag>/<tarball>
//
// No latest-redirect, no sha256sums.txt — those are install.sh's
// job. The bootstrap trusts the SHA baked into its source.
func bootstrapTestServer(t *testing.T, tarballName string, tarball []byte) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	path := fmt.Sprintf("/%s/releases/download/%s/%s", fakeRepo, bootstrapVersion, tarballName)
	mux.HandleFunc(path, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(tarball)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

// --- the tests ----------------------------------------------------

func TestBootstrap_HappyPath_DownloadsVerifiesExecs(t *testing.T) {
	tarballName, tarball, sum := fakeReleaseForBootstrap(t)
	server := bootstrapTestServer(t, tarballName, tarball)

	// Fill only the running platform's SHA with the real digest;
	// the other three get placeholder zeros so the bootstrap still
	// loads cleanly but would fail-secure if uname lied.
	sums := platformSums(t, sum)
	script := renderedBootstrap(t, server.URL,
		sums.DarwinArm64, sums.DarwinAmd64, sums.LinuxArm64, sums.LinuxAmd64)

	// HOME=tempdir so ~/.cache/envdoctor/... lands in the tempdir.
	home := t.TempDir()
	stdout, stderr, code := runShell(t, script, []string{}, map[string]string{"HOME": home})
	if code != 0 {
		t.Fatalf("bootstrap exit %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "envdoctor-fake-installed") {
		t.Errorf("bootstrap should exec the cached binary; stdout:\n%s", stdout)
	}

	// Cache layout pinned: ~/.cache/envdoctor/versions/<tag>/<os>_<arch>/envdoctor
	cached := filepath.Join(home, ".cache", "envdoctor", "versions", bootstrapVersion,
		fmt.Sprintf("%s_%s", runtime.GOOS, runtime.GOARCH), "envdoctor")
	if _, err := os.Stat(cached); err != nil {
		t.Errorf("expected cached binary at %s; %v", cached, err)
	}
}

func TestBootstrap_ReusesCacheOnSecondInvocation(t *testing.T) {
	tarballName, tarball, sum := fakeReleaseForBootstrap(t)
	server := bootstrapTestServer(t, tarballName, tarball)
	sums := platformSums(t, sum)
	script := renderedBootstrap(t, server.URL,
		sums.DarwinArm64, sums.DarwinAmd64, sums.LinuxArm64, sums.LinuxAmd64)
	home := t.TempDir()

	// First run: warms the cache.
	if _, stderr, code := runShell(t, script, nil, map[string]string{"HOME": home}); code != 0 {
		t.Fatalf("first run exit %d\nstderr:\n%s", code, stderr)
	}

	// Second run: server returns 500 for any further request, so
	// if the bootstrap touches it the test fails. Cache-hit must
	// be a pure exec.
	server.Close()
	stdout, stderr, code := runShell(t, script, nil, map[string]string{"HOME": home})
	if code != 0 {
		t.Fatalf("second run with offline server should still succeed via cache; exit %d\nstderr:\n%s", code, stderr)
	}
	if !strings.Contains(stdout, "envdoctor-fake-installed") {
		t.Errorf("cached binary should still exec; stdout:\n%s", stdout)
	}
}

func TestBootstrap_ChecksumMismatchAborts(t *testing.T) {
	tarballName, tarball, _ := fakeReleaseForBootstrap(t)
	server := bootstrapTestServer(t, tarballName, tarball)

	// All four SHAs are wrong. The bootstrap will pick the one
	// matching the runtime platform and reject the tarball.
	bogus := "0000000000000000000000000000000000000000000000000000000000000000"
	script := renderedBootstrap(t, server.URL, bogus, bogus, bogus, bogus)
	home := t.TempDir()

	_, stderr, code := runShell(t, script, nil, map[string]string{"HOME": home})
	if code == 0 {
		t.Fatalf("expected non-zero exit on SHA mismatch; got 0\nstderr:\n%s", stderr)
	}
	if !strings.Contains(stderr, "SHA-256 mismatch") {
		t.Errorf("stderr should mention SHA-256 mismatch; got:\n%s", stderr)
	}
	// And the cache must NOT have been polluted with the
	// bad-checksum tarball.
	cached := filepath.Join(home, ".cache", "envdoctor", "versions", bootstrapVersion,
		fmt.Sprintf("%s_%s", runtime.GOOS, runtime.GOARCH), "envdoctor")
	if _, err := os.Stat(cached); err == nil {
		t.Errorf("SHA mismatch must abort BEFORE writing to the cache; found %s", cached)
	}
}

func TestBootstrap_ForwardsArgsToBinary(t *testing.T) {
	// The default fake binary prints "envdoctor-fake-installed"
	// regardless of args. Swap to one that prints its arguments so
	// we can verify the bootstrap's `exec "$bin" "$@"` actually
	// forwards them.
	tarballName, tarball, sum := fakeReleaseForBootstrapWithBody(t, "#!/bin/sh\necho \"args:$*\"\n")
	server := bootstrapTestServer(t, tarballName, tarball)
	sums := platformSums(t, sum)
	script := renderedBootstrap(t, server.URL,
		sums.DarwinArm64, sums.DarwinAmd64, sums.LinuxArm64, sums.LinuxAmd64)
	home := t.TempDir()

	stdout, stderr, code := runShell(t, script, []string{"scan", "--json"}, map[string]string{"HOME": home})
	if code != 0 {
		t.Fatalf("exit %d\nstderr:\n%s", code, stderr)
	}
	if !strings.Contains(stdout, "args:scan --json") {
		t.Errorf("bootstrap must forward args via \"$@\"; got stdout:\n%s", stdout)
	}
}

// --- helpers shared with install_sh_test.go --- ------------------

type platformSumSet struct {
	DarwinAmd64 string
	DarwinArm64 string
	LinuxAmd64  string
	LinuxArm64  string
}

// platformSums returns the SHA set with the running platform's
// entry set to actual and the others set to a clearly-wrong
// placeholder. This way a uname lie in CI can't accidentally pass.
func platformSums(t *testing.T, actual string) platformSumSet {
	t.Helper()
	bogus := strings.Repeat("0", 64)
	set := platformSumSet{bogus, bogus, bogus, bogus}
	switch runtime.GOOS + "/" + runtime.GOARCH {
	case "darwin/amd64":
		set.DarwinAmd64 = actual
	case "darwin/arm64":
		set.DarwinArm64 = actual
	case "linux/amd64":
		set.LinuxAmd64 = actual
	case "linux/arm64":
		set.LinuxArm64 = actual
	default:
		t.Skipf("unsupported runtime platform for bootstrap tests: %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	return set
}

// fakeReleaseForBootstrapWithBody returns the same shape as
// fakeReleaseForBootstrap but lets the caller override the
// binary's body. Used by the args-forwarding test.
func fakeReleaseForBootstrapWithBody(t *testing.T, body string) (string, []byte, string) {
	t.Helper()
	versionNoV := strings.TrimPrefix(bootstrapVersion, "v")
	name := fmt.Sprintf("envdoctor_%s_%s_%s.tar.gz", versionNoV, runtime.GOOS, runtime.GOARCH)

	var buf strings.Builder
	gz := gzip.NewWriter(stringWriter{&buf})
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{
		Name: "envdoctor",
		Mode: 0o755,
		Size: int64(len(body)),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte(body)); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	tarball := []byte(buf.String())
	sum := sha256.Sum256(tarball)
	return name, tarball, hex.EncodeToString(sum[:])
}

// runShell runs a shell script via /bin/sh with the given args
// and env overrides. Returns stdout, stderr, exit code. Like
// runInstaller, but takes an explicit script path so it can be
// used for any generated script (the bootstrap, etc.).
func runShell(t *testing.T, scriptPath string, args []string, env map[string]string) (stdout, stderr string, exitCode int) {
	t.Helper()
	cmdArgs := append([]string{scriptPath}, args...)
	cmd := exec.Command("/bin/sh", cmdArgs...)
	baseEnv := []string{
		"PATH=" + os.Getenv("PATH"),
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
			t.Fatalf("script failed to start: %v", err)
		}
	}
	return outBuf.String(), errBuf.String(), exitCode
}
