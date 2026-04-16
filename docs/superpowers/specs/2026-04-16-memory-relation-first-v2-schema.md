# VariX Memory v2 关系优先 Schema 草案

## 1. Goal

这份文档把已经确定的 relation-first 设计进一步落到**字段级 schema**，用于后续：
- Go types 设计
- SQLite 表设计
- query / index 设计
- migration 设计

它不重新讨论产品原则，只负责把已经批准的模型具体化。

---

## 2. Schema 分层

Memory v2 的 schema 分为五层：

1. **Accepted substrate**
   - 复用现有 `AcceptedNode / AcceptanceEvent / OrganizationJob`
2. **Canonical layer**
   - `CanonicalEntity`
3. **Truth boundary layer**
   - `Relation`
4. **Mechanism layer**
   - `Mechanism`
   - `MechanismNode`
   - `MechanismEdge`
   - `PathOutcome`
5. **Derived view layer**
   - `DriverAggregate`
   - `TargetAggregate`
   - `ConflictView`
   - `CognitiveCard`
   - `CognitiveConclusion`
   - `TopMemoryItem`

---

## 3. Object Schemas

## 3.1 CanonicalEntity

### Responsibility
稳定锚定财经对象，承载别名、合并、拆分历史。

### Fields
| Field | Type | Required | Notes |
|---|---|---:|---|
| `entity_id` | string | yes | stable id |
| `entity_type` | enum | yes | `driver` / `target` / `both` |
| `canonical_name` | string | yes | primary display name |
| `aliases` | string[] | no | normalized alias list |
| `status` | enum | yes | `active` / `merged` / `split` / `retired` |
| `merge_history` | string[] | no | entity ids merged into this one or replaced by this one |
| `split_history` | string[] | no | descendant entity ids after split |
| `created_at` | timestamp | yes | |
| `updated_at` | timestamp | yes | |

### Constraints
- `canonical_name` must be normalized and non-empty
- one alias should not resolve to multiple active entities in the same namespace

---

## 3.2 Relation

### Responsibility
表示一条稳定的 driver → target 关系边界。

### Fields
| Field | Type | Required | Notes |
|---|---|---:|---|
| `relation_id` | string | yes | stable id |
| `driver_entity_id` | string | yes | FK-like ref to `CanonicalEntity` |
| `target_entity_id` | string | yes | FK-like ref to `CanonicalEntity` |
| `status` | enum | yes | `active` / `inactive` / `retired` / `merged` / `split` / `superseded` |
| `retired_at` | timestamp | no | |
| `superseded_by_relation_id` | string | no | self-ref |
| `merge_history` | string[] | no | relation ids |
| `split_history` | string[] | no | relation ids |
| `lifecycle_reason` | string | no | human-readable explanation |
| `created_at` | timestamp | yes | |
| `updated_at` | timestamp | yes | |

### Constraints
- unique active relation boundary on `(driver_entity_id, target_entity_id)`
- `driver_entity_id != target_entity_id` is allowed or forbidden depending on domain; MVP should allow self-relations only if explicitly needed
- relation does **not** store polarity

---

## 3.3 Mechanism

### Responsibility
表示某条 relation 在某个时间切片上的正文层。

### Fields
| Field | Type | Required | Notes |
|---|---|---:|---|
| `mechanism_id` | string | yes | stable row id |
| `relation_id` | string | yes | parent relation |
| `as_of` | timestamp | yes | evaluation timestamp |
| `valid_from` | timestamp | no | |
| `valid_to` | timestamp | no | |
| `confidence` | float | yes | normalized 0-1 |
| `status` | enum | yes | `active` / `historical` / `invalidated` |
| `source_refs` | string[] | no | source platform/id refs |
| `traceability_status` | enum | yes | `complete` / `partial` / `weak` |
| `created_at` | timestamp | yes | |
| `updated_at` | timestamp | yes | |

### Constraints
- one relation can have many mechanisms over time
- at most one `active` mechanism per `(relation_id, as_of-window)` in MVP

---

## 3.4 MechanismNode

### Responsibility
表达机制图中的单个节点。

### Fields
| Field | Type | Required | Notes |
|---|---|---:|---|
| `mechanism_node_id` | string | yes | |
| `mechanism_id` | string | yes | parent mechanism |
| `node_type` | enum | yes | see node taxonomy below |
| `label` | string | yes | rendered node text |
| `backing_accepted_node_ids` | string[] | no | accepted substrate refs |
| `sort_order` | int | no | display convenience only |
| `created_at` | timestamp | yes | |

