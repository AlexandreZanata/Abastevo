# P29-T03 — Pinned catalog candidate and production handoff

Status: LOCAL_DONE on `codex/phase-29-catalog-acceptance`. Task: P29-T03.
Auditable candidate record + operating model for the existing release
phases. No deployment, tag, pilot or production claim follows.

## Pinned candidate (code-ready, INTEGRATION_PENDING)

- Source head: `0622cf8` on `codex/phase-29-catalog-acceptance`
  (base: P27 checkpoint `dd0579f`; main baseline `b4754b2`).
- Backend migrations through `000035_suggestion_decisions`
  (000031 registry runs/assertions, 000032 assertion coords, 000033
  DOU run sources, 000034 suggestions, 000035 decisions); sqlc
  generated + vetted, no diff.
- OpenAPI `1.0.0-draft` (`vacuum` 0 errors, `apicontract` +
  `check-compat.sh` PASS) incl. 4 intake paths + 2 schemas.
- Android Room v8 (schema `8.json` committed; device migration proof
  owed to the end manual batch); flags default OFF; staging origin
  explicit, never production.
- P28: no source selected — core CSV/API/DOU/suggestion scope only;
  OSM/partner feeds explicitly excluded from this candidate.
- No national inauguration promise: grant/edition dates are
  authorization evidence with effective dates only.

## Operating model

- Runbooks: `docs/operator/registry-runs.md` (pause/retry/quarantine/
  last-success/rollback) + `docs/operator/dou-editions.md` (review
  queue, catch-up, freshness/escalation). Staffing/capacity stays
  P23-T03-owned; no headcount invented — backlog drill is the measured
  1.2 ms batch read plus the hourly sweep, not a staffing claim.
- Recovery drills (scoped, this phase): fresh-DB migration chain
  incl. 000031–000035 (every integration suite rebuilds it);
  privacy deletion-ledger restore suites rerun green just now
  (adapters + jobs + application + domain, real PostGIS); revocation
  and quarantine paths proven in T02/T04 suites.

## Handoff to P09/G09 and P10-T09

- Remaining real-production checks (G09-owned, not waived): edge/TLS,
  private storage provisioning, off-host restore, load, provider
  delivery, launch staffing/moderation. Deployment/tag/pilot need
  separate owner authorization.
- Next authorized phase: P30 (`codex/phase-30-station-profiles` via
  `--from-checkpoint`) — the catalog handoff is its entry
  prerequisite, not a release.

## Validation

- Docs-only on the tested tree (T01/T02 evidence holds the proof);
  `git diff --check` PASS; `scan-secrets.sh` PASS.

## Limits

- G29 code-ready is not acceptance of live scale; P10-T09 waits for
  G09 RELEASE_CERTIFIED; P11 stays optional; iOS stays deferred.
