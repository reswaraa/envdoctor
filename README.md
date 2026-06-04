# EnvDoctor

> Diagnose why a freshly cloned repo will not run on your machine, and get
> copy-pasteable fixes — no config required.

EnvDoctor is a pure-OSS (Apache-2.0) Go CLI that snapshots your local runtime
state, compares it with what the repo expects, and tells you exactly what is
wrong: wrong Node version, missing env vars, port 5432 already taken, Docker
not running, missing tools on PATH, Apple-Silicon vs x86 mismatch. It works
with zero configuration on any repo by reading the manifests you already
have (`.nvmrc`, `package.json`, `docker-compose.yml`, `pyproject.toml`, …).

**Status:** pre-0.1, in active development. Not yet released.

## Why

READMEs and Docker Compose files describe the _desired_ environment. They do
not diagnose _your actual broken machine_. EnvDoctor does.

## Install (coming soon)

```sh
# Once 0.1.0 ships:
curl -sSL https://envdoctor.dev/install.sh | sh
```

For repo maintainers, `envdoctor init` scaffolds a small committable
`./envdoctor` bootstrap so contributors can run the tool without installing
anything globally.

## What it checks (MVP)

- Runtime versions (Node, Python, Go, Ruby, Rust)
- Required env vars and `.env` file presence
- Docker daemon state and compose plugin
- Port collisions (`docker-compose.yml` ports already in use)
- Required commands on PATH (`make`, `psql`, `protoc`, …)
- Apple Silicon vs x86 mismatches

## What it never does

- Run shell from a repo's config file (no `shell:` directive — ever)
- Send telemetry; require an account; phone home
- Auto-run `sudo` commands
- Mutate your machine without explicit per-command consent

## License

Apache-2.0. See [`LICENSE`](./LICENSE).
