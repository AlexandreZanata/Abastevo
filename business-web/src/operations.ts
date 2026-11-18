import { ApiError, type CompanyTransport } from './api.js';
import { type Authority, record } from './model.js';
import { CanvasPanelMapAdapter, ListFallbackMapAdapter, type MapGeofenceShape, type MapLiveMarker, type PanelMapPort, type PanelMapScene } from './mapport.js';

function opsErrorMessage(err: unknown): string {
  if (err instanceof ApiError) {
    if (err.code === 'ROUTE_POINT_LIMIT') return 'Limite de pontos excedido (máximo de 100 paradas intermediárias).';
    if (err.code === 'GEOFENCE_INVALID_GEOMETRY') return 'Geometria de cerca geográfica inválida.';
    if (err.code === 'CONFLICT') return 'O registro foi alterado por outra operação. Atualize e tente novamente.';
    if (err.code === 'FRESH_AUTH_REQUIRED') return 'Confirmação recente de acesso necessária para esta alteração.';
    return 'Não foi possível concluir a operação solicitada.';
  }
  return err instanceof Error ? err.message : 'Falha na operação solicitada.';
}

export interface LiveItem {
  tripId: string;
  sessionId: string;
  sessionState: string;
  tripState: string;
  statusNote: string;
  unitId: string;
  unitName: string;
  assetLabel: string;
  driverName: string;
  freshness: 'CURRENT' | 'STALE' | 'NO_SIGNAL';
  latE7: number | null;
  lonE7: number | null;
  accuracyM: number | null;
  coarse: boolean;
  collectedAt: string | null;
  receivedAt: string | null;
  lastCommunicationAt: string | null;
  revision: number;
}

export interface AlertItem {
  id: string;
  kind: string;
  tripId: string;
  geofenceId: string;
  eventKind: string;
  anchorAt: string;
  observedAt: string;
  durationSeconds: number;
  thresholdSeconds: number;
}

export interface ReplayPointItem {
  collectedAt: string;
  receivedAt: string;
  latE7: number;
  lonE7: number;
  accuracyM: number;
  speedCmS: number | null;
  coarse: boolean;
}

export interface ReplayData {
  tripId: string;
  tripState: string;
  rawExpired: boolean;
  trimmedCount: number;
  quality: string;
  distanceM: number | null;
  points: ReplayPointItem[];
  simplifiedLines: Array<Array<{ latE7: number; lonE7: number }>>;
  gaps: Array<{ from: string; to: string; reason: string; detectedBy: string }>;
  logbook: Array<{ kind: string; occurredAt: string; summary: string }>;
}

export interface RoutePlanData {
  revision: number;
  plannedDistanceM: number | null;
  plannedDistanceSource: string | null;
  heavyAssetNotice: boolean;
  waypoints: Array<{ sequence: number; label: string; source: string; lat_e7: number; lon_e7: number }>;
}

export interface OperationsSource {
  listLive(orgId: string, sinceRevision: number): Promise<{ items: LiveItem[]; alerts: AlertItem[]; maxRevision: number }>;
  readReplay(orgId: string, tripId: string): Promise<ReplayData>;
  listGeofences(orgId: string): Promise<{ geofences: MapGeofenceShape[]; events: Array<{ id: string; geofenceName: string; eventKind: string; fixAt: string; detectedAt: string; lateSuppressed: boolean }>; alerts: AlertItem[] }>;
  createGeofence(orgId: string, payload: Record<string, unknown>): Promise<void>;
  readRoutePlan(orgId: string, ownerKind: 'MISSION' | 'TRIP', ownerId: string): Promise<{ plan: RoutePlanData | null }>;
  saveRoutePlan(orgId: string, payload: Record<string, unknown>): Promise<{ plan: RoutePlanData }>;
}

export interface OperationsState {
  phase: 'closed' | 'idle' | 'loading' | 'ready' | 'failed';
  message: string;
  authority: Authority | null;
  viewMode: 'MAP' | 'LIST';
  liveItems: LiveItem[];
  alerts: AlertItem[];
  geofences: MapGeofenceShape[];
  geofenceEvents: Array<{ id: string; geofenceName: string; eventKind: string; fixAt: string; detectedAt: string; lateSuppressed: boolean }>;
  selectedTripId: string;
  replay: ReplayData | null;
  replayCursorIndex: number;
  routePlan: RoutePlanData | null;
  maxRevision: number;
}

