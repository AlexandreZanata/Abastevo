# Free account access target

Status: ADOPTED target, NOT IMPLEMENTED. Phase P13; billing remains P11 and cannot gate registration.

## B-BR-A01…A05 / BUC-A01…A04

A01: Anyone can create a FREE account by **email with an access code, Google or Apple**. No purchase, card, entitlement or paid plan prerequisite. Existing free local/offline browsing and the anonymous price-contribution baseline remain available. The new station/fuel social writes require an authenticated active account.

A02: Email codes are short-lived, single-use, hashed at rest with bounded attempts/resend and atomic consume. Responses avoid account enumeration; mail adapter has deterministic local tests, rate limits and redacted failures. Define code TTL/length and recovery in P13-T01 using threat analysis before coding; do not invent an email service or spend money in planning.

A03: Google/Apple use established OIDC flows, system browser/native supported integration and PKCE where applicable. Backend verifies issuer, audience, signature/JWKS, expiry, nonce and replay; client identity flags are untrusted. Apple relay emails are supported. Do not merge accounts solely by equal email without authenticated linking proof. Provider key rotation, revoked grants, wrong-client tokens and unavailable providers get tests.

A04: Bind account to device contributor key only with fresh account reauthentication and key proof. Recovery cannot transfer another contributor's observations/reputation. Sessions are short-lived with rotating/revocable refresh families; logout, lost device, email/provider change and deletion invalidate appropriate access. Native secure storage stays behind ports. Public alias and private provider identity are separate; no tenant model.

A05: FREE access includes registration and social participation. Future paid entitlements buy explicitly validated hosted benefits and never trust. Server roles control moderation; device/app signing proof remains required by the applicable write contract. Export/erasure covers account subjects, linked keys, social revisions/votes and replicas/deletion ledger without leaking identity.

BUC-A01 signup/login, A02 link/unlink and recovery, A03 logout/revoke, A04 rights operations. Freeze additive contracts and a data inventory before implementation. Tests first: code replay/expiry/concurrent consume, token substitution, cross-account linking, stolen IDs, refresh reuse, suspension, provider outage and erasure/restore. Local provider stubs prove protocol behavior; real Google/Apple sandbox and native callback/device evidence remain phase acceptance, without production deployment.
