# VariX Memory synthesis SQLite Migration 草案

## 1. Goal

这份文档把 relation-first schema 继续落到 **SQLite migration 草案**。

目标是：
- 规划表创建顺序
- 规划索引策略
- 规划与现有 memory 表并存方式
- 为后续 migration 脚本和 test setup 提供直接参考

---

## 2. Migration Principles

1. **Additive first**
   - synthesis 表先增量创建，不破坏 cluster-first 表
2. **Coexistence by default**
   - cluster-first 与 relation-first synthesis 并存
3. **JSON for MVP**
   - 多值字段与 traceability map 先落 JSON text
4. **Stable IDs in app layer**
   - 核心对象 id 由应用层生成，不依赖 SQLite 自增主键
5. **Tombstone-friendly**
   - deletion / retraction 先通过 soft delete / tombstone + reevaluation 支持

---

## 3. New table creation order

Recommended order:

1. `memory_canonical_entities`
2. `memory_canonical_entity_aliases`
3. `memory_relations`
4. `memory_mechanisms`
5. `memory_mechanism_nodes`
6. `memory_mechanism_edges`
7. `memory_path_outcomes`
8. `memory_driver_aggregates`
9. `memory_target_aggregates`
10. `memory_conflict_views`
11. `memory_cognitive_cards`
12. `memory_cognitive_conclusions`
13. `memory_top_items`
14. optional reevaluation / tombstone helper tables if needed later

Rationale:
- entities first
- relation boundary second
- mechanism body third
- derived outputs after truth layers exist

---

## 4. Migration sketch

### 4.1 Canonical tables

```sql
CREATE TABLE IF NOT EXISTS memory_canonical_entities (
  entity_id TEXT PRIMARY KEY,
  entity_type TEXT NOT NULL,
  canonical_name TEXT NOT NULL,
  status TEXT NOT NULL,
  merge_history_json TEXT NOT NULL DEFAULT '[]',
  split_history_json TEXT NOT NULL DEFAULT '[]',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS memory_canonical_entity_aliases (
  alias_id INTEGER PRIMARY KEY AUTOINCREMENT,
  entity_id TEXT NOT NULL,
  alias_text TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_memory_entity_alias_unique
  ON memory_canonical_entity_aliases(alias_text);
CREATE INDEX IF NOT EXISTS idx_memory_entity_alias_entity
  ON memory_canonical_entity_aliases(entity_id);
```

### 4.2 Relation table

```sql
CREATE TABLE IF NOT EXISTS memory_relations (
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

CREATE UNIQUE INDEX IF NOT EXISTS idx_memory_relations_driver_target_active
  ON memory_relations(driver_entity_id, target_entity_id)
  WHERE status IN ('active','inactive');

CREATE INDEX IF NOT EXISTS idx_memory_relations_driver
  ON memory_relations(driver_entity_id);
CREATE INDEX IF NOT EXISTS idx_memory_relations_target
  ON memory_relations(target_entity_id);
```

### 4.3 Mechanism tables

```sql
CREATE TABLE IF NOT EXISTS memory_mechanisms (
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

CREATE INDEX IF NOT EXISTS idx_memory_mechanisms_relation_asof
  ON memory_mechanisms(relation_id, as_of DESC);

CREATE TABLE IF NOT EXISTS memory_mechanism_nodes (
  mechanism_node_id TEXT PRIMARY KEY,
  mechanism_id TEXT NOT NULL,
  node_type TEXT NOT NULL,
  label TEXT NOT NULL,
  backing_accepted_node_ids_json TEXT NOT NULL DEFAULT '[]',
  sort_order INTEGER,
  created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_memory_mechanism_nodes_mechanism
  ON memory_mechanism_nodes(mechanism_id);

CREATE TABLE IF NOT EXISTS memory_mechanism_edges (
  mechanism_edge_id TEXT PRIMARY KEY,
  mechanism_id TEXT NOT NULL,
  from_node_id TEXT NOT NULL,
  to_node_id TEXT NOT NULL,
  edge_type TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_memory_mechanism_edges_mechanism
  ON memory_mechanism_edges(mechanism_id);

CREATE TABLE IF NOT EXISTS memory_path_outcomes (
  path_outcome_id TEXT PRIMARY KEY,
  mechanism_id TEXT NOT NULL,
  node_path_json TEXT NOT NULL,
  outcome_polarity TEXT NOT NULL,
  outcome_label TEXT NOT NULL,
  condition_scope TEXT,
  confidence REAL NOT NULL,
  created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_memory_path_outcomes_mechanism
  ON memory_path_outcomes(mechanism_id);
CREATE INDEX IF NOT EXISTS idx_memory_path_outcomes_polarity
  ON memory_path_outcomes(outcome_polarity);
```

