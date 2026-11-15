# Pure budget oracle v1

EMP-BUDG-001 / B-BR-EMP-198..201, BUC-EMP-102..103, EDR-141..142. This
synthetic oracle covers a pure budget line (organization, half-open
reporting-day period, one closed dimension, exact limit), capacity derived from
approved consumption and active holds, the reservation and posting decisions
and the hold lifecycle. It is server-side only: no client mirror exists
(EDR-141) and any future client must consume this file. It does not cover
persistence, row locks or the real concurrency proof (EMP-BUDG-002),
reservation commands, owners and expiry bounds (EMP-BUDG-003), overrun
policies or exceptions (EMP-BUDG-004), the panel (EMP-BUDG-005) or driver and
asset limits at submission (EMP-BUDG-006).

## Format

`budget-v1.psv` uses the exact four-column header `id|operation|args|expected`.
Case fields are semicolon-separated `key=value`, split at the first equals
sign. List fields (`items`, `holds`, `post`, `steps`) hold `~`-separated
entries of comma-separated `key:value` pairs split at the first colon, so
instants keep their colons. ENUM inputs use lowercase even-length UTF-8 hex;
other text inputs are literal. The question mark is fixture-only absence,
distinct from a present empty text. No blank or unknown rows are skipped.
The consumer asserts the exact hash, count, operation counts, nonempty unique
case IDs, distinct field names, allowed argument and list-entry names, that
every operation, error code, outcome, dimension kind and the hold states are
exercised, and complete result maps. Unknown syntax fails closed.

Alias UUIDs use prefix `9e000000-0000-4000-8000-` plus a twelve-digit suffix:
A=1, B=2 (organizations); E=11, E2=12 … E9=19 (expenses); F=21, F2=22, F3=23
(fuelings); L=101, L2=102 (lines); H=201 … H4=204 (holds); U=301, U2=302
(units); C=311, C2=312 (cost centres); V=321, V2=322 (assets); D=331, D2=332
(employees). An unknown alias is literal input. Raw UUIDs prove no existence,
acceptance or authority.

## Fixed semantics

- Dimension kinds are exactly ORGANIZATION, UNIT, COST_CENTRE, ASSET and
  EMPLOYEE; hold states are exactly ACTIVE, CONSUMED, RELEASED and EXPIRED.
  Anything else, including lowercase, padding, COST_CENTER, VEHICLE, DRIVER,
  CATEGORY, TEAM and SELF, is `INVALID`.
- A line has one scoped identity, a half-open `[start, end)` pair of
  `YYYY-MM-DD` reporting days (the EMP-EXP-001 period), one dimension and a
  limit. ORGANIZATION has no target; every other kind needs one target of
  the same kind in the line's organization. An optional category filter uses
  the EMP-EXP-001 code profile. The limit is required canonical centavos
  0..1000000000000; absence, empty, sign, decimal, exponent, grouping,
  padding, words and values above the ceiling are `INVALID`; 0 is a known
  zero. No line, no limit: no capacity and no decision.
