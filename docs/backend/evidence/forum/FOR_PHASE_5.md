# Forum Phase 5 — deterministic Abastevo bulletins

Status: **LOCAL_DONE / INTEGRATION_PENDING** for FOR-25 only.
Validation: PASS (methodology, exact aggregates, revision pair, coverage,
duplicate handling, overflow and absent-region refusals).
Delivery: LOCAL_DONE / INTEGRATION_PENDING.

## Authorized scope and baseline

FOR-25 (Freeze methodology and aggregate port) is the first microtask of
Forum Phase 5, authorized by the user's Forum implementation directive.
Baseline: maintained `dev` `610a9079` (FOR-24); GitHub `main` `99f21830`.
No Android, HTTP route, worker job or published bulletin is added by this task.

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

- FOR-26 owns deterministic Portuguese templates, display rounding and
  correction identity; FOR-27 owns worker/admin/Android review and publication.
- No bulletin was generated or published; no source approval, operator
  coverage, deployment, staging, device acceptance or merge is claimed.
- Next microtask: FOR-26 (templates, provenance and correction identity).
