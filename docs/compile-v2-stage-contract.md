# Compile V2 Stage Contract Table

> 目的：明确 compile v2 各个 stage 的输入、输出、字段权威来源，以及下游应该消费哪一版结构。
>
> 这份文档是 `compile-v2-remediation-plan.md` 的第一阶段落地产物。

---

## 1. 总原则

1. **每个 stage 只对一部分字段负责**，避免多处同时改写同一字段。
2. **下游只消费最近一个权威版本**，不要同时混用多个 stage 的同名字段。
3. **关系层改图之后必须重新分类**，否则 role 会失真。
4. **off-graph 的 evidence / explanation / supplementary 必须来自明确的关系判定或显式降级，不允许无来源漂移。**

---

## 2. Stage Contract Table

| Stage | 主要职责 | 输入 | 输出 | 本 stage 的权威字段 | 下游消费者 |
|---|---|---|---|---|---|
| Stage 1 Extract | 从原文召回原子节点、候选边、图外项 | article text / bundle | `graphState{nodes, edges, off_graph}` | `nodes.text`, `nodes.source_quote`, 初始 `edges`, 初始 `off_graph` | Stage 2 |
| Stage 2 Dedup | 语义去重（候选对 + LLM equivalence + union-find）、canonical 选择、重复项下沉 supplementary | Stage1 graph | 更新后的 `graphState` | `nodes` 集合（去重后）、canonical 映射、重写后的 `edges`、由重复项产生的 `supplementary off_graph` | Stage 3 Relations |
| Stage 3 Relations | 对全节点集合一次性判定关系类型并改图 | Stage2 graph | 更新后的 `graphState` | `causal edges`、`evidence/explanation/supplementary off_graph` 的结构归属、哪些节点被降级出主图 | Stage 3 Classify |
| Stage 3 Classify | 给主图节点赋 role / ontology | Stage3Relations graph | 更新后的 `graphState` | `nodes.role`, `nodes.ontology` | Stage 4, Stage 5 |
| Stage 4 Validate | 以段落为单位做覆盖度检查、补漏、纠错 | Stage3Classify graph + article paragraphs | 更新后的 `graphState` | `missing_nodes`, `missing_edges`, `misclassified` 的 patch 合并结果；validate 轮次 `Rounds` | Stage 5 |
| Stage 5 Render | 提取主路径、翻译中文、组装最终 schema | Stage4 graph（若关闭 validate，则消费 Stage3Classify/Relations 后的 graph） | `compile.Output` | 最终 `drivers/targets/transmission_paths/evidence_nodes/explanation_nodes/supplementary_nodes/summary` | CLI / store / downstream |

---

## 3. 每个字段谁说了算

### 3.1 `nodes`

#### `id`
- **初始产出**：Stage 1
- **可修正**：Stage 1 内部补默认 id；Stage 2 在 dedup 时做 canonical 映射
- **最终权威**：**Stage 2** 之后的节点 id 集

#### `text`
- **初始产出**：Stage 1
- **可修正**：Stage 2 在 dedup 时选择 canonical 文本
- **最终权威**：**Stage 2**

#### `source_quote`
- **初始产出**：Stage 1
- **可修正**：Stage 2 选择更长/更可靠的 canonical quote
- **最终权威**：**Stage 2**

#### `role`
- **产出方**：Stage 3 Classify
- **约束**：Stage 3 Relations 改图后，必须重新跑 Stage 3 Classify
- **最终权威**：**最后一次 Stage 3 Classify**

#### `ontology`
- **产出方**：Stage 3 Classify
- **最终权威**：**最后一次 Stage 3 Classify**

---

### 3.2 `edges`

#### Stage 1 `edges`
- 含义：**候选因果边**
- 地位：只是召回结果，不可直接视为最终主图边

#### Stage 2 `edges`
- 含义：去重重定向后的候选边
- 地位：仍然是候选边，不是最终关系图

#### Stage 3 Relations 之后的 `edges`
- 含义：**只保留被判为 `causal` 的主图边**
- **最终权威**：**Stage 3 Relations**

> 结论：
> - Stage1/Stage2 的 edges 是候选
> - **Stage3Relations 的 causal edges 才是权威边**

---

### 3.3 `off_graph`

