# Test container fixtures

These Dockerfiles back the `test:` block of each recipe Fix. The recipe
contract harness ([`scripts/recipe-test/`](../../scripts/recipe-test/))
builds them on first use (cached by Dockerfile SHA in CI) and runs each
Fix inside the matching image to verify before/after assertions and
idempotence.

## Fixtures shipped

| Image tag | Dockerfile | Purpose |
|---|---|---|
| `envdoctor/test-linux-fresh:latest` | `linux-fresh.Dockerfile` | Minimal Debian + shell utilities. Used by recipes that only need `cp`, `lsof`, `kill`. |
| `envdoctor/test-linux-mise:latest`  | `linux-mise.Dockerfile`  | Debian + mise installed; shims on PATH. |
| `envdoctor/test-linux-fnm:latest`   | `linux-fnm.Dockerfile`   | Debian + fnm installed. |
| `envdoctor/test-linux-asdf:latest`  | `linux-asdf.Dockerfile`  | Debian + asdf 0.15.x sourced via `/etc/profile.d`. |

## Intentionally omitted

- **`linux-nvm`** — nvm is a shell function loaded from `nvm.sh`, not a
  binary on PATH. Reproducing the typical user environment requires
  sourcing nvm.sh in a way that `bash -c` honors, and the nvm Fix in
  `node-version.yaml` will only ever match for users who installed nvm
  as a binary. The Fix is documentation-only in the recipe library and
  has no contract test. Reintroducing a fixture is a future PR.
- **`darwin-brew`** — Docker cannot run macOS. The brew-on-darwin Fix
  in `node-version.yaml` and `port-free.yaml` is exercised manually on
  macOS dev hosts; CI does not validate it.

## Building locally

```sh
# Build all fixtures.
for f in testdata/containers/*.Dockerfile; do
  name=$(basename "$f" .Dockerfile)
  docker build -f "$f" -t "envdoctor/test-$name:latest" .
done

# Or build a single fixture.
docker build -f testdata/containers/linux-mise.Dockerfile \
  -t envdoctor/test-linux-mise:latest .
```

## What the harness does

For each Fix in the library:

1. Pulls or builds the `test.image` (cached).
2. Spawns the container and runs `before.check`. **Must exit non-zero**
   — otherwise the precondition was already satisfied and the test cannot
   prove the Fix did anything.
3. Runs the rendered `command`.
4. Runs `after.check`. **Must exit zero.**
5. If `after.idempotent: true`, re-runs the command and re-runs the
   after check. Both must still hold.

Failures (any of the above conditions) surface as a CI failure with the
Fix ID and container image in the message.