function asNum(v: unknown): number | null { return typeof v === 'number' && Number.isFinite(v) ? v : null; }
function asStr(v: unknown): string { return typeof v === 'string' ? v : ''; }

export class OperationsApi implements OperationsSource {
  constructor(private readonly api: CompanyTransport) {}

  async listLive(orgId: string, sinceRevision: number): Promise<{ items: LiveItem[]; alerts: AlertItem[]; maxRevision: number }> {
    const raw = record(await this.api.privatePost(`/v1/orgs/${orgId}/tracking/live/list`, { since_revision: sinceRevision, limit: 100 }));
    const items = (Array.isArray(raw.items) ? raw.items : []).map(row => {
      const r = record(row);
      const fr = asStr(r.freshness);
      return {
        tripId: asStr(r.trip_id), sessionId: asStr(r.session_id), sessionState: asStr(r.session_state),
        tripState: asStr(r.trip_state), statusNote: asStr(r.status_note), unitId: asStr(r.unit_id),
        unitName: asStr(r.unit_name), assetLabel: asStr(r.asset_label), driverName: asStr(r.driver_name),
        freshness: (fr === 'CURRENT' || fr === 'STALE' ? fr : 'NO_SIGNAL') as LiveItem['freshness'],
        latE7: asNum(r.lat_e7), lonE7: asNum(r.lon_e7), accuracyM: asNum(r.accuracy_m),
        coarse: r.coarse === true,
        collectedAt: typeof r.collected_at === 'string' ? r.collected_at : null,
        receivedAt: typeof r.received_at === 'string' ? r.received_at : null,
        lastCommunicationAt: typeof r.last_communication_at === 'string' ? r.last_communication_at : null,
        revision: asNum(r.revision) ?? 0,
      };
    });
    const alerts = (Array.isArray(raw.alerts) ? raw.alerts : []).map(row => {
      const r = record(row);
      return {
        id: asStr(r.id), kind: asStr(r.kind), tripId: asStr(r.trip_id), geofenceId: asStr(r.geofence_id),
        eventKind: asStr(r.event_kind), anchorAt: asStr(r.anchor_at), observedAt: asStr(r.observed_at),
        durationSeconds: asNum(r.duration_seconds) ?? 0, thresholdSeconds: asNum(r.threshold_seconds) ?? 0,
      };
    });
    return { items, alerts, maxRevision: asNum(raw.max_revision) ?? sinceRevision };
  }

  async readReplay(orgId: string, tripId: string): Promise<ReplayData> {
    const raw = record(await this.api.privatePost(`/v1/orgs/${orgId}/trips/${tripId}/track/replay`, {}));
    const sm = record(raw.summary);
    const points = (Array.isArray(raw.points) ? raw.points : []).map(pt => {
      const p = record(pt);
      return {
        collectedAt: asStr(p.collected_at), receivedAt: asStr(p.received_at),
        latE7: asNum(p.lat_e7) ?? 0, lonE7: asNum(p.lon_e7) ?? 0,
        accuracyM: asNum(p.accuracy_m) ?? 0, speedCmS: asNum(p.speed_cm_s), coarse: p.coarse === true,
      };
    });
    const lines = (Array.isArray(raw.simplified_lines) ? raw.simplified_lines : []).map(part =>
      (Array.isArray(part) ? part : []).map(xy => Array.isArray(xy) ? { latE7: asNum(xy[0]) ?? 0, lonE7: asNum(xy[1]) ?? 0 } : { latE7: 0, lonE7: 0 }),
    );
    const gaps = (Array.isArray(raw.gaps) ? raw.gaps : []).map(g => {
      const r = record(g);
      return { from: asStr(r.from), to: asStr(r.to), reason: asStr(r.reason), detectedBy: asStr(r.detected_by) };
    });
    const logbook = (Array.isArray(raw.logbook) ? raw.logbook : []).map(l => {
      const r = record(l);
      return { kind: asStr(r.kind), occurredAt: asStr(r.occurred_at), summary: asStr(r.summary) };
    });
    return {
      tripId: asStr(raw.trip_id), tripState: asStr(raw.trip_state),
      rawExpired: raw.raw_expired === true, trimmedCount: asNum(raw.trimmed_count) ?? 0,
      quality: asStr(sm.quality), distanceM: asNum(sm.distance_m),
      points, simplifiedLines: lines, gaps, logbook,
    };
  }

