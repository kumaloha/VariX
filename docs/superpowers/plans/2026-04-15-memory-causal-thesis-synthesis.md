# Memory Relation-First synthesis Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace cluster-first global memory presentation with a relation-first pipeline that uses canonical entities as anchors, relation as the stable truth boundary, mechanism as the explanatory body, and aggregates/cards/conclusions/conflicts as derived views.

**Architecture:** Keep accepted memory truth unchanged, add a parallel synthesis organization pipeline, and gradually demote `GlobalCluster` to a compatibility/debug layer. Build the new flow in six increments: canonical entity resolution, relation contracts, mechanism graph construction, conflict/path outcome modeling, aggregate derivation, and derived-view synthesis.

**Tech Stack:** Go, SQLite, existing `varix/memory` contracts, `varix/storage/contentstore` organizer patterns, Go test.

---

## File map

### Existing files to modify
- Modify: `varix/memory/types.go`
  - Add relation-first synthesis contracts while preserving cluster-first types.
- Modify: `varix/storage/contentstore/sqlite_memory_global.go`
  - Keep cluster-first behavior unchanged; only extract shared helpers if needed.
- Modify: `varix/storage/contentstore/sqlite_memory_test.go`
  - Keep only coexistence/integration coverage here if absolutely needed.

### New files to create
- Create: `varix/storage/contentstore/sqlite_memory_global_synthesis.go`
  - Synthesis organization entrypoints and persistence.
- Create: `varix/storage/contentstore/memory_entity_resolver.go`
  - Canonical entity resolution and alias handling.
- Create: `varix/storage/contentstore/memory_relation_builder.go`
  - Stable relation match/create and lifecycle management.
- Create: `varix/storage/contentstore/memory_mechanism_builder.go`
  - Mechanism head record creation.
- Create: `varix/storage/contentstore/memory_mechanism_nodes.go`
  - Mechanism node extraction and persistence.
- Create: `varix/storage/contentstore/memory_mechanism_edges.go`
  - Mechanism edge construction and persistence.
- Create: `varix/storage/contentstore/memory_path_outcomes.go`
  - Path outcome construction and polarity assignment.
- Create: `varix/storage/contentstore/memory_conflict_view.go`
  - Conflict view projection from conflicting path outcomes.
- Create: `varix/storage/contentstore/memory_driver_aggregate.go`
  - Driver aggregate derivation.
- Create: `varix/storage/contentstore/memory_target_aggregate.go`
  - Target aggregate derivation.
- Create: `varix/storage/contentstore/memory_card_synthesizer.go`
  - Cognitive card generation at `(relation_id, as_of)` granularity.
- Create: `varix/storage/contentstore/memory_conclusion_synthesizer.go`
  - Conclusion generation and top item projection.
- Create corresponding `*_test.go` files for each builder/synthesizer above.

---

## Implementation increments

### Increment 1: Canonical entity layer
- [ ] Add `CanonicalEntity` contracts to `varix/memory/types.go`
- [ ] Add entity resolver tests for alias merge/split and false-merge guards
- [ ] Implement entity resolution from accepted nodes to canonical anchors
- [ ] Persist entity identity and history metadata

### Increment 2: Relation boundary layer
- [ ] Add `Relation` contracts to `varix/memory/types.go`
- [ ] Write failing tests for one-driver/one-target relation identity
- [ ] Implement relation match/create using canonical driver + canonical target only
- [ ] Add relation lifecycle fields and tests for retire/supersede/merge/split
- [ ] Verify relation identity does not depend on path polarity

### Increment 3: Mechanism layer
- [ ] Add `Mechanism`, `MechanismNode`, `MechanismEdge`, and `PathOutcome` contracts
- [ ] Write failing tests for one relation with multiple evolving mechanisms over time
- [ ] Implement mechanism head creation with `as_of`, `valid_from`, `valid_to`, confidence, and traceability status
- [ ] Implement node and edge extraction with accepted-node traceability
- [ ] Implement path outcomes with polarity, condition scope, and confidence
- [ ] Do **not** add `primary_path` / `alternative_path` persistence fields

