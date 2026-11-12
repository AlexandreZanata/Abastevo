# ADR-025 — Organization boundary for private Abastevo Empresas data

Status: **ACCEPTED** on 2026-11-10 by the maintainer for the scope below.
Authority: EMP-GOV-001, [approval record](../enterprise/evidence/f0-organization-scope.md).
This decision amends [ADR-004](004-backend-scope-and-governance.md) within
Abastevo Empresas. See the [product contract](../product/ABASTEVO_EMPRESAS_CONTRACT.md)
for the rules and use cases.

## Context

A company needs to manage employees, vehicles and operations across units.
Multiple legal entities may be managed together, and independent franchisees
may share a CNPJ root. Personal accounts can participate in more than one
company and also use the free community and individual products.

## Decision

1. Each private Abastevo Empresas record has exactly one owning organization.
   CNPJs, establishments, units and teams sit within that boundary. The
   server resolves the requested organization against current membership
   and a role scoped to the action and resource. Organization and public
   catalog identifiers confer no authority. Begin with fixed role and
   permission keys.
2. Keep the existing Go modular monolith and PostgreSQL/PostGIS under
   [ADR-005](005-modular-monolith-and-postgresql.md). Pass resolved organization
   scope explicitly into repositories. Enforce same-organization private
   references through composite foreign keys and use PostgreSQL row-level
   security as defense in depth. Bind scope within each transaction, deny
   access when scope is absent and run ordinary requests and jobs under a
   role subject to the policies. Keep migration authority separate.
3. Carry the boundary through jobs, caches, search, exports, file access and
   device partitions. Cross-organization requests reveal no foreign record
   or existence hint. Any future sharing needs a separate contract naming
   recipients, permissions and the exact fields allowed.
4. Public and community access remain tenant-free. Personal Motoristas and
   Premium records remain account-owned. Free registration, source
   separation and payment neutrality continue to apply. The existing
   `ENTERPRISE` account tier and payment family confer no organization
   rights. Postos representation, real receipt intake and retention,
   tracking, organization billing and Data redistribution retain their
   separate decision gates.

## Alternatives

A CNPJ-root boundary can combine independent businesses. A unit boundary
prevents company-wide management. Relying on query filters alone leaves
the boundary dependent on each caller remembering the predicate. A separate
service or database adds deployment and maintenance work without a measured
need at this phase.

## Verification and follow-up

The F1 tenancy kernel and storage tasks must prove isolation with two
synthetic organizations on real PostGIS. Verify missing scope, a missing
query filter, cross-organization references and scope reuse across pooled
transactions. Exercise current membership and scoped permissions, revocation,
replay and asynchronous work through each affected entry point. Preserve
anonymous public reads and existing personal-account behavior.

Acceptance here is a governance decision. The
[F0 milestone](../enterprise/14-roadmap.md#3-entry-exit-and-activation-per-milestone),
[ADR-019](019-android-vps-integration-first.md) sequencing and existing
release gates govern implementation and activation.
