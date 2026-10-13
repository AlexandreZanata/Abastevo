# Security audit — 2026-10-11

Status: AUDIT_LOCAL_DONE / REMEDIATION_NOT_STARTED. Source review and synthetic local verification, not release certification or a production penetration test.

Audited commit: `954a8d800576fec24acc9edfd3b93aad2ac15450` on maintained dev. The checkout was occupied by concurrent CS02 work: execution used an immutable `git archive` snapshot. The opening untracked migration 000054 was recorded provisionally and was not applied. At report closure, parallel construction had reached `d04c96f0a910416a2da4e21b645453cb3cf37d8f` (CS07A), with further untracked Android helpfulness work. Those later CS02–CS07A commits and unfinished edits are outside this tested snapshot; SEC00/SEC09 require affected current-head review before delivery. No VPS, connected phone, production photos, deployed secrets, GitHub publication or wiki operation occurred.

## Executive decision

There are corrections to make before broader public exposure. This audit records **3 high and 9 medium findings/risks, and no demonstrated critical vulnerability**. Medium items include explicitly conditional availability and trust-boundary risks; the count is not a count of successful production exploits. No confirmed remote code execution, SQL injection or account takeover was established. This does not establish their absence.

The immediate priorities are uncontrolled account/comment growth, upload authorization without an enforced byte ceiling, and identity quotas shared behind the supplied proxy. Next, close account-erasure and photo-retention gaps, then harden synchronization, session handling and location trust. The [remediation plan](../planning/SECURITY_REMEDIATION_PLAN_2026-10-11.md) defines bounded tasks and acceptance evidence. This document authorizes no deployment or feature shutdown by itself.

## Scope and evidence

Inventory at the pinned commit: 2,515 tracked files, including 312 production/generated Go files, 268 Go test files, 533 production Kotlin files, 15 Rust source files and 53 SQL migrations. Inventory is not a claim of exhaustive line-by-line inspection. Repository-wide boundary searches were followed by deep review of composition roots, identity/account proof and sessions, community/feedback writes, media reservation/verification/retention, privacy erasure, private ownership, pagination/SQL, jobs, official ingestion, Android networking/cache/outbox and source downloads. Station-profile/moderation boundaries, landing scripts, manifests, Caddy and Compose were also examined. Historical iOS remains deferred.

Verification completed:

- All backend packages: `go test -race -p 2 -count=1 ./...`; **67 tested packages passed**, other generated/adapter packages reported no untagged tests. This snapshot also contained the first six audit characterization probes. No integration acceptance is inferred from an untagged package with no tests.
- Real disposable PostGIS 18/3.6: eight adapter packages passed with `-race -tags=integration` — account, identity, evidence, feedback, community, privacy, moderation and stationprofile. Only a uniquely named local tmpfs Compose project was created, then removed. Full HTTP API/provider integration was not run.
- Eight custom Go unit characterization cases and two custom real-SQL cases reproduced the behaviors described below. Two JVM photo checks and a bounded JVM XLSX case exercised unchanged production Kotlin sources. The JVM harness substituted only the Android key-provider interface, not cache/crypto behavior; it did not validate Android Keystore hardware or a release APK.
- `govulncheck v1.8.0`, symbol scan with Go 1.27.2: exit 0, **0 finding events**; public database last modified `2026-10-08T22:31:09Z`. The 201 advisory records in its stream are metadata, not 201 findings.
- RustSec `cargo audit`: exit 0, 25 lockfile dependencies, **0 reported vulnerabilities/warnings**; advisory DB commit `7eebec69c352c7191b1f13eb95dd510eeca5d1de`, updated 2026-10-09.
- Existing static security gate and tracked secret scan passed. These narrow gates did not detect the business-logic findings below.

[Sanitized evidence and limitations](evidence/audit-2026-10-11/summary.json), [reproduction instructions](evidence/audit-2026-10-11/README.md). Passing characterization probes means a vulnerable/risky behavior was reproduced; it does **not** mean the behavior is safe. No fixes were applied.

Initial sandbox-only attempts failed because the default Go cache was read-only and local sockets/DNS were restricted. The cache was moved to `/tmp`; approved local-socket/public-advisory reruns completed. Failed attempts are not counted as passes.

## High findings

### AUD-01 — Missing aggregate admission limits allow durable and in-memory growth