### Node taxonomy
- `driver`
- `macro_event`
- `policy_state`
- `liquidity_state`
- `market_behavior`
- `asset_flow`
- `condition`
- `boundary`
- `target_effect`

---

## 3.5 MechanismEdge

### Responsibility
表达机制图中的方向性边。

### Fields
| Field | Type | Required | Notes |
|---|---|---:|---|
| `mechanism_edge_id` | string | yes | |
| `mechanism_id` | string | yes | parent mechanism |
| `from_node_id` | string | yes | |
| `to_node_id` | string | yes | |
| `edge_type` | enum | yes | see taxonomy |
| `created_at` | timestamp | yes | |

### Edge taxonomy
- `causes`
- `amplifies`
- `suppresses`
- `transmits`
- `requires`
- `presets`
- `conflicts_with`

---

## 3.6 PathOutcome

### Responsibility
表达一条具体 mechanism 路径的结果，是 conflict 的最小比较单元。

### Fields
| Field | Type | Required | Notes |
|---|---|---:|---|
| `path_outcome_id` | string | yes | |
| `mechanism_id` | string | yes | parent mechanism |
| `node_path` | string[] | yes | ordered `mechanism_node_id` list |
| `outcome_polarity` | enum | yes | `bullish` / `bearish` / `mixed` / `conditional` / `unresolved` |
| `outcome_label` | string | yes | human-readable outcome summary |
| `condition_scope` | string | no | condition/regime description |
| `confidence` | float | yes | normalized 0-1 |
| `created_at` | timestamp | yes | |

### Constraints
- no `primary_path` / `alternative_path` persistence flag
- ranking is computed at render time

---

## 3.7 DriverAggregate

### Responsibility
按 driver 聚合 relation + mechanism neighborhood。

### Fields
| Field | Type | Required | Notes |
|---|---|---:|---|
| `aggregate_id` | string | yes | |
| `driver_entity_id` | string | yes | canonical anchor |
| `relation_ids` | string[] | yes | |
| `target_entity_ids` | string[] | yes | |
| `mechanism_labels` | string[] | no | display helper |
| `coverage_score` | float | yes | normalized 0-1 |
| `conflict_count` | int | yes | |
| `active_conclusion_ids` | string[] | no | |
| `traceability_status` | enum | yes | |
| `as_of` | timestamp | yes | |
| `created_at` | timestamp | yes | |

---

## 3.8 TargetAggregate

### Responsibility
按 target 聚合 relation + mechanism neighborhood。

### Fields
| Field | Type | Required | Notes |
|---|---|---:|---|
| `aggregate_id` | string | yes | |
| `target_entity_id` | string | yes | canonical anchor |
| `relation_ids` | string[] | yes | |
| `driver_entity_ids` | string[] | yes | |
| `mechanism_labels` | string[] | no | display helper |
| `coverage_score` | float | yes | |
| `conflict_count` | int | yes | |
| `active_conclusion_ids` | string[] | no | |
| `traceability_status` | enum | yes | |
| `as_of` | timestamp | yes | |
| `created_at` | timestamp | yes | |

---

## 3.9 ConflictView

### Responsibility
表达由相互冲突的 path outcomes 派生出的结构化冲突视图。

### Fields
| Field | Type | Required | Notes |
|---|---|---:|---|
| `conflict_id` | string | yes | |
| `scope_type` | enum | yes | `relation` / `driver_aggregate` / `target_aggregate` |
| `scope_id` | string | yes | relation or aggregate id |
| `left_path_outcome_ids` | string[] | yes | |
| `right_path_outcome_ids` | string[] | yes | |
| `conflict_reason` | string | yes | |
| `conflict_topic` | string | no | |
| `status` | enum | yes | `active` / `downgraded` / `resolved` |
| `as_of` | timestamp | yes | |
| `traceability_map` | json | no | |
| `created_at` | timestamp | yes | |

---

## 3.10 CognitiveCard

### Responsibility
relation detail card，默认粒度为 `(relation_id, as_of)`。

