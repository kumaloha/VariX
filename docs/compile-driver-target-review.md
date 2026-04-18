# Compile Driver-Target Schema v1 — Review Findings

## Scope reviewed

This review covers the implementation landed in worker-1 commit:

- `315a6bb` — `Add driver-target overlays to compile outputs`

It also records the current dependency state for the evaluation / evidence lane
assigned to worker-2.

---

## Review summary

### Verdict

**No blocking findings** in the landed worker-1 implementation.

The implementation matches the PRD intent in the areas covered by task 3:

- additive `drivers[]` / `targets[]` output fields
- backward-compatible parsing when those fields are absent
- validation rejects blank driver / target entries when fields are present
- prompt contract explicitly normalizes article order into `driver -> target`
- reasoning graph semantics remain intact rather than being replaced

---

## What was reviewed

### 1. Output schema remains additive

`varix/compile/result.go` adds:

- `Drivers []string`
- `Targets []string`

next to the existing output fields instead of changing graph node/edge schema.

**Assessment:** aligns with the v1 decision to keep the reasoning graph intact and
layer the driver-target contract on top.

### 2. Validation behavior is appropriately narrow

`ValidateWithThresholds()` now rejects blank driver / target entries via
`validateStringListEntries()`.

**Assessment:** good scope control.

- It enforces the PRD rule against empty strings.
- It does **not** require every legacy payload to include `drivers` / `targets`.
- It avoids breaking graph-only outputs unnecessarily.

### 3. Parse layer preserves backward compatibility

`varix/compile/parse.go` unmarshals the new fields and normalizes whitespace via
`normalizeStringList()`.

**Assessment:** good compatibility posture.

- old payloads without `drivers` / `targets` still parse
- present payloads get trimmed values before validation

### 4. Prompt contract covers the right normalization rules

`prompts/compile/system.tmpl` now requires:

- top-level `drivers` and `targets`
- normalization to fixed `driver -> target`
- concrete mechanism-bearing drivers
- target wording as changes / results rather than bare asset nouns
- no blank placeholders

**Assessment:** matches the PRD and the batch-1 schema note closely.

### 5. Tests cover the key implementation seam

Worker-1 added tests for:

- valid additive schema acceptance
- blank driver rejection
- blank target rejection
- parse preservation of drivers / targets
- backward compatibility without those fields
- prompt contract strings for driver-target normalization

**Assessment:** the test coverage is targeted and proportional to the schema
change.

---

## Non-blocking observations

1. `normalizeStringList()` trims entries but preserves order and duplicates.
   - This is acceptable for v1.
   - No dedupe requirement exists in the PRD.

2. The prompt now requires `drivers` / `targets`, while validation keeps them
   optional for backward compatibility.
   - This asymmetry is intentional and correct for rollout safety.

3. The current implementation still uses free-text arrays rather than pairwise
   relations.
   - This is consistent with the v1 decision.
   - Do not upgrade to pair objects without evaluation evidence first.

---

## Remaining dependency / blocker state

Task 3 also expects final evidence aggregation after the implementation and test
lanes land.

### Current dependency status at review time

- worker-1: landed implementation commit `315a6bb`
- worker-2: evaluation / test lane still appears in progress and not yet landed

### Practical implication

**Task 3 is operationally waiting on worker-2's evaluation evidence** before a
full cross-lane closeout can be claimed.

This is not a blocker on code quality for worker-1's implementation.
It is a blocker on the final evidence-aggregation portion of the review/docs
lane.

---

## Recommended closeout checklist

When worker-2 lands, task 3 should aggregate these pieces into the final team
handoff:

1. implementation commit hash(es)
2. compile test evidence
3. CLI test evidence
4. gold dataset shape evidence
5. any batch-1 evaluation report evidence for separate:
   - summary
   - node recall / typing
   - reasoning edges
   - drivers
   - targets

---

## Reviewer conclusion

The current implementation slice reviewed from worker-1 is consistent with the
compile driver-target v1 contract and introduces no review-blocking issues.

The only remaining dependency for task-3 closeout is worker-2's pending
evaluation/evidence lane.
