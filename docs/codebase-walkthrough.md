# VariX 代码导读

这份文档的目标不是逐文件穷举，而是帮你快速建立一个“系统是怎么流动的”整体心智模型。建议按“主链路”阅读，而不是按目录机械浏览。

---

## 1. 用一句话理解 VariX

VariX 是一个把外部内容抓进来，抽成结构化推理图，做验证，再进入记忆与组织层，最终支持后验修正的系统。

主链路可以先记成：

`ingest -> compile -> verify -> accept memory -> organize memory -> posterior verify`

---

## 2. 仓库顶层结构

项目根目录里最值得关心的是：

- `varix/`：Go 主模块，核心代码都在这里
- `docs/`：当前行为、设计边界、review 辅助文档
- `data/`：本地 sqlite 数据库等运行态数据
- `.omx/`：OMX 运行态状态、计划、上下文、团队执行痕迹
- `README.md`：当前 compile persistence 和 memory organization 的高层说明

如果你只想看真正的业务代码，主要盯 `varix/`。

---

## 3. 系统主链路

### 3.1 ingest：内容怎么进系统

入口关注：
- `varix/ingest/dispatcher/`
- `varix/ingest/polling/`
- `varix/ingest/sources/`
- `varix/ingest/provenance/`
- `varix/ingest/types/`

#### 关键概念
- `dispatcher`：根据 URL / 平台类型选择合适 source
- `sources/*`：各平台抓取器（youtube / bilibili / twitter / weibo / web / rss / search）
- `polling.Service`：真正执行 fetch/poll、持久化 raw capture 的中枢
- `provenance`：来源候选、来源判断、evidence 记录
- `types`：ingest 层统一数据结构

#### 最关键文件
- `varix/ingest/polling/service.go`

这是 ingest 执行中心，负责：
- Parse URL
- FetchByParsedURL / FetchDiscoveryItem
- hydrate references
- localize attachments
- preserve stored capture quality
- preserve stored provenance
- persist raw captures
- mark processed

一句话：

> 如果你想知道“内容是如何真正进入数据库的”，先看 `polling/service.go`。

#### 另一个关键点：provenance
- `varix/ingest/provenance/service.go`
- `varix/ingest/provenance/evidence.go`

这里处理：
- source candidate 合并
- source lookup 状态
- evidence 追加
- fidelity / relation / editorial layer

目前 evidence 追加已经统一走：
- `provenance.AppendEvidence(...)`

这是为了避免过去 dispatcher / polling 各自实现 helper 导致的语义漂移。

---

### 3.2 compile：内容怎么被抽象成“认知图”

入口关注：
- `varix/compile/client.go`
- `varix/compile/prompt.go`
- `varix/compile/result.go`
- `varix/compile/verifier.go`
- `varix/compile/verifier_retrieval.go`

#### `client.go`
compile 总控。

当前行为：
- 先跑一次 compile
- 如果 parse/validate 不满足要求，再 retry 一次
- 成功后进入 verifier

#### `prompt.go`
定义 compile prompt 规则。

这里规定了：
- 节点类型
- 边类型
- 节点拆分策略
- 长文最小节点数 / 边数
- 如何避免“胖节点”

如果你想看“模型被要求抽成什么结构”，这里最关键。

#### `result.go`
compile 的核心 contract。

这里定义：
- `GraphNode`
- `GraphEdge`
- `Output`
- `Verification`
- 各类 status enum

这是 compile / verify / memory 三层的桥梁。

---

### 3.3 verify：结构化图怎么被验证

核心文件：
- `varix/compile/verifier.go`
- `varix/compile/verifier_retrieval.go`

#### 当前状态
verify 已经不是纯单轮判断了。

当前更准确地说：
- facts：已进入 `verify_v2` 路径
  - `claim -> challenge -> adjudication`
- 其他类型：
  - 进入 pass/coverage 元数据框架
  - 仍然保留更轻量的单 pass 结构

#### 为什么这层重要
这层决定：
- compile 图里的哪些节点更可信
- 哪些只是暂时 unresolved
- 后续 memory 接受时该如何看待这些节点

#### `verifier_retrieval.go`
这里给 facts verifier 提供 retrieval 增强。

