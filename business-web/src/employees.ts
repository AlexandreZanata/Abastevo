import { ApiError, type CompanyTransport } from './api.js';
import { type Authority, type InvitationGrant, isUuid, record } from './model.js';
import { validGrant } from './company.js';

export interface Employee { id: string; state: string; accessState: string; displayName: string; registrationNumber: string; profileRevision: number; homeUnitId: string; workEmail: string; workPhone: string; jobTitle: string }
export interface Invitation { id: string; employeeId: string; membershipId: string; issuerMembershipId: string; state: string; revision: number; grants: InvitationGrant[]; expiresAt: string; validityDays: number; failedAttempts: number; cause?: string }
export interface Admission { invitationId: string; membershipId: string; accountAlias: string; state: string; revision: number; deadline: string }
export interface Delivery { code: string; link: string }
export interface Page<T> { items: T[]; nextCursor: string }
export interface EmployeeProfile { display_name: string; registration_number: string; work_email: string; work_phone: string; job_title: string; home_unit_id: string }
export interface AccessSource {
  employees(orgId: string, state: string, cursor: string, signal: AbortSignal): Promise<Page<Employee>>;
  employee(orgId: string, id: string, signal: AbortSignal): Promise<Employee>;
  matches(orgId: string, profile: EmployeeProfile, signal: AbortSignal): Promise<Page<Employee>>;
  create(orgId: string, body: unknown, signal: AbortSignal): Promise<Record<string, unknown>>;
  invitations(orgId: string, employeeId: string, cursor: string, signal: AbortSignal): Promise<Page<Invitation>>;
  issue(orgId: string, body: unknown, signal: AbortSignal): Promise<{ invitation: Invitation; delivery: Delivery | null }>;
  invitationAction(orgId: string, id: string, action: string, body: unknown, signal: AbortSignal): Promise<{ invitation: Invitation; delivery: Delivery | null }>;
  admission(orgId: string, id: string, signal: AbortSignal): Promise<Admission>;
  admissionAction(orgId: string, id: string, action: string, body: unknown, signal: AbortSignal): Promise<Admission>;
}
const text = (v: unknown, limit = 500): v is string => typeof v === 'string' && v.length <= limit;
function parseEmployee(raw: unknown): Employee {
  const v = record(raw);
  if (!isUuid(v.id) || !['ACTIVE', 'INACTIVE', 'TERMINATED', 'MERGED'].includes(String(v.state)) || !['NONE', 'INVITED', 'ACTIVE', 'SUSPENDED', 'UNKNOWN'].includes(String(v.access_state)) || !Number.isSafeInteger(v.profile_revision) || Number(v.profile_revision) < 0 ||
      !['display_name', 'registration_number', 'home_unit_id', 'work_email', 'work_phone', 'job_title'].every(k => text(v[k]))) throw new ApiError('unavailable');
  return { id: v.id, state: String(v.state), accessState: String(v.access_state), displayName: String(v.display_name), registrationNumber: String(v.registration_number), profileRevision: Number(v.profile_revision), homeUnitId: String(v.home_unit_id), workEmail: String(v.work_email), workPhone: String(v.work_phone), jobTitle: String(v.job_title) };
}
function parseInvitation(raw: unknown): Invitation {
  const v = record(raw);
  if (!isUuid(v.id) || !isUuid(v.employee_id) || !isUuid(v.membership_id) || !isUuid(v.issuer_membership_id) || !['PENDING', 'ACCEPTED', 'EXPIRED', 'REVOKED', 'SUPERSEDED', 'DECLINED'].includes(String(v.state)) ||
      !Number.isSafeInteger(v.revision) || Number(v.revision) < 1 || !Array.isArray(v.grants) || v.grants.length > 10 || !v.grants.every(validGrant) ||
      !text(v.expires_at, 60) || !Number.isFinite(Date.parse(v.expires_at)) || !Number.isInteger(v.validity_days) || !Number.isInteger(v.failed_attempts)) throw new ApiError('unavailable');
  return { id: v.id, employeeId: v.employee_id, membershipId: v.membership_id, issuerMembershipId: v.issuer_membership_id, state: String(v.state), revision: Number(v.revision), grants: v.grants.map(g => ({ ...g })), expiresAt: v.expires_at, validityDays: Number(v.validity_days), failedAttempts: Number(v.failed_attempts), cause: text(v.cause) ? v.cause : '' };
}
function parseAdmission(raw: unknown): Admission {
  const v = record(raw);
  if (!isUuid(v.invitation_id) || !isUuid(v.membership_id) || !text(v.account_alias, 200) || !text(v.state, 40) || !Number.isSafeInteger(v.revision) || Number(v.revision) < 1 || !text(v.confirmation_deadline, 60)) throw new ApiError('unavailable');
  return { invitationId: v.invitation_id, membershipId: v.membership_id, accountAlias: v.account_alias, state: v.state, revision: Number(v.revision), deadline: v.confirmation_deadline };
}
function parsePage<T>(raw: Record<string, unknown>, parse: (v: unknown) => T): Page<T> {
  if (!Array.isArray(raw.items) || raw.items.length > 50 || !text(raw.next_cursor, 8192)) throw new ApiError('unavailable');
  return { items: raw.items.map(parse), nextCursor: raw.next_cursor };
}
export class EmployeeAccessApi implements AccessSource {
  constructor(private readonly api: CompanyTransport) {}
  #path(org: string, tail: string): string { if (!isUuid(org)) throw new ApiError('denied'); return `/v1/orgs/${org}/${tail}`; }
  async employees(org: string, state: string, cursor: string, signal: AbortSignal): Promise<Page<Employee>> {
    return parsePage(await this.api.privatePost(this.#path(org, 'employees/list'), { limit: 50, ...(state ? { state } : {}), ...(cursor ? { cursor } : {}) }, signal), parseEmployee);
  }
  async employee(org: string, id: string, signal: AbortSignal): Promise<Employee> {
    if (!isUuid(id)) throw new ApiError('denied');
    return parseEmployee(await this.api.privatePost(this.#path(org, `employees/${id}/read`), {}, signal));
  }
  async matches(org: string, profile: EmployeeProfile, signal: AbortSignal): Promise<Page<Employee>> {
    return parsePage(await this.api.privatePost(this.#path(org, 'employees/matches'), { limit: 50, display_name: profile.display_name, work_email: profile.work_email, work_phone: profile.work_phone, exclude_employee_id: '' }, signal), parseEmployee);
  }
  create(org: string, body: unknown, signal: AbortSignal): Promise<Record<string, unknown>> { return this.api.privatePost(this.#path(org, 'employees/create'), body, signal); }
  async invitations(org: string, employeeId: string, cursor: string, signal: AbortSignal): Promise<Page<Invitation>> {
    return parsePage(await this.api.privatePost(this.#path(org, 'invitations/list'), { limit: 50, ...(employeeId ? { employee_id: employeeId } : {}), ...(cursor ? { cursor } : {}) }, signal), parseInvitation);
  }
  #delivery(raw: Record<string, unknown>): { invitation: Invitation; delivery: Delivery | null } {
    const invitation = parseInvitation(raw.invitation);
    if (raw.delivery === undefined) return { invitation, delivery: null };
    const v = record(raw.delivery);
    if (!text(v.code, 256) || !v.code.startsWith('AIN1.') || !text(v.link, 2048) || !v.link.endsWith('#invitation=' + encodeURIComponent(v.code))) throw new ApiError('unavailable');
    return { invitation, delivery: { code: v.code, link: v.link } };
  }
  async issue(org: string, body: unknown, signal: AbortSignal): Promise<{ invitation: Invitation; delivery: Delivery | null }> { return this.#delivery(await this.api.privatePost(this.#path(org, 'invitations/issue'), body, signal)); }
  async invitationAction(org: string, id: string, action: string, body: unknown, signal: AbortSignal): Promise<{ invitation: Invitation; delivery: Delivery | null }> {
    if (!isUuid(id) || !['resend', 'revoke'].includes(action)) throw new ApiError('denied');
    return this.#delivery(await this.api.privatePost(this.#path(org, `invitations/${id}/${action}`), body, signal));
  }
  async admission(org: string, id: string, signal: AbortSignal): Promise<Admission> {
    if (!isUuid(id)) throw new ApiError('denied');
    return parseAdmission(await this.api.privatePost(this.#path(org, `invitations/${id}/acceptance/read`), {}, signal));
  }
  async admissionAction(org: string, id: string, action: string, body: unknown, signal: AbortSignal): Promise<Admission> {
    if (!isUuid(id) || !['confirm', 'reject'].includes(action)) throw new ApiError('denied');
    return parseAdmission(await this.api.privatePost(this.#path(org, `invitations/${id}/${action}`), body, signal));
  }
}
export type AccessPhase = 'closed' | 'loading' | 'ready' | 'review' | 'failed' | 'denied';
export interface AccessState { phase: AccessPhase; authority: Authority | null; employees: Employee[]; nextCursor: string; filter: string; employee: Employee | null; invitations: Invitation[]; invitationCursor: string; admission: Admission | null; duplicates: Employee[]; delivery: Delivery | null; message: string }
const initial = (): AccessState => ({ phase: 'closed', authority: null, employees: [], nextCursor: '', filter: '', employee: null, invitations: [], invitationCursor: '', admission: null, duplicates: [], delivery: null, message: '' });
export class EmployeeAccessController {
  state = initial();
  #epoch = 0;
  #pending = new AbortController();
  #retry: (() => Promise<void>) | null = null;
  #reviewed = '';
  constructor(private readonly source: AccessSource, private readonly render: (state: AccessState) => void, private readonly sessionLost?: (message: string) => void) {}
  get retryable(): boolean { return this.#retry !== null; }
  #show(patch: Partial<AccessState>): void { this.state = { ...this.state, ...patch }; this.render(this.state); }
  #current(epoch: number): boolean { return epoch === this.#epoch && !this.#pending.signal.aborted; }
  clear(): void { this.#epoch++; this.#pending.abort(); this.#pending = new AbortController(); this.#retry = null; this.#reviewed = ''; this.state = initial(); this.render(this.state); }
  #allowed(permission: string): boolean { return this.state.authority?.permissions?.includes(permission) === true; }
  #org(): string { const id = this.state.authority?.organization.id; if (!id) throw new ApiError('denied'); return id; }
  #fail(error: unknown, epoch: number): void {
    if (!this.#current(epoch) || error instanceof ApiError && error.kind === 'cancelled') return;
    if (error instanceof ApiError && ['denied', 'unauthenticated', 'fresh'].includes(error.kind)) {
      const fresh = error.kind === 'fresh' || error.kind === 'unauthenticated';
      this.clear();
      if (fresh && this.sessionLost) { this.sessionLost('Sua confirmação de acesso expirou. Entre novamente.'); return; }
      this.#show({ phase: 'denied', message: fresh ? 'Confirme seu acesso novamente para continuar. Saia e entre no painel.' : 'Seu acesso a estes registros não pôde ser confirmado. Atualize seus acessos.' }); return;
    }
    if (error instanceof ApiError && error.kind === 'review') { this.#show({ phase: 'review', delivery: null, message: 'Há um possível cadastro duplicado. Confira os registros antes de salvar.' }); return; }
    if (error instanceof ApiError && error.kind === 'conflict') { this.#retry = null; this.#show({ phase: 'failed', delivery: null, admission: null, invitations: [], invitationCursor: '', message: 'Este registro mudou. Atualize antes de escolher uma nova ação.' }); return; }
    this.#show({ phase: 'failed', delivery: null, message: error instanceof ApiError && error.kind === 'conflict' ? 'Este registro mudou. Atualize antes de escolher uma nova ação.' : error instanceof ApiError && error.kind === 'invalid' ? 'Confira os campos. A matrícula pode já estar cadastrada.' : 'Não foi possível confirmar a operação. Tente novamente para consultar a mesma operação.' });
  }
  async #run(work: (epoch: number, signal: AbortSignal) => Promise<void>, retry = true): Promise<void> {
    this.#pending.abort(); this.#pending = new AbortController();
    const epoch = ++this.#epoch;
    if (retry) this.#retry = () => this.#run(work);
    this.#show({ phase: 'loading', delivery: null, message: '' });
    try { await work(epoch, this.#pending.signal); } catch (error) { this.#fail(error, epoch); }
  }
  #unconfirmEmployee(id: string): void {
    this.#show({ employee: this.state.employee?.id === id ? { ...this.state.employee, accessState: 'UNKNOWN' } : this.state.employee,
      employees: this.state.employees.map(v => v.id === id ? { ...v, accessState: 'UNKNOWN' } : v) });
  }
  async #refreshEmployee(org: string, id: string, epoch: number, signal: AbortSignal): Promise<void> {
    if (!this.#current(epoch)) return;
    try {
      const employee = await this.source.employee(org, id, signal);
      if (employee.id !== id) throw new ApiError('denied');
      if (this.#current(epoch)) this.#show({ employee: this.state.employee?.id === id ? employee : this.state.employee, employees: this.state.employees.map(v => v.id === id ? employee : v) });
    } catch (error) {
      if (!this.#current(epoch)) return;
      if (error instanceof ApiError && ['denied', 'unauthenticated', 'fresh'].includes(error.kind)) { this.#fail(error, epoch); return; }
      this.#show({ message: this.state.message + ' A situação atual do funcionário não pôde ser consultada. Atualize os registros.' });
    }
  }
  async open(authority: Authority): Promise<void> { this.clear(); this.#show({ authority }); await this.list(); }
  async list(filter = this.state.filter, cursor = ''): Promise<void> {
    if (!this.#allowed('idn.employee.read')) { this.#fail(new ApiError('denied'), this.#epoch); return; }
    const org = this.#org();
    await this.#run(async (epoch, signal) => {
      const page = await this.source.employees(org, filter, cursor, signal);
      if (this.#current(epoch)) this.#show({ phase: 'ready', employees: page.items, nextCursor: page.nextCursor, filter, employee: null, invitations: [], invitationCursor: '', admission: null, duplicates: [] });
    });
  }
  async openEmployee(id: string): Promise<void> {
    const org = this.#org();
    await this.#run(async (epoch, signal) => {
      const employee = await this.source.employee(org, id, signal);
      if (employee.id !== id) throw new ApiError('denied');
      if (!this.#current(epoch)) return;
      this.#show({ employee, invitations: [], invitationCursor: '', admission: null });
      if (this.#allowed('idn.invitation.manage')) {
        const page = await this.source.invitations(org, id, '', signal);
        if (!this.#current(epoch)) return;
        this.#show({ invitations: page.items, invitationCursor: page.nextCursor });
      }
      this.#show({ phase: 'ready' });
    });
  }
  async invitationPage(cursor: string): Promise<void> {
    const org = this.#org(), employee = this.state.employee;
    if (!employee || !this.#allowed('idn.invitation.manage')) return;
    await this.#run(async (epoch, signal) => { const page = await this.source.invitations(org, employee.id, cursor, signal); if (this.#current(epoch)) this.#show({ phase: 'ready', invitations: page.items, invitationCursor: page.nextCursor, admission: null }); });
  }
  async review(profile: EmployeeProfile): Promise<void> {
    if (!this.#allowed('idn.employee.manage')) { this.#fail(new ApiError('denied'), this.#epoch); return; }
    const org = this.#org();
    await this.#run(async (epoch, signal) => {
      const page = await this.source.matches(org, profile, signal);
      if (!this.#current(epoch)) return;
      this.#reviewed = JSON.stringify(profile);
      this.#show({ phase: page.items.length ? 'review' : 'ready', duplicates: page.items, message: page.items.length ? 'Compare os cadastros abaixo antes de confirmar um novo funcionário.' : 'Nenhum possível duplicado encontrado. Confirme para salvar.' });
    });
  }
  async create(profile: EmployeeProfile, allowDuplicate: boolean): Promise<void> {
    if (!this.#allowed('idn.employee.manage')) { this.#fail(new ApiError('denied'), this.#epoch); return; }
    if (this.state.duplicates.length && (!allowDuplicate || this.#reviewed !== JSON.stringify(profile))) { this.#show({ phase: 'review', message: 'Revise os possíveis duplicados e confirme antes de salvar.' }); return; }
    const org = this.#org();
    const body = { schema_version: 1, operation_id: crypto.randomUUID(), ...profile, allow_possible_duplicate: allowDuplicate && this.#reviewed === JSON.stringify(profile) };
    await this.#run(async (epoch, signal) => {
      const result = await this.source.create(org, body, signal);
      if (!isUuid(result.employee_id)) throw new ApiError('unavailable');
      if (!this.#current(epoch)) return;
      this.#show({ duplicates: [], message: 'Funcionário salvo no servidor.' });
      await this.openEmployee(result.employee_id);
    });
  }
  async issue(grants: InvitationGrant[]): Promise<void> {
    const employee = this.state.employee;
    if (!employee || !this.#allowed('idn.invitation.manage') || grants.length < 1 || grants.length > 10 || grants.some(g => !this.state.authority?.invitationGrants?.some(option => option.role === g.role && option.scope === g.scope))) { this.#fail(new ApiError('denied'), this.#epoch); return; }
    const org = this.#org(), body = { schema_version: 1, operation_id: crypto.randomUUID(), employee_id: employee.id, grants: grants.map(g => ({ ...g })) };
    await this.#run(async (epoch, signal) => {
      const result = await this.source.issue(org, body, signal);
      if (result.invitation.employeeId !== employee.id) throw new ApiError('denied');
      if (!this.#current(epoch)) return;
      this.#retry = null;
      this.#show({ phase: 'ready', invitations: [result.invitation, ...this.state.invitations.filter(i => i.id !== result.invitation.id)], delivery: result.delivery, admission: null, message: result.delivery ? 'Convite emitido. Guarde o código agora: ele não aparecerá no histórico.' : 'Operação confirmada. O código não pode ser exibido novamente; use Reemitir se precisar de um novo.' });
      this.#unconfirmEmployee(employee.id);
      await this.#refreshEmployee(org, employee.id, epoch, signal);
    });
  }
  async invitationAction(id: string, action: 'resend' | 'revoke'): Promise<void> {
    const invitation = this.state.invitations.find(i => i.id === id);
    if (!invitation || !this.#allowed('idn.invitation.manage')) return;
    const org = this.#org(), body = { schema_version: 1, operation_id: crypto.randomUUID(), revision: invitation.revision };
    await this.#run(async (epoch, signal) => {
      const result = await this.source.invitationAction(org, id, action, body, signal);
      if (result.invitation.employeeId !== invitation.employeeId) throw new ApiError('denied');
      if (!this.#current(epoch)) return;
      this.#retry = null; // The write is acknowledged; history failure never replays it.
      const message = action === 'resend' ? result.delivery ? 'Reemissão confirmada. O código anterior deixou de funcionar.' : 'Reemissão confirmada. O código não pode ser exibido novamente; use Reemitir se precisar de um novo.' : 'Revogação confirmada no servidor.';
      // Remove the old pending entry while its authoritative SUPERSEDED metadata
      // is refreshed. First-only delivery must survive a failed history read.
      this.#show({ phase: 'ready', invitations: [result.invitation, ...this.state.invitations.filter(i => i.id !== id && i.id !== result.invitation.id)], delivery: result.delivery, admission: null, message });
      this.#unconfirmEmployee(invitation.employeeId);
      try {
        const history = await this.source.invitations(org, invitation.employeeId, '', signal);
        if (this.#current(epoch)) this.#show({ invitations: [result.invitation, ...history.items.filter(i => i.id !== result.invitation.id)], invitationCursor: history.nextCursor });
      } catch (error) {
        if (this.#current(epoch)) {
          if (error instanceof ApiError && ['denied', 'unauthenticated', 'fresh'].includes(error.kind)) { this.#fail(error, epoch); return; }
          this.#show({ message: message + ' O histórico não pôde ser atualizado. Guarde o convite, se exibido, e atualize os registros depois.' });
        }
      }
      await this.#refreshEmployee(org, invitation.employeeId, epoch, signal);
    });
  }
  async readAdmission(id: string): Promise<void> {
    if (!this.#allowed('idn.invitation.manage')) return;
    const org = this.#org(), invitation = this.state.invitations.find(i => i.id === id);
    if (!invitation) return;
    await this.#run(async (epoch, signal) => { const admission = await this.source.admission(org, id, signal); if (admission.invitationId !== id || admission.membershipId !== invitation.membershipId) throw new ApiError('denied'); if (this.#current(epoch)) this.#show({ phase: 'ready', admission }); });
  }
  async admissionAction(action: 'confirm' | 'reject'): Promise<void> {
    const admission = this.state.admission;
    if (!admission || admission.state !== 'WAITING' || !this.#allowed('idn.invitation.manage')) return;
    const org = this.#org(), body = { schema_version: 1, operation_id: crypto.randomUUID(), revision: admission.revision };
    const employeeId = this.state.employee?.id;
    await this.#run(async (epoch, signal) => {
      const result = await this.source.admissionAction(org, admission.invitationId, action, body, signal);
      if (result.invitationId !== admission.invitationId || result.membershipId !== admission.membershipId) throw new ApiError('denied');
      if (!this.#current(epoch)) return;
      this.#retry = null;
      this.#show({ phase: 'ready', admission: result, message: action === 'confirm' ? 'Confirmação registrada pelo servidor.' : 'Recusa registrada pelo servidor.' });
      if (employeeId) { this.#unconfirmEmployee(employeeId); await this.#refreshEmployee(org, employeeId, epoch, signal); }
    });
  }
  closeDelivery(): void { this.#show({ delivery: null, message: '' }); }
  async retry(): Promise<void> { await this.#retry?.(); }
}
