import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import zlib from 'node:zlib';
import { execFileSync } from 'node:child_process';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { build } from '../scripts/build.mjs';
import { TOOL_FORMS } from '../scripts/content/data.mjs';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const ORIGIN = 'https://abastevo.com.br';

const compiledDir = fs.mkdtempSync(path.join(os.tmpdir(), 'abastevo-tools-js-'));
const localTsc = path.join(root, 'node_modules', '.bin', 'tsc');
execFileSync(fs.existsSync(localTsc) ? localTsc : 'tsc', ['--project', path.join(root, 'tsconfig.json'), '--outDir', compiledDir], { cwd: root, stdio: 'pipe' });
const tools = await import(pathToFileURL(path.join(compiledDir, 'tools.js')).href);

const outDir = fs.mkdtempSync(path.join(os.tmpdir(), 'abastevo-tools-dist-'));
build({ projectRoot: root, outDir, origin: ORIGIN });
test.after(() => {
  fs.rmSync(compiledDir, { recursive: true, force: true });
  fs.rmSync(outDir, { recursive: true, force: true });
});

const SLUGS = ['etanol-ou-gasolina', 'custo-de-viagem', 'consumo-medio', 'custo-por-km', 'encher-o-tanque'];
const page = (slug) => fs.readFileSync(path.join(outDir, 'ferramentas', slug, 'index.html'), 'utf8');

test('parseDecimalBR reads pt-BR numbers and rejects garbage', () => {
  const { parseDecimalBR } = tools;
  const cases = [
    ['6,29', 6.29], ['6,299', 6.299], [' R$ 6,29 ', 6.29], ['R$6,29', 6.29], ['0,95', 0.95], [',5', 0.5],
    ['1.234,56', 1234.56], ['1.200', 1200], ['12.500', 12500], ['1.234.567', 1234567],
    ['6.29', 6.29], ['0.500', 0.5], ['1,234.56', 1234.56], ['430', 430], ['-5', -5], ['+3', 3],
  ];
  for (const [raw, expected] of cases) assert.equal(parseDecimalBR(raw), expected, raw);
  // Price fields read a lone dot as decimal so "6.299" is a pump price, not 6299.
  assert.equal(parseDecimalBR('6.299', true), 6.299);
  assert.equal(parseDecimalBR('6.299'), 6299);
  for (const raw of ['', '   ', 'abc', '6,2,9', '1.23,4', '6.29.1', '1,23.4', '--5', '5-', '1e3', 'NaN', 'Infinity', '.', ',', '6,']) {
    assert.equal(parseDecimalBR(raw), null, `rejects ${JSON.stringify(raw)}`);
  }
});

test('pt-BR formatting is deterministic', () => {
  assert.equal(tools.formatNumberBR(1234567.891, 2), '1.234.567,89');
  assert.equal(tools.formatNumberBR(0.5, 0), '1');
  assert.equal(tools.formatNumberBR(-0.0001, 2), '0,00');
  assert.equal(tools.formatBRL(6.29), 'R$ 6,29');
  assert.equal(tools.formatBRL(4.403, 3), 'R$ 4,403');
  assert.equal(tools.formatPercent(0.6661), '66,6%');
});

test('validation: required, zero, negative, absurd, integer, choice and flag', () => {
  const price = { name: 'p', label: 'Preço da gasolina', kind: 'price', required: true, max: 30, unit: 'reais por litro', example: '6,29' };
  assert.deepEqual(tools.validateField(price, '6,29'), { ok: true, value: 6.29 });
  assert.equal(tools.validateField(price, '').error, 'Preço da gasolina: campo obrigatório.');
  assert.equal(tools.validateField(price, 'seis').error, 'Preço da gasolina: digite um número, por exemplo 6,29.');
  assert.equal(tools.validateField(price, '0').error, 'Preço da gasolina: o valor precisa ser maior que zero.');
  assert.equal(tools.validateField(price, '-1').error, 'Preço da gasolina: o valor não pode ser negativo.');
  assert.match(tools.validateField(price, '62,9').error, /valor alto demais \(máximo 30 reais por litro\)/);
  assert.deepEqual(tools.validateField({ ...price, required: false }, ''), { ok: true, value: null });
  assert.deepEqual(tools.validateField({ ...price, allowZero: true }, '0'), { ok: true, value: 0 });
  const people = { name: 'n', label: 'Pessoas', kind: 'integer', required: false, max: 50 };
  assert.equal(tools.validateField(people, '2,5').error, 'Pessoas: use um número inteiro.');
  const level = { name: 'l', label: 'Nível', kind: 'choice', required: true, choices: ['0', '25'] };
  assert.deepEqual(tools.validateField(level, '25'), { ok: true, value: 25 });
  assert.equal(tools.validateField(level, '30').error, 'Nível: escolha uma das opções.');
  const flag = { name: 'f', label: 'Ida e volta', kind: 'flag', required: false };
  assert.equal(tools.validateField(flag, 'on').value, true);
  assert.equal(tools.validateField(flag, '').value, false);
});

