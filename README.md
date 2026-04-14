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
