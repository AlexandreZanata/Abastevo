# Stage B — Community reliability without mandatory GPS

State: PLANNED / NOT_IMPLEMENTED_BY_THIS_PLAN, 2026-10-14.
Owner: [master](APP_CONSOLIDATION_PLAN.md). Tasks OPT-B00–B08.
Rules: B-BR-OPT-06–12/18. Use cases: BUC-OPT-03/04/10.

## Outcome, reuse and risk contract

Entry: A source checkpoint and current Community/Evidence/Trust/Moderation/Jobs
safety fixes reconciled. Improve verifiable evidence and independent support
under ADR-021: no GPS/150m admission, missing-location fraud, proximity advantage
or new attestation SDK.

Reuse SHA-256/dHash, owner/key/station binding, current short capture authorization,
24-hour operational deadline, signatures/nonces, quotas/idempotency, signals/
consensus and NEW/ESTABLISHED/BLOCKED trust. No replacement upload system, generic
risk service, forced numeric score, graph database, ML or D receipt/portal work.

Hard admissibility failures retain refusal: invalid proof, foreign owner,
invalid target/product/unit, quota denial, altered replay and invalid media.
Soft uncertainty is separate and can require support/review. No uncalibrated
sum or single similarity signal automatically bans a person.

- Distinguish exact retry, multi-fuel shared capture, recompression, honest new
  similar photos and cross-station/time reuse. Only independent evidence adds
  support. Benchmark dHash against pHash/PDQ on labelled cases before dependency
  changes; crop detection is not guaranteed and thresholds do not transfer.
- Similarity/signature/receipt does not prove malicious intent, fresh pixels or
  physical presence. Preserve capture/retry deadlines and operational photo TTL.
  Use bounded indexed candidates, not all-image quadratic comparisons.
- Behavior uses verified attribution and bounded event windows. Delayed offline
  uploads are not impossible travel; client timestamps are untrusted. Municipal
  centroids cannot certify movement. IP supports abuse/fairness, not unique people.
- Plausibility compares compatible product/unit/full condition with dated
  station/regional evidence; candidate is excluded from its own reference.
  Median/MAD needs sparse/zero-dispersion/promotion policy and calibrated limits.
  Dated ANP context cannot alone overwrite/veto community price.
- Likes/helpfulness/payment/profile status, key rotation and raw volume do not
  create truth/independent people. Ordinary mutual consensus cannot promote
  the same group's trust; existing promotion requires independent reviewed outcomes.
  Age/new account/novel photo alone proves neither truth nor fraud.
- Public states describe support/uncertainty/review; intake accepted is not price
  validated. BASE-50 weights and "two accounts = confirmed" are not adopted.
- Observe new soft signals in shadow first. Calibrate labelled honest/attack
  cases, false positives/negatives, appeals and cost before changing publication.
  Missing evidence keeps a proposed classifier in shadow; existing hard failures
  remain refused. Freeze conservative fallbacks and reviewed outcome thresholds.
- Use a new unoccupied policy version only for real semantic change. Preserve
  consensus-community-v2/signals-community-v2 replay, input provenance/cutoff,
  correction and fenced recomputation.
- Private derived signals obey minimized retention/erasure/restore; hashes are
  not automatically anonymous. Photos still expire within 24h, with no training
  consent exception for operational proof.

## Sequential tasks

### OPT-B00 — Map current safety and signal behavior

- **Depends:** OPT-A07 and current critical-fix acceptance.
- **Inputs/areas:** hash/media, derive/recompute, Trust worker, moderation/jobs.
- **Deliver:** reuse/gap, policy/authority/retention matrix and independent labels.
- **Tests/acceptance:** trace actual capture→media→signals→consensus/trust/review,
  including duplicate/failure paths; no source audit proves solved Sybil or runtime.
- **Recovery:** preserve existing fixes; known failures block dependent changes.

### OPT-B01 — Freeze reasons, outcomes and calibration

- **Depends:** OPT-B00.
- **Inputs/areas:** domain target/shared vectors/moderation/read contracts.
- **Deliver:** typed reasons, independence groups, review/appeal/fallback,
  threshold evaluation and versioning protocol.
- **Tests/acceptance:** hard versus soft, missing signal/GPS, linked keys,
  sparse context and shared capture; no arbitrary score/hash threshold.
- **Recovery:** keep current accepted policy while additions are shadow-only.

### OPT-B02 — Bounded media independence

