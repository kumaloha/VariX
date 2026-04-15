# Memory v2 Heuristic Backlog

This document captures the highest-value heuristic improvements still worth
doing after the thesis-first memory review system became feature-complete.

The current system is already usable and reviewable. The items below are
strictly about improving semantic stability and product quality.

---

## Priority 1 — Reduce bridge-node false merges (Partially improved)

### Problem
`buildCandidateTheses` still relies on pairwise links + connected components.
A single bridge node can cause two adjacent-but-different cognition groups to
collapse into one thesis.

### Why it matters
This is the highest-risk remaining error because a false merge corrupts:
- topic label
- card logic
- headline abstraction
- compare output

### Target improvement
- penalize weak bridge edges
- require at least one strong shared signal before two subgraphs merge
- distinguish “same object mention” from “same cognitive question”

### Success signal
Real review sets no longer produce giant mixed theses from loosely connected
macro material.

### Current progress
- same-theme-different-question separation is now covered at organizer level
- broad debt/object overmerge heuristics have been narrowed
- some same-source singleton nodes now reattach more safely
- remaining risk is still transitive bridge-node overmerge across longer chains

---

## Priority 2 — Recover cross-role same-topic grouping more systematically (Partially improved)

### Problem
Some same-topic content is still only merged when a narrow shared-object rule
or same-source structural rule happens to fire.

Examples:
- fact in one source + condition in another
- condition in one source + conclusion in another
- mechanism in one source + prediction in another

### Why it matters
If these miss each other, the system fragments one cognition into multiple
weak theses and loses abstraction quality.

### Target improvement
- introduce a clearer “same cognitive question” signal
- allow cross-role grouping when object + directional logic align
- keep this narrow enough not to reopen broad false merges

### Success signal
Same-topic cross-source narratives merge more consistently without reviving
theme-wide overmerge.

### Current progress
- cross-source fact↔conclusion and condition↔conclusion same-object cases are now covered
- remaining gaps are broader cross-role families beyond the currently tested patterns

---

## Priority 3 — Improve conflict-side evidence selection beyond shallow graph-local ranking (Partially improved)

### Problem
`Why A / Why B` is now graph-backed and ordered, but still shallow:
- direct support first
- one extra upstream layer

This is better than raw local snippets, but it is not yet “best evidence.”

### Why it matters
Conflict cards are now product-facing. Weak side explanations reduce trust in
the contradiction surface.

### Target improvement
- score candidate evidence by closeness + node kind + distinctiveness
- prefer strongest supporting facts over repetitive nearby paraphrases
- avoid bloated why lists when a single fact is enough

### Success signal
Conflict cards read like “here is the strongest case for each side,” not “here
are the first few nearby nodes.”

### Current progress
- side explanations are graph-backed
- fact-first ordering is in place
- one extra upstream layer can now be included behind direct supports
- remaining work is stronger scoring of the *best* support rather than local graph order

---

## Priority 4 — Expand headline abstraction families gradually and test-first (Partially improved)

### Problem
Headline abstraction is now clearly better, but still pattern-driven and sparse.
Uncovered families fall back to flatter driver/outcome phrasing.

### Why it matters
The first layer is the product’s “alpha” surface. Missing abstraction patterns
make some cards feel noticeably weaker than others.

### Target improvement
Add narrowly-scoped abstraction families only when:
1. a real review case proves the need
2. a unit test locks the wording family
3. an organizer-level regression confirms the final output

### Candidate next families
- labor / consumption slowdown
- inflation persistence / policy constraint
- liquidity squeeze / forced deleveraging
- supply shock / energy passthrough

### Success signal
More real-user conclusions read like higher-order cognition instead of sentence
compression.

### Current progress
- debt/purchasing-power
- petrodollar/private-credit
- oil-shock
- bank-resilience
all now have explicit abstraction patterns plus organizer-level protection for key cases

---

## Priority 5 — Strengthen compare output from “summary juxtaposition” toward “structural diff” (Not started)

### Problem
`global-compare` is already useful, but it is still essentially:
- v1 summary block
- v2 summary block

It does not yet explain:
- what merged
- what split
- what became a conflict
- what became a higher-order thesis

### Why it matters
As the system gets smarter, reviewers will want faster diagnosis of whether a
heuristic change helped or hurt.

### Target improvement
Possible incremental steps:
- annotate v2 entries with originating thesis ids
- mark items as “newly abstracted”, “still fragmented”, or “blocked by conflict”
- add optional structural diff bullets under each compare section

### Success signal
Reviewers can tell not only that v1 and v2 differ, but *how* they differ.

---

## Suggested execution order

1. Bridge-node false merge control
2. Cross-role same-topic grouping
3. Conflict-side evidence ranking depth
4. New headline families
5. Structural compare diff

---

## Guardrails

- Every heuristic change should come with at least one focused unit test.
- Any real-user pattern fix should also get one organizer-level regression.
- Do not widen broad theme/object rules just to make one case pass.
- Prefer narrow, named heuristics over opaque “magic similarity” behavior.
