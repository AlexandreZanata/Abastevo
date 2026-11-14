import test from 'node:test';
import assert from 'node:assert/strict';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { loadSite } from '../scripts/content/site.mjs';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const site = loadSite(root);

// SEO-11 wave 1 (2026-11-14): answer-first, sourced, linked, no promises.
const WAVE_1 = {
  'como-e-formado-o-preco-da-gasolina': 'precos',
  'icms-sobre-combustiveis': 'precos',
  'precos-diferentes-entre-cidades': 'precos',
  'gasolina-comum-ou-aditivada': 'escolher-combustivel',
  'diesel-s10-ou-s500': 'escolher-combustivel',
  'gnv-compensa': 'escolher-combustivel',
  'como-economizar-combustivel': 'economia',
  'como-calcular-consumo-medio': 'economia',
  'combustivel-adulterado': 'direitos-e-qualidade',
  'direitos-do-consumidor-no-posto': 'direitos-e-qualidade',
};

const OFFICIAL_HOSTS = ['www.gov.br', 'www.planalto.gov.br', 'www.confaz.fazenda.gov.br', 'www.consumidor.gov.br', 'precos.petrobras.com.br'];
const FORBIDDEN = [
  /posto mais barato perto de você/i,
  /menor preço garantido/i,
  /economia mágica/i,
  /preço (de )?hoje/i,
  /em tempo real(?! )/i,
  /milhares de usuários/i,
];

test('wave 1 ships ten guides in their planned categories', () => {
  for (const [slug, category] of Object.entries(WAVE_1)) {
    const guide = site.registry.get(`/guias/${slug}/`);
    assert.ok(guide, `${slug} exists`);
    assert.equal(guide.category, category, `${slug} category`);
    assert.equal(guide.datePublished, '2026-11-14');
    assert.equal(guide.dateModified, '2026-11-14');
  }
});

test('each wave-1 guide is 900-1,800 words, answer-first and source-cited', () => {
  for (const slug of Object.keys(WAVE_1)) {
    const guide = site.registry.get(`/guias/${slug}/`);
    assert.ok(guide.words >= 900 && guide.words <= 1800, `${slug} has ${guide.words} words`);
    assert.ok(guide.html.startsWith('<div class="guide-callout callout-resumo"><p><strong>Resposta rápida:</strong>'), `${slug} opens with the answer`);
    const external = guide.links.filter((l) => l.kind === 'external').map((l) => new URL(l.url).hostname);
    assert.ok(external.length >= 2, `${slug} cites sources`);
    for (const host of external) assert.ok(OFFICIAL_HOSTS.includes(host), `${slug} cites only official sources, got ${host}`);
    assert.ok(guide.headings.some((h) => h.level === 2 && h.id === 'fontes'), `${slug} lists its sources`);
  }
});

test('each wave-1 guide links to at least three distinct internal pages with descriptive anchors', () => {
  for (const slug of Object.keys(WAVE_1)) {
    const guide = site.registry.get(`/guias/${slug}/`);
    const internal = guide.links.filter((l) => l.kind === 'internal');
    const targets = new Set(internal.map((l) => l.url));
    assert.ok(targets.size >= 3, `${slug} links ${targets.size} internal pages`);
    assert.ok(internal.some((l) => l.url.startsWith('/ferramentas/') || l.url.startsWith('/precos/') || l.url === '/metodologia/'), `${slug} links a tool or data page`);
    for (const link of internal) {
      assert.doesNotMatch(link.text, /^(clique aqui|aqui|saiba mais|link)$/i, `${slug} anchor "${link.text}" must be descriptive`);
      assert.ok(link.text.trim().split(/\s+/).length >= 2 || /^[A-Z]{3,}$/.test(link.text), `${slug} anchor "${link.text}" too short`);
    }
  }
});

test('wave-1 guides avoid promises and real-time claims', () => {
  for (const slug of Object.keys(WAVE_1)) {
    const guide = site.registry.get(`/guias/${slug}/`);
    const text = guide.html.replace(/<[^>]+>/g, ' ');
    for (const pattern of FORBIDDEN) assert.doesNotMatch(text, pattern, `${slug} must not claim ${pattern}`);
    // Illustrative money figures are labelled as examples.
    if (/R\$ \d/.test(text)) assert.match(text, /ilustrativ|hipotétic|exemplo/i, `${slug} labels its example prices`);
  }
});
