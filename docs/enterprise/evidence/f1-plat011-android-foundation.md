# F1 EMP-PLAT-011 Android partition foundation

Status: LOCAL_DONE
Validation: PASS

Date 2026-11-11. This is scoped foundation acceptance, not aggregate lint,
coverage or protected-device acceptance. Delivery remains INTEGRATION_PENDING
through [PR202](https://github.com/AlexandreZanata/Abastevo/pull/202).
F1/M1 remain open.

## Entry and implemented behavior

The owner authorized the next enterprise task and GitHub publication.
The clean enterprise worktree started at published `5927654c` on
codex/enterprise-f0-f1, matching remote dev and PR202. Main `ef96cd13`
was already an ancestor. The occupied local dev worktree and the primary
checkout's unrelated Android/source edits remain untouched.

B-BR-EMP-25, B-BR-EMP-26 and BUC-EMP-14 were documented before behavior.
Existing :domain/:application/:data/:app and com.anpfuel are preserved.
No dependency, backend migration, personal schema, business screen, endpoint
or worker/dispatcher is added. Native iOS work stays deferred.

EnterprisePartition is an immutable validated account/origin/organization
value. HTTPS origin normalization converges host case, root slash and port
443 while keeping different servers/ports distinct. IDs are canonical non-nil
UUIDs. Stored value types copy opaque bytes on input/output and redact their
string representations. These values express local ownership, not membership
or server permission.

EnterpriseLocalContext captures partition/generation in a bound handle.
Selecting another or the same partition, refused selection, and explicit lock
invalidate old handles. A mutex serializes context changes with local calls.
Each call checks the current account and configured origin before use and
before releasing its result. Changed account/origin, foreign input/result,
unbounded pages and stale handles fail. Storage errors are redacted;
cancellation propagates. Restart has no persisted active selection. A
queued operation keeps its creation scope and cannot follow the current UI.

EnterpriseDatabase v1 is separate from AnpFuelDatabase v37. Each record and
pending-operation primary key/query carries all three partition components.
The Room adapter never imports or falls back to personal tables. Its
transactional insert/replay converges only on identical operation
ID/target/envelope; changed intent cannot overwrite held work. This is a
storage/queue port, not the future SYNC operation protocol or retry engine.

Hilt reuses AuthFlow and the named apiOrigin. Active/NeedsRefresh are local
owner hints only. The production persistence binding is deliberately
UnavailableEnterprisePersistence; no database builder is registered.
Room's unencrypted adapter is used only for explicit synthetic foundation
tests. EMP-PRIV-003 still owns encryption of DB metadata/journals, files/caches,
Keystore custody, logout/revocation composition, backup/transfer and device
proof before any real organization persistence. Existing backup exclusions
alone do not prove those protections.

## Executed validation

Linux amd64, Gradle 8.10.2, existing JDK17, Kotlin 2.0.21, Room 2.6.1,
KSP/Hilt and Android SDK35. No dependencies were fetched or added. Commands
used offline/no-daemon. ANDROID_HOME and ANDROID_SDK_ROOT were set to the same
existing SDK after the first setup attempt exposed conflicting inherited
paths. No emulator, physical phone or live service was started.

- RED: the new domain tests failed with unresolved EnterprisePartition/
  storage values. GREEN: all three new domain tests passed. Invalid origins,
  nil/noncanonical identifiers, scope equality and envelope isolation are
  exercised, including rejection without echoing sensitive input.
- Full `:domain:test` passed 1,060 tests; full `:application:test` passed
  538 tests. Zero failures/errors/skips. New application cases prove
  context generations, foreign operation refusal, replay, owner/origin loss,
  explicit lock, paused in-flight read vs lock, late-result refusal, foreign
  persistence results, page bounds, default-off persistence and redacted
  storage failure/cancellation.
- Scoped `:data:testDebugUnitTest` passed 47 tests, zero failure/error/skip:
  new enterprise adapter/DI plus existing local auth, origin policy and
  staging-origin wiring. Adapter tests execute the actual DAO replay method
  with a fake storage boundary; they do not claim native Room execution.
- `:data:kspDebugKotlin` compiled the separate entities/queries and exported
  the Room schema. `python3 scripts/tests/test-enterprise-local-sql.py`
  passed five real SQLite cases using that schema and the actual generated
  insert/update SQL. File-backed rows survive a new interpreter/connection.
  Deliberately colliding IDs remain separate across account/origin/org.
  Twelve concurrent writers admit one row, duplicate insert preserves the
  original target/envelope, rollback leaves no work and generated updates
  stay scoped. An existing populated personal v37 database remains byte
  identical; the enterprise file has only its own tables.
- `:data:compileDebugAndroidTestKotlin` passed, including two new native Room
  persistence/concurrency tests. Execution remains OWED in the owner's
  deferred emulator/device batch; host SQLite is separate evidence.
- `:data:lintDebug` passed (0 errors, 18 existing warnings), with no findings
  in the new sources. `:app:assembleDebug` passed through actual final Hilt
  composition and APK packaging; no APK was installed or published.
- `:app:lintDebug` FAILED with the existing MissingTranslation described
  below. The combined test/lint invocation therefore exits nonzero; it is
  not recorded as a passing command.
- Mobile static guards, enterprise verifier/generator regressions, new
  relative links, whitespace and changed/new-file secret reviews passed. Counts stay
  348 tasks, 545 rules, 167 use cases, 145 screens, zero structural problems.
  A broader scan of historical PROGRESS links found ten pre-existing MAP
  link failures, confirmed in `5927654c`; that history is preserved.

Reproducible scoped commands and the adapter contract are in the
[storage README](../../../data/src/main/kotlin/com/anpfuel/data/enterprise/README.md).
The full regression invocation selected local auth and origin suites as
well as the enterprise class. No backend behavior changed, so the critical
new SQL evidence uses real SQLite rather than an unrelated PostGIS run.

Tested implementation blobs:

- Domain: `7ed8b817ec2a03f5199ad2e71d360461cf1acb6a`.
- Application: `ad01a124fadf07eb0d1326da5d8ca6a99f9c910d`.
- Room schema/entities/DAO: `9f1bf259d7ed22cd589d7374c62e7f3bd6fe2f5d`.
- Adapter: `075510c699f6a64fa0eaccaeef8e7bb7bbe6bbc3`.
- Hilt: `b5d3064a39a0e27136c3acd34f98784501605f9a`.
- Exported v1 schema: `a560a53c760fa589e07f69d9b8c08487bc674e27`.

## Aggregate checks and retained gates

App lint reports one error and 347 warnings. The error is the unchanged
home_open_map string at app/src/main/res/values/strings.xml:1461, missing
de/es/fr/ja/ru/zh translations. The independent `git archive 5927654c`
baseline reproduces the same one error and 347 warnings. No translation,
lint baseline, suppression or threshold is changed by this task; the primary
checkout already has unrelated resource edits. Aggregate app lint remains
FAILED/PENDING.

The pre-existing aggregate domain coverage failure remains recorded in
[EMP-PLAT-009](f1-plat009-exact-values.md). The current report, with the same
configured exclusions, is 56,029/69,964 instructions = 80.0826%, below 90%.
The new classes cover 339/363 instructions (93.39%) and 100% source lines.
Coverage verification was not rerun solely to reproduce its known failure.
No threshold, exclusion or test was weakened; no all-green aggregate or
merge readiness is claimed.

Real-data/local protection and native device execution remain unaccepted.
Claude's server activation blockers, online npm audit and other retained
obligations remain in the [review record](f1-pr202-review.md). No merge,
deployment, wiki publication or release certification is performed.
F1/M1 stay open. EMP-PLAT-012 requires the EMP-GOV-014 channel decision;
EMP-PLAT-013 is the next eligible platform task. EMP-PRIV-003 remains the
required Android storage activation dependency.

## Second PR202 review correction (2026-11-11)

At `ec92ab38`, two of the three `RoomEnterprisePersistenceTest` methods,
including the production-graph test, were expression-bodied with a non-Unit
result, so JUnit Jupiter did not discover them. The 47 data tests above
therefore ran one of those three. `ef8bfff9` declared them `: Unit`, and the
current report runs all three. The production-graph test now passes the
shared session fence explicitly instead of a private default.
[Review record](f1-pr202-review.md#second-review-head-1f99b018).
