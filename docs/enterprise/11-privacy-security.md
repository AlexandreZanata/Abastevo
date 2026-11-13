# Privacy, isolation and security

Status: **PLANNED**, 2026-11-10. Module code `PRIV`. Engineering proposal, **not a declaration of legal compliance**. The points that need legal validation are listed in §10.4 and are a release condition for any pilot with real data (EMP-GOV-005). Tasks: `EMP-PRIV-001…005` in [backlog/ep01-platform](backlog/ep01-platform.md).

Evidence. The existing product is built on short retention: exact contributor GPS capped at 24 hours with "No location-history endpoint" ([SECURITY_PRIVACY](../security/SECURITY_PRIVACY.md)), all photo copies capped at 24 hours (B-BR-M01), raw driver traces kept local (B-BR-DRV-09), and a deletion ledger replayed after restores (`privacy` module, `IMPLEMENTED`). Those rules were written for the community and personal products. An organization's trip tracks and receipts have a different purpose and a different owner; keeping them longer is a real conflict with the wording of those rules and is listed as CX-06 and CX-07 in [16-decisions](16-decisions.md). Nothing here changes the community or personal rules.

## 1. Purpose

Keep each organization's data visible only to that organization's authorized members; collect only what the stated purpose needs; keep it only as long as the stated period; let people exercise their rights; and be honest about what the platform cannot guarantee.

## 2. Scope

- **Basic (F1–F3, cut C1):** tenant isolation on every surface; device-side protection and logout behaviour; audited access to documents; upload limits and safety checks; no personal data in logs.
- **Next (F4–F6):** location-specific controls; retention schedule enforcement; data-subject request routing; backup and restore handling; legal hold.
- **Out of scope:** end-to-end encryption claims, certification claims, and any statement that the platform is "LGPD compliant".

## 3. Entities and ownership

| Entity | Owner | Content | Invariants |
|---|---|---|---|
| Retention policy | Platform default, organization-adjustable inside limits | Category, period, action at expiry | An organization cannot set a period outside the platform's allowed range. |
| Legal hold | Organization (set by `OWNER` or `ADMIN`) | Scope, reason, start, end | Suspends deletion for its scope only; visible and audited; has a mandatory review date. |
| Data-subject request | Organization (as controller) | Requester, kind, scope, state, outcome | Handled by the organization; the platform provides the tooling and the record. |
| Access log entry | Organization | Who opened which sensitive item and why | Part of the audit trail (BR-AUD-003). |
| Local protected store | Device, per (account, origin, organization) | Cached reference data, pending operations, attachment files | Not included in device backups; unreadable by another account on the device. |

### 3.1 Implemented company-session credential inventory

EMP-PLAT-007 stores account/family UUIDs, a company lookup digest, separately
salted cookie/CSRF verifiers, authentication/creation/activity/expiry times
and optional revocation time in `company_web_sessions`. No raw account
proof, cookie, CSRF, contributor GPS, receipt or organization content is
retained there. Result/proof/record types redact default and structured
logging. Account/family deletion cascades to these credentials.