Severity: HIGH. Confidence: high for source behavior; production exposure/configuration not inspected. CWE-400 / OWASP API4.

Sources: [account routes](../../backend/internal/modules/account/adapters/http/http.go), [key account creation](../../backend/internal/modules/account/application/keyaccounts.go) at line 23, [email issuance](../../backend/internal/modules/account/application/accounts.go) at line 85, [API composition](../../backend/cmd/api/main.go) at lines 278–328, [memory mail outbox](../../backend/internal/modules/account/adapters/mail/memory.go), [comment submission](../../backend/internal/modules/feedback/application/comments.go) at line 62.

Anonymous callers can create distinct username/key accounts without an aggregate IP/device/global admission budget. Email issuance has per-address checks, but distinct addresses bypass them. The real API always wires the preview `mail.Outbox`, whose `sent` slice retains address/code pairs for the process lifetime without TTL/cardinality/byte limits; it does not send emails. A free active account can submit arbitrary numbers of separate comments/replies; the report quota does not cover ordinary comments. Repeated key logins also create additional session families without an aggregate budget.

Proof: 101 distinct accounts, 100 session creations and 100 distinct-address issuances were accepted in memory; real PostGIS accepted 100 durable comments for one account without a quota rejection. The route/composition review found no compensating limiter in these paths or the supplied Caddy file. This is bounded proof of missing admission, not an OOM/stress run or a measured attack rate.

Impact: a low-cost client can grow rows, indexes, revisions and the preview mailbox, consuming memory, disk and DB connections. Container resource ceilings can limit host impact but cannot prevent the Abastevo process failing within its own ceiling. No takeover or actual email spam was demonstrated.

Correction: disable unavailable preview email delivery in deployed configurations or replace it with an approved bounded adapter; atomically enforce layered per-operation and aggregate budgets before allocations; limit sessions and durable storage growth. Keep free registration and anonymous reads available. Device identity alone is resettable and is not a Sybil defense.

### AUD-02 — Direct photo PUT authorization does not enforce its advertised byte ceiling

Severity: HIGH for deployments accepting direct storage PUTs without a separate hard limit. Confidence: high for signing defect; actual provider/edge enforcement unverified. CWE-400.

Sources: [presigner](../../backend/internal/modules/evidence/adapters/storage/presign.go) at lines 68–104 and 185–202; [API presign wiring](../../backend/cmd/api/main.go) at lines 743–761; [bounded worker GET](../../backend/internal/modules/evidence/adapters/storage/transfer.go) at line 32.

`MaxBytes` is validated and returned as metadata. The PUT signature covers `content-type;host`, uses `UNSIGNED-PAYLOAD`, and binds neither length nor a provider-enforced range. The same input with a one-byte ceiling and a 3 MiB ceiling produces the **same URL**. The existing `TestPresignDoesNotEnforceSize` also explicitly recognizes this limitation. A malicious contributor can use its own authorization directly, ignoring the Android compression cap and later worker validation. Presigned URLs can be replayed during their validity window.

Impact: oversized transfers consume storage/network resources before the worker rejects them; a reservation counter does not bound bytes or repeated PUT traffic. The API's body cap helps uploads that actually pass through its staging proxy, but does not cap direct R2/S3 PUTs. No real provider upload or production storage abuse was attempted.

Correction: enforce the chosen exact length/ceiling before or during acceptance at the actual upload boundary, plus atomic byte reservations and aggregate limits. Evaluate a signed exact-length PUT only with provider/transport proof, or an existing bounded application/edge streaming path. Do not prescribe S3 POST policies for R2 without support verification; R2 documents POST form uploads as unsupported. Keep worker magic/dimension/hash/byte validation as a second layer.

### AUD-03 — Proxy peer identity can exhaust the identity quota for every user

Severity: HIGH for the supplied reverse-proxy topology. Confidence: high for configuration/source interaction; live topology unverified. CWE-400.

Sources: [identity quota HTTP handler](../../backend/internal/modules/identity/adapters/http/http.go) at line 120, [quota defaults](../../backend/internal/modules/identity/domain/quota.go) at line 43, [Caddy reverse proxy](../../infra/caddy/Caddyfile), [API router composition](../../backend/cmd/api/main.go).

