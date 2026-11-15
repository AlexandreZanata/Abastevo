# EMP-METER-003 — Reading validation: monotonicity, retroactive inserts and suspect flags

Date: 2026-11-15.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Bounded F2 prerequisite of F4 (module METER, cut C1, P0, type DOM, size S) on
`codex/enterprise-f4-meter003`, opened from `origin/dev` `8c2ee654`
(migrations through `000215`). It classifies field meter readings as `VALID`
or `SUSPECT` with one validator shared by the server (Go) and the app
(Kotlin), stores the classification as append-only history, keeps suspect
readings out of the current value and re-evaluates suspect neighbours.
Synthetic data only; no Android UI, panel, route, job or release change.

Sources read: the EMP-METER-003 block (and METER-004/005 for the boundary) of
[ep05](../backlog/ep05-fleet-meters.md); [fleet](../modules/fleet.md) §4
BR-METER-001..026 (BR-METER-008..012, 017 owned here), UC-METER-01/02, §6.5,
§8, §10.2 limits, §10.4 examples A, B and E, §10.5 codes; BR-OFFL-006 and the
§10.2 outcome table of [10-offline-sync](../10-offline-sync.md); BR-TIME-006
(clock-skew limit, default 5 minutes) of
[04-business-rules](../04-business-rules.md); the METER-001 pure domain
([evidence](f2-meter001-reading-domain.md)), the METER-002 persistence and its
full closure ([evidence](f2-meter002-persistence-intake.md)), the OFFL-008
held review ([evidence](f2-offl008-real-review.md)) and the FUEL-002 embedded
reading ([evidence](f3-fuel002-fueling-persistence.md)); EDR-105..109,
EDR-173..177.

IDs used: B-BR-EMP-280..285, BUC-EMP-142..144, EDR-184..186 (all of them).
Migration `000216_organization_meter_reading_states.sql` (append-only), which
also carries the person-decision causes that EMP-METER-004 writes (EDR-184).

## Rules frozen before behaviour

Observed before any change: readings are insert-only (`org_meter_readings`
statement guard) with a `state` column that only ever received `VALID`; the
latest-valid query filtered `r.state='VALID'`; the signed handler and the
fueling handler refused any reading whose stored state was not `VALID`; the
reviewed-reading branch of the observation guard required `VALID`; the closed
sync protocol admits `ACCEPTED_FOR_REVIEW` only with an OFFL-008 review code
and review item; no runtime consumer reads the organization's
`policies.clock_skew_limit_seconds` and no category carries a rate limit; the
upgrade rehearsals of 000198/199/201/215 populate older images through the
current meter store.

Rules B-BR-EMP-280, B-BR-EMP-281, B-BR-EMP-282, B-BR-EMP-283, B-BR-EMP-284, B-BR-EMP-285, BUC-EMP-142, BUC-EMP-143, BUC-EMP-144 are recorded in the [product contract](../../product/ABASTEVO_EMPRESAS_CONTRACT.md) section "EMP-METER-003 — Reading validation: monotonicity, retroactive inserts and suspect flags" (frozen before behaviour; one wording refinement during GREEN, below).

Decisions EDR-184, EDR-185, EDR-186 are recorded in [16-decisions](../16-decisions.md).

One refinement after the first integration run, recorded before the fix: the
upgrade rehearsals of older images populate through the current store, so an
unclassified reading (installation anchor, switch off) must write no history,
exactly as before 000216. B-BR-EMP-284 now says "without history" for both.

## Implementation

- Pure validator (Go `fleet/domain/meter/validation.go`, Kotlin
  `com.anpfuel.domain.enterprise.meter.MeterValidation`): `Plausibility`
  (rate 20 000..200 000 m/h, default 150 000; skew ≥ 0 s, default 300),
  `Check(kind, series, candidate)` → state, canonical flags, `RETROACTIVE`,
  previous/next valid neighbour; `Reevaluate(kind, series)` → final changes of
  `SUSPECT` members until a pass promotes nothing, keeping `FUTURE_INSTANT`.
  Neighbours are `VALID` members only; both new pairs are rate-checked; exact
  arbitrary-width integers over nanosecond elapsed time (`math/big`,
  `BigInteger`); a candidate at or before the opening reading, unordered or
  duplicate series, bad states/flags and bad configuration are `INVALID`.
  Inputs are copied, never retained.
