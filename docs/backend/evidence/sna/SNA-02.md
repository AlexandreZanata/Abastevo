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

## SNA-02B opening

B-BR-STATION-FACTS-02B: Canonical names use the pure name rule; only accepted
name corrections with an accepted audit decision outrank source names. Keep
source_display_name and legal/trade fields intact. A known curated label or
address is preserved rather than replaced by an importer. Address choices use
ANP > RFB > matched OSM > CNEFE > actual municipality name/UF, with unknown
source remaining unknown. Never produce an IBGE-code or coordinate address.

BUC-SNA-02B: Lock the station, verify the complete source evidence is bound to
its active CNPJ, reject incomplete/cross-identity/PMQC publication, compare
source edition clocks/checksums, then project source metadata and safe display
fields atomically. Older replays cannot undo newer editions. Accepting a name
correction updates its canonical name within the same audit transaction.
Prepared verification follows the applied source clock, independently of a
curated/approved display label, so replay cannot loop on such a difference.
Prove ordinary/prepared publication, source separation, accepted vs pending/
rejected corrections, curated preservation and concurrent ordering in PostGIS.

## SNA-02B checkpoint (2026-11-03)

Status: LOCAL_DONE
Validation: PASS

Tested parent 14604272 plus this child. RED covered the absent address rule,
an invalid UF fallback and a persisted trade-name fallback that failed to
recognize its previous source projection. GREEN recomputes names from retained
legal/trade fields; approved audited corrections win atomically with their
decision. Pending proposals and failed audit transactions never change the
canonical name. Source values/provenance stay unchanged. Curated display/address
values survive import; an unbound curated address has NULL source rather than
a false ANP label. Legacy unknown legal names remain unknown.

Append-only 000147 adds the edition clock/checksum pair and targeted indexes.
Publication locks a station and verifies complete source/CNPJ binding. Older
replays and concurrent callers cannot roll back a newer edition. Incomplete,
cross-identity and unbound prepared evidence are refused. Prepared pagination
and verification follow source clocks independently of the approved/curated
label, replacing the older display-equality retry guard. No location, status,
profile or community source assertion is rewritten.

Real PostGIS/race: adapters 27.241s, registry 34.257s, read 35.123s and directory
jobs 5.414s, including actual Rust→Go publication/recovery, concurrent source
updates and a simultaneous approved-name decision/import. Both 000146/000147
failure/recovery cases and partial clock/checksum negatives pass (final 3.081s).
Domain/reconcile race, full backend build, affected vet, sqlc vet/diff, formatting,
diff-check and secret review pass. No source acquisition or live data write.

SNA-02 remains IN_PROGRESS: SNA-02C must add an explicit bounded re-projection
job before SNA-03. It must not run on staging without separate approval.
