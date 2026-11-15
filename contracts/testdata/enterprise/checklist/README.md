# Checklist template oracle v1

EMP-CHECK-001 / B-BR-EMP-330..334, EDR-213. These 127 synthetic cases pin the
pure checklist rule shared by Go (`backend/internal/modules/maintenance/domain/checklist`)
and Kotlin (`com.anpfuel.domain.enterprise.maintenance.checklist`). Counts:
TEMPLATE 40, POLICY 11, EVALUATE 38, RESOLVE 22, POLICY_AT 16. SHA-256
`4231d227e82782a2dedf03655bc69e48168664bd8ef7a10a733ce3d3140165e2`.
Both consumers assert the digest, the counts, unique case IDs, the known
operations and argument keys, complete expected maps, that every operation
and every error code (`INVALID`, `ANSWER_NOT_ALLOWED`, `PHOTO_REQUIRED`,
`INCOMPLETE`) has a vector. No row is skipped or repaired. The oracle
activates no repository, route, device or run.

## Format

The exact header is `id|operation|args|expected`; every other line has four
columns. Fields are `key=value` joined by `;` (the first `=` splits). A
missing argument takes only the defaults below. An expected map is either
`error=<CODE>` alone or exactly the outputs of its operation.

Text values (names, labels, notes): `hex:<lowercase hex>` is the UTF-8 bytes,
`rep:<n>:<text>` repeats a text n times (`<text>` uses the same grammar),
anything else is literal.

Identifiers: `C<n>` (categories) maps to `9c000000-0000-4000-8000-` plus the
twelve-digit number `1000 + n`, `X<n>` (assets) to `2000 + n`, `T<n>`
(templates) to `3000 + n`; any other identifier is literal input. Outputs map
identifiers back to these aliases (`?` for none).

- Items: `~`-separated tokens `<flags>:<label>` where flags are `-` or any of
  `C` (critical), `O` (optional), `P` (photo required when not OK); the first
  `:` splits. A token `gen:<n>` stands for n items `-:Item 1` … `-:Item n`.
  An empty value is no item.
- Target: `ORGANIZATION`, or `<KIND>:<ids>` with comma-separated identifiers;
  `<KIND>:` is an explicit empty list and `<KIND>:gen:<n>` stands for
  `X1..Xn` (ASSETS) or `C1..Cn` (CATEGORIES).
- Answers: `~`-separated tokens `<answer>[,<photo>[,<note>]]` where answer is
  `OK`, `NOT_OK`, `NOT_APPLICABLE`, `?` (unanswered) or other literal input,
  photo `0`/`1` (default 0) and note a text value (default empty). A token
  `gen:<n>:<token>` repeats a token n times. An empty value is no answer.
- Candidates: `~`-separated `<template>/<context>/<target>`.
- History: `~`-separated `<revision>,<mode>,<validity>,<effective instant>`,
  instants canonical RFC 3339 UTC with an optional 1..9 digit fraction.

## Operations

- **TEMPLATE** — `name` (default `Synthetic checklist`), `context` (default
  `DEPARTURE`), `target` (default `ORGANIZATION`), `items` (default `gen:1`).
  Output `context;target;items;critical;optional;photo`: the canonical target
  with sorted identifiers, the item count and the `+`-joined positions of each
  flag (empty when none).
- **POLICY** — `mode` (default `BLOCK`), `validity` seconds (default 43200).
  Output `mode;validity`.
- **EVALUATE** — `items` (default `gen:1`), `mode`, `validity` (policy
  defaults as above) and `answers`. Output
  `outcome;issues;critical;acknowledge`: positions answered `NOT_OK`, the
  critical ones among them, and whether the author must acknowledge.
- **RESOLVE** — `context` (default `DEPARTURE`), `asset` (default `X1`),
  `category` (default `C1`) and `candidates` (the ACTIVE templates' current
  versions). Output `template;match`.
- **POLICY_AT** — `history` and `at`. Output `revision;mode;validity` of the
  revision in force, or `?` in all three when none is.

## Shared rule

- A definition has a name of 1..80 and labels of 1..120 Unicode scalars,
  valid UTF-8, with no C0/C1 control character and not only white space; a
  context `DEPARTURE`, `RETURN` or `AD_HOC`; 1..40 items; a critical item is
  never optional; a target `ORGANIZATION` with no identifier, or `CATEGORIES`
  / `ASSETS` with 1..100 distinct canonical lowercase non-nil UUIDs.
