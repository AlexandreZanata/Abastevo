# Operator access runbook (P07-T02)

Restricted operator commands for moderation and privacy (BUC-006/BUC-007).
There is no public admin HTTP path: privileged work runs through the
`ops` CLI over controlled operator access only.

## Access model

- Identity: `--operator <id>` flag or `ANPFUEL_OPERATOR_ID` environment.
  Anonymous invocations are denied before any work (`ErrUnauthorized`).
- Network: CLI runs on the operator machine with direct database access;
  the public API exposes no admin routes (`/health/*` and `/metrics`
  carry no admin capability; metrics stay on the private network).
- Credentials: database/R2 credentials follow the standard startup
  config (never printed, never committed). Rotate per incident procedure;
  revoke operator access by removing the identity at the access layer.
- MFA and allowlists live at the access layer per SECURITY_PRIVACY
  (admin CLI over restricted operator access with MFA); this runbook
  does not invent their configuration.

## Commands

All commands append actor/reason/policy audit rows and move the case
exactly once per action. Raw facts are never edited; appeals create new
actions, cases or reviewed facts.

```sh
# Triage / resolve / dismiss any case (audit + status + recompute job, atomically)
go run ./cmd/ops moderation act --case <case-id> --action REVIEW --reason "triage note" --operator op-7

# Reviewed observation invalidation: audit INVALIDATE, then the community
# VALIDATED→REJECTED decision bound to the same case ID plus its
# consensus recompute job (observation must be VALIDATED)
go run ./cmd/ops moderation invalidate --case <case-id> --reason "confirmed fake price" --operator op-7

# Reviewed contributor block: audit BLOCK, then the trust BLOCKED verdict
# bound to the same case ID (contributor cases only)
go run ./cmd/ops moderation block --case <case-id> --reason "reviewed Sybil ring" --operator op-7

# Privileged evidence review: 60 s owner-independent download bound to an
# actionable case referencing exactly that evidence. The signed URL goes
# to stdout for the operator terminal only; the audit line on stderr
# carries actor/case/evidence/timestamp, never the URL.
go run ./cmd/ops evidence-url --case <case-id> --evidence-id <id> --operator op-7
```

Action/target rules: `INVALIDATE` on OBSERVATION/DISPUTE/EVIDENCE,
`BLOCK` on CONTRIBUTOR, `REVIEW`/`RESOLVE`/`DISMISS` on any target.
Closed cases never reopen; a two-step command that fails downstream
reports `audit <id> recorded` so the follow-up is an audited retry,
never a silent drop.

## Appeal and recovery

- A disputed decision is appealed by recording a new action (e.g.
  `RESOLVE` with the review rationale) or by submitting a new reviewed
  observation referencing the old one; the original rows stay intact.
- Revoke a compromised operator identity at the access layer, then
  record `DISMISS`/`RESOLVE` corrections with the replacement identity.
- Projection rebuilds replay from the retained audit ledger; no manual
  SQL edits (unaudited manual SQL is a finding, not a procedure).

## Retention and privacy notes

- Audit rows (`moderation_actions`) carry actor/action/reason only: no
  media, GPS, IPs or URLs (B-BR-011). Accountability retention per
  SECURITY_PRIVACY (12 months initially).
- Evidence downloads expire in 60 s; expired URLs are not retried, a new
  audited command mints a fresh one while the case stays actionable.
- Export/erasure workflows (BUC-007) land in P07-T03…T05 and reuse this
  access model; losing the contributor key never discloses data.
