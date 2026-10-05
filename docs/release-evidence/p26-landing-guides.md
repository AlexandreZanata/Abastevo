# P26 Release Evidence — Landing Educational Guides

- **Milestone:** 20 — `P26 — Landing educational guides`
- **Gate:** G26-GUIDES-READY (local acceptance only)
- **Date:** 2026-10-05
- **Branch:** `codex/phase-26-landing-guides`
- **PR:** #117 (draft, merge OWED at conjunto closure)
- **Status:** LOCAL_DONE T01–T05; INTEGRATED/merge/wiki OWED at final conjunto closure per user directive.

## 1. Scope

Evergreen educational guides under `/guias/` (contract: `docs/product/LANDING_GUIDES_CONTRACT.md`):

| Path | Title (chars) | Description (chars) |
|---|---|---|
| `/guias/` | Guias Educativos sobre Combustíveis \| abastevo (46) | 149 |
| `/guias/pesquisa-anp/` | Como Funciona a Pesquisa da ANP — abastevo (42) | 157 |
| `/guias/etanol-ou-gasolina/` | Etanol ou Gasolina: Cálculo dos 70% — abastevo (46) | 156 |
| `/guias/como-ler-precos/` | Como Ler Preços de Combustíveis na Bomba — abastevo (51) | 152 |

Implemented titles paraphrase the contract wording but stay within the strict 30–65 / 110–165 bounds; recorded, not silently rewritten. B-BR-L01 truthfulness (no guaranteed-cheapest/magic-saving claims, ANP vs community separated, explicit non-affiliation), B-BR-L04 static without-JS readability, zero runtime deps beyond existing fingerprinted CSS/JS.

## 2. Task commits (pushed, merge pending)

- `6556a99` docs(p26): guides content contract (#112)
- `a84f5cb` feat(p26): evergreen guides + hub + build wiring (#113)
- `c304400` feat(p26): SEO/JSON-LD/sitemap/discovery (#114)
- `33cc11f` test(p26): guides regression suites + gate verification (#115)
- T05: this evidence file + PROGRESS update (merge/wiki still owed).

Pre-existing working-tree guide CSS tokens and legal/404 Guias navigation consistency were preserved (not discarded); EOF whitespace fixed for `git diff --check`.

## 3. Local validation (head `33cc11f`)

- `npm --prefix landing run build` — PASS (dist, CSS `styles.4056271c.css`, JS `main.72d6a45f.js`, prelaunch).
- `npm --prefix landing run check` — PASS (dist valid).
- `npm --prefix landing test` — 24/24 PASS (21 existing + 3 new `guides-content.test.mjs` suites: content/breadcrumbs/truthfulness, schemas/sitemap/llms, budgets).
- `bash scripts/tests/test-gate-selection.sh` — 15/15 PASS.
- `bash scripts/quick-verify.sh` — ok (selection=full, 10s of 300s budget).
- `git diff --check` — PASS. `bash scripts/scan-secrets.sh` — PASS.
- Sitemap dist: 8 URLs including all 4 guide URLs, no `lastmod`, no 404. `llms.txt`: 4 guide refs.
- JSON-LD: hub `CollectionPage` + `BreadcrumbList`; guides `TechArticle` + `BreadcrumbList`, unique `@id`s, no ratings/reviews personas.
- Budgets: CSS gzip ≤25 KiB, JS gzip ≤10 KiB, each guide page ≤500 KiB.

## 4. Backend de testes (externo, bloqueado sem bypass)

- `https://teste.abastevo.com.br`: TLS chain via Fortinet middlebox CA `FG6H0FTB23902129`; `curl`/`openssl` report `unable to get local issuer certificate` (exit 60). No `-k`/trust-all bypass applied.
- Live contract/fluxos seguem UNVERIFIED nesta fase (landing estática não consome o backend); verificação HTTPS/contratos/fluxos integrados pertence ao trilho Android/VPS (P34–P38 já LOCAL_DONE nos branches próprios; P32/P33 pendentes) e ao fechamento do conjunto.

## 5. Owed at conjunto closure (not done here)

Manual end batch, required CI (`Quick verification` on current head), cumulative PR review, guarded merge preserving commits, merged-SHA wiki sync. Draft PR #117 stays draft. iOS remains archived (`DEFERRED_EXPLICIT_RESUME_ONLY`). G09 stays UNCERTIFIED; no production deployment/tag/public pilot authorized by this evidence.
