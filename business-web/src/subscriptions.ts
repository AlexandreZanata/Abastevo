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

export interface VehiclePlanBasis {
  subscription: string;
  revision: number;
  active: number;
  ceiling: number;
}

interface PlanVehicle {
  id: string;
  name: string;
}

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
  vehicleSelection?: VehiclePlanBasis;
  pending?: { name: string; revision: string; effectiveAt: string };
}

export interface PlanSource {
  read(organization: string, signal: AbortSignal): Promise<PlanSummary[]>;
  vehicleOptions?(organization: string, basis: VehiclePlanBasis, signal: AbortSignal): Promise<PlanVehicle[]>;
  commitVehicles?(organization: string, basis: VehiclePlanBasis, operation: string, keep: string[], signal: AbortSignal): Promise<void>;

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
  if (raw.vehicle_selection !== undefined) {
    const v = record(raw.vehicle_selection);
    if (!isUuid(v.subscription_id) || !Number.isSafeInteger(v.revision) || Number(v.revision) < 2 ||
        !Number.isSafeInteger(v.ceiling) || Number(v.ceiling) < 0 || !Number.isSafeInteger(v.active) ||
        Number(v.active) <= Number(v.ceiling) || String(v.ceiling) !== limits.active_vehicles ||
        result.state !== 'ACTIVE' || !result.termCurrent) throw new ApiError('unavailable');
    result.vehicleSelection = { subscription: v.subscription_id, revision: Number(v.revision), active: Number(v.active), ceiling: Number(v.ceiling) };
  }
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

  async vehicleOptions(organization: string, basis: VehiclePlanBasis, signal: AbortSignal): Promise<PlanVehicle[]> {
    if (!isUuid(organization)) throw new ApiError('denied');
    const options = new Map<string, PlanVehicle>();
    const cursors = new Set<string>();
    let cursor = '';
    do {
      if (cursors.has(cursor)) throw new ApiError('unavailable');
      cursors.add(cursor);
      const result = await this.api.privatePost(`/v1/orgs/${organization}/fleet/assets/list`,
        { limit: 100, state: 'ACTIVE', kind: 'VEHICLE', ...(cursor ? { cursor } : {}) }, signal);
      if (!Array.isArray(result.items) || result.items.length > 100 || typeof result.next_cursor !== 'string' ||
          result.next_cursor.length > 8192) throw new ApiError('unavailable');
      for (const item of result.items) {
        const asset = record(item);
        if (!isUuid(asset.id) || asset.kind !== 'VEHICLE' || asset.state !== 'ACTIVE' || options.has(asset.id)) {
          throw new ApiError('unavailable');
        }
        const label = [text(asset.name), asset.fleet_number, asset.plate]
          .filter(part => typeof part === 'string' && part.trim() !== '').join(' · ');
        options.set(asset.id, { id: asset.id, name: label });
      }
      cursor = result.next_cursor;
      if (options.size > basis.active || (cursor !== '' && result.items.length === 0)) throw new ApiError('conflict');
    } while (cursor !== '');
    if (options.size !== basis.active) throw new ApiError('conflict');
    return [...options.values()];
  }

  async commitVehicles(organization: string, basis: VehiclePlanBasis, operation: string, keep: string[], signal: AbortSignal): Promise<void> {
    if (!isUuid(organization) || !isUuid(operation)) throw new ApiError('denied');
    const result = await this.api.privatePost(`/v1/orgs/${organization}/subscriptions/vehicles/select`,
      { schema_version: 1, operation_id: operation, subscription_id: basis.subscription, revision: basis.revision, keep_active: keep }, signal);
    if (!Array.isArray(result.kept) || result.kept.length !== keep.length || !Array.isArray(result.archived) ||
        result.archived.length !== basis.active - keep.length || typeof result.replayed !== 'boolean' ||
        new Set([...result.kept, ...result.archived]).size !== basis.active ||
        !result.kept.every(value => isUuid(value) && keep.includes(value)) || !result.archived.every(isUuid)) {
      throw new ApiError('unavailable');
    }
  }
}

interface VehiclePlanEdit {
  basis: VehiclePlanBasis;
  options: PlanVehicle[];
  busy: boolean;
  keep: string[];
  message: string;
}

export interface PlanPanelState {
  phase: 'closed' | 'loading' | 'ready' | 'unavailable';
  plans: PlanSummary[];
  message: string;
  canChooseVehicles?: boolean;
  vehicleEdit?: VehiclePlanEdit;
}

