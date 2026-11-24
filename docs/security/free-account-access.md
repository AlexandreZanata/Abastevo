# Free account access

## B-BR-A01…A05 / BUC-A01…A04

A01: Android users can sign in with Google or with an account key. To create a FREE key account, enter a public name and verify a recovery email with a six-digit code. Existing username-only accounts and native Apple sign-in remain supported. No purchase, card, entitlement or paid plan prerequisite. Existing free local/offline browsing and the anonymous price-contribution baseline remain available. The new station/fuel social writes require an authenticated active account.

A02: Email codes are short-lived, single-use, hashed at rest with bounded attempts/resend and atomic consume. Responses avoid account enumeration; mail adapter has deterministic local tests, rate limits and redacted failures. Codes verify control of an address for sign-up, key recovery or adding/changing a recovery email. The optional purpose and locale select only the message text. The code alone never issues a session. Issuance returns the same accepted response regardless of address ownership, throttling or delivery failure.

A03: Google/Apple use established OIDC flows, system browser/native supported integration and PKCE where applicable. Backend verifies issuer, audience, signature/JWKS, expiry, nonce and replay; client identity flags are untrusted. Apple relay emails are supported. Do not merge accounts solely by equal email without authenticated linking proof. Provider key rotation, revoked grants, wrong-client tokens and unavailable providers get tests.

A04: Bind account to device contributor key only with fresh account reauthentication and key proof. Recovery cannot transfer another contributor's observations/reputation. Sessions are short-lived with rotating/revocable refresh families; logout, lost device, email/provider change and deletion invalidate appropriate access. Native secure storage stays behind ports. An explicitly confirmed public name replaces the alias in public author projections. The recovery address, its hint and provider identity are private. No tenant model.

A05: FREE access includes registration and social participation. Future paid entitlements buy explicitly validated hosted benefits and never trust. Server roles control moderation; device/app signing proof remains required by the applicable write contract. Export/erasure covers account subjects, linked keys, social revisions/votes and replicas/deletion ledger without leaking identity.

BUC-A01 signup/login, A02 link/unlink and recovery, A03 logout/revoke, A04 rights operations. Freeze additive contracts and a data inventory before implementation. Tests first: code replay/expiry/concurrent consume, token substitution, cross-account linking, stolen IDs, refresh reuse, suspension, provider outage and erasure/restore. Local provider stubs prove protocol behavior; real Google/Apple sandbox and native callback/device evidence remain phase acceptance, without production deployment.

## Public name and account key

- Sign-up validates the name and email before storage, consumes the code once,
  and under the existing key-account admission ceiling atomically creates
  an account with a random alias, the display name, the verified address
  and the key credential. It then issues a session and returns the new key
  once. Parallel sign-ups with distinct valid codes converge on one address
  owner. After proof of address control, a sign-up for an already bound
  address consumes its code and returns `account.address-linked`. A code
  already used returns `account.code-consumed`. Legacy username sign-up remains
  available for older clients. The passwordless email-consume route returns
  410 `account.email-login-retired`.
- Normalize public names to NFC, trim and collapse inner whitespace. Allow
  2–40 runes, with Unicode letters and marks, spaces, apostrophes, hyphens,
  dots and digits. Require at least two letters and allow at most four
  digits. Reject control and format characters, `@`, `/`, `:`, and the
  case-insensitive reserved names abastevo, admin, administrador, moderador,
  moderacao, moderação, suporte, anp and oficial. These bounds reduce
  impersonation and accidental publication of contact details. The chosen
  name appears on comments and as the default editable author name on forum
  topics. Public reads fall back to the alias for older accounts and keep
  their existing deletion and suspension behavior.
- For key credentials without usernames, keep the required unique username
  hash and store SHA-256 hex of `account:<account_id>`. The colon and UUID
  put this in a separate input space from a valid legacy username. Keeping
  the column non-null lets the previous image read credentials after a
  rollback. Schema changes are additive.
- The private profile gives the public name, completeness, key and linked
  provider state, and only a masked recovery address hint. A legacy account
  can dismiss the prompt to set a name and recovery address. Google users
  confirm their public name in the client. The server never receives a
  Google display name as part of the provider assertion.

## Key recovery and recovery address

- Recovery verifies control of the currently bound email, consumes the code
  and atomically replaces the key credential (or creates it if absent),
  revokes the previous account session families, and writes a recovery audit
  entry. It issues a new session and returns the new key once. The old key
  can no longer sign in. A valid code for an unbound address returns
  `account.recovery-no-account`. Suspended accounts are refused before code
  consumption. Deleted accounts have no bound address or public display name.
- An active session can change the public name. Adding or replacing the
  single recovery address requires a login family at most 600 seconds old
  and a valid email code. A bound address owned by another account cannot
  be attached. A replacement unbinds the old address and audits the change.
  Refresh does not restart the freshness clock. The 600-second threshold
  caps the window in which a stolen access token can establish a new
  recovery method, below the 900-second access token lifetime.
