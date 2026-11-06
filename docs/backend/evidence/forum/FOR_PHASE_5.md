# Forum Phase 5 — deterministic Abastevo bulletins

Status: **LOCAL_DONE / INTEGRATION_PENDING** for FOR-25 and FOR-26.
Validation: PASS (methodology, exact aggregates, revision pair, coverage,
duplicate handling, overflow, absent-region refusals, deterministic template,
rounding and correction identity).
Delivery: LOCAL_DONE / INTEGRATION_PENDING.

## Authorized scope and baseline

FOR-25 (Freeze methodology and aggregate port) and FOR-26 (Templates,
provenance and correction identity) are the first microtasks of Forum Phase 5,
authorized by the user's Forum implementation directive.
FOR-25 baseline: maintained `dev` `610a9079` (FOR-24).
FOR-26 baseline: maintained `dev` `d5ccb537` (FOR-25).
GitHub `main`: `99f21830`.
No Android, HTTP route, worker job, persistence link or published bulletin is
added by these tasks.

Frozen normative behavior: [FORUM_ABASTEVO_BULLETINS.md](../../FORUM_ABASTEVO_BULLETINS.md),
methodology `forum-official-mean-v1`. Product rules: B-BR-FOR-15 and BUC-FOR-08.

## Implemented behavior

- Official-owned bounded port `BulletinReader` with
  `LatestTwoCurrentPublishedRevisions` and `ReadBulletinSamples`.
- SQL `db/queries/official/bulletin.sql` (sqlc-owned) selects:
  - the current published revision of the latest survey window;
  - the current published revision of the latest strictly earlier window;
  - one latest observation per station for the requested fuel/unit (same
    `collected_on DESC, id` rule as ranking);
  - exact `numeric` sum text, observation count, sampled stations, raw source
    observations and any-product source-station denominator per UF.
- Pure application rules:
  - methodology/fuel/unit validation against the Official ranking vocabulary;
  - >=30 sampled stations in both samples;
  - positive current and previous sums;
  - exact reduced rational mean with positive denominator;
  - deterministic UF order, typed skip reasons for absent/ineligible regions;
  - invalid aggregates, inconsistent coverage, duplicate UFs and missing
    revision pairs are refusals, never zero-filled results;
  - `ParseMilliSum` refuses zero, negative, malformed and `int64` overflow.
- Input manifest carries both revision UUIDs, survey periods, published time,
  source URL/checksum, parser version, methodology version, fuel and unit.

## FOR-26 template, rounding and correction identity

Implemented in `backend/internal/modules/forum/domain/bulletin.go` with
`bulletin_test.go`:

- `BulletinTemplateVersion = forum-bulletin-v1` and methodology
  `forum-official-mean-v1`; original plain pt-BR text with no LLM, network, HTML
  or Markdown.
- Exact rational mean and delta display rounded half away from zero to
  0.001 BRL/unit; percent rounded half away from zero to 0.1%; comma decimal
  separator, explicit `+`/`-` signs and unsigned zero.
- Title/excerpt/body carry both periods, sample and coverage counts, revision
  IDs, checksums, source URLs, parser versions, methodology/template versions
  and the explicit non-claims.
- Key = template + current revision UUID + previous revision UUID + UF + fuel +
  unit. Repeated identical input is byte-identical, so the same key is a no-op.
- `BulletinIdentity.IsCorrectionOf` recognizes a newer revision pair for the
  same two survey windows; different windows are a new bulletin. FOR-27 persists
  the `supersedes_post_id` link after review.
- Input validation refuses unknown UF/fuel/unit, malformed or overlapping
  periods, missing provenance, sub-30 samples, inconsistent coverage, zero or
  negative sums, unsupported percent range and content over the Forum field
  limits.

Tests: `go test -race -count=1 ./internal/modules/forum/domain/...` PASSED
(1.017s), including exact title/excerpt fragments, byte determinism, signed
positive/negative/zero and half-away ties, correction direction, source checksum
rendering, HTTPS/control-character refusals, oversized provenance and
arithmetic-range refusal. `go build ./...`, forum unit race suite, `go vet`,
`sqlc vet`, domain staticcheck, gofmt, diff-check and secret scan PASSED.

## Verification

Targeted real-PostGIS tests
`go test -count=1 -tags=integration ./internal/modules/official/adapters/read -run 'TestForumBulletin' -v`
PASSED (6 tests, 4.403s):

- two current windows with a same-window correction and a duplicate observation;
- corrected pointer used instead of the superseded current-week revision;
- latest observation wins without double-counting (source observations 36,
  sampled stations 35);
- exact sums/means for SP and MG, UF ordering, RJ below-threshold skip;
- no-pair revision refusal;
- `math.MaxInt64` current rows refused as sum overflow;
- wrong fuel/unit query refused;
- mixed-unit rows absent rather than silently converted.

Affected suites:

- `go test -race -count=1 ./internal/modules/official/...` PASSED (application
  1.011s, adapters/HTTP/jobs/source/anp/domain green).
- `go test -race -count=1 -tags=integration ./internal/modules/official/...`
  PASSED (read 12.092s, adapters 23.399s, HTTP 6.477s, jobs 5.100s, others
  green).
- `sqlc vet`, `sqlc diff`, `go vet ./internal/modules/official/...`, gofmt on
  changed Go files and `git diff --check` PASSED.
- `scripts/scan-secrets.sh` PASSED before commit; synthetic fixtures only.
- No Android test/build was required because no Android source changed.

## Blocked, deferred and next

- FOR-27 owns worker/admin/Android review, persistence and publication; the
  template task does not write a bulletin row or `supersedes_post_id`.
- No bulletin was generated or published; no source approval, operator
  coverage, deployment, staging, device acceptance or merge is claimed.
- Next microtask: FOR-27 (worker/admin/Android bulletin checkpoint).
