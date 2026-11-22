import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { test } from 'node:test';
import { build } from '../scripts/build.mjs';
import { loadSite } from '../scripts/content/site.mjs';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const source = ['/privacidade/', '/excluir-conta/', '/termos/'];
const translated = {
  en: ['/en/privacy/', '/en/delete-account/', '/en/terms/'],
  es: ['/es/privacidad/', '/es/eliminar-cuenta/', '/es/terminos/'],
};

test('informative legal translations preserve source navigation and stay out of indexing', () => {
  const site = loadSite(root);
  const outDir = fs.mkdtempSync(path.join(os.tmpdir(), 'abastevo-legal-locales-'));
  try {
    build({ projectRoot: root, outDir, origin: 'https://abastevo.com.br' });
    const sitemap = fs.readFileSync(path.join(outDir, 'sitemap.xml'), 'utf8');
    for (const [locale, routes] of Object.entries(translated)) {
      for (const [index, route] of routes.entries()) {
        const entry = site.registry.get(route);
        assert.equal(entry.translationOf, source[index]);
        assert.equal(entry.indexable, false);
        assert.equal(entry.stale, false);
        assert.equal(site.switcherMembers(route).find((link) => link.code === 'pt-BR').route, source[index]);
        assert.equal(site.switcherMembers(source[index]).find((link) => link.code === locale).route, route);
        const html = fs.readFileSync(path.join(outDir, entry.outputRel), 'utf8');
        assert.match(html, /<meta name="robots" content="noindex,follow">/);
        assert.doesNotMatch(html, /rel="canonical"|rel="alternate" hreflang=/);
        assert.ok(!sitemap.includes(route));
        assert.ok(html.includes('href="' + source[index] + '" hreflang="pt-BR"'));
        assert.ok(html.includes('href="' + routes[0] + '"'));
        if (index < 2) {
          assert.match(html, /href="tel:\+556696314240"/);
          assert.match(html, /href="https:\/\/wa\.me\/556696314240\?/);
        }
      }
    }
  } finally {
    fs.rmSync(outDir, { recursive: true, force: true });
  }
});
