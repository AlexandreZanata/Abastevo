// Content manifest loader (SEO-08): validates landing/content/manifest.json and
// the Markdown sources, and builds one route registry that drives page
// generation, the sitemap, llms.txt, hubs, "Leia também" and the footer.

import fs from 'node:fs';
import path from 'node:path';
import { parseFrontMatter, renderMarkdown } from './markdown.mjs';
import { isPortalRoute, portalRouteLabel, TOOL_FORMS } from './data.mjs';

export class ContentError extends Error {}

const SLUG_RE = /^[a-z0-9]+(?:-[a-z0-9]+)*$/;
const TAG_RE = /^[a-z0-9]+(?:-[a-z0-9]+)*$/;
const ROUTE_RE = /^\/(?:[a-z0-9-]+\/)*$/;
export const TITLE_BOUNDS = [25, 65];
export const DESCRIPTION_BOUNDS = [110, 165];
// Element ids emitted by the layouts; Markdown headings never reuse them.
export const RESERVED_IDS = ['conteudo', 'continue-explorando', 'leia-tambem', 'precos-de-referencia', 'outras-ferramentas', 'guias-relacionados'];

export function isValidDate(value) {
  if (typeof value !== 'string' || !/^\d{4}-\d{2}-\d{2}$/.test(value)) return false;
  const [y, m, d] = value.split('-').map(Number);
  const date = new Date(Date.UTC(y, m - 1, d));
  return date.getUTCFullYear() === y && date.getUTCMonth() === m - 1 && date.getUTCDate() === d;
}

/** `<title>` text: "{title} | abastevo" when it fits the 65-character bound. */
export function htmlTitle(title) {
  const full = `${title} | abastevo`;
  return full.length <= TITLE_BOUNDS[1] ? full : title;
}

export function outputRelFor(route) {
  if (route === '/') return 'index.html';
  return route.endsWith('/') ? `${route.slice(1)}index.html` : route.slice(1);
}

const SCHEMAS = {
  guide: {
    required: ['title', 'description', 'slug', 'category', 'tags', 'datePublished', 'dateModified', 'related'],
    optional: ['headline', 'summary'],
  },
  tool: {
    required: ['title', 'description', 'slug', 'datePublished', 'dateModified', 'related'],
    optional: ['headline', 'summary'],
  },
  page: {
    required: ['title', 'description', 'slug', 'datePublished', 'dateModified'],
    optional: ['headline', 'summary', 'schemaType'],
  },
};

function checkText(errors, where, field, value, [min, max]) {
  if (typeof value !== 'string' || value.trim() === '') {
    errors.push(`${where}: ${field} must be a non-empty string`);
    return;
  }
  if (value.length < min || value.length > max) {
    errors.push(`${where}: ${field} length ${value.length} outside [${min}, ${max}]`);
  }
}

/**
 * Validates one front matter object. `ctx` provides categories (Set) and the
 * expected slug (file name). Route existence is checked later, once the
 * registry is complete.
 */
export function validateFrontMatter(kind, data, where, ctx = {}) {
  const errors = [];
  const schema = SCHEMAS[kind];
  for (const key of schema.required) {
    if (!Object.hasOwn(data, key)) errors.push(`${where}: missing required field "${key}"`);
  }
  for (const key of Object.keys(data)) {
    if (!schema.required.includes(key) && !schema.optional.includes(key)) errors.push(`${where}: unknown field "${key}"`);
  }
  if (errors.length > 0) return errors;

  const finalTitle = typeof data.title === 'string' ? htmlTitle(data.title) : '';
  checkText(errors, where, 'title (as <title>)', finalTitle, TITLE_BOUNDS);
  checkText(errors, where, 'description', data.description, DESCRIPTION_BOUNDS);
  if (data.headline !== undefined) checkText(errors, where, 'headline', data.headline, [10, 120]);
  if (data.summary !== undefined) checkText(errors, where, 'summary', data.summary, [20, 200]);
  if (typeof data.slug !== 'string' || !SLUG_RE.test(data.slug)) errors.push(`${where}: invalid slug "${data.slug}"`);
  else if (ctx.expectedSlug && data.slug !== ctx.expectedSlug) errors.push(`${where}: slug "${data.slug}" must match file name "${ctx.expectedSlug}"`);
  for (const field of ['datePublished', 'dateModified']) {
    if (!isValidDate(data[field])) errors.push(`${where}: ${field} must be a real YYYY-MM-DD date`);
  }
  if (isValidDate(data.datePublished) && isValidDate(data.dateModified) && data.dateModified < data.datePublished) {
    errors.push(`${where}: dateModified precedes datePublished`);
  }
  if (kind === 'guide') {
    if (!ctx.categories?.has(data.category)) errors.push(`${where}: unknown category "${data.category}"`);
    if (!Array.isArray(data.tags) || data.tags.length < 1 || data.tags.length > 8) errors.push(`${where}: tags must list 1 to 8 entries`);
    else for (const tag of data.tags) if (!TAG_RE.test(tag)) errors.push(`${where}: invalid tag "${tag}"`);
  }
  if (kind === 'tool' && !Object.hasOwn(TOOL_FORMS, data.slug)) errors.push(`${where}: no calculator form for "${data.slug}"`);
  if (kind === 'page' && data.schemaType !== undefined && !['AboutPage', 'WebPage'].includes(data.schemaType)) {
    errors.push(`${where}: schemaType must be AboutPage or WebPage`);
  }
  if (schema.required.includes('related')) {
    if (!Array.isArray(data.related) || data.related.length < 1) errors.push(`${where}: related must list at least one route`);
    else for (const route of data.related) if (typeof route !== 'string' || !ROUTE_RE.test(route)) errors.push(`${where}: invalid related route "${route}"`);
  }
  return errors;
}

