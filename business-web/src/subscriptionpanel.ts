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
  const currency = new Intl.NumberFormat('pt-BR', { style: 'currency', currency: 'BRL' });
  const paymentDate = new Intl.DateTimeFormat('pt-BR', { dateStyle: 'short', timeZone: 'UTC' });
  function formatAmount(raw: string): string {
    const cents = BigInt(raw);
    const fraction = String(cents % 100n).padStart(2, '0');
    return currency.formatToParts(cents / 100n).map(part => part.type === 'fraction' ? fraction : part.value).join('');
  }

  function text(parent: HTMLElement, tag: 'h3' | 'p' | 'li', content: string): void {
    const node = doc.createElement(tag);
    node.textContent = content;
    parent.appendChild(node);
  }

  function renderVehicleSelection(parent: HTMLElement, basis: NonNullable<PlanPanelState['plans'][number]['vehicleSelection']>, state: PlanPanelState): void {
    const edit = state.vehicleEdit;
    text(parent, 'p', `${number.format(basis.active)} veículos ativos. O novo limite é ${number.format(basis.ceiling)}.`);
    if (!state.canChooseVehicles) return;
    if (!edit || edit.basis !== basis) {
      const open = doc.createElement('button');
      open.type = 'button';
      open.textContent = 'Escolher veículos ativos';
      open.addEventListener('click', () => { void controller.chooseVehicles(basis); });
      parent.appendChild(open);
      return;
    }

    const form = doc.createElement('form');
    const status = doc.createElement('p');
    status.setAttribute('role', 'status');
    status.textContent = edit.message;
    form.appendChild(status);
    const close = doc.createElement('button');
    close.type = 'button';
    close.textContent = 'Cancelar seleção';
    close.disabled = edit.busy;
    close.addEventListener('click', () => controller.cancelVehicleSelection());
    if (edit.options.length > 0) {
      const choices = doc.createElement('fieldset');
      const legend = doc.createElement('legend');
      legend.textContent = `Selecione ${number.format(basis.ceiling)} para manter em uso`;
      choices.appendChild(legend);
      choices.disabled = edit.busy;
      const counters = doc.createElement('p');
      counters.setAttribute('role', 'status');
      const commit = doc.createElement('button');
      commit.type = 'submit';
      commit.textContent = 'Confirmar veículos ativos';
      const selected = new Set(edit.keep);
      const showCount = (): void => {
        counters.textContent = `${number.format(selected.size)} de ${number.format(basis.ceiling)} selecionados`;
        commit.disabled = edit.busy || selected.size !== basis.ceiling;
      };
      for (const asset of edit.options) {
        const row = doc.createElement('p');
        const label = doc.createElement('label');
        const input = doc.createElement('input');
        input.type = 'checkbox';
        input.checked = selected.has(asset.id);
        input.addEventListener('change', () => {
          if (input.checked) selected.add(asset.id);
          else selected.delete(asset.id);
          showCount();
        });
        label.append(input, doc.createTextNode(` ${asset.name}`));
        row.appendChild(label);
        choices.appendChild(row);
      }
      text(choices, 'p', 'Os demais veículos saem do uso ativo e mantêm o histórico.');
      form.append(choices, counters, commit);
      form.addEventListener('submit', event => {
        event.preventDefault();
        void controller.confirmVehicles([...selected]);
      });
      showCount();
    }
    form.appendChild(close);
    parent.appendChild(form);
  }

  function render(state: PlanPanelState): void {
    panel.hidden = state.phase === 'closed';
    panel.setAttribute('aria-busy', String(state.phase === 'loading'));
    status.textContent = state.message;
    refresh.disabled = state.phase === 'loading' || state.vehicleEdit?.busy === true;
    list.replaceChildren();
    for (const plan of state.plans) {
      const item = doc.createElement('li');
      text(item, 'h3', plan.name);
      text(item, 'p', `${planStateLabel(plan)} · revisão ${number.format(BigInt(plan.revision))}`);
      text(item, 'p', `Vigência de ${date.format(new Date(plan.termStart))} a ${date.format(new Date(plan.termEnd))}`);
      if (plan.receipt) {
        text(item, 'p', `Cobrança liquidada: ${formatAmount(plan.receipt.amountCentavos)}.`);
        if (plan.receipt.paymentDate) text(item, 'p', `Data informada pelo provedor: ${paymentDate.format(new Date(plan.receipt.paymentDate))}.`);
        text(item, 'p', 'Situação fiscal não consultada.');
      } else if (plan.confirmedPaid) text(item, 'p', 'Cobrança liquidada para esse período.');
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
          if (limit === undefined) continue;
          const maximum = number.format(BigInt(limit));
          if (meter === 'storage_bytes' && plan.storageUsedBytes !== undefined) {
            const used = number.format(BigInt(plan.storageUsedBytes));
            text(limits, 'li', `${used} de ${maximum} bytes de armazenamento`);
          } else if (meter === 'active_vehicles' && plan.activeVehiclesUsed !== undefined) {
            text(limits, 'li', `${label}: ${number.format(BigInt(plan.activeVehiclesUsed))} de ${maximum}`);
          } else if (meter === 'active_stations' && plan.activeStationsUsed !== undefined) {
            text(limits, 'li', `${label}: ${number.format(BigInt(plan.activeStationsUsed))} de ${maximum}`);
          } else {
            text(limits, 'li', `${label}: ${maximum}`);
          }
        }
        item.appendChild(limits);
      }
      if (plan.vehicleSelection) renderVehicleSelection(item, plan.vehicleSelection, state);
      list.appendChild(item);
    }
  }

  const controller = new PlanPanelController(source, render, sessionLost);
  refresh.addEventListener('click', () => { void controller.refresh(); });
  return controller;
}
