# Compile Persistence Contract

## Purpose

Compile persistence exists to make compile outputs:

- reusable without recomputing every time,
- consistent across product surfaces,
- traceable enough to know which model/run produced the visible result.

This is **not** the memory layer. It is the stable persistence layer for a
single content unit's latest compile result.

---

## Canonical model

One content unit has **one latest compile result**.

Primary identity:

- `platform`
- `external_id`

Secondary convenience path:

- `url` -> parse -> canonical identity -> same stored row

There is no compile history timeline in the current design.

---

## Read/write behavior

### `compile run`

- If no compiled output exists, compute and persist it.
- If a compiled output already exists, return the cached result by default.

### `compile run --force`

- Recompute the compile output.
- Overwrite the existing stored row for the same canonical identity.

### `compile show`

- Read the persisted compile output and print the full stored record.

### `compile summary`

- Read the persisted compile output and print a human-readable summary view.

### `compile compare`

- Read the persisted raw capture and persisted compile output together.

### `compile card`

- Read the persisted compile output and render the product-facing card view.

All of the above support either:

- `--platform <platform> --id <external_id>`
- `--url <url>`

---

## Persistence shape

Persisted compile output includes at least:

- `platform`
- `external_id`
- `root_external_id`
- `model`
- `payload_json`
- `compiled_at`
- `updated_at`

The payload stores the current structured compile output shape:

- `summary`
- `graph`
- `details`
- `topics`
- `confidence`

The additive driver-target normalization contract is documented separately in
`docs/compile-driver-target-schema.md` so the persistence contract can stay
focused on storage behavior while the schema work lands.

---

## Product boundary

This layer is intentionally simple:

- **latest-only**
- **explicit-force overwrite**
- **URL lookup is a convenience path**
- **no automatic invalidation**

The system currently accepts that a cached compile may become stale if ingest
changes and the caller does not use `--force`. This is considered acceptable
for now because the event is low-probability and not worth adding invalidation
complexity yet.

---

## Explicit non-goals

Not included in this phase:

- compile version history
- diff / audit timeline
- stale warnings
- ingest-change detection
- automatic recompute
- cross-document recall
- memory / retrieval / knowledge graph over multiple articles

---

## Why this is not memory

Compile persistence stores **one computed interpretation for one content unit**.

Memory would mean later capabilities such as:

- cross-document retrieval,
- semantic recall,
- evolving knowledge state,
- linking many compiled outputs together.

Those should be built **on top of** stable compile persistence, not mixed into
it.

---

## Practical operator guidance

Use:

```bash
cd varix
go run ./cmd/cli compile run --url '<url>'
go run ./cmd/cli compile run --force --url '<url>'
go run ./cmd/cli compile show --url '<url>'
go run ./cmd/cli compile summary --url '<url>'
go run ./cmd/cli compile compare --url '<url>'
go run ./cmd/cli compile card --compact --url '<url>'
```

Interpretation:

- no `--force` = prefer persisted latest compile
- `--force` = recompute and overwrite

---

## Current implementation note

Compile outputs are already persisted in SQLite and reused by CLI surfaces.
This document exists to make that behavior an explicit product and engineering
contract instead of an implicit implementation detail.
