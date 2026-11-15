# Late intake decision profile v1

EMP-OFFL-009 / B-BR-EMP-303, EDR-197/198. `late-intake-v1.psv` pins the
server-only late decision: Go `orgsync/application.DecideLate` consumes every
row; the PostgreSQL function `org_sync_late_disposition` (migration 000218)
consumes every DECIDE row under UTC, America/New_York and America/Sao_Paulo
session zones. There is no client mirror: the device only presents the
frozen acknowledgement codes.

Header `id|operation|args|expected`. Args are `;`-separated `key=value`:
`end` (MEMBERSHIP or DEVICE), `ended`, `window` (days), `revision` (settings
revision, 0 = default policy), `declared`, `claimed` and `received`; instants
are canonical OrgTime strings and `?` means absent. Expectations are either
`disposition;result;code` or `error=INVALID`. DECIDE rows (47) are shared by
both consumers; EVIDENCE rows (7) check Go-only evidence shape that the
database derives itself. The window is elapsed seconds and includes its end
instant; an earlier server claim outranks the declaration; the declaration
never moves the window. SHA-256
`feb23b2d4c147569ac49977e060b5fdb110c3817cf54c457bb7652e244214dc8`.
