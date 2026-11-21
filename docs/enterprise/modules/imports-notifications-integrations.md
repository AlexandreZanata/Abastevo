# Imports, notifications and integrations

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

Import state follows uploaded, parsed, mapped, validated, confirmed, applying and applied or applied-with-errors. Failure and cancellation retain results. Reversal uses the owning correction path and never deletes approved history. Notifications are pending, delivered, failed or suppressed. Connections are configured, active, degraded or disabled. [The state catalog](../state-model.md) is canonical.

## 7. Roles and permissions

Use [import, notification and connection permissions](../permissions.md). Import confirmation needs the target workflow's create authority. Viewing a preview never grants the authority to apply it. Each report link rechecks the recipient's current resource permission. Platform support requires the approved scope.

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

The in-app notification core (EMP-NOTIF-001, module `orgnotify`, migration 000204) follows B-BR-EMP-215..218 and EDR-150/151: producer-keyed de-duplication, current authority at dispatch, per-kind preferences with mandatory kinds, hourly limits recorded as suppression, in-app channel only.

### Company connection registry

An integration approval binds a company, adapter, optional resource, payload
budget, review and readiness instant. An opaque lookup reference grants
no access by itself. Credential values live in the owning provider's
protected provisioning path.

The company commands configure, activate and disable a connection using
current `integ.connection.manage` authority. Configuration and activation
also require the current `integrations` capability. A connector operation
checks current company state, the exact connection version and the newest
approval under retained locks. A new approval revision can withdraw the
previous connection's authority without erasing its metadata. Listing
returns the provider, recorded state and whether its approval is still
current. It omits the credential and review references.

The vendor-neutral telematics port accepts signed quality observations for
the reviewed vehicle while its company trip and driver recording are
current. It keeps one scoped receipt with provider-attributed counts and
uses the existing company trip lock. The adapter accepts quality categories
and observation times. Phone fixes and live positions continue through the
existing device intake. Vendor transport and credential provisioning need
their recorded external approvals.

### Fuel card statement reconciliation

The company routes accept the reviewed `fuel-card-csv-v1` mapping for an
active connection bound to one opaque fuel card account. The mapped
columns are `transaction_id,fueling_id,occurred_at,currency,paid_centavos`.
Amounts are positive integer BRL centavos and times use canonical UTC
instants. An optional fueling candidate links only when its current
review is approved and its company payer, card method, instant and amount
match exactly. Other rows retain a review outcome without a foreign or
unverified fueling reference.

The preview returns row numbers, amounts, outcomes and a short-lived
proof. Confirmation resends the same mapped file and requires current
`import.batch.run` and `fuel.fueling.review` authority. A matching retry or
overlapping file never creates a second reconciliation row. Raw statement
files and card numbers are outside the stored record. A review item or
link from the statement has no independent expense effect.

### Outbound fleet webhooks

EMP-INTEG-006 applies REQ-17.3, BR-PRIV-003, BR-PRIV-008 and
BR-AUD-001 to vehicle deactivation and reactivation. A reviewed destination
pins the exact connector approval revision and vehicle. An organization
manager activates the connection with `integ.connection.manage`. Each
committed transition of the approved vehicle schedules one independent
delivery per authorized endpoint. A rejected or replayed command schedules
no new delivery. The version 1 JSON contains only the delivery ID, event
code, vehicle ID and UTC event time. The receiver uses the delivery ID for
idempotency. Account identities, reasons, credentials and other private
records are outside the contract.

The worker proves current organization, capability, connection and endpoint
authority at the durable claim. It signs the stable body with HMAC-SHA256
using the reviewed credential reference. HTTPS is restricted to the exact
reviewed destination and public addresses. The worker refuses redirects and
records only bounded outcomes. Delivery can be attempted three times. An
interrupted send may have reached the receiver before the outcome was
recorded, so receivers must deduplicate the delivery ID. The default worker
configuration has no key directory. Recorded provider approval and key
provisioning precede activation.

### Accounting export layout

EMP-INTEG-007 uses the existing private export worker and integer BRL
centavos (BR-RPT-002, BR-RPT-008, EDR-013). Competence and cash are distinct
kinds. When present, `period_start` and `period_end` are inclusive calendar
dates. Competence uses the stored reporting day of the approved operating
cost, including the independent review of a fueling expense. Cash groups
settled statements by their payment day in the organization time zone. The export cutoff is the recorded request instant. Manual exports may
omit either limit. A connector needs a fixed period and explicit layout
version, plus current authorization for the exact generated file. Delivery of
the same file retains its export ID and checksum.

A reviewed `accounting-csv-v1` connection binds one `layout_version: 1`
export by ID. The API checks the current approved connection, the original
requester's current company-wide permissions and the approval's size and
row limits. The consumer presents a bearer proof in the form
`<credential_uuid>.<lowercase-hex-hmac>`. The HMAC-SHA256 input is the UTF-8
sequence `abastevo-accounting-v1`, organization UUID, connection UUID and
export UUID separated by line feeds, with no final line feed. A repeated
request may use the returned checksum ETag. It carries the same bytes and
file ID, and creates no accounting entry. The standard manual download
remains available with its existing company checks.

To qualify a receiver, provision the reviewed credential in the API's
private `ANPFUEL_ENTERPRISE_ACCOUNTING_KEY_ROOT` directory. A file named
for the approved credential UUID contains exactly 32 raw random bytes.
Withhold the key directory until the recipient and data contract have
recorded approval. The connector uses a different credential directory
from outbound vehicle webhooks.

### Fiscal lookup and server extraction readiness

EMP-INTEG-004 and EMP-INTEG-005 can retain a scoped provider registration
while the reviewed adapter is pending. Required evidence includes the
organization's authorized fiscal route and certificate custody for fiscal
consultation, and the processor, retention and dependency review for
server extraction. A registration can stay `CONFIGURED` in that period.
Activation requires installed transport. Structural checks, personal OCR
and manual entry retain their own status, with fiscal consultation shown
as `NOT_CONSULTED` until an authorized source returns a result.

The fiscal recorder retains the source status code and response digest, scoped
to the stored document and the current reviewed connection. Its transaction
records one result and advances the document's fiscal revision together. The
serving API and worker cannot write consultation results with their ordinary
database login. A qualified provider still needs its own permitted route,
certificate custody and recorded authority before provisioning a recorder.
The provider adapter must use the existing integration entitlement and
connector authorization before accessing the source. The migration preserves
historical observations in place and creates no receipts for them.
