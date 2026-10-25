# Historical map remediation progress before the final 2026-10-25 review

This snapshot preserves the previous current-progress text. Its completion and
reader/native/artwork claims are superseded by the [current remediation evidence](../../backend/evidence/maps/MAP-15_20_REMEDIATION_20261025.md). It is not a new gate acceptance.

# Current execution state

- Map/EV construction & integration remediation 2026-10-25: MAP-15/16/17/18/19
  LOCAL_DONE / INTEGRATION_PENDING; MAP-20 LOCAL_DONE / INTEGRATION_PENDING
  with MINIMUM_MAP_GATE PARTIAL (no fake PASS) on `codex/map-ev-implementation`
  (`a203b7ad`, `5473ccfb`, `235d52e7`, `cf031ce9`, `27801f48`, `3496a5ba`
  plus this progress record). [MAP-15](../backend/evidence/maps/MAP-15.md)
  proves API-26 Monocypher/JNI Ed25519 with Go bytes, 18→26 migration,
  WorkManager retry/cancel, streaming byte-cap enforcement, incompatible
  Content-Range rejection, atomic `<formatVersion>/<file>.mwm` native layout
  materialization (`FindAllLocalMapsAndCleanup`), reader retention leases, and
  explicit distribution configuration gating. [MAP-16](../backend/evidence/maps/MAP-16.md)
  proves single-owner `MapViewModel.ensureInitialized(context)` with application
  context, readiness-gated surface, GPS/off/no-fix states, camera restore, and
  production route proof (`MapRouteInstrumentedTest` on physical device
  2311DRK48G / VKCQGAUCGYAMR4W4). [MAP-17](../backend/evidence/maps/MAP-17.md)
  delivers Explorar|Comunidade|Mapa|Perfil with real `MapHost`, ANP/Community
  selector, active-package availability refresh, wired `MapDownloadsScreen`
  actions (download/cancel/retry), and eight-locale strings.
  [MAP-18](../backend/evidence/maps/MAP-18.md) proves UUID-stable deterministic
  clusters, REVIEWED-only pins with unknown/city-centroid/null-coordinate
  stations selectable by UUID (`selectableByUuid`), accessible cluster member
  chooser (2nd/last member selection and cross-generation reconciliation),
  combined `#placemark-purple` KML style, and 48dp TalkBack list.
  [MAP-19](../backend/evidence/maps/MAP-19.md) wires real `SheetPriceSource`
  (ANP station detail + Community feed/detail + EV community reports), fixes
  the ANP EV source-switch race (`MapReviewRegressionTest` in permanent suite),
  and routes electric vs fuel actions honestly. [MAP-20](../backend/evidence/maps/MAP-20.md)
  connects viewport movement, city selection, paginated EE-18 charging snapshot
  queries (no silent 100-item truncation), extended fuel/EV filters, 36 map
  strings × 8 locales, reproducible `comapsSdkDir` SHA-256 verification, and
  full JVM/lint/release APK verification (78,103,646 bytes / 74.49 MiB, both
  ARM ABIs, 16 KB ELF LOAD alignment, under 80 MiB). Staging has no REVIEWED
  coords, station prices, or signed `.mwm` distribution host configured yet;
  `MINIMUM_MAP_GATE` stays PARTIAL, not certified. No push/PR/merge/wiki/deploy;
  MAP-21–33 untouched.
