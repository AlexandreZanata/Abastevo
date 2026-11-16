import { ApiError, type CompanyTransport, type ContentTransport } from './api.js';
import { type Authority, isUuid, record } from './model.js';

// EMP-CHECK-004 checklist and incident panel (B-BR-EMP-506..510): the
// read/command port over the existing company routes, strict parsers and a
// controller that shows only server-read state. Document content is never
// kept: the controller hands the bytes to the caller once.
export type RunOutcome = 'COMPLETED_OK' | 'COMPLETED_WITH_ISSUES' | 'BLOCKING';
export type ReportState = 'REPORTED' | 'ACKNOWLEDGED' | 'IN_TREATMENT' | 'RESOLVED' | 'DISMISSED';
export type ReportAction = 'ACKNOWLEDGE' | 'TREAT' | 'RESOLVE' | 'DISMISS';
export interface Run { id: string; assetId: string; templateId: string; context: string; version: number; policyRevision: number; serverPolicyRevision: number; revision: number; startedAt: string; completedAt: string; receivedAt: string; device: RunOutcome; recheck: RunOutcome; outcome: RunOutcome; differs: boolean; superseded: boolean; acknowledged: boolean; issues: number[]; critical: number[]; releasedAt: string }
export interface ReportSummary { id: string; type: 'DAMAGE' | 'INCIDENT'; kind: string; assetId: string; tripId: string; reporter: string; blocksUse: boolean; state: ReportState; revision: number; occurredAt: string; receivedAt: string; updatedAt: string }
export interface Transition { revision: number; action: ReportAction; from: ReportState; to: ReportState; note: string; duplicateOf: string; actor: string; recordedAt: string }
export interface Attachment { documentId: string; storage: string; linkedAt: string }
export interface Report extends ReportSummary { description: string; location: string; history: Transition[]; attachments: Attachment[] }
export interface DecisionAck { reportId: string; action: ReportAction; state: ReportState; revision: number; recordedAt: string; replayed: boolean }
export interface AssetLabel { id: string; name: string; fleetNumber: string; plate: string }
export interface Cause { availability: string; kind: string; recordId: string; from: string; to: string; reasonKind: string }
export interface Availability { assetId: string; availability: string; causes: Cause[]; missing: string[]; evaluatedAt: string }
export interface ContentFile { documentId: string; mediaType: string; bytes: Uint8Array<ArrayBuffer> }
export interface Page<T> { items: T[]; nextCursor: string }
export interface ReleaseBody { schema_version: 1; operation_id: string; revision: number; reason: string }
export interface DecisionBody { schema_version: 1; operation_id: string; revision: number; action: ReportAction; note?: string; duplicate_of?: string }
export interface CheckSource {
  runs(org: string, cursor: string, signal: AbortSignal): Promise<Page<Run>>;
  run(org: string, asset: string, run: string, signal: AbortSignal): Promise<Run>;
  release(org: string, run: string, body: ReleaseBody, signal: AbortSignal): Promise<Run>;
  reports(org: string, cursor: string, signal: AbortSignal): Promise<Page<ReportSummary>>;
  report(org: string, id: string, signal: AbortSignal): Promise<Report>;
  decide(org: string, id: string, body: DecisionBody, signal: AbortSignal): Promise<DecisionAck>;
  assets(org: string, cursor: string, signal: AbortSignal): Promise<Page<AssetLabel>>;
  availability(org: string, asset: string, signal: AbortSignal): Promise<Availability>;
  content(org: string, document: string, signal: AbortSignal): Promise<ContentFile>;
}

