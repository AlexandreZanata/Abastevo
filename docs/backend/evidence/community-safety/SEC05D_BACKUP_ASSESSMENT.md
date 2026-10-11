# SEC05-D — Backup and device-transfer assessment

Date: 2026-10-11. Status: LOCAL_DONE. Scope: selective exclusion
proof + restore posture — no soak (E), no new runtime. Head:
maintained `dev` + this task.
Plan: [remediation plan](../../../planning/SECURITY_REMEDIATION_PLAN_2026-10-11.md)
(SEC05, AUD hardening note). Contract: private metadata never
leaves the device via OS channels. Constraint: public offline
packs re-download (never invent coverage); restore boots the
existing fresh-install path.

Validation: PASS

## Assessment (verified categories)

- SharedPreferences (auth sessions, key backups, device
  identity): excluded in both rule files (pre-existing, pinned).
- Room database (`anp_fuel.db`: drafts, owner/evidence metadata,
  outbox, private sessions): INCLUDED by default before this
  task (the gap) — now excluded from Auto Backup, cloud backup
  AND device transfer (this task).
- Sealed photos under `cacheDir/anpfuel_photos`: never backed up
  by the platform (cacheDir exclusion is platform behavior, no
  rule needed); owner purge rides SEC04-D, expiry rides SEC05-C.
- Tradeoff (explicit): excluding the database also excludes
  public offline packs from restores. Packs are bounded and
  re-download on next sync; private metadata leaking to OS backup
  is the worse failure. No plaintext keys/photos were ever backed
  up (Keystore-held keys, sealed envelopes, excluded prefs).

## Changes

- `app/.../backup_rules.xml` + `data_extraction_rules.xml`:
  `<exclude domain="database" path="." />` in all three scopes
  with rationale comments.
- `app/.../BackupRulesTest.kt` (new, JVM XML parse): pins
  sharedpref + database exclusion in Auto Backup and in both
  extraction sections.

## Validation (actual)

- New XML tests green; `:app:testDebugUnitTest` (full) green.
  `git diff --check` clean; no secrets/PII.
- RED pre-GREEN: the tests failed before the database excludes
  (private metadata backup-eligible — the assessed gap).

## Limits / owed

- No on-device restore execution occurred (backup/restore device
  run unclaimed); restore posture follows the existing
  fresh-install path (rehydrate logged-out, packs re-sync),
  covered by existing startup tests, not a new restore run.
- Real 24 h every-copy observation: SEC05-E (time-gated). No
  production claim.

Next microtask: SEC05-E every-copy 24 h observation (time-gated).
