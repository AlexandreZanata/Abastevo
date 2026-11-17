# Documentation

Start with [AGENTS.md](../AGENTS.md) (how to work here) and the
[system overview](architecture/overview.md). Open work is in
[ROADMAP.md](../ROADMAP.md). Module READMEs (`backend/`, `infra/`, `landing/`,
`admin-web/`, `business-web/`, `contracts/`) cover building and running each
part.

Rules for this folder: English only (user-facing copy that must stay in
Portuguese is marked as such), one topic per file, kebab-case names, no
progress logs, task ledgers or evidence dumps. Update the owning document in
the same change as the code.

| Folder | Contents |
|--------|----------|
| [architecture/](architecture/) | [System overview](architecture/overview.md), [backend](architecture/backend.md), [Android](architecture/android.md) and its [stack](architecture/android-stack.md), [ANP data sources](architecture/data-sources.md), [source admission registers](architecture/sources/register.md). |
| [product/](product/README.md) | Business rules and behavior contracts: stations, community, forum, map, EV, driver suite, Premium, payments, landing. [Glossary](product/glossary.md), [licensing](product/licensing.md). |
| [backend/](backend/) | [API semantics](backend/api.md), [domain model](backend/domain-model.md), [data model](backend/data-model.md), [testing](backend/testing.md), [toolchain](backend/toolchain.md), [ANP ingestion](backend/anp-ingestion.md), [community pricing](backend/community-pricing.md), forum, photos, Premium and payment providers. |
| [android/](android/) | [Local build](android/build.md), [release build](android/release.md), [design system](android/design-system.md), [UI containers](android/ui-containers.md), [navigation](android/navigation.md), [map engine](android/map-engine.md), OCR, forum, Premium, games. |
| [enterprise/](enterprise/README.md) | Abastevo Empresas: [contract](enterprise/contract.md), [architecture](enterprise/architecture.md), [business rules](enterprise/business-rules.md), [permissions](enterprise/permissions.md), [modules](enterprise/modules/), [open backlog](enterprise/backlog/INDEX.md). |
| [operations/](operations/README.md) | Current deployment, deploy agent, backups, recovery, monitoring, registry sync, moderation and activation runbooks. |
| [security/](security/) | [Security and privacy](security/security-privacy.md), [privacy policy pointer](security/privacy-policy.md), [identity profile](security/identity-profile.md), [media and location](security/media-location-policy.md), [retention](security/retention.md), [risks](security/risks.md). |
| [decisions/](decisions/README.md) | Architecture decisions still in force. |
| [store/](store/README.md) | Google Play [readiness](store/play-readiness.md), [listing copy](store/play-store-listing.md), screenshots and cards. |
| [brand/](brand/) and [assets/](assets/) | [Brand identity](brand/identity.md), [app icons](brand/app-icons.md), design masters used by the asset scripts, [asset attribution](assets/attribution.md), [Android tutorial captures](assets/app-tutorial/README.md) and their [capture contract](assets/app-tutorial/CAPTURE_CONTRACT.md). |
