// HTML layouts for generated landing pages (guides, hubs, tools, E-E-A-T
// pages), the shared footer columns and the home integration sections.
// Every interpolated value is escaped; JSON-LD is serialized with <, > and &
// escaped so authored text can never close the script element.

import { escapeHtml as esc } from './markdown.mjs';
import { htmlTitle, relatedGuides } from './site.mjs';
import { STATES, GITHUB_URL, TOOL_FORMS, TOOL_NAV_LABELS } from './data.mjs';
import { localeRoute } from './i18n.mjs';
import { renderTutorialContent } from './tutorials.mjs';

const NATIVE_NAMES = { 'pt-BR': 'Português', en: 'English', es: 'Español' };

/** Looks up a message string and replaces `{placeholders}`. */
export function t(locale, key, vars = {}) {
  const value = key.split('.').reduce((node, part) => (node == null ? undefined : node[part]), locale.messages);
  if (typeof value !== 'string') throw new Error(`missing i18n string "${key}" for ${locale.code}`);
  return value.replace(/\{(\w+)\}/g, (_, name) => (Object.hasOwn(vars, name) ? String(vars[name]) : `{${name}}`));
}

export function formatDateLong(iso, locale) {
  const [y, m, d] = iso.split('-').map(Number);
  return t(locale, 'dateFormat', { day: d, month: locale.messages.months[m - 1], year: y });
}

export function jsonLdScript(data) {
  const json = JSON.stringify(data, null, 2)
    .replace(/</g, '\\u003c')
    .replace(/>/g, '\\u003e')
    .replace(/&/g, '\\u0026');
  return `<script type="application/ld+json">\n${json}\n  </script>`;
}

function teamNode(origin, locale) {
  return { '@type': 'Organization', name: t(locale, 'chrome.team'), url: `${origin}/sobre/` };
}

