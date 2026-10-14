# Community capture and Home source consolidation

Status: IN_PROGRESS

Opening: 2026-10-14, codex/community-first at c0752909; continuation of PR135 under the maintainer's explicit request. Main/dev and the occupied primary dirty evidence adapter remain preserved. No deployment or device operation is implied.

## Frozen behavior before implementation

B-BR-CF08 / BUC-CF06: remove developer capture UI and gallery/test-policy issuance from both Android variants. The ordinary camera→OCR→explicit review→signed upload/outbox→server receipt journey remains free and independent of GPS. Require canonical station selection, account proof, camera permission, immutable receipt/evidence deadlines and existing quotas/privacy. Previously saved developer reviews refuse with recovery, never become ordinary evidence. Server legacy receipt consumption remains compatible; removing the app mode does not rewrite audit history.

B-BR-CF09 / BUC-CF07: Home has an ANP / Community source selector. Each source uses the existing average/minimum/maximum filter. ANP retains its selected survey-week provenance; community summarizes the latest live STANDARD price projection for each active station in the selected municipality, one equal-weight station sample per product/unit. Do not average raw observations, helpfulness votes, historical prices or only the first feed page. Exclude expired, unknown/disputed, future-anchor and nonstandard-condition rows. Exact integer milli-BRL statistics; arithmetic mean rounds to the nearest milli-BRL, half upward. Currency, products and units never mix. No samples means unavailable, never an ANP substitution.

Community data updates by bounded foreground refresh on entry/resume, manual retry and every 15 seconds while Home displays Community. Cancel when hidden or switching source/city; late results cannot replace a newer city/request. Label the community sample count and update time separately from the weekly ANP source. This is near-real-time polling of committed projections, not a live ANP feed or a guarantee of immediate worker completion. Home must work for Community even without downloaded ANP history; offline/error states do not present cached community values as fresh.

The public station price contract merges official and community product/unit keys, including community-only stations; source sections stay typed and independent. Loading/pagination uses bounded queries and stable station UUID tie-breaks. Feed pagination cannot determine a city's aggregate; mutable live feeds may move between pages, so refresh resets pagination and clients deduplicate station IDs. Public outputs contain no contributor identity/GPS/private media.

## Bounded execution

- CFR01: remove app development capture ingress; test ordinary no-GPS capture/legacy recovery and debug/release/static compilation.
- CFR02: typed community station price loading and community-only product keys; immediate SQL/HTTP/codec negative/expiry/source tests.
- CFR03: one bounded municipal summary query and public endpoint, full-population/sample isolation and deterministic pagination tests against disposable PostGIS.
- CFR04: Home source selector, shared three filters, lifecycle-scoped community refresh with cancellation/error/city guards, localized source/freshness states; JVM/lint/build acceptance.
- CFR05: final evidence, source checks, atomic commits and cumulative PR135 update. No main merge/deploy/wiki; PRM22 remains DEFERRED. Native/staging execution remains OWED.

## CFR01 checkpoint

LOCAL_DONE / INTEGRATION_PENDING: removed debug/release DeveloperCaptureMode/Controls and DevelopmentPhotoCapture issuance adapters, their switch/gallery ingress and developer-only strings. Normal nominal search now has ordinary station-city/query/result names. Removed obsolete station-area gate UI. Camera always requests the normal v2 policy and refuses developer receipts; saved developer reviews still cannot enter either submission path. Ordinary camera tests replace the obsolete mode-specific campaigns and run in shared debug/release sources, preserving signed review/subset/received/partial/precision/changed-station assertions. Focused CaptureOcrViewModel, NormalCameraContribution and PhotoCaptureHttpClient JVM campaign PASS (Gradle 1m13s). Full variant/lint checks belong to final acceptance. No GPS prompt or developer authorization is added to the normal journey.
