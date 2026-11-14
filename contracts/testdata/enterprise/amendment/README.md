# Amendment kernel oracle v1

EMP-PLAT-016 / B-BR-EMP-190..193 / BUC-EMP-98..99. This synthetic oracle
covers the pure server-side decision of how a consuming module corrects an
organization record (BR-AMD-001..006) and the effect of a review result on a
pending amendment. It is not persistence, a route, a period command, a review
lifecycle, an authorization check or a recalculation job. Consumers supply
facts and receipt-time authority (BR-TIME-003) and apply the decision inside
their own transaction. The Go package `backend/internal/modules/orgamend/domain`
is the only implementation; this file is the oracle for any future client.

## Format

`amendment-v1.psv` uses the exact four-column header
`id|operation|args|expected`. Fields are semicolon-separated `key=value`,
split at the first equals sign; every row has at least one argument. Reason
fields (`*_hex`) are lowercase even-length hex of raw bytes, so invalid UTF-8
can be expressed; an empty value is the empty (absent) reason. The question
mark is fixture-only absence. Integers are canonical signed decimal int64.
Instants use the shared time profile (`time-v1`) and keep their offset when
passed to the kernel; outputs print the decision time as returned, so a
non-UTC result would differ. No blank or unknown rows are skipped. Consumers
assert the exact hash, count, operation counts, nonempty unique case IDs,
distinct field names, known argument keys, complete result maps and that every
outcome, cause, result and error code is exercised. Unknown syntax fails
closed.

Aliases use prefix `9e000000-0000-4000-8000-` plus a twelve-digit suffix:
F=1 (record), U=31 (actor). An unknown alias is literal input; `?` is absent.

## Fixture defaults (not domain defaults)

DECIDE: `record=F`, `state=APPROVED`, `effective=1`, `latest=1`,
`day=2026-10-31`, `period=OPEN`, no reopening, `intent=AMEND`, `actor=U`,
`at=2026-11-14T12:00:00Z`, reason "Pump quantity corrected", all three
authority facts `edit`, `amend`, `void` true. `expected` defaults to the
row's latest; `revision` to latest+1 for EDIT/AMEND and 0 otherwise; `base`
to latest for EDIT and to effective otherwise. ADJUST defaults
`posting_day=2026-11-14` and `posting_period=OPEN`; other intents default to
absent posting fields. `reopen=true|false` supplies a reopening fact (proven
or not) with reason "Late receipt correction" unless `reopen_reason_hex`
overrides it; `posting_reopen` and `posting_reopen_reason_hex` do the same for
the posting period.

RESOLVE: the same record/day/period/reopening defaults, `state=IN_REVIEW`,
`effective=1`, `latest=2`, `result=APPROVED`, `decided` = latest,
`at=2026-11-15T09:00:00Z`.

## Fixed semantics

- Review state tokens are the canonical BR-REV states plus `VOIDED`; period
  tokens are `OPEN`, `LOCKED`, `REOPENED`; intents `EDIT`, `AMEND`, `VOID`,
  `ADJUST`; results `APPROVED`, `RETURNED`, `REJECTED`, `CANCELLED`. Anything
  else, including lowercase, is `INVALID`.
- Facts must be consistent: latest at least 1; effective between 0 and
  latest; a never-approved record (effective 0) is never `APPROVED` or
  `VOIDED`; effective equal to latest is `APPROVED` or `VOIDED`; a later
  latest is never `APPROVED`. A pending amendment is effective at least 1,
  latest above it and latest in `DRAFT`, `SUBMITTED`, `IN_REVIEW` or
  `RETURNED`. A reopening fact exists only on a `REOPENED` period.
- Decide order: `INVALID`, `CONFLICT` (expected is not latest),
  `ALREADY_VOIDED`, state (`NOT_EDITABLE`/`APPROVED_IMMUTABLE` for EDIT;
  `NOT_APPROVED` then `AMENDMENT_PENDING` otherwise), append plan
  (`LOST_ORIGINAL`, or `INVALID` for a gap or a revision on VOID/ADJUST),
  reason (`REASON_REQUIRED`, `REASON_INVALID`), `UNAUTHORIZED`, period.
- EDIT applies only to a `DRAFT`/`RETURNED` latest revision and appends
  latest+1 linked to latest. It needs `edit` for a never-approved record and
  `amend` for a pending amendment; it has no period gate. Its reason is
  optional, but a supplied one must be valid (blank is `REASON_INVALID`).
- AMEND appends latest+1 linked to the effective revision, which stays
  effective. VOID appends nothing, keeps the effective revision and marks the
  record voided. ADJUST appends nothing to the record; it links the original
  record and effective revision to a strictly later posting day. AMEND and
  ADJUST need `amend`; VOID needs `void`. All three require a reason.
- A reason is valid UTF-8, not blank (Unicode white space only), free of
  control characters and at most 280 code points. Absent or blank required
  reasons are `REASON_REQUIRED`; other malformed reasons `REASON_INVALID`. The
  text is kept verbatim.
- Period gate: `OPEN` is writable, `LOCKED` is not, `REOPENED` is writable only
  with a reopening whose authority was proven and whose reason is valid
  (`REOPEN_UNAUTHORIZED`, `REASON_REQUIRED`, `REASON_INVALID`). AMEND and VOID
  need a writable record period (`PERIOD_LOCKED`). ADJUST needs a non-writable
  record period (`PERIOD_NOT_LOCKED`) and a writable posting period
  (`PERIOD_LOCKED`).
- Resolve order: `INVALID`, `CONFLICT` (decided is not latest),
  `ALREADY_VOIDED`, `NOT_APPROVED`, `CONFLICT` (no pending amendment), then
  for `APPROVED` only the record period gate. `APPROVED` makes latest
  effective and supersedes the previous effective revision; `RETURNED` keeps it
  pending; `REJECTED` and `CANCELLED` close it; the previous effective revision
  stays effective in those three.
- Derived marker: VOID gives `VOIDED` for the effective revision on the
  reporting day; ADJUST gives `ADJUSTED` for the effective revision on the
  posting day; an approved amendment gives `AMENDED` for the new revision,
  naming the superseded one, on the reporting day. Each carries the decision
  time. EDIT, AMEND and non-approving results give none.

## Outputs

Errors are exactly `error=` one of `INVALID`, `CONFLICT`,
`APPROVED_IMMUTABLE`, `NOT_EDITABLE`, `NOT_APPROVED`, `AMENDMENT_PENDING`,
`ALREADY_VOIDED`, `LOST_ORIGINAL`, `REASON_REQUIRED`, `REASON_INVALID`,
`UNAUTHORIZED`, `PERIOD_LOCKED`, `PERIOD_NOT_LOCKED`, `REOPEN_UNAUTHORIZED`.

- DECIDE returns `outcome` (`EDIT`, `AMENDMENT`, `VOID`, `ADJUSTMENT`),
  `record`, `revision` (0 when nothing is appended), `base`, `effective`,
  `voided`, `actor`, `at`, `reason_hex` (`?` when absent), `day`,
  `posting_day` and `derived`.
- RESOLVE returns `result`, `record`, `revision` (the judged revision),
  `effective`, `superseded` (0 when none), `pending` and `derived`.
- `derived=?` means no marker; otherwise `derived` is the cause and
  `derived_record`, `derived_revision`, `derived_superseded`, `derived_day`,
  `derived_at` follow.

The frozen corpus contains 193 cases: DECIDE 165, RESOLVE 28. SHA256:
`da7d99d9355d92a0c0869a1d97ceb1d55ab9b633e42d044eb348b89a08e81a2e`.
