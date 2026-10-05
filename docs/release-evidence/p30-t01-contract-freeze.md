# P30-T01 — Entity, proof, permission and privacy contract freeze

Status: LOCAL_DONE on `codex/phase-30-station-profiles`. Task: P30-T01.
Docs-only freeze (no schema/handler/gate disguised as docs — zero
runtime files touched); implementation starts in P30-T02.

## Frozen

- `docs/product/STATION_PROFILE_CONTRACT_FREEZE.md`: ceremony bounds
  (256-bit challenge, 30-min TTL, 5 attempts, 3 open claims, 5 MB
  PDF-only proof, exact 11-field declaration template), roles/scopes
  (administrator/manager subsets, no OWNER/moderator/community power),
  fresh-session + key-proof sensitive writes (attestation deferred
  explicitly), closed business-field enum, retention/backup/privacy
  rules (`[CALIBRATE: legal review]` on granted-case years),
  frozen claim/grant state machines, and 4 explicitly `[OPEN]` items
  with owning tasks (verifier selection, Receita/QSA access,
  staffing, moderation enum extension).

## Validation

- Consistency: all 3 doc links resolve; B-BR-P01–P16/BUC-P01–P10
  anchors verified present in the representation target; no OWNER
  role string in the freeze (`grep OWNER` clean by construction —
  the doc states its absence once in prose).
- `git diff --check` PASS; new-file whitespace PASS; secret-surface
  review PASS (`scan-secrets.sh` PASS; no keys/documents/PII).
- No backend/Android suites apply (docs-only planning batch).

## Limits and next

- `[OPEN]` items block P31-T01/P31-T02 and the moderation extension —
  recorded, not defaulted.
- Next: P30-T02 canonical operator link and public unclaimed profile.
