# SEC04-A — Erasure inventory freeze (planning only)

Date: 2026-10-11. Status: PLANNING — frozen inventory, not a fix.
Head: maintained `dev` + this task.
Plan: [remediation plan](../../../planning/SECURITY_REMEDIATION_PLAN_2026-10-11.md)
(SEC04, AUD-05). Contract: B-BR-A05 (export/erasure), B-BR-016
(ledger/replay), A05/FREE_ACCOUNT_ACCESS. Constraint: a receipt
names completed scopes; acknowledgement is never completion.

## Linked-owner inventory (captured durably BEFORE unlinking)

Account (`accounts`, `account_addresses`, `account_provider_links`,
`account_key_credentials`, `account_contributor_bindings`,
`account_binding_audit`):

- Account row (id/alias/status) stays as the audit record; later
  signups mint new accounts without carryover.
- Address + provider bindings drop at deletion; contributor
  bindings MUST be captured first — dropping them without capture
  orphans the contributor scopes below (the AUD-05 defect).
- Key credentials: unsalted lookup + salted verifier rows minimize
  on erasure (no secret survives the owner).

Contributor-owned scopes (keyed by contributor ID / token):

- Identity keys (contributor device keys, challenges/nonces —
  short-lived by construction).
- Social: feedback comments/replies (tombstone, alias anonymized),
  ratings (tombstone + stats rebuild), votes (author votes vanish
  from tallies; others' votes/replies on erased comments stay —
  denominators survive the author).
- Community observations/confirmations/disputes (unlink +
  price-key recompute; facts stay with anonymized refs),
  helpfulness votes, moderation cases (actor/action audit stays).
- Media: evidence sessions/objects (quarantine + finals purged),
  training derivatives + bytes (consent-withdrawal path, CS10).
- Queue: jobs payloads/dedupes referencing the owner; Android
  outbox rows under the erased owner (refuse/send nothing — D).
- Export archives (owner bytes purged; ledger rows stay).

## Retention exceptions (lawful/contractual)

- Code verifiers purged ≤24 h after expiry; sessions ≤7 d after
  absolute expiry; link/recovery audit 180 d (FREE_ACCOUNT_ACCESS).
- Evidence copies 24 h after first receipt; duplicate-signal
  hashes 90 d (restricted trust role, never public).
- Deletion ledger rows persist as the replay/audit trail
  (B-BR-016); moderation actor/action audit persists.
- OS backup/device transfer of private metadata rides SEC05-D
  assessment; cacheDir photos and excluded auth prefs are already
  separate categories.

## Receipt (not acknowledgement)

`privacy ErasureReport` + per-scope ledger rows + HTTP deletion
receipt `{status: deleted, erasure: {request, scopes}}`: the HTTP
200 means intent recorded + authorities revoked + scopes
enqueued/executed per the wired path — full physical deletion
follows worker execution and is reported by ledger replay, never
promised at acknowledge time. Terms/settings must describe this
demonstrated state (D).

No live readiness inference; no VPS/phone mutation.
