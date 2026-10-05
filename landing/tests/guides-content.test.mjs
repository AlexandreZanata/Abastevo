import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import zlib from 'node:zlib';
import { fileURLToPath } from 'node:url';
import { buildSite } from '../scripts/build.mjs';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const root = path.resolve(__dirname, '..');

const GUIDES = [
  { rel: 'guias/index.html', canonicalPath: '/guias/', h1: 'Guias Educativos abastevo' },
  { rel: 'guias/pesquisa-anp/index.html', canonicalPath: '/guias/pesquisa-anp/', h1: 'Como funciona a pesquisa semanal' },
  { rel: 'guias/etanol-ou-gasolina/index.html', canonicalPath: '/guias/etanol-ou-gasolina/', h1: 'Etanol ou gasolina' },
  { rel: 'guias/como-ler-precos/index.html', canonicalPath: '/guias/como-ler-precos/', h1: 'Como interpretar preços de combustíveis' },
];

const FORBIDDEN_CLAIMS = ['menor preço garantido', 'economia mágica', 'posto mais barato perto de você'];

test('guides content: pages exist with bounded titles, descriptions, breadcrumbs and truthful copy', () => {
  const tmpOut = path.join(root, '.test-guides-dist-' + Math.random().toString(36).slice(2, 8));
  try {
    buildSite({ projectRoot: root, outDir: tmpOut, canonicalOrigin: 'https://abastevo.com.br', storeState: 'prelaunch' });

    for (const guide of GUIDES) {
      const htmlPath = path.join(tmpOut, guide.rel);
      assert.ok(fs.existsSync(htmlPath), `Guide page must exist: ${guide.rel}`);
      const html = fs.readFileSync(htmlPath, 'utf8');

      const titleMatch = html.match(/<title>([^<]+)<\/title>/);
      assert.ok(titleMatch, `${guide.rel} must have <title>`);
      const titleLen = titleMatch[1].trim().length;
      assert.ok(titleLen >= 30 && titleLen <= 65, `${guide.rel} title length (${titleLen}) must be within [30, 65]`);

      const descMatch = html.match(/<meta\s+name="description"\s+content="([^"]+)"/);
      assert.ok(descMatch, `${guide.rel} must have meta description`);
      const descLen = descMatch[1].trim().length;
      assert.ok(descLen >= 110 && descLen <= 165, `${guide.rel} description length (${descLen}) must be within [110, 165]`);

      assert.ok(
        html.includes(`<link rel="canonical" href="https://abastevo.com.br${guide.canonicalPath}">`),
        `${guide.rel} must declare exact canonical URL`
      );

      assert.ok(html.includes('<nav class="breadcrumb-nav"'), `${guide.rel} must have breadcrumb nav`);
      assert.ok(/aria-label="(Breadcrumb|Navegação estrutural)"/.test(html), `${guide.rel} breadcrumb nav must carry an accessible label`);
      assert.ok(html.includes('<a href="/">Início</a>'), `${guide.rel} breadcrumb must link Início`);
      assert.ok(html.includes('<main'), `${guide.rel} must use semantic <main>`);
      assert.ok(html.includes('<article'), `${guide.rel} must use semantic <article>`);
      assert.ok(html.includes(guide.h1), `${guide.rel} must contain expected H1 text`);
      assert.ok(html.includes('/guias/'), `${guide.rel} must reference guides hub`);

      for (const claim of FORBIDDEN_CLAIMS) {
        assert.ok(!html.toLowerCase().includes(claim), `${guide.rel} must not contain forbidden claim: ${claim}`);
      }
      assert.ok(!html.includes('document.cookie'), `${guide.rel} must not read cookies`);
    }

    const hub = fs.readFileSync(path.join(tmpOut, 'guias/index.html'), 'utf8');
    assert.ok(hub.includes('/guias/pesquisa-anp/'), 'hub must link pesquisa-anp guide');
    assert.ok(hub.includes('/guias/etanol-ou-gasolina/'), 'hub must link etanol-ou-gasolina guide');
    assert.ok(hub.includes('/guias/como-ler-precos/'), 'hub must link como-ler-precos guide');

    const etanol = fs.readFileSync(path.join(tmpOut, 'guias/etanol-ou-gasolina/index.html'), 'utf8');
    assert.ok(etanol.includes('<table'), 'etanol guide must include parity table');
    assert.ok(etanol.includes('<caption>'), 'parity table must have <caption>');
    assert.ok(etanol.includes('<th scope="col">'), 'parity table must use scoped column headers');

    const index = fs.readFileSync(path.join(tmpOut, 'index.html'), 'utf8');
    assert.ok(index.includes('href="/guias/"'), 'landing index must link guides hub');
  } finally {
    fs.rmSync(tmpOut, { recursive: true, force: true });
  }
});

