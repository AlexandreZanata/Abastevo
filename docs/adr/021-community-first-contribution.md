# ADR-021 — Address-first community contribution without mandatory proximity

Date: 2026-10-13
Status: ACCEPTED for source construction by explicit maintainer request; deployment remains separately gated.

## Decision

B-BR-CF01–CF07 and BUC-CF01–CF05 in the PRM plan govern this source batch. A valid canonical station identity and nominal address/municipality/UF suffice for ingestion and selection. Latitude/longitude are optional enrichment. Preserve CNPJ/UUID identity, source provenance, unknown quality and existing reviewed points. Never manufacture coordinates or merge homonyms by address.

Normal signed photo capture and price contribution do not require location permission, a GPS fix, reviewed station coordinates or a 150m distance. Canonical active-station existence, identity/account/quota/idempotency, owner/key/station/evidence binding, original timestamps, media integrity/admission and 24-hour operational retention remain mandatory. Existing optional observation claims still undergo forgery checks; absent location is not forgery. No contributor fix is persisted or logged.

Normal capture exposes the existing city-scoped Directory name lookup with address disambiguation, independent of developer preview. Nearby discovery remains optional. Search, contribution, helpfulness, confirmations/disputes, reports and moderation remain free.

Initial consensus uses independently validated media and independent contributor support, with no proximity advantage or compulsory GPS review risk. Helpfulness never becomes price truth, consensus weight or moderator authority. Keep official/community sources separate. A receipt means workflow admission; receipt/observation acceptance does not itself mean validated price.

Mandatory proximity is DEFERRED until the product is validated, reliable verified establishment coordinates cover strictly more than 90% of active Brazilian stations, and the owner explicitly approves a new tested policy. Exactly 90%, candidates, centroids or duplicate sources do not qualify. No automatic enforcement switch is introduced.

## Supersession and limits

For this initial policy, this decision supersedes mandatory location admission in P37-PC01/B-BR-PC01, B-BR-CS07, P16 location/offline/photo contracts and associated app copy. Historical source evidence, legacy policy replay and optional discovery geometry remain valid in their original scope. No G09/device/provider/production acceptance is inferred. PRM22 and Premium commercial/provider activation are outside this source batch.

## Verification

PRM15 freezes compatibility and consensus/coverage policy before behavior. PRM16–PRM21 prove Rust→Directory→signed capture→media→submission→community moderation and Android recovery, including absent coordinates/denied permission/far contribution, auth negatives, replay/concurrency and real disposable PostGIS. See docs/planning/PREMIUM_BACKEND_APP_PLAN.md and the source evidence record.
