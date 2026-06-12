# EnvDoctor

[![envdoctor scan](https://reswaraa.github.io/envdoctor/badge.svg)](https://reswaraa.github.io/envdoctor/)

> Diagnose why a freshly cloned repo will not run on your machine, and get
> copy-pasteable fixes. No telemetry, no accounts, no server.

EnvDoctor is a pure-OSS (Apache-2.0) Go CLI that reads the manifest files you
already have (`.nvmrc`, `package.json`, `docker-compose.yml`, `pyproject.toml`,
`go.mod`, …), probes your local system, and tells you exactly what's broken
with the command that fixes it.

The wedge user is an **OSS contributor who just cloned a stranger's repo and
the `dev` command failed**. Every README in that situation describes the
*desired* environment. None of them diagnoses *your actual broken machine*.

## Install

```sh
curl -fsSL https://reswaraa.github.io/envdoctor/install.sh | sh
```

The installer detects your OS+arch, verifies SHA-256 against the
release's `sha256sums.txt`, and installs to `~/.local/bin` (or `/usr/local/bin`
if writable). Never auto-sudo.

For repo maintainers: `envdoctor init` writes a 45-line POSIX-sh bootstrap
your contributors can run as `./envdoctor scan` with no global install
required. See the [docs](https://reswaraa.github.io/envdoctor/) for the full
workflow.

## Quick example

```sh
git clone https://github.com/somebody/some-repo
cd some-repo
envdoctor scan
```

```
environment
  ✗ 5 required env vars missing: DATABASE_URL, JWT_SECRET, PORT, POSTGRES_PASSWORD, POSTGRES_USER
    fix: cp -n .env.example .env

runtime
  ✗ Node 18.20.8 detected; repo requires 20.10.0
    fix: brew install node@20
    docs: https://reswaraa.github.io/envdoctor/probes/node-version

Scan finished in 124ms. 2 errors.
```

Then `envdoctor fix` walks you through each one interactively (`y/n/s/q` per
recipe). Safe recipes auto-run with `--yes`; destructive and privileged
recipes always prompt.

## What it checks (v0.1.0)

| Category | Probe | Reads |
|---|---|---|
| runtime | [`node-version`](https://reswaraa.github.io/envdoctor/probes/node-version/) | `.nvmrc`, `.node-version`, `package.json#engines.node`, `.tool-versions`, `.mise.toml` |
| runtime | [`python-version`](https://reswaraa.github.io/envdoctor/probes/python-version/) | `pyproject.toml`, `.python-version`, `.tool-versions`, `mise.toml` |
| runtime | [`go-version`](https://reswaraa.github.io/envdoctor/probes/go-version/) | `go.mod`, `.tool-versions`, `mise.toml` |
| runtime | [`ruby-version`](https://reswaraa.github.io/envdoctor/probes/ruby-version/) | `Gemfile`, `.ruby-version`, `.tool-versions`, `mise.toml` |
| environment | [`env-required`](https://reswaraa.github.io/envdoctor/probes/env-required/) | `.env.example`, `docker-compose.yml#${VAR}` |
| ports | [`port-free`](https://reswaraa.github.io/envdoctor/probes/port-free/) | `docker-compose.yml#services.*.ports` |
| docker | [`docker-running`](https://reswaraa.github.io/envdoctor/probes/docker-running/) | `Dockerfile` / `docker-compose.yml` presence |
| path | [`path-command`](https://reswaraa.github.io/envdoctor/probes/path-command/) | `Makefile`, `Procfile`, compose `command`/`entrypoint` |
| architecture | [`arch-mismatch`](https://reswaraa.github.io/envdoctor/probes/arch-mismatch/) | `package-lock.json` (Apple Silicon only) |

## What it never does

- Run shell from a repo's config file (no `shell:` directive, ever).
- Send telemetry; require an account; phone home.
- Auto-run `sudo` commands (privileged recipes are printed for you to run).
- Mutate your machine without explicit per-command consent.

See [`CONTRIBUTING.md`](./CONTRIBUTING.md) for the full anti-features list.

## Contributing recipes

The defensibility play is a community-grown recipe library. Every recipe
ships with a `test:` block run twice in a fresh container: once to confirm
the broken state, and once after the fix to confirm repair. PRs that add a recipe
should follow the
[recipe schema](https://reswaraa.github.io/envdoctor/recipes/schema/) and use
the [recipe PR template](./.github/PULL_REQUEST_TEMPLATE/recipe.md).

## License

Apache-2.0. See [`LICENSE`](./LICENSE).
