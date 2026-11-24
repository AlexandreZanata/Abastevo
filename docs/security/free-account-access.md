# Free account access

## B-BR-A01…A05 / BUC-A01…A04

A01: Anyone can create a FREE account by **email with an access code, Google or Apple**. No purchase, card, entitlement or paid plan prerequisite. Existing free local/offline browsing and the anonymous price-contribution baseline remain available. The new station/fuel social writes require an authenticated active account.

A02: Email codes are short-lived, single-use, hashed at rest with bounded attempts/resend and atomic consume. Responses avoid account enumeration; mail adapter has deterministic local tests, rate limits and redacted failures. Define code TTL/length and recovery in P13-T01 using threat analysis before coding; do not invent an email service or spend money in planning.

A03: Google/Apple use established OIDC flows, system browser/native supported integration and PKCE where applicable. Backend verifies issuer, audience, signature/JWKS, expiry, nonce and replay; client identity flags are untrusted. Apple relay emails are supported. Do not merge accounts solely by equal email without authenticated linking proof. Provider key rotation, revoked grants, wrong-client tokens and unavailable providers get tests.

A04: Bind account to device contributor key only with fresh account reauthentication and key proof. Recovery cannot transfer another contributor's observations/reputation. Sessions are short-lived with rotating/revocable refresh families; logout, lost device, email/provider change and deletion invalidate appropriate access. Native secure storage stays behind ports. Public alias and private provider identity are separate; no tenant model.

A05: FREE access includes registration and social participation. Future paid entitlements buy explicitly validated hosted benefits and never trust. Server roles control moderation; device/app signing proof remains required by the applicable write contract. Export/erasure covers account subjects, linked keys, social revisions/votes and replicas/deletion ledger without leaking identity.

BUC-A01 signup/login, A02 link/unlink and recovery, A03 logout/revoke, A04 rights operations. Freeze additive contracts and a data inventory before implementation. Tests first: code replay/expiry/concurrent consume, token substitution, cross-account linking, stolen IDs, refresh reuse, suspension, provider outage and erasure/restore. Local provider stubs prove protocol behavior; real Google/Apple sandbox and native callback/device evidence remain phase acceptance, without production deployment.

The iOS client opens a session only with credentials issued by the account API.
The provider-link endpoint requires an existing live account session and does
not issue one. Older iOS previews' locally generated Apple sessions are refused.

Native Sign in with Apple (`POST /v1/accounts/providers/login`, P13-T05) is
the only provider route that issues a session:

- The app generates a random raw nonce, puts its lowercase hex SHA-256 in the
  `ASAuthorizationAppleIDRequest` nonce (Apple copies it verbatim into the
  identity token's `nonce` claim) and sends the identity token plus the RAW
  nonce. The server hashes the raw nonce before comparing, so a captured
  token alone never signs in, and consumes the claim once in the shared
  nonce ledger (only the hash is stored).
- The expected audience is `ANPFUEL_APPLE_NATIVE_AUDIENCE` (the bundle ID),
  never a request field and never the link audience `anpfuel-backend`, so
  link tokens and sign-in tokens are not interchangeable. Unset answers 503
  `account.provider-unavailable`. Google stays link-only (503).
- Issuer plus subject is the only lookup key. A linked subject signs in to
  its account; an unknown subject creates one FREE account whose only login
  method is that link (it cannot be unlinked). Suspended accounts refuse
  with `account.account-suspended`; deletion drops the link, so a later
  sign-in mints a new account without carryover. Parallel first sign-ins
  converge on one account. Token email is stored like a linked one
  (display/relay only, never a merge key). The token and the raw nonce are
  never stored or logged.
- The response is the email-code consume shape (`account`, `session`,
  `created`). Refusals: 400 `account.bad-body` (shape, token over 4096
  bytes, nonce outside 16–128 visible ASCII characters, authorization code
  over 512 bytes or not visible ASCII), 401 `account.oidc-*` (issuer,
  audience, signature, expiry, nonce mismatch or reuse), 403/410 account
  status, 429 session budget, 503 provider unavailable or JWKS outage.
- Token revocation on deletion (App Store guideline 5.1.1(v)): the request
  may carry the native `authorization_code`. With `ANPFUEL_APPLE_TEAM_ID`,
  `ANPFUEL_APPLE_KEY_ID`, `ANPFUEL_APPLE_PRIVATE_KEY_FILE` (the `.p8` Sign
  in with Apple key) and `ANPFUEL_APPLE_GRANT_KEY_FILE` (32 random bytes)
  configured, the server redeems it at Apple's `/auth/token` (client secret:
  ES256 JWT, `iss` team, `sub` and `client_id` the native audience, `aud`
  `https://appleid.apple.com`, 5-minute lifetime), keeps the refresh token
  only sealed with AES-256-GCM bound to the grant id, and only when the
  returned identity matches the verified subject. One grant per account and
  subject; a newer sign-in replaces it. Capture is best-effort: an
  unavailable endpoint or stale code never blocks sign-in (a redacted
  warning is logged), and without the four variables the code is ignored
  and deletion has nothing to revoke. Deletion moves the account's sealed
  grants to the revocation queue in the deletion transaction (a grant saved
  concurrently with deletion is queued, never kept); the worker job
  `account-provider-revoke` calls `/auth/revoke` and deletes each row on
  acceptance, retrying with backoff and dropping the sealed token 30 days
  after deletion at the latest. The queue keeps no account, subject or
  email.

