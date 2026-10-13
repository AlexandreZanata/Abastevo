# Community-first compatibility and validation policy

Authority: [ADR-021](../adr/021-community-first-contribution.md), B-BR-CF01–CF07; implementation tracked by PRM15–PRM21. No live activation claim.

## Capture compatibility — PRM15-T02

New normal receipts use `photo-capture-community-v2`, never the developer preview policy. Requests omit location by default; optional supplied metadata is checked for forgery without evaluating distance or physical travel. Require a current active canonical station, signed owner/key, account/quota/idempotency and existing media controls. Camera window remains two minutes; submission/operational original-media deadline remains twenty-four hours. No background GPS or deadline renewal.

Already issued `photo-capture-v1` receipts retain their immutable policy, binding and deadlines and remain consumable until expiration. Replaying their idempotency key returns the original receipt; never reinterpret or renew it. `photo-capture-ui-test-v1` retains its explicit debug deadline and isolation. Unknown policy refuses use. No schema rewrite of applied migrations.

Deploy schema allowing v2 before the compatible backend, then the new Android consumer. New Android expects v2 for fresh normal capture; an old backend/v1 fresh response returns a recoverable unsupported-policy error, without falling back to developer mode or fabricating a fix. Old apps that decode only v1 need an update for fresh capture; their valid existing drafts remain supported. New backend receives optional old-client location metadata without granting proximity trust. Backend rollback must retain v2 receipt-consumption support or drain valid v2 drafts before reverting; never delete drafts or extend deadlines.

Owner/key/station/origin/evidence changes and malformed/forged optional claims still refuse. Authentication, private media integrity, bounded intake and all-copy expiry are not location rules.

## Community validation and coverage — PRM15-T03

The new consensus version is `consensus-community-v2`. Retain v1 replay deterministically. Reuse existing exact-price grouping, one vote per contributor, NEW/ESTABLISHED factors, 48-hour window, runner conflict ratio 60%, MEDIUM minimum two independent supporters plus one independently validated photo, and HIGH minimum three supporters/two established contributors/two distinct validated photos within six hours plus runner weight below 25%. These existing numeric hypotheses are unchanged. V2 removes only the photo-proximity prerequisite. A photo alone or absent location never grants HIGH. Existing blocking moderation/disputes, duplicate media, blocked identities, stale anchors and missing evidence keep their gates. Helpfulness is separate and cannot supply these votes or photos.

Runtime signal derivation uses `signals-community-v2`: proximity stays UNKNOWN and missing/poor/distant GPS, unreviewed station geometry or station-hopping do not create contribution eligibility or review penalties. Retain media absence/duplicate, historical capture and regional-deviation review signals. Optional provided claims are still recomputed for forgery; geometric diagnostic/legacy functions remain separately tested.

Existing projections require idempotent re-derivation/recomputation from authoritative retained inputs under v2, with an append-only queued migration. Old audit inputs/results stay unchanged. Deleted/expired media cannot be recreated or counted just because policy changed.

Moderation keeps existing authenticated operator authority/actions and exact tally/reversal, independent confirmation/dispute and report budgets; no new majority-removal rule or service. Unconfirmed contributions may display honestly with LOW confidence; validation and visibility remain distinct.

Future coverage denominator: unique active canonical Brazilian establishments with an active authoritative registry identity in a pinned complete national snapshot, retaining inactive/excluded counts and date. Numerator: those same identities with independently reviewed establishment points, corroborated provenance and measured accuracy/site-matching evidence; exclude centroids, pending candidates, duplicates and stale/unverified points. Record national and regional gaps plus reproducible counts; require numerator/denominator strictly greater than 0.90. The maintainer owns the product-validation evidence decision (actual adoption/usability/community moderation and coordinate quality, not a fabricated adoption threshold). Both proofs plus an explicit new policy request are required for PRM22. No coverage percentage or product acceptance has been measured in this batch.

## Nominal Directory consumer contract — PRM17-T02

Reuse anonymous GET /v1/stations with manually selected municipality_code/state and optional q (empty lists first page, otherwise 2–100 Unicode characters), bounded limit and signed cursor. Existing literal-escaped ILIKE is case-insensitive and accent-sensitive; preserve this explicit contract rather than invent accent folding. Names do not merge identities. The current reader batches source enrichment in two bounded round trips, pages by canonical UUID and includes nominal address/CNPJ and honest null coordinates. Homonyms require visible address/municipality disambiguation. Unknown points are omitted only from nearby geometry, never nominal lookup. No new search service or dependency.
