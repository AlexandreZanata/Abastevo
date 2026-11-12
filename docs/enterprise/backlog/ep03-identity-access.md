# EP03 — Identity, employee accounts and authorization

Status: **IN_PROGRESS**, 2026-11-11.

Task IDs, titles, phase, cut, type, effort, priority and dependencies
follow the existing [generated index](INDEX.md). Task descriptions
were reconstructed from [identity specification](../modules/identity.md) and the [permissions specification](../07-permissions.md).

Local F1 requires M0 closure and protected integration of the current
ADR-019 construction batch. See the
[sequencing decision](../evidence/f0-m0-decisions.md#roadmap-sequencing).

C1 uses invitations for existing accounts. Managed corporate activation and its controls remain outside C1 and follow the credential decision and owning API tasks.

### EMP-IDN-001 — Identity domain: person, employee, driver profile, membership, credential, device
- **Meta:** module IDN · phase F1 · cut C1 · priority P0 · type DOM · size M · state LOCAL_DONE
- **Objective:** Separate identity, employee history, membership, credentials and devices.
- **Rules and use cases:** BR-IDN-001, BR-IDN-002, BR-IDN-003, BR-IDN-004, BR-IDN-007, BR-IDN-019, BR-IDN-033
- **Depends on:** EMP-PLAT-001
- **Read first:** [identity specification](../modules/identity.md) §§3–4 and 6, [permissions specification](../07-permissions.md) §4
- **Scope:** in: pure identities, associations and allowed transitions · out: storage and session issuance
- **Areas:** existing: account identity and organization scope types · proposed: identity domain package
- **Steps:** Pin employee and membership lifecycle vectors. Keep credential and device states independent of grants.
- **Deliverable:** A pure identity domain contract and regression vectors.
- **Acceptance:** An employee can exist without access. Organization memberships are independent and no credential confers a role.
- **Negative cases:** An employee record creates a session, an ended membership reopens, or an identifier confers authority.
- **Validation:** V-DOM · migration: none · rollback: remove the unused domain registration before consumers
- **Risks and assumptions:** Reuse the account identity boundary. Persistence proves concurrent uniqueness in its own task.
- **Done when:** Lifecycle and scope vectors pass without storage or network dependencies.

[Local identity/lifecycle evidence](../evidence/f1-idn001-identity-domain.md).

### EMP-IDN-002 — Employee records that exist without a login
- **Meta:** module IDN · phase F1 · cut C1 · priority P0 · type API · size M · state TODO
- **Objective:** Let an authorized organization maintain employee records without creating logins.
- **Rules and use cases:** BR-IDN-002, BR-IDN-003, BR-IDN-006, BR-IDN-034, BR-IDN-036, UC-IDN-01
- **Depends on:** EMP-IDN-001, EMP-ORG-002, EMP-AUTH-003
- **Read first:** [identity specification](../modules/identity.md) §§3–5, [permissions specification](../07-permissions.md) §7
- **Scope:** in: scoped register, minimal fields and duplicate handling · out: credentials and driver eligibility
- **Areas:** existing: organization, authorization and sqlc conventions · proposed: employee repository and routes
- **Steps:** Cover the permission matrix, duplicate registration and likely-duplicate warning. Persist the employee without provisioning access.
- **Deliverable:** A scoped employee register API and persistence tests.
- **Acceptance:** The record exists without a membership. Exact registration duplicates are rejected and likely matches prompt explicit review.
- **Negative cases:** Foreign unit or employee, missing permission, concurrent duplicate or automatic merge.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM, V-PRIV · migration: additive employee storage · rollback: disable admission and preserve employee history
- **Risks and assumptions:** Lifecycle changes that affect access use the corresponding membership guard.
- **Done when:** The register works with and without login through the normal entry points.

### EMP-IDN-003 — Membership persistence and states
- **Meta:** module IDN · phase F1 · cut C1 · priority P0 · type DB · size S · state LOCAL_DONE
- **Objective:** Persist scoped membership periods and deny duplicate live memberships.
- **Rules and use cases:** BR-IDN-004, BR-IDN-007, BR-IDN-030, BR-IDN-033, BR-IDN-042, BR-TEN-009
- **Depends on:** EMP-IDN-001, EMP-ORG-002
- **Read first:** [identity specification](../modules/identity.md) §§3, 4, 6 and 8, [privacy and security](../11-privacy-security.md) §4
- **Scope:** in: state and live uniqueness per organization, minimal account-erasure safeguard · out: invitations, offboarding UI and full erasure workflow
- **Areas:** existing: account lifecycle and organization root · proposed: membership schema, adapter and transaction hooks
- **Steps:** Prove uniqueness under concurrent writes and preserve ended periods. Wire the deletion safeguard without deleting organization records.
- **Deliverable:** An organization-scoped membership repository and upgrade evidence.
- **Acceptance:** One live period per employee and account per organization. Ending access preserves the employee and organization history.
- **Negative cases:** Foreign membership, overlapping live periods, role access without active state or an account-deletion path bypassing last-owner protection.
- **Validation:** V-SQL, V-ISO, V-IDEM, V-PRIV · migration: additive with populated fixtures · rollback: previous image and history preservation
- **Risks and assumptions:** A separate F6 task owns the complete personal account erasure workflow.
- **Done when:** Membership state, races and the account lifecycle safeguard pass on real PostGIS.

[Local persistence, erasure fencing and upgrade evidence](../evidence/f1-idn003-membership-persistence.md).

### EMP-IDN-004 — Invitations: issue, expire, resend, revoke
- **Meta:** module IDN · phase F1 · cut C1 · priority P0 · type API · size M · state TODO
- **Objective:** Issue, expire, replace and revoke invitations under organization authority.
- **Rules and use cases:** BR-IDN-005, BR-IDN-008, BR-IDN-010, BR-IDN-012, BR-IDN-013, BR-IDN-014, BR-IDN-015, BR-IDN-018, UC-IDN-02, UC-IDN-04
- **Depends on:** EMP-IDN-002, EMP-IDN-003
- **Read first:** [identity specification](../modules/identity.md) §§4–5 and 8, [permissions specification](../07-permissions.md) §4
- **Scope:** in: invitation state, single display, role scope and manager delivery · out: provider email and managed codes
- **Areas:** existing: account proof, employee and membership storage · proposed: invitation state, secret storage and manager commands
- **Steps:** Cover issuance, supersession and revocation races. Keep the secret out of persistence in recoverable form, logs, events and status responses.
- **Deliverable:** A scoped invitation lifecycle with manager-mediated delivery.
- **Acceptance:** One usable invitation exists per employee. The issuer sees its secret once, and an expired or replaced invitation cannot be redeemed.
- **Negative cases:** Foreign employee, prohibited role, invalid issuer state, duplicate reissue and secret disclosure.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM, V-PRIV · migration: additive invitation state · rollback: stop issuance and preserve audit history
- **Risks and assumptions:** The public redemption result is uniform. Issuer status is available only in the authorized organization.
- **Done when:** Issuer lifecycle, scope and secrecy checks pass with synthetic values.

### EMP-IDN-005 — Invitation acceptance by an existing Abastevo account
- **Meta:** module IDN · phase F1 · cut C1 · priority P0 · type API · size M · state TODO
- **Objective:** Allow an existing personal account to accept or decline an authorized invitation.
- **Rules and use cases:** BR-IDN-009, BR-IDN-011, BR-IDN-012, BR-IDN-015, BR-IDN-016, BR-IDN-017, BR-IDN-033, UC-IDN-03, UC-IDN-04
- **Depends on:** EMP-IDN-004
- **Read first:** [identity specification](../modules/identity.md) §§4–5, 8 and 10, [permissions specification](../07-permissions.md) §4
- **Scope:** in: preview, confirmation, acceptance and authenticated retry · out: managed activation and email provisioning
- **Areas:** existing: personal account session and invitation lifecycle · proposed: invitation acceptance and confirmation endpoints
- **Steps:** Exercise consent, role revalidation and single use in one transaction. Verify same-operation replay and concurrent attempts against the same invitation.
- **Deliverable:** A personal-account invitation flow with auditable role assignment.
- **Acceptance:** A valid account sees the proposed scope before accepting. Privileged access waits for confirmation and redemption produces one result.
- **Negative cases:** Expired, replaced or consumed invitation, stale issuer rights, duplicate member, managed account and a replay by the wrong caller.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM, V-PRIV · migration: preserve invitation and membership history · rollback: disable redemption and retain recorded outcomes
- **Risks and assumptions:** The same invalid result covers the invalid states specified by the identity contract.
- **Done when:** Acceptance, confirmation, time boundaries and transaction races pass on real PostGIS.

### EMP-IDN-006 — Managed corporate credential: activation code issuance
- **Meta:** module IDN · phase F1 · cut - · priority P1 · type API · size M · state TODO
- **Objective:** Define managed activation-code issuance after the separate credential review.
- **Rules and use cases:** BR-IDN-008, BR-IDN-010, BR-IDN-011, BR-IDN-013, BR-IDN-019, BR-IDN-020, BR-IDN-022, UC-IDN-05
- **Depends on:** EMP-IDN-002, EMP-IDN-003, EMP-GOV-013
- **Read first:** [identity specification](../modules/identity.md) §§4–5 and 10, [privacy and security](../11-privacy-security.md) §4
- **Scope:** in: scoped one-use code issuance and state · out: C1 activation and known manager credentials
- **Areas:** existing: organization identity and token lifecycle · proposed: managed code issuer and policy checks
- **Steps:** Use the approved managed-credential design when available. Prove fresh authentication, code replacement, expiry and limits with synthetic data.
- **Deliverable:** A gated corporate-code issuer with audited scope.
- **Acceptance:** Only an authorized issuer can create one usable code for an eligible employee. Issuance cannot assign owner rights.
- **Negative cases:** Live personal membership, stale proof, superseded code, prohibited role or code in an event.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM, V-PRIV · migration: additive code state · rollback: disable issuance and preserve auditable records
- **Risks and assumptions:** EMP-GOV-013 governs the security decision. Existing-account invitations remain the C1 path.
- **Done when:** The reviewed issuer contract and its negative cases pass before managed access is enabled.

### EMP-IDN-007 — Managed credential activation on the device: key enrolment and local lock
- **Meta:** module IDN · phase F1 · cut - · priority P1 · type API · size M · state TODO
- **Objective:** Bind managed account activation to the activating Android device.
- **Rules and use cases:** BR-IDN-009, BR-IDN-012, BR-IDN-019, BR-IDN-020, BR-IDN-021, BR-IDN-022, BR-IDN-023, UC-IDN-06
- **Depends on:** EMP-IDN-006
- **Read first:** [identity specification](../modules/identity.md) §§4–5 and 10, [privacy and security](../11-privacy-security.md) §4
- **Scope:** in: authorized code redemption, device enrolment and session boundary · out: personal account access, web login and C1
- **Areas:** existing: Android Keystore, device proof and account session conventions · proposed: managed activation transaction
- **Steps:** Test a lost response, competing activations and failure to commit an enrolment. Verify server proof and local lock behavior independently.
- **Deliverable:** A gated managed activation path tied to one device and organization.
- **Acceptance:** Code consumption, device enrolment and membership activation are atomic. A matching retry produces the existing result.
- **Negative cases:** Invalid or already-used code, mismatched proof, unsupported local lock, second-device use or private key leaving the device.
- **Validation:** V-API, V-SQL, V-APP, V-AUTHZ, V-ISO, V-IDEM, V-PRIV · migration: additive · rollback: disable activation without deleting organization records
- **Risks and assumptions:** Local unlock controls access to the device partition. It does not replace the enrolled key or server session.
- **Done when:** Transaction, identity-bound replay and Android lock evidence pass under the approved credential scope.

### EMP-IDN-008 — Managed credential recovery, reset and device revocation
- **Meta:** module IDN · phase F1 · cut - · priority P1 · type API · size S · state TODO
- **Objective:** Recover managed access by replacing device authority within the organization.
- **Rules and use cases:** BR-IDN-020, BR-IDN-024, BR-IDN-025, BR-IDN-038, UC-IDN-07
- **Depends on:** EMP-IDN-007
- **Read first:** [identity specification](../modules/identity.md) §§4–5, [privacy and security](../11-privacy-security.md) §4
- **Scope:** in: managed recovery, replacement enrolment and revocation · out: resetting personal credentials and offline remote wipe
- **Areas:** existing: managed activation and device revocation · proposed: scoped recovery command
- **Steps:** Pin atomic replacement and revocation outcomes. Test lost-device versus ordinary access-ending behavior.
- **Deliverable:** A managed recovery workflow that preserves membership history.
- **Acceptance:** The new key is enrolled and the previous enrolment is revoked. Managers gain no authority over the person's own account.
- **Negative cases:** Reused secret, concurrent replacement, old-device request, foreign enrolment and misleading cleanup acknowledgement.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM, V-PRIV · migration: rehearse any additive state · rollback: disable new recovery and preserve revocations
- **Risks and assumptions:** Unsent work on a lost device is handled under the documented policy and not claimed recovered.
- **Done when:** Recovery and old-device denial pass through the normal entry points.

### EMP-IDN-009 — Membership suspension and reactivation
- **Meta:** module IDN · phase F1 · cut - · priority P1 · type API · size S · state TODO
- **Objective:** Suspend and reactivate membership without rewriting historical assignments.
- **Rules and use cases:** BR-IDN-026, BR-IDN-030, BR-IDN-036, BR-AUTH-014, UC-IDN-08
- **Depends on:** EMP-IDN-003, EMP-AUTH-003
- **Read first:** [identity specification](../modules/identity.md) §§4–6, [permissions specification](../07-permissions.md) §6
- **Scope:** in: scoped suspension and reactivation · out: offboarding and automatic review of late operations
- **Areas:** existing: membership, authorization and organization audit · proposed: membership state commands
- **Steps:** Exercise current authority, the last-owner race and reactivation with expired assignments.
- **Deliverable:** Audited membership suspension and reactivation commands.
- **Acceptance:** The next request honors suspension. Reactivation restores only assignments still valid under their original terms.
- **Negative cases:** Self-suspension, concurrent last-owner removal, foreign membership or cache retaining access.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM · migration: reuse membership state · rollback: disable the new command and keep audit and state
- **Risks and assumptions:** Membership suspension and employee-state suspension must preserve their recorded cause.
- **Done when:** State changes, role denial and concurrency tests pass on PostGIS.

### EMP-IDN-010 — Offboarding: end membership, revoke sessions, hand over responsibilities
- **Meta:** module IDN · phase F1 · cut - · priority P1 · type API · size M · state TODO
- **Objective:** End organization access atomically and leave a clear handover record.
- **Rules and use cases:** BR-IDN-025, BR-IDN-027, BR-IDN-028, BR-IDN-029, BR-IDN-030, BR-IDN-031, UC-IDN-09, UC-IDN-10
- **Depends on:** EMP-IDN-009
- **Read first:** [identity specification](../modules/identity.md) §§4–5 and 8, [offline synchronization](../10-offline-sync.md) §10.2
- **Scope:** in: organization access ending, tokens and responsibility handover · out: personal-account revocation or automatic closure of work
- **Areas:** existing: membership, device registry, role assignments and audit · proposed: offboarding command and handover view
- **Steps:** Pin atomic access ending and responsibility validation. Preserve existing records and signal later intake through its dedicated contract.
- **Deliverable:** An auditable offboarding and self-leave workflow.
- **Acceptance:** Membership, scoped grants and enrolments end together. Personal access and organization history remain available under their own rules.
- **Negative cases:** Last owner, missing required successor, foreign employee, concurrent replay or revocation of personal credentials.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM, V-PRIV · migration: additive where handover needs state · rollback: preserve completed access revocations
- **Risks and assumptions:** Late offline facts follow review rules. Offboarding is not an automatic work closure.
- **Done when:** PostGIS proves consistent handover and access denial, including rollback and replay.

### EMP-IDN-011 — Rehire: a new membership period on the same employee record
- **Meta:** module IDN · phase F1 · cut - · priority P2 · type API · size S · state TODO
- **Objective:** Rehire on the existing employee record while opening a distinct access period.
- **Rules and use cases:** BR-IDN-002, BR-IDN-032, BR-IDN-033, UC-IDN-11
- **Depends on:** EMP-IDN-010
- **Read first:** [identity specification](../modules/identity.md) §§4–6, [permissions specification](../07-permissions.md) §4
- **Scope:** in: reopening a terminated employee and later provisioning · out: restoring ended assignments
- **Areas:** existing: employee, membership and issuance commands · proposed: rehire command
- **Steps:** Check reactivation eligibility and idempotency. Keep ended periods and require a fresh access decision.
- **Deliverable:** A scoped rehire workflow retaining the original employee identity.
- **Acceptance:** The same employee is retained. A new access period requires a new invitation or an approved managed path.
- **Negative cases:** Foreign or merged-away employee, an overlapping period, implicit role resurrection or historical reassignment.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM · migration: reuse historical identifiers · rollback: preserve existing periods
- **Risks and assumptions:** Rehire does not recover a personal account.
- **Done when:** Rehire, duplicate requests and intact history pass on synthetic data.

### EMP-IDN-012 — Multiple memberships and duplicate employee detection and merge
- **Meta:** module IDN · phase F1 · cut - · priority P2 · type API · size M · state TODO
- **Objective:** Keep memberships independent and resolve same-organization employee duplicates explicitly.
- **Rules and use cases:** BR-IDN-033, BR-IDN-034, BR-IDN-035, UC-IDN-12, UC-IDN-13
- **Depends on:** EMP-IDN-005
- **Read first:** [identity specification](../modules/identity.md) §§4–5 and 8, [privacy and security](../11-privacy-security.md) §4
- **Scope:** in: duplicate review and merge history in one organization · out: merging personal accounts or records across organizations
- **Areas:** existing: employee register and membership state · proposed: reviewed employee merge and survivor references
- **Steps:** Use separate organization fixtures. Prove warnings, explicit survivor selection, conflicting live access and historical lookup.
- **Deliverable:** An authorized duplicate-resolution workflow with stable historical references.
- **Acceptance:** Membership visibility stays independent. A reviewed merge preserves both identifiers and ends no unrelated access.
- **Negative cases:** Foreign candidate, two live memberships, automatic merge, cycle in survivor references or hidden history.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM, V-PRIV · migration: additive survivor links · rollback: preserve accepted merge history
- **Risks and assumptions:** A likely match is evidence for review rather than authority to merge.
- **Done when:** Scoped merge and two-organization independence pass under concurrent edits.

### EMP-IDN-013 — Contact change and invitation invalidation
- **Meta:** module IDN · phase F1 · cut - · priority P2 · type API · size S · state TODO
- **Objective:** Invalidate issued access tokens when an employee contact changes.
- **Rules and use cases:** BR-IDN-013, BR-IDN-018, BR-IDN-025, UC-IDN-14
- **Depends on:** EMP-IDN-004
- **Read first:** [identity specification](../modules/identity.md) §§4–5 and 8, [privacy and security](../11-privacy-security.md) §4
- **Scope:** in: scoped work-contact edit and token invalidation · out: personal account contact changes and external delivery
- **Areas:** existing: employee and invitation transactions · proposed: contact edit with token revocation
- **Steps:** Test the edit and token changes atomically. Exercise concurrent redemption and reissue around the edit.
- **Deliverable:** A contact update that preserves current organization access rules.
- **Acceptance:** The contact edit invalidates outstanding tokens in the same transaction. Future access requires an authorized reissue.
- **Negative cases:** Old-contact token redeems after commit, a foreign contact changes, or a partial edit preserves an invalid token.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM, V-PRIV · migration: reuse existing token state · rollback: preserve revocations
- **Risks and assumptions:** Existing active memberships are governed by their own state.
- **Done when:** The update and redemption race leaves a single consistent outcome.

### EMP-IDN-014 — Driver profile: licence category, expiry and eligibility
- **Meta:** module IDN · phase F2 · cut - · priority P1 · type API · size S · state TODO
- **Objective:** Derive driver eligibility from the approved minimal profile.
- **Rules and use cases:** BR-IDN-003, BR-IDN-040, BR-IDN-041, BR-PRIV-005, BR-TIME-004, UC-IDN-15
- **Depends on:** EMP-IDN-002
- **Read first:** [identity specification](../modules/identity.md) §§4–5, [privacy and security](../11-privacy-security.md) §§4 and 10
- **Scope:** in: licence categories, expiry and derived status · out: live lookup or deciding which trip requires a licence
- **Areas:** existing: employee register and time rules · proposed: optional driver profile and eligibility read
- **Steps:** Pin the local-date boundary and unknown values. Exercise masking, scope and disabled document fields.
- **Deliverable:** An optional profile API and deterministic eligibility result.
- **Acceptance:** Eligibility derives from the unit-local date and stored expiry. Unspecified status remains visible.
- **Negative cases:** Foreign profile, unapproved licence photo or identifier, defaulting unknown to eligible or using the wrong day.
- **Validation:** V-API, V-SQL, V-DOM, V-AUTHZ, V-ISO, V-PRIV · migration: additive optional profile · rollback: disable access and preserve authorized records
- **Risks and assumptions:** Real document collection requires its reviewed purpose and retention policy.
- **Done when:** Expiry, optional fields and negative access pass using synthetic fixtures.

### EMP-IDN-015 — Device registry, session list and revocation
- **Meta:** module IDN · phase F1 · cut - · priority P1 · type API · size S · state TODO
- **Objective:** List and revoke scoped device enrolments without exposing personal sessions.
- **Rules and use cases:** BR-IDN-025, BR-IDN-037, BR-IDN-038, BR-IDN-039, BR-PRIV-018, UC-IDN-16
- **Depends on:** EMP-IDN-005
- **Read first:** [identity specification](../modules/identity.md) §§4–5 and 8, [privacy and security](../11-privacy-security.md) §4
- **Scope:** in: device list, scoped session state and revocation class · out: manager access to personal sessions and immediate offline wipe
- **Areas:** existing: device proof and account session conventions · proposed: organization enrolment registry and revoke command
- **Steps:** Prove enrolment limits and immediate server denial under concurrent registration and revocation. Preserve scoped revocation reasons.
- **Deliverable:** An organization device registry with independent authority checks.
- **Acceptance:** An authorized request lists only the selected organization's enrolments. Revocation applies to the next server request.
- **Negative cases:** Foreign device, missing permission, last permitted slot race, revoked-key use and personal session disclosure.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM, V-PRIV · migration: additive enrolment registry · rollback: preserve revocations and recorded scope
- **Risks and assumptions:** Later intake and on-contact cleanup depend on the revocation class.
- **Done when:** Registry, revocation and scoped denial pass on real PostGIS.

### EMP-IDN-016 — Panel: employees, invitations and activation codes
- **Meta:** module IDN · phase F1 · cut C1 · priority P0 · type WEB · size M · state TODO
- **Objective:** Expose employee and invitation management in the company panel.
- **Rules and use cases:** BR-IDN-003, BR-IDN-008, BR-IDN-013, BR-IDN-016, BR-IDN-039, UC-IDN-01, UC-IDN-02, SCR-IDN-01, SCR-IDN-03, SCR-IDN-04
- **Depends on:** EMP-IDN-002, EMP-IDN-004, EMP-PLAT-008
- **Read first:** [identity specification](../modules/identity.md) §§5 and 11, [M0 decisions](../evidence/f0-m0-decisions.md) §Company web surface
- **Scope:** in: C1 employee and invitation screens · out: managed-code actions before their approved APIs and gate
- **Areas:** existing: approved /empresas/ scaffold and invitation API · proposed: employee and access views
- **Steps:** Verify the issue, list, resend, revoke and confirmation paths with accessible states. Keep the secret out of later list and history views.
- **Deliverable:** A scoped employee and invitation UI in the approved panel.
- **Acceptance:** A manager sees the selected organization's permitted records. Invitation secrets appear only during issuance.
- **Negative cases:** Foreign results after switching, missing fresh authentication, secret in navigation or premature managed-code controls.
- **Validation:** V-WEB, V-AUTHZ, V-ISO, V-PRIV · migration: none · rollback: remove the new routes and preserve issued invitations
- **Risks and assumptions:** The indexed title retains managed codes. C1 closes on the existing-account invitation path.
- **Done when:** The C1 screen flow passes with keyboard access and server-confirmed state.

### EMP-IDN-017 — App: invitation acceptance and managed activation
- **Meta:** module IDN · phase F1 · cut C1 · priority P0 · type APP · size M · state TODO
- **Objective:** Accept an existing-account invitation from Android.
- **Rules and use cases:** BR-IDN-005, BR-IDN-012, BR-IDN-017, BR-IDN-033, BR-PRIV-015, UC-IDN-03, SCR-IDN-09, SCR-IDN-10
- **Depends on:** EMP-IDN-005, EMP-PLAT-011
- **Read first:** [identity specification](../modules/identity.md) §§5 and 11, [M0 decisions](../evidence/f0-m0-decisions.md) §Android local protection
- **Scope:** in: C1 code entry, informed consent and server-confirmed acceptance · out: managed activation before its separately gated APIs
- **Areas:** existing: Android account, device proof and enterprise foundation · proposed: invitation entry and result screens
- **Steps:** Exercise sign-in, decline, offline and pending confirmation through the normal app entry point. Verify invalid results do not expose token state.
- **Deliverable:** An accessible invitation-acceptance flow for an existing account.
- **Acceptance:** Enterprise access appears only after the server confirms membership. Pending confirmation is visible and remains pending.
- **Negative cases:** Invalid token leaks details, local success without acknowledgement, wrong account or premature managed activation.
- **Validation:** V-APP, V-API, V-AUTHZ, V-ISO, V-PRIV · migration: preserve personal storage · rollback: hide the new entry and retain acknowledged membership
- **Risks and assumptions:** The indexed title retains managed activation. That part follows its own approval and API evidence outside C1.
- **Done when:** Existing-account acceptance and negative outcomes pass in the affected Android flow.

### EMP-IDN-018 — App: personal and enterprise context switch with the enterprise home shell
- **Meta:** module IDN · phase F1 · cut C1 · priority P0 · type APP · size M · state TODO
- **Objective:** Switch personal and organization context without changing stored operation ownership.
- **Rules and use cases:** BR-AUTH-012, BR-AUTH-014, BR-IDN-033, BR-TEN-004, BR-PRIV-016, UC-AUTH-04, SCR-IDN-12
- **Depends on:** EMP-AUTH-003, EMP-PLAT-011, EMP-IDN-017
- **Read first:** [identity specification](../modules/identity.md) §11, [permissions specification](../07-permissions.md) §5, [M0 decisions](../evidence/f0-m0-decisions.md) §Android local protection
- **Scope:** in: explicit context switch and enterprise home shell · out: moving queued data or claiming offline authority
- **Areas:** existing: Android personal entry and partitioned foundation · proposed: context selector and enterprise shell
- **Steps:** Use two organization fixtures. Exercise switching with queued data, stale membership lists, revocation and process recreation.
- **Deliverable:** An explicit and scoped Android context and home shell.
- **Acceptance:** The active organization is visible. Context changes do not move private data or queued operations. Cached authority is marked unconfirmed.
- **Negative cases:** A foreign partition opens, queued data retags, or a revoked membership still enables new commands.
- **Validation:** V-APP, V-SYNC, V-AUTHZ, V-ISO, V-PRIV · migration: preserve personal mode and held data · rollback: hide enterprise entry without discarding queues
- **Risks and assumptions:** The follow-up sync task owns logout and pending-queue semantics.
- **Done when:** The same account can switch between its authorized contexts without leaking private state.

### EMP-IDN-019 — Personal account erasure versus organization-owned history
- **Meta:** module IDN · phase F6 · cut - · priority P1 · type API · size M · state TODO
- **Objective:** Apply personal erasure while keeping properly owned organization history.
- **Rules and use cases:** BR-IDN-027, BR-IDN-030, BR-IDN-042, BR-IDN-043, BR-IDN-044, BR-PRIV-014, UC-IDN-17
- **Depends on:** EMP-IDN-010, EMP-PRIV-002
- **Read first:** [identity specification](../modules/identity.md) §§4–5, [privacy and security](../11-privacy-security.md) §4
- **Scope:** in: pre-deletion notice, last-owner block, tombstone and rights routing · out: deleting the organization's records with the personal account
- **Areas:** existing: personal erasure and membership lifecycle · proposed: organization erasure safeguard and notice
- **Steps:** Pin multi-organization state, last-owner checks and retry behavior. Test tombstones and ensure only the authorized owner sees the notice.
- **Deliverable:** A coordinated personal account erasure path that preserves organization records.
- **Acceptance:** Deletion ends memberships and removes personal linkage while scoped organization history remains under its reviewed schedule.
- **Negative cases:** Last owner deleted, personal linkage restored from backup, foreign list, partially ended grants or organization records removed.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM, V-PRIV · migration: rehearse tombstone and restored snapshot handling · rollback: preserve deletion records
- **Risks and assumptions:** Production rights and retention use the reviewed legal policy.
- **Done when:** Erasure, race, rollback and restore checks pass with scoped fixtures.

### EMP-AUTH-001 — Role and permission registry: system roles and permission keys
- **Meta:** module AUTH · phase F1 · cut C1 · priority P0 · type DOM · size S · state LOCAL_DONE
- **Objective:** Define the fixed system roles and permission keys used by the authorization service.
- **Rules and use cases:** BR-AUTH-001, BR-AUTH-003, BR-AUTH-007, EDR-008
- **Depends on:** EMP-PLAT-001
- **Read first:** [permissions specification](../07-permissions.md) §§4 and 7, [validation scenarios](../15-validation.md)
- **Scope:** in: fixed registry and role-to-permission mapping · out: runtime grants and custom roles
- **Areas:** existing: project role conventions · proposed: organization role and permission domain registry
- **Steps:** Pin approved keys and role rows. Validate unknown and duplicate keys and check that grants never imply record-rule exceptions.
- **Deliverable:** A deterministic permission catalog with tests.
- **Acceptance:** System roles resolve to approved permission sets. Unknown keys cannot create authority.
- **Negative cases:** A duplicate or unregistered permission, a role bypassing separation of duties or a paid tier treated as a role.
- **Validation:** V-DOM · migration: none · rollback: revert the new registry before dependent behavior is exposed
- **Risks and assumptions:** Custom roles are a separate phase and use the same keys.
- **Done when:** The fixed registry agrees with the documented permission matrix.

[Local fixed-catalog and matrix evidence](../evidence/f1-auth001-role-permission-catalog.md).

### EMP-AUTH-002 — Role assignments with organization and self scopes
- **Meta:** module AUTH · phase F1 · cut C1 · priority P0 · type DB · size S · state LOCAL_DONE
- **Objective:** Persist auditable role assignments over organization and self scope.
- **Rules and use cases:** BR-AUTH-001, BR-AUTH-004, BR-AUTH-005, BR-AUTH-006, UC-AUTH-01
- **Depends on:** EMP-AUTH-001, EMP-IDN-003
- **Read first:** [permissions specification](../07-permissions.md) §§3–5 and 8, [identity specification](../modules/identity.md) §6
- **Scope:** in: assignment persistence and initial ORG and SELF scope · out: custom roles and later structural scopes
- **Areas:** existing: role registry, membership store and organization schema · proposed: role-assignment repository
- **Steps:** Prove scope and membership references, assignment validity and duplicate prevention. Test the supported scopes on synthetic data.
- **Deliverable:** A scoped role-assignment store.
- **Acceptance:** Assignments refer to the same organization's membership and only valid rows are candidates for authorization.
- **Negative cases:** Foreign membership, overlapping duplicate, unsupported scope, ended assignment or bypassing last-owner constraints.
- **Validation:** V-SQL, V-ISO, V-IDEM · migration: additive with upgrade and compatibility rehearsal · rollback: keep role history and disable new routes
- **Risks and assumptions:** Storage is one layer. Request-time policy and changes are enforced by their own tasks.
- **Done when:** Role storage boundaries and concurrency cases pass on PostGIS.

[Local scoped assignment and owner-policy evidence](../evidence/f1-auth002-role-assignment-persistence.md).

### EMP-AUTH-003 — Authorization service and organization-context middleware
- **Meta:** module AUTH · phase F1 · cut C1 · priority P0 · type API · size M · state LOCAL_DONE
- **Objective:** Resolve each organization request against current server authority.
- **Rules and use cases:** BR-AUTH-001, BR-AUTH-002, BR-AUTH-013, BR-AUTH-014, BR-TEN-003, BR-TEN-006, UC-AUTH-05
- **Depends on:** EMP-AUTH-002, EMP-PLAT-004
- **Read first:** [permissions specification](../07-permissions.md) §§4–5 and 10, [architecture](../09-architecture.md)
- **Scope:** in: shared authorization decision and scoped request context · out: client-selected authority and separate late-intake exception
- **Areas:** existing: account and device proof, path scope, membership and roles · proposed: organization authorization service
- **Steps:** Pin allowed and denied cases through the normal entry point. Resolve organization, active membership, effective scope and record gates on the server.
- **Deliverable:** A shared authorization service and organization context middleware.
- **Acceptance:** The next request reflects revoked or changed authority. Foreign resources reveal no record data or existence hint.
- **Negative cases:** Invalid proof, foreign identifier, missing role or capability, stale cache, unauthorized reuse of context and sensitive reads without scope.
- **Validation:** V-API, V-AUTHZ, V-SQL, V-ISO, V-PRIV · migration: reuse scoped storage · rollback: keep protected routes closed when the guard is unavailable
- **Risks and assumptions:** Client context affects display only. Organization scope is resolved again for each authorized request.
- **Done when:** The allowed, denied and post-revocation matrix passes through real middleware and storage.

[Current authorization and real-proof acceptance](../evidence/f1-auth003-current-authorization.md).

### EMP-AUTH-004 — Entitlement gate port with a pilot adapter
- **Meta:** module AUTH · phase F1 · cut C1 · priority P0 · type DOM · size S · state LOCAL_DONE
- **Objective:** Apply capabilities independently of roles through a disabled-by-default pilot adapter.
- **Rules and use cases:** BR-AUTH-001, BR-AUTH-009, BR-PRIV-003
- **Depends on:** EMP-AUTH-003, EMP-PLAT-005
- **Read first:** [permissions specification](../07-permissions.md) §§4 and 10, [activation design](../09-architecture.md#10-activation-flags-and-rollback)
- **Scope:** in: entitlement port and pilot capability decisions · out: paid plans and granting roles
- **Areas:** existing: per-organization flags and authorization service · proposed: capability gate port and pilot adapter
- **Steps:** Test enabled and missing capability states. Verify the decision cannot widen role scope and denial survives rollback.
- **Deliverable:** A capability port and safe pilot adapter.
- **Acceptance:** Off or missing capability denies the relevant new action while existing reads follow the documented policy.
- **Negative cases:** Account tier or capability creates membership, missing adapter silently allows or a foreign flag affects this organization.
- **Validation:** V-DOM, V-AUTHZ, V-ISO · migration: reuse capability state · rollback: disable the pilot capability
- **Risks and assumptions:** Plan expiry changes capabilities, not assignments or account ownership.
- **Done when:** Capability and role decisions stay independent across the matrix.

[Pilot entitlement and rollback acceptance](../evidence/f1-auth004-pilot-entitlements.md).

### EMP-AUTH-005 — Role management with last-owner and separation-of-duties guards
- **Meta:** module AUTH · phase F1 · cut - · priority P1 · type API · size M · state TODO
- **Objective:** Change scoped roles without escalation or removing the last owner.
- **Rules and use cases:** BR-AUTH-003, BR-AUTH-006, BR-AUTH-007, BR-AUTH-010, BR-AUTH-014, UC-AUTH-01, UC-AUTH-02
- **Depends on:** EMP-AUTH-003, EMP-PLAT-003
- **Read first:** [permissions specification](../07-permissions.md) §§4–5 and 8, [identity specification](../modules/identity.md) §8
- **Scope:** in: authorized grants, changes, endings and last-owner race · out: custom roles and owner recovery
- **Areas:** existing: scoped role storage and audit · proposed: role-management application commands
- **Steps:** Exercise permission subsets, fresh proof, scope and last-owner contention. Commit each accepted change with one audit entry.
- **Deliverable:** Audited role-management endpoints with immediate effect.
- **Acceptance:** Grants stay within the actor's effective scope. Organization ownership survives every allowed change.
- **Negative cases:** Self-escalation, stale proof, foreign scope, conflicting grant and concurrent last-owner removal.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM, V-PRIV · migration: preserve assignment history · rollback: disable commands without undoing accepted revocations
- **Risks and assumptions:** Separation-of-duties and record-state rules still apply after a grant.
- **Done when:** Permission, last-owner and audit atomicity tests pass on real PostGIS.

### EMP-AUTH-006 — Unit, cost-centre, team and asset scopes with resource scope resolution
- **Meta:** module AUTH · phase F2 · cut - · priority P1 · type DB · size M · state TODO
- **Objective:** Resolve structural scopes against the appropriate current or historical resource context.
- **Rules and use cases:** BR-AUTH-004, BR-AUTH-005, BR-AUTH-013, BR-ORG-011, BR-TEN-005
- **Depends on:** EMP-AUTH-003, EMP-ORG-005, EMP-ORG-006, EMP-ORG-007, EMP-FLEET-002
- **Read first:** [permissions specification](../07-permissions.md) §§4 and 10, [organization structure](../modules/organizations.md)
- **Scope:** in: unit, cost-centre, team and asset scopes · out: station authority and invented inferred grants
- **Areas:** existing: scope resolver, organization structures and fleet storage · proposed: structural scope lookup and constraints
- **Steps:** Pin historical-versus-current scope examples. Prove foreign and archived scope handling with two organizations.
- **Deliverable:** Scoped structural resource resolution with PostGIS evidence.
- **Acceptance:** Movement or reassignment does not change the stored scope of a historical record.
- **Negative cases:** Foreign structure, team membership granting access on its own, reassigned asset widening history and wrong cost-centre match.
- **Validation:** V-SQL, V-AUTHZ, V-ISO, V-IDEM · migration: rehearse additive scoped references · rollback: remove new admission while retaining assignments
- **Risks and assumptions:** Scope calculation uses the record and applicable policy, never a caller assertion.
- **Done when:** Documented examples and foreign-scope denials pass at the resource entry points.

### EMP-AUTH-007 — Temporary delegation and substitutes
- **Meta:** module AUTH · phase F3 · cut - · priority P1 · type API · size S · state TODO
- **Objective:** Enable time-bounded delegation without expanding the delegator's rights.
- **Rules and use cases:** BR-AUTH-007, BR-AUTH-008, BR-AUTH-014, BR-AUD-005, BR-REV-003, UC-AUTH-03
- **Depends on:** EMP-AUTH-005
- **Read first:** [permissions specification](../07-permissions.md) §§4–5 and 8, [validation scenarios](../15-validation.md)
- **Scope:** in: scoped delegation, substitute authority, expiry and audit · out: independent substitute grants
- **Areas:** existing: scoped assignments, organization contacts and audit · proposed: delegation state and commands
- **Steps:** Pin server-time expiry, upstream grant revocation and delegated audit. Verify record separation of duties still applies.
- **Deliverable:** A bounded, revocable delegation workflow.
- **Acceptance:** The delegate can act only inside the approved scope and time. The audit record identifies both actor and delegation.
- **Negative cases:** No end date, excessive scope, lost delegator rights, foreign substitute or delegation enabling self-approval.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM · migration: additive delegation storage · rollback: stop new delegations and retain history
- **Risks and assumptions:** Substitute designation routes work. Authority comes from a valid role or delegation.
- **Done when:** Expiry, revocation and denied approvals pass on synthetic organizations.

### EMP-AUTH-008 — Panel: roles and access management
- **Meta:** module AUTH · phase F1 · cut - · priority P1 · type WEB · size S · state TODO
- **Objective:** Expose role changes and effective access in the authorized company panel.
- **Rules and use cases:** BR-AUTH-006, BR-AUTH-007, BR-AUTH-010, BR-AUTH-012, BR-AUTH-013, UC-AUTH-01, UC-AUTH-02, UC-AUTH-06
- **Depends on:** EMP-AUTH-005, EMP-PLAT-008
- **Read first:** [permissions specification](../07-permissions.md) §§5 and 11, [M0 decisions](../evidence/f0-m0-decisions.md) §Company web surface
- **Scope:** in: scoped access list, role assignment and confirmation UI · out: client-side authorization and station admin
- **Areas:** existing: /empresas/ scaffold and role endpoints · proposed: access management screens
- **Steps:** Verify role changes, conflict and fresh-proof prompts using the real session boundary and accessible controls.
- **Deliverable:** Company access screens driven by authoritative server state.
- **Acceptance:** Only permitted data is shown. The UI reports refused or stale operations without claiming success.
- **Negative cases:** Stale or foreign access shown after switching, hidden UI mistaken for enforcement or wrong last-owner status.
- **Validation:** V-WEB, V-AUTHZ, V-ISO, V-PRIV · migration: none · rollback: hide the new screens and preserve accepted role changes
- **Risks and assumptions:** Use approved panel session and no-store behavior.
- **Done when:** Authorized role management and negative states pass through the normal company routes.

### EMP-AUTH-009 — Authorization matrix conformance suite
- **Meta:** module AUTH · phase F1 · cut C1 · priority P0 · type TEST · size M · state TODO
- **Objective:** Prove authorization matrix conformance across organization boundaries.
- **Rules and use cases:** BR-AUTH-001, BR-AUTH-003, BR-AUTH-006, BR-AUTH-007, BR-AUTH-013, BR-AUTH-014, BR-PRIV-002
- **Depends on:** EMP-AUTH-003
- **Read first:** [permissions specification](../07-permissions.md) §§7 and 12, [validation scenarios](../15-validation.md)
- **Scope:** in: action, role, scope and negative fixtures · out: changing policy to fit a test
- **Areas:** existing: real PostGIS isolation harness and permission matrix · proposed: conformance suite
- **Steps:** Map each matrix row to explicit allow and deny cases. Exercise wrong organization and post-revocation access at the request boundary.
- **Deliverable:** A conformance suite with concrete positive and negative coverage.
- **Acceptance:** Every applicable matrix cell has an assertion, including missing and expired authority.
- **Negative cases:** A skipped deny case, tests with an overprivileged database role, foreign data in denial or stale permissions accepted.
- **Validation:** V-AUTHZ, V-ISO, V-SQL, V-PRIV · migration: none · rollback: retain required regression coverage
- **Risks and assumptions:** Use synthetic fixtures and normal entry points.
- **Done when:** The fixed matrix is covered with no unexplained omissions.

### EMP-AUTH-010 — Custom roles as named permission bundles
- **Meta:** module AUTH · phase F6 · cut - · priority P3 · type API · size M · state TODO
- **Objective:** Allow named permission bundles using only approved keys and scoped grant rules.
- **Rules and use cases:** EDR-008, BR-AUTH-001, BR-AUTH-003, BR-AUTH-006, BR-AUTH-007, BR-AUTH-014
- **Depends on:** EMP-AUTH-005, EMP-AUTH-009
- **Read first:** [permissions specification](../07-permissions.md) §§4 and 7, [validation scenarios](../15-validation.md)
- **Scope:** in: organization-defined role bundles and lifecycle · out: new permission kinds and unrestricted owner grants
- **Areas:** existing: fixed registry and role commands · proposed: custom role persistence and bundle validation
- **Steps:** Check keys against the registry. Test scope, grant limits, update effects and concurrent assignment changes.
- **Deliverable:** Scoped custom-role management using the existing authorization model.
- **Acceptance:** Custom bundles can compose only valid keys. Each grant still passes the actor's current role-management rules.
- **Negative cases:** Unknown key, foreign bundle, self-escalation, structural scope bypass or stale assignment after deletion.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM · migration: additive and compatible · rollback: freeze new changes and preserve audit
- **Risks and assumptions:** Existing system roles and separation-of-duties controls still apply.
- **Done when:** The custom-role matrix and rollback behavior pass without changing fixed-role semantics.

### EMP-AUTH-011 — Auditor read-only role and export permission gating
- **Meta:** module AUTH · phase F3 · cut - · priority P1 · type API · size S · state TODO
- **Objective:** Enforce the auditor read-only role and independently gate exports.
- **Rules and use cases:** BR-AUTH-001, BR-AUTH-003, BR-AUTH-010, BR-AUTH-011, BR-AUTH-013, BR-AUD-003
- **Depends on:** EMP-AUTH-003
- **Read first:** [permissions specification](../07-permissions.md) §§4 and 7, [privacy and security](../11-privacy-security.md) §4
- **Scope:** in: scoped auditor reads and distinct export permission · out: writes, implicit access to document bytes or location
- **Areas:** existing: permission registry, organization read routes and audit · proposed: auditor and export conformance cases
- **Steps:** Prove the read and write matrix. Verify full export and sensitive content require their respective authority.
- **Deliverable:** A scoped audit access matrix with explicit export controls.
- **Acceptance:** The auditor has only the documented read permissions. Exports and sensitive reads check additional permission and audit where required.
- **Negative cases:** Auditor mutates a record, exports without permission, reads a foreign record or accesses sensitive bytes through general read authority.
- **Validation:** V-API, V-AUTHZ, V-ISO, V-PRIV · migration: reuse role scope · rollback: disable the new access path and retain audit evidence
- **Risks and assumptions:** Export eligibility and payload contents remain owned by their workflow.
- **Done when:** Allowed reads and denied writes, exports and sensitive reads pass through the relevant entry points.
