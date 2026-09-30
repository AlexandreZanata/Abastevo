# P13-T01 — Account rules and additive contracts

Status: LOCAL_DONE on `codex/phase-13-free-accounts` (phase PR pending).
Issue: #22. Entry: G12 PARTIAL per owner decision 2026-09-30 — backend
account work starts while the P12-T04 macOS run pends; the Mac run still
gates P12 merge, G12/G13 and any native login. No endpoints, no migrations,
no mail delivery in this task.

## B-BR / BUC

B-BR-A01…A05 / BUC-A01…A04 owned by `docs/security/FREE_ACCOUNT_ACCESS.md`;
this task freezes the numbers A02 deferred (OTP length/TTL/attempts/resend),
session lifetimes, OIDC bounds, the subject/session/device proof boundary,
the data inventory/retention and additive versioning. Handlers (P13-T02),
provider verification (P13-T03), recovery/privacy (P13-T04) and native login
(P13-T05) consume this record.

## What changed

- `docs/security/FREE_ACCOUNT_ACCESS.md` — frozen-parameters section
  (threat rationale per number; executable mirror in Go).
- `backend/internal/modules/account/domain/` (new, stdlib-only,
  guard-tested): `policy.go` (OTP/session/OIDC constants + pure
  expiry/attempt/resend predicates), `ports.go` (`Clock`, `CodeHasher`,
  `MailSender`, `ProviderVerifier`, `ProviderSubject`, stable
  `VerdictCode`), `doc.go`.
- `contracts/testdata/account/*.json` (12 fixtures, own directory: the
  identity profile suite globs `testdata/identity/*.json`, so account
  vectors live separately): OTP
  valid/replay/expired/attempts-exhausted/enumeration/resend-cooldown,
  OIDC wrong-issuer/wrong-audience/expired/nonce-reused, cross-account and
  email-only link takeover refusals. Synthetic data only.
- `contracts/openapi/v1.yaml` — six additive preview schemas (`Account`,
  `ProviderLink`, `EmailCodeRequest`, `EmailCodeConsume`,
  `OidcLinkRequest`, `Session`); no paths, implemented v1 untouched.
- Tests: `policy_test.go` (boundary TTL/attempt/resend/session/skew),
  `stdlib_test.go` (stdlib-only + verdict-code stability),
  `fixtures_test.go` (shape, id==filename, verdict/code enums, no secrets).
- Delivery: `scripts/issues.sh` gh-2.45 create fix (same as P12 branch;
  merges cleanly), P13 milestone/issues/ledger.

## Validation (exact commands, this host)

```sh
cd backend && gofmt -l ./... && go vet ./internal/modules/account/... \
  && go test -count=1 ./internal/modules/account/... ./internal/modules/kernel/... ./internal/platform/apicontract/...
vacuum lint -r contracts/openapi/vacuum-rules.yaml contracts/openapi/v1.yaml --no-update-check
make quick-verify
git diff --check
```

Outcome:

- New package: all suites pass (policy boundaries, stdlib guard,
  verdict stability, 12/12 fixture vectors load with stable codes).
- `kernel` + `apicontract` (spec self-validation incl. new schemas): PASS.
- `vacuum`: baseline restored — 0 errors, 0 warnings, 9 informs (two
  intermediate warnings, unquoted numeric example and object-level
  date-time examples, found and fixed in-task).
- `gofmt`/`go vet` clean; `make quick-verify` ok; `git diff --check`
  clean; secrets scan clean.

## Limits (not claimed)

- No signup/login/logout endpoints, no sessions issued, no mail sent, no
  rows stored — acceptance of the FREE flow belongs to P13-T02…T05 with
  real-DB concurrency/race suites.
- No Google/Apple sandbox contact; provider outage/sandbox evidence stays
  in P13-T03. No native Keystore/Keychain work (P13-T05).
- G12 stays PARTIAL until the P12-T04 macOS run; G13 needs the full P13.

## Rollback

Docs/fixtures/schemas/ports only: deleting the new files and the YAML
block restores the tree bit-identically. No migration, no flag, no data.

## Next

P13-T02 email access-code backend (#23): hashed codes, bounded attempts,
revocable session families, real-DB replay/concurrency/enumeration suites.
Issue #22 stays open until the P13 phase PR merges.
