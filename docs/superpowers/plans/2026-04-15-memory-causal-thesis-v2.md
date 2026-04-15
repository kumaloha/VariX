# Memory Causal Thesis v2 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace cluster-first global memory presentation with a thesis-first pipeline that produces contradiction-gated cognitive cards and top-level abstract conclusions.

**Architecture:** Keep accepted memory truth unchanged, add a parallel v2 organization pipeline, and gradually demote `GlobalCluster` to a compatibility/debug layer. Build the new flow in four increments: v2 types/output shell, candidate thesis formation, conflict/card synthesis, and top-level conclusion synthesis.

**Tech Stack:** Go, SQLite, existing `varix/memory` contracts, `varix/storage/contentstore` organizer patterns, Go test.

---

## File map

### Existing files to modify
- Modify: `varix/memory/types.go`
  - Add thesis-first v2 output contracts while preserving v1 types.
- Modify: `varix/storage/contentstore/sqlite_memory_global.go`
  - Keep v1 behavior unchanged; only extract/retain shared helpers if needed.
- Modify: `varix/storage/contentstore/sqlite_memory_test.go`
  - Keep only coexistence/integration coverage here if absolutely needed.

### New files to create
- Create: `varix/storage/contentstore/sqlite_memory_global_v2.go`
  - V2 organization entrypoints and persistence.
- Create: `varix/storage/contentstore/memory_thesis_builder.go`
  - Candidate thesis formation logic.
- Create: `varix/storage/contentstore/memory_conflict_gate.go`
  - Conflict detection and conflict-set generation.
- Create: `varix/storage/contentstore/memory_causal_thesis.go`
  - Role assignment and causal-path synthesis.
- Create: `varix/storage/contentstore/memory_card_synthesizer.go`
  - Cognitive card generation.
- Create: `varix/storage/contentstore/memory_conclusion_synthesizer.go`
  - Conclusion generation and top item projection.
- Create: `varix/storage/contentstore/memory_thesis_builder_test.go`
- Create: `varix/storage/contentstore/memory_conflict_gate_test.go`
- Create: `varix/storage/contentstore/memory_causal_thesis_test.go`
- Create: `varix/storage/contentstore/memory_card_synthesizer_test.go`
- Create: `varix/storage/contentstore/memory_conclusion_synthesizer_test.go`
- Create: `varix/storage/contentstore/sqlite_memory_global_v2_test.go`

---

### Task 1: Add v2 memory contracts

**Files:**
- Modify: `varix/memory/types.go`
- Test: `varix/storage/contentstore/sqlite_memory_global_v2_test.go`

- [ ] **Step 1: Write the failing contract-shape test**

```go
func TestRunGlobalMemoryOrganizationV2_EmptyScaffold(t *testing.T) {
	store := newTestSQLiteStore(t)
	_, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{
		UserID:           "u-v2",
		SourcePlatform:   "weibo",
		SourceExternalID: "S1",
		NodeIDs:          []string{"n1"},
	})
	if err == nil {
		// fixture intentionally incomplete in this snippet; real test should seed compiled output first
	}
}
```

- [ ] **Step 2: Define new v2 types in `varix/memory/types.go`**

```go
type CandidateThesis struct {
	ThesisID       string    `json:"thesis_id"`
	UserID         string    `json:"user_id"`
	TopicLabel     string    `json:"topic_label"`
	NodeIDs        []string  `json:"node_ids,omitempty"`
	SourceRefs     []string  `json:"source_refs,omitempty"`
	ClusterReason  string    `json:"cluster_reason,omitempty"`
	CoverageScore  float64   `json:"coverage_score,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type ConflictSet struct {
	ConflictID      string    `json:"conflict_id"`
	ThesisID        string    `json:"thesis_id"`
	ConflictStatus  string    `json:"conflict_status"`
	ConflictTopic   string    `json:"conflict_topic,omitempty"`
	SideANodeIDs    []string  `json:"side_a_node_ids,omitempty"`
	SideBNodeIDs    []string  `json:"side_b_node_ids,omitempty"`
	SideASummary    string    `json:"side_a_summary,omitempty"`
	SideBSummary    string    `json:"side_b_summary,omitempty"`
	ConflictReason  string    `json:"conflict_reason,omitempty"`
	SharedQuestion  string    `json:"shared_question,omitempty"`
	UserResolution  string    `json:"user_resolution,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}
```