test('etanol ou gasolina: 70% rule, own consumption, tie and gasoline cases', () => {
  const rule = tools.compareEthanolGasoline({ gasoline: 6.29, ethanol: 4.19 });
  assert.equal(rule.choice, 'etanol');
  assert.equal(rule.threshold, 0.7);
  assert.ok(Math.abs(rule.ratio - 0.66614) < 1e-4);
  assert.ok(Math.abs(rule.maxEthanolPrice - 4.403) < 1e-9);
  const own = tools.compareEthanolGasoline({ gasoline: 6.29, ethanol: 4.19, kmPerLiterGasoline: 12.5, kmPerLiterEthanol: 8.7 });
  assert.equal(own.thresholdSource, 'consumo');
  assert.ok(Math.abs(own.threshold - 0.696) < 1e-9);
  assert.equal(tools.formatBRL(own.costPerKmEthanol, 3), 'R$ 0,482');
  assert.equal(tools.formatBRL(own.costPerKmGasoline, 3), 'R$ 0,503');
  assert.equal(tools.compareEthanolGasoline({ gasoline: 6, ethanol: 4.2 }).choice, 'empate');
  assert.equal(tools.compareEthanolGasoline({ gasoline: 6, ethanol: 4.5 }).choice, 'gasolina');
});

test('trip, consumption, cost per km and tank formulas', () => {
  const trip = tools.tripCost({ distanceKm: 430, kmPerLiter: 12, pricePerLiter: 6.29, roundTrip: true, tolls: 45.8, people: 2 });
  assert.equal(trip.totalKm, 860);
  assert.equal(tools.formatNumberBR(trip.liters, 1), '71,7');
  assert.equal(tools.formatBRL(trip.fuelCost), 'R$ 450,78');
  assert.equal(tools.formatBRL(trip.total), 'R$ 496,58');
  assert.equal(tools.formatBRL(trip.perPerson), 'R$ 248,29');
  assert.equal(tools.tripCost({ distanceKm: 100, kmPerLiter: 10, pricePerLiter: 5 }).people, 1);

  const avg = tools.averageConsumption({ distanceKm: 412, liters: 35.6, pricePerLiter: 6.29 });
  assert.equal(tools.formatNumberBR(avg.kmPerLiter, 1), '11,6');
  assert.equal(tools.formatNumberBR(avg.litersPer100Km, 1), '8,6');
  assert.equal(tools.formatBRL(avg.costPerKm, 3), 'R$ 0,544');
  assert.equal(tools.formatBRL(avg.spent), 'R$ 223,92');
  assert.equal(tools.averageConsumption({ distanceKm: 100, liters: 10 }).costPerKm, null);

  const km = tools.costPerKm({ fuelPrice: 6.29, kmPerLiter: 12, kwhPrice: 0.95, kmPerKwh: 6.5, monthlyKm: 1200 });
  assert.equal(km.cheaper, 'eletrico');
  assert.equal(tools.formatBRL(km.combustion, 3), 'R$ 0,524');
  assert.equal(tools.formatBRL(km.electric, 3), 'R$ 0,146');
  assert.equal(tools.formatBRL(km.difference, 3), 'R$ 0,378');
  assert.equal(tools.formatBRL(km.monthlyCombustion), 'R$ 629,00');
  assert.equal(tools.formatBRL(km.monthlyElectric), 'R$ 175,38');

  const tank = tools.fillTank({ capacityLiters: 48, currentPercent: 25, pricePerLiter: 6.29, kmPerLiter: 12 });
  assert.equal(tank.liters, 36);
  assert.equal(tools.formatBRL(tank.cost), 'R$ 226,44');
  assert.equal(tools.formatBRL(tank.fullTankCost), 'R$ 301,92');
  assert.equal(tank.rangeKm, 576);
});

// Each page's worked example must match what the calculator actually outputs.
const WORKED_EXAMPLES = {
  'etanol-ou-gasolina': [{ gasolina: '6,29', etanol: '4,19', consumo_gasolina: '12,5', consumo_etanol: '8,7' }, ['R$ 0,482', 'R$ 0,503', '69,6%']],
  'custo-de-viagem': [{ distancia: '430', consumo: '12', preco: '6,29', ida_e_volta: 'on', pedagio: '45,80', pessoas: '2' }, ['R$ 496,58', 'R$ 248,29', 'R$ 450,78']],
  'consumo-medio': [{ distancia: '412', litros: '35,6', preco: '6,29' }, ['11,6 km/l', 'R$ 0,544', 'R$ 223,92']],
  'custo-por-km': [{ preco_combustivel: '6,29', consumo: '12', tarifa_kwh: '0,95', consumo_eletrico: '6,5', km_mes: '1.200' }, ['R$ 0,524', 'R$ 0,146', 'R$ 0,378', 'R$ 629,00', 'R$ 175,38']],
  'encher-o-tanque': [{ capacidade: '48', nivel: '25', preco: '6,29', consumo: '12' }, ['R$ 226,44', 'R$ 301,92', '576 km']],
};

