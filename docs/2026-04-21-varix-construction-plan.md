# VariX 正式施工方案

> 日期：2026-04-21
> 状态：执行版施工方案
> 目标：在不碰 prompt 设计的前提下，把 VariX 工程主干推进到 graph-first / verify-first / memory-layered 版本

---

## 1. 施工总原则

1. **先立核心模型，再迁移能力**
   - 先有统一子图模型，再改 compile / verify / memory。
2. **兼容迁移，不硬切**
   - 不一次性推倒 legacy compile、legacy memory。
3. **先打通单内容闭环，再做高层抽象**
   - 先把 subgraph + verify queue + content memory 做稳。
4. **事件层先做对齐与汇总，规律层最后做可信度累积**
   - 不提前进入高抽象层。
5. **prompt 与工程解耦推进**
   - 工程先提供稳定 contract，你后续独立接 prompt。

---

## 2. 交付目标

施工完成后，系统应达到以下状态：

### 目标 A：单内容闭环成立
- 原始内容可 compile 成统一子图
- 子图可落库
- 子图可进入验证队列
- 验证结果可回写
- 内容图可进入第一层 memory

### 目标 B：事件层成立
- 多篇内容可按 driver/target + 时间桶形成事件层图
- 事件层允许冲突并可展示聚合结果

### 目标 C：规律层成立
- 可从事件层/内容层抽取范式
- 可根据验证成功/失败更新可信度

### 目标 D：前台投影成立
- compile card 默认只显示主图
- 展开态可查看完整图与验证信息

---

## 3. 施工阶段总览

### Phase 0：设计冻结与迁移准备
### Phase 1：统一子图模型落地
### Phase 2：compile graph-first 化
### Phase 3：verify queue 与 verdict 体系落地
### Phase 4：单内容 memory graph-first 化
### Phase 5：事件层聚合落地
### Phase 6：规律层 / 推理范式层落地
### Phase 7：展示层与运营能力补齐

---

## 4. Phase 0：设计冻结与迁移准备

## 4.1 目标
把施工所需的工程边界固定住，避免边写边改主模型。

## 4.2 主要工作
1. 冻结正式系统设计文档
2. 明确 review gate 只集中在：
   - 子图数据结构
   - 接口设计
3. 明确 prompt 不进入本轮
4. 建立迁移开关：
   - legacy path
   - new graph-first path

## 4.3 产物
- `docs/2026-04-21-varix-system-design.md`
- `docs/2026-04-21-varix-construction-plan.md`
- feature flag 方案（代码阶段实现）

## 4.4 验收
- 设计文档成为主参考文档
- 工程实现不再围绕旧 heuristic 文档零散推进

---

## 5. Phase 1：统一子图模型落地

## 5.1 目标
在代码里引入新的 graph domain model，并让它成为未来 compile / verify / memory 的共同语言。

## 5.2 主要工作

### 5.2.1 新建 model 包
建议新增：
- `varix/model/node.go`
- `varix/model/edge.go`
- `varix/model/subgraph.go`
- `varix/model/card.go`
- `varix/model/verify.go`

### 5.2.2 建立兼容 adapter
- 旧 `compile.Output` -> 新 `ContentSubgraph`
- 新 `ContentSubgraph` -> 旧展示对象

### 5.2.3 加入 schema validation
- node 主体不能为空
- change 不能为空
- edge from/to 必须落在图内
- 时间字段不能明显冲突

## 5.3 为什么先做这个
不先统一领域模型，后面 compile / verify / memory 只能继续各说各话。

## 5.4 验收
- 新 model 包可独立表达完整子图
- 旧 compile 结果能转换到新子图
- 子图 validation 测试通过

## 5.5 风险
- 旧字段与新字段映射不稳定

## 5.6 风险缓解
- 先做 additive adapter，不删旧模型
- 建立 fixture regression tests

---

## 6. Phase 2：compile graph-first 化

## 6.1 目标
让 compile 的核心产物从“面向展示的旧 output”转为“统一子图 + 卡片投影”。

## 6.2 主要工作

### 6.2.1 compile pipeline 改为先产 subgraph
- 记忆综合流程最终都产 `ContentSubgraph`
- card 只是 subgraph 的投影

### 6.2.2 主图与辅助图分离
- compile 阶段必须明确 `is_primary`
- 主边只保留 `drives`
- explain/support/context 只进辅助层

### 6.2.3 主体-变化结构化
- 在 normalize 阶段把 node 从纯文本节点提升为：
  - subject_text
  - change_text
  - time fields

### 6.2.4 compile 持久化升级
在现有 `compiled_outputs` 外，新增新图表或 graph payload 表。

## 6.3 与现有代码关系

### 可保留
- `varix/compile/client.go` 外壳
- `varix/compile/` 分阶段结构
- `compile run/show/card` CLI

### 需重做/增强
- output canonical truth
- 新旧 schema adapter
- graph-first persistence