- [ ] **Step 3: Add remaining causal/card/conclusion output types**

```go
type CausalEdge struct {
	From       string  `json:"from"`
	To         string  `json:"to"`
	Kind       string  `json:"kind"`
	Confidence float64 `json:"confidence,omitempty"`
}

type CausalThesis struct {
	CausalThesisID     string              `json:"causal_thesis_id"`
	ThesisID           string              `json:"thesis_id"`
	Status             string              `json:"status"`
	CoreQuestion       string              `json:"core_question,omitempty"`
	MechanismSummary   string              `json:"mechanism_summary,omitempty"`
	NodeRoles          map[string]string   `json:"node_roles,omitempty"`
	Edges              []CausalEdge        `json:"edges,omitempty"`
	EntryNodeIDs       []string            `json:"entry_node_ids,omitempty"`
	CorePathNodeIDs    []string            `json:"core_path_node_ids,omitempty"`
	SupportingNodeIDs  []string            `json:"supporting_node_ids,omitempty"`
	BoundaryNodeIDs    []string            `json:"boundary_node_ids,omitempty"`
	PredictionNodeIDs  []string            `json:"prediction_node_ids,omitempty"`
	SourceRefs         []string            `json:"source_refs,omitempty"`
	TraceabilityMap    map[string][]string `json:"traceability_map,omitempty"`
	CompletenessScore  float64             `json:"completeness_score,omitempty"`
	AbstractionReady   bool                `json:"abstraction_ready"`
}

type CardChainStep struct {
	Label          string   `json:"label"`
	Role           string   `json:"role"`
	BackingNodeIDs []string `json:"backing_node_ids,omitempty"`
}

type CognitiveCard struct {
	CardID           string          `json:"card_id"`
	CausalThesisID   string          `json:"causal_thesis_id"`
	CardType         string          `json:"card_type"`
	Title            string          `json:"title"`
	Summary          string          `json:"summary,omitempty"`
	CausalChain      []CardChainStep `json:"causal_chain,omitempty"`
	KeyEvidence      []string        `json:"key_evidence,omitempty"`
	Conditions       []string        `json:"conditions,omitempty"`
	Predictions      []string        `json:"predictions,omitempty"`
	SourceRefs       []string        `json:"source_refs,omitempty"`
	ConfidenceLabel  string          `json:"confidence_label,omitempty"`
	ConflictFlag     bool            `json:"conflict_flag,omitempty"`
	TraceEntry       []string        `json:"trace_entry,omitempty"`
}

type CognitiveConclusion struct {
	ConclusionID       string   `json:"conclusion_id"`
	CausalThesisID     string   `json:"causal_thesis_id"`
	Headline           string   `json:"headline"`
	Subheadline        string   `json:"subheadline,omitempty"`
	ConclusionType     string   `json:"conclusion_type,omitempty"`
	BackingCardIDs     []string `json:"backing_card_ids,omitempty"`
	CoreClaims         []string `json:"core_claims,omitempty"`
	WhyItExists        string   `json:"why_it_exists,omitempty"`
	AbstractionLevel   string   `json:"abstraction_level,omitempty"`
	TraceabilityStatus string   `json:"traceability_status,omitempty"`
	BlockedByConflict  bool     `json:"blocked_by_conflict,omitempty"`
	Freshness          string   `json:"freshness,omitempty"`
}

type TopMemoryItem struct {
	ItemID           string    `json:"item_id"`
	ItemType         string    `json:"item_type"`
	Headline         string    `json:"headline"`
	Subheadline      string    `json:"subheadline,omitempty"`
	BackingObjectID  string    `json:"backing_object_id"`
	SignalStrength   string    `json:"signal_strength,omitempty"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type GlobalMemoryV2Output struct {
	OutputID              int64                 `json:"output_id"`
	UserID                string                `json:"user_id"`
	GeneratedAt           time.Time             `json:"generated_at"`
	CandidateTheses       []CandidateThesis     `json:"candidate_theses,omitempty"`
	ConflictSets          []ConflictSet         `json:"conflict_sets,omitempty"`
	CausalTheses          []CausalThesis        `json:"causal_theses,omitempty"`
	CognitiveCards        []CognitiveCard       `json:"cognitive_cards,omitempty"`
	CognitiveConclusions  []CognitiveConclusion `json:"cognitive_conclusions,omitempty"`
	TopMemoryItems        []TopMemoryItem       `json:"top_memory_items,omitempty"`
}
```

- [ ] **Step 4: Run the relevant test target**

Run: `go test ./varix/storage/contentstore -run GlobalMemoryOrganizationV2`
Expected: FAIL at first because v2 entrypoints do not exist yet, then PASS after Task 2.

- [ ] **Step 5: Commit**

```bash
git add varix/memory/types.go
git commit -m "Define thesis-first memory contracts for v2

