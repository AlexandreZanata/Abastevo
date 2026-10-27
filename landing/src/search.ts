/**
 * Abastevo portal static search enhancement.
 *
 * Essential navigation stays in HTML. This module loads small immutable,
 * release-addressed files (an index of cities and fuels, a station prefix
 * shard, one city, one fuel or the official-records list), filters public
 * fields and renders text nodes only. It never calls the Abastevo API, the
 * ANP API or a database, and never downloads a national all-stations file.
 */

export const SEARCH_SCHEMA = 'abastevo-search-v1';
export const INDEX_SCHEMA = 'abastevo-search-index-v1';
export const PREFIX_SCHEMA = 'abastevo-search-prefix-v1';
export const PRICES_SCHEMA = 'abastevo-search-prices-v1';
export const FISCAL_SCHEMA = 'abastevo-search-fiscal-v1';
export const MAX_QUERY = 80;
export const MAX_RESULTS = 50;
export const MAX_TABLE_ROWS = 100;
const MAX_SUGGESTIONS = 8;

export const UF_NAMES: Record<string, string> = {
  AC: 'Acre', AL: 'Alagoas', AP: 'Amapá', AM: 'Amazonas', BA: 'Bahia', CE: 'Ceará',
  DF: 'Distrito Federal', ES: 'Espírito Santo', GO: 'Goiás', MA: 'Maranhão',
  MT: 'Mato Grosso', MS: 'Mato Grosso do Sul', MG: 'Minas Gerais', PA: 'Pará',
  PB: 'Paraíba', PR: 'Paraná', PE: 'Pernambuco', PI: 'Piauí', RJ: 'Rio de Janeiro',
  RN: 'Rio Grande do Norte', RS: 'Rio Grande do Sul', RO: 'Rondônia', RR: 'Roraima',
  SC: 'Santa Catarina', SP: 'São Paulo', SE: 'Sergipe', TO: 'Tocantins',
};
const VALID_UFS = new Set(Object.keys(UF_NAMES));

export interface SearchStation {
  i: string;
  n: string;
  s: string;
  c?: string;
  m?: string;
  mn?: string;
  u?: string;
  st?: string;
  sl?: string;
  p?: Record<string, number>;
  f?: number;
}

export interface SearchShard {
  schema: string;
  release: string;
  uf: string;
  municipio?: string;
  count: number;
  stations: SearchStation[];
}

export interface SearchMunicipality { m: string; n: string; s: string; u: string; k: number }
export interface SearchFuel { code: string; label: string; unit: string; count: number }

export interface SearchIndex {
  schema: string;
  release: string;
  prefix_length: number;
  stopwords: string[];
  fiscal: boolean;
  fuels: SearchFuel[];
  municipalities: SearchMunicipality[];
}

export interface PrefixShard { schema: string; release: string; key: string; count: number; stations: SearchStation[] }

export interface PriceRow { i: string; n: string; c?: string; m?: string; mn?: string; u?: string; v: number; d?: string }
export interface PriceShard { schema: string; release: string; fuel: string; label: string; unit: string; count: number; rows: PriceRow[] }

export interface FiscalRow { r: string; c: string; st: string; a?: string; d?: string; i?: string; n?: string; m?: string; mn?: string; u?: string }
export interface FiscalShard { schema: string; release: string; count: number; records: FiscalRow[] }

export type SearchScope =
  | { kind: 'uf'; uf: string; label: string }
  | { kind: 'city'; code: string; uf: string; label: string };

export interface SearchRuntime {
  document?: Document;
  fetch?: typeof fetch;
  /** Query string used to prefill, e.g. `?uf=SP&q=alfa` or `?m=3550308`. */
  search?: string;
  /** Navigation hook (tests); defaults to location.assign. */
  navigate?: (href: string) => void;
}

/** Reads a landing hand-off (`uf`, `q`); an unknown UF prefills nothing. */
export function readSearchHandoff(search: string): { uf: string; query: string } | null {
  const params = new URLSearchParams(search);
  const uf = (params.get('uf') ?? '').trim().toUpperCase();
  if (!VALID_UFS.has(uf)) return null;
  return { uf, query: (params.get('q') ?? '').slice(0, MAX_QUERY) };
}

