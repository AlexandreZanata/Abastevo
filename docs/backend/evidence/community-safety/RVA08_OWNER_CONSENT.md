# RVA08 owner-bound Android consent

Status: IN_PROGRESS

Dependency source checkpoint: RVA07 B3.3c.2 corrected successor `7f3c9e7`. RVA07 external elapsed/
provider/backup/device qualification remains PROOF_OWED and is not consumed as
accepted production evidence. Training collection remains OFF.

## A1 view ownership opening — 2026-10-12

B-BR-RVA08-01: consent UI state belongs to the currently prepared account and
view generation. Logout clears the retained caller immediately. A late refresh
or update from an earlier generation may not publish content, trigger a reload
under that generation, or clear a newer generation's single-flight guard.
Changing account starts a fresh loading state even if the previous account had
an update in progress. Repeated refreshes are ordered within the active view;
older responses cannot overwrite later results.

BUC: A refresh pauses, switch to B and complete B, then resume A: B stays visible.
A refresh/update pauses, logout, then resume A: guest stays visible and retry/
toggle perform no account IO. A update pauses, switch to B and start B update,
then finish A: A cannot reload or release B's update guard. Two same-owner
refreshes finish in reverse order: latest requested state wins.

Risk: private consent disclosure and misleading attribution. Scope: ViewModel
and coroutine regression tests only. Durable owner+origin command journal,
atomic OFF, server-revision/multi-device reconciliation, signed HTTP/private
reads, settings activation and device proofs remain required following tasks.
The UI fence is not a transport ownership or durable consent guarantee.

### A1 local checkpoint

Status: LOCAL_DONE
Validation: PASS

RED: the four new coroutine regressions all failed against the prior ViewModel
(12 tests, four failures). GREEN: UI decisions now run on Main and IO returns
to Main before generation checks, avoiding a check/publication race. Every
prepare clears old authority/guard; later requests invalidate earlier work.
Old mutations cannot reload or release the current owner's guard; logout retry
has no retained account. Completed receipts as well as late reads are covered.
The complete app JVM suite passed: 290 tests, zero failures/errors/skips.
Targeted consent class: 12 tests passed. Android lint initially failed on 27
untranslated helpfulness/consent strings from prior construction. Completed
all 27 keys in the seven existing locales, including pt-BR; XML/format
placeholders match the default resource. Full app tests plus lint passed in
32s after repair. No lint suppression/baseline/dependency addition was used.
The final resource wording avoids promising one human per vote or immediate
server deletion without an online acknowledgement; it does not activate terms
or the training feature. Existing nonfatal lint warnings remain in the report.

Commands from repository root: JDK17/SDK35 installed local paths, Gradle offline
max-workers=2 :app:testDebugUnitTest --tests '*TrainingConsentViewModelTest'
(RED then GREEN); :app:testDebugUnitTest :app:lintDebug; XML/placeholder
consistency script and git diff --check. Staged secret check before commit.
No phone/VPS/provider/elapsed soak/GitHub/merge proof belongs to this checkpoint.

Next A2: durable owner+origin+key scoped commands and atomic conservative OFF,
revision reconciliation and scope-safe resumption; C signed transport and actual
backend-verifier interoperability still required. Current global preference/
unsigned gateway remain known pending corrections, not certified by UI tests.

## A2/C executable consent opening — 2026-10-12

B-BR-RVA08-02: durable consent commands are scoped to account identity + signing
key + exact HTTPS origin. Global legacy preferences are never an authority or
a queue that can be reassigned. OFF and its command commit atomically before
network IO, survive ambiguous replies/process restart and dominate older grants.
Stable command identity/body is reused after ambiguous delivery. Initial server
revision seeds new decisions; stale grants never automatically rebase. A
confirmed stale OFF may rebind to a freshly observed revision with bounded work.
All private reads and writes use the established signed transport with expected
scope checks, no anonymous fallback/redirect/implicit mutation retry.

B-BR-RVA08-03: one full UTF-8 terms text has an exact version/hash on both client
and server; review is independent of the optional permission. A fresh unchecked
checkbox and explicit confirmation are required; opening/scanning/scrolling alone
never grants. Terms rotation invalidates prior review. The complete text is
available offline; network grants must bind the currently published pair.
Collection remains disabled while content minimization is unqualified; receipt
of a preference must never claim image collection or physical deletion completion.
Revocation, image deletion requests and rights access cannot require accepting
new terms. No consent gates free participation.

