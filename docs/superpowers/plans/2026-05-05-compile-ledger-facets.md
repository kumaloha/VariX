# Compile Ledger Facets Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a loss-minimizing compile ledger under salience so articles and meetings share preserved facts while keeping mainline and brief as separate reader views.

**Architecture:** Add a compact `model.Ledger` persisted in `model.Output`, build it from current salience units with deterministic enrichment, derive brief from ledger, and add a coverage audit that exposes view omissions without making mainline carry meeting coverage.

**Tech Stack:** Go, existing `compile` pipeline, existing JSON output persistence, Go tests, gofmt

---

### Task 1: Add the ledger model contract

**Files:**
- Create: `varix/model/result_ledger.go`
- Modify: `varix/model/result_output.go`
- Modify: `varix/model/result_stage_outputs.go`
- Modify: `varix/model/parse.go`
- Modify: `varix/model/result_validation.go`
- Test: `tests/model/content_graph_test.go`

- [ ] **Step 1: Write the failing model test**

Add a test that proves compiled output can carry a ledger item without depending
on render or brief.

```go
func TestOutputPreservesLedgerItems(t *testing.T) {
    out := model.Output{
        Summary: "Berkshire meeting digest",
        Ledger: model.Ledger{
            Items: []model.LedgerItem{{
                ID:       "ledger-001",
                Kind:     "list",
                Category: "portfolio",
                Claim:    "Berkshire discussed the major public holdings.",
                Entities: []string{"Apple", "American Express", "Coca-Cola", "Bank of America"},
                SourceIDs: []string{"semantic-014"},
            }},
        },
    }
    if err := out.Validate(); err != nil {
        t.Fatalf("Validate() = %v", err)
    }
    payload, err := json.Marshal(out)
    if err != nil {
        t.Fatalf("Marshal() = %v", err)
    }
    var roundTrip model.Output
    if err := json.Unmarshal(payload, &roundTrip); err != nil {
        t.Fatalf("Unmarshal() = %v", err)
    }
    if got := roundTrip.Ledger.Items[0].Entities; !slices.Contains(got, "Apple") {
        t.Fatalf("ledger entities = %#v, want Apple", got)
    }
}
```

- [ ] **Step 2: Run the focused test and verify it fails**

Run: `bash tests/go-test.sh ./model -run TestOutputPreservesLedgerItems -count=1`

Expected: FAIL because `model.Output` has no `Ledger` field and ledger types do
not exist yet.

- [ ] **Step 3: Add the ledger model**

Create `varix/model/result_ledger.go`:

```go
package model

type Ledger struct {
    Items []LedgerItem `json:"items,omitempty"`
}

type LedgerItem struct {
    ID        string   `json:"id,omitempty"`
    Kind      string   `json:"kind,omitempty"`
    Category  string   `json:"category,omitempty"`
    Claim     string   `json:"claim,omitempty"`
    Entities  []string `json:"entities,omitempty"`
    Numbers   []string `json:"numbers,omitempty"`
    Quote     string   `json:"quote,omitempty"`
    SourceIDs []string `json:"sourceIds,omitempty"`
    Salience  float64  `json:"salience,omitempty"`
}
```

- [ ] **Step 4: Wire ledger into output parsing and validation**

Add `Ledger Ledger json:"ledger,omitempty"` to `model.Output`,
`UnifiedCompileOutput`, and `StageOutputs`.

In `model/parse.go`, unmarshal `payload["ledger"]` into `out.Ledger`.

In `model/result_validation.go`, treat non-empty `Ledger.Items` like an existing
semantic detail field so ledger-only outputs are valid when they have a summary.

- [ ] **Step 5: Run the model tests**

Run: `bash tests/go-test.sh ./model -count=1`

Expected: PASS.

### Task 2: Build ledger items from salience without truncation

**Files:**
- Create: `varix/compile/pipeline_ledger.go`
- Modify: `varix/compile/pipeline.go`
- Modify: `varix/compile/pipeline_render.go`
- Test: `tests/compile/ledger_test.go`

