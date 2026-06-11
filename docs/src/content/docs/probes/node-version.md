---
title: node-version
description: Checks that the local Node.js binary satisfies the repo's declared version.
---

| Field | Value |
|---|---|
| Probe ID | `node-version` |
| Category | runtime |
| Severity | error |
| Inferred from | `.nvmrc`, `.node-version`, `package.json` (`engines.node`), `.tool-versions`, `.mise.toml`, `mise.toml` |

## What it means

The repo declares a Node.js version (or constraint) that your local `node --version` doesn't satisfy. Running `npm`, `pnpm`, `yarn`, or any of the project's scripts against the wrong Node major will at minimum produce confusing errors and at worst silently install incompatible native modules.

## How it's detected

1. The probe reads every supported manifest file under the repo root.
2. Each is parsed into a semver constraint (exact for `.nvmrc` / `.node-version`, range for `package.json#engines.node`).
3. `node --version` is exec'd locally (PATH-resolved).
4. The constraint is matched against the running version. A mismatch produces one finding with the strongest constraint surfaced in the summary.

The probe is silent if no manifest declares a Node version — inference-first means missing data is "not applicable," not "broken."

## Common causes

- Wrong version manager active (`mise use node@18` while the repo wants 20).
- No version manager — running the system Node.
- Multiple `node` binaries on `$PATH` and the first one wins.

## Recipes

The probe selects one fix based on the tools available on your machine. See the [YAML source](https://github.com/reswaraa/envdoctor/blob/main/internal/recipes/library/node-version.yaml) for the full Fix definitions.

<!-- BEGIN auto-recipes -->

| Fix | Class | When | Fallback |
|---|---|---|---|
| `mise-install-node` | safe | has_tool=mise |  |
| `fnm-install-node` | safe | has_tool=fnm |  |
| `nvm-install-node` | safe | has_tool=nvm |  |
| `asdf-install-node` | safe | has_tool=asdf |  |
| `brew-install-node` | shared | has_tool=brew | yes |

<!-- END auto-recipes -->
