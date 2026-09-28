# Documentation index

This repository contains the existing Android release and the planned backend-first evolution. Plans describe intended behavior, not shipped features. Start execution at [ROADMAP](../ROADMAP.md#p01-t01) and [PROGRESS](planning/PROGRESS.md).

## Canonical planning documents

- [CURRENT_STATE_AUDIT](CURRENT_STATE_AUDIT.md) — source provenance, real implementation, reuse and inconsistencies.
- [PRODUCT_CONTRACT](product/PRODUCT_CONTRACT.md) — free/community scope and backend-before-Android ordering.
- [TARGET_ARCHITECTURE](backend/TARGET_ARCHITECTURE.md) — C4 views, modules, ownership, flows, jobs and scale.
- [DOMAIN_MODEL](backend/DOMAIN_MODEL.md) — vocabulary, B-BR invariants, aggregates/events/states.
- [COMMUNITY_PRICING_SPEC](backend/COMMUNITY_PRICING_SPEC.md) — admissibility, conditions, consensus, trust and freshness.
- [API_PLAN](backend/API_PLAN.md) — contracts, proof, errors, idempotency and caching; OpenAPI introduced in P01.
- [DATA_MODEL](backend/DATA_MODEL.md) — proposed ownership, tables, constraints, indexes, transactions and volumes.
- [ANP_INGESTION](backend/ANP_INGESTION.md) — source discovery, parser validation, revisions, geo and shared fixtures.
- [SECURITY_PRIVACY](security/SECURITY_PRIVACY.md) — STRIDE, identity/media security, inventory and rights.
- [TEST_STRATEGY](backend/TEST_STRATEGY.md) — unit/integration/contracts/golden/migration/E2E/load and risk-based gates.
- [INFRASTRUCTURE_PLAN](backend/INFRASTRUCTURE_PLAN.md) — environments, deployment, recovery, observability and cost assumptions.
- [AI_ENGINEERING_CONTRACT](AI_ENGINEERING_CONTRACT.md) and [AGENTS](../AGENTS.md) — bounded task workflow and DOD-1.
- [MIGRATION_PLAN](MIGRATION_PLAN.md) — each existing feature, offline/conflict behavior and P10 integration.
- [Backend BUC-001…008](use-cases/backend-use-cases.md) — actor/preconditions/flows/rules/events/acceptance.
- [ADR_INDEX](ADR_INDEX.md) — existing decisions and nine added planning ADRs.
- [OPEN_SOURCE_BUSINESS](product/OPEN_SOURCE_BUSINESS.md) and [TRADEMARKS](../TRADEMARKS.md) — MIT preservation, brand and hosted value.
- [DECISIONS](planning/DECISIONS.md), [RISKS](planning/RISKS.md), [REQUIREMENTS_TRACEABILITY](planning/REQUIREMENTS_TRACEABILITY.md) — unresolved choices, failure risks and brief coverage.
- [BASELINE_VALIDATION](planning/BASELINE_VALIDATION.md) and [DOCUMENT_VALIDATION](planning/DOCUMENT_VALIDATION.md) — actual checks and limitations.

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
