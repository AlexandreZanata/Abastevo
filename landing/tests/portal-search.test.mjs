import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { execFileSync } from 'node:child_process';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { build } from '../scripts/build.mjs';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const root = path.resolve(__dirname, '..');

const compiled = (() => {
  const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'abastevo-search-'));
  execFileSync('tsc', ['--project', path.join(root, 'tsconfig.json'), '--outDir', tmpDir], {
    cwd: root,
    stdio: 'pipe',
  });
  const modPath = path.join(tmpDir, 'search.js');
  assert.ok(fs.existsSync(modPath), 'search.js must compile');
  return { tmpDir, modPath };
})();

test.after(() => {
  fs.rmSync(compiled.tmpDir, { recursive: true, force: true });
});

async function searchModule() {
  return import(pathToFileURL(compiled.modPath).href);
}

function station(overrides = {}) {
  return {
    i: '00000000-0000-4000-8000-000000000001',
    n: 'Posto Alfa',
    s: 'posto alfa',
    c: '04218406000104',
    m: '3550308',
    mn: 'São Paulo',
    u: 'SP',
    st: 'active',
    sl: 'Ativo (informado pela fonte)',
    p: { GASOLINE_REGULAR: 6199 },
    ...overrides,
  };
}

function shard(stations, overrides = {}) {
  return {
    schema: 'abastevo-search-v1',
    release: 'rel',
    uf: 'SP',
    count: stations.length,
    stations,
    ...overrides,
  };
}

test('normalizes accents, numeric and alphanumeric CNPJ', async () => {
  const { normalizeSearchText, normalizeCNPJQuery, formatCNPJ, isCNPJQuery } = await searchModule();
  assert.equal(normalizeSearchText('  São   Paulo  '), 'sao paulo');
  assert.equal(normalizeSearchText('Épsilon'), 'epsilon');
  assert.equal(normalizeCNPJQuery('04.218.406/0001-04'), '04218406000104');
  assert.equal(normalizeCNPJQuery('12.ABC.345/01DE-35'), '12ABC34501DE35');
  assert.equal(formatCNPJ('04218406000104'), '04.218.406/0001-04');
  assert.equal(isCNPJQuery('04.218.406/0001-04'), true);
  assert.equal(isCNPJQuery('12.ABC.345/01DE-35'), true);
  assert.equal(isCNPJQuery('Posto Alfa'), false);
});

test('findMatches handles duplicate names, accents, CNPJ, abuse and bounds', async () => {
  const { findMatches } = await searchModule();
  const rows = [
    station(),
    station({ i: '00000000-0000-4000-8000-000000000002', c: '11222333000181' }),
    station({ i: '00000000-0000-4000-8000-000000000003', n: 'Posto Épsilon', s: 'posto epsilon', c: '11444777000161', m: '3106200' }),
    station({ i: '00000000-0000-4000-8000-000000000004', n: 'Posto Alfa', s: 'posto alfa', c: '12ABC34501DE35' }),
  ];
  const data = shard(rows);
  assert.equal(findMatches(data, 'alfa').length, 3);
  assert.equal(findMatches(data, 'epsilon').length, 1);
  assert.equal(findMatches(data, '042184').length, 1);
  assert.equal(findMatches(data, '12.ABC.345/01DE-35').length, 1);
  assert.equal(findMatches(data, 'alfa', '3106200').length, 0);
  assert.equal(findMatches(data, 'alfa', '', 1).length, 1);
  assert.doesNotThrow(() => findMatches(data, '<script>alert(1)</script>'));
  assert.equal(findMatches(data, '<script>alert(1)</script>').length, 0);
  assert.doesNotThrow(() => findMatches(data, 'a'.repeat(500)));
  assert.ok(findMatches(data, 'a'.repeat(500)).length <= 50);
  assert.equal(findMatches(data, 'a').length, 0);
});

test('shard validation and loading are version-bound and regional-only', async () => {
  const { validateShard, loadShard } = await searchModule();
  const data = shard([station()]);
  assert.equal(validateShard(data, 'rel').release, 'rel');
  assert.throws(() => validateShard(data, 'other'), /shard-version/);
  assert.throws(() => validateShard({ ...data, schema: 'other' }, 'rel'), /shard-version/);

  const calls = [];
  const fetcher = async (url) => {
    calls.push(String(url));
    return { ok: true, json: async () => data };
  };
  const loaded = await loadShard('SP', 'rel', '/dados/busca/', fetcher);
  assert.equal(loaded.uf, 'SP');
  assert.deepEqual(calls, ['/dados/busca/rel/SP.json']);
  await assert.rejects(loadShard('BR', 'rel', '/dados/busca/', fetcher), /shard-uf/);
  await assert.rejects(loadShard('SP', 'other', '/dados/busca/', fetcher), /shard-version/);
  await assert.rejects(loadShard('SP', 'rel', '/dados/busca/', async () => ({ ok: false })), /shard-missing/);
  assert.ok(calls.every((url) => /^\/dados\/busca\/(rel|other)\/SP\.json$/.test(url)));
});

