# Pure expense oracle v1

EMP-EXP-001 / B-BR-EMP-180..184, BUC-EMP-94..95, EDR-131..133. This synthetic
oracle covers a pure expense value and its review-basis totals: one cost
origin with one source identity, exact paid and company amounts, a structural
category code, the competence instant with its reporting day, and approved
versus pending totals. It does not cover persistence, intake, the fueling
projection, review decisions, payables, payments, budgets, allocation or an
active runtime producer.

## Format

`expense-v1.psv` uses the exact four-column header `id|operation|args|expected`.
Fields are semicolon-separated `key=value`, split at the first equals sign.
ENUM inputs use lowercase even-length UTF-8 hex; other text inputs are literal.
The question mark is fixture-only absence, distinct from a present zero or a
present empty text. No blank or unknown rows are skipped. Consumers assert the
exact hash, count, operation counts, nonempty unique case IDs, distinct field
names, allowed argument names, coverage of every operation and every error
code, and complete result maps. Unknown syntax fails closed.

Alias UUIDs use prefix `9e000000-0000-4000-8000-` plus a twelve-digit suffix:
A=1, B=2 (organizations); E=11, E2=12 … E9=19 (expenses); F=21, F2=22, F3=23
(fuelings); W=31, W2=32 (work orders). An unknown alias is literal input; `?`
means absent. Raw UUIDs do not prove existence, acceptance or authority.

## Fixed semantics

- Cost-origin kinds are exactly FUELING, WORK_ORDER and DIRECT. PAYMENT,
  REIMBURSEMENT, CARD_STATEMENT, ADVANCE, PAYABLE, DOCUMENT, STOCK_PURCHASE and
  STOCK_TRANSFER are recognized and refused with `NOT_COST_ORIGIN`; any other
  token (empty, padded, lowercase, MAINTENANCE, MANUAL, OTHER) is `INVALID`.
  FUELING and WORK_ORDER require a source in the expense's organization;
  DIRECT has no source and is keyed by its own expense identity. The origin
  key is `organization:kind:source-or-expense`.
- Paid and company amounts are both required and reuse the accepted exact
  centavos parser: 0..1000000000000, no sign, decimal point, comma, exponent,
  grouping, whitespace, redundant leading zero or non-ASCII digit. Absent is
  `INVALID`, never 0; a present 0 is a known zero. Company above paid is
  `COMPANY_EXCEEDS_PAID`. No amount is rounded, recomputed or defaulted.
- Category codes match `^[A-Z][A-Z0-9_]{0,63}$` exactly; no normalization.
  The organization's configured catalogue is EMP-EXP-003's.
- The competence is a validated OrgTime declaration (instant, entry zone,
  offset). The reporting day is the local day of the instant in the unit zone
  when supplied, else the organization zone; an invalid unit zone never falls
  back; the entry zone never decides the day.
- Review states are DRAFT, SUBMITTED, IN_REVIEW, APPROVED, RETURNED, REJECTED,
  CANCELLED and VOIDED, counted as APPROVED (APPROVED), PENDING (SUBMITTED,
  IN_REVIEW) or EXCLUDED (the rest). Bases are APPROVED and INCLUDES_PENDING.
- A reporting period is a half-open `[start, end)` pair of `YYYY-MM-DD` days
  (years 0001..9999, start before end).
- Totals take one organization scope, one period and one basis. Each entry in
  input order is checked for organization (`SCOPE`), period (`OUT_OF_PERIOD`)
  and a repeated expense identity or origin key (`DUPLICATE_COST`, whatever
  the state) before counting; the first failure decides and no partial total
  is returned. `approved` sums APPROVED company amounts, `pending` sums
  SUBMITTED/IN_REVIEW company amounts, `excluded_count` counts the rest.
  `figure` is `approved` under APPROVED and `approved + pending` under
  INCLUDES_PENDING; `includes_pending` follows the basis. Sums are exact and
  uncapped.

## Operations and defaults

Errors are exactly `error=INVALID`, `SCOPE`, `NOT_COST_ORIGIN`,
`COMPANY_EXCEEDS_PAID`, `DUPLICATE_COST` or `OUT_OF_PERIOD`. A rejected
constructor has no successful output.

- ENUM (`kind=ORIGIN|STATE|BASIS;value_hex`) parses the closed token and
  returns `value`; STATE also returns `counting`. An unknown fixture kind is
  malformed oracle grammar and fails the loader.
- AMOUNTS (`paid;company`, `?` absent) returns `paid` and `company`.
- CATEGORY (`code`) returns `code`.
- PERIOD (`start;end`) returns `start` and `end`.
- EXPENSE constructs one expense. Defaults: org A, id E, origin FUELING,
  source F, category FUEL, paid 97350, company 97350,
  at 2026-11-14T12:00:00Z, entry UTC, offset 0, organization zone
  America/Sao_Paulo, no unit zone. These are explicit synthetic fixture
  values, not domain defaults. Overrides: `org`, `id`, `id_org`, `origin`,
  `source` (`?` absent), `source_org`, `category`, `paid`, `company`, `at`,
  `entry`, `offset`, `zone`, `unit_zone`. `omit` is a comma list from
  id, origin, category, amounts, competence, zone, removed before
  construction. Output: `ready`, `org`, `id`, `origin`, `source`, `key`,
  `category`, `paid`, `company`, `at` (canonical UTC), `entry`, `offset`,
  `day`, `zone`, `reporting_source` (`organization` or `unit`).
- TOTALS (`org;start;end;basis;items`, optional `zone`/`unit_zone` applied to
  every item, default America/Sao_Paulo and none) builds each `~`-separated
  item `id,org,kind,source,paid,company,at,state` as an EXPENSE in its own
  organization (category FUEL, entry UTC, offset 0), in order, before the
  period, basis and totals; an item construction error is the case result.
  Output: `org`, `start`, `end`, `basis`, `approved`, `approved_count`,
  `pending`, `pending_count`, `excluded_count`, `figure`, `includes_pending`.

The frozen corpus contains 222 cases: ENUM 40, AMOUNTS 36, CATEGORY 24,
PERIOD 17, EXPENSE 57, TOTALS 48. SHA256:
`b411e1cfa55c0aca4e4c9cadfab2f286ec22c3215e3103c119a767e013885dfe`.

```sh
(cd backend && go test -race -count=1 ./internal/modules/expense/domain)
./gradlew :domain:test --tests 'com.anpfuel.domain.enterprise.expense.*'
```
