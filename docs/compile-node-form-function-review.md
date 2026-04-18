# Compile Node Form+Function Redesign — Review Notes

This document now describes the **landed** form+function redesign, not just the
pre-rollout rationale. Use it as the review+documentation companion for the
current implementation in `varix/compile/*` and the prompt/test contract in
`prompts/compile/*`.

## Scope reviewed

This review covers the current compile node contract and the surfaces that will
have to change for the form+function redesign:

- `varix/compile/result.go`
- `varix/compile/parse.go`
- `varix/compile/verifier.go`
- `varix/compile/client.go`
- `prompts/compile/node_system.tmpl`
- `prompts/compile/node_challenge_system.tmpl`
- `prompts/compile/graph_system.tmpl`
- `varix/compile/prompt_test.go`
- batch-1 gold sample `G04` in `data/gold/compile-gold-batch1-v1.json`

The goal of this review is to document **why** the redesign was justified,
record the implementation contract that shipped, and lock the focused G04
regression target that now protects the rollout.

---

## Review summary

### Verdict

The redesign from a single node `kind` axis to separate **form** and
**function** axes is now implemented and aligns with the prompt behavior the
compile lane already asks models to follow.

### Main finding

The current schema mixes two different questions into one field:

1. **What shape of statement is this?**
   - observation
   - condition
   - judgment
   - forecast

2. **What role does it play in the reasoning graph?**
   - support
   - transmission
   - claim

That overload is currently handled informally in prompt wording rather than in
the node schema itself.

### Why the redesign was needed

Current prompts already distinguish:

- evidence/support nodes
- mechanism/transmission nodes
- judgment/claim nodes
- prediction nodes

But `varix/compile/result.go` still forces those meanings through one `kind`
enum:

- `事实`
- `显式条件`
- `隐含条件`
- `机制`
- `结论`
- `预测`

That works for coarse extraction, but it creates avoidable ambiguity in the
high-value cases this lane now cares about:

- a current-frame market mechanism can look like either `事实` or `机制`
- a conditional bridge can look like either `显式条件` or `隐含条件`
- a slogan-like thesis and a forward-looking thesis both behave like top-level
  claims, but are split across `结论` and `预测`

The new schema makes those distinctions first-class instead of prompt-only.

---

## Landed implementation contract

### Schema behavior in code

The rollout is additive rather than replacement-only:

- `GraphNode` still carries legacy `kind`
- `GraphNode` also carries first-class `form` and `function`
- JSON parsing accepts:
  - legacy `kind`-only payloads
  - dual-axis payloads with `form` + `function`
  - mixed payloads as long as they are internally consistent

`varix/compile/result.go` is the canonical mapping layer. It normalizes every
node into one internally consistent triple:

- legacy `kind`
- `form`
- `function`

That means downstream code can remain stable during the migration while prompts
and tests move to the dual-axis vocabulary.

### Current canonical mapping

The shipped normalization contract is:

| Legacy kind | Form | Function | Operational meaning |
| --- | --- | --- | --- |
| `事实` | `observation` | `support` | observed evidence / supporting state |
| `显式条件` | `condition` | `claim` | explicit if/when prerequisite stated as a top-level gating claim |
| `隐含条件` | `condition` | `support` | implicit premise or already-operative condition supporting a downstream claim |
| `机制` | `observation` | `transmission` | active world-state bridge / pricing-allocation mechanism |
| `结论` | `judgment` | `claim` | current judgment / slogan / thesis |
| `预测` | `forecast` | `claim` | future outcome |

Additional parser rules worth remembering:

- `condition + transmission` is accepted and normalized back into the legacy
  implicit-condition lane, preserving migration safety for bridge-like
  conditions.
- explicit condition text such as `如果 / 若 / 一旦` is auto-normalized away
  from generic `事实` into the explicit-condition lane.
- legacy validity windows are still normalized into the newer timing fields.

### Code-quality review findings

The current implementation is disciplined in the places that matter for this
rollout:

- `varix/compile/result.go` keeps the migration additive instead of forcing a
  flag day; legacy `kind` payloads and dual-axis payloads both normalize through
  one compatibility layer.
- `varix/compile/parse.go` keeps the rollout focused on taxonomy and timing
  normalization instead of mixing in broader graph or verifier redesign.
- `varix/compile/verifier.go` routes by normalized statement lane, which means
  transmission-bridge recovery improves node recall without requiring a full
  verification-taxonomy rewrite in the same change set.
- `varix/compile/prompt_test.go`, `varix/compile/result_test.go`, and
  `varix/compile/form_function_regression_test.go` lock the main regression
  target: preserve the support -> transmission -> claim separation for G04-style
  flow theses.

### Scope discipline: what shipped vs what stayed intentionally small

The valuable part of this rollout is not a general ontology expansion. The
valuable part is recovering the missing primary transmission bridge in
flow/positioning cases.

That means the current implementation made the right tradeoffs:

- it upgraded node semantics without rewriting the verifier architecture
- it tightened prompt guidance around bridge recovery instead of adding broad
  case-specific prompt hacks
- it preserved backward compatibility for legacy node and edge payloads while
  moving the canonical contract forward

Future edits should keep that same bias: improve bridge recovery, edge typing,
and focused regression coverage first; defer lower-value taxonomy polish unless
there is concrete failure evidence.

### Edge-schema compatibility note

The canonical full-graph edge contract is now:

- `drives`
- `substantiates`
- `gates`
- `explains`

`varix/compile/result.go` still accepts legacy aliases such as `正向`, `推出`,
`预设`, and `解释` during parsing, but those aliases are migration
compatibility only. Documentation and future prompt work should treat the
English edge kinds above as the source-of-truth schema.