class FakeElement {
  constructor(tagName) {
    this.tagName = tagName;
    this.textContent = '';
    this.className = '';
    this.href = '';
    this.value = '';
    this.disabled = false;
    this.dataset = {};
    this.children = [];
    this.handlers = {};
  }
  append(...nodes) { this.children.push(...nodes); }
  replaceChildren(...nodes) { this.children = nodes; }
  addEventListener(type, handler) { (this.handlers[type] ||= []).push(handler); }
  dispatch(type, event = {}) {
    for (const handler of this.handlers[type] || []) handler(event);
  }
}

function fakePortal() {
  const form = new FakeElement('form');
  const input = new FakeElement('input');
  const uf = new FakeElement('select');
  const city = new FakeElement('select');
  const status = new FakeElement('p');
  const results = new FakeElement('ul');
  const selectors = {
    '[data-search-query]': input,
    '[data-search-uf]': uf,
    '[data-search-city]': city,
    '[data-search-status]': status,
    '[data-search-results]': results,
  };
  form.querySelector = (selector) => selectors[selector] || null;
  form.dataset = { shardBase: '/dados/busca/', release: 'rel', resultLimit: '50' };
  const doc = {
    querySelector: (selector) => (selector === '[data-portal-search]' ? form : null),
    createElement: (tag) => new FakeElement(tag),
    addEventListener: () => {},
  };
  return { form, input, uf, city, status, results, doc };
}

test('setupPortalSearch loads one shard, renders text results and falls back', async () => {
  const { setupPortalSearch } = await searchModule();
  const fake = fakePortal();
  let calls = 0;
  let response = shard([
    station(),
    station({ i: '00000000-0000-4000-8000-000000000002', n: 'Posto Alfa', c: '11222333000181', m: '3106200', mn: 'Campinas' }),
  ]);
  setupPortalSearch({
    document: fake.doc,
    fetch: async (url) => {
      calls += 1;
      assert.match(String(url), /^\/dados\/busca\/rel\/(SP|MG)\.json$/);
      return { ok: true, json: async () => response };
    },
  });
  fake.uf.value = 'SP';
  fake.input.value = 'alfa';
  fake.form.dispatch('submit', { preventDefault() {} });
  await new Promise((resolve) => setTimeout(resolve, 20));
  assert.equal(calls, 1);
  assert.equal(fake.results.children.length, 2);
  assert.equal(fake.results.children[0].children[0].href, '/postos/00000000-0000-4000-8000-000000000001/');
  assert.equal(fake.results.children[0].children[0].textContent, 'Posto Alfa');
  assert.match(fake.results.children[0].children[1].textContent, /04\.218\.406\/0001-04/);
  assert.match(fake.status.textContent, /2 postos encontrados/);
  assert.ok(fake.city.children.some((option) => option.value === '3550308'));
  assert.ok(fake.city.children.some((option) => option.value === '3106200'));
  assert.equal(fake.city.disabled, false);

  fake.input.value = 'nao-existe';
  fake.form.dispatch('submit', { preventDefault() {} });
  await new Promise((resolve) => setTimeout(resolve, 20));
  assert.match(fake.status.textContent, /Nenhum posto/);
  assert.equal(fake.results.children.length, 1);

  response = shard([station()], { release: 'other' });
  fake.uf.value = 'MG';
  fake.uf.dispatch('change');
  await new Promise((resolve) => setTimeout(resolve, 20));
  assert.match(fake.status.textContent, /indisponível/);
});

test('search asset is fingerprintable and contains no unsafe HTML injection', async () => {
  const result = build({ projectRoot: root, outDir: path.join(root, '.test-search-dist-' + Date.now()) });
  try {
    assert.match(result.fingerprintedSearch, /^search\.[a-f0-9]{8}\.js$/);
    const asset = fs.readFileSync(path.join(result.outDir, 'assets', result.fingerprintedSearch), 'utf8');
    assert.match(asset, /textContent/);
    assert.doesNotMatch(asset, /innerHTML/);
    assert.doesNotMatch(asset, /\/v1\//);
    assert.doesNotMatch(asset, /eval\s*\(|new Function/);
  } finally {
    fs.rmSync(result.outDir, { recursive: true, force: true });
  }
});

test('compiled search never requests a national locator', async () => {
  const raw = fs.readFileSync(compiled.modPath, 'utf8');
  assert.match(raw, /dados\/busca/);
  assert.doesNotMatch(raw, /all\.json|nacional/i);
  assert.doesNotMatch(raw, /anp\.gov\.br\/api|api\.abastevo|\/v1\//);
});
