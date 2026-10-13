# LH06 — fleet action, standard margins and Android device

Status: LOCAL_DONE / INTEGRATION_PENDING. Validation: PASS. Construction base `4e999aa`, isolated `codex/landing-hero-art`. Owner-authorized visual refinement only.

## Result and preservation

Only the small-fleet scene was regenerated with built-in image_gen: a rose-shirted owner actively marking a fuel clipboard beside two white vans. References, exact prompt and original output are in [generation.json](generation.json); native transparent PNG is [fleet-native.png](fleet-native.png). Responsive WebPs are 11,332 and 28,112 bytes at 320/640 px. All other five scene sources and derivatives, the landscape and reusable decorations retain their prior SHA-256 hashes. No embedded text, digits or brands.

Header and hero foreground now use the same 72rem maximum width and 12px gutters as other site sections. The landscape remains full bleed and the desktop opening fills the viewport. Android framing uses a small centered punch-hole camera, thin metal edge, narrow bezels and right-hand volume/power controls, with no iOS notch. This is a CSS hardware mockup around an untouched authentic Android screen. The hero now selects the existing 720px capture as its minimum instead of 360px; 1080px remains available for higher pixel density. No screenshot redraw, retouch or upscale; app provenance and hashes retained.

OpenCode brand sizing, navigation, GitHub/Play controls, copy and hero-fact removal are preserved. Source badges remain outside the screen. No new dependencies, backend, mobile-runtime, hosting or release changes.

## Validation

- Typecheck, production build and output check: PASS. CSS `styles.868a7bc3.css`, JS `main.37b29769.js`; store state prelaunch.
- `npm --prefix landing test`: 45/45 PASS, including source/derivative integrity, asset budgets, accessibility, theme and truthful store behavior.
- Browser nine widths (320, 375, 390, 401, 768, 1024, 1280, 1440, 1920), both themes: zero horizontal overflow; header/hero bounds match the standard section container; header controls contained; full-width background; phone contained; source badges outside phone; one visible theme icon. [Measurements](responsive.json).
- [Desktop light](desktop-light.jpg), [desktop dark](desktop-dark.jpg), [mobile light](mobile-light.jpg), [all six scenes](six-scenes.jpg). Console warnings/errors: none.
- Source/derivative SHA-256 verification, `git diff --check`, secret scan and staged text review: PASS.

## Next action

Integrate the four isolated landing checkpoints into maintained dev after concurrent security work is saved, then follow protected batch delivery. Primary dev and reference checkout are untouched; pre-existing `hero-20261011/prompts/check.txt` edit stays excluded. No push/PR/merge/publication is claimed.
