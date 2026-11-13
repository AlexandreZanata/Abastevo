// Company UI values carry account and membership identity explicitly.
export interface Organization { id: string; name: string; membershipId: string }
export interface MembershipSnapshot { accountId: string; organizations: Organization[] }
export interface InvitationGrant { role: string; scope: 'ORG' | 'SELF' }
export interface Authority { accountId: string; organization: Organization; permissions?: string[]; invitationGrants?: InvitationGrant[]; invitationValidityDays?: number }
export type Route = { kind: 'login' | 'overview' | 'not-found' } | { kind: 'organization'; id: string };
export const HOME = '/empresas/';
export const LOGIN = '/empresas/entrar/';
export function isUuid(value: unknown): value is string {
  return typeof value === 'string' && /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/.test(value) && value !== '00000000-0000-0000-0000-000000000000';
}
export function route(path: string, search = ''): Route {
  if (search !== '') return { kind: 'not-found' };
  if (path === HOME) return { kind: 'overview' };
  if (path === LOGIN) return { kind: 'login' };
  const match = /^\/empresas\/organizacoes\/([^/]+)\/$/.exec(path);
  return match && isUuid(match[1]) ? { kind: 'organization', id: match[1] } : { kind: 'not-found' };
}
export function organizationPath(id: string): string {
  if (!isUuid(id)) throw new Error('Invalid organization reference');
  return `/empresas/organizacoes/${id}/`;
}
export function record(value: unknown): Record<string, unknown> {
  return value !== null && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : {};
}
export function validOrganization(value: unknown): value is Organization {
  const v = record(value);
  return isUuid(v.id) && isUuid(v.membershipId) && typeof v.name === 'string' && v.name.trim().length > 0 && v.name.length <= 200;
}
export function validSnapshot(value: unknown, accountId: string): value is MembershipSnapshot {
  const v = record(value);
  return v.accountId === accountId && isUuid(accountId) && Array.isArray(v.organizations) &&
    v.organizations.every(validOrganization) && new Set(v.organizations.map(o => o.id)).size === v.organizations.length &&
    new Set(v.organizations.map(o => o.membershipId)).size === v.organizations.length;
}
