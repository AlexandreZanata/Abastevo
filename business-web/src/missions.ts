import { ApiError, type CompanyTransport } from './api.js';
import { type Authority, isUuid, record } from './model.js';

// EMP-TRIP-022 mission planning and assignment board (B-BR-EMP-470..474):
// the read/command port over the existing company routes, strict parsers and
// a controller that shows only server-read state.
export const MISSION_STATES = ['DRAFT', 'PLANNED', 'ASSIGNED', 'ACCEPTED', 'DECLINED', 'IN_PROGRESS', 'COMPLETED', 'CANCELLED'] as const;
export type MissionState = typeof MISSION_STATES[number];
export type LoadKind = 'NONE' | 'CARGO' | 'PASSENGERS';
export interface MissionPlan { revision: number; destinationText: string; destinationUnitId: string; reason: string; costCentreId: string; load: LoadKind; cargo: string; passengers: number; plannedAt: string }
export interface Mission { id: string; unitId: string; assetId: string; title: string; startsAt: string; endsAt: string; state: MissionState; revision: number; assigneeId: string; answer: '' | 'ACCEPT' | 'DECLINE'; answerReason: string; plan: MissionPlan | null; updatedAt: string }
export interface Warning { code: string; availability: string }
export interface MissionAck { missionId: string; revision: number; state: MissionState; assigneeId: string; warning: Warning | null; replayed: boolean }
export interface BoardAsset { id: string; kind: string; name: string; fleetNumber: string; plate: string; unitId: string }
export interface Assignee { id: string; name: string; registration: string }
export interface CostCentre { id: string; name: string; code: string }
export interface Cause { availability: string; kind: string; lifecycle: string; from: string; to: string; reasonKind: string }
export interface Availability { assetId: string; availability: string; causes: Cause[]; missing: string[]; evaluatedAt: string }
export interface Reservation { id: string; state: string; startsAt: string; endsAt: string; endCause: string }
export interface Block { id: string; reasonKind: string; validFrom: string; validTo: string; removedAt: string }
export interface Page<T> { items: T[]; nextCursor: string }
export interface Agenda { assetId: string; availability: Availability; reservations: Reservation[]; blocks: Block[]; more: boolean }
export type CommandKind = 'create' | 'plan' | 'assign' | 'vehicle' | 'cancel';
export interface MissionCommandBody { schema_version: 1; operation_id: string; revision?: number; [field: string]: unknown }
export interface MissionSource {
  missions(org: string, cursor: string, signal: AbortSignal): Promise<Page<Mission>>;
  mission(org: string, id: string, signal: AbortSignal): Promise<Mission>;
  assignees(org: string, mission: string, cursor: string, signal: AbortSignal): Promise<Page<Assignee>>;
  assets(org: string, cursor: string, signal: AbortSignal): Promise<Page<BoardAsset>>;
  costCentres(org: string, cursor: string, signal: AbortSignal): Promise<Page<CostCentre>>;
  availability(org: string, asset: string, signal: AbortSignal): Promise<Availability>;
  reservations(org: string, asset: string, signal: AbortSignal): Promise<Page<Reservation>>;
  blocks(org: string, asset: string, signal: AbortSignal): Promise<Page<Block>>;
  command(org: string, kind: CommandKind, mission: string, body: MissionCommandBody, signal: AbortSignal): Promise<MissionAck>;
}

