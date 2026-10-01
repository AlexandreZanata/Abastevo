# P10-T05 — Contribution outbox and direct media flow

Status: LOCAL_DONE on `codex/phase-10-functional-integration` (phase PR
#54 pending, fifth task push). Issue: #49 (slice 5 of 8 local;
P10-T09 pilot stays deferred until G18/G09 and is not part of G10-LOCAL).
No ANP path touched, no legacy enum rename, no backend change.

## Slice acceptance (frozen before coding)

- T05 (this commit) — Retry durable commands safely while offline:
  domain `ContributionDraft` (UUID station, milli 1..1_000_000, BRL,
  non-blank unit/condition, no contributor/GPS/EXIF from client) +
  `ContributionStalenessRule` (24 h transient / +5 min future → historical,
  old never relabelled fresh) + `ContributionOutboxConfig` default OFF;
  application `EnqueueContributionUseCase` (Disabled / Queued with stable
  `client_submission_id`, duplicate bumps revision, attempts kept) +
  `ContributionOutboxRepository`/`ContributionSubmissionGateway` ports
  (queued/received/validated explicit, fresh nonce per send); data Room
  `contribution_outbox` (record shape field-for-field via
  `OutboxCommandMapper`) with explicit MIGRATION_5_6, `RoomOutboxRepository`
  (stale ack ignored, FAILED backoff), OkHttp
  `ContributionUploadHttpClient` (reserve → PUT → complete → observation,
  `Idempotency-Key`/`X-Nonce`, expired photo → metadata-only, finalize
  failure throws, network-only) behind preview `.invalid` base, `ContributionWorker`
  (CONNECTED, KEEP per command, flag OFF pauses, markDispatched/Acked/Failed) +
  `ContributionWorkScheduler`, `ContributionOutboxFlagStore` default OFF.

## What changed

- `domain/.../feature/ContributionOutboxConfig.kt` — DISABLED default.
- `domain/.../model/ContributionDraft.kt` — BUC-003/B-BR-003/005 validation.
- `domain/.../rule/ContributionStalenessRule.kt` — fresh/historical labelling.
- `domain/.../repository/ContributionOutboxPorts.kt` — Room/network ports.
- `application/.../port/ContributionOutboxFlagProvider.kt`.
- `application/.../usecase/contribution/EnqueueContributionUseCase.kt`.
- `data/.../preferences/ContributionOutboxFlagStore.kt` — default OFF.
- `data/.../entity/ContributionOutboxEntity.kt` + `dao/ContributionOutboxDao.kt`.
- `AnpFuelDatabase` v5→v6, `MIGRATION_5_6`, `DatabaseModule`, `6.json`.
- `data/.../repository/RoomContributionOutboxRepository.kt`.
- `data/.../remote/ContributionUploadHttpClient.kt` + `di/ContributionModule.kt`
  (preview `.invalid`, never resolves).
- `data/.../worker/ContributionWorker.kt` + `ContributionWorkScheduler.kt`.
- `RepositoryModule`/`UseCaseModule` bindings.
- Tests: 4 draft + 3 staleness + 5 enqueue (disabled/fresh/historical/
  duplicate/blank) + 1 migration-unit + 4 repo (duplicate/stale-ack/
  backoff/nonce-label) + 4 upload (metadata/photo/expired/finalize-fail) +
  1 androidTest V5→V6 (CI).

## RED → GREEN

- RED proven by new symbols before implementation (same pattern as
  P10-T01…T04): `ContributionDraft`, `ContributionStalenessRule`,
  `ContributionOutboxFlagProvider`, `EnqueueContributionUseCase`,
  `ContributionOutboxDao`, `ContributionUploadHttpClient`,
  `ContributionWorker` did not exist; new tests referenced them.
  GREEN after adding bounded adapters only.

## Validation (exact commands, this host)

```sh
./gradlew :domain:test --no-daemon --rerun-tasks
./gradlew :application:test --no-daemon --rerun-tasks
./gradlew :data:testDebugUnitTest --no-daemon --rerun-tasks
./gradlew :app:testDebugUnitTest :app:assembleDebug --no-daemon
bash scripts/check-mobile.sh --static-only
git diff --check
bash scripts/scan-secrets.sh
```

Outcome: `:domain:test` + `:application:test` + `:data:testDebugUnitTest`
green (incl. 21 new unit suites listed above); `:app:testDebugUnitTest`
+ `assembleDebug` BUILD SUCCESSFUL (ANP screens unchanged);
`check-mobile --static-only` ok; `diff --check`/secrets clean.
Limits: `:data:connectedDebugAndroidTest` + `:app:connectedDebugAndroidTest`
NOT run locally (no emulator); `V5ToV6DatabaseMigrationTest` added for
CI/phase exit. No backend change; no production URL invented. B-BR-010/011/015:
no public evidence, no GPS/EXIF/signed URL in payload/logs, quota/ownership
server-side.
