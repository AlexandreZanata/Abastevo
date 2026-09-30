# Backend privacy notice (DRAFT — P07-T05)

Status: engineering draft, not a declaration of legal compliance.
Review lawful bases, contact details, processor contracts and
international transfers with the responsible operator before any
pilot. The [LGPD text](https://www.planalto.gov.br/ccivil_03/_ato2015-2018/2018/lei/l13709.htm)
is the legal reference. Contact process and regulatory response
deadlines are set by legal review before pilot, not by engineering.

Anonymous installation keys and photographs can still constitute
personal data. This notice covers the backend MVP inventory only;
account, email, billing, sync and analytics are not collected in MVP
(P11 needs a separate inventory/basis/retention review).

## Your rights

- Access: request an export of your ownership history; you receive a
  bounded archive within a 24 h download window.
- Deletion: request erasure; writes revoke immediately, links and
  payload purge scope by scope, and you keep an auditable receipt.
  Price facts stay for history with anonymized references (never as
  your data). Backed-up copies age out with the encrypted backup
  horizon (35 days max); notices explain this delay accurately.
- Losing your device key loses the anonymous identity: data cannot be
  disclosed from unverifiable claims. A support procedure may help
  with alternate evidence, without disclosing unrelated data.

## Inventory: purpose, retention, access, deletion

| Data | Purpose | Retention | Access | Deletion |
|---|---|---|---|---|
| Contributor ID + public keys | Authentication, abuse control | Active lifetime, then rights-request processing | Restricted API/trust operators | Revoke keys, mark deleted, clear attribution token (erasure); no public browsing |
| Attribution token | Link owner rows without exposing identity | Same as contributor | Owning modules only | Cleared on erasure; facts unlink to opaque refs |
| Exact GPS/fix, OCR claims | Short-lived proximity derivation | Deleted after derivation, hard cap 24 h | Worker only | Immediate post-derivation delete (no long-lived store in v1) |
| Quarantine photo | Decoding/verification only | Deleted after sanitization, hard cap 24 h | Worker only | Retention sweeper (`evidence-sweep-hourly`) |
| Sanitized evidence | Private review | 14 days; substantiated open case extends to 30 days max | Restricted moderation via 60 s audited download | Sweeper; erasure purges immediately |
| Verified/perceptual hashes, fraud signals | Duplicate detection | 90 days | Restricted trust role | Sweeper; erasure purges immediately |
| Observation facts (product/price/condition/provenance) | Product history | 24 months initially, then anonymization review | Public (source-separated, no identities) | Unlinked on erasure; timed purge needs FK-consistent cascade design (metrics visible, enforcement follows) |
| Confirmations/disputes | Independent support/reporting | Same as observations | Owner history private; aggregates public | Reporter refs unlinked per row on erasure |
| Trust verdicts | Reliability from reviewed outcomes | Current view rebuildable; decisions under review | Restricted operators | Current dropped, history unlinked on erasure |
| Moderation cases/actions | Accountability | 12 months initially | Restricted operators | Closed cases purge after 12 months with audit rows first |
| Upload sessions/intents | Quota, idempotency, verification | 24 h idle/stuck expiry | Owner status only | Sweeper; erasure expires immediately |
| Challenges/nonces | Proof of possession, replay defense | Until expiry (5 min) | Server only | Retention sweeper (`privacy-retention-daily`) |
| Idempotency windows | Safe mobile retries | 7 days | Server only | Retention sweeper |
| IP digests/quota windows | Abuse/cost bounding | ≤24 h rotating digest | Server only | Self-clean on check; provider edge retention configured consistently |
| Export archives | Rights access | 24 h download window | Owner only | Bytes purged on expiry/erasure; receipts stay |
| Deletion ledger | Restore-safe replay | Backup horizon (35 days) | Restricted operators | Aged out by retention; never payload |
| Operational logs | Diagnosis | 14 days trace data; 12-month aggregates without individual labels | Operators | Rotation (no sensitive bodies/tokens/URLs ever logged) |
| Encrypted backups | Disaster recovery | 7 daily + 4 weekly (35 days max) | Recovery credentials only | Expiry; transient tables excluded so short retention survives backup |

## Enforcement

- `evidence-sweep-hourly`: quarantine/final/orphan/hash lifecycle.
- `privacy-retention-daily`: challenges, idempotency, closed
  moderation cases, expired export bytes, aged-out ledger rows, plus
  per-category overdue metrics in the structured log.
- Every sweep purges bounded oldest-first and reports purged counts
  with the oldest overdue instant; a stuck category fails its job
  loudly for retry instead of hiding backlog.

## Target revision pending P13/P14/P15

This remains an unapproved draft and the table above reflects v1 implementation. The new target requires free email-code/Google/Apple account data, authenticated social revisions/votes and **all photo copies expiring within 24 hours, with no 14/30-day exception**. [Account](../security/FREE_ACCOUNT_ACCESS.md) and [media/location](../security/LOCAL_MEDIA_LOCATION_POLICY.md) contracts own the changes. Revise the full inventory/table and verify enforcement before public G09/P10-T09 launch; this note is not a claim that current jobs meet the target.
