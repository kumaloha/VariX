# Memory Review Cheatsheet

A short operator-oriented guide for reviewing VariX memory outputs.

---

## 1. Recompute and inspect thesis-first cards

### Full card view
```bash
varix memory global-v2-card --run --user <user_id>
```
Use when you want the latest thesis-first memory cards in a human-readable form. The output now starts with an `Items` count for the current slice.

### Only conclusions
```bash
varix memory global-v2-card --run --user <user_id> --item-type conclusion
```
Use when you only want first-layer synthesized judgments.

### Only conflicts
```bash
varix memory global-v2-card --run --user <user_id> --item-type conflict
```
Use when you only want contradiction review.

### Limit long outputs
```bash
varix memory global-v2-card --run --user <user_id> --item-type conclusion --limit 5
```
Use when a user has many items and you only want the first few.

---

## 2. Compare v1 vs v2 quickly

### Persisted compare
```bash
varix memory global-compare --user <user_id>
```
Use when you want to compare the last stored cluster-first and thesis-first views.

### Fresh compare
```bash
varix memory global-compare --run --user <user_id>
```
Use when you want both sides recomputed before comparison.

### Compare only one v2 item class
```bash
varix memory global-compare --run --user <user_id> --item-type conflict
varix memory global-compare --run --user <user_id> --item-type conclusion
```
Use when the v2 side is too busy and you only want one class of first-layer items.

### Limit compare output
```bash
varix memory global-compare --run --user <user_id> --limit 3
```
Use when you want a smaller sample from both v1 and v2.

### What the compare output now includes
- section counts for v1 and v2
- optional v2-side filtering (`--item-type`)
- optional output shortening (`--limit`)
- explicit no-match guidance when the filtered v2 side is empty

---

## 3. Read raw v2 JSON

### Fresh JSON
```bash
varix memory global-v2-organize-run --user <user_id>
```
Use when you need the full machine-readable v2 payload.

### Stored JSON
```bash
varix memory global-v2-organized --user <user_id>
```
Use when you want the last persisted v2 output without recomputing.

---

## 4. What to look for during review

### Good signs
- topic labels are short enough to name a cognition object
- conflicts appear as conflicts, not forced conclusions
- headlines sound more abstract than the raw node text
- `Items`, `Why`, `Mechanism`, `Conditions`, `What next`, and `Sources` are clearly separated
- `Why A / Why B` on conflict cards point to real supporting context

### Warning signs
- one thesis swallows too many unrelated sources or claims
- topic labels fall back to huge raw text blobs
- headlines read like literal card-title-plus-prediction templates
- conflict cards show only disagreement direction but no usable supporting context
- filtered or compare outputs become too noisy to review quickly

---

## 5. Recommended review order

1. `global-v2-card --run --item-type conflict`
2. `global-v2-card --run --item-type conclusion --limit 5`
3. `global-compare --run --limit 5`
4. If needed: `global-v2-organize-run` for raw JSON inspection
