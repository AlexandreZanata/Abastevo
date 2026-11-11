# Abastevo Empresas product contract

Status: **F0 PRODUCT DECISIONS RECORDED**, 2026-11-10.
[ADR-025](../adr/025-private-abastevo-empresas-organization-boundary.md)
records the maintainer's decision. This contract governs the planned
Abastevo Empresas private fleet context. The
[F0 decision record](../enterprise/evidence/f0-m0-decisions.md) records
the approved panel, Android partition and private receipt scope.

## Business rules

- **B-BR-EMP-01 — Organization ownership.** Each private Empresas record
  belongs to one organization. Legal entities, CNPJs, operating units,
  teams and fleets are managed within that boundary. Ending a membership
  removes access and preserves records owned by the organization.
  [Shared tenancy rules](../enterprise/04-business-rules.md#1-tenancy-and-ownership--br-ten)
- **B-BR-EMP-02 — Current authority.** The server resolves organization
  access from a verified account and an active membership with the
  required permission on the resource. Account or staff status, payment,
  a shared CNPJ root and knowledge of an identifier grant no access.
  [Permission contract](../enterprise/07-permissions.md)
- **B-BR-EMP-03 — Isolated surfaces.** Reads, writes, lists, jobs, searches,
  caches, files, exports and local data carry organization scope. Access
  attempts reveal no foreign record or resource-existence hint. An
  offline operation retains the organization in which it was created.
  [Architecture](../enterprise/09-architecture.md)
- **B-BR-EMP-04 — Product separation.** Public catalog and community
  reads remain tenant-free. Referencing a public station grants no claim
  or write authority. Personal Motoristas and Premium records remain
  account-owned. The individual `ENTERPRISE` tier confers no organization
  privilege. Free participation, source separation and payment neutrality
  continue to apply. [Shared source rules](../enterprise/04-business-rules.md#2-source-separation-and-neutrality--br-src)
- **B-BR-EMP-05 — Company panel.** The company panel is served at
  `/empresas/` on the configured API origin. Its account session is
  separate from admin sessions. Each private request resolves current
  membership and permission. Mutations require exact-origin and CSRF
  checks. [Company web decision](../enterprise/evidence/f0-m0-decisions.md#company-web-surface)
- **B-BR-EMP-06 — Protected Android partition.** Local private data,
  attachments and queued operations are scoped by account, server origin
  and organization, protected by Keystore-backed keys and excluded from
  backup and transfer. An operation keeps its creation scope across retry
  and context changes. Logout locks the partition.
  [Android decision](../enterprise/evidence/f0-m0-decisions.md#android-local-protection)
- **B-BR-EMP-07 — Private fueling receipts.** A receipt belongs to the
  organization that owns the fueling. Member-bound short-lived access
  requires current permission and an audit record. C1 validation uses
  synthetic receipts. Real-receipt intake and retention require an approved
  category-specific policy and existing real-data gates. Human review is
  recorded independently of fiscal status.
  [Receipt decision](../enterprise/evidence/f0-m0-decisions.md#private-receipts)

## Use cases

- **BUC-EMP-01 — Act in an organization.** A verified member selects one
  organization. For each operation the server checks current membership
  and the permission on that resource. A foreign identifier yields no
  foreign data or existence hint. Switching the selected organization
  changes the context of future operations only.
- **BUC-EMP-02 — End membership.** An authorized member ends an employee's
  membership. Subsequent requests and access through replay or queued
  work recheck authority. The organization keeps its records and the
  employee retains access to their personal account according to its own
  contract.
- **BUC-EMP-03 — Attach and review a private receipt.** An authorized
  member attaches a synthetic receipt to a fueling in the same organization
  for C1 validation. A reviewer with the required permission opens it
  through the audited capability. Foreign, expired or revoked access is
  refused. The review state records the reviewer's decision.

Implementation requires the [F0 decision gates](../enterprise/16-decisions.md#5-what-is-actually-blocking).
Real-data use requires separate legal review and the existing production
gates. Tracking, Postos, billing and Data each have separate scope decisions.
