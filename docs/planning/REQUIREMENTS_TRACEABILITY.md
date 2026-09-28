# Commercialization brief traceability

Reference: user-supplied `plano-para-comercialização-domeuprojetoopensource.txt` (82 sections), reviewed as planning material. Direct user request controls ordering and scope. No embedded request for a role, command, production change or payment is treated as independent authorization.

## Adopted, adjusted and deferred

Adopted: preserve the existing app, simple Go modular monolith, PostgreSQL/PostGIS, private object media, distinct ANP/community sources, anonymous contribution, immutable facts, testable rules, privacy, fraud controls, explicit contracts and executable microtasks.

Adjusted: Android UI/CameraX/OCR and Kotlin fixture integration move after backend/infrastructure gate G09. The brief's suggested intermediate Android phase would contradict the direct user request. The original phase numbers are not reused. Snapshot import preserves the new repository's configured origin rather than replacing it with the upstream remote.

Refined after audit: numeric/alphanumeric CNPJ; milli-BRL with unit and raw decimal; GASOLINE_ADDITIVED versus legacy PREMIUM; revisioned official import versus first-import-wins; FTS4/schema4 actual baseline; P-256 recommendation instead of an unverified Ed25519 Keystore assumption; verification of frozen uploaded bytes; privacy erasure exception to business immutability.

Deferred: optional account, entitlement/billing, cloud sync and commercial products to P11 after pilot. Advanced scaling tools require measured need/ADR. No production implementation or dependency additions in this planning delivery.

## Section-to-artifact coverage

- **1–3 Product, platform, free/paid identity:** PRODUCT_CONTRACT, OPEN_SOURCE_BUSINESS, DOMAIN_MODEL, API_PLAN; P03/P09/P11.
- **4–8 Simplicity, monolith, agent maintenance, layout, preserve Android:** TARGET_ARCHITECTURE, AI_ENGINEERING_CONTRACT, MIGRATION_PLAN, ADR-004/005; P01 and G09 boundary.
- **9 Offline:** MIGRATION_PLAN outbox/cache/conflicts and TARGET_ARCHITECTURE; P10-T02/T05/T08.
- **10 ANP:** ANP_INGESTION, CURRENT_STATE_AUDIT, DATA_MODEL, ADR-006; P02.
- **11–17 Contexts/DDD/observations/conditions/states/projection/consensus:** DOMAIN_MODEL, COMMUNITY_PRICING_SPEC, DATA_MODEL; P04/P06.
- **18 OCR:** COMMUNITY_PRICING_SPEC and MIGRATION_PLAN; P10-T04 only after G09.
- **19–23 Media/location/geo/abuse/identity:** SECURITY_PRIVACY, API_PLAN, ANP_INGESTION, ADR-007/010; P02-T07, P03/P05/P06/P07.
- **24–30 API/cache/DB/migrations/transactions/jobs/events:** API_PLAN, DATA_MODEL, TARGET_ARCHITECTURE; P01/P03/P04/P06/P08.
- **31–37 TDD/test pyramid/determinism/contracts/migrations/performance/projection:** TEST_STRATEGY, ANP_INGESTION, COMMUNITY_PRICING_SPEC, INFRASTRUCTURE_PLAN; all relevant task tests/gates.
- **38–41 Security/privacy/retention/moderation:** SECURITY_PRIVACY, DOMAIN_MODEL, BUC-006/007; P07/P08.
- **42–43 Observability/failures:** INFRASTRUCTURE_PLAN failure-mode runbook, RISKS; P08/P09.
- **44–53 Dependency/agent rules/tasks/workflow/docs/AGENTS/progress/ADR/CI/Git:** AGENTS, AI_ENGINEERING_CONTRACT, ROADMAP, docs index, PROGRESS, ADR_INDEX; P01-T12 and task DOD-1.
- **54–55 Deployment/scale:** INFRASTRUCTURE_PLAN and TARGET_ARCHITECTURE, ADR-011; P08/G09. No speculative distributed infrastructure.
- **56–59 Build in public/license/brand/README:** OPEN_SOURCE_BUSINESS, TRADEMARKS, README, ADR-012; source license unchanged.
- **60 Existing audit:** CURRENT_STATE_AUDIT and BASELINE_VALIDATION; audit written before target plan.
- **61–62 Compatibility/migration phases:** MIGRATION_PLAN, fixture manifest design and ROADMAP; Android postponed to P10.
- **63–65 Gates/DOD/catastrophic guards:** ROADMAP G01…G11/G09 checklist, TEST_STRATEGY, AI_ENGINEERING_CONTRACT, RISKS.
- **66 Config:** SECURITY_PRIVACY, P01-T03 and P08-T01.
- **67 Admin/public separation:** API_PLAN, SECURITY_PRIVACY, BUC-006, P07-T02; restricted CLI before any dashboard.
- **68–69 Billing/sync:** OPEN_SOURCE_BUSINESS, MIGRATION_PLAN; bounded P11 tasks with new specifications required before implementation.
- **70–72 Analytics/product metrics/freshness:** PRODUCT_CONTRACT, SECURITY_PRIVACY inventory, COMMUNITY_PRICING_SPEC and INFRASTRUCTURE_PLAN; no invasive analytics SDK.
- **73 Planning only:** README/PROGRESS status and imported source comparison; no backend Go code, new dependencies or production migrations created.
- **74 Mandatory deliverables A–M:** all named files exist in docs index plus root ROADMAP.md (locations below).
- **75–76 Microtask format/priorities:** 73 tasks, each with ID/goal/why/inputs/areas/dependencies/tests/outline/acceptance/commands/risks/recovery/DOD; MUST backend/app versus LATER paid scope. SHOULD refinement list in PRODUCT_CONTRACT.
- **77 Risk matrix:** RISKS contains probability, impact, detection, mitigation, recovery and ownership for 15 risks.
- **78 Decisions:** DECISIONS contains options, recommendation, rationale, tradeoff and decision deadline for D01…D12.
- **79 Cost:** INFRASTRUCTURE_PLAN contains 1k/10k/100k/1M MAU scenarios, explicit workload assumptions, byte/operation drivers and measured scale triggers; no invented vendor quote.
- **80–82 Agent-ready handoff/principles/audit first:** AGENTS, ROADMAP P01-T01, PROGRESS, audit and validation artifacts; user receives direct entry links and honest baseline limitations.

## Mandatory named artifacts

1. `docs/CURRENT_STATE_AUDIT.md`
2. `docs/backend/TARGET_ARCHITECTURE.md`
3. `docs/backend/DOMAIN_MODEL.md`
4. `docs/backend/COMMUNITY_PRICING_SPEC.md`
5. `docs/backend/API_PLAN.md`
6. `docs/backend/DATA_MODEL.md`
7. `docs/security/SECURITY_PRIVACY.md`
8. `docs/backend/TEST_STRATEGY.md`
9. `docs/AI_ENGINEERING_CONTRACT.md`
10. `docs/MIGRATION_PLAN.md`
11. `ROADMAP.md`
12. `docs/ADR_INDEX.md`
13. `docs/product/OPEN_SOURCE_BUSINESS.md`

## Deliberate implementation boundaries

OpenAPI syntax, concrete fixtures, Go module, SQL migrations, Compose services, CI and operational scripts are subsequent roadmap outputs, not fabricated placeholders claimed as complete. Runtime privacy/legal review, hosting credentials/provider selection, restore/load results and device interoperability remain real execution gates. A “complete plan” describes those tasks and acceptance evidence; it does not imply the backend is already running.