function publisherNode(origin, locale) {
  return {
    ...teamNode(origin, locale),
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

function breadcrumbNav(items, locale) {
  const parts = items.map((item, index) => (index === items.length - 1
    ? `<li><span aria-current="page">${esc(item.name)}</span></li>`
    : `<li><a href="${esc(item.route)}"${item.hreflang ? ` hreflang="${esc(item.hreflang)}"` : ''}>${esc(item.name)}</a></li>`));
  return `<nav class="breadcrumb-nav" aria-label="${esc(t(locale, 'chrome.breadcrumb'))}">\n        <ol class="breadcrumb-list">\n          ${parts.join('\n          ')}\n        </ol>\n      </nav>`;
}

function headHtml({ origin, route, title, description, ogType = 'website', imageAlt, extraMeta = [], jsonLd, scripts = [], feed = false, locale, robots, canonical = true, dataScript }) {
  const url = `${origin}${route}`;
  const pageTitle = esc(htmlTitle(title));
  const desc = esc(description);
  const alt = esc(`abastevo — ${imageAlt || title}`);
  const image = `${origin}/assets/brand/og-image-v1.png`;
  const robotsMeta = robots ? `\n  <meta name="robots" content="${robots}">` : '';
  return `<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">${robotsMeta}
  <title>${pageTitle}</title>
  <meta name="description" content="${desc}">
${canonical ? `  <link rel="canonical" href="${esc(url)}">\n` : ''}
  <meta property="og:type" content="${ogType}">
  <meta property="og:site_name" content="abastevo">
  <meta property="og:locale" content="${esc(locale.ogLocale)}">
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

  <!-- TAB_ICONS -->
  <link rel="apple-touch-icon" sizes="180x180" href="/assets/brand/apple-touch-icon.png">
  <link rel="manifest" href="/site.webmanifest">
  <meta name="theme-color" content="#ffffff">

  ${jsonLdScript(jsonLd)}
${dataScript ? `\n  ${dataScript}\n` : ''}
  <link rel="stylesheet" href="/assets/styles.css">
  <script type="module" src="/assets/main.js"></script>
${scripts.map((src) => `  <script type="module" src="${src}"></script>\n`).join('')}${feed ? '  <link rel="alternate" type="application/atom+xml" title="Guias do abastevo" href="/guias/feed.xml">\n' : ''}</head>`;
}

function documentHtml({ head, bodyClass, main, locale }) {
  return `<!DOCTYPE html>
<html lang="${esc(locale.htmlLang)}">
${head}
<body class="${bodyClass}">
  <a href="#conteudo" class="skip-link">${esc(t(locale, 'chrome.skip'))}</a>

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

const CARD_ARROW = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M5 12h14M13 6l6 6-6 6"/></svg>';

function card(entry, kicker) {
  const label = kicker ? `<span class="topic-kicker">${esc(kicker)}</span>` : '';
  return `<li><a class="topic-card" href="${esc(entry.route)}">${label}<strong>${esc(entry.title)}</strong><span class="topic-summary">${esc(entry.summary || entry.description || '')}</span><span class="topic-arrow" aria-hidden="true">${CARD_ARROW}</span></a></li>`;
}

// With `site`, each guide card names its category above the title: use it
// where the list mixes categories, not inside a category's own block.
function cardList(entries, site, locale) {
  const kickerFor = (entry) => (entry.category && locale ? t(locale, `categories.${entry.category}.name`) : undefined);
  return `<ul class="topic-cards">\n        ${entries.map((entry) => card(entry, kickerFor(entry))).join('\n        ')}\n      </ul>`;
}

function linkList(site, routes, locale = site.defaultLocale) {
  const items = routes.map((route) => {
    const label = site.routeLabel(site.localizedRoute(route, locale.code));
    return `<li>${localizedNavigationLink(site, locale, route, label)}</li>`;
  });
  return `<ul class="link-list">\n        ${items.join('\n        ')}\n      </ul>`;
}

/**
 * Link from a page of `locale` to a pt-BR route: the same-locale translation
 * when the locale has one, otherwise the Portuguese page, declared as such.
 * pt-BR pages keep a plain link.
 */
function localeLink(site, locale, route, label) {
  const target = site.localizedRoute(route, locale.code);
  const portuguese = Boolean(locale.prefix) && target === route;
  return `<a href="${esc(target)}"${portuguese ? ' hreflang="pt-BR"' : ''}>${label}</a>`;
}

function localizedNavigationLink(site, locale, route, label, { className } = {}) {
  const target = site.localizedRoute(route, locale.code);
  const portuguese = Boolean(locale.prefix) && target === route;
  const text = portuguese ? t(locale, 'chrome.linkInPortuguese', { label }) : label;
  return `<a${className ? ` class="${esc(className)}"` : ''} href="${esc(target)}"${portuguese ? ' hreflang="pt-BR"' : ''}>${esc(text)}</a>`;
}

function datesLine(site, entry, locale, { published = true } = {}) {
  const parts = [`${t(locale, 'chrome.by')} ${localeLink(site, locale, '/sobre/', esc(t(locale, 'chrome.team')))}`];
  if (published) parts.push(`${t(locale, 'chrome.published')} <time datetime="${entry.datePublished}">${formatDateLong(entry.datePublished, locale)}</time>`);
  parts.push(`${t(locale, 'chrome.updated')} <time datetime="${entry.dateModified}">${formatDateLong(entry.dateModified, locale)}</time>`);
  return `<p class="article-meta">${parts.join(' · ')}</p>`;
}

function staleBanner(entry, locale) {
  return entry.stale
    ? `<p class="article-stale" role="note">${esc(t(locale, 'chrome.staleNote', { date: formatDateLong(entry.sourceDate, locale) }))}</p>`
    : '';
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

function localeHubRoute(locale, kind) {
  return `${locale.prefix}/${locale.segments[kind]}/`;
}

function homeCrumb(locale) {
  return { name: t(locale, 'chrome.home'), route: `${locale.prefix}/` };
}

export function renderGuide(site, guide, origin, locale) {
  const isLocale = Boolean(guide.locale);
  const categoryName = t(locale, `categories.${guide.category}.name`);
  const category = site.categories.find((c) => c.slug === guide.category);
  const categoryRoute = site.localizedRoute(category.route, locale.code);
  const categoryInPortuguese = isLocale && categoryRoute === category.route;
  const url = `${origin}${guide.route}`;
  const crumbs = [
    homeCrumb(locale),
    { name: t(locale, 'chrome.guides'), route: localeHubRoute(locale, 'guias') },
    categoryInPortuguese
      ? { name: t(locale, 'chrome.linkInPortuguese', { label: categoryName }), route: categoryRoute, hreflang: 'pt-BR' }
      : { name: categoryName, route: categoryRoute },
    { name: guide.title, route: guide.route },
  ];
  const sections = guide.headings.filter((h) => h.level === 2);
  const toc = sections.length >= 3
    ? `<nav class="article-toc" aria-label="${esc(t(locale, 'chrome.toc'))}">\n        <p class="article-toc-title">${esc(t(locale, 'chrome.toc'))}</p>\n        <ol>${sections.map((h) => `<li><a href="#${h.id}">${h.html}</a></li>`).join('')}</ol>\n      </nav>`
    : '';
  const pool = isLocale ? site.translations.filter((entry) => entry.locale === guide.locale && entry.kind === 'guide') : site.guides;
  const readAlso = relatedGuides(site, guide, 3, pool);
  const team = esc(t(locale, 'chrome.team'));
  const policy = localeLink(site, locale, '/politica-editorial/', esc(t(locale, 'chrome.policy')));
  const staleNote = staleBanner(guide, locale);
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
        inLanguage: locale.htmlLang,
        datePublished: guide.datePublished,
        dateModified: guide.dateModified,
        articleSection: categoryName,
        keywords: guide.tags.join(', '),
        image: `${origin}/assets/brand/og-image-v1.png`,
        author: teamNode(origin, locale),
        publisher: publisherNode(origin, locale),
        ...(guide.sources?.length > 0 ? {
          citation: guide.sources.map((source) => ({ '@type': 'CreativeWork', name: source.name, url: source.url })),
        } : {}),
        isPartOf: { '@id': `${origin}/#website` },
      },
      breadcrumbNode(crumbs, origin, url),
    ],
  };
  const main = `${breadcrumbNav(crumbs, locale)}

      <article class="legal-article guide-article">
        <p class="article-kicker">${localizedNavigationLink(site, locale, category.route, categoryName)}</p>
        <h1>${esc(guide.headline)}</h1>
        <p class="article-lead">${esc(guide.description)}</p>
        ${datesLine(site, guide, locale)}
        ${staleNote ? `${staleNote}\n        ` : ''}<p class="article-editorial">${t(locale, 'chrome.editorial', { team, policy })}</p>
        ${toc}
${guide.html}
        <section class="article-section" aria-labelledby="continue-explorando">
          <h2 id="continue-explorando">${esc(t(locale, 'chrome.continue'))}</h2>
          ${linkList(site, guide.related)}
        </section>
        <section class="article-section" aria-labelledby="leia-tambem">
          <h2 id="leia-tambem">${esc(t(locale, 'chrome.readAlso'))}</h2>
          ${cardList(readAlso, site, locale)}
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
        `<meta property="article:section" content="${esc(categoryName)}">`,
      ],
      jsonLd,
      feed: !isLocale,
      locale,
      robots: guide.stale ? 'noindex,follow' : undefined,
      canonical: !guide.stale,
    }),
    bodyClass: 'landing-home learning-page article-page',
    main,
    locale,
  });
}

// ---------------------------------------------------------------------------
// Hubs.

export function renderGuidesHub(site, origin, locale) {
  const isLocale = Boolean(locale.prefix);
  const hub = site.registry.get('/guias/');
  const route = localeHubRoute(locale, 'guias');
  const url = `${origin}${route}`;
  const title = isLocale ? t(locale, 'chrome.guidesHubTitle') : hub.title;
  const description = isLocale ? t(locale, 'chrome.guidesHubIntro') : hub.description;
  const entries = isLocale ? site.translations.filter((entry) => entry.locale === locale.code && entry.kind === 'guide') : site.guides;
  const crumbs = [homeCrumb(locale), { name: t(locale, 'chrome.guides'), route }];
  const groups = site.categories.map((category) => ({
    category,
    members: entries.filter((guide) => guide.category === category.slug),
  })).filter(({ members }) => members.length > 0);
  const blocks = groups.map(({ category, members }) => {
    const name = t(locale, `categories.${category.slug}.name`);
    const intro = isLocale ? t(locale, `categories.${category.slug}.intro`) : category.intro;
    const all = t(locale, 'chrome.allGuidesOf', { category: name.toLocaleLowerCase(locale.code) });
    return `<h2 id="${category.slug}">${esc(name)}</h2>
        <p>${esc(intro)}</p>
        ${cardList(members)}
        <p>${localizedNavigationLink(site, locale, category.route, all, { className: 'text-link' })}</p>`;
  });
  const toolsHub = localeHubRoute(locale, 'ferramentas');
  const tools = isLocale ? site.translations.filter((entry) => entry.locale === locale.code && entry.kind === 'tool') : site.tools;
  const jsonLd = {
    '@context': 'https://schema.org',
    '@graph': [
      {
        '@type': 'CollectionPage',
        '@id': `${url}#webpage`,
        url,
        name: htmlTitle(title),
        description,
        inLanguage: locale.htmlLang,
        isPartOf: { '@id': `${origin}/#website` },
        mainEntity: itemListNode(entries, origin),
      },
      breadcrumbNode(crumbs, origin, url),
    ],
  };
  const toolLink = `<a href="${toolsHub}">${esc(t(locale, 'chrome.toolsLabel'))}</a>`;
  const teamLink = localeLink(site, locale, '/sobre/', esc(t(locale, 'chrome.team')));
  const pricesLink = `<a href="${localeRoute(locale, 'prices')}">${esc(t(locale, 'chrome.pricesLabel'))}</a>`;
  const policyLink = localeLink(site, locale, '/politica-editorial/', esc(t(locale, 'chrome.policy')));
  const main = `${breadcrumbNav(crumbs, locale)}
      <article class="legal-article">
        <h1>${esc(isLocale ? title : t(locale, 'chrome.guidesTitle'))}</h1>
        <p class="article-lead">${esc(isLocale ? description : hub.intro)}</p>
        <nav class="category-nav" aria-label="${esc(t(locale, 'chrome.categoriesNav'))}">
          <ul class="chip-list">${groups.map(({ category }) => `<li>${localizedNavigationLink(site, locale, category.route, t(locale, `categories.${category.slug}.name`))}</li>`).join('')}</ul>
        </nav>
        ${blocks.join('\n        ')}
        ${tools.length > 0 ? `<h2 id="ferramentas">${esc(t(locale, 'chrome.calculators'))}</h2>
        <p>${t(locale, 'chrome.guidesExplain', { tools: toolLink })}</p>
        ${linkList(site, tools.map((entry) => entry.route))}` : ''}
        <h2 id="sobre-os-guias">${esc(t(locale, 'chrome.howGuides'))}</h2>
        <p>${t(locale, 'chrome.howGuidesBody', { team: teamLink, prices: pricesLink, policy: policyLink })}</p>
      </article>`;
  return documentHtml({
    head: headHtml({ origin, route, title, description, imageAlt: title, jsonLd, feed: !isLocale, locale }),
    bodyClass: 'landing-home learning-page learning-hub',
    main,
    locale,
  });
}

