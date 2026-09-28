# ADR-002: Price import immutability strategy (BR-003)

> **Status:** Accepted  
> **Date:** 2026-06-18  
> **Deciders:** Architecture / data layer  
> **Refs:** BR-003, UC-001, `docs/architecture.md#database-schema`

## Context

ANP weekly files may be downloaded and imported more than once (manual sync, retries, or corrected files for the same `SurveyWeek`). The domain rule **BR-003** requires that previously imported price rows are **never deleted or overwritten**. The import audit log is also append-only (agent core §6).

We must choose how Room handles duplicate imports for the same business keys within one survey week.

## Decision

### Average prices (`average_price`)

| Aspect | Choice |
|--------|--------|
| Primary key | Deterministic UUID from `(survey_week_id, state, municipality, fuel_product)` |
| Business uniqueness | `UNIQUE (survey_week_id, state, municipality, fuel_product)` |
| Insert strategy | `@Insert(onConflict = IGNORE)` — idempotent re-import |
| Updates | **No** `UPDATE` or `DELETE` on existing rows in v1 |

Re-importing the same week inserts zero duplicate average rows but **preserves** all existing records. Each import run appends new rows to `import_audit_log`.

### Station prices (`station_price`)

| Aspect | Choice |
|--------|--------|
| Primary key | Deterministic UUID from `(survey_week_id, cnpj, fuel_product)` |
| Business uniqueness | None (only primary key) |
| Insert strategy | `@Insert(onConflict = IGNORE)` on primary key |
| Updates | **No** `UPDATE` or `DELETE` on existing rows in v1 |

Same idempotent behaviour: re-import does not remove prior station rows.

### Audit log (`import_audit_log`)

Always **append** a new row per stage (`DISCOVERED`, `DOWNLOADED`, `IMPORTED`, `FAILED`). Never edited or deleted.

### Corrections from ANP (future)

When ANP republishes corrected values for an already-imported week, v1 keeps the first imported row (IGNORE). A future `import_run_id` column and append-only versioning (without `UNIQUE` on business key alone) may be introduced in Phase 5+ if product requires surfacing corrected values while retaining history.

## Consequences

**Positive**

- Re-import is safe and idempotent for sync retries (BR-011).
- BR-003 satisfied: no destructive writes to price tables.
- Audit trail grows monotonically for compliance and debugging.

**Negative**

- Corrected ANP values for the same week are not applied automatically in v1.
- Deterministic IDs prevent duplicate history entries for identical data (test expects stable row counts on re-import).

## Validation

- `ImmutableImportHistoryTest` (androidTest): double import → same price row count, increased audit entries, sample row ID unchanged.
