---
title: .envdoctor.yaml
description: The repo-level config file that extends envdoctor's inferred check set with declarative checks, overrides, and disables.
---

`.envdoctor.yaml` is **optional**. EnvDoctor infers most checks from the
manifest files your repo already has (`.nvmrc`, `package.json`,
`docker-compose.yml`, `pyproject.toml`, `go.mod`, …). The config file lets a
maintainer *add* checks, override severity, or disable specific probes on top
of inference. It does not replace inference.

The file lives at the repo root. To scaffold one, run `envdoctor init` (or
`envdoctor init --with-config` for a fully commented reference).

The authoritative reference is the JSON Schema at
<https://reswaraa.github.io/envdoctor/schemas/v1/config.json> — point your
editor at it for autocomplete and validation:

```yaml
# yaml-language-server: $schema=https://reswaraa.github.io/envdoctor/schemas/v1/config.json
schema_version: 1
```

## Minimal example

```yaml
schema_version: 1
envdoctor:
  min_version: "0.1.0"
```

That's a valid file. `min_version` is a semver constraint asserted at scan
time; older envdoctor binaries refuse to scan the repo with a clear error
rather than silently honoring the wrong contract.

## Anatomy

| Field | Required | Notes |
|---|---|---|
| `schema_version` | yes | Currently `1`. Bumped only at incompatible config-shape changes; the major one before lives on for one release window. |
| `envdoctor.min_version` | no | Semver constraint. Empty / `dev` envdoctor versions skip the check. |
| `checks` | no | List of additive declarative checks (see below). |
| `overrides` | no | Per-probe-ID overrides (today: severity). |
| `disable` | no | List of probe IDs to skip entirely. Use sparingly — disabling a probe hides the finding from every contributor, regardless of host state. |

## `checks` — the four types

Each entry is a [typed-list discriminator](https://json-schema.org/understanding-json-schema/reference/conditionals): the `type:` field selects which check kind is being declared and which fields are valid.

### `tool_version`

Require a named tool on PATH at a version satisfying a semver constraint.

```yaml
checks:
  - type: tool_version
    tool: terraform
    constraint: ">=1.6,<2.0"
```

| Field | Required | Notes |
|---|---|---|
| `tool` | yes | The command envdoctor will look up via `exec.LookPath` and then exec with `--version`. |
| `constraint` | yes | Any [Masterminds/semver/v3](https://github.com/Masterminds/semver) constraint (ranges, caret, tilde). |
| `min_version` | no | Shorthand for `constraint: ">=X.Y.Z"` if you don't need a range. |

### `port_free`

Require a TCP port to be free at scan time.

```yaml
checks:
  - type: port_free
    port: 5432
```

| Field | Required | Notes |
|---|---|---|
| `port` | yes | TCP port. Probed via `net.Listen("tcp", "127.0.0.1:<port>")`. |

### `env_required`

Require one or more env vars to be present in the process env or `.env`.
**Values are never read or stored** — only the key names enter the report.

```yaml
checks:
  - type: env_required
    keys: [MY_API_KEY, MY_DB_URL]
```

| Field | Required | Notes |
|---|---|---|
| `keys` | yes | List of variable names. Order is preserved in the finding's evidence. |

### `command_present`

Require a command to be on PATH. Cheaper than `tool_version` when you don't
care about the version.

```yaml
checks:
  - type: command_present
    command: kubectl
```

| Field | Required | Notes |
|---|---|---|
| `command` | yes | Looked up via `exec.LookPath`. |

## `overrides`

Modify how envdoctor surfaces an inferred check without disabling it. Today
only `severity` is honored.

```yaml
overrides:
  node-version:
    severity: warning
```

| Field | Allowed values |
|---|---|
| `severity` | `error`, `warning`, `info` |

## `disable`

Skip the named probes entirely. Use this when a probe genuinely doesn't
apply to your repo and the finding it produces would be noise for every
contributor.

```yaml
disable:
  - arch-mismatch
```

Note that disabling a probe takes the finding out of scan output for
**every contributor**, regardless of their host state. Prefer narrower
remediation — fix the root cause in inference, or open a recipe request —
over disabling.

## Validation

Run `envdoctor lint` to validate your file against the schema. Errors come
with stable codes (`E001` … `E010`) so they're scriptable:

```
$ envdoctor lint
E007: unknown check type "typo_here" (at checks[0].type)
```

Exit codes: `0` valid, `4` malformed, `3` crash.

## What `.envdoctor.yaml` can't do

These are **locked decisions**:

- **No `shell:` directive.** Repo-controlled YAML never executes shell. If
  your check needs custom shell, it's probably a new probe upstream, not a
  YAML hook.
- **No conditionals or templating** in the config schema.
- **No remote `include:`**. The whole config lives in the one file.
