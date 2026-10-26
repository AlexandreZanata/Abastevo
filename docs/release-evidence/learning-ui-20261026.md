# LH10 — shared learning UI, 2026-10-26

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

## Scope and source

Owner requested the existing home/consultation UI on Guias and Como usar,
including header, footer and controls, and deployment to
`https://teste.abastevo.com.br`. B-BR-LH10-01/02 and BUC-LH10-01 are in the
[content contract](../product/LANDING_CONTENT_CONTRACT.md#lh10-learning-pages-shared-ui--2026-10-26).

The isolated worktree starts at `9e6da1ae18d8da50a58f180cd5192efa696af535`,
the existing website PR #155 candidate. The independent maintained-dev map
PR #154 and occupied original checkout are preserved. This bounded change
continues the existing website delivery; no duplicate bookkeeping PR.

## Implementation

- Build the 21 learning pages from the canonical home header/footer, using
  root-qualified home anchors from nested routes. Remove duplicated legacy
  chrome from source pages; fail the build on a missing shared slot.
- Reuse the actual home CSS tokens, fonts, black navigation/footer, neutral
  surfaces, rounded cards and focus styles. Whole-card guide/journey links;
  two-column journey/chapter grids on desktop, one column on phones.
- Keep article text, formulas, source/date content, chapter captures, all
  30 chapter entries and availability states. Preserve SEO and no-JS links.
- Keep the existing CSS asset and all non-learning output in the deployed
  release. Only 21 HTML files and one new content-hashed CSS file change.

## Validation

- `npm run typecheck`: PASS.
- `PREVIEW=1 npm run build -- --origin https://teste.abastevo.com.br`: PASS.
- `npm run check`: PASS.
- `npm test`: 68/68 PASS, 0 failures/skips. Includes all learning routes'
  header/footer parity, home anchors, portal-link destinations, all chapter
  IDs/statuses, metadata, no-JS content, indexing and reproducible builds.
- Browser: desktop hubs, guide parity table and real-capture tutorial;
  375 px iframe view of both hubs and native mobile menu expansion: PASS.
  The narrow view is a browser viewport fixture, not a physical phone.
- `git diff --check` and staged secret scan recorded before commit.

## Publication plan and rollback

Confirmed web tier is `abastevo-temp-web` in `abastevo-temp`, on the existing
SSH host `sorrimobi-vps`. Landing root `/opt/abastevo-temp/web/landing`; current
release before change `releases/ab20fda8-f37c980d`. Copy that release, overlay
only the allowed learning HTML and new CSS, verify SHA-256 manifest, then
atomically switch `current`. Keep the old symlink target for immediate rollback.

Do not regenerate portal data, roll API/worker images, migrate, change
nginx/Caddy/DNS/security settings, or touch another namespace. Existing
noindex/robots/CSP remain. Verify each learning route and new asset over
real HTTPS, plus unchanged home/portal and ready web pod.

Publication: PENDING.