export function normalizeSearchText(value: string): string {
  return value
    .normalize('NFD')
    .replace(/[̀-ͯ]/g, '')
    .replace(/[\u0000-\u001f\u007f]/g, '')
    .toLowerCase()
    .replace(/\s+/g, ' ')
    .trim()
    .slice(0, MAX_QUERY);
}

export function searchTokens(value: string): string[] {
  return normalizeSearchText(value).split(/[^a-z0-9]+/).filter(Boolean);
}

export function normalizeCNPJQuery(value: string): string {
  return value
    .toUpperCase()
    .replace(/[.\-/\s]/g, '')
    .replace(/[^0-9A-Z]/g, '')
    .slice(0, 14);
}

export function isCNPJQuery(value: string): boolean {
  const raw = value.trim();
  const normalized = normalizeCNPJQuery(raw);
  if (normalized.length < 3) return false;
  if (/[./-]/.test(raw) && /^\d/.test(normalized)) return true;
  if (/^\d{3,14}$/.test(normalized)) return true;
  // Alphanumeric CNPJ: 12 base characters plus 2 numeric check digits, typed
  // as one token (a 14-letter name like "Auto Posto Shell" is not one).
  return !/\s/.test(raw) && /^[0-9A-Z]{12}\d{2}$/.test(normalized) && /[A-Z]/.test(normalized) && /\d/.test(normalized.slice(0, 12));
}

export function formatCNPJ(value?: string): string {
  const clean = (value ?? '').toUpperCase().replace(/[^0-9A-Z]/g, '');
  if (clean.length !== 14) return value || 'CNPJ não informado';
  return `${clean.slice(0, 2)}.${clean.slice(2, 5)}.${clean.slice(5, 8)}/${clean.slice(8, 12)}-${clean.slice(12)}`;
}

const UNIT_LABELS: Record<string, string> = { L: 'litro', M3: 'm³', KG_13: 'botijão 13 kg' };

/** Formats exact milli-BRL like the portal pages: R$ 6,349 (three decimals). */
export function formatMilliBRL(milli: number): string {
  const whole = Math.floor(milli / 1000);
  const frac = String(milli % 1000).padStart(3, '0');
  return `R$ ${String(whole).replace(/\B(?=(\d{3})+(?!\d))/g, '.')},${frac}`;
}

export function unitLabel(unit: string): string {
  return UNIT_LABELS[unit] ?? unit;
}

export function formatDateBR(value?: string): string {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value ?? '');
  return match ? `${match[3]}/${match[2]}/${match[1]}` : '';
}

/** True when every query token occurs in the normalized name, or the CNPJ starts with the query digits. */
export function matchesStation(name: string, searchName: string | undefined, cnpj: string | undefined, query: string): boolean {
  if (isCNPJQuery(query)) return (cnpj ?? '').startsWith(normalizeCNPJQuery(query));
  const tokens = searchTokens(query);
  if (tokens.length === 0) return true;
  const haystack = searchName || normalizeSearchText(name);
  return tokens.every((token) => haystack.includes(token));
}

export function inScope(row: { m?: string; u?: string }, scope: SearchScope | null): boolean {
  if (!scope) return true;
  if (scope.kind === 'uf') return row.u === scope.uf;
  return row.m === scope.code;
}

/** Picks the prefix shard for a national name/CNPJ query, or null if the query is too generic. */
export function prefixKeyFor(query: string, index: Pick<SearchIndex, 'prefix_length' | 'stopwords'>): string | null {
  const length = index.prefix_length;
  if (isCNPJQuery(query)) {
    const digits = normalizeCNPJQuery(query);
    return digits.length >= length ? `c-${digits.slice(0, length).toLowerCase()}` : null;
  }
  const stop = new Set(index.stopwords);
  let best = '';
  for (const token of searchTokens(query)) {
    if (token.length < length || stop.has(token) || /^\d+$/.test(token)) continue;
    if (token.length > best.length) best = token;
  }
  return best ? `n-${best.slice(0, length)}` : null;
}

