// HTML layouts for generated landing pages (guides, hubs, tools, E-E-A-T
// pages), the shared footer columns and the home integration sections.
// Every interpolated value is escaped; JSON-LD is serialized with <, > and &
// escaped so authored text can never close the script element.

import { escapeHtml as esc } from './markdown.mjs';
import { htmlTitle, relatedGuides } from './site.mjs';
import { STATES, GITHUB_URL, TOOL_FORMS, TOOL_NAV_LABELS } from './data.mjs';

export const EDITORIAL_TEAM = 'Equipe abastevo';
const MONTHS = ['janeiro', 'fevereiro', 'março', 'abril', 'maio', 'junho', 'julho', 'agosto', 'setembro', 'outubro', 'novembro', 'dezembro'];

export function formatDateLong(iso) {
  const [y, m, d] = iso.split('-').map(Number);
  return `${d} de ${MONTHS[m - 1]} de ${y}`;
}

export function jsonLdScript(data) {
  const json = JSON.stringify(data, null, 2)
    .replace(/</g, '\\u003c')
    .replace(/>/g, '\\u003e')
    .replace(/&/g, '\\u0026');
  return `<script type="application/ld+json">\n${json}\n  </script>`;
}

function teamNode(origin) {
  return { '@type': 'Organization', name: EDITORIAL_TEAM, url: `${origin}/sobre/` };
}

function publisherNode(origin) {
  return {
    ...teamNode(origin),
    logo: { '@type': 'ImageObject', url: `${origin}/assets/brand/icon-512.png` },
  };
}

function breadcrumbNode(items, origin, pageUrl) {
  return {
    '@type': 'BreadcrumbList',
    '@id': `${pageUrl}#breadcrumbs`,
    itemListElement: items.map((item, index) => ({
      '@type': 'ListItem',
      position: index + 1,
      name: item.name,
      item: `${origin}${item.route}`,
    })),
  };
}

function breadcrumbNav(items) {
  const parts = items.map((item, index) => (index === items.length - 1
    ? `<li><span aria-current="page">${esc(item.name)}</span></li>`
    : `<li><a href="${esc(item.route)}">${esc(item.name)}</a></li>`));
  return `<nav class="breadcrumb-nav" aria-label="Navegação estrutural">\n        <ol class="breadcrumb-list">\n          ${parts.join('\n          ')}\n        </ol>\n      </nav>`;
}

function headHtml({ origin, route, title, description, ogType = 'website', imageAlt, extraMeta = [], jsonLd, scripts = [] }) {
  const url = `${origin}${route}`;
  const pageTitle = esc(htmlTitle(title));
  const desc = esc(description);
  const alt = esc(`abastevo — ${imageAlt || title}`);
  const image = `${origin}/assets/brand/og-image-v1.png`;
  return `<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>${pageTitle}</title>
  <meta name="description" content="${desc}">
  <link rel="canonical" href="${esc(url)}">

  <meta property="og:type" content="${ogType}">
  <meta property="og:site_name" content="abastevo">
  <meta property="og:locale" content="pt_BR">
  <meta property="og:url" content="${esc(url)}">
  <meta property="og:title" content="${pageTitle}">
  <meta property="og:description" content="${desc}">
  <meta property="og:image" content="${image}">
  <meta property="og:image:width" content="1200">
  <meta property="og:image:height" content="630">
  <meta property="og:image:alt" content="${alt}">
${extraMeta.map((line) => `  ${line}\n`).join('')}
  <meta name="twitter:card" content="summary_large_image">
  <meta name="twitter:title" content="${pageTitle}">
  <meta name="twitter:description" content="${desc}">
  <meta name="twitter:image" content="${image}">
  <meta name="twitter:image:alt" content="${alt}">

  <link rel="icon" type="image/svg+xml" href="/assets/brand/favicon.svg">
  <link rel="icon" type="image/png" sizes="192x192" href="/assets/brand/icon-192.png">
  <link rel="icon" href="/favicon.ico">
  <link rel="apple-touch-icon" sizes="180x180" href="/assets/brand/apple-touch-icon.png">
  <link rel="manifest" href="/site.webmanifest">
  <meta name="theme-color" content="#ffffff">

  ${jsonLdScript(jsonLd)}

  <link rel="stylesheet" href="/assets/styles.css">
  <script type="module" src="/assets/main.js"></script>
${scripts.map((src) => `  <script type="module" src="${src}"></script>\n`).join('')}</head>`;
}

