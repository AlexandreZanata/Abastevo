# API plan

Status: contract design, not an implemented API. P01-T11 will encode the common schemas in `contracts/openapi/v1.yaml`; each endpoint must be specified and linted before its handler. This document explains semantics; the versioned OpenAPI file will own wire syntax. Conflicts require an explicit correction before implementation.

## Common conventions

HTTPS JSON under `/v1`; UUID identifiers; UTC RFC3339 timestamps; dates `YYYY-MM-DD`; exact integer milli-BRL prices with currency and unit. Limit JSON bodies to 64 KiB (observation metadata/OCR included). No base64 image in JSON. Limit text lengths (reason detail 500 characters, OCR normalized text 2,000). Reject duplicate JSON keys, malformed enum values, unknown write fields and noncanonical numeric forms; tolerate additive response fields in clients.

Success: resource or `{items, next_cursor, generated_at}`. Error: `{error:{code,message,trace_id,details:[{field,code}]}}`; message is safe, localized by the client using code. No SQL, keys, raw input or precise coordinates in errors. Status mapping: 400 malformed input/cursor; 401 missing/invalid proof or nonce; 403 prohibited action; 404 missing or non-owned resource (avoid ownership oracle); 409 conflict/idempotency mismatch; 413 body/file cap; 422 domain rejection; 429 quota with Retry-After; 503 unavailable dependency. Public 500 responses are generic and traced.

Pagination uses an opaque authenticated cursor containing filter hash, last sort key and expiry; no raw SQL offset parameter. Default limit 20, maximum 100. Directory sort by distance then station UUID, computed against the same request coordinate; reject cursor/filter changes. Results are live pages, not a full consistent snapshot; clients deduplicate IDs. For owner history use immutable `(received_at,id)` descending order and a snapshot cutoff in the cursor.

## Public reads (no contributor account)

- `GET /v1/stations?state=MT&municipality_code=...&q=...&limit=20&cursor=...`: directory search; q 2…100 characters; returns station ID, normalized CNPJ, display name, address, location quality and optional public station coordinates. Allowlisted filters; no arbitrary sort expressions. Cache 60 s only for non-sensitive queries.
- `GET /v1/stations/nearby?lat=...&lon=...&radius_m=3000&limit=20&cursor=...`: lat/lon bounds, radius 100…15000 m; returns public station fields plus distance_m. Exact request location is never returned, logged or cached; Cache-Control `no-store`. Stations lacking verified coordinates are not fabricated into results.
- `GET /v1/stations/{station_id}`: public station metadata and coordinate provenance/quality; ETag, `public,max-age=30,s-maxage=60`.
- `GET /v1/stations/{station_id}/prices`: product/condition groups with independent official/community sections; optional validated fuel filter. ETag/version and bounded cache TTL. Missing community data is null/UNKNOWN, not an ANP substitution.
- `GET /v1/stations/{station_id}/official-prices?fuel_product=...&limit=20&cursor=...`: revision-aware official history with survey_week, collected_on, source URL/checksum reference and exact amount. Current official value uses last successfully published applicable revision; old revisions remain queryable.

Defer `/prices/nearby`: station-nearby plus batched station price summaries is sufficient for MVP; do not create a second competing nearby contract without a measured client need. API v1 is new; the existing Android release does not call it.

Example price group (UUIDs illustrative, not fixture identities):

```json
{
  "station_id": "d6c74c23-63db-4c24-a2e5-408cb23bad26",
  "fuel_product": "GASOLINE_REGULAR",
  "unit": "L",
  "condition": {"kind": "STANDARD", "qualifier_id": null},
  "official": {
    "source": "ANP", "amount_milli_brl": 6030, "currency": "BRL",
    "collected_on": "2026-09-24", "survey_week": {"start": "2026-09-20", "end": "2026-09-26"},
    "revision_id": "aac027be-d331-40e4-8d63-52ec3b8d2f41"
  },
  "community": {
    "source": "COMMUNITY", "availability": "AVAILABLE", "amount_milli_brl": 5890,
    "currency": "BRL", "confidence": "MEDIUM", "freshness": "FRESH",
    "independent_supporters": 2, "confirmation_count": 1,
    "representative_observation_id": "45d0e8bc-01b1-441e-9b28-f8c16df7cb35",
    "last_observation_received_at": "2026-09-28T12:00:00Z",
    "expires_at": "2026-09-30T12:00:00Z", "algorithm_version": "consensus-v1"
  },
  "generated_at": "2026-09-28T12:05:00Z", "projection_version": 7
}
```

DISPUTED/UNKNOWN returns `amount_milli_brl:null`; historical/stale data may be included as a labelled separate last-known value. AVAILABLE includes a representative eligible observation ID for confirmation/dispute, bound to the displayed amount/product/condition. This opaque public reference grants no access to owner-only observation details. Public output omits contributor identities and the remaining private supporting event IDs. Revalidate target eligibility on write; a stale representative may return 409 and require refreshing the projection.

## Identity and proof of possession