/** Matches stations of a loaded shard; keeps the old regional API for city filters. */
export function findMatches(
  shard: { stations: SearchStation[] },
  query: string,
  city = '',
  limit = MAX_RESULTS,
): SearchStation[] {
  const nameQuery = normalizeSearchText(query);
  const cnpjQuery = normalizeCNPJQuery(query);
  if (nameQuery.length < 2 && cnpjQuery.length < 3) return [];
  return filterStations(shard.stations, query, city ? { kind: 'city', code: city, uf: '', label: '' } : null, limit);
}

export function filterStations(stations: SearchStation[], query: string, scope: SearchScope | null, limit = MAX_RESULTS): SearchStation[] {
  const matches = stations.filter((station) => inScope(station, scope) && matchesStation(station.n, station.s, station.c, query));
  matches.sort((a, b) => a.n.localeCompare(b.n, 'pt-BR') || a.i.localeCompare(b.i));
  return matches.slice(0, Math.min(Math.max(limit, 1), MAX_RESULTS * 4));
}

export interface PlaceSuggestion { scope: SearchScope; label: string; detail: string }

/** States and cities whose names start with (first) or contain the query. */
export function placeSuggestions(query: string, municipalities: SearchMunicipality[], within: SearchScope | null, limit = MAX_SUGGESTIONS): PlaceSuggestion[] {
  const q = normalizeSearchText(query);
  if (q.length < 2) return [];
  const out: PlaceSuggestion[] = [];
  if (!within) {
    for (const [uf, name] of Object.entries(UF_NAMES)) {
      const normalized = normalizeSearchText(name);
      if (normalized.startsWith(q) || q === uf.toLowerCase()) {
        out.push({ scope: { kind: 'uf', uf, label: `${name} (${uf})` }, label: `${name} (${uf})`, detail: 'Estado' });
      }
    }
  }
  const cities = municipalities
    .filter((city) => (!within || city.u === within.uf) && city.s.includes(q))
    .sort((a, b) => Number(b.s.startsWith(q)) - Number(a.s.startsWith(q)) || b.k - a.k || a.n.localeCompare(b.n, 'pt-BR'));
  for (const city of cities) {
    if (out.length >= limit) break;
    const label = `${city.n}/${city.u}`;
    out.push({
      scope: { kind: 'city', code: city.m, uf: city.u, label },
      label,
      detail: `${city.k} ${city.k === 1 ? 'posto' : 'postos'}`,
    });
  }
  return out.slice(0, limit);
}

export function filterPrices(rows: PriceRow[], query: string, scope: SearchScope | null, order: 'asc' | 'desc'): PriceRow[] {
  const matches = rows.filter((row) => inScope(row, scope) && matchesStation(row.n, undefined, row.c, query));
  matches.sort((a, b) => (order === 'asc' ? a.v - b.v : b.v - a.v) || a.n.localeCompare(b.n, 'pt-BR') || a.i.localeCompare(b.i));
  return matches;
}

export function filterFiscal(records: FiscalRow[], query: string, scope: SearchScope | null): FiscalRow[] {
  return records.filter((row) => inScope(row, scope) && matchesStation(row.n ?? '', undefined, row.c, query));
}

function checkRelease(value: unknown, schema: string, release: string): Record<string, unknown> {
  if (!value || typeof value !== 'object') throw new Error('shard-invalid');
  const record = value as Record<string, unknown>;
  if (record.schema !== schema || record.release !== release) throw new Error('shard-version');
  return record;
}

export function validateShard(value: unknown, release: string): SearchShard {
  const shard = checkRelease(value, SEARCH_SCHEMA, release);
  if (typeof shard.uf !== 'string' || !Array.isArray(shard.stations)) throw new Error('shard-version');
  return shard as unknown as SearchShard;
}

async function fetchJSON(url: string, fetcher: typeof fetch, signal?: AbortSignal): Promise<unknown> {
  const response = await fetcher(url, { credentials: 'omit', cache: 'force-cache', signal });
  if (!response.ok) throw new Error('shard-missing');
  return response.json();
}

function joinBase(base: string): string {
  return base.endsWith('/') ? base : `${base}/`;
}

export async function loadShard(
  uf: string,
  release: string,
  base: string,
  fetcher: typeof fetch = fetch,
  signal?: AbortSignal,
): Promise<SearchShard> {
  if (uf !== 'sem-uf' && !VALID_UFS.has(uf)) throw new Error('shard-uf');
  if (!/^[A-Za-z0-9_-]{1,64}$/.test(release)) throw new Error('shard-release');
  return validateShard(await fetchJSON(`${joinBase(base)}${release}/${uf}.json`, fetcher, signal), release);
}

