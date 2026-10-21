# OPT home/community loading — Phase 2 evidence (UI 15 s removal)

Change: `CommunityScreen` no longer states any refresh interval. The subtitle under
`feed_prices_title` keeps only the correct static condition label via the renamed resource
`feed_standard_prices` ("Standard prices" / "Preços padrão" / "Standardpreise" /
"Precios estándar" / "Prix standard" / "通常価格" / "Обычные цены" / "标准价格").
No separator, spacer, or empty area was left: the surrounding `Column` still holds title +
subtitle, now without interval text.

Refresh affordance preserved: top-app-bar `IconButton(onClick = onRefresh)` with
`R.string.feed_refresh` content description is untouched, enabled when
`state.city != null && !state.loading`.

Verification (worktree `codex/opt-home-community-feed`, head `f60bb3e2` + working tree):

- `rg feed_auto_refresh app/src/main` → zero hits (only the renamed `feed_standard_prices`
  remains: 1 Kotlin usage + 8 locale resources).
- Interval-text grep across `app/src/main` (`15 segundos|15 seconds|15 Sekunden|cada 15|
  every 15|alle 15|toutes les 15|15秒|каждые 15|每15秒`) → zero hits.
- `./gradlew :app:assembleDebug` → BUILD SUCCESSFUL (16 s).
- `./gradlew :app:testDebugUnitTest --tests "com.anpfuel.app.ui.community.*"` →
  BUILD SUCCESSFUL.
- `./gradlew :app:lintDebug` → BUILD SUCCESSFUL; `lint-results-debug.xml` contains 243
  `<issue` entries, zero with `severity="Error"`/`"Fatal"` (warnings only, pre-existing).
- `git diff --check` → clean. Secret-surface grep on touched files → no hits.

Backend `refresh_after_seconds: 15` echo is intentionally untouched in this phase (API contract,
asserted by `feed_test.go` and the Android decoders); it is not user-visible.

Status: LOCAL_DONE
Validation: PASS