export function renderCategory(site, entry, origin, locale) {
  const guides = localeHubRoute(locale, 'guias');
  const url = `${origin}${entry.route}`;
  const guidePool = entry.locale ? site.translations.filter((guide) => guide.locale === entry.locale && guide.kind === 'guide') : site.guides;
  const members = guidePool.filter((guide) => guide.category === entry.slug);
  const crumbs = [
    homeCrumb(locale),
    { name: t(locale, 'chrome.guides'), route: guides },
    { name: entry.name, route: entry.route },
  ];
  const others = site.categories.filter((category) => category.slug !== entry.slug);
  const jsonLd = {
    '@context': 'https://schema.org',
    '@graph': [
      {
        '@type': 'CollectionPage',
        '@id': `${url}#webpage`,
        url,
        name: htmlTitle(entry.title),
        description: entry.description,
        inLanguage: locale.htmlLang,
        isPartOf: { '@id': `${origin}/#website` },
        mainEntity: itemListNode(members, origin),
      },
      breadcrumbNode(crumbs, origin, url),
    ],
  };
  const guidesLink = localizedNavigationLink(site, locale, '/guias/', t(locale, 'chrome.guidesLabel'));
  const toolsLink = localizedNavigationLink(site, locale, '/ferramentas/', t(locale, 'chrome.calculatorsShort'));
  const pricesLink = `<a href="${localeRoute(locale, 'prices')}">${esc(t(locale, 'chrome.pricesLabel'))}</a>`;
  const main = `${breadcrumbNav(crumbs, locale)}
      <article class="legal-article">
        <h1>${esc(entry.title)}</h1>
        <p class="article-lead">${esc(entry.intro)}</p>
        <h2 id="artigos">${esc(t(locale, 'chrome.articles'))}</h2>
        ${cardList(members)}
        <h2 id="outras-categorias">${esc(t(locale, 'chrome.otherCategories'))}</h2>
        <ul class="chip-list">${others.map((category) => `<li>${localizedNavigationLink(site, locale, category.route, t(locale, `categories.${category.slug}.name`))}</li>`).join('')}</ul>
        <p>${t(locale, 'chrome.categoryFooter', { guides: guidesLink, tools: toolsLink, prices: pricesLink })}</p>
      </article>`;
  return documentHtml({
    head: headHtml({ origin, route: entry.route, title: entry.title, description: entry.description, imageAlt: entry.title, jsonLd, locale }),
    bodyClass: 'landing-home learning-page learning-hub',
    main,
    locale,
  });
}