/** Loads and caches the release's search files; every path is validated before fetching. */
export class SearchStore {
  private readonly cache = new Map<string, Promise<unknown>>();

  constructor(private readonly base: string, private readonly release: string, private readonly fetcher: typeof fetch) {
    if (!/^[A-Za-z0-9_-]{1,64}$/.test(release)) throw new Error('shard-release');
  }

  private load<T>(path: string, check: (value: unknown) => T): Promise<T> {
    const key = `${joinBase(this.base)}${this.release}/${path}`;
    let pending = this.cache.get(key) as Promise<T> | undefined;
    if (!pending) {
      pending = fetchJSON(key, this.fetcher).then(check);
      pending.catch(() => this.cache.delete(key));
      this.cache.set(key, pending);
    }
    return pending;
  }

  index(): Promise<SearchIndex> {
    return this.load('indice.json', (value) => {
      const index = checkRelease(value, INDEX_SCHEMA, this.release);
      if (!Array.isArray(index.municipalities) || !Array.isArray(index.stopwords) || !Array.isArray(index.fuels)) throw new Error('shard-version');
      return index as unknown as SearchIndex;
    });
  }

  prefix(key: string): Promise<PrefixShard> {
    if (!/^(n-[a-z0-9]{3}|c-[0-9a-z]{3})$/.test(key)) return Promise.reject(new Error('shard-key'));
    return this.load(`p/${key}.json`, (value) => {
      const shard = checkRelease(value, PREFIX_SCHEMA, this.release);
      if (shard.key !== key || !Array.isArray(shard.stations)) throw new Error('shard-version');
      return shard as unknown as PrefixShard;
    }).catch((error: Error) => {
      if (error.message === 'shard-missing') return { schema: PREFIX_SCHEMA, release: this.release, key, count: 0, stations: [] };
      throw error;
    });
  }

  city(code: string): Promise<SearchShard> {
    if (!/^\d{7}$/.test(code)) return Promise.reject(new Error('shard-city'));
    return this.load(`m/${code}.json`, (value) => {
      const shard = validateShard(value, this.release);
      if (shard.municipio !== code) throw new Error('shard-version');
      return shard;
    });
  }

  uf(uf: string): Promise<SearchShard> {
    if (!VALID_UFS.has(uf)) return Promise.reject(new Error('shard-uf'));
    return this.load(`${uf}.json`, (value) => validateShard(value, this.release));
  }

  prices(fuel: string): Promise<PriceShard> {
    if (!/^[A-Z0-9_]{2,32}$/.test(fuel)) return Promise.reject(new Error('shard-fuel'));
    return this.load(`precos/${fuel}.json`, (value) => {
      const shard = checkRelease(value, PRICES_SCHEMA, this.release);
      if (shard.fuel !== fuel || !Array.isArray(shard.rows)) throw new Error('shard-version');
      return shard as unknown as PriceShard;
    });
  }

  fiscal(): Promise<FiscalShard> {
    return this.load('fiscalizacao.json', (value) => {
      const shard = checkRelease(value, FISCAL_SCHEMA, this.release);
      if (!Array.isArray(shard.records)) throw new Error('shard-version');
      return shard as unknown as FiscalShard;
    });
  }
}

function plural(value: number, singular: string, pluralForm: string): string {
  return value === 1 ? singular : pluralForm;
}

function place(row: { mn?: string; u?: string }): string {
  return [row.mn, row.u].filter((value): value is string => Boolean(value)).join('/');
}

interface FinderOptions {
  stations: boolean;
  onChange: () => void;
  onPickStation?: (station: SearchStation) => void;
}

interface Finder {
  scope(): SearchScope | null;
  query(): string;
  setScope(scope: SearchScope | null): void;
  setQuery(value: string): void;
}