BUC: restart after offline OFF/lost reply; A→B/key/origin switches before/during
IO; revision 100/multiple-device conflict; stale grant after OFF; corrupt/disk
failure/oversized local records; expired auth/429/503; exact signed verifier;
unchecked/terms-changed/no-read confirmation and complete offline document.
Preserve bounded memory, disk/attempt quotas and truthful pending/received states.
Real-phone and VPS proofs require the actual device and Abastevo-only environment;
no elapsed acceptance can be inferred from a test clock. Existing unqualified
retention/processor/privacy-channel activation details are retained explicitly.

B-BR-RVA08-04: the authenticated account's signing-key alias must also be scoped
by account and HTTPS origin. A local journal prefix alone cannot isolate server
consent when two accounts reuse the same installation key. Account aliases use
an opaque SHA-256 suffix, retain keys across logout for original-owner recovery,
and never adopt the legacy anonymous alias as an authenticated owner. Existing
commands tied to that legacy key remain quarantined rather than being re-signed
under a replacement identity. At most 32 scoped account aliases are admitted;
refuse further creation without evicting retained keys/rights. Anonymous staging
keeps its existing alias, other origins have distinct anonymous aliases. Access
expiry retains the same account alias; pending-key recovery grants no signing
authority. Native Keystore execution remains device evidence, not a JVM claim.


### A2/C local consent checkpoint — 2026-10-12

Status: LOCAL_DONE
Validation: PASS

Scope: durable owner consent and the actual optional-consent settings surface;
whole RVA08/full offline program remains IN_PROGRESS. Isolated managed worktree
`codex/owner-consent` starts at b83625d. The original dev checkout contains an
unidentified removal of the tested evidence reservation gate and remains untouched.
It must not enter delivery. No push/PR/merge/deployment is claimed here.

RED: two application regressions failed (10 tests, two failures): online OFF on
transport error retained ON, and account B consumed account A’s pending OFF.
GREEN: synchronous atomic journal per account/key/origin, 32 records/64KiB max,
strict decoding, poisoned instance after failed disk commit, retained OFF/body/
revision/backoff across reconstruction, scope refusal before and after IO, and
no automatic ON. A new grant cannot manufacture a higher revision from a local
floor ahead of server history. Confirmed stale OFF may reconcile once; ambiguous
OFF keeps its assigned revision. OFF commits before waiting for an in-flight ON.
The VM also lets OFF overtake ON and fences its late UI result. Legacy global
preferences/legacy-key commands remain quarantined, never assigned to a new owner.

The signing aliases now separate authenticated accounts and origins, preserve
expired-session ownership/recovery and retain old account aliases after logout;
there are at most 32 admitted account aliases and no eviction of pending rights.
Shared signed transport sends fresh server nonce/proof on each actual attempt,
checks exact prepared scope, refuses redirects/implicit mutation retries and caps
success/error documents at 64KiB. It retains only bounded error codes, never remote
messages. Consent honors full delta-seconds/HTTP-date Retry-After on 429/503,
saturates overflow safely and persists its retry time. Contribution conversion
also saturates; its separate attempt/history/worker budget remains RVA10, not
certified by this consent correction.

The actual Settings route has privacy and rights cards, complete selectable
pt-BR terms available offline, an end-of-document “finished reading” action,
an initially unchecked accessible checkbox and a separate explicit confirmation.
Reading/scrolling alone never consents. Owner, key, origin or terms changes reset
review. Revocation/deletion request and rights remain available without accepting
terms or having a loaded document. Pending/offline/received states never assert
physical deletion completion. Periodic WorkManager (15 minutes, network required)
and startup/foreground attempts resume only the current original owner's OFF;
no ON or historical-owner scan. A wake has a 90s outer bound; persisted retry
limits avoid repeated HTTP during backoff. Android may delay background work;
there is no claim of immediate server receipt while offline.

