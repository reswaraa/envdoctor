<!--
Thanks for adding a recipe. Recipes are how envdoctor grows: every
real-world repair anyone has had to do manually can become a one-line
fix for the next contributor who hits the same wall.

Please fill out this template completely. The checklist mirrors the
contract every recipe in the library has to satisfy.

For the full schema reference, see:
  https://reswaraa.github.io/envdoctor/recipes/schema/
-->

## What this recipe fixes

<!-- One sentence: what broken state does this fix repair? Which probe
     emits the finding this recipe attaches to? -->

Probe ID: `…`

## Recipe ID

<!-- Recipe IDs are forever. Renaming any of them is an incompatible
     library change (it breaks every debug bundle and audit log entry
     produced before the rename). Pick a stable kebab-case name. -->

Recipe ID: `…`

## Safety class

Class chosen: `safe` / `shared` / `destructive` / `privileged`

Justification (one sentence — why this class and not the next-more-restrictive one):

<!-- Reminder of the four classes:

  safe         localized, idempotent (mise install, fnm install,
               cp -n .env.example .env). Touches a tool-local
               directory only. Auto-runs with --yes.

  shared       touches global state usable by other projects (brew,
               apt, npm i -g). Always prompts even with --yes; opt
               in via --yes --include=shared.

  destructive  terminates processes, deletes data, kills ports.
               Always prompts per-recipe. No batch consent.

  privileged   needs sudo. envdoctor NEVER auto-runs these; the
               command is printed for the user to type at their
               shell. If unsure, this is probably the right class.

  When in doubt, pick the more restrictive class. CI rejects recipes
  that bake a literal `sudo` into a command of any non-privileged
  class.
-->

## When clause

The Match clause is as small as it can be (every non-empty field is
required to be true; empty fields are wildcards):

```yaml
when:
  # …
```

## Test block

Every Fix must have a `test:` block. The contract harness runs it
twice in a fresh container per Fix.

- [ ] `test.image` points at a fixture under `testdata/containers/` (or one I added in this PR)
- [ ] `test.params` provides concrete values for every template variable
- [ ] `test.setup` (if needed) stages the broken state (e.g. installs an old runtime)
- [ ] `test.before.check` is a shell command that **exits zero when the broken state is present** (and non-zero otherwise)
- [ ] `test.after.check` is a shell command that **exits zero when the repaired state is present**
- [ ] `test.after.idempotent: true` is set; the harness re-runs the Fix command and asserts the second run is a no-op (no output drift, exit 0)

## Local verification

- [ ] `go test ./internal/recipes/... -race`
- [ ] `go run ./scripts/recipes-to-mdx` (regenerates the per-probe doc table)
- [ ] `golangci-lint run ./...`
- [ ] Smoke-tested on a repo where the broken state existed: scan emits the finding, fix executes the recipe, re-scan no longer emits

## Anti-features check

- [ ] Command does NOT start with `sudo` (unless class=privileged)
- [ ] Command does NOT execute arbitrary shell from `.envdoctor.yaml`
- [ ] Command does NOT phone home, send telemetry, or download anything beyond what the tool itself does
- [ ] Recipe ID is forever-stable (kebab-case, descriptive)

## Notes for review

<!-- Anything reviewers should know that isn't obvious from the diff. -->
