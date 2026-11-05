# Forum Android interaction and implementation design

Status: PROPOSED / NOT IMPLEMENTED. Source: GitHub main `3fc77acc` (PR #191).
The [product contract](../product/FORUM_ABASTEVO_CONTRACT.md) owns behavior.
Preserve current root modules, `com.anpfuel`, MIT notices and bottom navigation.

## Verified integration seams

- [CommunityScreen](../../app/src/main/kotlin/com/anpfuel/app/ui/community/CommunityScreen.kt)
  renders a LazyColumn: city → contribution card → price activity → filters →
  existing station price rows. Insert exactly one Forum item between the first
  two existing items. Do not replace `FeedPriceCard` or its actions.
- [CommunityFeedViewModel](../../app/src/main/kotlin/com/anpfuel/app/ui/community/CommunityFeedViewModel.kt)
  owns city/fuel/sort, foreground refresh and cancellation. It remains the price
  feed owner; Forum gets separate ViewModels and no price refresh loop.
- [Routes](../../app/src/main/kotlin/com/anpfuel/app/navigation/Routes.kt) and
  [AnpNavGraph](../../app/src/main/kotlin/com/anpfuel/app/navigation/AnpNavGraph.kt)
  own navigation. The graph already renders a common bottom bar on ordinary
  child destinations; Forum must not mount a second bar or Scaffold inset stack.
- [AbastevoBottomBar](../../app/src/main/kotlin/com/anpfuel/app/navigation/AbastevoBottomBar.kt)
  owns exactly Explore, Community, Map and Profile. Its route classifier may
  require a minimal `forum/` → COMMUNITY mapping to keep selection correct on
  children; do not add/reorder tabs, change icons or alter tab-base behavior.
- [ActionCardButton](../../app/src/main/kotlin/com/anpfuel/app/ui/components/ActionCardButton.kt),
  [Theme](../../app/src/main/kotlin/com/anpfuel/app/ui/theme/Theme.kt) and
  [container guidelines](UI_CONTAINER_GUIDELINES.md) supply current controls,
  active app theme, dynamic colors and shared `AnpContainerDefaults` border/shape.
- [AnpFuelDatabase](../../data/src/main/kotlin/com/anpfuel/data/local/AnpFuelDatabase.kt)
  is version 36 at inspection; allocate next Room version on current main,
  update explicit migrations/schema export without destructive fallback.
- Existing `domain`, `application`, `data`, `app` modules remain the dependency
  direction. Reuse `data/di` Hilt conventions and existing configured backend
  origin/session/signer; do not add a hardcoded origin or `/v1` twice.

## Navigation and state

Proposed routes: `forum/feed`, `forum/posts/{postId}`, `forum/new`,
`forum/posts/{postId}/edit`. UUID arguments validate before request. Entry uses
singleTop without clearing Community. Detail/form use normal push/pop. Back and
gesture return to the previous entry's city/filter/list position; a deliberate
bottom-tab action keeps current `navigateToTabBase` semantics. External article
opening returns to the same detail, not a new app root. No public deep links in
baseline; add only if a verified distribution need and host contract exist.

Use navigation-entry-scoped Hilt ViewModels, immutable StateFlow UiState and
`collectAsStateWithLifecycle`. State distinguishes initial loading, refreshing,
loading next page, empty, content, offline/stale, terminal error, cursor expiry,
auth-needed, pending-review receipt and write-unknown outcome. SavedStateHandle
stores filter, selected post ID and scroll anchor only. Keep a rememberSaveable LazyListState
per navigation entry and filter scope; do not recreate the Community back-stack
entry merely to return from Forum. User draft is memory-only
and cleared on account change/logout; process death explains draft loss rather
than persisting private text silently. No raw post text in analytics or logs.

Every query has normalized scope + request generation. Filter changes, refresh,
account/origin changes and navigation disposal cancel obsolete requests and
refuse their late results. One first-page and one next-page operation per scope;
append de-duplicates IDs while preserving server order. Refresh replaces snapshot
and cursor atomically. Repeated submit taps share one operation UUID; unknown
network result offers retry of the same operation, never optimistic duplication.
No timer/poll loop: load on entry, explicit refresh or one network recovery event
when stale and foreground. Preserve successful content on non-authoritative
network failure; a live 404/erasure clears it.

## Screens and visual specification

### Entry card (FOR-08)

Purpose: discover the new destination without competing with price contribution.
Use one full-width neutral `surfaceContainerLow` card with shared shape and
`borderStroke(effectiveFill)`; 16dp padding and existing 12dp inter-item spacing.
Leading Material conversation icon is decorative. Title “Fórum Abastevo”,
description “Notícias, novidades e discussões sobre o mundo automotivo.”, CTA
“Explorar fórum”. Existing `ActionCardButton` may provide the action only if it
does not add nested redundant outlines; otherwise use the existing Material
button style inside the card. One clear focusable CTA; no double clickable card
and button announcing the same action. Visible even without city selection.

Data/dependency: Forum feature-read capability only, no feed fetch from this
card. Show it only for an enabled compatible backend. Default-off gate avoids
a dead route in unsupported releases. Failure disables Forum entry availability
through config, never price contribution. Match app-selected light/dark/dynamic
theme; no fixed colors, heights, badge pile, banner artwork or emoji asset.

### Feed (FOR-08; filters FOR-24)

Purpose: discover content. Top app bar “Fórum Abastevo”, back and refresh actions;
one compact category selector, then reading-first LazyColumn. Each item shows
kind label, readable title, optional permitted short introduction, source/author
and original date. No image slot in MVP, no like counter, no giant stacked CTA.
News shows source date; editorial/topics show first publication date; changes
are labeled updated/corrected rather than silently replacing original time.

Data: GET categories/posts; repository public cache. Filter default is all
Brazil, not a hidden GPS/city restriction. Regional subject filtering becomes
explicit in FOR-24. Optional text search is debounced 400ms and cancellable;
recent order stays predictable. Add “Novo tópico” only after FOR-23 and writes
enabled; visitor opens existing sign-in and returns without auto-posting.

States: skeleton for first load; honest empty category/no content; retained list
with inline retry for page failure; explicit load-more initially (no unbounded
autofetch); refresh indicator separate from initial skeleton. Expired cursor
offers reload. Offline shows age and cached editorial rows, no cached user rows.
One explanatory error/retry replaces an empty inaccessible route.

### Post detail and discussion (FOR-08, FOR-19)

Purpose: read and understand provenance, then discuss. Title, type, source/alias,
original time, update/correction label, original introduction/body with generous
line spacing; explicit “Ler na fonte” with hostname for news. A bulletin uses a
compact methodology/provenance block, not an invented chart. Related station
uses existing UUID route when present; it does not override map selection.

Data: public post plus separately paginated comments/replies; no N+1 network
call per card/author. Text only; long-form reading does not use WebView. Comments
are a section of the same LazyColumn, with stable IDs and bounded one-level
replies opened on demand. Composer remains reachable above IME/navigation
insets. Owner actions use a separate private owner read; public alias comparison
is never authorization. “Denunciar” is available for eligible signed accounts,
with a simple reason dialog and safe receipt.

States: 404/hidden removes stale local copy and shows unavailable; transport
error can show eligible cached editorial with age. Social section offline offers
retry without claiming zero comments. Pending submission shows a review receipt,
not a new public comment. Expired session prompts existing auth, preserving only
memory draft for the same owner. Review rejection/hide reason and appeal contact
are shown in the private receipt view when applicable, not public report data.

### New/edit topic and comment editor (FOR-19/FOR-23)

Purpose: intentional, bounded publication. Topic form: title/body/category,
optional explicit region, counter/help and submit. Comment editor: plain text
and counter; reply target is named without recursive nesting. No attachment,
private location, automatic publisher link card or forced GPS/account payment.

Data: signed POST commands with server-derived identity, operation UUID and
expected revision; private owner read on edit. Disabled submit until valid;
while sending, retain the same intent and expose progress. Server limits remain
authoritative. 409 offers reload/copy current draft, never overwrites silently;
429 shows retry time; 401 routes to sign-in; foreign 404 closes editor safely;
unknown outcome retries same intent; cancellation does not falsely show failure
or create a second submission. Back with unsent text asks to discard; no silent
send. Delete has explicit confirmation and scrubs current/cached text on success.

## Repositories, cache and proposed file ownership

New `domain/.../forum` types and ports; `application/.../forum` use cases where
current analogous use cases live (confirm exact placement in FOR-06);
`data/remote/ForumHttpClient.kt`, `data/repository/ForumRepositoryImpl.kt`,
`data/di/ForumModule.kt`; `app/ui/forum/{ForumFeed,ForumPost,ForumEditor}ViewModel.kt`
and corresponding Compose routes/content. These are proposed new files, not
claims that these classes already exist. Reuse existing HTTP cancellation,
bounded decoding, coroutine dispatcher and auth refresh patterns; no new
networking/paging library is assumed.

Room entities: one cached post per origin/id/revision plus bounded feed scope
membership/order. Cache only public editorial/news/bulletin records. Limit five
scopes ×20 summaries and twenty details, maximum 2 MiB text total, 24h TTL;
LRU/expiry cleanup before admitting more. Do not store full unlimited cursor
chains. Filters/origin define cache scope; origin switch cannot reuse another
backend's UUID/content. Online refresh replaces memberships transactionally;
never return a remembered old cursor as live. Social public/private results
remain memory-only; logout clears drafts/private state and cancels owned work.
Future privacy/legal policy may demand a shorter cache horizon before launch.

## Accessibility, localization and evidence

Use existing typography and Material tokens with >=48dp touch targets; title
heading semantics, logical source/date/body/comment reading order, meaningful
button labels, decorative icon descriptions null. Do not truncate important
source attribution or reduce font size to fit. Support 320dp, long translated
labels, 100/130/200% font, light/dark/fixed/dynamic themes and IME/system bars.
Display device-local date/time with a timezone-accessible full timestamp; source
date stays unchanged in the API. No machine translation of source articles.

All new strings belong in default and every currently supported locale
(`values`, pt-BR, es, de, fr, ja, ru, zh-rCN at inspection); verify resource
directories on current main before editing. Tests cover keys/plurals/long labels;
editorial content remains its source language, marked when different.

Extend actual CommunityFeedScreenTest and BottomTabNavigationDeviceTest, plus
Forum-specific JVM/HTTP/Room/native tests in the
[validation plan](../planning/FORUM_ABASTEVO_VALIDATION.md). Native controller
navigation, screenshots, font/theme/IME and TalkBack traversal are distinct from
JVM tests or APK compilation. The final union/device batch remains OWED until
executed; preserve existing user deferral policy and report its state honestly.
