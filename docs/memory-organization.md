# Memory Organization

VariX currently has **two global memory organization surfaces**:

1. **v1 cluster-first**
2. **v2 thesis-first**

They coexist during rollout. Accepted memory truth (`user_memory_nodes`,
acceptance events, and jobs) remains unchanged.

---

## v1: cluster-first global memory

Primary output object:
- `GlobalOrganizationOutput`

Key characteristics:
- groups accepted nodes into heuristic global clusters
- shows supporting / conflicting / conditional / predictive members
- uses neutral proposition summaries
- remains useful for regression comparison and debugging

CLI:
- `varix memory global-organize-run --user <user_id>`
- `varix memory global-organized --user <user_id>`
- `varix memory global-card --user <user_id>`

---

## v2: thesis-first global memory

Primary output object:
- `GlobalMemoryV2Output`

Pipeline:

`AcceptedNode -> CandidateThesis -> ConflictSet | CausalThesis -> CognitiveCard -> CognitiveConclusion -> TopMemoryItem`

### CandidateThesis
A neutral grouping around a shared cognitive question or mechanism domain.

Important fields:
- `topic_label`
- `node_ids`
- `source_refs`
- `cluster_reason`

Typical `cluster_reason` values:
- `same_source_causal_chain`
- `contradiction_pair`
- `shared_mechanism_theme`
- `shared_semantic_phrase`

### ConflictSet
Blocks abstraction when the thesis contains unresolved contradiction.

Important fields:
- `conflict_topic`
- `side_a_node_ids`
- `side_b_node_ids`
- `conflict_reason`

### CausalThesis
The internal causal proposition structure.

Important fields:
- `core_question`
- `node_roles`
- `edges`
- `core_path_node_ids`
- `traceability_map`

### CognitiveCard
The readable cognition object.

Important fields:
- `title`
- `summary`
- `causal_chain`
- `key_evidence`
- `conditions`
- `predictions`

### CognitiveConclusion
The top-level abstract judgment produced only when the thesis is strong enough
and not blocked by contradiction.

Important fields:
- `headline`
- `subheadline`
- `backing_card_ids`
- `traceability_status`

### TopMemoryItem
The first-layer display shell.

Two item types:
- `conclusion`
- `conflict`

---

## Current v2 quality rules

- contradiction blocks abstraction
- a single source may still produce a conclusion if the causal chain is strong
- card summaries follow the full core path
- conditions are separated from key evidence
- key evidence focuses on core supporting drivers
- conflict wording is humanized for product-facing surfaces

---

## CLI

Raw JSON:
- `varix memory global-v2-organize-run --user <user_id>`
- `varix memory global-v2-organized --user <user_id>`

Human-readable cards:
- `varix memory global-v2-card --user <user_id>`
- `varix memory global-v2-card --user <user_id> --run`
- `varix memory global-v2-card --user <user_id> --item-type conclusion`
- `varix memory global-v2-card --user <user_id> --item-type conflict`
- `varix memory global-v2-card --user <user_id> --limit 5`

Compare surfaces:
- `varix memory global-compare --user <user_id>`
- `varix memory global-compare --user <user_id> --run`
- `varix memory global-compare --user <user_id> --item-type conclusion`
- `varix memory global-compare --user <user_id> --item-type conflict`
- `varix memory global-compare --user <user_id> --limit 5`

---

## Rollout intent

v2 is intended to become the product-facing memory layer because it better
supports:
- abstraction
- causal organization
- contradiction-first handling
- multi-source cognitive synthesis

v1 remains available until thesis-first output quality is consistently stronger
than cluster-first output on real memory sets.