### 4.4 Derived tables

```sql
CREATE TABLE IF NOT EXISTS memory_driver_aggregates (
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
CREATE INDEX IF NOT EXISTS idx_memory_driver_aggregates_driver_asof
  ON memory_driver_aggregates(driver_entity_id, as_of DESC);

CREATE TABLE IF NOT EXISTS memory_target_aggregates (
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
CREATE INDEX IF NOT EXISTS idx_memory_target_aggregates_target_asof
  ON memory_target_aggregates(target_entity_id, as_of DESC);

CREATE TABLE IF NOT EXISTS memory_conflict_views (
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
CREATE INDEX IF NOT EXISTS idx_memory_conflict_views_scope_asof
  ON memory_conflict_views(scope_type, scope_id, as_of DESC);

CREATE TABLE IF NOT EXISTS memory_cognitive_cards (
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
CREATE INDEX IF NOT EXISTS idx_memory_cognitive_cards_relation_asof
  ON memory_cognitive_cards(relation_id, as_of DESC);

CREATE TABLE IF NOT EXISTS memory_cognitive_conclusions (
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
CREATE INDEX IF NOT EXISTS idx_memory_cognitive_conclusions_source_asof
  ON memory_cognitive_conclusions(source_type, source_id, as_of DESC);

CREATE TABLE IF NOT EXISTS memory_top_items (
  item_id TEXT PRIMARY KEY,
  item_type TEXT NOT NULL,
  headline TEXT NOT NULL,
  subheadline TEXT,
  backing_object_id TEXT NOT NULL,
  signal_strength TEXT NOT NULL,
  as_of TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_memory_top_items_asof
  ON memory_top_items(as_of DESC);
CREATE INDEX IF NOT EXISTS idx_memory_top_items_type_asof
  ON memory_top_items(item_type, as_of DESC);
```

---

## 5. Migration staging plan

### Stage 1: additive tables only
- create all new synthesis tables
- do not backfill yet
- keep cluster-first reads/writes untouched

### Stage 2: test-only backfill helpers
- create fixture builders for canonical entities, relations, mechanisms, path outcomes
- verify organizer can write/read synthesis artifacts with no production migration risk

### Stage 3: live writer introduction
- organization job writes synthesis outputs
- read surfaces remain opt-in / compare-first

### Stage 4: read-surface adoption
- card / compare / organized surfaces may start reading synthesis by default where approved

---

## 6. Data compatibility notes

1. Existing accepted-memory tables stay unchanged.
2. synthesis tables are derived from accepted substrate rather than replacing it.
3. No destructive migration is required for MVP.
4. JSON columns are acceptable for MVP because most heavy traversal happens in application logic, not raw SQL joins.

---

## 7. Tombstone / retraction support

SQLite MVP support does not require dedicated tombstone tables yet if current accepted/source rows can carry invalidation markers elsewhere.

Minimum migration-compatible assumption:
- reevaluation logic reads tombstoned/invalidated state from substrate or source metadata
- downstream synthesis rows are recomputed and rewritten

If explicit tombstone tables are later needed, add:
- `memory_tombstones`
- `memory_reevaluation_jobs`

but do not block MVP on them.

---

## 8. Recommended first migration file shape

A first migration can be structured as:

1. begin transaction
2. create canonical tables
3. create relation table
4. create mechanism tables
5. create derived tables
6. create indexes
7. commit

This keeps failure rollback simple and mirrors dependency order.