export function renderToolsHub(site, origin, locale) {
  const isLocale = Boolean(locale.prefix);
  const hub = site.registry.get('/ferramentas/');
  const route = isLocale ? localeHubRoute(locale, 'ferramentas') : '/ferramentas/';
  const url = `${origin}${route}`;
  const title = isLocale ? t(locale, 'chrome.toolsHubTitle') : hub.title;
  const description = isLocale ? t(locale, 'chrome.toolsHubIntro') : hub.description;
  const entries = isLocale ? site.translations.filter((entry) => entry.locale === locale.code && entry.kind === 'tool') : site.tools;
  const crumbs = [homeCrumb(locale), { name: t(locale, 'chrome.tools'), route }];
  const jsonLd = {
    '@context': 'https://schema.org',
    '@graph': [
      {
        '@type': 'CollectionPage',
        '@id': `${url}#webpage`,
        url,
        name: htmlTitle(title),
        description,
        inLanguage: locale.htmlLang,
        isPartOf: { '@id': `${origin}/#website` },
        mainEntity: itemListNode(entries, origin),
      },
      breadcrumbNode(crumbs, origin, url),
    ],
  };
  const guideRoutes = ['/guias/etanol-ou-gasolina/', '/guias/como-calcular-consumo-medio/', '/guias/como-economizar-combustivel/']
    .filter((entryRoute) => site.registry.has(entryRoute));
  const pricesLink = `<a href="${localeRoute(locale, 'prices')}">${esc(t(locale, 'chrome.pricesAnpLabel'))}</a>`;
  const methodologyLink = `<a href="${localeRoute(locale, 'methodology')}">${esc(t(locale, 'chrome.methodology'))}</a>`;
  const main = `${breadcrumbNav(crumbs, locale)}
      <article class="legal-article">
        <h1>${esc(isLocale ? title : t(locale, 'chrome.calculatorsTitle'))}</h1>
        <p class="article-lead">${esc(isLocale ? description : hub.intro)}</p>
        <h2 id="calculadoras">${esc(t(locale, 'chrome.chooseCalculator'))}</h2>
        ${cardList(entries, site, locale)}
        <h2 id="como-funcionam">${esc(t(locale, 'chrome.howCalculators'))}</h2>
        <p>${esc(t(locale, 'chrome.howCalculatorsBody'))}</p>
        <p>${esc(t(locale, 'chrome.estimates'))}</p>
        <h2 id="precos-de-referencia">${esc(t(locale, 'chrome.wherePrices'))}</h2>
        <p>${t(locale, 'chrome.wherePricesBody', { prices: pricesLink, methodology: methodologyLink })}</p>
        ${guideRoutes.length > 0 ? `<h2 id="guias-relacionados">${esc(t(locale, 'chrome.relatedGuides'))}</h2>
        ${linkList(site, guideRoutes, locale)}` : ''}
      </article>`;
  return documentHtml({
    head: headHtml({ origin, route, title, description, imageAlt: title, jsonLd, locale }),
    bodyClass: 'landing-home learning-page learning-hub',
    main,
    locale,
  });
}

// ---------------------------------------------------------------------------
// Calculator page.