  async listGeofences(orgId: string) {
    const raw = record(await this.api.privatePost(`/v1/orgs/${orgId}/tracking/geofences/list`, {}));
    const geofences: MapGeofenceShape[] = (Array.isArray(raw.geofences) ? raw.geofences : []).map(item => {
      const r = record(item);
      return {
        id: asStr(r.id), name: asStr(r.name), kind: r.kind === 'POLYGON' ? 'POLYGON' : 'CIRCLE',
        centerLatE7: asNum(r.center_lat_e7), centerLonE7: asNum(r.center_lon_e7), radiusM: asNum(r.radius_m),
        vertices: (Array.isArray(r.vertices) ? r.vertices : []).map(v => ({ latE7: asNum(record(v).lat_e7) ?? 0, lonE7: asNum(record(v).lon_e7) ?? 0 })),
        enabled: r.enabled === true,
      };
    });
    const events = (Array.isArray(raw.events) ? raw.events : []).map(item => {
      const r = record(item);
      return {
        id: asStr(r.id), geofenceName: asStr(r.geofence_name), eventKind: asStr(r.event_kind),
        fixAt: asStr(r.fix_at), detectedAt: asStr(r.detected_at), lateSuppressed: r.late_suppressed === true,
      };
    });
    const alerts = (Array.isArray(raw.alerts) ? raw.alerts : []).map(row => {
      const r = record(row);
      return {
        id: asStr(r.id), kind: asStr(r.kind), tripId: asStr(r.trip_id), geofenceId: asStr(r.geofence_id),
        eventKind: asStr(r.event_kind), anchorAt: asStr(r.anchor_at), observedAt: asStr(r.observed_at),
        durationSeconds: asNum(r.duration_seconds) ?? 0, thresholdSeconds: asNum(r.threshold_seconds) ?? 0,
      };
    });
    return { geofences, events, alerts };
  }

  async createGeofence(orgId: string, payload: Record<string, unknown>): Promise<void> {
    await this.api.privatePost(`/v1/orgs/${orgId}/tracking/geofences/create`, payload);
  }

  async readRoutePlan(orgId: string, ownerKind: 'MISSION' | 'TRIP', ownerId: string): Promise<{ plan: RoutePlanData | null }> {
    const raw = record(await this.api.privatePost(`/v1/orgs/${orgId}/routes/plans/read`, { owner_kind: ownerKind, owner_id: ownerId }));
    if (!raw.plan) return { plan: null };
    const p = record(raw.plan);
    return {
      plan: {
        revision: asNum(p.revision) ?? 1,
        plannedDistanceM: asNum(p.planned_distance_m),
        plannedDistanceSource: typeof p.planned_distance_source === 'string' ? p.planned_distance_source : null,
        heavyAssetNotice: p.heavy_asset_notice === true,
        waypoints: Array.isArray(p.waypoints) ? p.waypoints as RoutePlanData['waypoints'] : [],
      },
    };
  }

  async saveRoutePlan(orgId: string, payload: Record<string, unknown>): Promise<{ plan: RoutePlanData }> {
    const raw = record(await this.api.privatePost(`/v1/orgs/${orgId}/routes/plans/save`, payload));
    const p = record(raw.plan);
    return {
      plan: {
        revision: asNum(p.revision) ?? 1,
        plannedDistanceM: asNum(p.planned_distance_m),
        plannedDistanceSource: typeof p.planned_distance_source === 'string' ? p.planned_distance_source : null,
        heavyAssetNotice: p.heavy_asset_notice === true,
        waypoints: Array.isArray(p.waypoints) ? p.waypoints as RoutePlanData['waypoints'] : [],
      },
    };
  }
}

export class OperationsController {
  state: OperationsState = {
    phase: 'closed', message: '', authority: null, viewMode: 'MAP',
    liveItems: [], alerts: [], geofences: [], geofenceEvents: [],
    selectedTripId: '', replay: null, replayCursorIndex: 0, routePlan: null, maxRevision: 0,
  };
  private mapAdapter: PanelMapPort;
  private readonly fallbackAdapter = new ListFallbackMapAdapter();

  constructor(
    private readonly source: OperationsSource,
    private readonly onRender: (s: OperationsState) => void,
    private readonly sessionLost?: (message: string) => void,
    initialAdapter: PanelMapPort = new CanvasPanelMapAdapter(),
  ) {
    this.mapAdapter = initialAdapter;
  }

