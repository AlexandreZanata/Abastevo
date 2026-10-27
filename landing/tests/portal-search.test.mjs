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
  const fetcher = async (url) => { calls.push(String(url)); return { ok: true, json: async () => data }; };
  const loaded = await loadShard('SP', 'rel', '/dados/busca/', fetcher);
  assert.equal(loaded.uf, 'SP');
  assert.deepEqual(calls, ['/dados/busca/rel/SP.json']);
  await assert.rejects(loadShard('BR', 'rel', '/dados/busca/', fetcher), /shard-uf/);
  await assert.rejects(loadShard('SP', 'other', '/dados/busca/', fetcher), /shard-version/);
  await assert.rejects(loadShard('SP', 'rel', '/dados/busca/', async () => ({ ok: false })), /shard-missing/);
});

const INDEX = {
  schema: 'abastevo-search-index-v1',
  release: 'rel',
  prefix_length: 3,
  stopwords: ['posto', 'auto', 'ltda', 'de'],
  fiscal: true,
  fuels: [{ code: 'GASOLINE_REGULAR', label: 'Gasolina comum', unit: 'L', count: 2 }],
  municipalities: [
    { m: '5107925', n: 'Sorriso', s: 'sorriso', u: 'MT', k: 26 },
    { m: '3550308', n: 'São Paulo', s: 'sao paulo', u: 'SP', k: 1525 },
    { m: '3549904', n: 'São José dos Campos', s: 'sao jose dos campos', u: 'SP', k: 120 },
  ],
};

test('prefix key selection skips stopwords and handles CNPJ', async () => {
  const { prefixKeyFor } = await searchModule();
  assert.equal(prefixKeyFor('Auto Posto Shell', INDEX), 'n-she');
  assert.equal(prefixKeyFor('AUTOPOSTOSHELL', INDEX), 'n-aut');
  assert.equal(prefixKeyFor('12ABC34501DE35', INDEX), 'c-12a');
  assert.equal(prefixKeyFor('posto ipiranga sao', INDEX), 'n-ipi');
  assert.equal(prefixKeyFor('posto de', INDEX), null);
  assert.equal(prefixKeyFor('sh', INDEX), null);
  assert.equal(prefixKeyFor('04.218.406/0001-04', INDEX), 'c-042');
  assert.equal(prefixKeyFor('042', INDEX), 'c-042');
  assert.equal(prefixKeyFor('Pôsto Ítalo', INDEX), 'n-ita');
});

test('place suggestions rank states and cities and respect a state scope', async () => {
  const { placeSuggestions } = await searchModule();
  const sao = placeSuggestions('são', INDEX.municipalities, null);
  assert.equal(sao[0].label, 'São Paulo (SP)');
  assert.equal(sao[0].scope.kind, 'uf');
  assert.deepEqual(sao.filter((s) => s.scope.kind === 'city').map((s) => s.label), ['São Paulo/SP', 'São José dos Campos/SP']);
  assert.equal(placeSuggestions('mt', INDEX.municipalities, null)[0].scope.uf, 'MT');
  const inMT = placeSuggestions('s', INDEX.municipalities, { kind: 'uf', uf: 'MT', label: 'Mato Grosso (MT)' });
  assert.deepEqual(inMT, []);
  const sorriso = placeSuggestions('sorr', INDEX.municipalities, { kind: 'uf', uf: 'MT', label: 'Mato Grosso (MT)' });
  assert.deepEqual(sorriso.map((s) => s.label), ['Sorriso/MT']);
});

test('price filters sort cheapest or most expensive within a place', async () => {
  const { filterPrices, formatMilliBRL, formatDateBR } = await searchModule();
  const rows = [
    { i: 'a', n: 'Posto A', c: '04218406000104', m: '5107925', mn: 'Sorriso', u: 'MT', v: 6390, d: '2026-10-20' },
    { i: 'b', n: 'Posto B', m: '5107925', u: 'MT', v: 5990 },
    { i: 'c', n: 'Posto C', m: '3550308', u: 'SP', v: 5490 },
  ];
  assert.deepEqual(filterPrices(rows, '', null, 'asc').map((r) => r.i), ['c', 'b', 'a']);
  assert.deepEqual(filterPrices(rows, '', { kind: 'uf', uf: 'MT', label: '' }, 'desc').map((r) => r.i), ['a', 'b']);
  assert.deepEqual(filterPrices(rows, '042184', null, 'asc').map((r) => r.i), ['a']);
  assert.equal(formatMilliBRL(6390), 'R$ 6,390');
  assert.equal(formatMilliBRL(104990), 'R$ 104,990');
  assert.equal(formatMilliBRL(1234567), 'R$ 1.234,567');
  assert.equal(formatDateBR('2026-10-20'), '20/10/2026');
});

