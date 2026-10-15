# FQ-04c — Immutable revision provenance

Status: LOCAL_DONE / INTEGRATION_PENDING. Date: 2026-10-14.
Base: maintained dev `658c3c50` (FQ-02B). Depends: FQ-01/FQ-03/FQ-04a/FQ-04b.
B-BR-FQ-04/05/12/16, BUC-FQ-01/03/09. No score, migration of existing facts,
coverage claim or rollout acceptance follows merely from the schema.

## Behavior before implementation

Manifests, resources and periods lived only at the mutable edition level:
a correction rewrote the edition summary, so a frozen revision could no
longer explain which counts, hashes, resources and periods it published.
Concurrent identical corrections could also fork extra revisions, and
legacy revisions carried no explicit provenance state at all.

## Delivered

- `backend/db/migrations/000098_quality_revision_provenance.sql` (new,
  append-only): `quality_revision_provenance` (one row per revision:
  VERIFIED with reconciled counts, manifest hash and publication key, or
  LEGACY_UNVERIFIED with everything else NULL and enforced by CHECK),
  `quality_revision_resources` and `quality_revision_periods` frozen
  per-revision snapshots, a partial unique index on publication key, and
  a backfill marking every pre-existing revision LEGACY_UNVERIFIED
  without inventing counts or hashes.
- Publisher (`quality/adapters`): editions are locked (`LockEditionByKey`
  FOR UPDATE); replay converges by publication key (same content returns
  the published revision, superseded content is refused to late retries
  via `application.ErrSupersededPublication`, which discovery
  acknowledges without watermark drift); new revisions record frozen
  provenance, resources and periods; corrected resource hashes are also
  registered in the mutable edition discovery index. The publication key
  covers source/stream/edition/manifest/counts/resources/periods/links
  and deliberately excludes the discovery predecessor/watermark so a
  replay cannot change identity merely because the import cursor advanced.
- Reads (`quality/adapters/read`): the lease joins per-revision
  provenance and periods; LEGACY_UNVERIFIED leases serve an explicit
  unavailable lease ("revision provenance unverified") instead of
  fabricated denominators. Verified reimport appends a new revision;
  original rows and migration checksums are preserved.
- sqlc `quality` stanza extended with the 000098 schema; `sqlc vet` and
  `generate` clean, generated code committed.

## Validation

RED→GREEN: the new suites passed first run; two acceptance gaps found on
review were closed with new assertions (per-revision period snapshots on
the events correction path; LEGACY_UNVERIFIED state plus unavailable
lease plus intact frozen provenance on the upgrade/replay path). No
production assertion was weakened.

- `go test -race -count=1 -tags=integration ./internal/modules/quality/...`:
  all suites PASS — new `TestRevisionProvenanceSurvivesCorrection`
  (differing-count correction keeps first revision counts/hash/resource
  intact, corrected source registered in the discovery index),
  `TestRevisionProvenanceConcurrentCorrectionConverges` (8 identical
  corrections under a lock barrier converge to one id, exactly 2
  revisions), `TestRevisionProvenancePeriodSnapshots` (exact per-revision
  period/resource/provenance snapshots on both sides of a correction),
  extended `TestNativePMQCResultExpansionUpgradeReplayAndRollback`
  (legacy seed → 097+098 upgrade replay, LEGACY_UNVERIFIED asserted with
  no invented hash, lease unavailable, history intact, failed correction
  keeps pointer and frozen provenance).
- `go build ./...`, `go vet -tags=integration`, `gofmt -l` clean;
  `sqlc vet`/`diff` clean.
- `git diff --check` clean; new-file Markdown links resolve;
  scoped secret/PII scan clean (synthetic CNPJs/URLs only).

## Recovery

Retain the original publication: superseded revisions keep serving
their rows with frozen provenance; replaying a manifest returns the
same revision id; failed corrections roll back whole transactions.
Legacy revisions require verified source reimport into a new revision
before explained publication/scoring. Never edit 000093–000098. Next:
immutable per-revision provenance is LOCAL_DONE; document-free FQ-11
and calibrated remaining FQ scope per FQ-02B sequencing (FQ-10 D-held).