- `POST /v1/identity/challenges`: bounded anonymous request with purpose REGISTER or SIGN and public-key fingerprint for registration, otherwise key_id. Response challenge_id, random nonce, server_time, expires_at (5 min). No indication whether an arbitrary key belongs to a blocked contributor. Rate limit and cap outstanding challenges.
- `POST /v1/contributors`: public JWK (EC/P-256), challenge_id and proof over exact request; registration proof uses the candidate key and a nonce bound to its fingerprint. Response 201 contributor_id, key_id, registered_at. Unique key fingerprint makes a lost registration response retry safe; do not allow overwriting the owner.
- `GET /v1/contributors/me`: signed request; private no-store; identity status, aggregate trust tier, quota information and rights-request availability. No other contributor's data.
- `POST /v1/contributors/me/keys/rotate`: old-key signed request containing new public key and new-key proof bound to a challenge. Rotate atomically, revoke old key for new writes, preserve contributor ID. If the old key is lost there is no anonymous recovery. Optional account recovery is later.

Initial protocol recommendation: [RFC 9421](https://www.rfc-editor.org/rfc/rfc9421.html), ECDSA P-256/SHA-256, existing crypto primitives and reviewed verification code. Cover method, authority, path, query, content-type, content-digest and idempotency-key when present; require created/expires/keyid/nonce. Reject ambiguous duplicate auth fields, algorithm substitution and incomplete covered components. Body digest covers exact transmitted bytes. Configure a canonical public authority behind trusted proxies. Final byte-level vectors, DER↔raw signature representation and proxy behavior must be frozen and independently checked in P03 before shipping. Never invent a new crypto algorithm.

Nonce expiry is enforced with server time, signature timestamps tolerate at most 5 min. Consume nonce atomically with accepted mutation; signed reads also atomically consume their nonce. Retry obtains a new challenge/signature with the same operation ID. Storing nonces/limits solely in API memory is prohibited. P-256 is a compatibility recommendation; the Android Keystore matrix is verified in P10 before app release. Ed25519 remains an alternative requiring evidence of supported secure key storage across minSdk 26 devices.

## Community commands (signed; private responses)

- `POST /v1/observations`: body fields from COMMUNITY_PRICING_SPEC; Idempotency-Key required; 201 `{id,validation_state:"RECEIVED",received_at,status_url}`. The 201 means durably received, not validated/published. A job is persisted in the same transaction.
- `GET /v1/observations/{id}`: owner-only full status/reason-safe fields, processing state and decision timeline; no-store. Moderators use a separate restricted interface.
- `GET /v1/contributors/me/observations`: own history with cursor pagination; no-store.
- `POST /v1/observations/{id}/confirmations`: `{client_submission_id}`; 201 confirmation_id/received_at. Natural uniqueness contributor+observation, no self-confirmation; optional supporting photo should be submitted as a new observation instead of expanding v1 confirmation complexity.
- `POST /v1/observations/{id}/disputes`: `{client_submission_id,reason,detail?,replacement_observation_id?}`; 201 dispute_id/status OPEN. Deduplicate active repeated reports. Dispute metadata is private.

Request idempotency scope = contributor_id + method + route template + key; store request hash, status and safe response transactionally for 7 days. Same key/body returns original status/resource; different body→409. Registration/rotation have additional unique-key safeguards. Observations/confirmations/disputes retain permanent client_submission_id uniqueness for their retained lifetime, so a mobile retry after seven days cannot create duplicate history. A replayed nonce is always rejected even when the operation ID is valid.

## Private media

- `POST /v1/uploads`: signed `{client_submission_id,content_type:"image/jpeg",size_bytes,sha256}`; reserve quota, generate server-only object key; 201 `{upload_id,method:"PUT",url,required_headers,expires_at,max_bytes}`. Initial upload URL TTL 5 min. Client hash is a claim, validated later.
- `POST /v1/uploads/{id}/complete`: signed/idempotent completion intent; 202 `{upload_id,state:"VERIFYING"}`. No arbitrary URL/bucket/object key accepted.
- `GET /v1/uploads/{id}`: owner-only status ISSUED/VERIFYING/READY/REJECTED/EXPIRED; READY returns evidence_id, never a public object URL.

Only restricted moderation can get a short-lived evidence download (60 s) through its application command. Private originals/sanitized photos are not public API resources. HEAD/GET validation and final immutable snapshot behavior are detailed in SECURITY_PRIVACY.

## Rights and operations

- `POST /v1/contributors/me/exports`: signed command; 202 export request ID; `GET /v1/contributors/me/exports/{id}` polls until it can issue a private short-lived download. Reauthenticate to download; download audit and expiry 24 h.
- `POST /v1/contributors/me/deletion-requests`: signed confirmation of deletion intent; 202 request ID; allow owner to see status while key remains active. Revoke contributor writes immediately, finish deletion and key removal through documented workflow. Losing the key prevents cryptographic proof; a support procedure must not disclose unrelated data.
- `/health/live` and `/health/ready`: operational responses, no config or secrets. `/metrics`: private network only. No public admin HTTP endpoints in MVP; restricted CLI over controlled operator access invokes audited application commands.

## Compatibility and cache safety

No silent enum rename or unit/amount reinterpretation in v1. Additive fields are allowed only after tolerant-reader tests; new mandatory write fields or changed semantics require a compatible rollout or v2. Capture the previous OpenAPI and run breaking-change checks. Domain values and language-neutral fixtures are reviewed together. Unknown future confidence values must display cautiously in the future client.

Cache allowlist is method+route+public response class. Strip authorization/cookies from public-read caching only after confirming responses are invariant to those headers; preferably reject auth on cacheable routes as unnecessary. No cache of errors containing owner data. Purge on moderation and bound TTL to expiration; CDN outage uses the origin, not a disabled-security shortcut.
