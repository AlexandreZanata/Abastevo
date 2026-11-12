# F0 decision review — Organization scope

Status: **OWNER_ACCEPTED / DOCUMENTED_LOCALLY**, 2026-11-10.
Task: EMP-GOV-001. Source reviewed: `a97ba3b3` in the
`enterprise-suite-plan` worktree, plus its uncommitted enterprise plan.
The maintainer explicitly approved the organization as the access boundary
only for private Abastevo Empresas data on 2026-11-10 and authorized the ADR,
contract updates and continued F0 work. [ADR-025](../../adr/025-private-abastevo-empresas-organization-boundary.md)
and the [Abastevo Empresas contract](../../product/ABASTEVO_EMPRESAS_CONTRACT.md)
record the decision. ADR-025 avoids a numbering collision with the OCR
workstream's ADR-024.

## Accepted scope

Make the organization the owner and isolation boundary for private Abastevo
Empresas records. Each private record has one organization. A legal entity,
CNPJ, operating unit, fleet or team belongs inside that boundary. Membership
and a role scoped to the action and resource confer access. An account may
have separate memberships in multiple organizations. Shared CNPJ roots and
public catalog references confer no access.

Keep the Go modular monolith and PostgreSQL/PostGIS. A scoped repository
requires the resolved organization, composite foreign keys enforce same-org
references between private records, and PostgreSQL row-level security adds
defence in depth. The application database role must be subject to those
policies. Validate the boundary independently at the API, database, job,
search, cache, storage and export surfaces with two synthetic organizations.
Use the fixed role and permission registry first. Any later sharing between
organizations requires an explicit contract with a field allowlist.

The scope is enterprise-private data (BR-TEN-001, BR-TEN-002, BR-TEN-003,
BR-TEN-005, BR-TEN-006, BR-TEN-007 and BR-TEN-010). Public catalog and
community routes retain their existing access and source rules. Motoristas
and personal Premium records retain account ownership. The existing
`ENTERPRISE` account tier and payment family confer no organization access.
Postos representation, paid organization plans and Data redistribution
have separate decisions and gates.

Organization scope handles several legal entities under one operator and
separate franchise operators under one CNPJ root. A CNPJ-root boundary would
conflate those operators. A unit boundary would prevent one company from
managing several units together. A separate service or database would add an
operational boundary before the plan identifies a measured need.

## Current rules and adoption changes

| Source | Disposition |
|---|---|
| [AGENTS.md](../../../AGENTS.md) rule 12 and [ADR-004](../../adr/004-backend-scope-and-governance.md) | Record the narrow private Empresas exception and link ADR-025. |
| [ADR-005](../../adr/005-modular-monolith-and-postgresql.md) | Reuse the existing stack and module boundaries. |
| [Premium Cloud Protocol](../../backend/PREMIUM_CLOUD_PROTOCOL.md), [Premium Product Contract](../../product/PREMIUM_PRODUCT_CONTRACT.md) and [Driver Suite Contract](../../product/DRIVER_SUITE_CONTRACT.md) | Keep account ownership and link the separate Abastevo Empresas contract. |
| [Product Contract](../../product/PRODUCT_CONTRACT.md) and [Open Source Business](../../product/OPEN_SOURCE_BUSINESS.md) | Cross-reference the new contract and preserve the public-product exclusions. |
| [ADR-023](../../adr/023-admin-suite-granted-tiers-contributors.md), [Admin Tiers Contract](../../product/ADMIN_TIERS_CONTRACT.md) and [migration 000130](../../../backend/db/migrations/000130_payment_enterprise_capabilities.sql) | Keep account tiers and payment capabilities separate from membership. Changes to organization offers require their own decision. |
| [ADR-017](../../adr/017-station-profile-and-verified-representation.md) and [Stage D](../../planning/FUTURE_EVIDENCE_BUSINESS_PLAN.md) | Keep station representation and purchase evidence within their existing scope. Receipt storage and any Stage D override require separate decisions. |

The migration records the payment family's scope. The
[payment policy tests](../../../backend/internal/modules/payments/domain/policy_test.go)
assert that the current enterprise offer carries no app capabilities. The
[cloud sync HTTP tests](../../../backend/internal/modules/cloudsync/adapters/http/http_test.go)
cover owner-scoped cursor isolation for personal records. Both are current
contract references. Future organization isolation has its own acceptance
criteria.

## Verification required for implementation

- A membership in organization A cannot list, read, write, search, export
  or fetch files from B. Using B's identifier in a route, cache, queued job
  or file request must return no B data or resource-existence hint.
- With two organizations in real PostGIS, prove that composite references
  cannot attach a record in A to a private record in B, and that an
  application-role query with missing scope or a missing explicit filter
  returns no foreign rows. Check successive transactions for context
  leakage through pooled connections.
- Offboarding removes access, including replay and queued work, while
  preserving organization ownership. Switching to personal mode or another
  organization cannot move an offline record.
- An existing account tier, payment, same-CNPJ-root relationship or staff
  login grants no organization membership or permission. Existing anonymous
  catalog reads and owner-scoped personal flows retain their contracts.

## F0 handoff

The accepted scope closes the organization-boundary question. Any source
work depends on the remaining M0 decisions in the
[enterprise roadmap](../14-roadmap.md#3-entry-exit-and-activation-per-milestone).
For sequencing, keep [ADR-019](../../adr/019-android-vps-integration-first.md)
as the runtime order until the maintainer records a separate decision.
Real-data pilot and release gates remain in the existing roadmap.

Local delivery state: approval recorded and affected contracts updated.
Issue, PR and remote checks are pending. Validation covers the decision,
contract amendments and their references.
The full plan checker requires the canonical backlog and module sources
referenced by the generated index.