test('fiscal filter matches CNPJ, linked station name and place', async () => {
  const { filterFiscal } = await searchModule();
  const rows = [
    { r: '1'.repeat(24), c: '04218406000104', st: 'Pago', n: 'Posto Alfa', m: '5103403', u: 'MT' },
    { r: '2'.repeat(24), c: '11222333000181', st: 'Aguardando' },
  ];
  assert.deepEqual(filterFiscal(rows, '11.222', null).map((r) => r.st), ['Aguardando']);
  assert.deepEqual(filterFiscal(rows, 'alfa', null).map((r) => r.st), ['Pago']);
  assert.deepEqual(filterFiscal(rows, '', { kind: 'city', code: '5103403', uf: 'MT', label: '' }).length, 1);
});

test('search store validates paths, caches files and never builds a national URL', async () => {
  const { SearchStore } = await searchModule();
  const urls = [];
  const store = new SearchStore('/dados/busca/', 'rel', async (url) => {
    urls.push(String(url));
    if (String(url).endsWith('/indice.json')) return { ok: true, json: async () => INDEX };
    if (String(url).includes('/p/')) return { ok: false };
    return { ok: true, json: async () => ({ schema: 'abastevo-search-v1', release: 'rel', uf: 'MT', municipio: '5107925', count: 0, stations: [] }) };
  });
  await store.index();
  await store.index();
  assert.equal(urls.filter((u) => u.endsWith('indice.json')).length, 1);
  assert.equal((await store.prefix('n-zzz')).stations.length, 0);
  await assert.rejects(store.prefix('../x'), /shard-key/);
  await assert.rejects(store.city('12'), /shard-city/);
  await assert.rejects(store.prices('../all'), /shard-fuel/);
  assert.equal((await store.city('5107925')).municipio, '5107925');
  assert.throws(() => new SearchStore('/dados/busca/', '../x', async () => ({ ok: false })), /shard-release/);
  assert.ok(urls.every((u) => /^\/dados\/busca\/rel\/(indice\.json|p\/n-zzz\.json|m\/5107925\.json)$/.test(u)));
});

class FakeElement {
  constructor(tagName) {
    this.tagName = tagName;
    this.textContent = '';
    this.className = '';
    this.href = '';
    this.value = '';
    this.type = '';
    this.hidden = false;
    this.disabled = false;
    this.dataset = {};
    this.attributes = {};
    this.children = [];
    this.handlers = {};
    this.selectors = {};
  }
  append(...nodes) { this.children.push(...nodes); }
  replaceChildren(...nodes) { this.children = nodes; }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  querySelector(selector) { return this.selectors[selector] || null; }
  addEventListener(type, handler) { (this.handlers[type] ||= []).push(handler); }
  dispatch(type, event = {}) {
    for (const handler of this.handlers[type] || []) handler({ preventDefault() {}, ...event });
  }
}

function fakePage(rootSelector, parts) {
  const root = new FakeElement('section');
  root.dataset = { shardBase: '/dados/busca/', release: 'rel' };
  const elements = {};
  for (const [selector, tag] of Object.entries(parts)) {
    elements[selector] = new FakeElement(tag);
    root.selectors[selector] = elements[selector];
  }
  const doc = {
    querySelector: (selector) => (selector === rootSelector ? root : null),
    createElement: (tag) => new FakeElement(tag),
    addEventListener: () => {},
  };
  return { root, doc, el: elements };
}

const FINDER = {
  '[data-finder-input]': 'input',
  '[data-finder-suggestions]': 'ul',
  '[data-finder-scope]': 'div',
};

const wait = (ms = 30) => new Promise((resolve) => setTimeout(resolve, ms));

function jsonFetch(map, urls) {
  return async (url) => {
    urls.push(String(url));
    const body = map[String(url)];
    return body ? { ok: true, json: async () => body } : { ok: false };
  };
}

