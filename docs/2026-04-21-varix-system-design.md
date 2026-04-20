# VariX 正式系统设计文档

> 日期：2026-04-21
> 状态：正式设计稿（工程主设计）
> 适用范围：VariX 下一阶段主线重构与持续建设
> review 边界：**数据结构与接口设计由你 review；其余工程设计默认按本文推进**
> prompt 边界：**本文只定义 prompt 输入/输出 contract 与调用边界，不设计任何 prompt 内容**

---

## 1. 项目定义

VariX 的核心目标不是“做摘要”，而是把复杂内容转化为**可验证、可记忆、可抽象**的因果认知系统。

完整价值链如下：

`内容 -> compile 成单内容子图 -> verify 这张图 -> 写入单内容 memory -> memory 系统内部抽象事件层 -> memory 系统内部抽象规律层`

其中：

1. **compile**
   - 面向用户：输出一眼能看完的卡片流。
   - 面向系统：输出结构化子图，作为 verify / memory 的唯一中间真相源。
2. **verify**
   - 验证节点真假。
   - 验证边是否成立。
   - 对预测类节点进行延迟验证与周期回查。
3. **memory**
   - 第一层：单内容图记忆。
   - 第二层：事件层图记忆。
   - 第三层：规律层 / 推理范式层。
4. **abstraction**
   - 事件层解决“多篇内容其实在说同一件事”。
   - 规律层解决“多次验证后可以抽象出稳定推理智慧”。

---

## 2. 设计原则

### 2.1 统一真相源原则
系统内部唯一核心中间产物是**子图（subgraph）**。任何 summary、card、event、rule 都必须可追溯回这张图。

### 2.2 时间优先原则
node 必须原生具备时效性。系统不是静态知识库，而是演化中的时态认知图。

### 2.3 主体-变化原则
node 的业务本体不是一段句子，而是：

`主体 + 变化 + 时间 + 验证状态`

原文句子只是来源表达，不是最终语义主键。

### 2.4 验证优先原则
未验证图可以进入系统，但不能直接提升为高层规律。越高层抽象，越依赖验证累积。

### 2.5 工程与 prompt 解耦原则
- 工程层负责：数据结构、调度、存储、状态机、可追溯性、幂等、回查、聚合。
- prompt 层负责：抽取与判定的语义能力。
- 两者通过稳定 contract 连接，互不混杂。

### 2.6 可替换原则
compile / verify / abstraction 的语义实现都必须可以替换，不能把系统绑死在某一套 prompt 或某一条 pipeline 上。

---

## 3. 系统边界

## 3.1 In Scope

- 内容采集与原始内容持久化
- 单内容图 compile
- 图级验证（节点 / 边 / 预测回查）
- 单内容 memory 落库
- 事件层聚合
- 规律层 / 推理范式层构建
- 用户卡片流渲染
- 周期任务、验证队列、组织任务
- traceability / provenance / evidence linkage

## 3.2 Out of Scope

- prompt 设计与调优
- 前端交互细节
- 图数据库迁移（当前阶段不以引入 Neo4j 为目标）
- 自动化交易、自动执行策略
- 通用聊天记忆系统

---

## 4. 总体架构

## 4.1 逻辑分层

### Layer A — Ingest Layer
负责把 URL / 平台内容拉进系统，生成 `RawContent`。

复用现有：
- `varix/bootstrap/`
- `varix/ingest/`
- `varix/storage/contentstore` 中 raw capture 持久化

### Layer B — Compile Layer
负责把单条内容编译成**单内容子图**与前台卡片流材料。

输出：
- 主图节点 / 边
- 辅助节点 / 边
- 卡片渲染所需主链路
- compile 元数据

### Layer C — Verification Layer
负责对 compile 结果进行异步验证与持续回查。

输出：
- node verdict
- edge verdict
- 预测节点回查结果
- 下次回查计划

### Layer D — Content Memory Layer
负责把单内容图作为第一层长期记忆写入系统。

输出：
- article-scoped memory graph
- 记忆接受事件
- 组织任务

### Layer E — Event Memory Layer
负责把多个内容图聚合成事件图。

输出：
- event graph
- canonical entity mapping
- 同事件汇总视图

