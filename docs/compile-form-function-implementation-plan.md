# Compile Redesign Plan

## Status

This document replaces the earlier compatibility-oriented redesign notes.

Current implementation review lives in `docs/compile-redesign-review.md`.
That review documents the landed state: top-level driver / target output is in
place, but the runtime compile pipeline is still node-first / edge-first and
does not yet expose first-class `transmission_paths`, `evidence_nodes`, or
`explanation_nodes`.
The target design is no longer "keep the old node kinds and layer new axes on top".
The target design is now a direct extraction pipeline centered on:

1. driver / target
2. transmission path
3. evidence / explanation

No code changes should proceed beyond this direction unless they align with this document.

---

## Core decision

The old node-first / edge-first design has become too indirect.
It encourages the system to overfit local node relations before it has identified the main market structure.

The new design starts from the main structure first, then fills in the middle and supporting layers.

---

## New pipeline

## Step 1 — Driver/Target extraction

### Objective
Identify the article's main drivers and main targets directly.
Do not begin from generic node extraction.

### Workflow
- generator
- challenge
- judge

### Output
```json
{
  "drivers": ["..."],
  "targets": ["..."]
}
```

### Generator responsibilities
- propose the main drivers
- propose the main targets
- normalize to driver -> target regardless of article writing order

### Challenge responsibilities
- check whether generator missed a driver
- check whether generator missed a target
- check whether one driver/target entry improperly merges multiple independent items
- challenge analytical center drift (for example, latching onto a side thesis instead of the main thesis)

### Judge responsibilities
- select the final driver set
- select the final target set
- reject unsupported side-thesis promotion

### Acceptance criterion
At the end of step 1, the system should already know:
- what is driving
- what is being driven

without relying on a generic node graph to infer that later.

---

## Step 2 — Transmission path extraction

### Objective
Given the final driver/target pair(s), extract the middle transmission path directly.
This step owns the causal spine.

### Workflow
- generator
- challenge
- judge

### Output
```json
{
  "transmission_paths": [
    {
      "driver": "...",
      "target": "...",
      "steps": ["...", "...", "..."]
    }
  ]
}
```

### Generator responsibilities
- propose the shortest sufficient path from driver to target
- include bridge mechanisms that are necessary to make the relation intelligible
- avoid padding the path with proof-only material

### Challenge responsibilities
- detect missing bridge steps
- detect over-compressed steps
- detect when a path step is actually two or more separate transmission steps
- detect when a proposed step is not transmission but merely support or explanation

### Judge responsibilities
- finalize the path
- keep only the transmission chain needed to connect driver to target
- reject side chains that do not belong to the main causal spine

### Acceptance criterion
At the end of step 2, the system should have a causal spine that can stand on its own.
This is the main chain.

---

## Step 3 — Evidence / explanation extraction

### Objective
Only after driver/target and transmission path are fixed, extract the auxiliary layer.
This step owns proof and framing.

### Workflow
- generator
- challenge
- judge

### Output
```json
{
  "evidence_nodes": ["..."],
  "explanation_nodes": ["..."]
}
```

### Generator responsibilities
- extract supporting evidence for the chosen driver/target/path
- extract explanation/framework material that helps interpret the path

### Challenge responsibilities
- detect missing evidence
- detect missing explanation/context
- detect evidence that actually belongs in the main transmission path instead of the auxiliary layer
- detect explanations that are really just repeated judgments

### Judge responsibilities
- finalize support layer vs explanation layer
- keep evidence and explanation outside the main causal spine

### Acceptance criterion
At the end of step 3, the system has:
- main chain = driver / target / transmission path
- auxiliary layer = evidence / explanation

---

## Resulting structure

The final compile product should conceptually separate:

### Main chain
- drivers
- targets
- transmission paths

### Auxiliary layer
- evidence
- explanation

The main chain should not be reconstructed later from generic nodes and edges.
It should be extracted directly.

---

## Why this replaces the old design

The old design tried to:
- extract generic nodes first
- add edges next
- derive the main thesis afterward

That approach is too indirect.
It often succeeds on local relations but misses the article's true main structure.

The new design does the opposite:
- identify the main thesis first
- identify the main path second
- attach support and explanation last

This better matches the actual review goal.

---

## What is explicitly removed

The target design no longer treats the old node kinds as the primary abstraction:
- `事实`
- `显式条件`
- `隐含条件`
- `机制`
- `结论`
- `预测`

Those may still exist temporarily in code during migration, but they are **not** the intended design target.
The intended design target is the three-step extraction pipeline above.

---

## Practical implementation order

1. implement step 1 prompts + outputs
2. implement step 2 prompts + outputs
3. implement step 3 prompts + outputs
4. only then decide whether any generic node/edge compatibility layer is still needed

---

## Evaluation order

### First evaluate
- driver correctness
- target correctness

### Then evaluate
- transmission path completeness and correctness

### Then evaluate
- evidence quality
- explanation quality

This order is intentional.
The main thesis must be right before the support layer is judged.

---

## Constraint

Do not drift back into bottom-up node-graph design during implementation.
If a proposed change starts from generic node taxonomy instead of the three-step extraction flow, it is off-plan.
