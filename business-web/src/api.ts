import { isUuid, record } from './model.js';

export type Failure = 'unauthenticated' | 'fresh' | 'denied' | 'unavailable' | 'cancelled' | 'limited' | 'conflict' | 'review' | 'invalid';
export class ApiError extends Error {
  readonly code: string;
  readonly fields: readonly string[];
  constructor(readonly kind: Failure, code = '', fields: string[] = []) {
    super('Company request refused');
    this.code = SAFE_CODES.has(code) ? code : '';
    this.fields = Object.freeze(fields.filter(safeField));
  }
}
const SAFE_CODES = new Set(['ORG_SETUP_INCOMPLETE','OWNER_REQUIRES_PERSONAL_ACCOUNT','OPERATION_ID_REUSED','CONFLICT','ENTITLEMENT_REQUIRED','ORG_SUSPENDED','ORG_ARCHIVED','VALIDATION_FAILED','FRESH_AUTH_REQUIRED','FORBIDDEN','NOT_FOUND','UNAUTHENTICATED','RATE_LIMITED','DUPLICATE_REVIEW_REQUIRED']);
const safeField = (value: string): boolean => ['name','time_zone','active_unit','active_owner','terms','reason','consumption_display','approval_levels_default','closure_period_days','review_required.trips','review_required.fuelings','review_required.expenses','policies.intake_window_days','policies.clock_skew_limit_seconds','policies.delegation_maximum_days','policies.invitation_validity_days','policies.activation_validity_hours','policies.require_invitation_confirmation'].includes(value) || /^responsibilities\.(?:review_fallback|platform_notices|data_protection|billing)\.(?:primary|substitute)$/.test(value);
export interface SessionView { accountId: string; authenticatedAt: string; expiresAt: string }
export interface SessionPort {
  readonly authenticated: boolean;
  readonly deadline: number;
  signIn(key: string, signal?: AbortSignal): Promise<SessionView>;
  read(signal?: AbortSignal): Promise<SessionView>;
  signOut(signal?: AbortSignal): Promise<void>;
  forget(): void;
}
export interface CompanyTransport {
  privatePost(path: string, body: unknown, signal?: AbortSignal): Promise<Record<string, unknown>>;
}
type Fetch = (path: string, init: RequestInit) => Promise<Response>;
const PREFIX = '/v1/org-access/session/';
const IDLE = 30 * 60_000;
const TIMEOUT = 10_000;

