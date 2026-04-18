# Compile Form+Function Implementation Plan

## Status

This document is the current implementation plan for the compile redesign.
No further code changes should be made until this document is reviewed.

## Goal

Improve compile so that graph construction is not just locally coherent but also globally complete enough to recover the primary transmission node in cases like G04.

## Core design

### 1. Node schema

Each node has two axes:

- `form`
  - `observation`
  - `condition`
  - `judgment`
  - `forecast`

- `function`
  - `support`
  - `transmission`
  - `claim`

Interpretation:

- `form` answers: what kind of statement is this?
- `function` answers: what role does this statement play in the reasoning chain?

### 2. Node generation workflow

#### Generator

Purpose:
- maximize recall
- prefer semantic splitting over sentence-level compression

Rules:
- split when one sentence mixes evidence, transmission, and claim
- split when one sentence contains multiple independently evaluable claims
- split when connectors imply multiple semantic steps (`because`, `therefore`, `due to`, `implying`, etc.)
- if evidence supports a claim but the pricing / allocation / positioning rule is missing, generate a transmission node for that bridge

#### Challenge

Purpose:
- catch missing nodes
- catch under-split nodes
- explicitly look for missing transmission bridges

Challenge checks:
- missing evidence nodes
- missing transmission nodes
- missing claim nodes
- evidence and claim present but no transmission bridge present
- a transmission rule collapsed into a judgment node
- multiple claim-level propositions packed into one node

### 3. Edge schema

Allowed edge kinds only:

- `drives`
- `substantiates`
- `gates`
- `explains`

Required shape:

```json
{"from":"...","to":"...","kind":"drives|substantiates|gates|explains"}
```

No alternate keys:
- no `source`
- no `target`
- no `relation`

### 4. Edge semantics

#### `drives`
World-state transmission.
A changes the world and thereby moves B.
This is the only edge type that belongs to the main causal spine.

#### `substantiates`
Evidential support.
A makes B more justified or more believable, but does not itself make B happen.

#### `gates`
Prerequisite / gate.
The source node must be a condition node, and the downstream node depends on it.

#### `explains`
Interpretive framing.
A tells the reader how to understand B or what theory / frame B belongs to, but does not directly prove or cause B.

### 5. Main chain vs auxiliary layer

#### Main chain
- only `drives`

#### Auxiliary layer
- `substantiates`
- `gates`
- `explains`

### 6. Causal projection

Causal projection is code-only, not prompt-based.

Rules:
- keep only `drives`
- discard `substantiates`
- discard `gates`
- discard `explains`
- drop isolated nodes that are not attached to any `drives` edge

### 7. Thesis mapping

Input:
- causal projection only

Output:
- `drivers`
- `targets`
- `summary`

Rules:
- do not invent new middle nodes
- do not re-read discarded auxiliary edges as if they were main-chain evidence
- treat `summary` as the natural-language organization of the final driver/target relation

## What is still failing today

### Main recurring gap

The system still struggles when a case requires a **primary transmission bridge** between:
- support observations
- and claim-level judgments

Typical missing bridge shape:
- one pricing force dominates another
- one allocation preference dominates another
- one positioning rule keeps capital allocated a certain way

The system often captures:
- evidence
- claim

but misses the bridge transmission node that explains why the evidence leads to the claim.

## General debugging rule

When a case fails, first ask:
1. Are the support nodes present?
2. Are the claim nodes present?
3. Is the transmission bridge missing?
4. If the bridge exists, is it incorrectly typed as judgment instead of transmission?
5. Are edges correct once the bridge exists?

## G04-specific diagnostic pattern (abstracted)

This plan intentionally avoids case-specific wording in prompts.
But the recurring abstract pattern is:
- support observations about continued flow / positioning
- a missing transmission rule about pricing / allocation dominance
- a claim about a market narrative not truly being present

That pattern should be addressed by general rules, not sample-specific prompt language.

## Acceptance criteria

### Nodes
- support / transmission / claim layers can be separated in the same article
- multiple independent claims are split
- bridge transmission nodes are generated when needed

### Edges
- all edges use exactly `from` / `to` / `kind`
- all edge kinds are one of the four allowed values
- no blank edge kind
- no alternate edge schema keys

### Main chain
- causal projection contains only `drives`
- auxiliary edges do not leak into the main causal spine

### Thesis
- driver and target come from the projected main chain
- summary matches that same chain

## Validation workflow

1. run focused compile tests
2. run targeted case probes (especially G04-like failures)
3. inspect:
   - nodes
   - edges
   - projection
   - final thesis
4. only then decide whether the failure is in recall, bridge recovery, edge typing, or thesis mapping