function readMarkdownDir(dir) {
  if (!fs.existsSync(dir)) return [];
  return fs.readdirSync(dir)
    .filter((name) => name.endsWith('.md'))
    .sort()
    .map((name) => ({ file: path.join(dir, name), name, expectedSlug: name.slice(0, -3) }));
}

function validateManifest(manifest, errors) {
  if (manifest.version !== 1) errors.push('manifest: version must be 1');
  if (!Array.isArray(manifest.staticPages) || manifest.staticPages.length === 0) errors.push('manifest: staticPages must be a non-empty array');
  if (!Array.isArray(manifest.categories) || manifest.categories.length === 0) errors.push('manifest: categories must be a non-empty array');
  for (const hub of ['guias', 'ferramentas']) {
    const meta = manifest.hubs?.[hub];
    if (!meta) {
      errors.push(`manifest: hubs.${hub} is required`);
      continue;
    }
    checkText(errors, `manifest hubs.${hub}`, 'title (as <title>)', htmlTitle(meta.title || ''), TITLE_BOUNDS);
    checkText(errors, `manifest hubs.${hub}`, 'description', meta.description, DESCRIPTION_BOUNDS);
  }
  for (const page of manifest.staticPages || []) {
    const where = `manifest page ${page.route}`;
    if (typeof page.route !== 'string' || !(ROUTE_RE.test(page.route) || page.route === '/404.html')) errors.push(`${where}: invalid route`);
    if (typeof page.source !== 'string' || page.source.includes('..') || !page.source.endsWith('.html')) errors.push(`${where}: invalid source`);
    if (!['home', 'shared', 'footer'].includes(page.chrome)) errors.push(`${where}: chrome must be home, shared or footer`);
    const indexable = page.indexable !== false;
    if (indexable && !isValidDate(page.lastmod)) errors.push(`${where}: indexable pages need a fixed lastmod date`);
    if (page.guide) {
      const g = page.guide;
      checkText(errors, where, 'guide.title', g.title, [10, 90]);
      checkText(errors, where, 'guide.summary', g.summary, [20, 200]);
      if (!manifest.categories?.some((c) => c.slug === g.category)) errors.push(`${where}: unknown guide category "${g.category}"`);
      if (!Array.isArray(g.tags) || g.tags.length === 0 || g.tags.some((t) => !TAG_RE.test(t))) errors.push(`${where}: invalid guide tags`);
    }
  }
  for (const category of manifest.categories || []) {
    const where = `manifest category ${category.slug}`;
    if (!SLUG_RE.test(category.slug || '')) errors.push(`${where}: invalid slug`);
    checkText(errors, where, 'name', category.name, [3, 40]);
    checkText(errors, where, 'title (as <title>)', htmlTitle(`Guias de ${category.name.toLowerCase()}`), TITLE_BOUNDS);
    checkText(errors, where, 'description', category.description, DESCRIPTION_BOUNDS);
    checkText(errors, where, 'intro', category.intro, [20, 400]);
  }
}

function resolveRouteLabel(registry, route) {
  const entry = registry.get(route);
  if (entry) return entry.indexable === false ? null : entry.title || entry.label || '';
  return portalRouteLabel(route);
}

