# LR00 — Landing redesign branch reconciliation

- Task: LR00 (reconcile branch and actual prior work)
- Date: 2026-10-11
- Branch: `landing` at `20bb603` (in sync with `origin/landing`)
- Status: LOCAL_DONE (docs audit only; no website/backend behavior change)

## Refs at audit time

| Ref | SHA | Note |
|---|---|---|
| `landing` (this worktree) | `20bb603` | clean tree, in sync with `origin/landing` |
| `origin/landing` | `20bb603` | no push needed |
| `dev` / `origin/dev` | `a853708` | in sync; occupied primary checkout, uncommitted CS edits untouched |
| `main` / `origin/main` | `a349d7c` | in sync |

- `landing` vs `dev`: 3 ahead (plan/fonts/LR00 commits), 7 behind (CS/backend chain). No backend files touched on `landing`.
- `landing` vs `main`: 20 ahead, 0 behind (17 inherited at opening + 3 preparation/reconciliation commits).
- Worktree: `.worktrees/landing-refresh` on `landing`; primary checkout stays on `dev` and was never switched, stashed or reset.

## Preserved prior work (no duplicate foundation)

- P25 static landing acceptance: `docs/release-evidence/p25-static-landing.md` present.
- P26 guides acceptance: `docs/release-evidence/p26-landing-guides.md` present.
- Preparation commits on `landing`: `61453e2` (redesign plan, prompt, 30-chapter catalog, authentic reference/media, local Manrope/Source Sans 3 WOFF2 with OFL/provenance/hash-copy regression, content-contract amendment, README/ROADMAP/PROGRESS notes), `1e353fd` (font-license whitespace) and `20bb603` (this LR00 reconciliation).
- Prepared artifacts verified present: `landing/static/assets/fonts/` (4 hashed WOFF2 + OFL notices + provenance), `landing/tests/local-fonts.test.mjs`, `docs/assets/landing/redesign-20261011/` (reference HTML/PNGs, manifest, 8 native captures, preparation evidence), `docs/product/LANDING_MOBILE_TUTORIAL_CATALOG.md`.

## Gates and limits

- Required `Quick verification` gate preserved; gate-selection tests cover `landing/`.
- No runtime suite for this audit per plan (docs-only). Artifact reconciliation re-verified now: 8/8 manifest SHA-256 + 1220x2712 dimensions OK, 4/4 font hashes + WOFF2 signatures + OFL notices OK, TU01–TU30 catalog present, reference v2 HTML + desktop/mobile PNGs present, P25/P26 evidence preserved. Preparation-reported build figures (typecheck/build/check green, 25 landing tests, gate selection 18/18) recorded as reported, not re-executed here.
- Validation: `git status --short --branch` clean, `git diff --stat` empty, `git log -5 --oneline`, `git worktree list`, `git diff --check` clean, `bash scripts/scan-secrets.sh` passed. No image-generation call spent; superseded AI concept stays excluded.
- No deployment, DNS/CDN/VPS, store listing, URL submission, issue/PR/merge/wiki, or production/Android acceptance follows from LR00.

## Next

LR01 — freeze actual capability and claim inventory (depends on LR00).
