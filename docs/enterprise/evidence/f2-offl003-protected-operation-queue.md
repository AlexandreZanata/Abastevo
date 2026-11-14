# EMP-OFFL-003 — Protected Android operation queue

Date: 2026-11-13.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Continue on owned codex/enterprise-f2 from tested/published OFFL-002 d24b0d4f,
backed up at origin/codex/enterprise-f2. At opening, six checkpoints were local
and58 audited nodes remained; full F2/M1/M2/C1 was still open. PR203 dev still has the earlier batch;
whole-phase publication stays dev → main, without construction CI waits,
merge, deployment, wiki or a real company pilot.

Read-only queue/protection/transport preparation preceded B-BR-EMP-115..122,
BUC-EMP-58..60 and EDR-076..080. The current opaque enqueue is not a completed
queue. Extend the accepted protected generation store with typed binding,
atomic local save/ACK projection, actual producer eligibility, strict signed
delivery, dependency-aware retries and opaque WorkManager scheduling. Preserve
legacy work unsent and personal/other-scope storage; no plaintext Room outbox,
new dependency/service or fake production producer is selected.

Use exact source/attempt leases independent of UI selection ownership, with
storage/fence released before network. Local account capture/save is offline
and does not refresh a15-minute access token; delivery alone refreshes the same
family before a new capture. This distinction was recorded in B-BR-EMP-117
before its implementation, preventing an accidental online-only local queue.
The server is dependency authority;
review ACKs remain immutable while children can later retry. Source defaults
are30s/6h equal-jitter retry without attempt ceiling and80% warnings against
the already accepted32MiB/10000-entry safety limits. Final receipts remain
retained until an owning explicit policy exists. These recommended reversible
technical choices are recorded for the maintainer's later review.

## Immediate acceptance

RED→GREEN pure protocol/queue/application cases, meaningful protected-store
failure/crash/recovery and all tuple/fence/late-result/privacy boundaries must
pass before dependent work. The user permits the emulator while the phone is
disconnected. Reuse only the owned enterprise-f2-api36 AVD/emulator-5588 and its
synthetic credential; preserve other devices/apps/data. It was parked at task
opening and now boots with authenticated gRPC; no native queue PASS is claimed.
Prove actual Keystore/process death, two-organization convergence, a lost actual
signed server answer and real Worker behavior with named opt-in test producers.
Inspect normal product bindings/APK so no test producer enters runtime.

Before storage behavior, B-BR-EMP-115 explicitly refuses typed operation IDs
already present in legacy OPERATION entries. Unknown old intent/binding cannot
prove safe adoption; preserve bytes and projection/head unchanged. A distinct
new operation ID may coexist; ordinary record/entity IDs are not restricted.

## Construction checks and counterexamples

Domain behavioral RED6/6 failed24s, then GREEN279 cases13s (all260 frozen vectors
retained). The first combined run had actual application6/6 assertion RED but
DATA compilation found an old Room fixture still calling the pre-PRIV003
no-argument unavailable DI provider. Update that fixture to the actual protected
store: deny before custody, persist/read committed work after confirmation,
invalidate old handles on background/account/origin change and permit retained
NeedsRefresh offline access without another contact. No production binding is
reverted. The recovered fixture's three tests pass; application six now pass.

The next combined run captures actual DATA behavior RED: protected queue9/9,
transport6/6 and runner5/5 fail against unavailable stubs, with no errors/skips
(52s overall). Two additional HTTP annotations were not discovered because
their expression bodies inferred non-Unit; make every such test explicitly
Unit and require all eight methods before accepting HTTP GREEN. Bounded5s
test barriers replace unbounded stub waits. Logs:
/tmp/enterprise-offl003-{combined-red,combined-behavior-red}.log.

Application review found two real convergence cases to prove before correction:
an obsolete retry boot could remain not-due forever after valid custody was
reconfirmed, and a worst-case token reservation could strand a valid operation
under reduced trusted byte limits. B-BR-EMP-119/120 record source-owned exact
encoded-proof budgets (no token in the budget) and a one-time full-delay retry
rebase only after the actual custody guard permits access. Preserve attempts,
Retry-After and all immutable intent/ACK bytes; add REDs before behavior fixes.

The next application run discovers all12 tests: exactly the two new convergence
counterexamples fail (0.342s). DATA does not execute because a newly implemented
runner smartcast crosses a public module property; copy that nullable value to
a local val before use. Existing DATA XML from the earlier run is stale and is
not evidence for the later eight-method HTTP suite. Require fresh report times.

Independent protected-store review records two more bounded corrections before
behavior: recheck custody clock after commit I/O without claiming a committed
head rolled back; and compare-and-swap retry metadata against the captured
attempts/clock/delay tuple, preventing a late valid worker from shortening a
newer Retry-After. B-BR-EMP-116/120 clarify the source contract. Actual security
expiry remains with the existing owner, while stale/conflicting retry updates
are no-ops with current state re-read for truthful scheduling.