function documentHtml({ head, bodyClass, main }) {
  return `<!DOCTYPE html>
<html lang="pt-BR">
${head}
<body class="${bodyClass}">
  <a href="#conteudo" class="skip-link">Pular para o conteúdo</a>

  <!-- LEARNING_HEADER -->

  <main id="conteudo" class="legal-page">
    <div class="shell">
      ${main}
    </div>
  </main>

  <!-- LEARNING_FOOTER -->
</body>
</html>
`;
}

function card(entry) {
  return `<li><a class="topic-card" href="${esc(entry.route)}"><strong>${esc(entry.title)}</strong><span>${esc(entry.summary || entry.description || '')}</span></a></li>`;
}

function cardList(entries) {
  return `<ul class="topic-cards">\n        ${entries.map(card).join('\n        ')}\n      </ul>`;
}

function linkList(site, routes) {
  const items = routes.map((route) => `<li><a href="${esc(route)}">${esc(site.routeLabel(route))}</a></li>`);
  return `<ul class="link-list">\n        ${items.join('\n        ')}\n      </ul>`;
}

function datesLine(entry, { published = true } = {}) {
  const parts = [`Por <a href="/sobre/">${EDITORIAL_TEAM}</a>`];
  if (published) parts.push(`Publicado em <time datetime="${entry.datePublished}">${formatDateLong(entry.datePublished)}</time>`);
  parts.push(`Atualizado em <time datetime="${entry.dateModified}">${formatDateLong(entry.dateModified)}</time>`);
  return `<p class="article-meta">${parts.join(' · ')}</p>`;
}

function itemListNode(entries, origin) {
  return {
    '@type': 'ItemList',
    itemListElement: entries.map((entry, index) => ({
      '@type': 'ListItem',
      position: index + 1,
      url: `${origin}${entry.route}`,
      name: entry.title,
    })),
  };
}

// ---------------------------------------------------------------------------
// Guide article.