export class PlanPanelController {
  #authority: Authority | undefined;
  #reading: AbortController | undefined;
  #vehicleRequest: AbortController | undefined;
  #vehicleAttempt: { key: string; operation: string } | undefined;

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
    this.#clearVehicleEdit();
    this.#reading = undefined;
    this.#authority = undefined;
    this.#show({ phase: 'closed', plans: [], message: '' });
  }

  async refresh(message = ''): Promise<void> {
    const authority = this.#authority;
    if (!authority) return;
    this.#reading?.abort();
    this.#clearVehicleEdit();
    const read = new AbortController();
    this.#reading = read;
    this.#show({ phase: 'loading', plans: [], message: 'Consultando o plano da empresa…' });
    try {
      const plans = await this.source.read(authority.organization.id, read.signal);
      if (this.#reading !== read) return;
      const canChooseVehicles = authority.permissions?.includes('subs.plan.manage') && authority.permissions.includes('fleet.asset.read');
      this.#show({ phase: 'ready', plans, message: plans.length ? message : 'Nenhum plano empresarial registrado.',
        ...(canChooseVehicles ? { canChooseVehicles } : {}) });
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

  async chooseVehicles(basis: VehiclePlanBasis): Promise<void> {
    const authority = this.#authority;
    if (!authority || !this.state.canChooseVehicles || !this.source.vehicleOptions || !this.source.commitVehicles ||
        !this.state.plans.some(plan => plan.vehicleSelection === basis)) return;
    this.#clearVehicleEdit();
    const request = new AbortController();
    this.#vehicleRequest = request;
    const edit: VehiclePlanEdit = { basis, options: [], busy: true, keep: [], message: 'Carregando os veículos ativos…' };
    this.#show({ ...this.state, vehicleEdit: edit });
    try {
      const options = await this.source.vehicleOptions(authority.organization.id, basis, request.signal);
      if (this.#vehicleRequest !== request) return;
      this.#show({ ...this.state, vehicleEdit: { ...edit, options, busy: false, message: '' } });
    } catch (error) {
      if (this.#vehicleRequest !== request || this.#selectionSessionLost(error)) return;
      this.#show({ ...this.state, vehicleEdit: { ...edit, busy: false, message: 'Não foi possível carregar a frota. Atualize o plano e tente novamente.' } });
    }
  }

  cancelVehicleSelection(): void {
    this.#clearVehicleEdit();
    this.#show({ ...this.state, vehicleEdit: undefined });
  }

  async confirmVehicles(keep: string[]): Promise<void> {
    const edit = this.state.vehicleEdit, authority = this.#authority;
    if (!edit || edit.busy || !authority || !this.state.canChooseVehicles || !this.source.commitVehicles ||
        keep.length !== edit.basis.ceiling || new Set(keep).size !== keep.length ||
        !keep.every(id => edit.options.some(asset => asset.id === id))) return;
    const chosen = [...keep].sort();
    const key = chosen.join(',');
    const operation = this.#vehicleAttempt?.key === key ? this.#vehicleAttempt.operation : crypto.randomUUID();
    this.#vehicleAttempt = { key, operation };
    const request = new AbortController();
    this.#vehicleRequest = request;
    this.#show({ ...this.state, vehicleEdit: { ...edit, keep: chosen, busy: true, message: 'Confirmando os veículos ativos…' } });
    try {
      await this.source.commitVehicles(authority.organization.id, edit.basis, operation, chosen, request.signal);
      if (this.#vehicleRequest === request) await this.refresh('Seleção confirmada. O histórico da frota foi preservado.');
    } catch (error) {
      if (this.#vehicleRequest !== request || this.#selectionSessionLost(error)) return;
      const message = error instanceof ApiError && error.code === 'ASSET_HAS_OPEN_OPERATIONS'
        ? 'Encerre as operações em aberto dos veículos que sairão do uso ativo.'
        : error instanceof ApiError && error.kind === 'conflict'
          ? 'A frota ou o plano mudou. Atualize o plano antes de confirmar.'
          : 'Não foi possível confirmar. Revise a seleção e tente novamente.';
      this.#show({ ...this.state, vehicleEdit: { ...edit, keep: chosen, busy: false, message } });
    }
  }

  #selectionSessionLost(error: unknown): boolean {
    if (!(error instanceof ApiError) || !['unauthenticated', 'fresh'].includes(error.kind)) return false;
    this.clear();
    this.sessionLost('Entre novamente para confirmar a seleção de veículos.');
    return true;
  }

  #clearVehicleEdit(): void {
    this.#vehicleRequest?.abort();
    this.#vehicleRequest = undefined;
    this.#vehicleAttempt = undefined;
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