The handler hashes `r.RemoteAddr`, not the client identity forwarded by the trusted edge. The supplied Caddy proxies through its own TCP connection; there is no API middleware resolving a verified origin IP. All clients therefore use the same peer-IP quota in that topology: 60 challenges/hour and 10 registrations/hour. Every signed sync step needing a fresh challenge depends on this path. A single client, or ordinary concurrent activity, can exhaust the shared bucket.

Proof: 61 distinct forwarded client IPs and distinct fingerprints behind one proxy peer produced a 429 for the last caller. The test registrar intentionally returned unavailable after admission; even failed work consumes the shared budget. No live endpoint was flooded.

Correction: resolve forwarding information only from an explicitly trusted immediate proxy with a defined header-chain policy; verify direct requests cannot spoof it. Separate client/NAT fairness budgets from an intentionally sized global circuit breaker and per-key limits. Do not blindly trust `X-Forwarded-For`, or simply remove global protections.

## Medium findings and conditional risks

### AUD-04 — Daily photo reservation admission is not atomic

Severity: MEDIUM. Confidence: reproduced with real SQL. CWE-362.

Sources: [Reserve](../../backend/internal/modules/evidence/application/reserve.go) at lines 137–169; [ReserveSession](../../backend/internal/modules/evidence/adapters/store.go) at line 95.

`CountSince` is separate from the insertion transaction. With nine existing reservations, twelve concurrent calls can each see nine and all insert. **21 distinct durable reservations were accepted for a configured budget of 10**, using the real PostGIS store and unique keys. The hourly write gate was stubbed in this narrowly scoped reproduction; 21 writes are still below its configured 200/hour allowance. This is independent of AUD-02's byte-limit defect.

Correction: make deduplication, contributor/day budget and row insertion one atomic operation; identical retries must replay their existing reservation without consuming another quota unit, including when the budget is full. Test 10th/11th admission, concurrent fresh requests, replay, rollback and UTC rollover.

### AUD-05 — App account deletion is disconnected from cross-module erasure

Severity: MEDIUM. Confidence: reproduced with real SQL. CWE-459 / lifecycle privacy defect.

Sources: [DeleteAccount](../../backend/internal/modules/account/application/status.go) at line 50, [SQL account deletion](../../backend/internal/modules/account/adapters/pgstore.go) at line 664, [explicit feedback erasure](../../backend/internal/modules/feedback/application/footprint.go) at line 189, [privacy orchestration](../../backend/internal/modules/privacy/application/erase.go), [public comment joins](../../backend/db/queries/feedback/comments.sql), [app deletion call](../../data/src/main/kotlin/com/anpfuel/data/local/auth/AccountHttpApi.kt) at line 140. Contract: [A05](FREE_ACCOUNT_ACCESS.md) and the [shipped account-deletion copy](../../landing/static/excluir-conta/index.html).

The app deletion route sets account status, revokes sessions and deletes address/provider/contributor bindings, but does not call/enqueue the feedback and contributor erasure workflows. Real PostGIS still resolved comment text and the author's public alias after account deletion. Account aliases/key credentials/binding audit are not automatically minimized by this method. It also removes the contributor binding without revoking the independent identity key through the privacy workflow.

The existing `TestBindingBlockedMatrix` intentionally treats a deleted, unbound contributor as the anonymous baseline. That baseline is **not** reported as an authentication bypass; the defect is the unfulfilled aggregate erasure contract and lost orchestration link. Operator erasure functions exist, but their existence does not prove the user route executes them.

Correction: durably capture all linked owner scopes before unlinking; revoke relevant authorities, enqueue resumable erasure with a receipt/deletion ledger, minimize data according to recorded retention exceptions, rebuild aggregates, and purge/disable local private queue/media as required. Test the actual user route through worker execution, failure/retry and restore replay. Do not promise complete physical deletion on initial acknowledgement.

### AUD-06 — Retention health omits overdue originals and is scheduled after the deadline

Severity: MEDIUM. Confidence: source plus injected-failure reproduction. CWE-459.

Sources: [overdue audit and sweep](../../backend/internal/modules/evidence/application/sweep.go) at lines 101 and 125, [SQL inventory](../../backend/db/queries/evidence/evidence.sql) at lines 95–122, [intake breaker](../../backend/cmd/api/main.go) at line 710, [sweep scheduling](../../backend/cmd/worker/main.go) at line 776.

