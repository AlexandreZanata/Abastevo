# Organization-state plan decision profile v1

EMP-OFFL-012 / B-BR-EMP-309, EDR-201. `state-intake-v1.psv` pins the
server-only plan rule over the operation's write capability row: Go
`orgsync/application.DecidePlan` and the PostgreSQL function
`org_sync_plan_disposition` (migration 000219) consume every row. There is no
client mirror. Suspension disposition is owned by EMP-ORG-009's
`AssessSuspension`; its SQL mirror `org_sync_suspension_disposition_of` is
checked against the Go domain by a generated PostGIS matrix instead.

Header `id|operation|args|expected`; every row is `PLAN`. Args: `state`
(ENABLED, DISABLED or ABSENT), `revision`, `changed` (the instant the current
state was set), `declared` and `received`; `?` means absent. Expectations:
`disposition=APPLY`, `disposition=DENY`, the hold
`disposition=HOLD;result=ACCEPTED_FOR_REVIEW;code=PLAN_CHANGED`, or
`error=INVALID`. 20 rows. SHA-256
`ecb5ec152d738be26caa644abe1c2f3700b3445e4c6e8a969084a4cdb5004d5c`.
