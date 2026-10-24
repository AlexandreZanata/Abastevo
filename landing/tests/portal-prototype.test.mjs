import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const prototype = path.join(root, 'prototype');

const pages = [
  'consultar/index.html',
  'postos/00000000-0000-4000-8000-000000000001/index.html',
  'postos/00000000-0000-4000-8000-000000000002/index.html',
  'precos/index.html',
  'metodologia/index.html',
  'fiscalizacao/index.html',
];

function read(rel) {
  return fs.readFileSync(path.join(prototype, rel), 'utf8');
}

test('prototype pages are static, labelled synthetic and JS-independent', () => {
  for (const rel of pages) {
    const html = read(rel);
    assert.match(html, /<html lang="pt-BR">/, `${rel} language`);
    assert.match(html, /name="viewport"/, `${rel} viewport`);
    assert.match(html, /class="skip"/, `${rel} skip link`);
    assert.match(html, /id="conteudo"/, `${rel} main landmark`);
    assert.equal((html.match(/<h1[\s>]/g) || []).length, 1, `${rel} single h1`);
    assert.match(html, /dados sintéticos/i, `${rel} synthetic label`);
    assert.doesNotMatch(html, /<script\b/i, `${rel} must not require JavaScript`);
    assert.doesNotMatch(html, /fetch\s*\(|XMLHttpRequest|\/v1\//, `${rel} must not call an API`);
    assert.doesNotMatch(html, /\b\d{3}\.\d{3}\.\d{3}-\d{2}\b/, `${rel} must not contain CPF`);
  }
});

test('consultar template keeps static results, search form and honest empty/stale states', () => {
  const html = read('consultar/index.html');
  assert.match(html, /<form[^>]*>/);
  assert.match(html, /method="get"/);
  assert.match(html, /action="\/consultar\/"/);
  assert.match(html, /name="q"/);
  assert.match(html, /name="uf"/);
  assert.match(html, /name="municipio"/);
  assert.match(html, /Sem preço ANP nesta pesquisa/);
  assert.match(html, /não é preço ao vivo/i);
  assert.match(html, /Fonte:<\/strong>\s*ANP/);
  assert.match(html, /\.\.\/postos\/00000000-0000-4000-8000-000000000001\//);
  assert.match(html, /\.\.\/postos\/00000000-0000-4000-8000-000000000002\//);
  assert.doesNotMatch(html, /mais barato|preço atual|atualmente/i);
});

test('station detail keeps identity, exact period, gaps and source provenance', () => {
  const html = read('postos/00000000-0000-4000-8000-000000000001/index.html');
  assert.match(html, /00000000-0000-4000-8000-000000000001/);
  assert.match(html, /04\.218\.406\/0001-04/);
  assert.match(html, /Ativo \(informado pela fonte\)/);
  assert.match(html, /R\$ 5,899/);
  assert.match(html, /04\/10\/2026 a 10\/10\/2026/);
  assert.match(html, /Sem observação nesta pesquisa/);
  assert.match(html, /Fonte:<\/strong>\s*ANP/);
  assert.match(html, /checksum/);
  assert.match(html, /Não afiliado à ANP/);
  assert.doesNotMatch(html, /mais barato|preço atual|atualmente/i);
});

test('suspended status is source-reported, not a quality score', () => {
  const html = read('postos/00000000-0000-4000-8000-000000000002/index.html');
  assert.match(html, /Suspenso \(informado pela fonte\)/);
  assert.match(html, /não é um score de qualidade/i);
  assert.match(html, /preços históricos não provam operação atual/i);
});

test('regional summary shows sample, derived label and insufficient-sample state', () => {
  const html = read('precos/index.html');
  assert.match(html, /4 postos/);
  assert.match(html, /5 observações/);
  assert.match(html, /R\$ 5,799/);
  assert.match(html, /R\$ 6,099/);
  assert.match(html, /R\$ 0,300/);
  assert.match(html, /Resumo derivado Abastevo/);
  assert.match(html, /não é uma estatística oficial da ANP/i);
  assert.match(html, /Amostra insuficiente/);
  assert.doesNotMatch(html, /mais barato|preço atual|atualmente/i);
});

test('fiscalizacao is gated and contains no fabricated records', () => {
  const html = read('fiscalizacao/index.html');
  assert.match(html, /ainda não disponível/i);
  assert.match(html, /nenhum registro/i);
  assert.match(html, /não prova adulteração/i);
  assert.match(html, /não é certidão negativa/i);
  assert.doesNotMatch(html, /<table\b/i);
});

test('metodologia covers sources, clocks, rights and the precision blocker', () => {
  const html = read('metodologia/index.html');
  for (const term of [
    'Levantamento de Preços',
    'Coleta',
    'Pesquisa',
    'Publicação Abastevo',
    'amostral',
    'Cálculo Abastevo',
    'licença',
    'precisão monetária',
    'não é preço ao vivo',
  ]) {
    assert.match(html, new RegExp(term, 'i'), `metodologia missing ${term}`);
  }
});

test('prototype relative links resolve locally', () => {
  for (const rel of pages) {
    const html = read(rel);
    const dir = path.dirname(path.join(prototype, rel));
    for (const match of html.matchAll(/href="([^"]+)"/g)) {
      const href = match[1];
      if (href.startsWith('#') || href.startsWith('http://') || href.startsWith('https://') || href.startsWith('mailto:')) {
        continue;
      }
      assert.ok(href.startsWith('../'), `${rel} uses non-relative link ${href}`);
      let target = path.resolve(dir, href);
      if (href.endsWith('/')) target = path.join(target, 'index.html');
      assert.ok(fs.existsSync(target), `${rel} broken link ${href} -> ${target}`);
    }
  }
});

test('prototype CSS keeps focus, responsive and print rules', () => {
  const css = fs.readFileSync(path.join(prototype, 'assets/prototype.css'), 'utf8');
  assert.match(css, /:focus-visible/);
  assert.match(css, /@media \(max-width: 48rem\)/);
  assert.match(css, /@media print/);
});