function toolField(field, locale, sourceSlug) {
  const localized = Boolean(locale.prefix) ? locale.messages.tools[sourceSlug]?.fields?.[field.name] : null;
  if (localized) {
    field = {
      ...field,
      label: localized.label ?? field.label,
      hint: localized.hint ?? field.hint,
      options: field.options ? field.options.map(([value, label]) => [value, localized.options?.[value] ?? label]) : undefined,
    };
  }
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

function toolI18nScript(locale, sourceSlug) {
  const toolMessages = locale.messages.tools[sourceSlug];
  if (!toolMessages) throw new Error(`i18n ${locale.code}: no calculator strings for ${sourceSlug}`);
  const fields = Object.fromEntries(Object.entries(toolMessages.fields).map(([name, field]) => [name, {
    label: field.label,
    ...(field.unit ? { unit: field.unit } : {}),
    ...(field.example ? { example: field.example } : {}),
  }]));
  const data = {
    decimal: locale.decimal,
    fields,
    messages: toolMessages.messages,
    validation: {
      ...locale.messages.tools.common.validation,
      review: locale.messages.tools.common.review,
      unknown: locale.messages.tools.common.unknown,
    },
  };
  const json = JSON.stringify(data)
    .replace(/</g, '\\u003c')
    .replace(/>/g, '\\u003e')
    .replace(/&/g, '\\u0026');
  return `<script type="application/json" id="tool-i18n">${json}</script>`;
}

export function renderTool(site, tool, origin, locale) {
  const isLocale = Boolean(locale.prefix);
  const sourceSlug = tool.formSlug || tool.slug;
  const toolsHub = localeHubRoute(locale, 'ferramentas');
  const url = `${origin}${tool.route}`;
  const crumbs = [
    homeCrumb(locale),
    { name: t(locale, 'chrome.tools'), route: toolsHub },
    { name: tool.title, route: tool.route },
  ];
  const fields = TOOL_FORMS[sourceSlug];
  const others = isLocale
    ? site.translations.filter((entry) => entry.locale === locale.code && entry.kind === 'tool' && entry.route !== tool.route)
    : site.tools.filter((entry) => entry.route !== tool.route);
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
        operatingSystem: t(locale, 'chrome.browserOs'),
        browserRequirements: t(locale, 'chrome.browserRequirements'),
        inLanguage: locale.htmlLang,
        isAccessibleForFree: true,
        offers: { '@type': 'Offer', price: '0', priceCurrency: 'BRL' },
        datePublished: tool.datePublished,
        dateModified: tool.dateModified,
        publisher: publisherNode(origin, locale),
        isPartOf: { '@id': `${origin}/#website` },
      },
      breadcrumbNode(crumbs, origin, url),
    ],
  };
  const pricesLink = `<a href="${localeRoute(locale, 'prices')}">${esc(t(locale, 'chrome.pricesAnpLabel'))}</a>`;
  const main = `${breadcrumbNav(crumbs, locale)}
      <article class="legal-article tool-article">
        <h1>${esc(tool.headline)}</h1>
        <p class="article-lead">${esc(tool.description)}</p>
        <div class="tool-panel" data-tool-panel>
          <p class="tool-fallback" data-tool-fallback>${esc(t(locale, 'chrome.toolFallback'))}</p>
          <form class="tool-form" data-tool="${esc(sourceSlug)}" novalidate hidden>
            ${fields.map((field) => toolField(field, locale, sourceSlug)).join('\n            ')}
            <div class="tool-actions"><button type="submit" class="btn-dark">${esc(t(locale, 'chrome.calculate'))}</button></div>
          </form>
          <div class="tool-result" data-tool-result role="status" aria-live="polite"></div>
          <p class="tool-privacy">${esc(t(locale, 'chrome.toolPrivacy'))}</p>
        </div>
        ${datesLine(site, tool, locale, { published: false })}${tool.stale ? `\n        ${staleBanner(tool, locale)}` : ''}
${tool.html}
        <section class="article-section" aria-labelledby="precos-de-referencia">
          <h2 id="precos-de-referencia">${esc(t(locale, 'chrome.toolPrices'))}</h2>
          <p>${t(locale, 'chrome.toolPricesBody', { prices: pricesLink })}</p>
        </section>
        <section class="article-section" aria-labelledby="outras-ferramentas">
          <h2 id="outras-ferramentas">${esc(t(locale, 'chrome.otherTools'))}</h2>
          ${linkList(site, others.map((entry) => entry.route))}
        </section>
        <section class="article-section" aria-labelledby="guias-relacionados">
          <h2 id="guias-relacionados">${esc(t(locale, 'chrome.relatedGuides'))}</h2>
          ${cardList(relatedGuidesList.map((route) => site.registry.get(route)), site, locale)}
          ${relatedOther.length > 0 ? linkList(site, relatedOther) : ''}
        </section>
      </article>`;
  return documentHtml({
    head: headHtml({
      origin,
      route: tool.route,
      title: tool.title,
      description: tool.description,
      imageAlt: tool.headline,
      jsonLd,
      scripts: ['/assets/tools.js'],
      locale,
      robots: tool.stale ? 'noindex,follow' : undefined,
      canonical: !tool.stale,
      dataScript: isLocale ? toolI18nScript(locale, sourceSlug) : undefined,
    }),
    bodyClass: 'landing-home learning-page tool-page',
    main,
    locale,
  });
}

// ---------------------------------------------------------------------------
// Translated app tutorials.

export function renderTutorial(site, entry, origin, locale) {
  const hub = entry.kind === 'tutorial-hub';
  const url = `${origin}${entry.route}`;
  const title = hub ? entry.title : `${entry.title} | ${entry.tutorial.common.metaLabel}`;
  const crumbs = [homeCrumb(locale)];
  if (!hub) {
    const hubEntry = site.registry.get(entry.tutorial.hubRoute);
    crumbs.push({ name: hubEntry.title, route: hubEntry.route });
  }
  crumbs.push({ name: entry.title, route: entry.route });
  const jsonLd = {
    '@context': 'https://schema.org',
    '@graph': [
      {
        '@type': hub ? 'CollectionPage' : 'Article',
        '@id': `${url}#webpage`,
        url,
        name: htmlTitle(title),
        description: entry.description,
        dateModified: entry.lastmod,
        inLanguage: locale.htmlLang,
        isPartOf: { '@id': `${origin}/#website` },
        publisher: publisherNode(origin, locale),
      },
      breadcrumbNode(crumbs, origin, url),
    ],
  };
  const navigation = (route, label) => localizedNavigationLink(site, locale, route, label);
  const main = `${breadcrumbNav(crumbs, locale)}
      ${renderTutorialContent(entry, navigation)}`;
  return documentHtml({
    head: headHtml({
      origin, route: entry.route, title, description: entry.description,
      ogType: hub ? 'website' : 'article', jsonLd, locale,
    }),
    bodyClass: `landing-home learning-page${hub ? ' learning-hub' : ''}`,
    main,
    locale,
  });
}

// ---------------------------------------------------------------------------
// E-E-A-T pages (/sobre/, /politica-editorial/).

