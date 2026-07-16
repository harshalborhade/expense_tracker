# ADR-0001: Consolidate transaction ingestion into the Go application

**Status:** Proposed
**Date:** 2026-07-16
**Deciders:** hb (project owner)

## Context

The expense tracker has two overlapping implementations of the same data-ingestion
logic, split across two languages:

- **Go app (`services/`)** — the long-running daemon. Runs incremental SimpleFIN
  and Splitwise syncs, applies the `RuleEngine` for auto-categorization, exports
  Ledger files, and serves the HTTP UI.
- **Python scripts (`util/`)** — one-off operational tools that talk to the same
  APIs with *different, more capable* logic.

Capability comparison as of this ADR:

| Concern | Go app (`services/`) | Python (`util/`) |
|---|---|---|
| SimpleFIN date-windowed backfill | ❌ default window only, no date params | ✅ `init_history.py` (45-day chunks) |
| Splitwise incremental sync | ✅ | ✅ `import_splitwise.py` (second impl) |
| Splitwise settlements | ❌ | ✅ `import_splitwise_settlements.py` |
| CSV import | ❌ | ✅ `import_csv.py` |
| Transfer/settlement matching | ❌ | ✅ `match_transfers_settlements.py` |
| Auto-categorization (RuleEngine) | ✅ | ❌ scripts hardcode `Expenses:Uncategorized` |
| SimpleFIN token claim | ❌ | ✅ `convert_setup_to_access.py` |

### Forces at play

1. **Divergence risk (primary).** The Go `SimpleFinService.Sync()`
   (`services/simpleFIN.go:62`) issues a bare `http.Get(AccessURL)` with no
   `start-date`/`end-date`, so it can only ever fetch the provider's default
   window. It structurally *cannot* backfill a gap. When the tracker isn't run for
   a while, the daemon silently leaves a hole that only the Python
   `init_history.py` can fill. This is exactly the failure observed on
   2026-07-16: a ~3-month gap the daemon could not close.
2. **Two sources of truth for the same record.** `import_splitwise.py` and
   `SplitwiseService.Sync()` compute owed/paid share independently. The same
   Splitwise expense can land with a different provider label or category
   depending on which path ran.
3. **Categorization inconsistency.** Anything imported via Python bypasses the
   `RuleEngine`, so backfilled rows arrive uncategorized while daemon-synced rows
   are auto-tagged.
4. **Operational friction.** Backfill requires running an interactive Python
   script from `util/` with the right cwd, feeding `y` to prompts, and knowing to
   run the settlement/matching scripts in the correct order afterward.

### Constraints

- Single-user, local-only app (server binds to `127.0.0.1`). No HA/scale concerns.
- SQLite via GORM; ingestion is naturally idempotent (`INSERT OR IGNORE` in
  Python; find-or-create upsert in Go).
- Small codebase, single maintainer. Simplicity and one obvious code path matter
  more than flexibility.
- Out of scope for this ADR: the hardcoded SimpleFIN credential in
  `util/endpoint_test.py:3` (tracked separately; needs rotation) and the Ollama
  categorization idea in `TODO.md`.

## Decision

Fold all recurring ingestion logic into the Go application behind a proper
**subcommand CLI**, so there is one implementation per data source, all flowing
through the `RuleEngine`. Retire the Python scripts as their Go equivalents land.
The Python scripts remain in `util/` (unexecuted) until each is replaced and
verified, then are deleted in the same PR that replaces them.