/** Loads and validates every content source. Throws ContentError listing all problems. */
export function loadSite(root) {
  const contentDir = path.join(root, 'content');
  const errors = [];
  let manifest;
  try {
    manifest = JSON.parse(fs.readFileSync(path.join(contentDir, 'manifest.json'), 'utf8'));
  } catch (err) {
    throw new ContentError(`manifest: cannot read content/manifest.json (${err.message})`);
  }
  validateManifest(manifest, errors);
  if (errors.length > 0) throw new ContentError(`Content validation failed:\n  - ${errors.join('\n  - ')}`);

  const categories = manifest.categories.map((c) => ({ ...c, route: `/guias/${c.slug}/` }));
  const categorySlugs = new Set(categories.map((c) => c.slug));
  const registry = new Map();
  const add = (entry) => {
    if (registry.has(entry.route)) errors.push(`duplicate route ${entry.route}`);
    registry.set(entry.route, { indexable: true, ...entry, outputRel: outputRelFor(entry.route) });
  };

  for (const page of manifest.staticPages) {
    add({
      route: page.route,
      kind: page.guide ? 'html-guide' : 'static',
      source: page.source,
      chrome: page.chrome,
      lastmod: page.lastmod,
      indexable: page.indexable !== false,
      title: page.guide?.title,
      label: page.label,
      summary: page.guide?.summary,
      category: page.guide?.category,
      tags: page.guide?.tags || [],
    });
  }

  const parse = (kind, dir) => readMarkdownDir(path.join(contentDir, dir)).map(({ file, expectedSlug }) => {
    const where = path.relative(root, file);
    let parsed;
    try {
      parsed = parseFrontMatter(fs.readFileSync(file, 'utf8'), where);
    } catch (err) {
      errors.push(err.message);
      return null;
    }
    const problems = validateFrontMatter(kind, parsed.data, where, { categories: categorySlugs, expectedSlug });
    errors.push(...problems);
    return problems.length > 0 ? null : { where, data: parsed.data, body: parsed.body };
  }).filter(Boolean);

  const guides = parse('guide', 'guias');
  const tools = parse('tool', 'ferramentas');
  const pages = parse('page', 'paginas');

  for (const g of guides) {
    if (categorySlugs.has(g.data.slug)) errors.push(`${g.where}: slug collides with category "${g.data.slug}"`);
    add({
      route: `/guias/${g.data.slug}/`,
      kind: 'guide',
      title: g.data.title,
      headline: g.data.headline || g.data.title,
      description: g.data.description,
      summary: g.data.summary || g.data.description,
      category: g.data.category,
      tags: g.data.tags,
      related: g.data.related,
      datePublished: g.data.datePublished,
      dateModified: g.data.dateModified,
      lastmod: g.data.dateModified,
      body: g.body,
      where: g.where,
    });
  }
  for (const t of tools) {
    add({
      route: `/ferramentas/${t.data.slug}/`,
      kind: 'tool',
      slug: t.data.slug,
      title: t.data.title,
      headline: t.data.headline || t.data.title,
      description: t.data.description,
      summary: t.data.summary || t.data.description,
      related: t.data.related,
      datePublished: t.data.datePublished,
      dateModified: t.data.dateModified,
      lastmod: t.data.dateModified,
      body: t.body,
      where: t.where,
    });
  }
  for (const p of pages) {
    add({
      route: `/${p.data.slug}/`,
      kind: 'page',
      title: p.data.title,
      headline: p.data.headline || p.data.title,
      description: p.data.description,
      summary: p.data.summary || p.data.description,
      schemaType: p.data.schemaType || 'WebPage',
      datePublished: p.data.datePublished,
      dateModified: p.data.dateModified,
      lastmod: p.data.dateModified,
      body: p.body,
      where: p.where,
    });
  }

  const allGuides = [...registry.values()].filter((e) => e.kind === 'guide' || e.kind === 'html-guide');
  const maxDate = (entries) => entries.map((e) => e.lastmod).filter(Boolean).sort().at(-1);
  for (const category of categories) {
    const members = allGuides.filter((g) => g.category === category.slug);
    if (members.length === 0) errors.push(`category ${category.slug} has no guides`);
    add({
      route: category.route,
      kind: 'category',
      title: `Guias de ${category.name.toLowerCase()}`,
      name: category.name,
      slug: category.slug,
      description: category.description,
      summary: category.intro,
      intro: category.intro,
      lastmod: maxDate(members),
    });
  }
  add({ route: '/guias/', kind: 'guides-hub', ...manifest.hubs.guias, summary: manifest.hubs.guias.intro, lastmod: maxDate(allGuides) });
  const toolEntries = [...registry.values()].filter((e) => e.kind === 'tool');
  add({ route: '/ferramentas/', kind: 'tools-hub', ...manifest.hubs.ferramentas, summary: manifest.hubs.ferramentas.intro, lastmod: maxDate(toolEntries) });

  for (const route of manifest.popularGuides || []) {
    const entry = registry.get(route);
    if (!entry || (entry.kind !== 'guide' && entry.kind !== 'html-guide')) errors.push(`manifest popularGuides: ${route} is not a guide`);
  }
  for (const slug of Object.keys(TOOL_FORMS)) {
    if (!registry.has(`/ferramentas/${slug}/`)) errors.push(`calculator "${slug}" has no content/ferramentas/${slug}.md`);
  }

  // Render Markdown and check every internal link and related route.
  const checkRoute = (where, route, what) => {
    const [pathname] = route.split(/[?#]/);
    if (resolveRouteLabel(registry, pathname) === null) {
      errors.push(`${where}: ${what} ${route} is not a landing page or a known portal route`);
    }
  };
  for (const entry of registry.values()) {
    if (entry.body === undefined) continue;
    try {
      const rendered = renderMarkdown(entry.body, { reservedIds: RESERVED_IDS });
      entry.html = rendered.html;
      entry.headings = rendered.headings;
      entry.links = rendered.links;
      entry.words = rendered.words;
    } catch (err) {
      errors.push(`${entry.where}: ${err.message}`);
      continue;
    }
    for (const link of entry.links) {
      if (link.kind === 'internal') checkRoute(entry.where, link.url, 'link');
      if (link.kind === 'anchor' && !entry.headings.some((h) => `#${h.id}` === link.url)) errors.push(`${entry.where}: anchor ${link.url} has no heading`);
    }
    for (const route of entry.related || []) {
      checkRoute(entry.where, route, 'related route');
      if (resolveRouteLabel(registry, route) === '') errors.push(`${entry.where}: related route ${route} has no label in the manifest`);
      if (route === entry.route) errors.push(`${entry.where}: related route points to itself`);
    }
  }

  if (errors.length > 0) throw new ContentError(`Content validation failed:\n  - ${errors.join('\n  - ')}`);

  const site = {
    manifest,
    categories,
    registry,
    routeLabel: (route) => resolveRouteLabel(registry, route),
    guides: allGuides.sort((a, b) => a.route.localeCompare(b.route)),
    tools: toolEntries.sort((a, b) => Object.keys(TOOL_FORMS).indexOf(a.slug) - Object.keys(TOOL_FORMS).indexOf(b.slug)),
    popularGuides: (manifest.popularGuides || []).map((route) => registry.get(route)),
  };
  return site;
}

/** "Leia também": shared tags weigh most, then category, then recency. */
export function relatedGuides(site, entry, limit = 3) {
  const tags = new Set(entry.tags || []);
  return site.guides
    .filter((g) => g.route !== entry.route)
    .map((g) => ({
      g,
      score: (g.tags || []).filter((t) => tags.has(t)).length * 3 + (g.category === entry.category ? 2 : 0),
    }))
    .sort((a, b) => b.score - a.score || (b.g.lastmod || '').localeCompare(a.g.lastmod || '') || a.g.route.localeCompare(b.g.route))
    .slice(0, limit)
    .map(({ g }) => g);
}

export function renderSitemap(site, origin) {
  const urls = [...site.registry.values()]
    .filter((e) => e.indexable)
    .sort((a, b) => a.route.localeCompare(b.route));
  const body = urls.map((e) => `  <url>\n    <loc>${origin}${e.route}</loc>\n    <lastmod>${e.lastmod}</lastmod>\n  </url>`).join('\n');
  return `<?xml version="1.0" encoding="UTF-8"?>\n<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">\n${body}\n</urlset>\n`;
}

export function renderLlmsLinks(site, origin) {
  const lines = [
    `- Sobre o abastevo: ${origin}/sobre/`,
    `- Política editorial: ${origin}/politica-editorial/`,
    `- Guias (hub de artigos): ${origin}/guias/`,
  ];
  for (const category of site.categories) lines.push(`- Categoria de guias: ${category.name}: ${origin}${category.route}`);
  for (const guide of site.guides) lines.push(`- Guia: ${guide.title}: ${origin}${guide.route}`);
  lines.push(`- Ferramentas (calculadoras): ${origin}/ferramentas/`);
  for (const tool of site.tools) lines.push(`- Ferramenta: ${tool.title}: ${origin}${tool.route}`);
  return lines.join('\n');
}

export { isPortalRoute };