export function renderPage(site, page, origin, locale) {
  const url = `${origin}${page.route}`;
  const crumbs = [homeCrumb(locale), { name: page.title, route: page.route }];
  const graph = [
    {
      '@type': page.schemaType,
      '@id': `${url}#webpage`,
      url,
      name: htmlTitle(page.title),
      description: page.description,
      inLanguage: locale.htmlLang,
      dateModified: page.dateModified,
      isPartOf: { '@id': `${origin}/#website` },
      about: { '@id': `${origin}/#organization` },
      publisher: publisherNode(origin, locale),
    },
    breadcrumbNode(crumbs, origin, url),
  ];
  if ((page.translationOf || page.route) === '/sobre/') {
    graph.push({
      '@type': 'Organization',
      '@id': `${url}#equipe`,
      name: t(locale, 'chrome.team'),
      url,
      parentOrganization: { '@id': `${origin}/#organization` },
      publishingPrinciples: `${origin}${site.localizedRoute('/politica-editorial/', locale.code)}`,
      sameAs: [GITHUB_URL],
    });
  }
  const updated = `<time datetime="${page.dateModified}">${formatDateLong(page.dateModified, locale)}</time>`;
  const legalNotice = page.legal
    ? `<p class="callout">${esc(t(locale, 'chrome.legalTranslation'))} <a href="${esc(page.translationOf)}" hreflang="pt-BR" lang="pt-BR">Português</a></p>`
    : '';
  const main = `${breadcrumbNav(crumbs, locale)}
      <article class="legal-article">
        <h1>${esc(page.headline)}</h1>
        <p class="article-lead">${esc(page.description)}</p>
        <p class="article-meta">${t(locale, 'chrome.pageUpdated', { date: updated, team: esc(t(locale, 'chrome.team')) })}</p>${page.stale ? `\n        ${staleBanner(page, locale)}` : ''}
${legalNotice}
${page.html}
      </article>`;
  return documentHtml({
    head: headHtml({
      origin,
      route: page.route,
      title: page.title,
      description: page.description,
      imageAlt: page.headline,
      jsonLd: { '@context': 'https://schema.org', '@graph': graph },
      locale,
      robots: page.legal || page.stale ? 'noindex,follow' : undefined,
      canonical: !page.legal && !page.stale,
    }),
    bodyClass: 'landing-home legal-document',
    main,
    locale,
  });
}

// ---------------------------------------------------------------------------
// Shared footer columns and home sections.

export function renderFooterColumns(site, locale) {
  const localized = Boolean(locale.prefix);
  const chrome = (key) => t(locale, `chrome.${key}`);
  const link = (source, key) => localizedNavigationLink(site, locale, source, chrome(key));
  const portal = (kind, key) => `<a href="${localeRoute(locale, kind)}">${esc(chrome(key))}</a>`;
  const states = STATES.map((state) => `<li><a href="${localeRoute(locale, 'prices', '', state.uf)}">${esc(state.name)}</a></li>`).join('');
  const categories = site.categories.map((category) =>
    localizedNavigationLink(site, locale, category.route, t(locale, `categories.${category.slug}.name`)),
  ).join('');
  const tools = site.tools.map((tool) => {
    const target = site.localizedRoute(tool.route, locale.code);
    const label = target === tool.route ? TOOL_NAV_LABELS[tool.slug] || tool.title : site.routeLabel(target);
    return localizedNavigationLink(site, locale, tool.route, label);
  }).join('');
  const appHref = localized ? `${locale.prefix}/#aplicativo` : '#aplicativo';
  return `<div class="footer-cols footer-cols-hubs">
        <nav aria-label="${esc(chrome('prices'))}" class="footer-col-precos">
          <h2>${esc(chrome('prices'))}</h2>
          ${portal('prices', 'footerPrices')}
          ${link('/consultar/', 'footerConsult')}
          ${link('/fiscalizacao/', 'footerEnforcement')}
          ${portal('methodology', 'footerMethodology')}
          ${localized ? portal('data', 'footerData') : ''}
          <ul class="footer-states">${states}</ul>
        </nav>
        <nav aria-label="${esc(chrome('guides'))}">
          <h2>${esc(chrome('guides'))}</h2>
          ${link('/guias/', 'footerAllGuides')}
          ${categories}
          ${link('/como-usar/', 'footerHowTo')}
          <a href="${appHref}">${esc(chrome('footerApp'))}</a>
        </nav>
        <nav aria-label="${esc(chrome('tools'))}">
          <h2>${esc(chrome('tools'))}</h2>
          ${link('/ferramentas/', 'footerAllTools')}
          ${tools}
        </nav>
        <nav aria-label="${esc(chrome('project'))}">
          <h2>${esc(chrome('project'))}</h2>
          ${link('/sobre/', 'footerAbout')}
          ${link('/politica-editorial/', 'footerEditorial')}
          ${link('/contribuidores/', 'footerContributors')}
          ${link('/forum/', 'footerForum')}
          <a href="${GITHUB_URL}">${esc(chrome('github'))}</a>
          <a href="${GITHUB_URL}/issues">${esc(chrome('footerContact'))}</a>
          <a href="${GITHUB_URL}/blob/main/TRADEMARKS.md">${esc(chrome('footerTrademarks'))}</a>
          ${localized ? `<a href="/" lang="pt-BR" hreflang="pt-BR">${NATIVE_NAMES['pt-BR']}</a>` : ''}
        </nav>
        <nav aria-label="${esc(chrome('privacy'))}">
          <h2>${esc(chrome('privacy'))}</h2>
          ${link('/privacidade/', 'footerPrivacy')}
          ${link('/excluir-conta/', 'footerDelete')}
          ${link('/termos/', 'footerTerms')}
        </nav>
      </div>`;
}

export function renderHomeStates(locale) {
  const items = STATES.map((s) => `<li><a href="${localeRoute(locale, 'prices', '', s.uf)}"><span class="state-uf" aria-hidden="true">${s.uf.toUpperCase()}</span><span class="state-name">${esc(s.name)}</span></a></li>`);
  return `<section id="precos-por-estado" class="block home-states">
      <div class="shell">
        <div class="block-head">
          <h2 class="h2">${esc(t(locale, 'chrome.homeStatesTitle'))}</h2>
        </div>
        <ul class="state-grid">
          ${items.join('\n          ')}
        </ul>
        <div class="lp-actions"><a class="btn-dark" href="${localeRoute(locale, 'prices')}">${esc(t(locale, 'chrome.homeSeePrices'))}</a><a class="text-link" href="${localeRoute(locale, 'methodology')}">${esc(t(locale, 'chrome.homeHowCollected'))}</a></div>
      </div>
    </section>`;
}