作用：
- 搜外部网页
- 把网页摘要放进 verifier 上下文

所以 facts verify 已经不是完全闭门判断。

---

### 3.4 memory：哪些节点进入长期记忆

入口关注：
- `varix/storage/contentstore/sqlite_memory.go`
- `varix/memory/organization_types.go`
- `varix/storage/contentstore/sqlite_memory_organizer.go`
- `varix/cmd/cli/memory_commands.go`

#### `sqlite_memory.go`
负责 accepted memory 主入口。

核心职责：
- `AcceptMemoryNodes(...)`
- 查询用户 memory
- 创建 acceptance events
- 入队 organization jobs

一句话：

> 这里是“节点什么时候正式进入 memory”的入口。

#### `organization_types.go`
这里定义 source-scoped memory organization 的输出结构。

例如：
- `AcceptedNode`
- `NodeHint`
- `OrganizationOutput`

现在 posterior state 也已经投影到这里。

#### `sqlite_memory_organizer.go`
这是 memory 的组织器。

它会：
- 读 accepted nodes
- 取 compile record
- dedupe
- contradiction grouping
- hierarchy 推导
- open questions
- node hints
- active / inactive 划分

一句话：

> 它不是“存储层”，而是“把记忆整理成可读结构”的引擎。

---

### 3.5 posterior verify：记忆如何事后修正

这是当前最重要的新 frontier。

入口关注：
- `varix/memory/posterior_types.go`
- `varix/storage/contentstore/sqlite_memory_posterior.go`
- `varix/storage/contentstore/sqlite_memory_organizer.go`

#### 目标
解决的问题是：

> conclusion / prediction 进入 memory 后，不能永远停留在一个抽象但未复验的状态，否则长期会让 memory 失效。

#### 当前 phase-1 方向
设计上已经明确：
- facts 不进入 posterior lifecycle
- conclusion / prediction 才进入
- condition 是 gate，不是独立 lifecycle 主体

核心状态包括：
- `pending`
- `verified`
- `falsified`
- `blocked`

#### `posterior_types.go`
posterior 的 canonical 类型定义在这里。

例如：
- `PosteriorState`
- `PosteriorDiagnosisCode`
- `PosteriorStateRecord`
- `PosteriorRunResult`

当前要记住一句：

> posterior state 的唯一 source of truth 应该在这里。

#### `sqlite_memory_posterior.go`
posterior phase-1 的持久化与 runner 核心。

它已经在处理：
- posterior sidecar table
- pending seeding
- store-layer posterior run（`RunPosteriorVerification`）
- posterior refresh trigger
- state mutation

也就是说，这层已经不只是文档设计，已经开始进入真正实现。  
但要注意：**当前 operator surface 还没有完全闭环**——CLI 里还没有独立
`memory posterior-run` 命令，`memory list` / `memory show-source` 也还没有把
posterior projection 直接暴露给终端读路径；phase-1 目前最完整的成品面仍然是
store seam + organizer/stale-output guard。

---

## 4. CLI 层：你平时真正操作的入口

关注：
- `varix/cmd/cli/main.go`
- `varix/cmd/cli/ingest_commands.go`
- `varix/cmd/cli/compile_commands.go`
- `varix/cmd/cli/memory_commands.go`
- `varix/cmd/cli/main_test.go`

### `ingest_commands.go`
包括：
- `ingest fetch`
- `ingest poll`
- `provenance-run`

注意一个非常重要的事实：

> `ingest fetch` 只是把 fetched items 打到 stdout；真正 persistence seam 是 `Polling.FetchURL` / `Polling.Poll`。

### `compile_commands.go`
包括：
- `compile run`
- `compile show`
- `compile summary`
- `compile compare`
- `compile card`

这是观察 compile/verify 结果的主要操作面。

### `memory_commands.go`
包括：
- `memory accept`
- `memory accept-batch`
- `memory organize-run`
- `memory organized`
- `global-v2-*`

也就是说，posterior 的 store 能力已经存在，但 CLI operator surface 目前仍以
accept / organize / organized 为主；如果后面把 `posterior-run` 正式暴露出来，
入口大概率还是扩在这里。

### `main_test.go`
非常值得重视。

