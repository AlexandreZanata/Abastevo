# Anonymous proof-of-possession signature profile (frozen, P03-T01)

Status: frozen for implementation. Algorithm `ecdsa-p256-sha512`. Changes
require a new profile version and fresh vectors; never accept both shapes
during a migration without an explicit compatibility window.

## Scope and non-goals

This profile freezes byte-level construction so independent verifiers agree.
It is inspired by RFC 9421 but is NOT a full RFC implementation claim: the
covered set below is fixed, negotiation and`Accept-Signature` discovery are
absent, and RFC 9421's own test suite is reviewed separately before G09
(ADR-007). Vectors here pin OUR profile; cross-library interop replays them.

## Keys and identifiers

- P-256 keys as JWK: `{"kty":"EC","crv":"P-256","x":"…","y":"…"}`
  (base64url, 32 bytes each). Private `d` appears only in test vectors.
- Key fingerprint (`keyid` value `fp:<hex>`): SHA-256 over the RFC 7638
  canonical thumbprint `{"crv":"P-256","kty":"EC","x":"…","y":"…"}` with
  members in that exact order and no whitespace.
- Signatures transmit base64url of raw `R || S` (64 bytes). DER is never
  accepted on the wire; a DER blob fails closed.

## Covered components (exact set, fixed order)

`"@method"`, `"@authority"`, `"@path"`, `"@query"`, then, only when a body
is present, `"content-type"` and `"content-digest"`, then `"created"`,
`"expires"`, `"keyid"`, `"nonce"`. Any other set — missing, extra, or
reordered lines, including duplicate auth fields — fails closed.

## Signature base

ASCII lines joined by single `\n`, no trailing newline:

```text
"@method": POST
"@authority": api.example.invalid
"@path": /v1/contributors
"@query":
"created": 1735689600
"expires": 1735689900
"keyid": "fp:<hex>"
"nonce": "<challenge-id>.<client-nonce>"
```

- `@method` upper-case token; `@authority` the configured canonical host
  (test vectors use `api.example.invalid`, never a real host).
- `@query` is the raw query string without `?`, empty when absent.
- `content-type` is the exact header value; `content-digest` is
  `sha-512=:base64:` over the exact transmitted bytes.
- `created`/`expires` are Unix seconds; `expires - created` is at most
  300 and `expires` must be in the future at verification.
- `nonce` binds the server challenge: `<challenge-id>.<client-nonce>`.

The base is hashed with SHA-512 and verified with ECDSA P-256 over the
digest. Clock skew tolerance is zero beyond the 5-minute window above.

## Proxy behavior

Proxies must not rewrite method, authority, path or query before
verification. `X-Forwarded-*` headers are ignored: the authority is the
configured canonical value, never derived from the connection. Deployments
behind trusted proxies terminate TLS there and forward the untouched
origin-form target. Any deviation fails closed.

## Test keys

Vectors use fixed-entropy generated keys for byte stability. They are
synthetic, carry no identity, and must never leave `contracts/testdata/`.
Production signs with `crypto/rand`; test-only determinism never ships.

## HTTP registration and rotation binding

The identity endpoints use JSON objects of at most 64 KiB. Unknown or duplicate
fields (including case aliases), trailing JSON/text and private JWK members are
rejected. Anonymous challenge/registration quotas use a daily HMAC of the socket
peer IP plus the fingerprint; untrusted forwarding headers never select an owner.
Only public `EC`/`P-256` JWK coordinates are accepted.

Registration signs the eight bodyless profile lines for the actual POST path,
query and configured authority. Its challenge/nonce binds the independently
verified public key. The envelope containing the signature cannot sign itself.
Rotation signs ten lines, including `application/json` and the digest of this
canonical intent (UTF-8, no whitespace, field order exactly as shown):

```json
{"new_jwk":{"kty":"EC","crv":"P-256","x":"<x>","y":"<y>"},"old_challenge_id":"<old-id>","new_challenge_id":"<new-id>"}
```

Both old and new proofs cover that same intent, each with its own keyid/nonce.
The server reconstructs those bytes. Substituting the new key, either challenge,
path, authority or query invalidates the proof. Challenge metadata and verification
time come from the server. Failure consumes neither challenge; successful rotation
consumes both and revokes the old key atomically. This implements B-BR-004/005
without password recovery or a tenant model.
