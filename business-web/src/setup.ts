import { ApiError, type CompanyTransport } from './api.js';
import { isUuid, record } from './model.js';

export const SYNTHETIC_TERMS = Object.freeze({ version: 'enterprise-synthetic-dpa-v1', digest: '590cc4cf31e1909ac9baacb76092897f3dba07b95719953b0f4588b51348f8b3', kind: 'SYNTHETIC' as const });
export const ORGANIZATION_KINDS = ['DELIVERY', 'CARRIER', 'SERVICES', 'FARM', 'OTHER'] as const;
export interface SetupDraft { name: string; time_zone: string; kinds: string[]; unit_name: string; acceptedTerms: boolean }
export interface ResponsibilityPair { primary: string; substitute: string }
export interface SettingsValues {
  time_zone: string; consumption_display: 'KM_PER_L' | 'L_PER_100KM';
  review_required: { trips: boolean; fuelings: boolean; expenses: boolean };
  approval_levels_default: number; closure_period_days: number;
  policies: { intake_window_days: number; clock_skew_limit_seconds: string; delegation_maximum_days: number; invitation_validity_days: number; activation_validity_hours: number; require_invitation_confirmation?: boolean };
  responsibilities: { review_fallback: ResponsibilityPair; platform_notices: ResponsibilityPair; data_protection: ResponsibilityPair; billing: ResponsibilityPair };
}
export interface SettingsVersion { revision: number; effective_at: string; values: SettingsValues; source: string; reason: string; data_mode: string; author?: { account_id: string; membership_id: string }; operation_id?: string }
export interface SetupOrganization { id: string; name: string; time_zone: string; kinds: string[]; state: string; revision: number; suspension_reason: string }
export interface SetupMember { membership_id: string; display_name: string; profile_pending: boolean }
export interface ResponsibilityMember extends SetupMember { eligible: boolean }
export interface SetupSnapshot {
  settings: SettingsVersion; root_revision: number; organization: SetupOrganization;
  setup: { ready: boolean; missing: string[]; terms: { version: string; digest: string; kind: 'SYNTHETIC'; recorded: boolean } };
  context: { account_id: string; membership_id: string; permissions: string[]; fresh: boolean; capability_enabled: boolean };
  responsibility_members: ResponsibilityMember[];
}
export interface SetupSummary { id: string; name: string; membership_id: string; state: string; root_revision: number }
export interface SetupState {
  phase: 'closed' | 'loading' | 'create' | 'ready' | 'failed' | 'denied';
  accountId: string; organizationId: string; snapshot: SetupSnapshot | null;
  setups: SetupSummary[]; nextSetupCursor: string; members: SetupMember[]; nextMemberCursor: string;
  history: SettingsVersion[]; nextHistoryCursor: string; historical: SettingsVersion | null;
  historicalMembers: ResponsibilityMember[]; message: string; fields: string[];
}
export interface SetupSource {
  read(account: string, org: string, signal: AbortSignal): Promise<SetupSnapshot>;
  setups(account: string, after: string, signal: AbortSignal): Promise<{ items: SetupSummary[]; next: string }>;
  members(account: string, org: string, member: string, after: string, signal: AbortSignal): Promise<{ items: SetupMember[]; next: string }>;
  create(account: string, body: unknown, signal: AbortSignal): Promise<Record<string, unknown>>;
  activate(org: string, body: unknown, signal: AbortSignal): Promise<Record<string, unknown>>;
  change(org: string, body: unknown, signal: AbortSignal): Promise<Record<string, unknown>>;
  history(org: string, cursor: string, signal: AbortSignal): Promise<{ items: SettingsVersion[]; next: string }>;
  revision(account: string, org: string, revision: number, signal: AbortSignal): Promise<SetupSnapshot>;
}
const states = ['PENDING_SETUP', 'ACTIVE', 'SUSPENDED', 'ARCHIVED'];
const missingKeys = ['name', 'time_zone', 'active_unit', 'active_owner', 'terms'];
const permissionKeys = ['org.settings.read', 'org.settings.manage', 'org.structure.manage'];
const responsibilityKeys = ['review_fallback', 'platform_notices', 'data_protection', 'billing'] as const;
const clone = <T>(value: T): T => structuredClone(value);
const invalid = (): ApiError => new ApiError('unavailable');
const positive = (value: unknown): value is number => Number.isSafeInteger(value) && Number(value) >= 1;
const bounded = (value: unknown, min: number, max: number): value is number => Number.isSafeInteger(value) && Number(value) >= min && Number(value) <= max;
const identifier = (value: unknown): value is string => value === '' || isUuid(value);
function metadata(value: unknown, required = false, maximum = 120): value is string {
  if (typeof value !== 'string' || [...value].length > maximum || /[\p{Cc}\u202a-\u202e\u2066-\u2069]/u.test(value) || [...value].some(c => { const n = c.codePointAt(0)!; return n >= 0xd800 && n <= 0xdfff; })) return false;
  return !required || value.replace(/[\s\p{Cf}]/gu, '') !== '';
}
function instant(value: unknown): value is string { return typeof value === 'string' && /^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.\d{1,9})?Z$/.test(value) && Number.isFinite(Date.parse(value)); }
function zone(value: unknown): value is string {
  if (typeof value !== 'string' || !/^[A-Za-z0-9_+/-]{1,128}$/.test(value) || value !== 'UTC' && !value.includes('/') || value.includes('//') || /^(?:posix|right|SystemV)\//.test(value)) return false;
  try { new Intl.DateTimeFormat('en', { timeZone: value }); return true; } catch { return false; }
}
function parseValues(raw: unknown): SettingsValues {
  const v = record(raw), review = record(v.review_required), policies = record(v.policies), responsibilities = record(v.responsibilities);
  if (!zone(v.time_zone) || !['KM_PER_L', 'L_PER_100KM'].includes(String(v.consumption_display)) || !['trips', 'fuelings', 'expenses'].every(k => typeof review[k] === 'boolean') ||
      !bounded(v.approval_levels_default, 1, 3) || !bounded(v.closure_period_days, 7, 90) || !bounded(policies.intake_window_days, 0, 60) || !bounded(policies.delegation_maximum_days, 1, 180) || !bounded(policies.invitation_validity_days, 1, 30) || !bounded(policies.activation_validity_hours, 1, 168) ||
      typeof policies.clock_skew_limit_seconds !== 'string' || !/^(?:0|[1-9][0-9]*)$/.test(policies.clock_skew_limit_seconds) || BigInt(policies.clock_skew_limit_seconds) > 9223372036854775807n || policies.require_invitation_confirmation !== undefined && typeof policies.require_invitation_confirmation !== 'boolean') throw invalid();
  for (const key of responsibilityKeys) {
    const pair = record(responsibilities[key]);
    if (!identifier(pair.primary) || !identifier(pair.substitute) || pair.substitute !== '' && pair.substitute === pair.primary) throw invalid();
  }
  const pairs = Object.fromEntries(responsibilityKeys.map(key => { const pair = record(responsibilities[key]); return [key, { primary: pair.primary, substitute: pair.substitute }]; }));
  return { time_zone: v.time_zone, consumption_display: v.consumption_display as SettingsValues['consumption_display'], review_required: { trips: review.trips, fuelings: review.fuelings, expenses: review.expenses }, approval_levels_default: v.approval_levels_default, closure_period_days: v.closure_period_days, policies: { intake_window_days: policies.intake_window_days, clock_skew_limit_seconds: policies.clock_skew_limit_seconds, delegation_maximum_days: policies.delegation_maximum_days, invitation_validity_days: policies.invitation_validity_days, activation_validity_hours: policies.activation_validity_hours, ...(policies.require_invitation_confirmation !== undefined ? { require_invitation_confirmation: policies.require_invitation_confirmation } : {}) }, responsibilities: pairs } as SettingsValues;
}
function parseVersion(raw: unknown): SettingsVersion {
  const v = record(raw);
  if (!positive(v.revision) || !instant(v.effective_at) || !['ONBOARDING', 'BOOTSTRAP', 'COMMAND', 'MEMBERSHIP_REDUCTION'].includes(String(v.source)) || !metadata(v.reason, true) || v.data_mode !== 'SYNTHETIC') throw invalid();
  const result: SettingsVersion = { revision: v.revision, effective_at: v.effective_at, values: parseValues(v.values), source: String(v.source), reason: v.reason, data_mode: v.data_mode };
  if (v.author !== undefined) { const author = record(v.author); if (!isUuid(author.account_id) || !isUuid(author.membership_id)) throw invalid(); result.author = { account_id: author.account_id, membership_id: author.membership_id }; }
  if (v.operation_id !== undefined) { if (!isUuid(v.operation_id)) throw invalid(); result.operation_id = v.operation_id; }
  return result;
}
function parseMember(raw: unknown): SetupMember {
  const v = record(raw);
  if (!isUuid(v.membership_id) || !metadata(v.display_name, false, 200) || typeof v.profile_pending !== 'boolean' || !v.profile_pending && !metadata(v.display_name, true, 200)) throw invalid();
  return { membership_id: v.membership_id, display_name: v.display_name, profile_pending: v.profile_pending };
}
function parseSnapshot(raw: unknown, account: string, org: string): SetupSnapshot {
  const root = record(raw), organization = record(root.organization), setup = record(root.setup), terms = record(setup.terms), context = record(root.context);
  if (context.account_id !== account || organization.id !== org || !isUuid(context.membership_id)) throw new ApiError('denied');
  if (!positive(root.root_revision) || !positive(organization.revision) || root.root_revision !== organization.revision || !metadata(organization.name) || organization.time_zone !== '' && !zone(organization.time_zone) || !Array.isArray(organization.kinds) || organization.kinds.length > 5 || !organization.kinds.every(k => ORGANIZATION_KINDS.includes(k)) || new Set(organization.kinds).size !== organization.kinds.length || !states.includes(String(organization.state)) || typeof organization.suspension_reason !== 'string' || organization.suspension_reason.length > 120 ||
      typeof setup.ready !== 'boolean' || !Array.isArray(setup.missing) || setup.missing.length > 5 || !setup.missing.every(k => missingKeys.includes(k)) || new Set(setup.missing).size !== setup.missing.length || setup.ready !== (setup.missing.length === 0) || terms.version !== SYNTHETIC_TERMS.version || terms.digest !== SYNTHETIC_TERMS.digest || terms.kind !== 'SYNTHETIC' || typeof terms.recorded !== 'boolean' ||
      !Array.isArray(context.permissions) || context.permissions.length > 3 || !context.permissions.every(k => permissionKeys.includes(k)) || new Set(context.permissions).size !== context.permissions.length || typeof context.fresh !== 'boolean' || typeof context.capability_enabled !== 'boolean' || !Array.isArray(root.responsibility_members) || root.responsibility_members.length > 8) throw invalid();
  const contextPermissions = context.permissions;
  const settings = parseVersion(root);
  const members: ResponsibilityMember[] = root.responsibility_members.map(item => { const value = record(item), member = parseMember(value); if (typeof value.eligible !== 'boolean') throw invalid(); return { ...member, eligible: value.eligible }; });
  members.sort((a, b) => a.membership_id.localeCompare(b.membership_id));
  const selected = new Set(responsibilityKeys.flatMap(k => Object.values(settings.values.responsibilities[k])).filter(Boolean));
  if (new Set(members.map(m => m.membership_id)).size !== members.length || members.length !== selected.size || members.some(m => !selected.has(m.membership_id))) throw invalid();
  return { settings, root_revision: root.root_revision, organization: { id: org, name: organization.name, time_zone: organization.time_zone, kinds: [...organization.kinds], state: String(organization.state), revision: organization.revision, suspension_reason: organization.suspension_reason }, setup: { ready: setup.ready, missing: [...setup.missing], terms: { ...SYNTHETIC_TERMS, recorded: terms.recorded } }, context: { account_id: account, membership_id: context.membership_id, permissions: permissionKeys.filter(key => contextPermissions.includes(key)), fresh: context.fresh, capability_enabled: context.capability_enabled }, responsibility_members: members };
}
function nextId(value: unknown): string { if (!identifier(value)) throw invalid(); return value; }
function pageOrder(ids: string[], after: string, next: string): void {
  if (ids.some((id, i) => id <= after || i > 0 && id <= ids[i - 1]) || next !== '' && (next <= after || ids.some(id => id > next))) throw invalid();
}
export class OrganizationSetupApi implements SetupSource {
  constructor(private readonly api: CompanyTransport) {}
  async read(account: string, org: string, signal: AbortSignal): Promise<SetupSnapshot> { return parseSnapshot(await this.api.privatePost(`/v1/orgs/${org}/settings/read`, {}, signal), account, org); }
  async revision(account: string, org: string, revision: number, signal: AbortSignal): Promise<SetupSnapshot> { return parseSnapshot(await this.api.privatePost(`/v1/orgs/${org}/settings/revision/read`, { revision }, signal), account, org); }
  async setups(account: string, after: string, signal: AbortSignal): Promise<{ items: SetupSummary[]; next: string }> {
    const root = await this.api.privatePost('/v1/org-access/organizations/setups/list', { limit: 50, ...(after ? { after_org_id: after } : {}) }, signal);
    if (root.account_id !== account) throw new ApiError('denied');
    if (!Array.isArray(root.setups) || root.setups.length > 50) throw invalid();
    const items: SetupSummary[] = root.setups.map(raw => { const v = record(raw); if (!isUuid(v.id) || !isUuid(v.membership_id) || !metadata(v.name) || !states.includes(String(v.state)) || !positive(v.root_revision)) throw invalid(); return clone(v) as unknown as SetupSummary; });
    const next = nextId(root.next_after_org_id); pageOrder(items.map(v => v.id), after, next); return { items, next };
  }
  async members(account: string, org: string, member: string, after: string, signal: AbortSignal): Promise<{ items: SetupMember[]; next: string }> {
    const root = await this.api.privatePost(`/v1/orgs/${org}/settings/responsibility-members/list`, { limit: 50, ...(after ? { after_membership_id: after } : {}) }, signal);
    if (root.account_id !== account || root.organization_id !== org || root.membership_id !== member) throw new ApiError('denied');
    if (!Array.isArray(root.members) || root.members.length > 50) throw invalid();
    const items = root.members.map(parseMember), next = nextId(root.next_after_membership_id); pageOrder(items.map(v => v.membership_id), after, next); return { items, next };
  }
  async create(_account: string, body: unknown, signal: AbortSignal): Promise<Record<string, unknown>> {
    const result = await this.api.privatePost('/v1/org-access/organizations/create', body, signal);
    if (!isUuid(result.organization_id) || !isUuid(result.membership_id) || result.state !== 'PENDING_SETUP' || !positive(result.revision) || result.data_mode !== 'SYNTHETIC' || typeof result.replayed !== 'boolean') throw invalid();
    return result;
  }
  async activate(org: string, body: unknown, signal: AbortSignal): Promise<Record<string, unknown>> {
    const result = await this.api.privatePost(`/v1/orgs/${org}/organization/activate`, body, signal);
    if (result.organization_id !== org || result.state !== 'ACTIVE' || !positive(result.revision) || result.data_mode !== 'SYNTHETIC' || typeof result.replayed !== 'boolean') throw invalid();
    return result;
  }
  async change(org: string, body: unknown, signal: AbortSignal): Promise<Record<string, unknown>> {
    const result = await this.api.privatePost(`/v1/orgs/${org}/settings/change`, body, signal);
    if (!positive(result.revision) || !positive(result.root_revision) || !instant(result.effective_at) || result.data_mode !== 'SYNTHETIC' || typeof result.replayed !== 'boolean') throw invalid();
    return result;
  }
  async history(org: string, cursor: string, signal: AbortSignal): Promise<{ items: SettingsVersion[]; next: string }> {
    const root = await this.api.privatePost(`/v1/orgs/${org}/settings/history`, { limit: 20, ...(cursor ? { cursor } : {}) }, signal);
    if (!Array.isArray(root.items) || root.items.length > 20 || typeof root.next_cursor !== 'string' || root.next_cursor.length > 2048) throw invalid();
    const items = root.items.map(parseVersion);
    if (items.some((item, i) => i > 0 && item.revision >= items[i - 1].revision)) throw invalid();
    return { items, next: root.next_cursor };
  }
}
const initial = (): SetupState => ({ phase: 'closed', accountId: '', organizationId: '', snapshot: null, setups: [], nextSetupCursor: '', members: [], nextMemberCursor: '', history: [], nextHistoryCursor: '', historical: null, historicalMembers: [], message: '', fields: [] });
export class OrganizationSetupController {
  #epoch = 0;
  #pending = new AbortController();
  #retry: (() => Promise<void>) | null = null;
  #probe: AbortController | null = null;
  #probeGeneration = 0;
  #confirmed = false;
  state: SetupState = initial();
  constructor(private readonly source: SetupSource, private readonly render: (state: SetupState) => void, private readonly sessionLost?: (message: string) => void) {}
  get retryable(): boolean { return this.#retry !== null; }
  get canEdit(): boolean { const s = this.state.snapshot; return this.#confirmed && !this.retryable && this.state.phase !== 'loading' && s !== null && s.context.permissions.includes('org.settings.manage') && s.context.fresh && s.context.capability_enabled && ['PENDING_SETUP', 'ACTIVE'].includes(s.organization.state); }
  get canActivate(): boolean { return this.canEdit && this.state.snapshot?.organization.state === 'PENDING_SETUP' && this.state.snapshot.setup.ready && this.state.fields.length === 0; }
  get canChooseMembers(): boolean { const c = this.state.snapshot?.context; return this.#confirmed && !this.retryable && this.state.phase !== 'loading' && c !== undefined && c.permissions.includes('org.settings.manage') && c.fresh; }
  #show(values: Partial<SetupState>): void { this.state = { ...this.state, ...values }; this.render(this.state); }
  #current(epoch: number): boolean { return this.#epoch === epoch && !this.#pending.signal.aborted; }
  #cancelProbe(): void { this.#probe?.abort(); this.#probe = null; this.#probeGeneration++; }
  clear(): void { this.#cancelProbe(); this.#confirmed = false; this.#pending.abort(); this.#pending = new AbortController(); this.#epoch++; this.#retry = null; this.state = initial(); this.render(this.state); }
  #failure(error: unknown, epoch: number): void {
    if (!this.#current(epoch) || error instanceof ApiError && error.kind === 'cancelled') return;
    if (error instanceof ApiError && ['ENTITLEMENT_REQUIRED', 'ORG_SUSPENDED', 'ORG_ARCHIVED'].includes(error.code)) {
      this.#retry = null; this.#confirmed = false; this.#show({ phase: 'failed', snapshot: null, members: [], history: [], historical: null, historicalMembers: [], fields: [], message: 'A ação depende da situação e habilitação atuais da empresa. Atualize os dados para consultar os detalhes.' }); return;
    }
    if (error instanceof ApiError && ['unauthenticated', 'fresh', 'denied'].includes(error.kind)) {
      const session = error.kind !== 'denied'; this.clear();
      this.#show({ phase: 'denied', message: session ? 'Confirme seu acesso novamente para continuar.' : 'Seu acesso a esta configuração não pôde ser confirmado.' });
      if (session) this.sessionLost?.('Sua confirmação de acesso expirou. Entre novamente.');
      return;
    }
    if (error instanceof ApiError && error.code === 'ORG_SETUP_INCOMPLETE') {
      this.#retry = null; this.#confirmed = false;
      const missing = missingKeys.filter(key => error.fields.includes(key)), old = this.state.snapshot;
      const snapshot = old && missing.length ? { ...old, setup: { ...old.setup, ready: false, missing, terms: { ...old.setup.terms, recorded: missing.includes('terms') ? false : old.setup.terms.recorded } } } : null;
      this.#show({ phase: 'failed', snapshot, fields: missing, message: 'O servidor confirmou itens pendentes no checklist. Atualize os dados antes de concluir a configuração.' }); return;
    }
    if (error instanceof ApiError && error.kind === 'conflict' && error.code !== 'ORG_SETUP_INCOMPLETE') {
      this.#retry = null; this.#confirmed = false; this.#show({ phase: 'failed', snapshot: null, members: [], history: [], historical: null, historicalMembers: [], fields: [], message: error.code === 'OWNER_REQUIRES_PERSONAL_ACCOUNT' ? 'A criação de empresa exige uma conta pessoal.' : 'Esta configuração mudou. Atualize os dados antes de decidir novamente.' }); return;
    }
    if (error instanceof ApiError && (error.kind === 'invalid' || error.code === 'ORG_SETUP_INCOMPLETE' || error.kind === 'limited')) this.#retry = null;
    this.#show({ phase: 'failed', fields: error instanceof ApiError ? [...error.fields] : [], message: error instanceof ApiError && error.code === 'ORG_SETUP_INCOMPLETE' ? 'A configuração ainda tem itens pendentes. Confira o checklist.' : error instanceof ApiError && error.kind === 'invalid' ? 'Confira os campos indicados e tente novamente.' : error instanceof ApiError && error.kind === 'limited' ? 'Muitas tentativas. Aguarde antes de tentar novamente.' : 'Não foi possível confirmar a operação. Consulte novamente a mesma tentativa ou atualize os dados.' });
  }
  async #run(work: (epoch: number, signal: AbortSignal) => Promise<void>, retry = false): Promise<void> {
    if (!retry && this.#retry) return;
    this.#cancelProbe(); this.#pending.abort(); this.#pending = new AbortController(); const epoch = ++this.#epoch;
    this.#retry = retry ? () => this.#run(work, true) : null;
    this.#show({ phase: 'loading', message: '', fields: [] });
    try { await work(epoch, this.#pending.signal); } catch (error) { this.#failure(error, epoch); }
  }
  async #read(account: string, org: string, epoch: number, signal: AbortSignal, message = ''): Promise<void> {
    const snapshot = await this.source.read(account, org, signal); if (!this.#current(epoch)) return;
    if (snapshot.context.account_id !== account || snapshot.organization.id !== org) throw new ApiError('denied');
    this.#retry = null; this.#confirmed = true;
    this.#show({ phase: 'ready', accountId: account, organizationId: org, snapshot, members: [], history: [], historical: null, historicalMembers: [], nextMemberCursor: '', nextHistoryCursor: '', message, fields: [] });
  }
  async open(accountId: string, organizationId?: string): Promise<void> {
    if (this.#retry && accountId === this.state.accountId && (organizationId ?? '') === this.state.organizationId) return;
    this.clear(); if (!isUuid(accountId) || organizationId !== undefined && !isUuid(organizationId)) { this.#show({ phase: 'denied' }); return; }
    this.#show({ accountId, organizationId: organizationId ?? '' });
    if (organizationId) await this.#run((epoch, signal) => this.#read(accountId, organizationId, epoch, signal));
    else await this.loadSetups();
  }
  async select(orgId: string): Promise<void> { const account = this.state.accountId; await this.open(account, orgId); }
  async revalidate(): Promise<void> {
    const account = this.state.accountId, org = this.state.organizationId;
    if (!isUuid(account)) return;
    const epoch = this.#epoch, member = this.state.snapshot?.context.membership_id, pending = this.retryable;
    this.#cancelProbe(); const probe = new AbortController(), generation = this.#probeGeneration; this.#probe = probe;
    const retained = (): boolean => this.#epoch === epoch && this.#probeGeneration === generation && !probe.signal.aborted && this.state.accountId === account && this.state.organizationId === org;
    try {
      if (org) {
        const snapshot = await this.source.read(account, org, probe.signal);
        if (!retained()) return;
        if (snapshot.context.account_id !== account || snapshot.organization.id !== org || member !== undefined && snapshot.context.membership_id !== member) throw new ApiError('denied');
        if (!pending) {
          const unchanged = this.#confirmed && JSON.stringify(snapshot) === JSON.stringify(this.state.snapshot);
          this.#confirmed = true;
          if (!unchanged) this.#show({ phase: 'ready', snapshot, members: [], nextMemberCursor: '', message: '', fields: [] });
        }
      } else {
        const page = await this.source.setups(account, '', probe.signal);
        if (retained() && !pending) {
          const unchanged = this.#confirmed && JSON.stringify(page.items) === JSON.stringify(this.state.setups) && page.next === this.state.nextSetupCursor;
          this.#confirmed = true;
          if (!unchanged) this.#show({ phase: 'create', setups: page.items, nextSetupCursor: page.next, fields: [], message: '' });
        }
      }
    } catch (error) {
      if (!retained()) return;
      if (error instanceof ApiError && ['denied', 'fresh', 'unauthenticated'].includes(error.kind)) this.#failure(error, epoch);
      else if (!pending) { this.#confirmed = false; this.#show({ phase: 'failed', message: 'Não foi possível confirmar os dados atuais. Atualize antes de alterar a configuração.' }); }
    } finally { if (this.#probe === probe) this.#probe = null; }
  }
  async refresh(): Promise<void> { if (this.#retry) return; const { accountId, organizationId } = this.state; await this.open(accountId, organizationId || undefined); }
  async loadSetups(): Promise<void> {
    const account = this.state.accountId; if (!isUuid(account)) return;
    await this.#run(async (epoch, signal) => { const page = await this.source.setups(account, '', signal); if (this.#current(epoch)) { this.#confirmed = true; this.#show({ phase: 'create', setups: page.items, nextSetupCursor: page.next, snapshot: null, organizationId: '' }); } });
  }
  async moreSetups(): Promise<void> {
    const { accountId, nextSetupCursor } = this.state; if (!nextSetupCursor) return;
    await this.#run(async (epoch, signal) => { const page = await this.source.setups(accountId, nextSetupCursor, signal); if (!this.#current(epoch)) return; if (page.items.some(item => this.state.setups.some(known => known.id === item.id)) || page.next === nextSetupCursor) throw invalid(); this.#show({ phase: 'create', setups: [...this.state.setups, ...page.items], nextSetupCursor: page.next }); });
  }
  async #recover(account: string, org: string, epoch: number, signal: AbortSignal, message: string): Promise<void> {
    const read = (nextEpoch: number, nextSignal: AbortSignal) => this.#read(account, org, nextEpoch, nextSignal, message);
    this.#retry = () => this.#run(read, true);
    this.#confirmed = false;
    this.#show({ organizationId: org, snapshot: null, members: [], history: [], historical: null, historicalMembers: [] });
    await read(epoch, signal);
  }
  async create(draft: SetupDraft): Promise<void> {
    if (this.#retry) { this.#show({ message: 'Uma criação anterior ainda não foi confirmada. Consulte a mesma tentativa ou os cadastros salvos.' }); return; }
    const account = this.state.accountId; if (!isUuid(account)) return;
    if (!metadata(draft.name, true) || !metadata(draft.unit_name, true) || !zone(draft.time_zone) || !draft.acceptedTerms || draft.kinds.length < 1 || draft.kinds.length > 5 || !draft.kinds.every(k => ORGANIZATION_KINDS.includes(k as typeof ORGANIZATION_KINDS[number])) || new Set(draft.kinds).size !== draft.kinds.length) { this.#show({ phase: 'failed', message: 'Confira nome, tipo, fuso, unidade e aceite dos termos de teste.' }); return; }
    const body = clone({ schema_version: 1, operation_id: crypto.randomUUID(), name: draft.name.trim(), time_zone: draft.time_zone, kinds: draft.kinds, unit_name: draft.unit_name.trim(), terms: { version: SYNTHETIC_TERMS.version, digest: SYNTHETIC_TERMS.digest, accepted: true } });
    await this.#run(async (epoch, signal) => { const result = await this.source.create(account, clone(body), signal); if (!this.#current(epoch)) return; if (!isUuid(result.organization_id)) throw invalid(); await this.#recover(account, result.organization_id, epoch, signal, 'Empresa salva no servidor. A situação atual está confirmada abaixo.'); }, true);
  }
  async save(values: SettingsValues, reason: string): Promise<void> {
    if (!this.canEdit) return;
    if (!metadata(reason, true)) { this.#show({ phase: 'failed', message: 'Informe um motivo válido de até 120 caracteres.' }); return; }
    let captured: SettingsValues; try { captured = parseValues(values); } catch { this.#show({ phase: 'failed', message: 'Confira os valores das configurações.' }); return; }
    const snapshot = this.state.snapshot!, account = this.state.accountId, org = this.state.organizationId;
    for (const key of responsibilityKeys) for (const part of ['primary', 'substitute'] as const) {
      const id = captured.responsibilities[key][part];
      if (!id) continue;
      const saved = snapshot.responsibility_members.find(m => m.membership_id === id && m.eligible);
      const selected = this.state.members.find(m => m.membership_id === id && !m.profile_pending && metadata(m.display_name, true, 200));
      if (id !== snapshot.context.membership_id && !saved && !selected) {
        this.#show({ phase: 'failed', fields: [`responsibilities.${key}.${part}`], message: 'Escolha uma pessoa identificada na lista atual. A designação salva foi mantida.' }); return;
      }
    }
    const body = clone({ schema_version: 1, operation_id: crypto.randomUUID(), revision: snapshot.settings.revision, values: captured, reason: reason.trim() });
    await this.#run(async (epoch, signal) => { await this.source.change(org, clone(body), signal); if (!this.#current(epoch)) return; await this.#recover(account, org, epoch, signal, 'Configurações salvas no servidor.'); }, true);
  }
  async activate(): Promise<void> {
    if (!this.canActivate) return;
    const account = this.state.accountId, org = this.state.organizationId;
    const body = { schema_version: 1, operation_id: crypto.randomUUID(), revision: this.state.snapshot!.root_revision };
    await this.#run(async (epoch, signal) => { await this.source.activate(org, clone(body), signal); if (!this.#current(epoch)) return; await this.#recover(account, org, epoch, signal, 'Configuração concluída no servidor. A disponibilidade operacional depende das permissões e habilitações da empresa.'); }, true);
  }
  async members(): Promise<void> { await this.#members(false); }
  async moreMembers(): Promise<void> { await this.#members(true); }
  async #members(more: boolean): Promise<void> {
    if (!this.canChooseMembers) return;
    const { accountId, organizationId, snapshot, nextMemberCursor } = this.state; const after = more ? nextMemberCursor : ''; if (more && !after) return;
    await this.#run(async (epoch, signal) => { const page = await this.source.members(accountId, organizationId, snapshot!.context.membership_id, after, signal); if (!this.#current(epoch)) return; const prior = more ? this.state.members : []; if (page.items.some(item => prior.some(p => p.membership_id === item.membership_id)) || after !== '' && page.next === after) throw invalid(); this.#show({ phase: 'ready', members: [...prior, ...page.items], nextMemberCursor: page.next }); });
  }
  async history(): Promise<void> { await this.#history(false); }
  async moreHistory(): Promise<void> { await this.#history(true); }
  async #history(more: boolean): Promise<void> {
    if (!this.state.snapshot) return;
    const org = this.state.organizationId, cursor = more ? this.state.nextHistoryCursor : ''; if (more && !cursor) return;
    await this.#run(async (epoch, signal) => { const page = await this.source.history(org, cursor, signal); if (!this.#current(epoch)) return; const prior = more ? this.state.history : []; if (page.items.some(item => prior.some(p => p.revision === item.revision)) || cursor !== '' && page.next === cursor) throw invalid(); this.#show({ phase: 'ready', history: [...prior, ...page.items], nextHistoryCursor: page.next }); });
  }
  async readRevision(revision: number): Promise<void> {
    if (!this.state.snapshot || !positive(revision)) return;
    const account = this.state.accountId, org = this.state.organizationId, member = this.state.snapshot.context.membership_id;
    await this.#run(async (epoch, signal) => {
      const historical = await this.source.revision(account, org, revision, signal);
      if (!this.#current(epoch)) return;
      if (historical.settings.revision !== revision) throw invalid();
      let current: SetupSnapshot;
      try { current = await this.source.read(account, org, signal); }
      catch (error) { if (this.#current(epoch)) { this.#confirmed = false; this.#show({ snapshot: null }); } throw error; }
      if (!this.#current(epoch)) return;
      if (historical.context.membership_id !== member || current.context.membership_id !== member) throw new ApiError('denied');
      this.#confirmed = true;
      this.#show({ phase: 'ready', snapshot: current, historical: historical.settings, historicalMembers: historical.responsibility_members });
    });
  }
  async retry(): Promise<void> { await this.#retry?.(); }
}