#### Stage 1 `off_graph`
- 含义：抽取器已经明确觉得不属于主图的 evidence / explanation / supplementary
- 地位：初始草稿

#### Stage 2 新增 `off_graph`
- 来源：重复节点被合并后，非 canonical 成员降成 `supplementary`
- **权威**：Stage 2 对 dedup 衍生 supplementary 拥有写权限

#### Stage 3 Relations 新增 `off_graph`
- 来源：
  - `supports` → evidence
  - `explains` → explanation
  - `supplements` → supplementary
- **权威**：Stage 3 Relations 对关系归属拥有写权限

#### Stage 4 Validate 新增 `off_graph`
- 来源：当前最小版里 `misclassified` 先临时落 supplementary 备注
- 地位：辅助调试/提示，不应覆盖 Stage 3 的正式关系归属

> 结论：
> - `off_graph` 是可累积字段
> - 但 evidence / explanation / supplementary 的**关系来源**以 **Stage 3 Relations** 为主
> - Stage 2 只负责 dedup 衍生 supplementary

---

### 3.4 最终 `compile.Output`

由 **Stage 5 Render** 统一组装，具体字段权威如下：

#### `drivers`
- 来源：最后一次 `role=driver` 的节点
- 权威：**Stage 5 Render**（但依赖最后一次 Stage 3 Classify）

#### `targets`
- 来源：最后一次 `role=target` 的节点
- 权威：**Stage 5 Render**（但依赖最后一次 Stage 3 Classify）

#### `transmission_paths`
- 来源：主图 causal edges 上提取的最短路径
- 权威：**Stage 5 Render**

#### `evidence_nodes`
- 来源：off_graph 中 `role=evidence`
- 权威：**Stage 5 Render**（但内容来源主要来自 Stage 3 Relations）

#### `explanation_nodes`
- 来源：off_graph 中 `role=explanation`
- 权威：**Stage 5 Render**

#### `supplementary_nodes`
- 来源：off_graph 中 `role=supplementary`
- 权威：**Stage 5 Render**

#### `summary`
- 来源：Stage 5 summary call
- 权威：**Stage 5**

---

## 4. 当前强约束（必须遵守）

### 4.1 关系层后必须重分类
顺序必须是：

1. Stage 1 Extract
2. Stage 2 Dedup
3. Stage 3 Classify（初次）
4. Stage 3 Relations（改图）
5. **Stage 3 Classify（再次）**
6. Stage 4 Validate
7. Stage 5 Render

原因：
- Stage 3 Relations 会删边、降节点、挪到 off-graph
- 如果不 reclassify，role 一定漂

---

### 4.2 Stage 5 不能发明主线
- 如果图结构不足，Stage 5 不应硬造主线
- fallback 只能做**最小兜底**，且必须显式可见/可调试
- 长期目标是删除粗暴 fallback

---

### 4.3 Stage 4 的 patch 不是新权威
- Stage 4 只是补漏/纠错
- 它的 patch 必须回流到 Stage 2 / Stage 3 再跑
- 不能跳过 re-dedup / reclassify 直接成为最终结果

---

## 5. 当前已知薄弱点

### 薄弱点 1：Stage 2 仍偏弱
虽然已经不是纯字符串 dedup，但还没到“高质量语义 dedup”的程度。

### 薄弱点 2：Stage 3 Relations 仍可能过重
虽然已经从逐边单 call 改到全节点一次性关系编排，但复杂样本仍需验证稳定性。

### 薄弱点 3：Stage 4 validate 仍是 MVP
目前是最小可运行版，还没有强 patch 质量控制。

### 薄弱点 4：Stage 5 fallback 还未完全清理
目前仍有粗 fallback 的遗留，需要后续收紧。

---

## 6. 推荐后续执行顺序

1. 先按这份 contract 固化代码注释与文档引用
2. 继续增强 Stage 2 dedup
3. 继续增强 Stage 3 relation 质量
4. 再收 Stage 4 validate
5. 最后削弱 Stage 5 fallback

---

## 7. 一句话总结

> Stage 1 和 Stage 2 产出的图，都是“候选图”；
> **Stage 3 Relations 产出的 causal edges 与 off-graph 归属，才是最终结构的关键权威来源；**
> Stage 3 Classify 则负责在该结构上重新标注 driver / transmission / target；
> Stage 5 只负责渲染，不负责定义真相。

