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
- `compile run` and `compile show` now expose `metrics.compile_elapsed_ms`
  in the JSON record payload for compile observability
- `compile summary` and `compile compare` now render compile elapsed time plus
  stage timing summaries in their human-readable output

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
- cluster-first outputs remain available while the current synthesis pipeline
  rolls out in parallel

Useful memory commands:
- `memory event-evidence --user <user>` to inspect persisted event evidence links
- `memory event-evidence --event-graph-id <id> --user <user>` to inspect links for one event graph (prints a no-match message when empty)
- `memory event-evidence --card --user <user>` to render a readable event-evidence view
- `memory paradigm-evidence --user <user>` to inspect persisted paradigm evidence links
- `memory paradigm-evidence --paradigm-id <id> --user <user>` to inspect links for one paradigm (prints a no-match message when empty)
- `memory paradigm-evidence --card --user <user>` to render a readable paradigm-evidence view
- `memory global-synthesis-run --user <user>` now refreshes event/paradigm projections before building the global view
- `memory project-all --user <user>` to manually reproject event, paradigm, and global-synthesis layers from current graph-first content memory
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
- `memory backfill --layer content --user <user> --platform <platform> --id <external_id>`
  to backfill one content graph from compiled output
- `memory backfill --layer event|paradigm|global-synthesis|all --user <user>`
  to rebuild graph-first aggregate layers from persisted content graphs
- `memory cleanup-stale --user <user> --older-than 24h`
  to delete stale queued/running memory organization jobs older than the threshold
- `memory cleanup-stale --user <user> --older-than 24h --platform <platform> [--id <external_id>]`
  to scope stale-job cleanup to one source platform or one source item
- `memory cleanup-stale --user <user> --older-than 24h --dry-run`
  to preview how many stale jobs would be deleted without mutating data
- `memory jobs --user <user> --status queued|running|done`
  to inspect one job status lane only
- `memory jobs --user <user> --platform <platform> [--id <external_id>]`
  to scope job inspection to one source platform or one source item
- `memory jobs --user <user> --summary`
  to inspect job counts, `stale_candidates`, `stale_queued`, `stale_running`,
  and oldest queued/running timestamps
- `memory canonical-entities`
  to inspect persisted canonical subject/entity anchors and aliases
- `memory canonical-entities --id <entity_id>`
  to inspect one canonical entity directly by id
- `memory canonical-entities --alias <alias>`
  to resolve one alias to its persisted canonical entity
- `memory canonical-entities --type <driver|target|both> --status <active|merged|split|retired>`
  to filter the canonical entity catalog by type and lifecycle state
- `memory canonical-entities --card`
  to render canonical entities in a readable operator-facing view
- `memory canonical-entities --summary`
  to inspect canonical entity counts by type/status plus total alias volume
- `memory canonical-entity-upsert --id <entity_id> --type <driver|target|both> --name <canonical_name> [--aliases a,b]`
  to apply a human-reviewed canonical override/alias mapping
- `memory canonical-entity-upsert --status <active|merged|split|retired> ...`
  to explicitly control canonical entity lifecycle state when applying an override
- `memory event-graphs --user <user>` to inspect projected event-layer objects
- `memory event-graphs --scope driver|target --user <user>` to narrow by event scope
- `memory event-graphs --subject <subject> --user <user>` to narrow by anchor subject
  (supports canonical alias lookup)
- `memory event-graphs --card --user <user>` to render a readable event card view
- `memory event-graphs --card --scope driver|target --user <user>` to render only one scope as cards
- `memory event-graphs --card --subject <subject> --user <user>` to render cards for one anchor subject
  (supports canonical alias lookup)
- `memory event-graphs --run --user <user>` to force a fresh event projection
- `memory paradigms --user <user>` to inspect projected paradigm objects
- `memory paradigms --subject <subject> --user <user>` to narrow by subject
  (supports canonical alias lookup)
- `memory paradigms --card --user <user>` to render a readable paradigm card view
- `memory paradigms --card --subject <subject> --user <user>` to render cards for one subject
  (supports canonical alias lookup)
- `memory paradigms --run --user <user>` to force a fresh paradigm projection
- `memory global-organize-run|global-organized|global-card` for the existing
  cluster-first global memory view
- `memory global-synthesis-run|global-synthesis` for raw synthesis
  JSON output
- `memory global-synthesis-card` for a human-readable synthesis first-layer card surface
- `memory global-synthesis-card --run` to recompute the latest synthesis output and render
  cards in one step
- `memory global-synthesis-card --item-type conclusion|conflict` to review only one
  class of first-layer items
- `memory global-synthesis-card --limit N` to sample the first N synthesis cards in large
  review sets
- `memory global-compare` to compare the persisted cluster-first and synthesis
  views side by side
- `memory global-compare --run` to recompute both sides before comparing
- `memory global-compare --item-type conclusion|conflict --limit N` to narrow
  the synthesis side and shorten large compare outputs
- `memory project-all --user <user>` now emits rebuild metrics for event graphs,
  paradigms, and global-synthesis output
- `verify queue --summary` now emits queue status counts, object types, due count,
  `total_count`, oldest scheduled item, and `pending_age_buckets`

Useful verify commands:
- `verify queue --limit N` to inspect the current queue across statuses
- `verify queue --status queued|running|retry|done --limit N` to narrow the queue view by status
- `verify queue --summary` to inspect status counts, object-type counts, `due_count`, and `oldest_scheduled_at` at a glance
- `verify sweep --limit N` to process due queue items using current graph state
- `verify show --platform <platform> --id <external_id>` now falls back to current graph-first verification state when no legacy `verification_results` row exists
- `verify run --platform <platform> --id <external_id>` now updates legacy verification results and syncs the graph-first content/event/paradigm chain

See `docs/memory-organization.md` for the organizer output contract.

## Development

Go tests live under the repository-level `tests/` tree, mirroring the package
layout under `varix/`. Run them through the overlay helper so same-package tests
are mounted back into the Go module. Plain `go test ./...` from `varix/` only
sees package-local tests checked into the module, so CI and local verification
must use this entrypoint:

```bash
./tests/go-test.sh -count=1 ./...
```
