# SEC01-B — Bounded mailbox and config-gated delivery

Date: 2026-10-11. Status: LOCAL_DONE. Scope: memory-outbox
ceiling, delivery gate, `ANPFUEL_MAIL_MODE`, production wiring
— no SMTP, no aggregate admission (SEC01-C). Head: maintained
`dev` + this task.
Plan: [remediation plan](../../../planning/SECURITY_REMEDIATION_PLAN_2026-10-11.md)
(SEC01, AUD-01). Constraint: unavailable delivery leaves no
retained issuances; no oracle leaks status.

Validation: PASS

## Changes

- `account/adapters/mail`: `NewOutbox(clock, maxRetained)`
  with ring cap (`DefaultMaxRetained` = 100, non-positive
  clamps); oldest drops past the ceiling. The adapter stays a
  bounded deterministic test/local tool — "not safe for
  production delivery" still holds for real sending.
- `account/application`: `Service.MailEnabled` (default
  false). Off answers nil before any store write — identical
  to quota/cooldown/unknown, so unavailable delivery leaves
  zero rows and zero mail with no oracle. Enabled path is
  byte-identical to before. Test fixtures opt in explicitly.
- `platform/config`: `ANPFUEL_MAIL_MODE` off (default) or
  memory; anything else (including `smtp`) refuses naming the
  variable. Production stays off until delivery is deployed
  and configured.
- `cmd/api`: outbox with the frozen cap; `mailEnabled` from
  config. The old "explicit preview" comment (codes issuing
  nowhere) is replaced: off issues nothing at all.

## Validation (actual)

- New: outbox cap/ring/clamp tests; disabled-retains-nothing
  (zero code rows via counting decorator, zero mail) and
  enabled-unchanged tests; config default/memory/reject
  tests. RED before GREEN (new API/field/config).
- Existing fixtures updated to opt in (application service +
  links, http ×3, pgstore/pglinks integration) — no
  expectation changed, only the explicit delivery flag.
- `go test ./internal/modules/account/...` unit green;
  `-race -tags=integration` green. `go build ./...`,
  `go vet`, `gofmt -l`, `git diff --check` clean;
  `scan-secrets.sh` passed (synthetic addresses only).

## Limits / owed

- Aggregate admission, per-account budgets, invalid-first
  validation, steady real-SQL flood/rollback proofs:
  SEC01-C/D (SEC01-A skips still pending).
- SMTP: needs explicit need/license/security record — not
  added. Local delivery stays memory-only and explicit.

Next microtask: SEC01-C atomic admission + budgets.
