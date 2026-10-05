# P37-T03 — Private storage and all-copy expiry

Status: LOCAL_DONE on `codex/phase-37-live-contributions`. Task: P37-T03.
Binds all-copy 24h maximum (P15 budgets, P21-T04 storage evidence) and
B-BR-011 (no GPS/EXIF/identity in media paths). Verification slice: no
new storage semantics were needed; the VPS smoke record shows no
attached private media, so the live photo journey is explicitly
BLOCKED_MEDIA — never fabricated green.

## Behavior (verified, not rebuilt)

- Client: `PortablePhoto.TRANSIENT_TTL_MILLIS` (24h) with
  `isTransientExpired` checked on launch/resume/read; photo cache
  `sweepExpired`; wire cap `fitsWireCap`; expired photos fall back to
  metadata-only with the payload's own historical label (gateway test
  proves it — an old photo is never relabelled fresh).
- Outbox: stable ids, photo-id references (never bytes in queue
  payloads), cancel/drop semantics, restart recovery (existing suites).
- Server: private evidence intake/processing/deletion, bounded
  immutable processing, owner/reviewer-only access, expiry jobs,
  backup/restore with revocation replay and expired-proof purge
  (existing suites, rerun below against disposable PostGIS).
- Staging reality: the current VPS smoke has no private media
  attached and storage provisioning needs its recorded scope (not done
  by any app plan). Only source/isolated storage tests pass here;
  synthetic images only, restricted test namespace, no production
  photos in Git/fixtures/logs.

## Validation

- Android/domain: `PortablePhotoTest` + `ContributionStalenessRuleTest`
  + `ContributionStateRuleTest` PASS; data media + outbox +
  `ContributionUploadHttpClientTest` (incl. expired→metadata-only,
  object-success/finalize-failure) PASS.
- Backend: `go test ./internal/modules/evidence/...
  ./internal/modules/privacy/...` PASS (11 pkgs); real-PostGIS
  `-tags=integration` evidence + privacy PASS (11 pkgs, incl. adapters
  7.2s/5.4s).
- Contracts unchanged: `vacuum lint` + `apicontract` PASS; live `curl`
  60 unchanged (no bypass, no trust-all, no HTTP fallback).
- `git diff --check` PASS; `scan-secrets.sh` PASS (no photos/secrets/
  PII/GPS in diff; only docs change in this task).

## Limits and next

- BLOCKED_MEDIA: live photo capture/upload/expiry unverified (no
  attached VPS private storage + TLS trust gap). Synthetic-only proof
  here; real-provider/storage proof OWED.
- OWED: end manual/device batch (incl. low-end encode + camera proof),
  provider proof. No PR/CI/merge/wiki per ADR-018. iOS archived. G09
  UNCERTIFIED.
- Next: P37 phase exit (LOCAL_DONE checkpoint + evidence), then P38
  (`codex/phase-38-app-acceptance` via `--from-checkpoint`).
