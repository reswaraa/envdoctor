# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Recipe-only changes go in their own `Recipes` section because they ship
on a higher cadence than the rest of the codebase.

## [Unreleased]

### Added

### Changed

### Deprecated

### Removed

### Fixed

### Recipes

## [0.1.0] — 2026-06-12

First public release. The wedge user is an OSS contributor who just cloned
a stranger's repo and the dev command failed.

### Added

#### Probes — nine ship in v0.1.0

- `node-version` — infers from `.nvmrc`, `.node-version`, `package.json#engines.node`, `.tool-versions`, `.mise.toml`, `mise.toml`.
- `python-version` — infers from `pyproject.toml#project.requires-python`, `pyproject.toml#tool.poetry.dependencies.python`, `.python-version`, `.tool-versions`, `mise.toml`.
- `go-version` — infers from `go.mod` (the `go` directive), `.tool-versions`, `mise.toml`.
- `ruby-version` — infers from `Gemfile` (`ruby '…'`), `.ruby-version`, `.tool-versions`, `mise.toml`.
- `env-required` — reads `.env.example` and `docker-compose.yml#${VAR}` references; values never enter the report (structural redaction).
- `port-free` — walks `docker-compose.yml#services.*.ports` and probes each via real `net.Listen`.
- `docker-running` — distinguishes "CLI missing" from "daemon down" via `has_tool:docker` + `docker info` (3s timeout).
- `path-command` — scans `Makefile`, `Procfile`, `docker-compose.yml#services.*.command/entrypoint`; emits one finding per missing command.
- `arch-mismatch` — Apple-Silicon-only; flags known-x86-only versions of `sharp`, `canvas`, `cypress` in `package-lock.json`.

Plus the `custom` meta-probe that interprets `.envdoctor.yaml#checks` declaratively.

#### Recipes — about 25 across the library

Per-tool fixes for `mise`, `fnm`, `nvm`, `asdf`, `brew`, `uv`, `pyenv`, `rbenv`, `colima`, Docker Desktop, `apt`, `systemctl`. Every recipe ships with a `test:` block run twice by the contract harness in CI.

#### Commands

- `envdoctor scan` — runs probes, emits findings in repair order. `--json`, `--bundle`, `--bundle-include-paths`.
- `envdoctor fix` — sequential consent (y/n/s/q), `--yes [--include=shared,destructive]`, `--dry-run`. Privileged recipes are printed, never executed. Audit log at `~/.local/state/envdoctor/audit.log`.
- `envdoctor explain <bundle.json>` — re-render a saved bundle.
- `envdoctor init` — scaffold `./envdoctor` bootstrap + `.envdoctor.yaml`. Flags: `--with-ci`, `--with-config`, `--readme-badge`, `--force`, `--skip-scan`.
- `envdoctor upgrade` — in-place atomic self-update for global installs. Refuses bootstrap-managed copies under `~/.cache/envdoctor/versions/`.
- `envdoctor lint` — validate `.envdoctor.yaml` against the schema.
- `envdoctor version` — print envdoctor build metadata.

#### Output

- Canonical JSON `Report` schema with stable `schema_version: "1"`.
- TTY renderer is a pure presentation layer over the JSON. `NO_COLOR`, `FORCE_COLOR`, `CI` honored.
- Debug bundle: structural-redacted JSON (env values + file contents never enter; `$HOME` and `/Users/<u>` paths stripped by default; `--bundle-include-paths` opts out).
- Exit codes: 0 ok, 1 repairable, 2 no-recipe, 3 crash, 4 config-parse-error.

#### Config

- `.envdoctor.yaml` schema (typed-list discriminator: `tool_version`, `port_free`, `env_required`, `command_present`).
- JSON Schema published at <https://reswaraa.github.io/envdoctor/schemas/v1/config.json>.
- `envdoctor.min_version` enforced via semver.
- `disable:` filter by probe ID.

#### Distribution

- `scripts/install.sh` — POSIX-sh curl|sh installer. Detects OS+arch, resolves latest release tag from `/releases/latest` 302, verifies SHA-256 against `sha256sums.txt`, installs to `~/.local/bin` (or `/usr/local/bin` if writable; never auto-sudo).
- `internal/cli/bootstrap_template.sh` — per-repo bootstrap. Pinned by SHA-256 per `(os, arch)` at `envdoctor init` time. Caches at `~/.cache/envdoctor/versions/`.
- GoReleaser publishes four binaries (`darwin/linux × amd64/arm64`) + `sha256sums.txt` on tag push.
- Docs site live at <https://reswaraa.github.io/envdoctor/>.

### Recipes

Initial library:

- `node-version` × 5 (mise / fnm / nvm / asdf / brew)
- `python-version` × 4 (mise / uv / pyenv / brew)
- `go-version` × 3 (mise / asdf / brew)
- `ruby-version` × 4 (mise / rbenv / asdf / brew)
- `env-required` × 1 (`cp -n .env.example .env`)
- `port-free` × 2 (kill-by-pid destructive; `brew services list` shared fallback)
- `docker-cli-missing` × 2 (`brew --cask` shared; `apt-get` privileged)
- `docker-daemon-down` × 3 (colima safe; Docker Desktop safe; systemctl privileged)
- `path-command-missing` × 2 (brew shared; apt privileged) covering 7 curated commands
- `arch-mismatch` × 1 (bump to arm-compatible pin)

[Unreleased]: https://github.com/reswaraa/envdoctor/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/reswaraa/envdoctor/releases/tag/v0.1.0