The intake breaker and `AuditOverdue` inspect only final objects. An overdue quarantine original can remain after a delete failure while this health audit still reports zero. The sweep reports storage errors but succeeds as a job, so ordinary job retry does not immediately retry that deletion. The hourly sweep selects already expired objects in batches of 100; without independently proven deadline deletion, an object expiring just after a sweep waits until the next run, and backlog can extend the delay.

Proof: a 48-hour original plus simulated storage deletion failure produced `StorageErrors=1`, retained bytes and a healthy zero from the final-only audit. Important negative finding: the real SQL `QuarantineCandidates` includes all old sessions; the live-job guard by itself does **not** indefinitely protect an original from the later cleanup pass. An earlier fake-only hypothesis to that effect was discarded after tracing the SQL.

Correction: inventory/health every original/final/provider copy and reconcile storage errors; make overdue originals trip admission and alert without hiding successes. Schedule/purge with a measured safety margin and bounded catch-up throughput, and prove every-copy deletion or explicitly report a policy failure. Access expiry and a green worker job are not proof of physical deletion within 24 hours.

### AUD-07 — Android transient-cache cleanup misses orphan temporary files and clock rollback

Severity: MEDIUM. Confidence: unchanged source reproduced on JVM. CWE-459.

Sources: [AndroidPhotoCache](../../data/src/main/kotlin/com/anpfuel/data/local/media/AndroidPhotoCache.kt) at lines 33 and 75–94; [expiry predicate](../../domain/src/main/kotlin/com/anpfuel/domain/portable/PortablePhoto.kt) at line 98.

Interrupted atomic writes can leave sealed `photo_*.tmp` files. The sweep excludes every `.tmp`, so they can survive indefinitely; `delete(id)` also addresses only the final filename. An orphan survived a sweep at capture +72h. The sealed-cache expiry predicate only computes `now - capture >= 24h`, so a rollback before capture is accepted as live; the JVM read returned bytes under that condition. The raw `PrivateCaptureFiles` adapter does reject future timestamps, which does not fix the separate sealed cache.

Correction: recover/purge abandoned writes safely, include all private-copy paths in deletion, and implement versioned clock/expiry policy using monotonic elapsed time where available plus reboot/rollback failure handling. Test crash-before-rename, concurrent writer/sweep, missing key, rollback/reboot and app resume. A powered-off phone cannot execute physical deletion until it runs again; retain the documented limitation. No plaintext photo disclosure was established: the affected blobs are encrypted.

### AUD-08 — A random refresh token revokes a known session family

Severity: MEDIUM, conditional targeted logout. Confidence: reproduced. CWE-287 / authentication failure handling.

Source: [Refresh](../../backend/internal/modules/account/application/accounts.go) at lines 211–247.

Any hash mismatch revokes the family as reuse, without establishing that the supplied token was an authentic previously issued refresh token. A random never-issued token plus a valid family ID revoked the owner's valid access token in the probe.

Precondition: the attacker must learn the victim's high-entropy family ID. This audit found no public disclosure of that ID and does not claim it is guessable, an account takeover, or an unauthenticated logout of arbitrary users. Distinguish this precondition when prioritizing.

Correction: distinguish an invalid credential from proven reuse using a bounded, hashed rotation history or another reviewed proof mechanism. Preserve fail-closed rejection and real replay detection. Test unknown family, random token, valid superseded token, simultaneous refresh, lost response and client serialization/recovery; do not silently weaken reuse defenses.

### AUD-09 — JWKS cache freshness is shared across providers

Severity: MEDIUM, conditional delayed key retirement. Confidence: reproduced. CWE-613 / cache validity error.

Source: [CachedKeys](../../backend/internal/modules/account/adapters/oidc/jwks.go) at lines 17–45.

Cached sets are keyed by issuer but one `at` timestamp covers all issuers. Fetching Apple can extend the apparent freshness of Google's old keyset. The probe served Google's 70-minute-old set under a 60-minute TTL after an Apple refresh. Repeating another provider's refresh at expiry can keep the older provider's set stale substantially longer.

Impact: delayed provider key removal/rotation; accepting a forged token additionally requires possession of a corresponding signing key. The audit does not demonstrate that compromise. Signature, issuer, audience, expiry and nonce checks otherwise passed their tests.

