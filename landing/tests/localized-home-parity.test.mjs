import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { build } from '../scripts/build.mjs';

const root = path.resolve(import.meta.dirname, '..');

function builtHomes(options, verify) {
  const out = fs.mkdtempSync(path.join(os.tmpdir(), 'abastevo-home-parity-'));
  try {
    const result = build({ projectRoot: root, outDir: out, ...options });
    const read = (rel) => fs.readFileSync(path.join(out, rel), 'utf8');
    const main = (rel) => read(rel).match(/<main\b[\s\S]*?<\/main>/)?.[0] || '';
    verify(main('index.html'), main('en/index.html'), main('es/index.html'), read(`assets/${result.fingerprintedJs}`), read);
  } finally {
    fs.rmSync(out, { recursive: true, force: true });
  }
}

test('localized homes retain the Portuguese modules and usable controls without JavaScript', () =>
  builtHomes({}, (pt, en, es, js) => {
    const sections = (html) => [...html.matchAll(/<section\b([^>]*)>/g)].map(([, attributes]) => attributes);
    const faqItems = (html) => [...html.matchAll(/<details class="faq-item">/g)].length;
    const tiles = (html, name) => {
      const section = html.match(new RegExp(`<section id="${name}"[\\s\\S]*?</section>`))?.[0] || '';
      return [...section.matchAll(/<li>/g)].length;
    };
    for (const [html, location, type, portuguese] of [
      [en, 'Choose a state', 'Station, CNPJ or brand', 'Search results are in Portuguese.'],
      [es, 'Elige un estado', 'Estación de servicio, CNPJ o marca', 'Los resultados de la búsqueda están en portugués.'],
    ]) {
      assert.deepEqual(sections(html), sections(pt));
      assert.equal(faqItems(html), faqItems(pt));
      assert.equal(tiles(html, 'guias-populares'), tiles(pt, 'guias-populares'));
      assert.equal(tiles(html, 'ferramentas'), tiles(pt, 'ferramentas'));
      assert.match(html, /<form class="lp-search" action="\/consultar\/" method="get"/);
      assert.equal([...html.matchAll(/<option value="[A-Z]{2}">/g)].length, 27);
      assert.ok(html.includes(`<option value="">${location}</option>`));
      assert.match(html, /<input type="search" name="q" maxlength="80" autocomplete="off"/);
      assert.ok(html.includes(`placeholder="${type}"`));
      assert.ok(html.includes(portuguese));
      assert.match(html, /data-copy-target="#btn-github" data-copy-success="[^"]+"/);
      assert.equal([...html.matchAll(/class="app-shot"/g)].length, [...pt.matchAll(/class="app-shot"/g)].length);
      assert.doesNotMatch(html, /<img[^>]*\/assets\/app\//, 'no screenshots of the app in Portuguese on a translated page');
      assert.match(html, /href="\/#aplicativo" hreflang="pt-BR"/);
      assert.match(html, /class="store-btn store-pill" aria-disabled="true"/);
      assert.doesNotMatch(html, /\{\{HOME_|Conheça o aplicativo|Perguntas frequentes|Dados abertos, direto no navegador|Copiar link/);
    }
    assert.match(en, /href="\/en\/methodology\/"/);
    assert.match(es, /href="\/es\/metodologia\/"/);
    assert.match(js, /dataset\[.copySuccess.\]/);
  }));

test('the locale homes respect the verified Play Store publication switch', () =>
  builtHomes({ storeState: 'published', storeUrl: 'https://play.google.com/store/apps/details?id=com.anpfuel' }, (pt, en, es, _js, read) => {
    for (const [html, path, label, prelaunch] of [
      [pt, 'index.html', 'Ver na Google Play', 'Em breve no Google Play'],
      [en, 'en/index.html', 'View on Google Play', 'Coming soon to Google Play'],
      [es, 'es/index.html', 'Ver en Google Play', 'Próximamente en Google Play'],
    ]) {
      assert.match(html, /href="https:\/\/play.google.com\/store\/apps\/details\?id=com.anpfuel"/);
      assert.ok(html.includes(`<strong>${label}</strong>`));
      assert.ok(!html.includes(prelaunch), 'published page drops the prelaunch pill');
      const graph = read(path).match(/<script type="application\/ld\+json">([\s\S]*?)<\/script>/)[1];
      const app = JSON.parse(graph)['@graph'].find((node) => node['@type'] === 'MobileApplication');
      assert.equal(app?.installUrl, 'https://play.google.com/store/apps/details?id=com.anpfuel');
    }
  }));
