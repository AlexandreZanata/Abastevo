import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { buildSite } from '../scripts/build.mjs';
import { checkDist } from '../scripts/check.mjs';
import { loadLocales } from '../scripts/content/i18n.mjs';

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

function headerLanguagePicker(html) {
  const header = html.match(/<header\b[\s\S]*?<\/header>/)?.[0];
  return header?.match(/<details class="language-picker"[^>]*>[\s\S]*?<\/details>/)?.[0] || '';
}

test('multilingual build: reviewed pages, hubs, sitemap, head and switcher', () => {
  const outDir = path.join(os.tmpdir(), 'abastevo-i18n-build-' + Math.random().toString(36).slice(2, 8));
  try {
    buildFixture(outDir);

    for (const rel of [
      'en/index.html',
      'es/index.html',
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
    for (const route of ['/en/', '/es/', '/en/guides/ethanol-or-gasoline/', '/es/guias/etanol-o-gasolina/', '/en/tools/ethanol-or-gasoline-calculator/', '/en/guides/', '/es/guias/', '/en/tools/']) {
      assert.ok(sitemap.includes(`<loc>${origin}${route}</loc>`), `sitemap lists ${route}`);
    }
    // Home hreflang group across /, /en/, /es/ and localized home copy.
    const homePt = read(outDir, 'index.html');
    const homeEn = read(outDir, 'en/index.html');
    const homeEs = read(outDir, 'es/index.html');
    const homes = [
      { code: 'pt-BR', route: '/', name: 'Português', html: homePt, title: 'Mais informação para quem vive na estrada.' },
      { code: 'en', route: '/en/', name: 'English', html: homeEn, title: 'More information for life on the road.' },
      { code: 'es', route: '/es/', name: 'Español', html: homeEs, title: 'Más información para quien vive en la ruta.' },
    ];
    for (const { code, route, name, html, title } of homes) {
      assert.deepEqual(
        hreflangs(html).filter((link) => link.hreflang !== 'x-default'),
        [{ hreflang: 'pt-BR', route: '/' }, { hreflang: 'en', route: '/en/' }, { hreflang: 'es', route: '/es/' }],
        `${route} home hreflang group`,
      );
      assert.deepEqual(hreflangs(html).filter((link) => link.hreflang === 'x-default'), [{ hreflang: 'x-default', route: '/' }], `${route} x-default`);
      assert.ok(html.includes(`<h1 class="lp-title">${title}</h1>`), `${route} uses its localized heading`);
      const picker = headerLanguagePicker(html);
      assert.ok(picker.includes(`lang="${code}" aria-current="true">${name}</span>`), `${route} marks its current language`);
      for (const other of homes.filter((home) => home.code !== code)) {
        assert.ok(picker.includes(`href="${other.route}" lang="${other.code}" hreflang="${other.code}">${other.name}</a>`), `${route} can switch to ${other.route}`);
      }
      assert.doesNotMatch(html, /class="lang-switch"/, `${route} has one language picker in its header`);
    }
    assert.ok(!homeEn.includes('Pular para o conteúdo') && !homeEn.includes('Mais informação'));
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
    assert.ok(en.includes('<meta property="og:locale:alternate" content="pt_BR">') && en.includes('<meta property="og:locale:alternate" content="es_419">'));
    assert.match(headerLanguagePicker(en), /<summary aria-label="Languages · English">/);
    assert.ok(!en.includes('/guias/feed.xml'), 'translated guide does not advertise the Portuguese feed');
    assert.ok(headerLanguagePicker(en).includes('<a href="/guias/etanol-ou-gasolina/" lang="pt-BR" hreflang="pt-BR">Português</a>'));
    assert.ok(headerLanguagePicker(en).includes('<a href="/es/guias/etanol-o-gasolina/" lang="es" hreflang="es">Español</a>'));
    assert.ok(headerLanguagePicker(pt).includes('<a href="/en/guides/ethanol-or-gasoline/" lang="en" hreflang="en">English</a>'), 'pt-BR page links its translations');

    // Locale head and dates.
    assert.ok(en.includes('<html lang="en">'));
    assert.ok(en.includes('<meta property="og:locale" content="en_US">'));
    assert.ok(en.includes('"inLanguage": "en"'));
    assert.ok(en.includes('November 17, 2026'));
    assert.ok(es.includes('<html lang="es">'));
    assert.ok(es.includes('<meta property="og:locale" content="es_419">'));
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
    assert.ok(headerLanguagePicker(tool).includes('<a href="/es/" lang="es" hreflang="es">Español</a>'), 'untranslated locale in switcher falls back to that locale home');

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
    buildSite({ projectRoot: root, outDir: first, canonicalOrigin: origin, storeState: 'prelaunch', enableLocales: [] });
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
    const picker = headerLanguagePicker(read(second, 'index.html'));
    assert.match(picker, /lang="pt-BR" aria-current="true">Português<\/span>/);
    assert.doesNotMatch(picker, /href=/, 'the header does not link to unreleased languages');
    assert.ok(!read(second, 'guias/index.html').includes('hreflang'), 'pt-BR hub declares no hreflang when locales are off');
  } finally {
    fs.rmSync(first, { recursive: true, force: true });
    fs.rmSync(second, { recursive: true, force: true });
  }
});

test('localized guide navigation restores the Portuguese navigation groups and category trail', () => {
  const outDir = fs.mkdtempSync(path.join(os.tmpdir(), 'abastevo-i18n-navigation-'));
  try {
    buildFixture(outDir);
    const englishHub = read(outDir, 'en/guides/index.html');
    const spanishHub = read(outDir, 'es/guias/index.html');
    assert.match(englishHub, /<nav class="category-nav"[^>]*>[\s\S]*?href="\/en\/guides\/choosing-fuel\//);
    assert.match(spanishHub, /<nav class="category-nav"[^>]*>[\s\S]*?href="\/es\/guias\/elegir-combustible\//);
    const englishCategory = read(outDir, 'en/guides/choosing-fuel/index.html');
    assert.match(englishCategory, /href="\/en\/guides\/ethanol-or-gasoline\//);
    assert.deepEqual(hreflangs(englishCategory).filter((link) => link.hreflang !== 'x-default'), [
      { hreflang: 'pt-BR', route: '/guias/escolher-combustivel/' },
      { hreflang: 'en', route: '/en/guides/choosing-fuel/' },
      { hreflang: 'es', route: '/es/guias/elegir-combustible/' },
    ]);
    assert.ok(!fs.existsSync(path.join(outDir, 'es/guias/precios-e-impuestos/index.html')), 'a stale-only category is not published');
    const guide = read(outDir, 'en/guides/ethanol-or-gasoline/index.html');
    const breadcrumb = guide.match(/<nav class="breadcrumb-nav"[\s\S]*?<\/nav>/)?.[0];
    assert.match(breadcrumb, /href="\/en\/"[\s\S]*?href="\/en\/guides\/"[\s\S]*?href="\/en\/guides\/choosing-fuel\//);
    const header = guide.match(/<header class="topbar">[\s\S]*?<\/header>/)?.[0];
    assert.match(header, /href="\/en\/#aplicativo"/);
    assert.match(header, /href="\/como-usar\/" hreflang="pt-BR"/);
    assert.match(header, /class="pill-light hide-sm" href="\/consultar\/" hreflang="pt-BR"/);
    const footer = guide.match(/<footer class="footer">[\s\S]*?<\/footer>/)?.[0];
    assert.equal((footer.match(/<nav aria-label=/g) || []).length, 5);
    assert.match(footer, /href="\/en\/fuel-prices\/sp\//);
    assert.match(footer, /href="\/contribuidores\/" hreflang="pt-BR"/);
    assert.match(footer, /href="\/excluir-conta\/" hreflang="pt-BR"/);
    assert.match(footer, /class="footer-legal"/);
    const calculatorHub = read(outDir, 'en/tools/index.html');
    const related = calculatorHub.match(/<h2 id="guias-relacionados">[\s\S]*?<\/ul>/)?.[0];
    assert.match(related, /href="\/en\/guides\/ethanol-or-gasoline\/"/);
    assert.match(related, /href="\/guias\/como-economizar-combustivel\/" hreflang="pt-BR"/);
  } finally {
    fs.rmSync(outDir, { recursive: true, force: true });
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
        const file = path.join(dir, 'en/index.html');
        fs.writeFileSync(file, fs.readFileSync(file, 'utf8').replace('id="aplicativo"', 'id="app-unavailable"'));
      },
      /links to a missing home anchor \/en\/#aplicativo/,
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

test('production locales: every translation is AI-reviewed, fresh and emitted with hreflang', () => {
  const outDir = path.join(os.tmpdir(), 'abastevo-i18n-production-' + Math.random().toString(36).slice(2, 8));
  const production = {
    en: {
      prefix: '/en',
      portal: ['/en/fuel-prices/', '/en/methodology/', '/en/data/'],
      guias: ['guides', [
        'anp-weekly-price-survey', 'diesel-s10-or-s500', 'ethanol-or-gasoline', 'fuel-station-consumer-rights',
        'how-gasoline-prices-are-formed', 'how-to-read-fuel-prices', 'icms-fuel-tax-explained', 'is-cng-worth-it',
        'measure-fuel-economy', 'recognize-and-report-adulterated-fuel', 'regular-or-additive-gasoline', 'save-fuel',
        'why-fuel-prices-vary-by-city',
      ]],
      categories: ['prices-and-taxes', 'choosing-fuel', 'fuel-economy', 'rights-and-quality', 'data-and-transparency'],
      ferramentas: ['tools', ['cost-per-km-calculator', 'ethanol-or-gasoline-calculator', 'fill-up-cost-calculator', 'fuel-consumption-calculator', 'trip-fuel-cost-calculator']],
      paginas: ['', ['about', 'editorial-policy']],
      disclosure: 'This English version was translated and reviewed with AI against the Portuguese original, which prevails.',
    },
    es: {
      prefix: '/es',
      portal: ['/es/precios/', '/es/metodologia/', '/es/datos/'],
      guias: ['guias', [
        'ahorrar-combustible', 'calcular-consumo-medio', 'como-leer-los-precios', 'como-se-forma-el-precio-de-la-gasolina',
        'conviene-el-gnv', 'derechos-del-consumidor-en-la-estacion', 'diesel-s10-o-s500', 'encuesta-semanal-anp',
        'etanol-o-gasolina', 'gasolina-regular-o-con-aditivos', 'icms-sobre-combustibles',
        'identificar-y-denunciar-combustible-adulterado', 'por-que-varian-los-precios-entre-ciudades',
      ]],
      categories: ['precios-e-impuestos', 'elegir-combustible', 'ahorro-de-combustible', 'derechos-y-calidad', 'datos-y-transparencia'],
      ferramentas: ['herramientas', ['calculadora-consumo-medio', 'calculadora-costo-de-viaje', 'calculadora-costo-por-km', 'calculadora-etanol-o-gasolina', 'calculadora-llenar-el-tanque']],
      paginas: ['', ['politica-editorial', 'sobre']],
      disclosure: 'Esta versión en español fue traducida y revisada con IA a partir del original en portugués, que prevalece.',
    },
  };
  try {
    // ADR-027: a translation ships only as `reviewed`, and the record names
    // the AI review instead of a person.
    const routes = [];
    const registry = loadLocales(root);
    for (const [code, locale] of Object.entries(production)) {
      const { reviewers } = registry.locales.find((entry) => entry.code === code);
      const dictionary = JSON.parse(read(root, `content/i18n/${code}.json`));
      assert.ok(reviewers.includes(dictionary._review.reviewedBy), `${code} dictionary records a registered reviewer`);
      routes.push([code, `${locale.prefix}/`]);
      for (const slug of locale.categories) {
        routes.push([code, `${locale.prefix}/${locale.guias[0]}/${slug}/`]);
      }
      for (const dir of ['guias', 'ferramentas', 'paginas']) {
        const [segment, slugs] = locale[dir];
        const files = fs.readdirSync(path.join(root, 'content', 'translations', code, dir)).sort();
        assert.deepEqual(files, slugs.map((slug) => `${slug}.md`), `${code}/${dir} holds exactly the launched pages`);
        if (segment) routes.push([code, `${locale.prefix}/${segment}/`]);
        for (const slug of slugs) {
          const rel = `content/translations/${code}/${dir}/${slug}.md`;
          const raw = fs.readFileSync(path.join(root, rel), 'utf8');
          assert.match(raw, /^translationStatus: reviewed$/m, `${rel} is reviewed`);
          const reviewer = /^reviewedBy: (.+)$/m.exec(raw)?.[1];
          assert.match(reviewer ?? '', /^AI review \([^)]+\)$/, `${rel} records the AI review`);
          assert.ok(reviewers.includes(reviewer), `${rel} records a registered reviewer`);
          assert.match(raw, /^reviewedAt: \d{4}-\d{2}-\d{2}$/m, `${rel} dates its review`);
          assert.doesNotMatch(raw, /!\[/, `${rel} embeds no Portuguese screenshot`);
          routes.push([code, segment ? `${locale.prefix}/${segment}/${slug}/` : `${locale.prefix}/${slug}/`]);
        }
      }
    }

    // The registry flags alone, with no test override, emit both locales.
    buildSite({ projectRoot: root, outDir, canonicalOrigin: origin, storeState: 'prelaunch' });
    const sitemap = read(outDir, 'sitemap.xml');
    assert.equal((sitemap.match(/<url>/g) || []).length, 49 + routes.length, 'sitemap adds exactly the locale pages to the pt-BR set');
    for (const [code, route] of routes) {
      const html = read(outDir, `${route.slice(1)}index.html`);
      assert.ok(sitemap.includes(`<loc>${origin}${route}</loc>`), `sitemap lists ${route}`);
      assert.match(html, /<meta name="robots" content="index,follow/, `${route} is fresh and indexable`);
      assert.ok(html.includes(`<link rel="canonical" href="${origin}${route}">`), `${route} is self-canonical`);
      const links = hreflangs(html);
      assert.ok(links.some((link) => link.hreflang === code && link.route === route), `${route} lists itself in hreflang`);
      assert.ok(links.some((link) => link.hreflang === 'pt-BR'), `${route} lists its Portuguese original`);
      assert.equal(links.filter((link) => link.hreflang === 'x-default').length, 1, `${route} has one x-default`);
      assert.doesNotMatch(html, /<img[^>]+src="\/assets\/app\//, `${route} shows no Portuguese app capture`);
    }

    for (const [code, locale] of Object.entries(production)) {
      const home = read(outDir, `${code}/index.html`);
      const header = home.match(/<header class="topbar">([\s\S]*?)<\/header>/)[1];
      const main = home.match(/<main id="conteudo">([\s\S]*?)<\/main>/)[1];
      const footer = home.match(/<footer class="footer">([\s\S]*?)<\/footer>/)[1];
      for (const section of [header, footer]) {
        for (const hub of locale.portal) {
          assert.ok(section.includes(`href="${hub}"`), `${code} chrome links to ${hub}`);
        }
      }
      for (const hub of [locale.portal[0], locale.portal[1], `${locale.portal[0]}pr/`]) {
        assert.ok(main.includes(`href="${hub}"`), `${code} home links to ${hub}`);
      }
      const guideHub = read(outDir, `${code}/${locale.guias[0]}/index.html`);
      const categoryNav = guideHub.match(/<nav class="category-nav"[\s\S]*?<\/nav>/)?.[0];
      assert.deepEqual(
        [...categoryNav.matchAll(/href="([^"]+)"/g)].map((match) => match[1]),
        locale.categories.map((slug) => `${locale.prefix}/${locale.guias[0]}/${slug}/`),
        `${code} guides link every localized category`,
      );
      for (const [pageCode, route] of routes.filter(([pageCode]) => pageCode === code)) {
        const html = read(outDir, `${route.slice(1)}index.html`);
        assert.doesNotMatch(html, /href="\/(?:precos|metodologia|dados)\//, `${pageCode} ${route} uses the same-locale portal`);
      }
    }

    // Readers are told how each translated guide was made.
    for (const [code, locale] of Object.entries(production)) {
      const [segment, slugs] = locale.guias;
      for (const slug of slugs) {
        assert.ok(read(outDir, `${code}/${segment}/${slug}/index.html`).includes(locale.disclosure), `${code} guide ${slug} discloses the AI translation`);
      }
    }
    assert.ok(read(outDir, 'en/editorial-policy/index.html').includes('the Portuguese text prevails'), 'English policy states which version prevails');
    assert.ok(read(outDir, 'es/politica-editorial/index.html').includes('prevalece el texto en portugués'), 'Spanish policy states which version prevails');

    // The Portuguese pages now offer both languages.
    const picker = headerLanguagePicker(read(outDir, 'index.html'));
    assert.ok(picker.includes('<a href="/en/" lang="en" hreflang="en">English</a>'), 'pt-BR home links the English home');
    assert.ok(picker.includes('<a href="/es/" lang="es" hreflang="es">Español</a>'), 'pt-BR home links the Spanish home');

    assert.doesNotThrow(() => checkDist({ projectRoot: root, distDir: outDir, origin }));
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

test('localized chrome: byline, editorial line and footer link the same-locale about and policy pages', () => {
  const tmpTrans = fs.mkdtempSync(path.join(os.tmpdir(), 'abastevo-i18n-chrome-'));
  const outDir = path.join(os.tmpdir(), 'abastevo-i18n-chrome-out-' + Math.random().toString(36).slice(2, 8));
  const page = (slug, source, title) => `---
title: "${title}"
description: "Fixture page ${slug}: it stands for the reviewed translation of one Portuguese institutional page of the abastevo site."
slug: ${slug}
datePublished: 2026-11-21
dateModified: 2026-11-21
translationOf: ${source}
sourceRevision: 2026-11-21
translationStatus: reviewed
reviewedBy: Test Reviewer
reviewedAt: 2026-11-21
---

## Section

Fixture body.
`;
  try {
    fs.cpSync(translationsDir, tmpTrans, { recursive: true });
    fs.mkdirSync(path.join(tmpTrans, 'en', 'paginas'), { recursive: true });
    fs.writeFileSync(path.join(tmpTrans, 'en', 'paginas', 'about.md'), page('about', '/sobre/', 'About abastevo: who we are'));
    fs.writeFileSync(path.join(tmpTrans, 'en', 'paginas', 'editorial-policy.md'), page('editorial-policy', '/politica-editorial/', 'Editorial policy: sources and dates'));
    buildSite({
      projectRoot: root,
      outDir,
      canonicalOrigin: origin,
      storeState: 'prelaunch',
      enableLocales: ['en', 'es'],
      translationsDir: tmpTrans,
    });

    // English has both pages: every chrome link stays in English.
    const enGuide = read(outDir, 'en/guides/ethanol-or-gasoline/index.html');
    assert.ok(enGuide.includes('By <a href="/en/about/">abastevo Team</a>'), 'en byline links the English about page');
    assert.ok(enGuide.includes('<a href="/en/editorial-policy/">editorial policy</a>'), 'en editorial line links the English policy');
    assert.ok(!enGuide.includes('href="/sobre/"') && !enGuide.includes('href="/politica-editorial/"'), 'en guide has no Portuguese about or policy link');
    const enFooter = enGuide.match(/<footer\b[\s\S]*?<\/footer>/)[0];
    assert.ok(enFooter.includes('<a href="/en/about/">About abastevo</a>'), 'en footer links the English about page');
    assert.ok(enFooter.includes('<a href="/en/editorial-policy/">Editorial policy</a>'), 'en footer links the English policy');
    const enTool = read(outDir, 'en/tools/ethanol-or-gasoline-calculator/index.html');
    assert.ok(enTool.includes('By <a href="/en/about/">abastevo Team</a>'), 'en tool byline links the English about page');
    const enHub = read(outDir, 'en/guides/index.html');
    assert.ok(enHub.includes('<a href="/en/about/">abastevo Team</a>') && enHub.includes('<a href="/en/editorial-policy/">editorial policy</a>'), 'en guides hub links the English pages');

    // Spanish has neither page: the links fall back to Portuguese, marked as such.
    const esGuide = read(outDir, 'es/guias/etanol-o-gasolina/index.html');
    assert.ok(esGuide.includes('Por <a href="/sobre/" hreflang="pt-BR">Equipo abastevo</a>'), 'es byline falls back to the Portuguese about page');
    assert.ok(esGuide.includes('<a href="/politica-editorial/" hreflang="pt-BR">política editorial</a>'), 'es editorial line falls back to the Portuguese policy');
    const esFooter = esGuide.match(/<footer\b[\s\S]*?<\/footer>/)[0];
    assert.match(esFooter, /<a href="\/sobre\/" hreflang="pt-BR">Sobre abastevo \(en portugués\)<\/a>/);
    assert.match(esFooter, /<a href="\/politica-editorial\/" hreflang="pt-BR">Política editorial \(en portugués\)<\/a>/);

    // The Portuguese pages are unchanged.
    const ptGuide = read(outDir, 'guias/icms-sobre-combustiveis/index.html');
    assert.ok(ptGuide.includes('Por <a href="/sobre/">Equipe abastevo</a>'), 'pt-BR byline is unchanged');
    assert.ok(ptGuide.includes('<a href="/politica-editorial/">política editorial</a>'), 'pt-BR editorial line is unchanged');

    assert.doesNotThrow(() => checkDist({ projectRoot: root, distDir: outDir, origin }));
  } finally {
    fs.rmSync(tmpTrans, { recursive: true, force: true });
    fs.rmSync(outDir, { recursive: true, force: true });
  }
});

test('i18n review gate: refuses enabling a locale whose dictionary is still a draft', () => {
  const outDir = path.join(os.tmpdir(), 'abastevo-i18n-gate-' + Math.random().toString(36).slice(2, 8));
  const messagesDir = fs.mkdtempSync(path.join(os.tmpdir(), 'abastevo-i18n-messages-'));
  try {
    fs.cpSync(path.join(root, 'content', 'i18n'), messagesDir, { recursive: true });
    const file = path.join(messagesDir, 'en.json');
    const dictionary = JSON.parse(fs.readFileSync(file, 'utf8'));
    dictionary._review = { status: 'draft', reviewedBy: '', reviewedAt: '' };
    fs.writeFileSync(file, JSON.stringify(dictionary));
    assert.throws(() => buildSite({
      projectRoot: root,
      outDir,
      canonicalOrigin: origin,
      storeState: 'prelaunch',
      enableLocales: ['en'],
      enforceReviewGates: true,
      messagesDir,
    }), /i18n en: enabled locale dictionary must have _review\.status = "reviewed"/);
  } finally {
    fs.rmSync(outDir, { recursive: true, force: true });
    fs.rmSync(messagesDir, { recursive: true, force: true });
  }
});
