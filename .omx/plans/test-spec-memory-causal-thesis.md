# Test Spec: Memory Relation-First v2

## Purpose
Define regression and behavioral tests for the relation-first memory redesign before implementation.

## Test layers

### 1. Canonical entity resolution
- different surface names for the same driver should resolve to one `CanonicalEntity`
- different surface names for the same target should resolve to one `CanonicalEntity`
- aliases that are genuinely different concepts must not be over-merged
- merge/split history on canonical entities should preserve traceability

### 2. Relation matching and lifecycle
- the same canonical driver + canonical target should match the same `Relation`
- multi-driver or multi-target candidates must not collapse into one relation
- relation merge/split/supersede/retire flows should preserve history
- relation identity should not depend on a path-level polarity decision

### 3. Mechanism construction
- one relation may have multiple `Mechanism` records over time
- each mechanism should yield traceable `MechanismNode` and `MechanismEdge` records
- weakly connected content should be demoted instead of polluting path outcomes
- mechanism updates that only add evidence should not force a brand-new relation

### 4. Path outcomes
- one mechanism may produce multiple competing `PathOutcome` records
- path outcomes must carry polarity, condition scope, and confidence
- contradictory paths with disjoint conditions should not automatically become conflict
- path outcomes should remain queryable without relying on primary/alternative labels

### 5. Conflict detection
- conflicting path outcomes inside one relation should produce a `ConflictView`
- conflicting path outcomes across relation neighborhoods should produce aggregate-level conflict views
- additive/supporting paths should not be mislabeled as contradiction
- time-sliced complementary claims should not be treated as contradiction when they belong to different windows

### 6. Driver and target aggregates
- strong relation neighborhoods should produce both driver and target aggregates
- aggregates should roll up repeated structure without changing underlying relations
- aggregates should preserve traceability back to canonical entities, relations, mechanisms, and accepted nodes
- aggregates must be recomputed per `as_of`

### 7. Cognitive cards
- a card should be generated at `(relation_id, as_of)` granularity
- cards should not degenerate into raw node dumps
- cards should use the active mechanism state and surface competing paths internally when present
- cards should not explode into one default card per path outcome

### 8. Cognitive conclusions
- a traceable, non-conflicting relation neighborhood can produce a top-level abstract conclusion
- hard-gate failure must block conclusion generation before any LLM soft judge runs
- a generic or empty conclusion candidate must be rejected by the soft judge
- a single-source but complete relation may still produce a top-level conclusion
- a conflict-blocked relation or aggregate must not produce any top-level conclusion

### 9. TopMemoryItem output
- first-layer output should render aggregate, card, conflict, and conclusion items through one shape
- contradictory relation neighborhoods should surface `item_type=conflict`
- non-conflicting abstractable relation neighborhoods should surface `item_type=conclusion`
- relation detail surfaces should remain accessible through card items

### 10. Deletion / retraction semantics
- tombstoned accepted nodes should trigger reevaluation of all downstream mechanisms
- invalidated sources should degrade or retire affected path outcomes and cards
- relations with no remaining support should become inactive or retired rather than silently lingering as active
- reevaluation should update conflicts, conclusions, and top items consistently

### 11. Migration safety
- existing `GlobalOrganizationOutput` generation still works during coexistence
- accepted memory rows/events/jobs are unchanged by v2 organizer work
- cluster outputs remain queryable for debugging during rollout
- v2 outputs can be generated alongside v1 outputs from the same accepted memory state

## Verification commands (expected later once implemented)
- `go test ./varix/storage/contentstore -run Memory`
- `go test ./varix/memory/...`
- `go test ./...`

## Required fixture scenarios
1. one-source complete relation with one active mechanism
2. one relation with competing path outcomes of opposite polarity
3. one relation whose conflict disappears after condition scoping is refined
4. cross-source reinforcement of the same relation without creating a new relation identity
5. mechanism evolution over time for the same relation
6. canonical entity false-merge and false-split guards
7. relation lifecycle scenario with retire or supersede
8. deletion/retraction scenario with tombstone-triggered reevaluation
9. migration coexistence scenario with both cluster and relation outputs present
