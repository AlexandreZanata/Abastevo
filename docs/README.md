# abastevo documentation

Current commercial direction: [community delivery plan](planning/COMMERCIAL_COMMUNITY_PLAN.md), [experience contract](product/COMMUNITY_EXPERIENCE.md), [Android/iOS scope ADR](adr/016-android-commercial-community-ios-deferred.md), [parked iOS backlog](planning/archive/IOS_DEFERRED.md) and [native icon guide](brand/APP_ICONS.md).

This repository contains the preserved Android app, integrated backend phase history and the next functional multiplatform plan. Current status is in [PROGRESS](planning/PROGRESS.md); prior command outcomes are preserved as evidence, not rerun claims. Next implementation: [P12-T01](../ROADMAP.md#p12-t01) after G09-LOCAL correction integration; real-production G09 waits until G18.

## Canonical planning documents

Next construction: [Android/VPS P34–P38](planning/ANDROID_VPS_PLAN.md) and [continuation prompt](planning/CONTINUE_ANDROID_VPS_PROMPT.md); subsequent [national catalog](planning/STATION_CATALOG_PLAN.md) and [station representation](planning/STATION_PROFILE_PLAN.md). Plans are not runtime proof.

- [CURRENT_STATE_AUDIT](CURRENT_STATE_AUDIT.md) — source provenance, real implementation, reuse and inconsistencies.
- [PRODUCT_CONTRACT](product/PRODUCT_CONTRACT.md) — free/community scope and revised local-backend → functional-app → real-production ordering.
- [TARGET_ARCHITECTURE](backend/TARGET_ARCHITECTURE.md) — C4 views, modules, ownership, flows, jobs and scale.
- [DOMAIN_MODEL](backend/DOMAIN_MODEL.md) — vocabulary, B-BR invariants, aggregates/events/states.
- [COMMUNITY_PRICING_SPEC](backend/COMMUNITY_PRICING_SPEC.md) — admissibility, conditions, consensus, trust and freshness.
- [API_PLAN](backend/API_PLAN.md) — contracts, proof, errors, idempotency and caching; OpenAPI introduced in P01.
- [DATA_MODEL](backend/DATA_MODEL.md) — proposed ownership, tables, constraints, indexes, transactions and volumes.
- [ANP_INGESTION](backend/ANP_INGESTION.md) — source discovery, parser validation, revisions, geo and shared fixtures.
- [SECURITY_PRIVACY](security/SECURITY_PRIVACY.md) — STRIDE, identity/media security, inventory and rights.
- [TEST_STRATEGY](backend/TEST_STRATEGY.md) — unit/integration/contracts/golden/migration/E2E/load and risk-based gates.
- [INFRASTRUCTURE_PLAN](backend/INFRASTRUCTURE_PLAN.md) — environments, deployment, recovery, observability and cost assumptions.
- [DELIVERY_WORKFLOW](planning/DELIVERY_WORKFLOW.md), [FAST_EXECUTION](planning/FAST_EXECUTION.md) and [CI_PLAN](planning/CI_PLAN.md) — isolated phase branches/checkpoints, task issues/commits, immediate targeted tests, final project batch CI/PR merge/wiki and separate full release.
- [Phase record](planning/templates/PHASE_RECORD.md), [task issue](planning/templates/TASK_ISSUE.md) and [phase PR](planning/templates/PHASE_PR.md) — templates for G01-FLOW helpers; no remote records are implied.
- [AI_ENGINEERING_CONTRACT](AI_ENGINEERING_CONTRACT.md) and [AGENTS](../AGENTS.md) — bounded task workflow and DOD-1.
- [MIGRATION_PLAN](MIGRATION_PLAN.md) — each existing feature, offline/conflict behavior and P10 integration.
- [Backend BUC-001…008](use-cases/backend-use-cases.md) — actor/preconditions/flows/rules/events/acceptance.
- [ADR_INDEX](ADR_INDEX.md) — existing decisions plus backend and delivery policy decisions.
- [OPEN_SOURCE_BUSINESS](product/OPEN_SOURCE_BUSINESS.md) and [TRADEMARKS](../TRADEMARKS.md) — MIT preservation, brand and hosted value.
- [DECISIONS](planning/DECISIONS.md), [RISKS](planning/RISKS.md), [REQUIREMENTS_TRACEABILITY](planning/REQUIREMENTS_TRACEABILITY.md) — unresolved choices, failure risks and brief coverage.
- [BASELINE_VALIDATION](planning/BASELINE_VALIDATION.md), [original plan validation](planning/DOCUMENT_VALIDATION.md), [P01 evidence archive](planning/history/P01_FOUNDATION_EVIDENCE.md) and [workflow revision validation](planning/WORKFLOW_PLAN_VALIDATION.md) — dated evidence and limitations.

## Read only what the task needs

Every task: AGENTS → PROGRESS → selected roadmap task → relevant product/rule/BUC → ADR → specific API/schema → relevant tests/code. Import work adds ANP_INGESTION; security/media adds SECURITY_PRIVACY; deployment adds INFRASTRUCTURE_PLAN. No need to reload every plan per task.

Legacy Android references: [user-business-logic](user-business-logic.md), [architecture](architecture.md), [tech-stack](tech-stack.md), [glossary/BR-001…028](glossary.md), [data-sources](data-sources.md), [local build](local-build.md), [privacy policy](privacy-policy.md), [use cases](use-cases/). These remain historical/current-app context; audit conflicts are explicitly tracked.

## References checked for this plan

Technical/legal/provider facts were checked on 2026-09-28; recheck at implementation/procurement where relevant.

- [Android Keystore](https://developer.android.com/privacy-and-security/keystore) — key protection/support considerations.
- [RFC 9421](https://www.rfc-editor.org/rfc/rfc9421.html) — message-signature profile basis.
- [PostgreSQL SELECT](https://www.postgresql.org/docs/current/sql-select.html), [PostGIS ST_DWithin](https://postgis.net/docs/ST_DWithin.html) — queue lock and spatial-query semantics.
- [R2 presigned URLs](https://developers.cloudflare.com/r2/api/s3/presigned-urls/) and [pricing](https://developers.cloudflare.com/r2/pricing/) — direct-upload capability and cost categories.
- [Nominatim usage policy](https://operations.osmfoundation.org/policies/nominatim/) — provider restrictions; no national bulk assumption.
- [Receita Federal CNPJ program](https://www.gov.br/receitafederal/pt-br/acesso-a-informacao/acoes-e-programas/programas-e-atividades/cnpj-alfanumerico) — alphanumeric identifier compatibility.
- [LGPD](https://www.planalto.gov.br/ccivil_03/_ato2015-2018/2018/lei/l13709.htm) and [MIT license text](https://opensource.org/license/mit) — review references, not a claim that the project is legally certified.

## Functional multiplatform extension

- [MOBILE_DELIVERY_PLAN](planning/MOBILE_DELIVERY_PLAN.md) — explicit phase sequence and functionality-first acceptance.
- [KOTLIN_MULTIPLATFORM_AUDIT](mobile/KOTLIN_MULTIPLATFORM_AUDIT.md) — actual versions, portability gaps and official compatibility sources.
- [STATION_FUEL_FEEDBACK](product/STATION_FUEL_FEEDBACK.md) — stars, 280-character text, votes and moderation.
- [FREE_ACCOUNT_ACCESS](security/FREE_ACCOUNT_ACCESS.md) — free email-code/Google/Apple signup and secure recovery.
- [LOCAL_MEDIA_LOCATION_POLICY](security/LOCAL_MEDIA_LOCATION_POLICY.md) — low-memory encoding, all-copy 24-hour expiry and simulated-location controls.
- [ADR-014](adr/014-functional-app-before-production-release.md), [ADR-015](adr/015-kotlin-multiplatform-shared-domain.md) — release order and Kotlin/Swift boundaries.

- [Brand identity](brand/IDENTITY.md) — selected name/logo and local README artwork awaiting approval.
