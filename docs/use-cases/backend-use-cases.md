# Backend use cases

Planned contracts; no implementation implied. Backend IDs avoid the existing Android UC-001…015 namespace. Domain rules are in [DOMAIN_MODEL](../backend/DOMAIN_MODEL.md); wire contracts in [API_PLAN](../backend/API_PLAN.md).

## BUC-001 — Import official revision

Actor: scheduled worker/restricted operator. Preconditions: allowlisted source, parser fixtures validated, import lease available. Flow: discover → bounded download/checksum → parse/stage → resolve stations → quarantine/validate → atomically publish revision → record report. Alternatives: same bytes no-op; provider unavailable retries; layout/quality failure retains previous publication; corrected bytes create superseding revision. Rules: B-BR-001/002/013/014. Events: StationRegistered, OfficialRevisionPublished. Postcondition: no partially published official week. Acceptance: identical rerun stable, correction preserves prior prices, malformed import never clears current catalog. Tasks: P02-T01…T08.

## BUC-002 — Establish and maintain anonymous identity

Actor: installation or synthetic client. Preconditions: generated supported key; no traditional account required. Flow: challenge bound to fingerprint → proof → unique contributor/key registration → signed action using fresh challenge → rotation with old/new proofs when requested. Alternatives: replay/expiry rejects; duplicate registration returns existing identity after proof; blocked key denied; lost key cannot recover anonymous identity. Rules: B-BR-004/005/015. Events: ContributorRegistered, ContributorKeyRotated. Postcondition: private key never leaves client; server identity derives from proof. Acceptance: race/replay/key ownership and rotation fixtures. Tasks: P03-T01…T06; Android only P10-T03.

## BUC-003 — Submit and validate a price observation

Actor: anonymous contributor, then worker. Preconditions: signed identity, valid station/product/unit/condition, stable client submission ID, quota. Flow: validate request → persist immutable fact + idempotency + job → acknowledge RECEIVED → worker obtains station/evidence/trust signals → append VALIDATED or REJECTED decision → enqueue consensus if eligible. Alternatives: metadata-only accepted under lower confidence policy; photo pending waits; old capture historical-only; transient dependency fails/retries without fake approval; retry returns same resource; correction creates superseding observation. Rules: B-BR-001…005/010/011/014/015. Events: PriceObserved, ObservationValidated/Rejected. Acceptance: atomicity, immutability, owner-only status and no client-controlled trust. Tasks: P04 and P06-T01.

## BUC-004 — Provide private photographic evidence

Actor: contributor and evidence worker. Preconditions: identity, quota and allowed JPEG intent. Flow: reserve session → short presigned PUT → complete intent → bounded immutable snapshot validation → strip metadata/write sanitized final key → READY → bind to owner observation. Alternatives: duplicate completion idempotent; bad media rejected; overwrite after presign cannot change verified final bytes; DB/object failure reconciled; orphan/expiry cleanup. Rules: B-BR-010/011/015/016. Events: internal EvidenceVerified, EvidenceRejected, EvidenceDeleted. Postcondition: no public evidence and no unvalidated photo counted as proof. Acceptance: actual staging storage behavior plus security fixtures. Tasks: P05; camera/OCR only P10-T04.

## BUC-005 — Confirm, dispute and publish current price

Actor: distinct contributor; consensus worker. Preconditions: public projection exposes a representative eligible observation ID, identity and quota; target validated and eligible. Flow: view amount/product/full condition → submit unique confirmation or structured dispute/replacement → append fact and recompute job → reduce independent votes → publish versioned projection with expiry. Alternatives: self-confirmation denied; repeated request deduplicated; supported conflict returns DISPUTED; no eligible anchor returns UNKNOWN; expiry applies even with worker down. Rules: B-BR-006…009/012. Events: PriceConfirmed, ObservationDisputed, CommunityPriceChanged. Acceptance: golden results identical under permutation; payment immaterial. Tasks: P06-T02…T05.

## BUC-006 — Moderate abuse with an audit trail

Actor: authorized operator via restricted CLI. Preconditions: strong operator access, open case or documented target/reason. Flow: inspect minimal signals and authorized private evidence → record reviewed decision → application command invalidates/block/reviews → append audit + recalculation jobs → verify projected result. Alternatives: no privilege/reason denies; appeal produces a new decision or reviewed replacement fact; deleted evidence explicitly unavailable. Rules: B-BR-003/004/009/011/012/016. Events: ModerationActionRecorded, ObservationRejected, ContributorTrustChanged as applicable. Acceptance: no public admin route, direct destructive edit or unaudited override. Tasks: P07-T01/T02.

## BUC-007 — Export, erase and expire personal data

Actor: verified contributor; privacy worker; responsible operator for exceptional requests. Preconditions: current ownership proof or separately reviewed support procedure. Flow: register request → gather inventory → private expiring export OR revoke writes/purge/unlink per approved policy → recompute affected facts/projections → receipt → retain minimal deletion ledger through backup horizon. Periodic cleanup enforces inventory deadlines. Alternatives: lost-key request cannot disclose unverifiable data; retention hold requires lawful reviewed reason/deadline; restore must replay deletions before traffic. Rules: B-BR-011/016. Events: EvidenceDeleted and minimal PrivacyRequestCompleted. Acceptance: own data exported, erasure effective and no resurrection after restore. Tasks: P07-T03…T05, P08-T04.

## BUC-008 — Release and recover the backend

Actor: operator. Preconditions: tested release, fresh backup, compatible migration, separate deployment privileges. Flow: stage → backup → migrate → deploy → smoke/health → monitor → record release evidence. Recovery: disable writes if needed → restore off-host backup into new isolated environment → verify rows/projections/jobs/object references → apply erasure ledger → smoke → switch traffic. Alternatives: failed migration/readiness stops rollout; failed restore blocks launch; provider outage uses documented source-specific degradation. Rules: B-BR-003/005/008/011/013/016 and infrastructure runbooks. Acceptance: actual measured RPO/RTO, no erased-data leak, no false write acceptance. Tasks: P08/P09.
