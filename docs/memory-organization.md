# Memory Organization

VariX currently has **two global memory organization surfaces**:

1. **v1 cluster-first**
2. **v2 relation-first**

They coexist during rollout. Accepted memory truth (`user_memory_nodes`,
acceptance events, and jobs) remains unchanged. v2 re-expresses that truth as
canonical-entity-anchored relation output.

---

## v1: cluster-first global memory

Primary output object:
- `GlobalOrganizationOutput`

Key characteristics:
- groups accepted nodes into heuristic global clusters
- shows supporting / conflicting / conditional / predictive members
- uses neutral proposition summaries
- remains useful for regression comparison and debugging

CLI:
- `varix memory global-organize-run --user <user_id>`
- `varix memory global-organized --user <user_id>`
- `varix memory global-card --user <user_id>`

---

## v2: relation-first global memory

Primary output object:
- `GlobalMemoryV2Output`

Pipeline:

`AcceptedNode -> CanonicalEntity -> Relation -> Mechanism -> DriverAggregate | TargetAggregate -> ConflictView | CognitiveCard | CognitiveConclusion -> TopMemoryItem`

### CanonicalEntity
The stable anchor for concepts that may act as drivers, targets, or both.

Important fields:
- `entity_id`
- `entity_type`
- `canonical_name`
- `aliases`
- `merge_history`
- `split_history`

Rules:
- aggregates always anchor on canonical entities, not raw strings
- relation endpoints always point at canonical entities
- alias/merge/split logic lives here, not on relation records

### Relation
The stable truth boundary: one canonical driver affects one canonical target.

Important fields:
- `relation_id`
- `driver_entity_id`
- `target_entity_id`
- `status`
- `retired_at`
- `superseded_by_relation_id`
- `merge_history`
- `split_history`

Rules:
- one relation has exactly one driver and one target
- relation stores boundary/identity, not final polarity
- relation supports lifecycle changes such as merge, split, retire, supersede

### Mechanism
The dense/high-information body for a relation at a given `as_of` slice.

Logical sub-objects:
- `Mechanism`
- `MechanismNode`
- `MechanismEdge`
- `PathOutcome`

Rules:
- mechanism explains *how* a relation transmits
- path outcomes carry polarity, condition scope, and confidence
- no `primary_path` / `alternative_path` ground-truth fields exist; path ranking is a rendering concern
- mechanisms are historical records and may evolve over time for the same relation

### DriverAggregate
A derived upper-layer view answering: “this driver currently affects what?”

Important fields:
- `aggregate_id`
- `driver_entity_id`
- `relation_ids`
- `target_entity_ids`
- `coverage_score`
- `conflict_count`
- `as_of`

### TargetAggregate
A derived upper-layer view answering: “this target is currently affected by what?”

Important fields:
- `aggregate_id`
- `target_entity_id`
- `relation_ids`
- `driver_entity_ids`
- `coverage_score`
- `conflict_count`
- `as_of`

### ConflictView
A derived contradiction view built from conflicting path outcomes.

Important fields:
- `conflict_id`
- `scope_type`
- `scope_id`
- `left_path_outcome_ids`
- `right_path_outcome_ids`
- `conflict_reason`
- `status`
- `as_of`

Rules:
- conflict is path-level, not mechanism-blob-level
- conflicts are derived views, not source truth
- conflict can suppress unsupported abstraction without replacing underlying relations

### CognitiveCard
The readable cognition object.

Granularity:
- one main card per `(relation_id, as_of)`

Important fields:
- `card_id`
- `relation_id`
- `as_of`
- `title`
- `summary`
- `mechanism_chain`
- `key_evidence`
- `conditions`
- `predictions`

Rules:
- cards use the active mechanism state as content
- competing paths appear inside the card, not as default separate cards

### CognitiveConclusion
The top-level abstract judgment produced only when hard gates and soft-judge gates pass.

Important fields:
- `conclusion_id`
- `source_type`
- `source_id`
- `headline`
- `subheadline`
- `backing_card_ids`
- `traceability_status`
- `blocked_by_conflict`
- `as_of`

### TopMemoryItem
The first-layer display shell.

Item types:
- `driver_aggregate`
- `target_aggregate`
- `card`
- `conclusion`
- `conflict`

---

## Current v2 quality rules

- canonical entities are the only stable browse anchors
- relations are atomic single-driver/single-target boundaries
- mechanism carries explanatory structure and path-level polarity
- aggregates are derived indexes over relations/mechanisms, not free-form truth
- conclusions require hard-gate success plus reproducible soft-judge approval
- conflicts are derived from contradictory path outcomes and block abstraction
- top items preserve coexistence with v1 and stay useful for debugging/regression comparison
- deletion or retraction is handled via tombstone + reevaluation, not silent partial mutation

---

## CLI

Raw JSON:
- `varix memory global-v2-organize-run --user <user_id>`
- `varix memory global-v2-organized --user <user_id>`
- `global-v2-organize-run` now refreshes persisted event/paradigm projections first, then rebuilds the global view

