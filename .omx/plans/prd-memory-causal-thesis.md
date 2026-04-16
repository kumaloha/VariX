# PRD: Memory Relation-First v2

## Goal
Evolve VariX memory from cluster-centered organization into a long-term cognitive database centered on canonical entities, stable relation boundaries, rich mechanisms, dual aggregates, and traceable derived views.

## User value
A user should see compressed insight first, then expand into readable cards that explain the active mechanism behind a relation, while contradictions are surfaced before any forced abstraction.

## Why v1 is not enough
Current memory/global organizer outputs are useful scaffolding but remain cluster-centric:
- clustering is still heuristic and mostly based on shared phrases/themes
- summaries are derived from clusters rather than from stable relation boundaries
- contradiction signaling exists but is not the explicit abstraction gate
- the primary product object is still cluster-like, not relation-like

## Product principles
1. **Canonical entities anchor the system** — all relation and aggregate logic must resolve to stable canonical entities rather than raw strings.
2. **Relation is the truth boundary** — the base truth object is a single canonical driver affecting a single canonical target.
3. **Mechanism is the high-information layer** — the real explanatory value lives in mechanism nodes, edges, and path outcomes.
4. **Aggregates are indexes, not truth** — driver and target aggregates summarize relation neighborhoods but do not create new truth.
5. **Derived views stay derived** — cards, conclusions, and conflicts are projections over grounded structure.
6. **Conflict blocks abstraction** — unresolved contradiction must surface as conflict instead of a forced conclusion.
7. **Traceability is mandatory** — every derived view must be explainable via canonical entities, relations, mechanisms, and accepted nodes.
8. **Memory is a cognitive database, not an investment engine** — investing workflows may consume memory later but should not drive the memory model.

## Core model shift
### v1 primary object
`GlobalCluster`

### v2 stable objects
- `CanonicalEntity` — stable object anchor for driver/target identity
- `Relation` — stable relation boundary between one driver entity and one target entity

### v2 time-varying content
- `Mechanism`
- `MechanismNode`
- `MechanismEdge`
- `PathOutcome`
- `DriverAggregate`
- `TargetAggregate`

### v2 product-facing objects
- `ConflictView`
- `CognitiveCard`
- `CognitiveConclusion`
- `TopMemoryItem`

## Proposed pipeline
`AcceptedNode -> CanonicalEntity resolution -> Relation match/create -> Mechanism build -> Conflict detection -> DriverAggregate/TargetAggregate -> CognitiveCard/CognitiveConclusion/ConflictView -> TopMemoryItem`

## Object responsibilities
### CanonicalEntity
The stable canonical object for a concept that may play driver, target, or both roles. Owns canonical name, aliases, and merge/split history.

### Relation
The stable truth boundary: one driver entity, one target entity, and a lifecycle. It does not carry final polarity.

### Mechanism
The relation's detailed explanatory body for a given `as_of` slice. Holds metadata only; graph structure lives in nodes/edges/path outcomes.

### MechanismNode
One mechanism graph node with explicit traceability back to accepted memory.

### MechanismEdge
One mechanism graph edge connecting two mechanism nodes.

### PathOutcome
A concrete mechanism path result, including polarity, condition scope, and confidence. Path outcomes are the unit of contradiction.

### DriverAggregate
A derived rollup answering: "this driver currently affects what?"

### TargetAggregate
A derived rollup answering: "this target is currently affected by what?"

### ConflictView
A derived contradiction view built from conflicting path outcomes inside a relation or across aggregates.

### CognitiveCard
A readable relation detail view at `(relation_id, as_of)` granularity, using the active mechanism and its path outcomes as content.

### CognitiveConclusion
A high-level abstract judgment generated only when hard gates and soft-judge gates pass.

### TopMemoryItem
A unified first-layer display shell for `driver_aggregate`, `target_aggregate`, `card`, `conclusion`, and `conflict`.

## In scope
- canonical-entity resolution after accepted memory ingestion
- relation creation and lifecycle management
- mechanism graph construction over accepted memory
- path-outcome modeling with polarity at the path layer
- conflict detection at relation and aggregate stages
- driver/target aggregate synthesis
- card synthesis at `(relation_id, as_of)` granularity
- conclusion synthesis with hard/soft abstraction gates
- top-layer memory items over aggregate/card/conclusion/conflict
- migration path from current cluster-based output
- regression-safe coexistence with existing v1 organizer outputs during rollout
- deletion/retraction semantics via tombstone plus reevaluation

## Out of scope
- automatic conflict resolution on behalf of the user
- investment recommendations or portfolio actions
- multi-driver or multi-target bottom-layer truth objects
- replacing compile-time node extraction in this phase
- ranking/recommendation sophistication beyond simple display ordering
- unlimited fully automatic canonicalization with no review surface

