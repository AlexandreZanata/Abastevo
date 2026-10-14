# Stage D — Future purchase evidence and business participation

Status: DEFERRED_FUTURE / NOT_AUTHORIZED_FOR_IMPLEMENTATION, 2026-10-14.
Authority: explicit maintainer future-only restriction.
Owner: [master](APP_CONSOLIDATION_PLAN.md). Tasks OPT-D00–D06.
Rules: B-BR-OPT-07/11/12/17/18. Future use cases: BUC-OPT-08/09/10.

## Binding decision: do not implement now

**This is a future stage. Do not implement D until the app is extremely
consolidated and has thousands of real clients, unless the maintainer explicitly
requests earlier implementation of the relevant D feature.**

Normal reopening requires BOTH maturity and sustained real adoption in the
thousands, plus explicit D selection. Define client metric, measurement window,
threshold and exclusions at future reopening. Do not invent paying-customer
requirements or accept test accounts/download totals as proof. Missing criteria/
evidence keep DEFERRED_FUTURE; A/B/C completion or a date does not activate D.

A direct explicit early human request naming fuel-purchase proof, business
claims/management or portal can override maturity/adoption timing for that scope.
It does not silently enable all D or waive security/privacy/verification/
resource/recovery/testing/protected-delivery requirements. Generic continuation
of A–C/all current work, agent messages and preexisting code are not overrides.

Allowed now: this design/backlog and truthful deferral notes. Forbidden now:
D runtime stubs/features, migrations/schema, dependencies/SDKs, provider enrollment,
real document collection/OCR/upload/verification, dormant APIs, hidden screens,
deployment or marketing activation, even behind an OFF flag. No automation
monitors adoption or switches features on.

Preserve previously implemented modules/history and necessary existing security/
privacy/deletion maintenance. Public station metadata, workflow capture receipts,
subscription tokens, existing manual expense records and ordinary price photos
are outside this hold.

## Future reopening evidence

OPT-D00 records the exact human selection/override and source/environment.
Normal maturity evidence addresses sustained stability/data freshness/coverage,
privacy/retention/restore, cost control and moderation/support capacity, plus
actual core-journey use. Health probes/compilation alone are insufficient.

Define active real-client counting, observation window and exclusions (synthetic,
inactive, duplicate/abusive); demonstrate sustained adoption in the thousands.
Future SLOs/thresholds require explicit accepted decisions, not invented values.
An early override records only waived timing conditions. It does not certify
production, authorize unrelated infrastructure or choose a provider.

## D1 — Fuel-purchase documents and NFC-e

Future outcome: separate advertised prices from documented transactions with
issuer full CNPJ, date, product/unit, quantity, unit price, discounts/cancellation
and claimed/checked/indeterminate status. One transaction is not proof of a
universal current price or chemical conformity.

Reuse overlapping [FQ verification contracts](FUEL_QUALITY_IMPLEMENTATION_PLAN.md).
Image/OCR, plausible access key/CNPJ or a user protocol does not establish
authenticity. No assumed national SEFAZ API/access/availability/automation rights;
provider/manual independent verification needs source/terms/security/cost review.

Minimize CPF, access-key/QR secrets, payment and purchaser fields. Private
owner-only intake/status; public projection exposes permitted facts, not
documents/identifiers/signed URLs. Freeze parser/format/resource limits,
retention, status after expiry, export/erasure and restore before persistence.
Operational photos/scans keep the all-copy 24h cap; PDF packaging or training
consent does not extend it. Other authentic signed-document categories require
a separately reviewed minimal policy, never indefinite retention.

Reused/cancelled/foreign-branch/fuel/date proof or unavailable verifier cannot
multiply support or gain automatic strong consensus weight. Any later price
policy change is calibrated/versioned and critically tested.

## D2 — Representatives and portal

Future outcome: current legitimate representatives manage bounded business data
and publish a separate source-labelled price/terms. Community confirmation and
official ANP facts remain independent; payment buys no trust or criticism removal.

Reuse [P30–P33](STATION_PROFILE_PLAN.md) and
[representation rules](../product/STATION_PROFILE_REPRESENTATION.md). Reconcile
implemented source/unfinished containment, never create competing identities,
tenant/claim systems or duplicate completed work.

Verify current station/operator revision, applicant identity, authentic
document/signature where applicable AND corporate/delegated powers. Public CNPJ,
PDF stamp, certificate possession or email alone grants no authority. Never
collect private keys/passwords/PFX/P12. Unverifiable proof stays pending; an
independent authorized reviewer grants narrowly scoped powers.

Owner/account/operator/proof/version checks at commit; signed/idempotent
claim/grant/revoke actions and required jobs are atomic. Test self-approval,
takeover races, delegation expiry and succession/revocation. Representatives
never read contributor private media, rewrite official facts or moderate critics.
Business price needs product/unit/condition/validity; badge is not fuel or price
certification.