export function renderHomeHero(locale) {
  return `<p class="kicker"><span class="dot" aria-hidden="true"></span>${esc(t(locale, 'home.hero.kicker'))}</p>
          <h1 class="lp-title">${esc(t(locale, 'home.hero.title'))}</h1>
          <p class="lp-lead">${esc(t(locale, 'home.hero.lead'))}</p>`;
}

export function renderHomeGuides(site, locale) {
  const isLocale = Boolean(locale.prefix);
  const entries = site.popularGuides
    .map((guide) => site.registry.get(site.localizedRoute(guide.route, locale.code)))
    .filter((guide) => !isLocale || guide.locale === locale.code);
  if (isLocale && entries.length === 0) return '';
  const allRoute = isLocale ? localeHubRoute(locale, 'guias') : '/guias/';
  return `<section id="guias-populares" class="block home-topics">
      <div class="shell">
        <div class="block-head">
          <h2 class="h2">${esc(t(locale, 'chrome.homeGuides'))}</h2>
        </div>
        ${cardList(entries, site, locale)}
        <div class="lp-actions"><a class="text-link" href="${allRoute}">${esc(t(locale, 'chrome.homeAllGuides'))}</a></div>
      </div>
    </section>`;
}

export function renderHomeTools(site, locale) {
  const isLocale = Boolean(locale.prefix);
  const entries = site.tools
    .map((tool) => site.registry.get(site.localizedRoute(tool.route, locale.code)))
    .filter((tool) => !isLocale || tool.locale === locale.code);
  if (isLocale && entries.length === 0) return '';
  const allRoute = isLocale ? localeHubRoute(locale, 'ferramentas') : '/ferramentas/';
  return `<section id="ferramentas" class="block home-topics">
      <div class="shell">
        <div class="block-head">
          <h2 class="h2">${esc(t(locale, 'chrome.homeTools'))}</h2>
        </div>
        ${cardList(entries, site, locale)}
        <div class="lp-actions"><a class="text-link" href="${allRoute}">${esc(t(locale, 'chrome.homeAllTools'))}</a></div>
      </div>
    </section>`;
}

