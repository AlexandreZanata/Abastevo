import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { loadSite, validateFrontMatter, htmlTitle, isValidDate, relatedGuides, ContentError } from '../scripts/content/site.mjs';

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

// Copies landing/content into a throwaway project root so a fixture can be broken.
function withContent(mutate) {
  const tmp = fs.mkdtempSync(path.join(os.tmpdir(), 'abastevo-content-'));
  fs.cpSync(path.join(root, 'content'), path.join(tmp, 'content'), { recursive: true });
  try {
    mutate(path.join(tmp, 'content'));
    return () => loadSite(tmp);
  } finally {
    setImmediate(() => fs.rmSync(tmp, { recursive: true, force: true }));
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
  assert.equal(htmlTitle('Seus direitos no posto de combustível'), 'Seus direitos no posto de combustível | abastevo');
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
  const kinds = [...site.registry.values()].reduce((acc, e) => ({ ...acc, [e.kind]: (acc[e.kind] || 0) + 1 }), {});
  assert.equal(kinds.tool, 5);
  assert.equal(kinds.page, 2);
  assert.equal(kinds.category, site.manifest.categories.length);
  assert.equal(kinds['html-guide'], 3);
  const routes = [...site.registry.keys()];
  assert.equal(new Set(routes).size, routes.length, 'routes are unique');
  for (const entry of site.registry.values()) {
    if (entry.indexable) assert.ok(isValidDate(entry.lastmod), `${entry.route} has a fixed lastmod`);
  }
  assert.equal(site.registry.get('/404.html').indexable, false);
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
      (dir) => editFile(path.join(dir, 'manifest.json'), '"route": "/termos/", ', '"route": "/privacidade/", '),
      /duplicate route \/privacidade\//,
    ],
    [
      (dir) => editFile(path.join(dir, 'manifest.json'), '"lastmod": "2026-10-26" },\n    { "route": "/como-usar/escolher-cidade/"', '"lastmod": "ontem" },\n    { "route": "/como-usar/escolher-cidade/"'),
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