  setMapAdapter(adapter: PanelMapPort): void {
    this.mapAdapter = adapter;
    this.state.viewMode = adapter.mode === 'LIST' ? 'LIST' : 'MAP';
    this.onRender(this.state);
  }

  activeAdapter(): PanelMapPort {
    return this.state.viewMode === 'LIST' ? this.fallbackAdapter : this.mapAdapter;
  }

  scene(): PanelMapScene {
    const markers: MapLiveMarker[] = this.state.liveItems.map(it => ({
      tripId: it.tripId,
      label: it.assetLabel || it.driverName || it.tripId.slice(0, 8),
      freshness: it.freshness,
      coarse: it.coarse,
      latE7: it.latE7,
      lonE7: it.lonE7,
      accuracyM: it.accuracyM,
      collectedAt: it.collectedAt,
      lastCommunicationAt: it.lastCommunicationAt,
      statusNote: it.statusNote || null,
    }));
    const trackParts: PanelMapScene['trackParts'] = [];
    let cursorPoint: PanelMapScene['cursorPoint'] = null;
    if (this.state.replay) {
      if (this.state.replay.points.length > 0 && this.state.replay.quality !== 'INSUFFICIENT') {
        trackParts.push({ points: this.state.replay.points.map(p => ({ latE7: p.latE7, lonE7: p.lonE7 })) });
        const pt = this.state.replay.points[Math.min(this.state.replayCursorIndex, this.state.replay.points.length - 1)];
        if (pt) cursorPoint = { latE7: pt.latE7, lonE7: pt.lonE7 };
      } else if (this.state.replay.simplifiedLines.length > 0 && this.state.replay.quality !== 'INSUFFICIENT') {
        for (const part of this.state.replay.simplifiedLines) trackParts.push({ points: part, dashed: true });
      }
    }
    return {
      markers,
      geofences: this.state.geofences,
      trackParts,
      selectedTripId: this.state.selectedTripId,
      cursorPoint,
      attribution: '© OpenStreetMap contributors',
    };
  }

  select(authority?: Authority): void {
    if (!authority) { this.clear(); return; }
    const perms = authority.permissions ?? [];
    const allowed = perms.includes('gps.location.read_live') || perms.includes('gps.track.read') || perms.includes('gps.geofence.manage') || perms.includes('route.plan.manage');
    if (!allowed) { this.clear(); return; }
    this.state = { ...this.state, phase: 'idle', authority, message: 'Selecione atualizar para carregar a operação ao vivo.', liveItems: [], alerts: [], geofences: [], geofenceEvents: [], selectedTripId: '', replay: null, routePlan: null, maxRevision: 0 };
    this.onRender(this.state);
  }

  clear(): void {
    this.state = { phase: 'closed', message: '', authority: null, viewMode: 'MAP', liveItems: [], alerts: [], geofences: [], geofenceEvents: [], selectedTripId: '', replay: null, replayCursorIndex: 0, routePlan: null, maxRevision: 0 };
    this.onRender(this.state);
  }

  toggleViewMode(): void {
    this.state = { ...this.state, viewMode: this.state.viewMode === 'MAP' ? 'LIST' : 'MAP' };
    this.onRender(this.state);
  }

  async refresh(incremental = false): Promise<void> {
    const auth = this.state.authority;
    if (!auth) return;
    const perms = auth.permissions ?? [];
    this.state = { ...this.state, phase: 'loading', message: 'Atualizando operação ao vivo…' };
    this.onRender(this.state);
    try {
      let liveItems = this.state.liveItems;
      let alerts = this.state.alerts;
      let maxRevision = this.state.maxRevision;
      if (perms.includes('gps.location.read_live')) {
        const res = await this.source.listLive(auth.organization.id, incremental ? maxRevision : 0);
        if (incremental && maxRevision > 0) {
          const merged = new Map(liveItems.map(i => [i.tripId, i]));
          for (const item of res.items) merged.set(item.tripId, item);
          liveItems = Array.from(merged.values());
        } else {
          liveItems = res.items;
        }
        alerts = res.alerts;
        maxRevision = Math.max(maxRevision, res.maxRevision);
      }
      let geofences = this.state.geofences;
      let geofenceEvents = this.state.geofenceEvents;
      if (perms.includes('gps.geofence.manage')) {
        const gRes = await this.source.listGeofences(auth.organization.id);
        geofences = gRes.geofences;
        geofenceEvents = gRes.events;
        if (alerts.length === 0) alerts = gRes.alerts;
      }
      this.state = { ...this.state, phase: 'ready', message: 'Operação ao vivo atualizada.', liveItems, alerts, geofences, geofenceEvents, maxRevision };
    } catch (err) {
      if (err instanceof ApiError && err.kind === 'unauthenticated' && this.sessionLost) {
        this.sessionLost(opsErrorMessage(err));
        return;
      }
      this.state = { ...this.state, phase: 'failed', message: opsErrorMessage(err) };
    }
    this.onRender(this.state);
  }

