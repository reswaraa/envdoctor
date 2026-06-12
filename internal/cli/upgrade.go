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
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// tagResolver resolves the "latest" GitHub release for a repo to
// its tag string. In production this calls fetchLatestTagFromGitHub;
// tests pass a stub that returns a fixed tag without hitting the network.
type tagResolver func(ctx context.Context, repo string) (string, error)

// binaryDownloader fetches the platform-specific release tarball
// and returns its bytes plus the filename. The filename is part
// of the return so the caller can use it in checksum verification
// and error messages without re-deriving it.
type binaryDownloader func(ctx context.Context, repo, tag, goos, goarch string) (tarball []byte, tarballName string, err error)

type upgradeOpts struct {
	version string // pin a specific tag (empty = latest)
	dryRun  bool

	// Test seams. All zero values fall through to production
	// implementations (fetchSumsFromGitHub, fetchLatestTagFromGitHub,
	// downloadBinaryFromGitHub, os.Executable, runtime.Version).
	currentPath    string
	currentVersion string
	repo           string
	fetcher        sumFetcher
	resolver       tagResolver
	downloader     binaryDownloader
	goos           string
	goarch         string
}

func newUpgradeCmd() *cobra.Command {
	f := upgradeOpts{}
	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Upgrade the global envdoctor binary in place",
		Long: `Upgrade resolves the latest envdoctor release (or the tag
passed to --version), downloads the platform tarball, verifies its
SHA-256, and atomically replaces the running binary.

Refuses to touch copies installed under ~/.cache/envdoctor/versions/
— those are managed by per-repo bootstraps. To bump a bootstrap pin,
re-run ` + "`envdoctor init`" + ` in that repo.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runUpgrade(cmd, f)
		},
	}
	cmd.Flags().StringVar(&f.version, "version", "", "pin a specific tag (e.g. v0.2.0); empty = latest")
	cmd.Flags().BoolVar(&f.dryRun, "dry-run", false, "print the upgrade plan without downloading or replacing")
	return cmd
}

func runUpgrade(cmd *cobra.Command, opts upgradeOpts) error {
	ctx := cmd.Context()
	stdout := cmd.OutOrStdout()
	stderr := cmd.ErrOrStderr()

	// --- defaults for production paths ---------------------------
	if opts.currentPath == "" {
		exe, err := os.Executable()
		if err != nil {
			return &exitErr{code: ExitCrashed, err: fmt.Errorf("os.Executable: %w", err)}
		}
		opts.currentPath = exe
	}
	if opts.currentVersion == "" {
		opts.currentVersion = Version
	}
	if opts.repo == "" {
		opts.repo = defaultRepo
	}
	if opts.fetcher == nil {
		opts.fetcher = fetchSumsFromGitHub
	}
	if opts.resolver == nil {
		opts.resolver = fetchLatestTagFromGitHub
	}
	if opts.downloader == nil {
		opts.downloader = downloadBinaryFromGitHub
	}
	if opts.goos == "" {
		opts.goos = runtime.GOOS
	}
	if opts.goarch == "" {
		opts.goarch = runtime.GOARCH
	}

	// --- refuse on dev builds ------------------------------------
	if opts.currentVersion == "" || opts.currentVersion == "dev" {
		return &exitErr{code: ExitCrashed, err: fmt.Errorf(
			"`envdoctor upgrade` needs a released envdoctor (Version=%q); install via curl|sh first",
			opts.currentVersion,
		)}
	}

	// --- refuse on bootstrap-managed copies ----------------------
	if isBootstrapManaged(opts.currentPath) {
		return &exitErr{code: ExitCrashed, err: fmt.Errorf(
			"%s is bootstrap-managed; bump the version pin via `envdoctor init --force` in the owning repo",
			opts.currentPath,
		)}
	}

	// --- resolve target version ----------------------------------
	target := opts.version
	if target == "" {
		var err error
		target, err = opts.resolver(ctx, opts.repo)
		if err != nil {
			return &exitErr{code: ExitCrashed, err: fmt.Errorf("resolve latest tag: %w", err)}
		}
	}
	if !strings.HasPrefix(target, "v") {
		target = "v" + target
	}

	currentCanonical := opts.currentVersion
	if !strings.HasPrefix(currentCanonical, "v") {
		currentCanonical = "v" + currentCanonical
	}
	if currentCanonical == target {
		writef(stderr, "envdoctor: already on %s\n", target)
		return nil
	}

	writef(stderr, "envdoctor: current %s → target %s\n", currentCanonical, target)

	if opts.dryRun {
		writef(stdout, "would upgrade %s → %s (binary: %s)\n", currentCanonical, target, opts.currentPath)
		return nil
	}

	// --- fetch sums + download tarball ---------------------------
	sums, err := opts.fetcher(ctx, opts.repo, target)
	if err != nil {
		return &exitErr{code: ExitCrashed, err: fmt.Errorf("fetch sha256sums: %w", err)}
	}
	expected, err := pickSumForPlatform(sums, opts.goos, opts.goarch)
	if err != nil {
		return &exitErr{code: ExitCrashed, err: err}
	}

	writef(stderr, "envdoctor: downloading envdoctor %s for %s/%s\n", target, opts.goos, opts.goarch)
	tarball, tarballName, err := opts.downloader(ctx, opts.repo, target, opts.goos, opts.goarch)
	if err != nil {
		return &exitErr{code: ExitCrashed, err: fmt.Errorf("download binary: %w", err)}
	}

	// --- verify SHA-256 ------------------------------------------
	sum := sha256.Sum256(tarball)
	actual := hex.EncodeToString(sum[:])
	if actual != expected {
		return &exitErr{code: ExitCrashed, err: fmt.Errorf(
			"SHA-256 mismatch on %s\n  expected: %s\n  actual:   %s",
			tarballName, expected, actual,
		)}
	}
	writef(stderr, "envdoctor: SHA-256 verified\n")

	// --- extract envdoctor binary from tarball -------------------
	binary, err := extractEnvdoctorBinary(tarball)
	if err != nil {
		return &exitErr{code: ExitCrashed, err: fmt.Errorf("extract from %s: %w", tarballName, err)}
	}

	// --- atomic in-place replace ---------------------------------
	if err := atomicWriteFile(opts.currentPath, binary, 0o755); err != nil {
		return &exitErr{code: ExitCrashed, err: fmt.Errorf("replace %s: %w", opts.currentPath, err)}
	}
	writef(stderr, "envdoctor: upgraded %s → %s\n", currentCanonical, target)
	return nil
}

// isBootstrapManaged reports whether path lives under the
// per-version cache directory that ./envdoctor bootstraps populate.
// Direct upgrade of those copies would put the bootstrap and the
// binary out of sync (the bootstrap still references the old SHA),
// so we refuse and tell the user to re-init the owning repo.
func isBootstrapManaged(path string) bool {
	// The cache layout is ~/.cache/envdoctor/versions/<tag>/<os_arch>/envdoctor.
	// We don't try to resolve "~" — a literal substring match is
	// sufficient and covers $HOME on every supported platform.
	return strings.Contains(filepath.ToSlash(path), "/.cache/envdoctor/versions/")
}

func pickSumForPlatform(sums platformSHAs, goos, goarch string) (string, error) {
	switch goos + "/" + goarch {
	case "darwin/amd64":
		return sums.DarwinAmd64, nil
	case "darwin/arm64":
		return sums.DarwinArm64, nil
	case "linux/amd64":
		return sums.LinuxAmd64, nil
	case "linux/arm64":
		return sums.LinuxArm64, nil
	}
	return "", fmt.Errorf("unsupported platform %s/%s", goos, goarch)
}

// extractEnvdoctorBinary scans the gzipped tar archive and returns
// the bytes of the "envdoctor" entry. Errors when the archive is
// malformed, the entry is missing, or the entry is larger than
// 50 MiB (sanity cap to prevent a malicious tarball from filling
// memory). Real envdoctor binaries are < 20 MiB.
func extractEnvdoctorBinary(archive []byte) ([]byte, error) {
	const (
		maxBytes = 50 * 1024 * 1024
		want     = "envdoctor"
	)
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, fmt.Errorf("gzip: %w", err)
	}
	defer func() { _ = gz.Close() }()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("entry %q not found in archive", want)
		}
		if err != nil {
			return nil, fmt.Errorf("tar: %w", err)
		}
		if hdr.Name != want {
			continue
		}
		if hdr.Size > maxBytes {
			return nil, fmt.Errorf("entry %q too large: %d bytes", want, hdr.Size)
		}
		buf := bytes.NewBuffer(make([]byte, 0, hdr.Size))
		if _, err := io.CopyN(buf, tr, hdr.Size); err != nil {
			return nil, fmt.Errorf("read %q: %w", want, err)
		}
		return buf.Bytes(), nil
	}
}

// fetchLatestTagFromGitHub resolves
//
//	https://github.com/<repo>/releases/latest
//
// to its tag by reading the 302 Location header. GitHub's release
// pages always redirect /latest → /tag/<tag>; following the
// redirect would require parsing the HTML, so we don't.
func fetchLatestTagFromGitHub(ctx context.Context, repo string) (string, error) {
	url := fmt.Sprintf("https://github.com/%s/releases/latest", repo)
	httpClient := &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	loc := resp.Header.Get("Location")
	if loc == "" {
		return "", fmt.Errorf("no Location header from %s (HTTP %d)", url, resp.StatusCode)
	}
	if idx := strings.Index(loc, "/tag/"); idx >= 0 {
		return loc[idx+len("/tag/"):], nil
	}
	return "", fmt.Errorf("unexpected Location format: %s", loc)
}

// downloadBinaryFromGitHub fetches the tarball for the given
// (tag, goos, goarch). Returns the tarball bytes and the
// canonical filename. 60s timeout — release tarballs are ~5-10 MiB,
// any host that takes longer than a minute is broken.
func downloadBinaryFromGitHub(ctx context.Context, repo, tag, goos, goarch string) ([]byte, string, error) {
	versionNoV := strings.TrimPrefix(tag, "v")
	name := fmt.Sprintf("envdoctor_%s_%s_%s.tar.gz", versionNoV, goos, goarch)
	url := fmt.Sprintf("https://github.com/%s/releases/download/%s/%s", repo, tag, name)
	httpClient := &http.Client{Timeout: 60 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, name, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, name, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, name, fmt.Errorf("GET %s: HTTP %d", url, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, name, err
	}
	return body, name, nil
}
