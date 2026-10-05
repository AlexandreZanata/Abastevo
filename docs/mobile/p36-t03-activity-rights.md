# P36-T03 — Private activity and account rights

Status: LOCAL_DONE on `codex/phase-36-live-community`. Task: P36-T03.
Binds P13/P22 account/privacy rules: deletion revokes sessions and
clears local data; export ships only after a scoped backend
contract/privacy decision with owner-isolation tests. No destructive
change; no device run.

## Behavior (DDD)

- `StationsViewModel` binds `serverAccountId` from `AuthSessionStore`
  once per server selection (nullable binding, legacy construction
  unaffected): signed sessions authenticate discussion writes as the
  session owner; missing/cleared/failed stores fail closed to honest
  guest (`""`, reads only). Store exceptions never break detail
  selection. T02's `StationFeedbackSection` consumes the id with
  `SignInRequired` gating intact.
- Deletion journey (verified, not rebuilt): app two-step confirm +
  `Deleted` notice with failure-keeps-session (`AuthViewModel`,
  P22-T01); portable `deleteAccount` clears the local store then calls
  server `POST /v1/accounts/deletion`; backend `DeleteAccount` marks
  the account deleted, revokes **every** session family and drops
  addresses/provider bindings/contributor bindings atomically
  (`PGStore`, one transaction; the row stays as an audit record with no
  reputation carryover).
- Own activity/status/retry reuse: feedback `retry()` reuses the stable
  op id (no amplification); queued ops replay offline; abuse denial,
  concurrent vote/report and revoked-session paths stay in the reused
  transports (existing P22-T04 matrices, rerun below).

## Frozen export decision (NOT IMPLEMENTED)

- Contract: `POST /v1/accounts/export` (POST convention, session proof
  in JSON body, never URL) returning only caller-owned rows: account
  status/created, provider subjects (opaque), contributor bindings,
  own ratings/comments/votes/reports with server timestamps. No
  passwords, keys, nonces, precise GPS or other users' data.
- Privacy: proof-required, owner-isolated (account-id predicate on
  every row), rate-limited, `no-store`; revoked/deleted accounts
  denied; erasure tombstones replay on restore.
- Owner-isolation test plan (owning implementation slice): foreign
  account id cannot read another export (IDOR 404/denied), expired/
  revoked session denied, concurrent export-during-delete is
  serialized, empty account exports an honest empty envelope.
- Implementation is explicitly deferred to its owning backend slice;
  no endpoint, migration or client call is claimed here.

## Validation

- NEW: `StationsServerDiscoveryTest` +2 (signed binds id, guest blank;
  failing store fails closed, detail still opens) → suite 10/10 PASS.
- Regression: full `:app:testDebugUnitTest` (incl. untouched
  `AuthViewModelTest`/`StationsViewModelTest`) PASS;
  `:app:assembleDebug` PASS.
- Backend: `go test -race ./internal/modules/account/...
  ./internal/modules/identity/...` PASS; real-PostGIS `-tags=integration`
  account PASS; `-race` feedback + moderation unit PASS; real-PostGIS
  `-race` feedback + moderation PASS (expired/IDOR/concurrent/offline/
  abuse matrices rerun, 0-fail).
- Contracts unchanged: `vacuum lint` + `apicontract` PASS; live `curl`
  60 unchanged (no bypass).
- `git diff --check` PASS; `scan-secrets.sh` PASS.

## Limits and next

- BLOCKED_LIVE: account/privacy reads/writes unverified from this
  runner (TLS trust gap). OWED: export endpoint implementation per the
  frozen decision + device/session proof at the end manual batch.
- OWED: end manual/device batch, media proof. No PR/CI/merge/wiki per
  ADR-018. iOS archived. G09 UNCERTIFIED.
- Next: P36 phase exit (LOCAL_DONE checkpoint + evidence), then P37
  (`codex/phase-37-live-contributions` via `--from-checkpoint`).
