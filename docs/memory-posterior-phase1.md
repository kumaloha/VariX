# Memory Posterior Verification — Phase 1 Design Contract

This document captures the **phase-1 posterior verification contract** for
source-scoped memory. It is a design/review artifact for the implementation
planned in `.omx/plans/ralplan-posterior-verification-phase1-20260417.md`.

The goal is to add a narrow posterior lifecycle on top of accepted memory
without redesigning compile persistence, prior verification, or global memory
surfaces.

---

## Scope boundary

Phase 1 is intentionally small:

- applies only to accepted `结论` / `预测` memory nodes
- keeps facts in the existing prior verify flow
- stores posterior lifecycle state as mutable sidecar data
- updates **source-scoped** memory reads and organization output
- keeps global v1/v2 organization behavior unchanged in this pass

Non-goals for this phase:

- version history for posterior decisions
- reviewer UI / workflow inboxes
- cross-source retrospective repair
- generic model-driven conclusion adjudication
- compile persistence redesign

---

## Why posterior exists separately from prior verify

The current store already persists:

- accepted memory nodes in `user_memory_nodes`
- prior verification output in compile payloads and organizer hints
- source-scoped organization jobs and outputs

Those seams answer whether a node was accepted and what the compile/verifier
said at acceptance time. They do **not** answer whether an accepted conclusion
or prediction later proved true, false, or blocked.

Phase 1 therefore adds a new lifecycle layer with one hard rule:

> accepted-node snapshot semantics stay stable; posterior state mutates beside
> them, not inside them.

---

## Current code seams the implementation should preserve

The existing code already exposes the main integration points:

- `varix/memory/organization_types.go`
  - `AcceptedNode`
  - `NodeHint`
  - `OrganizationOutput`
- `varix/storage/contentstore/sqlite_memory.go`
  - acceptance + source-scoped list/show-source reads
- `varix/storage/contentstore/sqlite_memory_organizer.go`
  - source-scoped organization pipeline
  - `GetLatestMemoryOrganizationOutput`
- `varix/cmd/cli/memory_commands.go`
  - `memory list`
  - `memory show-source`
  - `memory organize-run`
  - `memory organized`

Code-review guidance:

1. Keep `user_memory_nodes` as the acceptance snapshot table.
2. Prefer additive read-model fields over wrapper-only replacement objects.
3. Keep `GetLatestMemoryOrganizationOutput` a low-level “latest stored output”
   primitive if possible; put stale-read enforcement in a higher-level helper or
   user-facing read path.
4. Do not broaden phase 1 into `global-organize-run` or `global-v2-organize-run`.

---

## Posterior data model

Recommended persistence shape:

- table: `memory_posterior_states`
- key: accepted `memory_id`
- mutable columns:
  - `state`
  - `diagnosis_code`
  - `reason`
  - `blocked_by_node_ids_json`
  - `last_evaluated_at`
  - `last_evidence_at`
  - `updated_at`

Why a sidecar table:

- avoids widening the acceptance snapshot table
- makes posterior mutation explicit
- keeps room for later posterior metadata without distorting acceptance history

---

## State machine

Phase-1 states:

- `pending`
- `verified`
- `falsified`
- `blocked`

Rules:

- newly accepted conclusions/predictions start as `pending`
- facts do **not** receive posterior rows in phase 1
- `blocked` means required conditions are unsatisfied or cannot yet be cleared
- `blocked` is **not** a falsification
- `falsified` must carry at least:
  - `fact_error`, or
  - `logic_error`

Judgment boundaries:

- if due-time has not arrived and no fresher strong evidence exists, keep
  `pending`
- strong fresher support may move a node to `verified`
- strong fresher contradiction may move a node to `falsified`
- unresolved gating conditions move a node to `blocked`
- insufficient deterministic evidence leaves a conclusion `pending`

---

## Read-model contract

Phase 1 should project posterior state through the existing source-scoped read
models instead of inventing a separate accepted-node shell.

