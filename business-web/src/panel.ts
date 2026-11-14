import { type AccessSource, type AccessState, type EmployeeProfile, EmployeeAccessController } from './employees.js';
import { type Authority } from './model.js';
import { ROLES } from './company.js';

const STATES: Record<string, string> = { ACTIVE: 'Ativo', INACTIVE: 'Inativo', TERMINATED: 'Desligado', MERGED: 'Unificado', PENDING: 'Pendente', ACCEPTED: 'Aceito', EXPIRED: 'Expirado', REVOKED: 'Revogado', SUPERSEDED: 'Substituído por nova emissão', DECLINED: 'Recusado', WAITING: 'Aguardando confirmação', CONFIRMED: 'Confirmado', REJECTED: 'Recusado pelo responsável', ENDED: 'Encerrado' };
const ACCESS: Record<string, string> = { NONE: 'Sem acesso', INVITED: 'Convidado', ACTIVE: 'Ativo', SUSPENDED: 'Suspenso', UNKNOWN: 'Não confirmado' };
const CAUSES: Record<string, string> = { EXPIRED: 'O prazo de validade terminou.', REPLACED: 'Um novo código substituiu este convite.', MANAGER_REVOKED: 'O responsável revogou este convite.', TOO_MANY_ATTEMPTS: 'O limite de tentativas inválidas foi atingido.', CONTACT_CHANGED: 'Os contatos do funcionário foram alterados.', EMPLOYEE_UNAVAILABLE: 'O cadastro do funcionário não está disponível para acesso.', ISSUER_REDUCED: 'O acesso de quem emitiu o convite mudou.', RESERVATION_ENDED: 'O vínculo reservado para este convite foi encerrado.', ORGANIZATION_ARCHIVED: 'A empresa foi arquivada.', PERSON_ACCEPTED: 'A pessoa convidada aceitou o convite.', PERSON_DECLINED: 'A pessoa convidada recusou o convite.' };
export function mountEmployeeAccess(doc: Document, browser: Window, source: AccessSource, sessionLost?: (message: string) => void): { select(authority?: Authority): void; clear(): void } {
  const el = <T extends HTMLElement>(id: string): T => { const element = doc.getElementById(id); if (!element) throw new Error('Employee panel is incomplete'); return element as T; };
  let identity = '';
  let previousDelivery = false;
  let lastEmployee = '';
  const controller = new EmployeeAccessController(source, render, sessionLost);
  function element<K extends keyof HTMLElementTagNameMap>(tag: K, value: string): HTMLElementTagNameMap[K] { const node = doc.createElement(tag); node.textContent = value; return node; }
  function button(value: string, action: () => void): HTMLButtonElement { const node = element('button', value); node.type = 'button'; node.className = 'button-quiet'; node.addEventListener('click', action); return node; }
  function profile(): EmployeeProfile { return { display_name: el<HTMLInputElement>('employee-name').value.trim(), registration_number: el<HTMLInputElement>('employee-registration').value.trim(), work_email: el<HTMLInputElement>('employee-email').value.trim(), work_phone: el<HTMLInputElement>('employee-phone').value.trim(), job_title: el<HTMLInputElement>('employee-job').value.trim(), home_unit_id: '' }; }
  function employeeLabel(v: { displayName: string; registrationNumber: string; state: string; accessState: string }): string { return `${v.displayName || 'Cadastro sem perfil'} · Matrícula: ${v.registrationNumber || 'não informada'} · Cadastro: ${STATES[v.state] ?? v.state} · Acesso: ${ACCESS[v.accessState] ?? 'Não confirmado'}`; }
  function render(state: AccessState): void {
    const root = el('employee-access');
    root.hidden = state.phase === 'closed';
    const busy = state.phase === 'loading';
    root.setAttribute('aria-busy', String(busy));
    el('employee-status').textContent = busy ? 'Consultando o servidor…' : state.message;
    el('employee-retry').hidden = state.phase !== 'failed' || !controller.retryable;
    const manage = state.authority?.permissions?.includes('idn.employee.manage') === true;
    el('employee-create').hidden = !manage;
    el('employee-filter-form').hidden = state.authority === null;
    const list = el('employee-list'); list.replaceChildren();
    if (state.employees.length === 0 && state.authority) list.append(element('li', busy ? 'Carregando funcionários…' : 'Nenhum funcionário nesta página. Você pode adicionar um cadastro, se tiver permissão.'));
    for (const employee of state.employees) {
      const item = element('li', ''); item.append(element('p', employeeLabel(employee)), button('Abrir ' + (employee.displayName || 'cadastro'), () => { void controller.openEmployee(employee.id); })); list.append(item);
    }
    el('employees-next').hidden = state.nextCursor === '';
    el<HTMLSelectElement>('employee-state').value = state.filter;
    const duplicates = el('employee-duplicates'); duplicates.replaceChildren();
    for (const v of state.duplicates) duplicates.append(element('li', `${employeeLabel(v)} · E-mail: ${v.workEmail || 'não informado'} · Telefone: ${v.workPhone || 'não informado'}`));
    el('employee-duplicate-confirm').closest('label')?.toggleAttribute('hidden', state.duplicates.length === 0);
    const detail = state.employee; el('employee-detail').hidden = detail === null;
    const fields = el('employee-fields'); fields.replaceChildren();
    if (detail) {
      el('employee-detail-name').textContent = detail.displayName || 'Cadastro sem perfil';
      for (const [name, value] of [['Situação do cadastro', STATES[detail.state] ?? detail.state], ['Situação do acesso', ACCESS[detail.accessState] ?? 'Não confirmado'], ['Matrícula', detail.registrationNumber], ['Cargo', detail.jobTitle], ['E-mail profissional', detail.workEmail], ['Telefone profissional', detail.workPhone], ['Unidade', detail.homeUnitId ? 'Unidade vinculada' : '']]) fields.append(element('dt', name), element('dd', value || 'Não informado'));
      if (detail.id !== lastEmployee && !busy) el('employee-detail-name').focus({ preventScroll: true });
      lastEmployee = detail.id;
    }
    const canInvite = state.authority?.permissions?.includes('idn.invitation.manage') === true;
    el('invitation-section').hidden = !canInvite || detail === null;
    el('invitation-validity').textContent = `Validade definida pela empresa: ${state.authority?.invitationValidityDays ?? 0} dias. A validade exata é confirmada na emissão.`;
    const role = el<HTMLSelectElement>('invitation-role'), chosen = role.value; role.replaceChildren();
    role.append(element('option', 'Selecione um papel e alcance')); role.options[0].value = '';
    for (const [key, description] of Object.entries(ROLES)) for (const scope of ['ORG', 'SELF']) {
      if (key === 'OWNER' && scope === 'SELF') continue;
      const option = element('option', `${description} · ${scope === 'ORG' ? 'Empresa' : 'Próprios registros'}`);
      option.value = `${key}:${scope}`;
      option.disabled = !state.authority?.invitationGrants?.some(g => g.role === key && g.scope === scope);
      if (option.disabled) option.textContent += ' (sem permissão para conceder)';
      role.append(option);
    }
    role.value = chosen;
    const invitations = el('invitation-list'); invitations.replaceChildren();
    if (state.invitations.length === 0) invitations.append(element('li', 'Nenhum convite para este funcionário nesta página.'));
    for (const invitation of state.invitations) {
      const item = element('li', '');
      const grantCopy = invitation.grants.map(g => `${ROLES[g.role] ?? g.role} (${g.scope === 'ORG' ? 'Empresa' : 'Próprios registros'})`).join('; ');
      item.append(element('p', `${STATES[invitation.state] ?? invitation.state} · Expira em ${new Date(invitation.expiresAt).toLocaleString('pt-BR')} · Tentativas inválidas: ${invitation.failedAttempts}`), element('p', grantCopy), element('p', invitation.issuerMembershipId === state.authority?.organization.membershipId ? 'Emitido por você' : 'Outro responsável da empresa'));
      if (invitation.cause) item.append(element('p', CAUSES[invitation.cause] ?? 'Estado atualizado pelo servidor.'));
      if (invitation.state === 'PENDING') item.append(button('Reemitir convite', () => { if (browser.confirm('Emitir um novo código? O código anterior deixará de funcionar.')) void controller.invitationAction(invitation.id, 'resend'); }));
      if (invitation.state === 'PENDING') item.append(button('Revogar convite', () => { if (browser.confirm('Revogar este convite? Ele deixará de funcionar.')) void controller.invitationAction(invitation.id, 'revoke'); }));
      if (invitation.state === 'ACCEPTED') item.append(button('Consultar aceite', () => { void controller.readAdmission(invitation.id); }));
      invitations.append(item);
    }
    el('invitations-next').hidden = state.invitationCursor === '';
    const admission = state.admission; el('invitation-admission').hidden = admission === null;
    el('admission-description').textContent = admission ? `${STATES[admission.state] ?? admission.state} · Conta: ${admission.accountAlias} · Prazo: ${admission.deadline ? new Date(admission.deadline).toLocaleString('pt-BR') : 'não se aplica'}. ${admission.state === 'WAITING' ? 'Confirme somente se esta for a pessoa convidada.' : 'Esta situação foi confirmada pelo servidor.'}` : '';
    el('admission-confirm').hidden = admission?.state !== 'WAITING'; el('admission-reject').hidden = admission?.state !== 'WAITING';
    const delivery = state.delivery;
    el('invitation-delivery').hidden = delivery === null;
    el<HTMLInputElement>('delivery-code').value = delivery?.code ?? ''; el<HTMLInputElement>('delivery-link').value = delivery?.link ?? '';
    if (!delivery) el('delivery-status').textContent = '';
    if (delivery && !previousDelivery) el('delivery-title').focus({ preventScroll: true });
    previousDelivery = delivery !== null;
    for (const control of root.querySelectorAll<HTMLButtonElement | HTMLInputElement | HTMLSelectElement>('button,input,select')) control.disabled = busy || state.authority === null;
    el<HTMLButtonElement>('invitation-issue').disabled ||= !canInvite || state.authority?.invitationGrants?.length === 0 || detail?.state !== 'ACTIVE' || detail?.profileRevision === 0;
    el<HTMLButtonElement>('employee-save').disabled ||= !manage;
  }
  el('employee-filter-form').addEventListener('submit', event => { event.preventDefault(); void controller.list(el<HTMLSelectElement>('employee-state').value); });
  el('employees-refresh').addEventListener('click', () => { void controller.list(); });
  el('employees-next').addEventListener('click', () => { void controller.list(controller.state.filter, controller.state.nextCursor); });
  el('invitations-next').addEventListener('click', () => { void controller.invitationPage(controller.state.invitationCursor); });
  el('employee-retry').addEventListener('click', () => { void controller.retry(); });
  el('employee-create-form').addEventListener('submit', event => { event.preventDefault(); if (el<HTMLFormElement>('employee-create-form').reportValidity()) { el<HTMLInputElement>('employee-duplicate-confirm').checked = false; void controller.review(profile()); } });
  el('employee-save').addEventListener('click', () => { if (el<HTMLFormElement>('employee-create-form').reportValidity()) void controller.create(profile(), el<HTMLInputElement>('employee-duplicate-confirm').checked); });
  el('invitation-issue-form').addEventListener('submit', event => {
    event.preventDefault(); const [role, scope] = el<HTMLSelectElement>('invitation-role').value.split(':');
    if (scope !== 'ORG' && scope !== 'SELF') { el<HTMLSelectElement>('invitation-role').focus(); return; }
    void controller.issue([{ role, scope }]);
  });
  el('admission-confirm').addEventListener('click', () => { if (browser.confirm('Confirmar o acesso desta conta à empresa com os papéis oferecidos?')) void controller.admissionAction('confirm'); });
  el('admission-reject').addEventListener('click', () => { if (browser.confirm('Recusar o aceite desta conta?')) void controller.admissionAction('reject'); });
  el('delivery-close').addEventListener('click', () => { controller.closeDelivery(); el('invitations-title').focus({ preventScroll: true }); });
  el('delivery-print').addEventListener('click', () => { if (controller.state.delivery) browser.print(); });
  for (const [buttonId, fieldId] of [['delivery-copy-code', 'delivery-code'], ['delivery-copy-link', 'delivery-link']]) el(buttonId).addEventListener('click', () => {
    const delivery = controller.state.delivery;
    if (!delivery) return;
    const value = el<HTMLInputElement>(fieldId).value;
    if (!browser.navigator.clipboard) { el('delivery-status').textContent = 'Selecione o campo e copie o convite manualmente.'; el<HTMLInputElement>(fieldId).select(); return; }
    void browser.navigator.clipboard?.writeText(value).then(() => { if (controller.state.delivery === delivery) el('delivery-status').textContent = 'Copiado. Entregue somente à pessoa convidada.'; }).catch(() => { if (controller.state.delivery === delivery) { el('delivery-status').textContent = 'Selecione o campo e copie o convite manualmente.'; el<HTMLInputElement>(fieldId).select(); } });
  });
  function clear(): void {
    identity = ''; lastEmployee = ''; controller.clear();
    el<HTMLFormElement>('employee-create-form').reset(); el<HTMLSelectElement>('invitation-role').value = '';
  }
  return {
    clear,
    select(authority?: Authority): void {
      if (!authority?.permissions) { clear(); return; }
      const next = JSON.stringify(authority);
      if (identity === next) return;
      clear(); identity = next; void controller.open(authority);
    },
  };
}