### Increment 4: Conflict model
- [ ] Write failing tests for contradictory path outcomes inside one relation
- [ ] Write failing tests for aggregate-level contradiction across relation neighborhoods
- [ ] Implement relation-stage conflict detection
- [ ] Implement aggregate-stage conflict detection
- [ ] Persist `ConflictView` keyed by path-outcome references, not mechanism blobs

### Increment 5: Aggregate layer
- [ ] Write failing tests for driver and target aggregates recomputed per `as_of`
- [ ] Implement `DriverAggregate` derivation from relation + mechanism neighborhoods
- [ ] Implement `TargetAggregate` derivation from relation + mechanism neighborhoods
- [ ] Ensure aggregates remain traceable back to canonical entities, relations, mechanisms, and accepted nodes

### Increment 6: Derived views and reevaluation
- [ ] Write failing tests for `CognitiveCard` at `(relation_id, as_of)` granularity
- [ ] Write failing tests for hard-gate blocked conclusion generation
- [ ] Implement cards using the active mechanism state and internal competing-path rendering
- [ ] Implement conclusions with hard gates first, then reproducible soft-judge metadata
- [ ] Implement `TopMemoryItem` projection over aggregates/cards/conclusions/conflicts
- [ ] Implement tombstone-driven reevaluation for deletion/retraction flows

---

## Contract sketch

### Stable identity layer
```go
type CanonicalEntity struct {
    EntityID       string
    EntityType     string
    CanonicalName  string
    Aliases        []string
    Status         string
    MergeHistory   []string
    SplitHistory   []string
}

type Relation struct {
    RelationID              string
    DriverEntityID          string
    TargetEntityID          string
    Status                  string
    RetiredAt               time.Time
    SupersededByRelationID  string
    MergeHistory            []string
    SplitHistory            []string
    LifecycleReason         string
}
```

### Time-varying mechanism layer
```go
type Mechanism struct {
    MechanismID         string
    RelationID          string
    AsOf                time.Time
    ValidFrom           time.Time
    ValidTo             time.Time
    Confidence          float64
    Status              string
    SourceRefs          []string
    TraceabilityStatus  string
}

type MechanismNode struct {
    MechanismNodeID        string
    MechanismID            string
    NodeType               string
    Label                  string
    BackingAcceptedNodeIDs []string
}

type MechanismEdge struct {
    MechanismEdgeID string
    MechanismID     string
    FromNodeID      string
    ToNodeID        string
    EdgeType        string
}

type PathOutcome struct {
    PathOutcomeID   string
    MechanismID     string
    NodePath        []string
    OutcomePolarity string
    OutcomeLabel    string
    ConditionScope  string
    Confidence      float64
}
```

### Derived layer
```go
type ConflictView struct {
    ConflictID           string
    ScopeType            string
    ScopeID              string
    LeftPathOutcomeIDs   []string
    RightPathOutcomeIDs  []string
    ConflictReason       string
    Status               string
    AsOf                 time.Time
}

type CognitiveCard struct {
    CardID          string
    RelationID      string
    AsOf            time.Time
    Title           string
    Summary         string
    MechanismChain  []string
    KeyEvidence     []string
    Conditions      []string
    Predictions     []string
}
```

---

## Verification sequence

### Contract/organizer verification
- `go test ./varix/storage/contentstore -run Memory`
- `go test ./varix/memory/...`
- `go test ./...`

### Review checkpoints
- after canonical entity contracts land
- after relation + mechanism contracts land
- after conflict and aggregate derivation land
- after card/conclusion/tombstone reevaluation land

---

## Migration notes

### Coexistence
- cluster-first `GlobalCluster` remains queryable for debugging/regression comparison
- relation-first synthesis outputs are generated side-by-side from the same accepted memory substrate

### Presentation swap
- first-layer UX moves from cluster summaries to `TopMemoryItem`
- top items may be aggregates, cards, conclusions, or conflicts

### Cluster demotion
- cluster-first becomes diagnostic only once relation-first output quality is proven stronger on real memory sets

---

## Guardrails
- do not persist multi-driver or multi-target truth objects
- do not store path polarity on `Relation`
- do not use mechanism blobs as the unit of contradiction
- do not introduce `primary_path` / `alternative_path` as persistent truth
- do not skip tombstone reevaluation semantics in the final design