### `memory.AcceptedNode`

Additive projection fields are preferred for:

- posterior state
- diagnosis code
- reason
- last updated timestamp

These fields are for read projection only. The acceptance snapshot meaning of
the existing fields should not change.

### `memory.NodeHint`

Organizer-facing hints should gain posterior metadata such as:

- `posterior_state`
- `posterior_diagnosis`
- `blocked_by_node_ids`

This is where organization behavior should become materially different for
`verified`, `pending`, `blocked`, and `falsified` nodes.

---

## Organizer behavior contract

Posterior state must affect more than a label.

Required source-scoped behavior:

- `verified`
  - may stay preferred/stable in organized output
- `pending`
  - should remain provisional and may feed open questions
- `blocked`
  - should surface condition-gate context
  - must not be rendered as falsified
- `falsified`
  - should be demoted from stable presentation
  - should surface diagnosis metadata

Facts remain governed by existing `FactVerifications` and related organizer
logic.

---

## CLI flow contract

Phase 1 should keep posteriority explicit rather than hiding it inside
organization runs.

Expected source-scoped operator flow:

```bash
varix memory accept-batch --user <user> --platform <platform> --id <external_id> --nodes <...>
varix memory posterior-run --user <user> [--platform <platform> --id <external_id>]
varix memory organize-run --user <user>
varix memory organized --user <user> --platform <platform> --id <external_id>
```

Command intent:

- `accept-batch`
  - records accepted nodes and queues the normal source-scoped organization job
- `posterior-run`
  - evaluates eligible conclusion/prediction nodes
  - updates posterior sidecar rows in place
  - writes a synthetic refresh trigger event
  - queues a fresh source-scoped organization job
- `organize-run`
  - refreshes organized output from queued jobs
- `organized`
  - reads the latest **fresh** source-scoped output

---

## Freshness and stale-read contract

Posterior mutation makes prior organized output stale until the queued refresh
job completes.

Phase-1 contract:

- `posterior-run` writes a synthetic `memory_acceptance_events` row with a
  trigger such as `posterior_refresh`
- that synthetic event reuses the existing
  `memory_acceptance_events -> memory_organization_jobs` path
- `memory_organization_jobs.trigger_event_id` remains non-null in phase 1
- `memory organized` must reject stale reads when a newer queued/running job
  exists for the same source than the latest completed output

This keeps staleness explicit for operators instead of silently returning old
payloads as current.

---

## Determinism guardrails

Phase 1 should remain repo-shaped and deterministic:

- use existing compile verification seams
- use accepted-node graph relationships already available in the stored compile
  output
- use evidence freshness only when it is materially grounded in current store
  inputs
- do not add a general retrospective verifier
- do not infer falsification when signals are incomplete

If the repo cannot deterministically adjudicate a conclusion, the correct result
is usually `pending`, not “smart” guesswork.

---

## Review checklist for implementation

When reviewing the implementation, confirm:

1. facts never receive posterior rows
2. acceptance snapshot fields remain semantically unchanged
3. conclusions/predictions seed `pending` rows on accept or lazy backfill
4. `blocked` and `falsified` stay distinct in both storage and output
5. diagnosis codes are present for falsified rows
6. source-scoped `organized` rejects stale output after posterior mutation
7. global v1/v2 commands remain out of scope for phase 1
8. tests prove behavioral change, not only new fields

---

## Suggested verification matrix

Minimum coverage for the phase-1 implementation:

- store tests for posterior-row seeding
- due-time gating tests for predictions
- condition-blocking tests
- diagnosis tests for `fact_error` vs `logic_error`
- refresh-trigger/job-queue tests after posterior mutation
- organizer output tests showing state-aware behavior
- CLI tests for `posterior-run`
- stale-output tests for `memory organized`

The implementation is not complete if posterior state is merely persisted but
does not change the source-scoped operator experience.
