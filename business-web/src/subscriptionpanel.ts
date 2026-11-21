import { METERS, PlanPanelController, planStateLabel, type PlanPanelState, type PlanSource } from './subscriptions.js';

export function mountPlanPanel(doc: Document, source: PlanSource, sessionLost: (message: string) => void): PlanPanelController {
  function element<T extends HTMLElement>(id: string): T {
    const node = doc.getElementById(id);
    if (!node) throw new Error('Organization billing panel is incomplete');
    return node as T;
  }
  const panel = element('subscription-panel');
  const status = element('subscription-status');
  const list = element('subscription-list');
  const refresh = element<HTMLButtonElement>('subscription-refresh');
  const date = new Intl.DateTimeFormat('pt-BR', { dateStyle: 'medium', timeStyle: 'short' });
  const number = new Intl.NumberFormat('pt-BR');

  function text(parent: HTMLElement, tag: 'h3' | 'p' | 'li', content: string): void {
    const node = doc.createElement(tag);
    node.textContent = content;
    parent.appendChild(node);
  }

  function render(state: PlanPanelState): void {
    panel.hidden = state.phase === 'closed';
    panel.setAttribute('aria-busy', String(state.phase === 'loading'));
    status.textContent = state.message;
    refresh.disabled = state.phase === 'loading';
    list.replaceChildren();
    for (const plan of state.plans) {
      const item = doc.createElement('li');
      text(item, 'h3', plan.name);
      text(item, 'p', `${planStateLabel(plan)} · revisão ${number.format(BigInt(plan.revision))}`);
      text(item, 'p', `Vigência de ${date.format(new Date(plan.termStart))} a ${date.format(new Date(plan.termEnd))}`);
      if (plan.confirmedPaid) text(item, 'p', 'Cobrança liquidada para esse período.');
      if (plan.state === 'PAST_DUE' && !plan.confirmedPaid) text(item, 'p', 'Aguardando confirmação da cobrança.');
      if (plan.pending) text(item, 'p', `Próximo plano em análise para ${date.format(new Date(plan.pending.effectiveAt))}: ${plan.pending.name}`);
      if (plan.capabilities.length) {
        const names = plan.capabilities.map(capability => capability === 'organizations' ? 'Operações da empresa' : capability);
        text(item, 'p', `Recursos: ${names.join(', ')}`);
      }
      if (Object.keys(plan.limits).length) {
        const limits = doc.createElement('ul');
        for (const [meter, label] of Object.entries(METERS)) {
          const limit = plan.limits[meter as keyof typeof METERS];
          if (limit !== undefined) text(limits, 'li', `${label}: ${number.format(BigInt(limit))}`);
        }
        item.appendChild(limits);
      }
      list.appendChild(item);
    }
  }

  const controller = new PlanPanelController(source, render, sessionLost);
  refresh.addEventListener('click', () => { void controller.refresh(); });
  return controller;
}