---

## Proposed contract

### Form axis

Use `form` for the temporal/logical shape of the statement:

- `observation`
- `condition`
- `judgment`
- `forecast`

### Function axis

Use `function` for the node's role in the article's causal spine:

- `support`
- `transmission`
- `claim`

### Practical mapping

| Example role | Form | Function | Notes |
| --- | --- | --- | --- |
| observed evidence / reported state | `observation` | `support` | facts that help justify another node |
| current operative market mechanism | `observation` | `transmission` | pricing/allocation rule already active in the article frame |
| explicit if/when prerequisite | `condition` | `claim` | top-level gating clause that should drive a `预设` edge |
| implicit supporting prerequisite | `condition` | `support` | unstated or already-operative condition that helps justify another node |
| author slogan / current thesis | `judgment` | `claim` | e.g. “there is no sell America trade” |
| future outcome | `forecast` | `claim` | future state/outcome to be tested later |

### Backward-compatibility rule

Rollout is additive and parser-safe:

- keep accepting legacy `kind` payloads during migration
- normalize legacy `kind` into `form` + `function`
- do not weaken timing normalization already handled in `parse.go`
- do not broaden edge semantics just because node semantics become clearer

### Time-field rule

The redesign preserves the current timing behavior:

- current-frame operative observations/conditions still use `occurred_at`
- forecasts still use `prediction_start_at`
- `prediction_due_at` remains optional and inference-based

Do not regress the current timestamp normalization while changing taxonomy.

### Verifier compatibility rule

The verifier lane remains intentionally conservative:

- `varix/compile/parse.go` normalizes dual-axis nodes back into a compatible
  legacy `kind`
- `varix/compile/verifier.go` still routes passes by normalized statement lane:
  - facts
  - explicit conditions
  - implicit conditions
  - predictions

So the redesign clarifies extraction semantics without forcing a full verifier
taxonomy rewrite in the same rollout.

---

## Focused G04 regression contract

### Why G04 is the right regression sample

Batch-1 sample `G04` is the cleanest stress case for this redesign because it
contains all three roles that the current prompts already care about:

1. **support observation**
   - overseas money is still flowing into US assets
2. **transmission bridge**
   - growth / return expectations still dominate political-risk pricing
3. **claim-level judgment**
   - no `sell America` trade forms
   - no `hedge America` trade forms

Because the redesign is now live, G04 should continue to avoid collapsing those
roles into a flat `事实 + 结论` shape.

### Gold evidence snapshot

The current gold dataset already encodes the intended top-level meaning for
`G04`:

- drivers:
  - US growth narrative still attracts global capital
  - political risk has not overridden market preference for US assets
- targets:
  - overseas capital continues flowing into US assets
  - no `sell America` trade forms
  - no `hedge America` trade forms

### Minimum node-level expectations for G04

The current implementation should preserve at least this separation:

1. `observation + transmission`
   - growth / return expectations dominate political-risk pricing
2. `observation + support`
   - foreign capital continues flowing into US assets
3. `judgment + claim`
   - no `sell America` trade forms
4. `judgment + claim`
   - no `hedge America` trade forms

### Minimum edge-level expectations for G04

The accompanying graph contract is just as important as the node contract:

1. `support -> claim` should usually stay `substantiates`
   - observed inflow evidence supports the judgment
2. `transmission -> claim` should usually stay `drives`
   - the allocation/pricing bridge is the world-state mechanism
3. conditional downside branches should use `gates`
   - only when the source node is a condition

Legacy aliases (`推出`, `正向`, `预设`) may still parse successfully during
compatibility handling, but they are no longer the canonical edge names.

### What must not regress

- Do **not** flatten the transmission bridge into a generic support fact.
- Do **not** collapse the flow observation and the judgment slogan into one fat
  node.
- Do **not** replace the current flow/positioning thesis with side macro
  commentary.
- Do **not** remove the distinction between evidential support and causal
  transmission when wiring graph edges.

G04 should remain the quick-check article for
`observation -> transmission -> claim` coverage.

---

## Implementation checklist

### Schema / parsing

- `varix/compile/result.go`
  - adds the new axes without weakening validation
  - keeps migration-safe handling for legacy `kind`
- `varix/compile/parse.go`
  - normalizes old payloads into the new axes
  - preserves current timing normalization

### Verification routing

- `varix/compile/verifier.go`
- `varix/compile/client.go`

Verifier routing should remain compatible with the normalized statement lane
while still preserving the current pass split:

- facts / support observations
- explicit conditions
- implicit conditions
- predictions

The redesign should clarify node meaning without forcing a verifier-lane
rewrite.

### Prompt / test expectations

- `prompts/compile/node_system.tmpl`
- `prompts/compile/node_challenge_system.tmpl`
- `prompts/compile/graph_system.tmpl`
- `varix/compile/prompt_test.go`

Prompt text and tests should state the dual-axis contract explicitly:

- form = `observation | condition | judgment | forecast`
- function = `support | transmission | claim`

And the G04 regression checks should confirm that the compile lane still keeps:

- support observations
- transmission bridge nodes
- claim/judgment nodes

distinct from each other.

---

## Review conclusion

No blocking design objection was found for the form+function redesign, and the
current implementation matches the intended rollout shape.

The main caution remains rollout discipline during future edits: keep legacy
parsing intact, keep timing normalization intact, keep verifier routing stable,
and use G04 as the focused regression that proves the schema still separates
**support**, **transmission**, and **claim** roles without losing the existing
observation / condition / judgment / forecast distinction.