  async openReplay(tripId: string): Promise<void> {
    const auth = this.state.authority;
    if (!auth) return;
    this.state = { ...this.state, selectedTripId: tripId, phase: 'loading', message: 'Carregando trajeto da viagem…' };
    this.onRender(this.state);
    try {
      const replay = await this.source.readReplay(auth.organization.id, tripId);
      let msg = 'Trajeto carregado.';
      if (replay.quality === 'INSUFFICIENT') msg = 'Dados insuficientes para reconstrução do trajeto; exibindo apenas o diário.';
      else if (replay.rawExpired) msg = 'Prazo de retenção de pontos encerrado; exibindo apenas geometria simplificada e resumo.';
      this.state = { ...this.state, phase: 'ready', replay, replayCursorIndex: 0, message: msg };
    } catch (err) {
      this.state = { ...this.state, phase: 'failed', replay: null, message: err instanceof Error ? err.message : 'Falha ao carregar replay.' };
    }
    this.onRender(this.state);
  }

  setReplayCursor(index: number): void {
    if (!this.state.replay || this.state.replay.points.length === 0) return;
    const bounded = Math.max(0, Math.min(index, this.state.replay.points.length - 1));
    this.state = { ...this.state, replayCursorIndex: bounded };
    this.onRender(this.state);
  }

  async createCircleGeofence(operationId: string, name: string, centerLatE7: number, centerLonE7: number, radiusM: number): Promise<void> {
    const auth = this.state.authority;
    if (!auth) return;
    this.state = { ...this.state, phase: 'loading', message: 'Salvando cerca geográfica…' };
    this.onRender(this.state);
    try {
      await this.source.createGeofence(auth.organization.id, {
        schema_version: 1, operation_id: operationId, name, kind: 'CIRCLE',
        center_lat_e7: centerLatE7, center_lon_e7: centerLonE7, radius_m: radiusM,
        on_enter: true, on_exit: true, dwell_seconds: 0, enabled: true,
      });
      await this.refresh(false);
    } catch (err) {
      this.state = { ...this.state, phase: 'failed', message: err instanceof Error ? err.message : 'Falha ao salvar cerca geográfica.' };
      this.onRender(this.state);
    }
  }

  async saveManualRoute(operationId: string, ownerKind: 'MISSION' | 'TRIP', ownerId: string, originLabel: string, oLat: number, oLon: number, destLabel: string, dLat: number, dLon: number, distanceM: number | null, revision: number | null): Promise<void> {
    const auth = this.state.authority;
    if (!auth) return;
    this.state = { ...this.state, phase: 'loading', message: 'Salvando plano de rota…' };
    this.onRender(this.state);
    try {
      const res = await this.source.saveRoutePlan(auth.organization.id, {
        schema_version: 1, operation_id: operationId, revision, owner_kind: ownerKind, owner_id: ownerId,
        waypoints: [
          { sequence: 1, label: originLabel, source: 'COORDINATES', lat_e7: oLat, lon_e7: oLon },
          { sequence: 2, label: destLabel, source: 'COORDINATES', lat_e7: dLat, lon_e7: dLon },
        ],
        options: { avoid_toll: false, avoid_motorway: false, avoid_ferry: false, avoid_unpaved: false },
        planned_distance_m: distanceM, planned_distance_source: distanceM ? 'MANUAL' : null, heavy_asset_notice: false,
      });
      this.state = { ...this.state, phase: 'ready', routePlan: res.plan, message: `Plano de rota revisão ${res.plan.revision} salvo.` };
    } catch (err) {
      this.state = { ...this.state, phase: 'failed', message: err instanceof Error ? err.message : 'Falha ao salvar plano de rota.' };
    }
    this.onRender(this.state);
  }
}
