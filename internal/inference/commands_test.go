// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

package inference

import (
	"reflect"
	"testing"
)

func TestFirstWord(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"make", "make"},
		{"make -C subdir", "make"},
		{"  echo hi  ", "echo"},
		{"\t cmd arg", "cmd"},
		{"", ""},
		{"   ", ""},
	}
	for _, c := range cases {
		if got := firstWord(c.in); got != c.want {
			t.Errorf("firstWord(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestValidBinaryName(t *testing.T) {
	good := []string{"make", "psql", "git-lfs", "node", "redis-cli", "yq", "a"}
	bad := []string{"", "$(GO)", "./manage.py", "1invalid", "-x", "/usr/bin/cmd"}
	for _, s := range good {
		if !validBinaryName(s) {
			t.Errorf("%q should be a valid binary name", s)
		}
	}
	for _, s := range bad {
		if validBinaryName(s) {
			t.Errorf("%q should NOT be a valid binary name", s)
		}
	}
}

func TestInferCommands_EmptyRepo(t *testing.T) {
	reqs, err := InferCommands(t.TempDir())
	if err != nil {
		t.Fatalf("InferCommands: %v", err)
	}
	if len(reqs) != 0 {
		t.Errorf("expected 0; got %+v", reqs)
	}
}

func TestInferCommands_Makefile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "Makefile", `
all: build test
\t# a comment line
\tmake -C subdir
\t@echo hello
\t-rm -f build/*
\tpsql -d app -f schema.sql
\t$(GO) build .
\tprotoc --proto_path=proto api.proto
`)
	// The above writeFile inserts literal "\t" characters via the
	// backslash-t escape we wrote — those need to be real tabs to mimic
	// a Makefile. Rewrite with actual tabs.
	writeFile(t, dir, "Makefile", "all: build test\n"+
		"\t# comment\n"+
		"\tmake -C subdir\n"+
		"\t@echo hello\n"+
		"\t-rm -f build/*\n"+
		"\tpsql -d app -f schema.sql\n"+
		"\t$(GO) build .\n"+
		"\tprotoc --proto_path=proto api.proto\n")

	reqs, err := InferCommands(dir)
	if err != nil {
		t.Fatalf("InferCommands: %v", err)
	}
	got := map[string]bool{}
	for _, r := range reqs {
		got[r.Command] = true
	}
	wantPresent := []string{"make", "psql", "protoc"}
	for _, w := range wantPresent {
		if !got[w] {
			t.Errorf("expected %q in inferred commands; got %v", w, got)
		}
	}
	wantSkipped := []string{"echo", "rm", "go"}
	for _, w := range wantSkipped {
		if got[w] {
			t.Errorf("%q is a builtin/runtime and must be skipped; got %v", w, got)
		}
	}
}

func TestInferCommands_Procfile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "Procfile", `# a comment
web: bundle exec puma -p $PORT
worker: redis-cli FLUSHDB
release: ./release.sh
notvalid no colon
`)
	reqs, err := InferCommands(dir)
	if err != nil {
		t.Fatalf("InferCommands: %v", err)
	}
	got := map[string]string{}
	for _, r := range reqs {
		got[r.Command] = r.Source
	}
	if got["redis-cli"] != "Procfile" {
		t.Errorf("redis-cli must come from Procfile; got %v", got)
	}
	if _, found := got["bundle"]; found {
		t.Error("bundle is in skipBuiltins; must not surface")
	}
}

func TestInferCommands_ComposeCommandAndEntrypoint(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "docker-compose.yml", `services:
  app:
    image: alpine
    command: jq .
  worker:
    image: alpine
    entrypoint: ["yq", "-r", "."]
`)
	reqs, err := InferCommands(dir)
	if err != nil {
		t.Fatalf("InferCommands: %v", err)
	}
	got := map[string]string{}
	for _, r := range reqs {
		got[r.Command] = r.Source
	}
	if got["jq"] == "" {
		t.Errorf("jq missing; got %v", got)
	}
	if got["yq"] == "" {
		t.Errorf("yq missing; got %v", got)
	}
}

func TestInferCommands_DedupesAcrossSources(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "Makefile", "all:\n\tpsql -d app\n")
	writeFile(t, dir, "Procfile", "web: psql -d app\n")
	reqs, err := InferCommands(dir)
	if err != nil {
		t.Fatalf("InferCommands: %v", err)
	}
	count := 0
	for _, r := range reqs {
		if r.Command == "psql" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("psql should appear once across sources; got %d", count)
	}
	// First source (Makefile) wins.
	for _, r := range reqs {
		if r.Command == "psql" && r.Source != "Makefile" {
			t.Errorf("expected Source=Makefile; got %q", r.Source)
		}
	}
}

func TestInferCommands_OrderingIsStable(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "Makefile", "all:\n\tpsql\n\tprotoc\n")
	writeFile(t, dir, "Procfile", "web: redis-cli\n")
	reqs, err := InferCommands(dir)
	if err != nil {
		t.Fatalf("InferCommands: %v", err)
	}
	want := []string{"psql", "protoc", "redis-cli"}
	got := make([]string, len(reqs))
	for i, r := range reqs {
		got[i] = r.Command
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("order: got %v, want %v", got, want)
	}
}