它不只是 CLI 单测，而是一个大型 integration harness 文件。
最近已经被用来串：
- fake app
- fake compile client
- real sqlite
- ingest → compile → memory 的 no-network 测试链路

如果你想快速理解“系统层面的主路径”，它比只看单元测试更有价值。

---

## 5. 现在项目最值得盯住的几个控制点

如果你没时间全看，我建议优先盯下面几个文件：

1. `varix/ingest/polling/service.go`
   - 内容如何进入系统

2. `varix/compile/client.go`
   - compile / verify 总控

3. `varix/storage/contentstore/sqlite_memory.go`
   - memory 接受生命周期入口

4. `varix/storage/contentstore/sqlite_memory_organizer.go`
   - memory 组织输出引擎

5. `varix/storage/contentstore/sqlite_memory_posterior.go`
   - 当前最重要的新 frontier

6. `varix/cmd/cli/main_test.go`
   - 系统主路径 integration harness

---

## 6. 建议阅读顺序

### 路线 A：先重新找回全局感
1. `README.md`
2. `varix/ingest/types/raw_content.go`
3. `varix/ingest/polling/service.go`
4. `varix/compile/result.go`
5. `varix/compile/client.go`
6. `varix/storage/contentstore/sqlite_memory.go`
7. `varix/storage/contentstore/sqlite_memory_organizer.go`
8. `varix/storage/contentstore/sqlite_memory_posterior.go`
9. `varix/cmd/cli/main_test.go`

### 路线 B：只盯当前最重要战场
如果你只想抓现在最关键的问题，建议直接读：
1. `varix/memory/posterior_types.go`
2. `varix/storage/contentstore/sqlite_memory_posterior.go`
3. `varix/storage/contentstore/sqlite_memory_organizer.go`
4. `varix/memory/organization_types.go`
5. `varix/cmd/cli/memory_commands.go`

---

## 7. 当前项目状态（一句话）

当前项目不是“没有结构”，而是已经进入一个新的阶段：

> ingest / compile / memory 主链基本成型，真正需要强收口的是 posterior verification 对 memory 的闭环。

所以你后面如果重新推进，不要同时拉太多新战线。最稳的办法是：
- 把 posterior phase-1 跑通
- 让它真正影响 memory organization
- 然后再考虑更深的 memory abstraction 或更多内容类型

---

## 8. 相关文档索引

如果你要继续深入看设计文档，可以从这里接：

- `docs/compile-persistence.md`
- `docs/compile-node-splitting.md`
- `docs/memory-organization.md`
- `docs/memory-posterior-phase1.md`
- `docs/memory-review-cheatsheet.md`


---

## 9. 数据流图版（从外部内容到记忆）

下面这张图不是精确调用栈，而是帮助你快速建立“数据怎么流”的地图。

```text
┌──────────────────────┐
│ 外部内容 URL / Source │
└──────────┬───────────┘
           │
           ▼
┌─────────────────────────────┐
│ ingest/router + dispatcher  │
│ - 解析 URL                  │
│ - 判断平台/内容类型         │
│ - 路由到具体 source         │
└──────────┬──────────────────┘
           │
           ▼
┌─────────────────────────────┐
│ ingest/sources/*            │
│ - youtube / bilibili / web  │
│ - weibo / twitter / rss     │
│ - metadata / transcript     │
└──────────┬──────────────────┘
           │
           ▼
┌─────────────────────────────┐
│ ingest/polling/service.go   │
│ - FetchURL / Poll           │
│ - hydrate refs              │
│ - preserve quality          │
│ - persist raw captures      │
└──────────┬──────────────────┘
           │
           ▼
┌─────────────────────────────┐
│ raw_captures (sqlite)       │
└──────────┬──────────────────┘
           │
           ▼
┌─────────────────────────────┐
│ compile/client.go           │
│ - compile                   │
│ - retry once if needed      │
│ - run verifier              │
└──────────┬──────────────────┘
           │
           ▼
┌─────────────────────────────┐
│ compile/result.go           │
│ - summary                   │
│ - topics                    │
│ - graph(nodes/edges)        │
│ - verification              │
└──────────┬──────────────────┘
           │
           ▼
┌─────────────────────────────┐
│ compiled_outputs (sqlite)   │
└──────────┬──────────────────┘
           │
           ▼
┌─────────────────────────────┐
│ memory accept / accept-batch│
│ sqlite_memory.go            │
│ - accepted nodes            │
│ - acceptance events         │
│ - organization jobs         │
└──────────┬──────────────────┘
           │
           ▼
┌─────────────────────────────┐
│ user_memory_nodes           │
│ memory_acceptance_events    │
│ memory_organization_jobs    │
└──────────┬──────────────────┘
           │
           ▼
┌─────────────────────────────┐
│ sqlite_memory_organizer.go  │
│ - dedupe                    │
│ - contradiction             │
│ - hierarchy                 │
│ - hints / open questions    │
└──────────┬──────────────────┘
           │
           ▼
┌─────────────────────────────┐
│ organized memory output     │
│ - source scoped             │
│ - global v1/v2              │
└──────────┬──────────────────┘
           │
           ▼
┌─────────────────────────────┐
│ posterior verification      │
│ sqlite_memory_posterior.go  │
│ - pending/verified/...      │
│ - posterior-run             │
│ - refresh trigger           │
└──────────┬──────────────────┘
           │
           ▼
┌─────────────────────────────┐
│ updated organizer output    │
│ - posterior state affects   │
│   hints / display / status  │
└─────────────────────────────┘
```