### Fields
| Field | Type | Required | Notes |
|---|---|---:|---|
| `card_id` | string | yes | |
| `relation_id` | string | yes | |
| `as_of` | timestamp | yes | |
| `title` | string | yes | |
| `summary` | string | yes | |
| `mechanism_chain` | string[] | no | rendered chain |
| `key_evidence` | string[] | no | |
| `conditions` | string[] | no | |
| `predictions` | string[] | no | |
| `source_refs` | string[] | no | |
| `confidence_label` | enum | yes | `weak` / `medium` / `strong` |
| `trace_entry` | string[] | no | |
| `created_at` | timestamp | yes | |

---

## 3.11 CognitiveConclusion

### Responsibility
高层抽象结论，必须先过 hard gate 再过 LLM soft judge。

### Fields
| Field | Type | Required | Notes |
|---|---|---:|---|
| `conclusion_id` | string | yes | |
| `source_type` | enum | yes | `relation` / `driver_aggregate` / `target_aggregate` |
| `source_id` | string | yes | |
| `headline` | string | yes | |
| `subheadline` | string | no | |
| `backing_card_ids` | string[] | yes | |
| `core_claims` | string[] | no | |
| `traceability_status` | enum | yes | |
| `blocked_by_conflict` | bool | yes | |
| `as_of` | timestamp | yes | |
| `judge_model` | string | no | soft gate metadata |
| `judge_prompt_version` | string | no | |
| `judge_scores` | json | no | per-dimension scores |
| `judge_passed` | bool | no | |
| `judged_at` | timestamp | no | |
| `created_at` | timestamp | yes | |

---

## 3.12 TopMemoryItem

### Responsibility
统一第一层 feed 壳。

### Fields
| Field | Type | Required | Notes |
|---|---|---:|---|
| `item_id` | string | yes | |
| `item_type` | enum | yes | `driver_aggregate` / `target_aggregate` / `card` / `conclusion` / `conflict` |
| `headline` | string | yes | |
| `subheadline` | string | no | |
| `backing_object_id` | string | yes | |
| `signal_strength` | enum | yes | `low` / `medium` / `high` |
| `as_of` | timestamp | yes | |
| `updated_at` | timestamp | yes | |

---

## 4. SQLite Table Draft

## 4.1 Canonical tables

### `memory_canonical_entities`
```sql
CREATE TABLE memory_canonical_entities (
  entity_id TEXT PRIMARY KEY,
  entity_type TEXT NOT NULL,
  canonical_name TEXT NOT NULL,
  status TEXT NOT NULL,
  merge_history_json TEXT NOT NULL DEFAULT '[]',
  split_history_json TEXT NOT NULL DEFAULT '[]',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
```

### `memory_canonical_entity_aliases`
```sql
CREATE TABLE memory_canonical_entity_aliases (
  alias_id INTEGER PRIMARY KEY AUTOINCREMENT,
  entity_id TEXT NOT NULL,
  alias_text TEXT NOT NULL,
  created_at TEXT NOT NULL
);
CREATE UNIQUE INDEX idx_memory_entity_alias_unique ON memory_canonical_entity_aliases(alias_text);
CREATE INDEX idx_memory_entity_alias_entity ON memory_canonical_entity_aliases(entity_id);
```

---

## 4.2 Relation tables

### `memory_relations`
```sql
CREATE TABLE memory_relations (
  relation_id TEXT PRIMARY KEY,
  driver_entity_id TEXT NOT NULL,
  target_entity_id TEXT NOT NULL,
  status TEXT NOT NULL,
  retired_at TEXT,
  superseded_by_relation_id TEXT,
  merge_history_json TEXT NOT NULL DEFAULT '[]',
  split_history_json TEXT NOT NULL DEFAULT '[]',
  lifecycle_reason TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE UNIQUE INDEX idx_memory_relations_driver_target_active
  ON memory_relations(driver_entity_id, target_entity_id)
  WHERE status IN ('active','inactive');
CREATE INDEX idx_memory_relations_driver ON memory_relations(driver_entity_id);
CREATE INDEX idx_memory_relations_target ON memory_relations(target_entity_id);
```

---

## 4.3 Mechanism tables

### `memory_mechanisms`
```sql
CREATE TABLE memory_mechanisms (
  mechanism_id TEXT PRIMARY KEY,
  relation_id TEXT NOT NULL,
  as_of TEXT NOT NULL,
  valid_from TEXT,
  valid_to TEXT,
  confidence REAL NOT NULL,
  status TEXT NOT NULL,
  source_refs_json TEXT NOT NULL DEFAULT '[]',
  traceability_status TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX idx_memory_mechanisms_relation_asof ON memory_mechanisms(relation_id, as_of DESC);
```