export function renderGuide(site, guide, origin) {
  const category = site.categories.find((c) => c.slug === guide.category);
  const url = `${origin}${guide.route}`;
  const crumbs = [
    { name: 'Início', route: '/' },
    { name: 'Guias', route: '/guias/' },
    { name: category.name, route: category.route },
    { name: guide.title, route: guide.route },
  ];
  const sections = guide.headings.filter((h) => h.level === 2);
  const toc = sections.length >= 3
    ? `<nav class="article-toc" aria-label="Neste guia">\n        <p class="article-toc-title">Neste guia</p>\n        <ol>${sections.map((h) => `<li><a href="#${h.id}">${h.html}</a></li>`).join('')}</ol>\n      </nav>`
    : '';
  const readAlso = relatedGuides(site, guide);
  const jsonLd = {
    '@context': 'https://schema.org',
    '@graph': [
      {
        '@type': 'Article',
        '@id': `${url}#article`,
        headline: guide.headline,
        description: guide.description,
        url,
        mainEntityOfPage: url,
        inLanguage: 'pt-BR',
        datePublished: guide.datePublished,
        dateModified: guide.dateModified,
        articleSection: category.name,
        keywords: guide.tags.join(', '),
        image: `${origin}/assets/brand/og-image-v1.png`,
        author: teamNode(origin),
        publisher: publisherNode(origin),
        isPartOf: { '@id': `${origin}/#website` },
      },
      breadcrumbNode(crumbs, origin, url),
    ],
  };
  const main = `${breadcrumbNav(crumbs)}

      <article class="legal-article guide-article">
        <p class="article-kicker"><a href="${category.route}">${esc(category.name)}</a></p>
        <h1>${esc(guide.headline)}</h1>
        <p class="article-lead">${esc(guide.description)}</p>
        ${datesLine(guide)}
        <p class="article-editorial">Texto da ${EDITORIAL_TEAM}, revisado por pessoas com base nas fontes citadas. Veja a <a href="/politica-editorial/">política editorial</a> e como pedir uma correção.</p>
        ${toc}
${guide.html}
        <section class="article-section" aria-labelledby="continue-explorando">
          <h2 id="continue-explorando">Continue explorando</h2>
          ${linkList(site, guide.related)}
        </section>
        <section class="article-section" aria-labelledby="leia-tambem">
          <h2 id="leia-tambem">Leia também</h2>
          ${cardList(readAlso)}
        </section>
      </article>`;
  return documentHtml({
    head: headHtml({
      origin,
      route: guide.route,
      title: guide.title,
      description: guide.description,
      ogType: 'article',
      imageAlt: guide.headline,
      extraMeta: [
        `<meta property="article:published_time" content="${guide.datePublished}">`,
        `<meta property="article:modified_time" content="${guide.dateModified}">`,
        `<meta property="article:section" content="${esc(category.name)}">`,
      ],
      jsonLd,
    }),
    bodyClass: 'landing-home learning-page article-page',
    main,
  });
}

// ---------------------------------------------------------------------------
// Hubs.

export function renderGuidesHub(site, origin) {
  const hub = site.registry.get('/guias/');
  const url = `${origin}/guias/`;
  const crumbs = [{ name: 'Início', route: '/' }, { name: 'Guias', route: '/guias/' }];
  const blocks = site.categories.map((category) => {
    const members = site.guides.filter((g) => g.category === category.slug);
    return `<h2 id="${category.slug}">${esc(category.name)}</h2>
        <p>${esc(category.intro)}</p>
        ${cardList(members)}
        <p><a class="text-link" href="${category.route}">Todos os guias de ${esc(category.name.toLowerCase())}</a></p>`;
  });
  const jsonLd = {
    '@context': 'https://schema.org',
    '@graph': [
      {
        '@type': 'CollectionPage',
        '@id': `${url}#webpage`,
        url,
        name: htmlTitle(hub.title),
        description: hub.description,
        inLanguage: 'pt-BR',
        isPartOf: { '@id': `${origin}/#website` },
        mainEntity: itemListNode(site.guides, origin),
      },
      breadcrumbNode(crumbs, origin, url),
    ],
  };
  const main = `${breadcrumbNav(crumbs)}
      <article class="legal-article">
        <h1>Guias Educativos abastevo</h1>
        <p class="article-lead">${esc(hub.intro)}</p>
        <nav class="category-nav" aria-label="Categorias de guias">
          <ul class="chip-list">${site.categories.map((c) => `<li><a href="${c.route}">${esc(c.name)}</a></li>`).join('')}</ul>
        </nav>
        ${blocks.join('\n        ')}
        <h2 id="ferramentas">Calculadoras para fazer a conta</h2>
        <p>Os guias explicam o raciocínio; as <a href="/ferramentas/">calculadoras de combustível</a> fazem a conta com os seus números, sem cadastro.</p>
        ${linkList(site, site.tools.map((t) => t.route))}
        <h2 id="sobre-os-guias">Como os guias são feitos</h2>
        <p>Os guias são escritos pela <a href="/sobre/">${EDITORIAL_TEAM}</a>, citam a fonte oficial de cada informação e mostram a data da última revisão. Preços citados são sempre exemplos; para valores reais, consulte os <a href="/precos/">preços de referência por estado</a> com a data da coleta. Regras completas na <a href="/politica-editorial/">política editorial</a>.</p>
      </article>`;
  return documentHtml({
    head: headHtml({ origin, route: '/guias/', title: hub.title, description: hub.description, imageAlt: hub.title, jsonLd }),
    bodyClass: 'landing-home learning-page learning-hub',
    main,
  });
}

