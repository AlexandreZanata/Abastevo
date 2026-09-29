# G09 backend readiness sign-off (P09-T03)

Status: **G09 BLOCKED** — bounded backend MVP is locally rehearsed, but
provisioned-staging acceptance and P09 phase integration are still open.
P10 MUST NOT start. No Android source changes in this task.

## Candidate and tree

- Candidate: `4ce5aaf79aa3bf8d8cf31c05635dfa2b30714556` (P08 merge, PR #9).
- Rehearsal tree: P09 branch adds only harness/docs (`backend/testdata/e2e`,
  `contracts/testdata/compat`, `infra/scripts/rehearse.sh`, `scripts/`,
  `Makefile` entries, release evidence). Core product paths
  (`backend/cmd`, `backend/internal`, `backend/db`, `backend/go.*`,
  `contracts/openapi`) match the candidate.
- Schema: 19 migrations on the ledger (disposable drill count); PostGIS
  3.6. Policy: consensus-v1, moderation-v1. Images: pinned
  postgis/caddy digests + immutable release tags per P08-T01/T02.

## Checklist vs evidence

- [x] P01–P08 phase PRs/fixes integrated (PRs #4–#9); G01…G08 linked in
  PROGRESS; P01 direct commits recorded without retroactive PRs.
- [x] Candidate selected (`4ce5aaf`); local matrix green (see below). No
  release certified from a phase merge alone.
- [x] Catalog, identity, observation, media, validation, consensus,
  moderation, privacy flows run end to end in real-PostGIS suites
  (57 packages ok) + local rehearsal smoke (`p09-t01`).
- [x] Empty/upgrade migrations + duplicate/replay/concurrency cases pass
  in P01-T08/P02-T03/P03/P04/P06 suites (real PostGIS, race).
- [x] OpenAPI frozen + shared fixtures + legacy deltas documented
  (`p09-t02`, `legacy-deltas-v1`); wire has no `GASOLINE_PREMIUM`.
- [x] Android regression baseline passes in a supported environment
  (BUILD SUCCESSFUL; 88 tasks up-to-date, no Android inputs changed).
- [x] Private storage, role/secret/dependency gates + no critical/high
  findings locally (`p08-t08`); topology TLS/origin rules reviewed
  (`p08-t01`); live edge/TLS at deploy stays pending below.
- [ ] Off-host encrypted backup restored with measured RPO/RTO — PENDING:
  disposable drills pass (`p08-t03`/`p08-t04`), provisioned-host drill
  with measured RPO≤24h/RTO≤4h not run (no provisioned host).
- [x] Retention/export/erasure runtime + deletion replay pass on
  disposable DB (P07-T03…T05, `p08-t04` replay); privacy notice is a
  draft pending legal review (see blockers).
- [ ] Load/fault/cache at accepted capacity — PENDING: bounded local
  smoke passes (`p08-t07`: 54,951 reqs, p95 8.1 ms, faults 7/7;
  `p08-t06` cache 10/10); 30-minute staging matrix not run.
- [x] Operator runbooks tested as harnesses (deploy/backup/restore/
  monitoring/edge-cache/retention/recovery); live staging drill pending
  (same provisioned-host blocker).

## Blockers (all must clear before G09 COMPLETE)

1. P09 phase integration: PR #10 (T01…T03) still open/draft; merge only
   after required `Quick verification` + phase exit on the final head.
2. Provisioned staging: 30-minute load matrix, R2 media suite, live
   TLS/edge + cache-through-proxy, off-host restore with measured
   RPO/RTO, worker kill-and-drain.
3. Legal review of `docs/backend/PRIVACY_NOTICE.md` + retention matrix
   before any pilot; record reviewer + date, never checkbox-only.
4. Wiki mirror: phases INTEGRATED with WIKI_PENDING; sync once per merged
   phase from the merged SHA (no wiki claims here).

## Operator readiness / rollback

- Deploy: `infra/scripts/deploy.sh` (backup precheck, migrate, rollout,
  smoke; auto-return to previous compatible release). Rollback: prior
  binary + forward schema fix; pause writes if needed; never destructive
  down-migration. Restore: `restore.sh` verify-first + ledger replay.
- If any blocker materializes as regression, withdraw this sign-off,
  keep P10 unstarted, and fix on a new task commit.

## Decision

G09 remains **BLOCKED**; P10-T01 stays blocked. `scripts/check-g09.sh`
refuses certification while any box above is unchecked, and
`scripts/tests/test-g09.sh` proves that refusal.