Graph-first inspection commands:
- `varix memory event-evidence --user <user_id>`
- `varix memory event-evidence --event-graph-id <id> --user <user_id>`
- `varix memory paradigm-evidence --user <user_id>`
- `varix memory paradigm-evidence --paradigm-id <id> --user <user_id>`
- `varix memory project-all --user <user_id>`
- `varix memory content-graphs --user <user_id>`
- `varix memory content-graphs --platform <platform> --id <external_id> --user <user_id>`
- `varix memory content-graphs --card --user <user_id> --platform <platform> --id <external_id>`
- `varix memory content-graphs --run --user <user_id> --platform <platform> --id <external_id>`
- `varix memory event-graphs --user <user_id>`
- `varix memory event-graphs --scope driver|target --user <user_id>`
- `varix memory event-graphs --card --user <user_id>`
- `varix memory event-graphs --card --scope driver|target --user <user_id>`
- `varix memory event-graphs --run --user <user_id>`
- `varix memory paradigms --user <user_id>`
- `varix memory paradigms --subject <subject> --user <user_id>`
- `varix memory paradigms --card --user <user_id>`
- `varix memory paradigms --card --subject <subject> --user <user_id>`
- `varix memory paradigms --run --user <user_id>`

Verify execution commands:
- `varix verify queue --limit 20`
- `varix verify queue --status queued|running|retry|done --limit 20`
- `varix verify queue --summary`
- `varix verify sweep --limit 20`
- `verify sweep` consumes due queue items using current content-graph state and propagates verdict updates back into graph/memory/event/paradigm projections

Human-readable cards:
- `varix memory global-v2-card --user <user_id>`
- `varix memory global-v2-card --user <user_id> --run`
- `varix memory global-v2-card --user <user_id> --item-type conclusion`
- `varix memory global-v2-card --user <user_id> --item-type conflict`
- `varix memory global-v2-card --user <user_id> --limit 5`
- v2 card output includes an `Items` header for the currently rendered slice

Compare surfaces:
- `varix memory global-compare --user <user_id>`
- `varix memory global-compare --user <user_id> --run`
- `varix memory global-compare --user <user_id> --item-type conclusion`
- `varix memory global-compare --user <user_id> --item-type conflict`
- `varix memory global-compare --user <user_id> --limit 5`
- compare output includes section counts for both v1 and v2

Review-friendly behaviors:
- invalid filter values fail fast with explicit guidance
- empty filtered views render a no-match message instead of blank output
- compare headers include item counts and current v2 filter context

---

## Source-scoped organization output

Source-scoped memory keeps its own JSON surface:

- `varix memory organize-run --user <user_id>`
- `varix memory organized --user <user_id> --platform <platform> --id <external_id>`

Primary output object:
- `OrganizationOutput`

Important review-facing fields:
- `node_hints[].node_verdict` — compact per-node verdict such as `supported`,
  `needs_review`, `contradicted`, `blocked`, or `falsified`
- `node_hints[].driver_role` — whether a node is currently treated as the
  `primary` or `supporting` driver in the source-scoped path
- `dominant_driver` — summary of the current primary driver, supporting driver
  ids, and the explanation for the primary-vs-supporting split
- `feedback` — strongest-error-first feedback items for failed predictions,
  blocked/falsified posterior state, weak evidence, conflicts, and near
  duplicates

Intent:
- keep raw accepted nodes visible, but attach UI-ready verdicts
- highlight the strongest current upstream driver without hiding secondary
  drivers
- let read surfaces show the most actionable problems first instead of forcing
  clients to sort many low-level hints themselves

Rules:
- source-scoped driver ranking is a display aid, not a new truth object
- primary/supporting labels must stay traceable to accepted nodes + hierarchy
- feedback ordering should keep hard failures ahead of weak-evidence warnings
- stale source-scoped output still fails fast after posterior mutation

---

## Source-scoped posterior verification (phase 1 design)

Source-scoped memory also has a planned posterior-verification extension for
accepted `结论` / `预测` nodes.

Key design rules:

- posterior lifecycle is **not** a replacement for prior fact verification
- facts stay in the existing verify flow
- accepted-node snapshot semantics stay stable
- posterior state is stored as mutable sidecar data
- source-scoped `organized` reads must treat post-posterior stale output as an
  explicit error, not silently current data
- global v1/v2 organization surfaces stay out of scope for the first pass

See `docs/memory-posterior-phase1.md` for the detailed phase-1 contract and
review checklist.

---

## Rollout intent

v2 is intended to become the product-facing memory layer because it better
supports:
- canonical entity anchoring
- atomic relation truth boundaries
- mechanism-rich explanatory structure
- driver and target navigation
- contradiction-aware derived views
- multi-source cognitive synthesis

v1 remains available until relation-first output quality is consistently stronger
than cluster-first output on real memory sets.


- `verify show --platform <platform> --id <external_id>` now falls back to current graph-first verification state when no legacy `verification_results` row exists

- `verify run --platform <platform> --id <external_id>` now updates legacy verification results and syncs the graph-first content/event/paradigm chain