export function renderCategory(site, category, origin) {
  const entry = site.registry.get(category.route);
  const url = `${origin}${category.route}`;
  const members = site.guides.filter((g) => g.category === category.slug);
  const crumbs = [
    { name: 'Início', route: '/' },
    { name: 'Guias', route: '/guias/' },
    { name: category.name, route: category.route },
  ];
  const others = site.categories.filter((c) => c.slug !== category.slug);
  const jsonLd = {
    '@context': 'https://schema.org',
    '@graph': [
      {
        '@type': 'CollectionPage',
        '@id': `${url}#webpage`,
        url,
        name: htmlTitle(entry.title),
        description: entry.description,
        inLanguage: 'pt-BR',
        isPartOf: { '@id': `${origin}/#website` },
        mainEntity: itemListNode(members, origin),
      },
      breadcrumbNode(crumbs, origin, url),
    ],
  };
  const main = `${breadcrumbNav(crumbs)}
      <article class="legal-article">
        <h1>${esc(entry.title)}</h1>
        <p class="article-lead">${esc(category.intro)}</p>
        <h2 id="artigos">Artigos</h2>
        ${cardList(members)}
        <h2 id="outras-categorias">Outras categorias</h2>
        <ul class="chip-list">${others.map((c) => `<li><a href="${c.route}">${esc(c.name)}</a></li>`).join('')}</ul>
        <p>Veja todos os <a href="/guias/">guias sobre combustíveis</a>, use as <a href="/ferramentas/">calculadoras</a> ou consulte os <a href="/precos/">preços de referência por estado</a>.</p>
      </article>`;
  return documentHtml({
    head: headHtml({ origin, route: category.route, title: entry.title, description: entry.description, imageAlt: entry.title, jsonLd }),
    bodyClass: 'landing-home learning-page learning-hub',
    main,
  });
}

export function renderToolsHub(site, origin) {
  const hub = site.registry.get('/ferramentas/');
  const url = `${origin}/ferramentas/`;
  const crumbs = [{ name: 'Início', route: '/' }, { name: 'Ferramentas', route: '/ferramentas/' }];
  const jsonLd = {
    '@context': 'https://schema.org',
    '@graph': [
      {
        '@type': 'CollectionPage',
        '@id': `${url}#webpage`,
        url,
        name: htmlTitle(hub.title),
        description: hub.description,
        inLanguage: 'pt-BR',
        isPartOf: { '@id': `${origin}/#website` },
        mainEntity: itemListNode(site.tools, origin),
      },
      breadcrumbNode(crumbs, origin, url),
    ],
  };
  const guideRoutes = ['/guias/etanol-ou-gasolina/', '/guias/como-calcular-consumo-medio/', '/guias/como-economizar-combustivel/']
    .filter((route) => site.registry.has(route));
  const main = `${breadcrumbNav(crumbs)}
      <article class="legal-article">
        <h1>Calculadoras de combustível</h1>
        <p class="article-lead">${esc(hub.intro)}</p>
        <h2 id="calculadoras">Escolha uma calculadora</h2>
        ${cardList(site.tools)}
        <h2 id="como-funcionam">Como as calculadoras funcionam</h2>
        <p>Cada página mostra a fórmula usada e um exemplo resolvido passo a passo, então dá para conferir a conta à mão, inclusive com o JavaScript desligado. A versão interativa calcula no seu navegador: os valores digitados não são enviados, salvos nem associados a você.</p>
        <p>Os resultados são estimativas. Consumo real muda com trânsito, carga, relevo, calibragem e jeito de dirigir, e o preço muda de posto para posto e de semana para semana.</p>
        <h2 id="precos-de-referencia">Onde conseguir preços para a conta</h2>
        <p>Use o preço que você viu na bomba ou consulte os <a href="/precos/">preços de referência da ANP por estado e cidade</a>, sempre com a data da coleta. Entenda o que esses números significam na <a href="/metodologia/">metodologia</a>.</p>
        <h2 id="guias-relacionados">Guias relacionados</h2>
        ${linkList(site, guideRoutes)}
      </article>`;
  return documentHtml({
    head: headHtml({ origin, route: '/ferramentas/', title: hub.title, description: hub.description, imageAlt: hub.title, jsonLd }),
    bodyClass: 'landing-home learning-page learning-hub',
    main,
  });
}