/** Combobox over one text input: suggests states, cities and (optionally) stations. */
function setupFinder(root: Element, doc: Document, store: SearchStore, options: FinderOptions): Finder | null {
  const input = root.querySelector<HTMLInputElement>('[data-finder-input]');
  const list = root.querySelector<HTMLElement>('[data-finder-suggestions]');
  const chip = root.querySelector<HTMLElement>('[data-finder-scope]');
  if (!input || !list || !chip) return null;
  let scope: SearchScope | null = null;
  let items: Array<{ label: string; detail: string; pick: () => void }> = [];
  let active = -1;
  let ticket = 0;
  let debounce: ReturnType<typeof setTimeout> | undefined;

  const close = (): void => {
    list.hidden = true;
    list.replaceChildren();
    input.setAttribute('aria-expanded', 'false');
    active = -1;
  };
  const highlight = (): void => {
    list.children && Array.from(list.children).forEach((child, index) => {
      (child as HTMLElement).className = index === active ? 'suggestion active' : 'suggestion';
      (child as HTMLElement).setAttribute('aria-selected', index === active ? 'true' : 'false');
    });
  };
  const renderChip = (): void => {
    chip.replaceChildren();
    chip.hidden = scope === null;
    if (!scope) return;
    const text = doc.createElement('span');
    text.textContent = `${scope.kind === 'uf' ? 'Estado' : 'Cidade'}: ${scope.label}`;
    const clear = doc.createElement('button');
    clear.type = 'button';
    clear.className = 'chip-clear';
    clear.textContent = 'Remover filtro';
    clear.addEventListener('click', () => {
      api.setScope(null);
      options.onChange();
    });
    chip.append(text, clear);
  };
  const show = (): void => {
    list.replaceChildren();
    items.forEach((item, index) => {
      const option = doc.createElement('li');
      option.className = 'suggestion';
      option.setAttribute('role', 'option');
      option.setAttribute('aria-selected', 'false');
      const label = doc.createElement('span');
      label.textContent = item.label;
      const detail = doc.createElement('span');
      detail.className = 'muted';
      detail.textContent = ` — ${item.detail}`;
      option.append(label, detail);
      option.addEventListener('mousedown', (event: Event) => {
        event.preventDefault();
        active = index;
        item.pick();
      });
      list.append(option);
    });
    list.hidden = items.length === 0;
    input.setAttribute('aria-expanded', items.length ? 'true' : 'false');
  };
  const suggest = async (): Promise<void> => {
    const id = ++ticket;
    const value = input.value;
    let index: SearchIndex;
    try {
      index = await store.index();
    } catch {
      return;
    }
    const next: typeof items = placeSuggestions(value, index.municipalities, scope && scope.kind === 'city' ? null : scope)
      .filter(() => !(scope && scope.kind === 'city'))
      .map((suggestion) => ({
        label: suggestion.label,
        detail: suggestion.detail,
        pick: () => {
          api.setScope(suggestion.scope);
          api.setQuery('');
          close();
          options.onChange();
        },
      }));
    if (options.stations && options.onPickStation) {
      const key = prefixKeyFor(value, index);
      if (key) {
        try {
          const shard = await store.prefix(key);
          if (id !== ticket) return;
          for (const station of filterStations(shard.stations, value, scope, MAX_SUGGESTIONS)) {
            if (next.length >= MAX_SUGGESTIONS * 2) break;
            next.push({
              label: station.n,
              detail: [formatCNPJ(station.c), place(station)].filter(Boolean).join(' · '),
              pick: () => options.onPickStation?.(station),
            });
          }
        } catch {
          // Suggestions are best-effort; results still render below.
        }
      }
    }
    if (id !== ticket) return;
    items = next;
    active = -1;
    show();
  };

  input.addEventListener('input', () => {
    if (debounce) clearTimeout(debounce);
    debounce = setTimeout(() => {
      void suggest();
      options.onChange();
    }, 120);
  });
  input.addEventListener('keydown', (event: Event) => {
    const key = (event as KeyboardEvent).key;
    if (key === 'ArrowDown' && items.length) {
      event.preventDefault();
      active = (active + 1) % items.length;
      highlight();
    } else if (key === 'ArrowUp' && items.length) {
      event.preventDefault();
      active = (active - 1 + items.length) % items.length;
      highlight();
    } else if (key === 'Enter') {
      event.preventDefault();
      if (active >= 0 && items[active] && !list.hidden) items[active].pick();
      else {
        close();
        options.onChange();
      }
    } else if (key === 'Escape') {
      close();
    }
  });
  input.addEventListener('blur', () => setTimeout(close, 150));

  const api: Finder = {
    scope: () => scope,
    query: () => input.value,
    setScope: (value) => {
      scope = value;
      renderChip();
    },
    setQuery: (value) => {
      input.value = value.slice(0, MAX_QUERY);
    },
  };
  renderChip();
  return api;
}