const OUTCOMES: RunOutcome[] = ['COMPLETED_OK', 'COMPLETED_WITH_ISSUES', 'BLOCKING'];
const STATES: ReportState[] = ['REPORTED', 'ACKNOWLEDGED', 'IN_TREATMENT', 'RESOLVED', 'DISMISSED'];
const ACTIONS: ReportAction[] = ['ACKNOWLEDGE', 'TREAT', 'RESOLVE', 'DISMISS'];
const STEP: Record<ReportAction, ReportState> = { ACKNOWLEDGE: 'ACKNOWLEDGED', TREAT: 'IN_TREATMENT', RESOLVE: 'RESOLVED', DISMISS: 'DISMISSED' };
const STORAGE = ['UPLOADING', 'RECEIVED', 'STORED', 'QUARANTINED', 'FAILED', 'REPLACED', 'DELETED'];
const AVAILABILITIES = ['AVAILABLE', 'RESERVED', 'IN_USE', 'IN_MAINTENANCE', 'BLOCKED', 'UNAVAILABLE'];
const SOURCES = ['ACTIVE_USE', 'CHECKLIST_RUN', 'MAINTENANCE_HOLD'];
const MAX_TEXT = 1000, MAX_NOTE = 500;
const scalars = (v: unknown, limit: number): v is string => typeof v === 'string' && [...v].length <= limit;
const text = (v: unknown, limit: number): v is string => scalars(v, limit) && v.trim().length > 0;
const instant = (v: unknown): v is string => typeof v === 'string' && v.length <= 40 && Number.isFinite(Date.parse(v));
const count = (v: unknown, min: number): boolean => Number.isSafeInteger(v) && Number(v) >= min;
const bad = (): never => { throw new ApiError('unavailable'); };
const optional = (v: unknown, check: (v: unknown) => boolean): boolean => v === undefined || check(v);
const positions = (v: unknown): v is number[] => Array.isArray(v) && v.length <= 40 && v.every((p, i) => Number.isInteger(p) && p >= 1 && p <= 40 && (i === 0 || p > v[i - 1]));
function page<T>(raw: Record<string, unknown>, parse: (v: unknown) => T): Page<T> {
  if (!Array.isArray(raw.items) || raw.items.length > 100 || typeof raw.next_cursor !== 'string' || raw.next_cursor.length > 8192) bad();
  return { items: (raw.items as unknown[]).map(parse), nextCursor: raw.next_cursor as string };
}
// A run is usable only with the stored invariants (B-BR-EMP-441/506):
// revision 1 is current, revision 2 is a released BLOCKING run.
export function parseRun(raw: unknown): Run {
  const v = record(raw);
  const device = v.device_outcome as RunOutcome, recheck = v.recheck_outcome as RunOutcome, outcome = v.outcome as RunOutcome;
  if (!isUuid(v.run_id) || !isUuid(v.asset_id) || !isUuid(v.template_id) || !['DEPARTURE', 'RETURN', 'AD_HOC'].includes(String(v.context)) || !count(v.template_version, 1) ||
      !count(v.policy_revision, 1) || !count(v.server_policy_revision, 1) || !instant(v.started_at) || !instant(v.completed_at) || !instant(v.received_at) ||
      !OUTCOMES.includes(device) || !OUTCOMES.includes(recheck) || !OUTCOMES.includes(outcome) || typeof v.outcome_differs !== 'boolean' || v.review_required !== v.outcome_differs ||
      v.outcome_differs !== (device !== recheck) || typeof v.template_superseded !== 'boolean' || typeof v.acknowledged !== 'boolean' || !positions(v.issues) || !positions(v.critical_issues) ||
      !(v.critical_issues as number[]).every(p => (v.issues as number[]).includes(p)) || ((v.issues as number[]).length === 0) !== (recheck === 'COMPLETED_OK') || v.data_mode !== 'SYNTHETIC') bad();
  const released = v.revision === 2 && recheck === 'BLOCKING' && outcome === 'COMPLETED_WITH_ISSUES' && instant(v.released_at);
  const current = v.revision === 1 && outcome === recheck && v.released_at === undefined;
  if (!released && !current) bad();
  return { id: v.run_id as string, assetId: v.asset_id as string, templateId: v.template_id as string, context: v.context as string, version: Number(v.template_version), policyRevision: Number(v.policy_revision),
    serverPolicyRevision: Number(v.server_policy_revision), revision: Number(v.revision), startedAt: v.started_at as string, completedAt: v.completed_at as string, receivedAt: v.received_at as string,
    device, recheck, outcome, differs: v.outcome_differs as boolean, superseded: v.template_superseded as boolean, acknowledged: v.acknowledged as boolean,
    issues: [...v.issues as number[]], critical: [...v.critical_issues as number[]], releasedAt: released ? v.released_at as string : '' };
}
function parseSummary(raw: unknown): ReportSummary {
  const v = record(raw);
  if (!isUuid(v.report_id) || !['DAMAGE', 'INCIDENT'].includes(String(v.report_type)) || !optional(v.incident_kind, k => ['BREAKDOWN', 'ACCIDENT', 'THEFT', 'FINE_NOTICE', 'OTHER'].includes(String(k))) ||
      !isUuid(v.asset_id) || !optional(v.trip_id, isUuid) || !isUuid(v.reporter_membership_id) || typeof v.blocks_use !== 'boolean' || !STATES.includes(v.state as ReportState) ||
      !Number.isInteger(v.revision) || Number(v.revision) < 1 || Number(v.revision) > 4 || !instant(v.occurred_at) || !instant(v.received_at) || !instant(v.updated_at)) bad();
  return { id: v.report_id as string, type: v.report_type as 'DAMAGE' | 'INCIDENT', kind: (v.incident_kind as string | undefined) ?? '', assetId: v.asset_id as string, tripId: (v.trip_id as string | undefined) ?? '',
    reporter: v.reporter_membership_id as string, blocksUse: v.blocks_use as boolean, state: v.state as ReportState, revision: Number(v.revision), occurredAt: v.occurred_at as string,
    receivedAt: v.received_at as string, updatedAt: v.updated_at as string };
}
// The history must lead from REPORTED to the current state and revision.
export function parseReport(raw: unknown): Report {
  const v = record(raw), summary = parseSummary(raw);
  if (!text(v.description, MAX_TEXT) || !optional(v.location, l => text(l, MAX_TEXT)) || !Array.isArray(v.history) || v.history.length > 3 || !Array.isArray(v.attachments) || v.attachments.length > 50 || v.data_mode !== 'SYNTHETIC') bad();
  let state: ReportState = 'REPORTED', revision = 1;
  const history = (v.history as unknown[]).map(item => {
    const h = record(item), action = h.action as ReportAction;
    if (!ACTIONS.includes(action) || h.revision !== revision + 1 || h.from_state !== state || h.to_state !== STEP[action] || !isUuid(h.actor_membership_id) || !instant(h.recorded_at) ||
        (action === 'ACKNOWLEDGE') !== (h.note === undefined) || !optional(h.note, n => text(n, MAX_NOTE)) || !optional(h.duplicate_of, d => isUuid(d) && action === 'DISMISS')) bad();
    state = h.to_state as ReportState; revision = Number(h.revision);
    return { revision, action, from: h.from_state as ReportState, to: state, note: (h.note as string | undefined) ?? '', duplicateOf: (h.duplicate_of as string | undefined) ?? '', actor: h.actor_membership_id as string, recordedAt: h.recorded_at as string };
  });
  if (state !== summary.state || revision !== summary.revision) bad();
  const attachments = (v.attachments as unknown[]).map(item => {
    const a = record(item);
    if (!isUuid(a.document_id) || !STORAGE.includes(String(a.storage_state)) || !instant(a.linked_at) || Object.keys(a).length !== 3) bad();
    return { documentId: a.document_id as string, storage: a.storage_state as string, linkedAt: a.linked_at as string };
  });
  return { ...summary, description: v.description as string, location: (v.location as string | undefined) ?? '', history, attachments };
}
function parseAsset(raw: unknown): AssetLabel {
  const v = record(raw);
  if (!isUuid(v.id) || !scalars(v.name, 200) || !scalars(v.fleet_number, 64) || !scalars(v.plate, 16)) bad();
  return { id: v.id as string, name: v.name as string, fleetNumber: v.fleet_number as string, plate: v.plate as string };
}
export function runOutcomeLabel(outcome: RunOutcome): string {
  switch (outcome) {
    case 'BLOCKING': return 'Bloqueando o uso';
    case 'COMPLETED_WITH_ISSUES': return 'Concluído com pendências';
    case 'COMPLETED_OK': return 'Concluído sem pendências';
  }
}
export function reportStateLabel(state: ReportState): string {
  switch (state) {
    case 'REPORTED': return 'Aguardando tratamento';
    case 'ACKNOWLEDGED': return 'Ciente';
    case 'IN_TREATMENT': return 'Em tratamento';
    case 'RESOLVED': return 'Resolvida';
    case 'DISMISSED': return 'Descartada';
  }
}
const OPEN: ReportState[] = ['REPORTED', 'ACKNOWLEDGED', 'IN_TREATMENT'];
// Open reports first, blocking marks first among them; otherwise the server order.
export function reportOrder(reports: ReportSummary[]): ReportSummary[] {
  const rank = (r: ReportSummary): number => OPEN.includes(r.state) ? (r.blocksUse ? 0 : 1) : 2;
  return reports.map((r, i) => ({ r, i })).sort((a, b) => rank(a.r) - rank(b.r) || a.i - b.i).map(x => x.r);
}
export function allowedActions(state: ReportState): ReportAction[] {
  switch (state) {
    case 'REPORTED': return ['ACKNOWLEDGE', 'TREAT', 'RESOLVE', 'DISMISS'];
    case 'ACKNOWLEDGED': return ['TREAT', 'RESOLVE', 'DISMISS'];
    case 'IN_TREATMENT': return ['RESOLVE', 'DISMISS'];
    default: return [];
  }
}
function base64(value: unknown, size: number): Uint8Array<ArrayBuffer> {
  if (typeof value !== 'string' || value.length === 0 || value.length > 6_990_508 || value.length % 4 !== 0 || !/^[A-Za-z0-9+/]*={0,2}$/.test(value)) bad();
  let binary = '';
  try { binary = atob(value as string); } catch { bad(); }
  if (binary.length !== size) bad();
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);
  return bytes;
}
export class CheckPanelApi implements CheckSource {
  constructor(private readonly api: CompanyTransport & ContentTransport) {}
  #path(org: string, tail: string): string { if (!isUuid(org)) throw new ApiError('denied'); return `/v1/orgs/${org}/${tail}`; }
  #id(id: string): string { if (!isUuid(id)) throw new ApiError('denied'); return id; }
  async runs(org: string, cursor: string, signal: AbortSignal): Promise<Page<Run>> {
    return page(await this.api.privatePost(this.#path(org, 'checklists/runs/list'), { limit: 50, ...(cursor ? { cursor } : {}) }, signal), parseRun);
  }
  async run(org: string, asset: string, run: string, signal: AbortSignal): Promise<Run> {
    const value = parseRun(await this.api.privatePost(this.#path(org, `checklists/assets/${this.#id(asset)}/runs/${this.#id(run)}/read`), {}, signal));
    if (value.id !== run || value.assetId !== asset) bad();
    return value;
  }
  // The acknowledgement must be the released run at the next revision.
  async release(org: string, run: string, body: ReleaseBody, signal: AbortSignal): Promise<Run> {
    const raw = await this.api.privatePost(this.#path(org, `checklists/runs/${this.#id(run)}/release`), body, signal);
    if (typeof raw.replayed !== 'boolean') bad();
    const { replayed: _replayed, ...fields } = raw;
    const value = parseRun(fields);
    if (value.id !== run || value.revision !== body.revision + 1 || value.outcome !== 'COMPLETED_WITH_ISSUES') bad();
    return value;
  }
  async reports(org: string, cursor: string, signal: AbortSignal): Promise<Page<ReportSummary>> {
    return page(await this.api.privatePost(this.#path(org, 'incidents/reports/list'), { limit: 50, ...(cursor ? { cursor } : {}) }, signal), parseSummary);
  }
  async report(org: string, id: string, signal: AbortSignal): Promise<Report> {
    const value = parseReport(await this.api.privatePost(this.#path(org, `incidents/reports/${this.#id(id)}/read`), {}, signal));
    if (value.id !== id) bad();
    return value;
  }
  async decide(org: string, id: string, body: DecisionBody, signal: AbortSignal): Promise<DecisionAck> {
    const v = await this.api.privatePost(this.#path(org, `incidents/reports/${this.#id(id)}/decide`), body, signal);
    if (v.report_id !== id || v.action !== body.action || v.state !== STEP[body.action] || v.revision !== body.revision + 1 || !instant(v.recorded_at) || typeof v.replayed !== 'boolean') bad();
    return { reportId: id, action: body.action, state: STEP[body.action], revision: body.revision + 1, recordedAt: v.recorded_at as string, replayed: v.replayed as boolean };
  }
  async assets(org: string, cursor: string, signal: AbortSignal): Promise<Page<AssetLabel>> {
    return page(await this.api.privatePost(this.#path(org, 'fleet/assets/list'), { limit: 100, ...(cursor ? { cursor } : {}) }, signal), parseAsset);
  }
  async availability(org: string, asset: string, signal: AbortSignal): Promise<Availability> {
    const v = await this.api.privatePost(this.#path(org, `fleet/assets/${this.#id(asset)}/availability/read`), {}, signal);
    if (v.asset_id !== asset || !AVAILABILITIES.includes(String(v.availability)) || !Array.isArray(v.causes) || v.causes.length > 50 || !Array.isArray(v.missing_sources) ||
        v.missing_sources.length > SOURCES.length || !v.missing_sources.every(s => SOURCES.includes(String(s))) || !instant(v.evaluated_at)) bad();
    const causes = (v.causes as unknown[]).map(raw => {
      const c = record(raw);
      if (!AVAILABILITIES.includes(String(c.availability)) || !scalars(c.kind, 40) || !optional(c.record_id, isUuid) || !optional(c.valid_from, instant) || !optional(c.valid_to, instant) ||
          !optional(c.lifecycle_state, s => scalars(s, 40)) || !optional(c.reason_kind, s => scalars(s, 40))) bad();
      return { availability: String(c.availability), kind: c.kind as string, recordId: (c.record_id as string | undefined) ?? '', from: (c.valid_from as string | undefined) ?? '', to: (c.valid_to as string | undefined) ?? '', reasonKind: (c.reason_kind as string | undefined) ?? '' };
    });
    return { assetId: asset, availability: String(v.availability), causes, missing: [...v.missing_sources as string[]], evaluatedAt: v.evaluated_at as string };
  }
  // The audited content path (DOC-002/PRIV-005): an issued capability, used
  // once for the read and never returned to the caller.
  async content(org: string, document: string, signal: AbortSignal): Promise<ContentFile> {
    const grant = await this.api.privateContent(this.#path(org, `documents/${this.#id(document)}/content/access`), {}, signal);
    if (grant.document_id !== document || typeof grant.capability !== 'string' || grant.capability.length === 0 || grant.capability.length > 2048 || !instant(grant.expires_at)) bad();
    const v = await this.api.privateContent(this.#path(org, `documents/${document}/content/read`), { capability: grant.capability }, signal);
    if (v.document_id !== document || v.data_mode !== 'SYNTHETIC' || v.media_type !== 'image/jpeg' || !Number.isInteger(v.bytes) || Number(v.bytes) < 1 || Number(v.bytes) > 5_242_880) bad();
    return { documentId: document, mediaType: 'image/jpeg', bytes: base64(v.content_base64, Number(v.bytes)) };
  }
}

export type CheckPhase = 'closed' | 'idle' | 'loading' | 'ready' | 'failed' | 'denied';
export interface CheckState {
  phase: CheckPhase; authority: Authority | null; assets: AssetLabel[] | null;
  runs: Run[]; runCursor: string; runsAt: string; reports: ReportSummary[]; reportCursor: string; reportsAt: string;
  run: Run | null; report: Report | null; readAt: string; stale: boolean; availability: Availability | null;
  contentAllowed: boolean; conflict: boolean; selfApproval: boolean; message: string;
}
export const initialChecks = (): CheckState => ({ phase: 'closed', authority: null, assets: null, runs: [], runCursor: '', runsAt: '', reports: [], reportCursor: '', reportsAt: '', run: null, report: null, readAt: '', stale: false, availability: null, contentAllowed: false, conflict: false, selfApproval: false, message: '' });
const DENIED = 'Seu acesso a estes registros não pôde ser confirmado. Atualize seus acessos.';
const FRESH = 'Sua confirmação de acesso expirou. Entre novamente.';
const CONFLICT = 'Registro já atualizado por outra pessoa, ou a ação não é mais possível. Carregamos a versão atual do servidor; confira antes de decidir de novo.';
const UNCONFIRMED = 'A disponibilidade do veículo não pôde ser confirmada agora; não a considere liberada.';
type Intent = { kind: 'release'; id: string; asset: string; body: ReleaseBody } | { kind: 'decide'; id: string; body: DecisionBody };
export class CheckPanelController {
  state = initialChecks();
  #epoch = 0;
  #pending = new AbortController();
  #retry: (() => Promise<void>) | null = null;
  #intent: Intent | null = null;
  constructor(private readonly source: CheckSource, private readonly render: (state: CheckState) => void, private readonly sessionLost?: (message: string) => void, private readonly now = () => new Date()) {}
  get retryable(): boolean { return this.#retry !== null; }
  get pendingWrite(): boolean { return this.#intent !== null; }
  #show(patch: Partial<CheckState>): void { this.state = { ...this.state, ...patch }; this.render(this.state); }
  #current(epoch: number): boolean { return epoch === this.#epoch && !this.#pending.signal.aborted; }
  #org(): string { const id = this.state.authority?.organization.id; if (!id) throw new ApiError('denied'); return id; }
  #stamp(): string { return this.now().toISOString(); }
  // Every scope change, refusal or reset discards private records and late work.
  clear(): void { this.#epoch++; this.#pending.abort(); this.#pending = new AbortController(); this.#retry = null; this.#intent = null; this.state = initialChecks(); this.render(this.state); }
  open(authority: Authority): void {
    this.clear();
    this.#show({ phase: 'idle', authority, contentAllowed: authority.permissions?.includes('doc.document.view_content') === true });
  }
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
    this.#show({ phase: 'failed', message: this.#intent ? 'Não foi possível confirmar a operação. Tente novamente para consultar a mesma operação ou atualize o registro.' :
      this.state.stale ? 'A operação foi confirmada pelo servidor, mas a situação atual não pôde ser consultada. Tente novamente para consultar.' : 'Não foi possível consultar o servidor. Tente novamente.' });
  }
  async #run(work: (epoch: number, signal: AbortSignal) => Promise<void>, retry: boolean): Promise<void> {
    this.#pending.abort(); this.#pending = new AbortController();
    const epoch = ++this.#epoch;
    this.#retry = retry ? () => this.#run(work, true) : null;
    this.#show({ phase: 'loading', message: '' });
    try { await work(epoch, this.#pending.signal); } catch (error) { this.#fail(error, epoch); }
  }
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
      const runs = await this.source.runs(org, '', signal);
      if (!this.#current(epoch)) return;
      const reports = await this.source.reports(org, '', signal);
      if (!this.#current(epoch)) return;
      const stamp = this.#stamp();
      this.#show({ runs: runs.items, runCursor: runs.nextCursor, runsAt: stamp, reports: reportOrder(reports.items), reportCursor: reports.nextCursor, reportsAt: stamp, run: null, report: null, readAt: '', availability: null, conflict: false, selfApproval: false, stale: false });
      // Labels only: a refused asset list leaves neutral labels (EDR-330).
      const assets = await this.#all(epoch, cursor => this.source.assets(org, cursor, signal));
      if (!this.#current(epoch)) return;
      this.#retry = null;
      this.#show({ phase: 'ready', assets });
    }, true);
  }
  async moreRuns(): Promise<void> {
    const org = this.#org(), cursor = this.state.runCursor;
    if (!cursor) return;
    await this.#run(async (epoch, signal) => {
      const next = await this.source.runs(org, cursor, signal);
      if (!this.#current(epoch)) return;
      this.#retry = null;
      this.#show({ phase: 'ready', runs: [...this.state.runs, ...next.items.filter(r => !this.state.runs.some(o => o.id === r.id))], runCursor: next.nextCursor });
    }, true);
  }
  async moreReports(): Promise<void> {
    const org = this.#org(), cursor = this.state.reportCursor;
    if (!cursor) return;
    await this.#run(async (epoch, signal) => {
      const next = await this.source.reports(org, cursor, signal);
      if (!this.#current(epoch)) return;
      this.#retry = null;
      this.#show({ phase: 'ready', reports: reportOrder([...this.state.reports, ...next.items.filter(r => !this.state.reports.some(o => o.id === r.id))]), reportCursor: next.nextCursor });
    }, true);
  }
  // The asset's FLEET-007 availability as the server reads it now; a failed
  // read is never shown as available (B-BR-EMP-508).
  async #availability(org: string, asset: string, epoch: number, signal: AbortSignal): Promise<string> {
    try {
      const availability = await this.source.availability(org, asset, signal);
      if (this.#current(epoch)) this.#show({ availability });
      return '';
    } catch (error) {
      if (!(error instanceof ApiError) || error.kind !== 'denied' && error.kind !== 'unavailable') throw error;
      if (this.#current(epoch)) this.#show({ availability: null });
      return UNCONFIRMED;
    }
  }
  async #readRun(org: string, asset: string, id: string, epoch: number, signal: AbortSignal, patch: Partial<CheckState>): Promise<string> {
    const run = await this.source.run(org, asset, id, signal);
    if (!this.#current(epoch)) return '';
    this.#show({ ...patch, run, report: null, readAt: this.#stamp(), stale: false, availability: null, runs: this.state.runs.map(r => r.id === run.id ? run : r) });
    return this.#availability(org, run.assetId, epoch, signal);
  }
  async #readReport(org: string, id: string, epoch: number, signal: AbortSignal, patch: Partial<CheckState>): Promise<string> {
    const report = await this.source.report(org, id, signal);
    if (!this.#current(epoch)) return '';
    const { description: _d, location: _l, history: _h, attachments: _a, ...summary } = report;
    this.#show({ ...patch, report, run: null, readAt: this.#stamp(), stale: false, availability: null, reports: this.state.reports.map(r => r.id === report.id ? summary : r) });
    return this.#availability(org, report.assetId, epoch, signal);
  }
  // Only a listed record is opened: no free reference reaches the server.
  async openRun(id: string): Promise<void> {
    const listed = this.state.runs.find(r => r.id === id);
    if (!listed) return;
    const org = this.#org();
    this.#intent = null;
    await this.#run(async (epoch, signal) => {
      const message = await this.#readRun(org, listed.assetId, id, epoch, signal, { conflict: false, selfApproval: false });
      if (this.#current(epoch)) { this.#retry = null; this.#show({ phase: 'ready', message }); }
    }, true);
  }
  async openReport(id: string): Promise<void> {
    if (!this.state.reports.some(r => r.id === id)) return;
    const org = this.#org();
    this.#intent = null;
    await this.#run(async (epoch, signal) => {
      const message = await this.#readReport(org, id, epoch, signal, { conflict: false, selfApproval: false });
      if (this.#current(epoch)) { this.#retry = null; this.#show({ phase: 'ready', message }); }
    }, true);
  }
  async reload(): Promise<void> {
    if (this.state.run) await this.openRun(this.state.run.id);
    else if (this.state.report) await this.openReport(this.state.report.id);
    else await this.load();
  }
  actions(): ReportAction[] { return this.state.report ? allowedActions(this.state.report.state) : []; }
  // A duplicate repeats a listed report of the same asset (BR-CHECK-017).
  duplicates(): ReportSummary[] { const r = this.state.report; return r ? this.state.reports.filter(o => o.id !== r.id && o.assetId === r.assetId) : []; }
  canOpenContent(document: string): boolean {
    return this.state.contentAllowed && this.state.report?.attachments.some(a => a.documentId === document && a.storage === 'STORED') === true;
  }
  // The explicit content request (B-BR-EMP-507): only with the hint and a
  // STORED attachment of the open report; the bytes are returned once.
  async content(document: string): Promise<ContentFile | null> {
    if (!this.canOpenContent(document) || this.state.phase === 'loading') return null;
    const org = this.#org(), epoch = this.#epoch, signal = this.#pending.signal;
    try {
      const file = await this.source.content(org, document, signal);
      if (!this.#current(epoch)) return null;
      this.#show({ message: 'Conteúdo entregue pelo servidor; o acesso foi registrado.' });
      return file;
    } catch (error) {
      if (!this.#current(epoch) || error instanceof ApiError && error.kind === 'cancelled') return null;
      if (error instanceof ApiError && (error.kind === 'unauthenticated' || error.kind === 'fresh')) { this.#fail(error, epoch); return null; }
      if (error instanceof ApiError && error.kind === 'denied' && error.code === 'FORBIDDEN') { this.#show({ contentAllowed: false, message: 'Sua conta não tem permissão para abrir o conteúdo de documentos. O registro continua disponível.' }); return null; }
      this.#show({ message: error instanceof ApiError && error.kind === 'denied' ? 'O conteúdo deste documento não está disponível.' : 'Não foi possível obter o conteúdo agora. Tente novamente.' });
      return null;
    }
  }
  #refuse(message: string): void { this.#show({ message }); }
  #reread(org: string, intent: Intent, epoch: number, signal: AbortSignal, patch: Partial<CheckState>): Promise<string> {
    return intent.kind === 'release' ? this.#readRun(org, intent.asset, intent.id, epoch, signal, patch) : this.#readReport(org, intent.id, epoch, signal, patch);
  }
  // One immutable operation per decision; an unknown outcome is retried only
  // as the same operation; CONFLICT re-reads; SELF_APPROVAL changes nothing
  // and is never retried (B-BR-EMP-509).
  async #write(intent: Intent, done: string): Promise<void> {
    if (this.#intent) { this.#refuse('Há uma operação sem confirmação. Tente novamente a mesma operação ou atualize o registro antes de decidir de novo.'); return; }
    const org = this.#org();
    this.#intent = intent;
    const attempt = (): Promise<void> => this.#run(async (epoch, signal) => {
      try {
        if (intent.kind === 'release') await this.source.release(org, intent.id, structuredClone(intent.body), signal);
        else await this.source.decide(org, intent.id, structuredClone(intent.body), signal);
      } catch (error) {
        if (!this.#current(epoch) || !(error instanceof ApiError) || error.kind !== 'conflict') throw error;
        this.#intent = null; this.#retry = null;
        if (error.code === 'SELF_APPROVAL') {
          this.#show({ phase: 'ready', selfApproval: true, conflict: false, message: intent.kind === 'release' ? 'Você registrou este checklist: outro membro com permissão precisa liberar o veículo. Nada foi alterado.' : 'Você registrou esta ocorrência: outro membro com permissão precisa encerrá-la. Nada foi alterado.' });
          return;
        }
        const note = await this.#reread(org, intent, epoch, signal, { conflict: true, selfApproval: false });
        if (this.#current(epoch)) this.#show({ phase: 'ready', conflict: true, message: [CONFLICT, note].filter(Boolean).join(' ') });
        return;
      }
      if (!this.#current(epoch)) return;
      this.#intent = null;
      // The acknowledged write is never replayed; only the read is retried.
      const again = (): Promise<void> => this.#run(async (e, s) => { const note = await this.#reread(org, intent, e, s, {}); if (this.#current(e)) { this.#retry = null; this.#show({ phase: 'ready', stale: false, message: [done, note].filter(Boolean).join(' ') }); } }, false).then(() => { if (this.state.phase === 'failed') this.#retry = again; });
      this.#retry = again;
      this.#show({ conflict: false, selfApproval: false, stale: true, message: done });
      const note = await this.#reread(org, intent, epoch, signal, { conflict: false, selfApproval: false });
      if (!this.#current(epoch)) return;
      this.#retry = null;
      this.#show({ phase: 'ready', message: [done, note].filter(Boolean).join(' ') });
    }, false).then(() => { if (this.#intent === intent && this.state.phase === 'failed') this.#retry = attempt; });
    await attempt();
  }
  async release(reason: string): Promise<void> {
    const run = this.state.run, value = reason.trim();
    if (!run) return;
    if (run.outcome !== 'BLOCKING') { this.#refuse('Somente um checklist que está bloqueando o uso pode ser liberado.'); return; }
    if (!text(value, MAX_NOTE)) { this.#refuse('Informe o motivo da liberação (até 500 caracteres).'); return; }
    await this.#write({ kind: 'release', id: run.id, asset: run.assetId, body: { schema_version: 1, operation_id: crypto.randomUUID(), revision: run.revision, reason: value } }, 'Liberação registrada no servidor.');
  }
  async decide(action: ReportAction, note: string, duplicateOf: string): Promise<void> {
    const report = this.state.report, value = note.trim();
    if (!report) return;
    if (!allowedActions(report.state).includes(action)) { this.#refuse('Esta ação não está disponível na situação atual da ocorrência.'); return; }
    if (action !== 'ACKNOWLEDGE' && !text(value, MAX_NOTE)) { this.#refuse('Informe a ação, a solução ou o motivo (até 500 caracteres).'); return; }
    if (duplicateOf && (action !== 'DISMISS' || !this.duplicates().some(r => r.id === duplicateOf))) { this.#refuse('Escolha a ocorrência original entre as ocorrências listadas do mesmo veículo.'); return; }
    const body: DecisionBody = { schema_version: 1, operation_id: crypto.randomUUID(), revision: report.revision, action, ...(action === 'ACKNOWLEDGE' ? {} : { note: value }), ...(duplicateOf ? { duplicate_of: duplicateOf } : {}) };
    await this.#write({ kind: 'decide', id: report.id, body }, 'Tratamento registrado no servidor.');
  }
  async retry(): Promise<void> { await this.#retry?.(); }
}
