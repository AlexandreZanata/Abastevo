# Staging landing/portal performance (#155) — what remains

Status: PLANNED (2026-10-27). The #155 work is STAGING_DONE and merged into
`codex/phase-integration-20261027` through #162; PR #155 itself is still
open against `main`. None of the items below blocks staging; they are the
follow-ups #155 recorded as "measured, not changed" or production-only.

## Context

- PR #155 "Not included / follow-ups"; evidence
  `docs/backend/evidence/STAGING_LANDING_PORTAL_2026-10-26.md` ("Limits and
  follow-ups", addendum and incident); PROGRESS entry "Staging landing +
  portal search + ingestion 2026-10-26" ("Directory CPF-name projection as
  its own task").

## Remaining work

1. Close PR #155 as superseded once the integration PR merges. Acceptance:
   closing comment links the merged PR (DELIVERY_WORKFLOW "One PR per
   planning stage"); its unchecked "required Quick verification" box is
   satisfied by the integration PR's checks, not re-run on #155.
2. Registry `load_publish` batching. Measured only: one row per round trip,
   164 s/day, inside its 18 min bound. Acceptance (if picked up): batched
   staging with identical published rows on real PostGIS, before/after
   timing recorded; otherwise record it as accepted as-is.
3. SP search shard redesign. The SP shard stays 461 KB gzip (search v2
   prefix shards are ≤ 92 KB). Acceptance: shard layout that bounds the
   largest file, `portalcheck` strict verification still green, browser
   search unchanged.
4. Sitemap index for indexable national builds. Indexable sitemaps would
   exceed 50,000 URLs; staging is noindex, so this is production-only
   (G09 scope). Acceptance: sitemap index split under the 50,000-URL limit,
   tested in the landing/portal build; deferred until a production build is
   requested.
5. Directory CPF-name projection (verify scope). B-BR-017
   `kernel.PublicStationName` already strips CPF-shaped tokens on every
   public read and the export; PROGRESS names a separate "Directory CPF-name
   projection" task. Acceptance: decide whether the stored projection must
   also be minimized; if yes, its own B-BR/BUC entry, migration review and
   PostGIS tests.

## Blockers and owner decisions

- Items 2–3: owner priority (no current SLO breach recorded).
- Item 4: production scope (G09), owner request required.
- Item 5: privacy scope decision (owner).

## Delivery: one branch, one PR

Items 2, 3 and 5 (if approved) form one stage on branch
`codex/staging-perf-followups`, one PR to `main`. Item 1 is a PR closure, not
a PR. Item 4 waits for the G09 production stage and ships with it.
