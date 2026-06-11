// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

// Package docslint holds tests that enforce contracts between
// envdoctor's runtime output (Finding.DocURL, JSON Schema $id,
// recipe IDs) and the docs site under docs/. The point: shipping
// a probe whose DocURL 404s is a worse experience than shipping
// a probe with no docs at all — at least the latter is honest.
//
// Each test in this package treats the docs site as part of the
// product contract, not an afterthought.
package docslint

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// docURLRe matches any string literal that looks like a probe
// DocURL. We scan source files rather than reflecting on running
// constants so the test catches DocURLs declared anywhere in the
// codebase — including future probes that don't expose theirs
// through BuiltinProbes.
var docURLRe = regexp.MustCompile(`"https://envdoctor\.dev/probes/([a-z0-9-]+)"`)

// TestProbeDocURLsResolveToContent walks every Go source file
// under internal/probes/ (excluding _test.go), extracts every
// probe DocURL it finds, and asserts each one resolves to a real
// page under docs/src/content/docs/probes/.
//
// This is the Phase 8 doc_url lint test — the spec from
// implementation.md says CI must fail when a probe ships without
// a docs page. Running through `go test ./...` rather than as a
// separate command means the gate is on every contributor's
// pre-commit + every CI run, not just the docs CI.
func TestProbeDocURLsResolveToContent(t *testing.T) {
	root := repoRoot(t)
	probesDir := filepath.Join(root, "internal", "probes")
	contentDir := filepath.Join(root, "docs", "src", "content", "docs", "probes")

	urls := scanProbeDocURLs(t, probesDir)
	if len(urls) == 0 {
		t.Fatalf("no DocURLs found under %s — has the URL format changed?", probesDir)
	}
	t.Logf("scanned %d unique probe DocURL(s)", len(urls))

	for id, sourceFile := range urls {
		if !pageExists(contentDir, id) {
			t.Errorf("probe %q (declared in internal/probes/%s) has DocURL\n"+
				"  https://envdoctor.dev/probes/%s\n"+
				"but no page exists. Expected one of:\n"+
				"  docs/src/content/docs/probes/%s.md\n"+
				"  docs/src/content/docs/probes/%s.mdx\n"+
				"  docs/src/content/docs/probes/%s/index.md\n"+
				"  docs/src/content/docs/probes/%s/index.mdx",
				id, sourceFile, id, id, id, id, id)
		}
	}
}

// TestProbePageMustHaveADeclaringProbe is the reverse check:
// every doc page under docs/src/content/docs/probes/ must
// correspond to a DocURL declared somewhere in internal/probes/.
//
// Without this we'd accumulate dead pages: a probe gets deleted
// or renamed, the page lingers, and a contributor reads stale
// information that no scan output ever links to.
func TestProbePageMustHaveADeclaringProbe(t *testing.T) {
	root := repoRoot(t)
	probesDir := filepath.Join(root, "internal", "probes")
	contentDir := filepath.Join(root, "docs", "src", "content", "docs", "probes")

	declared := scanProbeDocURLs(t, probesDir)
	pages, err := filepath.Glob(filepath.Join(contentDir, "*.md"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	mdxPages, _ := filepath.Glob(filepath.Join(contentDir, "*.mdx"))
	pages = append(pages, mdxPages...)
	if len(pages) == 0 {
		t.Fatalf("no probe pages found under %s", contentDir)
	}

	for _, p := range pages {
		id := strings.TrimSuffix(filepath.Base(p), filepath.Ext(p))
		if _, ok := declared[id]; !ok {
			rel, _ := filepath.Rel(root, p)
			t.Errorf("docs page %s exists but no probe in internal/probes/ declares DocURL\n"+
				"  https://envdoctor.dev/probes/%s\n"+
				"Delete the page, or add the DocURL to the corresponding probe.",
				rel, id)
		}
	}
}

// scanProbeDocURLs reads every non-test Go file under probesDir
// and returns a map of probe-id → source-file-basename for every
// DocURL constant it finds. The basename in the value makes
// failure messages easier to act on ("declared in nodeversion.go"
// rather than "declared somewhere").
func scanProbeDocURLs(t *testing.T, probesDir string) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(probesDir)
	if err != nil {
		t.Fatalf("read probes dir: %v", err)
	}
	out := map[string]string{}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(probesDir, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		for _, m := range docURLRe.FindAllStringSubmatch(string(raw), -1) {
			out[m[1]] = name
		}
	}
	return out
}

// pageExists reports whether any of the four conventional
// Starlight content paths exists for the given probe ID. The
// four candidates mirror what Astro/Starlight will route to
// /probes/<id>/ — both file and directory layouts, both .md
// and .mdx variants.
func pageExists(contentDir, id string) bool {
	for _, candidate := range []string{
		filepath.Join(contentDir, id+".md"),
		filepath.Join(contentDir, id+".mdx"),
		filepath.Join(contentDir, id, "index.md"),
		filepath.Join(contentDir, id, "index.mdx"),
	} {
		if _, err := os.Stat(candidate); err == nil {
			return true
		}
	}
	return false
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for range 10 {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatalf("could not find go.mod above %s", dir)
	return ""
}