// ---------------------------------------------------------------------------
// Calculator page.

function toolField(field) {
  const id = `campo-${field.name.replace(/_/g, '-')}`;
  const errorId = `${id}-erro`;
  const error = `<p class="tool-error" id="${errorId}" data-error-for="${field.name}" hidden></p>`;
  if (field.kind === 'checkbox') {
    return `<div class="tool-field tool-check">
              <input id="${id}" name="${field.name}" type="checkbox" value="on">
              <label for="${id}">${esc(field.label)}</label>
            </div>`;
  }
  const hintId = `${id}-dica`;
  const described = field.hint ? `${hintId} ${errorId}` : errorId;
  const hint = field.hint ? `\n              <p class="tool-hint" id="${hintId}">${esc(field.hint)}</p>` : '';
  const required = field.required ? ' required' : '';
  const control = field.kind === 'select'
    ? `<select id="${id}" name="${field.name}"${required} aria-describedby="${described}">${field.options.map(([value, label]) => `<option value="${esc(value)}">${esc(label)}</option>`).join('')}</select>`
    : `<input id="${id}" name="${field.name}" type="text" inputmode="${field.inputmode || 'decimal'}" autocomplete="off" spellcheck="false"${required} aria-describedby="${described}">`;
  return `<div class="tool-field">
              <label for="${id}">${esc(field.label)}</label>
              ${control}${hint}
              ${error}
            </div>`;
}