Read-time limits are 30-minute idle, at most eight-hour absolute and current
active account/live source family; fresh actions use a fixed 15-minute
window. The existing daily privacy job removes invalid rows in bounded
batches, even with the company surface off. This technical credential
lifecycle does not resolve real-receipt retention or legal-review gates.
[Operational cleanup](../operator/retention.md#per-category-behavior) and
[local security evidence](evidence/f1-plat007-company-web-session.md).

## 4. Business rules

Isolation:

- **BR-PRIV-001 — Isolation is layered.** Scoped queries are the first control; composite keys and row-level security are the second; object-storage key prefixes per organization and audited access are the third. No layer is trusted alone.
- **BR-PRIV-002 — Every surface is tested with two organizations.** Reads, lists, writes, exports, jobs, caches, search and file access are each attempted across organizations and must fail without revealing data (validation profile `V-ISO`).
- **BR-PRIV-003 — Caches and jobs carry the organization.** A cache key or a job payload for organization data always includes the organization identifier; shared public caches never hold organization data; organization responses are `no-store`.
- **BR-PRIV-004 — Files are private and links are short.** Documents and exports are stored under an organization prefix in private storage. Content is delivered through an API capability that expires in at most 5 minutes, is bound to the requesting member and document, and is audited. No storage URL is ever given to a client (existing ingress pattern).

Minimisation and purpose:

- **BR-PRIV-005 — Collect for a stated purpose.** Each personal field of an employee, driver or representative has a purpose listed in the module that defines it. Fields without a purpose are not collected. Documents such as a driving licence are stored only when the organization enables the control that needs them.
- **BR-PRIV-006 — No personal trips.** Location is recorded only inside a tracking session of an organization trip that the employee started in enterprise mode, with a visible indicator, and stops when the trip pauses or ends ([modules/gps-navigation](modules/gps-navigation.md) BR-GPS). Personal mode never feeds organization data.
- **BR-PRIV-007 — Free text is bounded and guided.** Notes, reasons and incident descriptions have length limits and a hint not to enter health or other sensitive details about people.
- **BR-PRIV-008 — Logs carry identifiers, not content.** Logs, metrics, traces, events and error messages never contain document content, coordinates, tokens, activation codes, voucher codes, names or contact data. Metric labels never use unbounded identifiers.

Retention and deletion:

- **BR-PRIV-009 — Every category has a period.** The schedule in §10.1 applies. Expiry deletes content and files, or reduces them to the stated minimal form. Expiry is checked at read time as well, so a stopped job cannot expose expired data.
- **BR-PRIV-010 — Retention never becomes "keep forever".** Audit and approved records are kept for their period and then deleted or reduced. A legal hold is the only extension and has a review date.
- **BR-PRIV-011 — Restores respect deletion.** After restoring a backup, expired and deleted organization data is purged again before the system serves traffic, using a deletion ledger kept outside the restored data (existing practice for personal data).
- **BR-PRIV-012 — Closing an organization.** On closure the organization can export its data, then access ends; data is deleted at the end of the closure period unless a category has a longer stated period or a legal hold ([modules/organizations](modules/organizations.md)).

People's rights:

- **BR-PRIV-013 — The organization answers for its employees' data.** A request from an employee about data in an organization is handled by that organization. The platform gives it tools to find, export, correct and restrict that person's data, and records the request.
- **BR-PRIV-014 — Personal account deletion does not erase organization records.** Deleting a personal account ends its memberships and replaces the account link with a tombstone. Employee records and the operations that person recorded stay with the organization under its schedule (BR-TEN-009). The person is told this before deleting.
- **BR-PRIV-015 — Employees can see what is collected.** In enterprise mode the employee can always see what the organization collects, when tracking is active, and their own tracking sessions.

Devices:

- **BR-PRIV-016 — Local data is protected and partitioned.** Enterprise data on the device is stored per (account, origin, organization), encrypted with a key held in the Android Keystore, and excluded from backups (existing backup rules exclude the database and session stores).
- **BR-PRIV-017 — Logout and shared devices.** Logging out removes the session and locks the local partition. Pending operations follow [10-offline-sync](10-offline-sync.md) UC-OFFL-06. Another account on the same device never sees the partition.
- **BR-PRIV-018 — Lost devices.** Revoking a device or session takes effect on the server immediately. The local partition is erased the next time the app reaches the server, or when its offline validity (default 30 days without contact) expires and the key is discarded. The product never promises immediate remote erasure of a disconnected device (BR-OFFL-014).
- **BR-PRIV-019 — App lock for corporate access.** A managed account requires a local unlock (device credential or biometric) after inactivity. This protects the device session; it is not a server credential.

EMP-PRIV-003's [F2 continuation](evidence/f2-priv003-protection-completion.md)
adds exact signed custody, head-bound encrypted generations and durable scoped
maintenance. Security loss erases; ended access and observed clock/reboot
reconfirmation preserve ciphertext. Generic errors cannot issue wipe commands.
Host crypto/process and ordinary PostGIS/signed HTTP proof remain separate
from actual Android Keystore, credential, backup/transfer, reboot and restore
acceptance. Actual API36 emulator/source acceptance passes and the application
binds the accepted protected store and signed client. Managed server policy,
legal retention and production
activation retain their own gates. The [earlier source record](evidence/f1-priv003-protected-device-store.md)
is historical evidence, not a completed native-acceptance claim.

Uploads and platform operators:

- **BR-PRIV-020 — Uploads are bounded and inspected.** Type is decided by content, not by name. Limits (defaults): image 5 MiB, PDF 10 MiB, XML 1 MiB. PDF with active content or encryption, XML with external entities, archives and executables are refused. Originals are preserved unmodified once accepted.
- **BR-PRIV-021 — Support access is exceptional.** A platform operator opens organization content only inside an approved support session ([modules/back-office](modules/back-office.md)); document content and location need the session to name them explicitly.

## 5. Use cases

**UC-PRIV-01 — Answer an employee's data request.** Actor: `OWNER` or `ADMIN`. Flow: open the request tool → choose the employee and the kind (access, correction, restriction) → the tool lists that person's data by category → export or correct → record the outcome. Alternatives: the person is also a member of other organizations → only this organization's data is shown; data under legal hold or mandatory retention → shown with the reason it cannot be deleted. Result: request recorded with what was done.

**UC-PRIV-02 — Apply the retention schedule.** Actor: system job. Flow: for each category, find items past their period and not under hold → delete content and files or reduce to the minimal form → write the deletion ledger. Alternatives: storage failure → item stays listed for retry and is still unreadable (read-time check); job stopped → read-time check still hides expired items. Result: expired data is gone or reduced.

**UC-PRIV-03 — Lost phone.** Actor: manager with `idn.credential.manage`, or the user from another device. Flow: revoke the device as lost → server refuses it at once → operations it sends later are refused (`DEVICE_REVOKED`) → if the phone reconnects, the app erases the partition. Alternative: the phone never reconnects → the partition becomes unreadable when its offline validity expires. Result: no further access; the manager is told that unsent records on that phone are lost.

**UC-PRIV-04 — Set a legal hold.** Actor: `OWNER` or `ADMIN`. Flow: choose scope (records of a period, an employee, an asset) → reason and review date → deletion is suspended for that scope → reminders before the review date. Result: hold visible and audited.

**UC-PRIV-05 — Open a sensitive item.** Actor: member with the specific permission. Flow: request the content of a document or a track → server checks permission and scope → writes the access log → returns a short-lived capability. Result: content shown; access recorded.

## 6. States and transitions

| Entity | States | Notes |
|---|---|---|
| Data-subject request | `RECEIVED` → `IN_PROGRESS` → `ANSWERED` or `REFUSED_WITH_REASON` | Reason required for a refusal; due date shown. |
| Legal hold | `ACTIVE` → `RELEASED`; `ACTIVE` → `REVIEW_DUE` → `ACTIVE` or `RELEASED` | Release needs a reason. |
| Retained item | `LIVE` → `EXPIRED_PENDING_PURGE` → `PURGED` or `REDUCED` | `EXPIRED_PENDING_PURGE` is already unreadable. |
| Local partition | `UNLOCKED` ↔ `LOCKED` → `ERASED` | Erased on revocation contact or offline-validity expiry. |

## 7. Roles and permissions

`org.settings.manage` sets retention inside limits and legal holds. `idn.employee.manage` runs data-subject requests. `doc.document.view_content`, `gps.location.read_live` and `gps.track.read` gate sensitive reads (BR-AUTH-011). `audit.event.read` reads access logs. Platform operators act only through support sessions.

## 8. Offline, concurrency, idempotency and corrections

Retention jobs are safe to re-run and keyed by item; a restore replays the deletion ledger before serving. Revocation is evaluated on every request. Device-side erase is idempotent. A correction requested by a data subject is an amendment when the record is approved (BR-AMD).

## 9. Events, notifications and integrations

Produced: `priv.item.purged`, `priv.hold.created`, `priv.hold.released`, `priv.request.answered`, `priv.sensitive.accessed`. Consumed: `org.organization.closed`, `idn.membership.ended`, `idn.device.revoked`, existing account-deletion events. Notifications: upcoming purge of categories the organization may want to export; holds reaching their review date.

## 10. Data, formulas and examples

### 10.1 Retention schedule (defaults; every period needs legal validation)

| Category | Purpose | Default period | Range the organization may set | At expiry |
|---|---|---|---|---|
| Raw track fixes | Reconstruct a trip and compute GPS distance | 90 days after the trip ends | 30 to 365 days | Deleted; simplified geometry and summary stay with the trip |
| Live position | Show operations in progress | Until the trip ends plus 24 hours | fixed | Deleted |
| Simplified track and trip summary | Operational history, findings | Same as the trip | — | With the trip |
| Trips, logbook, fuelings, expenses, work orders | Operational and cost history | 5 years after the end of the calendar year | 2 to 10 years | Deleted |
| Receipts and fiscal documents | Support for costs | Pending legal review by category. Five years after calendar-year end is a hypothesis for review. | Pending legal review | Files deleted or reduced to approved minimal metadata under the recorded policy |
| Meter readings | Consumption and maintenance | Same as the asset's records | — | With the asset's records |
| Employee record | Employment relation with the organization | While any membership or open record exists, then 5 years | 2 to 10 years | Contact data removed first; name and registration number kept only inside retained records |
| Driving licence data and other personal documents | Eligibility control | Until membership ends plus 90 days | 0 to 365 days | Deleted |
| Invitations and activation codes | Access provisioning | 30 days after a terminal state | fixed | Deleted; audit event stays |
| Operations and acknowledgements | Proof of who recorded what | Same as the resulting record | — | With the record |
| Review items | Decisions about held operations | Same as the resulting record; refused items 1 year | — | Deleted |
| Audit events | Accountability | 5 years | 2 to 10 years | Deleted |
| Export files | Delivery of a requested export | 7 days | 1 to 30 days | Deleted |
| Support session records | Accountability of platform access | 5 years | fixed | Deleted |
| Agreement transactions | Commercial settlement between two organizations | 5 years after the end of the calendar year | fixed | Deleted for both parties |
| Station claim evidence | Representation review | Existing rule: while the grant is active plus 2 years; denied or withdrawn 90 days; scans 24 hours | fixed (STATION_PROFILE_CONTRACT_FREEZE) | Deleted |
| Data API usage records | Billing and abuse control | 13 months | fixed | Aggregated |

### 10.2 Proposed roles under the LGPD (to be validated)

| Data | Proposed controller | Proposed operator |
|---|---|---|
| Employee, driver, trip, track, fueling, document, expense data of an organization | The organization | Abastevo (the legal entity named in the public privacy page, `landing/static/privacidade/index.html`) |
| Account, session, device and security data of the platform | Abastevo | — |
| Billing data of the organization as a customer | Abastevo | Payment provider |
| Station claim evidence | Abastevo (review of representation) | — |
| Public community data | Abastevo, as today | — |

Any use of organization data for a platform purpose (for example market statistics) makes Abastevo a controller for that use and is blocked by BR-SRC-005 unless separately decided.

### 10.3 Isolation checklist per surface

| Surface | Control | Test |
|---|---|---|
| Database | `org_id` on every table, composite foreign keys, row-level security, queries that take an organization scope | Cross-organization read and write attempts on every repository method |
| API | Organization in the path; membership resolved on the server; not-found for other organizations | Every route called with another organization's identifiers |
| Cache | Organization responses `no-store`; no shared cache for private data | Response headers asserted; edge cache rules reviewed |
| Jobs | Payload carries the organization; handler loads through scoped queries | Job run with a payload naming organization A and data of B |
| Search and lists | Filters always include the organization; cursors are sealed with it | Cursor of A replayed as B |
| Files | Key prefix per organization; capability bound to member and document | Capability of A used by a member of B; expired capability |
| Exports | Built inside one organization's scope; file under its prefix; link bound to the requester | Export job of A read by B; link after expiry |
| Notifications | Addressed to memberships; content names the organization | Member of two organizations receives each with the right label |
| Logs and metrics | Identifiers only; no content | Log scan in tests for seeded secrets and coordinates |
| Backups | Restored data purged by the deletion ledger before serving | Restore drill with expired items |

### 10.4 Points that need legal validation

| ID | Question | Why it matters | Proposed starting position |
|---|---|---|---|
| LV-01 | Legal basis for processing employee location during work trips | Consent is fragile in employment; legitimate interest needs a documented test | Organization as controller documents purpose and necessity; tracking limited to trips; transparency to the employee |
| LV-02 | Controller and operator roles, and the data-processing terms between Abastevo and each organization | Defines who answers requests and incidents | Table in §10.2; processing terms accepted at organization creation |
| LV-03 | Notice to employees about what is collected | Transparency duty | In-app notice at first entry in enterprise mode and at each tracking start |
| LV-04 | Retention periods in §10.1, especially documents and tracks | Fiscal and labour limitation periods differ; over-retention is a risk | Defaults in §10.1 until reviewed |
| LV-05 | Whether the operational logbook can be mistaken for a journey or time record | Lei 13.103/2015 names "diário de bordo" as a journey-control instrument; Portaria MTP 671/2021 regulates electronic time registration | Product text and terms state that it is not a time or journey record; no journey computation is offered |
| LV-06 | Location and logbook data as evidence in labour disputes | Courts have accepted geolocation as evidence | Organizations are told; exports are audited |
| LV-07 | International transfer when storage or providers are outside Brazil | Resolução CD/ANPD 19/2024 | Prefer Brazilian region where offered; otherwise standard clauses before real data |
| LV-08 | Incident handling between operator and controller | Resolução CD/ANPD 15/2024: three business days | Platform notifies the organization without delay; roles in the processing terms |
| LV-09 | Storing driving licence and other personal documents | Minimisation | Off by default; enabled per organization with a stated control |
| LV-10 | Sensitive data inside incident descriptions and photos | Health data may appear after an accident | Guidance text; no dedicated health fields; restricted access |
| LV-11 | Receipt retention versus the existing 24-hour photo rule | Existing rule covers "all app-owned photos and scanned images" | Private C1 receipts are approved for synthetic validation. Real-receipt intake requires a reviewed category-specific retention policy. See [EMP-GOV-002](evidence/f0-m0-decisions.md#private-receipts). |
| LV-12 | Station representative evidence when the claimant is an organization | Existing retention is account-based | Keep existing periods; map the holder to the organization |
| LV-13 | Commercial reuse of public ANP data in Data products | Source licence terms | Blocked until rights are recorded (EMP-GOV-016) |

## 11. Screens

| ID | Surface | Objective | Actor | Data | Actions | Rules and permissions | Validations and states |
|---|---|---|---|---|---|---|---|
| SCR-PRIV-01 | APP | Tell the employee what the company collects | Any member, first entry in enterprise mode and on demand | Categories collected, when tracking is active, who can see what, retention summary | Acknowledge, open own tracking sessions | BR-PRIV-006, 015 | Plain language; readable at large font sizes; no pre-ticked boxes. |
| SCR-PRIV-02 | PANEL | Set retention inside limits and see the schedule | `org.settings.manage` | Categories, defaults, chosen periods, next purge | Change period, export before purge | BR-PRIV-009, 010 | Values outside the range are refused with the allowed range shown. |
| SCR-PRIV-03 | PANEL | Handle a person's data request | `idn.employee.manage` | Person's data by category, holds and mandatory retention | Export, correct, restrict, record outcome | BR-PRIV-013 | Empty: no data for this person. Shows what cannot be deleted and why. |
| SCR-PRIV-04 | PANEL | Manage legal holds | `org.settings.manage` | Holds, scope, reason, review date | Create, release | BR-PRIV-010 | Review date mandatory. |
| SCR-PRIV-05 | APP | Unlock corporate access | Managed account holder | — | Device credential or biometric | BR-PRIV-019 | After repeated failures, asks for the device credential; never shows organization data behind the lock. |
| SCR-PRIV-06 | PANEL | See who opened sensitive items | `audit.event.read` | Access log filtered by item, person, period | Filter, export | BR-AUD-003 | Empty state explains what is logged. |

## 12. Acceptance, dependencies, risks, phase and microtasks

Acceptance criteria:

1. The isolation checklist in §10.3 passes for every module that stores organization data.
2. A document capability cannot be used after 5 minutes, by another member, or for another document.
3. With the retention job stopped, an expired item is still unreadable.
4. After a restore drill, items purged before the backup are not served.
5. Deleting a personal account ends its memberships, keeps organization records and shows the tombstone in place of the account.
6. A seeded secret, coordinate and document fragment never appear in logs during the test suites.
7. A revoked device erases its partition at next contact and is refused before that.

Dependencies: EMP-GOV-002 (document retention decision), EMP-GOV-005 (legal validation), EMP-GOV-008 (device storage decision), existing `privacy` module (deletion ledger).

Risks: over-retention by default (ranges and review); managers using location to watch people rather than operations (BR-PRIV-006, access log, employee transparency); unsent data on lost devices (stated openly); legal positions changing after validation (schedule is configuration, not code).

Phase: F1 device protection and access log; F6 retention enforcement, request routing and restore handling. Microtasks: EMP-PRIV-001 to EMP-PRIV-005.