function stationItem(doc: Document, station: SearchStation): HTMLElement {
  const item = doc.createElement('li');
  const link = doc.createElement('a');
  link.href = `/postos/${encodeURIComponent(station.i)}/`;
  link.textContent = station.n;
  item.append(link);
  const meta = doc.createElement('span');
  meta.className = 'muted';
  const fuelCount = station.p ? Object.keys(station.p).length : station.f ?? 0;
  const priceText = fuelCount
    ? `${fuelCount} ${plural(fuelCount, 'combustível com preço', 'combustíveis com preço')}`
    : 'sem preço nesta pesquisa';
  const where = place(station);
  meta.textContent = ` — ${formatCNPJ(station.c)}${where ? ` · ${where}` : ''} · ${priceText}`;
  item.append(meta);
  return item;
}

function setupConsultar(form: HTMLElement, doc: Document, store: SearchStore, runtime: SearchRuntime): void {
  const status = form.querySelector<HTMLElement>('[data-finder-status]');
  const results = form.querySelector<HTMLElement>('[data-finder-results]');
  if (!status || !results) return;
  const navigate = runtime.navigate ?? ((href: string) => location.assign(href));
  let ticket = 0;
  const finder = setupFinder(form, doc, store, {
    stations: true,
    onChange: () => void render(),
    onPickStation: (station) => navigate(`/postos/${encodeURIComponent(station.i)}/`),
  });
  if (!finder) return;

  const show = (stations: SearchStation[], total: number, where: string): void => {
    results.replaceChildren(...stations.map((station) => stationItem(doc, station)));
    if (stations.length === 0) {
      const empty = doc.createElement('li');
      empty.textContent = 'Nenhum posto encontrado.';
      results.append(empty);
      status.textContent = `Nenhum posto encontrado${where}.`;
      return;
    }
    status.textContent = total > stations.length
      ? `Mostrando ${stations.length} de ${total} postos${where}. Refine pelo nome ou CNPJ.`
      : `${total} ${plural(total, 'posto encontrado', 'postos encontrados')}${where}.`;
  };

  const render = async (): Promise<void> => {
    const id = ++ticket;
    const scope = finder.scope();
    const query = finder.query();
    try {
      if (scope && scope.kind === 'city') {
        status.textContent = `Carregando postos de ${scope.label}...`;
        const shard = await store.city(scope.code);
        if (id !== ticket) return;
        const all = filterStations(shard.stations, query, null, shard.stations.length || 1);
        show(all.slice(0, MAX_TABLE_ROWS), all.length, ` em ${scope.label}`);
        return;
      }
      if (scope && scope.kind === 'uf' && searchTokens(query).length === 0 && !isCNPJQuery(query)) {
        const index = await store.index();
        if (id !== ticket) return;
        const cities = index.municipalities.filter((city) => city.u === scope.uf).sort((a, b) => b.k - a.k).slice(0, 24);
        results.replaceChildren(...cities.map((city) => {
          const item = doc.createElement('li');
          const button = doc.createElement('button');
          button.type = 'button';
          button.className = 'link-button';
          button.textContent = `${city.n} — ${city.k} ${plural(city.k, 'posto', 'postos')}`;
          button.addEventListener('click', () => {
            finder.setScope({ kind: 'city', code: city.m, uf: city.u, label: `${city.n}/${city.u}` });
            void render();
          });
          item.append(button);
          return item;
        }));
        status.textContent = `Escolha uma cidade de ${scope.label} ou digite o nome do posto.`;
        return;
      }
      if (scope && scope.kind === 'uf') {
        status.textContent = `Carregando postos de ${scope.label}...`;
        const shard = await store.uf(scope.uf);
        if (id !== ticket) return;
        const all = filterStations(shard.stations, query, null, shard.stations.length || 1);
        show(all.slice(0, MAX_RESULTS), all.length, ` em ${scope.label}`);
        return;
      }
      if (normalizeSearchText(query).length === 0) {
        results.replaceChildren();
        status.textContent = 'Digite o nome do posto, parte do CNPJ, uma cidade ou um estado.';
        return;
      }
      const index = await store.index();
      const key = prefixKeyFor(query, index);
      if (!key) {
        results.replaceChildren();
        status.textContent = 'Digite ao menos 3 letras de um termo do nome (ex.: Shell, Ipiranga) ou 3 dígitos do CNPJ, ou escolha uma cidade.';
        return;
      }
      status.textContent = 'Buscando em todo o Brasil...';
      const shard = await store.prefix(key);
      if (id !== ticket) return;
      const all = filterStations(shard.stations, query, null, shard.stations.length || 1);
      show(all.slice(0, MAX_RESULTS), all.length, ' no Brasil');
    } catch (error) {
      if (id !== ticket) return;
      if ((error as { name?: string }).name === 'AbortError') return;
      results.replaceChildren();
      status.textContent = 'Índice de busca indisponível. Use a navegação por estado abaixo.';
    }
  };

  form.addEventListener('submit', (event: Event) => {
    event.preventDefault();
    void render();
  });

  const search = runtime.search ?? (typeof location === 'undefined' ? '' : location.search);
  const handoff = readSearchHandoff(search);
  const params = new URLSearchParams(search);
  const city = params.get('m') ?? '';
  if (handoff) {
    finder.setScope({ kind: 'uf', uf: handoff.uf, label: `${UF_NAMES[handoff.uf]} (${handoff.uf})` });
    finder.setQuery(handoff.query);
    void render();
  } else if (/^\d{7}$/.test(city)) {
    void store.index().then((index) => {
      const match = index.municipalities.find((item) => item.m === city);
      if (!match) return;
      finder.setScope({ kind: 'city', code: match.m, uf: match.u, label: `${match.n}/${match.u}` });
      finder.setQuery((params.get('q') ?? '').slice(0, MAX_QUERY));
      void render();
    }, () => undefined);
  } else if (params.get('q')) {
    finder.setQuery((params.get('q') ?? '').slice(0, MAX_QUERY));
    void render();
  }
}

