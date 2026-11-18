import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { buildSite } from '../scripts/build.mjs';
import { checkDist } from '../scripts/check.mjs';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const translationsDir = path.join(root, 'tests', 'fixtures', 'i18n');
const origin = 'https://abastevo.com.br';

function buildFixture(outDir) {
  buildSite({
    projectRoot: root,
    outDir,
    canonicalOrigin: origin,
    storeState: 'prelaunch',
    enableLocales: ['en', 'es'],
    translationsDir,
  });
}

function read(outDir, rel) {
  return fs.readFileSync(path.join(outDir, rel), 'utf8');
}

function hreflangs(html) {
  return [...html.matchAll(/<link\s+rel="alternate"\s+hreflang="([^"]+)"\s+href="([^"]+)">/g)]
    .map((match) => ({ hreflang: match[1], route: match[2].slice(origin.length) }));
}

test('multilingual build: reviewed pages, hubs, sitemap, head and switcher', () => {
  const outDir = path.join(os.tmpdir(), 'abastevo-i18n-build-' + Math.random().toString(36).slice(2, 8));
  try {
    buildFixture(outDir);

    for (const rel of [
      'en/guides/ethanol-or-gasoline/index.html',
      'en/guides/index.html',
      'en/tools/ethanol-or-gasoline-calculator/index.html',
      'en/tools/index.html',
      'es/guias/etanol-o-gasolina/index.html',
      'es/guias/index.html',
    ]) {
      assert.ok(fs.existsSync(path.join(outDir, rel)), `${rel} is emitted`);
    }
    assert.ok(!fs.existsSync(path.join(outDir, 'en/guides/draft-guide/index.html')), 'draft is never emitted');

    const sitemap = read(outDir, 'sitemap.xml');
    for (const route of ['/en/guides/ethanol-or-gasoline/', '/es/guias/etanol-o-gasolina/', '/en/tools/ethanol-or-gasoline-calculator/', '/en/guides/', '/es/guias/', '/en/tools/']) {
      assert.ok(sitemap.includes(`<loc>${origin}${route}</loc>`), `sitemap lists ${route}`);
    }
    assert.ok(!sitemap.includes('/es/guias/stale-guide/'), 'stale page stays out of the sitemap');
    assert.ok(!sitemap.includes('/en/guides/draft-guide/'), 'draft stays out of the sitemap');
    assert.match(sitemap, /<loc>https:\/\/abastevo\.com\.br\/en\/guides\/ethanol-or-gasoline\/<\/loc>\s*<lastmod>2026-11-17<\/lastmod>/);

    // Stale translation: emitted, noindex, visible note, no hreflang/canonical.
    const stale = read(outDir, 'es/guias/stale-guide/index.html');
    assert.ok(stale.includes('<meta name="robots" content="noindex,follow">'), 'stale page is noindex,follow');
    assert.ok(stale.includes('El original en portugués se actualizó el 17 de noviembre de 2026.'), 'stale note is visible in Spanish');
    assert.equal(hreflangs(stale).length, 0, 'stale page has no hreflang');
    assert.ok(!stale.includes('rel="canonical"'), 'noindex stale page has no canonical');

    // Reviewed group: pt-BR + en + es, reciprocal, with x-default and og:locale:alternate.
    const pt = read(outDir, 'guias/etanol-ou-gasolina/index.html');
    const en = read(outDir, 'en/guides/ethanol-or-gasoline/index.html');
    const es = read(outDir, 'es/guias/etanol-o-gasolina/index.html');
    const expectedGroup = [
      ['pt-BR', '/guias/etanol-ou-gasolina/'],
      ['en', '/en/guides/ethanol-or-gasoline/'],
      ['es', '/es/guias/etanol-o-gasolina/'],
    ];
    for (const [rel, html] of [['pt-BR', pt], ['en', en], ['es', es]]) {
      assert.deepEqual(hreflangs(html).filter((link) => link.hreflang !== 'x-default'), expectedGroup.map(([hreflang, route]) => ({ hreflang, route })), `${rel} lists the whole group`);
      assert.deepEqual(hreflangs(html).filter((link) => link.hreflang === 'x-default'), [{ hreflang: 'x-default', route: '/guias/etanol-ou-gasolina/' }], `${rel} x-default`);
      assert.ok(html.indexOf('hreflang="x-default"') < html.indexOf('</head>'), `${rel} hreflang sits before </head>`);
    }
    assert.ok(en.includes('<meta property="og:locale:alternate" content="pt_BR">') && en.includes('<meta property="og:locale:alternate" content="es_ES">'));
    assert.ok(en.includes('<nav class="lang-switch" aria-label="Languages">\n        <div class="shell">'));
    assert.ok(!en.includes('/guias/feed.xml'), 'translated guide does not advertise the Portuguese feed');
    assert.ok(en.includes('<a href="/guias/etanol-ou-gasolina/" lang="pt-BR" hreflang="pt-BR">Português</a>'));
    assert.ok(en.includes('<a href="/es/guias/etanol-o-gasolina/" lang="es" hreflang="es">Español</a>'));
    assert.ok(pt.includes('<a href="/en/guides/ethanol-or-gasoline/" lang="en" hreflang="en">English</a>'), 'pt-BR page links its translations');

    // Locale head and dates.
    assert.ok(en.includes('<html lang="en">'));
    assert.ok(en.includes('<meta property="og:locale" content="en_US">'));
    assert.ok(en.includes('"inLanguage": "en"'));
    assert.ok(en.includes('November 17, 2026'));
    assert.ok(es.includes('<html lang="es">'));
    assert.ok(es.includes('<meta property="og:locale" content="es_ES">'));
    assert.ok(es.includes('17 de noviembre de 2026'));

    // English chrome and content; no Portuguese chrome strings.
    for (const text of ['Skip to content', 'Breadcrumb', 'In this guide', 'Keep exploring', 'Read also', 'Updated on']) {
      assert.ok(en.includes(text), `English chrome has "${text}"`);
    }
    for (const text of ['Pular para o conteúdo', 'Navegação estrutural', 'Neste guia', 'Continue explorando', 'Leia também', 'Atualizado em']) {
      assert.ok(!en.includes(text), `English page has no Portuguese chrome string "${text}"`);
    }
    assert.ok(en.includes('Ethanol pays off when its price is below 70%'), 'article body is in the HTML without JavaScript');

    const tool = read(outDir, 'en/tools/ethanol-or-gasoline-calculator/index.html');
    assert.ok(tool.includes('Gasoline price (BRL/liter)'));
    assert.ok(tool.includes('Calculate'));
    assert.ok(tool.includes('The math runs in your browser.'));
    assert.ok(!tool.includes('Preço da gasolina'));
    assert.ok(tool.includes('data-tool="etanol-ou-gasolina"'), 'calculator keeps its pt-BR form id');
    assert.ok(tool.includes('class="lang-switch"'));

    // Hubs list emitted entries only.
    const enGuidesHub = read(outDir, 'en/guides/index.html');
    assert.ok(enGuidesHub.includes('/en/guides/ethanol-or-gasoline/'));
    assert.ok(!enGuidesHub.includes('draft-guide'));
    assert.ok(!enGuidesHub.includes('/guias/feed.xml'), 'translated guides hub omits the Portuguese feed');
    assert.ok(pt.includes('/guias/feed.xml'), 'pt-BR guide keeps the Portuguese feed link');
    const esGuidesHub = read(outDir, 'es/guias/index.html');
    assert.ok(esGuidesHub.includes('/es/guias/etanol-o-gasolina/'));
    assert.ok(esGuidesHub.includes('/es/guias/stale-guide/'), 'stale translation is still emitted and listed');
    assert.ok(!esGuidesHub.includes('draft-guide'));
    assert.ok(read(outDir, 'en/tools/index.html').includes('/en/tools/ethanol-or-gasoline-calculator/'));

    assert.doesNotThrow(() => checkDist({ projectRoot: root, distDir: outDir, origin }));
  } finally {
    fs.rmSync(outDir, { recursive: true, force: true });
  }
});

