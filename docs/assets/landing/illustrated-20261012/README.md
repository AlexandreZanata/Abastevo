# Abastevo illustrated assets — LH05

Status: LOCAL_DONE / INTEGRATION_PENDING. Generated on 2026-10-12 with the built-in image generator, under the user's supplied visual references. These are artistic illustrations, not station coverage or live fuel-price data.

## Files and reuse

- `masters/hero-background-native.png`: original generator output, 1672×941.
- `masters/hero-background-4k.png`: 3840×2160 export enlarged from that original with Lanczos. It is not native 4K generation. The flat editorial shapes tolerate resizing, but this adds no native detail.
- `masters/{trees,skyline,road,car}-native.png`: reusable transparent decorations in the same flat editorial style. No embedded labels or brands.
- `masters/{consult,check,contribute,driver,ride-driver,fleet}.png`: six transparent action/persona scenes with blue, green, orange, violet, teal and rose accents, respectively. No embedded text. These are the more dimensional minimal card illustrations requested earlier.
- `assets.json`: original hashes, export method and all responsive WebP derivatives.
- `generation.json`: exact final hero/decoration prompts and tool output paths. Card intent is summarized below, rather than represented as an exact prompt transcript.
- Browser proofs: desktop/mobile light/dark and six scenes. `responsive-checks.json` records 18 measurements across nine viewport widths and both themes. Layout width excludes the browser scrollbar.

Production uses only optimized responsive WebP files in `landing/static/assets/art/`, declared in the manifest. The large PNG masters and browser evidence are not shipped in the website output. Hero sizes: 640, 1280, 1920 and 2560 px, 18–95 KB. Scenes/decorations: 320 and 640 px, at most 35 KB. Use `srcset`, explicit dimensions and decorative empty alt text when meaningful adjacent copy already explains the scene.

The phone is HTML/CSS around an untouched authentic Android capture, separate from the generated landscape. Its real provenance remains in the existing app-asset manifest. Source/date/store availability copy stays in HTML.

## Card generation intent

A cohesive family of polished minimal isolated scenes, transparent backgrounds, no text/brand names, uncluttered silhouettes and one dominant accent per illustration:

- Consult: blue compact car, small station and location pin.
- Check: green driver checking a pump/card, checkmark and clock.
- Contribute: orange person photographing a fuel pump with a phone.
- Private driver: violet person holding car keys beside a compact car.
- Ride driver: teal cockpit, driver and route pins with a fuel symbol.
- Small fleet: rose accent, three white vans and a simple cost chart.

## Delivery boundary

Preserves the OpenCode landing-refresh 41d1372 changes to gutters, larger branding, removed hero facts and 404 recovery header. This local worktree builds the preview on port 4175. Primary dev's security work and the unrelated existing hero prompt edit are excluded. No remote integration, hosting deployment or release readiness is claimed.
