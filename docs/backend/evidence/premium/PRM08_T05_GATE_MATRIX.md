# PRM08-T05 backend gate matrix (LOCAL_DONE / INTEGRATION_PENDING)

Pinned revision: `b3149241` on `codex/premium-backend` (PR131 draft).
Config: disposable PostGIS `127.0.0.1:32769`, `sslmode=disable`, synthetic
credentials only. Schema: 76 append-only migrations (`000001`–`000090`).
Contracts: `contracts/openapi/v1.yaml` v1 `1.0.0-draft` (104 live
operationIds), 147 API golden vectors plus `financial-v1.json` (15 vectors).

## Phase checkpoints

| Phase | Tasks | Checkpoint | Evidence |
|---|---|---|---|
| PRM01 | T01–T06 + PRM01-T07 | LOCAL_DONE | PRM_FIRST_TEN.md, PRM_NEXT_TEN.md |
| PRM02 | T01–T05 | LOCAL_DONE | PRM_NEXT_TEN.md |
| PRM03 | T01–T05 | LOCAL_DONE | PRM_NEXT_TEN.md |
| PRM04 | T01–T06 | LOCAL_DONE | PRM_NEXT_TEN.md |
| PRM05 | T01–T06 | LOCAL_DONE | PRM_NEXT_TEN.md |
| PRM06 | T01–T05 | LOCAL_DONE | PRM_NEXT_TEN.md |
| PRM07 | T01–T06 | LOCAL_DONE | PRM_NEXT_TEN.md |
| PRM08 | T01–T04 | LOCAL_DONE | PRM_NEXT_TEN.md |

## Audit results at the pinned revision

- Evidence SHAs: all 32 `tested working tree atop` SHAs resolve to commits.
- Skipped selectors: 4 justified (docker edge proof, soak DSNs, root pool,
  vector regeneration); 1 dead skip removed in this task and re-proven green.
- Zero-test packages: `application` ports-only packages per module (no logic).
- Planned-but-generic rows still marked planned in
  `contracts/openapi/premium-planned.json` (15 generic envelopes superseded by
  exact live routes; professional namespace rows stay future).
- Job rows: `alerts-dispatch` enqueued at evaluation; terminal cleanup rides
  platform retention. `subscription-acknowledge` and `subscription-refresh`
  retain their existing workers.
- Providers: OneSignal sends, S3-compatible object storage and Play Billing
  proofs stay owed (recorded, not claimed).
- Free preservation: full unit suites plus cloudsync (45s), ledger (20s) and
  alerts (15s) integration suites green with `-race` at this revision.

## Known nonblocking release-only obligations

Real Play sandbox (PROOF_OWED), staging object and push provider proofs,
production certification after G24, public pilot and separate releases. Only a
full backend gate PASS permits PRM09; LOCAL_DONE here is not production
certification. PR131 stays draft with no merge, deploy, wiki or mobile edits.
