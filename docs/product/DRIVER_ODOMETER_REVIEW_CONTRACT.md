# DRV-02 — Legacy odometer review contract

Date: 2026-10-26. B-BR-DRV-02/03/17/18; BUC-DRV-04/06. Freeze before implementation.

Missing provenance is UNVERIFIED regardless of the magnitude/date of an old number. No real user database is inspected or rewritten to close this phase. Synthetic old/current records exercise the actual local storage boundary.

## Local state and wire compatibility

Add a versioned, local-only `_abastevo_odometer` object to the existing Room `ledger_draft.body_json` object. It is envelope metadata, not a signed command field or a server schema. Existing typed LedgerDraft decoding ignores extra local fields; outgoing bodies are always reconstructed by `LedgerDraft.bodyJson()` and never forward this envelope. This avoids colliding with the other front's Room schema/migration work or overloading the charge session-context column.

Version 1 metadata records explicit VERIFIED_METRES versus UNVERIFIED. New corrected UI input supplies VERIFIED_METRES only after exact conversion. Generic imports/restores without explicit proof remain UNVERIFIED. Existing missing/unknown metadata fails closed for measured consumption, while raw purchases remain readable/exportable. Version 1 must be an integer JSON value; fractional, string or boolean versions cannot certify a record through numeric coercion. Equal-value record heuristics, timestamps and successful server parsing are not proof of original human units.

A review receipt holds only original numeric reading, canonical original-body SHA-256, expected revision, explicit AS_METRES/AS_KILOMETRES choice and resulting canonical-body hash. Do not retain old note text or raw full financial payload in the receipt. Account/origin/dataset/record keys remain in the existing row scope; no global identifier authorizes review.

## Preview and commit

1. Load the current owner's row, classify missing provenance and show the original stored number as unverified, without guessing a km value.
2. Owner explicitly selects whether that number was metres or kilometres. Show the converted km proposal; preserve prices, quantities, event, vehicle and notes. Charge keeps its stricter existing bound. Impossible conversion/overflow fails without writing.
3. Commit only with the expected canonical-body hash and current server revision. A parameterized SQLite compare-and-set on owner/origin/dataset/record/revision/full stored body fences local edits occurring after the preview.
4. The identical retry returns the existing accepted receipt/result. A different choice, foreign owner, changed revision/body, deleted row or obsolete receipt fails visibly. Cancellation before commit changes nothing; a lost acknowledgement after commit is recovered by identical retry.
5. Correction is local and retains sync revision; enqueue is explicit through the existing flow. An older queued body or revision cannot silently overwrite the correction. After the suspending current-draft read, dispatch rechecks the live owner/session and device/dataset consent before conflict mutation or transport; revocation leaves the bounded claim pending without sending or acknowledging it. Apply the record guard only to refuel/expense/charge rows; gap/reset intents have no draft row and retain their existing authorized transport path. Restoring the original numeric value is a new owner-reviewed correction with a new preview, not a destructive down migration.

## Financial and UI behavior

Unverified odometer rows cannot supply measured consumption, bought-energy efficiency or a cost/km denominator. Keep exact purchase/expense totals, raw rows and export, with `odometer-unit-unverified` and an actionable review entry. Do not silently label a zero distance as measured travel. Existing owner-configured reference efficiency remains separately labeled estimated.

The old entry editor requires explicit unit review before saving a legacy odometer; editing a note cannot accidentally attest a guessed distance. Fresh entry/edit round-trips use km↔metres symmetrically. The receipt is removed with its owning row/account; notes corrected/deleted later are not resurrected from provenance. A backup that excludes the local proof loses certification, not the financial record, and must be reviewed again.

## Required failure evidence

Pure preview cases: proven metres, explicit old-km choice, ambiguous classification, no-reading, each dataset bound, overflow and unaltered non-odometer fields. Actual Room: legacy row, new proven row, metadata restart, owner isolation, stale edit, competing choices, identical concurrent retries, cancelled commit, raw-money preservation and deletion. Actual command/codec tests prove the local envelope never reaches the server. Monthly use-case and UI tests prove unknown metrics retain real totals and expose review. Backend SQL and original financial-v1 are unchanged.

## DRV-02-T03 manual reference acceptance

Keep `financial-v1` and the planning JSON byte-identical. Execute the existing
unit/consumption runtime rows in Go and Kotlin. Separately execute manually
worked earnings/time examples in test-only reference consumers in both
languages: detailed versus inclusive net earnings, payouts kept separate,
recorded versus modeled result, signed loss, overlapping interval union, and
missing/nonpositive denominators. Use integer centavos/seconds/metres and exact
rational arithmetic; expected values come from the frozen examples, never from
the consumer being tested. These reference checks freeze future contracts;
they do not certify a runtime earnings or shift feature (DRV-03 onward).
