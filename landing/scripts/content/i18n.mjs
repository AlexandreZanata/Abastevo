// SEO-25: locale registry and URL contract shared with the Go portal (SEO-28).
// Both generators read contracts/i18n/locales.json and the shared test
// vectors, so the same locale always produces the same route.

import fs from 'node:fs';
import path from 'node:path';

const SLUG_RE = /^[a-z0-9]+(?:-[a-z0-9]+)*$/;
const PREFIX_RE = /^\/[a-z]{2}$/;
const LOCALE_FIELDS = ['code', 'prefix', 'htmlLang', 'hreflang', 'ogLocale', 'decimal', 'enabled', 'segments'];
const SEGMENT_KEYS = ['guias', 'ferramentas'];
const KINDS = ['guide', 'tool', 'page'];

export class LocaleConfigError extends Error {}

/**
 * Reads and validates the locale registry next to the landing directory.
 * Returns `{ version, default, locales }`, where every locale is a plain
 * object with the fields of locales.json.
 */
export function loadLocales(landingRoot) {
  const file = path.join(landingRoot, '..', 'contracts', 'i18n', 'locales.json');
  let data;
  try {
    data = JSON.parse(fs.readFileSync(file, 'utf8'));
  } catch (err) {
    throw new LocaleConfigError(`locales.json: cannot read ${file} (${err.message})`);
  }
  const errors = [];
  if (!Object.hasOwn(data, 'version')) errors.push('missing field "version"');
  else if (data.version !== 1) errors.push(`unsupported version ${JSON.stringify(data.version)}`);
  if (!Object.hasOwn(data, 'default')) errors.push('missing field "default"');
  if (!Array.isArray(data.locales) || data.locales.length === 0) {
    errors.push('"locales" must be a non-empty array');
  }

  const codes = new Map();
  const prefixes = new Map();
  for (const [index, locale] of (Array.isArray(data.locales) ? data.locales : []).entries()) {
    const where = `locale at index ${index}`;
    if (!locale || typeof locale !== 'object' || Array.isArray(locale)) {
      errors.push(`${where} must be an object`);
      continue;
    }
    for (const field of LOCALE_FIELDS) {
      if (!Object.hasOwn(locale, field)) errors.push(`${where} is missing field "${field}"`);
    }
    if (typeof locale.code === 'string') {
      if (codes.has(locale.code)) errors.push(`duplicate locale code "${locale.code}" (${codes.get(locale.code)} and index ${index})`);
      else codes.set(locale.code, `index ${index}`);
    }
    if (typeof locale.prefix === 'string' && (locale.prefix === '' || PREFIX_RE.test(locale.prefix))) {
      if (prefixes.has(locale.prefix)) errors.push(`duplicate prefix "${locale.prefix}" (${prefixes.get(locale.prefix)} and index ${index})`);
      else prefixes.set(locale.prefix, `index ${index}`);
    } else {
      errors.push(`${where} (${locale.code || '?'}) prefix ${JSON.stringify(locale.prefix)} is not "" or /xx`);
    }
    if (!locale.segments || typeof locale.segments !== 'object' || Array.isArray(locale.segments)) {
      if (Object.hasOwn(locale, 'segments')) errors.push(`${where} (${locale.code || '?'}) segments must be an object`);
      continue;
    }
    for (const key of SEGMENT_KEYS) {
      if (!Object.hasOwn(locale.segments, key)) {
        errors.push(`${where} (${locale.code || '?'}) segments is missing "${key}"`);
      } else if (typeof locale.segments[key] !== 'string' || !SLUG_RE.test(locale.segments[key])) {
        errors.push(`${where} (${locale.code || '?'}) segments.${key} ${JSON.stringify(locale.segments[key])} is not a slug`);
      }
    }
  }

  if (errors.length > 0) throw new LocaleConfigError(`locales.json is invalid:\n  - ${errors.join('\n  - ')}`);
  return { version: data.version, default: data.default, locales: data.locales };
}

/** URL for one page kind of a locale entry from `loadLocales`. */
export function localeRoute(locale, kind, slug) {
  if (!locale || typeof locale !== 'object' || !locale.segments) {
    throw new LocaleConfigError('localeRoute needs a locale entry from loadLocales');
  }
  if (!KINDS.includes(kind)) throw new LocaleConfigError(`localeRoute: unknown kind "${kind}"`);
  if (typeof slug !== 'string' || !SLUG_RE.test(slug)) throw new LocaleConfigError(`localeRoute: invalid slug ${JSON.stringify(slug)}`);
  const base = locale.prefix;
  if (kind === 'guide') return `${base}/${locale.segments.guias}/${slug}/`;
  if (kind === 'tool') return `${base}/${locale.segments.ferramentas}/${slug}/`;
  return `${base}/${slug}/`;
}
