# SNA-02 — Stored source facts and projections

Status: IN_PROGRESS

Execution bounds declared before behavior: SNA-02A adds nullable append-only
storage and lossless assertion round trips; SNA-02B applies tested name/address
projection while preserving reviewed corrections and curated facts; SNA-02C
adds the bounded, resumable, concurrent-safe re-projection job. Complete all
three before SNA-03. No database migration or job runs against staging without
separate owner approval.

B-BR-STATION-FACTS-02A: Keep legal_name verbatim, trade_name separately,
address_source in the documented enum, the source's own display name and
trade-name provenance independently. Legacy unknown fields remain NULL;
never reconstruct an unknown legal name from a community/canonical label.
Immutable source rows are not rewritten on replay. Migration failure rolls
back every new column and retry succeeds without losing old identities/facts.

BUC-SNA-02A: Stage and reload source facts through both batch and ordinary
registry paths, including review evidence reads. Reject unsupported address
sources in SQL. Verify upgrade/failure/recovery and immutable replay against
real disposable PostGIS; do not claim canonical or public projection yet.

## SNA-02A checkpoint (2026-11-03)

Status: LOCAL_DONE
Validation: PASS

Tested parent a3cebed4 plus this child. RED: source metadata was absent from
the assertion boundary. Append-only migration 000146 adds nullable canonical
metadata and separate assertion legal/trade/source-display/address-source/name
provenance fields; no guessed historical backfill. Known ANP components are
marked anp, municipality text alone municipality, absent text remains unknown.

GREEN: actual Rust v3→Go→PostGIS preserves names, source display and pinned
provenance. Direct staging/list/review-evidence reads round trip raw legal
names; immutable replay cannot overwrite them; SQL rejects unknown address
sources. A deliberately failed 000146 rolls back its DDL, preserves legacy
station identity/name/address and succeeds on recovery/idempotent rerun.
Targeted critical cases 4.407s; full real PostGIS/race registry 33.479s, read
35.022s and publication/recovery/concurrency adapters 22.939s. Full backend
build, affected vet, domain guard, sqlc vet/diff, formatting, diff-check and
secret review PASS. No national fetch, import or staging mutation.

The owner-approved read-only baseline is preserved in
[staging-database-baseline-2026-11-03.json](staging-database-baseline-2026-11-03.json).
It counts database station rows and charging tables only; it is not an OSM
extract/device map-package inventory or a nationwide charging coverage claim.

Next: SNA-02B canonical projection, then SNA-02C bounded re-projection job;
SNA-02 as a whole remains IN_PROGRESS and API additions are still pending.
