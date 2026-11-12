# Imports, notifications and integrations

Status: **PLANNED / RECONSTRUCTED**, 2026-11-11.
Recovered from the surviving enterprise requirements, shared rules and backlog.
The accepted F0 decisions and existing release gates govern any implementation.

## 1. Purpose

Bring authorized organization data into existing workflows and deliver scoped outcomes without creating parallel business rules or mandatory external providers.

## 2. Scope

F1/F2 provide previewed employee and asset imports and in-app notices where required. Later imports use existing fueling, meter and expense rules. External email, push, telematics, fiscal retrieval, extraction and accounting connections require their own decision, reviewed dependency and activation.

## 3. Entities and ownership

The organization owns import batch, mapped row, row outcome, notification preference, inbox event and integration connection. Secrets stay in the approved protected store and only safe references appear in records. A connection has no authority outside its assigned organization and purpose.

## 4. Business rules

- **BR-IMPORT-001 — Preview precedes apply.** Validate mapped rows and require explicit authorized confirmation. Applying uses the owning command and records each result.
- **BR-NOTIF-001 — Current scoped recipient.** Preferences, current membership and event scope decide delivery. Duplicate events do not create repeated effects.
- **BR-INTEG-001 — Reviewed boundary.** Each integration records permitted data, purpose, provider, secret scope, budget and fallback before activation.
- **BR-INTEG-002 — No secret payloads.** Logs, notification text and retry evidence carry bounded references under BR-PRIV-008.

## 5. Use cases

**UC-IMPORT-01 — Preview and apply.** A permitted member uploads a bounded admitted file, maps columns and reviews valid rows and errors. Confirmation rechecks current scope and applies each eligible row through its owning workflow. Retry preserves accepted row identities.

**UC-NOTIF-01 — Dispatch a notice.** A scoped accepted event identifies eligible recipients. The worker checks preferences and current authority, stores one inbox outcome and uses only enabled approved channels. A failed external send leaves the in-app path available.

**UC-INTEG-01 — Configure a connection.** An authorized member selects an admitted adapter and scope. Secret material is entered through its protected path. Health reports expose safe status. Disabling stops new work without deleting prior facts.

## 6. States and transitions

Import state follows uploaded, parsed, mapped, validated, confirmed, applying and applied or applied-with-errors. Failure and cancellation retain results. Reversal uses the owning correction path and never deletes approved history. Notifications are pending, delivered, failed or suppressed. Connections are configured, active, degraded or disabled. [The state catalog](../08-state-model.md) is canonical.

## 7. Roles and permissions

Use [import, notification and connection permissions](../07-permissions.md). Import confirmation needs the target workflow's create authority. Viewing a preview never grants the authority to apply it. Each report link rechecks the recipient's current resource permission. Platform support requires the approved scope.

## 8. Offline, concurrency, idempotency and corrections

Apply and provider setup require the server. Imported data obeys the same revision, money and scope rules as entered data. Each row and delivery has stable identity. Partial failure records exact outcomes. Retry is bounded for provider work, preserves idempotency and never silently discards durable accepted facts.

## 9. Events, notifications and integrations

Existing PostgreSQL jobs drive apply, delivery and health work. Adapters use typed ports and reviewed allowlists. Manual entry, CSV export and in-app notification remain useful when an optional integration is disabled. No new broker or service is implied.

## 10. Data, formulas and examples

Import values use explicit mapping, units and time zones. Unknown input remains an error or review result. An error report carries authorized row number and safe reason. Notification links contain no document bytes or activation secret. Each provider has recorded limits and costs before enablement, with no guessed production capacity.

## 11. Screens

| ID | Surface | Objective | Actor | Data and actions | Validation |
|---|---|---|---|---|---|
| SCR-IMPORT-01 | PANEL | Preview and confirm mapped rows | Permitted importer | Mapping, row errors and apply summary | Explicit confirmation, keyboard access and no false success |
| SCR-NOTIF-01 | APP, PANEL | Read own notices | Current member | Scoped event and authorized link | Context visible, safe lock-screen behavior and five shared states |
| SCR-INTEG-01 | PANEL | Manage an admitted connection | Authorized manager | Safe metadata, health and disable | Secrets never redisplayed, permission checked on every action |

## 12. Acceptance, dependencies, risks, phase and microtasks

[EP18](../backlog/ep18-imports-notifications-integrations.md) owns the microtasks and phases. Immediate tests cover foreign rows, apply permission loss, malformed files, money/unit conversion, duplicate delivery, partial apply and provider outage. External legal, rights, email/push and fiscal decisions stay explicit prerequisites.
