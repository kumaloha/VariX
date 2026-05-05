# Evaluation

This directory stores repository-level evaluation code and assets.

- `gold/` contains checked-in gold datasets used by compile scoring tests and
  CLI evaluation commands.
- `cmd/goldscore/` scores compile candidate outputs against a gold dataset.

Run a scorecard from this module with:

```bash
go run ./cmd/goldscore --gold ./gold/compile-gold-batch1-baseline.json --candidate-dir <candidate-dir>
```

Compile v2 anchor checks use the four user-approved reference samples:

```bash
go run ./cmd/goldscore --gold ./gold/compile-gold-compile-v2-anchor4.json --candidate-dir <candidate-dir>
```

The anchor-4 dataset includes structural expectations for ledger, brief,
branches, transmission paths, and coverage audit output. Candidate files can be
named after the sample id, or can include `sample_id` next to `output`.

Keep runtime state under `data/` and Go tests under `tests/`.