- [ ] **Step 1: Write failing compile tests**

Create `tests/compile/ledger_test.go` with tests for list preservation, category
separation, and source ID retention.

```go
func TestLedgerPreservesPortfolioListFacts(t *testing.T) {
    state := graphState{
        SemanticUnits: []SemanticUnit{{
            ID:       "semantic-014",
            Subject:  "existing portfolio / circle of competence",
            Claim:    "Greg Abel said Berkshire remains comfortable with Apple, American Express, Coca-Cola, and Bank of America because the businesses remain understandable.",
            Force:    "answer",
            Salience: 0.77,
        }},
    }

    got := buildLedger(state)
    item := ledgerItemByCategory(got.Items, "portfolio")
    if item == nil {
        t.Fatalf("ledger items = %#v, want portfolio item", got.Items)
    }
    for _, entity := range []string{"Apple", "American Express", "Coca-Cola", "Bank of America"} {
        if !slices.Contains(item.Entities, entity) {
            t.Fatalf("portfolio entities = %#v, want %q", item.Entities, entity)
        }
    }
    if !slices.Contains(item.SourceIDs, "semantic-014") {
        t.Fatalf("source IDs = %#v, want semantic-014", item.SourceIDs)
    }
}
```

- [ ] **Step 2: Run the focused compile test and verify failure**

Run: `bash tests/go-test.sh ./compile -run TestLedgerPreservesPortfolioListFacts -count=1`

Expected: FAIL because `buildLedger` does not exist.

- [ ] **Step 3: Implement `pipeline_ledger.go`**

Add:

```go
func stageLedger(state graphState) graphState {
    if len(state.Ledger.Items) > 0 {
        return state
    }
    state.Ledger = buildLedger(state)
    return state
}

func buildLedger(state graphState) Ledger {
    items := make([]LedgerItem, 0, len(state.SemanticUnits))
    for _, unit := range state.SemanticUnits {
        item := ledgerItemFromSemanticUnit(unit)
        if strings.TrimSpace(item.Claim) == "" {
            continue
        }
        item.ID = fmt.Sprintf("ledger-%03d", len(items)+1)
        items = append(items, item)
    }
    return Ledger{Items: items}
}
```

Use the compile package aliases for model types, matching existing aliases in
`varix/compile/model_alias.go`.

- [ ] **Step 4: Add deterministic enrichment helpers**

In `pipeline_ledger.go`, add helpers for:

- `ledgerKind(unit SemanticUnit) string`
- `ledgerCategory(unit SemanticUnit) string`
- `ledgerEntities(text string) []string`
- `ledgerNumbers(text string) []string`
- `containsAIReference(text string) bool`

Apply these rules:

- list kind when a claim has three or more known named entities
- portfolio category before AI category
- operations category before AI category for BNSF, margin, railroad, operating
  plan, and cost-reduction claims
- AI category only when text contains explicit `AI`, `artificial intelligence`,
  `人工智能`, or `大模型`
- culture and succession stay separate when subject/claim points to one of them

- [ ] **Step 5: Wire stage and run tests**

Add `Ledger Ledger` to `graphState`.

Call `stageLedger` after `stageSalience` and before `stageBrief`.

Run: `bash tests/go-test.sh ./compile -run Ledger -count=1`

Expected: PASS.

### Task 3: Derive brief from ledger with category coverage guarantees

**Files:**
- Modify: `varix/compile/pipeline_brief.go`
- Test: `tests/compile/brief_test.go`

- [ ] **Step 1: Add failing brief coverage tests**

Add tests proving portfolio and succession survive into meeting brief even when
capital, insurance, and AI items have higher salience.

