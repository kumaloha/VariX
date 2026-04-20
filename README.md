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
- `memory event-evidence --user <user>` to inspect persisted event evidence links
- `memory event-evidence --event-graph-id <id> --user <user>` to inspect links for one event graph (prints a no-match message when empty)
- `memory paradigm-evidence --user <user>` to inspect persisted paradigm evidence links
- `memory paradigm-evidence --paradigm-id <id> --user <user>` to inspect links for one paradigm (prints a no-match message when empty)
- `memory global-v2-organize-run --user <user>` now refreshes event/paradigm projections before building the global view
- `memory project-all --user <user>` to manually reproject event, paradigm, and global-v2 layers from current graph-first content memory
- `memory content-graphs --user <user>` to inspect graph-first content memory
  snapshots persisted per source
- `memory content-graphs --platform <platform> --id <external_id> --user <user>`
  to narrow to one persisted source snapshot
- `memory content-graphs --subject <subject> --user <user>`
  to narrow snapshots by subject mention
- `memory content-graphs --card --user <user> --platform <platform> --id <external_id>`
  to render a readable content-graph card
- `memory content-graphs --card --subject <subject> --user <user>`
  to render cards for one matching subject
- `memory content-graphs --run --user <user> --platform <platform> --id <external_id>`
  to rebuild one graph-first content snapshot from the latest compiled output
- `memory event-graphs --user <user>` to inspect projected event-layer objects
- `memory event-graphs --scope driver|target --user <user>` to narrow by event scope
- `memory event-graphs --subject <subject> --user <user>` to narrow by anchor subject
- `memory event-graphs --card --user <user>` to render a readable event card view
- `memory event-graphs --card --scope driver|target --user <user>` to render only one scope as cards
- `memory event-graphs --card --subject <subject> --user <user>` to render cards for one anchor subject
- `memory event-graphs --run --user <user>` to force a fresh event projection
- `memory paradigms --user <user>` to inspect projected paradigm objects
- `memory paradigms --subject <subject> --user <user>` to narrow by subject
- `memory paradigms --card --user <user>` to render a readable paradigm card view
- `memory paradigms --card --subject <subject> --user <user>` to render cards for one subject
- `memory paradigms --run --user <user>` to force a fresh paradigm projection
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

Useful verify commands:
- `verify queue --limit N` to inspect the current queue across statuses
- `verify queue --status queued|running|retry|done --limit N` to narrow the queue view by status
- `verify queue --summary` to inspect status counts, object-type counts, `due_count`, and `oldest_scheduled_at` at a glance
- `verify sweep --limit N` to process due queue items using current graph state

See `docs/memory-organization.md` for the organizer output contract.


- `verify show --platform <platform> --id <external_id>` now falls back to current graph-first verification state when no legacy `verification_results` row exists

- `verify run --platform <platform> --id <external_id>` now updates legacy verification results and syncs the graph-first content/event/paradigm chain
