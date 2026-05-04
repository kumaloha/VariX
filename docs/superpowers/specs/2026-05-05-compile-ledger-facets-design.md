# Compile Ledger And Facets Design

## Goal

Make compile compatible with both analysis articles and meetings without using a
single lossy mainline path as the shared representation.

The internal contract should preserve source-level facts first, then derive
reader-facing views from that preserved inventory. Loss is allowed only at view
projection time, never in the shared compile state.

## Problem

The current compile flow can now produce `salience`, `brief`, `relations`,
`mainline`, and `render`. This is already better than treating meeting coverage
as a mainline problem, but the system still has a structural weakness:

```text
source -> salience -> brief/mainline -> render
```

`salience` is an importance layer. It ranks meaningful units. It is not a
coverage ledger. Meetings, shareholder Q&A, interviews, and policy briefings can
contain many independent agenda items that are not subordinate to one dominant
argument. Analysis articles, by contrast, often do have a dominant thesis and a
causal spine.

If both forms are forced through the same ranked semantic inventory, the system
can improve one form while hurting another:

- wider meeting budgets make article cards noisy
- tighter article budgets drop meeting agenda items
- dedupe can merge distinct meeting commitments such as culture continuity and
  succession planning
- render output can hide whether extraction preserved an item

## Core Decision

Introduce a first-class `Ledger` layer and move compatibility there.

```text
source
  -> ledger
  -> facets
  -> views
```

The ledger is the shared internal truth. Facets are form-specific derivations.
Views are reader-facing projections.

This means article and meeting compatibility is not:

```text
article + meeting -> mainline
```

It is:

```text
article + meeting -> ledger
ledger -> article facets
ledger -> meeting facets
facets -> selected views
```

## Layer Contracts

### Ledger

The ledger stores recoverable source facts. It is not a summary and does not
optimize for card length.

Required responsibilities:

- preserve claims with stable IDs
- preserve source evidence spans or quotes
- preserve named entities and named lists
- preserve numbers and units
- preserve commitments, boundaries, risks, disclosures, and decisions
- preserve questions and answers when the source has a meeting/interview shape
- preserve source IDs that connect derived views back to the ledger

The ledger may be produced from existing salience units at first, but the
contract must not depend on salience ranking. A later LLM ledger extractor can
replace the adapter without changing downstream facet/view contracts.

### Facets

Facets are structured interpretations of the ledger for a content form or
reader job.

Initial facets:

- `ArgumentFacet`: dominant thesis, driver/target, transmission, support. Used
  by analysis articles.
- `MeetingFacet`: agenda inventory, Q&A items, commitments, category coverage,
  list facts. Used by meetings, interviews, and long management discussions.
- `RiskFacet`: risks, underwriting boundaries, regulatory boundaries, litigation
  boundaries, operational constraints.
- `PortfolioFacet`: holdings, asset lists, transactions, allocation principles,
  concentration claims.

Facets can overlap. A Berkshire meeting can have both a meeting facet and an
argument facet; an article can have a portfolio facet if it discusses holdings.

### Views

Views are allowed to be lossy because they are presentation products.

Initial views:

- `Mainline`: compact narrative spine for analysis and reader intent.
- `Brief`: category-balanced digest for meetings and interview-like sources.
- `Card`: terminal/CLI render projection.
- `CoverageAudit`: diagnostic view that compares ledger inventory against the
  selected reader view.

Mainline must not be responsible for complete meeting coverage. Brief must not
be treated as the source of truth. Both should reference ledger IDs.

## Data Shape

The first implementation should add a compact ledger shape to `model.Output`:

```go
type LedgerItem struct {
    ID        string
    Kind      string
    Category  string
    Claim     string
    Entities  []string
    Numbers   []string
    Quote     string
    SourceIDs []string
    Salience  float64
}

type Ledger struct {
    Items []LedgerItem
}
```

Kinds are deliberately broad at first:

- `claim`
- `list`
- `number`
- `commitment`
- `boundary`
- `risk`
- `disclosure`
- `question`
- `answer`

Categories should reuse the emerging brief taxonomy:

- `capital`
- `buyback`
- `portfolio`
- `insurance`
- `ai`
- `energy`
- `operations`
- `culture`
- `succession`
- `governance`
- `macro`
- `shareholder`
- `international`

The implementation should avoid introducing a new dependency or database table
in the first pass. Persisting the ledger inside the existing compiled output JSON
is enough.

## Pipeline Shape

Target compile flow:

```text
coverage/classify/relations inputs
  -> salience
  -> ledger
  -> facets
  -> brief
  -> mainline
  -> render
```

In the first implementation:

- `stageLedger` builds ledger items from salience units and deterministic
  extractors.
- `stageBrief` reads from ledger instead of directly from salience.
- `stage3Mainline` continues to use a budgeted salience slice.
- render/card reads brief first for digest views and mainline first for analysis
  views.
- preview/rerender can rebuild ledger and brief from existing persisted salience
  if the older output does not have ledger data.

## Coverage Audit

Add a diagnostic audit after brief construction.

The audit should not fail production compile in the first pass. It should be
available to tests, previews, and debug output.

Initial checks:

- every non-low-salience ledger category present in a meeting-like source should
  appear in brief or be explicitly marked omitted by budget
- every `list` ledger item should retain at least one named entity in a digest
  view
- culture and succession should remain separate when both exist
- portfolio items should not disappear from shareholder meeting briefs
- AI classification should require an actual AI reference, not any word that
  merely contains the letters `ai`

## Compatibility

Existing compiled outputs without `ledger` remain valid.

Fallback rules:

- if `Output.Ledger` is empty and `SemanticUnits` exists, build an in-memory
  ledger adapter for rendering and preview
- if `Brief` exists, use it as the display projection
- if neither `Brief` nor `Ledger` exists, fall back to semantic key points

The legacy `semantic_units` JSON key remains unchanged because it is already a
public output field. Only the internal stage name has changed from
`semantic_coverage` to `salience`.

## Non-Goals

- Do not redesign storage tables in this pass.
- Do not introduce a second LLM call for ledger extraction until the deterministic
  adapter proves the contract.
- Do not force article cards to show meeting digest sections.
- Do not make mainline larger to cover meetings.
- Do not delete `semantic_units`; it remains the compact salience inventory.

## Acceptance Criteria

- A meeting source can preserve more agenda facts than the default card renders.
- Brief coverage is derived from ledger inventory, not salience top-N position.
- Article mainline behavior remains focused and budgeted.
- Portfolio/list facts survive into ledger and can be audited even when omitted
  from a compact view.
- Existing output JSON can be parsed and rendered without ledger data.
- Full Go test suite passes.

## Risks

- The first deterministic ledger adapter will still depend on what salience
  extracted. It removes downstream loss, not upstream extraction misses.
- Category taxonomy can become too finance-specific if future content forms are
  not reviewed.
- If the card output shows too much ledger detail by default, analysis articles
  may regress. Keep ledger diagnostic output separate from reader views.
