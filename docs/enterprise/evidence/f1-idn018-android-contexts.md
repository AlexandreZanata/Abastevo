# EMP-IDN-018 — Android personal and company context selection

Date: 2026-11-13.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Construction source: tested017 checkpoint `8728dff6` on owned
`codex/enterprise-idn016-018`; [task214](https://github.com/AlexandreZanata/Abastevo/issues/214),
cumulative [PR203](https://github.com/AlexandreZanata/Abastevo/pull/203).
B-BR-EMP-94/95, BUC-EMP-48 and EDR-059 precede behavior.

Use the actual current-account cookie context endpoints from016, never guessed
routes. Retain only bounded memory metadata, not persisted authority. Explicit
selection rechecks the exact organization and membership; personal return and
switch attempts synchronously withdraw the preceding local handle. Test late
I/O and retained original-partition work without claiming rollback or enabling
the unavailable protected persistence/contact authority. No migration, new
production dependency, managed activation, operational route or queue dispatcher.

Required acceptance: meaningful domain/application/HTTP cases and affected
Android unit/build/lint/static checks, two-company physical-device journeys,
offline/background/refusal/late-result/recreation and personal separation.
Use only the isolated validation package on the actual connected ADB phone;
preserve the normal app and its data. Final Quick and union regression follow
the frozen018 implementation, before publication. No F1/M1 closure, production
activation, merge, wiki or deployment is claimed.

## Implementation and review

The selector uses one bounded page, including empty eligible pages with a next
scan cursor. A selection always opens the current personal proof and reads the
exact membership again. Catalog confirmation and selected-company confirmation
are distinct. A sticky company name remains visible while scrolling; every
selection button names its company for accessibility. Personal return removes
the private navigation entry; system back opens/closes the selector without
silently changing the company. No operational shortcuts, queue counts or offline
grants are inferred. All24 new strings have matching placeholders in eight
locales; base Portuguese equals pt-BR.

Independent review found and corrected two lifecycle races before acceptance.
Outgoing invitation/context cleanup could cancel a newer destination using the
shared transport. Each unscoped gateway now owns an opaque client owner; opens
acquire the channel, old cleanup is conditional and an old gateway cannot borrow
the new owner's channel. The client remains singleton and keeps the same-account/
origin login-only retirement hint. No extra company-cookie pool is introduced.

The shared local context similarly uses per-flow owners and per-attempt immutable
leases. Owned lock/select checks the lease after mutex waits before mutation;
select consumes it. Cleanup can withdraw only the current owner, while a stale
attempt cannot invalidate a newer attempt of the same flow. Post-I/O compare-and-
set withholds old results without revoking a newer reserved lease. Explicit
global logout/fence invalidation remains available. Deterministic regressions
cover old-flow closure, old selection/personal lock waiting on I/O, same-flow
replacement and both transport handoff directions plus same-route replacement.

## Backend union validation

The backend and contract artifacts are unchanged throughout018. Scoped final
real PostGIS/race acceptance passes: organization11 top-level tests/14 cases
(14.629s), all companysession18 top-level tests/47 cases across four packages
(adapters16.307s), and the complete API contract44 top-level tests/570 cases
(116.404s). Every suite has zero failures/skips. Integration-tag vet1.586s and
staticcheck3.998s pass. Ordinary current-context migration recovery, account/
organization isolation, proof waits and native cookie rotation remain covered.

```sh
go test -json -tags=integration -race -count=1 -timeout=20m ./internal/modules/organization/adapters -run 'TestAccountContext|TestEmployee(Register|Profiles|AccessView)'
go test -json -tags=integration -race -count=1 -timeout=20m ./internal/modules/companysession/...
go test -json -count=1 ./internal/platform/apicontract/...
go vet -tags=integration ./internal/modules/organization/... ./internal/modules/companysession/...
GOTOOLCHAIN=go1.27.1 XDG_CACHE_HOME=/tmp/enterprise-staticcheck staticcheck -tags=integration ./internal/modules/organization/... ./internal/modules/companysession/...
```

These run from backend/ using the explicit owned disposable PostGIS URL,
GOCACHE=/tmp/enterprise-go-build and pinned Go1.27.4 (staticcheck tool1.27.1).
Tested backend tree:`a0a04682ed790780f53071da7d208c6f25be2e47`;
Contract Git object: `44e16fe9003b1f4ac2fd73c994007ba92a2c28ea`.
## Android source acceptance

Domain RED preceded implementation. Lower GREEN passes in39s: the three new
domain context tests, application enterprise31 tests (context12, local11,
invitation8) and data debug enterprise56 tests (context HTTP7, invitation HTTP17,
protected-vault/Room/process32), with zero failures/errors/skips. A compiler
refusal of a cross-module nullable smartcast was fixed by capturing the cursor
locally; no assertion was removed. Independent final boundary review passes.

The normal application union passes in3m52s: domain enterprise231 and app36
tests (context VM3, invitation VM3, existing auth VM29 and reauthentication1),
with zero failures/errors/skips. Normal debug assembly and full app lint pass.
The normal APK retains com.anpfuel.app.debug and contains no instrumentation
fixture/HiltTestApplication classes; it is never installed on the phone.
Four protection checks, five local-SQL harness checks, twenty Android release
checks and the348-task plan verifier pass, with zero plan problems.

The initial local Quick attempt stopped before the fast suite because the host
Go1.22 formatter differed from the project's pinned Go1.27.4 formatter. No
backend change was needed; the pinned formatter produces no diff. The actual
final Quick uses the pinned toolchain first on PATH and passes in228s, including
the full Go fast gate, sqlc drift, contracts, business-web30 tests/artifact checks,
mobile/static/deployment guards and vulnerability scan (no vulnerabilities).
No gate or format assertion was weakened. Strict required Quick verification
and administrator enforcement on main were reverified through the actual GitHub
protection API. Required remote acceptance still applies to the published PR
head; local results do not certify CI, integration or production.

Tested source trees (unchanged after the final code freeze):

- App main:`95a4053a32d9523660b17de8626469c4bc78ca3c`.
- Native fixture tree:`e4ebcaddb23170561286ea2ef313a31f9760f7c5`.
- Native context test Git object:`12af3e07cd4fb87a18f946fe1f60a66f31c227f1`.
- Native context bindings Git object:`40f64042e4efb85f968917d391259e01538cc21f`.
- Application main:`bb86c9164d9d2286f8b0a177b3f9ac51d2dfdf5b`.
- Data main:`046c2af5efd132ba4131f388aa4cff0c36ee050b`.
- Domain main:`828bee157cb7257e3445b091b4949e1fe01550ac`.

## Physical Android acceptance

The final isolated app/test builds and full app lint pass in3m08s; the final
test-only harness adjustment also passes affected assembly/lint in33s. Both
invitation and context callbacks capture primitive rendered epochs in compiled
bytecode. Manifest/DEX checks prove the exact isolated application/test IDs and
runner, with no legacy provider or instrumentation fixtures in the normal APK.

The first combined physical run executed12 tests:11 passed and one failed on a
test assumption that refreshing an already selected company must close the
selector. The contract does not require that navigation; immediate source
results can coalesce confirmation states. The harness now waits for exact
completed list/read counts and follows the actual open/closed selector before
continuing pagination. CompanyA/B, active selection, header, source counts,
privacy, back and personal-return assertions remain. No main source changed.

The final physical Android16/API36 run executes12 actual tests with zero
failures/errors/skips in85.538s: six invitation regressions plus two-company pagination/
switching/back/personal return, offline/background unconfirmed metadata, late
read/definitive refusal, Activity recreation, and the ordered warm/cold process
pair. The cold pair preserves the existing synthetic account session without
logout or package-data clearing between methods, force-stops only the isolated
target and verifies a fresh instrumentation process starts without selected
company authority. Only a test marker boolean is persisted by that fixture.
Zero-test instrumentation output is rejected.

Final report:
`app/build/reports/enterprise-device-validation/idn018-all-20261113T113150/TEST-EnterpriseDeviceValidation.xml`.
Application APK SHA256:
`477eb2591ffc75a1ef8c49fc775199196777847c3a45a6b607e603c4d60b3811`;
test APK:
`d0279e16dafd4dc7c97a8d6c269a74c47d6925cda9d1ea5a592d89e57a1bfcec`.
Installs use ordinary `adb -d install --no-streaming -r -t` and manual acceptance
if requested by Android. Each method runs through the exact Hilt runner with
`adb -d shell am instrument -w -r -e class CLASS#METHOD`; the driver checks one
completed test per method and writes the aggregate XML. Normal app metadata is
unchanged before/after every method. No emulator, automatic installer click,
security-setting change or access to the existing app's private data is used.

The stateful cold pair must run in this order in separate instrumentation
processes, preserving the isolated data between its methods. A default
clear-after-every-method orchestrator run does not reproduce that case:

```sh
adb -d shell pm clear --user current com.anpfuel.app.enterprisevalidation
adb -d shell am instrument -w -r -e class com.anpfuel.app.ui.enterprise.EnterpriseContextDeviceTest#prepareCompanySelectionForTheFollowingFreshProcess com.anpfuel.app.enterprisevalidation.test/com.anpfuel.app.ui.enterprise.EnterpriseDeviceTestRunner
adb -d shell am force-stop com.anpfuel.app.enterprisevalidation
adb -d shell am instrument -w -r -e class com.anpfuel.app.ui.enterprise.EnterpriseContextDeviceTest#aFreshProcessStartsPersonalWithoutReplacingTheExistingAccount com.anpfuel.app.enterprisevalidation.test/com.anpfuel.app.ui.enterprise.EnterpriseDeviceTestRunner
```

Each call must report exactly one completed test and no failure/skip. All other
methods in the two dedicated device-test classes run independently with only
the isolated target cleared first. After final acceptance the isolated target
was stopped and its synthetic data cleared; the normal app metadata remained
unchanged. No test package or pre-existing helper/application was uninstalled.

This closes the requested018 local task. F1/M1, protected-vault/contact, managed
access, actual offline intake, runtime/legal/staging/release gates remain open.
