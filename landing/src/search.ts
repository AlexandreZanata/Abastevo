/**
 * Abastevo portal static search enhancement.
 *
 * Essential station/region navigation stays in HTML. This module only loads
 * one bounded regional shard, filters public fields and renders text nodes.
 * It never calls the Abastevo API, the ANP API or a database.
 */

export const SEARCH_SCHEMA = 'abastevo-search-v1';
export const MAX_QUERY = 80;
export const MAX_RESULTS = 50;

const VALID_UFS = new Set([
  'AC', 'AL', 'AP', 'AM', 'BA', 'CE', 'DF', 'ES', 'GO', 'MA', 'MT', 'MS',
  'MG', 'PA', 'PB', 'PR', 'PE', 'PI', 'RJ', 'RN', 'RS', 'RO', 'RR', 'SC',
  'SP', 'SE', 'TO',
]);

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
}

export interface SearchShard {
  schema: string;
  release: string;
  uf: string;
  count: number;
  stations: SearchStation[];
}

export interface SearchRuntime {
  document?: Document;
  fetch?: typeof fetch;
}

export function normalizeSearchText(value: string): string {
  return value
    .normalize('NFD')
    .replace(/[\u0300-\u036f]/g, '')
    .replace(/[\u0000-\u001f\u007f]/g, '')
    .toLowerCase()
    .replace(/\s+/g, ' ')
    .trim()
    .slice(0, MAX_QUERY);
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
  if (normalized.length < 6) return false;
  if (/[./-]/.test(raw)) return true;
  if (/^\d{6,14}$/.test(normalized)) return true;
  return /^[0-9A-Z]{14}$/.test(normalized) && /[A-Z]/.test(normalized);
}

export function formatCNPJ(value?: string): string {
  const clean = (value ?? '').toUpperCase().replace(/[^0-9A-Z]/g, '');
  if (clean.length !== 14) return value || 'CNPJ não informado';
  return `${clean.slice(0, 2)}.${clean.slice(2, 5)}.${clean.slice(5, 8)}/${clean.slice(8, 12)}-${clean.slice(12)}`;
}

export function findMatches(
  shard: SearchShard,
  query: string,
  city = '',
  limit = MAX_RESULTS,
): SearchStation[] {
  const nameQuery = normalizeSearchText(query);
  const cnpjQuery = normalizeCNPJQuery(query);
  if (nameQuery.length < 2 && cnpjQuery.length < 4) return [];
  const cnpjLike = isCNPJQuery(query);
  const matches = shard.stations.filter((station) => {
    if (city && station.m !== city) return false;
    if (cnpjLike && (station.c ?? '').startsWith(cnpjQuery)) return true;
    return nameQuery.length >= 2 && (station.s || normalizeSearchText(station.n)).includes(nameQuery);
  });
  matches.sort((a, b) => a.n.localeCompare(b.n, 'pt-BR') || a.i.localeCompare(b.i));
  const cap = Math.min(Math.max(limit, 1), MAX_RESULTS);
  return matches.slice(0, cap);
}

export function validateShard(value: unknown, release: string): SearchShard {
  if (!value || typeof value !== 'object') throw new Error('shard-invalid');
  const shard = value as Partial<SearchShard>;
  if (
    shard.schema !== SEARCH_SCHEMA ||
    shard.release !== release ||
    typeof shard.uf !== 'string' ||
    !Array.isArray(shard.stations)
  ) {
    throw new Error('shard-version');
  }
  return shard as SearchShard;
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
  const normalizedBase = base.endsWith('/') ? base : `${base}/`;
  const response = await fetcher(`${normalizedBase}${release}/${uf}.json`, {
    credentials: 'omit',
    cache: 'force-cache',
    signal,
  });
  if (!response.ok) throw new Error('shard-missing');
  return validateShard(await response.json(), release);
}

function plural(value: number, singular: string, pluralForm: string): string {
  return value === 1 ? singular : pluralForm;
}

