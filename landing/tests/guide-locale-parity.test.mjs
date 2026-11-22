import assert from 'node:assert/strict';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { test } from 'node:test';
import { loadSite } from '../scripts/content/site.mjs';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');

test('every published Portuguese editorial guide has a fresh en and es counterpart', () => {
  const site = loadSite(root);
  for (const locale of ['en', 'es']) {
    const missing = site.guides.filter(({ route }) => {
      const translated = site.registry.get(site.localizedRoute(route, locale));
      return translated?.locale !== locale || translated.stale || !translated.indexable;
    });
    assert.deepEqual(missing.map(({ route }) => route), [], `${locale} guide coverage`);
  }
});
