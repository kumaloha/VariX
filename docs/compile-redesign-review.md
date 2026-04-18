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

The redesign is **partially landed, not fully landed**.

What is already true:

- driver / target has a real top-level output contract
- transmission is a first-class **node function** in the graph schema
- support vs transmission vs claim distinctions are now explicit in prompts,
  parsing, and regression coverage

What is **not** yet true:

- compile does **not** start from driver / target extraction as its primary
  architecture
- compile does **not** expose first-class `transmission_paths`
- compile does **not** expose first-class `evidence_nodes` and
  `explanation_nodes`
- the planned generator -> challenge -> judge flow does **not** exist yet for
  step 1 / step 2 / step 3 as dedicated compile stages

The current system is still primarily a **node-first / edge-first pipeline with
an added thesis overlay**, not the final three-step redesign described in the
plan doc.

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

Step 1 is the most advanced part of the redesign.

The following pieces are already in place:

- `prompts/compile/system.tmpl` defines the top-level driver / target thesis
  contract
- `promptRegistry.buildThesisInstruction()` and
  `promptRegistry.buildThesisPrompt()` make thesis extraction a separate stage
- `ParseThesisOutput()` parses `summary`, `drivers`, and `targets`
- `Output` / `ThesisOutput` carry top-level `drivers[]` and `targets[]`
- validation rejects blank driver / target entries when those fields are present
- tests in `prompt_test.go`, `result_test.go`, and
  `form_function_regression_test.go` lock the main prompt and parser behavior

### What is still missing

Step 1 is still **late-bound** rather than architecture-first.

Current gap vs the plan:

- thesis extraction happens **after** node extraction and graph construction
- there is no dedicated compile-stage **challenge** for driver / target recall
- there is no dedicated compile-stage **judge** that finalizes driver / target
  independently of the later graph merge
- driver / target is still an overlay on top of the graph pipeline rather than
  the first object the compile flow commits to

### Review finding

The current driver / target implementation is a good additive rollout layer, but
it should not be documented as if step 1 already replaced the old architecture.
It has not.

---

## Step 2 — Transmission path extraction

### What is landed

The codebase has useful building blocks for step 2:

- node `function=transmission` is first-class in `varix/compile/result.go`
- prompt guidance strongly protects support -> transmission -> claim separation
- graph extraction uses `drives` for market transmission relations
- node and edge challenge prompts already try to recover missing bridge
  transmission nodes

### What is still missing

Step 2 is **not** yet implemented as a dedicated transmission-path stage.

Current gap vs the plan:

- there is no top-level `transmission_paths` output contract
- there is no step-2 generator -> challenge -> judge loop
- transmission is still represented indirectly through graph nodes and edges
- `buildCausalProjection()` reconstructs the main chain from positive edges
  after graph extraction instead of extracting the path directly

### Review finding

This is the main architecture gap.

The current branch improved transmission **representation**, but not yet
transmission-path **ownership**.

In the target design, step 2 owns the causal spine directly.
In the current design, the causal spine is still assembled bottom-up from graph
artifacts.

---

## Step 3 — Evidence / explanation extraction

### What is landed

The current graph model already contains pieces that belong to the future
auxiliary layer:

- support nodes
- `substantiates` edges
- `explains` edges
- verification outputs for factual support
- `details.caveats` for bounded free-form context

### What is still missing

Step 3 is also **not** yet implemented as a dedicated auxiliary-layer stage.

Current gap vs the plan:

- there is no first-class `evidence_nodes` output field
- there is no first-class `explanation_nodes` output field
- evidence and explanation are still interleaved with the main graph instead of
  being extracted only after the main chain is fixed
- there is no step-3 generator -> challenge -> judge loop that explicitly keeps
  evidence / explanation outside the causal spine

### Review finding

The current branch has the raw ingredients for evidence / explanation, but it
has not yet separated them into the plan's explicit auxiliary layer.

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

Until the next implementation round lands, docs should describe the system this
way:

- **landed now:** graph extraction plus a driver / target thesis overlay
- **partially landed:** transmission as a first-class role in graph semantics
- **not landed yet:** dedicated transmission-path extraction stage
- **not landed yet:** dedicated evidence / explanation extraction stage
- **not landed yet:** full generator -> challenge -> judge pipeline for each of
  the three target steps

Do **not** describe the current branch as if the compile pipeline already starts
with driver / target and only later fills transmission and support. That is the
plan target, not the current runtime architecture.

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