// AbortSignal.any is missing before Safari 17.4, Chrome 116 and Firefox 124.
function bounded(signal?: AbortSignal): AbortSignal {
  const timeout = AbortSignal.timeout(TIMEOUT);
  if (!signal) return timeout;
  if (typeof AbortSignal.any === 'function') return AbortSignal.any([signal, timeout]);
  const combined = new AbortController();
  for (const source of [signal, timeout]) {
    if (source.aborted) combined.abort(source.reason);
    else source.addEventListener('abort', () => combined.abort(source.reason), { once: true });
  }
  return combined.signal;
}
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
  #request(path: string, body: unknown, csrf: string, signal?: AbortSignal, keepalive = false): Promise<Response> {
    const headers: Record<string, string> = { 'Content-Type': 'application/json', Accept: 'application/json' };
    if (csrf !== '') headers['X-CSRF'] = csrf;
    return this.send(path, {
      method: 'POST', headers, body: JSON.stringify(body), credentials: 'same-origin',
      cache: 'no-store', redirect: 'error', referrerPolicy: 'no-referrer', keepalive, signal: bounded(signal),
    });
  }
  // The company plane answers a credential-free read with 401/403 when it is
  // on; 404 means it is off. Checking first keeps the account key, and the
  // account session family its login creates, out of a sign-in that cannot work.
  async #available(signal?: AbortSignal): Promise<void> {
    try {
      const response = await this.#request(PREFIX + 'read', {}, '', signal);
      await response.body?.cancel();
      if (response.status === 401 || response.status === 403) return;
    } catch {
      if (signal?.aborted) throw new ApiError('cancelled');
    }
    throw new ApiError('unavailable');
  }
  // JSON bodies are bounded at 64 KiB; an oversized body is refused unread.
  async #json(response: Response): Promise<Record<string, unknown>> {
    if (!response.headers.get('Content-Type')?.toLowerCase().startsWith('application/json')) throw new ApiError('unavailable');
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
    return record(JSON.parse(new TextDecoder('utf-8', { fatal: true }).decode(bytes)));
  }
  // A 401 is an invalid credential unless the registered company code says the
  // account proof is no longer fresh; an unreadable refusal stays a 401.
  async #refusal(response: Response): Promise<Failure> {
    try { return record((await this.#json(response)).error).code === 'FRESH_AUTH_REQUIRED' ? 'fresh' : 'unauthenticated'; }
    catch { return 'unauthenticated'; }
  }
  async #post(path: string, body: unknown, csrf: string, signal?: AbortSignal, keepalive = false): Promise<Record<string, unknown>> {
    try {
      const response = await this.#request(path, body, csrf, signal, keepalive);
      if (!response.ok) {
        if (response.status === 401) throw new ApiError(await this.#refusal(response));
        if (response.status === 403 || response.status === 404) throw await this.#businessError(response, 'denied');
        if (response.status === 429) throw new ApiError('limited');
        if (response.status === 409) {
          const raw = await this.#json(response);
          const refusal = record(raw.error);
          throw this.#safeError(refusal, refusal.code === 'DUPLICATE_REVIEW_REQUIRED' ? 'review' : 'conflict');
        }
        if (response.status === 422 || response.status === 400) throw await this.#businessError(response, 'invalid');
        throw new ApiError('unavailable');
      }
      if (!response.headers.get('Cache-Control')?.split(',').map(v => v.trim().toLowerCase()).includes('no-store')) throw new ApiError('unavailable');
      return await this.#json(response);
    } catch (error) {
      if (signal?.aborted) throw new ApiError('cancelled');
      if (error instanceof ApiError) throw error;
      throw new ApiError('unavailable');
    }
  }
  #safeError(raw: Record<string, unknown>, kind: Failure): ApiError {
    const fields = Array.isArray(raw.details) ? raw.details.slice(0, 32).map(v => record(v).field).filter((v): v is string => typeof v === 'string' && safeField(v)) : [];
    return new ApiError(kind, typeof raw.code === 'string' ? raw.code : '', fields);
  }
  async #businessError(response: Response, kind: Failure): Promise<ApiError> {
    try { return this.#safeError(record((await this.#json(response)).error), kind); } catch { return new ApiError(kind); }
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
      await this.#available(signal);
      this.#current(epoch);
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
      if (epoch === this.#epoch && error instanceof ApiError && ['unauthenticated', 'fresh', 'denied'].includes(error.kind)) this.forget();
      throw error;
    }
  }
  async privatePost(path: string, body: unknown, signal?: AbortSignal): Promise<Record<string, unknown>> {
    if (!/^\/v1\/org-access\/(?:contexts\/(?:list|read)|organizations\/(?:create|setups\/list))$/.test(path) &&
        !/^\/v1\/orgs\/[0-9a-f-]{36}\/(?:organization\/activate|settings\/(?:read|change|history|revision\/read|responsibility-members\/list))$/.test(path) &&
        !/^\/v1\/orgs\/[0-9a-f-]{36}\/(?:employees\/(?:list|create|matches|[0-9a-f-]{36}\/read)|invitations\/(?:list|issue|[0-9a-f-]{36}\/(?:read|resend|revoke|confirm|reject|acceptance\/read)))$/.test(path)) throw new ApiError('denied');
    if (!this.authenticated) { this.forget(); throw new ApiError('unauthenticated'); }
    const epoch = this.#epoch;
    try {
      const raw = await this.#post(path, body, this.#csrf, signal);
      this.#current(epoch);
      if (!this.authenticated) { this.forget(); throw new ApiError('unauthenticated'); }
      this.#seen = this.now();
      return raw;
    } catch (error) {
      if (epoch === this.#epoch && error instanceof ApiError && ['unauthenticated', 'fresh'].includes(error.kind)) this.forget();
      throw error;
    }
  }
  // Logout is not tied to page work: keepalive lets it finish after pagehide.
  async signOut(signal?: AbortSignal): Promise<void> {
    const csrf = this.#csrf;
    this.forget();
    if (csrf === '') return;
    const raw = await this.#post(PREFIX + 'logout', {}, csrf, signal, true);
    if (raw.logged_out !== true) throw new ApiError('unavailable');
  }
}