Constraint: v1 cluster outputs must stay intact during migration
Rejected: Replace GlobalCluster in place | too risky for coexistence and regression isolation
Confidence: high
Scope-risk: moderate
Directive: Keep v2 output additive until thesis-first rendering is proven stable
Tested: Contract compilation + targeted v2 organizer tests
"
```

### Task 2: Add v2 organizer entrypoint and persistence shell

**Files:**
- Create: `varix/storage/contentstore/sqlite_memory_global_v2.go`
- Test: `varix/storage/contentstore/sqlite_memory_global_v2_test.go`

- [ ] **Step 1: Write the failing v2 persistence test**

```go
func TestRunGlobalMemoryOrganizationV2_PersistsOutput(t *testing.T) {
	store := newTestSQLiteStore(t)
	seedAcceptedMemoryFixture(t, store, "u-v2")
	out, err := store.RunGlobalMemoryOrganizationV2(context.Background(), "u-v2", time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("RunGlobalMemoryOrganizationV2 error = %v", err)
	}
	if out.UserID != "u-v2" {
		t.Fatalf("UserID = %q, want u-v2", out.UserID)
	}
}
```

- [ ] **Step 2: Implement v2 organizer shell**

```go
func (s *SQLiteStore) RunGlobalMemoryOrganizationV2(ctx context.Context, userID string, now time.Time) (memory.GlobalMemoryV2Output, error) {
	if strings.TrimSpace(userID) == "" {
		return memory.GlobalMemoryV2Output{}, fmt.Errorf("user id is required")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	nodes, err := s.ListUserMemory(ctx, strings.TrimSpace(userID))
	if err != nil {
		return memory.GlobalMemoryV2Output{}, err
	}
	output := memory.GlobalMemoryV2Output{
		UserID:      strings.TrimSpace(userID),
		GeneratedAt: now,
	}
	_ = nodes // used in later tasks
	return persistGlobalMemoryV2Output(ctx, s.db, output)
}
```

- [ ] **Step 3: Add fetch method + persistence helper**

```go
func (s *SQLiteStore) GetLatestGlobalMemoryOrganizationV2Output(ctx context.Context, userID string) (memory.GlobalMemoryV2Output, error) {
	var payload string
	err := s.db.QueryRowContext(ctx, `SELECT payload_json FROM global_memory_v2_outputs WHERE user_id = ? ORDER BY created_at DESC, output_id DESC LIMIT 1`, strings.TrimSpace(userID)).Scan(&payload)
	if err != nil {
		return memory.GlobalMemoryV2Output{}, err
	}
	var out memory.GlobalMemoryV2Output
	if err := json.Unmarshal([]byte(payload), &out); err != nil {
		return memory.GlobalMemoryV2Output{}, err
	}
	return out, nil
}
```

- [ ] **Step 4: Add schema migration in test setup**

```sql
CREATE TABLE IF NOT EXISTS global_memory_v2_outputs (
  output_id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id TEXT NOT NULL UNIQUE,
  payload_json TEXT NOT NULL,
  created_at TEXT NOT NULL
);
```

- [ ] **Step 5: Run tests**

Run: `go test ./varix/storage/contentstore -run 'GlobalMemoryOrganizationV2|SQLiteStore'`
Expected: PASS for new v2 output persistence tests.

### Task 3: Build candidate theses

**Files:**
- Create: `varix/storage/contentstore/memory_thesis_builder.go`
- Test: `varix/storage/contentstore/memory_thesis_builder_test.go`

- [ ] **Step 1: Write same-question vs false-merge tests**

```go
func TestBuildCandidateTheses_GroupsByCognitiveQuestion(t *testing.T) {}
func TestBuildCandidateTheses_DoesNotMergeSameThemeDifferentQuestion(t *testing.T) {}
```

- [ ] **Step 2: Implement thesis builder skeleton**

```go
func buildCandidateTheses(nodes []memory.AcceptedNode, now time.Time) []memory.CandidateThesis {
	if len(nodes) == 0 {
		return nil
	}
	// start with existing cluster heuristics as scaffolding, then upgrade grouping rules
	return nil
}
```

- [ ] **Step 3: Add grouping helpers**

```go
func sameCognitiveQuestion(left, right memory.AcceptedNode) bool { return false }
func sharedMechanism(left, right memory.AcceptedNode) bool { return false }
func sharedConclusionTarget(left, right memory.AcceptedNode) bool { return false }
func sharedBoundary(left, right memory.AcceptedNode) bool { return false }
```

- [ ] **Step 4: Reuse safe v1 signals without inheriting v1 semantics wholesale**

```go
// allowed as bootstrap signals
// - contradiction group membership is NOT grouping evidence
// - phrase/theme overlap is only weak evidence
// - node kind families remain secondary filters
```

- [ ] **Step 5: Run tests**

Run: `go test ./varix/storage/contentstore -run CandidateTheses`
Expected: PASS with at least one positive grouping fixture and one false-merge guard fixture.

### Task 4: Add conflict gate

**Files:**
- Create: `varix/storage/contentstore/memory_conflict_gate.go`
- Test: `varix/storage/contentstore/memory_conflict_gate_test.go`

- [ ] **Step 1: Write conflict-block tests**

```go
func TestDetectThesisConflict_BlocksConclusionConflict(t *testing.T) {}
func TestDetectThesisConflict_DoesNotFlagSupportingNodes(t *testing.T) {}
```

- [ ] **Step 2: Implement conflict gate entrypoint**

```go
type thesisConflictResult struct {
	Blocked  bool
	Conflict *memory.ConflictSet
}

func detectThesisConflict(thesis memory.CandidateThesis, nodesByID map[string]memory.AcceptedNode, now time.Time) thesisConflictResult {
	return thesisConflictResult{}
}
```

- [ ] **Step 3: Implement conflict classifiers**

```go
func isConclusionConflict(left, right memory.AcceptedNode) bool { return false }
func isMechanismConflict(left, right memory.AcceptedNode) bool { return false }
func isConditionConflict(left, right memory.AcceptedNode) bool { return false }
```

- [ ] **Step 4: Build `ConflictSet` payload**

```go
func buildConflictSet(thesis memory.CandidateThesis, leftIDs, rightIDs []string, reason string, now time.Time) memory.ConflictSet {
	return memory.ConflictSet{ThesisID: thesis.ThesisID, ConflictStatus: "blocked", ConflictReason: reason, CreatedAt: now, UpdatedAt: now}
}
```

- [ ] **Step 5: Run tests**

Run: `go test ./varix/storage/contentstore -run Conflict`
Expected: PASS for contradiction blocking and non-contradiction guard cases.

### Task 5: Build causal theses

**Files:**
- Create: `varix/storage/contentstore/memory_causal_thesis.go`
- Test: `varix/storage/contentstore/memory_causal_thesis_test.go`

- [ ] **Step 1: Write role-assignment and core-path tests**

```go
func TestBuildCausalThesis_AssignsRoles(t *testing.T) {}
func TestBuildCausalThesis_ExtractsCorePath(t *testing.T) {}
```

- [ ] **Step 2: Implement causal thesis builder**

```go
func buildCausalThesis(thesis memory.CandidateThesis, nodesByID map[string]memory.AcceptedNode) memory.CausalThesis {
	return memory.CausalThesis{ThesisID: thesis.ThesisID, Status: "draft"}
}
```

- [ ] **Step 3: Implement helpers**

```go
func assignNodeRoles(nodes []memory.AcceptedNode) map[string]string { return map[string]string{} }
func buildCausalEdges(nodes []memory.AcceptedNode, roles map[string]string) []memory.CausalEdge { return nil }
func extractCorePath(edges []memory.CausalEdge, roles map[string]string) []string { return nil }
```

- [ ] **Step 4: Compute traceability + readiness**

```go
func buildTraceabilityMap(nodes []memory.AcceptedNode) map[string][]string { return map[string][]string{} }
```

- [ ] **Step 5: Run tests**

Run: `go test ./varix/storage/contentstore -run CausalThesis`
Expected: PASS for role mapping and core-path extraction fixtures.

### Task 6: Build cognitive cards

**Files:**
- Create: `varix/storage/contentstore/memory_card_synthesizer.go`
- Test: `varix/storage/contentstore/memory_card_synthesizer_test.go`

- [ ] **Step 1: Write card-shape tests**

```go
func TestBuildCognitiveCards_ProducesReadableCard(t *testing.T) {}
func TestBuildCognitiveCards_DoesNotDumpAllNodes(t *testing.T) {}
```

- [ ] **Step 2: Implement card synthesis**

```go
func buildCognitiveCards(thesis memory.CausalThesis, nodesByID map[string]memory.AcceptedNode) []memory.CognitiveCard {
	return nil
}
```

- [ ] **Step 3: Add card builders**

```go
func buildJudgmentCard(thesis memory.CausalThesis, nodesByID map[string]memory.AcceptedNode) memory.CognitiveCard { return memory.CognitiveCard{} }
func buildMechanismCard(thesis memory.CausalThesis, nodesByID map[string]memory.AcceptedNode) memory.CognitiveCard { return memory.CognitiveCard{} }
func buildPredictionCard(thesis memory.CausalThesis, nodesByID map[string]memory.AcceptedNode) (memory.CognitiveCard, bool) { return memory.CognitiveCard{}, false }
```

- [ ] **Step 4: Run tests**

Run: `go test ./varix/storage/contentstore -run CognitiveCards`
Expected: PASS with at least one title/summary/causal-chain assertion.

### Task 7: Build conclusions and top items

**Files:**
- Create: `varix/storage/contentstore/memory_conclusion_synthesizer.go`
- Test: `varix/storage/contentstore/memory_conclusion_synthesizer_test.go`

- [ ] **Step 1: Write conclusion gate tests**

```go
func TestBuildCognitiveConclusion_AllowsSingleSourceCompleteChain(t *testing.T) {}
func TestBuildCognitiveConclusion_RejectsGenericHeadline(t *testing.T) {}
func TestBuildTopMemoryItems_PrioritizesConflict(t *testing.T) {}
```

- [ ] **Step 2: Implement conclusion synthesis**

```go
func buildCognitiveConclusion(thesis memory.CausalThesis, cards []memory.CognitiveCard) (memory.CognitiveConclusion, bool) {
	return memory.CognitiveConclusion{}, false
}
```

- [ ] **Step 3: Add gating helpers**

```go
func isConclusionAbstractable(thesis memory.CausalThesis, cards []memory.CognitiveCard) bool { return false }
func isGenericConclusion(headline string) bool { return false }
func buildTopMemoryItems(conflicts []memory.ConflictSet, conclusions []memory.CognitiveConclusion, now time.Time) []memory.TopMemoryItem { return nil }
```

- [ ] **Step 4: Run tests**

Run: `go test ./varix/storage/contentstore -run 'CognitiveConclusion|TopMemoryItems'`
Expected: PASS with one positive abstraction case and one generic-rejection case.

### Task 8: Integrate v2 pipeline end-to-end

**Files:**
- Modify: `varix/storage/contentstore/sqlite_memory_global_v2.go`
- Test: `varix/storage/contentstore/sqlite_memory_global_v2_test.go`

- [ ] **Step 1: Wire organizer stages in order**

```go
candidateTheses := buildCandidateTheses(activeNodes, now)
conflicts := make([]memory.ConflictSet, 0)
causalTheses := make([]memory.CausalThesis, 0)
cards := make([]memory.CognitiveCard, 0)
conclusions := make([]memory.CognitiveConclusion, 0)
```

- [ ] **Step 2: Apply conflict gate before any abstraction**

```go
for _, thesis := range candidateTheses {
	result := detectThesisConflict(thesis, nodesByID, now)
	if result.Blocked {
		conflicts = append(conflicts, *result.Conflict)
		continue
	}
	causal := buildCausalThesis(thesis, nodesByID)
	causalTheses = append(causalTheses, causal)
	thesisCards := buildCognitiveCards(causal, nodesByID)
	cards = append(cards, thesisCards...)
	if conclusion, ok := buildCognitiveConclusion(causal, thesisCards); ok {
		conclusions = append(conclusions, conclusion)
	}
}
```

- [ ] **Step 3: Project first-layer items**

```go
topItems := buildTopMemoryItems(conflicts, conclusions, now)
```

- [ ] **Step 4: Persist final v2 payload**

```go
output := memory.GlobalMemoryV2Output{
	UserID:               userID,
	GeneratedAt:          now,
	CandidateTheses:      candidateTheses,
	ConflictSets:         conflicts,
	CausalTheses:         causalTheses,
	CognitiveCards:       cards,
	CognitiveConclusions: conclusions,
	TopMemoryItems:       topItems,
}
```

- [ ] **Step 5: Run full memory tests**

Run: `go test ./varix/storage/contentstore`
Expected: PASS with v1 + v2 coexistence.

### Task 9: Regression verification and docs

**Files:**
- Modify: `README.md`
- Optionally modify: `docs/memory-organization.md` (create if still absent)

- [ ] **Step 1: Document v1/v2 coexistence**

```md
## Memory v2 (thesis-first)

VariX is migrating from cluster-first memory organization to a thesis-first pipeline.
During rollout, cluster outputs remain available for debugging and regression comparison, while v2 introduces:
- candidate theses
- contradiction-first conflict sets
- causal theses
- cognitive cards
- cognitive conclusions
- top memory items
```

- [ ] **Step 2: Run project verification**

Run: `go test ./...`
Expected: PASS

- [ ] **Step 3: Run diagnostics on touched files**

Run: `go test ./varix/storage/contentstore -run 'Memory|Global'`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add README.md docs/memory-organization.md varix/memory/types.go varix/storage/contentstore/*.go varix/storage/contentstore/*_test.go
git commit -m "Shift memory planning toward thesis-first cognition

Constraint: accepted-node truth and v1 cluster outputs must coexist during migration
Rejected: Build conclusions directly from clusters | too easy to hallucinate or force-merge divergent views
Confidence: medium
Scope-risk: broad
Directive: Keep conflict gating ahead of all abstraction and preserve traceability at every synthesized layer
Tested: go test ./varix/storage/contentstore && go test ./...
Not-tested: Frontend rendering of new top memory item shapes
"
```

---

## Self-review
- Spec coverage: covers thesis formation, conflict gate, causal synthesis, cards, conclusions, top items, migration safety, and docs.
- Placeholder scan: implementation stubs remain only where the step explicitly asks the engineer to fill in behavior in that task; no TBD/TODO placeholders remain in plan text.
- Type consistency: all new object names align with `.omx/plans/prd-memory-causal-thesis.md`.
