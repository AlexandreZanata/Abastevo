# EP03 — Identity, employee accounts and authorization

Task IDs, titles, phase, cut, type, effort, priority and dependencies
follow the existing [generated index](INDEX.md). Task descriptions
were reconstructed from [identity specification](../modules/identity.md) and the [permissions specification](../permissions.md).

Local F1 requires M0 closure and protected integration of the current
ADR-019 construction batch. See the
sequencing decision.

C1 uses invitations for existing accounts. Managed corporate activation and its controls remain outside C1 and follow the credential decision and owning API tasks.

### EMP-IDN-006 — Managed corporate credential: activation code issuance
- **Meta:** module IDN · phase F1 · cut - · priority P1 · type API · size M · state TODO
- **Objective:** Define managed activation-code issuance after the separate credential review.
- **Rules and use cases:** BR-IDN-008, BR-IDN-010, BR-IDN-011, BR-IDN-013, BR-IDN-019, BR-IDN-020, BR-IDN-022, UC-IDN-05
- **Depends on:** EMP-IDN-002, EMP-IDN-003, EMP-GOV-013
- **Read first:** [identity specification](../modules/identity.md) §§4–5 and 10, [privacy and security](../privacy-security.md) §4
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
- **Read first:** [identity specification](../modules/identity.md) §§4–5 and 10, [privacy and security](../privacy-security.md) §4
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
- **Read first:** [identity specification](../modules/identity.md) §§4–5, [privacy and security](../privacy-security.md) §4
- **Scope:** in: managed recovery, replacement enrolment and revocation · out: resetting personal credentials and offline remote wipe
- **Areas:** existing: managed activation and device revocation · proposed: scoped recovery command
- **Steps:** Pin atomic replacement and revocation outcomes. Test lost-device versus ordinary access-ending behavior.
- **Deliverable:** A managed recovery workflow that preserves membership history.
- **Acceptance:** The new key is enrolled and the previous enrolment is revoked. Managers gain no authority over the person's own account.
- **Negative cases:** Reused secret, concurrent replacement, old-device request, foreign enrolment and misleading cleanup acknowledgement.
- **Validation:** V-API, V-SQL, V-AUTHZ, V-ISO, V-IDEM, V-PRIV · migration: rehearse any additive state · rollback: disable new recovery and preserve revocations
- **Risks and assumptions:** Unsent work on a lost device is handled under the documented policy and not claimed recovered.
- **Done when:** Recovery and old-device denial pass through the normal entry points.

### EMP-IDN-014 — Driver profile: licence category, expiry and eligibility
- **Meta:** module IDN · phase F2 · cut - · priority P1 · type API · size S · state TODO
- **Objective:** Derive driver eligibility from the approved minimal profile.
- **Rules and use cases:** BR-IDN-003, BR-IDN-040, BR-IDN-041, BR-PRIV-005, BR-TIME-004, UC-IDN-15
- **Depends on:** EMP-IDN-002
- **Read first:** [identity specification](../modules/identity.md) §§4–5, [privacy and security](../privacy-security.md) §§4 and 10
- **Scope:** in: licence categories, expiry and derived status · out: live lookup or deciding which trip requires a licence
- **Areas:** existing: employee register and time rules · proposed: optional driver profile and eligibility read
- **Steps:** Pin the local-date boundary and unknown values. Exercise masking, scope and disabled document fields.
- **Deliverable:** An optional profile API and deterministic eligibility result.
- **Acceptance:** Eligibility derives from the unit-local date and stored expiry. Unspecified status remains visible.
- **Negative cases:** Foreign profile, unapproved licence photo or identifier, defaulting unknown to eligible or using the wrong day.
- **Validation:** V-API, V-SQL, V-DOM, V-AUTHZ, V-ISO, V-PRIV · migration: additive optional profile · rollback: disable access and preserve authorized records
- **Risks and assumptions:** Real document collection requires its reviewed purpose and retention policy.
- **Done when:** Expiry, optional fields and negative access pass using synthetic fixtures.

### EMP-AUTH-007 — Temporary delegation and substitutes
- **Meta:** module AUTH · phase F3 · cut - · priority P1 · type API · size S · state TODO
- **Objective:** Enable time-bounded delegation without expanding the delegator's rights.
- **Rules and use cases:** BR-AUTH-007, BR-AUTH-008, BR-AUTH-014, BR-AUD-005, BR-REV-003, UC-AUTH-03
- **Depends on:** EMP-AUTH-005
- **Read first:** [permissions specification](../permissions.md) §§4–5 and 8, [validation scenarios](../testing.md)
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
- **Read first:** [permissions specification](../permissions.md) §§5 and 11, M0 decisions §Company web surface
- **Scope:** in: scoped access list, role assignment and confirmation UI · out: client-side authorization and station admin
- **Areas:** existing: /empresas/ scaffold and role endpoints · proposed: access management screens
- **Steps:** Verify role changes, conflict and fresh-proof prompts using the real session boundary and accessible controls.
- **Deliverable:** Company access screens driven by authoritative server state.
- **Acceptance:** Only permitted data is shown. The UI reports refused or stale operations without claiming success.
- **Negative cases:** Stale or foreign access shown after switching, hidden UI mistaken for enforcement or wrong last-owner status.
- **Validation:** V-WEB, V-AUTHZ, V-ISO, V-PRIV · migration: none · rollback: hide the new screens and preserve accepted role changes
- **Risks and assumptions:** Use approved panel session and no-store behavior.
- **Done when:** Authorized role management and negative states pass through the normal company routes.