```go
func TestBriefKeepsMandatoryMeetingCategoriesFromLedger(t *testing.T) {
    ledger := Ledger{Items: []LedgerItem{
        {ID: "ledger-001", Category: "capital", Claim: "Hold cash until the right opportunity appears.", Salience: 0.98},
        {ID: "ledger-002", Category: "insurance", Claim: "Do not write cyber risk when aggregation cannot be modeled.", Salience: 0.96},
        {ID: "ledger-003", Category: "ai", Claim: "AI must remain additive and supervised.", Salience: 0.94},
        {ID: "ledger-004", Category: "portfolio", Kind: "list", Claim: "The portfolio includes Apple, American Express, Coca-Cola, and Bank of America.", Entities: []string{"Apple", "American Express", "Coca-Cola", "Bank of America"}, Salience: 0.7},
        {ID: "ledger-005", Category: "succession", Claim: "The board has succession plans for Greg Abel and Ajit Jain.", Salience: 0.68},
    }}

    got := buildBriefFromLedger(ledger, "reader_interest")
    if briefItemByCategory(got, "portfolio") == nil {
        t.Fatalf("brief = %#v, want portfolio", got)
    }
    if briefItemByCategory(got, "succession") == nil {
        t.Fatalf("brief = %#v, want succession", got)
    }
}
```

- [ ] **Step 2: Run the focused test and verify failure**

Run: `bash tests/go-test.sh ./compile -run TestBriefKeepsMandatoryMeetingCategoriesFromLedger -count=1`

Expected: FAIL because brief still builds directly from salience or lacks ledger
mandatory category selection.

- [ ] **Step 3: Change brief builder input to ledger**

Replace direct semantic-unit selection in `stageBrief` with ledger-based
selection:

```go
func stageBrief(state graphState) graphState {
    if len(state.Brief) > 0 {
        return state
    }
    if len(state.Ledger.Items) == 0 {
        state = stageLedger(state)
    }
    state.Brief = buildBriefFromLedger(state.Ledger, state.ArticleForm)
    return state
}
```

- [ ] **Step 4: Add category coverage selection**

Implement:

- mandatory meeting categories: `capital`, `portfolio`, `insurance`, `ai`,
  `energy`, `culture`, `succession`, `governance`
- max two items per category
- overall default budget 14
- list items outrank non-list items inside the same category
- after mandatory categories are seeded, fill remaining slots by salience

- [ ] **Step 5: Run compile tests**

Run: `bash tests/go-test.sh ./compile -count=1`

Expected: PASS.

### Task 4: Add coverage audit diagnostics

**Files:**
- Create: `varix/compile/pipeline_coverage_audit.go`
- Modify: `varix/model/result_output.go`
- Modify: `varix/model/result_stage_outputs.go`
- Modify: `varix/model/parse.go`
- Test: `tests/compile/coverage_audit_test.go`

- [ ] **Step 1: Write failing audit tests**

Create tests for missing category detection and list-entity preservation.

```go
func TestCoverageAuditReportsLedgerCategoriesMissingFromBrief(t *testing.T) {
    ledger := Ledger{Items: []LedgerItem{
        {ID: "ledger-001", Category: "portfolio", Kind: "list", Claim: "Apple and American Express remain core holdings.", Entities: []string{"Apple", "American Express"}},
    }}
    brief := []BriefItem{
        {ID: "brief-001", Category: "capital", Claim: "Keep cash ready."},
    }

    audit := auditBriefCoverage(ledger, brief)
    if len(audit.MissingCategories) != 1 || audit.MissingCategories[0] != "portfolio" {
        t.Fatalf("missing categories = %#v, want portfolio", audit.MissingCategories)
    }
}
```

- [ ] **Step 2: Run focused audit tests and verify failure**

Run: `bash tests/go-test.sh ./compile -run CoverageAudit -count=1`

Expected: FAIL because audit types/functions do not exist.

- [ ] **Step 3: Add model type**

Add:

```go
type CoverageAudit struct {
    MissingCategories []string `json:"missingCategories,omitempty"`
    MissingListItems  []string `json:"missingListItems,omitempty"`
    OmittedLedgerIDs  []string `json:"omittedLedgerIds,omitempty"`
}
```

Persist it as `Output.CoverageAudit`.

- [ ] **Step 4: Implement audit builder**

Compare ledger categories and brief categories. Include omitted ledger IDs for
items not referenced by any brief `SourceIDs`. Do not fail compile; store the
diagnostic.