Both additional protected-store failures were reproduced as fresh actual host
tests2/2 failing0.891s before their fixes (48s build). Storage then adds actual
postcommit custody validation and mandatory structural retry CAS, preserving a
newer floor without a new generation. Application13 fresh cases reproduce the
three precise convergence failures; all eight HTTP methods are now actually
discovered and fail1.842s against their stubs before transport implementation.
Logs: /tmp/enterprise-offl003-{final-red,data-two-red}.log.

Transport review found an independent wall-clock Retry-After shortening case.
EDR-081/B-BR-EMP-120 record the recommended optional HTTP-date profile before
its RED/fix: require the response's unique valid authenticated Date, use the
full server-date interval and ignore transit conservatively. Delta-seconds
remains unchanged; unusable date hints retain normal backoff and never acquire
device wall authority or erasure power.

First freeze narrow application/typed-store/transport interfaces, then domain
RED→GREEN in nonoverlapping files. Root owns docs, Gradle/ADB lease scheduling,
normal source composition, native acceptance and final review. No new queue
behavior or acceptance is claimed by this opening record.

## Consolidated host acceptance

The pre-review consolidated host run passes182 fresh cases: domain8,
application56 and DATA118, with no failures, errors or skipped tests. This
includes the13 queue application cases,24 protected queue cases and14 actual
transport/runner cases. All methods are discovered with Unit signatures;
stale XML after compile failures is excluded. Retain
/tmp/enterprise-offl003-host-consolidated.{log,summary.json}.
The independent source manifests are SHA256
aeca39ba3054ab34ebacd64cc793765f0b713ba0946a9b78a3ec844f281a16e5
(domain/application),
2db9681c9924570979c823d0dc6ff66e6c4d25892aa35e1a0bd4281ddd7923a0
(protected DATA), and
8d2192306d71ee57d7a979d8de16701e9b0df144606c7bbdbd614931e82aa5aa
(transport/runner). Native acceptance and normal product wiring remain pending.

The opt-in integration&&nativefixture source compiles against the actual
ordinary runtime and starts with two synthetic organizations. Its one discarded
answer follows a real accepted commit; completion requires two distinct scoped
business records with matching effect, ACK and dispatched follow-up counts.
The fixture uses the existing PREVIEW identity-key namespace exactly; no new
product origin, permissive TLS or test-control endpoint is introduced. The
private fixture and public trust anchor stay outside Git, mode0600 in mode0700
/tmp storage. Compile-only and server startup are not native behavioral PASS.

Independent review strengthened the native fixture before any device attempt:
require the exact lost ACK response on a later replay and equality of the two
known(org,operation) sets across claims, records, effects, ACKs and follow-ups,
including owner/member/target and revision2. The initial prepared fixture was
stopped without an acceptance claim; v2 contains these assertions.

Final service review also reproduced two additional assertion REDs before a
correction: a replaced source could return private attention/retry counts from
a no-send branch, and a cancelled retained-source attempt could return success.
Both fresh tests fail0.130s, with no errors/skips (Gradle16s). The recorded
B-BR-EMP-117 ownership requirement applies to every final result, including
unknown producer, delayed work, cycles and empty queues. The correction adds a
shared final result guard rather than relying on an external runner's check.
The corrected application suite passes all15 discovered methods0.212s,
zero failures/errors/skips (Gradle16s), including the two new counterexamples.
Retain /tmp/enterprise-offl003-final-lease-{red,green}.log. The final
domain/application manifest is
/tmp/enterprise-offl003-domain-app-source-final.json, SHA256
072b13672cf4ca0cf9295e38c35c9b53cfae7e4a29eaf89612025017c3be12be;
the earlier manifest remains the preceding regression snapshot.

## Native counterexample before activation

The first native APK compiles32s; explicit private-fixture parser/trust
redaction passes one API36 test0.014s. Review strengthens cleanup to require
successful deletion/absence of the app-private fixture containing synthetic
proof; its incremental build passes16s. The owned emulator's restarted adbd
initially refuses the privileged reverse443 listener. Prepare only that AVD's
debug adbd for the listener; application UID, AndroidKeyStore and the ordinary
source runtime remain unchanged. No physical device is touched.

The real seed test passes1.432s: source commit/answer loss leaves A retryable,
B independently accepts, ciphertext/legacy work/private attachment persist.
After explicit target force-stop, recovery fails14.106s: the real Worker
returns Retry instead of Success. Cleanup completes; no convergence PASS or
product activation is declared. Independent reviews identify the exact cause:
new current-account discovery requests narrow denied_/lock_ alias prefixes,
but the actual Android provider whitelist rejects them before descriptor reads
or HTTP. The permissive host fake masked this adapter mismatch. Add an actual
two-account native prefix-isolation RED before the minimal whitelist correction;
retain /tmp/enterprise-offl003-native-device.log. Repeat the whole scenario
against a fresh ordinary source after the correction.

