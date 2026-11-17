# Store material

Everything needed for the Google Play listing, plus the reference for a future
App Store release.

| Path | Contents |
|---|---|
| [PLAY_STORE_READINESS.md](play-readiness.md) | Play requirements checklist with verdicts (valid / fixed / blocking / to confirm) and sources |
| APPLE_APP_STORE_ASSETS.md | App Store Connect screenshot, preview and icon specs (iOS is archived) |
| [../play-store-listing.md](play-store-listing.md) | Listing text (pt-BR default, en-US), metadata, alt text, graphics map |
| `screenshots/raw/` | Unedited emulator captures, opaque RGB PNG |
| `cards/` | Card generator: `build_cards.py`, `cards.json` (captions/sources), `card.html`, `feature.html` |
| `listing-assets/` | Upload-ready files: `icon-512.png`, `feature-graphic-1024x500.png`, `phone/`, `tablet-7in/`, `tablet-10in/` |

## Screenshots

Captured on 2026-11-15 from the debug build of `origin/main` `93f3a88d` (and the
same tree with targetSdk 36 for the tablets) talking to the production origin
`https://abastevo.com.br`, on the owned API 36 emulator (`sdk_gphone64_x86_64`):

- app locale pt-BR (`cmd locale set-app-locales`), light theme, System UI demo
  mode (10:00, full battery and Wi-Fi, no notifications);
- `wm size`/`wm density` overrides: phone 1080 × 1920 @ 420 dpi, 7-inch tablet
  1080 × 1920 @ 280 dpi (sw ≈ 617 dp), 10-inch tablet 1440 × 2560 @ 320 dpi
  (sw 720 dp); all 9:16 so they qualify for Play promotion;
- cities: São Paulo, SP and Sorriso, MT; synthetic emulator GPS; no account
  signed in; vehicle label "Meu carro"; offline map packs and five survey weeks
  downloaded through the app.

The prices are real production data for the week 8–14 Nov 2026. Station names
and addresses are public ANP records. No personal data, contributor location or
photos of people appear. Recapture after visible UI changes.

## Regenerating the cards

```bash
python3 docs/store/cards/build_cards.py            # all sets + feature graphic + icon
python3 docs/store/cards/build_cards.py --only phone
```

Requirements (tooling only, not app dependencies): Python 3 with Pillow and a
local Google Chrome or Chromium (pass `--chrome PATH` if it is not on `PATH`).
The template uses the app's bundled Manrope font and the approved brand icon.
Captions live in `cards/cards.json`; `{braces}` mark the highlighted words. Keep
captions short (they must stay under 20 % of the image) and free of claims or
calls to action — see the readiness doc §4.6.

Design: brand navy gradient (`#0d1d57` → `#050b22`) with blue `#0052f7` and mint
`#63e7a4` glows, white Manrope ExtraBold captions with mint highlights, the real
screenshot with rounded corners and no device frame. The feature graphic keeps
the icon, name and tagline centered and puts decoration only at the edges, as
Play asks.
