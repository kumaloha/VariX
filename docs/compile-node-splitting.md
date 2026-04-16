# Compile Node Splitting Rules

This document defines a practical v1 rule set for splitting large mixed
sentences into smaller reasoning-graph nodes before downstream verification
and memory organization.

The goal is not maximal fragmentation. The goal is to avoid **fat nodes**
that hide meaningful internal causal structure.

---

## Core principle

**One node should express one independently meaningful claim.**

If a sentence already contains an internal causal chain, the graph should
prefer to expose that chain as multiple nodes plus edges rather than hiding
it inside one large node.

---

## Fast decision rule

If a sentence can naturally be rewritten with **two or more arrows**, it
should usually be split.

Example:

`伊朗战争封锁霍尔木兹海峡 → 油价飙升 → 美国通胀上行 → 美联储维持高利率`

This should not remain one node.

---

## Required split cases

### 1. Multi-step causal chains

Split when one sentence contains two or more causal hops:

- `A 导致 B，进而导致 C`
- `A 推升 B，因此 C`
- `A → B → C`

**Target shape**
- one node per causal step
- one edge per causal relation

---

### 2. Mixed time layers in one sentence

Split when one sentence mixes:

- already-observed fact
- current conclusion
- future prediction

Example:

`油价已飙升，黄金因此暴跌，未来美联储将被迫宽松`

Should become:
- fact: 油价已飙升
- conclusion/fact: 黄金暴跌
- prediction: 美联储将被迫宽松

---

### 3. Explicit condition + future result

When a sentence contains:

- `若/如果/一旦 ...`
- followed by a future result (`将/会/可能/大概率/引发/导致`)

Split into:
- explicit condition node
- prediction node

Do **not** keep the whole sentence as one explicit-condition node.

---

### 4. Multiple independently checkable facts

Split when one sentence contains multiple clauses that could each be
verified or falsified independently.

Example:

`战争引发美元流动性紧张，土耳其央行抛售黄金，全球黄金ETF也出现大规模流出`

These should usually become multiple fact nodes.

---

### 5. Hidden mechanism embedded between fact and conclusion

Split when a sentence implicitly contains:

- an observed fact
- a mechanism
- a conclusion

Example:

`高资产价格环境在宏观负面冲击下会放大系统脆弱性，因此风险资产承压`

Should become:
- fact/condition: 高资产价格环境在宏观负面冲击下
- mechanism: 系统脆弱性被放大
- conclusion: 风险资产承压

---

## Usually keep as one node

### 1. Pure paraphrase / rhetorical repetition

If a sentence repeats one idea in multiple ways but does not introduce
separate causal or temporal structure, keep one node.

### 2. Short atomic claims

If a sentence is already a compact single claim:

- one fact
- one conclusion
- one prediction

then keep one node.

### 3. Non-essential modifiers

Intensity, tone, rhetorical framing, and emphasis should not become their
own nodes unless they alter the actual logic.

---

## Preferred decomposition order

When a sentence is long and complex, prefer splitting out this skeleton
first:

1. **fact**
2. **explicit condition**
3. **implicit condition / mechanism**
4. **conclusion**
5. **prediction**

This keeps the graph useful even before finer-grained splitting is added.

---

## Edge mapping rules

Recommended initial mapping:

- fact → fact: `derives` or `positive` when one observed event clearly causes another
- fact → conclusion: `derives`
- condition → prediction: `presets`
- mechanism → conclusion: `derives`
- conclusion → prediction: `derives`

Do not overfit edge semantics early. It is better to expose the chain than
to perfectly label every edge.

---

## Practical trigger words

These often indicate split points:

### Causal
- 导致
- 使得
- 推升
- 压制
- 引发
- 带来
- 进而
- 因此
- 所以

### Conditional
- 若
- 如果
- 一旦
- 假如
- 只要

### Predictive
- 将
- 会
- 可能
- 大概率
- 预计
- 未来
- 今后
- 接下来

Trigger words are heuristics, not hard truth. They should guide splitting,
not replace reading comprehension.

---

## Anti-slop guardrails

### Do not oversplit into trivia

Bad:
- every noun phrase becomes a node

Good:
- only split when a clause can stand as a meaningful causal or temporal step

### Do not undersplit long macro chains

Bad:
- entire macro story compressed into one “fat fact”

Good:
- enough nodes that later verification and memory can reason over the chain

### Do not lose role semantics

If a clause is clearly a condition or prediction, do not flatten it into a
fact just because it appears inside a larger sentence.

---

## Example transformations

### Example A

Input:

`伊朗战争封锁霍尔木兹海峡导致油价飙升，推高美国通胀，美联储维持高利率并释放加息预期。`

Recommended split:

- fact: 伊朗战争封锁霍尔木兹海峡
- fact: 油价飙升
- fact: 美国通胀上行
- conclusion/fact: 美联储维持高利率并释放加息预期

Edges:

- n1 → n2
- n2 → n3
- n3 → n4

### Example B

Input:

`若AI应用冲击导致SaaS企业现金流断裂，私募信贷极大概率爆发挤兑，并可能波及华尔街。`

Recommended split:

- explicit condition: 若AI应用冲击导致SaaS企业现金流断裂
- prediction: 私募信贷极大概率爆发挤兑，并可能波及华尔街

Edge:

- n1 → n2 (`presets`)

### Example C

Input:

`高资产价格环境在宏观负面冲击下会放大系统脆弱性，因此风险资产承压。`

Recommended split:

- condition/fact: 高资产价格环境在宏观负面冲击下
- mechanism: 系统脆弱性被放大
- conclusion: 风险资产承压

Edges:

- n1 → n2
- n2 → n3

---

## Recommended next implementation step

Start with a **split-candidate detector** that scores sentences for:

- multi-hop causality
- time-layer mixing
- condition/result combinations
- multi-fact density

Then only split above a threshold.

That lets the system improve “fat node” cases first without forcing all
sentences through aggressive decomposition.