- **Depends:** OPT-B01. Critical media/privacy/SQL.
- **Inputs/areas:** sanitizer/hash/store and existing signal ports.
- **Deliver:** explicit reuse classes and indexed bounded candidates; different
  library only after need/license/security/performance review.
- **Tests/acceptance:** replay, crop/recompression/rotation limits, honest similar
  signage, multi-fuel/cross-station reuse, collisions, concurrent uploads,
  expiry/erasure and real SQL/S3 faults.
- **Recovery:** disable new similarity rule, retain integrity/dedupe/cleanup and
  conservative independent-support counts.

### OPT-B03 — Offline-safe behavioral abuse signals

- **Depends:** OPT-B02.
- **Inputs/areas:** quotas/attribution, bounded event queries and jobs.
- **Deliver:** burst/multi-target reasons and review/resource ceilings, no claim
  of physical travel from arrival order.
- **Tests/acceptance:** queued offline batch, linked keys, shared IP/device,
  replay/erasure, large history, concurrent admission and real SQL query plans.
- **Recovery:** disable soft behavior signal; retain admission quotas.

### OPT-B04 — Robust price-context signals

- **Depends:** OPT-B03.
- **Inputs/areas:** community/official read ports and pure context rules.
- **Deliver:** comparable population, conservative categories, insufficient
  context and shadow explanations.
- **Tests/acceptance:** median/MAD oracle, sparse/MAD=0, stale reference, wrong
  condition/unit, promotion/market shift, candidate exclusion and malicious
  extremes; real SQL where population selection changes.
- **Recovery:** unavailable context stays unknown, no fabricated regional value.

### OPT-B05 — Audited contributor outcomes

- **Depends:** OPT-B04.
- **Inputs/areas:** current Trust decisions, reviewer authority and worker.
- **Deliver:** reviewed successes/abuse/corrections with bounded provenance and
  tier replay; no second Trust aggregate or consensus-driven promotion.
- **Tests/acceptance:** agreeing farm cannot self-promote, key rotation adds no
  people, erased/revoked owners gain no authority, concurrent review/trust emits
  one durable affected recomputation.
- **Recovery:** suspend new tier rule; preserve audited decisions and rehabilitation.

### OPT-B06 — Calibrate and publish selected policy

- **Depends:** accepted OPT-B02–B05 evidence.
- **Inputs/areas:** labelled shadow results, pure rules, projection inputs/SQL.
- **Deliver:** reviewed thresholds/outcomes, review/resource budget and supported
  deterministic policy change with rebuild/recovery.
- **Tests/acceptance:** independent golden oracle, order invariance, minority
  real price, insufficient support, moderation/erasure races, lease fencing,
  concurrent publication and legacy replay on real PostGIS.
- **Recovery:** select earlier accepted projection, preserve new audit/facts;
  never silently overwrite old version semantics.

### OPT-B07 — Explain actual states in Android

- **Depends:** OPT-B06 backend/read contracts.
- **Inputs/areas:** station/community/status panels and transport.
- **Deliver:** support/review/uncertainty reasons and next action, protected
  owner status and bounded public explanation.
- **Tests/acceptance:** guest/new user, reported/confirmed/disputed, hidden/
  revoked/expired/offline, unknown enum and late response; JVM/lint/APK and no
  private voter/media leak.
- **Recovery:** hide added explanation while retaining truthful existing status.

### OPT-B08 — Abuse and regression acceptance

- **Depends:** OPT-B07 and preceding critical checks.
- **Inputs/areas:** pinned policy/app/backend/dataset/retention manifest.
- **Deliver:** honest/attack results, appeal/recovery, bounded cost and checkpoint.
- **Tests/acceptance:** signed no-GPS contribution through private storage/worker
  to Home; replay/farm/reuse/outlier and dependency faults. Actual provider/
  native/retention obligations remain named; no absolute abuse-immunity claim.
- **Recovery:** independently disable soft classifier/new policy; mandatory
  integrity, free access and privacy duties remain.

## Exit and references

Exit: supported calibrated rules/explanations, reproducible source acceptance
and measured limits; known critical failures block closure. New policy lacking
evidence is not published. Continue selected C without per-stage remote waits.

Primary references: [PDQ reference](https://github.com/facebook/ThreatExchange/blob/main/pdq/README.md)
and [NIST outlier guidance](https://www.itl.nist.gov/div898/handbook/eda/section3/eda35h.htm).
They guide evaluation; they do not establish efficacy in Abastevo.