function cell(doc: Document, text: string, className = ''): HTMLElement {
  const td = doc.createElement('td');
  td.textContent = text;
  if (className) td.className = className;
  return td;
}

function setupPrices(section: HTMLElement, doc: Document, store: SearchStore): void {
  const fuel = section.querySelector<HTMLSelectElement>('[data-prices-fuel]');
  const order = section.querySelector<HTMLSelectElement>('[data-prices-order]');
  const status = section.querySelector<HTMLElement>('[data-prices-status]');
  const table = section.querySelector<HTMLElement>('[data-prices-table]');
  const body = section.querySelector<HTMLElement>('[data-prices-body]');
  if (!fuel || !order || !status || !table || !body) return;
  let ticket = 0;
  const finder = setupFinder(section, doc, store, { stations: false, onChange: () => void render() });
  if (!finder) return;

  const render = async (): Promise<void> => {
    const id = ++ticket;
    if (!fuel.value) return;
    status.textContent = 'Carregando preços...';
    try {
      const shard = await store.prices(fuel.value);
      if (id !== ticket) return;
      const rows = filterPrices(shard.rows, finder.query(), finder.scope(), order.value === 'desc' ? 'desc' : 'asc');
      body.replaceChildren(...rows.slice(0, MAX_TABLE_ROWS).map((row) => {
        const tr = doc.createElement('tr');
        const name = doc.createElement('td');
        const link = doc.createElement('a');
        link.href = `/postos/${encodeURIComponent(row.i)}/`;
        link.textContent = row.n;
        name.append(link);
        tr.append(name, cell(doc, place(row) || '—'), cell(doc, formatMilliBRL(row.v), 'num'), cell(doc, formatDateBR(row.d) || '—'));
        return tr;
      }));
      table.hidden = rows.length === 0;
      const where = finder.scope() ? ` em ${finder.scope()?.label}` : ' no Brasil';
      status.textContent = rows.length === 0
        ? `Nenhum preço de ${shard.label} encontrado${where}.`
        : `${shard.label} por ${unitLabel(shard.unit)}${where}: ${rows.length} ${plural(rows.length, 'posto', 'postos')}, ${order.value === 'desc' ? 'maior' : 'menor'} preço primeiro${rows.length > MAX_TABLE_ROWS ? ` (mostrando ${MAX_TABLE_ROWS})` : ''}.`;
    } catch {
      if (id !== ticket) return;
      table.hidden = true;
      status.textContent = 'Preços indisponíveis nesta publicação.';
    }
  };
  fuel.addEventListener('change', () => void render());
  order.addEventListener('change', () => void render());
  void render();
}