### 你可以把它压成三句话
1. ingest 负责把内容安全、稳定地抓进 sqlite。  
2. compile/verify 负责把内容变成结构化推理图。  
3. memory/posterior 负责把图变成长期可组织、可修正的认知对象。  

---

## 10. 沿一条 YouTube 内容走一遍（实战例子）

这里用当前项目里已经跑过的一条 YouTube 做例子：

- URL: `https://www.youtube.com/watch?v=62iEHRT36Qg`
- external_id: `62iEHRT36Qg`

目标：让你看到它从 URL 进入系统之后，每一层会发生什么。

### Step 1：URL 进入 ingest
相关代码：
- `varix/ingest/dispatcher/service.go`
- `varix/ingest/sources/youtube/*`
- `varix/ingest/polling/service.go`

系统先做：
1. 解析 URL，识别它是 `youtube` 内容
2. 交给 YouTube source 抓取 metadata / transcript
3. 统一整理成 `types.RawContent`
4. 通过 `Polling.FetchURL(...)` 落库

这时它会进入：
- `raw_captures`

你可以把这一层理解成：
> “先把世界上的原始内容，稳定地变成系统内部统一原料。”

---

### Step 2：compile 把 raw content 抽成结构化图
相关代码：
- `varix/compile/client.go`
- `varix/compile/prompt.go`
- `varix/compile/result.go`

系统会生成：
- `summary`
- `topics`
- `graph.nodes`
- `graph.edges`
- `verification`

这条 YouTube 最近一次 rerun 的核心结果是：

#### 一句话总结
> 全球高债务与增长放缓正削弱信用货币体系的信任基础，促使资本向实物资产再配置及主权国家转向地缘博弈；同时居民收入预期转弱将挤出资产乐观溢价，若宏观保障力度不足，经济将面临内需收缩与预防性储蓄上升的压力。

#### topics
- 宏观债务周期
- 信用货币体系
- 资产配置再平衡
- 居民资产负债表
- 地缘经济博弈

#### 图结构规模
- nodes: `9`
- edges: `7`

---

### Step 3：图里具体会长什么样
抽出来的主要节点是：

#### facts
- `n1` 全球主要经济体债务规模持续扩张，但自然经济增长动能已显著放缓
- `n2` 现代金融财富本质是基于债务人未来现金流承诺的信用凭证
- `n7` 房地产等核心资产的估值高度依赖居民未来收入预期与偿债能力

#### implicit condition
- `n3` 债务人的实际收入增长与税基扩张无法覆盖存量债务的本息偿付需求

#### explicit condition
- `n5` 若高债务经济体无法通过生产力突破或财政整顿实现内部收支平衡

#### conclusions
- `n4` 信用货币体系的信任基础正在削弱，金融财富面临挤兑与有序贬值风险
- `n8` 居民收入预期转弱将直接挤出资产估值中的乐观溢价，引发资产价格重估