test('guides schemas and discovery: JSON-LD graphs, sitemap and llms.txt', () => {
  const tmpOut = path.join(root, '.test-guides-schema-' + Math.random().toString(36).slice(2, 8));
  try {
    buildSite({ projectRoot: root, outDir: tmpOut, canonicalOrigin: 'https://abastevo.com.br', storeState: 'prelaunch' });

    for (const guide of GUIDES) {
      const html = fs.readFileSync(path.join(tmpOut, guide.rel), 'utf8');
      const blocks = Array.from(html.matchAll(/<script\s+type="application\/ld\+json">([\s\S]*?)<\/script>/g));
      assert.ok(blocks.length >= 1, `${guide.rel} must embed JSON-LD`);
      const ids = new Set();
      let hasArticle = false;
      let hasBreadcrumbs = false;
      for (const block of blocks) {
        const parsed = JSON.parse(block[1].trim());
        assert.ok(parsed['@context'], `${guide.rel} JSON-LD must declare @context`);
        const nodes = parsed['@graph'] && Array.isArray(parsed['@graph']) ? parsed['@graph'] : [parsed];
        for (const node of nodes) {
          if (node['@id']) {
            assert.ok(!ids.has(node['@id']), `${guide.rel} JSON-LD duplicate @id: ${node['@id']}`);
            ids.add(node['@id']);
          }
          if (node['@type'] === 'Article' || node['@type'] === 'TechArticle' || node['@type'] === 'CollectionPage') hasArticle = true;
          if (node['@type'] === 'BreadcrumbList') hasBreadcrumbs = true;
        }
      }
      assert.ok(hasArticle, `${guide.rel} JSON-LD must include Article/TechArticle/CollectionPage`);
      assert.ok(hasBreadcrumbs, `${guide.rel} JSON-LD must include BreadcrumbList`);
      assert.ok(!html.includes('aggregateRating'), `${guide.rel} must not fabricate ratings`);
      assert.ok(!html.includes('"review"'), `${guide.rel} must not fabricate reviews`);
      assert.ok(html.includes('<meta name="twitter:card" content="summary_large_image">'), `${guide.rel} must use Twitter large card`);
    }

    const sitemap = fs.readFileSync(path.join(tmpOut, 'sitemap.xml'), 'utf8');
    for (const guide of GUIDES) {
      assert.ok(sitemap.includes(`<loc>https://abastevo.com.br${guide.canonicalPath}</loc>`), `sitemap must list ${guide.canonicalPath}`);
    }
    assert.ok(!sitemap.includes('<lastmod>'), 'sitemap must not fabricate lastmod');
    assert.ok(!sitemap.includes('404.html'), 'sitemap must not list 404 page');

    const llms = fs.readFileSync(path.join(tmpOut, 'llms.txt'), 'utf8');
    assert.ok(llms.includes('/guias/'), 'llms.txt must reference guides hub');
  } finally {
    fs.rmSync(tmpOut, { recursive: true, force: true });
  }
});

test('guides budgets: CSS/JS within limits and each guide page transfer under 500 KiB', () => {
  const tmpOut = path.join(root, '.test-guides-budget-' + Math.random().toString(36).slice(2, 8));
  try {
    buildSite({ projectRoot: root, outDir: tmpOut, canonicalOrigin: 'https://abastevo.com.br', storeState: 'prelaunch' });
    const assetsDir = path.join(tmpOut, 'assets');
    const assetFiles = fs.readdirSync(assetsDir);
    const cssFile = assetFiles.find((f) => /^styles\.[a-f0-9]{8}\.css$/.test(f));
    const jsFile = assetFiles.find((f) => /^main\.[a-f0-9]{8}\.js$/.test(f));
    assert.ok(cssFile, 'fingerprinted CSS must exist');
    assert.ok(jsFile, 'fingerprinted JS must exist');
    assert.ok(zlib.gzipSync(fs.readFileSync(path.join(assetsDir, cssFile))).length <= 25 * 1024, 'CSS gzipped must stay within 25 KiB');
    assert.ok(zlib.gzipSync(fs.readFileSync(path.join(assetsDir, jsFile))).length <= 10 * 1024, 'JS gzipped must stay within 10 KiB');

    for (const guide of GUIDES) {
      const size = fs.statSync(path.join(tmpOut, guide.rel)).size;
      assert.ok(size <= 500 * 1024, `${guide.rel} transfer (${size} bytes) must stay under 500 KiB`);
    }
  } finally {
    fs.rmSync(tmpOut, { recursive: true, force: true });
  }
});