export function renderTool(site, tool, origin) {
  const url = `${origin}${tool.route}`;
  const crumbs = [
    { name: 'Início', route: '/' },
    { name: 'Ferramentas', route: '/ferramentas/' },
    { name: tool.title, route: tool.route },
  ];
  const fields = TOOL_FORMS[tool.slug];
  const others = site.tools.filter((t) => t.route !== tool.route);
  const relatedGuidesList = tool.related.filter((route) => site.registry.get(route)?.kind?.endsWith('guide'));
  const relatedOther = tool.related.filter((route) => !relatedGuidesList.includes(route));
  const jsonLd = {
    '@context': 'https://schema.org',
    '@graph': [
      {
        '@type': 'WebApplication',
        '@id': `${url}#app`,
        name: tool.headline,
        url,
        description: tool.description,
        applicationCategory: 'UtilitiesApplication',
        operatingSystem: 'Qualquer navegador moderno',
        browserRequirements: 'O cálculo interativo usa JavaScript; a fórmula e o exemplo funcionam sem ele.',
        inLanguage: 'pt-BR',
        isAccessibleForFree: true,
        offers: { '@type': 'Offer', price: '0', priceCurrency: 'BRL' },
        datePublished: tool.datePublished,
        dateModified: tool.dateModified,
        publisher: publisherNode(origin),
        isPartOf: { '@id': `${origin}/#website` },
      },
      breadcrumbNode(crumbs, origin, url),
    ],
  };
  const main = `${breadcrumbNav(crumbs)}
      <article class="legal-article tool-article">
        <h1>${esc(tool.headline)}</h1>
        <p class="article-lead">${esc(tool.description)}</p>
        <div class="tool-panel" data-tool-panel>
          <p class="tool-fallback" data-tool-fallback>A calculadora interativa precisa de JavaScript. Sem ele, a conta continua simples: siga a fórmula e o exemplo resolvido logo abaixo.</p>
          <form class="tool-form" data-tool="${esc(tool.slug)}" novalidate hidden>
            ${fields.map(toolField).join('\n            ')}
            <div class="tool-actions"><button type="submit" class="btn-dark">Calcular</button></div>
          </form>
          <div class="tool-result" data-tool-result role="status" aria-live="polite"></div>
          <p class="tool-privacy">A conta é feita no seu navegador. Nada do que você digita é enviado ou salvo.</p>
        </div>
        ${datesLine(tool, { published: false })}
${tool.html}
        <section class="article-section" aria-labelledby="precos-de-referencia">
          <h2 id="precos-de-referencia">Preços de referência para a conta</h2>
          <p>Use o preço que você viu na bomba ou consulte os <a href="/precos/">preços de referência da ANP por estado e cidade</a>, com a data da coleta. Preço de referência é amostra da semana pesquisada, não garantia do valor no posto hoje.</p>
        </section>
        <section class="article-section" aria-labelledby="outras-ferramentas">
          <h2 id="outras-ferramentas">Outras ferramentas</h2>
          ${linkList(site, others.map((t) => t.route))}
        </section>
        <section class="article-section" aria-labelledby="guias-relacionados">
          <h2 id="guias-relacionados">Guias relacionados</h2>
          ${cardList(relatedGuidesList.map((route) => site.registry.get(route)))}
          ${relatedOther.length > 0 ? linkList(site, relatedOther) : ''}
        </section>
      </article>`;
  return documentHtml({
    head: headHtml({ origin, route: tool.route, title: tool.title, description: tool.description, imageAlt: tool.headline, jsonLd, scripts: ['/assets/tools.js'] }),
    bodyClass: 'landing-home learning-page tool-page',
    main,
  });
}

// ---------------------------------------------------------------------------
// E-E-A-T pages (/sobre/, /politica-editorial/).

export function renderPage(site, page, origin) {
  const url = `${origin}${page.route}`;
  const crumbs = [{ name: 'Início', route: '/' }, { name: page.title, route: page.route }];
  const graph = [
    {
      '@type': page.schemaType,
      '@id': `${url}#webpage`,
      url,
      name: htmlTitle(page.title),
      description: page.description,
      inLanguage: 'pt-BR',
      dateModified: page.dateModified,
      isPartOf: { '@id': `${origin}/#website` },
      about: { '@id': `${origin}/#organization` },
      publisher: publisherNode(origin),
    },
    breadcrumbNode(crumbs, origin, url),
  ];
  if (page.route === '/sobre/') {
    graph.push({
      '@type': 'Organization',
      '@id': `${origin}/sobre/#equipe`,
      name: EDITORIAL_TEAM,
      url: `${origin}/sobre/`,
      parentOrganization: { '@id': `${origin}/#organization` },
      publishingPrinciples: `${origin}/politica-editorial/`,
      sameAs: [GITHUB_URL],
    });
  }
  const main = `${breadcrumbNav(crumbs)}
      <article class="legal-article">
        <h1>${esc(page.headline)}</h1>
        <p class="article-lead">${esc(page.description)}</p>
        <p class="article-meta">Atualizado em <time datetime="${page.dateModified}">${formatDateLong(page.dateModified)}</time> · ${EDITORIAL_TEAM}</p>
${page.html}
      </article>`;
  return documentHtml({
    head: headHtml({ origin, route: page.route, title: page.title, description: page.description, imageAlt: page.headline, jsonLd: { '@context': 'https://schema.org', '@graph': graph } }),
    bodyClass: 'landing-home legal-document',
    main,
  });
}

