import { type Authority } from './model.js';
import { IntelligenceController, type IntelligenceSource, type IntelligenceState } from './intelligence.js';

export function mountIntelligencePanel(doc: Document, browser: Window, source: IntelligenceSource, sessionLost?: (message: string) => void): { select(authority?: Authority): void; clear(): void } {
  const el = <T extends HTMLElement>(id: string): T => {
    const node = doc.getElementById(id);
    if (!node) throw new Error('Intelligence panel is incomplete');
    return node as T;
  };
  const controller = new IntelligenceController(source, render, sessionLost);
  const txt = (tag: 'li' | 'p', value: string): HTMLElement => {
    const n = doc.createElement(tag);
    n.textContent = value;
    return n;
  };

  function render(state: IntelligenceState): void {
    const root = el('intel-panel');
    root.hidden = state.phase === 'closed';
    if (state.phase === 'closed') return;
    root.setAttribute('aria-busy', String(state.phase === 'loading'));
    el('intel-status').textContent = state.message;

    const kpiList = el('intel-kpi-list');
    kpiList.replaceChildren();
    if (state.kpis.length === 0) kpiList.append(txt('li', 'Nenhum indicador disponível no recorte.'));
    for (const k of state.kpis) {
      const suffix = k.status === 'COMPUTED'
        ? `${k.displayValue} ${k.unit}${k.sampleSmall ? ' (amostra reduzida)' : ''}`
        : `Sem dados (${k.insufficiencyReason || 'INSUFICIENTE'})`;
      kpiList.append(txt('li', `${k.indicatorId} · ${k.namePt} · ${suffix}`));
    }

    const findingList = el('intel-finding-list');
    findingList.replaceChildren();
    if (state.findings.length === 0) findingList.append(txt('li', 'Nenhum achado pendente ou registrado.'));
    for (const f of state.findings) {
      findingList.append(txt('li', `${f.findingType} · ${f.severity} · ${f.state} · ${f.title} (${f.nonProofNote})`));
    }

    const maintList = el('intel-maint-list');
    maintList.replaceChildren();
    if (state.plans.length === 0 && state.workOrders.length === 0) maintList.append(txt('li', 'Nenhum plano preventivo ou OS ativa.'));
    for (const p of state.plans) {
      maintList.append(txt('li', `Plano: ${p.name} · ${p.occurrenceState || 'UPCOMING'} (${p.decidingTrigger})`));
    }
    for (const w of state.workOrders) {
      maintList.append(txt('li', `OS #${w.number}: ${w.kind} · ${w.state} · ${w.description}${w.holdOpen ? ' [INDISPONÍVEL]' : ''}`));
    }

    const tankList = el('intel-stock-list');
    tankList.replaceChildren();
    if (state.tanks.length === 0) tankList.append(txt('li', 'Nenhum tanque interno cadastrado.'));
    for (const t of state.tanks) {
      tankList.append(txt('li', `${t.name} · ${t.product} · ${(t.quantityMilli / 1000).toFixed(3)} ${t.unit} · R$ ${(t.valueCentavos / 100).toFixed(2)}`));
    }

    const auditList = el('intel-audit-list');
    auditList.replaceChildren();
    if (state.auditEvents.length === 0) auditList.append(txt('li', 'Nenhum evento de auditoria recente.'));
    for (const e of state.auditEvents) {
      auditList.append(txt('li', `${e.occurredAt} · ${e.action} · ${e.targetType} (${e.targetId.slice(0, 8)})`));
    }
  }

  el('intel-load').addEventListener('click', () => { void controller.refresh(); });
  el('intel-detect-btn').addEventListener('click', () => {
    void controller.detectFindings(browser.crypto.randomUUID());
  });
  el('intel-dimension').addEventListener('change', () => {
    const dim = el<HTMLSelectElement>('intel-dimension').value;
    const pending = el<HTMLInputElement>('intel-include-pending').checked;
    controller.setFilters(dim, pending);
  });
  el('intel-include-pending').addEventListener('change', () => {
    const dim = el<HTMLSelectElement>('intel-dimension').value;
    const pending = el<HTMLInputElement>('intel-include-pending').checked;
    controller.setFilters(dim, pending);
  });
  el('intel-tank-form').addEventListener('submit', ev => {
    ev.preventDefault();
    void controller.createTank(
      browser.crypto.randomUUID(),
      el<HTMLInputElement>('intel-tank-name').value.trim(),
      el<HTMLSelectElement>('intel-tank-product').value,
      'LITRE',
      Number(el<HTMLInputElement>('intel-tank-cap').value),
    );
  });
  el('intel-wo-form').addEventListener('submit', ev => {
    ev.preventDefault();
    void controller.createWorkOrder(
      browser.crypto.randomUUID(),
      el<HTMLInputElement>('intel-wo-asset').value.trim(),
      el<HTMLSelectElement>('intel-wo-kind').value,
      el<HTMLInputElement>('intel-wo-desc').value.trim(),
    );
  });

  return {
    select(authority?: Authority): void {
      if (authority) controller.select(authority);
      else controller.clear();
    },
    clear(): void { controller.clear(); },
  };
}
