# LH07 — minimal product-led opening

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Construction base `e90edab`, isolated `codex/landing-hero-art`. Owner's latest request supersedes the illustrated-background presentation. B-BR/BUC authority is recorded in the LH07 amendment of the canonical landing content contract before implementation.

## Result

Removed the landscape image from the homepage and all four background derivatives from the runtime art manifest. Those derivatives no longer ship in dist; historical generated masters stay available for later reuse. No replacement bitmap was generated.

The opening now has a clean white/black neutral canvas, restrained circular CSS support behind the authentic Android device, a balanced two-column desktop grid, clear typography, generous spacing and consistent 72rem/12px site bounds. Mobile stacks the copy and Android preview. Source badges remain outside the phone; the source/date and development disclosures remain visible. The caption no longer refers to the removed landscape.

Header branding/navigation, existing GitHub/Play and application actions, headline/body copy, six card illustrations (including LH06 fleet action), reusable asset bytes and the real app screenshot are preserved. Obsolete hero landscape/mask/overlay rules were removed while consolidating the scoped opening CSS. Dark UI stays black/grayscale with white emphasis. No dependencies or new interactions added.

## Validation

- `npm --prefix landing run typecheck`, `run build`, `run check`: PASS. CSS `styles.4c8d9327.css`, JS `main.37b29769.js`, store state prelaunch.
- `npm --prefix landing test`: 45/45 PASS. Updated the existing artwork/content expectations to the owner's explicit landscape removal; source/date and all other progressive-enhancement checks remain.
- Runtime manifest contains zero hero-background derivatives; filesystem inspection confirms none in dist. Retained source and derivative entries match the prior committed manifest exactly.
- Browser nine widths (320, 375, 390, 401, 768, 1024, 1280, 1440, 1920) in light/dark: no horizontal overflow, standard header/hero margins, header controls contained, phone contained, badges outside phone, one visible theme icon, no hero background image and desktop opening reaches the initial viewport bottom. [Measurements](responsive.json).
- [Desktop light](desktop-light.jpg), [desktop dark](desktop-dark.jpg), [mobile light](mobile-light.jpg). Browser warnings/errors: none.
- `git diff --check`, repository secret scan and staged text review: PASS.

## Delivery

Primary dev's active evidence/security edit and reference checkout are untouched. The unrelated existing `hero-20261011/prompts/check.txt` change is preserved and excluded. Preview is local on port 4175; no push/PR/merge/deployment/release is claimed. Next: integrate the five isolated landing checkpoints after concurrent dev work is saved, then protected batch delivery.
