# FQ-11 — Document-free community review and signals

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING.
Date: 2026-10-15.
Scope: document-free review only; full purchase-supported FQ-11 acceptance D-held.

Selected scope: B-BR-FQ-07/08/09/10/14/15, BUC-FQ-05/07/08. Purchase-supported
independence and document verification remain D-held; no receipt/provider/schema
for them is introduced. GPS is neither accepted nor required. No paid capability
enters account participation, moderation or trust.

Before behavior:

- Existing restricted operator moderation handles revision-bound QUALITY_REPORT
  targets. Review targets bind an exact report revision; a stale target cannot
  change a newer edit/appeal. Legacy targets without a provable revision remain
  explicitly unbound and may only be dismissed as superseded. Report commands and durable review intents commit atomically; the worker
  creates revision targets and moderation cases together in a separate replay-safe transaction.
- Operator REVIEW/TRIAGE changes case triage only. RESOLVE/CLAIM_RECORDED marks a
  claim reviewed for moderation, never chemically/purchase verified. DISMISS
  accepts INSUFFICIENT_EVIDENCE, UNSUPPORTED_LINK or DUPLICATE; INVALIDATE accepts
  ABUSE or PRIVACY. Both hide the claim; PRIVACY also clears its text. All
  decisions preserve actor/reason/policy audit through existing moderation.
  Official facts, station linkage and numeric official score are unaffected.
- Signed owner APPEAL requires the current reviewed/hidden revision and bounded
  justification. It creates a new report revision/review target and retains a
  USER_CLAIM evidence level. New/edited/appealed intake uses the existing identity
  v1 write budget (200/hour) per account across all keys, in the command
  transaction; stable replay costs no second unit. Withdrawal remains available
  without consuming intake quota. The recovery switch preserves appeal/withdrawal
  and owner reads. No new quota number is invented.
- Document-free grouping is private operator review only, for an explicit
  station/fuel/incident date, at most 100 records per page. Returned counts
  explicitly describe that page only (scope PAGE), never a complete station total;
  counts must not be summed across pages to infer independence. Reviewed claims from the same account collapse to
  one support regardless of keys/repeated reports; pending, hidden, withdrawn,
  inactive/erased owners and superseded links are separate reasons. Different
  accounts do not prove different humans or purchases. Coordinated/farmed claims
  therefore cannot automatically publish an alert, verified status or score.
  The policy always returns REVIEW_ONLY / INDEPENDENCE_UNPROVEN; there is no
  guessed public threshold or time window. Source/price trust stays separate.
- Private review inspection and grouping use restricted operator access with
  bounded pages. No anonymous report text/owner/group count endpoint is exposed.
  Account erasure cascades private targets/appeal text while preserving only
  unlinked minimal audit references. Stale jobs cannot recreate erased material.

Immediate acceptance: pure decision/appeal/signal rules RED→GREEN; operator denial,
stale revision, concurrent edit/moderation/appeal, atomic audit/queue rollback,
replay, actual transactional quota rollback, related-key/account collapse,
farms/coordination under both official positive/negative histories, private
transport, erasure/recovery, real PostGIS and affected operator/API/worker builds.

## Validation and recovery

Behavior parent: 9f596151; source and tests are committed together with this evidence.
No dependency, provider, GPS, receipt/document or business portal was added.
Migration 000099 is new and append-only relative to the applied chain.

- Domain RED: undefined decision/signal functions and refused appeal envelope;
  GREEN: closed review reasons, bounded target-preserving appeal and account-farm
  non-promotion. Invalid purchase verification reasons remain rejected.
- Real PostGIS/race: all quality, identity, moderation and account packages PASS.
  Full API process suite PASS after supplying its required explicit disposable
  admin URL; initial omitted-URL refusal was an environment precondition, not ignored.
  Operator/worker and contract regression packages PASS.
- Seven new review SQL cases PASS: exact revision/IDOR/appeal replay, private
  erasure, failed queue rollback, edit/moderation race, transactional quota rollback
  and exhaustion with rights bypass, 98→99 legacy upgrade/replay without invented
  revisions, positive/negative official histories unchanged, suspended accounts,
  repeat-account collapse, concurrent triage (one of eight) and concurrent appeal
  (one of eight). Restricted queue/inspection exercised through their actual stores.
- Worker SQL: old/new edit jobs create distinct targets, replay converges,
  withdrawn/erased work cannot reopen review, unknown versions and unaudited
  decision intents refuse; audited decision replay succeeds without public effects.
- HTTP: signed exact-body/owner/no-store regressions, recovery-switch appeal and
  withdrawal, and 429/Retry-After without private identifiers PASS. Three additive
  appeal contract vectors PASS. Namespaced existing reused YAML anchors so standard
  parsers no longer encounter duplicate anchor declarations; strict lint has zero errors.
- Affected go vet/staticcheck and builds PASS; sqlc vet and byte-stable regeneration,
  gofmt, git diff --check and tracked/new secret/PII surface review PASS.

Commands (backend/, local disposable PostGIS only):

```sh
GOCACHE=/tmp/fq-completion-go-cache go test -race -tags=integration ./internal/modules/quality/... ./internal/modules/identity/adapters ./internal/modules/moderation/... ./internal/modules/account/adapters
ANPFUEL_TEST_DATABASE_URL='<local disposable admin DSN>' GOCACHE=/tmp/fq-completion-go-cache go test -race -tags=integration ./cmd/api ./cmd/ops ./cmd/worker ./internal/platform/apicontract
GOCACHE=/tmp/fq-completion-go-cache go test -race -tags=integration ./internal/modules/quality/adapters ./internal/modules/quality/adapters/jobs -run 'TestDocumentFree|TestReportReview' -count=1
```

Initial new-migration text/UUID join failure was reproduced and corrected before
publication; test-fixture conformity/retained-audit ID errors were also corrected,
not hidden by weakening SQL constraints. Staticcheck required writable XDG cache.
Diagnostic logs remain outside Git under /tmp/fq11-*.log.

Recovery: disable new/edit intake; signed appeal, withdrawal and owner reads remain
available. Public community quality signals are suppressed by construction.
Legacy, withdrawn, edited and erased case references only accept SUPERSEDED dismissal;
no decision can silently modify newer claims. Retain minimal unlinked decision audit.
This phase does not deploy migration 99 or certify staging/mobile/production.
Native importer wiring, complete historical API, FQ-12/13 methodology/projection,
Android and FQ-16 rights/restore acceptance remain owed. PR137 and final protected
main integration remain pending until construction finishes. D stays future-only.