- Shared oracle `contracts/testdata/enterprise/meter/validation-v1.psv`
  (104 cases: POLICY 8, CHECK 79, REEVALUATE 17; SHA-256
  `65f0cc26b961c8f2cd661b11f89e8b242816a6d4426254efa8a942b4477bee95`) with
  its grammar in `VALIDATION.md`. Expectations were hand-written and
  cross-checked against an independent arithmetic model before freezing
  (`/data/dev/worktrees/f4-logs/meter003-oracle/build.py`: 104 cases,
  0 mismatches). Both consumers assert the digest, counts, unique IDs, every
  operation, every flag and the error code, and refuse grammar corruption.
- Migration 000216: `org_meter_flags_canonical(text[])`; the forced-RLS,
  insert-only (`org_asset_keep_history`) `org_meter_reading_states` with
  composite same-organization foreign keys to the reading, the acting
  membership and the superseding reading, CHECKs for flags/state, system vs
  person causes, reasons (`org_asset_text_valid` ≤ 280), intake = revision 1,
  terminal `VOIDED`, `CONFIRMED` → `VALID`, supersession link; unique void and
  unique supersession per reading; a BEFORE INSERT guard that serialises on the
  organization root and the meter and admits only revision = previous + 1,
  `INTAKE` equal to the recorded classification and the fleet §6.5
  transitions (no demotion of `VALID`, `VOIDED` terminal, no confirmation over
  a monotonicity flag, re-evaluation must change something and keep
  `FUTURE_INSTANT`, supersession by a reading of the same meter and segment
  created in the same operation). The 000201 observation guard is replaced
  with exactly one clause changed (the reviewed branch admits `SUSPECT`).
- sqlc (`db/queries/fleet`, 000216 added to the stanza): the latest-valid
  query selects the latest reading whose current state (latest revision, else
  recorded) is `VALID`; `MeterSegmentSeries`; `InsertMeterReadingState`; the
  reviewed insert takes the classification; the resolution read returns the
  held value, instant and original ACK receipt.
- Store (`fleet/adapters/meter_validation.go`, `meter_codec.go`,
  `meter_review_actions.go`): the embedded path (signed intake and fueling)
  classifies under the root, asset and meter locks with the database clock
  read just before the insert, stores the recorded classification and
  revision 1, and re-evaluates when the new reading is `VALID`, all in the
  caller's transaction; the accepted held reading locks the meter and is
  classified at its original receipt (revisions under the decision's
  operation). The installation opening reading and the switched-off path
  write no history. The signed and fueling handlers accept a stored
  `SUSPECT` reading as evidence (`ACCEPTED`, revision 1, byte-identical
  replay).
- Configuration: `ANPFUEL_ENTERPRISE_METER_PLAUSIBILITY` (default `true`) →
  `enterprisesuite.Options.MeterPlausibilityOff` → the default policy on the
  meter and review stores; `false` is the rollback.

## Acceptance and negatives

