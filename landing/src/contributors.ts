// Public contributors list progressive enhancement (Front C2, CTB-09/10).
//
// Reads the API origin from <meta name="abastevo-api"> set at build time;
// absent or empty means disabled and the module stays inert. Cards render
// with textContent only; links pass the same CTB-08 allowlist as the app.

export interface ContributorLinkItem {
  platform: string;
  handle: string;
  url: string;
}

export interface ContributorItem {
  display_name: string;
  links: ContributorLinkItem[];
  github_login?: string;
  contributor_since?: string;
  roles: string[];
}

export interface ContributorsRuntime {
  document?: ParentNodeLike;
  fetchFn?: (url: string) => Promise<{ ok: boolean; json(): Promise<unknown> }>;
}

export interface ParentNodeLike {
  createElement(tag: string): ElementLike;
  getElementById(id: string): ElementLike | null;
}

export interface ElementLike {
  textContent: string | null;
  appendChild(child: ElementLike): void;
  setAttribute(name: string, value: string): void;
}

const PLATFORM_HOSTS: Record<string, string> = {
  github: 'github.com',
  gitlab: 'gitlab.com',
  linkedin: 'www.linkedin.com',
  x: 'x.com',
  instagram: 'www.instagram.com',
  youtube: 'www.youtube.com',
  bluesky: 'bsky.app',
};

const SINCE_PATTERN = /^\d{4}-\d{2}$/;

export function isAllowedLink(platform: string, url: string): boolean {
  const expected = PLATFORM_HOSTS[platform];
  if (expected === undefined) {
    return false;
  }
  let parsed: URL;
  try {
    parsed = new URL(url);
  } catch {
    return false;
  }
  return parsed.protocol === 'https:' && parsed.host === expected;
}

interface LinkRecord {
  platform: string;
  handle: string;
  url: string;
}

function isLinkRecord(value: unknown): value is LinkRecord {
  if (typeof value !== 'object' || value === null) {
    return false;
  }
  const record = value as Record<string, unknown>;
  return (
    typeof record['platform'] === 'string' &&
    typeof record['handle'] === 'string' &&
    typeof record['url'] === 'string'
  );
}

export function isContributorItem(value: unknown): value is ContributorItem {
  if (typeof value !== 'object' || value === null) {
    return false;
  }
  const item = value as Record<string, unknown>;
  if (typeof item['display_name'] !== 'string' || item['display_name'].trim() === '') {
    return false;
  }
  if (!Array.isArray(item['links']) || !Array.isArray(item['roles'])) {
    return false;
  }
  return true;
}

export function renderContributors(
  doc: ParentNodeLike,
  container: ElementLike,
  items: unknown,
): number {
  let rendered = 0;
  if (!Array.isArray(items)) {
    return rendered;
  }
  for (const raw of items.slice(0, 50)) {
    if (!isContributorItem(raw)) {
      continue;
    }
    const card = doc.createElement('article');
    const name = doc.createElement('h3');
    name.textContent = raw.display_name;
    card.appendChild(name);
    if (typeof raw.github_login === 'string' && raw.github_login !== '') {
      const badge = doc.createElement('p');
      badge.textContent = `GitHub verificado: ${raw.github_login}`;
      card.appendChild(badge);
    }
    if (typeof raw.contributor_since === 'string' && SINCE_PATTERN.test(raw.contributor_since)) {
      const since = doc.createElement('p');
      since.textContent = `Desde ${raw.contributor_since}`;
      card.appendChild(since);
    }
    for (const role of raw.roles.slice(0, 8)) {
      if (typeof role !== 'string' || role === '') {
        continue;
      }
      const chip = doc.createElement('p');
      chip.textContent = role;
      card.appendChild(chip);
    }
    for (const link of raw.links.slice(0, 2)) {
      if (!isLinkRecord(link)) {
        continue;
      }
      if (!isAllowedLink(link.platform, link.url)) {
        continue;
      }
      const anchor = doc.createElement('a');
      anchor.textContent = link.handle;
      anchor.setAttribute('href', link.url);
      anchor.setAttribute('target', '_blank');
      anchor.setAttribute('rel', 'noopener noreferrer nofollow ugc');
      card.appendChild(anchor);
    }
    container.appendChild(card);
    rendered += 1;
  }
  return rendered;
}

export function apiOrigin(doc: ParentNodeLike & { querySelector?(s: string): { getAttribute(n: string): string | null } | null }): string {
  if (typeof doc.querySelector !== 'function') {
    return '';
  }
  const meta = doc.querySelector('meta[name="abastevo-api"]');
  const content = meta?.getAttribute('content')?.trim() ?? '';
  if (content === '') {
    return '';
  }
  try {
    const url = new URL(content);
    if (url.protocol !== 'https:' || url.pathname !== '/' || url.search !== '' || url.hash !== '') {
      return '';
    }
    return url.origin;
  } catch {
    return '';
  }
}

export async function setupContributorsList(runtime: ContributorsRuntime = {}): Promise<void> {
  const doc = runtime.document ?? (typeof document === 'undefined' ? undefined : document as unknown as ParentNodeLike);
  if (doc === undefined) {
    return;
  }
  const origin = apiOrigin(doc);
  if (origin === '') {
    return;
  }
  const container = doc.getElementById('contributors-list');
  if (container === null) {
    return;
  }
  const fetchFn = runtime.fetchFn ?? ((url: string) => fetch(url).then(async (response) => ({
    ok: response.ok,
    json: (): Promise<unknown> => response.json() as Promise<unknown>,
  })));
  let payload: unknown;
  try {
    const response = await fetchFn(`${origin}/v1/contributors`);
    if (!response.ok) {
      return;
    }
    payload = await response.json();
  } catch {
    return;
  }
  if (typeof payload !== 'object' || payload === null || !Array.isArray((payload as Record<string, unknown>)['items'])) {
    return;
  }
  renderContributors(doc, container, (payload as Record<string, unknown>)['items']);
}

if (typeof document !== 'undefined') {
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', () => setupContributorsList());
  } else {
    setupContributorsList();
  }
}
