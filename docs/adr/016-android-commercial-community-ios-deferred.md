# ADR-016: Android commercial community and explicit iOS deferral

Date: 2026-10-01. Status: ACCEPTED product/platform direction by explicit maintainer request; future functionality remains PLANNED.

## Context

The maintainer selected abastevo and its approved A logo, requested a simpler commercial community experience led by users' photographed station prices, and explicitly asked to archive iOS until a new explicit resumption request. No Mac is available. PR #67 integrated P18's local validation slice; it did not accept the full Android/iOS G18 matrix.

## Decision

1. Plan Android-first commercial delivery through P19–P24. Community current-price projections are the primary experience; ANP observations remain dated reference data. A new photo is evidence submitted for validation, not automatically a trusted current price. Existing consensus, exact money and price-condition contracts remain authoritative.
2. Preserve existing free features and modular Kotlin domain/application ports. Move expert ANP history/calculations into secondary navigation rather than deleting them. Free registration and social participation remain independent of payment.
3. Archive the unfinished iOS/native acceptance workstream as **DEFERRED_EXPLICIT_RESUME_ONLY**. Preserve code, KMP ports, icon assets, historical evidence and unresolved rows. A Mac becoming available, Android phase completion, CI results or a release request do not automatically resume iOS. A new explicit maintainer request is required; then audit Xcode/toolchains, host target and all native parity/device evidence.
4. Preserve historical **G18_NOT_ACCEPTED**. Use **G24-ANDROID-COMMERCIAL** for the new integrated Android functional/usability/performance acceptance campaign. It must include unresolved Android/security/storage requirements from P18 plus the new flows. Completing a plan is not passing this gate.
5. Android/backend real-production G09 may be considered only after G24-ANDROID-COMMERCIAL and all backend prerequisites. G09 retains its full real edge/TLS/private storage/all-copy deletion/restore/load/provider/security/privacy/operator evidence. Certification must explicitly state Android/backend scope; it never certifies iOS. Public P10-T09 pilot follows G09; optional paid P11 releases remain separate. No deployment, stable tag or public pilot is authorized by this decision alone.

This supersedes only ADR-014's requirement to wait for *both* Android and iOS before an Android/backend commercial release, and its preservation of the previous visual design. ADR-015's architectural boundaries, historical evidence, immediate critical tests and protected phase delivery remain intact. Historical G18 device requirements remain valid for eventual multiplatform acceptance.

## Consequences

Backend extensions precede corresponding app consumers. No speculative feed service, permanent public photo archive, paid confidence boost or compulsory payment registration is introduced. No new mandatory-photo API rule, trust weighting, freshness window or proximity threshold is invented here; changes require explicit contracts and tests in the owning task.

The archived [iOS resumption record](../planning/archive/IOS_DEFERRED.md), [commercial plan](../planning/COMMERCIAL_COMMUNITY_PLAN.md), current [progress](../planning/PROGRESS.md) and ROADMAP define execution. P18 issues stay open with unresolved evidence; they are not closed merely because iOS is deferred.
