# ADR-022 — CoMaps-derived native map engine behind a narrow Android port

Date: 2026-10-24
Status: ACCEPTED for the owned native extraction. The local ENGINE_GATE
checkpoint is recorded in [MAP-14_ENGINE_GATE](../backend/evidence/maps/MAP-14_ENGINE_GATE.md).
Product integration and production certification retain their separate gates.

## Context

Abastevo needs an offline vector map with fuel/charging overlays, search, car
routing and guidance. The approved user direction is a pinned, audited
CoMaps-derived C++ core isolated behind an Android interface, not a second app
and not a silent switch to a proprietary or WebView/MapLibre engine.

The upstream source at `comaps/comaps`
`4587b8ad4d12c9f59982709fdaf03cc925948827` provides an Apache-2.0 core, an
in-repository `android/sdk` Gradle module, `.mwm` map packages and native
generator tools. It is not a published drop-in SDK: it builds the whole native
core and many third-party submodules, and its resources/styles/fonts and map
data require separate admission.

## Decision

1. Use a fixed CoMaps revision as the candidate native engine. The exact SHA,
   submodule gitlinks, LICENSE/NOTICE and third-party copyright inventory are
   recorded before any AAR/APK is built or shipped.
2. Keep the engine behind an Abastevo-owned Android port. Compose/domain/data
   code must not depend on `app.organicmaps.*` types. The adapter exposes only
   the verified operations needed by Abastevo: map surface lifecycle, camera,
   marker/overlay selection, search, route and guidance events.
3. Preserve `com.anpfuel`, existing modules, MIT notices and the current
   navigation/DI roots. Native libraries remain inside the adapter module.
4. Price/community overlays stay in Abastevo's canonical UUID model. Prices and
   overlays never require regenerating the routing graph. OSM reference POIs
   and Abastevo stations are linked only through explicit, reversible identity
   evidence.
5. The native map generator is a separately pinned build-time tool. Map format
   is not reimplemented in Rust or parsed ad hoc in Go. Go orchestrates
   acquisition, bounded generation and publication; Rust continues to prepare
   admitted local inputs deterministically.
6. No upstream CDN, demo server, API key or public tile bulk download is a
   production dependency. Abastevo hosts admitted regional packages through its
   own signed manifest/download contract.
7. The artifact must prove real render, pan/zoom, custom POI selection, offline
   search and one offline car route before any map UI depends on it. A mock,
   screenshot, external intent or compile-only APK does not pass ENGINE_GATE.
8. If extraction fails, record the concrete build/interface/resource failure and
   revise the bounded design. Do not silently substitute MapLibre, Google Maps,
   a WebView or a mocked router.

## Licensing boundaries

- CoMaps core code: Apache-2.0; retain LICENSE/NOTICE, mark modifications and
  review every shipped transitive dependency.
- OSM/Geofabrik data: ODbL, attribution and derived-database obligations; not
  relicensed as MIT.
- ANP, Open Charge Map, provider data and community contributions: their own
  admission/retention terms. An `opendata` flag or public URL is not admission.
- Generated Abastevo pictograms: original project artwork under the repository
  MIT notice; no CoMaps/operator logo is copied.
- Fonts, styles, icons and map resources are audited separately from code.

## Consequences

- Native build cost and APK size/Memory/ABI support become explicit measured
  budgets (MAP-14/MAP-29), not hidden assumptions.
- The app can keep working without a downloaded map, and map failure is shown
  explicitly instead of breaking browsing.
- The adapter boundary permits a later evidence-backed engine change, but any
  alternative requires a revised ADR and plan; it is not a runtime fallback.
- This ADR does not accept transit/hiking/tourism/OSM-editor scope, iOS/CarPlay,
  live traffic, charging payments/reservations or guaranteed arrival.

## Verification

MAP-14 evidence must include the pinned source/toolchain, real native build and
launch on an authorized emulator/device, render/search/route/POI selection,
ABI/16 KB page-size/minSdk checks, packaged dependency/network inventory and
lifecycle/leak results. The local extraction checkpoint now passes for its
recorded runtime matrix. The real map tab and public release remain their
owning phases; failed emulator profiles are not accepted by this decision.