- [ ] **Step 5: Wire audit after brief**

Call audit after `stageBrief`, before render output is finalized.

Run: `bash tests/go-test.sh ./compile -count=1`.

Expected: PASS.

### Task 5: Make card projection expose compact audit evidence

**Files:**
- Modify: `varix/cmd/cli/compile_card_projection.go`
- Modify: `varix/cmd/cli/compile_card_format.go`
- Test: `tests/cmd/cli/compile_card_mainline_test.go`

- [ ] **Step 1: Add failing CLI projection test**

Add a test that a card with ledger omissions shows a compact diagnostic line
without dumping the full ledger.

```go
func TestFormatCompileCardShowsCoverageAuditSummary(t *testing.T) {
    card := compileCardProjection{
        Summary: "Meeting digest",
        KeyPoints: []string{"capital: Keep cash ready."},
        CoverageAudit: []string{"missing: portfolio"},
    }
    got := formatCompileCard(card, compileCardOptions{})
    if !strings.Contains(got, "Coverage audit") {
        t.Fatalf("card missing coverage audit:\n%s", got)
    }
    if !strings.Contains(got, "missing: portfolio") {
        t.Fatalf("card missing portfolio diagnostic:\n%s", got)
    }
}
```

- [ ] **Step 2: Run focused CLI test and verify failure**

Run: `bash tests/go-test.sh ./cmd/cli -run TestFormatCompileCardShowsCoverageAuditSummary -count=1`

Expected: FAIL because the projection has no audit summary.

- [ ] **Step 3: Add compact audit projection**

Add `CoverageAudit []string` to `compileCardProjection`.

Project only category-level diagnostics, not every omitted ledger item:

- `missing: portfolio`
- `missing: macro`
- `omitted ledger items: 7`

- [ ] **Step 4: Render audit after key points**

In `formatCompileCard`, render:

```text
Coverage audit
- missing: portfolio
```

Only render this section when diagnostics exist.

- [ ] **Step 5: Run CLI tests**

Run: `bash tests/go-test.sh ./cmd/cli -count=1`

Expected: PASS.

### Task 6: Compatibility and full verification

**Files:**
- Modify: `varix/storage/contentstore/sqlite_compiled_output.go`
- Modify: `tests/storage/contentstore/sqlite_store_test.go`
- Review: `varix/compile/preview.go`

- [ ] **Step 1: Add storage round-trip coverage**

Extend the compiled-output storage test with:

- one ledger item
- one coverage audit missing category
- one brief item referencing the ledger item

- [ ] **Step 2: Verify legacy fallback**

Add or update a render/preview test where `Output.SemanticUnits` exists but
`Output.Ledger` is empty. The expected behavior is:

- no parse error
- in-memory ledger adapter can build key points
- existing brief output remains preferred when present

- [ ] **Step 3: Run focused packages**

Run:

```bash
bash tests/go-test.sh ./model -count=1
bash tests/go-test.sh ./compile -count=1
bash tests/go-test.sh ./cmd/cli -count=1
bash tests/go-test.sh ./storage/contentstore -count=1
```

Expected: all PASS.

- [ ] **Step 4: Run full verification**

Run:

```bash
git diff --check
bash tests/go-test.sh ./... -count=1
```

Expected: both PASS.

- [ ] **Step 5: Manual sample run**

Run the Berkshire meeting sample:

```bash
cd varix
go run ./cmd/cli compile run --force --llm-cache read-through --url 'https://www.youtube.com/watch?v=4VwLwtiuxVQ' --timeout 30m > /tmp/varix-4v-ledger-compile.json
```

Expected:

- output contains non-empty `ledger.items`
- `ledger.items` includes portfolio/list facts when source/salience contains them
- `brief` includes portfolio when present in the ledger
- card output shows brief first and audit diagnostics only when omissions remain

- [ ] **Step 6: Commit with Lore protocol**

Commit implementation separately from the design docs.

Use a Lore message whose directive states that ledger is the shared internal
truth and mainline/brief are view projections.
