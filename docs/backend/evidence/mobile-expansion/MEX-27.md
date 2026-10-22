# MEX-27 — Station filters and separated official timeline Android

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Parent: `69d584bb` (maintained dev, PR #146 head). Date: 2026-10-22.
Depends MEX-24/25/26, accepted. User-authorized Android scope.
Rules B-BR-MEX-10/11/13–15/18; BUC-MEX-08/09. Risk: attribution/UI
(critical: company vs source-global separation, no clean-filter
inference, preserved pagination).

Before behavior: the explorer gains a concise filter sheet (UF,
segment, subject kind with clear/apply), a company/source-global
timeline (date-descending sections of the loaded window, never a
global sort claim), and preserved filters/pages across detail
navigation and process restart. Every sheet selection hits the wire
as an exact API facet; multi-filter changes reset the cursor; no
"No infractions" or clean/safe filter exists anywhere. Wrong or
retired bindings stay excluded by the API; unknown fuels page empty
through exact matching. Recovery: disable the sheet without hiding
global facts.

## Delivered

- `OccurrenceFilter` extended (segment, subjectKind, isEmpty, clear)
  with SavedStateHandle persistence; gateway calls carry all facets.
- `timelineSections`: company-kind versus source-global sections,
  date-descending, first-seen stable; pure and unit-tested.
- `OccurrenceExplorerScreen`: modal filter sheet, timeline toggle
  with section headings semantics, clear-filters action, and the
  existing coverage/stale/empty states untouched.
- Eight-locale strings for sheet/timeline/clear/apply/all/sections.
- `OccurrenceExplorerViewModelTest` extended: multi-facet reset and
  clear, plus timeline separation and ordering.

## Actual checks

From this worktree (2026-10-22, parent `69d584bb`):

- `./gradlew :app:testDebugUnitTest --offline --tests
  "com.anpfuel.app.ui.occurrence.*"` → PASS, 6 tests (4 prior +
  multi-facet reset/clear and timeline separation).
- `./gradlew :app:testDebugUnitTest --offline --tests
  "com.anpfuel.app.navigation.*"` → PASS (no navigation regression).
- `./gradlew :app:lint --offline` → PASS (no new issues; eight
  locales complete).
- `git diff --check` PASS; `bash scripts/scan-secrets.sh` PASS.
- No backend, contract, worker, Room, iOS or navigation-tab
  changes; no source activation; no PII/secrets in Git/fixtures;
  `com.anpfuel` and MIT notices preserved.

## Limits

- Timeline sections separate by source-stated subject kind
  (company-shaped vs rest), not per-record link state: company-kind
  records may link to current companies (verified separately),
  source-global records never do. True link state needs the
  association read, not available to the list cell.
- Timeline order covers the loaded window only; cross-page global
  date ordering is not claimed. Cursors still bind revision and
  filters; changing any facet restarts from the first page.
- Municipality and period facets stay on the record API; the sheet
  exposes the most-used subset (UF/segment/kind) with clear. No
  clean/safe filter exists by design.
- No device farm, no production, no Play release and no release
  acceptance is claimed.

Next: batch closure needs definition (PR #146 holds MEX-25/26/27);
MEX-31 stays blocked on its own chain.
