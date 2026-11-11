# Internal administration and scoped support

Status: **PLANNED / RECONSTRUCTED**, 2026-11-11.
Recovered from the surviving enterprise requirements, shared rules and backlog.
The accepted F0 decisions and existing release gates govern any implementation.

## 1. Purpose

Let authorized platform staff review narrow operational cases and support an organization with its approval, while keeping organization-private content protected.

## 2. Scope

F1 defines reviewed staff responsibilities and organization administration. F3 adds support sessions. F7 adds station and legal-entity review queues and their admin UI. F8 adds approved subscription adjustments. Stage D and billing decisions remain prerequisites for their own capabilities.

## 3. Entities and ownership

Staff assignments and admin sessions remain platform-owned. Review cases refer to their owning workflow and organization. A support session records organization approval, staff actor, categories, validity and audit references. Its existence grants no general membership.

## 4. Business rules

- **BR-BACK-001 — Explicit consumer power.** A code-defined staff role has only the permissions enforced by a reviewed consumer under the existing admin contract.
- **BR-BACK-002 — Approved support window.** Private support access requires the exact approved organization, categories and validity at each request (BR-TEN-008, BR-PRIV-021).
- **BR-BACK-003 — Audited sensitive reads.** Every support read is logged as bounded actor, scope and target references. Content, coordinates and secrets never enter the audit.
- **BR-BACK-004 — Separate decisions.** Moderation, station authority, legal verification, subscription status and company approval remain their owning workflows.

## 5. Use cases

**UC-BACK-01 — Request support.** A permitted staff actor requests named categories and time for one organization. An authorized organization member approves or denies. Access begins only under both live staff authority and the approved window. Expiry or end stops access.

**UC-BACK-02 — Review a case.** A permitted reviewer opens the case in the admin suite, checks only authorized evidence and records a revision-bound outcome and reason through the owning command. Competing decisions conflict rather than overwriting.

**UC-BACK-03 — Administer organization state.** A permitted staff actor invokes the owning suspension or reactivation command with its reason. State and audit commit together. The action does not permit arbitrary private-content browsing.

## 6. States and transitions

Support transitions follow requested → approved → active → ended, with denied or expired outcomes. Organization approval, valid staff role and time are checked before each access. Revocation prevents reuse of a prior session. Review case states remain owned by station claims or organization verification. Recorded outcomes are preserved rather than reset through the UI.

## 7. Roles and permissions

Reuse [admin and staff roles](../../product/ADMIN_TIERS_CONTRACT.md) and keep company cookies separate. Reviewed station-review and support actions require specific endpoint checks. Document content and location need their explicit category and permission. A staff label or account tier does not grant membership or a general organization read.

## 8. Offline, concurrency, idempotency and corrections

Admin actions require the online server. Mutations are idempotent and revision-bound. A support session's expiry is checked at read time. Failed audit storage prevents protected reads. Concurrent reviewers commit one outcome. The normal organization and private file boundaries still apply.

## 9. Events, notifications and integrations

The existing admin audit captures each accepted mutation. Review queues consume typed owning-workflow requests. Organization members can inspect support history. Notification payloads contain safe references and links. Manual subscription adjustments use the owning approved billing contract.

## 10. Data, formulas and examples

The support journey in [JRN-13](../05-journeys.md) requests the import area for two hours. Approval permits the named import case only. Its documents and positions remain closed. Duration is explicit approved data, not a blanket maximum. The time source is the server.

## 11. Screens

| ID | Surface | Objective | Actor | Data and actions | Validation |
|---|---|---|---|---|---|
| SCR-BACK-01 | ADMIN | Review a permitted case | Reviewed staff role | Evidence references, revision and reason | Scope, stale decision and accessible deny states |
| SCR-BACK-02 | PANEL | Approve a support request | Authorized organization member | Actor, categories and interval | Explicit consent, no preselection and five shared states |
| SCR-BACK-03 | ADMIN | Act inside a support session | Authorized approved actor | Only permitted case data | Window visible, expiry and forbidden content handled |

## 12. Acceptance, dependencies, risks, phase and microtasks

[EP19](../backlog/ep19-back-office.md) defines the tasks. GOV-004 and applicable Stage D decisions remain required. Test staff/company session confusion, foreign scope, unsupported category, expiry, failed audit, competing decisions and revoked actor immediately. Real-data support requires the reviewed privacy terms.
