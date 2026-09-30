# Feedback moderation runbook (P14-T05A/B)

Scope: station/fuel ratings, comments, replies and votes. Reports,
triage, visibility moves, abuse limits plus account-footprint
export/erasure (P14-T05B). Device evidence stays deferred to release.

## Report → triage → action

1. Any active account reports: `POST
   /v1/feedback/comments/{id}/report` with a live session and a
   reason (1–200 chars). Duplicate reports converge on the open
   COMMENT case instead of flooding the queue; the comment flags
   once. Report volume is quota bounded (429 + `Retry-After`).
2. Triage in the moderation queue (OPEN, P2 default for COMMENT).
   Cases carry identifiers and the reason only — never reporter
   identity, media, GPS or URLs.
3. Decide with `ops moderation act --case <id> --action
   REVIEW|INVALIDATE|RESOLVE|DISMISS --reason <text> --operator
   <id>` (audited; BLOCK stays contributor-only and refuses
   COMMENT targets).
4. Move visibility to match the decision:
   `ops feedback hide --comment <id> --case <id>` (INVALIDATE)
   or `ops feedback show --comment <id> --case <id>` (RESOLVE).
   The command refuses unless the case is OPEN/IN_REVIEW and
   targets the comment as COMMENT. Author tombstones win over
   later un-hides (deletion is terminal for display).

## Visibility semantics

- `visible`: default, reads normally.
- `flagged`: under review, reads normally.
- `hidden`: vanishes from public reads and views; stays for
  audit and for the author's own history queries.
- Author `deleted_at` tombstone: invisible everywhere, takes
  precedence over any visibility move.
- Suspended/deleted accounts cannot write (403/410 at every
  write path); past ratings and comments stay counted and
  readable until moderated or erased.

## Abuse limits

- Report quota per reporter/operation (shared limiter config);
  429 answers carry `Retry-After`, and denials never reveal
  whether the target exists beyond the 404 shape.
- Reports never grant moderation power: filing against another
  author's comment changes nothing but the case queue.
- Reporter account deletion cannot cascade: cases store no
  reporter identity, so the queue and the flag survive.
- Appeal route: a new reviewed case or fact (moderation
  NextStatus never reopens closed cases); reporters and authors
  re-report or contact support instead of editing history.

## Account-footprint export and erasure (P14-T05B)

Policy (frozen): suspension/deletion blocks new writes at the gate;
past rows stay counted/readable until this explicit erasure runs.
Erasure tombstones (history stays for audit, aggregates ignore it)
and rebuilds every touched rating key and vote tally from live rows.
Votes by the erased account vanish; votes by others on the erased
comments stay; replies by others on erased comments stay readable.

- Export: `ops feedback export --account <id> --operator <id>`
  prints the deterministic `feedback-export-v1` JSON (only that
  account's ratings/comments/votes, sorted; empty sections are `[]`).
- Erase: `ops feedback erase --account <id> --reason <text>
  --operator <id>` prints
  `feedback-erase account=<id> ratings=N comments=N votes=N keys=N
  tallies=N`. Re-running converges zero; after a restore, re-running
  re-tombstones resurrected rows (restore replay uses the same
  command, no separate ledger).
- No public HTTP export/erase path: owner archives and erasures run
  through the restricted operator tool with the reviewed reason.

## Verification

- `cd backend && go test -tags=integration ./internal/modules/feedback/...`
  (flag-on-report, hidden-leak, reporter-deletion plus export/erase
  and G14-exit suites).
- `ops feedback hide --comment <id> --case <id>` receipt prints
  `feedback <id> visibility=hidden case=<id>`.
- `ops feedback export --account <id>` prints `feedback-export-v1`
  with only that account's rows.
- `ops feedback erase --account <id> --reason <text>` receipt prints
  `feedback-erase account=<id> ratings=…`.
