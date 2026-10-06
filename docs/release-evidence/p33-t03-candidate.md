# P33-T03 — Pinned profile candidate and release operating handoff

Status: LOCAL_DONE on `codex/phase-33-profile-acceptance`. Task: P33-T03.
Auditable candidate record + operating model for the existing release
phases. No deployment, tag, pilot or production claim follows.

## Pinned candidate (code-ready, INTEGRATION_PENDING)

- Source head: `590613c` on `codex/phase-33-profile-acceptance`
  (base: P32 exit `acb28db`; P31 backend `1299e9c`).
- Backend migrations through `000040_comment_business_attribution`
  (000036 station profiles, 000037 claims/declarations, 000038 proofs,
  000039 decisions/grants, 000040 business attribution); append-only,
  no destructive rollback. P33 adds no migration.
- OpenAPI `1.0.0-draft` (`vacuum` 0 errors per `check-compat.sh`;
  `apicontract` green on P31 base) incl. claim open/replay/conflict,
  mine/status/reissue/proof/cancel, plus new `409 claim.busy-retry`.
- Android P32 code slice on this branch lineage: public profile/badge
  (T01), claim export-sign-import (T02), management/invitations/contest
  (T03), offline/restart code acceptance (T04); `:app:assembleDebug`
  PASS; full domain/application/data/app suites 0-fail (P32-T04).
  Compose screens/device rows owed to the conjunto manual batch.
- Flags default OFF for management/claim capabilities; staging origin
  explicit (`https://teste.abastevo.com.br`, TLS UNVERIFIED — Fortinet
  middlebox CA, curl 60, no bypass); never production.
- P28 optional sources excluded from this candidate; no nationwide or
  instant-approval promise. Terminal states never reopen; competing
  cases stay private.

## Operating model

- Runbooks: `docs/operator/profile-review.md` (queue, approval
  checklist, impersonation escalation, suspension/revocation, disable
  and binary rollback, rights) + `docs/operator/retention.md` (24 h
  all-copy purge) + `docs/operator/feedback-moderation.md` (adjacent
  social moderation). Staffing stays P23-T03-owned; no headcount
  invented — the measured artifact is the 10/10 `-race` adversarial
  containment plus the locked lifecycle suite, not a staffing claim.
- Recovery drills (scoped, this phase): publish-window busy/replay
  matrix (P33-T01), terminal-closed lifecycle (P33-T02), revocation
  paths in P31 suites. Full off-host restore stays G09-owned.

## Handoff to P09/G09 and P10-T09

- Remaining real-production checks (G09-owned, not waived): edge/TLS,
  private storage provisioning, off-host restore, load, provider
  delivery, launch staffing/moderation, device matrix. Deployment/tag/
  pilot need separate owner authorization.
- Next: conjunto-closure manual batch → cumulative final CI/PR guarded
  merge → merged-SHA wiki → P09/G09 certification → P10 public pilot.
  P11 stays optional; iOS stays deferred.

## Validation

- P33-T01/T02 evidence holds the proof (`-race` stationprofile 5/5,
  compat ok, lifecycle locked). This task is docs-only on the tested
  tree: Markdown links/state consistency, `git diff --check` PASS,
  `scan-secrets.sh` PASS.

## Limits

- G33 code-ready is not acceptance of live scale, provider trust, or
  device coverage; P10-T09 waits for G09 RELEASE_CERTIFIED.
