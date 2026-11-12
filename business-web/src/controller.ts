import { ApiError, type SessionPort } from './api.js';
import { type Authority, type MembershipSnapshot, type Organization, type Route, validOrganization, validSnapshot } from './model.js';

// AUTH/IDN will supply current server-backed memberships. This port invents
// no endpoint and infers no permission from browser state.
export interface MembershipSource {
  list(accountId: string, signal: AbortSignal): Promise<MembershipSnapshot>;
  authorize(accountId: string, orgId: string, signal: AbortSignal): Promise<Authority>;
}
export const unavailableMemberships: MembershipSource = {
  async list() { throw new ApiError('unavailable'); },
  async authorize() { throw new ApiError('unavailable'); },
};
export type Phase = 'signed-out' | 'loading' | 'choose' | 'empty' | 'ready' | 'denied' | 'unavailable' | 'signing-out' | 'not-found';
export interface State { phase: Phase; organizations: Organization[]; selected: Organization | null; message: string }
const sameOrganization = (a: Organization | null, b: Organization | null): boolean =>
  a === b || (a !== null && b !== null && a.id === b.id && a.name === b.name && a.membershipId === b.membershipId);
const sameState = (a: State, b: State): boolean => a.phase === b.phase && a.message === b.message &&
  sameOrganization(a.selected, b.selected) && a.organizations.length === b.organizations.length &&
  a.organizations.every((o, i) => sameOrganization(o, b.organizations[i] ?? null));
export class CompanyController {
  #epoch = 0;
  #pending = new AbortController();
  #quiet = -1;
  state: State = { phase: 'signed-out', organizations: [], selected: null, message: '' };
  constructor(private readonly session: SessionPort, private readonly source: MembershipSource, private readonly render: (state: State, unchanged: boolean) => void) {}
  #show(phase: Phase, organizations: Organization[] = [], selected: Organization | null = null, message = ''): void {
    const next = { phase, organizations, selected, message };
    // A background recheck that lands where it started changes nothing visible.
    const unchanged = this.#quiet === this.#epoch && sameState(this.state, next);
    this.state = next;
    this.render(next, unchanged);
  }
  #begin(phase: Phase | null): { epoch: number; signal: AbortSignal } {
    this.#pending.abort();
    this.#pending = new AbortController();
    this.#epoch++;
    if (phase === null) this.#quiet = this.#epoch;
    else this.#show(phase);
    return { epoch: this.#epoch, signal: this.#pending.signal };
  }
  reset(message = ''): void { this.#begin('signed-out'); this.session.forget(); this.#show('signed-out', [], null, message); }
  #failure(error: unknown, epoch: number): void {
    if (epoch !== this.#epoch || (error instanceof ApiError && error.kind === 'cancelled')) return;
    if (!this.session.authenticated || (error instanceof ApiError && (error.kind === 'unauthenticated' || error.kind === 'fresh'))) {
      this.session.forget();
      this.#show('signed-out', [], null, 'Sua sessão expirou. Entre novamente.');
    } else if (error instanceof ApiError && error.kind === 'denied') this.#show('denied');
    else this.#show('unavailable');
  }
  async #load(route: Route, epoch: number, signal: AbortSignal): Promise<void> {
    try {
      const session = await this.session.read(signal);
      if (epoch !== this.#epoch) return;
      const snapshot = await this.source.list(session.accountId, signal);
      if (epoch !== this.#epoch) return;
      if (!validSnapshot(snapshot, session.accountId)) throw new ApiError('denied');
      const organizations = snapshot.organizations.map(o => ({ ...o }));
      if (route.kind !== 'organization') {
        this.#show(organizations.length === 0 ? 'empty' : 'choose', organizations);
        return;
      }
      const requested = organizations.find(o => o.id === route.id);
      if (!requested) throw new ApiError('denied');
      const authority = await this.source.authorize(session.accountId, requested.id, signal);
      if (epoch !== this.#epoch) return;
      if (authority.accountId !== session.accountId || !validOrganization(authority.organization) ||
          authority.organization.id !== requested.id || authority.organization.membershipId !== requested.membershipId) throw new ApiError('denied');
      this.#show('ready', organizations, { ...authority.organization });
    } catch (error) { this.#failure(error, epoch); }
  }
  async navigate(route: Route): Promise<void> {
    if (route.kind === 'not-found') { this.#begin('not-found'); return; }
    if (route.kind === 'login' || !this.session.authenticated) { this.reset(); return; }
    const { epoch, signal } = this.#begin('loading');
    await this.#load(route, epoch, signal);
  }
  // Tab return rechecks the current scope without a loading state: the view
  // stays until the server answers, and any refusal still clears it.
  async revalidate(route: Route): Promise<void> {
    if (route.kind === 'login' || !this.session.authenticated) { this.reset(); return; }
    const { epoch, signal } = this.#begin(null);
    if (route.kind === 'not-found') { this.#show('not-found'); return; }
    await this.#load(route, epoch, signal);
  }
  async signIn(key: string, route: Route): Promise<void> {
    const { epoch, signal } = this.#begin('loading');
    try {
      await this.session.signIn(key, signal);
      if (epoch !== this.#epoch) return;
      await this.#load(route.kind === 'login' ? { kind: 'overview' } : route, epoch, signal);
    } catch (error) {
      if (epoch !== this.#epoch) return;
      this.session.forget();
      const message = error instanceof ApiError && error.kind === 'unauthenticated' ? 'Chave inválida. Confira e tente novamente.' :
        error instanceof ApiError && error.kind === 'fresh' ? 'Sua confirmação de acesso expirou. Entre novamente.' :
        error instanceof ApiError && error.kind === 'limited' ? 'Muitas tentativas. Aguarde antes de tentar novamente.' : 'Não foi possível entrar. Tente novamente.';
      this.#show('signed-out', [], null, message);
    }
  }
  // Navigation must not cancel the server logout, so it gets no page signal.
  async signOut(): Promise<void> {
    const { epoch } = this.#begin('signing-out');
    let message = '';
    try { await this.session.signOut(); }
    catch { message = 'A saída no servidor não pôde ser confirmada. Entre novamente para continuar.'; }
    if (epoch !== this.#epoch) return;
    this.session.forget();
    this.#show('signed-out', [], null, message);
  }
}
