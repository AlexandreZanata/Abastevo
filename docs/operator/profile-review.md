# Profile claim review — operator runbook

Scope: human review of station representation claims (`profile-v1`).
Server decisions are final; reviewers never grant outside the
`DecideAtomically` path. Staffing levels stay P23-T03-owned; this
runbook defines the procedure, not headcount.

## 1. Review queue

- Source: pending claims with active declarations (`ClaimStore` open
  states). Order: oldest declaration first; impersonation flags first.
- Each review records: decision (approved/denied), reason (required for
  denial), evidence outcomes (signature + authority), reviewer id.
  Denial without reason is rejected by the service (`ErrVerifyReason`).

## 2. Approval checklist

1. Signature outcome valid (independent verifier, not the uploader).
2. Authority sufficient for the exact branch/CNPJ (no joint-authority
   assumption; mismatch or stale operator evidence → deny/stale).
3. Account live; no self-review (requester ≠ reviewer, enforced).
4. Scopes within the claim's role; grants stay narrow to claim scopes.

## 3. Urgent impersonation review

- Rival or fraudulent claim flags pause auto-processing for that
  station (disable intake flag for the station, keep reads).
- Both competing cases stay private; never disclose one claimant to
  the other. Escalate to the station operator channel when available.

## 4. Suspension and revocation

- Grant revocation/suspension takes effect on next capability check;
  in-flight edits fail closed (`DeniedRevoked`/`ServerRefused`, no
  replay). Contested access follows the same path via
  `ContestAccess` + fresh auth.
- Revoked PII/proof follows the 24 h all-copy purge
  (`docs/operator/retention.md`); deletion-ledger replay restores the
  revocation on recovery.

## 5. Disable and rollback

- Feature disable: turn off the claim/management capability flags;
  public unclaimed reads keep working (no profile deletion).
- Binary rollback: restore the previous verified backend build; grants
  and revocations persist in the database (never rolled back with the
  binary). Re-run the P33-T01/T02 suites on the restored build.

## 6. Contact and rights

- Source/document rights: registry/DOU snapshots and reviewer notes
  stay within their licensed scope; private proof bytes are never
  published or attached to public responses (all error paths are
  `no-store`).