- Email is normalized and hashed for address lookup. Persist only the hash
  and the hint (the first character of the local part, `***@`, and the
  domain). Never include the address or hint in a public response. Audit
  rows contain only an ID, account ID, action and timestamp, and expire at
  180 days. Delete the stored name and recovery address in the account
  deletion transaction.

## Native Google sign-in and link

- Require the configured Google Web client ID as the native audience.
  Unconfigured native login and linking return `account.provider-unavailable`.
  If authorized Android parties are configured, require the token's `azp`
  to match one of them. Native tokens use the same raw-nonce proof and
  single-use ledger as Apple's native flow. Existing provider linking keeps
  its separate audience and nonce contract.
- Accept Google's two documented issuer spellings, `https://accounts.google.com`
  and `accounts.google.com`, and canonicalize to the HTTPS form before
  subject lookup. This admits both legitimate token forms without
  separating their subject namespaces or trusting any other issuer.
- Identify a Google account by issuer and subject. On first sign-in only,
  bind a recovery address in the same transaction if the email is
  authoritative: a verified Gmail or Googlemail address, or a Workspace
  address whose domain equals the `hd` claim. Accept boolean true and the
  string `"true"` for Google's email verification claim. Treat an
  unverified email as ineligible. If the authoritative address is already
  bound, refuse with `account.provider-email-in-use` and create nothing.
  Third-party email claims do not establish a recovery channel. No provider
  signs in to a different account by a matching email string.
- Native Google linking requires a login family at most 600 seconds old.
  It is idempotent for the already linked account and refuses a subject
  owned by another account. A captured older access token cannot establish
  a new provider link.

## Code delivery

- SMTP supports implicit TLS and mandatory STARTTLS, with TLS 1.2 or later,
  certificate verification, PLAIN authentication only after TLS and a
  10-second context-bounded exchange. Accept exactly one recipient and
  reject control characters in addresses before I/O. Messages are plain
  text, with the purpose and ten-minute validity, no links and no code in
  the subject. The default language is pt-BR and English is available.
- SMTP failures are redacted and compensate the issuance so it does not
  consume the recipient's retry budget. Logs never include names,
  addresses, codes, credentials, account keys, provider tokens or nonces.
- The global mail budget defaults to 200 codes per hour, configurable from
  20 to 2,000 via `ANPFUEL_MAIL_MAX_CODES_PER_HOUR`. The former 20/hour
  ceiling would suppress legitimate sign-ups once SMTP is live. Operators
  choose a ceiling within the provider's delivery quota to bound cost and
  outbound abuse. Per-address limits, code entropy and expiry are unchanged.

The iOS client opens a session only with credentials issued by the account API.
The provider-link endpoint requires an existing live account session and does
not issue one. Older iOS previews' locally generated Apple sessions are refused.

Native Sign in with Apple (`POST /v1/accounts/providers/login`, P13-T05) is
an established provider flow that issues a session:

- The app generates a random raw nonce, puts its lowercase hex SHA-256 in the
  `ASAuthorizationAppleIDRequest` nonce (Apple copies it verbatim into the
  identity token's `nonce` claim) and sends the identity token plus the RAW
  nonce. The server hashes the raw nonce before comparing, so a captured
  token alone never signs in, and consumes the claim once in the shared
  nonce ledger (only the hash is stored).
- The expected audience is `ANPFUEL_APPLE_NATIVE_AUDIENCE` (the bundle ID),
  never a request field and never the link audience `anpfuel-backend`, so
  link tokens and sign-in tokens are not interchangeable. Unset answers 503
  `account.provider-unavailable`.
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
- OIDC (A03): Google's two canonicalized issuer spellings and Apple issuer
  exactly `https://appleid.apple.com`; exact audience match; mandatory single-use
  nonce; 120s clock skew; JWKS cache 1h; outage/rotation refuses closed,
  never a weaker fallback. Apple relay accepted; equal-email merge refused.
  Native sign-in: raw nonce 16–128 visible ASCII characters hashed with
  SHA-256 before comparison; identity token at most 4096 bytes.
- Proof boundary: social writes require active account + live session +
  fresh device-key proof together; public reads stay anonymous.
- Inventory/retention: account row, provider links, sealed Apple grants
  (dropped ≤30d after deletion), code verifiers (purged
  ≤24h after expiry), sessions (purged ≤7d after absolute expiry),
  public display name and masked verified address hint (cleared on
  deletion), recovery audit (180d). SMTP processes the address and code
  for delivery only. Provider-side delivery retention depends on the SMTP
  operator. Erasure cascades to keys, social
  revisions/votes and replicas via deletion ledger (P13-T04).
- Versioning: new account routes and fields are additive. The old email
  login route has an explicit retirement verdict. Legacy key sign-up and
  existing provider linking remain compatible.
