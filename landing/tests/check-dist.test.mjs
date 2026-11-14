import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { build } from '../scripts/build.mjs';
import { checkDist, routeForHtml } from '../scripts/check.mjs';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const pristine = fs.mkdtempSync(path.join(os.tmpdir(), 'abastevo-check-'));
build({ projectRoot: root, outDir: pristine });
test.after(() => fs.rmSync(pristine, { recursive: true, force: true }));

function mutated(edit) {
  const copy = fs.mkdtempSync(path.join(os.tmpdir(), 'abastevo-check-copy-'));
  fs.cpSync(pristine, copy, { recursive: true });
  edit(copy);
  return copy;
}

function patch(dir, rel, from, to) {
  const file = path.join(dir, rel);
  const text = fs.readFileSync(file, 'utf8');
  assert.ok(text.includes(from), `fixture text missing in ${rel}`);
  fs.writeFileSync(file, text.replace(from, to));
}

function expectFailure(edit, pattern) {
  const dir = mutated(edit);
  try {
    assert.throws(() => checkDist({ distDir: dir }), pattern);
  } finally {
    fs.rmSync(dir, { recursive: true, force: true });
  }
}

test('routeForHtml maps files to URL paths', () => {
  assert.equal(routeForHtml('index.html'), '/');
  assert.equal(routeForHtml('guias/index.html'), '/guias/');
  assert.equal(routeForHtml('404.html'), '/404.html');
});

test('a fresh build passes the extended check and reports counts', () => {
  const result = checkDist({ distDir: pristine });
  assert.ok(result.ok);
  assert.ok(result.pages >= 30, `pages ${result.pages}`);
  assert.equal(result.sitemapUrls, result.pages - 1, 'every page except 404 is in the sitemap');
  assert.match(result.toolsFile, /^tools\.[a-f0-9]{8}\.js$/);
  assert.ok(result.totalBytes > 0);
});

test('check rejects duplicate titles and descriptions', () => {
  const title = fs.readFileSync(path.join(pristine, 'sobre/index.html'), 'utf8').match(/<title>([^<]+)<\/title>/)[1];
  expectFailure((dir) => {
    const current = fs.readFileSync(path.join(dir, 'politica-editorial/index.html'), 'utf8').match(/<title>([^<]+)<\/title>/)[1];
    patch(dir, 'politica-editorial/index.html', `<title>${current}</title>`, `<title>${title}</title>`);
  }, /duplicates the <title>/);
  const desc = fs.readFileSync(path.join(pristine, 'sobre/index.html'), 'utf8').match(/<meta name="description" content="([^"]+)"/)[1];
  expectFailure((dir) => {
    const current = fs.readFileSync(path.join(dir, 'ferramentas/index.html'), 'utf8').match(/<meta name="description" content="([^"]+)"/)[1];
    patch(dir, 'ferramentas/index.html', `<meta name="description" content="${current}"`, `<meta name="description" content="${desc}"`);
  }, /duplicates the meta description/);
});

test('check rejects canonical drift, other origins and broken JSON-LD', () => {
  expectFailure((dir) => patch(dir, 'sobre/index.html', '<link rel="canonical" href="https://abastevo.com.br/sobre/">', '<link rel="canonical" href="https://abastevo.com.br/">'), /is not the self URL/);
  expectFailure((dir) => patch(dir, 'sobre/index.html', 'href="https://abastevo.com.br/sobre/">', 'href="https://teste.abastevo.com.br/sobre/">'), /is not the self URL/);
  expectFailure((dir) => patch(dir, 'ferramentas/index.html', '"@context": "https://schema.org",', '"@context": "https://schema.org",,'), /malformed JSON-LD/);
  assert.throws(() => checkDist({ distDir: pristine, origin: 'https://example.org' }), /is not the self URL https:\/\/example\.org/);
});

test('check rejects broken internal links but allowlists portal routes', () => {
  expectFailure((dir) => patch(dir, 'guias/index.html', 'href="/guias/dados/"', 'href="/guias/inexistente/"'), /nonexistent local resource: \/guias\/inexistente\//);
  expectFailure((dir) => patch(dir, 'sobre/index.html', 'href="/politica-editorial/"', 'href="/#nao-existe"'), /missing home anchor \/#nao-existe/);
  const dir = mutated((d) => patch(d, 'sobre/index.html', 'href="/precos/"', 'href="/postos/qualquer/"'));
  try {
    assert.ok(checkDist({ distDir: dir }).ok, 'portal paths are external to the landing');
  } finally {
    fs.rmSync(dir, { recursive: true, force: true });
  }
});

test('check rejects sitemap drift and inline scripts', () => {
  expectFailure((dir) => patch(dir, 'sitemap.xml', /<url>\s*<loc>https:\/\/abastevo\.com\.br\/sobre\/<\/loc>\s*<lastmod>[^<]+<\/lastmod>\s*<\/url>\s*/.exec(fs.readFileSync(path.join(dir, 'sitemap.xml'), 'utf8'))[0], ''), /\/sobre\/ is missing from sitemap\.xml/);
  expectFailure((dir) => patch(dir, 'sitemap.xml', '<lastmod>2026-11-14</lastmod>', '<lastmod>ontem</lastmod>'), /needs a YYYY-MM-DD lastmod/);
  expectFailure((dir) => patch(dir, 'sitemap.xml', '</urlset>', '  <url>\n    <loc>https://abastevo.com.br/fantasma/</loc>\n    <lastmod>2026-11-14</lastmod>\n  </url>\n</urlset>'), /not an emitted indexable landing page/);
  expectFailure((dir) => patch(dir, 'sobre/index.html', '</body>', '<script>alert(1)</script></body>'), /inline or non-fingerprinted script/);
});