The focused actual Android regression reproduces that exact whitelist failure
before correction:1 test/1 assertion failure0.073s (APK build15s). The minimal
provider correction admits only `abastevo_enterprise_v2_(denied|lock)_` followed
by an exact64-hex account tag and hyphen. It preserves the existing alias-name
filter and discovery budget; broader/truncated prefixes stay refused. The test
creates six real keys for two accounts sharing an organization and proves every
normal/denial/lock prefix returns only its exact account's matching key. All
created keys are explicitly removed.

Final API36 native acceptance passes all four methods: scoped key discovery
0.050s, fixture/trust redaction0.007s, actual lost-answer/two-org seed1.324s,
then process-death recovery15.191s. The driver force-stops only the target
between the two lifecycle methods without clearing/reinstalling data. Actual
P256/session/vault Keystore state reopens; the retry respects its remaining
real default delay, and the real Worker runs via its named test factory with
empty WorkData. A repeated Worker does not resend or reproject a final receipt.
Legacy opaque work and the private attachment remain intact before cleanup.
This custom factory proves Worker behavior, not normal Hilt or OS scheduling.

The fresh v3 ordinary PostGIS/HTTPS source separately passes its real race test
107.147s (106.11s source uptime), including exact original lost-response replay,
the two known joined source/record/effect/ACK/follow-up tuples and revision2.
No duplicate effect exists. Its disposable child database/runtime role and
private proof/CA files are cleaned; the owned reverse443 listener is removed.
Retain /tmp/enterprise-offl003-native-source-v3.log,
/tmp/enterprise-offl003-native-device-result.json and
/tmp/enterprise-offl003-native-primitives-green.json. Final native test APK
SHA25611ba5c311bd9e81be1ddd1d5b4d84047733946759e8676e776457683c7eff913,
retained at /tmp/enterprise-offl003-apks/data-native-prefix-green.apk.
At that native checkpoint, normal DI/startup/factory acceptance was still pending.

## Normal product composition and closure

After native acceptance, actual product providers now share the existing
protected store, identity-v1 signer and one concrete singleton session source
between queue service and HTTP. Shared trusted limits, actual Android clock
and bounded jitter feed the service. The producer registry is empty; no
test-only or online-only business producer is installed. The existing Hilt
factory constructs the assisted Worker, and Application schedules opaque
one-time startup plus15-minute connected periodic work.

Normal app assembly, full app/DATA lint and all seven affected enterprise
ViewModel tests pass in one Gradle run2m35s. Both lint reports have zero errors;
their existing warning totals are351/17, with none in changed files. The
generated ordinary Hilt component contains the actual assisted factory and
runner provider. No test graph supplies these bindings.

The ordinary debug APK is installed with update/preservation on only the owned
emulator and launched normally. Its OS WorkManager actually executes the
operation Worker and reports RETRY from the unavailable current source.
The one-time startup row is ENQUEUED with attempt1 and interval0; the periodic
row has interval900000ms. Both have exact empty version1 WorkData, zero entries
and eight encoded bytes. A missing factory would fail before startWork; this
observation proves real normal construction/execution, not merely enqueue.
The copied closed WorkDB/WAL has integrity_check=ok; only the two operation-job
metadata rows are inspected and no payload or credentials are logged. The APK
contains no native test producer/class. Source absence preserves work rather
than creating business acceptance. The app is then stopped without clearing
data. Retain /tmp/enterprise-offl003-product-smoke/{result.json,worker-observation.log};
normal APK SHA256d1c3677e800bfb0bc231a3d46833d231aaae15314bfa27653e86e96a61543855.

Affected Go vet/staticcheck and explicit gofmt checks pass. Protection/SQL
boundary scripts pass4/5 cases respectively. The general plan checker reports
348 tasks,545 rules,167 use cases,145 screens and zero problems. Diff/secret
review includes all changed/untracked source, not only old tracked files;
runtime proof/CA/PIN/key files stay outside Git. No dependency was added.

The final29-file source/test/README manifest is
[f2-offl003-source-manifest.sha256](f2-offl003-source-manifest.sha256), SHA256
bc1c6837648be1cb6c14772c4f585038b0fef19ea3c8dc70a57bfc5f93a16c7c.
This checkpoint remains INTEGRATION_PENDING, with no F2/M1/M2/C1 or production
readiness claim. Continue the dependency-ready EMP-OFFL-004 status/pending UI
from this tested source before its dependent operations. Keep real receipts,
managed-provider/legal/staging/release and pilot gates attached to their owners.
