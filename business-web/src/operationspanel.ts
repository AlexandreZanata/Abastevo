import { type Authority } from './model.js';
import { OperationsController, type OperationsSource, type OperationsState } from './operations.js';

export function mountOperationsPanel(doc: Document, browser: Window, source: OperationsSource, sessionLost?: (message: string) => void): { select(authority?: Authority): void; clear(): void } {
  const el = <T extends HTMLElement>(id: string): T => {
    const node = doc.getElementById(id);
    if (!node) throw new Error('Operations panel is incomplete');
    return node as T;
  };
  let pollTimer = 0;
  const controller = new OperationsController(source, render, sessionLost);
  const txt = (tag: 'li' | 'p' | 'button', value: string): HTMLElement => { const n = doc.createElement(tag); n.textContent = value; return n; };

  function stopPolling(): void { browser.clearInterval(pollTimer); pollTimer = 0; }
  function startPolling(): void {
    stopPolling();
    pollTimer = browser.setInterval(() => {
      if (controller.state.authority && controller.state.phase !== 'closed' && controller.state.authority.permissions?.includes('gps.location.read_live')) {
        void controller.refresh(true);
      }
    }, 15_000);
  }

  function render(state: OperationsState): void {
    const root = el('ops-panel');
    root.hidden = state.phase === 'closed';
    if (state.phase === 'closed') { stopPolling(); controller.activeAdapter().clear(el('ops-map-host')); return; }
    root.setAttribute('aria-busy', String(state.phase === 'loading'));
    el('ops-status').textContent = state.message;
    el('ops-toggle-view').textContent = state.viewMode === 'MAP' ? 'Ver como lista' : 'Ver no mapa';
    controller.activeAdapter().render(el('ops-map-host'), controller.scene(), tripId => { void controller.openReplay(tripId); });

    const alertList = el('ops-alert-list'); alertList.replaceChildren();
    if (state.alerts.length === 0) alertList.append(txt('li', 'Nenhum alerta operacional ativo.'));
    for (const a of state.alerts) {
      alertList.append(txt('li', `${a.kind} · Viagem ${a.tripId.slice(0, 8)} · Observado em ${a.observedAt} (${a.durationSeconds}s / limite ${a.thresholdSeconds}s)`));
    }

    const fenceList = el('ops-geofence-list'); fenceList.replaceChildren();
    if (state.geofences.length === 0) fenceList.append(txt('li', 'Nenhuma cerca cadastrada.'));
    for (const g of state.geofences) {
      fenceList.append(txt('li', `${g.name} · ${g.kind} · ${g.enabled ? 'Ativa' : 'Inativa'}`));
    }

    const replayCard = el('ops-replay-detail');
    replayCard.hidden = state.replay === null;
    if (state.replay) {
      el('ops-replay-summary').textContent = state.replay.quality === 'INSUFFICIENT'
        ? 'Dados insuficientes; exibindo somente o diário da viagem.'
        : (state.replay.rawExpired ? 'Pontos brutos expirados; exibindo geometria simplificada e resumo.' : `${state.replay.points.length} ponto(s) carregado(s) · Distância: ${state.replay.distanceM ?? 0} m`);
      const cursorInput = el<HTMLInputElement>('ops-replay-cursor');
      cursorInput.max = String(Math.max(0, state.replay.points.length - 1));
      cursorInput.value = String(state.replayCursorIndex);
      const pt = state.replay.points[state.replayCursorIndex];
      el('ops-replay-cursor-readout').textContent = pt
        ? `Coletado: ${pt.collectedAt} · Recebido: ${pt.receivedAt} · Precisão: ${pt.accuracyM} m · Velocidade: ${pt.speedCmS ?? 0} cm/s`
        : 'Sem ponto selecionado.';
      const logList = el('ops-replay-logbook'); logList.replaceChildren();
      for (const ev of state.replay.logbook) logList.append(txt('li', `${ev.occurredAt} · ${ev.kind} · ${ev.summary}`));
    }
  }

  el('ops-load').addEventListener('click', () => { void controller.refresh(false); });
  el('ops-toggle-view').addEventListener('click', () => { controller.toggleViewMode(); });
  el('ops-replay-cursor').addEventListener('input', () => { controller.setReplayCursor(Number(el<HTMLInputElement>('ops-replay-cursor').value) || 0); });
  el('ops-geofence-form').addEventListener('submit', ev => {
    ev.preventDefault();
    const id = browser.crypto.randomUUID();
    void controller.createCircleGeofence(id, el<HTMLInputElement>('ops-fence-name').value.trim(),
      Number(el<HTMLInputElement>('ops-fence-lat').value), Number(el<HTMLInputElement>('ops-fence-lon').value),
      Number(el<HTMLInputElement>('ops-fence-radius').value));
  });
  el('ops-route-form').addEventListener('submit', ev => {
    ev.preventDefault();
    const id = browser.crypto.randomUUID();
    const dist = el<HTMLInputElement>('ops-route-distance').value.trim();
    const rev = el<HTMLInputElement>('ops-route-revision').value.trim();
    void controller.saveManualRoute(id, el<HTMLSelectElement>('ops-route-kind').value === 'MISSION' ? 'MISSION' : 'TRIP',
      el<HTMLInputElement>('ops-route-owner').value.trim(),
      el<HTMLInputElement>('ops-route-origin').value.trim(), Number(el<HTMLInputElement>('ops-route-olat').value), Number(el<HTMLInputElement>('ops-route-olon').value),
      el<HTMLInputElement>('ops-route-dest').value.trim(), Number(el<HTMLInputElement>('ops-route-dlat').value), Number(el<HTMLInputElement>('ops-route-dlon').value),
      dist ? Number(dist) : null, rev ? Number(rev) : null);
  });

  return {
    select(authority?: Authority): void { controller.select(authority); if (authority) startPolling(); else stopPolling(); },
    clear(): void { stopPolling(); controller.clear(); },
  };
}