### Layer F — Rule / Paradigm Layer
负责从验证过的事件与路径中抽取规律和推理范式。

输出：
- paradigm
- confidence / credibility
- supporting evidence set
- failure penalty history

### Layer G — Presentation Layer
负责把 compile 与 memory 结果转成用户能快速读取的 card flow。

输出：
- compile card
- event card
- paradigm card

---

## 4.2 运行流

### 4.2.1 内容进入主线
1. ingest 抓取内容
2. raw capture 落库
3. compile job 触发
4. compile 输出单内容子图
5. compile 子图落库
6. verify queue 根据图内容排队
7. content memory 接收该图
8. memory 系统异步组织事件层与规律层

### 4.2.2 预测回查主线
1. verify scheduler 扫描 `pending` 节点与边
2. 判断是否到达 `next_verify_at`
3. 拉取最新观测证据
4. 执行验证
5. 更新 verdict
6. 若仍无法判定，重排下次验证时间
7. 若 verdict 变化影响 memory 抽象，触发 refresh

---

## 5. 核心领域模型（此章为 review 重点）

> 本章是你需要重点 review 的部分。
> 我会按此设计工程主干，但结构和接口由你最后拍板。

## 5.1 统一子图模型

一个内容被 compile 后，系统内部必须得到一张 `ContentSubgraph`。

它不是 UI 数据，而是领域真相源。

---

## 5.2 Node 设计

### 5.2.1 业务定义
一个 node 表示：

**某个主体在某个时间范围内发生了某种变化，或被预测将发生某种变化。**

例如：
- 美联储 / 加息 25bp / 2026Q2
- 美股 / 下跌 / 最近一周
- 伊朗战争 / 升级 / 最近一周

### 5.2.2 Node 标准结构

```ts
export type NodeKind = "observation" | "prediction";

export type VerificationStatus =
  | "pending"
  | "proved"
  | "disproved"
  | "unverifiable";

export type GraphNode = {
  id: string;

  // 原文来源
  source_article_id: string;
  source_platform: string;
  source_external_id: string;
  source_quote?: string;
  source_text_span?: string;

  // 核心语义
  subject_text: string;
  subject_canonical?: string;
  change_text: string;
  change_kind?: string;
  change_direction?: "up" | "down" | "flat" | "mixed" | "unknown";
  change_value?: number;
  change_unit?: string;

  // 时间
  time_text?: string;
  time_start?: string;   // ISO8601
  time_end?: string;     // ISO8601
  time_bucket?: "intraday" | "1d" | "1w" | "1m" | "1q" | "1y" | "custom";

  // 图角色
  kind: NodeKind;
  graph_role?: "driver" | "target" | "intermediate" | "evidence" | "context";
  is_primary: boolean;

  // 验证
  verification_status: VerificationStatus;
  verification_reason?: string;
  verification_as_of?: string;
  next_verify_at?: string;
  last_verified_at?: string;

  // 观测目标（供 verify 使用）
  verification_target?: {
    metric?: string;
    subject?: string;
    comparator?: "eq" | "gt" | "gte" | "lt" | "lte" | "contains" | "trend";
    expected_value_text?: string;
    expected_value_num?: number;
    expected_unit?: string;
    evaluation_window_start?: string;
    evaluation_window_end?: string;
  };

  // 置信度
  compile_confidence?: number;
  verify_confidence?: number;
};
```

### 5.2.3 设计说明

1. **`subject_text + change_text` 是当前 compile 层最重要的结构化结果**。
2. `subject_canonical` 不要求 compile 阶段完全正确，可以在 memory 阶段增强。
3. `kind` 与 `verification_status` 必须分离：
   - prediction 不是一种 verdict
   - pending 也不是 prediction 专属
4. `verification_target` 是验证调度的关键扩展位，用来支持未来自动化验证。

---

## 5.3 Edge 设计

### 5.3.1 业务定义
edge 表示两个 node 的关系。

你已经明确：
- 主边只有一种：`A drives B`
- 但系统内部允许存在辅助关系

### 5.3.2 Edge 标准结构

```ts
export type EdgeType =
  | "drives"
  | "supports"
  | "explains"
  | "context";

export type GraphEdge = {
  id: string;
  from: string;
  to: string;

  type: EdgeType;
  is_primary: boolean;

  confidence?: number;

  verification_status?: VerificationStatus;
  verification_reason?: string;
  verification_as_of?: string;
  next_verify_at?: string;
  last_verified_at?: string;
};
```

