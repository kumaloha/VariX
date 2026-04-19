# Compile Redesign — Architecture Review

This document reviews the **current implementation** against the target design in
`docs/compile-form-function-implementation-plan.md`.

Use the plan doc as the target architecture and this review doc as the
current-state map for what is already landed vs what still remains to change.

## Scope reviewed

Main architecture surfaces reviewed:

- `varix/compile/client.go`
- `varix/compile/prompt_registry.go`
- `varix/compile/result.go`
- `varix/compile/parse.go`
- `varix/compile/verifier.go`
- `prompts/compile/system.tmpl`
- `prompts/compile/node_system.tmpl`
- `prompts/compile/node_challenge_system.tmpl`
- `prompts/compile/graph_system.tmpl`
- `varix/compile/prompt_test.go`
- `varix/compile/result_test.go`
- `varix/compile/form_function_regression_test.go`

---

## Review summary

### Verdict

The redesign is **architecturally landed but still quality-sensitive**.

What is already true:

- compile now starts from a unified generator -> challenge -> judge flow
- driver / target has a first-class top-level output contract
- compile now exposes first-class `transmission_paths`
- compile now exposes first-class `evidence_nodes` and `explanation_nodes`
- support vs transmission vs explanation distinctions are explicit in prompts,
  parsing, regression coverage, and the direct compile runtime

What is **not** yet true:

- target selection is not yet guaranteed to be optimal on every case
- driver/source selection can still drift in difficult articles
- retained paths and auxiliary layers still need quality tuning against gold
  cases

The current system is no longer the old node-first / edge-first overlay design,
but it still needs prompt-quality iteration to stabilize boundaries.

---

## Current architecture in code

Today `Client.Compile()` runs this high-level sequence:

1. **node extraction**
2. **node challenge**
3. **full graph / edge extraction**
4. **edge challenge**
5. **thesis extraction** from a causal projection built from `drives` edges
6. **verification** on the merged output

That orchestration is visible in `varix/compile/client.go`.

The important architectural implication is:

- the graph still comes first
- the top-level thesis still comes later
- the main chain is still inferred from the graph instead of being extracted as
  the primary object from the start

That means the current branch has improved the compile contract materially, but
it has **not** yet fully switched to the plan's direct-extraction architecture.

---

## Step-by-step mapping against the target plan

## Step 1 — Driver / target extraction

### What is landed

Step 1 is now a first-class stage in the direct compile pipeline.

The following pieces are already in place:

- unified generator / challenge / judge prompts now emit final top-level
  `drivers[]` / `targets[]` directly
- the direct compile runtime now commits to top-level endpoints before building
  the compatibility graph
- `ParseOutput()` / unified output parsing preserve `summary`, `drivers`, and
  `targets`
- `Output` / `ThesisOutput` carry top-level `drivers[]` and `targets[]`
- validation rejects blank driver / target entries when those fields are present
- regression tests now cover unified generator / challenge / judge prompt
  contracts and 3-call orchestration

### What is still missing

The main remaining risk is not stage existence but **boundary quality**.

Current gap vs the plan:

- target selection can still drift toward a too-narrow or too-broad market
  outcome
- driver selection can still confuse shared sources with strong mid-chain
  bridges
- gold-case quality still depends on prompt discipline, not only stage shape

### Review finding

Step 1 is now architecturally landed, but it remains quality-sensitive.

The most important rule is:
- **target first**
- target = final **market outcome**
- driver = shared source of retained transmission paths

---

## Step 2 — Transmission path extraction

### What is landed

Step 2 is now a first-class part of the unified direct compile flow.

The following pieces are now in place:

- unified generator returns first-class `transmission_paths`
- unified challenge audits missing bridges, fat steps, and misplaced
  support/explanation items
- unified judge returns the final retained transmission paths as part of the
  full compile package
- the compatibility graph is now downstream of first-class path extraction
  rather than the source of path ownership

### What is still missing

The remaining risk is boundary quality and path sharpness.

Current gap vs the plan:

- the retained path can still be phrased too broadly
- some cases may still compress two hops into one step
- some cases may still over-promote a side branch into the retained path set

### Review finding

Step 2 ownership is now landed.

The open issue has shifted from architecture to quality:
- does the path really represent the shortest sufficient causal spine?
- does it terminate at the right market outcome?

---

## Step 3 — Evidence / explanation extraction

### What is landed

Step 3 is now a first-class part of the unified direct compile flow.

The following pieces are now in place:

- unified generator emits `evidence_nodes` and `explanation_nodes`
- unified challenge audits misplaced auxiliary material
- unified judge regenerates the final full package with auxiliary material kept
  outside the retained causal spine
- compatibility graph attachment now happens after these fields are finalized

### What is still missing

The remaining gap is quality, not ownership.

Current gap vs the plan:

- evidence can still be too broad or too numerous
- explanation can still absorb material that should really be transmission
- some cases still need tighter auxiliary-layer pruning

### Review finding

Step 3 separation is now architecturally landed.

The remaining question is how strictly the prompts preserve:
- transmission = main causal spine
- evidence = why believe it
- explanation = how to interpret it

---

## Code-quality findings

### 1. Stage boundaries are cleaner than before

`varix/compile/prompt_registry.go` and `varix/compile/client.go` now keep node,
graph, thesis, and verification stages reasonably separated.

**Assessment:** good implementation discipline.

That makes the next redesign step safer because new dedicated step-1 / step-2 /
step-3 outputs can be introduced without rewriting the whole compile package in
one change.

### 2. The rollout remains additive instead of destructive

`varix/compile/result.go` and `varix/compile/parse.go` keep the migration
backward-compatible:

- legacy graph payloads still work
- top-level `drivers[]` / `targets[]` are additive
- transmission is introduced through form/function semantics rather than a
  flag-day schema replacement

**Assessment:** correct rollout strategy.

### 3. Verification is still downstream of the old critical path

`varix/compile/verifier.go` is downstream of the merged graph-oriented output.
That is fine for the current branch, but it also shows that the redesign has not
finished moving the system to a main-chain-first architecture.

**Assessment:** acceptable for now, but not the target end state.

### 4. `buildCausalProjection()` is the clearest sign of the remaining gap

`buildCausalProjection()` selects `drives` edges from the graph and passes that
projection into thesis extraction.

**Assessment:** this is a reasonable bridge implementation, but it is still the
old architecture in condensed form.

The main chain is still being **derived** from graph structure rather than being
**extracted first** as the primary compile object.

---

## Documentation guardrails

Docs should now describe the system this way:

- **landed now:** unified generator -> challenge -> judge compile pipeline
- **landed now:** first-class `drivers[]`, `targets[]`, `transmission_paths`,
  `evidence_nodes`, and `explanation_nodes`
- **landed now:** target-first / driver-source / spine-vs-auxiliary boundary
  rules in the unified prompt contract
- **still evolving:** output quality on hard cases and the exact boundary tuning

Do **not** describe the current branch as if it were still only a graph overlay
without first-class path / auxiliary outputs. That is no longer accurate.

---

## Recommended next implementation order

To stay aligned with the plan doc and avoid low-value churn:

1. add a first-class step-2 output contract for `transmission_paths`
2. make step 2 a dedicated stage before graph backfill or compatibility work
3. add first-class step-3 outputs for `evidence_nodes` and `explanation_nodes`
4. only then decide how much of the generic node/edge graph should remain as the
   canonical product vs a compatibility layer

This keeps the migration focused on the plan's core architecture instead of
expanding taxonomy or verifier detail prematurely.
