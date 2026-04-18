# Compile Node Form+Function Redesign — Review Notes

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

The goal of this review is to document **why** the redesign is justified,
define the minimal rollout contract, and lock one focused regression target
before the implementation and test lanes land their code changes.

---

## Review summary

### Verdict

The redesign from a single node `kind` axis to separate **form** and
**function** axes is justified and aligns with the prompt behavior the compile
lane is already asking models to follow.

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

### Why the redesign is needed

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
| if/when prerequisite | `condition` | `support` or `transmission` | keep `condition` for gating clauses; choose `transmission` only when the clause itself is the causal bridge |
| author slogan / current thesis | `judgment` | `claim` | e.g. “there is no sell America trade” |
| future outcome | `forecast` | `claim` | future state/outcome to be tested later |

### Backward-compatibility rule

Rollout should be additive and parser-safe:

- keep accepting legacy `kind` payloads during migration
- normalize legacy `kind` into `form` + `function`
- do not weaken timing normalization already handled in `parse.go`
- do not broaden edge semantics just because node semantics become clearer

### Time-field rule

The redesign should preserve the current timing behavior:

- current-frame operative observations/conditions still use `occurred_at`
- forecasts still use `prediction_start_at`
- `prediction_due_at` remains optional and inference-based

Do not regress the current timestamp normalization while changing taxonomy.

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

If the redesign is correct, G04 should stop collapsing those roles into a flat
`事实 + 结论` shape.

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

The redesign should preserve at least this separation:

1. `observation + transmission`
   - growth / return expectations dominate political-risk pricing
2. `observation + support`
   - foreign capital continues flowing into US assets
3. `judgment + claim`
   - no `sell America` trade forms
4. `judgment + claim`
   - no `hedge America` trade forms

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
  - add the new axes without weakening validation
  - keep migration-safe handling for legacy `kind`
- `varix/compile/parse.go`
  - normalize old payloads into the new axes
  - preserve current timing normalization

### Verification routing

- `varix/compile/verifier.go`
- `varix/compile/client.go`

Verifier routing should key primarily off statement shape (`form`) while still
preserving the current pass split:

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

No blocking design objection was found for the form+function redesign.

The main caution is rollout discipline: keep legacy parsing intact, keep timing
normalization intact, and use G04 as the focused regression that proves the new
schema separates **support**, **transmission**, and **claim** roles without
losing the existing observation / condition / judgment / forecast distinction.
