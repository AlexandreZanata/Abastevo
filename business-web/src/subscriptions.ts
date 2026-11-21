import { ApiError, type CompanyTransport } from './api.js';
import { isUuid, record, type Authority } from './model.js';

const STATES = {
  TRIALING: 'Em avaliação', ACTIVE: 'Ativo', PAST_DUE: 'Pagamento em atraso', SUSPENDED: 'Suspenso',
  CANCELLING: 'Cancelamento em andamento', CANCELLED: 'Cancelado',
} as const;

export const METERS = {
  active_vehicles: 'Veículos ativos', active_stations: 'Postos ativos',
  storage_bytes: 'Armazenamento (bytes)', api_calls: 'Chamadas à API',
} as const;

type State = keyof typeof STATES;
type Meter = keyof typeof METERS;

export interface PlanSummary {
  name: string;
  product: string;
  revision: string;
  state: State;
  termStart: string;
  termEnd: string;
  termCurrent: boolean;
  confirmedPaid: boolean;
  receipt?: { amountCentavos: string; settledAt: string; fiscalStatus: 'NOT_CONSULTED'; paymentDate?: string };
  capabilities: string[];
  limits: Partial<Record<Meter, string>>;
  storageUsedBytes?: string;
  pending?: { name: string; revision: string; effectiveAt: string };
}

export interface PlanSource {
  read(organization: string, signal: AbortSignal): Promise<PlanSummary[]>;
}

function text(value: unknown): string {
  if (typeof value !== 'string' || value.trim() === '') throw new ApiError('unavailable');
  return value;
}

function integer(value: unknown, minimum = 0n): string {
  const raw = text(value);
  if (!/^(0|[1-9][0-9]{0,18})$/.test(raw) || BigInt(raw) < minimum || BigInt(raw) > 9223372036854775807n) throw new ApiError('unavailable');
  return raw;
}

function instant(value: unknown): string {
  const raw = text(value);
  if (!Number.isFinite(Date.parse(raw))) throw new ApiError('unavailable');
  return raw;
}

function plan(value: unknown): PlanSummary {
  const raw = record(value);
  if (typeof raw.state !== 'string' || !Object.hasOwn(STATES, raw.state) || typeof raw.term_current !== 'boolean' ||
      typeof raw.confirmed_paid !== 'boolean' || !Array.isArray(raw.capabilities) || raw.limits === null ||
      typeof raw.limits !== 'object' || Array.isArray(raw.limits)) throw new ApiError('unavailable');
  const termStart = instant(raw.term_start);
  const termEnd = instant(raw.term_end);
  if (Date.parse(termStart) >= Date.parse(termEnd)) throw new ApiError('unavailable');
  const limits: PlanSummary['limits'] = {};
  for (const [meter, limit] of Object.entries(record(raw.limits))) {
    if (!Object.hasOwn(METERS, meter)) throw new ApiError('unavailable');
    limits[meter as Meter] = integer(limit);
  }
  const result: PlanSummary = {
    name: text(raw.name), product: text(raw.product), revision: integer(raw.revision, 1n),
    state: raw.state as State, termStart, termEnd, termCurrent: raw.term_current,
    confirmedPaid: raw.confirmed_paid, capabilities: raw.capabilities.map(text), limits,
  };
  if (raw.storage_used_bytes !== undefined) {
    if (limits.storage_bytes === undefined) throw new ApiError('unavailable');
    result.storageUsedBytes = integer(raw.storage_used_bytes);
  }
  if (raw.receipt !== undefined) {
    const receipt = record(raw.receipt);
    if (!result.confirmedPaid || receipt.currency !== 'BRL' || receipt.fiscal_status !== 'NOT_CONSULTED') throw new ApiError('unavailable');
    result.receipt = { amountCentavos: integer(receipt.amount_centavos, 1n), settledAt: instant(receipt.settled_at),
      fiscalStatus: receipt.fiscal_status };
    if (receipt.payment_date !== undefined) {
      const day = text(receipt.payment_date);
      if (!/^\d{4}-\d{2}-\d{2}$/.test(day) || !Number.isFinite(Date.parse(day))) throw new ApiError('unavailable');
      result.receipt.paymentDate = day;
    }
  }
  if (raw.pending !== undefined) {
    const pending = record(raw.pending);
    const effectiveAt = instant(pending.effective_at);
    if (Date.parse(effectiveAt) !== Date.parse(termEnd)) throw new ApiError('unavailable');
    result.pending = { name: text(pending.name), revision: integer(pending.revision, 1n), effectiveAt };
  }
  return result;
}

export class PlanOverviewApi implements PlanSource {
  constructor(private readonly api: CompanyTransport) {}
  async read(organization: string, signal: AbortSignal): Promise<PlanSummary[]> {
    if (!isUuid(organization)) throw new ApiError('denied');
    const view = await this.api.privatePost(`/v1/orgs/${organization}/subscriptions/plan/read`, {}, signal);
    instant(view.observed_at);
    if (!Array.isArray(view.plans)) throw new ApiError('unavailable');
    return view.plans.map(plan);
  }
}

export interface PlanPanelState {
  phase: 'closed' | 'loading' | 'ready' | 'unavailable';
  plans: PlanSummary[];
  message: string;
}

export class PlanPanelController {
  #authority: Authority | undefined;
  #reading: AbortController | undefined;
  state: PlanPanelState = { phase: 'closed', plans: [], message: '' };

  constructor(private readonly source: PlanSource, private readonly render: (state: PlanPanelState) => void,
    private readonly sessionLost: (message: string) => void) {}

  select(authority?: Authority): void {
    if (!authority?.permissions?.includes('subs.plan.read')) { this.clear(); return; }
    this.#authority = authority;
    void this.refresh();
  }

  clear(): void {
    this.#reading?.abort();
    this.#reading = undefined;
    this.#authority = undefined;
    this.#show({ phase: 'closed', plans: [], message: '' });
  }

  async refresh(): Promise<void> {
    const authority = this.#authority;
    if (!authority) return;
    this.#reading?.abort();
    const read = new AbortController();
    this.#reading = read;
    this.#show({ phase: 'loading', plans: [], message: 'Consultando o plano da empresa…' });
    try {
      const plans = await this.source.read(authority.organization.id, read.signal);
      if (this.#reading !== read) return;
      this.#show({ phase: 'ready', plans, message: plans.length ? '' : 'Nenhum plano empresarial registrado.' });
    } catch (error) {
      if (this.#reading !== read) return;
      if (error instanceof ApiError && error.kind === 'unauthenticated') {
        this.clear();
        this.sessionLost('Sua sessão expirou. Entre novamente.');
        return;
      }
      if (error instanceof ApiError && error.kind === 'denied') {
        this.clear();
        return;
      }
      this.#show({ phase: 'unavailable', plans: [], message: 'Não foi possível consultar o plano. Tente atualizar.' });
    }
  }

  #show(state: PlanPanelState): void {
    this.state = state;
    this.render(state);
  }
}

export function planStateLabel(plan: PlanSummary): string {
  if (!plan.termCurrent && plan.state === 'ACTIVE') return 'Vigência encerrada';
  if (!plan.termCurrent && plan.state === 'TRIALING') return 'Período de avaliação encerrado';
  return STATES[plan.state];
}
