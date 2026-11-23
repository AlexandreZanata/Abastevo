# Premium backend integration guide

Transport: `POST` JSON with `family_id` plus `access_token` session proof.
Writes add the exact request body signed by the account P-256 key and the
bound `device_id`. Cursors are opaque HMAC strings: return `next_cursor`
verbatim until empty. All responses are `no-store` without ETags.

Route groups: `/v1/ledger/*` (records, consent, erase, reports, CSV),
`/v1/alerts/*` (rules, inbox, erase, push tokens), `/v1/metrics/*`
(compare), `/v1/cloud/*` (sync, snapshots, restore). Paid writes need an
active capability and fail `403` on lapse; reads, deletes and erasures stay
free so retained data and exits remain reachable.

Android ledger sync (`LedgerSyncHttpClient`): commands and consent go
through `PhotoProofTransport`, which refreshes the session for each attempt
and signs the exact body with the device key bound to the account; the
operation ID is the command's idempotency key. Record pages stay unsigned
session reads. The backend refuses every upsert outside the account's consent
mask (default off), so "Participate in private sync" first records signed
consent for the datasets enabled on the device and turns on only when the
answer contains them all. A dataset turned on while participating is
consented the same way. Turning participation off stops the device at once,
then withdraws consent for every dataset. Device and dataset revocations stay
local. Android has no cloud-erase action yet.

Errors: `400` malformed, `401` authentication, `403` capability, `404`
unknown owner-scoped resource, `409` revision conflict with base, current
and proposed versions for explicit resolution, `503` unavailable. Never send
GPS, photos, credentials or provider tokens; CSV cells are formula-safe.
Golden vectors in `contracts/testdata/api` pin request shapes.
