import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { build } from '../scripts/build.mjs';
import { loadSite, htmlTitle } from '../scripts/content/site.mjs';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const ORIGIN = 'https://abastevo.com.br';
const outDir = fs.mkdtempSync(path.join(os.tmpdir(), 'abastevo-generated-'));
build({ projectRoot: root, outDir, origin: ORIGIN });
const site = loadSite(root);
test.after(() => fs.rmSync(outDir, { recursive: true, force: true }));

const read = (route) => fs.readFileSync(path.join(outDir, route === '/' ? 'index.html' : `${route.slice(1)}index.html`), 'utf8');
const jsonLd = (html) => [...html.matchAll(/<script type="application\/ld\+json">([\s\S]*?)<\/script>/g)].map((m) => JSON.parse(m[1]));
const nodes = (html) => jsonLd(html).flatMap((d) => d['@graph'] || [d]);
const visibleText = (html) => decode(html.replace(/<script[\s\S]*?<\/script>/g, " ").replace(/<[^>]+>/g, " ")).replace(/\s+/g, " ");
const decode = (s) => s.replace(/&quot;/g, '"').replace(/&#39;/g, "'").replace(/&lt;/g, '<').replace(/&gt;/g, '>').replace(/&amp;/g, '&');

const mdGuides = [...site.registry.values()].filter((e) => e.kind === 'guide');

test('every Markdown guide renders the shared article layout', () => {
  assert.ok(mdGuides.length >= 10, 'editorial wave 1 ships ten Markdown guides');
  for (const guide of mdGuides) {
    const html = read(guide.route);
    const category = site.categories.find((c) => c.slug === guide.category);
    const url = `${ORIGIN}${guide.route}`;

    assert.ok(html.includes(`<title>${htmlTitle(guide.title)}</title>`), `${guide.route} title`);
    assert.ok(html.includes(`<link rel="canonical" href="${url}">`), `${guide.route} self canonical`);
    assert.ok(html.includes('<meta property="og:type" content="article">'), `${guide.route} og:type article`);
    assert.ok(html.includes(`<meta property="article:modified_time" content="${guide.dateModified}">`));
    assert.match(html, /<body class="landing-home learning-page article-page">/);

    // Breadcrumbs: Início › Guias › Categoria › Artigo, mirrored in JSON-LD.
    const crumbs = html.match(/<nav class="breadcrumb-nav"[\s\S]*?<\/nav>/)[0];
    assert.ok(crumbs.includes('<a href="/">Início</a>'));
    assert.ok(crumbs.includes('<a href="/guias/">Guias</a>'));
    assert.ok(crumbs.includes(`<a href="${category.route}">${category.name}</a>`));
    assert.ok(decode(crumbs).includes(`<span aria-current="page">${guide.title}</span>`));
    const breadcrumb = nodes(html).find((n) => n['@type'] === 'BreadcrumbList');
    assert.deepEqual(breadcrumb.itemListElement.map((i) => i.item), [`${ORIGIN}/`, `${ORIGIN}/guias/`, `${ORIGIN}${category.route}`, url]);
    assert.deepEqual(breadcrumb.itemListElement.map((i) => i.position), [1, 2, 3, 4]);

    // Article JSON-LD describes what is visible.
    const article = nodes(html).find((n) => n['@type'] === 'Article');
    assert.ok(article, `${guide.route} Article JSON-LD`);
    assert.equal(article.headline, guide.headline);
    assert.ok(decode(html).includes(`<h1>${guide.headline}</h1>`), 'headline matches the visible H1');
    assert.equal(article.description, guide.description);
    assert.equal(article.datePublished, guide.datePublished);
    assert.equal(article.dateModified, guide.dateModified);
    assert.equal(article.mainEntityOfPage, url);
    assert.equal(article.image, `${ORIGIN}/assets/brand/og-image-v1.png`);
    for (const role of ['author', 'publisher']) {
      assert.equal(article[role]['@type'], 'Organization');
      assert.equal(article[role].name, 'Equipe abastevo');
      assert.equal(article[role].url, `${ORIGIN}/sobre/`);
    }
    assert.ok(!html.includes('aggregateRating') && !html.includes('"review"'), 'no fabricated ratings');

    // Visible dates and editorial line.
    assert.ok(html.includes(`<time datetime="${guide.datePublished}">`), 'visible publication date');
    assert.ok(html.includes(`<time datetime="${guide.dateModified}">`), 'visible update date');
    assert.ok(html.includes('Por <a href="/sobre/">Equipe abastevo</a>'), 'editorial byline');
    assert.ok(html.includes('<a href="/politica-editorial/">política editorial</a>'), 'editorial policy link');

    // "Continue explorando" mirrors the related routes; "Leia também" links other guides.
    const explore = html.match(/<h2 id="continue-explorando">[\s\S]*?<\/section>/)[0];
    assert.deepEqual([...explore.matchAll(/href="([^"]+)"/g)].map((m) => m[1]), guide.related);
    const readAlso = html.match(/<h2 id="leia-tambem">[\s\S]*?<\/section>/)[0];
    const readRoutes = [...readAlso.matchAll(/href="([^"]+)"/g)].map((m) => m[1]);
    assert.equal(readRoutes.length, 3);
    for (const route of readRoutes) {
      assert.notEqual(route, guide.route);
      assert.ok(site.guides.some((g) => g.route === route), `${route} is a guide`);
    }
  }
});

test('/guias/ is the articles hub with every category and guide', () => {
  const html = read('/guias/');
  for (const category of site.categories) assert.ok(html.includes(`href="${category.route}"`), `hub links ${category.route}`);
  for (const guide of site.guides) assert.ok(html.includes(`href="${guide.route}"`), `hub links ${guide.route}`);
  const page = nodes(html).find((n) => n['@type'] === 'CollectionPage');
  assert.equal(page.mainEntity['@type'], 'ItemList');
  assert.equal(page.mainEntity.itemListElement.length, site.guides.length);
  assert.ok(html.includes('href="/ferramentas/"'), 'hub links the tools hub');
});

test('category hubs list exactly their guides with a 3-level breadcrumb', () => {
  for (const category of site.categories) {
    const html = read(category.route);
    const members = site.guides.filter((g) => g.category === category.slug);
    const list = html.match(/<h2 id="artigos">[\s\S]*?<\/ul>/)[0];
    assert.deepEqual([...list.matchAll(/href="([^"]+)"/g)].map((m) => m[1]).sort(), members.map((g) => g.route).sort());
    const breadcrumb = nodes(html).find((n) => n['@type'] === 'BreadcrumbList');
    assert.equal(breadcrumb.itemListElement.length, 3);
    assert.equal(breadcrumb.itemListElement[2].item, `${ORIGIN}${category.route}`);
    for (const other of site.categories.filter((c) => c !== category)) assert.ok(html.includes(`href="${other.route}"`));
  }
});

test('/sobre/ states independence, sources, team, license and contact', () => {
  const html = read('/sobre/');
  const text = visibleText(html);
  assert.match(html, /<body class="landing-home legal-document">/);
  assert.match(text, /não é afiliado à ANP, à Petrobras, a nenhum governo/);
  assert.match(text, /Equipe abastevo/);
  assert.match(text, /licença de atribuição visível Abastevo/);
  assert.match(text, /Levantamento de Preços de Combustíveis da ANP/);
  assert.match(text, /Nenhum posto, marca ou anunciante paga/);
  assert.ok(html.includes('href="https://github.com/AlexandreZanata/abastevo/issues"'), 'contact via GitHub issues');
  assert.ok(html.includes('href="/politica-editorial/"'));
  const about = nodes(html).find((n) => n['@type'] === 'AboutPage');
  assert.equal(about.url, `${ORIGIN}/sobre/`);
  const team = nodes(html).find((n) => n['@id'] === `${ORIGIN}/sobre/#equipe`);
  assert.equal(team.name, 'Equipe abastevo');
  assert.equal(team.publishingPrinciples, `${ORIGIN}/politica-editorial/`);
});

test('/politica-editorial/ covers sources, dates, corrections, AI review and paid placement', () => {
  const text = visibleText(read('/politica-editorial/'));
  assert.match(text, /Fonte primária primeiro/);
  assert.match(text, /Sem números inventados/);
  assert.match(text, /semana da coleta/);
  assert.match(text, /Não usamos "hoje" para dados semanais/);
  assert.match(text, /Correções/);
  assert.match(text, /Nenhum texto é publicado sem revisão humana/);
  assert.match(text, /Pagamento nunca compra posição, destaque ou confiança/);
  assert.match(text, /não é afiliado à ANP/);
});

test('E-E-A-T: home Organization links /sobre/ and /politica-editorial/; no personal names', () => {
  const org = nodes(read('/')).find((n) => n['@type'] === 'Organization');
  assert.equal(org.publishingPrinciples, `${ORIGIN}/politica-editorial/`);
  assert.equal(org.department.name, 'Equipe abastevo');
  assert.equal(org.department.url, `${ORIGIN}/sobre/`);
  for (const route of ['/sobre/', '/politica-editorial/', ...mdGuides.map((g) => g.route)]) {
    assert.doesNotMatch(visibleText(read(route)), /Alexandre|Zanata/, `${route} names no person (D-SEO-09)`);
  }
});
