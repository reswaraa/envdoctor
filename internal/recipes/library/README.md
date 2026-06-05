# Recipe library

Each `*.yaml` in this directory is a `Recipe` — a logical group of repair
commands addressing the same kind of finding (e.g. "node version mismatch").
A Recipe has multiple `fixes`; the matcher picks the best one for the
user's `(os, arch, distro, has_tool)` tuple.

The full schema lives in [`docs/recipes/_schema.md`](../../../docs/recipes/_schema.md);
the Go types are in `../types.go`. Validation is enforced at load time by
`../embed.go`; CI rejects PRs whose recipes do not parse, are missing
required fields, or duplicate an existing ID.

See `CONTRIBUTING.md` at the repo root for the contribution checklist.
