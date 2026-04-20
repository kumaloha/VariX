# Compile V2 整改计划

## 目标
把 compile v2 从“可运行 MVP”推进到“结构正确、可评估、可逐步替代 legacy”的版本。

---

## 核心问题

### 1. Stage contract 不清
- Stage1 的 `edges` 与 Stage3Relations 的关系输出谁是权威不清晰
- `role` / `ontology` 的生产与覆盖时机不清晰
- off-graph 项何时生成、何时重写不清晰

### 2. Stage2 dedup 已进入语义去重阶段，但候选生成与合并质量仍需增强
- 语义近似节点仍大量保留
- 直接影响 driver/target/path 的数量与质量

### 3. Stage3 已拆成关系层 + 分类层，但仍需在复杂样本上继续稳定
- 关系判定虽然已接入，但仍需要稳定化与简化调用成本
- 应先判关系，再判角色，而不是只做单节点分类

### 4. Stage5 fallback 仍然过粗
- 不应在图结构不足时硬造主线
- 应显式失败或降 confidence，而不是伪造正确结构

### 5. Prompt 定义仍有 vibes 成分
- 需要操作化定义
- 需要补 boundary few-shot，而不是继续靠经验加规则

### 6. 目前缺最小评测集
- 没有稳定的质量回归就容易盲调

---

## 整改原则

1. **不允许过拟合单样本**
   - 不写 G04 特判
   - 规则必须对多类型财经文章成立

2. **代码承担确定性逻辑，LLM 只做语义判断**
   - dedup / 图算法 / path 提取 / schema 组装尽量放代码

3. **先立 contract，再改实现**
   - 避免 stage 之间职责重叠和字段漂移

4. **宁可失败，不要假主线**
   - 删除或大幅削弱 fake fallback

5. **先做最小评测，再扩大规则集**
   - 每加一条规则，都要能回答：修了哪个错误

---

## 分阶段计划

### Phase 1：建立 Stage Contract Table
产物：`docs/compile-v2-stage-contract.md`

需要明确：
- Stage1 输入/输出
- Stage2 输入/输出
- Stage3Relations 输入/输出
- Stage3Classify 输入/输出
- Stage4Validate 输入/输出
- Stage5Render 输入/输出
- 哪个 stage 对 `edges / role / ontology / off_graph` 拥有最终写权限

验收标准：
- 每个 stage 的字段权威来源明确
- downstream 只消费一份明确语义的数据

---

### Phase 2：做真 Stage2 dedup
当前方向：
- candidate pairs
- LLM equivalence 判断
- union-find 合并
- canonical 选择
- 非 canonical 节点降为 supplementary

下一步增强：
- candidate pair 选择更稳
- canonical 规则更清晰
- 减少近义重复 target/driver

验收标准：
- G04 这类 case 中，近义/标签化句子不再大面积并列残留
- 节点数显著收敛，但不误杀主线

---

### Phase 3：重构 Stage3 为“关系层 + 分类层”

#### 3A. 关系层
只判：
- causal
- supports
- supplements
- explains
- none

要求：
- 批量判定，不逐边逐 call
- 结果先改图，再进入分类层

#### 3B. 分类层
在关系层改图后再判：
- driver
- transmission
- target_candidate
- target

验收标准：
- G04 中“主结果 / 标签化表述 / 解释”能分层
- 不再把结果句简单并列成多个 target

---

### Phase 4：削弱或移除 Stage5 假主线 fallback
改动目标：
- 没有足够图结构时，不硬造 driver/target/path
- 允许返回空或低 confidence
- 把失败暴露给下游，而不是伪造主线

验收标准：
- 不再出现“第一节点硬当 driver，最后节点硬当 target”的错误输出

---

### Phase 5：Prompt 操作化
目标：
- 把模糊定义改成可操作定义
- 给 Stage2 / Stage3 / Stage4 加少量 boundary few-shot

优先 prompt：
- Stage2 equivalence
- Stage3 relation
- Stage3 classify
- Stage4 validate

验收标准：
- Prompt 里每条重要规则都能对应一个边界样本
- 减少 purely speculative wording

---

### Phase 6：建立最小评测集
首批样本建议：
- G01
- G02
- G03
- G04
- 再补 2~4 篇不同风格文章

每篇至少标注：
- 主 driver
- 主 target
- 关键 transmission
- 关键 off-graph 归类

验收标准：
- 可比较 legacy / v2
- 可比较 stage 改动前后质量变化

---

## 当前执行优先级

### 第一优先级
1. Stage contract table
2. Stage2 真 dedup 做稳
3. Stage3 关系层/分类层继续收敛
4. 去掉 Stage5 假主线 fallback

### 第二优先级
5. Prompt 操作化 + few-shot
6. 最小评测集

### 第三优先级
7. 用评测集持续对打 legacy / v2
8. 决定何时把 v2 提升为默认 pipeline

---

## 当前状态（写文档时）
- compile v2 已有独立代码路径
- CLI 已支持 `--pipeline v2`
- Stage1/2/3/4/5 都已有最小实现
- debug 落盘已完成
- Stage3 已开始引入关系判定层
- 但复杂样本（尤其 G04）仍未完全稳定

