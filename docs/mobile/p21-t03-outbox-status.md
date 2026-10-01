# P21-T03 — Outbox replay and contributor status

Status: LOCAL_DONE on `codex/phase-21-photo-contribution`. Issue: #83. Binds B-BR-C01/C06 and BUC-C02/C03 to existing contracts. No schema migration (read-only addition), no backend change, no new permission.

## What this task adds

- Domain port `ContributionOutboxRepository.listOwned()` with `OwnedContribution` (id, revision, attempts, phase) and fail-closed phase mapping (`OwnedContributionPhase.phaseOf` throws on any stored string outside QUEUED/IN_FLIGHT/FAILED/CANCELLED — an ACKED row would mean delete-on-ack was bypassed).
- Data `RoomContributionOutboxRepository.listOwned()` over the existing DAO `listAll()` (no DAO/entity change, no migration).
- Application `GetOwnedContributionsUseCase` mapping durable phases to the frozen P21-T01 states (FAILED → Queued retryable, CANCELLED → no status) plus `CancelOwnedContributionUseCase` (blank id rejected).
- App Perfil "My contributions" card + `ContributionStatusViewModel` (loading/content/empty/error, cancel reloads); cancelled rows render an explicit cancelled label since the contract gives them no status.
- Location integrity: encoded payload test proves no lat/lng/gps/location/exif/contributor/signed-url/mock/simulated keys leave the device (`PortableLocation` DENIED/UNKNOWN/SIMULATED classification itself stays tested in domain).

## Reused (not rebuilt)

Stable-id enqueue with revision bump, stale-ack protection, FAILED backoff, dispatch nonce (all covered by existing `RoomContributionOutboxRepositoryTest`), `ContributionWorker` FIFO dispatch, `PortableOutbox` state machine.

## Explicitly deferred (not waived)

- Worker unit test: needs work-testing/Robolectric scaffolding absent from this tree; dispatch decisions stay covered at repo/port level only.
- Restart-restore proof against a real Room file and token-expiry replay: owned by P21-T04 storage matrix / P22 auth flows with real services.
- Review feed (pending/accepted/disputed/rejected from server): arrives with P04/P22; local mapping defaults to lifecycle states only.
- Strings en + pt-BR; other locales fall back to en.

## Validation

- `RoomContributionOutboxRepositoryTest` 6/0-fail, `GetOwnedContributionsUseCaseTest` 2/0-fail, `EnqueueContributionUseCaseTest` 6/0-fail (payload location-integrity), `ContributionStatusViewModelTest` 4/0-fail (RED compile-fail → GREEN).
- `./gradlew :domain:test :application:test :data:testDebugUnitTest :app:testDebugUnitTest :app:assembleDebug` (affected suites).
- `git diff --check` clean; scoped secret review (no secrets/PII/GPS/photo content).
