# Recipe schema

Recipes are the YAML files under [`internal/recipes/library/`](../../internal/recipes/library/).
Each file is a single `Recipe` — a logical group of repair commands for one
kind of finding (e.g. "node version mismatch"). The matcher selects at most
one `Fix` per finding based on the user's system facts.

This page is the human-readable reference. The authoritative Go types are
in [`internal/recipes/types.go`](../../internal/recipes/types.go); validation
runs in [`internal/recipes/embed.go`](../../internal/recipes/embed.go) at
binary build time, and CI rejects PRs whose recipes don't parse or are
missing required fields.

## Quick example

```yaml
id: node-version-mismatch        # unique recipe ID (kebab-case, forever)
probe: node-version              # the probe ID this recipe addresses

fixes:
  - id: mise-install-node        # unique within this recipe
    class: safe                  # safe | shared | destructive | privileged
    when:                        # selection clause; empty fields are wildcards
      has_tool: mise
    label: "Install Node {{.Required}} via mise"
    command: "mise install node@{{.Required}} && mise use node@{{.Required}}"
    test:                        # before/after fixture for the contract harness
      image: envdoctor/test-linux-mise:latest
      params: { Required: "20.10.0" }
      before:
        check: "! node --version 2>/dev/null | grep -q '^v20.10.0'"
      after:
        check: "node --version | grep -q '^v20.10.0'"
        idempotent: true

  - id: brew-install-node
    class: shared
    when: { has_tool: brew }
    command: "brew install node@{{.MajorVersion}}"
    fallback: true               # only used when no non-fallback Fix matches
    test: { ... }
```

## Top-level fields (`Recipe`)

| Field | Required | Type | Notes |
|---|---|---|---|
| `id` | yes | kebab-case string | Unique across the entire library. **Never renamed.** Renaming breaks every debug bundle and every Finding produced before the rename. |
| `probe` | yes | string | The ID of the probe this recipe applies to. One probe may have multiple recipes (e.g. "version mismatch" vs "not installed"); the probe chooses which recipe by setting `Finding.RecipeID`. |
| `fixes` | yes | list of `Fix` | At least one. |

## `Fix` fields

