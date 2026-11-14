# Pure payables oracle v1

EMP-FIN-001 / B-BR-EMP-194..197, BUC-EMP-100..101, EDR-139..140. This
synthetic oracle covers the pure payable domain: supplier, reimbursement and
card statement obligations recognized from approved expenses, exact
obligation and settlement amounts, the derived settlement state, explicit
cancellation, and separate cost and settlement views proving that obligations
and settlements never become a cost (EDR-013). It does not cover persistence,
the reimbursement command, advances, payment records, statements grouping,
dates, cash views, panel screens, provider money movement or an active runtime
producer.

The domain is server-side only (EDR-139): payables, settlement and payment
records are online-only panel/server facts, so no client mirror exists. Any
future client must consume this same file.

## Format

`payable-v1.psv` uses the exact four-column header `id|operation|args|expected`.
Fields are semicolon-separated `key=value`, split at the first equals sign.
ENUM inputs and cancellation reasons use lowercase even-length hex (ENUM must
decode to UTF-8; a reason may be any byte string so that invalid UTF-8 reaches
the domain); other text inputs are literal. The question mark is fixture-only
absence, distinct from a present empty text. No blank or unknown rows are
skipped. The consumer asserts the exact hash, count, operation counts,
nonempty unique case IDs, distinct field names, allowed argument names,
coverage of every operation and every error code, and complete result maps.
Unknown syntax fails closed.

Alias UUIDs use prefix `9e000000-0000-4000-8000-` plus a twelve-digit suffix:
A=1, B=2 (organizations); E=11 … E5=15 (expenses); F=21 … F5=25 (fuelings);
W=31 (work order); U=41, U2=42 (employees); V=51, V2=52 (suppliers); D=61
(document); T=71, T2=72 (statements); P=81 … P4=84 (payables); S1=91 … S5=95
(settlements). An unknown alias is literal input. Raw UUIDs do not prove
existence, acceptance, authority or payment.

## Fixed semantics

- Payable kinds are exactly SUPPLIER, REIMBURSEMENT and CARD_STATEMENT. Every
  other token (ADVANCE, PAYMENT, PAYABLE, empty, padded, lowercase) is
  `INVALID`. Payable states are exactly OPEN, PARTIALLY_SETTLED, SETTLED and
  CANCELLED.
- Counterparties: REIMBURSEMENT has exactly a beneficiary employee; SUPPLIER
  exactly a supplier and optionally a supplier document; CARD_STATEMENT exactly
  a statement. A missing or extra counterparty is `INVALID`.
- Lines: 1..1000 per payable (REIMBURSEMENT exactly 1; the 1000 bound is
  unit-tested). Each references one constructed EMP-EXP-001 expense with its
  review state (the expense vocabulary: DRAFT … VOIDED) and payer (COMPANY or
  EMPLOYEE; a paying employee exactly for EMPLOYEE), and an amount parsed as
  canonical centavos, at least 1. The obligation is the sum of the lines and
  at most 1000000000000.
- Recognition checks, first failure wins, lines in input order within each
  step: `SCOPE` for an invalid scope; `INVALID` (reference, kind,
  counterparties, line count, source construction, state and payer tokens,
  paying employee presence and shape, amounts, total ceiling); `SCOPE` (the
  payable, counterparties, source expenses and paying employees must be in the
  scope); `NOT_COST_ORIGIN` (a source expense identity or its origin source
  equals the payable or statement identity); `DUPLICATE_SOURCE` (a repeated
  expense identity or origin key); `NOT_ELIGIBLE` (source not APPROVED;
  REIMBURSEMENT needs EMPLOYEE paid by the beneficiary; SUPPLIER and
  CARD_STATEMENT need COMPANY); `EXCEEDS_SOURCE` (line above the source's
  company amount) or `PARTIAL_REIMBURSEMENT` (reimbursement line below it).
