# Compile Driver-Target Schema baseline

## Purpose

This document defines the baseline contract for adding a normalized driver-target
layer to VariX compile outputs.

The driver-target layer is **additive**. It does not replace the existing
reasoning graph.

The graph still carries:

- facts
- explicit conditions
- implicit conditions / mechanisms
- conclusions
- predictions
- local reasoning edges

The driver-target layer exists to normalize market logic into a stable
`driver -> target` view even when the source article presents the narrative in
reverse order.

---

## Why this exists

Batch-1 gold review showed that the reasoning graph alone is not sufficient for
财经分析.

The graph can preserve local reasoning structure, but it does not always make
the article's main market-moving relation explicit.

Typical failure mode:

1. the article states the market outcome first
2. the article explains the real cause later
3. the graph captures the local chain
4. downstream evaluation still needs a stable "what moved what" overlay

The driver-target layer fixes that gap without redesigning node taxonomy.

---

## Core invariants

1. **Additive only**
   - `drivers[]` and `targets[]` sit alongside `summary`, `graph`, `details`,
     `topics`, `confidence`, and `verification`
   - the existing graph remains required and authoritative for local reasoning

2. **Normalized direction**
   - extraction always normalizes to `driver -> target`
   - author writing order does not matter
   - a target-first article still compiles into driver-first overlay fields

3. **Drivers are concrete forces**
   - a driver should describe the mechanism or force moving markets
   - a retained driver should be the shared source of the retained transmission
     paths, not a mid-chain bridge
   - vague placeholders like "many risks exist" are not acceptable drivers

4. **Targets are market outcomes**
   - a target should describe the final market outcome of the thesis
   - the target may be a narrow pricing move or a broader trading /
     positioning / market-state outcome
   - target entries should avoid raw object nouns alone such as `housing` or
     `stocks`
   - target entries should not be background context, pure interpretation, or a
     mid-chain bridge

5. **Free-text baseline**
   - baseline uses free-text `drivers[]` and `targets[]`
   - baseline does **not** require explicit pair objects or node-id references

---

## Output shape

Compile outputs should support this additive shape:

```json
{
  "summary": "...",
  "drivers": ["concrete force 1", "concrete force 2"],
  "targets": ["repricing / flow / pressure change 1"],
  "graph": {"nodes": [], "edges": []},
  "details": {"caveats": []},
  "topics": ["..."],
  "confidence": "medium",
  "verification": {}
}
```

### Validation expectations

- missing `drivers` / `targets` remains parse-compatible for older payloads
- when present, each entry must be non-empty after trimming whitespace
- the graph validation contract remains unchanged
- no new node kinds are introduced for baseline

---

## Normalization rules

### Drivers

Prefer entries like:

- `US growth narrative remains dominant over political-risk pricing`
- `Middle East conflict weakens confidence in the US security umbrella`
- `Private-credit marking opacity amplifies funding fragility`

Avoid entries like:

- `many risks exist`
- `things are uncertain`
- `the market is complicated`

### Targets

Prefer entries like:

- `continued inflow into US assets`
- `credit spreads widen`
- `housing prices reprice lower`
- `household de-risking accelerates`
- `volatility in debt-priced assets rises`
- `sell America trade does not materialize`
- `US assets remain in an overweight / unhedged positioning regime`

Avoid entries like:

- `US assets`
- `housing`
- `private credit`
- `stocks`
- `inflation remains high`
- `the market is nervous`
- `growth narrative dominates political risk` (bridge / mechanism rather than
  final outcome)

### Narrative-order rule

If an article presents:

- target first, then explanation later

compile should still output:

- `drivers[]` = explanatory forces
- `targets[]` = resulting changes

This keeps evaluation stable across writing styles.

### Target-width rule

Targets do **not** need to be maximally narrow.

Some articles end in:
- an explicit repricing / yield / FX / spread move
- a broader trading / positioning / market-state outcome

Both are acceptable if they are the thesis' true final market outcome.

### Driver-source rule

When a compile output also carries first-class transmission paths, the retained
drivers should correspond to the shared sources of those retained paths.

---

## Evaluation contract

Gold evaluation should score the driver-target layer separately from the
reasoning graph.

Minimum report structure:

1. `summary`
2. `node recall / node typing`
3. `reasoning edges`
4. `drivers`
5. `targets`

This separation matters because:

- a compile output may have a mostly correct graph but weak normalized drivers
- a compile output may capture the right targets while missing graph detail
- downstream review should distinguish those failure modes instead of blending
  them into one score

---

## Gold dataset contract

Current benchmark:

- `eval/gold/compile-gold-batch1-baseline.json`

Current expected shape:

- JSON document with `samples`
- 9 batch-1 samples
- each sample contains:
  - `summary`
  - non-empty `drivers[]`
  - non-empty `targets[]`

The batch stays representable without adding new reasoning-graph node kinds.

---

## Implementation review checklist

Any implementation of this contract should preserve these code-level
invariants:

- `varix/compile/result.go`
  - add fields without weakening graph validation
  - reject blank driver/target entries when fields are present
- `varix/compile/parse.go`
  - remain backward compatible with payloads that omit the new fields
- `prompts/compile/system.tmpl`
  - instruct normalized `driver -> target` extraction
  - require targets to be phrased as changes
  - require drivers to be concrete mechanisms
- tests
  - cover validation acceptance / rejection
  - cover parse roundtrip + backward compatibility
  - cover prompt contract strings
  - validate batch-1 gold dataset shape

---

## Operator verification commands

```bash
cd varix && go test ./compile/...
cd varix && go test ./cmd/cli/...
python3 - <<'PY'
import json
from pathlib import Path
path = Path('../eval/gold/compile-gold-batch1-baseline.json')
data = json.loads(path.read_text())
assert len(data['samples']) == 9
assert all(item.get('summary', '').strip() for item in data['samples'])
assert all(any(s.strip() for s in item.get('drivers', [])) for item in data['samples'])
assert all(any(s.strip() for s in item.get('targets', [])) for item in data['samples'])
print('gold dataset shape OK:', len(data['samples']), 'samples')
PY
```

---

## Explicit non-goals

Not included in baseline:

- explicit many-to-many driver-target pair objects
- node-id-linked driver-target relations
- verifier redesign
- semantic scoring policy beyond a separate report section for drivers and
  targets

baseline should stay small: preserve the graph, add the overlay, and make review
output more faithful to the article's true market logic.