## 6.4 验收
- `compile run` 能产出统一子图
- 默认 card 只显示主图
- 展开态具备辅助层数据
- 新图 payload 可持久化读取

## 6.5 风险
- 旧 card surface 与新图不一致

## 6.6 风险缓解
- 保留兼容渲染函数
- 对 `compile card` 做回归测试

---

## 7. Phase 3：verify queue 与 verdict 体系落地

## 7.1 目标
把 verify 从“单次命令”升级成“可调度、可重排、可回写的持续系统”。

## 7.2 主要工作

### 7.2.1 建立 verify queue
新增：
- `verify_queue`
- `verify_verdict_history`

### 7.2.2 建立 queue scheduler
职责：
- 找到 due items
- 生成执行批次
- 控制重试
- 更新状态

### 7.2.3 node / edge 双对象验证
- node 与 edge 都进入统一队列
- edge verdict 与 node verdict 分离写入

### 7.2.4 pending 预测回查
- scheduler 周期扫描 pending
- 若证据不足则重排
- 若已可判定则更新 verdict

### 7.2.5 verdict writeback
- 回写 node/edge 当前状态
- 写 verdict history
- 必要时触发 memory refresh

## 7.3 与现有代码关系

### 可保留
- `verify run/show` 命令组
- `compile.Verification` 兼容层
- `verification_results` 存储思想
- `sqlite_memory_posterior.go` 的 pending / refresh 调度思路

### 需重做/增强
- 对象粒度从旧 checks 升级为 node/edge verdict
- pending 队列标准化
- edge verification 正式化

## 7.4 验收
- pending 节点会自动进入队列
- edge 也能进入队列
- 未到窗口的预测不会被误判
- 无法判定对象可被重新调度
- verdict history 可查询

## 7.5 风险
- 长期 pending 对象堆积

## 7.6 风险缓解
- 增加优先级与 age 监控
- 区分 not_due / due-but-insufficient 两类重排逻辑

---

## 8. Phase 4：单内容 memory graph-first 化

## 8.1 目标
把 memory 第一层从“接受若干节点”升级成“接受单内容图”。

## 8.2 主要工作

### 8.2.1 content memory graph 表落地
- 接受的不再只是 node snapshots
- 而是整个 article-scoped graph snapshot

### 8.2.2 acceptance event 扩展
- 继续保留 acceptance event
- payload 改为 graph-aware

### 8.2.3 memory refresh 机制升级
- 当 verdict 变化时，不是只改 node state
- 而是对 content memory graph 做增量刷新或重投影

## 8.3 与现有代码关系

### 可保留
- `AcceptMemoryNodes` 的事件与 job 思路
- `memory_organization_jobs`
- `memory_organization_outputs`

### 需重做/增强
- memory 主对象从 node-first 改为 graph-first
- organizer 输入从 node list 扩展为 content graph

## 8.4 验收
- 一篇内容的完整图可进入 memory
- verdict 变化能影响 memory graph 展示层
- 单内容 memory 仍保留 provenance 与回溯

---

## 9. Phase 5：事件层聚合落地

## 9.1 目标
把多篇内容围绕同一事件的图聚合出来，而不是只做文本 cluster。

## 9.2 主要工作

### 9.2.1 建立 canonical subject pipeline
- subject alias normalize
- canonical subject assignment
- alias table / mapping table

### 9.2.2 建立 event bucketing
两条主线：
- driver scope = `(canonical driver + time bucket)`
- target scope = `(canonical target + time bucket)`

### 9.2.3 建立 event graph assembler
- 合并来自不同内容图的节点/边
- 保留冲突
- 输出事件层 summary graph

### 9.2.4 建立 event layer render
- 提供 driver 视角与 target 视角汇总
- 支持一周内涨跌并存的情况

## 9.3 与现有代码关系

### 可保留
- relation-first projection 的部分 canonical entity / relation store 思路
- `memory_canonical_entities` / `memory_relations` 的表组织理念

### 需重做/增强
- 现有 canonicalization 主要是字符串层，需要提升为 subject-first
- 现有 thesis cluster heuristics 不能作为长期主干

## 9.4 验收
- 同一主体 + 时间桶能形成 event graph
- event graph 可以容纳冲突 observation
- 能从 event graph 生成摘要视图

## 9.5 风险
- 主体归一化误合并/漏合并

## 9.6 风险缓解
- canonicalization 分三层：
  1. exact alias
  2. normalized alias
  3. human-reviewed override

---

## 10. Phase 6：规律层 / 推理范式层落地

## 10.1 目标
从内容层和事件层中提取可复用的推理模式，并建立动态可信度。

## 10.2 主要工作

### 10.2.1 定义 paradigm extractor
输入：
- verified event graphs
- verified content subgraphs

输出：
- edge pattern
- path pattern
- subgraph pattern