test('multilingual build: locales disabled leave the pt-BR output unchanged', () => {
  const first = path.join(os.tmpdir(), 'abastevo-i18n-off-1-' + Math.random().toString(36).slice(2, 8));
  const second = path.join(os.tmpdir(), 'abastevo-i18n-off-2-' + Math.random().toString(36).slice(2, 8));
  try {
    buildSite({ projectRoot: root, outDir: first, canonicalOrigin: origin, storeState: 'prelaunch' });
    buildSite({
      projectRoot: root,
      outDir: second,
      canonicalOrigin: origin,
      storeState: 'prelaunch',
      enableLocales: [],
      translationsDir,
    });
    for (const rel of ['index.html', 'guias/index.html', 'guias/etanol-ou-gasolina/index.html', 'guias/icms-sobre-combustiveis/index.html', 'sitemap.xml', 'llms.txt', '404.html']) {
      assert.equal(read(second, rel), read(first, rel), `${rel} is byte-identical with locales disabled`);
    }
    assert.ok(!fs.existsSync(path.join(second, 'en')) && !fs.existsSync(path.join(second, 'es')), 'no locale directories');
    assert.ok(!read(second, 'guias/index.html').includes('hreflang'), 'pt-BR hub declares no hreflang when locales are off');
  } finally {
    fs.rmSync(first, { recursive: true, force: true });
    fs.rmSync(second, { recursive: true, force: true });
  }
});