The first actual text is shared directly from
`backend/internal/platform/legalterms/text/training-consent-pt-BR.txt` into Android
assets; public HTTP and configuration use its SHA-256. The former literal-only
provisional digest is rejected at startup if explicitly configured. Update the
Abastevo deployment’s old override together with this artifact, or omit it to use
the embedded exact digest; never point the grant validator at an unrelated hash.
Existing grants on an old digest require explicit review again. Text rotation
also requires an updated app artifact with the new reviewed document. Full draft
community/offline promises remain inactive. Digital privacy-channel operation,
processors/regions and retention are not falsely declared verified. The runtime
text uses the supplied company postal address for rights and explicitly explains
collection OFF, no retroactive selection, future re-consent and request vs deletion.
Reference rights source: https://www.gov.br/anpd/pt-br/assuntos/titular-de-dados/direito-dos-titulares.

Cross-runtime proof: opt-in Go integration fixture owns a fresh PostGIS database,
real production verifier/registrar/store and TLS certificate. The actual Kotlin
PhotoProofTransport + TrainingConsentHttpClient make signed HTTP requests using
synthetic EC keys; enrollment/challenge seams are test-owned, not permissive
consent handlers. Seeded server revision 100 → Kotlin grant 101 → revoke 102 →
exact replay; stale grant refuses, forged signature and unsigned private GET
refuse, other signed owner sees default OFF. Public document hash/text agree;
final persisted owner state is REVOKED/102. Generated credentials stay in a 0600
temporary file removed with the fixture, never fixtures/Git/logs. This is JVM
transport interoperability, not an Android Keystore/device/provider claim.

Validation commands (repository root, installed JDK17/SDK35, Gradle offline,
max-workers=2; GOCACHE=/tmp/abastevo-audit-go-cache, GOMAXPROCS=2 for Go):

- `./gradlew :domain:test :application:test :data:testDebugUnitTest :app:testDebugUnitTest :app:lintDebug :app:assembleDebug :app:assembleDebugAndroidTest`.
  Full affected JVM suites, lint and both APKs. Native tests compile; not executed.
  Fixed pre-existing native auth fixture missing its required erasure dependency
  using owned in-memory Room, preserving the original credential/logout assertions.
  Two opt-in data JVM tests are excluded without their live/harness environment;
  the new cross-runtime test is executed explicitly by the Go harness below.
- `ANPFUEL_ANDROID_CONSENT_INTEROP=1 go -C backend test -race -p 2 -tags=integration ./internal/modules/privacy/adapters/http -run TestAndroidConsentInteroperability -count=1 -v` with explicit local disposable DSN and JDK/SDK environment.
- `go test -race -p 2 -tags=integration ./internal/modules/privacy/... ./internal/platform/config ./cmd/api` with the owned loopback PostGIS/MinIO fixtures: all requested existing packages passed (privacy SQL/S3 21.230s, signed HTTP 4.416s, API 4.152s).
  One mistyped extra package `internal/platform/contracts` made that command exit
  FAIL; it was not treated as a pass. Correct `internal/platform/apicontract`
  subsequently passed the frozen vectors/OpenAPI self-validation (2.499s).
- Affected `go vet`, gofmt, OpenAPI vacuum lint (no errors; existing warnings),
  diff whitespace and staged secret-surface review.

Remaining: run the compiled native review/checkbox/OFF and two-alias Keystore
cases on the actual phone; two ADB inspections returned no connected device.
RVA07.C real elapsed/provider/version/backup/temp/device evidence and measured
Abastevo-only deployment budget remain OWED. Training collection remains OFF.
RVA08 helpfulness integration is still separate pending work, followed by RVA10–14
and CS15–25. This checkpoint does not certify the complete offline feature matrix,
VPS throughput, 24h retention, release, public launch or GitHub integration.


Final local inventory: 490 domain + 341 application + 350 data + 291 app JVM
tests discovered; zero failures/errors, two explicit opt-in skips in the regular
data run. The Android↔Go opt-in subsequently passed for real (11.14s, Go race
package 12.152s); the ANP live-catalog opt-in is unrelated and remains outside
this task. Lint/debug APK/native-test APK pass. An additional isolated native
SharedPreferences case verifies synchronous disk XML and owner-safe reopen;
compiled, not executed. Actual terms SHA-256:
`4a9763192620d267311f49fd43c9f4502144fa3bf78edc04b4112982e87bf8ce`.
The evidence Store retains corrected predecessor hash
`f495194cda7304ad0bb2949a1db80d09acf24de9976b4ab0f60a847a8fc5c525`.
No original dirty checkout change has been copied or discarded.
