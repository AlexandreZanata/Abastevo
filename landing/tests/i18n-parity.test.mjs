import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const codes = ['pt-BR', 'en', 'es'];

function readMessages(code) {
  return JSON.parse(fs.readFileSync(path.join(root, 'content', 'i18n', `${code}.json`), 'utf8'));
}

function shape(value, prefix = '') {
  if (Array.isArray(value)) return { [`${prefix}.length`]: value.length, ...value.reduce((acc, item, index) => ({ ...acc, ...shape(item, `${prefix}[${index}]`) }), {}) };
  if (value && typeof value === 'object') {
    return Object.fromEntries(Object.keys(value).sort().map((key) => [key, shape(value[key], prefix ? `${prefix}.${key}` : key)]));
  }
  return typeof value;
}

function placeholders(value, prefix = '', out = {}) {
  if (typeof value === 'string') {
    out[prefix] = [...value.matchAll(/\{(\w+)\}/g)].map((match) => match[1]).sort();
  } else if (Array.isArray(value)) {
    value.forEach((item, index) => placeholders(item, `${prefix}[${index}]`, out));
  } else if (value && typeof value === 'object') {
    for (const key of Object.keys(value)) placeholders(value[key], prefix ? `${prefix}.${key}` : key, out);
  }
  return out;
}

test('i18n parity: the three dictionaries share keys, types and placeholders', () => {
  const messages = Object.fromEntries(codes.map((code) => [code, readMessages(code)]));
  for (const code of ['en', 'es']) {
    assert.deepEqual(shape(messages[code]), shape(messages['pt-BR']), `${code} keys and types match pt-BR`);
    assert.deepEqual(placeholders(messages[code]), placeholders(messages['pt-BR']), `${code} placeholders match pt-BR`);
  }
  for (const code of codes) {
    assert.equal(messages[code].months.length, 12, `${code} has 12 month names`);
    assert.equal(Object.keys(messages[code].categories).length, 5, `${code} translates every category`);
    assert.equal(Object.keys(messages[code].tools).filter((key) => key !== 'common').length, 5, `${code} translates every calculator`);
    for (const slug of ['etanol-ou-gasolina', 'custo-de-viagem', 'consumo-medio', 'custo-por-km', 'encher-o-tanque']) {
      assert.ok(messages[code].tools[slug], `${code} has calculator strings for ${slug}`);
      assert.ok(Object.keys(messages[code].tools[slug].messages).length > 0, `${code} has result messages for ${slug}`);
    }
  }
  assert.equal(messages['pt-BR'].languageName, 'Português');
  assert.equal(messages.en.languageName, 'English');
  assert.equal(messages.es.languageName, 'Español');
});
