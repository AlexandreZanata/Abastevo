# EP01 — Platform foundations and privacy

Task metadata follows the existing [generated index](INDEX.md). The task
definitions reconstruct the missing backlog from the approved contract,
architecture, privacy rules and validation plan. The owning source phases
must produce implementation evidence after the applicable entry gates.
Local F1 requires M0 closure and protected integration of the current
ADR-019 construction batch. See the sequencing decision.

### EMP-PLAT-012 — Transactional e-mail port and adapter for enterprise notices
- **Meta:** module PLAT · phase F1 · cut - · priority P1 · type INT · size S · state TODO
- **Objective:** Add transactional email only if the delivery decision selects it.
- **Rules and use cases:** EDR-032, OQ-09, BR-PRIV-005, BR-PRIV-008
- **Depends on:** EMP-GOV-014, EMP-PLAT-004
- **Read first:** [architecture](../architecture.md), [privacy and security](../privacy-security.md)
- **Scope:** in: approved notification port, bounded retries and scoped delivery · out: unapproved provider or production sends
- **Areas:** existing: job queue and planned in-app path · proposed: transactional email port and reviewed adapter
- **Steps:** Confirm the approved channel and review provider and dependency scope. Test in-app fallback and retries with synthetic fixtures.
- **Deliverable:** A delivery port and approved adapter behind its own off flag.
- **Acceptance:** The in-app path works independently. Retries are bounded and an accepted send is not duplicated.
- **Negative cases:** Wrong recipient or organization, credential in logs, disabled provider sends, retry storm and delivery outage.
- **Validation:** V-JOB, V-IDEM, V-PRIV · migration: owning notification storage remains additive · rollback: preserve data and follow the relevant scoped rollback in the architecture.
- **Risks and assumptions:** A channel approval is separate from credentials, provider setup and production use.
- **Done when:** The authorized adapter and in-app fallback pass scoped delivery tests.
