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
    const mapped = ERROR_MESSAGES[error.code] ?? extraErrorMessage(error.code);
    if (mapped !== undefined && mapped !== null) {
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

  async githubContributors(refresh: boolean): Promise<GithubSnapshot> {
    return this.request<GithubSnapshot>(
      'GET',
      `/v1/admin/github/contributors?refresh=${refresh ? 'true' : 'false'}`,
    );
  }

  async listClaims(state: string, cursor: string): Promise<ClaimPage> {
    const params = new URLSearchParams();
    if (state !== '') {
      params.set('state', state);
    }
    if (cursor !== '') {
      params.set('cursor', cursor);
    }
    const query = params.toString();
    const body = await this.request<{ items: Record<string, unknown>[]; next_cursor: string }>(
      'GET',
      `/v1/admin/contributor-claims${query === '' ? '' : `?${query}`}`,
    );
    return { items: body.items.map(normClaim), next_cursor: body.next_cursor };
  }

  async approveClaim(claimId: string, reason: string, override: boolean): Promise<ApproveResult> {
    const body = await this.request<{ claim: Record<string, unknown>; grant: GrantView | null; login: string }>(
      'POST',
      `/v1/admin/contributor-claims/${encodeURIComponent(claimId)}/approve`,
      { body: { reason, override } },
    );
    return { claim: normClaim(body.claim), grant: body.grant, login: body.login };
  }

  async rejectClaim(claimId: string, reason: string): Promise<ClaimView> {
    const body = await this.request<Record<string, unknown>>(
      'POST',
      `/v1/admin/contributor-claims/${encodeURIComponent(claimId)}/reject`,
      { body: { reason } },
    );
    return normClaim(body);
  }

  async linkGitHub(accountId: string, login: string, reason: string, override: boolean): Promise<{ account_id: string; github_login: string }> {
    return this.request<{ account_id: string; github_login: string }>(
      'POST',
      `/v1/admin/accounts/${encodeURIComponent(accountId)}/github-link`,
      { body: { login, reason, override } },
    );
  }

  async setProfileHidden(accountId: string, reason: string, hidden: boolean): Promise<ContributorProfile> {
    const body = await this.request<Record<string, unknown>>(
      'POST',
      `/v1/admin/contributor-profiles/${encodeURIComponent(accountId)}/${hidden ? 'hide' : 'unhide'}`,
      { body: { reason } },
    );
    return normProfile(body);
  }

  async getRoles(): Promise<RoleInfo[]> {
    const body = await this.request<{ catalog_version: string; roles: RoleInfo[] }>(
      'GET',
      '/v1/admin/roles',
    );
    return body.roles;
  }

  async assignRole(accountId: string, role: string, endAt: string | null, reason: string): Promise<StaffAssignment> {
    return this.request<StaffAssignment>(
      'POST',
      `/v1/admin/accounts/${encodeURIComponent(accountId)}/roles`,
      { body: { role, end_at: endAt, reason } },
    );
  }

  async revokeRole(assignmentId: string, reason: string): Promise<StaffAssignment> {
    return this.request<StaffAssignment>(
      'POST',
      `/v1/admin/role-assignments/${encodeURIComponent(assignmentId)}/revoke`,
      { body: { reason } },
    );
  }

  async listAudit(action: string, targetId: string, cursor: string): Promise<AuditPage> {
    const params = new URLSearchParams();
    if (action !== '') {
      params.set('action', action);
    }
    if (targetId !== '') {
      params.set('target_id', targetId);
    }
    if (cursor !== '') {
      params.set('cursor', cursor);
    }
    const query = params.toString();
    return this.request<AuditPage>(
      'GET',
      `/v1/admin/audit${query === '' ? '' : `?${query}`}`,
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

export interface SnapshotContributor {
  login: string;
  linked_account_id?: string;
  claim_id?: string;
  claim_state?: string;
  claim_proof_status?: string;
}

export interface GithubSnapshot {
  repo: string;
  fetched_at: string;
  stale: boolean;
  contributors: SnapshotContributor[];
}

export interface ClaimView {
  claim_id: string;
  account_id?: string;
  github_login: string;
  state: string;
  proof_status: string;
  gist_id?: string;
  override: boolean;
  decision_reason?: string;
  created_at: string;
  expires_at: string;
  decided_at?: string;
}

export interface ClaimPage {
  items: ClaimView[];
  next_cursor: string;
}

export interface ApproveResult {
  claim: ClaimView;
  grant: GrantView | null;
  login: string;
}

export interface RoleInfo {
  code: string;
  label: string;
}

export interface StaffAssignment {
  id: string;
  account_id: string;
  role: string;
  label: string;
  state: string;
  start_at: string;
  end_at: string | null;
  reason: string;
  created_at: string;
  revoked_at: string | null;
  revoke_reason?: string;
}

export interface AuditEvent {
  id: number;
  at: string;
  session_id?: string;
  action: string;
  target_type: string;
  target_id: string;
  reason: string;
  before?: unknown;
  after?: unknown;
  request_id: string;
  client_ip_hash: string;
}

export interface AuditPage {
  items: AuditEvent[];
  next_cursor: string;
}

export interface ContributorProfile {
  account_id?: string;
  display_name: string;
  links: { platform: string; handle: string; url: string }[];
  visibility: string;
  public_consent: boolean;
  show_roles_publicly: boolean;
  hidden_by_admin?: boolean;
  created_at?: string;
  updated_at?: string;
}

function asString(value: unknown): string {
  return typeof value === 'string' ? value : '';
}

function asOptionalString(value: unknown): string | undefined {
  return typeof value === 'string' && value !== '' ? value : undefined;
}

// The admin claim/profile endpoints predate the lowercase list views: field
// names may arrive capitalized. Normalize both shapes into the documented
// lowercase view.
function normClaim(raw: Record<string, unknown>): ClaimView {
  const pick = (...keys: string[]): string => {
    for (const key of keys) {
      const value = raw[key];
      if (typeof value === 'string' && value !== '') {
        return value;
      }
    }
    return '';
  };
  return {
    claim_id: pick('claim_id', 'ID'),
    account_id: asOptionalString(raw['account_id'] ?? raw['AccountID']),
    github_login: pick('github_login', 'GitHubLogin'),
    state: pick('state', 'State'),
    proof_status: pick('proof_status', 'ProofStatus'),
    gist_id: asOptionalString(raw['gist_id'] ?? raw['GistID']),
    override: raw['override'] === true || raw['Override'] === true,
    decision_reason: asOptionalString(raw['decision_reason'] ?? raw['DecisionReason']),
    created_at: pick('created_at', 'CreatedAt'),
    expires_at: pick('expires_at', 'ExpiresAt'),
    decided_at: asOptionalString(raw['decided_at'] ?? raw['DecidedAt']),
  };
}

function normProfile(raw: Record<string, unknown>): ContributorProfile {
  const links = Array.isArray(raw['links']) ? raw['links'] : raw['Links'];
  return {
    account_id: asOptionalString(raw['account_id'] ?? raw['AccountID']),
    display_name: asString(raw['display_name'] ?? raw['DisplayName']),
    links: (Array.isArray(links) ? links : []) as ContributorProfile['links'],
    visibility: asString(raw['visibility'] ?? raw['Visibility']),
    public_consent: (raw['public_consent'] ?? raw['PublicConsent']) === true,
    show_roles_publicly: (raw['show_roles_publicly'] ?? raw['ShowRolesPublicly']) === true,
    hidden_by_admin: (raw['hidden_by_admin'] ?? raw['HiddenByAdmin']) === true ? true : undefined,
  };
}

const EXTRA_ERROR_MESSAGES: Record<string, string> = {
  'contributor.invalid': 'Entrada inválida. Revise os campos.',
  'contributor.forbidden': 'Operação não permitida para esta conta.',
  'contributor.not-found': 'Claim ou perfil não encontrado.',
  'contributor.conflict': 'Já existe um registro concorrente.',
  'contributor.quota': 'Cota esgotada. Tente mais tarde.',
  'contributor.override-required': 'É preciso marcar override com motivo para este caso.',
  'contributor.claim-state': 'A claim não está mais pendente.',
  'contributor.unavailable': 'Serviço indisponível. Tente mais tarde.',
  'staff.invalid': 'Papel ou data inválidos.',
  'staff.account-ineligible': 'Conta suspensa, excluída ou inexistente.',
  'staff.exists': 'Já existe assignment ativo desse papel para a conta.',
  'staff.already-revoked': 'Assignment já revogado.',
  'staff.not-found': 'Assignment não encontrado.',
  'staff.unavailable': 'Serviço indisponível. Tente mais tarde.',
};

export function extraErrorMessage(code: string): string | null {
  return EXTRA_ERROR_MESSAGES[code] ?? null;
}
