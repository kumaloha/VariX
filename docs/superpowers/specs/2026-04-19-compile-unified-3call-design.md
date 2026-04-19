# Compile unified 3-call redesign

## Goal

Reduce compile generation latency by replacing the current 9-call direct pipeline
with a 3-call pipeline while preserving the existing external output schema.

Current direct pipeline:
1. driver / target generator + challenge + judge
2. transmission path generator + challenge + judge
3. evidence / explanation generator + challenge + judge

Target direct pipeline:
1. unified generator
2. unified challenge
3. unified judge

The final output schema remains unchanged:
- `summary`
- `drivers`
- `targets`
- `transmission_paths`
- `evidence_nodes`
- `explanation_nodes`
- `details`
- `topics`
- `confidence`

## Non-goals

- No public schema redesign
- No verifier redesign
- No persistence contract changes
- No CLI contract changes
- No change to downstream compatibility graph fields beyond consuming the same
  final top-level compile output

## Why this change

The current 9-call pipeline is too slow in production-like runs. Recent reruns of
G01-G03 required roughly 528s, 578s, and 581s respectively, with latency spread
across all three layers rather than one isolated stuck stage.

The current split also creates consistency problems:
- driver / target can drift from transmission paths
- transmission challenge can over-expand because it does not adjudicate against
  the full auxiliary layer at the same time
- summary quality degrades because it is assembled from path output rather than
  judged together with the full thesis package

A unified 3-call design improves both latency and global coherence by forcing
all three semantic layers to be generated, challenged, and judged together.

## Canonical semantic layers

The compile output still has three semantic layers:

1. **Endpoints** — `drivers` and `targets`
2. **Main causal spine** — `transmission_paths`
3. **Auxiliary layer** — `evidence_nodes` and `explanation_nodes`

The prompts must make the boundaries between these layers explicit and stable.

## Boundary rules

### Driver

A driver is an upstream force in the article's dominant thesis.

A valid driver must satisfy all of the following:
- it is upstream of the chosen top-level target
- removing it would materially weaken the dominant thesis
- it is part of the article's main causal story, not just background or side
  commentary

A driver is **not**:
- generic background macro context
- a side forecast
- a restatement of a transmission step
- a restatement of the target

### Target

A target is the article's true downstream conclusion or market implication.

A valid target must satisfy all of the following:
- it is an endpoint the author wants the reader to believe
- it belongs to the dominant thesis rather than a side branch
- it is downstream of at least one retained driver

A target is **not**:
- a mid-chain transmission step
- auxiliary interpretation or framing
- a near-duplicate paraphrase of another retained target

### Transmission path

A transmission path is the shortest sufficient world-state causal chain that
connects a chosen driver to a chosen target.

A valid transmission step must satisfy all of the following:
- it describes how the world changes, not merely why we believe something
- it is needed to keep the driver -> target causal spine intelligible
- it belongs on the main spine rather than the auxiliary layer

A transmission step is **not**:
- supporting proof or observed evidence
- framework or interpretive commentary
- a duplicated rewrite of the driver or target
- a fat merged step that really contains multiple causal hops

Decision rule:
- if removing the step breaks the causal spine, it belongs in transmission
- if removing the step only weakens confidence but not the spine, it belongs in
  evidence or explanation

### Evidence

Evidence is supporting proof, observation, or factual support for the dominant
thesis or a retained transmission step.

Evidence belongs outside the main spine.

Evidence is **not**:
- a causal bridge step
- a theory or interpretive frame
- a duplicate of the final claim

Short rule:
- evidence answers **why believe this**
- transmission answers **how it propagates**

### Explanation

Explanation is framework, interpretation, or theory that helps the reader
understand the thesis.

Explanation belongs outside the main spine.

Explanation is **not**:
- direct supporting evidence
- a causal bridge step
- a duplicate of the top-level judgment

Short rule:
- explanation answers **how to interpret this**
- not **what caused it** and not **what proves it**

### Priority order for ambiguous items

When one item could fit multiple layers, prompts must apply this ordering:

1. If it is required to preserve the main causal spine, classify it as
   transmission.
2. Else if it mainly proves a retained endpoint or path step, classify it as
   evidence.
3. Else if it mainly frames or interprets the thesis, classify it as
   explanation.
4. Otherwise omit it.

This ordering is intended to suppress the most common failure modes:
- support mistaken for causation
- explanation mistaken for mechanism
- side commentary promoted to driver / target

## Prompt design

### Unified generator

The unified generator produces one full JSON object containing all public output
fields.

