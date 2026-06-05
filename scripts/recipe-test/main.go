// Copyright 2026 The EnvDoctor Authors
// SPDX-License-Identifier: Apache-2.0

// Command recipe-test runs the contract test for every Fix in the
// embedded recipe library against the container fixtures under
// testdata/containers/.
//
// For each Fix with a `test:` block whose `image:` is in the shipped
// fixture set:
//
//  1. Ensure the image is built (cached by Docker locally).
//  2. Render the Fix's `command` template against `test.params`.
//  3. In a single fresh container, execute:
//     - optional `test.setup` to stage the broken state.
//     - `before.check` — must exit zero (broken state confirmed).
//     - `command`      — must exit zero (the Fix succeeds).
//     - `after.check`  — must exit zero (repaired state confirmed).
//     - If `after.idempotent: true`, re-run command and after.check.
//
// Fixes whose image is not in the fixture set are skipped with a
// clear message (e.g. nvm shell-function, darwin-brew).
//
// Run from the repo root:
//
//	go run ./scripts/recipe-test
//
// Exits 0 if every applicable Fix passes; non-zero otherwise.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/reswaraa/envdoctor/internal/recipes"
)

const (
	libraryPath   = "internal/recipes/library"
	containersDir = "testdata/containers"
)

// fixtures maps a recipe's `test.image:` value to the Dockerfile that
// builds it. Images not in this map are skipped — the recipe Fix is
// kept in the library for documentation but the contract test is not
// run.
var fixtures = map[string]string{
	"envdoctor/test-linux-fresh:latest": "linux-fresh.Dockerfile",
	"envdoctor/test-linux-mise:latest":  "linux-mise.Dockerfile",
	"envdoctor/test-linux-fnm:latest":   "linux-fnm.Dockerfile",
	"envdoctor/test-linux-asdf:latest":  "linux-asdf.Dockerfile",
}

func main() {
	verbose := flag.Bool("v", false, "print container output for failing tests")
	flag.Parse()

	code := run(*verbose)
	os.Exit(code)
}

func run(verbose bool) int {
	lib, err := recipes.LoadFS(os.DirFS(libraryPath), ".")
	if err != nil {
		fmt.Fprintln(os.Stderr, "load library:", err)
		return 1
	}

	var passed, failed, skipped int
	built := map[string]bool{}

	for _, r := range lib.Recipes {
		for _, fix := range r.Fixes {
			res := runFix(r, fix, built, verbose)
			label := fmt.Sprintf("%s/%s", r.ID, fix.ID)
			switch res.kind {
			case kindPassed:
				passed++
				fmt.Printf("[PASS] %s\n", label)
			case kindSkipped:
				skipped++
				fmt.Printf("[SKIP] %s — %s\n", label, res.reason)
			case kindFailed:
				failed++
				fmt.Printf("[FAIL] %s — %s\n", label, res.reason)
				if verbose && res.output != "" {
					fmt.Println("---")
					fmt.Println(res.output)
					fmt.Println("---")
				}
			}
		}
	}

	fmt.Printf("\n%d passed, %d failed, %d skipped\n", passed, failed, skipped)
	if failed > 0 {
		return 1
	}
	return 0
}

const (
	kindPassed  = "passed"
	kindFailed  = "failed"
	kindSkipped = "skipped"
)

type result struct {
	kind   string
	reason string
	output string
}

func runFix(r recipes.Recipe, fix recipes.Fix, built map[string]bool, _ bool) result {
	if fix.Test.Image == "" {
		return result{kind: kindSkipped, reason: "no test block"}
	}
	dockerfile, ok := fixtures[fix.Test.Image]
	if !ok {
		return result{kind: kindSkipped, reason: "image not in fixture set: " + fix.Test.Image}
	}
	if !built[fix.Test.Image] {
		if err := ensureImage(fix.Test.Image, filepath.Join(containersDir, dockerfile)); err != nil {
			return result{kind: kindFailed, reason: "build image: " + err.Error()}
		}
		built[fix.Test.Image] = true
	}

	cmd, err := renderTemplate(fix.Command, mapAny(fix.Test.Params))
	if err != nil {
		return result{kind: kindFailed, reason: "render command: " + err.Error()}
	}

	script := buildScript(fix.Test.Setup, fix.Test.Before.Check, cmd, fix.Test.After.Check, fix.Test.After.Idempotent)
	output, exitErr := dockerRun(fix.Test.Image, script)
	if exitErr != nil {
		reason := "container exit: " + exitErr.Error()
		if exitCode(exitErr) == 2 {
			reason = "precondition NOT present (before.check exited non-zero); setup or check is wrong"
		}
		return result{kind: kindFailed, reason: reason, output: output}
	}
	_ = r
	return result{kind: kindPassed}
}

// buildScript composes the bash script run inside the test container.
// All phases share one container so state (installed binaries,
// dropped files) persists across them.
//
// Semantics:
//
//	setup        — optional. Stages the broken state.
//	before.check — asserts the broken state. Must exit ZERO.
//	               If it exits non-zero, the harness exits 2: the
//	               precondition isn't present so the fix cannot be
//	               meaningfully tested.
//	command      — the rendered Fix. Must exit zero.
//	after.check  — asserts the repaired state. Must exit zero.
//	idempotent   — re-run command + after.check; both must still hold.
func buildScript(setup, beforeCheck, cmd, afterCheck string, idempotent bool) string {
	var b strings.Builder
	b.WriteString("#!/usr/bin/env bash\n")
	b.WriteString("set -e\n")
	if setup != "" {
		b.WriteString("# setup: stage broken state\n")
		b.WriteString(setup + "\n")
	}
	if beforeCheck != "" {
		b.WriteString("# precondition: before.check must exit zero (broken state confirmed)\n")
		b.WriteString("if ! eval " + shellQuote(beforeCheck) + "; then exit 2; fi\n")
	}
	b.WriteString("# 1st run\n")
	b.WriteString(cmd + "\n")
	if afterCheck != "" {
		b.WriteString(afterCheck + "\n")
	}
	if idempotent {
		b.WriteString("# 2nd run (idempotence)\n")
		b.WriteString(cmd + "\n")
		if afterCheck != "" {
			b.WriteString(afterCheck + "\n")
		}
	}
	return b.String()
}

func ensureImage(tag, dockerfile string) error {
	if err := exec.Command("docker", "image", "inspect", tag).Run(); err == nil {
		return nil
	}
	cmd := exec.Command("docker", "build", "-f", dockerfile, "-t", tag, ".")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func dockerRun(image, script string) (string, error) {
	cmd := exec.Command("docker", "run", "--rm", "-i", image, "bash", "-c", script)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func renderTemplate(tmpl string, params map[string]any) (string, error) {
	t, err := template.New("cmd").Option("missingkey=error").Parse(tmpl)
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	if err := t.Execute(&sb, params); err != nil {
		return "", err
	}
	return sb.String(), nil
}

func mapAny(in map[string]string) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func exitCode(err error) int {
	var ee *exec.ExitError
	if err == nil {
		return 0
	}
	if asExit(err, &ee) {
		return ee.ExitCode()
	}
	return -1
}

func asExit(err error, target **exec.ExitError) bool {
	for err != nil {
		if e, ok := err.(*exec.ExitError); ok {
			*target = e
			return true
		}
		type unwrapper interface{ Unwrap() error }
		u, ok := err.(unwrapper)
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}