- Settlement entries have a scoped identity and an amount of canonical
  centavos, at least 1 (`INVALID`; a foreign reference is `SCOPE`). Recording
  one checks `SCOPE`/`INVALID`, then `NOT_COST_ORIGIN` (its identity equals the
  payable, statement, a source expense or that expense's origin source), then
  `CANCELLED`, `DUPLICATE_ENTRY` (identity already recorded) and
  `EXCEEDS_REMAINING`.
- The state is derived: CANCELLED when cancelled, else OPEN with no
  settlement, PARTIALLY_SETTLED below the obligation and SETTLED at it. The
  remaining amount is obligation minus settled, and 0 when CANCELLED.
- Cancellation checks `SCOPE`/`INVALID` (reason nonblank, valid UTF-8, no
  control character, at most 280 Unicode scalars, kept verbatim), then
  `CANCELLED` (already cancelled) and `HAS_SETTLEMENT` (any settlement).
- An asserted state must be a known token (`INVALID`) equal to the derived
  state (`STATE_MISMATCH`).
- Views: the cost view is the unchanged EMP-EXP-001 totals over the cost
  entries (their own errors pass unchanged: `INVALID`, `SCOPE`,
  `DUPLICATE_COST`, `OUT_OF_PERIOD`). Then, payables in input order: `INVALID`
  (unconstructed), `SCOPE`, `DUPLICATE_ENTRY` (payable or settlement identity
  repeated) and `DUPLICATE_SOURCE` (an expense identity or origin key already
  a source of a non-cancelled payable). Last, `NOT_COST_ORIGIN` when a cost
  entry's expense identity or origin source equals any payable, settlement or
  statement identity, cancelled payables included. The settlement view counts
  payables by state and sums, as exact uncapped integers, the obligation of
  non-cancelled payables, every settlement and the remaining amounts.

## Operations and defaults

Errors are exactly `error=INVALID`, `SCOPE`, `NOT_ELIGIBLE`,
`EXCEEDS_SOURCE`, `PARTIAL_REIMBURSEMENT`, `DUPLICATE_SOURCE`,
`DUPLICATE_ENTRY`, `EXCEEDS_REMAINING`, `CANCELLED`, `HAS_SETTLEMENT`,
`STATE_MISMATCH`, `NOT_COST_ORIGIN`, `DUPLICATE_COST` or `OUT_OF_PERIOD`. A
rejected case has no successful output.

Fixture construction rule: errors of the payable package's own constructors
(payable, statement and settlement references, settlement entries) and of the
expense package while building LEDGER items, the period or a review state are
case results. An employee, supplier or document reference that the fixture
cannot construct, and a PAYABLE source expense in a malformed organization,
are handed to the domain as zero values, so that the domain itself refuses
them; any other fixture construction failure fails the loader. All source
expenses use category FUEL, competence 2026-11-14T12:00:00Z (entry UTC, offset
0) and organization zone America/Sao_Paulo unless an item states its instant.
These are explicit synthetic fixture values, not domain defaults.

- ENUM (`kind=KIND|STATE;value_hex`) parses the closed token and returns
  `value`. An unknown fixture kind fails the loader.
- SETTLEMENT (`org;id;id_org;amount`, defaults org A, id S1, id_org = org,
  amount 97350, `?` absent) builds the reference in `id_org` and the entry in
  `org`. Output: `org`, `id`, `amount`.
- PAYABLE recognizes one payable, applies `events` in order, then checks
  `assert` when given. Defaults: org A, id P (`omit=id` passes the zero
  reference), id_org = org, kind REIMBURSEMENT (cast, not parsed, so the
  domain refuses unknown kinds). Counterparty defaults by kind token:
  REIMBURSEMENT beneficiary U, SUPPLIER supplier V, CARD_STATEMENT statement
  T; anything else absent; `?` forces absence. `beneficiary_org`,
  `supplier_org`, `document_org` and `statement_org` default to org. Without
  `lines`, one line is built from `expense` (E; `?` = zero expense),
  `expense_org` (org), `origin` (FUELING), `source` (F; `?` absent), `paid`
  (97350), `company` (97350), `state` (APPROVED), `payer` (EMPLOYEE for kind
  REIMBURSEMENT, else COMPANY), `paid_by` (U when the payer is EMPLOYEE, else
  `?`), `paid_by_org` (org) and `amount` (97350; `?` absent). With `lines`,
  those keys are forbidden and lines are `~`-separated tuples
  `expense,expense_org,origin,source,paid,company,state,payer,paid_by,amount`
  (paying employee in the case org); an empty value means no line. `events`
  are `~`-separated: `S:<id>:<amount>[:<org>]` builds a settlement reference
  and entry in `<org>` (default the case org) and records it under the case
  scope; `C:<reason hex>[:<org>]` cancels under the scope `<org>` (default the
  case org). Output: `ready`, `org`, `id`, `kind`, `beneficiary`, `supplier`,
  `document`, `statement` (alias or `?`), `lines` (`+`-joined
  `expense:amount:company`), `keys` (`+`-joined origin keys
  `organization:kind:source-or-expense`), `obligation`, `settled`,
  `remaining`, `state`, `settlements` (`+`-joined `id:amount`, or `-`) and
  `cancelled`.
- LEDGER (`org;start;end;basis;costs;sources;payables`) builds `costs` and
  `sources` as `~`-separated expense items `id,org,kind,source,paid,company,at,state`
  (the origin kind and state are parsed by the expense package), then the
  `~`-separated payables `id,org,kind,counterparty,payer,lines,settlements,cancel`
  in their own organization: the counterparty is the beneficiary,
  supplier or statement by kind; the paying employee is U for EMPLOYEE;
  `lines` are `+`-joined `expense:amount` naming a cost or source item;
  `settlements` are `+`-joined `id:amount` or `-`; `cancel` is `-` or
  `CANCEL` (fixed reason). Settlements are recorded before the cancellation.
  The period comes from the expense package and the basis is cast. Output:
  `org`, `start`, `end`, `basis`, `approved`, `approved_count`, `pending`,
  `pending_count`, `excluded_count`, `figure`, `includes_pending` (the cost
  view) and `payable_count`, `open_count`, `partial_count`, `settled_count`,
  `cancelled_count`, `obligation`, `settled`, `remaining` (the settlement
  view). For every successful LEDGER case the consumer also recomputes the
  EMP-EXP-001 totals over the costs alone and requires the cost view to equal
  them field by field.

The frozen corpus contains 275 cases: ENUM 27, SETTLEMENT 25, PAYABLE 183,
LEDGER 40. SHA256:
`5ac0b07637b66631dba97e441a66c750af9441c867461da8899dcec132904282`.

```sh
(cd backend && go test -race -count=1 ./internal/modules/expense/domain/payable)
```
