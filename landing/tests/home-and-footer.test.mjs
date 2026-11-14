import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { build } from '../scripts/build.mjs';
import { listHtmlPages } from '../scripts/check.mjs';
import { STATES } from '../scripts/content/data.mjs';
import { loadSite } from '../scripts/content/site.mjs';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const outDir = fs.mkdtempSync(path.join(os.tmpdir(), 'abastevo-home-'));
build({ projectRoot: root, outDir });
test.after(() => fs.rmSync(outDir, { recursive: true, force: true }));
const home = fs.readFileSync(path.join(outDir, 'index.html'), 'utf8');

const UFS = ['ac', 'al', 'ap', 'am', 'ba', 'ce', 'df', 'es', 'go', 'ma', 'mt', 'ms', 'mg', 'pa', 'pb', 'pr', 'pe', 'pi', 'rj', 'rn', 'rs', 'ro', 'rr', 'sc', 'sp', 'se', 'to'];

test('state list covers the 27 federative units with lowercase codes', () => {
  assert.deepEqual(STATES.map((s) => s.uf), UFS);
  assert.equal(new Set(STATES.map((s) => s.name)).size, 27);
});

test('home "Preços por estado" grid links every /precos/{uf}/ with the full state name', () => {
  const section = home.match(/<section id="precos-por-estado"[\s\S]*?<\/section>/)?.[0];
  assert.ok(section, 'states section exists');
  assert.match(section, /<h2 class="h2">Preços por estado<\/h2>/);
  for (const { uf, name } of STATES) {
    assert.ok(section.includes(`<a href="/precos/${uf}/"><span class="state-uf" aria-hidden="true">${uf.toUpperCase()}</span><span class="state-name">${name}</span></a>`), `state ${uf}`);
  }
  assert.ok(section.includes('href="/precos/"'));
  assert.doesNotMatch(section, /R\$\s?\d/, 'no prices invented on the home');
});

test('home shows popular guides and tools that resolve to built pages', () => {
  const site = loadSite(root);
  const guides = home.match(/<section id="guias-populares"[\s\S]*?<\/section>/)[0];
  for (const entry of site.popularGuides) assert.ok(guides.includes(`href="${entry.route}"`), `popular ${entry.route}`);
  const toolsSection = home.match(/<section id="ferramentas"[\s\S]*?<\/section>/)[0];
  for (const tool of site.tools) assert.ok(toolsSection.includes(`href="${tool.route}"`), `tool ${tool.route}`);
  for (const href of [...guides.matchAll(/href="(\/[^"]+)"/g), ...toolsSection.matchAll(/href="(\/[^"]+)"/g)].map((m) => m[1])) {
    assert.ok(fs.existsSync(path.join(outDir, href.slice(1), 'index.html')), `home link ${href} resolves`);
  }
});

test('every landing page carries the shared footer hubs; no /forum/ link anywhere', () => {
  const footer = home.match(/<footer\b[\s\S]*?<\/footer>/)[0];
  const column = (label) => footer.match(new RegExp(`<nav aria-label="${label}"[^>]*>[\\s\\S]*?</nav>`))?.[0];
  const precos = column('Preços');
  assert.ok(precos.includes('<a href="/precos/">Preços por estado</a>'));
  for (const { uf, name } of STATES) assert.ok(precos.includes(`<a href="/precos/${uf}/">${name}</a>`), `footer state ${uf}`);
  const guias = column('Guias');
  assert.ok(guias.includes('href="/guias/"'));
  for (const category of loadSite(root).categories) assert.ok(guias.includes(`href="${category.route}"`));
  const ferramentas = column('Ferramentas');
  assert.ok(ferramentas.includes('href="/ferramentas/"'));
  assert.equal((ferramentas.match(/href="\/ferramentas\/[a-z-]+\/"/g) || []).length, 5);
  const projeto = column('Projeto');
  for (const href of ['/sobre/', '/politica-editorial/', 'https://github.com/AlexandreZanata/abastevo']) assert.ok(projeto.includes(`href="${href}"`), `projeto ${href}`);
  const privacidade = column('Privacidade');
  for (const href of ['/privacidade/', '/excluir-conta/', '/termos/']) assert.ok(privacidade.includes(`href="${href}"`));

  const shared = footer.replaceAll('href="#', 'href="/#');
  for (const rel of listHtmlPages(outDir)) {
    const html = fs.readFileSync(path.join(outDir, rel), 'utf8');
    const pageFooter = html.match(/<footer\b[\s\S]*?<\/footer>/)?.[0];
    assert.equal(pageFooter, rel === 'index.html' ? footer : shared, `${rel} uses the shared footer`);
    assert.doesNotMatch(html, /href="\/forum\//, `${rel} must not link /forum/ yet`);
  }
});

test('the shared header links the tools hub', () => {
  assert.match(home, /<nav class="topnav"[\s\S]*?<a href="\/ferramentas\/">Ferramentas<\/a>[\s\S]*?<\/nav>/);
  assert.match(home, /<nav class="site-nav"[\s\S]*?<a href="\/ferramentas\/">Ferramentas<\/a>[\s\S]*?<\/nav>/);
});
