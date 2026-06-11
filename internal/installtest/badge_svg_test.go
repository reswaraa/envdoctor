// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

package installtest

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestBadgeSVG_IsValidXML is the cheap check that catches a
// hand-edited badge.svg with a typo (unclosed tag, mismatched
// attribute quote). A malformed badge would 200 OK from GitHub
// Pages but render as a broken-image icon in every consumer's
// README — silent failure mode.
func TestBadgeSVG_IsValidXML(t *testing.T) {
	path := findRepoFile(t, filepath.Join("docs", "public", "badge.svg"))
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var v any
	if err := xml.Unmarshal(raw, &v); err != nil {
		t.Fatalf("badge.svg is not valid XML: %v", err)
	}
}

// TestBadgeSVG_HasExpectedDimensions pins the shields.io-style
// canvas size. Changing it is a presentation choice the
// maintainer can make, but the test acts as a tripwire so a
// "tiny CSS tweak" doesn't accidentally produce a 2000px badge.
func TestBadgeSVG_HasExpectedDimensions(t *testing.T) {
	path := findRepoFile(t, filepath.Join("docs", "public", "badge.svg"))
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`width="148"`,
		`height="20"`,
	} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("badge.svg missing %q (shields.io-style 148×20)", want)
		}
	}
}

// TestBadgeSVG_ContainsExpectedText pins the label / status text
// so a future refactor can't silently rename it. The
// --readme-badge flag (internal/cli/init.go) prints a snippet
// pointing at this exact badge URL; the label has to stay in
// sync with what the snippet promises.
func TestBadgeSVG_ContainsExpectedText(t *testing.T) {
	path := findRepoFile(t, filepath.Join("docs", "public", "badge.svg"))
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	for _, want := range []string{
		"envdoctor",
		"scan ready",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("badge.svg missing text %q", want)
		}
	}
}

// TestReadmeBadgeURLContract is the cross-package contract test:
// internal/cli/init.go's --readme-badge flag prints a snippet
// referencing https://reswaraa.github.io/envdoctor/badge.svg. The docs site
// must serve the badge AT that path (i.e. docs/public/badge.svg
// gets copied verbatim to dist/badge.svg). This test asserts the
// file exists at the path Astro will copy from.
//
// If the badge ever moves (e.g. to /static/badge.svg), the
// init.go snippet and this test must be updated together —
// failing here gives the contributor a one-line pointer to the
// right place.
func TestReadmeBadgeURLContract(t *testing.T) {
	path := findRepoFile(t, filepath.Join("docs", "public", "badge.svg"))
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the --readme-badge flag prints https://reswaraa.github.io/envdoctor/badge.svg; "+
			"docs/public/badge.svg must exist to serve it. got: %v", err)
	}
}
