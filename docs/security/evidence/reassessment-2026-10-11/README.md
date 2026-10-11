# Independent reassessment evidence — 2026-10-11

Audited tracked revision: `603d5ba8acd32da6b5b0de72988ac614e8c18ee6`.
Source snapshot: `/tmp/abastevo-review-final-20261011-kk55rep3`, created with
`git archive HEAD`. Existing dirty/untracked planning documents were read
separately; they are not part of this tested commit. No runtime fix was applied.

See [summary.json](summary.json) for counts and probe SHA-256 values, the
[assessment](../../SECURITY_OFFLINE_REASSESSMENT_2026-10-11.md) for interpretation,
and the [correction plan](../../../planning/SECURITY_OFFLINE_CORRECTION_PLAN_2026-10-11.md).

## What was actually run

- Backend: `GOMAXPROCS=2 go test -race -p 2 -count=1 ./...` from the snapshot's
  `backend/`, using `/tmp/abastevo-audit-go-cache`. All 71 tested packages passed
  in [backend-unit.txt](backend-unit.txt). An initial restricted run could not
  bind `httptest` sockets; the authorized rerun is the reported result.
- Real PostgreSQL/PostGIS: eight adapter/HTTP packages passed with `-race`,
  `-tags=integration`, `-p 2`, `-count=1`, `-timeout 8m`; see
  [postgis-integration.txt](postgis-integration.txt). Packages: account/adapters,
  privacy/adapters, community/adapters, evidence/adapters, feedback/adapters,
  identity/adapters, privacy/adapters/http, community/adapters/http.
- Ten added Go unit characterizations: [go-characterization.txt](go-characterization.txt)
  and [oidc-characterization.txt](oidc-characterization.txt). Three additional
  real-SQL characterizations: [sql-characterization.txt](sql-characterization.txt).
  Source is retained under [probes](probes/) as `.go.txt`, not installed into the
  application. Counts overlap aggregate package execution; do not add them as
  independent aggregate suites.
- Android/JVM: 43 tests in [android-selected.txt](android-selected.txt), then 51
  disjoint affected tests in [android-ui-auth.txt](android-ui-auth.txt), all
  passed. The first group includes five added characterization cases retained
  in [android-characterization.patch.txt](android-characterization.patch.txt).
  Tests used cached JDK 17, existing Android SDK, offline Gradle, two workers,
  no dependency installation. These are JVM tests, not phone/UI acceptance.
- Parser: a child JVM with `-Xmx32m -XX:MaxMetaspaceSize=64m` ran unchanged
  compiled production parser classes and the synthetic harness in
  [ReviewParser.java.txt](ReviewParser.java.txt). A 172,161-byte compressed XLSX
  containing 4,096 allowed-length shared strings exhausted the heap; see
  [parser-heap.txt](parser-heap.txt). The synthetic temporary XLSX was removed.

**A passing characterization means the defect was reproduced. It is not a
passing safety requirement.** Remediation must invert the assertion and show
the unsafe behavior is refused/recovered without weakening existing tests.

## Reproduction boundaries

1. Archive the exact commit into a new disposable directory. Copy only the
   seven listed probe files to their original paths after removing `.txt`;
   apply the Android patch only inside that snapshot. Hashes in summary.json
   cover the copied probe bytes. Use `go test -run '^TestReview' -v` in the
   relevant packages; add `-tags=integration` for the two SQL probe files.
2. Verify the Docker endpoint is local. Start only `db` from
   `infra/compose.validation.yml`, with a unique project name and loopback port.
   The review used pinned PostGIS 18/3.6 and disposable tmpfs storage. Pass the
   selected loopback database URL through `ANPFUEL_TEST_DATABASE_URL`; do not
   put real credentials in retained logs. Run integration packages serially
   within their normal suite isolation rules.
3. Android commands, from the snapshot root, with the existing JDK 17/SDK:

   ```text
   ./gradlew --offline --no-daemon --max-workers=2 -Dorg.gradle.parallel=false :application:test --tests '*TrainingConsentUseCaseTest' :data:testDebugUnitTest --tests '*RoomContributionOutboxRepositoryTest' --tests '*TrainingConsentHttpClientTest' --tests '*AnpFileDownloaderTest'
   ./gradlew --offline --no-daemon --max-workers=2 -Dorg.gradle.parallel=false :application:test --tests '*Sec07c*' --tests '*DispatchContributionUseCaseTest' :data:testDebugUnitTest --tests '*CommunityHelpfulnessHttpClientTest' --tests '*OutboxHistoryScaleTest' :app:testDebugUnitTest --tests '*CommunityHelpfulness*' --tests '*TrainingConsent*' --tests '*TrainingPhotoOptIn*'
   ```

4. Compile the retained Java harness against the snapshot's compiled data/domain
   classes and existing Kotlin/kxml dependencies; invoke `ReviewParser` with
   the bounded heap above. Keep the child-process budget; a full app OOM or
   production resource stress is unnecessary to demonstrate the retained-memory
   gap. The 32 MiB result does not estimate the supported device's actual heap.
5. Remove only the uniquely named local Compose project created for this run.
   This review project was successfully removed, including its disposable
   volumes. Existing SEC05-E/CS10C soak projects were not touched.

## Document closure checks

Local links, seven probe hashes, finding/task IDs and whitespace checks passed.
`check-security.sh --static-only` and the tracked-file secret gate passed; an
additional review covered the owned untracked documents/evidence that the tracked
gate excludes. See [documentation-checks.txt](documentation-checks.txt). These
are structural checks, not a new vulnerability advisory scan.

## Limits

No VPS, staging, real storage provider, phone, production photograph, real user
account, existing soak, GitHub or wiki mutation occurred. No new wall-clock
24-hour proof or dependency/container advisory scan was performed. No maximum
users, production readiness, complete offline support or exhaustive absence of
vulnerabilities follows from these results.
