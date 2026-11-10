# Android first-use terms acceptance and consent confirmation

Status: LOCAL_DONE / INTEGRATION_PENDING (2026-11-10). Owner request of
2026-11-10: the app must present the terms before anything else can be done,
and ticking and accepting must be real. Scope: Android only; no backend change.

This document owns the first-use acceptance. The optional training consent
stays owned by B-BR-CS06 in
[COMMUNITY_SAFETY_EXECUTION_PLAN](../planning/COMMUNITY_SAFETY_EXECUTION_PLAN.md)
and [AI_TRAINING_IMAGE_CONSENT_PLAN](AI_TRAINING_IMAGE_CONSENT_PLAN.md); only
its confirmation step is refined here.

## Behavior contract

- **B-BR-TA01 — Terms come first.** GIVEN no acceptance record for the current
  document versions, WHEN the app opens, THEN the only content is the terms
  screen: what is being accepted, links to the complete Terms of Use and
  Privacy Policy, their versions, one unticked box and the accept action. No
  other screen, navigation or deep link is composed before acceptance. The
  accept action stays disabled until the box is ticked; the box is never
  pre-ticked. Declining closes the app and records nothing.
- **B-BR-TA02 — The record.** Accepting stores the Terms of Use version, the
  Privacy Policy version and the acceptance instant (UTC epoch milliseconds) on
  the device. Nothing else is stored: no name, account, key, location or device
  identifier. The record is never logged and never sent by this feature. An
  acceptance without a tick, without a positive instant or for versions other
  than the current ones is refused.
- **B-BR-TA03 — Versions.** The accepted versions are the identifiers published
  on the public documents: the Terms of Use "Última atualização" month
  (`2026-10`) and the Privacy Policy version (`1.0`). When either public
  document changes, the constant changes with it and every earlier record stops
  being current: the terms screen returns and says the documents changed. A
  unit test compares the constants with `landing/static/termos/index.html` and
  `landing/static/privacidade/index.html`.
- **B-BR-TA04 — Revocable.** Privacy shows the stored record (versions and
  local date and time) and a "revoke acceptance" action behind a confirmation.
  Revoking deletes the record and returns to the terms screen at once. It does
  not delete the account or server data; those keep their own paths (account
  deletion, training-consent revocation).
- **B-BR-TA05 — Not a consent for optional processing.** Accepting the Terms
  of Use and acknowledging the Privacy Policy is the condition to use the app.
  It never grants the optional training consent, never ticks it and never
  changes what the server holds. No "general advance consent" is introduced
  (B-BR-STOREPRIV-01).
- **B-BR-TA06 — Corrupt or foreign data.** An unreadable, partial, future-dated
  or blank stored record counts as no record.

### B-BR-CS06 refinement — confirming the optional training consent

The complete training terms and the unticked box are shown together in one
modal. Ticking is possible there, and "accept and record" sends the grant for
exactly the displayed version and digest. Closing the modal, or ticking without
accepting, sends nothing. On the Privacy card the box opens that modal while
the complete document has not been displayed for the current owner and terms;
a tap is never a no-op. The server stays the only record of the grant: the
card shows the server's state, terms version and decision time, and offers
revocation at all times. Collection remains OFF.

## Use cases

- **BUC-TA01:** first open → terms screen → tick → accept → app. Test: no tick
  refuses, decline records nothing, record carries versions and instant.
- **BUC-TA02:** a later open with a current record goes straight to the app; a
  changed document version, a corrupt record or a future-dated record returns
  to the terms screen.
- **BUC-TA03:** Privacy → revoke acceptance → confirmation → terms screen;
  cancelling the confirmation keeps the record.
- **BUC-CS05 (refined step):** Privacy → box or "read the complete terms" →
  modal → tick → accept → server grant → card shows the server state; later
  revoke → server revocation.

## Known limits

- The record is device-local. A signed-in account does not carry it to another
  device, and the server keeps no proof of the first-use acceptance. A
  server-side account record needs a backend contract and is not part of this
  task.
- The gate is a UI gate. Background work that the application schedules at
  process start (for example the pending-revocation retry) is not held back.
- The document links point at the staging host, like the existing Privacy
  links.
