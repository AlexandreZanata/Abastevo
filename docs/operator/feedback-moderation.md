# Feedback moderation runbook (P14-T05A)

Scope: station/fuel ratings, comments, replies and votes. Reports,
triage, visibility moves and abuse limits. Appeals and social
export/erasure stay with the privacy flows; device evidence stays
deferred to release.

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

## Verification

- `cd backend && go test -tags=integration ./internal/modules/feedback/...`
  (flag-on-report, hidden-leak, reporter-deletion suites).
- `ops feedback hide --comment <id> --case <id>` receipt prints
  `feedback <id> visibility=hidden case=<id>`.
