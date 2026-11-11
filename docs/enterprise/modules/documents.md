# Private documents and receipts

Status: **PLANNED / RECONSTRUCTED**, 2026-11-11.
Recovered from the surviving enterprise requirements, shared rules and backlog.
The accepted F0 decisions and existing release gates govern any implementation.

## 1. Purpose

Attach verifiable private evidence to an organization's operational records. Field members capture originals and reviewers inspect only the documents they may read.

## 2. Scope

C1 includes synthetic receipt capture, private storage, scoped links and a viewer. F3 adds structured fields, human conference, allocation and reconciliation. F6 enforces reviewed retention and holds. Real receipts require the [F0 legal decision](../evidence/f0-m0-decisions.md#private-receipts). Fiscal consultation and optional extraction keep separate approvals.

## 3. Entities and ownership

The organization owns each document, upload, original, extraction result, item and allocation. A link references a same-organization operation or private record. One original can support several operations and one operation several documents. File identity and the financial allocation are separate. Public catalog references grant no access.

## 4. Business rules

- **BR-DOC-001 — Independent checks.** Storage, extraction, structure, conference, reconciliation and fiscal consultation remain independent under EDR-014 and BR-TRUTH-001.
- **BR-DOC-002 — Scoped evidence.** Links and content reads require current same-organization authority. A link never grants document-content permission (BR-PRIV-004, BR-AUTH-011).
- **BR-DOC-003 — Exact allocation.** The source item and document totals bound all allocations in their locking transaction. A link, payment or second copy creates no new expense (BR-MONEY-007, EDR-013).
- **BR-DOC-004 — Reviewed retention.** Synthetic fixtures support C1. Real intake needs a reviewed category, period, trigger, hold and erasure policy. A five-year planning assumption is not an active policy.

## 5. Use cases

**UC-DOC-01 — Attach evidence.** An enrolled member with the owning record permission captures a synthetic file, persists its declaration and operation, then uploads on reconnect. The server verifies scope, bounds and digest before stored state. An interruption leaves a recoverable pending attachment. A mismatch is refused without claiming fiscal invalidity.

**UC-DOC-02 — Inspect and confer.** A member with document-content and review permission opens an audited short-lived capability, compares the matching revision and records conference outcome and reason. Expired access, foreign scope and stale decisions fail. Fiscal status remains unchanged.

**UC-DOC-03 — Allocate and reconcile.** An authorized reviewer selects same-organization financial targets and exact amounts. The server locks the source, checks remaining capacity, commits allocations and records the resulting reconciliation basis. A competing over-allocation fails with no partial writes.

## 6. States and transitions

The six status sets are canonical in [the state catalog](../08-state-model.md). Intake moves storage from uploading to received, then stored only after checks, or quarantine/failure. Replacement retains the prior reference. Authorized human review changes conference state on a specified revision. Reconciliation follows committed allocations. A fiscal status requires an authorized consultation and its time. Cancellation inside the app does not certify a fiscal cancellation. Reversal preserves the original and decision history.

## 7. Roles and permissions

Use the [permission registry](../07-permissions.md). Metadata read and content read are distinct. Capture or association requires the owning record's permission and a scoped target. Review, allocation and retention changes require their named managing role and current scope. Support sees content only when an approved support session explicitly includes it.

## 8. Offline, concurrency, idempotency and corrections

The [shared queue](../10-offline-sync.md) seals account, origin, organization and attachment identity. Accepted operations may retain pending bytes. Retries reuse the operation and declared file identity. Each allocation locks the source totals, and edits to reviewed facts use a new revision. Expiry checks deny reads while cleanup is unavailable.

## 9. Events, notifications and integrations

Upload and safety workers consume scoped declarations. Completed storage and changed conference or allocation results notify owning consumers through the [existing job queue](../09-architecture.md). Audit events contain bounded references, never document content. Optional extraction and fiscal retrieval use separately reviewed ports.

## 10. Data, formulas and examples

Use integer centavos and exact source quantities. For a document of 97 350 centavos, allocations of 60 000 and 30 000 leave 7 350. An attempted 8 000 is refused. Of two racing requests for the remaining 7 350 only one commits. Size, digest and media checks follow BR-PRIV-020. Original bytes stay private and immutable until the reviewed retention workflow disposes of them.

## 11. Screens

| ID | Surface | Objective | Actor | Data and actions | Validation |
|---|---|---|---|---|---|
| SCR-DOC-01 | APP | Capture or import synthetic evidence | Authorized field member | Original, legibility, pending state, capture and retry | No success before durable save, accessible labels, missing file remains visible |
| SCR-DOC-02 | PANEL | Inspect linked receipt | Authorized reviewer | Audited content and separate check states | Expired and forbidden content never displayed, keyboard and large-text support |
| SCR-DOC-03 | PANEL | Confer and allocate | Authorized document manager | Revision, remaining centavos, reason and targets | Exact totals, stale conflict and five shared screen states |

## 12. Acceptance, dependencies, risks, phase and microtasks

[EP09](../backlog/ep09-documents.md) owns the microtasks. Real PostGIS must prove scoped references, allocation races, atomic audit and replay. Isolated storage tests cover incomplete uploads, expiry and cleanup failures. C1 is synthetic. The [privacy schedule](../11-privacy-security.md) and [VAL-12](../15-validation.md) supply retention and allocation gates. Open risk remains the category-specific legal decision for real receipts.
