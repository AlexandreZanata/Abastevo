# Security and privacy plan

Status: engineering proposal, not a declaration of legal compliance. Review lawful bases, notices, processor contracts, international transfers and data-subject handling with the responsible operator before a pilot. The [LGPD text](https://www.planalto.gov.br/ccivil_03/_ato2015-2018/2018/lei/l13709.htm) is the legal reference; anonymous installation keys and photographs can still constitute personal data.

## Trust boundaries and STRIDE

- **Spoofing:** forged contributor/key, fake GPS and Sybil identities. Controls: proof of possession, single-use server challenges, key revocation, aggregate quotas, independent evidence and conservative trust. Test replay/concurrent nonce consumption and key substitution. Residual risk: one person can create multiple installations; signatures authenticate a key, not a human or a real-world price.
- **Tampering:** altered amount/condition/body, overwritten upload and changed ANP file. Controls: signed covered components/body digest, append-only business facts, checksummed official revisions, server-verified frozen evidence. Test changed body/method/path/authority, object overwrite races and migration checksums.
- **Repudiation:** contributor/admin denies an action. Controls: request ID, verified key reference, timestamp and minimal append-only decision log. No promise of legal nonrepudiation. Test atomic fact/audit commit and restore.
- **Information disclosure:** photo/EXIF/location/IP leaks, guessed evidence IDs and signed URLs in logs. Controls: private bucket, short authorized downloads, object ownership, redacted logs, separated personal payloads, retention and least privilege. Test IDOR, log redaction, response schemas, object ACL and cache bypass.
- **Denial of service:** photo floods, oversized JSON/ZIP, unbounded geo query, nonce/registration spam, poison jobs. Controls: edge and DB quotas, limits/timeouts, bounded decoding, radius/pagination, work leases/retry caps, disk alerts. Test slow bodies, decompression bombs, concurrent quotas and dead-letter recovery.
- **Elevation:** anonymous caller uses admin path or production DB role performs DDL. Controls: admin CLI via restricted operator access with MFA on access layer, no public admin routes, separate roles/credentials, mandatory action reason/audit. Test denied roles and no-bypass operator commands.

## Identity decisions

Use reviewed P-256/SHA-256 proof and RFC 9421 profile from API_PLAN. Private keys stay on-device. [Android Keystore documentation](https://developer.android.com/privacy-and-security/keystore) supports non-exportable keys and documents hardware-dependent capabilities; hardware support must be detected rather than assumed. Ed25519 support across all existing devices is not presumed. No custom cryptographic primitive, mandatory hardware attestation or traditional account in MVP.

Server stores public keys, hashed expiring nonces, key/contributor state and operation idempotency. Revoke old keys on a successful rotation with proofs from both keys. Lost device without linked account means lost anonymous identity; never promise recovery from a public key alone. An optional later account can bind a new key after strong reauthentication, without linking or stealing someone else's reputation.

## Evidence protocol and race safety

Initial policy: JPEG only, maximum 3 MiB and 12 megapixels, one still image per observation; reject invalid dimensions, animated/polyglot/unsupported formats and decoder resource exhaustion. Client crop/compression is helpful but untrusted. Validate magic bytes plus full bounded decode, strip EXIF and re-encode to a server-owned sanitized object. Compute SHA-256 and perceptual hash server-side; metadata claims are never sufficient.

The [R2 presigned URL API](https://developers.cloudflare.com/r2/api/s3/presigned-urls/) allows direct object uploads. A signed Content-Type does not prove file contents, and presigned PUT must not be assumed to enforce a size range. Reserve per-contributor quota before URL issuance; verify actual length/checksum afterward and delete oversize objects. Test storage-side supported constraints; residual bandwidth abuse is capped by issuance quotas and budget alerts.

Client uploads to a random quarantine key. A presigned URL may remain reusable until expiry: a HEAD followed by a later copy could validate one object then publish another. Therefore the worker downloads a bounded snapshot, validates/hashes those exact bytes, and uploads sanitized bytes to a **different final key never writable through client credentials/URLs**. The DB points to the verified final object only after upload success. If DB commit fails, orphan cleanup removes the unreferenced final object. No public access, no arbitrary URL ingestion, no uploads through the API VPS request path. Worker processing traffic is bounded and asynchronous.

ISSUED→VERIFYING after completion intent; VERIFYING→READY after frozen validation; invalid format→REJECTED; unused session after 24 h→EXPIRED. Repeated complete is idempotent; pending observation waits for READY or the 24 h evidence deadline. Orphan sweeper uses DB references and a grace period longer than active processing lease. Download authorization is owner-independent only for privileged moderation, expires in 60 s and is never logged.

## Proposed data inventory and retention

Retention values below are initial minimization choices requiring pilot review; not statutory periods. Delete both DB payload and object where relevant. Infrastructure providers' own logs must be configured consistently.

- **Contributor ID/public keys:** authentication and abuse control; Identity tables; restricted API/trust operators; active lifetime then rights-request processing. No public contributor browsing. Export ownership history after proof; revoke/delete or unlink on erasure as legally appropriate.
- **Exact GPS/fix data:** short-lived proximity derivation; encrypted private payload only, worker access; delete immediately after successful validation, hard cap 24 h. Store distance band/accuracy band and quality outcome for at most 90 days, not coordinates. No location-history endpoint. Keyed hashes do not automatically anonymize GPS.
- **Quarantine original photo:** decoding/verification only; delete after sanitization, hard cap 24 h. **Sanitized evidence:** private review, delete at 14 days. A substantiated open case can extend to 30 days with reason and deadline; no indefinite hold by default. Respect export/deletion requests as applicable.
- **Verified/perceptual hashes and derived fraud signals:** duplicate detection, 90 days; restricted trust role; considered potentially linkable. Do not expose hashes publicly. Purge on expiry/appropriate erasure.
- **Raw OCR text:** optional parsing audit, 24 h alongside private payload. Retain only selected fuel/price/condition and minimal confidence category in the long-lived fact.
- **Network metadata:** edge IP for immediate limiting, configure provider retention; application IP digest with rotating secret ≤24 h. No precise coordinates/query strings in access logs. Security investigation logs ≤7 days unless a specifically recorded case justifies a limited extension.
- **Observation core and public price projection:** product history; initial observation retention 24 months, with anonymization/unlinking review on erasure and aggregation afterward. “Immutable” means no business-history rewriting; it does not authorize indefinite personal retention. Official ANP history is a separate public-source dataset subject to its terms.
- **Moderation decisions:** accountability, 12 months initially; minimal actor/action/reason, no copied raw media/GPS. Operator access audited. Keep only necessary restricted deletion ledger data through backup expiry.
- **Operational logs:** 14 days, trace ID/status/latency/error class, no sensitive bodies/tokens/signed URLs; aggregate service/product metrics 12 months without individual labels.
- **Encrypted backups:** 7 daily plus 4 weekly snapshots, maximum 35 days; exclude exact-GPS/raw-OCR transient table data, challenges and IP rate windows so their short retention is not defeated by backup copies. No independent backup of expiring evidence. On restore, reapply deletion ledger and expiry jobs BEFORE reopening traffic. Pending validation whose transient evidence is unavailable must reject/request a new submission, never invent successful validation. A documented deletion request for backed-up persistent data may take until backup expiry to age out of protected immutable backups; notices must explain this accurately.
- **Account, email, billing, sync and analytics:** not collected in MVP. Separate inventory/basis/retention review required before implementing P11.

Rights workflow: prove current key → export/deletion request → record request receipt → export via restricted expiring download or revoke writes/process erasure → receipt/result → apply deletions after any restore. Regulatory response deadlines are set by legal review before pilot, not invented by engineering. Losing the key requires an alternate evidence-based support process; do not disclose data from unverifiable identity claims.

## Deployment and operational controls

TLS at edge and origin; strict origin authentication/firewall, trusted proxy IP allowlist and exact forwarded-header policy. Default deny DB ports; no public PostgreSQL, metrics, Docker socket or admin endpoint. API DB role cannot DDL; worker cannot bypass observation history policy; migrator credentials exist only during deploy. S3 credentials scoped to bucket/prefix and separated for processing/deletion/backups where supported.

Config validates once at startup: environment, DSN, permitted origins, R2 endpoint/bucket, upload limits, worker concurrency, retention and policy versions. `.env.example` has names/descriptions only, no plausible production credentials; current `.gitignore` hides all `.env.*`, so add a narrow allowlist when the example is introduced. Secret rotation and compromised-contributor-key procedures need tests. Do not print secrets even during startup failure.

Use parametrized SQL, context deadlines, server read-header/body/write/idle timeouts, request IDs and response allowlists. OWASP-style checks cover authz/IDOR/replay/upload/SSRF; dependency/secret scanning covers changed documentation and fixtures too. Open source does not imply open production data.

Before G09: no unresolved critical/high security findings, demonstrable rights/retention jobs, restricted admin with audit, off-host restore drill, redaction tests, frozen auth vectors and review of platform policy/LGPD notices. This is a concrete launch gate, not an assertion that planning alone makes the product compliant.
