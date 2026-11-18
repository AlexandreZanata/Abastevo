export interface MapLiveMarker {
  tripId: string;
  label: string;
  freshness: 'CURRENT' | 'STALE' | 'NO_SIGNAL';
  coarse: boolean;
  latE7: number | null;
  lonE7: number | null;
  accuracyM: number | null;
  collectedAt: string | null;
  lastCommunicationAt: string | null;
  statusNote: string | null;
}

export interface MapGeofenceShape {
  id: string;
  name: string;
  kind: 'CIRCLE' | 'POLYGON';
  centerLatE7: number | null;
  centerLonE7: number | null;
  radiusM: number | null;
  vertices: Array<{ latE7: number; lonE7: number }>;
  enabled: boolean;
}

export interface MapTrackPart {
  points: Array<{ latE7: number; lonE7: number }>;
  dashed?: boolean;
}

export interface PanelMapScene {
  markers: MapLiveMarker[];
  geofences: MapGeofenceShape[];
  trackParts: MapTrackPart[];
  selectedTripId: string;
  cursorPoint: { latE7: number; lonE7: number } | null;
  attribution: string;
}

export interface PanelMapPort {
  readonly mode: 'CANVAS' | 'LIST';
  render(container: HTMLElement, scene: PanelMapScene, onSelectTrip: (tripId: string) => void): void;
  clear(container: HTMLElement): void;
}

function textEl(doc: Document, tag: 'p' | 'ul' | 'li' | 'button' | 'span' | 'canvas', text = ''): HTMLElement {
  const n = doc.createElement(tag);
  if (text !== '') n.textContent = text;
  return n;
}

const FRESH_COPY: Record<string, string> = {
  CURRENT: 'Atualizada',
  STALE: 'Defasada',
  NO_SIGNAL: 'Sem sinal recente',
};

export class ListFallbackMapAdapter implements PanelMapPort {
  readonly mode = 'LIST' as const;
  render(container: HTMLElement, scene: PanelMapScene, onSelectTrip: (tripId: string) => void): void {
    const doc = container.ownerDocument;
    const items: HTMLElement[] = [];
    const list = textEl(doc, 'ul') as HTMLUListElement;
    list.className = 'record-list';
    list.setAttribute('aria-label', 'Operações ao vivo em lista');
    if (scene.markers.length === 0) {
      list.append(textEl(doc, 'li', 'Nenhuma operação ativa no escopo selecionado.'));
    }
    for (const m of scene.markers) {
      const li = doc.createElement('li');
      const precision = m.coarse ? 'Área aproximada' : (m.accuracyM !== null ? `Precisão ${m.accuracyM} m` : 'Sem coordenada');
      const note = m.statusNote ? ` · ${m.statusNote}` : '';
      const summary = `${m.label} · ${FRESH_COPY[m.freshness] ?? m.freshness}${note} · ${precision} · Coleta: ${m.collectedAt ?? 'não recebida'} · Comunicação: ${m.lastCommunicationAt ?? 'não recebida'}`;
      li.append(textEl(doc, 'p', summary));
      const btn = textEl(doc, 'button', m.tripId === scene.selectedTripId ? 'Viagem selecionada' : 'Abrir viagem') as HTMLButtonElement;
      btn.type = 'button';
      btn.className = 'button-quiet';
      btn.addEventListener('click', () => onSelectTrip(m.tripId));
      li.append(btn);
      list.append(li);
    }
    items.push(list);
    if (scene.geofences.length > 0) {
      items.push(textEl(doc, 'p', `Cercas no mapa: ${scene.geofences.map(g => `${g.name} (${g.kind === 'CIRCLE' ? 'círculo' : 'polígono'})`).join(', ')}`));
    }
    if (scene.attribution) items.push(textEl(doc, 'p', scene.attribution));
    container.replaceChildren(...items);
  }
  clear(container: HTMLElement): void { container.replaceChildren(); }
}

export class CanvasPanelMapAdapter implements PanelMapPort {
  readonly mode = 'CANVAS' as const;
  private readonly hitBoxes: Array<{ tripId: string; x: number; y: number }> = [];
  private readonly fallback = new ListFallbackMapAdapter();

