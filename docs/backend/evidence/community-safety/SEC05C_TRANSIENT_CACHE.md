# SEC05-C — Android transient-cache cleanup

Date: 2026-10-11. Status: LOCAL_DONE. Scope: orphan `.tmp`
recovery, discard-path coverage, versioned rollback policy,
restart lifetime — no backup change (D), no soak (E). Head:
maintained `dev` + this task.
Plan: [remediation plan](../../../planning/SECURITY_REMEDIATION_PLAN_2026-10-11.md)
(SEC05, AUD-07). Contract: 24 h transient TTL (portable
predicate, frozen). Constraint: never race a live writer; small
skew stays available; powered-off deletion waits for the next run
(documented limitation); no plaintext disclosure (blobs stay
encrypted — the finding was retention, not exposure).

Validation: PASS

## Changes

- `data/.../AndroidPhotoCache.kt` (cache policy v1): sweep
  purges abandoned `.tmp` past a 1 h file-age grace (live writers
  rename within one call — no race; rollback keeps, never
  deletes); `delete(id)` drops the `.tmp` sibling too;
  expiry = portable 24 h predicate OR capture >1 h in the future
  (honest skew stays live, far-future rollback/tamper refuses and
  deletes). Lifetime still derives from the sealed envelope, so
  restart/reboot preserve it.
- Portable `isTransientExpired` untouched (frozen shared
  semantics for contribution staleness); the tolerance lives at
  the cache layer where the finding was.
- Tests: stale-tmp purges/fresh-tmp survives, rollback-live
  inside skew + far-future refusal (+file deletion),
  delete-drops-sibling, restart lifetime, owner-id purge
  (SEC04-D support), all prior cache suites re-green.

## Validation (actual)

- `:data:testDebugUnitTest` (full) green, incl. 4 new cache
  tests. `git diff --check` clean; no secrets/PII in fixtures.
- RED pre-GREEN: orphan `.tmp` survived the sweep and rollback
  served bytes in the pre-fix code (audit probes reproduced on
  JVM); stale-tmp purge failed before the age grace (fresh
  siblings at risk without it).

## Limits / owed

- Powered-off phones execute no deletion until the next run;
  retained limitation, disclosed in UI copy by P-label owners.
- tmp mtime is filesystem wall-clock: rollback keeps orphans
  longer (safe direction), never deletes live writers.
- No VPS/phone change beyond local unit scope; no production
  claim.

Next microtask: SEC05-D backup assessment.
