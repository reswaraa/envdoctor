# Contributing to EnvDoctor

Thanks for thinking about contributing. EnvDoctor is a small, opinionated tool
and we keep it that way on purpose. Please read the anti-features list before
opening a PR. Most "wouldn't it be great if envdoctor also did X" ideas have
already been considered and deliberately ruled out.

## Anti-features (red lines)

These are not "we haven't done them yet." They are decisions. If you want one
of them, open an issue first and explain why the trade-off has changed.

- **No shell probes from repo-controlled YAML.** `.envdoctor.yaml` is purely
  declarative. There is no `shell:` directive. A contributor running
  envdoctor against a stranger's repo must not get arbitrary code executed
  on their machine.
- **No telemetry, no accounts, no server, no `envdoctor login`.** The tool
  is pure OSS and works fully offline. The only network calls are the
  installer download, the bootstrap binary fetch, and `envdoctor upgrade`.
- **No `sudo` auto-wrapping.** Recipes that require elevation print the
  `sudo` command for the user to run themselves. The shell sudo prompt is
  the trust gate.
- **No rollback promises and no half-baked `undo`.** Most fixes
  (`mise install node@20`) are not reversible. The audit log records what
  was run; we do not pretend to reverse it.
- **No remote recipe fetch in v1.** Recipes are embedded in the binary and
  ship with each release. The repo-pinned bootstrap gives reproducibility.
- **No plugin API.** Extension is by PR to the recipe library, not by
  loading third-party Go plugins.
- **No daemon / background mode.** EnvDoctor is a one-shot tool.
- **No native Windows in MVP.** WSL2 is supported; native Windows is a
  different OS surface and is not in scope for v1.
- **No probe ID renames, ever.** New semantics get a new probe ID and a
  deprecation page on the old one. `Finding.doc_url` is a forever URL.
- **No conditionals or templating in `.envdoctor.yaml`.** Once you have an
  expression language, you have a programming language, and the schema is
  no longer declarative.
- **No network calls during `scan`.** A scan on an airplane gives the same
  output as a scan online. No "check for updates" pings.
- **No README mutation by `envdoctor init`.** We print snippets for the
  user to paste. We never write to files we did not create.

## How to add a probe

Every probe is three artifacts in one PR:

1. **Code** under `internal/probes/<id>.go` implementing the `Probe`
   interface, plus inference (under `internal/inference/`) if the probe
   reads new manifest files.
2. **Tests** under `internal/probes/<id>_test.go`. At least one happy-path
   integration test and two failure-mode tests. **No mocks of OS state** —
   use a container fixture from `testdata/containers/`. Mocked probes pass
   while reality breaks.
3. **Docs** under `docs/probes/<id>.md` following the per-probe template.
   The CI docs build fails if any emitted `Finding.doc_url` 404s.

Probe IDs are kebab-case and forever. If semantics change, deprecate and
add a new ID.

## How to add a recipe

Every recipe is one YAML file under `internal/recipes/library/<id>.yaml`
with a `test:` block that the recipe-contract harness runs **twice** in a
fresh container (idempotence check). PRs without a `test:` block do not
merge.

Recipes declare a safety `class`:

- `safe` : localized, idempotent installs (e.g. `mise install`). Auto-run
  with `--yes`.
- `shared` : touches global state (e.g. `brew install`). Always prompts
  even with `--yes`; opt-in via `--include=shared`.
- `destructive` : terminates processes or deletes data
  (e.g. `docker volume rm`). Always prompts per-recipe, no batch consent.
- `privileged` : needs `sudo`. **Never auto-runs.** The command is printed;
  the user types it themselves.

Recipes must be idempotent. The CI harness runs each recipe twice; the
second run must be a no-op.

## How to add a check type to `.envdoctor.yaml`

Schema changes that are _additive_ (new optional fields, new check `type:`)
ship in a minor version without bumping `schema_version`. Schema changes
that break parsing of existing configs bump `schema_version` (e.g. `1` to
`2`), supported in parallel for one major version. Both cases require
updating the JSON Schema in `internal/config/json_schema.go` and the doc
page under `docs/schema/`.

## Code style

- Apache-2.0 license header at the top of every Go source file.
- `gofmt` + `golangci-lint` clean. CI enforces.
- No new top-level Go packages outside `internal/` (we make no API
  promises in v1).
- Output discipline: `--json` writes JSON-only to stdout, ever. Logs go to
  stderr. Honor `NO_COLOR` and `CI=true`.
- Probes run in parallel, are panic-recovered, and propagate `context.Context`.

## Performance budget

`envdoctor scan` must finish in **< 2 s** wall-clock on a 2020-era laptop
and **< 5 s** in CI. Any probe slower than 500 ms median is a bug. PRs that
regress these numbers will be asked for justification.

## Versioning

envdoctor versions four independent axes: the binary, the recipe
library, the `.envdoctor.yaml` schema, and probe IDs. Each follows
different stability guarantees — probe IDs and schema field names are
immutable; the binary follows semver; recipes are updated independently.

## Reporting bugs

Open an issue with the output of `envdoctor scan --bundle ./bundle.json`
attached. Bundles are JSON, redacted by the probe schema (env values, file
contents, hostname are never included). Review before posting.

## Code of conduct

Be kind. Default to charitable interpretations. We are all trying to make
local dev environments suck less.
