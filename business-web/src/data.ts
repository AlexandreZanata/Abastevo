import { ApiError, type CompanyTransport } from './api.js';
import { isUuid, record } from './model.js';

export const DATA_KEY_STATES = { ACTIVE: 'Ativa', ROTATING: 'Em rotação', REVOKED: 'Revogada', EXPIRED: 'Expirada' } as const;

export interface DataKey {
  id: string; label: string; revision: number; state: keyof typeof DATA_KEY_STATES;
  expiresAt: string; overlapUntil?: string;
}
interface Dataset {
  key: string; revision: string; name: string; region: string; method: string; minimum: number;
}
export interface DataUsage {
  enabled: boolean;
  term?: { start: string; end: string; used: string; ceiling: string };
  rate?: { seconds: number; calls: string };
  datasets: Dataset[];
  cursor: string;
}
interface KeyTerms { label: string; expiresAt: string }
export type KeyChange =
  | { kind: 'issue'; terms: KeyTerms }
  | { kind: 'rotate'; key: DataKey; terms: KeyTerms; overlapUntil?: string }
  | { kind: 'revoke'; key: DataKey };

function text(value: unknown, maximum: number, allowEmpty = false): string {
  if (typeof value !== 'string' || (!allowEmpty && value.trim() === '') || value.length > maximum) throw new ApiError('unavailable');
  return value;
}
function when(value: unknown): string {
  const instant = text(value, 40);
  if (!Number.isFinite(Date.parse(instant))) throw new ApiError('unavailable');
  return instant;
}
function int64(value: unknown): string {
  const count = text(value, 19);
  if (!/^(0|[1-9][0-9]{0,18})$/.test(count) || BigInt(count) > 9223372036854775807n) throw new ApiError('unavailable');
  return count;
}
function natural(value: unknown): number {
  if (typeof value !== 'number' || !Number.isSafeInteger(value) || value < 1) throw new ApiError('unavailable');
  return value;
}
function path(org: string, suffix: string): string {
  if (!isUuid(org)) throw new ApiError('denied');
  return `/v1/orgs/${org}/data/${suffix}`;
}
function key(raw: unknown): DataKey {
  const row = record(raw);
  if (!isUuid(row.id) || typeof row.state !== 'string' || !Object.hasOwn(DATA_KEY_STATES, row.state) ||
      !Array.isArray(row.scopes) || row.scopes.length !== 1 || row.scopes[0] !== 'regional.read') throw new ApiError('unavailable');
  when(row.issued_at);
  return { id: row.id, label: text(row.label, 100), revision: natural(row.revision), state: row.state as DataKey['state'],
    expiresAt: when(row.expires_at), ...(row.overlap_until !== undefined ? { overlapUntil: when(row.overlap_until) } : {}) };
}
function dataset(value: unknown): Dataset {
  const row = record(value);
  const result = { key: text(row.key, 64), revision: int64(row.revision), name: text(row.name, 120),
    region: text(row.region, 2), method: text(row.method, 60), minimum: natural(row.minimum_stations) };
  if (!/^[a-z][a-z0-9]*(-[a-z0-9]+)*$/.test(result.key) || result.revision === '0' ||
      !/^[A-Z]{2}$/.test(result.region) || result.minimum < 5) throw new ApiError('unavailable');
  return result;
}
export class DataApi {
  constructor(private readonly api: CompanyTransport) {}

  async usage(org: string, cursor: string, signal: AbortSignal): Promise<DataUsage> {
    const view = await this.api.privatePost(path(org, 'usage/read'), { cursor, limit: 20 }, signal);
    when(view.observed_at);
    if (typeof view.enabled !== 'boolean' || !Array.isArray(view.datasets) || view.datasets.length > 20) throw new ApiError('unavailable');
    const result: DataUsage = { enabled: view.enabled, datasets: view.datasets.map(dataset), cursor: text(view.next_cursor, 2048, true) };
    if (view.term !== undefined) {
      const term = record(view.term);
      result.term = { start: when(term.start), end: when(term.end), used: int64(term.used), ceiling: int64(term.ceiling) };
    }
    if (view.rate !== undefined) {
      const rate = record(view.rate);
      result.rate = { seconds: natural(rate.window_seconds), calls: int64(rate.calls) };
    }
    if (result.enabled && !result.term) throw new ApiError('unavailable');
    return result;
  }

  async keys(org: string, cursor: string, signal: AbortSignal): Promise<{ keys: DataKey[]; cursor: string }> {
    const view = await this.api.privatePost(path(org, 'keys/list'), { cursor, limit: 20 }, signal);
    if (!Array.isArray(view.keys) || view.keys.length > 20) throw new ApiError('unavailable');
    return { keys: view.keys.map(key), cursor: text(view.next_cursor, 2048, true) };
  }

  async change(org: string, op: string, change: KeyChange, signal: AbortSignal): Promise<{ key: DataKey; bearer?: string; replayed: boolean }> {
    if (!isUuid(op) || (change.kind !== 'issue' && !isUuid(change.key.id))) throw new ApiError('denied');
    const suffix = change.kind === 'issue' ? 'keys/issue' : `keys/${change.key.id}/${change.kind}`;
    const body = {
      schema_version: 1, operation_id: op,
      ...(change.kind !== 'issue' ? { revision: change.key.revision } : {}),
      ...(change.kind !== 'revoke' ? { label: change.terms.label, scopes: ['regional.read'], expires_at: change.terms.expiresAt } : {}),
      ...(change.kind === 'rotate' && change.overlapUntil ? { overlap_until: change.overlapUntil } : {}),
    };
    const response = await this.api.privatePost(path(org, suffix), body, signal);
    const item = key(response.key);
    if (typeof response.replayed !== 'boolean' || item.id !== (change.kind === 'revoke' ? change.key.id : op)) throw new ApiError('unavailable');
    if (response.replayed || change.kind === 'revoke') {
      if (response.bearer !== undefined) throw new ApiError('unavailable');
      return { key: item, replayed: response.replayed };
    }
    const bearer = text(response.bearer, 110);
    if (!bearer.startsWith(`abv_data_${item.id}.`) || !/^[0-9a-f]{64}$/.test(bearer.slice(-64)) || bearer.length !== 110) throw new ApiError('unavailable');
    return { key: item, bearer, replayed: false };
  }
}
