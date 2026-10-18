# FQ-15A — Document-free signed owner client

Status: LOCAL_DONE
Validation: PASS

Parent 6c470b15, maintained dev. B-BR-FQ-07/08/09/14/15;
BUC-FQ-05/07/08 (BUC-FQ-06 documents remain D-held). Implement typed document-free report commands and owner-only
read port over the existing exact-body device proof and free account session.
Credentials/nonces/signatures remain ephemeral; stable command IDs survive a
retry with fresh authority. Pin origin/account/device before and after every
request and refuse anonymous/changed sessions without leaking private data.
No private report is cached or persisted in this increment. No D documents,
portal, payment trust, GPS, public claim promotion or automatic ANP handoff.

Acceptance: invalid dates/text/links/action/revision before network, identical
command replay, newly refreshed session per retry, changed-owner refusal before
and during response, bounded pages/receipts and raw-error containment. Domain
RED → GREEN and data tests precede UI work; backend existing signed/replay/IDOR
regressions remain mandatory. Next FQ-15B form and private owner recovery.

## Acceptance, 2026-10-18

Three domain tests first failed on missing rejection (text/date/action/link);
after implementation all pass, including Unicode scalar/invalid-surrogate and
UTC calendar boundaries. `./gradlew --offline :domain:test --tests
'*QualityReportCommandTest' :data:testDebugUnitTest --tests
'*QualityReportHttpClientTest' --tests '*PhotoProofTransportTest'`: PASS.
Six new client cases cover invalid-before-network, private late-owner response,
ordered/unique/bounded pages, nil owner list, receipt identity/revision, fixed
errors, fresh session with stable canonical command and logged-out/replaced
session refusal. Existing real-socket exact-body proof/nonce/redirect tests pass.

`go test -count=1 -race -tags=integration ./internal/modules/quality/adapters
./internal/modules/quality/adapters/http -run
'Test(Report|Reports|DocumentFreeReport)'`: PASS against disposable PostGIS,
including ownership/replay/edit/withdraw, concurrency/rollback, stale/forged
sample target, transaction revocation fence, quota, signed no-store owner reads.
No backend/schema change. Whitespace and secret surface reviewed; only synthetic
authority in test fixtures. Owner scope includes origin/account/device; no
credentials, nonce, proof, report or raw diagnostic is persisted. This is the
client prerequisite only, not completed FQ-15 UI/offline acceptance. D held.