### 5.3.3 设计说明

1. 用户默认只看 `is_primary = true && type = drives` 的边。
2. verify 会验证边是否成立，因此 edge 也具备独立 verdict。
3. 规律层主要统计主边，不直接依赖辅助边。

---

## 5.4 ContentSubgraph 设计

```ts
export type ContentSubgraph = {
  id: string;
  article_id: string;
  source_platform: string;
  source_external_id: string;
  root_external_id?: string;

  nodes: GraphNode[];
  edges: GraphEdge[];

  compile_version: string;
  compiled_at: string;
  updated_at: string;
};
```

---

## 5.5 Card 输出模型

### 5.5.1 原则
card 是图的投影，不是另一个真相源。

### 5.5.2 默认展示
- 只显示主图 primary nodes
- 只显示主边 drives
- 只显示主逻辑链

### 5.5.3 展开展示
- 辅助节点
- explain/support/context 关系
- 验证信息
- 原文引用

### 5.5.4 CompileCard 结构

```ts
export type CompileCard = {
  card_id: string;
  article_id: string;
  title: string;
  summary: string;
  primary_node_ids: string[];
  primary_edge_ids: string[];
  main_paths: string[][];
  evidence_refs?: string[];
  compact_view: boolean;
};
```

---

## 5.6 Verify 状态机（此章为 review 重点）

### 5.6.1 最终状态集

```ts
pending -> proved | disproved | unverifiable
```

### 5.6.2 语义定义
- `pending`
  - 尚未到验证窗口，或虽然到窗口但证据仍不足
- `proved`
  - 当前证据足以支持该节点/边成立
- `disproved`
  - 当前证据足以说明该节点/边不成立
- `unverifiable`
  - 无法构造可执行验证，或长期缺乏足够观测渠道

### 5.6.3 调度规则

#### 对 node
- 新 compile 进入系统时默认可先是 `pending`
- 预测型节点通常初始是 `pending`
- 到达 `next_verify_at` 时进入验证 worker
- 若仍无法判定：
  - 保持 `pending`
  - 推迟 `next_verify_at`
- 若已经获得明确结果：
  - 转 `proved` / `disproved` / `unverifiable`

#### 对 edge
- 主边 `drives` 也进入验证队列
- 边的验证结果不一定跟节点 verdict 同步
- 边被 `disproved` 时，不要求立即删除节点，但会影响：
  - 主图展示
  - 事件层路径可信度
  - 规律层范式可信度

---

## 6. Memory 分层设计

## 6.1 第一层：单内容 memory

### 6.1.1 定义
单内容 memory 是 article-scoped graph memory。

### 6.1.2 作用
- 保存原始 compile 图
- 保存 verify 后的节点/边状态
- 提供事件层抽象的原材料
- 保留 provenance 与回溯能力

### 6.1.3 存储原则
- 不做激进覆盖式重写
- compile 结果是内容事实快照
- verify verdict 是覆盖层，不直接抹掉原始图

---

## 6.2 第二层：事件层 memory

### 6.2.1 定义
事件层把多篇文章中关于同一事件主体的内容聚合。

### 6.2.2 聚合锚点
你已明确两类主键：
- `(同一 driver 主体 + 时间桶)`
- `(同一 target 主体 + 时间桶)`

工程上事件层不要求只有一种入口，而是允许形成两类 event index：
- driver-centric event
- target-centric event

### 6.2.3 事件层核心对象

```ts
export type EventGraph = {
  event_id: string;
  event_scope: "driver" | "target";
  anchor_subject_canonical: string;
  time_bucket: string;

  source_subgraph_ids: string[];
  canonical_node_ids: string[];
  canonical_edge_ids: string[];

  summary_node_ids: string[];
  summary_edge_ids: string[];

  confidence?: number;
  updated_at: string;
};
```

### 6.2.4 难点
- 主体 canonicalization
- 同一时间桶内可能存在涨跌并存
- 事件层不是简单多数投票，要保留冲突与分歧

### 6.2.5 处理原则
- 同一事件下允许存在多个相反 observation node
- 不在事件层强行抹平冲突
- 事件层是**汇总图**，不是裁判层