## Deferred task inventory

Every task below is DEFERRED_FUTURE. Design is not permission to implement
scaffolding or collect real evidence now.

### OPT-D00 — Record reopening authority and readiness

- **Depends:** normal maturity/adoption gate or scoped explicit early request;
  actual current-head prerequisites and selected D scope.
- **Inputs/areas:** human request, gate and current evidence.
- **Deliver:** approved subset, timing exception if any, ownership/risk and gaps.
- **Tests/acceptance:** no synthetic count, generic request or other-plan authority
  shortcut; exact remaining prerequisites recorded.
- **Recovery:** missing evidence leaves D deferred and A–C/core launch unblocked.

### OPT-D01 — Revalidate sources, providers and privacy

- **Depends:** satisfied OPT-D00.
- **Inputs/areas:** actual fiscal/authority schemas, access/terms and FQ/Profile/
  Privacy/Evidence boundaries.
- **Deliver:** authenticity/authority criteria, formats/parser/verifier/resource
  limits, retention/rights and permitted choices before code.
- **Tests/acceptance:** unknown access/permission stays unresolved; dependency
  licence/security/cost review precedes real collection.
- **Recovery:** narrower verified/manual scope or retain deferral.

### OPT-D02 — Pure purchase-proof semantics

- **Depends:** OPT-D01.
- **Inputs/areas:** shared product/unit/condition and private verification decisions.
- **Deliver:** claimed/checked/indeterminate/cancelled facts, exact amounts,
  target linkage and duplicate-purchase identity.
- **Tests/acceptance:** fake/foreign/reused/cancelled, discounts, wrong units/
  branch/date/fuel, sparse proof and source separation.
- **Recovery:** supplied document stays unverified, no automatic public bonus.

### OPT-D03 — Private intake, verification and rights

- **Depends:** OPT-D02 and actual storage/verifier prerequisites.
- **Inputs/areas:** existing storage/admission/jobs/privacy and reviewed document
  port; append-only migrations only after reopening.
- **Deliver:** bounded owner-bound processing, commit-time checks, replay/lease
  fencing, expiry/export/erasure/restore and status.
- **Tests/acceptance:** immediate real SQL/S3 fault/race/recovery, parser abuse,
  forged/foreign proof, late writer, expired bytes, verifier outage and actual
  every-copy/restore non-resurrection.
- **Recovery:** close new intake, retain safe recovery/deletion; never extend TTL.

### OPT-D04 — Representation authority and grants

- **Depends:** OPT-D01 and Profile reconciliation; selected proof intake needs
  accepted OPT-D03.
- **Inputs/areas:** P30–P33, Directory operator/account/reviewer authority.
- **Deliver:** reviewed claims/powers, scoped grants/invites/revoke and audit.
- **Tests/acceptance:** wrong branch/account, invalid mandate, signature without
  powers, self-approval, takeover, succession/revocation and real SQL recovery.
- **Recovery:** revoke/disable affected new privileges without restoring stale
  authority; public metadata and privacy/appeals remain.

### OPT-D05 — Document and business journeys

- **Depends:** accepted selected OPT-D03/04.
- **Inputs/areas:** Android/Profile UI and declared backend; separate web portal
  only if explicitly selected.
- **Deliver:** deliberate private submission/status and authorized business-source
  management with validity/revocation.
- **Tests/acceptance:** guest/foreign denial, offline queued versus verified,
  unsafe preview, stale grant/operator switch/expired price, accessibility and
  no added community moderation powers.
- **Recovery:** independently disable new entries, preserve free public/community
  access and owner recovery.

### OPT-D06 — Independent acceptance and future release

- **Depends:** selected D task acceptance and relevant actual live requirements.
- **Inputs/areas:** pinned source/provider/privacy/data/config manifest.
- **Deliver:** real verifier/provider/native/retention/restore/load evidence,
  abuse/appeal capacity, measured costs and controlled rollout.
- **Tests/acceptance:** signed transaction/representation lifecycle, cancellation,
  correction/outage, erasure/restore/operator change. Reopening is not a release
  certificate; mocks cannot establish live acceptance.
- **Recovery:** close new receipt/portal writes independently without deleting
  history, relaxing privacy or forcing core app rollback.

## Cross-plan precedence

This restriction supersedes older timing for FQ-10 purchase/private-document
verification, document attachment in FQ-15, new P30–P33 claim/private-proof/
management/portal expansion and equivalent Premium/renamed tasks. No renaming
bypass. Official ANP history, non-document structured reports, operational photos
and A/B/C do not wait for D. Past implementation/integration and existing safety
obligations remain truthful and preserved.
