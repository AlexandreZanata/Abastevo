# P21-T01 — Contribution state and evidence contract

Status: LOCAL_DONE on `codex/phase-21-photo-contribution`. Issue: #81. Entry G20 satisfied (P20 merged `2788356`, issues #76–#79 CLOSED, wiki `472b331`). Binds B-BR-C01/C06 and BUC-C02/C03 to existing contracts. No app/backend change in this task; domain state contract only.

## Frozen presentation states

`domain/.../contribution/ContributionState.kt` — five states, review precedence REJECTED > DISPUTED > ACCEPTED > lifecycle:

| Local / remote / review | Status |
|---|---|
| never sent / in flight / queued receipt, no review | QUEUED (retryable only after transport failure) |
| transport failure, no review | QUEUED retryable |
| RECEIVED receipt, no review | PENDING |
| VALIDATED receipt, no review | ACCEPTED |
| review ACCEPTED / DISPUTED / REJECTED | ACCEPTED / DISPUTED / REJECTED (override lifecycle) |
| cancelled (any) | no status — never left the device |

The client never derives review outcomes; the review parameter mirrors the server. Server timestamps and consensus stay authoritative; client clocks only label capture age (`ContributionStalenessRule`).

## Retention boundary

`ContributionRetention`: photo copies are transient 24 h (`PortablePhoto.TRANSIENT_TTL_MILLIS`, server-enforced); status records are metadata and persist. Expiry never blocks fact validation — metadata-only drafts stay valid with weaker confidence, per `ContributionDraft`. Full object/cache/temp/restore proof belongs to P21-T04 against a real S3-compatible local service.

## Compatibility notes

- Optional-evidence API unchanged: `photoId` stays nullable; no mandatory-photo decision taken here (a future decision is documented separately and must not silently tighten this contract).
- Reuses `ContributionDraft` (exact milli-BRL, no client trust), `ContributionOutboxPorts` (stable id, revision retries, fresh nonce), `ContributionRemoteStatus`, `ContributionStalenessRule`.
- Condition qualifiers (APP/LOYALTY) and community consensus wiring arrive with backend coverage (P04/P22); nothing invented here.

## Validation

- `ContributionStateRuleTest` 9/0-fail (RED compile-fail → GREEN).
- `./gradlew :domain:test --no-daemon` (affected module).
- `git diff --check` clean; scoped secret review (no secrets/PII/GPS/photo content).
