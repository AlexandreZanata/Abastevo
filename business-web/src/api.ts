import { isUuid, record } from './model.js';

export type Failure = 'unauthenticated' | 'denied' | 'unavailable' | 'cancelled' | 'limited';
export class ApiError extends Error {
  constructor(readonly kind: Failure) { super('Company request refused'); }
}
export interface SessionView { accountId: string; authenticatedAt: string; expiresAt: string }
export interface SessionPort {
  readonly authenticated: boolean;
  readonly deadline: number;
  signIn(key: string, signal?: AbortSignal): Promise<SessionView>;
  read(signal?: AbortSignal): Promise<SessionView>;
  signOut(signal?: AbortSignal): Promise<void>;
  forget(): void;
}
type Fetch = (path: string, init: RequestInit) => Promise<Response>;
const PREFIX = '/v1/org-access/session/';
const IDLE = 30 * 60_000;
export class CompanyApi implements SessionPort {
  #csrf = '';
  #accountId = '';
  #expires = 0;
  #seen = 0;
  #epoch = 0;
  constructor(private readonly send: Fetch, private readonly now = () => Date.now()) {}
  get authenticated(): boolean { return this.#csrf !== '' && this.now() < this.deadline; }
  get deadline(): number { return Math.min(this.#expires, this.#seen + IDLE); }
  forget(): void { this.#epoch++; this.#csrf = ''; this.#accountId = ''; this.#expires = 0; this.#seen = 0; }
  async #post(path: string, body: unknown, csrf: string, signal?: AbortSignal): Promise<Record<string, unknown>> {
    const headers: Record<string, string> = { 'Content-Type': 'application/json', Accept: 'application/json' };
    if (csrf !== '') headers['X-CSRF'] = csrf;
    try {
      const response = await this.send(path, {
        method: 'POST', headers, body: JSON.stringify(body), credentials: 'same-origin',
        cache: 'no-store', redirect: 'error', referrerPolicy: 'no-referrer',
        signal: signal ? AbortSignal.any([signal, AbortSignal.timeout(10_000)]) : AbortSignal.timeout(10_000),
      });
      if (!response.ok) {
        if (response.status === 401) throw new ApiError('unauthenticated');
        if (response.status === 403) throw new ApiError('denied');
        if (response.status === 429) throw new ApiError('limited');
        throw new ApiError('unavailable');
      }
      if (!response.headers.get('Cache-Control')?.split(',').map(v => v.trim().toLowerCase()).includes('no-store') ||
          !response.headers.get('Content-Type')?.toLowerCase().startsWith('application/json')) throw new ApiError('unavailable');
      const reader = response.body?.getReader();
      if (!reader) throw new ApiError('unavailable');
      let size = 0;
      const chunks: Uint8Array[] = [];
      try {
        for (;;) {
          const part = await reader.read();
          if (part.done) break;
          size += part.value.length;
          if (size > 64 * 1024) { await reader.cancel(); throw new ApiError('unavailable'); }
          chunks.push(part.value);
        }
      } finally { reader.releaseLock(); }
      const bytes = new Uint8Array(size);
      let offset = 0;
      for (const chunk of chunks) { bytes.set(chunk, offset); offset += chunk.length; }
      return record(JSON.parse(new TextDecoder().decode(bytes)));
    } catch (error) {
      if (signal?.aborted) throw new ApiError('cancelled');
      if (error instanceof ApiError) throw error;
      throw new ApiError('unavailable');
    }
  }
  #view(raw: Record<string, unknown>): SessionView {
    if (!isUuid(raw.account_id) || typeof raw.authenticated_at !== 'string' || typeof raw.expires_at !== 'string' ||
        !Number.isFinite(Date.parse(raw.authenticated_at)) || !Number.isFinite(Date.parse(raw.expires_at)) ||
        Date.parse(raw.expires_at) <= this.now()) throw new ApiError('unauthenticated');
    return { accountId: raw.account_id, authenticatedAt: raw.authenticated_at, expiresAt: raw.expires_at };
  }
  #current(epoch: number): void { if (epoch !== this.#epoch) throw new ApiError('cancelled'); }
  async signIn(key: string, signal?: AbortSignal): Promise<SessionView> {
    this.forget();
    const epoch = this.#epoch;
    if (key.length === 0 || key.length > 256) throw new ApiError('unauthenticated');
    try {
      // Account proof is local to this exchange; no refresh token is kept.
      const proof = await this.#post('/v1/accounts/keys/login', { account_key: key }, '', signal);
      this.#current(epoch);
      const session = record(proof.session);
      const account = record(proof.account);
      if (!isUuid(session.family_id) || typeof session.access_token !== 'string' || session.access_token.length === 0 || !isUuid(account.account_id)) throw new ApiError('unavailable');
      const raw = await this.#post(PREFIX + 'login', { family_id: session.family_id, access_token: session.access_token }, '', signal);
      this.#current(epoch);
      const view = this.#view(raw);
      if (view.accountId !== account.account_id || typeof raw.csrf_token !== 'string' || !/^abv_company_csrf_[0-9a-f]{64}$/.test(raw.csrf_token)) throw new ApiError('unauthenticated');
      this.#csrf = raw.csrf_token; this.#accountId = view.accountId;
      this.#expires = Date.parse(view.expiresAt); this.#seen = this.now();
      return view;
    } catch (error) { if (epoch === this.#epoch) this.forget(); throw error; }
  }
  async read(signal?: AbortSignal): Promise<SessionView> {
    if (!this.authenticated) { this.forget(); throw new ApiError('unauthenticated'); }
    const epoch = this.#epoch;
    try {
      const raw = await this.#post(PREFIX + 'read', {}, this.#csrf, signal);
      this.#current(epoch);
      const view = this.#view(raw);
      if (view.accountId !== this.#accountId) throw new ApiError('unauthenticated');
      this.#expires = Date.parse(view.expiresAt); this.#seen = this.now();
      return view;
    } catch (error) {
      if (epoch === this.#epoch && error instanceof ApiError && ['unauthenticated', 'denied'].includes(error.kind)) this.forget();
      throw error;
    }
  }
  async signOut(signal?: AbortSignal): Promise<void> {
    const csrf = this.#csrf;
    this.forget();
    if (csrf === '') return;
    const raw = await this.#post(PREFIX + 'logout', {}, csrf, signal);
    if (raw.logged_out !== true) throw new ApiError('unavailable');
  }
}