  render(container: HTMLElement, scene: PanelMapScene, onSelectTrip: (tripId: string) => void): void {
    const doc = container.ownerDocument;
    const canvas = doc.createElement('canvas');
    canvas.width = 760;
    canvas.height = 300;
    canvas.className = 'ops-canvas';
    canvas.setAttribute('role', 'img');
    canvas.setAttribute('aria-label', 'Mapa operacional de posições ao vivo, trajetos e cercas geográficas');
    let ctx: CanvasRenderingContext2D | null = null;
    try { ctx = canvas.getContext('2d'); } catch { ctx = null; }
    if (!ctx) {
      this.fallback.render(container, scene, onSelectTrip);
      return;
    }

    const pts: Array<{ lat: number; lon: number }> = [];
    for (const m of scene.markers) if (m.latE7 !== null && m.lonE7 !== null) pts.push({ lat: m.latE7, lon: m.lonE7 });
    for (const part of scene.trackParts) for (const p of part.points) pts.push({ lat: p.latE7, lon: p.lonE7 });
    for (const g of scene.geofences) {
      if (g.centerLatE7 !== null && g.centerLonE7 !== null) pts.push({ lat: g.centerLatE7, lon: g.centerLonE7 });
      for (const v of g.vertices) pts.push({ lat: v.latE7, lon: v.lonE7 });
    }
    if (scene.cursorPoint) pts.push({ lat: scene.cursorPoint.latE7, lon: scene.cursorPoint.lonE7 });

    let minLat = -236000000, maxLat = -234000000, minLon = -467000000, maxLon = -465000000;
    if (pts.length > 0) {
      minLat = Math.min(...pts.map(p => p.lat)) - 20000;
      maxLat = Math.max(...pts.map(p => p.lat)) + 20000;
      minLon = Math.min(...pts.map(p => p.lon)) - 20000;
      maxLon = Math.max(...pts.map(p => p.lon)) + 20000;
    }
    const pad = 28, w = canvas.width - pad * 2, h = canvas.height - pad * 2;
    const spanLon = Math.max(1000, maxLon - minLon);
    const spanLat = Math.max(1000, maxLat - minLat);
    const proj = (latE7: number, lonE7: number): [number, number] => [
      pad + ((lonE7 - minLon) / spanLon) * w,
      pad + (1 - (latE7 - minLat) / spanLat) * h,
    ];

    ctx.fillStyle = '#f5f7fb';
    ctx.fillRect(0, 0, canvas.width, canvas.height);
    ctx.strokeStyle = '#dde3ee';
    ctx.lineWidth = 1;
    for (let i = 1; i <= 4; i++) {
      const y = (canvas.height / 5) * i;
      ctx.beginPath(); ctx.moveTo(0, y); ctx.lineTo(canvas.width, y); ctx.stroke();
    }

    for (const g of scene.geofences) {
      ctx.strokeStyle = g.enabled ? '#2b59c3' : '#8a94a6';
      ctx.fillStyle = g.enabled ? '#2b59c31a' : '#8a94a61a';
      ctx.lineWidth = 1.5;
      if (g.kind === 'CIRCLE' && g.centerLatE7 !== null && g.centerLonE7 !== null) {
        const [cx, cy] = proj(g.centerLatE7, g.centerLonE7);
        const r = Math.max(14, Math.min(80, Math.round((g.radiusM ?? 300) / 30)));
        ctx.beginPath(); ctx.arc(cx, cy, r, 0, Math.PI * 2); ctx.fill(); ctx.stroke();
      } else if (g.kind === 'POLYGON' && g.vertices.length >= 3) {
        ctx.beginPath();
        g.vertices.forEach((v, idx) => {
          const [vx, vy] = proj(v.latE7, v.lonE7);
          if (idx === 0) ctx!.moveTo(vx, vy); else ctx!.lineTo(vx, vy);
        });
        ctx.closePath(); ctx.fill(); ctx.stroke();
      }
    }

    for (const part of scene.trackParts) {
      if (part.points.length < 2) continue;
      ctx.strokeStyle = part.dashed ? '#b45309' : '#0f766e';
      ctx.lineWidth = 2.5;
      ctx.beginPath();
      part.points.forEach((p, idx) => {
        const [x, y] = proj(p.latE7, p.lonE7);
        if (idx === 0) ctx!.moveTo(x, y); else ctx!.lineTo(x, y);
      });
      ctx.stroke();
    }

    this.hitBoxes.length = 0;
    for (const m of scene.markers) {
      if (m.latE7 === null || m.lonE7 === null) continue;
      const [x, y] = proj(m.latE7, m.lonE7);
      this.hitBoxes.push({ tripId: m.tripId, x, y });
      if (m.coarse) {
        ctx.fillStyle = '#d9770633';
        ctx.strokeStyle = '#d97706';
        ctx.beginPath(); ctx.arc(x, y, 22, 0, Math.PI * 2); ctx.fill(); ctx.stroke();
      } else {
        ctx.fillStyle = m.freshness === 'CURRENT' ? '#15803d' : (m.freshness === 'STALE' ? '#b45309' : '#64748b');
        ctx.beginPath(); ctx.arc(x, y, m.tripId === scene.selectedTripId ? 8 : 6, 0, Math.PI * 2); ctx.fill();
      }
      ctx.fillStyle = '#0f172a';
      ctx.font = '12px sans-serif';
      ctx.fillText(m.label, x + 10, y + 4);
    }

    if (scene.cursorPoint) {
      const [cx, cy] = proj(scene.cursorPoint.latE7, scene.cursorPoint.lonE7);
      ctx.strokeStyle = '#9d2038';
      ctx.lineWidth = 2;
      ctx.beginPath(); ctx.arc(cx, cy, 9, 0, Math.PI * 2); ctx.stroke();
    }

    canvas.addEventListener('click', ev => {
      const rect = canvas.getBoundingClientRect();
      const x = ((ev.clientX - rect.left) / Math.max(1, rect.width)) * canvas.width;
      const y = ((ev.clientY - rect.top) / Math.max(1, rect.height)) * canvas.height;
      for (const box of this.hitBoxes) {
        if (Math.hypot(box.x - x, box.y - y) <= 18) { onSelectTrip(box.tripId); break; }
      }
    });

    const note = textEl(doc, 'p', scene.markers.length === 0 ? 'Sem posições ativas para desenhar.' : `${scene.markers.length} viagem(ns) no mapa.`);
    note.className = 'field-help';
    const attr = textEl(doc, 'p', scene.attribution);
    attr.className = 'field-help';
    container.replaceChildren(canvas, note, attr);
  }

  clear(container: HTMLElement): void {
    this.hitBoxes.length = 0;
    container.replaceChildren();
  }
}