| Item | Test(s) |
|---|---|
| AC1 example A: `VALID` + `RETROACTIVE`, intervals 210 000 + 322 000 = 532 000 | oracle `example-a-retroactive-valid`; Go `TestValidationExamplesAAndB`; Kotlin `examples A and B and the retroactive intervals`; PostGIS `TestMeterValidationExamplesThroughTheEmbeddedPort` (current value stays R2) |
| AC2 example B: `SUSPECT` `READING_BELOW_PREVIOUS`, current value unchanged | oracle `example-b-typing-error`; PostGIS examples test (latest stays R2; replay keeps one revision); signed `TestMeterValidationSignedIntakeAcceptsSuspectAndReplaysTheAck` |
| AC3 194 400 000 m one day after 184 190 000 m → `IMPLAUSIBLE_RATE` | oracle `ac3-implausible-rate`; PostGIS examples test |
| AC4 hour meter above elapsed + 300 s | oracle `hour-ac4-boundary-valid` / `hour-ac4-more-than-elapsed`, hour-meter nanosecond and same-instant cases |
| AC5 app warns with the same validator and still saves | Kotlin `the device warning is a classification that never blocks saving` (SUSPECT verdict, then appended to a `MeterSeries`); the warning UI is retained (no screen in this task) |
| Equal to predecessor is valid | oracle `equal-to-predecessor`, `equal-to-successor-retroactive` |
| Two readings at the same instant, different values | oracle `same-instant-within-1000` / `-above-1000` / `-lower-value`, hour `-300`/`-301`; ordering by receipt and identifier |
| Reading before the opening reading | oracle `before-opening-instant` / `-receipt` / `-identifier` (INVALID); PostGIS service window `TestMeterEmbeddedRejectsActualOneNanosecondBeforeServiceBeforeInsert` (unchanged METER-002 refusal) |
| Instant later than receipt + skew | oracle `future-*` (exact limit valid, +1 ns suspect, skew 0, max skew); signed intake future reading `SUSPECT` `FUTURE_INSTANT` |
| Suspect neighbour ignored | oracle `suspect-predecessor-ignored`, `suspect-successor-ignored`, `example-b-suspect-predecessor-ignored`; PostGIS R5 after R4 |
| Re-evaluation of flagged neighbours | oracle `reevaluate-*` (fixpoint, keep FUTURE, never demote, earliest first); PostGIS `TestMeterValidationReevaluatesSuspectNeighboursInTheSameTransaction` (same transaction and operation, rollback leaves it suspect) |
| Fueling embedded reading gets the same validation | `TestMeterValidationAppliesToTheFuelingEmbeddedReading` (accepted fueling, `SUSPECT` reading, byte-identical replay, plausible one becomes current) |
| Accepted held reading | `TestMeterValidationClassifiesAnAcceptedHeldReading` (classified at its original receipt, `SUSPECT`, receipt kept) |
| Concurrent pair on one meter, both orders | `TestMeterValidationConcurrentPairOnOneMeterSerialisesInBothOrders` (second blocked behind the first's locks, observed with `pg_blocking_pids`) |
| Replay | examples test (embedded replay, one revision); signed and fueling byte-identical ACK |
| Rollback switch | `TestMeterValidationSwitchOffStoresValidWithoutHistory`; config `TestEnterpriseMeterPlausibilityDefaultsOnAndValidatesSwitch` |
| Forced RLS, insert-only, guards | `TestMeterReadingStateSchemaGuardsTheHistory` (14 refused writes, void allowed then terminal, UPDATE/DELETE/TRUNCATE 55000, no rows without organization, foreign organization 42501) |
| Populated upgrade 000215 → 000216 | `TestMeterReadingStatesPopulatedUpgradeFailureRecoveryAndPreviousImage` (injected failure leaves schema, guard and data; recovery once; no-op replay; previous image; only the one guard clause changed; no fabricated history; guard governs legacy readings) |

## Validation — RED → GREEN and gates

Environment: worktree backend, Go 1.27.4 toolchain, `GOCACHE=/data/dev/cache/f4-go-build`,
shared PostGIS `127.0.0.1:32775` through orgtest isolated child databases,
JDK 17 under the shared Gradle lock. Logs in `/data/dev/worktrees/f4-logs/meter003-*`.

- Domain RED (compiled stubs returning a non-domain error):
  `go test -json -count=1 ./internal/modules/fleet/domain/meter/ -run TestValidation`
  → `meter003-go-domain-red.jsonl`: 6 of 7 top-level fail, 104 of 104 oracle
  subtests fail (the loader-grammar baseline passes). Kotlin RED (`TODO()`
  stubs), `:domain:test --tests '…EnterpriseMeterValidation*'` →
  `meter003-kotlin-red.log`: 109 tests, 108 failures, 0 errors (the grammar
  baseline passes).
- Domain GREEN: Go meter package `-race` with coverage →
  `meter003-go-domain-green.jsonl`: 23 top-level / 459 subtests pass,
  `validation.go` 100% of statements, package 94.8%. Kotlin
  `:domain:test --tests 'com.anpfuel.domain.enterprise.*'` →
  `meter003-kotlin-green.log`: 2 394 tests, 0 failures/errors/skips (meter
  431, of which the new validation classes 109).
- PostGIS RED: the new integration tests against compiled stubs of the two
  adapter functions (`classifyCandidate` always `VALID`, no history), then
  restored byte for byte → `meter003-integration-red.jsonl`: 7 of 9 top-level
  fail (the switched-off and upgrade tests pass by construction).
- Expectation corrections during GREEN, recorded before the fix: the
  examples test first expected R4 unchanged after a valid reading at its own
  instant, but B-BR-EMP-282 checks the successor pair too, so R4 gains
  `IMPLAUSIBLE_RATE` (revision 2); the signed test first declared readings
  hours before the fixture's ADMIN grant, which the METER-002 classifier holds
  as `PERMISSION_CHANGED`, so the readings are declared after the grant start
  (the future reading is proven on the embedded port instead). One existing
  unit case, `TestMeterIntakeRejectsMismatchedActualEvidenceAndLateDenial/state`,
  treated a stored `SUSPECT` as a mismatch; B-BR-EMP-284 makes it legitimate
  evidence, so the case moved to the positive
  `TestMeterIntakeAcceptsASuspectRecordedClassification`. Two adapter defects
  found by the first regression run were fixed: a shadowed `err` in the
  reviewed-reading path and history writes on older upgrade images (the
  B-BR-EMP-284 refinement above); the shared `meterStore` helper grants the
  history table only when it exists.
- PostGIS GREEN focused (`-race -tags=integration`, `^(TestMeterValidation|TestMeterReadingState)`)
  → `meter003-focused-green.jsonl`: 9 top-level / 16 subtests pass.
- Affected race union `go test -json -race -tags=integration -count=1 -p 2 -timeout=60m ./internal/modules/organization/adapters ./internal/modules/fleet/... ./internal/modules/fueling/... ./internal/modules/orgsync/... ./internal/modules/orgexport/... ./internal/platform/migrate/... ./internal/platform/enterprisesuite/... ./internal/platform/config/... ./cmd/api/...`
  → `meter003-race-union.jsonl`: 22 packages, 793 top-level / 5 213 subtests
  pass; the only 12 failures were the MinIO-dependent tests (missing
  `ANPFUEL_TEST_STORAGE_ENDPOINT`; organization adapters 1 317.6 s). Those 12
  (plus `TestExportDataAuthorityFailsClosed`) rerun with a throwaway MinIO
  (`ANPFUEL_VALIDATION_PROJECT=f4-meter003-s3`) → `meter003-storage-rerun.jsonl`:
  13 top-level / 86 subtests pass.
- Untagged `go test -count=1 -timeout=45m -p 2 ./...` → `meter003-untagged-all.log`:
  214 packages ok, 78 without tests, 0 failures.
- `go build ./...`, `go vet ./...`, `go vet -tags=integration ./...` clean;
  toolchain `gofmt -l ./internal ./cmd ./db` empty; staticcheck (go1.27.1)
  plain on fleet, fueling, config, enterprisesuite, cmd/api and
  `-tags=integration` on organization adapters, fleet and fueling: no
  findings; `sqlc vet`, `sqlc generate` and `sqlc diff` clean (fleet stanza
  plus 000216); `git diff --check` clean; `check_plan.py` 348 tasks / 545
  rules / 167 use cases / 145 screens, 0 problems.
- Secret/PII review of the staged diff: synthetic UUIDs, synthetic labels and
  the documented local MinIO validation credentials only in the command
  environment (not in the diff); no credential, real name, plate, CNPJ or
  location.

## Retained (not part of this task's acceptance)

- The app warning UI on SCR-METER-01 (the shared Kotlin validator is ready;
  no Android screen changes here).
- `ACCEPTED_FOR_REVIEW` with the flag as code for a suspect standalone reading
  (fleet §8; EDR-186): needs review codes in the closed protocol and client.
- Per-category rate limits and the organization's versioned clock-skew
  setting as validator inputs (EDR-185); the defaults apply to every category.
- `POSSIBLE_ROLLOVER` and segment boundaries (EMP-METER-005); confirmation,
  correction, void, the suspect list and `meter.usage.recalculated`
  (EMP-METER-004); the maintained projection and recalculation for intake
  (EMP-METER-006).
- The RPT-002 reading list still exports the recorded classification
  (`org_meter_readings.state`); the as-of current state belongs with the
  first correction producer (EMP-METER-004).
- Capacity: the whole segment series is read per classified reading; no
  measurement is claimed.
- Production grants for `org_meter_reading_states` (backend README) and the
  migrator ownership of `org_meter_observation_guard()`.

## Shared files touched

`docs/product/ABASTEVO_EMPRESAS_CONTRACT.md` (section appended),
`docs/enterprise/16-decisions.md` (EDR-184..186 appended),
`docs/enterprise/backlog/ep05-fleet-meters.md` (METER-003 state and note),
`backend/README.md` (new subsection), `backend/sqlc.yaml` (fleet stanza).
No route, OpenAPI, matrix or check-manifest change.