Target CLI shape (replacing today's `-export` / `-no-sync` flags):

```
expense_tracker serve                              # default: run daemon (sync + HTTP)
expense_tracker serve --no-sync                    # daemon, skip initial network sync
expense_tracker export                             # regenerate Ledger files and exit
expense_tracker sync                               # incremental sync and exit
expense_tracker sync --backfill --since 2026-03-01 # date-windowed backfill  (init_history.py)
expense_tracker import-csv <file>                  # (import_csv.py)
expense_tracker match-settlements                  # (match_transfers_settlements.py)
```

The daemon keeps running incremental sync in the background; the difference is
that backfill, settlements, CSV, and matching become first-class Go code paths
sharing the same models, DB layer, and `RuleEngine`.

## Options Considered

### Option A: Consolidate into Go subcommands (proposed)

| Dimension | Assessment |
|-----------|------------|
| Complexity | Medium — port ~4 scripts; add date-windowing to SimpleFIN fetch |
| Cost | One-time porting effort; lower ongoing maintenance |
| Scalability | N/A (single user) — but removes the structural backfill gap |
| Team familiarity | High — single Go codebase, already the primary language |

**Pros:**
- One implementation per data source; no daemon-vs-script divergence.
- Backfill and settlements run through the `RuleEngine` — consistent categories.
- Single binary, no Python/venv/`requests`/`dotenv` runtime dependency.
- Non-interactive, scriptable, cron-friendly (no `y`-prompt feeding).
- Shared models mean schema changes touch one ingestion layer, not two.

**Cons:**
- Upfront porting work and re-testing against live APIs.
- Go is more verbose for quick data-munging than the Python scripts.
- Loses the ad-hoc "edit the script and re-run" ergonomics for experiments.

### Option B: Keep Go daemon thin, move *all* ingestion to Python

| Dimension | Assessment |
|-----------|------------|
| Complexity | Medium — Go daemon shrinks, Python grows into a package |
| Cost | Rewrites the working Go sync/export/RuleEngine paths |
| Scalability | N/A |
| Team familiarity | Medium — splits logic across two languages permanently |

**Pros:** Python is faster to iterate for import/matching heuristics.
**Cons:** Throws away working Go code (RuleEngine, export, HTTP); still two
runtimes; the daemon would have to shell out to Python or duplicate logic anyway.
Rejected — moves the divergence rather than removing it.

### Option C: Status quo — Go daemon + Python scripts, documented

| Dimension | Assessment |
|-----------|------------|
| Complexity | Low now, higher over time |
| Cost | Zero upfront; recurring manual toil and drift |
| Scalability | N/A |
| Team familiarity | Split |

**Pros:** No work today. **Cons:** Every force in Context persists — the backfill
gap, dual share logic, uncategorized backfills, and run-order footguns remain.
Rejected as the target state; acceptable only as the interim.

## Trade-off Analysis

The decisive factor is **one code path per data source**, not language preference.
The concrete bug — a daemon that cannot backfill — is caused by the split, and
the split also produces silent data-quality drift (labels, categories) that is
hard to notice until reconciliation. Option A pays a bounded, one-time porting
cost to remove an unbounded, recurring source of inconsistency. Options B and C
each leave two implementations alive, so the drift continues.

Because ingestion is already idempotent, the migration is low-risk: the Go
subcommand and the Python script can be run against the same DB and their results
diffed before the script is deleted. This makes the port verifiable step by step
rather than a big-bang cutover.

## Consequences

**Easier:**
- Closing gaps: `sync --backfill --since <date>` becomes a one-liner, no cwd/prompt dance.
- Consistent categorization on all ingested rows (RuleEngine everywhere).
- Deployment/automation: single static binary, cron-schedulable.

**Harder:**
- Quick data experiments lose the edit-a-script loop; a Go rebuild is needed.
- Porting the Splitwise share/settlement heuristics must faithfully reproduce the
  Python semantics (owed vs. paid share, payer labeling) — the subtlest part.

**To revisit:**
- SimpleFIN default-window vs. explicit-window behavior — confirm the bridge
  honors `start-date`/`end-date` the same way the Python path relies on.
- The `sw_<id>` primary-key collision between regular expenses and settlements
  (same ID, different provider label) — decide the canonical labeling in Go so
  ingestion order no longer determines the outcome. This is what
  `match-settlements` currently patches up after the fact.
- Whether `import-csv` stays a subcommand or the daemon watches a directory.

## Action Items

Ordered smallest-value-first so each step is independently shippable and verifiable
against the existing Python output.

1. [ ] **Backfill (highest value).** Add optional `start-date`/`end-date` to
   `SimpleFinService` fetch; add `sync --backfill --since` subcommand with chunked
   windows (port `init_history.py`). Verify row-for-row against a Python run on a
   copy of the DB.
2. [ ] **Subcommand skeleton.** Replace `-export`/`-no-sync` bool flags in
   `main.go` with a subcommand dispatcher (`serve`/`sync`/`export`), keeping
   current default behavior (`serve`) intact.
3. [ ] **Splitwise settlements + share logic.** Move settlement handling and the
   canonical `sw_<id>` labeling into `SplitwiseService`; run backfilled rows
   through the `RuleEngine`. Retire `import_splitwise.py` and
   `import_splitwise_settlements.py`.
4. [ ] **Settlement/transfer matching.** Port `match_transfers_settlements.py` to
   `match-settlements` subcommand.
5. [ ] **CSV import.** Port `import_csv.py` to `import-csv <file>`.
6. [ ] **Cleanup.** Delete replaced scripts; keep only genuinely one-off helpers
   (e.g. `convert_setup_to_access.py` token claim) or fold them in too. Update
   README with the new CLI.
7. [ ] **Separate track (not this ADR):** rotate and remove the hardcoded
   SimpleFIN credential in `util/endpoint_test.py`.