- A policy has mode `BLOCK` or `WARN` and a validity of 3600..259200 seconds.
- Answers: exactly one entry per item; a note has at most 140 scalars with the
  same text rule, and a non-empty note must not be only white space. Errors in
  this order: `INVALID` (items, policy, count, unknown answer or note),
  `ANSWER_NOT_ALLOWED` (`NOT_APPLICABLE` on a non-optional item),
  `PHOTO_REQUIRED` (`NOT_OK` without photo where required), `INCOMPLETE`.
  Outcome `COMPLETED_OK` without `NOT_OK`; `BLOCKING` with a critical `NOT_OK`
  under `BLOCK`; otherwise `COMPLETED_WITH_ISSUES`. Acknowledgement exactly
  for a critical `NOT_OK` under `WARN`.
- Resolution over a consistent candidate set (canonical unique template IDs,
  valid contexts and targets, and per context no asset, category or
  organization default named twice, else `INVALID`): the candidate of the
  context naming the asset, else naming the category, else the organization
  default, else none.
- Policy in force: revisions numbered 1..n in order with non-decreasing
  effective instants and valid policies (else `INVALID`); the revision with
  the latest effective instant not after `at`, ties by the higher revision.

```sh
(cd backend && go test -race ./internal/modules/maintenance/domain/checklist -count=1)
./gradlew :domain:test --tests 'com.anpfuel.domain.enterprise.maintenance.checklist.*'
```

# Incident report oracle v1

EMP-CHECK-003 / B-BR-EMP-369, B-BR-EMP-373, EDR-241. `incident-v1.psv` holds
98 synthetic cases shared by Go (`backend/internal/modules/maintenance/domain/incident`)
and Kotlin (`com.anpfuel.domain.enterprise.maintenance.incident`). Counts:
REPORT 35, DECIDE 63. SHA-256
`401ba762533077b059ffd5c39161451622adb6cf1fcf40b7de8ca8b235549119`.
Both consumers assert the digest, the counts, unique case IDs, the known
operations and argument keys, complete expected maps, that every operation
and every error code (`INVALID`, `CONFLICT`, `TRANSITION`, `SELF_APPROVAL`)
has a vector, that every operation has an `INVALID` vector and that every
action and state appears in an accepted step. The oracle activates no
repository, route, device or report.

## Format

The header, field and text grammar are those of `checklist-v1.psv` above
(`hex:`, `rep:<n>:`). `?` is an absent value. In Kotlin, `hex:` bytes that are
not valid UTF-8 cannot form a string and become an unpaired surrogate, which
the text profile refuses like Go refuses invalid UTF-8. Identifiers: `R<n>`
(reports) maps to `9c000000-0000-4000-8000-` plus the twelve-digit
`4000 + n`, `A<n>` (accounts) `5000 + n`, `M<n>` (memberships) `6000 + n`,
`E<n>` (employee records) `7000 + n`; anything else is literal input.

## Operations

- **REPORT** — `type` (default `DAMAGE`), `kind` (default `?`), `description`
  (default `Synthetic damage`), `location` (default `?`), `blocks` (default
  `false`). Output `type;kind;state;revision;blocks` of the new report
  (`REPORTED`, revision 1).
- **DECIDE** — the current fact `state` (default `REPORTED`), `revision`
  (default 1), `blocks` (default `false`), `report` (default `R1`),
  `reporter` (comma-separated identity keys, default `A1,M1,E1`), and the
  command `action`, `expected` (default the current revision), `note`
  (default `?`), `duplicate` (default `?`), `handler` (default `A2,M2,E2`).
  Output `from;to;revision;action` of the step.

## Shared rule

- A report is `DAMAGE` without a kind or `INCIDENT` with one of `BREAKDOWN`,
  `ACCIDENT`, `THEFT`, `FINE_NOTICE`, `OTHER`; a description of 1..1000 and an
  optional location of 1..140 Unicode scalars, valid text with no C0/C1
  control character and not only white space.
- A step: `ACKNOWLEDGE` (`REPORTED` → `ACKNOWLEDGED`, no note), `TREAT`
  (`REPORTED`/`ACKNOWLEDGED` → `IN_TREATMENT`), `RESOLVE` and `DISMISS` (any
  open state → `RESOLVED`/`DISMISSED`), each but `ACKNOWLEDGE` with a note of
  1..500 scalars; a duplicate only on `DISMISS`, canonical and not the report
  itself. The current fact is consistent (revision 1..4, `REPORTED` exactly at
  revision 1). Errors in this order: `INVALID` (fact, action, expected
  revision, handler, note policy, duplicate), `CONFLICT` (expected revision
  differs), `TRANSITION` (step not allowed from the state), `SELF_APPROVAL`
  (`RESOLVE` or `DISMISS` of a blocking report by a handler sharing any
  identity key with the reporter). A step advances the revision by one.

Run:

```sh
(cd backend && go test -race -count=1 ./internal/modules/maintenance/domain/incident)
./gradlew :domain:test --tests 'com.anpfuel.domain.enterprise.maintenance.incident.*'
```