### `memory_mechanism_nodes`
```sql
CREATE TABLE memory_mechanism_nodes (
  mechanism_node_id TEXT PRIMARY KEY,
  mechanism_id TEXT NOT NULL,
  node_type TEXT NOT NULL,
  label TEXT NOT NULL,
  backing_accepted_node_ids_json TEXT NOT NULL DEFAULT '[]',
  sort_order INTEGER,
  created_at TEXT NOT NULL
);
CREATE INDEX idx_memory_mechanism_nodes_mechanism ON memory_mechanism_nodes(mechanism_id);
```

### `memory_mechanism_edges`
```sql
CREATE TABLE memory_mechanism_edges (
  mechanism_edge_id TEXT PRIMARY KEY,
  mechanism_id TEXT NOT NULL,
  from_node_id TEXT NOT NULL,
  to_node_id TEXT NOT NULL,
  edge_type TEXT NOT NULL,
  created_at TEXT NOT NULL
);
CREATE INDEX idx_memory_mechanism_edges_mechanism ON memory_mechanism_edges(mechanism_id);
```

### `memory_path_outcomes`
```sql
CREATE TABLE memory_path_outcomes (
  path_outcome_id TEXT PRIMARY KEY,
  mechanism_id TEXT NOT NULL,
  node_path_json TEXT NOT NULL,
  outcome_polarity TEXT NOT NULL,
  outcome_label TEXT NOT NULL,
  condition_scope TEXT,
  confidence REAL NOT NULL,
  created_at TEXT NOT NULL
);
CREATE INDEX idx_memory_path_outcomes_mechanism ON memory_path_outcomes(mechanism_id);
CREATE INDEX idx_memory_path_outcomes_polarity ON memory_path_outcomes(outcome_polarity);
```

---

## 4.4 Derived tables

### `memory_driver_aggregates`
```sql
CREATE TABLE memory_driver_aggregates (
  aggregate_id TEXT PRIMARY KEY,
  driver_entity_id TEXT NOT NULL,
  relation_ids_json TEXT NOT NULL,
  target_entity_ids_json TEXT NOT NULL,
  mechanism_labels_json TEXT NOT NULL DEFAULT '[]',
  coverage_score REAL NOT NULL,
  conflict_count INTEGER NOT NULL,
  active_conclusion_ids_json TEXT NOT NULL DEFAULT '[]',
  traceability_status TEXT NOT NULL,
  as_of TEXT NOT NULL,
  created_at TEXT NOT NULL
);
CREATE INDEX idx_memory_driver_aggregates_driver_asof ON memory_driver_aggregates(driver_entity_id, as_of DESC);
```

### `memory_target_aggregates`
```sql
CREATE TABLE memory_target_aggregates (
  aggregate_id TEXT PRIMARY KEY,
  target_entity_id TEXT NOT NULL,
  relation_ids_json TEXT NOT NULL,
  driver_entity_ids_json TEXT NOT NULL,
  mechanism_labels_json TEXT NOT NULL DEFAULT '[]',
  coverage_score REAL NOT NULL,
  conflict_count INTEGER NOT NULL,
  active_conclusion_ids_json TEXT NOT NULL DEFAULT '[]',
  traceability_status TEXT NOT NULL,
  as_of TEXT NOT NULL,
  created_at TEXT NOT NULL
);
CREATE INDEX idx_memory_target_aggregates_target_asof ON memory_target_aggregates(target_entity_id, as_of DESC);
```

### `memory_conflict_views`
```sql
CREATE TABLE memory_conflict_views (
  conflict_id TEXT PRIMARY KEY,
  scope_type TEXT NOT NULL,
  scope_id TEXT NOT NULL,
  left_path_outcome_ids_json TEXT NOT NULL,
  right_path_outcome_ids_json TEXT NOT NULL,
  conflict_reason TEXT NOT NULL,
  conflict_topic TEXT,
  status TEXT NOT NULL,
  as_of TEXT NOT NULL,
  traceability_map_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL
);
CREATE INDEX idx_memory_conflict_views_scope_asof ON memory_conflict_views(scope_type, scope_id, as_of DESC);
```