Correction: maintain independent per-issuer expiry and controlled refresh on unknown key IDs. Add alternating-provider and removed-key cases. Separately harden `HTTPKeys`: its comment says redirects are refused, but the default Go client follows them; constrain redirects/HTTPS and test that contract without presenting a hypothetical provider redirect as a demonstrated takeover.

### AUD-10 — Android official-data download/XLSX parsing lacks resource ceilings

Severity: MEDIUM, malformed/untrusted-source prerequisite. Confidence: concrete parser exhaustion reproduced. CWE-400 / CWE-770.

Sources: [downloader](../../data/src/main/kotlin/com/anpfuel/data/remote/AnpFileDownloader.kt) at lines 36–68, [listing reads](../../data/src/main/kotlin/com/anpfuel/data/remote/AnpListingScraper.kt) at line 85, [URL classifier](../../data/src/main/kotlin/com/anpfuel/data/remote/AnpPriceTableUrlParser.kt), [XLSX reader](../../data/src/main/kotlin/com/anpfuel/data/parser/StreamingXlsxParser.kt) at lines 99–116 and 177–230, [cell reference](../../data/src/main/kotlin/com/anpfuel/data/parser/XlsxCellReference.kt) at line 5.

Downloads use unbounded `copyTo`; the listing buffers the whole response. XLSX shared strings and expanded text have no explicit byte/count bound. A row allocates a dense list up to the highest declared column without enforcing Excel's valid range or checked arithmetic. URL validation recognizes a filename/path pattern rather than an approved HTTPS origin; default client redirects do not add an official-origin allowlist.

Proof: a **748-byte synthetic XLSX**, containing one `ZZZZZZ1` cell, caused `OutOfMemoryError` in an isolated **32 MiB JVM** executing the production parser. No OOM was induced in the phone, backend or VPS. This establishes an input-amplification defect, not a network route by which an arbitrary third party can currently force the official site to serve that input. An altered upstream/listing, unexpected file or another future import route supplies that prerequisite.

Correction: enforce origins and redirect policy, compressed/download/inflated-entry/total bytes, worksheet/cell/string counts and text limits; reject invalid/overflowing coordinates before allocation; close ZIP handles on open failure; clean partial downloads atomically. Test honest large ANP samples as well as small adversarial fixtures; do not just increase heap or catch OOM.

### AUD-11 — Contribution dispatch materializes the whole local history per command

Severity: MEDIUM, local availability/abuse risk. Confidence: source/complexity evidence; no production-scale benchmark or remote exploitation demonstrated. CWE-400.

Sources: [outbox DAO](../../data/src/main/kotlin/com/anpfuel/data/local/dao/ContributionOutboxDao.kt), [repository dispatch listing](../../data/src/main/kotlin/com/anpfuel/data/repository/RoomContributionOutboxRepository.kt) at lines 101–112 and 138–158, [dispatcher](../../application/src/main/kotlin/com/anpfuel/application/usecase/contribution/DispatchContributionUseCase.kt) at lines 22–30, [per-command scheduler](../../data/src/main/kotlin/com/anpfuel/data/worker/ContributionWorkScheduler.kt).

`listDispatchable` selects all rows/payloads and filters in memory; a worker scheduled for a known ID still does this whole scan. `listOwned` repeats it on several paths. Received/terminal receipts remain durable without a configured queue/history size policy. N workers over N retained rows can approach quadratic decoding/scanning cost. Permanent domain refusals enter FAILED even though the immediate worker returns failure, so the stored state does not encode a terminal refusal distinctly.

Precondition: a large locally accumulated queue/history; no remote queue injection was found. Impact: reconnect stalls, memory/storage pressure and unnecessary retries. Atomic dispatch leases, owner/origin binding and frozen revision checks are positive controls and should be retained.

Correction: select/claim by command ID or bounded eligible keyset batch in SQL, enforce item/byte/retention budgets with honest user feedback, bound concurrency and attempts, and persist terminal versus retryable refusal. Verify work scales with batch size rather than full history; test two owners, offline restart, changing account/origin, retries and 429 `Retry-After` behavior.

### AUD-12 — The location gate validates geometry, not the origin of device assertions

