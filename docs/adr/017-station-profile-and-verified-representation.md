# ADR-017 — Station profiles and scoped verified representation

Date: 2026-10-02. Status: **ADOPTED FOR PLANNING**, NOT IMPLEMENTED. Authority: maintainer requested a station profile entity and the complete secure process for proving representation. Details: [product rules/use cases](../product/STATION_PROFILE_REPRESENTATION.md), [P30–P33 delivery plan](../planning/STATION_PROFILE_PLAN.md). Existing ADR-016 platform/release scope remains in force.

## Context

Directory already owns a stable station UUID; new business profiles need representation proof without duplicating the station catalog or granting competitors privileges. Company registration/ANP authorization, signer identity, corporate powers and physical property ownership are different assertions. Public documents or possession of a certificate do not alone authorize the applicant's account.

## Decision

1. Reuse Directory Station and verified effective operating-entity revisions. Add a bounded `stationprofile` context for profile, claim, proof/decision and scoped grant; no tenant/service split.
2. Verify a request/account/station-specific signed declaration, exact bytes/content and independent corporate powers. Support company or personal recognized signatures plus authority evidence. Initial independent human review is mandatory; permitted validators are selected before implementation. No private keys/password/PFX, unrestricted gov.br integration assumption or automated weak-document approval.
3. Grant station/operator/account-scoped capabilities, not property title or generic OWNER authority. Free access; no paid trust, criticism deletion, regulatory overwrite or price-confidence privilege. Official replies preserve existing account/280-scalar/moderation rules.
4. Contestation, delegation, transfer, operator succession, expiry, suspension and erasure are first-class tested lifecycle behavior. Query-time enforcement denies stale/revoked authority; restore cannot resurrect it.
5. Freeze private proof retention and safe exact-byte processing before intake. All photos/scans retain maximum 24h even packaged inside PDF. Signed authorizations need their own minimal reviewed policy; no unbounded audit retention or production documents in fixtures.
6. Supersede planned P28-T04 with P30–P33 while preserving its ID/history. Selected profile scope passes G33 after catalog G29 and before the existing P09/G09 certification. Changed Android inputs acquire fresh scoped acceptance; historical G18 remains unaccepted and iOS explicit-resume-only.

## Alternatives and consequences

Phone/email/photo-only proof is simpler but insufficient against impersonation. e-CNPJ-only automation conflates signer/custodian with the applicant's actual authority and excludes personal/delegated paths. Paid verification or biometric onboarding is not assumed necessary. The chosen layered, reviewed process increases staff effort but permits scoped authority, revocation and recovery with clear source/privacy boundaries. Provider and retention decisions remain implementation prerequisites, not fictitious completed integrations.

No cryptographic or document process eliminates compromised credentials or fraudulent authority absolutely; reduce risk through independent facts, restricted capabilities, review, contestation and revocation. Production/provider/device acceptance remains separate from documentation and local mocks.
