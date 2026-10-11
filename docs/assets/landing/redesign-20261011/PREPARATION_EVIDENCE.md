# Local preparation evidence — 2026-10-11

Branch: `landing`; isolated worktree `.worktrees/landing-refresh`.
Opening source: `954a8d800576fec24acc9edfd3b93aad2ac15450`. Evidence covers the
local font/preparation diff, not a remote published revision. Primary dev was
occupied by unrelated community work and was not changed by this task.

## Implemented and verified

- Local variable Manrope + Source Sans 3, four content-hash WOFF2 files and two
  original OFL notices, recorded official HTTPS provenance and bounded build copy.
- Meaningful font regression observed RED (expected four emitted font references,
  got zero), then GREEN after CSS/build integration. Existing tests retained.
- `npm --prefix landing run typecheck`: PASS.
- `npm --prefix landing run build`: PASS; after the final CSS provenance-comment
  correction, emitted CSS `styles.c083a948.css`, JS `main.72d6a45f.js`.
- `npm --prefix landing run check`: PASS, including final artifact.
- `npm --prefix landing test`: 25/25 PASS after font behavior integration. Later
  source edits were documentation, reference tooling and a CSS comment only;
  build/check and actual browser font loading were reverified, without repeating
  an unchanged aggregate suite.
- `bash scripts/tests/test-gate-selection.sh`: 18/18 PASS in the permitted execution
  environment. An initial sandbox-limited invocation could not run subprocesses;
  that attempt was not accepted as test evidence.
- FontTools inspection: valid variable Manrope and Source Sans 3 axes; required
  Portuguese accented glyphs available. Four WOFF2 files total 128,916 bytes;
  actual browser on the Portuguese page fetched the two Latin files (53,708
  source bytes, excluding HTTP overhead). Performance follow-up must include
  font bytes; the existing checker is not a full Core Web Vitals measurement.
- Actual HTTP `landing/dist` browser: both requested font families loaded from
  local 200 responses, correct computed body/H1 families, no CSS injection.
- Reference browser: original images decoded, Next/Previous changed between the
  genuine captures, no page errors or horizontal overflow at 320/375/768/1024/
  1440px, desktop/mobile DPR2 renders inspected. This is bounded prototype QA,
  not completed LR32 accessibility or full tutorial acceptance.
- Eight original PNG hashes and dimensions validated; every accepted capture
  visually inspected for private data. Rejected music-overlay/history attempts
  were not included. Original dark theme restored and confirmed through the
  `Ativar tema claro` control; only the created temporary device UI XML removed.
- LR00–LR39 and TU01–TU30 IDs verified, new document links resolve, diff check and
  repository secret scan pass. Scoped new documentation/manifest/HTML secret
  surface separately reviewed because the repository scanner excludes docs.

## Still pending

Production redesign, complete tutorials, populated/private/conditional lesson
captures, device/source SHA proof, owning app acceptance, LR performance/accessibility
exit, public domain/server responses, webmaster ownership and indexation evidence.
No backend/mobile runtime code, storage reset, account/data submission, app font
change, deployment, external URL submission, issue/PR/merge or wiki publication.
SEO research is a dated design input, not proof of indexation or ranking.

Next action: LR00 reconciles these prepared artifacts and actual branch state,
then proceeds with the next authorized bounded phase. Recheck the installed app
and changing official SEO documentation at implementation time.
