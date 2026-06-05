// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

package inference

import (
	"reflect"
	"testing"
)

func TestReadEnvKeys_Basic(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".env.example", `# A comment
DATABASE_URL=postgres://x
JWT_SECRET=
EMPTY=
# DISABLED=foo
DEBUG=1
`)
	keys, err := ReadEnvKeys(dir + "/.env.example")
	if err != nil {
		t.Fatalf("ReadEnvKeys: %v", err)
	}
	want := []string{"DATABASE_URL", "JWT_SECRET", "EMPTY", "DEBUG"}
	if !reflect.DeepEqual(keys, want) {
		t.Errorf("got %v, want %v", keys, want)
	}
}

func TestReadEnvKeys_HandlesExportPrefix(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".env.example", "export DATABASE_URL=x\nexport JWT=y\n")
	keys, err := ReadEnvKeys(dir + "/.env.example")
	if err != nil {
		t.Fatalf("ReadEnvKeys: %v", err)
	}
	want := []string{"DATABASE_URL", "JWT"}
	if !reflect.DeepEqual(keys, want) {
		t.Errorf("got %v, want %v", keys, want)
	}
}

func TestReadEnvKeys_SkipsInvalidNames(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".env.example", "1BAD=x\nGOOD_KEY=y\n=valueonly\n")
	keys, err := ReadEnvKeys(dir + "/.env.example")
	if err != nil {
		t.Fatalf("ReadEnvKeys: %v", err)
	}
	want := []string{"GOOD_KEY"}
	if !reflect.DeepEqual(keys, want) {
		t.Errorf("got %v, want %v", keys, want)
	}
}

func TestReadEnvKeys_MissingFileReturnsNil(t *testing.T) {
	keys, err := ReadEnvKeys(t.TempDir() + "/no-such-file")
	if err != nil {
		t.Fatalf("ReadEnvKeys: %v", err)
	}
	if keys != nil {
		t.Errorf("missing file should return nil; got %v", keys)
	}
}

func TestExtractComposeVarRefs(t *testing.T) {
	const compose = `
services:
  db:
    image: postgres
    environment:
      POSTGRES_USER: ${POSTGRES_USER}
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}
      POSTGRES_DB: ${POSTGRES_DB:-app}
      OPTIONAL: ${OPT_OUT:?some error}
      EQ_DEFAULT: ${EQ:=foo}
      PLUS_VALUE: ${PLUS:+bar}
  web:
    image: ${WEB_IMAGE}
    environment:
      - JWT_SECRET=${JWT_SECRET}
      - DEBUG=${DEBUG:-0}
      - DATABASE_URL=${DATABASE_URL}
`
	got := extractComposeVarRefs(compose)
	want := []string{"POSTGRES_USER", "POSTGRES_PASSWORD", "WEB_IMAGE", "JWT_SECRET", "DATABASE_URL"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestInferEnv_EmptyRepoReturnsNothing(t *testing.T) {
	reqs, err := InferEnv(t.TempDir())
	if err != nil {
		t.Fatalf("InferEnv: %v", err)
	}
	if len(reqs) != 0 {
		t.Errorf("expected 0; got %+v", reqs)
	}
}

func TestInferEnv_EnvExampleOnly(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".env.example", "DATABASE_URL=x\nJWT_SECRET=y\n")
	reqs, err := InferEnv(dir)
	if err != nil {
		t.Fatalf("InferEnv: %v", err)
	}
	want := []EnvRequirement{
		{Source: ".env.example", Key: "DATABASE_URL"},
		{Source: ".env.example", Key: "JWT_SECRET"},
	}
	if !reflect.DeepEqual(reqs, want) {
		t.Errorf("got %+v, want %+v", reqs, want)
	}
}

func TestInferEnv_ComposeOnly(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "docker-compose.yml", `services:
  db:
    environment:
      POSTGRES_URL: ${POSTGRES_URL}
`)
	reqs, err := InferEnv(dir)
	if err != nil {
		t.Fatalf("InferEnv: %v", err)
	}
	if len(reqs) != 1 || reqs[0].Source != "docker-compose.yml" || reqs[0].Key != "POSTGRES_URL" {
		t.Errorf("got %+v; want one compose req for POSTGRES_URL", reqs)
	}
}

func TestInferEnv_DeduplicatesAcrossSources(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".env.example", "DATABASE_URL=x\n")
	writeFile(t, dir, "docker-compose.yml", `services:
  app:
    environment:
      DATABASE_URL: ${DATABASE_URL}
      NEW_ONLY: ${NEW_ONLY}
`)
	reqs, err := InferEnv(dir)
	if err != nil {
		t.Fatalf("InferEnv: %v", err)
	}
	// .env.example wins for DATABASE_URL because it's read first.
	want := []EnvRequirement{
		{Source: ".env.example", Key: "DATABASE_URL"},
		{Source: "docker-compose.yml", Key: "NEW_ONLY"},
	}
	if !reflect.DeepEqual(reqs, want) {
		t.Errorf("got %+v, want %+v", reqs, want)
	}
}