#### predictions
- `n6` 资本将加速向实物资产再配置，主权国家将通过地缘博弈与贸易威慑重塑外部信用支撑
- `n9` 若宏观政策未能实质性提升居民现金流可得性与社会保障，经济将陷入预防性储蓄上升与内需收缩的负向循环

---

### Step 4：verify 怎么处理这张图
相关代码：
- `varix/compile/verifier.go`
- `varix/compile/verifier_retrieval.go`

当前这条内容已经在新结构下跑出：
- `version = verify_v2`
- `rollout_stage = facts_only`

这表示：
- facts 走 `claim -> challenge -> adjudication`
- 其他节点类型已经统一纳入 pass/coverage 元数据，但还没全部升级成同样的对抗流程

这次实际 pass 有：
- `fact`
- `explicit_condition`
- `implicit_condition`
- `prediction`

并且：
- `coverage_valid = true`

这意味着：
> 这条内容已经不是“只拿一个总 verdict”，而是会留下 pass-level 验证轨迹。

---

### Step 5：如果接受进 memory，会发生什么
相关代码：
- `varix/storage/contentstore/sqlite_memory.go`
- `varix/cmd/cli/memory_commands.go`

当你执行：
- `memory accept`
- 或 `memory accept-batch`

系统会：
1. 把选中的 graph nodes 写进 `user_memory_nodes`
2. 记录 acceptance event
3. 创建 organization job

也就是说：
> 不是整篇文章直接进 memory，而是“你接受的那些节点”进 memory。

这条 YouTube 当前库里已经存在一条 acceptance event：
- `event_id = 18`
- `user_id = rerun-youtube-62iEHRT36Qg`
- `accepted_count = 10`

---

### Step 6：organizer 怎么整理它
相关代码：
- `varix/storage/contentstore/sqlite_memory_organizer.go`
- `varix/memory/organization_types.go`

organizer 会把 accepted nodes 整理成：
- active / inactive
- hierarchy
- node hints
- open questions
- contradiction / dedupe

这一步的目标不是改写记忆事实，而是：
> 给你一个“这批记忆现在怎么看起来更有认知结构”的视图。

---

### Step 7：posterior verification 现在接到了哪，后面还差什么
相关代码：
- `varix/memory/posterior_types.go`
- `varix/storage/contentstore/sqlite_memory_posterior.go`

对这条 YouTube 来说，phase-1 里 accepted 的 conclusion / prediction 节点现在已经能在
store/organizer seam 上进入：
- `pending`
- `verified`
- `falsified`
- `blocked`

而且这些状态不只是颜色，会影响：
- `NodeHint`
- `OpenQuestions`
- 组织结果里的稳定性 / 展示优先级

也就是说，最终它会从：

> “一篇被抽象过的内容”

变成：

> “一组可追踪生命周期、能事后修正的记忆节点”

但当前还差最后一层 operator 收口：
- 独立 CLI `posterior-run`
- `memory list` / `memory show-source` 的 posterior 读投影
- 更完整的端到端回归覆盖

---

## 11. 如果你要沿代码真正追一次这条 YouTube
最推荐的阅读路径是：

1. `varix/ingest/sources/youtube/*`
2. `varix/ingest/polling/service.go`
3. `varix/compile/client.go`
4. `varix/compile/result.go`
5. `varix/compile/verifier.go`
6. `varix/storage/contentstore/sqlite_memory.go`
7. `varix/storage/contentstore/sqlite_memory_organizer.go`
8. `varix/storage/contentstore/sqlite_memory_posterior.go`
9. `varix/cmd/cli/main_test.go`

这样你会看到一条内容如何：
- 被抓进来
- 被抽成图
- 被验证
- 被接受进 memory
- 被组织
- 最终准备进入 posterior lifecycle

---

## 12. 你现在最该怎么用这份导读
如果你最近有“失控感”，不要试图同时盯全部模块。

推荐做法是：
- 先看本文第 3 节，把主链路重新记住
- 再选一种阅读路线：
  - 想恢复全局感：按第 6 节路线 A
  - 想推进当前最关键问题：按第 6 节路线 B
- 永远把当前 active frontier 限定为 **posterior verification phase-1**

因为现在真正需要强收口的，是：

> **让 conclusion / prediction 进入 memory 后，不会长期变成失效垃圾。**
