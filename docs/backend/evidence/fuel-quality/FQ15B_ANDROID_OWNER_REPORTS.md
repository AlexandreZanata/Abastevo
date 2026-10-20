# FQ-15B — Android document-free reports and owner recovery

Status: LOCAL_DONE
Validation: PASS

Parent f3a9e89d, maintained dev. B-BR-FQ-07/08/09/14/15;
BUC-FQ-05/07/08 (BUC-FQ-06 documents remain D-held). Free-account gated private form and bounded owner history,
explicit confirmation before submit/edit/withdraw/appeal, exact revision and
sample linkage-review from the displayed public sample. User claims never
become official findings/independent purchases or a numerical score.

Preserve a stable in-memory command after uncertain response, block double tap
and mutation of unresolved intent, obtain fresh proof on retry. Clear owner data
on changed scope; hide private state while stopped or revalidating identity.
No SavedState/backup/plaintext report storage. Deliberate browser confirmation
opens the verified ANP guidance without sending report content. D stays held.

Acceptance: guest/account return, unknown status/evidence, retry/double tap,
conflict recovery, owner switch/logout/late response, report lifecycle and exact
sample context; targeted JVM, lint/app and native-test compilation. Native
execution remains deferred and cannot be claimed from compilation.

## Acceptance, 2026-10-18

Canonical station quality route opens a private account-gated journey. A displayed
sample opens LINK_REVIEW with exact public revision/ref/fuel; owner report edits
preserve original identity and optimistic revision. Confirmations precede every
mutation. Unknown status/evidence/reason never becomes approval. Review reasons
are localized explanations, not raw implementation codes. Eight resource sets
include plain claim/no-purchase/no-official-verdict and deliberate browser handoff.
ANP guidance URL verified read-only on 2026-10-18; no report is sent externally.

A pending uncertain command stays frozen in memory, double taps cannot fork it,
and retry retains command/report IDs. Account scope is revalidated before/after
responses and on resume; stop hides private content. SecureAccountWindow is
reused for screenshots/recents protection. No private field is placed in saved
state, logs, navigation or disk. FQ-16 still owes durable encrypted intent and
restart/reconnect recovery; this source checkpoint does not claim them.

`./gradlew --offline :app:testDebugUnitTest --tests
'*QualityReportsViewModelTest' --tests '*QualityViewModelTest' :app:lintDebug
:app:assembleDebug :app:assembleDebugAndroidTest`: PASS, seven new and four
existing ViewModel cases. Guest/no private I/O, double tap/frozen fields,
uncertain same-command replay, changed owner during response, stopped/late-read
isolation, exact sample linkage and edit/withdraw/appeal revision checks PASS.
Lint and app/test APKs PASS. Base Portuguese resource tools:locale metadata fixes
two English-dictionary false positives without suppression; affected lint/test
APK rerun PASS (26s), zero findings in quality files. Public summary raw assay
cells now explicitly show their conformity label beside count/date.

Two added native cases (free-login privacy and explicit submit/ANP confirmation)
COMPILED ONLY; four quality UI cases total remain EXECUTION_OWED under the
maintainer-deferred native batch. No device/VPS/source activation/schema/dependency
change. Whitespace, localized resource completeness and secret surface reviewed.
Logs /tmp/fq15b-android.log and /tmp/fq15b-locale-final.log are local only.

Next FQ-16 public offline cache and encrypted owner intent/erasure compatibility,
then unresolved live enforcement/fines and complete FQ-14/FQ-17 acceptance. Full
original FQ-15 document features remain DEFERRED_FUTURE / NOT_AUTHORIZED.
