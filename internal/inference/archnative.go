// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

package inference

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// NativeDep is one Node package the arch-mismatch probe cares about,
// along with its pinned version from a lockfile.
type NativeDep struct {
	Source  string
	Name    string
	Version string
}

// nativeDepNames is the curated set of Node packages the arch probe
// scans for. Each has a documented "first arm64 prebuilt" version that
// the probe compares against. Adding to this set requires updating
// knownX86Issues in internal/probes/archmismatch.go.
var nativeDepNames = []string{"sharp", "canvas", "cypress"}

// HasNodeLockfile reports whether the repo has a lockfile the arch
// probe knows how to parse. Used by AppliesTo for an O(1) check before
// expensive lockfile reads.
func HasNodeLockfile(root string) bool {
	for _, name := range []string{"package-lock.json"} {
		if _, err := os.Stat(filepath.Join(root, name)); err == nil {
			return true
		}
	}
	return false
}

// InferNativeArchDeps scans known lockfiles for pinned versions of
// the native packages in nativeDepNames. Returns deduplicated entries;
// first lockfile (package-lock.json) wins for any given package name.
func InferNativeArchDeps(root string) ([]NativeDep, error) {
	out, err := readPackageLockNative(root)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// readPackageLockNative handles both npm lockfile schemas:
//
//	v1: top-level "dependencies": { "<name>": { "version": "X" } }
//	v2/v3: top-level "packages": { "node_modules/<name>": { "version": "X" } }
//
// Both shapes coexist in v2 lockfiles (npm writes both for back-compat);
// we read whichever has data and de-dup by name.
func readPackageLockNative(root string) ([]NativeDep, error) {
	b, err := os.ReadFile(filepath.Join(root, "package-lock.json"))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read package-lock.json: %w", err)
	}

	var doc struct {
		LockfileVersion int `json:"lockfileVersion"`
		Packages        map[string]struct {
			Version string `json:"version"`
		} `json:"packages"`
		Dependencies map[string]struct {
			Version string `json:"version"`
		} `json:"dependencies"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, fmt.Errorf("parse package-lock.json: %w", err)
	}

	seen := map[string]bool{}
	var out []NativeDep

	for _, name := range nativeDepNames {
		// v2/v3 form
		if info, ok := doc.Packages["node_modules/"+name]; ok && info.Version != "" {
			out = append(out, NativeDep{
				Source: "package-lock.json", Name: name, Version: info.Version,
			})
			seen[name] = true
			continue
		}
		// v1 form
		if info, ok := doc.Dependencies[name]; ok && info.Version != "" {
			out = append(out, NativeDep{
				Source: "package-lock.json", Name: name, Version: info.Version,
			})
			seen[name] = true
		}
	}
	return out, nil
}
