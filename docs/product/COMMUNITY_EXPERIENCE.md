# abastevo community experience — product specification

State: **PLANNED**, 2026-10-01. This describes the redesigned experience, not shipped screens. [ADR-016](../adr/016-android-commercial-community-ios-deferred.md) and the [execution plan](../planning/COMMERCIAL_COMMUNITY_PLAN.md) govern implementation. Existing backend/account/social modules are locally integrated; the new commercial UX is not.

## Promise and primary journey

Help a driver find a recent, supported price at a nearby station and help keep it current with the community. Use plain Brazilian Portuguese, progressive permissions and a readable price before exposing expert tools. Do not claim real-time coverage, universal GPS-spoof prevention, fuel laboratory quality or guaranteed savings.

Proposed navigation, to validate in P19:

- **Explorar:** selected city and fuel; a useful station list first, optional map; recent community prices with observation time, support/state and price condition; clear route action. Search, distance/price filters and sort are explicit. No unconditional cheapest-price ranking for loyalty/app-only prices.
- **Comunidade:** local station activity, contributions and discussions; select city manually without collecting precise contributor GPS. Structured records rather than an endless photo gallery. Favorites/following/notifications are introduced only after the scoped contracts below.
- **Perfil:** free account, contribution status/history, rights/privacy and settings. Existing vehicles, tank calculations, ANP history and advanced analysis remain accessible under clearly named tools.
- **Atualizar preço:** labeled action leading to capture and confirmation. The Home floating action is temporarily omitted by the 2026-10-06 user request; contextual contribution entry points remain available. It is an action, not a fourth feed tab or a mandatory onboarding step.

Guest users can explore first; contextual free registration is required for comments, replies, ratings and votes. Price-observation proof remains governed by existing identity rules: do not silently remove the anonymous signed contribution API. Onboarding offers email access code, Google and Apple where the provider/platform flow is supported and verified. Native Apple login on Android is validated as its own provider flow; iOS deferral is not provider acceptance.

## Proposed business rules and use cases

These additions document intended presentation. Existing feedback/media/consensus contracts remain authoritative; any API change is additive and tested before the consumer.

- **B-BR-C01 — Primary price truth:** the primary price is a community current-price projection with fuel, exact currency/unit, condition, provenance and timestamp. Fresh/stale/disputed/unknown states reflect server rules. ANP data appears only as a dated reference; missing community coverage says so. Never promote ANP into an unlabeled live price or reset its date on refresh.
- **B-BR-C02 — Evidence versus publication:** photographing a price starts a submission. OCR proposes editable values; the user confirms station, fuel, price and condition. Backend validation/consensus decides publication. Pending/rejected evidence does not immediately replace a supported price. Photo-led UX does not by itself change the existing optional-evidence API contract.
- **B-BR-C03 — Temporary private media:** photos are audit evidence, private and limited to 24 hours across all owned copies. Public activity/history contains structured data, not persistent photo URLs. Metadata may have a separately documented lawful retention; no restored photo may bypass expiry. Existing P15 size/pixel/memory budgets apply until an owning task proves a change.
- **B-BR-C04 — Separate social signals:** personal fuel ratings, comment validity agreement and price confidence have separate labels and calculations. Comments/replies have at most 280 Unicode scalar characters; one reply level and revision-bound unique valid/invalid votes follow the existing feedback contract. No votes means no percentage, not zero reliability. Show denominator and explain that votes are community agreement, not fact certification.
- **B-BR-C05 — Free and fair participation:** free account registration and social participation stay free. Paid benefits never change trust, moderation, ranking, contribution weight or existing free entitlements, including three local vehicles. No artificial contributions or paid reputation seed the community.
- **B-BR-C06 — Respectful local discovery:** city/manual browsing works without GPS; camera/location requests occur when useful. Known simulated location denies proximity claims; unknown integrity is explained honestly. Public content and logs never expose contributor precise GPS, private photos or credentials.
- **BUC-C01:** first-use guest selects city/fuel, reads recent community price or explicit absence, sees dated ANP reference if needed, opens station/route; no forced signup/GPS.
- **BUC-C02:** contributor captures a legible photo, confirms OCR fields/price condition, submits once, and sees pending/accepted/disputed/rejected/retry states. Offline queue uses stable identity/idempotency and never labels queued data as published.
- **BUC-C03:** free account rates/comment/replies/votes at a station/fuel, edits where allowed, reports abuse, and receives understandable moderation/outdated-revision errors. Owner-only private reads stay protected.
- **BUC-C04:** user follows a favorite station and opts into useful price/activity alerts; revoke permission/unfollow/delete account stops owned subscriptions. New alerts never expose private contribution data or guarantee unavailable prices.
- **BUC-C05:** moderator reviews reports with bounded access, resolves/removes/appeals according to recorded policy and leaves auditable non-PII outcomes. Reports do not automatically delete competitors' content.

## Screens and states

Station detail prioritizes current community fuel cards, time/condition, route and update action. Discussions/ratings and a collapsed dated ANP reference follow. The contribution flow is capture → review/edit → submit → status; registration/permission interruptions restore the draft without duplication or exceeding retention.

Cover first launch, manual city, denied permission, unsupported integrity, simulated GPS, no nearby stations, no recent community price, stale/disputed price, offline cache, expired media, illegible photo, wrong OCR, condition ambiguity, rejected contribution, timeout/retry, expired account, deleted comment revision, rate limit and moderation. Never display invented coverage, mock members or estimated prices as real.

## Visual identity and accessibility

Use the approved blue/green A on white, navy reference wordmark and generous whitespace. Blue is the principal action color; green signals contribution/community without implying price truth. Derive named theme tokens from approved assets; measure contrast before use, especially white text on green. Warning/error/state labels need text/icons as well as color. Retain readable money/time/fuel hierarchy, large touch targets, screen-reader labels, reduced-motion behavior and font-scale/layout testing. Do not redesign the approved logo.

P19 defines a small reusable set of price card, source/time badge, state, station header, contribution progress, discussion/reply/vote and permission components within existing modules. Domain state drives presentation; no business rules in composables. A later polish phase may refine aesthetics after acceptance; useful navigation/readability is part of the commercial functional work now.

## Commercial community operations

Start with a bounded geographic pilot after G09 certification. Recruit real voluntary contributors, explain audit expiry and moderation, provide report/support channels and define staff coverage before opening a city. Publish coverage honestly; sparse areas remain usable through clearly dated ANP reference. No scraper-generated fake community activity or financial incentive tied to trust.

Measure aggregate fresh station coverage, observation age, confirmation delay, valid/rejected/disputed share, successful contribution completion, moderation backlog/response and opt-in return participation. Define purpose, denominator, observation window, minimum aggregation and deletion before instrumentation; do not introduce advertising IDs or precise location analytics. Proposed usability/speed targets are frozen after baseline measurements in P19/P24, not fabricated outcomes.

Optional monetization follows evidence from the pilot: hosted backup/sync, advanced opt-in alerts or other validated convenience can be considered in P11 with separate billing/security/consumer-rights acceptance. No new paid restriction or subscription service is implemented by this plan.