/** Render the same home modules for every language from the canonical home template. */
export function renderHomeMain(template, site, locale, storeState) {
  const copy = (key) => esc(t(locale, 'home.' + key));
  const route = (source) => {
    const kind = { '/precos/': 'prices', '/metodologia/': 'methodology' }[source];
    const href = kind ? localeRoute(locale, kind) : site.localizedRoute(source, locale.code);
    return `href="${esc(href)}"${locale.prefix && href === source ? ' hreflang="pt-BR"' : ''}`;
  };
  let main = template
    .replace('{{HOME_SEARCH_ACTION}}', () => esc(localeRoute(locale, 'search')))
    .replace('<!-- HOME_HERO -->', () => renderHomeHero(locale))
    .replace('<!-- HOME_STATES -->', () => renderHomeStates(locale))
    .replace('<!-- HOME_GUIDES -->', () => renderHomeGuides(site, locale))
    .replace('<!-- HOME_TOOLS -->', () => renderHomeTools(site, locale))
    .replaceAll(/\{\{HOME_TEXT:([a-zA-Z.]+)\}\}/g, (_, key) => copy(key))
    .replaceAll(/\{\{HOME_STORE_TEXT:([a-zA-Z.]+)\}\}/g, (_, key) => copy(key + '.' + storeState))
    .replaceAll(/\{\{HOME_LINK:(\/[a-z/-]+\/?)\}\}/g, (_, source) => route(source));
  if (locale.prefix) {
    const features = ['weekly', 'community', 'navigation', 'fill', 'offline', 'forum'];
    const descriptions = features.map((key) =>
      `<figure class="app-shot" role="listitem"><figcaption><strong>${copy(`app.features.${key}.title`)}</strong> ${copy(`app.features.${key}.body`)}</figcaption></figure>`
    );
    const gallery = `<div class="app-gallery" role="list" aria-label="${copy('app.featuresLabel')}" tabindex="0">
          ${descriptions.join('\n          ')}
        </div>`;
    main = main
      .replace(/<!-- HOME_CAPTURES -->[\s\S]*?<!-- \/HOME_CAPTURES -->/, () => gallery)
      .replace('<!-- HOME_CAPTURE_LINK -->', () => `<a class="text-link" href="/#aplicativo" hreflang="pt-BR">${copy('app.portugueseCaptures')}</a>`);
  }
  if (/\{\{HOME_/.test(main)) throw new Error('Unrendered home template marker for ' + locale.code);
  return main;
}

export function renderLocaleHome(origin, locale, main, appSchema) {
  const route = `${locale.prefix}/`;
  const url = `${origin}${route}`;
  const title = t(locale, 'home.title');
  const description = t(locale, 'home.description');
  const jsonLd = {
    '@context': 'https://schema.org',
    '@graph': [
      {
        '@type': 'WebPage',
        '@id': `${url}#webpage`,
        url,
        name: htmlTitle(title),
        description,
        inLanguage: locale.htmlLang,
        isPartOf: { '@id': `${origin}/#website` },
        about: { '@id': `${origin}/#organization` },
        publisher: publisherNode(origin, locale),
      },
      ...(appSchema ? [appSchema] : []),
    ],
  };
  const head = headHtml({ origin, route, title, description, imageAlt: title, jsonLd, locale });
  return `<!DOCTYPE html>
<html lang="${esc(locale.htmlLang)}">
${head}
<body class="landing-home">
  <a href="#conteudo" class="skip-link">${esc(t(locale, 'chrome.skip'))}</a>

  <!-- LEARNING_HEADER -->

  ${main}

  <!-- LEARNING_FOOTER -->
</body>
</html>
`;
}

// ---------------------------------------------------------------------------
// Localized chrome and the language switcher.

function localizedPortalLinks(locale) {
  return [['prices', 'footerPrices'], ['methodology', 'footerMethodology'], ['data', 'footerData']]
    .map(([kind, label]) => `<a href="${localeRoute(locale, kind)}">${esc(t(locale, `chrome.${label}`))}</a>`);
}

export function renderLocalizedHeader(site, locale, languagePicker) {
  const homeHref = `${locale.prefix}/`;
  const chrome = (key) => t(locale, `chrome.${key}`);
  const link = (source, key, options) => localizedNavigationLink(site, locale, source, chrome(key), options);
  const sectionLink = (section, label) => `<a href="${homeHref}#${section}">${esc(label)}</a>`;
  const appLink = sectionLink('aplicativo', chrome('footerApp'));
  const hubs = [link('/como-usar/', 'footerHowTo'), link('/guias/', 'guides'), link('/ferramentas/', 'tools')];
  const portal = localizedPortalLinks(locale);
  const nav = [
    portal[0],
    appLink,
    ...hubs,
  ].join('\n        ');
  const menu = [
    sectionLink('explorar', chrome('navExplore')),
    sectionLink('dados-abertos', t(locale, 'home.data.title')),
    appLink,
    sectionLink('perguntas', chrome('navQuestions')),
    link('/consultar/', 'navConsultData'),
    ...hubs,
    ...portal,
    link('/sobre/', 'footerAbout'),
    link('/politica-editorial/', 'footerEditorial'),
    link('/privacidade/', 'footerPrivacy'),
  ].join('\n            ');
  return `<header class="topbar">
    <div class="shell topbar-inner">
      <a href="${homeHref}" class="brand" aria-label="${esc(t(locale, 'chrome.brandHome'))}">
        <img src="/assets/brand/abastevo-logo.svg" alt="" width="28" height="28">
        <img src="/assets/brand/abastevo-wordmark.svg" alt="abastevo" class="brand-word" width="96" height="12">
      </a>
      <nav class="topnav" aria-label="${esc(t(locale, 'chrome.mainNav'))}">
        ${nav}
      </nav>
      <div class="topbar-actions">
        <a class="topbar-link" href="${GITHUB_URL}" target="_blank" rel="noopener" aria-label="${esc(t(locale, 'chrome.githubLabel'))}"><img class="topbar-github-mark" src="/assets/brand/github-mark.svg" width="18" height="18" alt=""><span class="hide-sm">${esc(t(locale, 'chrome.github'))}</span></a>
        ${link('/consultar/', 'navConsultData', { className: 'pill-light hide-sm' })}
        ${languagePicker}
        <details class="nav-toggle" name="landing-header">
          <summary class="nav-button" aria-label="${esc(t(locale, 'chrome.openMenu'))}"><span aria-hidden="true"></span></summary>
          <nav class="site-nav" aria-label="${esc(t(locale, 'chrome.mainNav'))}">
            ${menu}
          </nav>
        </details>
      </div>
    </div>
  </header>`;
}

export function renderLocalizedFooter(site, locale) {
  return `<footer class="footer">
    <div class="shell">
      <div class="footer-top">
        <a href="${locale.prefix}/" class="brand" aria-label="${esc(t(locale, 'chrome.brandHome'))}">
          <img src="/assets/brand/abastevo-logo.svg" alt="" width="28" height="28">
          <img src="/assets/brand/abastevo-wordmark.svg" alt="abastevo" class="brand-word" width="96" height="12">
        </a>
        <p class="footer-tagline">${esc(t(locale, 'chrome.footerTagline'))}</p>
      </div>
      ${renderFooterColumns(site, locale)}
      <div class="footer-legal">
        <p>${esc(t(locale, 'chrome.footerLegal'))}</p>
      </div>
    </div>
  </footer>`;
}

function renderLanguageLink(member) {
  return `<a href="${esc(member.route)}" lang="${esc(member.htmlLang)}" hreflang="${esc(member.hreflang)}">${esc(member.languageName)}</a>`;
}

export function renderHeaderLanguagePicker(members, locale) {
  const label = t(locale, 'chrome.languages');
  const options = members.map((member) => member.code === locale.code
    ? `<li><span lang="${esc(member.htmlLang)}" aria-current="true">${esc(member.languageName)}</span></li>`
    : `<li>${renderLanguageLink(member)}</li>`);
  return `<details class="language-picker" name="landing-header">
          <summary aria-label="${esc(label)} · ${esc(locale.messages.languageName)}">
            <span class="language-picker-globe" aria-hidden="true"></span>
            <span class="language-picker-code" aria-hidden="true">${esc(locale.code)}</span>
          </summary>
          <ul class="language-picker-options">
            ${options.join('\n            ')}
          </ul>
        </details>`;
}

/** Plain links to the other active locales (or exact translations when available); the build inserts it before the footer. */
export function renderLanguageSwitch(members, currentRoute, locale) {
  const others = members.filter((member) => member.code !== locale.code && member.route !== currentRoute);
  if (others.length === 0) return '';
  const links = others.map(renderLanguageLink);
  return `<nav class="lang-switch" aria-label="${esc(t(locale, 'chrome.languages'))}">\n        <div class="shell">\n          ${links.join('\n          ')}\n        </div>\n      </nav>\n      `;
}
