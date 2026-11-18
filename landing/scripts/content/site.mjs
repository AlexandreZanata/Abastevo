// Content manifest loader (SEO-08): validates landing/content/manifest.json and
// the Markdown sources, and builds one route registry that drives page
// generation, the sitemap, llms.txt, hubs, "Leia também" and the footer.

import fs from 'node:fs';
import path from 'node:path';
import { parseFrontMatter, renderMarkdown } from './markdown.mjs';
import { isPortalRoute, portalRouteLabel, TOOL_FORMS } from './data.mjs';
import { loadLocales, localeRoute } from './i18n.mjs';

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
    optional: ['headline', 'summary', 'primaryQuery'],
  },
  tool: {
    required: ['title', 'description', 'slug', 'datePublished', 'dateModified', 'related'],
    optional: ['headline', 'summary', 'primaryQuery'],
  },
  page: {
    required: ['title', 'description', 'slug', 'datePublished', 'dateModified'],
    optional: ['headline', 'summary', 'schemaType', 'primaryQuery'],
  },
};

/**
 * Interface dictionary for one locale. Full key parity is asserted by the
 * landing tests; the build only needs the files to exist and parse.
 */
export function loadMessages(root, code) {
  const file = path.join(root, 'content', 'i18n', `${code}.json`);
  let data;
  try {
    data = JSON.parse(fs.readFileSync(file, 'utf8'));
  } catch (err) {
    throw new ContentError(`i18n: cannot read ${file} (${err.message})`);
  }
  const errors = [];
  if (typeof data.languageName !== 'string' || data.languageName.trim() === '') errors.push('languageName must be a non-empty string');
  if (typeof data.dateFormat !== 'string' || !data.dateFormat.includes('{day}') || !data.dateFormat.includes('{month}') || !data.dateFormat.includes('{year}')) {
    errors.push('dateFormat must be a string with {day}, {month} and {year}');
  }
  if (!Array.isArray(data.months) || data.months.length !== 12 || data.months.some((month) => typeof month !== 'string')) errors.push('months must list 12 names');
  for (const key of ['categories', 'chrome', 'tools']) {
    if (!data[key] || typeof data[key] !== 'object' || Array.isArray(data[key])) errors.push(`${key} must be an object`);
  }
  if (errors.length > 0) throw new ContentError(`i18n ${code}: ${errors.join('; ')}`);
  return data;
}

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
  if (data.primaryQuery !== undefined) checkText(errors, where, 'primaryQuery', data.primaryQuery, [3, 80]);
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
  if (kind === 'tool' && !Object.hasOwn(TOOL_FORMS, ctx.toolSlug || data.slug)) errors.push(`${where}: no calculator form for "${data.slug}"`);
  if (kind === 'page' && data.schemaType !== undefined && !['AboutPage', 'WebPage'].includes(data.schemaType)) {
    errors.push(`${where}: schemaType must be AboutPage or WebPage`);
  }
  if (schema.required.includes('related')) {
    if (!Array.isArray(data.related) || data.related.length < 1) errors.push(`${where}: related must list at least one route`);
    else for (const route of data.related) if (typeof route !== 'string' || !ROUTE_RE.test(route)) errors.push(`${where}: invalid related route "${route}"`);
  }
  return errors;
}

/**
 * Lookup for app captures used in Markdown, built from the asset provenance
 * (`static/assets/app/provenance.json`). Authors reference the 720w
 * derivative; the lookup returns the 9:16 display box and every shipped width
 * so the renderer can emit explicit dimensions and a srcset. Unknown sources
 * return null and fail the build in the renderer.
 */
