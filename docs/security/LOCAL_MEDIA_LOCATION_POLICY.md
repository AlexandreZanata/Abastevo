# Lightweight media and location integrity target

Status: ADOPTED target, NOT IMPLEMENTED. P15 owns media; P16 owns location. Existing v1 media limits/14-day sanitized retention remain an implementation gap until forward changes pass their tests. This policy supersedes the older target; it does not claim deployed compliance.

## B-BR-M01…M06: all app-owned photos expire within 24 hours

M01: Audit photos, including original/quarantine/sanitized/thumbnail copies, have a single deadline: server first receipt + **24 hours**. Retries, processing, copying, disputes and downloads cannot extend it. No 14-day review or 30-day case exception. App capture cache has its own capture-time deadline of at most 24 hours; delete originals immediately after successful encoding. Keep photos out of Gallery, logs, fixtures, public CDN and independent media backups.

M02: Sample-decode using native Android/iOS codecs before allocating full camera pixels; process off the UI thread, one image at a time. Supported device formats (including JPEG/PNG/HEIF where the platform decodes them) convert to the frozen wire format; malformed, animated or unsupported inputs fail safely with a useful error. “Any photo” is not permission to run arbitrary decoders. Strip EXIF/GPS/orientation/private metadata and re-encode locally. Proposed target JPEG ≤150 KiB, hard cap ≤256 KiB, longest edge ≤1600 pixels and ≤2 megapixels, at most three bounded encoding attempts. Preserve legible pump/price text; reject unreadable results instead of silently compressing away evidence. Freeze these measured limits in P15-T01 before changing OpenAPI's current 3 MiB limit.

M03: Additional mobile working-memory hypothesis ≤32 MiB per processed image, measured on the P12 low-resource devices. Backend streams capped uploads, checks magic/dimensions before decoding, strips metadata and performs one bounded re-encode. Worker concurrency is derived from measured per-image peak RSS and a configured memory budget; never rely on content-type/client dimensions or unbounded buffers. Test oversized pixels, compression bombs, corrupt headers, alpha/orientation, interrupted uploads and parallel pressure. Local hashing/dedup is not a substitute for readable proof.

M04: Private storage only. Access expires at the deadline even if cleanup fails; signed reads last at most min(60 seconds, remaining lifetime). Schedule deletion by deadline, retry failures, alert on overdue physical objects and stop new evidence intake if enforcement is unhealthy. Provider lifecycle/versioning must be proven to cover **every copy**, not assumed to meet 24-hour granularity. A failure that leaves physical bytes past 24 hours is a policy violation and blocks release; access denial alone is not physical deletion evidence.

M05: On-device encrypted ephemeral cache, expiry checks on launch/resume/read and OS background cleanup. A powered-off device cannot run a deletion job: expiry/key-access controls and cleanup at next execution must be tested and this limitation disclosed; do not promise remote physical deletion while offline. Scope excludes photos copied independently by the user outside the app. Restore must purge expired media before traffic, with deletion-ledger replay and no deadline reset. Do not back up transient photo bytes. Existing old objects need append-only timestamp migration and purge/recovery tests before rollout.

M06: Retain only necessary price facts and minimized validation outcomes after photos expire. Separate hash/privacy retention review; no indefinite cross-user photo fingerprint store. Expired evidence cannot be advertised as still available for audit; explicit confidence/explanation rules must be tested. Rights deletion purges sooner. Freeze notice and policies before public pilot.

BUC-M01 capture/encode/upload, M02 revalidate/sanitize, M03 expiry/delete/read refusal, M04 restore/reconcile. Contract and device/backend fixtures precede implementation.

## B-BR-L01…L04: location integrity

L01: Block location-sensitive contribution/proximity claims when the OS reports simulated/mock location. Android has LocationCompat.isMock; iOS has CLLocationSourceInformation.isSimulatedBySoftware. These detect platform-marked simulation, not all spoofing on compromised devices. [Android API](https://developer.android.com/reference/androidx/core/location/LocationCompat), [Apple API](https://developer.apple.com/documentation/corelocation/cllocationsourceinformation/issimulatedbysoftware).

L02: Missing/unsupported signal, stale or inaccurate fix, permission denial and clock anomalies yield UNKNOWN or a denied location-dependent claim, never “verified GPS”. Keep anonymous browsing/manual station lookup and existing offline functions available. Do not block accessibility, development settings or unrelated apps. Specify the allowed degraded contribution policy before code; do not silently promote manual location to trusted proximity.

L03: Shared risk state with native ports; backend independently validates bounded freshness/proximity and consumes the signal as untrusted input. A forged isMock=false cannot alone grant HIGH confidence. Optional Play Integrity/App Attest are a later measured risk decision, server-verified if adopted; hardware availability is not universal. Debug location injection is limited to nonrelease builds and isolated tests, with release artifact assertions.

L04: Exact fixes remain private, short-lived and unlogged; persist only distance/accuracy/risk bands permitted by privacy policy. Test Android mock provider, iOS simulation, missing source info, coarse permission, stale/teleport/replay/clock cases, offline resume and release-debug exclusion on real/simulated devices. No “100% fake GPS prevention” claim.

BUC-L01 obtain/classify fix, L02 authorize location-dependent action, L03 present denial/degraded state and recovery. Implement before app contribution integration; performance checks ensure no busy polling or background tracking.
