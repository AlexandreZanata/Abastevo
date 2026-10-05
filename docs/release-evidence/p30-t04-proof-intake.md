# P30-T04 — Private immutable signed-document intake and expiry

Status: LOCAL_DONE on `codex/phase-30-station-profiles`. Task: P30-T04.
Binds B-BR-P05/P08/P15/P16 and BUC-P03/P09. Validated intake with
hash-bound immutable metadata and lifecycle expiry; object storage
explicitly unprovisioned so intake refuses (503) instead of any
approval fallback.

## Behavior (TDD, critical-first)

- Migration `000038_claim_proofs.sql` (append-only): hash-bound
  metadata (claim/declaration FKs, sha256, size, format, kind,
  server-generated object key, status, expiry); one proof per
  declaration (partial unique index); expiry index. Bytes never touch
  PostgreSQL/Git/logs.
- Domain: PDF-by-magic, 5 MB cap, encrypted/active-content and
  key-bundle refusal; kind retention (scans 24 h all-copy cap,
  authorizations multi-year audit `[CALIBRATE: legal review]`,
  unknown kinds fail closed).
- Application: `SubmitProof` (owner + open claim + single active
  declaration + unexpired checks, validation, dedup-by-hash with
  orphan-byte cleanup, immutable server keys); `PurgeExpired`
  (mark-expired + physical byte delete, rows stay as audit for
  restore replay).
- Adapters: PG stores, proof HTTP route (base64 JSON, bounded,
  `no-store`, 404/409/503 mapping), `cmd/api` wiring with `Bytes:
  nil` (documented 503, storage scope owns the wiring).
- OpenAPI proof path + `ClaimProof` schema + `Unavailable` response
  (`vacuum` PASS after fixing two merged lines, `apicontract` +
  `check-compat.sh` PASS).

## Validation

- Unit (`-race`): format matrix (9 refusals + valid), kind expiry,
  submit/dedup/foreign/kind/stale/no-storage/purge — PASS.
- Integration (real PostGIS, `-race`): submit→parallel redelivery
  binds once, expiry/purge semantics, full claim lifecycle with
  privacy. Fresh disposable DBs (migrations incl. 000037/000038).
- Fixed from real failures (not weakened): typed-nil interface
  panic, test JSON literal interpolation, non-UUID test ids,
  duplicate helper, YAML merges.
- Regression: full `stationprofile` + `directory` unit + integration
  PASS (zero failures); `go vet` clean; `go build ./...` PASS.
- `git diff --check` PASS; `scan-secrets.sh` PASS (digests only).

## Limits and next

- Object-storage provisioning + verifier review tooling are explicit
  follow-ups (P31 + storage scope); restore-purge replay is
  privacy-owned.
- Next: P30 phase exit (LOCAL_DONE checkpoint + evidence), then P31
  (`codex/phase-31-verified-representation` via `--from-checkpoint`).
