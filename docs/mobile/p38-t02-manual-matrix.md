# P38-T02 — Consolidated end Android manual matrix (PLAN ONLY)

Status: LOCAL_DONE on `codex/phase-38-app-acceptance`. Task: P38-T02.
Per the 2026-10-02 directive, no emulator/device runs happen until all
selected construction phases complete; this task consolidates the
union of P24/P29/P33 device/manual/accessibility/performance/provider
rows plus the new P35–P37 journeys into one end batch. Nothing here is
executed or passed — execution stays OWED. Test proof and user
acceptance decisions remain separate fields; no blanket waiver.

## Execution rule

Run once at project closure on the frozen candidate (P38 code + all
selected catalog/profile phases), reusing unchanged evidence only for
identical artifacts/inputs. Missing required rows block acceptance;
old passes never certify changed flows. iOS remains archived
throughout; G09 still needs its own real-production matrix afterward.

## Matrix

### A. Functional journeys (real devices, staging origin)

- A1 Explore list-first: manual city + fuel chips + name search + price
  sort; failed-refresh cache recovery; empty-city and no-price honesty.
- A2 Nearby bounded lookup: explicit permission grant/deny, transient
  GPS (no persistence), no-fix honesty, stale-position never shown.
- A3 Station detail (UUID): community-primary slot, dated ANP
  reference, stale/unknown/disputed states, route + update-price
  actions, screen-reader source/condition labels.
- A4 Free accounts: email-code + Google/Apple login completion,
  session/key binding, refresh/revocation/deletion, provider denial
  presentation, no paid gate.
- A5 Discussion: 1–5 stars, 280-scalar comments/one-level replies,
  valid/invalid votes with agreement denominator, reports, author
  ownership, revoked-account and hidden-content behavior.
- A6 Capture/review/submit: station/fuel context, legibility feedback,
  minimal permissions, one explicit submit, queued/accepted status,
  cancel, idempotent retry, process-death recovery.
- A7 Privacy: self-deletion (session revocation + local wipe),
  export where implemented per frozen contract, erasure tombstones.

### B. Novice usability + accessibility (P24-T02 carryover)

- B1 First successful price lookup unaided; source/recency/condition
  comprehension (ANP reference vs community state never confused).
- B2 Photo contribution without assistance; readable large fonts;
  TalkBack traversal of Explore/detail/discussion/capture.
- Sample, task protocol, pass thresholds and remediation follow the
  P19 baseline before running; no unnecessary PII collected.

### C. Performance (P24-T03 carryover, low-end/support devices)

- C1 Genuine cold-process start (the earlier cached-home 382 ms is
  regression evidence only, not a cold-start result).
- C2 List scrolling, peak heap, photo encode/OCR time, background/
  offline replay, battery — frozen budgets before optimization,
  P15 limits preserved.

### D. Security/compatibility/privacy failure matrix (P24-T04 carryover)

- D1 Auth/ownership/negative inputs, exact money/units, media privacy,
  fail-partial cases, signature/location denial/unknown/mock-GPS,
  offline replay, revoked identity, media expiry, migration/search
  recovery, server outage.
- D2 Signed release behavior on the frozen candidate (keystore
  signing config under owner control; no debug keys).

### E. Provider/storage/device prerequisites (explicitly OWED)

- E1 TLS trust resolution for `https://teste.abastevo.com.br` from the
  test runner/devices (today: Fortinet middlebox CA, `curl` 60).
- E2 Email-code delivery, Google/Apple client IDs/redirects, cert/
  signature configuration under local secret ownership.
- E3 VPS private media provisioning (or recorded equivalent scope)
  for the live photo journey; all-copy 24h verified end to end.
- E4 Catalog/profile phases (P25–P33) rerun affected rows on the new
  candidate (P29/P33 acceptances); unchanged proof carries forward
  only for identical artifacts/inputs.

## Limits and next

- All rows OWED; G24 evidence stays valid only for its tested inputs.
  No row is marked passed by this plan.
- Next: P38-T03 candidate, final integration and G09 handoff.
