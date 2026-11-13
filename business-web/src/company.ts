import { ApiError, type CompanyTransport } from './api.js';
import { type MembershipSource } from './controller.js';
import { type Authority, type InvitationGrant, type MembershipSnapshot, type Organization, isUuid, record, validOrganization } from './model.js';

export const ROLES: Record<string, string> = {
  OWNER: 'Proprietário — administração e responsabilidade pela empresa',
  ADMIN: 'Administrador — configuração e gestão de acessos',
  FLEET_MANAGER: 'Gestor de frota — gestão da operação e veículos',
  SUPERVISOR: 'Supervisor — acompanhamento da equipe',
  FINANCE: 'Financeiro — despesas e finanças',
  APPROVER: 'Aprovador — análise de solicitações',
  DRIVER: 'Motorista — registros da própria operação',
  OPERATIONS_STAFF: 'Operação — apoio aos registros operacionais',
  AUDITOR: 'Auditor — consulta e auditoria',
  DATA_MANAGER: 'Gestor de dados — tratamento dos dados da empresa',
};
export function validGrant(value: unknown): value is InvitationGrant {
  const v = record(value);
  return typeof v.role === 'string' && Object.hasOwn(ROLES, v.role) && (v.scope === 'ORG' || v.scope === 'SELF') && (v.role !== 'OWNER' || v.scope === 'ORG');
}
function organization(raw: unknown): Organization {
  const v = record(raw);
  const result = { id: v.id, membershipId: v.membership_id, name: v.name };
  if (!validOrganization(result)) throw new ApiError('unavailable');
  return result;
}
export class CurrentMemberships implements MembershipSource {
  #memberships = new Map<string, string>();
  #account = '';
  constructor(private readonly api: CompanyTransport) {}
  clear(): void { this.#memberships.clear(); this.#account = ''; }
  async list(accountId: string, signal: AbortSignal): Promise<MembershipSnapshot> {
    this.#memberships.clear(); this.#account = '';
    const organizations: Organization[] = [];
    let after = '';
    // A corrupt/cyclic/oversized directory must fail closed, never retain a partial list.
    for (let page = 0; page < 100; page++) {
      const raw = await this.api.privatePost('/v1/org-access/contexts/list', { limit: 100, ...(after ? { after_org_id: after } : {}) }, signal);
      if (raw.account_id !== accountId || !Array.isArray(raw.organizations) || raw.organizations.length > 100 || typeof raw.next_after_org_id !== 'string') throw new ApiError('denied');
      for (const item of raw.organizations) {
        const value = organization(item);
        if (organizations.some(o => o.id === value.id || o.membershipId === value.membershipId) || value.id <= after || organizations.length > 0 && value.id <= organizations.at(-1)!.id) throw new ApiError('denied');
        organizations.push(value);
      }
      const next = raw.next_after_org_id;
      if (next === '') {
        if (signal.aborted) throw new ApiError('cancelled');
        this.#account = accountId;
        this.#memberships = new Map(organizations.map(o => [o.id, o.membershipId]));
        return { accountId, organizations };
      }
      if (!isUuid(next) || next <= after || raw.organizations.some(o => String(record(o).id) > next)) throw new ApiError('denied');
      after = next;
    }
    throw new ApiError('unavailable');
  }
  async authorize(accountId: string, orgId: string, signal: AbortSignal): Promise<Authority> {
    const membershipId = this.#memberships.get(orgId);
    if (accountId !== this.#account || !membershipId) throw new ApiError('denied');
    const raw = await this.api.privatePost('/v1/org-access/contexts/read', { organization_id: orgId, membership_id: membershipId }, signal);
    const selected = organization(raw.organization);
    if (raw.account_id !== accountId || selected.id !== orgId || selected.membershipId !== membershipId ||
        !Array.isArray(raw.permissions) || raw.permissions.length > 500 || !raw.permissions.every(p => typeof p === 'string' && /^[a-z][a-z0-9_.]{1,99}$/.test(p)) ||
        !Array.isArray(raw.invitation_grants) || raw.invitation_grants.length > 20 || !raw.invitation_grants.every(validGrant) ||
        !Number.isInteger(raw.invitation_validity_days) || Number(raw.invitation_validity_days) < 0 || Number(raw.invitation_validity_days) > 365) throw new ApiError('denied');
    return { accountId, organization: selected, permissions: [...raw.permissions] as string[], invitationGrants: raw.invitation_grants.map(g => ({ ...g })), invitationValidityDays: Number(raw.invitation_validity_days) };
  }
}
