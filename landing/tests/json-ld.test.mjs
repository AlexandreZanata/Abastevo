import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { buildSite } from '../scripts/build.mjs';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const root = path.resolve(__dirname, '..');

test('json-ld: prelaunch state has factual graph without phantom mobile app or ratings', () => {
  const tmpOut = path.join(root, '.test-jsonld-prelaunch-' + Math.random().toString(36).slice(2, 8));
  try {
    buildSite({
      projectRoot: root,
      outDir: tmpOut,
      canonicalOrigin: 'https://abastevo.com.br',
      storeState: 'prelaunch',
    });

    const indexHtml = fs.readFileSync(path.join(tmpOut, 'index.html'), 'utf8');
    const jsonLdMatch = indexHtml.match(/<script type="application\/ld\+json">([\s\S]*?)<\/script>/);
    assert.ok(jsonLdMatch, 'index.html must contain JSON-LD block');

    const data = JSON.parse(jsonLdMatch[1].trim());
    assert.equal(data['@context'], 'https://schema.org');
    assert.ok(Array.isArray(data['@graph']), '@graph must be an array');

    const types = data['@graph'].map((n) => n['@type']);
    assert.ok(types.includes('WebSite'), '@graph must include WebSite');
    assert.ok(types.includes('Organization'), '@graph must include Organization');
    assert.ok(types.includes('WebPage'), '@graph must include WebPage');

    // Negative assertions for prelaunch state: NO phantom app or ratings
    assert.ok(!types.includes('MobileApplication'), 'Prelaunch must NOT include MobileApplication');
    assert.ok(!types.includes('SoftwareApplication'), 'Prelaunch must NOT include SoftwareApplication');
    assert.ok(!indexHtml.includes('aggregateRating'), 'Prelaunch must NOT claim ratings');
    assert.ok(!indexHtml.includes('ratingValue'), 'Prelaunch must NOT claim rating values');
    assert.ok(!indexHtml.includes('reviewCount'), 'Prelaunch must NOT claim reviews');

    // Organization assertions
    const org = data['@graph'].find((n) => n['@type'] === 'Organization');
    assert.equal(org.name, 'abastevo');
    assert.equal(org.url, 'https://abastevo.com.br/');
    assert.equal(org.logo, 'https://abastevo.com.br/assets/brand/icon-512.png');
    assert.ok(org.sameAs.includes('https://github.com/AlexandreZanata/abastevo'));

    // ID uniqueness
    const ids = data['@graph'].map((n) => n['@id']).filter(Boolean);
    const uniqueIds = new Set(ids);
    assert.equal(ids.length, uniqueIds.size, 'All @id entries in graph must be unique');
  } finally {
    fs.rmSync(tmpOut, { recursive: true, force: true });
  }
});

test('json-ld: published state fixture contains valid MobileApplication schema', () => {
  const tmpOut = path.join(root, '.test-jsonld-pub-' + Math.random().toString(36).slice(2, 8));
  const validStoreUrl = 'https://play.google.com/store/apps/details?id=com.anpfuel';
  try {
    buildSite({
      projectRoot: root,
      outDir: tmpOut,
      canonicalOrigin: 'https://abastevo.com.br',
      storeState: 'published',
      storeUrl: validStoreUrl,
    });

    const indexHtml = fs.readFileSync(path.join(tmpOut, 'index.html'), 'utf8');
    const jsonLdMatch = indexHtml.match(/<script type="application\/ld\+json">([\s\S]*?)<\/script>/);
    assert.ok(jsonLdMatch, 'index.html must contain JSON-LD block');

    const data = JSON.parse(jsonLdMatch[1].trim());
    const app = data['@graph'].find((n) => n['@type'] === 'MobileApplication');
    assert.ok(app, 'Published state must include MobileApplication schema');
    assert.equal(app.name, 'abastevo');
    assert.equal(app.operatingSystem, 'Android');
    assert.equal(app.applicationCategory, 'UtilitiesApplication');
    assert.equal(app.installUrl, validStoreUrl);
    assert.equal(app.offers.price, '0');
    assert.equal(app.offers.priceCurrency, 'BRL');

    // Organization sameAs includes storeUrl in published state
    const org = data['@graph'].find((n) => n['@type'] === 'Organization');
    assert.ok(org.sameAs.includes(validStoreUrl), 'Organization sameAs must include store URL when published');
  } finally {
    fs.rmSync(tmpOut, { recursive: true, force: true });
  }
});

test('json-ld: legal pages include valid BreadcrumbList structured data', () => {
  const tmpOut = path.join(root, '.test-jsonld-breadcrumbs-' + Math.random().toString(36).slice(2, 8));
  try {
    buildSite({
      projectRoot: root,
      outDir: tmpOut,
      canonicalOrigin: 'https://abastevo.com.br',
      storeState: 'prelaunch',
    });

    const legalPages = ['privacidade/index.html', 'excluir-conta/index.html', 'termos/index.html'];

    for (const rel of legalPages) {
      const html = fs.readFileSync(path.join(tmpOut, rel), 'utf8');
      const jsonLdMatch = html.match(/<script type="application\/ld\+json">([\s\S]*?)<\/script>/);
      assert.ok(jsonLdMatch, `${rel} must contain JSON-LD block`);

      const data = JSON.parse(jsonLdMatch[1].trim());
      assert.equal(data['@context'], 'https://schema.org');
      assert.equal(data['@type'], 'BreadcrumbList');
      assert.equal(data.itemListElement.length, 2);
      assert.equal(data.itemListElement[0].position, 1);
      assert.equal(data.itemListElement[0].item, 'https://abastevo.com.br/');
      assert.equal(data.itemListElement[1].position, 2);
      assert.ok(data.itemListElement[1].item.startsWith('https://abastevo.com.br/'));
    }
  } finally {
    fs.rmSync(tmpOut, { recursive: true, force: true });
  }
});
