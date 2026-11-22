import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { loadSite, validateFrontMatter, htmlTitle, isValidDate, normalizeQuery, relatedGuides, ContentError } from '../scripts/content/site.mjs';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');

const validGuide = () => ({
  title: 'Como é formado o preço da gasolina no Brasil',
  description: 'As parcelas do preço da gasolina: valor do produtor ou importador, tributos federais, ICMS por litro, etanol anidro e margens de revenda.',
  slug: 'guia-teste',
  category: 'precos',
  tags: ['gasolina', 'impostos'],
  datePublished: '2026-11-14',
  dateModified: '2026-11-14',
  related: ['/precos/'],
});
const ctx = { categories: new Set(['precos']), expectedSlug: 'guia-teste' };

// Copies landing/content into a throwaway project root so a fixture can be
// broken. The app-capture provenance and original tutorial sources come along
// so the Markdown images and localized tutorial steps still resolve.
function withContent(mutate) {
  const base = fs.mkdtempSync(path.join(os.tmpdir(), 'abastevo-content-'));
  const tmp = path.join(base, 'landing');
  fs.mkdirSync(tmp);
  fs.cpSync(path.join(root, 'content'), path.join(tmp, 'content'), { recursive: true });
  fs.mkdirSync(path.join(base, 'contracts'));
  fs.cpSync(path.join(root, '..', 'contracts', 'i18n'), path.join(base, 'contracts', 'i18n'), { recursive: true });
  const provenanceRel = path.join('static', 'assets', 'app', 'provenance.json');
  fs.mkdirSync(path.dirname(path.join(tmp, provenanceRel)), { recursive: true });
  fs.copyFileSync(path.join(root, provenanceRel), path.join(tmp, provenanceRel));
  const tutorials = path.join('static', 'como-usar');
  fs.cpSync(path.join(root, tutorials), path.join(tmp, tutorials), { recursive: true });
  try {
    mutate(path.join(tmp, 'content'));
    return () => loadSite(tmp);
  } finally {
    setImmediate(() => fs.rmSync(base, { recursive: true, force: true }));
  }
}

function editFile(file, from, to) {
  const text = fs.readFileSync(file, 'utf8');
  assert.ok(text.includes(from), `fixture text not found in ${file}: ${from}`);
  fs.writeFileSync(file, text.replace(from, to));
}

test('helpers: date validation and <title> composition', () => {
  assert.ok(isValidDate('2026-11-14'));
  assert.ok(!isValidDate('2026-02-30'));
  assert.ok(!isValidDate('14/11/2026'));
  assert.equal(htmlTitle('Seus direitos no posto de combustível'), 'Seus direitos no posto de combustível | Abastevo');
  const long = 'ICMS sobre combustíveis: como funciona o valor por litro';
  assert.equal(htmlTitle(long), long, 'suffix dropped when it would exceed 65 characters');
});

test('front matter validation accepts a complete guide', () => {
  assert.deepEqual(validateFrontMatter('guide', validGuide(), 'g.md', ctx), []);
});

test('front matter validation rejects missing, unknown and malformed fields', () => {
  const cases = [
    [(g) => delete g.title, /missing required field "title"/],
    [(g) => delete g.related, /missing required field "related"/],
    [(g) => { g.author = 'Fulano'; }, /unknown field "author"/],
    [(g) => { g.description = 'Curta demais.'; }, /description length/],
    [(g) => { g.title = 'Curto'; }, /title \(as <title>\) length/],
    [(g) => { g.slug = 'Guia_Teste'; }, /invalid slug/],
    [(g) => { g.slug = 'outro-nome'; }, /must match file name/],
    [(g) => { g.category = 'inexistente'; }, /unknown category/],
    [(g) => { g.tags = []; }, /tags must list 1 to 8/],
    [(g) => { g.tags = ['Com Espaço']; }, /invalid tag/],
    [(g) => { g.datePublished = '2026-13-01'; }, /datePublished must be a real/],
    [(g) => { g.dateModified = '2026-11-01'; }, /dateModified precedes datePublished/],
    [(g) => { g.related = []; }, /related must list at least one route/],
    [(g) => { g.related = ['https://example.com/']; }, /invalid related route/],
    [(g) => { g.primaryQuery = ''; }, /primaryQuery must be a non-empty string/],
    [(g) => { g.primaryQuery = 'x'.repeat(81); }, /primaryQuery length 81 outside \[3, 80\]/],
  ];
  for (const [mutate, pattern] of cases) {
    const guide = validGuide();
    mutate(guide);
    const errors = validateFrontMatter('guide', guide, 'g.md', ctx);
    assert.ok(errors.some((e) => pattern.test(e)), `expected ${pattern} in ${JSON.stringify(errors)}`);
  }
});