test('multilingual check: broken hreflang and lang fail the dist check', () => {
  const outDir = path.join(os.tmpdir(), 'abastevo-i18n-check-' + Math.random().toString(36).slice(2, 8));
  const cases = [
    [
      (dir) => {
        const file = path.join(dir, 'en/guides/ethanol-or-gasoline/index.html');
        fs.writeFileSync(file, fs.readFileSync(file, 'utf8').replace(/  <link rel="alternate" hreflang="x-default"[^\n]*\n/, ''));
      },
      /needs exactly one x-default/,
    ],
    [
      (dir) => {
        const file = path.join(dir, 'en/guides/ethanol-or-gasoline/index.html');
        fs.writeFileSync(file, fs.readFileSync(file, 'utf8').replace('https://abastevo.com.br/guias/etanol-ou-gasolina/', 'https://abastevo.com.br/guias/nao-existe/'));
      },
      /points to missing page/,
    ],
    [
      (dir) => {
        const file = path.join(dir, 'en/guides/ethanol-or-gasoline/index.html');
        fs.writeFileSync(file, fs.readFileSync(file, 'utf8').replace(/<html lang="en">/, '<html lang="pt-BR">'));
      },
      /html lang pt-BR does not match \/en/,
    ],
    [
      (dir) => {
        const file = path.join(dir, 'en/guides/ethanol-or-gasoline/index.html');
        fs.writeFileSync(file, fs.readFileSync(file, 'utf8').replace(
          /<link rel="alternate" hreflang="x-default" href="[^"]+">/,
          '<link rel="alternate" hreflang="x-default" href="https://abastevo.com.br/en/guides/ethanol-or-gasoline/">',
        ));
      },
      /x-default .* must point to the unprefixed default locale/,
    ],
  ];
  try {
    buildFixture(outDir);
    for (const [mutate, pattern] of cases) {
      const copy = outDir + '-' + Math.random().toString(36).slice(2, 6);
      fs.cpSync(outDir, copy, { recursive: true });
      try {
        mutate(copy);
        assert.throws(() => checkDist({ projectRoot: root, distDir: copy, origin }), pattern);
      } finally {
        fs.rmSync(copy, { recursive: true, force: true });
      }
    }
  } finally {
    fs.rmSync(outDir, { recursive: true, force: true });
  }
});