| Field | Required | Type | Notes |
|---|---|---|---|
| `id` | yes | kebab-case string | Unique within this recipe. Used in `Finding.RecipeID` and audit-log entries. |
| `class` | yes | enum | See [Safety classes](#safety-classes). |
| `when` | no | `Match` | Empty (`when: {}` or omitted) is a wildcard — the Fix always applies. |
| `command` | yes | `text/template` string | Rendered against probe-supplied params with `missingkey=error`. A typo (`{{.Verison}}`) fails loudly rather than producing a silently empty command. |
| `label` | no | string | Human-readable summary used in TTY output. May reference template variables. |
| `fallback` | no | bool | When `true`, this Fix is only chosen if no other Fix in the recipe matched. Encodes "prefer A, fall back to B" without an explicit priority field. |
| `test` | no | `Test` | The before/after fixture run by the recipe contract harness. Required for any Fix merged into the library (CI enforces). |

## `Match` (the `when:` clause)

A Fix's Match clause is satisfied when **every non-empty field** equals
the corresponding system fact. Empty fields are wildcards.

| Field | Source |
|---|---|
| `os` | `runtime.GOOS` — `darwin` or `linux` |
| `arch` | `runtime.GOARCH` — `amd64` or `arm64` |
| `distro` | Linux only: parsed from `/etc/os-release` `ID=` |
| `has_tool` | Checked via `Facts.HasTool` which uses `exec.LookPath`. Note: `nvm` is a shell function in most installations and **will not be detected** via this path. |

## Safety classes

Recipes declare a `class` that controls how the Phase 6 `--fix` command
handles the recipe. The four classes:

| Class | When to use | `--fix` behavior |
|---|---|---|
| `safe` | Localized, idempotent installs (`mise install`, `fnm install`, `asdf install`). Touches a tool-local directory only. | Auto-run with `--yes`. |
| `shared` | Touches global state usable by other projects: `brew install`, `apt install`, `npm i -g`. | Always prompts even with `--yes`. Opt in via `--yes --include=shared`. |
| `destructive` | Terminates processes, deletes data: `docker volume rm`, `kill $(lsof -ti …)`. | Always prompts per-recipe. No batch consent. |
| `privileged` | Needs `sudo`. | **Never auto-run.** envdoctor prints the command; the user types `sudo` themselves at their shell. |

If you're unsure, choose the more restrictive class. CI rejects recipes
that bake a literal `sudo` into a command of any class — `sudo` is the
user's responsibility, not envdoctor's.

## Template variables in `command` and `label`

The `command` and `label` strings are `text/template` expressions
rendered against a `map[string]any` provided by the probe. With
`Option("missingkey=error")`, a typo in the variable name is a hard
error at scan time — caught by the recipe contract test, not by a
contributor in production.

Conventional variable names:

| Variable | Meaning | Set by |
|---|---|---|
| `{{.Required}}` | An exact version string the user should install. | node-version, python-version |
| `{{.MajorVersion}}` | Just the major segment of a version. | node-version, python-version (for `brew install node@N`) |
| `{{.Port}}` | A TCP port number as a string. | port-free |
| `{{.Holder}}` | Best-effort description of the process holding a port. | port-free |
| `{{.Key}}` / `{{.Keys}}` | Env var name(s). | env-required (when shipped) |

Adding a new template variable is a contract: the probe must supply it
in `params`, and every Fix in the affected recipe must either use the
new variable or ignore it. Add the variable to the table above when
introducing it.

## `Test` block

Every Fix shipped to the library must include a `test:` block. The
recipe contract harness ([`scripts/recipe-test/`](../../scripts/recipe-test/))
runs it twice per Fix in a fresh container and asserts idempotence.
PRs without a `test:` block do not merge.

```yaml
test:
  image: envdoctor/test-linux-mise:latest   # container fixture
  params:                                   # passed to template rendering
    Required: "20.10.0"
  setup: |                                  # optional: stage broken state
    mise install node@18.20.0
    mise use node@18.20.0
  before:                                   # broken-state assertion
    check: "node --version | grep -q '^v18'"
  after:                                    # repaired-state assertion
    check: "node --version | grep -q '^v20.10.0'"
    idempotent: true                        # re-run; second run must be no-op
```

| Field | Required | Notes |
|---|---|---|
| `image` | yes | Container image with the tool stack the Fix expects. Maintained under [`testdata/containers/`](../../testdata/containers/). |
| `setup` | no | Bash snippet run before `before.check`. Stages the broken state (e.g. creates `.env.example`, binds a port, installs an old language runtime). Not part of the Fix the user runs. |
| `params` | yes if `command` uses any | Concrete values for template variables. |
| `before.check` | yes | A shell command that **must exit zero**, asserting the broken state the Fix repairs. If it exits non-zero the broken state isn't present and the test cannot prove the Fix did anything. |
| `after.check` | yes | A shell command that **must exit zero** after the Fix runs. |
| `after.idempotent` | recommended | When `true`, the harness re-runs the Fix command and re-asserts the after check. Second run must complete with exit 0 and produce no side-effect drift. |

## Validation rules enforced at build time

`recipes.LoadFS` (called by `recipes.DefaultLibrary`) rejects the
following at binary build time, so CI catches them before merge:

- missing `id` / `probe` / `fixes`
- empty `fixes` list
- a `Fix` missing `id`, `command`, or `class`
- a `Fix` with an unknown `class` value
- duplicate `Fix` IDs within a recipe
- duplicate Recipe IDs across files

## Adding a recipe — checklist

1. Create `internal/recipes/library/<probe-id>.yaml` (one recipe per
   probe is the convention; multiple are allowed).
2. Choose a stable Recipe ID. **It is forever.**
3. For each Fix:
   - Pick a stable Fix ID.
   - Choose the most restrictive `class` that fits.
   - Write the smallest `when:` clause that selects the right tool/OS.
   - Use template variables for anything that varies between findings.
   - Add a `test:` block matching one of the container fixtures.
4. Run locally: `go test ./internal/recipes/... -race`. The harness in
   `scripts/recipe-test/` runs the actual containers.
5. Open a PR. CI builds containers (cached by Dockerfile SHA) and
   executes each new Fix's before/after blocks twice.
