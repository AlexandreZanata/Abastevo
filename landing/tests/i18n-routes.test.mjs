import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { loadLocales, localeRoute, LocaleConfigError } from '../scripts/content/i18n.mjs';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const registryPath = path.join(root, '..', 'contracts', 'i18n', 'locales.json');

function withRegistry(mutate) {
  const tmp = fs.mkdtempSync(path.join(os.tmpdir(), 'abastevo-i18n-'));
  const landingRoot = path.join(tmp, 'landing');
  const dir = path.join(tmp, 'contracts', 'i18n');
  fs.mkdirSync(landingRoot);
  fs.mkdirSync(dir, { recursive: true });
  const data = JSON.parse(fs.readFileSync(registryPath, 'utf8'));
  mutate(data);
  fs.writeFileSync(path.join(dir, 'locales.json'), JSON.stringify(data));
  return { landingRoot, cleanup: () => fs.rmSync(tmp, { recursive: true, force: true }) };
}

test('locale registry: pt-BR is the default and en/es stay disabled', () => {
  const registry = loadLocales(root);
  assert.equal(registry.version, 1);
  assert.equal(registry.default, 'pt-BR');
  assert.deepEqual(registry.locales.map((locale) => locale.code), ['pt-BR', 'en', 'es']);
  assert.equal(registry.locales.find((locale) => locale.code === 'pt-BR').enabled, true);
  for (const code of ['en', 'es']) {
    assert.equal(registry.locales.find((locale) => locale.code === code).enabled, false, `${code} must start disabled`);
  }
});

test('locale routes: the shared vectors match localeRoute', () => {
  const registry = loadLocales(root);
  const vectors = JSON.parse(fs.readFileSync(path.join(root, '..', 'contracts', 'testdata', 'i18n', 'routes.json'), 'utf8'));
  assert.ok(vectors.length >= 4, 'vectors cover every locale and kind');
  for (const vector of vectors) {
    const locale = registry.locales.find((entry) => entry.code === vector.locale);
    assert.ok(locale, `vector locale ${vector.locale} exists in the registry`);
    assert.equal(localeRoute(locale, vector.kind, vector.slug), vector.route, `${vector.locale} ${vector.kind} ${vector.slug}`);
  }
  assert.equal(localeRoute(registry.locales[0], 'guide', 'gnv-compensa'), '/guias/gnv-compensa/');
  assert.throws(() => localeRoute(registry.locales[0], 'hub', 'guias'), /unknown kind/);
});

test('locale registry: invalid inputs fail with a clear message', () => {
  const cases = [
    [(data) => { delete data.locales[1].enabled; }, /missing field "enabled"/],
    [(data) => { data.locales[1].code = 'pt-BR'; }, /duplicate locale code "pt-BR"/],
    [(data) => { data.locales[1].prefix = ''; }, /duplicate prefix ""/],
    [(data) => { data.locales[1].prefix = '/english'; }, /prefix "\/english" is not "" or \/xx/],
    [(data) => { data.locales[1].segments.guias = 'Guides'; }, /segments\.guias .* is not a slug/],
  ];
  for (const [mutate, pattern] of cases) {
    const fixture = withRegistry(mutate);
    try {
      assert.throws(() => loadLocales(fixture.landingRoot), (err) => err instanceof LocaleConfigError && pattern.test(err.message), String(pattern));
    } finally {
      fixture.cleanup();
    }
  }
});
