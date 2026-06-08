// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

package inference

import (
	"reflect"
	"testing"
)

func TestHasNodeLockfile(t *testing.T) {
	if HasNodeLockfile(t.TempDir()) {
		t.Error("empty repo must not signal a Node lockfile")
	}
	dir := t.TempDir()
	writeFile(t, dir, "package-lock.json", "{}")
	if !HasNodeLockfile(dir) {
		t.Error("package-lock.json must signal a lockfile")
	}
}

func TestInferNativeArchDeps_PackageLockV3(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package-lock.json", `{
  "lockfileVersion": 3,
  "packages": {
    "": { "name": "app", "version": "1.0.0" },
    "node_modules/sharp": { "version": "0.31.0" },
    "node_modules/cypress": { "version": "12.17.0" },
    "node_modules/unrelated": { "version": "1.0.0" }
  }
}`)
	deps, err := InferNativeArchDeps(dir)
	if err != nil {
		t.Fatalf("InferNativeArchDeps: %v", err)
	}
	want := []NativeDep{
		{Source: "package-lock.json", Name: "sharp", Version: "0.31.0"},
		{Source: "package-lock.json", Name: "cypress", Version: "12.17.0"},
	}
	if !reflect.DeepEqual(deps, want) {
		t.Errorf("got %+v, want %+v", deps, want)
	}
}

func TestInferNativeArchDeps_PackageLockV1(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package-lock.json", `{
  "lockfileVersion": 1,
  "dependencies": {
    "sharp": { "version": "0.32.0" },
    "canvas": { "version": "2.10.0" }
  }
}`)
	deps, err := InferNativeArchDeps(dir)
	if err != nil {
		t.Fatalf("InferNativeArchDeps: %v", err)
	}
	want := []NativeDep{
		{Source: "package-lock.json", Name: "sharp", Version: "0.32.0"},
		{Source: "package-lock.json", Name: "canvas", Version: "2.10.0"},
	}
	if !reflect.DeepEqual(deps, want) {
		t.Errorf("got %+v, want %+v", deps, want)
	}
}

func TestInferNativeArchDeps_EmptyRepoIsNotAnError(t *testing.T) {
	deps, err := InferNativeArchDeps(t.TempDir())
	if err != nil {
		t.Fatalf("expected no error; got %v", err)
	}
	if len(deps) != 0 {
		t.Errorf("expected 0; got %+v", deps)
	}
}

func TestInferNativeArchDeps_MalformedLockfileIsError(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package-lock.json", "{ not json")
	_, err := InferNativeArchDeps(dir)
	if err == nil {
		t.Error("expected parse error")
	}
}

func TestInferNativeArchDeps_PrefersV3OverV1WhenBothPresent(t *testing.T) {
	// npm v2 lockfiles ship both shapes; we should read packages[] first.
	dir := t.TempDir()
	writeFile(t, dir, "package-lock.json", `{
  "lockfileVersion": 2,
  "packages": {
    "node_modules/sharp": { "version": "0.30.0" }
  },
  "dependencies": {
    "sharp": { "version": "9.99.99" }
  }
}`)
	deps, err := InferNativeArchDeps(dir)
	if err != nil {
		t.Fatalf("InferNativeArchDeps: %v", err)
	}
	if len(deps) != 1 || deps[0].Version != "0.30.0" {
		t.Errorf("expected v3 packages[] to win with 0.30.0; got %+v", deps)
	}
}
