import { type Authority } from './model.js';
import { AgreementsController, type AgreementsSource, type AgreementsState } from './agreements.js';

export function mountAgreementsPanel(doc: Document, browser: Window, source: AgreementsSource, sessionLost?: (message: string) => void): { select(authority?: Authority): void; clear(): void } {
  const el = <T extends HTMLElement>(id: string): T => {
    const node = doc.getElementById(id);
    if (!node) throw new Error('Agreements panel is incomplete');
    return node as T;
  };
  const controller = new AgreementsController(source, render, sessionLost);
  const txt = (tag: 'li' | 'p', value: string): HTMLElement => {
    const n = doc.createElement(tag);
    n.textContent = value;
    return n;
  };

  function render(state: AgreementsState): void {
    const root = el('agreements-panel');
    root.hidden = state.phase === 'closed';
    if (state.phase === 'closed') return;
    root.setAttribute('aria-busy', String(state.phase === 'loading'));
    el('agreements-status').textContent = state.message;

    const agreementList = el('agreements-list');
    agreementList.replaceChildren();
    if (state.agreements.length === 0) agreementList.append(txt('li', 'Nenhum convênio registrado para a organização.'));
    for (const a of state.agreements) {
      const terms = a.terms.map((t) => `${t.product}/${t.unit} por ${t.priceMilliBrl} milli-BRL`).join(', ');
      agreementList.append(txt('li', `${a.orgLabel} · ${a.state}${a.state !== a.derivedState ? ` (${a.derivedState})` : ''} · rev. ${a.revision} · ${terms}${a.conditions ? ` · ${a.conditions}` : ''}`));
    }

    const authorizationList = el('authorizations-list');
    authorizationList.replaceChildren();
    if (state.authorizations.length === 0) authorizationList.append(txt('li', 'Nenhuma autorização de abastecimento emitida.'));
    for (const a of state.authorizations) {
      const limits = `até ${a.maxQuantityMilli} milli e ${a.maxCentavos} centavos`;
      authorizationList.append(txt('li', `${a.product}/${a.unit} · ${a.derivedState} · ${limits} · válido até ${a.validUntil}${a.failedAttempts > 0 ? ` · ${a.failedAttempts} tentativa(s) inválida(s)` : ''}`));
    }

    const redemptionList = el('redemptions-list');
    redemptionList.replaceChildren();
    if (state.redemptions.length === 0) redemptionList.append(txt('li', 'Nenhum resgate confirmado pelos postos conveniados.'));
    for (const r of state.redemptions) {
      redemptionList.append(txt('li', `${r.product}/${r.unit} · ${r.quantityMilli} milli · total ${r.totalCentavos} centavos · confirmado ${r.confirmedAt}`));
    }

    const codeNote = el('authorization-code-note');
    codeNote.replaceChildren();
    if (state.lastCode) {
      codeNote.append(txt('p', 'Código único da autorização emitida — copie agora, ele não será exibido novamente.'));
      codeNote.append(txt('p', state.lastCode));
    }
  }

  el('agreement-propose-button').addEventListener('click', () => {
    const auth = controller.state.authority;
    if (!auth) return;
    void controller.propose(
      auth.organization.id,
      el<HTMLInputElement>('agreement-station-input').value.trim(),
      el<HTMLInputElement>('agreement-label-input').value.trim() || 'Frota parceira',
      el<HTMLInputElement>('agreement-from-input').value,
      el<HTMLInputElement>('agreement-until-input').value,
      [{ product: el<HTMLSelectElement>('agreement-product-input').value, unit: 'L', priceMilliBrl: Number(el<HTMLInputElement>('agreement-price-input').value) }],
    );
  });
  el('agreement-suspend-button').addEventListener('click', () => {
    const auth = controller.state.authority;
    const agreement = controller.state.agreements.find((a) => a.state === 'ACTIVE');
    if (!auth || !agreement) return;
    void controller.apply(auth.organization.id, agreement.id, 'SUSPEND', agreement.revision, 'Revisão contratual em andamento');
  });
  el('agreement-end-button').addEventListener('click', () => {
    const auth = controller.state.authority;
    const agreement = controller.state.agreements.find((a) => a.state === 'ACTIVE' || a.state === 'SUSPENDED');
    if (!auth || !agreement) return;
    void controller.apply(auth.organization.id, agreement.id, 'END', agreement.revision, 'Encerramento acordado entre as partes');
  });
  el('authorization-issue-button').addEventListener('click', () => {
    const auth = controller.state.authority;
    const agreement = controller.state.agreements.find((a) => a.state === 'ACTIVE');
    if (!auth || !agreement) return;
    void controller.issue(
      auth.organization.id,
      agreement.id,
      el<HTMLInputElement>('authorization-employee-input').value.trim(),
      el<HTMLSelectElement>('authorization-product-input').value,
      'L',
      Number(el<HTMLInputElement>('authorization-quantity-input').value) || 200000,
      Number(el<HTMLInputElement>('authorization-total-input').value) || 300000,
      el<HTMLInputElement>('authorization-until-input').value,
    );
  });
  el('authorization-cancel-button').addEventListener('click', () => {
    const auth = controller.state.authority;
    const authorization = controller.state.authorizations.find((a) => a.derivedState === 'ISSUED');
    if (!auth || !authorization) return;
    void controller.cancel(auth.organization.id, authorization.id, 'Cancelamento pela organização');
  });
  el('agreements-refresh-button').addEventListener('click', () => void controller.refresh());

  return {
    select(authority?: Authority): void {
      if (!authority) {
        controller.clear();
        return;
      }
      controller.select(authority);
    },
    clear(): void {
      controller.clear();
    },
  };
}