test('runTool reproduces the worked example printed on each tool page', () => {
  for (const slug of SLUGS) {
    const [input, figures] = WORKED_EXAMPLES[slug];
    const outcome = tools.runTool(slug, input);
    assert.ok(outcome.ok, `${slug} computes: ${JSON.stringify(outcome)}`);
    const text = outcome.lines.join(' ');
    const html = page(slug);
    for (const figure of figures) {
      assert.ok(text.includes(figure), `${slug} result shows ${figure}: ${text}`);
      assert.ok(html.includes(figure), `${slug} page explains ${figure} without JavaScript`);
    }
  }
});

test('runTool reports field errors instead of computing', () => {
  const missing = tools.runTool('custo-de-viagem', { distancia: '', consumo: '0', preco: '99' });
  assert.equal(missing.ok, false);
  assert.deepEqual(Object.keys(missing.errors).sort(), ['consumo', 'distancia', 'preco']);
  const half = tools.runTool('etanol-ou-gasolina', { gasolina: '6,29', etanol: '4,19', consumo_gasolina: '12' });
  assert.equal(half.ok, false);
  assert.match(half.errors.consumo_etanol, /preencha os dois consumos/);
  assert.equal(tools.runTool('nao-existe', {}).ok, false);
});

test('query prefill reads only known fields with bounded length', () => {
  const spec = tools.TOOL_SPECS['etanol-ou-gasolina'];
  assert.deepEqual(tools.readPrefill('?gasolina=6,29&etanol=4,19&x=<script>', spec), { gasolina: '6,29', etanol: '4,19' });
  assert.deepEqual(tools.readPrefill(`?gasolina=${'9'.repeat(40)}`, spec), {});
  assert.deepEqual(tools.readPrefill('', spec), {});
});

test('HTML forms and TypeScript specs declare the same fields in the same order', () => {
  assert.deepEqual(Object.keys(TOOL_FORMS).sort(), Object.keys(tools.TOOL_SPECS).sort());
  for (const slug of SLUGS) {
    assert.deepEqual(TOOL_FORMS[slug].map((f) => f.name), tools.TOOL_SPECS[slug].fields.map((f) => f.name), slug);
    for (const field of TOOL_FORMS[slug]) {
      const rule = tools.TOOL_SPECS[slug].fields.find((f) => f.name === field.name);
      assert.equal(Boolean(field.required), rule.required, `${slug}.${field.name} required flag`);
    }
  }
});

function fakeDom(slug) {
  const names = tools.TOOL_SPECS[slug].fields.map((f) => f.name);
  const el = (extra = {}) => ({
    hidden: false, attrs: {}, textContent: '', children: [],
    setAttribute(n, v) { this.attrs[n] = v; if (n === 'hidden') this.hidden = true; },
    removeAttribute(n) { delete this.attrs[n]; },
    replaceChildren(...c) { this.children = c; },
    ...extra,
  });
  const doc = { focused: null, createElement: (tag) => el({ tag }) };
  const fields = Object.fromEntries(names.map((n) => [n, el({ name: n, value: '', type: 'text', focus() { doc.focused = n; } })]));
  const errors = Object.fromEntries(names.map((n) => [n, el({ hidden: true })]));
  const result = el();
  const fallback = el();
  const panel = el({ querySelector: (s) => (s === '[data-tool-result]' ? result : s === '[data-tool-fallback]' ? fallback : null) });
  const listeners = {};
  const form = el({
    hidden: true,
    dataset: { tool: slug },
    elements: { namedItem: (n) => fields[n] || null },
    querySelector: (s) => errors[/data-error-for="([^"]+)"/.exec(s)?.[1]] || null,
    closest: () => panel,
    addEventListener: (type, fn) => { listeners[type] = fn; },
    ownerDocument: doc,
  });
  doc.querySelectorAll = () => [form];
  return { doc, form, fields, errors, result, fallback, listeners };
}

