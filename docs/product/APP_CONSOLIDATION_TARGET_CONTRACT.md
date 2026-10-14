# App consolidation target contract

Date: 2026-10-14. Owner: [master plan](../planning/APP_CONSOLIDATION_PLAN.md).
A/B/C behavior is PROPOSED before owning contract/calibration tasks. D's timing
restriction in rule 17 is ADOPTED by explicit maintainer request now.
No API/runtime/provider rule exists merely because it is described here.

## B-BR-OPT-01–18

- **B-BR-OPT-01 — Scope:** GIVEN public local data, WHEN Home reads, THEN match
  origin/city/source/product/unit/full condition/version; no ANP/community or
  city/environment substitution.
- **B-BR-OPT-02 — Validity:** GIVEN saved/checked data, WHEN time/network changes,
  THEN retain source dates and distinguish live/history/stale/unknown.
  Save/retry/304 never renews source age.
- **B-BR-OPT-03 — Refresh:** GIVEN existing scoped data and failed refresh, WHEN
  resuming/reopening, THEN preserve useful data/history. Successful empty response
  is distinct from failure and updates availability.
- **B-BR-OPT-04 — Work bounds:** GIVEN invisible/offline or duplicate triggers,
  WHEN refreshing, THEN avoid unnecessary polling and converge requests while
  respecting manual/city change and bounded cancellation/retry.
- **B-BR-OPT-05 — Conditional public reads:** GIVEN ETag/reuse, WHEN validating,
  THEN bind scope and expiration/invalidation; no expired population 304.
  Private owner/proof/commands remain no-store. Payload and SQL savings differ.
- **B-BR-OPT-06 — No GPS gate:** GIVEN ordinary contribution without location,
  WHEN processing, THEN enforce identity/ownership/integrity/quota, never new
  mandatory GPS/150m, missing-location fraud or proximity weight.
- **B-BR-OPT-07 — Evidence limits:** GIVEN similar media/capture receipt, WHEN
  counting support, THEN distinguish legitimate replay/shared capture and
  independent evidence. It does not prove intent, fresh pixels or purchase;
  no D document intake before its gate.
- **B-BR-OPT-08 — Context:** GIVEN unusual price/behavior, WHEN assessing, THEN
  use compatible dated context and allow unknown/review. Sparse/MAD-zero data,
  delayed offline batch and shared IP are not automatic fraud/travel proof.
- **B-BR-OPT-09 — Independence:** GIVEN linked keys/media/attribution, WHEN
  counting support, THEN no extra people from replay/self-support/reused proof,
  likes/payment or nominally separate coordinated accounts.
- **B-BR-OPT-10 — Reviewed trust:** GIVEN promotion/restriction, WHEN changing
  tiers, THEN require independent reviewed outcome and actor/reason/provenance
  with recovery; ordinary mutual consensus cannot train its own reputation.
  New/low-history alone is not abuse.
- **B-BR-OPT-11 — Versioned meaning:** GIVEN changed semantics, WHEN publishing,
  THEN preserve old replay and record new policy/input/explanation. Observation
  risk, price confidence, contributor trust, station quality and business badge
  remain distinct; none is purchased/inferred from another.
- **B-BR-OPT-12 — Privacy:** GIVEN stored state/objects/signals, WHEN erasing/
  restoring, THEN minimize, bound retention and replay deletions. Operational
  photos keep 24h; no precise contributor GPS/private document/key/token dump
  in public responses/logs.
- **B-BR-OPT-13 — History:** GIVEN incomplete sampling, WHEN presenting trends,
  THEN expose observed source/period/coverage/gaps/corrections; no unseen
  continuous records, source equivalence or invented forecast.
- **B-BR-OPT-14 — Alerts:** GIVEN a match, WHEN dispatching, THEN recheck source/
  condition/freshness, scope/rule/revocation and applicable authority; consent,
  quiet policy and bounded dedupe/retry apply. Queue/provider acceptance is not
  device receipt; existing basic alerts survive.
- **B-BR-OPT-15 — Exact estimated economy:** GIVEN compatible current prices,
  volume/consumption and explicit baseline, WHEN comparing, THEN use explicit
  units/rounding and incremental travel fuel/tolls. Time is separate unless
  user-valued; missing inputs yield insufficiency.
- **B-BR-OPT-16 — Routes:** GIVEN geometry/origin/route, WHEN ranking savings,
  THEN use permitted bounded current evidence with limits. Straight-line/unknown
  points cannot become actual detours; routing cannot gate contribution or
  produce persistent travel logs.
- **B-BR-OPT-17 — Future D lock:** GIVEN receipt/NFC-e or portal expansion, WHEN
  selecting implementation, THEN keep DEFERRED_FUTURE until extreme consolidation
  AND thousands of real clients with documented criteria plus D selection; only
  an explicit early maintainer request waives timing for its named scope.
  A–C completion/downloads/generic continuation do not. No hidden D code/schema/
  intake/dependency behind a flag in A/B/C.
- **B-BR-OPT-18 — Acceptance:** GIVEN plans/checkpoints/old evidence, WHEN declaring
  completion, THEN distinguish planned/source/integrated/native/provider/
  production; preserve modules/source, actual authorization and protected delivery.

## BUC-OPT-01–10

- **BUC-OPT-01:** Read/resume/reopen Room public summary; test refresh failure,
  empty success, history/expiry, scope and no-data behavior.
- **BUC-OPT-02:** Schedule/revalidate public summary; test ETag/expiry/invalidation,
  concurrency/outage and actual bytes/SQL bounds.
- **BUC-OPT-03:** Derive bounded independent media/behavior/price signals;
  test replay/ambiguity/offline burst and conservative fallback.
- **BUC-OPT-04:** Review/correct/appeal contributor/price decisions; test authority,
  collusion, transactional recompute and old policy replay.
- **BUC-OPT-05:** Read bounded observed history with gaps/corrections/cache/pages.
- **BUC-OPT-06:** Configure/evaluate/dedupe/deliver/cancel alerts; test free
  baseline, ownership/revoke, expiry, provider failure and receipt.
- **BUC-OPT-07:** Explain net fueling economy with explicit reference/volume/
  consumption/detour/tolls/time and unavailable outcomes.
- **BUC-OPT-08:** Future D only: submit/check/cancel private purchase proof with
  authenticity, linkage and rights after D reopening.
- **BUC-OPT-09:** Future D only: claim/review/grant/revoke business authority
  and publish business-source information after D reopening.
- **BUC-OPT-10:** Export/erase/recover scope-owned data/projections without
  resurrection, source substitution or extended privacy deadline.

Pure RED → GREEN → REFACTOR and immediate critical auth/money/privacy/jobs/SQL/
PostGIS/S3 tests follow the repository contract, never deferred known failures.