- A fact matches a line when it is in the line's organization (`SCOPE`,
  including foreign tags, targets and consuming expenses), its reporting day
  is in the period (`OUT_OF_PERIOD`; the day of an expense's competence or a
  hold's reservation instant in the unit zone, else the organization zone)
  and, for a non-ORGANIZATION line, its tag of that kind equals the target,
  with the category equal to any filter (`DIMENSION_MISMATCH`; an absent tag
  or the same UUID under another kind never matches). A hold must also name
  the line.
- Capacity: `consumed` sums APPROVED company amounts; `pending` sums
  SUBMITTED and IN_REVIEW company amounts and is never deducted;
  `excluded_count` counts DRAFT, RETURNED, REJECTED, CANCELLED and VOIDED.
  `held` sums ACTIVE holds; `closed_holds` counts the rest.
  `available = limit − consumed − held`, exact and uncapped; `over_committed`
  is `available < 0`. Consumption entries, then holds, are checked in order
  for `INVALID`, `SCOPE`, `OUT_OF_PERIOD`, `DIMENSION_MISMATCH` and
  `DUPLICATE` (repeated expense identity or origin key, or hold identity);
  the first failure decides. A stored hold needs amount 1..1000000000000, a
  nonzero instant, a category and a consuming expense exactly when CONSUMED.
- A claimed available is compared only: equal canonical signed integer →
  `current=true`; different → `STALE_AVAILABLE`; malformed → `INVALID`.
- RESERVE: the request is checked like a stored ACTIVE hold, then for a
  repeated hold identity in any state (`DUPLICATE`). `ACTIVATE` when
  amount ≤ available (a new ACTIVE hold), otherwise `EXCEEDS` with
  `shortfall = amount − available` and no hold.
- POST: the expense is checked like a fact, must be APPROVED
  (`NOT_APPROVED`), must not be among the facts (`DUPLICATE`), and an
  optional converted hold must be an ACTIVE hold of the facts
  (`HOLD_NOT_ACTIVE`). `headroom = available + converted amount`; `POST`
  when the company amount is 0 or ≤ headroom (the hold becomes CONSUMED by
  that expense), otherwise `EXCEEDS` with the shortfall and no change.
- Transitions exist only from ACTIVE (to CONSUMED by a posting, RELEASED,
  EXPIRED); any other is `HOLD_NOT_ACTIVE`. No overrun policy is applied.

## Operations and defaults

Errors are exactly `error=INVALID`, `SCOPE`, `OUT_OF_PERIOD`,
`DIMENSION_MISMATCH`, `DUPLICATE`, `HOLD_NOT_ACTIVE`, `NOT_APPROVED` or
`STALE_AVAILABLE`; an error result has no other field. The first failing
construction or decision is the case result.

- ENUM (`kind=DIMENSION|HOLD_STATE;value_hex`) returns `value`.
- LINE builds a line in scope `org`. Defaults: `line_id` L in `line_id_org`
  (default `org`), `start` 2026-11-01, `end` 2026-12-01, `dimension`
  ORGANIZATION, `target` absent (built as `target_kind`, default the
  dimension, in `target_org`, default `org`), `filter` absent, `limit`
  500000 (`?` absent). Output: `org`, `id`, `start`, `end`, `dimension`,
  `target` (`?` none), `filter` (`?` none), `limit`.
- CAPACITY builds the line in `line_org` (default `org`) with the LINE keys,
  then `items` and `holds`, and computes capacity in scope `org`. `zone`
  (default America/Sao_Paulo) and `unit_zone` (default none) apply to every
  fact. Item keys: `id`, `company` (both required), `org` (case org),
  `origin` DIRECT, `source` none, `category` FUEL, `paid` (= company), `at`
  2026-11-14T12:00:00Z, `state` APPROVED, tags `unit`, `cc`, `asset`,
  `employee` built in `tag_org` (item org). Hold keys: `id`, `amount`,
  `org`, `line` L, `at`, `state` ACTIVE, `category` FUEL, the tags,
  `tag_org`, `by` (consuming expense) in `by_org`. Expense entries use entry
  zone UTC and offset 0; these are explicit synthetic fixture values, not
  domain defaults. Output: `limit`, `consumed`, `consumed_count`, `pending`,
  `pending_count`, `excluded_count`, `held`, `active_holds`, `closed_holds`,
  `available`, `over_committed`, plus `current=true` when `claimed` matches.
- RESERVE adds a request (`hold` H in `hold_org`, `hold_line` L, `amount`,
  `at`, `category` FUEL, case-level tags and `tag_org`) decided in scope
  `decide_org` (default `org`). Output: `outcome`, `requested`,
  `available_before`, `available_after`, `shortfall`, `hold` (`?` none) and,
  on ACTIVATE, `hold_line`, `day`, `hold_state`.
- POST adds one `post` item (item keys) and an optional `hold` (in
  `hold_org`) decided in `decide_org`. Output: `outcome`, `amount`,
  `available_before`, `available_after`, `shortfall`, `converted` (`?`
  none) and, when converted, `converted_state` and `consumed_by`.
- SEQUENCE replays `steps` in scope `org` the way the owning transaction does
  under the line's row lock: each RESERVE (`id`, `amount`, request keys) or
  POST (item keys plus `hold`) recomputes capacity from the current facts and
  appends the new hold or expense, and replaces a converted hold, only on
  success; RELEASE and EXPIRE (`id`) transition the stored hold of that
  identity, and a hold absent from the facts (an EXCEEDS stores nothing) is
  the zero hold, which is `INVALID`. Output: `outcomes` (ACTIVATE, EXCEEDS,
  POST, RELEASED, EXPIRED in order), final `consumed`, `held`, `available`,
  `active_holds` and `closed_holds`.

The frozen corpus contains 261 cases: ENUM 24, LINE 44, CAPACITY 94,
RESERVE 40, POST 35, SEQUENCE 24. SHA256:
`fb9b99c33a0e82a131d2ae375276bd4b49380cfdb30ed479f089080fbaf03985`.

```sh
(cd backend && go test -race -count=1 ./internal/modules/budget/domain)
```