export function setupPortalSearch(runtime: SearchRuntime = {}): void {
  const doc = runtime.document ?? (typeof document === 'undefined' ? undefined : document);
  if (!doc) return;
  const fetcher = runtime.fetch ?? (typeof fetch === 'undefined' ? undefined : fetch);
  if (!fetcher) return;

  const form = doc.querySelector<HTMLFormElement>('[data-portal-search]');
  if (!form) return;
  const input = form.querySelector<HTMLInputElement>('[data-search-query]');
  const ufSelect = form.querySelector<HTMLSelectElement>('[data-search-uf]');
  const citySelect = form.querySelector<HTMLSelectElement>('[data-search-city]');
  const status = form.querySelector<HTMLElement>('[data-search-status]');
  const results = form.querySelector<HTMLElement>('[data-search-results]');
  if (!input || !ufSelect || !citySelect || !status || !results) return;

  const shardBase = form.dataset.shardBase || '/dados/busca/';
  const release = form.dataset.release || '';
  const limit = Math.min(Number(form.dataset.resultLimit ?? '50') || MAX_RESULTS, MAX_RESULTS);
  let shard: SearchShard | null = null;
  let loadedUF = '';
  let requestId = 0;
  let controller: AbortController | null = null;
  let debounce: ReturnType<typeof setTimeout> | undefined;

  const setStatus = (text: string): void => {
    status.textContent = text;
  };
  const clearResults = (): void => {
    results.replaceChildren();
  };

  const populateCities = (): void => {
    if (!shard) return;
    const current = citySelect.value;
    const unique = new Map<string, string>();
    for (const station of shard.stations) {
      if (!station.m) continue;
      if (!unique.has(station.m)) unique.set(station.m, station.mn || `Município ${station.m}`);
    }
    citySelect.replaceChildren();
    const all = doc.createElement('option');
    all.value = '';
    all.textContent = 'Todos';
    citySelect.append(all);
    for (const [code, name] of [...unique.entries()].sort((a, b) => a[1].localeCompare(b[1], 'pt-BR'))) {
      const option = doc.createElement('option');
      option.value = code;
      option.textContent = name;
      citySelect.append(option);
    }
    citySelect.disabled = unique.size === 0;
    if (current && unique.has(current)) citySelect.value = current;
  };

  const render = (): void => {
    if (!shard) return;
    const matches = findMatches(shard, input.value, citySelect.value, limit);
    results.replaceChildren();
    for (const station of matches) {
      const item = doc.createElement('li');
      const link = doc.createElement('a');
      link.href = `/postos/${encodeURIComponent(station.i)}/`;
      link.textContent = station.n;
      item.append(link);
      const meta = doc.createElement('span');
      meta.className = 'muted';
      const place = [station.mn, station.u].filter((value): value is string => Boolean(value)).join('/');
      const fuelCount = station.p ? Object.keys(station.p).length : 0;
      const priceText = fuelCount
        ? `${fuelCount} ${plural(fuelCount, 'combustível com preço', 'combustíveis com preço')}`
        : 'sem preço nesta pesquisa';
      meta.textContent = ` — ${formatCNPJ(station.c)}${place ? ` · ${place}` : ''} · ${station.sl || station.st || 'status não informado'} · ${priceText}`;
      item.append(meta);
      results.append(item);
    }
    if (matches.length === 0) {
      const item = doc.createElement('li');
      item.textContent = 'Nenhum posto encontrado nesta amostra.';
      results.append(item);
    }
    setStatus(matches.length === 0
      ? 'Nenhum posto encontrado.'
      : `${matches.length} ${plural(matches.length, 'posto encontrado', 'postos encontrados')}.`);
  };

  const load = async (): Promise<void> => {
    const uf = ufSelect.value;
    if (!uf) {
      shard = null;
      loadedUF = '';
      clearResults();
      citySelect.disabled = true;
      setStatus('Escolha uma UF e digite ao menos 2 caracteres.');
      return;
    }
    if (loadedUF === uf && shard) {
      render();
      return;
    }
    requestId += 1;
    const id = requestId;
    controller?.abort();
    controller = new AbortController();
    setStatus('Carregando índice regional...');
    try {
      const loaded = await loadShard(uf, release, shardBase, fetcher, controller.signal);
      if (id !== requestId) return;
      shard = loaded;
      loadedUF = uf;
      populateCities();
      render();
    } catch (error) {
      if (id !== requestId) return;
      if ((error as { name?: string }).name === 'AbortError') return;
      shard = null;
      loadedUF = '';
      clearResults();
      citySelect.disabled = true;
      setStatus('Índice regional indisponível. Use a navegação por estado abaixo.');
    }
  };

  form.addEventListener('submit', (event) => {
    event.preventDefault();
    void load();
  });
  ufSelect.addEventListener('change', () => {
    if (loadedUF !== ufSelect.value) citySelect.value = '';
    void load();
  });
  citySelect.addEventListener('change', render);
  input.addEventListener('input', () => {
    if (debounce) clearTimeout(debounce);
    debounce = setTimeout(() => {
      if (shard) render();
    }, 150);
  });
}

if (typeof document !== 'undefined') {
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', () => setupPortalSearch());
  } else {
    setupPortalSearch();
  }
}