// ---------------------------------------------------------------------------
// Shared footer columns and home sections.

export function renderFooterColumns(site) {
  const states = STATES.map((s) => `<li><a href="/precos/${s.uf}/">${esc(s.name)}</a></li>`).join('');
  const categories = site.categories.map((c) => `<a href="${c.route}">${esc(c.name)}</a>`).join('');
  const tools = site.tools.map((t) => `<a href="${t.route}">${esc(TOOL_NAV_LABELS[t.slug] || t.title)}</a>`).join('');
  return `<div class="footer-cols footer-cols-hubs">
        <nav aria-label="Preços" class="footer-col-precos"><h2>Preços</h2><a href="/precos/">Preços por estado</a><a href="/consultar/">Consultar dados</a><a href="/fiscalizacao/">Fiscalização</a><a href="/metodologia/">Metodologia</a><ul class="footer-states">${states}</ul></nav>
        <nav aria-label="Guias"><h2>Guias</h2><a href="/guias/">Todos os guias</a>${categories}<a href="/como-usar/">Como usar o app</a><a href="#aplicativo">Aplicativo</a></nav>
        <nav aria-label="Ferramentas"><h2>Ferramentas</h2><a href="/ferramentas/">Todas as ferramentas</a>${tools}</nav>
        <nav aria-label="Projeto"><h2>Projeto</h2><a href="/sobre/">Sobre o abastevo</a><a href="/politica-editorial/">Política editorial</a><a href="/contribuidores/">Contribuidores</a><a href="${GITHUB_URL}">GitHub</a><a href="${GITHUB_URL}/issues">Contato</a><a href="${GITHUB_URL}/blob/main/TRADEMARKS.md">TRADEMARKS</a></nav>
        <nav aria-label="Privacidade"><h2>Privacidade</h2><a href="/privacidade/">Privacidade</a><a href="/excluir-conta/">Excluir conta</a><a href="/termos/">Termos</a></nav>
      </div>`;
}

export function renderHomeStates() {
  const items = STATES.map((s) => `<li><a href="/precos/${s.uf}/"><span class="state-uf" aria-hidden="true">${s.uf.toUpperCase()}</span><span class="state-name">${esc(s.name)}</span></a></li>`);
  return `<section id="precos-por-estado" class="block home-states">
      <div class="shell">
        <div class="block-head">
          <h2 class="h2">Preços por estado</h2>
          <p>Referência semanal da ANP por estado e cidade pesquisada, com a data da coleta. É amostra oficial da semana, não o preço do posto agora.</p>
        </div>
        <ul class="state-grid">
          ${items.join('\n          ')}
        </ul>
        <div class="lp-actions"><a class="btn-dark" href="/precos/">Ver preços por estado</a><a class="text-link" href="/metodologia/">Como os preços são coletados</a></div>
      </div>
    </section>`;
}

export function renderHomeGuides(site) {
  return `<section id="guias-populares" class="block home-topics">
      <div class="shell">
        <div class="block-head">
          <h2 class="h2">Guias populares</h2>
          <p>Como o preço se forma, qual combustível compensa e o que fazer quando algo dá errado no posto, sempre com fonte oficial.</p>
        </div>
        ${cardList(site.popularGuides)}
        <div class="lp-actions"><a class="text-link" href="/guias/">Ver todos os guias</a></div>
      </div>
    </section>`;
}

export function renderHomeTools(site) {
  return `<section id="ferramentas" class="block home-topics">
      <div class="shell">
        <div class="block-head">
          <h2 class="h2">Ferramentas</h2>
          <p>Calculadoras gratuitas, sem cadastro, com a fórmula explicada em cada página.</p>
        </div>
        ${cardList(site.tools)}
        <div class="lp-actions"><a class="text-link" href="/ferramentas/">Ver todas as ferramentas</a></div>
      </div>
    </section>`;
}