function setupFiscal(section: HTMLElement, doc: Document, store: SearchStore): void {
  const status = section.querySelector<HTMLElement>('[data-fiscal-status]');
  const results = section.querySelector<HTMLElement>('[data-fiscal-results]');
  if (!status || !results) return;
  let ticket = 0;
  const finder = setupFinder(section, doc, store, { stations: false, onChange: () => void render() });
  if (!finder) return;

  const render = async (): Promise<void> => {
    const id = ++ticket;
    const scope = finder.scope();
    const query = finder.query();
    if (!scope && normalizeSearchText(query).length < 2) {
      results.replaceChildren();
      status.textContent = 'Digite o nome do posto, parte do CNPJ, uma cidade ou um estado.';
      return;
    }
    status.textContent = 'Carregando registros...';
    try {
      const shard = await store.fiscal();
      if (id !== ticket) return;
      const rows = filterFiscal(shard.records, query, scope);
      results.replaceChildren(...rows.slice(0, MAX_TABLE_ROWS).map((row) => {
        const item = doc.createElement('li');
        const link = doc.createElement('a');
        link.href = `/fiscalizacao/registro/${encodeURIComponent(row.r)}/`;
        link.textContent = `${formatCNPJ(row.c)} — ${row.st}`;
        item.append(link);
        const meta = doc.createElement('span');
        meta.className = 'muted';
        const parts = [row.a, row.d, row.n ? `${row.n}${place(row) ? ` (${place(row)})` : ''}` : 'sem posto vinculado'];
        meta.textContent = ` · ${parts.filter(Boolean).join(' · ')}`;
        item.append(meta);
        return item;
      }));
      if (rows.length === 0) {
        const empty = doc.createElement('li');
        empty.textContent = 'Nenhum registro encontrado.';
        results.append(empty);
      }
      const note = scope ? ' Cidade e estado só existem para registros vinculados a um posto.' : '';
      status.textContent = `${rows.length} ${plural(rows.length, 'registro encontrado', 'registros encontrados')}${rows.length > MAX_TABLE_ROWS ? ` (mostrando ${MAX_TABLE_ROWS})` : ''}.${note}`;
    } catch {
      if (id !== ticket) return;
      results.replaceChildren();
      status.textContent = 'Busca de registros indisponível nesta publicação.';
    }
  };
}

/** Wires every search widget present on the page. */
export function setupPortalSearch(runtime: SearchRuntime = {}): void {
  const doc = runtime.document ?? (typeof document === 'undefined' ? undefined : document);
  if (!doc) return;
  const fetcher = runtime.fetch ?? (typeof fetch === 'undefined' ? undefined : fetch);
  if (!fetcher) return;
  const widgets: Array<[string, (root: HTMLElement, store: SearchStore) => void]> = [
    ['[data-portal-finder]', (root, store) => setupConsultar(root, doc, store, runtime)],
    ['[data-portal-prices]', (root, store) => setupPrices(root, doc, store)],
    ['[data-portal-fiscal]', (root, store) => setupFiscal(root, doc, store)],
  ];
  for (const [selector, setup] of widgets) {
    const root = doc.querySelector<HTMLElement>(selector);
    const release = root?.dataset.release ?? '';
    if (!root || !/^[A-Za-z0-9_-]{1,64}$/.test(release)) continue;
    setup(root, new SearchStore(root.dataset.shardBase || '/dados/busca/', release, fetcher));
  }
}

if (typeof document !== 'undefined') {
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', () => setupPortalSearch());
  } else {
    setupPortalSearch();
  }
}
