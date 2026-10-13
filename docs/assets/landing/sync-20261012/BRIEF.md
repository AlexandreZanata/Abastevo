# Landing synchronization — LH04

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING
Base: codex/landing-hero-art 5360f8b, isolated from occupied primary dev.
Scope: landing copy, branded actions and responsive header; local preview only.

## Authority and behavior

The user's 2026-10-12 instruction adopts the new buttons, GitHub icons and text
from localhost:8099. Its read-only source is landing-refresh df2b992. The LH04
amendment in the canonical landing content contract records this before code.
The headline, introductory paragraph, two-line GitHub/Play actions and four
hero facts now match that reference. Existing authentic Android/Brazil artwork,
six illustrations, source badges and neutral dark theme remain in place.

All 25 script-enabled pages share the branded header and native navigation
disclosure. Links close the menu; Escape closes it. GitHub remains a labeled
external link with noopener. Play is a non-link prelaunch status; only the
validated published configuration creates a store link. Narrow headers retain
theme and navigation controls, with the full GitHub action in the hero.

GitHub and Google Play vectors reuse the reference's recorded SimpleIcons CC0
assets; provenance is in landing/static/assets/brand/icon-provenance.json.
There are no new dependencies. Explicit artwork stacking keeps the map visible
behind the real phone when the adopted headline changes the desktop row height.

## Validation

- TypeScript typecheck, production build and landing output check: PASS.
- `npm --prefix landing test`: 44/44 PASS, including prelaunch/published states,
  all-page header/navigation checks, authentic artwork and emitted theme logic.
- Final CSS: styles.b5b44ae8.css; JS: main.37b29769.js; store state: prelaunch.
- Browser widths 320, 375, 390, 401, 768, 1024 and 1440: zero horizontal overflow,
  header controls inside content padding, phone contained, badges outside phone,
  exactly one theme action icon. [Measurements](responsive.json).
- Desktop copy and stage have identical height at 1024 and 1440 (294.546875px).
- Actual menu opening and link navigation/closure verified on mobile; Escape
  closure verified on desktop. Console warnings/errors: none.
- [Desktop dark](desktop-dark.jpg), [desktop light](desktop-light.jpg),
  [mobile light](mobile-light.jpg). Screenshots are browser previews.
- `git diff --check`, repository secret scan and staged text-surface review: PASS.

## Delivery

Primary dev's active security changes and reference worktree are untouched.
An unrelated pre-existing edit to hero-20261011/prompts/check.txt is preserved
and excluded from this checkpoint. Integrate the two landing commits into dev
after its active work is saved, then use the protected final batch workflow.
No push, PR, merge, hosting or release acceptance is claimed.
