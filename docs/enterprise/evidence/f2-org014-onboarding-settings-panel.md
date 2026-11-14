# EMP-ORG-014 — Persisted company onboarding and settings panel

Date: 2026-11-13.
Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Source: tested AUTH-009 checkpoint 4743e0b9 on owned codex/enterprise-f2.
The maintainer authorizes completion of the whole F2 and recorded recommended
engineering decisions. This F1 prerequisite stays within the full
[execution](f2-execution.md), [phase tracker 215](https://github.com/AlexandreZanata/Abastevo/issues/215)
and cumulative protected dev → main PR203. It does not close F2/M1/M2/C1.
B-BR-EMP-101/102, BUC-EMP-52 and EDR-064/065 precede product changes.
Remote tracker metadata is handled separately; it never blocks this local
opening or certifies a remote check.

## Recorded source gaps and decisions

The current contexts/read returns organization ID/name/membership plus identity
hints, not persisted organization state, setup facts or settings hints.
Settings/read supplies values, settings revision and root_revision, but lacks
the profile/state/checklist projection required for truthful reload. Extend
that existing read model with explicit account/org/member binding and
source-derived settings UI hints. Reuse the owning activation rules and
ordinary transaction/proof boundaries; do not build a parallel runtime service.
Current root state and historical settings must remain distinguishable.

Creation already saves one pending root/unit/employee/owner/terms result
atomically and has immutable receipts. Activation and settings changes already
have source-bound revisions, audit and current fresh proof. Reuse them.
The root revision belongs to activation; the settings revision belongs to
settings changes/history. Conflicts require a new read and explicit resubmission.
Do not infer current activation from a replay of the original creation ACK.

Operational context discovery intentionally hides capability-off organizations.
Authorized settings reads already preserve visibility without that write
capability. The isolated setup route rechecks the current session and scoped
settings-read source before rendering. Creation ACKs and URL IDs are selectors,
never authority. Keep native context metadata and operational CanSelect intact.
The operator-owned organizations capability remains default-off; explain saved
pending setup truthfully and refuse writes until actual gates allow them.

Within-view uncertain creation retries keep the exact operation ID/body.
For reload/lost-ACK recovery without browser private storage, add a bounded
own-account created-setup/receipt projection in the existing onboarding module,
filtering each candidate through current scoped settings-read authority.
This is a recovery directory, not an operational selector or general account
lookup. Exact DTOs, pagination and entry descriptors will be pinned with the
backend owner before implementation; they must retain those boundaries.

Responsibility values hold membership UUIDs, while employee DTOs hold employee
identity/profile and access state. Supply a bounded scoped named eligible-member
picker, never an editable UUID field or an inferred ID conversion. Show all
current settings fields and revision history, preserve unrelated values during
edits, and report read-only, suspended/archived/capability-pending, validation,
conflict and incomplete-checklist states through source-confirmed data.
Name and kinds are editable in the creation draft only; their persisted
summary is read-only. Timezone edits use the existing versioned settings
command. Do not add a pending/active profile mutation endpoint to this web
task, silently rename an organization or claim to repair a legacy incomplete
profile. Persisted missing checklist items and actual 409 responses still stay
visible; concurrent source changes never justify early activation.

The backend proposal keeps current read projections in the existing module:
settings/read adds explicit organization, setup and context sections;
/settings/responsibility-members/list supplies bounded named eligible member
choices; /v1/org-access/organizations/setups/list uses the account's existing
CREATED receipt source and current scoped read filtering. The existing
creation_account RLS/query can support that directory without a migration.
Settings-specific hints are derived separately; native ContextHints and wire
remain unchanged. Exact DTO fields/cursors will be frozen jointly before code.

## Planned acceptance

- Ordinary NOSUPERUSER/NOBYPASSRLS real PostGIS proves current source binding,
  persisted checklist/profile reads, bounded recovery and named member choice,
  foreign/ended/suspended denials, failure recovery and concurrent revision or
  idempotency races. Preserve applied migrations and personal data.
- Use the actual HTTPS company entry and existing browser harness for creation
  from an empty list, same-intent retry, lost-ACK/reload resume, capability-off
  saved state, incomplete persisted setup and server-confirmed activation.
- Exercise every current settings field/history, named primary/substitute,
  stale-revision conflict, read-only member, account/session replacement, A/B
  navigation, late replies, cancellation, keyboard/focus and responsive layout.
- Expand CompanyApi and Caddy routes narrowly, preserving no-store, current
  CSRF/session ownership, explicit error-field allowlists and real404 responses.
  Deep-link reloads use the same restricted shell fallback in the HTTPS harness.
  Update the ordinary harness grants only where a real owning command needs
  them; never run the application API under the migration/admin role.
- Use only source-pinned synthetic terms and fixtures. Add no dependency.
  Existing native/vault/contact, real-data/legal/provider/runtime and release
  gates remain open. No phone or emulator test is needed for this web task.

No product implementation or acceptance is claimed at this opening. Retain
actual failures, commands, tested source hashes and final evidence here as the
bounded task progresses. Final phase acceptance/publication remains separate.


## Recorded security correction before behavior

Independent review found a pre-existing complete-intent defect:
SettingsWrite.Digest omits Policies.RequireInvitationConfirmation although the
codec stores it and this UI edits it. B-BR-EMP-103, BUC-EMP-53 and EDR-066 record
the correction before implementation: v2 complete digest, immutable receipts,
legacy hash compatibility only after verifying the complete original stored
settings command/revision (including flag/reason/operation/expected revision).
Flipped confirmation policy is changed intent, never an identical retry.
Capture RED then actual legacy false/true replay, changed-intent refusals and
missing/inconsistent history failure through ordinary real PostGIS. No applied
migration or receipt may be rewritten to make the test pass.


## Final contract-run recovery

The affected race run uncovered two distinct test-environment/harness failures:
cmd/api's real HTTP stream test could not open its loopback socket under the
sandbox; rerun that package with the authorized local-network escalation.
The complete apicontract package reached its600-second timeout while repeatedly
self-validating the same large canonical document in loadSpec. The stack was
computing JSON-schema meta-validation, not waiting on SQL/network. Preserve all
negative assertions and default deadlines. Cache only successful/failed canonical
self-validation for the exact immutable input bytes; every caller still gets a
fresh parsed/resolved document before any test mutation. Never share a mutable
document between counterexamples, and never reuse validation for changed bytes.
EDR-067 records that bounded harness correction before implementation.


## Implemented local checkpoint

ORG-014 now ships the actual company create/resume/configuration view at
/empresas/criar/ and /empresas/organizacoes/{canonical_org_uuid}/configuracao/.
It uses the existing current account/company session, organization commands and
settings module. The current settings snapshot includes profile/state/checklist,
exact account/member binding, source settings hints and named responsibility
labels under the retained scoped source transaction. A bounded fresh named
membership picker and own-account CREATED-receipt recovery directory are actual
guarded routes. Their current-source filtering preserves capability-off reads,
current rehire membership and opaque pagination progress. Normal operational
contexts/CanSelect stay unchanged; saved creation never enables capability.

The panel exposes every current setting/history with separate settings/root
revisions, exact int64 clock-skew strings, scalar-correct text bounds and named
primary/substitute references. Persisted name/kinds stay read-only; draft edits
precede creation. Current-state and historical values are distinct. Same-intent
creation/activation/settings retries retain body/ID; a successful write ACK then
failed source read becomes read-only recovery. Background proof checks preserve
unchanged focus/drafts, update actual missing checklist facts, reject stale
membership/account responses and invalidate pending work on authority loss.
Newer revisions cannot silently overwrite a dirty form. Generic server text,
raw membership UUID inputs and private browser storage are not used.

The confirmation-policy digest defect is fixed: new v2 hashes cover the full
settings intent. Genuine immutable v1 false/true receipts replay their original
acknowledgements only after complete recorded revision/provenance/value checks.
Flipped flags or other changed intent are refused. Applied receipts/migrations
were not rewritten. No new third-party dependency, profile mutation command,
native-context change, real terms or production activation was introduced.

## Final verification

- Nine affected backend unit/race packages pass. cmd/api's sandbox-only socket
  failure was recovered with normal loopback authorization; full package race
  passes15.969s. The seven original unit packages and enterprisesuite passed
  their actual selected run. The final application diagnostic/identity check
  passes1.017s after private filter formatting was corrected.
- Fifteen selected real PostGIS/race cases pass37.227s, including the full
  current120-cell/1,689-request authorization matrix, actual source proof/root
  waits, settings CAS/replay/deferred rollback/designation/upgrade recovery,
  current snapshot/recovery/picker/rehire/concurrent/fault checks and genuine
  legacy false/true identity cases. Expanded actual HTTP tests also prove guest,
  origin/CSRF, foreign scope, stale freshness, cap-off refusal, invalid designee
  rollback, changed boolean replay409, stale CAS409, history/current separation
  and AUDITOR read-only behavior. All normal proof/business providers use the
  ordinary runtime login where asserted; admin access is synthetic setup only.
- The complete apicontract race suite passes50 top-level tests /582 named
  cases,0 failed/0 skipped,343.968s package (344.462s measured process). Its
  prior600-second timeout remains a failed attempt, recovered by EDR-067's
  exact-source validation cache. Three focused cache regressions prove nested
  mutation/reference isolation, actual invalid-example rejection/recovery,
  changed input/location and eight concurrent independent documents. Default
  deadlines and all counterexamples remain intact. Root independently reviewed
  the helper; scoped vet3.196s/static3.248s pass with pinned toolchain/cache.
- TypeScript, build and artifact checks pass; exactly25 fingerprinted/allowlisted
  files. The full business-web suite passes51 tests,0 failures/skips,1.030s.
  Actual regressions cover ACK-success/read-failure recovery, nonwrite actions
  preserving uncertain intents, current/history coherence, full scalar/int64
  bounds, controlled member references, quiet probes and late-write invalidation.
- Actual HTTPS/ordinary PostGIS browser acceptance passes the existing login,
  employee/invitation and context regressions plus creation retry/reload recovery,
  capability-off and persisted incomplete setup, activation replay, all settings,
  named choices/history/two-tab conflicts, read-only/foreign/expired/late-account
  response and dirty ADMIN revocation. Keyboard/focus, one visible h1, no private
  browser storage and390px/1440px layouts pass. Both screenshots were inspected
  independently and by root: no clipping/overflow. No independent browser process
  duration was recorded; polling time is not substituted for one.
- Actual isolated Docker Caddy acceptance passes exact deep links, unknown
  suffix/case/missing-slash404, no-store/no validators, fingerprinted assets and
  public/admin/API separation. The verified ConfigMap mirror matches. No Caddy
  process duration was independently measured. Owned temporary databases, roles,
  API services and Caddy container were cleaned after successful/failed attempts.
- Affected vet/staticcheck, pinned gofmt, stable sqlc generation/vet, Python syntax,
  diff/secret review and the plan checker (348 tasks/0 problems) plus its3 tests
  pass. No assertion, coverage threshold, timeout or configured check was weakened.

The browser's initial new fixtures exposed a20-character synthetic username
limit and then an in-flight replay-inspection race; the fixtures now use a
source-valid username and await completed aria-busy state. These were harness
failures, not hidden product acceptance. Other actual RED findings included
missing projections/routes, diagnostic filter redaction, complete digest
identity, stale/pending client responses and coherent current-history reads.
Every concrete finding is corrected and independently reviewed. Tool permission,
read-only cache and pinned staticcheck export-format failures were recovered
with the documented environment; none is reported as a successful attempt.

## Frozen inputs, rollback and continuation

[Source manifest](f2-org014-source-manifest.sha256) covers48 changed product,
contract, routing and test files against source4743e0b9. Its SHA256 is
`25ca33237864b52218e87cd82f89ff1512d334ad2cc32a87cd996eef7d822296`.
The backend owner's27-file manifest SHA256 is
`2465ea40ff89d2e56698c873d046ad91517bdc37ef9ac3991cbbaafe173fe0a9`.
Local detailed JSON/logs live under /tmp/enterprise-org014-*; they contain no
production data and are not repository payloads. The browser input manifest
SHA256 is `b0e5465d27d98af954b5e996c5fb72d34b5c27f29a534c8705f898a59e3b1e1d`;
contract input manifest is
`6a23ec3a010782d7977642b3c9849fd8d21befb4287a81b74c9fd6ece3861f98`.
Backend diagnostic redaction and test-cache changes after browser inputs do not
change the tested HTTP/UI behavior; their affected privacy/contract checks pass.

Reproducible commands include scoped `go test -race -count=1` on the affected
backend packages; `go test -tags=integration -race` with an explicitly owned
ANPFUEL_TEST_DATABASE_URL for the selected source cases; `npm run typecheck`,
`npm run build`, `npm run check`, `npm test`; and the actual browser.py/caddy.py
harnesses. GOCACHE is writable task storage; staticcheck uses the README's
GOTOOLCHAIN=go1.27.1. Final union/Quick belongs to F2 closure, not this checkpoint.

Rollback may remove only the web entry while retaining persisted organization,
settings, history and receipts. Keep the digest security correction or disable
enterprise writes before reverting its server implementation; never rewrite
history or restore the known incomplete identity behavior as a safe rollback.
The owned emp-f2-20261113 PostGIS project remains for following scoped tasks.

This checkpoint is LOCAL_DONE / INTEGRATION_PENDING. Full F2/M1/M2/C1, native
vault/contact/offline consumers, managed-policy, legal/real receipts, staging and
release gates remain open. The latest user instruction now permits an emulator
while the phone is disconnected; no native runtime change/test was required by
this web task. Next bounded prerequisite: finish EMP-PRIV-003 protection and its
actual contact/maintenance integration, recording decisions before behavior.
