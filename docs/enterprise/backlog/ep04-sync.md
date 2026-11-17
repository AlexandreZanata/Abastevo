# EP04 — Offline synchronization

Task IDs, titles, phase, cut, type, effort, priority and dependencies
follow the existing [generated index](INDEX.md). Task descriptions
were reconstructed from the [offline synchronization specification](../offline-sync.md), [privacy and security](../privacy-security.md) and [validation scenarios](../testing.md).

Local F1 requires M0 closure and protected integration of the current
ADR-019 construction batch. See the
sequencing decision.

The enterprise queue preserves the approved account, server origin and organization partition. C1 attachment validation uses synthetic receipts. The F4 late-intake and device-cleanup tasks retain their separate gates.

### EMP-OFFL-007 — Device clock capture, skew estimation and time fields
- **Meta:** module OFFL · phase F1 · cut C1 · priority P0 · type DOM · size S · state TODO
- **Objective:** Keep device occurrence time, server receipt time and skew separate.
- **Rules and use cases:** BR-OFFL-008, BR-OFFL-009, BR-TIME-001, BR-TIME-002, BR-TIME-003, BR-TIME-006
- **Depends on:** EMP-OFFL-002, EMP-PLAT-015
- **Read first:** [offline synchronization specification](../offline-sync.md) §§4 and 10.1–10.2, [time rules](../business-rules.md#5-time--br-time)
- **Scope:** in: timestamp capture, offset and zone, skew evidence and warning vectors · out: changing authority or rewriting occurrence time
- **Areas:** existing: organization time kernel and operation contract · proposed: deterministic skew calculations and shared vectors
- **Steps:** Pin validity, offsets and skew boundaries. Keep the client declaration and server receipt unchanged and record skew as evidence.
- **Deliverable:** A tested sync time and skew representation.
- **Acceptance:** Authorization uses server time and history. Clock skew stays visible without silently changing a declared timestamp.
- **Negative cases:** Future declaration treated as authority, wrong offset sign, missing or malformed zone, skew without a flag and timestamp overwritten.
- **Validation:** V-DOM, V-SYNC · migration: none beyond the owning intake task · rollback: preserve raw recorded times
- **Risks and assumptions:** Warnings and eligibility for late intake are distinct decisions.
- **Done when:** Boundary vectors prove that client time cannot authorize an operation.

### EMP-OFFL-010 — Context switch, logout and shared-device behaviour with a pending queue
- **Meta:** module OFFL · phase F1 · cut C1 · priority P0 · type APP · size M · state TODO
- **Objective:** Keep pending work safe through Android context switch and logout.
- **Rules and use cases:** BR-OFFL-003, BR-OFFL-013, BR-TEN-004, BR-AUTH-012, BR-PRIV-016, BR-PRIV-017, UC-OFFL-05, UC-OFFL-06, SCR-OFFL-04
- **Depends on:** EMP-OFFL-003, EMP-IDN-018
- **Read first:** [offline synchronization specification](../offline-sync.md) §§4–5 and 11, M0 decisions §Android local protection
- **Scope:** in: context transition, send-or-keep choice and shared-device isolation · out: retagging work or sending under a different identity
- **Areas:** existing: persistent queue, context switch and account logout · proposed: enterprise logout and partition lock wiring
- **Steps:** Exercise switching while work is queued or in flight. Verify explicit logout choices, re-login and a different account on the same device.
- **Deliverable:** A scoped context and logout flow that preserves pending work.
- **Acceptance:** Pending work remains sealed to its original partition. Kept work is locked at logout and available only to its authorized original context.
- **Negative cases:** Wrong account or origin opens old data, late acknowledgement lands in another partition, silent deletion or worker uses replacement credentials.
- **Validation:** V-APP, V-SYNC, V-ISO, V-AUTHZ, V-PRIV · migration: preserve existing held queues · rollback: disable new entry while retaining stored work
- **Risks and assumptions:** Only send when the original session permits it. Explain the local consequences of reset or uninstall.
- **Done when:** Context and logout tests pass with two accounts, two organizations and pending writes.

### EMP-OFFL-014 — Panel: sync review queue
- **Meta:** module OFFL · phase F2 · cut C1 · priority P0 · type WEB · size S · state TODO
- **Objective:** Expose scoped sync review and its actual server outcome in the company panel.
- **Rules and use cases:** BR-OFFL-006, BR-OFFL-009, BR-REV-003, BR-REV-007, BR-AUTH-013, UC-OFFL-04, SCR-OFFL-03
- **Depends on:** EMP-OFFL-008, EMP-PLAT-008
- **Read first:** [offline synchronization specification](../offline-sync.md) §§5–7 and 11, M0 decisions §Company web surface
- **Scope:** in: scoped review list, reason and resolution UI · out: client-side approval authority or showing unrelated records
- **Areas:** existing: /empresas/ scaffold and review API · proposed: sync review queue screens
- **Steps:** Test reviewer scope, keyboard and screen-reader access, concurrent resolution conflict and failed or stale decisions through the panel.
- **Deliverable:** An accessible sync review UI with server-confirmed outcomes.
- **Acceptance:** Reviewers see only permitted items and their reasons. The server resolves once and conflict or refusal is visible.
- **Negative cases:** Foreign data after context switch, offline approval, self-review, hidden conflict and client status implying approval before acknowledgement.
- **Validation:** V-WEB, V-AUTHZ, V-ISO, V-SYNC, V-PRIV · migration: none · rollback: hide the queue UI and preserve server-held work
- **Risks and assumptions:** Only the authorized record-specific review permission enables a resolution.
- **Done when:** The review flow and denied states pass through the company session and normal routes.