---

## 6.3 第三层：规律层 / 推理范式层

### 6.3.1 定义
规律层不是文章总结，而是从 memory 中抽出的**可复用推理范式**。

例如：
- 央行收紧 -> 流动性收缩 -> 风险资产承压
- 地缘冲突升级 -> 能源价格上行 -> 通胀压力抬升 -> 利率预期上行

### 6.3.2 范式对象

```ts
export type Paradigm = {
  paradigm_id: string;
  name: string;
  pattern_type: "edge_pattern" | "path_pattern" | "subgraph_pattern";

  canonical_subjects?: string[];
  pattern_nodes: string[];
  pattern_edges: string[];

  supporting_event_ids: string[];
  supporting_subgraph_ids: string[];

  credibility_score: number;
  credibility_state: "latent" | "candidate" | "explicit" | "degraded";

  success_count: number;
  failure_count: number;
  last_recomputed_at: string;
};
```

### 6.3.3 可信度机制

- 每次验证成功：增加支持权重
- 每次验证失败：显著降低可信度
- 可信度达到阈值：从 `latent/candidate` 提升为 `explicit`
- 若持续失败：进入 `degraded`

### 6.3.4 核心原则
规律层必须来源于：
- 已验证或部分验证的事件层/内容层数据
- 可追溯的 supporting evidence set

不能由 prompt 单次“拍脑袋总结”直接生成长期智慧。

---

## 7. Compile 系统设计

## 7.1 Compile 的职责
compile 负责：
1. 从长内容抽取节点与边
2. 识别主图与辅助图
3. 形成统一子图
4. 输出卡片所需主链路

不负责：
- 最终真实性裁判
- 长期规律抽象

## 7.2 Compile 输出 contract
compile 必须输出：
- `ContentSubgraph`
- `CompileCard`
- compile metadata

## 7.3 Prompt 边界
compile 工程层只定义：
- stage 输入
- stage 输出
- 失败与重试策略
- parse / validate / normalize 规则

所有 prompt 内容由你负责。

## 7.4 推荐 stage 形态

### Stage 1 Extract
- 抽 node / edge 候选
- 识别原文引用

### Stage 2 Normalize
- 做主体-变化初步结构化
- 填充时间字段
- 去重

### Stage 3 Primary Graph Decide
- 标出主图节点/边
- 识别辅助图

### Stage 4 Render Materialize
- 产出 card 所需主逻辑链
- 产出 subgraph payload

---

## 8. Verification 系统设计

## 8.1 Verify 的职责
verify 负责三种验证：

1. **Node verification**
   - 这个主体变化是否成立
2. **Edge verification**
   - 这个驱动关系是否成立
3. **Prediction verification**
   - 预测是否已经被现实验证 / 证伪 / 仍未决

## 8.2 Verify worker 架构

### VerifyQueue
负责维护待验证对象。

```ts
export type VerifyQueueItem = {
  id: string;
  object_type: "node" | "edge";
  object_id: string;
  source_article_id: string;
  priority: number;
  scheduled_at: string;
  attempts: number;
  last_error?: string;
  status: "queued" | "running" | "done" | "retry";
};
```

### VerifyResult

```ts
export type VerifyVerdict = {
  object_type: "node" | "edge";
  object_id: string;
  verdict: VerificationStatus;
  reason?: string;
  evidence_refs?: string[];
  as_of: string;
  next_verify_at?: string;
};
```

## 8.3 核心流程
1. scheduler 扫描 due items
2. worker 拉取 object 与上下文
3. 调用 verifier adapter
4. 解析 verdict
5. 回写 object status
6. 触发 memory refresh（如 verdict materially changes）

## 8.4 工程原则
- verify 结果必须可追踪到 evidence refs
- verify 必须幂等
- verify queue 不能因为单个对象长期 pending 而阻塞
- 预测节点可以无限次重排，直到有足够证据或被人工/系统标记为 `unverifiable`

---

## 9. 存储设计

## 9.1 总体策略
继续使用 **Go + SQLite** 作为当前阶段主存储。

原因：
- 现有工程已大面积依赖 SQLite store
- 当前阶段主瓶颈不是图数据库能力，而是领域模型稳定性
- 引入外部图数据库会放大迁移成本与建模噪音

