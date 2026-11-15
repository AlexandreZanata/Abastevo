# Pure review lifecycle oracle v1

EMP-APPR-001 / B-BR-EMP-175..179 / EDR-128..130. This synthetic oracle covers
the single-level (C1) review lifecycle shared by trips, fuelings, expenses and
work orders (BR-REV, EDR-012). It is not persistence, a route, an approval
policy, a level, a partial amount, a substitute, a period lock, an amendment or
an app screen. A later review-v2 file adds levels and amounts; v1 rows stay
valid for a single-level policy.

## Format

`review-v1.psv` uses the exact four-column header `id|operation|args|expected`.
Fields are semicolon-separated `key=value`, split at the first equals sign.
Free text uses lowercase even-length UTF-8 hex. Consumers assert the exact
hash, row count, per-operation counts, unique nonblank case IDs, distinct and
allowed field names, required keys and complete result maps. An expected error
is exactly one `error=` field from the closed vocabulary. Every closed error
code and every state/command pair (7 × 7, through APPLY) must be exercised.
Unknown syntax fails the loader; it is never turned into a domain result.

Identity specs are comma lists of `a:N` (account), `m:N` (membership) and
`e:N` (canonical employee). A decimal `N` (up to 12 digits) stands for
`9e000000-0000-4000-8000-` plus `N` padded to 12 digits; any other text is
literal input for the domain to validate. An empty spec is an empty identity.
These are synthetic identifiers; they prove no membership, role or merge.

## Fixed semantics

- States: DRAFT, SUBMITTED, IN_REVIEW, APPROVED, RETURNED, REJECTED,
  CANCELLED. Commands: SUBMIT, CANCEL, RESUBMIT (author) and OPEN, APPROVE,
  RETURN, REJECT (reviewer). Exact tokens; READY_TO_SEND, sync, approval-step
  and owner-extension tokens are INVALID.
- Transitions: DRAFT→SUBMITTED (SUBMIT) or CANCELLED (CANCEL);
  SUBMITTED→IN_REVIEW (OPEN) or CANCELLED; IN_REVIEW→APPROVED, RETURNED or
  REJECTED; RETURNED→SUBMITTED (RESUBMIT, revision n+1) or CANCELLED.
  Decisions only from IN_REVIEW; APPROVED/REJECTED/CANCELLED are terminal.
- Every command names a revision ≥ 1; a different current revision is
  CONFLICT. History is append-only: `KIND:REVISION:STATE:REASON_HEX` per entry,
  where REVISION and STATE are the results of the command.
- Facts at receipt: actor, author, optional beneficiary and the permission
  answer. Absent/false permission is FORBIDDEN; an author command by someone
  who shares no identifier with the author is FORBIDDEN; a reviewer command by
  someone sharing any identifier (any kind) with the author or beneficiary is
  SELF_APPROVAL, permission notwithstanding.
- Reasons: RETURN/REJECT required, APPROVE/CANCEL optional, SUBMIT/RESUBMIT/
  OPEN none. Existing private rule: valid Unicode, no control scalar, at most
  280 scalars, preserved exactly. Empty or whitespace-only is REASON_REQUIRED
  when required and INVALID otherwise.
- First failure wins: INVALID, REASON_REQUIRED, FORBIDDEN, SELF_APPROVAL,
  CONFLICT, INVALID_TRANSITION. A refusal produces no case and the input case
  is unchanged (both consumers assert it).

## Operations

- STATE (`value_hex`) returns `state`, `terminal` and the BR-REV-009 `basis`
  (COUNTED for APPROVED, PENDING for SUBMITTED/IN_REVIEW, EXCLUDED otherwise).
- COMMAND (`value_hex`) returns `command`, `role` and `reason` policy.
- APPLY starts from a new draft, applies `steps` (comma list, possibly absent)
  with eligible default facts, the current revision and the setup reason
  "Synthetic reason" for RETURN/REJECT, then applies `command` (raw token).
  Defaults for the tested command: `revision` = current; no reason; author
  `a:1,m:11,e:21`; actor = author for author commands, otherwise
  `a:2,m:12,e:22`; no beneficiary; permitted. `reason_hex` with optional
  `reason_repeat=N` repeats the decoded text N times. `actor`, `author`,
  `beneficiary` (`?` = absent) and `permitted` (`true`, `false`, `?` = absent
  answer, not granted) override. Returns `state`, `revision`, `history`.
  Consumers also replay every accepted history to the same case.
- REPLAY (`history`, possibly empty, `~`-separated entries with raw tokens)
  returns `state`, `revision` and `entries`, or INVALID for any history the
  lifecycle could not have produced.
- PRESENT (`server` state or `?`, `local` pending command or `?`) returns the
  review `state` shown, `server_confirmed` and `ready_to_send`. Without a
  server case only a local SUBMIT is representable (DRAFT, ready to send);
  with one, the server state is shown unchanged. Local decisions are INVALID.

The frozen corpus contains 232 cases: STATE 16, COMMAND 15, APPLY 155,
REPLAY 22, PRESENT 24. SHA256:
`fac6c92c9c6929dc59f724b6f9912149d2ec92206138ada14beeec2739dffa08`.
