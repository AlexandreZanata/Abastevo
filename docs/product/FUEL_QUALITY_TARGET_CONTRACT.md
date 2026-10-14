# Fuel quality and station reliability target contract

Status: PROPOSED / NOT IMPLEMENTED by this planning change, 2026-10-14.
Owner: [FQ-00–FQ-17 implementation plan](../planning/FUEL_QUALITY_IMPLEMENTATION_PLAN.md).
Existing price, account, feedback, media, trust and profile rules retain their
meanings. FQ-01 adopts/finalizes these targets before behavior; FQ-12 separately
freezes numeric score methodology. This file changes no API or runtime policy.

## Product promise

For each canonical station, expose dated official evidence by fuel, distinct
enforcement/sanction records, clearly identified consumer experiences and a
versioned explanation of available reliability measures. Every station can have
a quality profile; lack of observations means insufficient evidence.

A user can inspect all safely matched published official cases, open their
source, request correction of a station association and attach a related
experience. User votes/claims do not validate chemical quality or legal guilt.

## B-BR-FQ-01–16: proposed behavior and acceptance

- **B-BR-FQ-01 — Source separation.** GIVEN PMQC, enforcement, sanctions,
  community reports, ratings and price confidence, WHEN presenting a station,
  THEN expose their provenance/meaning separately. No user command edits an
  official finding; PMQC is not an infraction notice.
- **B-BR-FQ-02 — Exact historical identity.** GIVEN a source event, WHEN binding
  it, THEN use a valid full establishment CNPJ and explicit chronology/review.
  A name/address/nearby point, CNPJ root or different branch cannot establish the
  binding. Unmatched/ambiguous cases remain in quarantine/review, not lost.
- **B-BR-FQ-03 — Lossless samples and assays.** GIVEN several assay rows for a
  sample, WHEN importing, THEN retain each distinct assay/result/unit/conformity
  and count the sample independently. Unknown fields/fuels/status stay explicit.
  Missing or conflicting native identity never silently merges samples.
- **B-BR-FQ-04 — Versioned official facts.** GIVEN a revised resource, WHEN
  publishing, THEN retain hash/source/schema/native-key provenance and append
  correction/supersession/withdrawal. Identical content is idempotent; changed
  content cannot overwrite historical evidence or double count a sample.
- **B-BR-FQ-05 — Complete-run visibility.** GIVEN a failed/partial import or
  concurrent replay, WHEN serving, THEN retain the previous complete published
  revision. Counts reconcile all inputs and all matching dispositions; a URL
  label alone never proves a period's coverage.
- **B-BR-FQ-06 — Honest procedural status.** GIVEN an action/fine/proceeding
  snapshot, WHEN displaying, THEN preserve actual scope/status/as-of date.
  Autuation, payment, absence of later records or unverified linkage never means
  definitive guilt, acquittal or current legal status.
- **B-BR-FQ-07 — Public and private authorization.** GIVEN a guest, WHEN reading
  official public history, THEN allow bounded reads. GIVEN a write/private owner
  read, THEN require current authenticated account and verified caller binding;
  reject forged owner/station/event/fuel and revoked or foreign proof.
- **B-BR-FQ-08 — Claims versus checked purchases.** GIVEN a consumer report,
  WHEN assigning evidence level, THEN distinguish user claim, supplied document
  and independently checked purchase. An image/OCR/valid-looking key/CNPJ alone
  never verifies a purchase, adulteration or ANP protocol. Failed/unavailable
  verification stays explicitly unresolved.
- **B-BR-FQ-09 — Temporal correlation.** GIVEN a related official case and
  consumer experience, WHEN linking, THEN verify station/fuel/revision and show
  dates. Proximity in time does not prove the same batch/tank, cause or diagnosis.
- **B-BR-FQ-10 — Independent community signals.** GIVEN related accounts/keys
  or reused receipts, WHEN aggregating, THEN avoid multiple support for one
  attribution/purchase. Distinct accounts are not guaranteed distinct humans.
  Thresholds need frozen evidence; unresolved abuse stays under review.
- **B-BR-FQ-11 — Score sufficiency and isolation.** GIVEN too little/stale/
  unmapped evidence, WHEN reading a score, THEN return null with reason/counts.
  Eligible per-fuel scores have frozen sample semantics/window/rounding and
  version. Payment, rating, brand, profile badge, raw report count and pending
  non-quality proceedings cannot affect the proposed v1 numeric score.
- **B-BR-FQ-12 — Reproducible dated explanations.** GIVEN identical eligible
  official facts, policy and injected clock, WHEN rebuilding, THEN produce the
  same score/flags/counts independent of input order. Return as-of, expiry,
  coverage and input/policy revision. Expired views are stale even if jobs stop.
  A current favorable score cannot hide a dated nonconformity.