## Hard rules
1. A `Relation` always has exactly one driver entity and one target entity.
2. `Relation` does not store final polarity; polarity exists only at `PathOutcome` level.
3. `Mechanism` must remain traceable through mechanism nodes and accepted nodes.
4. Contradiction is evaluated at path-outcome level, not at mechanism-blob level.
5. A conflict-blocked relation or aggregate must not produce a `CognitiveConclusion`.
6. No top-level conclusion may exist without traceability back to cards, mechanisms, relations, canonical entities, and accepted nodes.
7. Empty/generic abstraction is considered failure and should be dropped instead of displayed.
8. Current cluster outputs may remain as migration scaffolding but must stop being the user-facing mental model.
9. Deletion/retraction uses soft delete or tombstone plus reevaluation; no silent partial stale outputs are acceptable.

## Temporal model
### Stable identity layer
- `CanonicalEntity`
- `Relation`

### Time-varying content layer
- `Mechanism`
- `DriverAggregate`
- `TargetAggregate`
- `ConflictView`
- `CognitiveCard`
- `CognitiveConclusion`

### Temporal rules
- old mechanisms are retained as historical records
- queries are evaluated at a chosen `as_of`
- one relation may have multiple mechanisms over time
- relation lifecycle must support merge, split, retire, and supersede flows

## Conflict model
### Conflict stages
1. **Relation-stage conflict detection** — detect competing path outcomes inside one relation
2. **Aggregate-stage conflict detection** — detect contradictions across relation neighborhoods in driver/target aggregates

### Conflict candidate rule
A conflict candidate requires all of:
- same driver + target scope (or same aggregate scope)
- overlapping time window
- overlapping condition scope
- opposite path-outcome polarity
- no satisfactory condition-based explanation for coexistence

### Conflict lifecycle
`ConflictView` is derived for the current `as_of`, not a bottom-layer truth object. It can disappear, split, or downgrade when new evidence arrives.

## Abstraction gate
### Hard gates
A conclusion candidate must satisfy:
1. `conflict_free == true`
2. `traceability_complete == true`
3. `backing_card_count >= N`
4. `evidence_node_count >= M`
5. `mechanism_path_count >= 1`
6. core canonical entities resolved

### Soft gate
After hard-gate success, an LLM judge records:
- `non_generic_score`
- `summary_quality_score`
- `headline_sharpness_score`
- `novelty_score`
- `judge_passed`
- `judge_model`
- `judge_prompt_version`
- `judged_at`

Soft-gate decisions must be reproducible and auditable.

## Deletion / retraction semantics
Current design uses:
- soft delete / tombstone on withdrawn accepted nodes or false/retracted sources
- full downstream reevaluation on the next organization job

Reevaluation scope includes:
- `Mechanism`
- `PathOutcome`
- `ConflictView`
- `CognitiveCard`
- `CognitiveConclusion`
- `TopMemoryItem`

## Migration strategy
### Phase 1 — coexistence
Keep existing v1 outputs intact while adding v2 structures in parallel.
- clusters remain available for debugging and fallback
- v2 relation pipeline runs side-by-side
- UI/read APIs can compare cluster-first vs relation-first outputs

### Phase 2 — presentation swap
Move first-layer display from `GlobalCluster` summaries to `TopMemoryItem`.
- first layer shows aggregates, cards, conclusions, or conflicts
- clusters become internal/debugging artifacts

### Phase 3 — cluster demotion
Reduce `GlobalCluster` to a compatibility or diagnostic layer only.
- cards, conclusions, and conflicts remain derived product views
- cluster summaries stop being the default product surface

## Data model implications
New persisted payload types are expected under `varix/memory/types.go` and store outputs analogous to current organization outputs.
Likely additions:
- `CanonicalEntity`
- `Relation`
- `Mechanism`
- `MechanismNode`
- `MechanismEdge`
- `PathOutcome`
- `DriverAggregate`
- `TargetAggregate`
- `ConflictView`
- `CognitiveCard`
- `CognitiveConclusion`
- `TopMemoryItem`
- `GlobalMemoryV2Output` (or equivalent umbrella output)

## Acceptance criteria
1. The system can resolve raw accepted-node references into canonical entities with alias/merge/split support.
2. The system can create or match a stable `Relation` from one canonical driver and one canonical target.
3. The system can build one or more `Mechanism` records for a relation over time without overwriting historical states.
4. Mechanism graph structure is traceable through nodes, edges, and path outcomes.
5. Contradictory path outcomes yield a `ConflictView` and block top-level conclusions.
6. Non-contradictory relation neighborhoods can produce `DriverAggregate` and `TargetAggregate` outputs.
7. A `CognitiveCard` is generated at `(relation_id, as_of)` granularity and reflects the active mechanism state.
8. A `CognitiveConclusion` is generated only after hard gates and soft-judge gates pass.
9. First-layer memory display can render aggregates, cards, conclusions, and conflicts through one display shell.
10. Existing accepted memory truth remains unchanged during migration.
11. Existing cluster output remains available during coexistence rollout.
12. Tombstone/retraction flows trigger reevaluation of all affected derived outputs.