test('the shipped content manifest and Markdown sources validate', () => {
  const site = loadSite(root);
  const count = (entries) => entries.reduce((acc, e) => ({ ...acc, [e.kind]: (acc[e.kind] || 0) + 1 }), {});
  const kinds = count([...site.registry.values()].filter((e) => !e.locale));
  // Each locale ships editorial, legal and tutorial pages alongside its calculators.
  for (const code of ['en', 'es']) {
    const localized = count([...site.registry.values()].filter((e) => e.locale === code && !e.hub));
    assert.deepEqual(localized, { 'tutorial-hub': 1, tutorial: 16, guide: 13, tool: 5, page: 5 }, `${code} translations`);
  }
  assert.equal(kinds.tool, 5);
  assert.equal(kinds.page, 2);
  assert.equal(kinds.category, 5);
  assert.equal(kinds['html-guide'], 3);
  assert.ok(kinds.guide >= 10);
  const routes = [...site.registry.keys()];
  assert.equal(new Set(routes).size, routes.length, 'routes are unique');
  for (const entry of site.registry.values()) {
    if (entry.indexable) assert.ok(isValidDate(entry.lastmod), `${entry.route} has a fixed lastmod`);
  }
  assert.equal(site.registry.get('/404.html').indexable, false);

  // SEO-20: every guide and tool answers one query, and no two share it.
  // The map is scoped by locale, so a translation may answer the same query
  // in its own language; here the pt-BR pages are counted.
  const keywordPages = [...site.registry.values()].filter((entry) => !entry.locale && (entry.kind === 'guide' || entry.kind === 'html-guide' || entry.kind === 'tool'));
  assert.equal(keywordPages.length, 18, '13 guides and 5 tools carry the keyword map');
  for (const code of ['en', 'es']) {
    const localized = [...site.registry.values()].filter((entry) => entry.locale === code && ['guide', 'tool'].includes(entry.kind));
    const queries = localized.map((entry) => normalizeQuery(entry.primaryQuery || ''));
    assert.ok(queries.every(Boolean), `every ${code} guide and tool names its query`);
    assert.equal(new Set(queries).size, queries.length, `${code} queries are unique`);
  }
  for (const entry of keywordPages) {
    assert.equal(typeof entry.primaryQuery, 'string', `${entry.route} needs a primaryQuery`);
  }
  const normalized = keywordPages.map((entry) => normalizeQuery(entry.primaryQuery));
  assert.equal(new Set(normalized).size, normalized.length, 'primary queries are unique');
  assert.ok(normalized.every((query) => query === query.toLowerCase()), 'primary queries are lowercase');
});

test('loadSite rejects broken fixtures with a precise message', () => {
  const cases = [
    [
      (dir) => editFile(path.join(dir, 'ferramentas', 'consumo-medio.md'), '](/ferramentas/etanol-ou-gasolina/)', '](/guias/nao-existe/)'),
      /link \/guias\/nao-existe\/ is not a landing page/,
    ],
    [
      (dir) => editFile(path.join(dir, 'ferramentas', 'consumo-medio.md'), '  - /precos/\n', '  - /precos/zz/\n'),
      /related route \/precos\/zz\/ is not a landing page or a known portal route/,
    ],
    [
      (dir) => editFile(path.join(dir, 'paginas', 'sobre.md'), 'slug: sobre', 'slug: quem-somos'),
      /must match file name/,
    ],
    [
      (dir) => editFile(path.join(dir, 'ferramentas', 'consumo-medio.md'), '/assets/app/vehicles-06-fuel-log-720w.webp', '/assets/app/contribute-price-05-phone-camera-720w.webp'),
      /App capture must be a 720w derivative listed in the asset provenance/,
    ],
    [
      (dir) => editFile(path.join(dir, 'paginas', 'sobre.md'), '## Independência', '# Independência'),
      /Level-1 headings are reserved/,
    ],
    [
      (dir) => editFile(path.join(dir, 'paginas', 'sobre.md'), '[repositório do abastevo no GitHub]', '[código](javascript:alert(1)) [repositório do abastevo no GitHub]'),
      /Unsafe link URL/,
    ],
    [
      (dir) => fs.rmSync(path.join(dir, 'ferramentas', 'custo-por-km.md')),
      /calculator "custo-por-km" has no content\/ferramentas\/custo-por-km.md/,
    ],
    [
      (dir) => fs.copyFileSync(path.join(dir, 'ferramentas', 'consumo-medio.md'), path.join(dir, 'ferramentas', 'outra.md')),
      /slug "consumo-medio" must match file name "outra"/,
    ],
    [
      (dir) => editFile(path.join(dir, 'ferramentas', 'consumo-medio.md'), 'primaryQuery: calculadora consumo médio', 'primaryQuery: calculadora custo de viagem'),
      /primaryQuery "calculadora custo de viagem" collides between \/ferramentas\/consumo-medio\/ and \/ferramentas\/custo-de-viagem\//,
    ],
    [
      (dir) => editFile(path.join(dir, 'manifest.json'), '"route": "/termos/", ', '"route": "/privacidade/", '),
      /duplicate route \/privacidade\//,
    ],
    [
      (dir) => editFile(path.join(dir, 'manifest.json'), '"lastmod": "2026-11-17" },\n    { "route": "/como-usar/escolher-cidade/"', '"lastmod": "ontem" },\n    { "route": "/como-usar/escolher-cidade/"'),
      /indexable pages need a fixed lastmod date/,
    ],
  ];
  for (const [mutate, pattern] of cases) {
    const load = withContent(mutate);
    assert.throws(load, (err) => err instanceof ContentError && pattern.test(err.message), String(pattern));
  }
});

test('"Leia também" ranks by shared tags, then category, never itself', () => {
  const site = loadSite(root);
  const etanol = site.registry.get('/guias/etanol-ou-gasolina/');
  const picks = relatedGuides(site, etanol);
  assert.equal(picks.length, Math.min(3, site.guides.length - 1));
  assert.ok(!picks.includes(etanol));
  const shared = (g) => g.tags.filter((t) => etanol.tags.includes(t)).length;
  for (let i = 1; i < picks.length; i++) {
    const score = (g) => shared(g) * 3 + (g.category === etanol.category ? 2 : 0);
    assert.ok(score(picks[i - 1]) >= score(picks[i]), 'ordered by score');
  }
});