### `memory_cognitive_cards`
```sql
CREATE TABLE memory_cognitive_cards (
  card_id TEXT PRIMARY KEY,
  relation_id TEXT NOT NULL,
  as_of TEXT NOT NULL,
  title TEXT NOT NULL,
  summary TEXT NOT NULL,
  mechanism_chain_json TEXT NOT NULL DEFAULT '[]',
  key_evidence_json TEXT NOT NULL DEFAULT '[]',
  conditions_json TEXT NOT NULL DEFAULT '[]',
  predictions_json TEXT NOT NULL DEFAULT '[]',
  source_refs_json TEXT NOT NULL DEFAULT '[]',
  confidence_label TEXT NOT NULL,
  trace_entry_json TEXT NOT NULL DEFAULT '[]',
  created_at TEXT NOT NULL
);
CREATE INDEX idx_memory_cognitive_cards_relation_asof ON memory_cognitive_cards(relation_id, as_of DESC);
```

### `memory_cognitive_conclusions`
```sql
CREATE TABLE memory_cognitive_conclusions (
  conclusion_id TEXT PRIMARY KEY,
  source_type TEXT NOT NULL,
  source_id TEXT NOT NULL,
  headline TEXT NOT NULL,
  subheadline TEXT,
  backing_card_ids_json TEXT NOT NULL,
  core_claims_json TEXT NOT NULL DEFAULT '[]',
  traceability_status TEXT NOT NULL,
  blocked_by_conflict INTEGER NOT NULL,
  as_of TEXT NOT NULL,
  judge_model TEXT,
  judge_prompt_version TEXT,
  judge_scores_json TEXT NOT NULL DEFAULT '{}',
  judge_passed INTEGER,
  judged_at TEXT,
  created_at TEXT NOT NULL
);
CREATE INDEX idx_memory_cognitive_conclusions_source_asof ON memory_cognitive_conclusions(source_type, source_id, as_of DESC);
```

### `memory_top_items`
```sql
CREATE TABLE memory_top_items (
  item_id TEXT PRIMARY KEY,
  item_type TEXT NOT NULL,
  headline TEXT NOT NULL,
  subheadline TEXT,
  backing_object_id TEXT NOT NULL,
  signal_strength TEXT NOT NULL,
  as_of TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX idx_memory_top_items_asof ON memory_top_items(as_of DESC);
CREATE INDEX idx_memory_top_items_type_asof ON memory_top_items(item_type, as_of DESC);
```

---

## 5. Query Patterns

### 5.1 By Driver
1. resolve input → `CanonicalEntity`
2. fetch latest `DriverAggregate` by `driver_entity_id + as_of`
3. expand relation ids → cards / conclusions / conflicts

### 5.2 By Target
1. resolve input → `CanonicalEntity`
2. fetch latest `TargetAggregate` by `target_entity_id + as_of`
3. expand relation ids → cards / conclusions / conflicts

### 5.3 Relation Detail
1. fetch `Relation`
2. fetch latest `Mechanism` by `(relation_id, as_of)`
3. fetch nodes / edges / path outcomes
4. fetch current `CognitiveCard`

### 5.4 Feed
1. fetch `memory_top_items` by latest `as_of`
2. resolve backing objects lazily

---

## 6. Mutation Rules

### 6.1 New accepted nodes
- run canonical resolution
- match/create relation
- update or create mechanism
- recompute path outcomes
- recompute conflicts / aggregates / cards / conclusions / top items

### 6.2 Tombstone / retraction
- mark accepted node or source as tombstoned
- do not hard delete immediately
- rerun organization for affected relation set
- degrade / retire downstream artifacts as needed

### 6.3 Relation lifecycle changes
- merge: keep tombstoned prior relations in history
- split: create new relations, mark old one superseded/split
- retire: keep historical mechanisms, stop surfacing as active

---

## 7. Open Design Notes

1. `CanonicalEntity` alias governance may eventually need a review workflow
2. relation uniqueness on `(driver_entity_id, target_entity_id)` is the current MVP assumption
3. mechanism records may later be versioned more explicitly if update volume is high
4. SQLite stores arrays/maps as JSON text in MVP; can normalize further if query pressure grows

---

## 8. Implementation Order Recommendation

1. `CanonicalEntity`
2. `Relation`
3. `Mechanism`
4. `MechanismNode`
5. `MechanismEdge`
6. `PathOutcome`
7. `ConflictView`
8. `DriverAggregate` / `TargetAggregate`
9. `CognitiveCard`
10. `CognitiveConclusion`
11. `TopMemoryItem`
