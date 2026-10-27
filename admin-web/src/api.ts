// Admin API client. DOM-free: safe to import in Node tests.
// Calls relative /v1/admin/... paths with same-origin credentials; the CSRF
// token lives in a module-level variable only.

export interface AccountSummary {
  account_id: string;
  alias: string;
  status: string;
  created_at: string;
}

export interface GrantView {
  id: string;
  account_id: string;
  tier: string;
  policy_version: number;
  state: string;
  start_at: string;
  end_at: string | null;
  reason: string;
  created_at: string;
  revoked_at: string | null;
  revoke_reason?: string;
}

export interface PaidSummary {
  state: string;
  valid_until: string | null;
}

export interface AccountDetail {
  account_id: string;
  alias: string;
  status: string;
  created_at: string;
  grants: GrantView[];
  paid: PaidSummary;
}

export interface GrantResult {
  grant: GrantView;
  replaced: GrantView | null;
}

export interface CreateGrantInput {
  tier: string;
  endAt: string | null;
  reason: string;
  replace: boolean;
}

export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly retryAfter: number | null;

  constructor(status: number, code: string, message: string, retryAfter: number | null = null) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.code = code;
    this.retryAfter = retryAfter;
  }
}

export function isUnauthorized(error: unknown): boolean {
  return error instanceof ApiError && error.status === 401;
}

const ERROR_MESSAGES: Record<string, string> = {
  'admin.denied': 'Chave ou sessão inválida.',
  'admin.csrf': 'Falha de CSRF ou origem. Recarregue a página e tente de novo.',
  'admin.invalid': 'Entrada inválida. Revise os campos.',
  'admin.too-large': 'Pedido grande demais.',
  'admin.conflict': 'Conflito de idempotência. Tente de novo.',
  'admin.not-found': 'Recurso não encontrado.',
  'admin.unavailable': 'Serviço indisponível. Tente mais tarde.',
  'entitlement.exists': 'Já existe um grant ativo desse tier para a conta.',
  'entitlement.account-ineligible': 'Conta suspensa, excluída ou inexistente.',
  'entitlement.invalid': 'Tier ou data final inválidos.',
  'entitlement.already-revoked': 'Grant já revogado.',
  'entitlement.not-found': 'Grant não encontrado.',
  'entitlement.unavailable': 'Serviço indisponível. Tente mais tarde.',
};

export function errorMessage(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.code === 'admin.rate-limited') {
      const wait = error.retryAfter === null ? '' : ` Aguarde ${error.retryAfter} s.`;
      return `Muitas tentativas.${wait}`;
    }
    const mapped = ERROR_MESSAGES[error.code];
    if (mapped !== undefined) {
      return mapped;
    }
    return `Erro inesperado (${error.code}).`;
  }
  return 'Falha de rede. Verifique a conexão e tente de novo.';
}

export type FetchFunction = (input: string, init?: RequestInit) => Promise<Response>;
export type UnauthorizedHandler = () => void;

// The DOM fetch must run with the window receiver; wrap it so the client
// never depends on how it was passed in.
function defaultFetch(input: string, init?: RequestInit): Promise<Response> {
  return fetch(input, init);
}

interface ErrorEnvelope {
  error?: { code?: string; message?: string };
}

export class AdminApi {
  private csrfToken: string | null = null;

  constructor(
    private readonly fetchFn: FetchFunction = defaultFetch,
    private readonly onUnauthorized: UnauthorizedHandler = () => {},
  ) {}

  hasSession(): boolean {
    return this.csrfToken !== null;
  }

  clearSession(): void {
    this.csrfToken = null;
  }

  async login(key: string): Promise<{ expiresAt: string }> {
    const body = await this.request<{ csrf_token: string; expires_at: string }>(
      'POST',
      '/v1/admin/session',
      { body: { key }, mutate: true, allowAnonymous: true },
    );
    if (typeof body.csrf_token !== 'string' || body.csrf_token === '') {
      throw new ApiError(500, 'admin.unavailable', 'Resposta de login inválida.');
    }
    this.csrfToken = body.csrf_token;
    return { expiresAt: body.expires_at };
  }

