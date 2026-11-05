# P25-T03 legal page presentation — 2026-11-05

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

The user requested the same header, footer and visual standards as the home
for `/privacidade/`, `/excluir-conta/` and `/termos/`. The isolated maintained
`dev` checkout was fast-forwarded to `origin/main` at `c7f9bd3b`, the merge
of PR #188, before implementation. The occupied primary checkout was untouched.
Tested revision: the source files included in this commit.

## Contract and implementation

[B-BR-L07-UI01/02 and BUC-L07-UI01](../product/LANDING_CONTENT_CONTRACT.md#legal-pages-shared-ui--2026-11-05)
were recorded before implementation. This is a presentation-only follow-up
to P25-T03, with standard risk and no new dependency or runtime service.

The static build now inserts the canonical home header and footer into the
three legal routes. Home-section links are qualified through `/`, matching
the existing learning routes. The pages use the same home shell, tokens,
Manrope headings and Source Sans 3 prose. Shared article rules retain the
learning pages' readable width and responsive heading scale. Breadcrumbs
identify the current legal page. The existing native mobile menu is retained.

The legal article and complete head metadata of each page were compared
byte-for-byte with the parent commit and are unchanged. No account deletion,
privacy retention or moderation behavior was modified.

## Verification

- Extended the existing chrome-parity contract to the three legal routes.
  Before implementation it failed on the privacy header, reproducing the
  visual divergence. After implementation it passes for all legal/learning
  routes, including home anchor destinations.
- `npm --prefix landing run typecheck`, `run build`, `run check` and `test`
  passed. All 76 tests passed without skips. Existing static navigation,
  SEO/JSON-LD, no-JavaScript content, store-state and reproducibility checks
  remain active. Emitted CSS is `styles.bcc00d0c.css`.
- Browser checks against the built local artifact at 320, 390 and 1440 CSS
  pixels covered all three routes. Document scroll width equalled client
  width in all nine cases. Header/footer backgrounds are black; headings
  use Manrope and prose uses Source Sans 3. Each page has one H1.
- Mobile footer links opened account deletion and terms. The mobile menu
  opened and closed with Escape. The desktop footer displays the same four
  navigation groups as the home.
- `git diff --check` and `bash scripts/scan-secrets.sh` passed. Scoped review
  found no new credentials, personal data or changes to backend/app sources.

## Delivery and recovery

Publish through a protected `dev` → `main` PR. Required current-head checks,
merge and the automatic staging deployment are separate from this local
validation. The live staging pages still used the legacy chrome when inspected
at task opening. No remote deployment is claimed by this record.

Rollback is a normal revert of this bounded presentation commit, followed by
the existing landing build/check and protected delivery workflow.