Severity: MEDIUM, adversarial-client integrity risk. Confidence: source/controlled application probe. This is a trust-boundary limitation requiring an explicit policy decision, not a promise that GPS can be made unspoofable.

Sources: [wire location fields](../../backend/internal/modules/community/adapters/http/http.go) at line 118, [VerifyDeviceFix](../../backend/internal/modules/community/application/location_verify.go) at line 81, [photo eligibility](../../backend/internal/modules/community/application/photo_capture.go) at line 48, [signal derivation](../../backend/internal/modules/community/application/derive.go) at line 81. Contract: [L01–L04](LOCAL_MEDIA_LOCATION_POLICY.md).

The backend recomputes risk from client-supplied source-info, simulated flag, accuracy, capture timestamp and coordinates. A request signature proves control of a contributor key; it does not authenticate Android OS location measurements. A custom client can supply a consistent `source_info_present=true`, `simulated=false` assertion and the public station coordinates. The application probe accepted such entirely synthetic metadata with a precise-station distance result of zero. It did not perform a device/server attestation or live signed HTTP capture.

The normal app's mock-provider detection remains useful, and explicit `simulated=true` is refused. However, changing all consistent fields is different from changing only a contradictory verdict; existing inconsistent-field tests do not prove physical presence.

Correction: version/document the assurance level, prevent self-attestation alone from granting independent HIGH trust, retain geometry/freshness/teleport checks, and test a separately registered adversarial client in isolated staging. Assess platform attestation as a bounded additional signal only after measuring support/privacy/cost; it is not proof of physical GPS by itself. Preserve honest manual/offline and unsupported-device outcomes. No universal anti-spoof guarantee.

## Positive controls and remaining verification

Reviewed controls include signed request/body/nonce binding, revoked-key and owner checks, natural-key conflict handling, parameterized SQL, capped cursor pagination on community feeds, account status/session checks, private media namespaces, bounded worker downloads/image budgets, HTTPS/nonexported Android providers, sealed credential/photo stores and redacted backend logs. These reduce exposure and passed relevant local suites; they are not blanket certification of every path.

Additional hardening/verification obligations, not counted as demonstrated high/medium exploits:

- Android backup rules exclude shared preferences but not Room databases. Private draft/owner/evidence metadata may therefore enter OS backup/device transfer; photos under cacheDir and excluded auth preferences are different categories. Decide and test selective private-table/database exclusion or encrypted/no-backup ownership handling. Do not claim plaintext keys/photos are backed up or cross-account disclosure without a restore test.
- Community idempotency orchestration in `cmd/api/main.go` passes a transaction to the runner callback but `run(ctx)` performs its own store transaction. Existing natural-key convergence reduces duplicate effects. Crash-after-business-commit/before-ledger-completion, concurrent expiry and pool exhaustion still need route-level fault injection before claiming one transactional boundary.
- No full Android dependency advisory scan, container/base-image advisory scan, release APK/static-bytecode review, real phone/Fake GPS run, real S3/R2 policy check, WAF/firewall/origin review, live load test, 24/48h media soak or backup/restore device execution occurred. Go/Rust clean scans do not cover these surfaces.
- Later CS02–CS07A helpfulness/moderation SQL/HTTP changes and unfinished Android work must be audited at their final commit, alongside queued social actions, consent/revocation and full offline extensions. A plan/draft term is not a shipped capability or evidence that consent is being collected.
- Availability protection must combine backend admission with a separately authorized edge/network layer. An app installation key, IP quota or finite benchmark cannot guarantee protection against every DDoS or every storage abuse.

## Primary references

Use these as control guidance, not evidence that the deployment already implements it:

- [OWASP API4: unrestricted resource consumption](https://api-security.owasp.org/editions/2023/en/0xa4-unrestricted-resource-consumption/): request and allocation admission controls.
- [Cloudflare R2 presigned URLs](https://developers.cloudflare.com/r2/api/s3/presigned-urls/): supported upload mechanisms, signed headers and URL reuse.
- [Android Auto Backup](https://developer.android.com/identity/data/autobackup): default database inclusion versus cache/no-backup exclusions.
- [Go vulnerability management](https://go.dev/doc/security/vuln/): source reachability scanning is one layer alongside application review.

All findings reference the pinned source; changed head/configuration requires affected revalidation. No runtime remediation or release acceptance is implied by this report.