export function loadAppCaptures(root) {
  const file = path.join(root, 'static', 'assets', 'app', 'provenance.json');
  const byStem = new Map();
  if (fs.existsSync(file)) {
    for (const output of JSON.parse(fs.readFileSync(file, 'utf8')).outputs || []) {
      const match = /^([a-z0-9-]+)-(\d+)w\.webp$/.exec(output.file);
      if (!match || Number(match[2]) !== output.width) continue;
      if (!byStem.has(match[1])) byStem.set(match[1], []);
      byStem.get(match[1]).push(output);
    }
  }
  return (src) => {
    const match = /^\/assets\/app\/([a-z0-9-]+)-720w\.webp$/.exec(src);
    const variants = match ? byStem.get(match[1]) : null;
    if (!variants || !variants.some((v) => v.width === 720)) return null;
    const sorted = [...variants].sort((a, b) => a.width - b.width);
    const smallest = sorted[0];
    return {
      width: smallest.width,
      height: smallest.height,
      candidates: sorted.map((v) => ({ src: `/assets/app/${v.file}`, width: v.width })),
    };
  };
}

/**
 * Links of a guide's "## Fontes" section, from the heading to the next level-2
 * heading: `{ name, url }` for https URLs only, ready for JSON-LD citation.
 */
export function extractSources(body) {
  const lines = String(body).replace(/\r\n?/g, '\n').split('\n');
  const start = lines.findIndex((line) => /^##\s+(?:Fontes|Sources|Fuentes)\s*#*\s*$/.test(line));
  if (start === -1) return [];
  const section = [];
  for (let i = start + 1; i < lines.length; i++) {
    if (/^##\s/.test(lines[i])) break;
    section.push(lines[i]);
  }
  const sources = [];
  for (const match of section.join('\n').matchAll(/\[([^\]]+)\]\((https:[^)\s]+)\)/g)) {
    sources.push({ name: match[1].trim(), url: match[2] });
  }
  return sources;
}

/** Keyword-map key: case, accent and repeated-space insensitive. */
export function normalizeQuery(value) {
  return String(value)
    .normalize('NFD')
    .replace(/[\u0300-\u036f]/g, '')
    .toLowerCase()
    .replace(/\s+/g, ' ')
    .trim();
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
      if (g.primaryQuery !== undefined) checkText(errors, where, 'guide.primaryQuery', g.primaryQuery, [3, 80]);
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
  if (entry) return entry.indexable === false && !entry.stale ? null : entry.title || entry.label || '';
  return portalRouteLabel(route);
}