test('enhancement: reveals the form, applies prefill, renders text-only results and errors', () => {
  const dom = fakeDom('etanol-ou-gasolina');
  tools.setupTools(dom.doc, '?gasolina=6,29&etanol=4,19');
  assert.equal(dom.form.hidden, false, 'form revealed');
  assert.equal(dom.fallback.hidden, true, 'no-JS fallback hidden');
  assert.equal(dom.fields.gasolina.value, '6,29');
  assert.match(dom.result.children[0].textContent, /etanol tende a sair mais barato/);
  assert.ok(dom.result.children.every((p) => p.tag === 'p' && !('innerHTML' in p)), 'results use textContent only');

  dom.fields.gasolina.value = '0';
  let prevented = false;
  dom.listeners.submit({ preventDefault: () => { prevented = true; } });
  assert.ok(prevented, 'submit never navigates');
  assert.equal(dom.fields.gasolina.attrs['aria-invalid'], 'true');
  assert.equal(dom.errors.gasolina.hidden, false);
  assert.match(dom.errors.gasolina.textContent, /maior que zero/);
  assert.equal(dom.doc.focused, 'gasolina', 'focus moves to the first invalid field');
  assert.match(dom.result.children[0].textContent, /Revise os campos/);
});

test('built tool pages: no-JS explanation, accessible form, JSON-LD, links and CSP-safe module', () => {
  const hub = fs.readFileSync(path.join(outDir, 'ferramentas', 'index.html'), 'utf8');
  const scriptNames = new Set();
  for (const slug of SLUGS) {
    const html = page(slug);
    const route = `/ferramentas/${slug}/`;
    assert.ok(hub.includes(`href="${route}"`), `hub links ${route}`);
    assert.ok(html.includes(`<link rel="canonical" href="${ORIGIN}${route}">`), `${slug} canonical is the clean URL`);
    assert.match(html, new RegExp(`<form class="tool-form" data-tool="${slug}" novalidate hidden>`), 'form hidden until enhanced');
    assert.match(html, /data-tool-fallback>A calculadora interativa precisa de JavaScript/);
    assert.match(html, /<h2 id="como-o-calculo-funciona">/, 'formula explained in HTML');
    assert.match(html, /<h2 id="exemplo-resolvido">/, 'worked example in HTML');
    assert.match(html, /<div class="tool-result" data-tool-result role="status" aria-live="polite"><\/div>/);

    // Every control has a label and valid aria-describedby targets.
    for (const control of html.matchAll(/<(input|select) id="([^"]+)"[^>]*>/g)) {
      assert.ok(html.includes(`<label for="${control[2]}">`), `${slug} label for ${control[2]}`);
      const described = control[0].match(/aria-describedby="([^"]+)"/);
      if (described) for (const id of described[1].split(' ')) assert.ok(html.includes(`id="${id}"`), `${slug} describedby ${id}`);
    }
    for (const field of TOOL_FORMS[slug]) assert.ok(html.includes(`name="${field.name}"`), `${slug} field ${field.name}`);
    assert.match(html, /inputmode="decimal"/, 'decimal keyboard on mobile');

    const app = [...html.matchAll(/<script type="application\/ld\+json">([\s\S]*?)<\/script>/g)]
      .flatMap((m) => JSON.parse(m[1])['@graph'])
      .find((n) => n['@type'] === 'WebApplication');
    assert.ok(app, `${slug} WebApplication JSON-LD`);
    assert.equal(app.url, `${ORIGIN}${route}`);
    assert.equal(app.offers.price, '0');
    assert.equal(app.isAccessibleForFree, true);

    const others = html.match(/<h2 id="outras-ferramentas">[\s\S]*?<\/section>/)[0];
    for (const other of SLUGS.filter((s) => s !== slug)) assert.ok(others.includes(`href="/ferramentas/${other}/"`), `${slug} links ${other}`);
    assert.ok(html.includes('href="/precos/"'), `${slug} links /precos/`);
    assert.match(html.match(/<h2 id="guias-relacionados">[\s\S]*?<\/section>/)[0], /href="\/guias\//, `${slug} related guides`);

    const scripts = [...html.matchAll(/<script type="module" src="\/assets\/([^"]+)"><\/script>/g)].map((m) => m[1]);
    const toolsScript = scripts.find((s) => s.startsWith('tools.'));
    assert.match(toolsScript, /^tools\.[a-f0-9]{8}\.js$/, 'fingerprinted calculators module');
    scriptNames.add(toolsScript);
    assert.doesNotMatch(html, /<script(?![^>]*(?:type="application\/ld\+json"|src="))[^>]*>/, 'no inline executable script');
    assert.doesNotMatch(html, /\sstyle="/, 'no inline styles (CSP style-src self)');
  }
  assert.equal(scriptNames.size, 1, 'one shared calculators module');
  const [toolsFile] = scriptNames;
  const bytes = fs.readFileSync(path.join(outDir, 'assets', toolsFile));
  assert.ok(zlib.gzipSync(bytes).length <= 10 * 1024, 'calculators JS within 10 KiB gzip');
  assert.ok(!fs.readFileSync(path.join(outDir, 'index.html'), 'utf8').includes(toolsFile), 'home does not load the calculators');
});