### 10.2.2 建立 credibility updater
- 验证成功 -> credibility 增加
- 验证失败 -> credibility 大幅下降
- 达阈值 -> explicit
- 长期失败 -> degraded

### 10.2.3 建立 paradigm evidence links
- 每个范式要能回溯 supporting events / subgraphs

### 10.2.4 建立 paradigm presentation
- 规律卡片
- 为什么成立
- 何时失效
- 目前可信度

## 10.3 与现有代码关系

### 可保留
- relation/mechanism/path outcome 等类型定义中的部分高层抽象思路

### 需重做/增强
- 现有 thesis/card/conclusion 体系不是“验证反馈驱动的范式层”
- 需引入新的 credibility 机制

## 10.4 验收
- 范式可以从事件层中抽出
- 一个验证失败可以降低相关范式分数
- 达阈值范式能显式展示

## 10.5 风险
- 过早抽象导致假规律

## 10.6 风险缓解
- 仅从已验证样本构建范式
- 设定最小 supporting evidence 数量门槛

---

## 11. Phase 7：展示层与运维能力补齐

## 11.1 目标
让系统不仅能算，还能稳态运行、可检查、可调试。

## 11.2 主要工作

### 展示
- compile card 改为主图投影
- event card 输出
- paradigm card 输出

### 调试
- graph inspection CLI
- verify queue inspection CLI
- event graph dump CLI
- paradigm dump CLI

### 观测
- queue metrics
- pending age metrics
- event rebuild metrics
- paradigm recompute metrics

### 运维
- retry policy
- stale job cleanup
- backfill commands

## 11.3 验收
- 工程链路能被 CLI 与日志清晰观察
- 出问题时能定位到内容层 / verify 层 / memory 层 / paradigm 层

---

## 12. 文件与模块施工建议

## 12.1 保留主目录结构
继续保留：
- `varix/cmd/cli`
- `varix/compile`
- `varix/compile`
- `varix/storage/contentstore`
- `varix/memory`

## 12.2 建议新增目录

### 新增领域模型层
- `varix/model/`

### 新增验证调度层
- `varix/verifyqueue/`

### 新增事件层
- `varix/eventmem/`

### 新增范式层
- `varix/paradigm/`

### 新增渲染层
- `varix/render/`

## 12.3 不建议现在做的事
- 不引入新的外部数据库
- 不把系统改成 Python 主干
- 不把 Graphiti/Loki 整套接进来
- 不先做 UI 重构

---

## 13. 施工顺序建议

最推荐的真实施工顺序是：

1. **model**
2. **compile graph-first persistence**
3. **verify queue**
4. **content memory graph-first**
5. **event graph**
6. **paradigm layer**
7. **presentation + observability**

原因：
- 前四步能先形成可运行闭环
- 后三步是抽象与增强层

---

## 14. 关键风险与处理

### 风险 1：旧系统与新模型长期双轨，维护成本上升
处理：
- 从一开始定义兼容边界
- 明确旧模型只是 adapter，不是主真相源

### 风险 2：subject canonicalization 难度被低估
处理：
- 先用简单 canonical layer 起步
- 人工 override 保底
- 不把事件层正确性完全押在一次性自动归一化上

### 风险 3：verify 队列长期堆积
处理：
- 优先级 + 批次调度
- 最大重试与重排策略
- pending 年龄监控

### 风险 4：规律层过早抽象出错误智慧
处理：
- 最小支持样本门槛
- credibility 降权机制
- 必须保留 evidence trace

### 风险 5：prompt 改动扰动工程判定
处理：
- 严格隔离 prompt contract 与工程主模型
- 工程以 parseable contract 为准，不以内联 prompt heuristics 为准

---

## 15. 验收口径

## 15.1 第一阶段验收（闭环成立）
满足以下条件即算第一阶段完成：
- 单内容可 compile 成统一子图
- 子图可落库
- node/edge 可进入 verify queue
- verdict 可回写
- 单内容 memory graph 可形成

## 15.2 第二阶段验收（事件层成立）
满足以下条件即算第二阶段完成：
- 可生成 driver-centric event graph
- 可生成 target-centric event graph
- 同一时间桶下冲突信息可共存

## 15.3 第三阶段验收（规律层成立）
满足以下条件即算第三阶段完成：
- 可生成 paradigm
- verification 成败可影响 credibility
- explicit paradigms 可展示与追溯

---

## 16. 当前立即执行动作

在本文之后，我会按这个顺序推进工程：

### Action 1
以正式设计文档为准，固化 graph-first 主线。

### Action 2
围绕统一子图模型，开始重构 compile / verify / memory 的工程边界。

### Action 3
优先完成“单内容闭环”，不先陷入事件层和规律层细枝末节。

---

## 17. 你需要介入的唯一节点

你只需要优先 review：
- 数据结构
- 接口设计

其余工程判断我会直接负责推进。