Graph DB 可以是未来扩展项，而不是当前阶段前提。

---

## 9.2 建议表结构

### 9.2.1 内容图表

#### `content_subgraphs`
- `subgraph_id`
- `platform`
- `external_id`
- `root_external_id`
- `compile_version`
- `payload_json`
- `compiled_at`
- `updated_at`

#### `content_graph_nodes`
- `node_id`
- `subgraph_id`
- `subject_text`
- `subject_canonical`
- `change_text`
- `change_kind`
- `change_direction`
- `change_value`
- `change_unit`
- `kind`
- `graph_role`
- `is_primary`
- `time_start`
- `time_end`
- `time_bucket`
- `verification_status`
- `verification_reason`
- `verification_as_of`
- `next_verify_at`
- `compile_confidence`
- `verify_confidence`
- `source_quote`
- `created_at`
- `updated_at`

#### `content_graph_edges`
- `edge_id`
- `subgraph_id`
- `from_node_id`
- `to_node_id`
- `edge_type`
- `is_primary`
- `confidence`
- `verification_status`
- `verification_reason`
- `verification_as_of`
- `next_verify_at`
- `created_at`
- `updated_at`

### 9.2.2 验证队列表

#### `verify_queue`
- `queue_id`
- `object_type`
- `object_id`
- `source_platform`
- `source_external_id`
- `priority`
- `scheduled_at`
- `status`
- `attempts`
- `last_error`
- `created_at`
- `updated_at`

#### `verify_verdict_history`
- `verdict_id`
- `object_type`
- `object_id`
- `verdict`
- `reason`
- `evidence_refs_json`
- `as_of`
- `next_verify_at`
- `created_at`

### 9.2.3 单内容 memory 表

保留现有 acceptance event 思路，但从“只接 node”升级为“图快照 + 节点/边索引”。

#### `memory_content_graphs`
- `memory_graph_id`
- `user_id`
- `subgraph_id`
- `accepted_at`
- `payload_json`
- `created_at`

### 9.2.4 事件层表

#### `event_graphs`
- `event_id`
- `event_scope`
- `anchor_subject_canonical`
- `time_bucket`
- `payload_json`
- `confidence`
- `updated_at`

#### `event_graph_sources`
- `event_id`
- `subgraph_id`
- `contribution_type`

### 9.2.5 范式层表

#### `paradigms`
- `paradigm_id`
- `name`
- `pattern_type`
- `credibility_score`
- `credibility_state`
- `success_count`
- `failure_count`
- `payload_json`
- `updated_at`

#### `paradigm_evidence_links`
- `paradigm_id`
- `event_id`
- `subgraph_id`
- `weight`

---

## 10. 与现有代码的衔接策略

## 10.1 直接保留

### Ingest 主链路
保留：
- `varix/bootstrap/`
- `varix/ingest/`
- raw capture / dispatcher / polling / provenance 基础能力

### CLI 框架
保留：
- `varix/cmd/cli/main.go`
- `compile / verify / memory` 命令组入口

### SQLite 主存储框架
保留：
- `varix/storage/contentstore/` 的 store 组织方式
- migration 机制
- `compiled_outputs` / `verification_results` 的演进思路

---

## 10.2 保留外壳，重构内核

### Compile output model
当前 `compile.Output` / `ReasoningGraph` 可继续存在，但要作为**兼容层**，而不是未来主领域模型。

建议：
- 新建统一 graph domain model
- 旧 `compile.Output` 由新模型渲染生成

### Verify
保留：
- `verify run/show`
- verifier stage orchestration

重构：
- verification target 定义
- pending scheduler
- edge verification 常态化

### Memory
保留：
- acceptance event / organization job / posterior refresh 这一套任务模式

重构：
- source of truth 从“accepted nodes”为主，升级到“accepted content graph”为主
- 事件层 / 规律层从文本 heuristic 为主，升级到 graph abstraction 为主

---

## 10.3 建议新增模块

### `varix/graphmodel/`
统一领域模型：
- node
- edge
- subgraph
- event graph
- paradigm

### `varix/graphstore/`
图对象的持久化与读写 adapter。

### `varix/verifyqueue/`
验证队列、scheduler、worker contract。