const AVAILABILITIES = ['AVAILABLE', 'RESERVED', 'IN_USE', 'IN_MAINTENANCE', 'BLOCKED', 'UNAVAILABLE'];
const scalars = (v: unknown, limit: number): v is string => typeof v === 'string' && [...v].length <= limit;
const instant = (v: unknown): v is string => typeof v === 'string' && v.length <= 40 && Number.isFinite(Date.parse(v));
const bad = (): never => { throw new ApiError('unavailable'); };
const optional = (v: unknown, check: (v: unknown) => boolean): boolean => v === undefined || check(v);
function page<T>(raw: Record<string, unknown>, parse: (v: unknown) => T): Page<T> {
  if (!Array.isArray(raw.items) || raw.items.length > 100 || typeof raw.next_cursor !== 'string' || raw.next_cursor.length > 8192) bad();
  return { items: (raw.items as unknown[]).map(parse), nextCursor: raw.next_cursor as string };
}
function parsePlan(raw: unknown, revision: number): MissionPlan {
  const v = record(raw), d = record(v.destination), l = record(v.load);
  if (!Number.isSafeInteger(v.revision) || Number(v.revision) < 2 || Number(v.revision) > revision || !scalars(v.reason, 280) || !instant(v.planned_at) || !optional(v.cost_centre_id, isUuid)) bad();
  const text = d.kind === 'TEXT' && scalars(d.text, 280) && d.unit_id === undefined, unit = d.kind === 'UNIT' && isUuid(d.unit_id) && d.text === undefined;
  const none = l.kind === 'NONE' && l.description === undefined && l.count === undefined;
  const cargo = l.kind === 'CARGO' && scalars(l.description, 280) && l.count === undefined;
  const people = l.kind === 'PASSENGERS' && Number.isInteger(l.count) && Number(l.count) >= 1 && Number(l.count) <= 999 && l.description === undefined;
  if (!(text || unit) || !(none || cargo || people)) bad();
  return { revision: Number(v.revision), destinationText: text ? String(d.text) : '', destinationUnitId: unit ? String(d.unit_id) : '', reason: String(v.reason), costCentreId: typeof v.cost_centre_id === 'string' ? v.cost_centre_id : '',
    load: l.kind as LoadKind, cargo: cargo ? String(l.description) : '', passengers: people ? Number(l.count) : 0, plannedAt: String(v.planned_at) };
}
// A mission is usable only with the server's own invariants: an answer has
// an assignee, ACCEPTED/DECLINED carry exactly their answer (B-BR-EMP-471).
export function parseMission(raw: unknown): Mission {
  const v = record(raw), answer = v.answer === undefined ? null : record(v.answer);
  if (!isUuid(v.mission_id) || !isUuid(v.unit_id) || !optional(v.asset_id, isUuid) || !optional(v.assignee_employee_id, isUuid) || !scalars(v.title, 280) ||
      !instant(v.starts_at) || !instant(v.ends_at) || !instant(v.updated_at) || !MISSION_STATES.includes(v.state as MissionState) || !Number.isSafeInteger(v.revision) || Number(v.revision) < 1) bad();
  if (answer && (!['ACCEPT', 'DECLINE'].includes(String(answer.response)) || !optional(answer.reason, r => scalars(r, 280)) || v.assignee_employee_id === undefined)) bad();
  const response = answer ? String(answer.response) as 'ACCEPT' | 'DECLINE' : '';
  if (v.state === 'ACCEPTED' && response !== 'ACCEPT' || v.state === 'DECLINED' && response !== 'DECLINE' || v.state === 'ASSIGNED' && v.assignee_employee_id === undefined) bad();
  const revision = Number(v.revision);
  const plan = v.plan === undefined ? null : parsePlan(v.plan, revision);
  if (v.state === 'PLANNED' && !plan) bad();
  return { id: v.mission_id as string, unitId: v.unit_id as string, assetId: (v.asset_id as string | undefined) ?? '', title: v.title as string, startsAt: v.starts_at as string, endsAt: v.ends_at as string, state: v.state as MissionState, revision,
    assigneeId: (v.assignee_employee_id as string | undefined) ?? '', answer: response, answerReason: answer && typeof answer.reason === 'string' ? answer.reason : '', plan, updatedAt: v.updated_at as string };
}
const EXPECTED: Record<CommandKind, (state: string) => boolean> = {
  create: s => s === 'DRAFT', plan: s => s === 'PLANNED', assign: s => s === 'ASSIGNED', cancel: s => s === 'CANCELLED', vehicle: s => ['PLANNED', 'ASSIGNED', 'ACCEPTED', 'DECLINED'].includes(s),
};
// An acknowledgement must match the decision it answers; otherwise its
// outcome is unknown and only the same operation may be retried.
export function parseAck(raw: unknown, kind: CommandKind, mission: string, body: MissionCommandBody): MissionAck {
  const v = record(raw), w = v.asset_warning === undefined ? null : record(v.asset_warning);
  const revision = Number(v.revision), expected = kind === 'create' ? 1 : Number(body.revision) + 1;
  if (!isUuid(v.mission_id) || kind !== 'create' && v.mission_id !== mission || revision !== expected || !EXPECTED[kind](String(v.state)) || typeof v.replayed !== 'boolean' || !optional(v.assignee_employee_id, isUuid) ||
      kind === 'assign' && v.assignee_employee_id !== body.assignee_employee_id || w && (kind !== 'assign' || !['ASSET_UNAVAILABLE', 'ASSET_BUSY'].includes(String(w.code)) || !AVAILABILITIES.includes(String(w.availability)))) bad();
  return { missionId: v.mission_id as string, revision, state: v.state as MissionState, assigneeId: (v.assignee_employee_id as string | undefined) ?? '', warning: w ? { code: String(w.code), availability: String(w.availability) } : null, replayed: v.replayed as boolean };
}
function parseAsset(raw: unknown): BoardAsset & { state: string } {
  const v = record(raw);
  if (!isUuid(v.id) || !['VEHICLE', 'MACHINE', 'IMPLEMENT'].includes(String(v.kind)) || !scalars(v.name, 200) || !scalars(v.fleet_number, 64) || !scalars(v.plate, 16) || typeof v.state !== 'string' || !(v.unit_id === '' || isUuid(v.unit_id))) bad();
  return { id: v.id as string, kind: v.kind as string, name: v.name as string, fleetNumber: v.fleet_number as string, plate: v.plate as string, unitId: v.unit_id as string, state: v.state as string };
}
function parseAssignee(raw: unknown): Assignee {
  const v = record(raw);
  if (!isUuid(v.employee_id) || !scalars(v.display_name, 200) || !scalars(v.registration_number, 64) || Object.keys(v).length !== 3) bad();
  return { id: v.employee_id as string, name: v.display_name as string, registration: v.registration_number as string };
}
function parseCostCentre(raw: unknown): CostCentre & { state: string } {
  const v = record(raw);
  if (!isUuid(v.id) || !scalars(v.name, 200) || !scalars(v.code, 64) || typeof v.state !== 'string') bad();
  return { id: v.id as string, name: v.name as string, code: v.code as string, state: v.state as string };
}
export class MissionBoardApi implements MissionSource {
  constructor(private readonly api: CompanyTransport) {}
  #path(org: string, tail: string): string { if (!isUuid(org)) throw new ApiError('denied'); return `/v1/orgs/${org}/${tail}`; }
  #id(id: string): string { if (!isUuid(id)) throw new ApiError('denied'); return id; }
  #list(cursor: string, extra: Record<string, unknown> = {}): Record<string, unknown> { return { limit: 100, ...extra, ...(cursor ? { cursor } : {}) }; }
  async missions(org: string, cursor: string, signal: AbortSignal): Promise<Page<Mission>> {
    return page(await this.api.privatePost(this.#path(org, 'missions/list'), { limit: 20, ...(cursor ? { cursor } : {}) }, signal), parseMission);
  }
  async mission(org: string, id: string, signal: AbortSignal): Promise<Mission> {
    const value = parseMission(await this.api.privatePost(this.#path(org, `missions/${this.#id(id)}/read`), {}, signal));
    if (value.id !== id) bad();
    return value;
  }
  async assignees(org: string, mission: string, cursor: string, signal: AbortSignal): Promise<Page<Assignee>> {
    return page(await this.api.privatePost(this.#path(org, `missions/${this.#id(mission)}/assignees/list`), this.#list(cursor), signal), parseAssignee);
  }
  // Only ACTIVE main assets can carry a mission (EDR-219, B-BR-EMP-472).
  async assets(org: string, cursor: string, signal: AbortSignal): Promise<Page<BoardAsset>> {
    const raw = page(await this.api.privatePost(this.#path(org, 'fleet/assets/list'), this.#list(cursor, { state: 'ACTIVE' }), signal), parseAsset);
    return { items: raw.items.filter(a => a.state === 'ACTIVE' && a.kind !== 'IMPLEMENT' && isUuid(a.unitId)).map(({ state: _state, ...a }) => a), nextCursor: raw.nextCursor };
  }
  async costCentres(org: string, cursor: string, signal: AbortSignal): Promise<Page<CostCentre>> {
    const raw = page(await this.api.privatePost(this.#path(org, 'cost-centres/list'), this.#list(cursor, { state: 'ACTIVE' }), signal), parseCostCentre);
    return { items: raw.items.filter(c => c.state === 'ACTIVE').map(({ state: _state, ...c }) => c), nextCursor: raw.nextCursor };
  }
  async availability(org: string, asset: string, signal: AbortSignal): Promise<Availability> {
    const v = await this.api.privatePost(this.#path(org, `fleet/assets/${this.#id(asset)}/availability/read`), {}, signal);
    if (v.asset_id !== asset || !AVAILABILITIES.includes(String(v.availability)) || !Array.isArray(v.causes) || v.causes.length > 50 || !Array.isArray(v.missing_sources) || v.missing_sources.length > 2 || !v.missing_sources.every(s => s === 'ACTIVE_USE' || s === 'MAINTENANCE_HOLD') || !instant(v.evaluated_at)) bad();
    const causes = (v.causes as unknown[]).map(raw => {
      const c = record(raw);
      if (!AVAILABILITIES.includes(String(c.availability)) || typeof c.kind !== 'string' || !optional(c.valid_from, instant) || !optional(c.valid_to, instant) || !optional(c.lifecycle_state, s => scalars(s, 40)) || !optional(c.reason_kind, s => scalars(s, 40))) bad();
      return { availability: String(c.availability), kind: c.kind as string, lifecycle: (c.lifecycle_state as string | undefined) ?? '', from: (c.valid_from as string | undefined) ?? '', to: (c.valid_to as string | undefined) ?? '', reasonKind: (c.reason_kind as string | undefined) ?? '' };
    });
    return { assetId: asset, availability: String(v.availability), causes, missing: [...v.missing_sources as string[]], evaluatedAt: v.evaluated_at as string };
  }
  // Purposes, notes and reservation people are not kept (EDR-306).
  async reservations(org: string, asset: string, signal: AbortSignal): Promise<Page<Reservation>> {
    return page(await this.api.privatePost(this.#path(org, `fleet/assets/${this.#id(asset)}/reservations/list`), this.#list(''), signal), raw => {
      const v = record(raw);
      if (!isUuid(v.reservation_id) || v.asset_id !== asset || !scalars(v.state, 40) || !instant(v.starts_at) || !instant(v.ends_at) || !optional(v.end_cause, s => scalars(s, 60))) bad();
      return { id: v.reservation_id as string, state: v.state as string, startsAt: v.starts_at as string, endsAt: v.ends_at as string, endCause: (v.end_cause as string | undefined) ?? '' };
    });
  }
  async blocks(org: string, asset: string, signal: AbortSignal): Promise<Page<Block>> {
    return page(await this.api.privatePost(this.#path(org, `fleet/assets/${this.#id(asset)}/blocks/list`), this.#list(''), signal), raw => {
      const v = record(raw);
      if (!isUuid(v.block_id) || v.asset_id !== asset || !scalars(v.reason_kind, 40) || !instant(v.valid_from) || !optional(v.valid_to, instant) || !optional(v.removed_at, instant)) bad();
      return { id: v.block_id as string, reasonKind: v.reason_kind as string, validFrom: v.valid_from as string, validTo: (v.valid_to as string | undefined) ?? '', removedAt: (v.removed_at as string | undefined) ?? '' };
    });
  }
  async command(org: string, kind: CommandKind, mission: string, body: MissionCommandBody, signal: AbortSignal): Promise<MissionAck> {
    const tail = kind === 'create' ? 'missions/create' : `missions/${this.#id(mission)}/${kind === 'vehicle' ? 'vehicle/change' : kind}`;
    return parseAck(await this.api.privatePost(this.#path(org, tail), body, signal), kind, mission, body);
  }
}

export interface PlanDraft { destination: string; reason: string; costCentreId: string; load: LoadKind; cargo: string; passengers: number }
export interface CreateDraft { title: string; startsAt: string; endsAt: string; assetId: string }
export type BoardPhase = 'closed' | 'idle' | 'loading' | 'ready' | 'failed' | 'denied';
export interface BoardState {
  phase: BoardPhase; authority: Authority | null; missions: Mission[]; cursor: string; listedAt: string;
  assets: BoardAsset[] | null; costCentres: CostCentre[] | null;
  selected: Mission | null; readAt: string; stale: boolean;
  assignees: Assignee[]; assigneeCursor: string; agenda: Agenda | null;
  warning: Warning | null; conflict: boolean; message: string;
}
export const initialBoard = (): BoardState => ({ phase: 'closed', authority: null, missions: [], cursor: '', listedAt: '', assets: null, costCentres: null, selected: null, readAt: '', stale: false, assignees: [], assigneeCursor: '', agenda: null, warning: null, conflict: false, message: '' });
// The label comes only from the server's state and answer (B-BR-EMP-471).
export function missionStateLabel(m: Mission): string {
  switch (m.state) {
    case 'DRAFT': return 'Rascunho';
    case 'PLANNED': return 'Planejada';
    case 'ASSIGNED': return 'Designada — aguardando resposta';
    case 'ACCEPTED': return m.answer === 'ACCEPT' ? 'Aceita pela pessoa designada' : 'Situação não confirmada';
    case 'DECLINED': return m.answer === 'DECLINE' ? 'Recusada pela pessoa designada' : 'Situação não confirmada';
    case 'IN_PROGRESS': return 'Em andamento';
    case 'COMPLETED': return 'Concluída';
    case 'CANCELLED': return 'Cancelada';
  }
}
const DENIED = 'Seu acesso a estas missões não pôde ser confirmado. Atualize seus acessos.';
const FRESH = 'Sua confirmação de acesso expirou. Entre novamente.';
const ASSIGNEE_PAGES = 20;
interface Intent { kind: CommandKind; mission: string; body: MissionCommandBody }
export class MissionBoardController {
  state = initialBoard();
  #epoch = 0;
  #pending = new AbortController();
  #retry: (() => Promise<void>) | null = null;
  #intent: Intent | null = null;
  constructor(private readonly source: MissionSource, private readonly render: (state: BoardState) => void, private readonly sessionLost?: (message: string) => void, private readonly now = () => new Date()) {}
  get retryable(): boolean { return this.#retry !== null; }
  get pendingWrite(): boolean { return this.#intent !== null; }
  #show(patch: Partial<BoardState>): void { this.state = { ...this.state, ...patch }; this.render(this.state); }
  #current(epoch: number): boolean { return epoch === this.#epoch && !this.#pending.signal.aborted; }
  #org(): string { const id = this.state.authority?.organization.id; if (!id) throw new ApiError('denied'); return id; }
  #stamp(): string { return this.now().toISOString(); }
  // Every scope change, refusal or reset discards private records and late work.
  clear(): void { this.#epoch++; this.#pending.abort(); this.#pending = new AbortController(); this.#retry = null; this.#intent = null; this.state = initialBoard(); this.render(this.state); }
  open(authority: Authority): void { this.clear(); this.#show({ phase: 'idle', authority }); }
  #fail(error: unknown, epoch: number): void {
    if (!this.#current(epoch) || error instanceof ApiError && error.kind === 'cancelled') return;
    if (error instanceof ApiError && ['denied', 'unauthenticated', 'fresh'].includes(error.kind)) {
      const authority = this.state.authority, session = error.kind !== 'denied';
      this.clear();
      if (session && this.sessionLost) { this.sessionLost(FRESH); return; }
      this.#show({ phase: 'denied', authority, message: session ? 'Confirme seu acesso novamente para continuar. Saia e entre no painel.' : DENIED });
      return;
    }
    if (error instanceof ApiError && error.kind === 'invalid') { this.#retry = null; this.#intent = null; this.#show({ phase: 'failed', message: 'Confira os dados informados. O servidor recusou a operação e nada foi alterado.' }); return; }
    this.#show({ phase: 'failed', message: this.#intent ? 'Não foi possível confirmar a operação. Tente novamente para consultar a mesma operação ou atualize a missão.' :
      this.state.stale ? 'A operação foi confirmada pelo servidor, mas a situação atual da missão não pôde ser consultada. Tente novamente para consultar.' : 'Não foi possível consultar o servidor. Tente novamente.' });
  }
  async #run(work: (epoch: number, signal: AbortSignal) => Promise<void>, retry: boolean): Promise<void> {
    this.#pending.abort(); this.#pending = new AbortController();
    const epoch = ++this.#epoch;
    this.#retry = retry ? () => this.#run(work, true) : null;
    this.#show({ phase: 'loading', message: '' });
    try { await work(epoch, this.#pending.signal); } catch (error) { this.#fail(error, epoch); }
  }
  #upsert(mission: Mission): Mission[] { const found = this.state.missions.some(m => m.id === mission.id); return found ? this.state.missions.map(m => m.id === mission.id ? mission : m) : [mission, ...this.state.missions]; }
  async #assets(org: string, epoch: number, signal: AbortSignal): Promise<BoardAsset[] | null> { return this.#all(epoch, cursor => this.source.assets(org, cursor, signal)); }
  // Optional reads: a refusal disables only their picker (EDR-305).
  async #all<T>(epoch: number, read: (cursor: string) => Promise<Page<T>>): Promise<T[] | null> {
    const items: T[] = [];
    let cursor = '';
    try {
      for (let i = 0; i < 10; i++) {
        const next = await read(cursor);
        if (!this.#current(epoch)) return null;
        items.push(...next.items);
        if (!next.nextCursor) return items;
        cursor = next.nextCursor;
      }
      return items;
    } catch (error) {
      if (error instanceof ApiError && error.kind === 'denied') return null;
      throw error;
    }
  }
  async load(): Promise<void> {
    if (!this.state.authority) return;
    const org = this.#org();
    await this.#run(async (epoch, signal) => {
      const first = await this.source.missions(org, '', signal);
      if (!this.#current(epoch)) return;
      this.#show({ missions: first.items, cursor: first.nextCursor, listedAt: this.#stamp(), selected: null, readAt: '', assignees: [], assigneeCursor: '', agenda: null, warning: null, conflict: false, stale: false });
      const assets = await this.#assets(org, epoch, signal);
      if (!this.#current(epoch)) return;
      const costCentres = await this.#all(epoch, cursor => this.source.costCentres(org, cursor, signal));
      if (!this.#current(epoch)) return;
      this.#retry = null;
      this.#show({ phase: 'ready', assets, costCentres, message: assets === null ? 'A lista de veículos não está disponível para sua conta; criar missões e trocar veículos fica indisponível.' : '' });
    }, true);
  }
  async more(): Promise<void> {
    const org = this.#org(), cursor = this.state.cursor;
    if (!cursor) return;
    await this.#run(async (epoch, signal) => {
      const next = await this.source.missions(org, cursor, signal);
      if (!this.#current(epoch)) return;
      this.#retry = null;
      this.#show({ phase: 'ready', missions: [...this.state.missions, ...next.items.filter(m => !this.state.missions.some(o => o.id === m.id))], cursor: next.nextCursor });
    }, true);
  }
  // Reads the mission, its eligible assignees and its vehicle agenda.
  async #detail(org: string, id: string, epoch: number, signal: AbortSignal, patch: Partial<BoardState>): Promise<void> {
    const mission = await this.source.mission(org, id, signal);
    if (!this.#current(epoch)) return;
    this.#show({ ...patch, selected: mission, readAt: this.#stamp(), stale: false, missions: this.#upsert(mission), assignees: [], assigneeCursor: '', agenda: null });
    let assignees: Assignee[] = [], cursor = '';
    for (let i = 0; i < ASSIGNEE_PAGES; i++) {
      const next = await this.source.assignees(org, id, cursor, signal);
      if (!this.#current(epoch)) return;
      assignees = [...assignees, ...next.items];
      cursor = next.nextCursor;
      if (!cursor || assignees.length > 0) break;
    }
    this.#show({ assignees, assigneeCursor: cursor });
    if (mission.assetId) await this.#agenda(org, mission.assetId, epoch, signal);
  }
  async #agenda(org: string, asset: string, epoch: number, signal: AbortSignal): Promise<void> {
    try {
      const availability = await this.source.availability(org, asset, signal);
      const reservations = await this.source.reservations(org, asset, signal);
      const blocks = await this.source.blocks(org, asset, signal);
      if (this.#current(epoch)) this.#show({ agenda: { assetId: asset, availability, reservations: reservations.items, blocks: blocks.items, more: reservations.nextCursor !== '' || blocks.nextCursor !== '' } });
    } catch (error) {
      if (!(error instanceof ApiError) || error.kind !== 'denied' && error.kind !== 'unavailable') throw error;
      if (this.#current(epoch)) this.#show({ agenda: null, message: 'A agenda do veículo não pôde ser consultada.' });
    }
  }
  async select(id: string): Promise<void> {
    const org = this.#org();
    this.#intent = null;
    await this.#run(async (epoch, signal) => {
      await this.#detail(org, id, epoch, signal, { warning: null, conflict: false });
      if (!this.#current(epoch)) return;
      this.#retry = null;
      this.#show({ phase: 'ready' });
    }, true);
  }
  async reload(): Promise<void> { if (this.state.selected) await this.select(this.state.selected.id); else await this.load(); }
  async moreAssignees(): Promise<void> {
    const org = this.#org(), mission = this.state.selected, cursor = this.state.assigneeCursor;
    if (!mission || !cursor) return;
    await this.#run(async (epoch, signal) => {
      const next = await this.source.assignees(org, mission.id, cursor, signal);
      if (!this.#current(epoch)) return;
      this.#retry = null;
      this.#show({ phase: 'ready', assignees: [...this.state.assignees, ...next.items.filter(a => !this.state.assignees.some(o => o.id === a.id))], assigneeCursor: next.nextCursor });
    }, true);
  }
  async agendaFor(asset: string): Promise<void> {
    const org = this.#org();
    if (!this.state.assets?.some(a => a.id === asset)) return;
    await this.#run(async (epoch, signal) => { await this.#agenda(org, asset, epoch, signal); if (this.#current(epoch)) { this.#retry = null; this.#show({ phase: 'ready' }); } }, true);
  }
  #refuse(message: string): void { this.#show({ message }); }
  // One immutable operation per decision; an unknown outcome is retried only
  // as the same operation, a conflict re-reads the server (B-BR-EMP-473).
  async #write(kind: CommandKind, mission: string, fields: Record<string, unknown>, message: string): Promise<void> {
    if (this.#intent) { this.#refuse('Há uma operação sem confirmação. Tente novamente a mesma operação ou atualize a missão antes de decidir de novo.'); return; }
    const org = this.#org(), selected = this.state.selected;
    const body: MissionCommandBody = { schema_version: 1, operation_id: crypto.randomUUID(), ...(kind === 'create' ? {} : { revision: selected?.revision }), ...fields };
    const intent: Intent = { kind, mission, body: structuredClone(body) };
    this.#intent = intent;
    const attempt = (): Promise<void> => this.#run(async (epoch, signal) => {
      let ack: MissionAck;
      try { ack = await this.source.command(org, intent.kind, intent.mission, structuredClone(intent.body), signal); } catch (error) {
        if (!this.#current(epoch) || !(error instanceof ApiError) || error.kind !== 'conflict') throw error;
        this.#intent = null; this.#retry = null;
        await this.#conflict(org, intent.kind === 'create' ? '' : intent.mission, epoch, signal);
        return;
      }
      if (!this.#current(epoch)) return;
      this.#intent = null;
      // The acknowledged write is never replayed; only the read is retried.
      const reread = (): Promise<void> => this.#run(async (e, s) => { await this.#detail(org, ack.missionId, e, s, {}); if (this.#current(e)) { this.#retry = null; this.#show({ phase: 'ready', stale: false }); } }, false).then(() => { if (this.state.phase === 'failed') this.#retry = reread; });
      this.#retry = reread;
      this.#show({ warning: ack.warning, conflict: false, stale: true, message });
      await this.#detail(org, ack.missionId, epoch, signal, { warning: ack.warning, conflict: false });
      if (!this.#current(epoch)) return;
      this.#retry = null;
      this.#show({ phase: 'ready', message: message + (ack.warning ? ' Confira o aviso de disponibilidade do veículo.' : '') });
    }, false).then(() => { if (this.#intent === intent && this.state.phase === 'failed') this.#retry = attempt; });
    await attempt();
  }
  async #conflict(org: string, mission: string, epoch: number, signal: AbortSignal): Promise<void> {
    const message = 'A missão mudou ou a ação não é mais possível na situação atual. Carregamos a versão atual do servidor; confira antes de decidir de novo.';
    if (!mission) { this.#show({ phase: 'failed', conflict: true, message }); return; }
    await this.#detail(org, mission, epoch, signal, { conflict: true, warning: null });
    if (this.#current(epoch)) this.#show({ phase: 'ready', conflict: true, message });
  }
  async create(draft: CreateDraft): Promise<void> {
    const asset = this.state.assets?.find(a => a.id === draft.assetId);
    if (!asset) { this.#refuse('Escolha um veículo da lista de veículos ativos.'); return; }
    await this.#write('create', '', { unit_id: asset.unitId, asset_id: asset.id, title: draft.title, starts_at: draft.startsAt, ends_at: draft.endsAt }, 'Missão criada no servidor.');
  }
  async plan(draft: PlanDraft): Promise<void> {
    const mission = this.state.selected;
    if (!mission) return;
    if (draft.costCentreId && !this.state.costCentres?.some(c => c.id === draft.costCentreId)) { this.#refuse('Escolha um centro de custo da lista atual.'); return; }
    const load = draft.load === 'CARGO' ? { kind: 'CARGO', description: draft.cargo } : draft.load === 'PASSENGERS' ? { kind: 'PASSENGERS', count: draft.passengers } : { kind: 'NONE' };
    await this.#write('plan', mission.id, { destination: { kind: 'TEXT', text: draft.destination }, reason: draft.reason, ...(draft.costCentreId ? { cost_centre_id: draft.costCentreId } : {}), load }, 'Planejamento salvo no servidor.');
  }
  async assign(employee: string): Promise<void> {
    const mission = this.state.selected;
    if (!mission) return;
    if (!this.state.assignees.some(a => a.id === employee) || employee === mission.assigneeId) { this.#refuse('Escolha uma pessoa da lista de pessoas elegíveis desta missão.'); return; }
    await this.#write('assign', mission.id, { assignee_employee_id: employee }, 'Designação registrada no servidor. A pessoa ainda precisa responder.');
  }
  async changeVehicle(asset: string, reason: string): Promise<void> {
    const mission = this.state.selected;
    if (!mission) return;
    if (!this.state.assets?.some(a => a.id === asset) || asset === mission.assetId) { this.#refuse('Escolha outro veículo da lista de veículos ativos.'); return; }
    await this.#write('vehicle', mission.id, { asset_id: asset, reason }, 'Troca de veículo registrada no servidor.');
  }
  async cancel(reason: string): Promise<void> {
    const mission = this.state.selected;
    if (!mission) return;
    await this.#write('cancel', mission.id, { reason }, 'Cancelamento registrado no servidor.');
  }
  async retry(): Promise<void> { await this.#retry?.(); }
}
