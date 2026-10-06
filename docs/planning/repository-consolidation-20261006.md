# Repository consolidation and maintained branches

Status: LOCAL_DONE / INTEGRATION_PENDING
Validation: PASS

The user explicitly authorized consolidating every mobile/backend branch into main, merging the cumulative PR, closing all open issues, removing old work branches after verified ancestry, synchronizing the local checkout, and using dev → main for subsequent development. This authorization does not certify production or require a deployment.

## Behavior and acceptance

B-BR-DELIVERY-01: main receives every existing branch tip through preserved ancestry. Only integrated branches may be deleted; dirty detached worktrees and their files remain available. dev starts from the merged main and receives future changes before a protected PR merge.

B-BR-PROFILE-CONTAINMENT-01: production private representation claims, owner status, proof intake, edits and replies must refuse with 503/no-store before reading request bodies or touching ports while contributor proof/binding and private proof storage are unaccepted. Public profiles and existing account/community behavior remain available. No token-only path may create claims, reveal private status, edit profiles or submit business replies. Restore these routes only in a tested integration of the frozen proof and storage ceremony.

BUC-DELIVERY-01: preserve all existing task commits and current docs in PR #122, run affected backend/PostGIS/Android checks, verify current-head required Quick verification and main ancestry, guarded merge, then safe branch cleanup and dev initialization.

BUC-PROFILE-CONTAINMENT-01: unsigned, signed, token-shaped, replayed and oversized private requests receive the same non-cacheable unavailable response; no handler execution or private database writes. Actual API-process/PostGIS validation proves mounting of all eight routes.

## Retained obligations after administrative issue closure

- #118: integrated public profile source; full live provider/runtime validation remains separate.
- #119: claim export/import adapter and device evidence preserved; private runtime remains unavailable until contributor proof/binding and storage are accepted.
- #120: supported business management source preserved; invitation, contest and reverification remain deferred.
- #121: scoped Android UI/URI acceptance preserved; TalkBack, novice, low-memory and full performance matrix remain owed.
- #123: city-scoped feed source integrated; no staging deployment or successful staging TLS smoke is claimed.
- #60–#62: historical G18 multiplatform acceptance remains NOT_ACCEPTED; native iOS explicitly deferred, Android full manual/performance/security matrix not retroactively certified.
- #13: G09 production RELEASE remains UNCERTIFIED, including real private storage/provider/restore/load evidence.
- #53: controlled public pilot remains deferred and unlaunched.

Implemented source issues may close through the merged PR. Incomplete release/acceptance trackers close administratively as not planned on the user's explicit request; this record preserves their obligations for future work. No deployment, release tag, public pilot or certificate-trust bypass is authorized.

## Local draft preservation

All local and remote branch tips are ancestors of d18942c. Dirty detached brand/icon worktrees contain historical post-merge notes; incorporate useful notes into canonical records. The station-catalog planning worktree contains older planning drafts superseded by the current plans; preserve its files in place, detach it without changing its revision, and do not overwrite current plans with stale drafts. Old worktree directories are retained rather than deleted.

## Final local validation

Source anchor d18942c plus the consolidation changes; no Android behavior was changed during consolidation.

- RED: private-profile containment regression failed because the guard did not exist; GREEN: targeted test passed after the production middleware was added.
- `ANPFUEL_TEST_DATABASE_URL=<documented disposable local PostGIS DSN> go test -race -count=1 -tags=integration ./...` from backend: PASS, including actual API process, all eight contained routes with unsigned/signed/replayed requests, unchanged private rows, auth/ownership/concurrency, migration and city-feed suites. A wrong test-envelope assertion was corrected before the final passing run.
- `bash scripts/check-backend-fast.sh`: PASS; full Go build/vet/unit/staticcheck, sqlc vet/diff, OpenAPI and secret review. OpenAPI 58 warnings / 39 informs, no errors.
- Android `:domain:test :application:test :data:testDebugUnitTest :app:testDebugUnitTest :app:lintDebug :app:assembleDebug :app:assembleDebugAndroidTest :app:assembleRelease :app:verifyReleaseApkSize`: PASS. Release APK 3,123,082 bytes, within 15MiB budget.
- Unchanged Android source retains scoped actual Poco Home/icon/Community and Poco/API26 profile evidence in the linked source records; this consolidation does not claim new live-provider or full manual certification.
- `bash scripts/tests/test-git-flow.sh`: 39 PASS / 0 FAIL. Project-batch flow harness PASS; CI cadence 2/2 PASS; shell syntax, changed-document links, repository baseline, secret surface and whitespace review PASS.
- The required remote checks, guarded merge, actual merged SHA, issue closure, branch cleanup and wiki publication are recorded in PR #122 metadata after execution. This source record does not fabricate future outcomes.