Responsibilities:
1. identify the dominant thesis
2. choose the final driver / target sets for that thesis
3. build the shortest sufficient transmission paths
4. attach auxiliary evidence and explanation outside the main spine
5. write a summary consistent with the same thesis package

Generator rules:
- all top-level fields must describe the same dominant thesis
- side commentary must not be promoted to retained drivers / targets
- transmission paths must remain minimal and sufficient
- evidence and explanation must stay outside the causal spine
- avoid near-duplicate paraphrases across drivers, targets, and path steps

### Unified challenge

The unified challenger does not regenerate the whole answer from scratch. It
returns targeted corrections covering the entire package.

Challenge responsibilities:
1. endpoint audit
   - missing drivers / targets
   - side commentary incorrectly promoted
   - duplicated or near-duplicated retained endpoints
2. path audit
   - missing transmission bridges
   - fat transmission steps that should split
   - support / explanation incorrectly placed on the main spine
3. auxiliary audit
   - missing evidence
   - missing explanation
   - items that belong in transmission rather than auxiliary, or vice versa
4. consistency audit
   - summary inconsistency with retained endpoints and main spine

The challenge output should be incremental and correction-oriented, not a second
full final answer.

### Unified judge

The judge is both the final adjudicator and the final generator.

The judge must:
- read the generator output
- read the challenge corrections
- decide which corrections to accept or reject
- produce the final full public output object

The judge is therefore not a narrow yes/no referee. It is the final synthesizer
that emits the single authoritative output payload.

Judge rules:
- final output must be complete and schema-valid
- final summary, endpoints, paths, and auxiliary layer must align to one
  dominant thesis
- if challenge findings are rejected, the judge must still return a coherent
  final package rather than partial deltas

## Public schema contract

The final public output remains the current contract:

```json
{
  "summary": "...",
  "drivers": ["..."],
  "targets": ["..."],
  "transmission_paths": [
    {"driver": "...", "target": "...", "steps": ["..."]}
  ],
  "evidence_nodes": ["..."],
  "explanation_nodes": ["..."],
  "details": {},
  "topics": ["..."],
  "confidence": "medium"
}
```

Internal challenge payloads may use a different correction-oriented schema, but
that schema must remain internal to the orchestration layer.

## Implementation shape

### Prompt templates

Introduce unified prompt templates:
- `compile/unified_generator_system.tmpl`
- `compile/unified_generator_user.tmpl`
- `compile/unified_challenge_system.tmpl`
- `compile/unified_challenge_user.tmpl`
- `compile/unified_judge_system.tmpl`
- `compile/unified_judge_user.tmpl`

The wording must stay aligned with existing compile docs and prompt rules,
especially around:
- normalized `driver -> target`
- shortest sufficient transmission path
- support vs transmission vs explanation boundaries
- one dominant thesis across the whole output

### Prompt registry

Add unified render helpers while preserving old helpers until the migration is
complete. The direct pipeline should switch to the unified helpers.

### Client orchestration

Replace the current direct pipeline sequence:
- `compileDriverTarget`
- `compileTransmissionPaths`
- `compileEvidenceExplanation`

with:
- `compileUnifiedGenerate`
- `compileUnifiedChallenge`
- `compileUnifiedJudge`

The judge returns the final full compile payload, which then feeds the existing
merge/output path without changing external field names.

### Parsing

Add parser support for:
- unified full output
- unified challenge corrections

The final judge output must validate against the existing final-output
validation rules.

## Validation and tests

Follow test-first implementation.

Required regression coverage:
1. direct compile uses exactly 3 compile-stage LLM calls instead of 9
2. unified judge returns the full public output payload
3. generator/challenge/judge prompt registry paths resolve correctly
4. summary remains consistent with retained drivers, targets, and transmission
   paths
5. challenge can flag misplaced evidence/explanation items that were incorrectly
   placed in the transmission spine
6. final parse/output path preserves the current public schema

Quality-oriented checks to keep or add:
- no blank driver / target entries
- no empty transmission path steps
- evidence/explanation remain optional individually but not both empty when the
  article clearly contains auxiliary material
- final outputs remain backward-compatible with downstream consumers

## Risks

1. **Larger single-call payloads**
   - The prompts must stay disciplined to avoid drift and verbosity.
2. **Challenge schema complexity**
   - The correction format must stay narrow enough that judge integration remains
     deterministic.
3. **Boundary drift under compression**
   - The unified prompts must explicitly restate the priority order for
     transmission vs evidence vs explanation.
4. **Prompt/doc mismatch**
   - Existing compile docs and prompt contracts must remain the source of truth.

## Recommendation

Implement the 3-call unified pipeline now, with public schema preserved and
boundary rules made stricter in prompt language than they are today.
