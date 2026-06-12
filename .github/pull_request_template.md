<!--
Thanks for the PR. Please fill out this template.

If you're adding or changing a recipe in internal/recipes/library/,
use the recipe template instead — append `?template=recipe.md` to
the PR URL. The checklist there mirrors the Phase 2C contract.
-->

## Summary

<!-- One paragraph: what changes, why. Skip if it's obvious from the title. -->

## Test plan

- [ ] `go vet ./...`
- [ ] `go test -race -timeout=2m ./...`
- [ ] `gofmt -l internal cmd scripts` produces no output
- [ ] `golangci-lint run ./...` produces no output
- [ ] Manually exercised the affected code path (describe how below)

<!-- "Affected code path" notes: what command / probe / recipe did you run, against what fixture, what did you see? -->

## Anti-features check

This PR does NOT (check all that apply, or leave unchecked + explain why):

- [ ] Add a `shell:` directive to a config schema
- [ ] Add telemetry, accounts, login flows, or any network call during `scan`
- [ ] Wrap or auto-execute `sudo`
- [ ] Rename a probe ID or `doc_url`
- [ ] Introduce conditionals or templating in `.envdoctor.yaml`
- [ ] Mutate the user's README, CONTRIBUTING, or other docs files

If this PR intentionally adopts a new behavior that touches any of the
above, link the discussion that reached that decision in the summary.

## Backwards compatibility

<!-- Does this PR affect the JSON output schema, exit codes, probe IDs,
     bundle format, or `.envdoctor.yaml` schema? If yes, what's the
     migration story? -->
