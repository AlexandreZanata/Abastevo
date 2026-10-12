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
