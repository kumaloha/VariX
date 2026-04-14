# VariX

## Compile persistence

VariX persists **one latest compile result per content unit**.

Canonical identity:
- `platform + external_id`

Convenience identity:
- `url -> parse -> platform + external_id`

Default behavior:
- `compile run` returns cached compile output when it already exists
- `compile run --force` recomputes and overwrites the stored result
- `compile show|summary|compare|card` read persisted compile output directly

Current contract:
- latest-only storage
- no compile version history
- no automatic stale detection
- no automatic recompute when ingest changes

Stored metadata includes at least:
- `model`
- `compiled_at`
- `root_external_id`

See `docs/compile-persistence.md` for the full product/engineering contract.

## Memory organization

VariX also persists a derived **organized memory view** for each accepted-memory
event stream.

Current organizer contract:
- active vs inactive nodes are split strictly by validity windows
- dedupe and contradiction groups are heuristic overlays, not memory rewrites
- hierarchy links carry `source` + `hint` metadata for frontend rendering
- verifier and validity signals can suppress structural influence without
  deleting accepted memory truth

See `docs/memory-organization.md` for the organizer output contract.
