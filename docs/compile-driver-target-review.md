# Compile Driver-Target Schema baseline — Review Findings

## Scope reviewed

This review covers the landed implementation and test/evaluation commits for the
compile driver-target schema lane:

- `315a6bb` — `Add driver-target overlays to compile outputs`
- `5126ed0` — `Make gold evaluation tests work from team worktrees`

It records the final task-3 review verdict and aggregates the concrete evidence
reported by the implementation and test lanes.

---

## Review summary

### Verdict

**No blocking findings** in the landed worker-1 / worker-2 changes.

The delivered work matches the PRD intent in the areas covered by task 3:

- additive `drivers[]` / `targets[]` output fields
- backward-compatible parsing when those fields are absent
- validation rejects blank driver / target entries when fields are present
- prompt contract explicitly normalizes article order into `driver -> target`
- reasoning graph semantics remain intact rather than being replaced
- gold evaluation tests run from nested OMX team worktrees as well as normal
  repo layouts

---

## What was reviewed

### 1. Output schema remains additive

`varix/compile/result.go` adds:

- `Drivers []string`
- `Targets []string`

next to the existing output fields instead of changing graph node/edge schema.

**Assessment:** aligns with the baseline decision to keep the reasoning graph intact and
layer the driver-target contract on top.

### 2. Validation behavior is appropriately narrow

`ValidateWithThresholds()` rejects blank driver / target entries via
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

### 5. Evaluation tests are runtime-layout tolerant

`tests/eval/gold_eval_test.go` now walks upward from the package working
folder until the repo-level gold dataset is found.

**Assessment:** good operational hardening.

- fixes the failing assumption that the dataset always lives exactly two levels
  above the compile package
- keeps evaluation evidence runnable from nested OMX team worktrees
- avoids brittle hardcoded worker-path depth assumptions

### 6. Tests cover the key implementation seam

Worker-1 added tests for:

- valid additive schema acceptance
- blank driver rejection
- blank target rejection
- parse preservation of drivers / targets
- backward compatibility without those fields
- prompt contract strings for driver-target normalization

Worker-2 kept the gold evaluation surface runnable and validated report
structure across:

- `summary`
- `node recall / node typing`
- `reasoning edges`
- `drivers`
- `targets`

**Assessment:** the combined test coverage is targeted and proportional to the
schema change.

---

## Non-blocking observations

1. `normalizeStringList()` trims entries but preserves order and duplicates.
   - This is acceptable for baseline.
   - No dedupe requirement exists in the PRD.

2. The prompt requires `drivers` / `targets`, while validation keeps them
   optional for backward compatibility.
   - This asymmetry is intentional and correct for rollout safety.

3. The current implementation uses free-text arrays rather than pairwise
   relations.
   - This is consistent with the baseline decision.
   - Do not upgrade to pair objects without evaluation evidence first.

---

## Aggregated evidence

### Landed commits

- implementation: `315a6bb`
- test/evaluation: `5126ed0`
- docs/contract: `69c5312`
- review findings: `8f7ff16`

### Verification evidence collected

From task-1 result:

- PASS `go test ./compile/...`
- PASS `go test ./cmd/cli/...`
- PASS `go test ./...`
- PASS gold dataset shape validation on `eval/gold/compile-gold-batch1-baseline.json`
  with 9 samples and non-empty drivers / targets

From task-2 result:

- PASS `go test ./compile/...`
- PASS `./tests/go-test.sh ./eval -run 'TestLoadGoldDatasetBatch1|TestBuildGoldEvaluationReportBatch1|TestGoldDatasetValidateRejectsBlankDriverOrTarget' -v`
- PASS `go test ./cmd/cli/...`
- PASS Python JSON load of `eval/gold/compile-gold-batch1-baseline.json`
- PASS evaluation report structure with distinct sections for:
  - `summary`
  - `node_recall_type`
  - `reasoning_edges`
  - `drivers`
  - `targets`

From task-3 lane:

- PASS `git diff --check` on docs/review changes
- PASS review of worker-1 additive schema implementation
- PASS review of worker-2 worktree-tolerant evaluation test fix

---

## Reviewer conclusion

The compile driver-target baseline work is review-complete.

The schema remains additive to the existing reasoning graph, prompt and parser
behavior align with the PRD, blank-entry validation is enforced, and the gold
evaluation surface is runnable from OMX team worktrees. No review-blocking
issues were found in the landed implementation or test/evaluation commits.
