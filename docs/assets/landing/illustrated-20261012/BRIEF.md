# LH05 — full-viewport illustrated landing

Status: LOCAL_DONE / INTEGRATION_PENDING. Branch `codex/landing-hero-art`; construction base `c126127`. Scope is the user-authorized landing presentation and reusable art only.

## Result

A cream/teal/green flat editorial landscape now spans the entire opening section, outside the content container. Desktop opening fills the initial viewport; mobile stacks readable text above the real phone and landscape. The untouched Android home remains separate from the generated scene. ANP and community badges remain below the phone, with truthful source/date and development status. The latest instruction supersedes the old equal art/copy-height rule.

The six cards use unique-color responsive scenes without embedded text. Four additional transparent flat-style decorations are available for reuse. The largest production landscape is 95,000 bytes; card images at 640 px are 20,408–34,394 bytes. PNG masters stay outside the production output. The 3840×2160 PNG is an enlarged export of native 1672×941 generation, explicitly recorded in the manifest and README.

OpenCode reference `landing-refresh` at `41d1372` was read without modification. Its 12 px gutters, larger desktop brand, hero-fact removal and 404 header are retained. User-facing headline and CTA were updated under the LH05 canonical contract. Earlier branded GitHub/Play assets, exclusive sun/moon controls and neutral dark UI remain intact. No dependencies added.

## Validation

- `npm --prefix landing run typecheck`: PASS.
- `npm --prefix landing run build`: PASS, prelaunch state.
- `npm --prefix landing run check`: PASS.
- `npm --prefix landing test`: 45/45 PASS; existing progressive-enhancement, store-state and content tests retained.
- Final CSS `styles.41a6f810.css`, JS `main.37b29769.js`.
- Browser widths 320, 375, 390, 401, 768, 1024, 1280, 1440 and 1920 in both themes: no horizontal overflow; header contained; background edge to edge; phone contained; source badges outside phone; exactly one visible theme icon. Desktop hero reaches the first viewport bottom. [Measurements](responsive-checks.json).
- Browser console warnings/errors: none. [Desktop light](desktop-light.jpg), [desktop dark](desktop-dark.jpg), [mobile light](mobile-light.jpg), [mobile dark](mobile-dark.jpg), [six scenes](six-scenes.jpg).
- `git diff --check`, repository secret scan and staged text-surface review: PASS.

## Next action

Integrate this checkpoint and the two preceding landing commits into maintained dev after the concurrent security work is saved, then follow protected batch delivery. Primary dev and the reference checkout remain untouched. The pre-existing edit in `hero-20261011/prompts/check.txt` is preserved and excluded. No push, PR, merge, hosting or release acceptance is claimed.