- **B-BR-FQ-13 — Fuel scope.** GIVEN evidence for only one fuel, WHEN presenting
  the station, THEN identify its coverage. Gasoline findings cannot establish
  diesel quality; a station-wide numeric index needs its own accepted aggregation
  policy. Every station can expose an unavailable score-state object.
- **B-BR-FQ-14 — Privacy and retention.** GIVEN receipts/workshop/lab documents,
  WHEN storing or rendering, THEN keep them private, minimize identifying fields
  and apply the established operational media ceiling of 24 hours. Private
  protocol/CPF/keys/GPS/object references never appear publicly or in logs.
  Training consent is unrelated. Erasure/restore replay and checked-status
  degradation are documented before persistence.
- **B-BR-FQ-15 — Review and recovery.** GIVEN disputed linkage, report abuse,
  source correction or erasure, WHEN deciding, THEN append actor/reason/policy,
  adjust eligibility and enqueue affected rebuilds atomically. Corrected wrong
  links are removed from the active wrong-station view; appeals cannot rewrite
  source records or grant station representatives moderation power.
- **B-BR-FQ-16 — Measurable complete access.** GIVEN one frozen complete source
  edition, WHEN traversing the public case API/app, THEN every matched eligible
  event is reachable exactly once and each has source/date/detail. Record the
  denominator plus unmatched/quarantined/missing periods. Offline cached content
  is dated; queued writes are not server-acknowledged validations.

## BUC-FQ-01–09 and required scenarios

- **BUC-FQ-01:** Discover/prepare/stage/publish official evidence. Test malformed
  archive/schema/period/encoding, limits, loss accounting, deduplication, partial
  failure, retry/concurrency and prior-publication preservation.
- **BUC-FQ-02:** Resolve/review station linkage. Test full-CNPJ branch distinctions,
  leading zeros/alphanumeric values, missing/historical identity, conflicting
  mappings, succession and corrective replay.
- **BUC-FQ-03:** Revise/retract/recompute. Test corrected assays/identity/status,
  source resource replacement, duplicate annual/monthly data, atomic visibility,
  stale jobs and rebuild convergence.
- **BUC-FQ-04:** Inspect official station/fuel history and case detail. Test guest
  access, source separation, no sample, partial fuel coverage, stale data,
  procedural labels, bounded complete pagination and source links.
- **BUC-FQ-05:** Submit/edit/withdraw own structured experience and request linkage
  review. Test signed owner proof, idempotency/replay, invalid targets/text/dates,
  foreign-owner denial, parallel writes, moderation and source immutability.
- **BUC-FQ-06:** Attach/check/minimize/expire private supporting documents. Test
  fabricated/reused/foreign evidence, unavailable verifier, false protocol claim,
  expiry races, every-copy deletion, erasure and restoration.
- **BUC-FQ-07:** Detect/review/appeal a community pattern. Test related attribution,
  account farms, duplicate purchases, conflicting official history, insufficient
  independence, audit and no numeric official-score contamination.
- **BUC-FQ-08:** Export/erase/recover private quality data and command state. Test
  logout/account/origin isolation, expiry, restored deletions, redacted audit,
  lost-response convergence and no resurrection of checked/private evidence.
- **BUC-FQ-09:** Explain/rebuild available reliability measures. Test hand-calculated
  sample denominators, no/sparse/stale inputs, exact arithmetic, input reordering,
  fuel coverage, policy changes and chronological recovery without history loss.

Critical SQL/auth/privacy/jobs scenarios use immediate real PostGIS and actual
affected storage/transport boundaries; pure rules use deterministic RED → GREEN
→ REFACTOR. Runtime completion requires those tests, not this target document.

## Proposed UI language and limits

UI is localized; the following Portuguese phrases convey the intended meaning:

- "Amostra ANP/PMQC: conforme em [data]" or "Ensaio não conforme registrado".
- "Ação de fiscalização registrada"; show source-proven procedural status.
- "Relato do usuário", "Documento apresentado" and "Compra verificada" only
  where a recorded independent verification supports the latter.
- "Dados insuficientes para calcular a nota"; show fuel/date/coverage reasons.
- "Nota baseada nas amostras históricas disponíveis"; link methodology and
  source denominator. Avoid "combustível garantido", "fraude comprovada" without
  appropriate official proof, or claims of ANP certification/affiliation.
- "Informar divergência neste registro", "Relacionar minha experiência" and
  "Consultar canal oficial da ANP". No automatic complaint transmission.

Concrete wire enums, endpoints, legal/privacy copy, verification providers and
score thresholds are frozen by their owning tasks; none exists merely because
its target label appears here.