test('consultar searches nationally by name without choosing a state', async () => {
  const { setupPortalSearch } = await searchModule();
  const page = fakePage('[data-portal-finder]', { ...FINDER, '[data-finder-status]': 'p', '[data-finder-results]': 'ul' });
  const urls = [];
  setupPortalSearch({
    document: page.doc,
    search: '',
    fetch: jsonFetch({
      '/dados/busca/rel/indice.json': INDEX,
      '/dados/busca/rel/p/n-she.json': { schema: 'abastevo-search-prefix-v1', release: 'rel', key: 'n-she', count: 2, stations: [
        station({ n: 'Auto Posto Shell Centro', s: 'auto posto shell centro', mn: 'Sorriso', u: 'MT' }),
        station({ i: '00000000-0000-4000-8000-000000000002', n: 'Shell Avenida', s: 'shell avenida', mn: 'Curitiba', u: 'PR' }),
      ] },
    }, urls),
  });
  page.el['[data-finder-input]'].value = 'shell cent';
  page.root.dispatch('submit');
  await wait();
  assert.deepEqual(urls, ['/dados/busca/rel/indice.json', '/dados/busca/rel/p/n-she.json']);
  const results = page.el['[data-finder-results]'].children;
  assert.equal(results.length, 1);
  assert.equal(results[0].children[0].textContent, 'Auto Posto Shell Centro');
  assert.match(results[0].children[1].textContent, /Sorriso\/MT/);
  assert.match(page.el['[data-finder-status]'].textContent, /1 posto encontrado no Brasil/);
});

test('consultar applies the landing hand-off as a state scope', async () => {
  const { setupPortalSearch } = await searchModule();
  const page = fakePage('[data-portal-finder]', { ...FINDER, '[data-finder-status]': 'p', '[data-finder-results]': 'ul' });
  const urls = [];
  setupPortalSearch({
    document: page.doc,
    search: '?uf=SP&q=alfa',
    fetch: jsonFetch({ '/dados/busca/rel/SP.json': shard([station(), station({ i: '00000000-0000-4000-8000-000000000003', n: 'Posto Beta', s: 'posto beta' })]) }, urls),
  });
  await wait();
  assert.deepEqual(urls, ['/dados/busca/rel/SP.json']);
  assert.equal(page.el['[data-finder-input]'].value, 'alfa');
  assert.match(page.el['[data-finder-scope]'].children[0].textContent, /Estado: São Paulo \(SP\)/);
  assert.equal(page.el['[data-finder-results]'].children.length, 1);
});

test('prices explorer ranks one fuel and links each price to its station', async () => {
  const { setupPortalSearch } = await searchModule();
  const page = fakePage('[data-portal-prices]', {
    ...FINDER,
    '[data-prices-fuel]': 'select',
    '[data-prices-order]': 'select',
    '[data-prices-status]': 'p',
    '[data-prices-table]': 'table',
    '[data-prices-body]': 'tbody',
  });
  page.el['[data-prices-fuel]'].value = 'GASOLINE_REGULAR';
  page.el['[data-prices-order]'].value = 'desc';
  const urls = [];
  setupPortalSearch({
    document: page.doc,
    fetch: jsonFetch({ '/dados/busca/rel/precos/GASOLINE_REGULAR.json': {
      schema: 'abastevo-search-prices-v1', release: 'rel', fuel: 'GASOLINE_REGULAR', label: 'Gasolina comum', unit: 'L', count: 2,
      rows: [
        { i: '00000000-0000-4000-8000-000000000001', n: 'Barato', u: 'SP', mn: 'São Paulo', v: 5490, d: '2026-10-20' },
        { i: '00000000-0000-4000-8000-000000000002', n: 'Caro', u: 'MT', mn: 'Sorriso', v: 6990, d: '2026-10-21' },
      ],
    } }, urls),
  });
  await wait();
  const rows = page.el['[data-prices-body]'].children;
  assert.equal(rows.length, 2);
  assert.equal(rows[0].children[0].children[0].textContent, 'Caro');
  assert.equal(rows[0].children[0].children[0].href, '/postos/00000000-0000-4000-8000-000000000002/');
  assert.equal(rows[0].children[2].textContent, 'R$ 6,990');
  assert.equal(page.el['[data-prices-table]'].hidden, false);
  assert.match(page.el['[data-prices-status]'].textContent, /maior preço primeiro/);
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