test('draft translations: validated in content/translations, never emitted', () => {
  const outDir = path.join(os.tmpdir(), 'abastevo-i18n-drafts-' + Math.random().toString(36).slice(2, 8));
  const drafts = [
    'content/translations/en/guias/ethanol-or-gasoline.md',
    'content/translations/en/ferramentas/ethanol-or-gasoline-calculator.md',
    'content/translations/es/guias/etanol-o-gasolina.md',
    'content/translations/es/ferramentas/calculadora-etanol-o-gasolina.md',
  ];
  try {
    for (const rel of drafts) {
      const raw = fs.readFileSync(path.join(root, rel), 'utf8');
      assert.match(raw, /^translationStatus: draft$/m, `${rel} stays a draft`);
      assert.match(raw, /^translationOf: \/(guias|ferramentas)\//m, `${rel} names its pt-BR source`);
      assert.match(raw, /^sourceRevision: \d{4}-\d{2}-\d{2}$/m, `${rel} records its source revision`);
    }
    buildSite({
      projectRoot: root,
      outDir,
      canonicalOrigin: origin,
      storeState: 'prelaunch',
      enableLocales: ['en', 'es'],
    });
    assert.ok(!fs.existsSync(path.join(outDir, 'en')), 'no English output from drafts');
    assert.ok(!fs.existsSync(path.join(outDir, 'es')), 'no Spanish output from drafts');
    assert.equal((read(outDir, 'sitemap.xml').match(/<url>/g) || []).length, 49, 'sitemap keeps the pt-BR URL count');
    assert.ok(!read(outDir, 'sitemap.xml').includes('/en/') && !read(outDir, 'sitemap.xml').includes('/es/'));
  } finally {
    fs.rmSync(outDir, { recursive: true, force: true });
  }
});

test('multilingual build: stale tools, translated pages, Sources/Fuentes citations and per-locale primaryQuery', () => {
  const tmpTrans = fs.mkdtempSync(path.join(os.tmpdir(), 'abastevo-i18n-trans-'));
  const outDir = path.join(os.tmpdir(), 'abastevo-i18n-out-');
  try {
    fs.cpSync(translationsDir, tmpTrans, { recursive: true });
    fs.mkdirSync(path.join(tmpTrans, 'en', 'paginas'), { recursive: true });
    fs.mkdirSync(path.join(tmpTrans, 'es', 'ferramentas'), { recursive: true });

    // 1) Add ## Sources / ## Fuentes and share identical normalized primaryQuery across en & es.
    const enGuidePath = path.join(tmpTrans, 'en', 'guias', 'ethanol-or-gasoline.md');
    fs.writeFileSync(
      enGuidePath,
      `${fs.readFileSync(enGuidePath, 'utf8')}\n\n## Sources\n\n- [ANP — Agência Nacional do Petróleo](https://www.gov.br/anp/)\n`,
    );
    const esGuidePath = path.join(tmpTrans, 'es', 'guias', 'etanol-o-gasolina.md');
    fs.writeFileSync(
      esGuidePath,
      `${fs.readFileSync(esGuidePath, 'utf8').replace(/^title: .*$/m, '$&\nprimaryQuery: ethanol or gasoline')}\n\n## Fuentes\n\n- [ANP — regulador brasileño](https://www.gov.br/anp/)\n`,
    );

    // 2) Reviewed translated About page (inherits AboutPage schemaType from /sobre/).
    fs.writeFileSync(path.join(tmpTrans, 'en', 'paginas', 'about.md'), `---
title: "About abastevo"
headline: "About abastevo"
description: "abastevo is an independent fuel price transparency project with public ANP sources, open source code and visible collection dates."
slug: about
datePublished: 2026-11-14
dateModified: 2026-11-14
translationOf: /sobre/
sourceRevision: 2026-11-14
translationStatus: reviewed
reviewedBy: Test Reviewer
reviewedAt: 2026-11-14
---

## What abastevo is

An independent fuel price transparency project.
`);

    // 3) Stale translated calculator (sourceRevision older than /ferramentas/etanol-ou-gasolina/).
    fs.writeFileSync(path.join(tmpTrans, 'es', 'ferramentas', 'calculadora-stale.md'), `---
title: "Calculadora etanol o gasolina"
description: "Compara el precio del etanol y de la gasolina en Brasil con la regla del 70% o con el consumo real de tu propio coche flex en cada parada."
summary: "Compara el precio del etanol y la gasolina por km."
slug: calculadora-stale
datePublished: 2026-11-14
dateModified: 2026-11-16
translationOf: /ferramentas/etanol-ou-gasolina/
sourceRevision: 2026-11-16
translationStatus: reviewed
reviewedBy: Test Reviewer
reviewedAt: 2026-11-16
related:
  - /es/guias/etanol-o-gasolina/
  - /precos/
---

## Cómo funciona la cuenta

La cuenta divide el precio del etanol entre el precio de la gasolina.
`);

    buildSite({
      projectRoot: root,
      outDir,
      canonicalOrigin: origin,
      storeState: 'prelaunch',
      enableLocales: ['en', 'es'],
      translationsDir: tmpTrans,
    });

    const staleTool = read(outDir, 'es/herramientas/calculadora-stale/index.html');
    assert.ok(staleTool.includes('<meta name="robots" content="noindex,follow">'), 'stale tool is noindex,follow');
    assert.ok(!staleTool.includes('rel="canonical"'), 'stale tool omits canonical');
    assert.ok(staleTool.includes('El original en portugués se actualizó el 17 de noviembre de 2026.'), 'stale tool renders staleNote');
    assert.equal(hreflangs(staleTool).length, 0, 'stale tool omits hreflang');
    assert.ok(!read(outDir, 'sitemap.xml').includes('/es/herramientas/calculadora-stale/'), 'stale tool stays out of sitemap');

    const aboutPage = read(outDir, 'en/about/index.html');
    assert.match(aboutPage, /"@type":\s*"AboutPage"/, 'translated /sobre/ preserves AboutPage schemaType');

    const enGuide = read(outDir, 'en/guides/ethanol-or-gasoline/index.html');
    const esGuide = read(outDir, 'es/guias/etanol-o-gasolina/index.html');
    assert.match(enGuide, /"citation":\s*\[\s*\{\s*"@type":\s*"CreativeWork",\s*"name":\s*"ANP/, '## Sources populates JSON-LD citation');
    assert.match(esGuide, /"citation":\s*\[\s*\{\s*"@type":\s*"CreativeWork",\s*"name":\s*"ANP/, '## Fuentes populates JSON-LD citation');

    assert.doesNotThrow(() => checkDist({ projectRoot: root, distDir: outDir, origin }));

    // Same-locale primaryQuery collision must still fail.
    const enToolPath = path.join(tmpTrans, 'en', 'ferramentas', 'ethanol-or-gasoline-calculator.md');
    fs.writeFileSync(
      enToolPath,
      fs.readFileSync(enToolPath, 'utf8').replace(/^title: .*$/m, '$&\nprimaryQuery: ethanol or gasoline'),
    );
    assert.throws(() => buildSite({
      projectRoot: root,
      outDir,
      canonicalOrigin: origin,
      storeState: 'prelaunch',
      enableLocales: ['en', 'es'],
      translationsDir: tmpTrans,
    }), /primaryQuery "ethanol or gasoline" collides/);
  } finally {
    fs.rmSync(tmpTrans, { recursive: true, force: true });
    fs.rmSync(outDir, { recursive: true, force: true });
  }
});
