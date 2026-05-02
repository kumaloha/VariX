# Evaluation

This directory stores repository-level evaluation code and assets.

- `gold/` contains checked-in gold datasets used by compile scoring tests and
  CLI evaluation commands.
- `cmd/goldscore/` scores compile candidate outputs against a gold dataset.

Run a scorecard from this module with:

```bash
go run ./cmd/goldscore --gold ./gold/compile-gold-batch1-baseline.json --candidate-dir <candidate-dir>
```

Keep runtime state under `data/` and Go tests under `tests/`.