For account deletion, the iOS client requires a deletion receipt with
`status: deleted` and `erasure_requested: true` before it confirms the request
and purges the local owner data. A transport failure keeps local data
available for a retry. The receipt acknowledges a recorded erasure request.

## Frozen parameters

Threat analysis behind each number; values change only with a new analysis,
never to make a failing test pass. Executable mirror:
`backend/internal/modules/account/domain/policy.go`.

- OTP (A02): 6 numeric digits, TTL 600s inclusive, 5 attempts then lock,
  resend cooldown 60s, max 5 issuances per address per hour. Guessing odds
  5/10⁶ per code; enumeration-safe identical responses for unknown
  addresses; codes stored as salted SHA-256 verifiers with constant-time
  compare, never plaintext or logs.
- Sessions (A04): access TTL 900s, rotating refresh families, absolute
  30-day ceiling; refresh reuse revokes the whole family.
- OIDC (A03): issuers exactly `https://accounts.google.com` and
  `https://appleid.apple.com`; exact audience match; mandatory single-use
  nonce; 120s clock skew; JWKS cache 1h; outage/rotation refuses closed,
  never a weaker fallback. Apple relay accepted; equal-email merge refused.
  Native sign-in: raw nonce 16–128 visible ASCII characters hashed with
  SHA-256 before comparison; identity token at most 4096 bytes.
- Proof boundary: social writes require active account + live session +
  fresh device-key proof together; public reads stay anonymous.
- Inventory/retention: account row, provider links, sealed Apple grants
  (dropped ≤30d after deletion), code verifiers (purged
  ≤24h after expiry), sessions (purged ≤7d after absolute expiry),
  link/recovery audit (180d). Erasure cascades to keys, social
  revisions/votes and replicas via deletion ledger (P13-T04).
- Versioning: additive only under future `/v1/accounts*` paths; existing v1
  untouched. New component schemas (`Account`, `ProviderLink`,
  `EmailCodeRequest`, `EmailCodeConsume`, `OidcLinkRequest`, `Session`) are
  preview contracts; paths land with handlers in P13-T02/T03.

## Native iOS account data export

The account export uses the active owner, session family and API origin. It gathers the available owner-scoped cloud, subscription, ledger, community, station, privacy and other private projections in one shareable JSON document. Each paginated route uses its server cursor. When a route is unavailable, incomplete or bounded before the end of the data, the export names that section and explains the limit. Fields are allowlisted and authentication credentials, proof material and precise capture data are excluded. The existing local JSON export remains available. The native share interface only receives data after the user requests the export.
