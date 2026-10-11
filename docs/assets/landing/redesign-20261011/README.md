# Authentic landing reference and app captures

Date: 2026-10-11. Branch: **landing**. Scope: local design reference, genuine media,
font integration and planning; no publication or complete app acceptance.

## Selected reference

- [Desktop PNG](landing-reference-v2-desktop.png): 2880 × 6184, browser render at
  1440 CSS pixels and device scale 2.
- [Mobile PNG](landing-reference-v2-mobile.png): 750 pixels wide, browser render at
  375 CSS pixels and device scale 2.
- [Interactive HTML](landing-reference-v2.html): no phone in the hero; separate
  genuine app gallery and tutorial section; Previous/Next uses two real captures
  with numbered red rectangle overlays. The complete 30 chapters remain planned.
- [Browser evidence](browser-verification.json) and [render recipe](render-reference.cjs).
- [Composition brief](REFERENCE_BRIEF.md): native HTML/CSS composition, not an
  AI-generated app screen. The discarded first image used built-in imagegen and
  is superseded by the user's genuine-screen requirement; it is not delivered.

The brand SVG is the approved repository artwork. Fonts are the requested local
Manrope and Source Sans 3. This reference is outside `landing/static/` and is not
included in `landing/dist`; it does not replace production page markup.

## Original screenshots

[Manifest](manifest.json) lists fourteen **real, original 1220 × 2712 PNGs**, SHA-256,
visible state, privacy review and capture limitations. Open individual files:

- [Explore, light](screenshots/explore-light.png) / [dark](screenshots/explore-dark.png)
- [City selector](screenshots/city-picker.png)
- [ANP price list](screenshots/anp-prices.png)
- [Station list](screenshots/stations-list.png) (LR05: fuel filters, search, sort)
- [Station detail](screenshots/station-detail.png) / [source separation](screenshots/station-sources.png) (LR05)
- [Community empty state](screenshots/community-empty.png) / [feed, dark](screenshots/community-feed.png) (LR05)
- [History chart](screenshots/history.png) / [week picker](screenshots/week-picker.png) (LR05)
- [Contributions empty state](screenshots/contributions-empty.png) / [settings](screenshots/settings.png) (LR06: view-only, no account or toggle change)
- [Guest profile](screenshots/profile-guest.png)
- [Vehicle list empty state](screenshots/vehicles-empty.png)
- [Blank vehicle form](screenshots/vehicle-form.png)

Captured losslessly on the user's unlocked physical Android using
`adb exec-out screencap -p`; copied byte-for-byte, with system bars intact.
No synthetic content, recreated screen, retouch, upscale or AI image editor was
used. Browser phone borders and red SVG overlays are separate HTML elements.
1220 × 2712 is the original device resolution; DPR 2 of the reference composition
does not turn the original screenshots into higher resolution device evidence.

Installed app: `com.anpfuel.app.debug`, 3.1.0-debug/code 4; installed source SHA
unknown. ANP values in these frames are dated weekly survey references. Empty
community/vehicle/contribution states and a blank form do not demonstrate a
successful write. LR05 session (2026-10-11, authorized): read-only navigation
only — city stayed Sorriso, no account/vote/upload/permission/setting change;
one shopping notification frame and one secure-screen black frame rejected
(see manifest). Account creation/key, populated community, photo/OCR, permission
denials, deletion, alerts, full offline flow and other chapter frames remain
**MEDIA_PENDING** (see `lr06Pending` in the manifest).

LR06 constraint (2026-10-11, observed): auth/account-key screens set
FLAG_SECURE — `screencap` returns black while the hierarchy renders — so no
key content can leak through screenshots, and TU11/TU12 visual frames stay
pending without invented pixels. The auth layout text itself ("Sem email ou
senha", "Já tenho uma chave") corroborates the LR01 username/key freeze.
See the [full catalog](../../../product/LANDING_MOBILE_TUTORIAL_CATALOG.md).

## Safe capture environment (LR04) — 2026-10-11

Standardize before every capture session; record deviations in the manifest.
No capture was taken for this task — the eight accepted frames are carried
forward from the preparation manifest below.

- **Device selection:** the user's physical phone is used only for read-only
  public-state navigation with explicit per-session ADB authorization. Fixture,
  account, deletion, permission-denial and destructive flows run on a
  disposable emulator/profile, never on the user's phone.
- **Seeder prohibition:** `InstrumentedAppDataSeeder.clearAppStorage`
  (`app/src/androidTest/.../support/InstrumentedAppDataSeeder.kt:262`) and its
  callers (`seedReturningUserHomeState:37`, `seedAppendixA2PostSyncState:52`)
  must never run against the user's phone. No install, DB seed, logout,
  storage reset or persistent-settings change on that device.
- **Environment standard:** app language pt-BR, system font scale 1.0 unless
  the lesson teaches otherwise, portrait orientation, original 1220×2712
  resolution, keyboard hidden unless teaching input. Theme per lesson
  (light/dark recorded per frame); restore the user's theme afterwards
  (dark theme restored 2026-10-11, verified via `Ativar tema claro`).
- **Lock/privacy gate:** a locked black frame is rejected; capture only after
  the user unlocks. Close notification/media overlays with the user; reject
  frames with account keys, IDs, contributor GPS, private photos or
  notification content (music-overlay history attempts already rejected).
- **Capture command:** `adb -s <selected-device> exec-out screencap -p >
  <output>.png`, copied byte-for-byte, no recompression. Installed build
  carried forward: `com.anpfuel.app.debug` 3.1.0-debug/code 4, source SHA
  UNKNOWN (not linked to a repository HEAD).

## Capture continuation

1. Verify installed version, actual screen and permission for the selected task.
   Preserve the user's data and preferences; never run storage-clearing seeders
   on this phone. Use a disposable emulator/profile for fixture/deletion flows.
2. Close private notification/media overlays with the user's assistance. Do not
   expose credentials, contributor GPS, private photos or account keys.
3. Navigate visible app controls; inspect accessibility bounds where available.
   Capture stable screens directly to a local PNG, without lossy recompression.
4. Review every image before copying into this directory. Reject private or
   transitional frames. Hash and record dimensions, version, state and source SHA
   only when known. Keep original and derived web assets distinctly named.
5. Store red boxes as native-coordinate SVG/data overlays. Keep marketing
   captures clean and tutorial text usable independently of the overlay.
6. Restore changed preferences; do not submit, save, delete or upload real data
   just to obtain marketing shots. Record missing result/error states honestly.

## Re-render and preview

`render-reference.cjs` uses an existing external Playwright installation and
Chromium; it adds no project/runtime dependency. Set `PLAYWRIGHT_MODULE` to the
installed module path and optionally `CHROMIUM_EXECUTABLE` to an existing browser,
then run `node docs/assets/landing/redesign-20261011/render-reference.cjs` after
building landing. It starts a temporary localhost server, verifies actual emitted
font responses, tutorial controls and responsive widths, writes PNG/evidence and
closes the server. It never publishes the page.

To inspect interactively, serve the repository root on a separate localhost port
and open `/docs/assets/landing/redesign-20261011/landing-reference-v2.html`.
Do not expose this documentation server publicly.
