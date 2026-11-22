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

test('locale registry: pt-BR is the default and en/es are enabled with a recorded AI review', () => {
  const registry = loadLocales(root);
  assert.equal(registry.version, 1);
  assert.equal(registry.default, 'pt-BR');
  assert.deepEqual(registry.locales.map((locale) => locale.code), ['pt-BR', 'en', 'es']);
  assert.equal(registry.locales.find((locale) => locale.code === 'pt-BR').enabled, true);
  assert.ok(registry.locales.find((locale) => locale.code === 'pt-BR').reviewers.length >= 1);
  // ADR-027: the review of en and es is made by AI and recorded as such.
  for (const code of ['en', 'es']) {
    const entry = registry.locales.find((locale) => locale.code === code);
    assert.equal(entry.enabled, true, `${code} is enabled`);
    assert.ok(entry.reviewers.length);
    for (const reviewer of entry.reviewers) assert.match(reviewer, /^AI review \([^)]+\)$/, `${code} records the AI review`);
  }
});

test('locale routes: the shared vectors match localeRoute', () => {
  const registry = loadLocales(root);
  const vectors = JSON.parse(fs.readFileSync(path.join(root, '..', 'contracts', 'testdata', 'i18n', 'routes.json'), 'utf8'));
  assert.deepEqual(new Set(vectors.map((vector) => vector.kind)),
    new Set(['guide', 'tool', 'page', 'prices', 'methodology', 'data', 'insights',
      'search', 'stations', 'forum', 'enforcement', 'enforcementPage', 'enforcementRecord']));
  for (const vector of vectors) {
    const locale = registry.locales.find((entry) => entry.code === vector.locale);
    assert.ok(locale, `vector locale ${vector.locale} exists in the registry`);
    assert.equal(localeRoute(locale, vector.kind, vector.slug, vector.area, vector.city), vector.route, `${vector.locale} ${vector.kind} ${vector.slug}`);
  }
  assert.equal(localeRoute(registry.locales[0], 'guide', 'gnv-compensa'), '/guias/gnv-compensa/');
  assert.throws(() => localeRoute(registry.locales[0], 'hub', 'guias'), /unknown kind/);
  const en = registry.locales.find((entry) => entry.code === 'en');
  assert.throws(() => localeRoute(en, 'forum', 'data/../../outside'), /invalid forum suffix/);
  assert.throws(() => localeRoute(en, 'enforcementRecord', 'invalid'), /invalid enforcementRecord identifier/);
  assert.throws(() => localeRoute(en, 'enforcementPage', '1'), /invalid enforcementPage identifier/);
  assert.throws(() => localeRoute(en, 'prices', 'unknown'), /unknown fuel slug/);
  assert.throws(() => localeRoute(en, 'prices', '', 'PR'), /invalid state or city/);
  assert.throws(() => localeRoute(en, 'prices', '', '', 'foz-do-iguacu'), /invalid state or city/);
  assert.throws(() => localeRoute(en, 'prices', 'gasolina-comum', 'pr', 'foz-do-iguacu'), /fuel by city is available only in pt-BR/);
});

test('locale registry: invalid inputs fail with a clear message', () => {
  const cases = [
    [(data) => { delete data.locales[1].enabled; }, /missing field "enabled"/],
    [(data) => { data.locales[1].code = 'pt-BR'; }, /duplicate locale code "pt-BR"/],
    [(data) => { data.locales[1].prefix = ''; }, /duplicate prefix ""/],
    [(data) => { data.locales[1].prefix = '/english'; }, /prefix "\/english" is not "" or \/xx/],
    [(data) => { data.locales[1].segments.guias = 'Guides'; }, /segments\.guias .* is not a slug/],
    [(data) => { delete data.locales[1].segments.precos; }, /segments is missing "precos"/],
    [(data) => { delete data.locales[1].fuelSlugs.gnv; }, /fuelSlugs must have the same keys/],
    [(data) => { data.locales[1].fuelSlugs.gnv = 'pr'; }, /invalid fuel slug for gnv/],
    [(data) => { data.locales[1].fuelSlugs.gnv = data.locales[1].fuelSlugs.glp; }, /duplicate fuel slug/],
    [(data) => { data.default = 'fr'; }, /default locale "fr" is missing/],
    [(data) => { data.locales[1].enabled = 'yes'; }, /enabled must be a boolean/],
    [(data) => { data.locales[1].decimal = ';'; }, /decimal must be "," or "\."/],
    [(data) => { data.locales[1].ogLocale = ''; }, /ogLocale must be a non-empty string/],
    [(data) => { data.locales[1].enabled = true; data.locales[1].reviewers = []; }, /cannot be enabled without at least one named reviewer/],
    [(data) => { data.locales[1].reviewers = ['']; }, /reviewers must be an array of non-empty strings/],
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
