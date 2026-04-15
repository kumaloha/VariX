# Memory v2 Review Samples

This document captures representative **human-readable** `memory global-v2-card`
shapes so the thesis-first memory output can be reviewed without reading raw
JSON.

---

## Sample: conflict card

```text
Conflict
关于「油价」的判断

Signal
high

Summary
同一判断方向相反

Side A
- 油价会下降

Side B
- 油价会上升

Why A
- 油价会下降

Why B
- 油价会上升

Sources A
- twitter:TX3

Sources B
- weibo:TX2
```

### What this is proving
- conflict is first-layer, not buried under a synthesized conclusion
- both sides are visible as distinct cognition objects
- each side can point back to concrete source references

---

## Sample: conclusion card

```text
Conclusion
流动性收紧正在把风险资产推向承压与更高波动

Signal
high

Summary
流动性收紧 → 若融资环境继续恶化 → 风险资产承压 → 未来数月波动加大

Logic
- 流动性收紧 (fact)
- 若融资环境继续恶化 (condition)
- 风险资产承压 (conclusion)
- 未来数月波动加大 (prediction)

Why
- 流动性收紧

Conditions
- 若融资环境继续恶化

What next
- 未来数月波动加大

Sources
- weibo:TX1
```

### What this is proving
- the first layer is now a synthesized judgment rather than a cluster label
- the card exposes a causal path instead of a node dump
- evidence, conditions, predictions, and sources are split into separate sections

---

## Current known limitations

- headline abstraction is still template-driven and only partially generalized
- conflict-side `Why A / Why B` currently falls back to local available evidence and may be shallow on sparse inputs
- thesis grouping still uses bounded heuristics rather than full semantic question matching
