# P37-T02 stable contribution ownership across access expiry

B-BR-PC04 / BUC-PC04, AK03 refinement, 2026-10-08: access-token expiry must not
change a retained account contribution into an anonymous intent. Local owner
scope uses the retained, shape-checked account identity while refresh is owed.
This is identity continuity, never expired-token authorization: signed photo
operations still require fresh server-issued nonces and the exact device key;
account HTTP access/refresh/revocation rules remain unchanged. Explicit logout,
account/device-key/origin changes and pending unverified key-account recovery
must never dispatch another owner's command or reveal its private receipt.

Source parent: `6ca2e94`, maintained dev. Connected POCO read-only native diagnosis
for the exact existing capture found five account commands, five matching origins,
zero matching current owner scopes, NeedsRefresh and access_live=false. WorkManager
had terminated the five attempts as FAILED after owner-or-origin-changed. The
server photo/job recovery is already complete. Normal foreground rescheduling
uses the same owner-scoped durable command IDs; recovery creates no new intent,
revision, photo/session or retention extension.

Status: LOCAL_DONE
Validation: PASS

## Local evidence and remaining runtime work

The focused auth/owner/dispatcher/receipt slice passed 112 distinct cases,
including expiry during an in-flight signed request, offline logout, changed
account/key/origin, pending key recovery and no expired bearer transmission.
Commands and raw local results: `/tmp/owner-scope-green-build.log`,
`/tmp/owner-scope-inflight-test.log`; final unchanged transport regression on
2026-10-09: `:data:testDebugUnitTest --tests '*PhotoProofTransportTest'` passed.
App/data debug lint reported zero errors; debug/instrumentation builds and
release/R8 passed (unsigned APK 4,249,809 bytes). Two native feedback dialog
cases passed on the combined APK, installed without account/data wipe.

The temporary exact-capture recovery harness returned five frozen commands,
zero eligible dispatches, zero received/validated receipts and no mutations.
It therefore proves no completed mobile delivery. The operator harness is
removed from source; no next-day replay or retention extension is authorized
by this local checkpoint. Live receipt/end-user confirmation remains OWED.
Backend same-photo recovery and original deadlines are unchanged.

No deployment, remote Git publication, production certification or premium
fuel behavior is part of this ownership repair.

Transport/test source fingerprint: `5919b9dfe6d165a5c346b51894e0dddd40346554e354c78939e96644802cec6d`.
