# SEC00-C — Versioned budget freeze (planning only)

Date: 2026-10-11. Status: PLANNING — frozen numbers, not a fix.
Unpublished by policy (planning-only work does not publish).
Every ceiling below was read from the cited constant on
maintained `dev`; anything without a code anchor is marked
PROPOSED, never advertised capacity.

## Item budgets (counts)

| Intake / queue | Ceiling | Source |
|----------------|---------|--------|
| Account register | 10/hour (quota v1) | `identity/domain/quota.go` |
| Signed writes | 200/hour (quota v1) | `identity/domain/quota.go` |
| Challenges | 60/hour (quota v1) | `identity/domain/quota.go` |
| Photo reservations | 10/contributor/UTC-day | `evidence/application/reserve.go:35` |
| Queue attempts | 5 max per job, then park | `Enqueue` call sites; `jobs/queue.go` |
| Observation history page | 1–100 rows | `community/application/submit.go:252` |
| Feedback page | ≤100 rows | `feedback/application/comments.go:219` |

## Byte budgets

| Intake / transfer | Ceiling | Source |
|-------------------|---------|--------|
| Upload sessions | 3 MiB declared max | `evidence/domain/session.go:20` |
| Consent HTTP bodies | 64 KiB, strict decode | CS09C `privacy/adapters/http` |
| Other HTTP bodies | 8/16/64 KiB per route | `ReadBody` call sites (5×64, 2×8, 1×16) |
| Minimized derivatives | ≤3 MiB presign cap | CS10C `promotion_objects.go` |
| Existence reads | 1 content byte (overflow proves presence) | CS10C `Exists` |

## Time budgets

| Token / grant / lease | Lifetime | Source |
|-----------------------|----------|--------|
| OTP email codes | 600 s | `account/domain/policy.go:10` |
| Session access | 900 s | `account/domain/policy.go:21` |
| Identity challenges | 5 min | `identity/domain/identity.go:20` |
| Idempotency replay | 7 days | `identity/domain/identity.go:126` |
| Evidence sessions | 24 h idle expiry | `evidence/domain/session.go:21` |
| Photo captures | 2-min camera / 24-h use windows | `community/application/photo_capture.go` |
| Presigned URLs | 5-min TTL | `cmd/api:835`, `cmd/worker:163,180,320` |
| JWKS cache | 3600 s per issuer set | `account/domain/policy.go:31` |
| Job lease / retry delay | 5 min / 1 min defaults | `platform/jobs/dispatch.go:33,42` |
| Revoked training copies | 24 h purge TTL, daily sweep | CS10C `RevokedCopyTTL` |

## Concurrency posture (verified, not ceilings)

- Reservation admission is non-atomic under parallel load
  (audit probe still reproduces; SEC03-A owns the fix).
- Forwarded clients behind one proxy share one quota bucket
  (probe reproduces; SEC02 owns the resolver).
- No global item/byte/concurrency admission exists for
  accounts/sessions/mail (probe reproduces; SEC01-C owns it).

## PROPOSED (no code anchor — do not advertise)

- Aggregate cross-subject ceilings for accounts/sessions/mail.
- Per-route byte caps beyond the listed `ReadBody` sites.
- 429/503 emitted semantics under saturation (Retry-After
  exists on quota denial; global-overload shape unfrozen).
- NAT fairness weights; expiry of parked jobs.

## 429/503 semantics (current, verified)

- Quota denial returns stable retry metadata
  (`RetryAfter`; community maps to 429; consent has no quota —
  self-limiting singleton, CS12A record).
- Parked jobs (post-cap) await audited replay; no automatic
  drop claimed.

No live readiness inference; no VPS/phone mutation.