  async whoami(): Promise<{ fingerprint: string; expiresAt: string }> {
    const body = await this.request<{ key_fingerprint: string; expires_at: string }>(
      'GET',
      '/v1/admin/session',
    );
    return { fingerprint: body.key_fingerprint, expiresAt: body.expires_at };
  }

  async logout(reason: string): Promise<void> {
    await this.request<unknown>('DELETE', '/v1/admin/session', { body: { reason } });
    this.csrfToken = null;
  }

  async searchAccounts(query: string): Promise<AccountSummary[]> {
    const body = await this.request<{ items: AccountSummary[] }>(
      'GET',
      `/v1/admin/accounts?q=${encodeURIComponent(query)}`,
    );
    return body.items;
  }

  async getAccount(accountId: string): Promise<AccountDetail> {
    return this.request<AccountDetail>('GET', `/v1/admin/accounts/${encodeURIComponent(accountId)}`);
  }

  async createGrant(accountId: string, input: CreateGrantInput): Promise<GrantResult> {
    return this.request<GrantResult>('POST', `/v1/admin/accounts/${encodeURIComponent(accountId)}/grants`, {
      body: { tier: input.tier, end_at: input.endAt, reason: input.reason, replace: input.replace },
    });
  }

  async revokeGrant(grantId: string, reason: string): Promise<GrantView> {
    return this.request<GrantView>(
      'POST',
      `/v1/admin/grants/${encodeURIComponent(grantId)}/revoke`,
      { body: { reason } },
    );
  }

  private async request<T>(
    method: string,
    path: string,
    options: { body?: unknown; mutate?: boolean; allowAnonymous?: boolean } = {},
  ): Promise<T> {
    const mutate = options.mutate ?? method !== 'GET';
    const headers: Record<string, string> = {};
    let payload: string | undefined;
    if (options.body !== undefined) {
      headers['Content-Type'] = 'application/json';
      payload = JSON.stringify(options.body);
    }
    if (mutate) {
      if (this.csrfToken === null && !options.allowAnonymous) {
        throw new Error('admin client has no csrf token');
      }
      if (this.csrfToken !== null) {
        headers['X-Admin-CSRF'] = this.csrfToken;
      }
      headers['Idempotency-Key'] = crypto.randomUUID();
    }
    let response: Response;
    try {
      // Call without a receiver: some fetch implementations (DOM) throw on
      // illegal invocation when called as a method of another object.
      const doFetch = this.fetchFn;
      response = await doFetch(path, { method, headers, body: payload, credentials: 'same-origin' });
    } catch (err) {
      throw new Error(`admin request failed: ${err instanceof Error ? err.message : 'network error'}`);
    }
    if (response.status === 401) {
      this.onUnauthorized();
    }
    if (!response.ok) {
      throw await AdminApi.toApiError(response);
    }
    if (response.status === 204) {
      return undefined as T;
    }
    return (await response.json()) as T;
  }

  private static async toApiError(response: Response): Promise<ApiError> {
    let code = 'admin.unavailable';
    let message = 'Serviço indisponível. Tente mais tarde.';
    try {
      const envelope = (await response.json()) as ErrorEnvelope;
      if (envelope.error?.code) {
        code = envelope.error.code;
      }
      if (envelope.error?.message) {
        message = envelope.error.message;
      }
    } catch {
      // Keep the generic code/message when the body is not JSON.
    }
    let retryAfter: number | null = null;
    if (response.status === 429) {
      const raw = response.headers.get('Retry-After');
      const parsed = raw === null ? Number.NaN : Number.parseInt(raw, 10);
      retryAfter = Number.isFinite(parsed) && parsed > 0 ? parsed : null;
      code = 'admin.rate-limited';
    }
    return new ApiError(response.status, code, message, retryAfter);
  }
}
