# Bounded-city pilot operations (P23-T04)

Opening, staffing and stopping one pilot city. Normative metric
definitions live in [COMMUNITY_METRICS](../product/COMMUNITY_METRICS.md);
reporting/moderation procedure in [feedback-moderation](feedback-moderation.md);
backups/restore in [backup](backup.md)/[recovery](recovery.md).
No city opens without its pilot plan recording the checklist below.

## Opening checklist (all recorded before opening)

- Frozen city scope: municipality + state, directory station list,
  survey-week cadence. No simulated members, prices or activity —
  synthetic fixtures stay in tests and rehearsals.
- Named moderation coverage with response target and triage
  cadence; support route staffed (in-app re-report path + Help
  surface). Coverage measured per COMMUNITY_METRICS, never promised.
- Privacy operations reachable: export/erase runbook path tested,
  24-hour evidence expiry active, 12-month audit purge scheduled.
- Low-coverage behavior verified on-device: UNKNOWN/no-coverage
  states, dated ANP reference, no fake content.

## Stop / rollback triggers

- Moderation backlog exceeds the recorded response target for two
  consecutive triage windows: freeze new contributions, keep reads.
- Abuse volume exceeds reviewer capacity: tighten report quota,
  keep audit trail; never auto-delete.
- Privacy incident or unrecoverable data error: stop writes,
  snapshot, run recovery; disclose per retention policy.
- Rollback reverts product changes only through versioned releases;
  moderation audit rows are append-only and never rolled back.

## Unresolved pilot risks (owned, not waived)

- Real-device proof (providers, low-end encode, GPS behavior) is
  P24-owned; the rehearsal below is synthetic by design.
- Reporter case-status and author-history reads need a future
  backend extension plus privacy review.
- In-screen discussion/report buttons wait on the station-identity
  mapping; reporting stays reachable via Help policy copy.
- Pilot hosting/storage/staffing costs attach real numbers only in
  the pilot plan (no invented budget).