### `varix/eventmem/`
事件层聚合逻辑。

### `varix/paradigm/`
规律层抽象与可信度计算。

### `varix/render/`
compile card / event card / paradigm card 的投影层。

---

## 11. 外部项目复用决策

## 11.1 采用策略
不引入外部项目作为主引擎，只借其局部能力或设计思想。

### Graphiti
用途：
- 参考 temporal context graph / validity window / provenance 模型

不采用原因：
- Python + graph DB 路线与当前工程主干不匹配

### OpenFactVerification / Loki
用途：
- 参考 claim decomposition / evidence retrieval / verification pipeline

不采用原因：
- 它是 claim-centric，不是 subgraph-centric
- 适合作 verify 子能力，不适合作主干

### GraphRAG
用途：
- 参考长文图化 pipeline 思路

不采用原因：
- 更偏 corpus indexing 与 retrieval
- 对当前单内容 causal graph 主线不匹配

### TemporalFC
用途：
- 参考 temporal fact checking 与 time-point prediction 研究思路

不采用原因：
- 更像研究工程，不像当前可直接接生产的模块

---

## 12. 非功能性要求

## 12.1 可追溯性
任一 event / paradigm 都必须追溯到：
- source subgraph
- node / edge
- verdict evidence

## 12.2 幂等性
- compile 重跑不能产生重复对象
- verify 重试不能重复写脏状态
- memory refresh 必须幂等

## 12.3 可观测性
需要记录：
- compile stage duration
- verify queue length
- pending age distribution
- event graph rebuild duration
- paradigm recompute duration

## 12.4 降级策略
- compile 失败：允许仅保留 raw capture
- verify 失败：保持 pending + retry
- event aggregation 失败：不阻塞单内容 memory
- paradigm build 失败：不阻塞 event layer

---

## 13. 测试策略

## 13.1 单元测试
- node / edge normalization
- verification status transition
- queue scheduling
- event bucketing
- paradigm credibility update

## 13.2 集成测试
- raw content -> compile -> graph persist
- graph -> verify queue -> verdict writeback
- subgraph -> memory content graph -> event graph
- event graph -> paradigm refresh

## 13.3 回归测试
- 现有 compile card 不崩
- 现有 verify run/show 不崩
- 现有 memory 读写命令保持基础兼容

## 13.4 数据验收测试
- 单篇内容能输出主图与辅助图
- pending 预测节点能被成功调度
- 同一 driver/target + 时间桶可形成 event graph
- 验证成功/失败可改变 paradigm credibility

---

## 14. 迁移原则

## 14.1 迁移方式
采用**兼容迁移**，不做硬切换。

### 阶段 1
- 保留旧 compile / memory surfaces
- 新建 graph domain model 与新表

### 阶段 2
- compile 同时写旧输出与新 subgraph
- verify 开始读取新 graph model

### 阶段 3
- content memory 改写为 graph-first
- event layer 基于新 graph model 构建

### 阶段 4
- 规律层上线
- 旧 heuristic thesis builder 逐步退居兼容层或调试层

---

## 15. 明确的人机分工

## 15.1 你负责
- 数据结构 review
- 接口设计 review
- prompt 设计与调优
- 最终业务语义拍板

## 15.2 我负责
- 工程主架构
- 存储与迁移
- 队列与状态机
- verify 调度
- memory 分层工程化
- 事件层 / 范式层的非-prompt实现
- 测试与交付

---

## 16. 本文后的执行约束

在未被你否决前，后续工程默认按以下约束执行：

1. 统一子图是主真相源。
2. node 以“主体 + 变化 + 时间 + 验证状态”为核心。
3. edge 默认主边为 `drives`，辅助边只做系统内部支撑。
4. 单内容 memory 先落地，再由系统内部抽事件层与规律层。
5. prompt 不进入本轮工程设计与实现。
6. 保留现有 Go + SQLite 主干，不以引入外部图数据库为前提。

---

## 17. 需要你 review 的唯一高优先级章节

如果你只优先 review 三个章节，请先看：

1. **第 5 章：核心领域模型**
2. **第 6 章：Memory 分层设计**
3. **第 9 章：存储设计**

这三章一旦定稿，剩余工程我可以直接推进。