/** Loads and validates every content source. Throws ContentError listing all problems. */
export function loadSite(root, options = {}) {
  const contentDir = path.join(root, 'content');
  const errors = [];
  const localeRegistry = loadLocales(root);
  const override = Array.isArray(options.enableLocales) ? new Set(options.enableLocales) : null;
  const locales = localeRegistry.locales.map((locale) => ({
    ...locale,
    enabled: locale.code === localeRegistry.default ? true : (override ? override.has(locale.code) : locale.enabled),
    messages: loadMessages(root, locale.code),
  }));
  const localeByCode = new Map(locales.map((locale) => [locale.code, locale]));
  const defaultLocale = localeByCode.get(localeRegistry.default);
  if (!defaultLocale) throw new ContentError(`locales.json: default locale "${localeRegistry.default}" is not listed`);
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
      primaryQuery: page.guide?.primaryQuery,
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
      primaryQuery: g.data.primaryQuery,
      related: g.data.related,
      datePublished: g.data.datePublished,
      dateModified: g.data.dateModified,
      lastmod: g.data.dateModified,
      body: g.body,
      sources: extractSources(g.body),
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
      primaryQuery: t.data.primaryQuery,
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
      primaryQuery: p.data.primaryQuery,
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

  // SEO-26: reviewed translations. Drafts and disabled locales are validated
  // but never added to the registry, so they cannot reach dist/.
  const translationsDir = path.resolve(root, options.translationsDir || path.join('content', 'translations'));
  const translationFields = ['translationOf', 'sourceRevision', 'translationStatus', 'reviewedBy', 'reviewedAt'];
  const sourceKinds = { guide: ['guide', 'html-guide'], tool: ['tool'], page: ['page'] };
  const pairs = new Set();
  for (const locale of locales) {
    for (const [dirName, kind] of [['guias', 'guide'], ['ferramentas', 'tool'], ['paginas', 'page']]) {
      for (const { file, expectedSlug } of readMarkdownDir(path.join(translationsDir, locale.code, dirName))) {
        const where = path.relative(root, file);
        let parsed;
        try {
          parsed = parseFrontMatter(fs.readFileSync(file, 'utf8'), where);
        } catch (err) {
          errors.push(err.message);
          continue;
        }
        const data = parsed.data;
        const base = Object.fromEntries(Object.entries(data).filter(([key]) => !translationFields.includes(key)));
        const source = typeof data.translationOf === 'string' ? registry.get(data.translationOf) : null;
        const problems = validateFrontMatter(kind, base, where, {
          categories: categorySlugs,
          expectedSlug,
          toolSlug: source?.slug,
        });
        for (const field of ['translationOf', 'sourceRevision', 'translationStatus']) {
          if (!Object.hasOwn(data, field)) problems.push(`${where}: missing required field "${field}"`);
        }
        if (data.translationStatus !== undefined && !['draft', 'reviewed'].includes(data.translationStatus)) {
          problems.push(`${where}: translationStatus must be draft or reviewed`);
        }
        if (data.sourceRevision !== undefined && !isValidDate(data.sourceRevision)) {
          problems.push(`${where}: sourceRevision must be a real YYYY-MM-DD date`);
        }
        if (data.translationStatus === 'reviewed') {
          if (typeof data.reviewedBy !== 'string' || data.reviewedBy.trim() === '') problems.push(`${where}: reviewed translations need reviewedBy`);
          if (!isValidDate(data.reviewedAt)) problems.push(`${where}: reviewed translations need a real reviewedAt date`);
        } else if (Object.hasOwn(data, 'reviewedAt') && !isValidDate(data.reviewedAt)) {
          problems.push(`${where}: reviewedAt must be a real YYYY-MM-DD date`);
        }
        if (data.translationOf !== undefined) {
          if (typeof data.translationOf !== 'string' || !ROUTE_RE.test(data.translationOf)) {
            problems.push(`${where}: translationOf must be a pt-BR route`);
          } else if (!source || source.locale) {
            problems.push(`${where}: translationOf ${data.translationOf} is not an existing pt-BR page`);
          } else if (!sourceKinds[kind].includes(source.kind)) {
            problems.push(`${where}: translationOf ${data.translationOf} is not a ${kind === 'guide' ? 'guide' : kind}`);
          }
        }
        if (problems.length > 0) {
          errors.push(...problems);
          continue;
        }
        if (!locale.enabled || data.translationStatus !== 'reviewed') continue;
        const pair = `${locale.code}|${data.translationOf}`;
        if (pairs.has(pair)) {
          errors.push(`${where}: ${locale.code} already translates ${data.translationOf}`);
          continue;
        }
        pairs.add(pair);
        const route = localeRoute(locale, kind, data.slug);
        const sourceDate = source.dateModified || source.lastmod;
        add({
          route,
          kind,
          locale: locale.code,
          translationOf: data.translationOf,
          sourceDate,
          sourceRevision: data.sourceRevision,
          stale: sourceDate > data.sourceRevision,
          title: data.title,
          headline: data.headline || data.title,
          description: data.description,
          summary: data.summary || data.description,
          primaryQuery: data.primaryQuery,
          schemaType: kind === 'page' ? data.schemaType || source.schemaType || 'WebPage' : undefined,
          category: data.category,
          tags: data.tags || [],
          related: data.related,
          datePublished: data.datePublished,
          dateModified: data.dateModified,
          lastmod: data.dateModified,
          body: parsed.body,
          sources: extractSources(parsed.body),
          formSlug: kind === 'tool' ? source.slug : undefined,
          where,
          indexable: !(sourceDate > data.sourceRevision),
          chrome: 'locale',
        });
      }
    }
  }

  // Locale hubs exist only for enabled locales with an emitted entry.
  for (const locale of locales.filter((entry) => entry.enabled && entry.code !== defaultLocale.code)) {
    for (const [kind, dirName] of [['guide', 'guias'], ['tool', 'ferramentas']]) {
      const entries = [...registry.values()].filter((entry) => entry.locale === locale.code && entry.kind === kind);
      if (entries.length === 0) continue;
      const chrome = locale.messages.chrome;
      add({
        route: `${locale.prefix}/${locale.segments[dirName]}/`,
        kind: kind === 'guide' ? 'guides-hub' : 'tools-hub',
        locale: locale.code,
        translationOf: kind === 'guide' ? '/guias/' : '/ferramentas/',
        hub: true,
        title: kind === 'guide' ? chrome.guidesHubTitle : chrome.toolsHubTitle,
        description: kind === 'guide' ? chrome.guidesHubIntro : chrome.toolsHubIntro,
        summary: kind === 'guide' ? chrome.guidesHubIntro : chrome.toolsHubIntro,
        lastmod: maxDate(entries),
        indexable: true,
        chrome: 'locale',
      });
    }
  }

  // One translation per locale and source, so every hreflang group has at
  // most one member per language.
  const hreflangGroups = new Map();
  for (const entry of registry.values()) {
    if (entry.locale || !entry.indexable) continue;
    hreflangGroups.set(entry.route, [{
      code: defaultLocale.code,
      route: entry.route,
      htmlLang: defaultLocale.htmlLang,
      hreflang: defaultLocale.hreflang,
      ogLocale: defaultLocale.ogLocale,
      languageName: defaultLocale.messages.languageName,
    }]);
  }
  for (const entry of registry.values()) {
    if (!entry.locale || !entry.indexable || entry.stale) continue;
    const members = hreflangGroups.get(entry.translationOf);
    if (!members) continue;
    const locale = localeByCode.get(entry.locale);
    members.push({
      code: locale.code,
      route: entry.route,
      htmlLang: locale.htmlLang,
      hreflang: locale.hreflang,
      ogLocale: locale.ogLocale,
      languageName: locale.messages.languageName,
    });
  }
  const hreflangMembers = (route) => {
    const entry = registry.get(route);
    if (!entry || !entry.indexable || entry.stale) return [];
    const key = entry.locale ? entry.translationOf : route;
    const ptBR = registry.get(key);
    if (!ptBR || !ptBR.indexable) return [];
    const members = hreflangGroups.get(key) || [];
    return members.length >= 2 ? members : [];
  };

  // SEO-20 keyword map: one URL per query, compared without case or accents,
  // scoped by locale so translations in different languages never collide.
  const queryOwners = new Map();
  for (const entry of registry.values()) {
    if (!entry.primaryQuery) continue;
    const key = `${entry.locale || defaultLocale.code}|${normalizeQuery(entry.primaryQuery)}`;
    const owner = queryOwners.get(key);
    if (owner) errors.push(`primaryQuery "${entry.primaryQuery}" collides between ${owner} and ${entry.route}`);
    else queryOwners.set(key, entry.route);
  }

  for (const route of manifest.popularGuides || []) {
    const entry = registry.get(route);
    if (!entry || (entry.kind !== 'guide' && entry.kind !== 'html-guide')) errors.push(`manifest popularGuides: ${route} is not a guide`);
  }
  for (const slug of Object.keys(TOOL_FORMS)) {
    if (!registry.has(`/ferramentas/${slug}/`)) errors.push(`calculator "${slug}" has no content/ferramentas/${slug}.md`);
  }

  // Render Markdown and check every internal link and related route.
  const imageInfo = loadAppCaptures(root);
  const checkRoute = (where, route, what) => {
    const [pathname] = route.split(/[?#]/);
    if (resolveRouteLabel(registry, pathname) === null) {
      errors.push(`${where}: ${what} ${route} is not a landing page or a known portal route`);
    }
  };
  for (const entry of registry.values()) {
    if (entry.body === undefined) continue;
    try {
      const rendered = renderMarkdown(entry.body, { reservedIds: RESERVED_IDS, imageInfo });
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
      const target = registry.get(route.split(/[?#]/)[0]);
      if (entry.locale && target?.locale && target.locale !== entry.locale) {
        errors.push(`${entry.where}: related route ${route} belongs to another locale`);
      }
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
    locales,
    localeByCode,
    defaultLocale,
    translations: [...registry.values()].filter((entry) => entry.locale && !entry.hub),
    hreflangMembers,
  };
  return site;
}

/** "Leia também": shared tags weigh most, then category, then recency. */
export function relatedGuides(site, entry, limit = 3, pool = site.guides) {
  const tags = new Set(entry.tags || []);
  return pool
    .filter((g) => g.route !== entry.route)
    .map((g) => ({
      g,
      score: (g.tags || []).filter((t) => tags.has(t)).length * 3 + (g.category === entry.category ? 2 : 0),
    }))
    .sort((a, b) => b.score - a.score || (b.g.lastmod || '').localeCompare(a.g.lastmod || '') || a.g.route.localeCompare(b.g.route))
    .slice(0, limit)
    .map(({ g }) => g);
}

const FEED_AUTHOR = 'Equipe abastevo';

function escapeXml(value) {
  return String(value)
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#39;');
}

/**
 * SEO-22: Atom feed with the 20 most recently modified guides (Markdown and
 * authored HTML). Entry and feed dates come from content, so builds stay
 * reproducible; nothing here reads the clock.
 */
export function renderGuidesFeed(site, origin) {
  const feedDate = (entry) => entry.dateModified || entry.lastmod;
  const guides = [...site.guides]
    .filter((entry) => entry.indexable !== false && feedDate(entry))
    .sort((a, b) => feedDate(b).localeCompare(feedDate(a)) || a.route.localeCompare(b.route))
    .slice(0, 20);
  if (guides.length === 0) throw new ContentError('guides feed needs at least one dated guide');
  const updated = `${feedDate(guides[0])}T00:00:00Z`;
  const entries = guides.map((entry) => {
    const url = `${origin}${entry.route}`;
    return `  <entry>
    <id>${escapeXml(url)}</id>
    <link rel="alternate" type="text/html" href="${escapeXml(url)}"/>
    <title>${escapeXml(entry.title)}</title>
    <summary>${escapeXml(entry.description || entry.summary || '')}</summary>
    <updated>${feedDate(entry)}T00:00:00Z</updated>
  </entry>`;
  }).join('\n');
  return `<?xml version="1.0" encoding="utf-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <title>Guias do abastevo</title>
  <id>${escapeXml(`${origin}/guias/feed.xml`)}</id>
  <link rel="self" type="application/atom+xml" href="${escapeXml(`${origin}/guias/feed.xml`)}"/>
  <link rel="alternate" type="text/html" href="${escapeXml(`${origin}/guias/`)}"/>
  <updated>${updated}</updated>
  <author><name>${FEED_AUTHOR}</name></author>
${entries}
</feed>
`;
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
  for (const locale of site.locales.filter((entry) => entry.enabled && entry.code !== site.defaultLocale.code)) {
    const entries = [...site.registry.values()]
      .filter((entry) => entry.locale === locale.code)
      .sort((a, b) => a.route.localeCompare(b.route));
    if (entries.length === 0) continue;
    lines.push('', `## ${locale.messages.languageName} (${locale.code})`);
    for (const entry of entries) lines.push(`- ${entry.title}: ${origin}${entry.route}`);
  }
  return lines.join('\n');
}

export { isPortalRoute };
