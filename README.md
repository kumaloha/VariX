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
See `docs/compile-driver-target-schema.md` for the additive driver-target normalization contract layered on top of compile outputs.

## Memory organization

VariX also persists a derived **organized memory view** for each accepted-memory
event stream.

Current organizer contract:
- active vs inactive nodes are split strictly by validity windows
- dedupe and contradiction groups are heuristic overlays, not memory rewrites
- hierarchy links carry `source` + `hint` metadata for frontend rendering
- verifier and validity signals can suppress structural influence without
  deleting accepted memory truth
- v1 cluster outputs remain available while the new v2 thesis-first pipeline
  rolls out in parallel

Useful memory commands:
- `memory global-organize-run|global-organized|global-card` for the existing
  cluster-first global memory view
- `memory global-v2-organize-run|global-v2-organized` for raw thesis-first v2
  JSON output
- `memory global-v2-card` for a human-readable v2 first-layer card surface
- `memory global-v2-card --run` to recompute the latest v2 output and render
  cards in one step
- `memory global-v2-card --item-type conclusion|conflict` to review only one
  class of first-layer items
- `memory global-v2-card --limit N` to sample the first N v2 cards in large
  review sets
- `memory global-compare` to compare the persisted v1 cluster-first and v2
  thesis-first views side by side
- `memory global-compare --run` to recompute both sides before comparing
- `memory global-compare --item-type conclusion|conflict --limit N` to narrow
  the v2 side and shorten large compare outputs

See `docs/memory-organization.md` for the organizer output contract.
