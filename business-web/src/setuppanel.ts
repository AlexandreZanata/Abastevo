import { OrganizationSetupApi, OrganizationSetupController, type SetupDraft, type SetupState, type SettingsValues, type ResponsibilityMember, type SettingsVersion } from './setup.js';

const KINDS: Record<string, string> = { DELIVERY: 'Entregas', CARRIER: 'Transportadora', SERVICES: 'Serviços', FARM: 'Fazenda', OTHER: 'Outras atividades' };
const STATES: Record<string, string> = { PENDING_SETUP: 'Configuração pendente', ACTIVE: 'Ativa', SUSPENDED: 'Suspensa', ARCHIVED: 'Arquivada' };
const CHECKLIST: Record<string, string> = { name: 'Nome da empresa', time_zone: 'Fuso horário válido', active_unit: 'Unidade ativa', active_owner: 'Proprietário com acesso ativo', terms: 'Termos de demonstração registrados' };
const SOURCES: Record<string, string> = { ONBOARDING: 'Padrões do cadastro', BOOTSTRAP: 'Padrões iniciais', COMMAND: 'Alteração confirmada', MEMBERSHIP_REDUCTION: 'Responsabilidade atualizada por mudança de vínculo' };
const AUTOMATIC_REASONS: Record<string, string> = { ONBOARDING: 'Configurações do cadastro inicial.', BOOTSTRAP: 'Padrões iniciais da empresa.', MEMBERSHIP_REDUCTION: 'Responsabilidade removida após mudança no vínculo.' };
const RESPONSIBILITIES = ['review_fallback', 'platform_notices', 'data_protection', 'billing'] as const;
const RESPONSIBILITY_LABELS = ['Revisão de registros', 'Avisos da empresa', 'Proteção de dados', 'Cobrança'];
const SCALAR_FIELDS = ['setup-name', 'setup-unit-name', 'setup-reason'];

export function mountOrganizationSetup(doc: Document, browser: Window, source: OrganizationSetupApi, sessionLost: (message: string) => void, selected: (organizationId: string) => void): { select(accountId: string, organizationId?: string): void; clear(): void; revalidate(): void } {
  const el = <T extends HTMLElement>(id: string): T => { const node = doc.getElementById(id); if (!node) throw new Error('Company setup panel is incomplete'); return node as T; };
  const input = (id: string): HTMLInputElement => el<HTMLInputElement>(id);
  const createForm = el<HTMLFormElement>('setup-create-form');
  const settingsForm = el<HTMLFormElement>('setup-settings-form');
  let identity = '', formScope = '', formRevision = 0, dirty = false, staleDraft = false, lastHeading = '', lastErrors = '', lastHistorical = 0;
  const controller = new OrganizationSetupController(source, render, sessionLost);
  function node<K extends keyof HTMLElementTagNameMap>(tag: K, value: string): HTMLElementTagNameMap[K] { const item = doc.createElement(tag); item.textContent = value; return item; }
  function button(label: string, action: () => void, busy: boolean): HTMLButtonElement { const item = node('button', label); item.type = 'button'; item.className = 'button-quiet'; item.disabled = busy; item.addEventListener('click', action); return item; }
  function date(value: string, zone = 'UTC'): string { try { return new Intl.DateTimeFormat('pt-BR', { timeZone: zone, dateStyle: 'short', timeStyle: 'short' }).format(new Date(value)); } catch { return value; } }
  function textFields(target: HTMLElement, rows: [string, string][]): void { target.replaceChildren(); for (const [label, value] of rows) target.append(node('dt', label), node('dd', value || 'Não informado')); }
  function person(id: string, members: ResponsibilityMember[]): string { if (!id) return 'Não definido'; const member = members.find(v => v.membership_id === id); return member ? (member.display_name || 'Cadastro sem perfil') + (member.eligible ? '' : ' · vínculo não disponível') : 'Vínculo histórico não disponível'; }
  function setValues(values: SettingsValues): void {
    input('setup-settings-time-zone').value = values.time_zone;
    el<HTMLSelectElement>('setup-consumption-display').value = values.consumption_display;
    input('setup-approval-levels').value = String(values.approval_levels_default); input('setup-closure-days').value = String(values.closure_period_days);
    for (const kind of ['trips', 'fuelings', 'expenses'] as const) input('setup-review-' + kind).checked = values.review_required[kind];
    const policies = values.policies;
    for (const [id, value] of [['intake-days', policies.intake_window_days], ['clock-skew', policies.clock_skew_limit_seconds], ['delegation-days', policies.delegation_maximum_days], ['invitation-days', policies.invitation_validity_days], ['activation-hours', policies.activation_validity_hours]] as const) input('setup-' + id).value = String(value);
    input('setup-invitation-confirmation').checked = policies.require_invitation_confirmation === true;
    for (const kind of RESPONSIBILITIES) for (const part of ['primary', 'substitute'] as const) el<HTMLSelectElement>(`setup-${kind}-${part}`).value = values.responsibilities[kind][part];
    el<HTMLTextAreaElement>('setup-reason').value = '';
  }
  function values(state: SetupState): SettingsValues {
    const v = structuredClone(state.snapshot!.settings.values);
    v.time_zone = input('setup-settings-time-zone').value.trim(); v.consumption_display = el<HTMLSelectElement>('setup-consumption-display').value as SettingsValues['consumption_display'];
    v.approval_levels_default = Number(input('setup-approval-levels').value); v.closure_period_days = Number(input('setup-closure-days').value);
    for (const kind of ['trips', 'fuelings', 'expenses'] as const) v.review_required[kind] = input('setup-review-' + kind).checked;
    v.policies.intake_window_days = Number(input('setup-intake-days').value); v.policies.clock_skew_limit_seconds = input('setup-clock-skew').value.trim();
    v.policies.delegation_maximum_days = Number(input('setup-delegation-days').value); v.policies.invitation_validity_days = Number(input('setup-invitation-days').value); v.policies.activation_validity_hours = Number(input('setup-activation-hours').value); v.policies.require_invitation_confirmation = input('setup-invitation-confirmation').checked;
    for (const kind of RESPONSIBILITIES) for (const part of ['primary', 'substitute'] as const) v.responsibilities[kind][part] = el<HTMLSelectElement>(`setup-${kind}-${part}`).value;
    return v;
  }
  function options(state: SetupState, reset: boolean): void {
    const snapshot = state.snapshot!;
    const members = new Map<string, { membership_id: string; display_name: string; profile_pending: boolean; eligible: boolean }>();
    for (const member of snapshot.responsibility_members) members.set(member.membership_id, member);
    for (const member of state.members) members.set(member.membership_id, { ...member, eligible: true });
    for (const kind of RESPONSIBILITIES) for (const part of ['primary', 'substitute'] as const) {
      const select = el<HTMLSelectElement>(`setup-${kind}-${part}`);
      const chosen = reset ? snapshot.settings.values.responsibilities[kind][part] : select.value;
      select.replaceChildren(); const empty = node('option', 'Não definido'); empty.value = ''; select.append(empty);
      for (const member of members.values()) { const option = node('option', (member.display_name || 'Cadastro sem perfil') + (member.eligible ? '' : ' · vínculo não disponível')); option.value = member.membership_id; option.disabled = !member.eligible; select.append(option); }
      if (chosen && !members.has(chosen)) { const unavailable = node('option', 'Pessoa não disponível — escolha novamente'); unavailable.value = chosen; unavailable.disabled = true; select.append(unavailable); }
      select.value = chosen;
    }
  }
  function historical(version: SettingsVersion, members: ResponsibilityMember[]): void {
    const v = version.values, yes = (value: boolean): string => value ? 'Sim' : 'Não';
    const rows: [string, string][] = [['Revisão', String(version.revision)], ['Em vigor desde', date(version.effective_at, v.time_zone)], ['Origem', SOURCES[version.source] || 'Revisão registrada'], ['Motivo', AUTOMATIC_REASONS[version.source] || version.reason], ['Fuso', v.time_zone], ['Consumo', v.consumption_display === 'KM_PER_L' ? 'km/L' : 'L/100 km'], ['Revisão de viagens', yes(v.review_required.trips)], ['Revisão de abastecimentos', yes(v.review_required.fuelings)], ['Revisão de despesas', yes(v.review_required.expenses)], ['Níveis de aprovação', String(v.approval_levels_default)], ['Fechamento (dias)', String(v.closure_period_days)], ['Recebimento após vínculo (dias)', String(v.policies.intake_window_days)], ['Diferença do relógio (segundos)', v.policies.clock_skew_limit_seconds], ['Delegação (dias)', String(v.policies.delegation_maximum_days)], ['Convite (dias)', String(v.policies.invitation_validity_days)], ['Ativação (horas)', String(v.policies.activation_validity_hours)], ['Confirmação após aceite de convite', yes(v.policies.require_invitation_confirmation === true)]];
    RESPONSIBILITIES.forEach((kind, i) => { rows.push([RESPONSIBILITY_LABELS[i] + ' — responsável', person(v.responsibilities[kind].primary, members)], [RESPONSIBILITY_LABELS[i] + ' — substituto', person(v.responsibilities[kind].substitute, members)]); });
    textFields(el('setup-history-values'), rows);
  }
  function scalarValidity(): void { for (const id of SCALAR_FIELDS) { const field = el<HTMLInputElement | HTMLTextAreaElement>(id); field.setCustomValidity([...field.value].length > 120 ? 'Use até 120 caracteres.' : ''); } }
  function fieldId(field: string, current: boolean): string | undefined {
    const ids: Record<string, string> = { name: 'setup-name', unit_name: 'setup-unit-name', time_zone: current ? 'setup-settings-time-zone' : 'setup-time-zone', consumption_display: 'setup-consumption-display', approval_levels_default: 'setup-approval-levels', closure_period_days: 'setup-closure-days', reason: 'setup-reason', 'policies.intake_window_days': 'setup-intake-days', 'policies.clock_skew_limit_seconds': 'setup-clock-skew', 'policies.delegation_maximum_days': 'setup-delegation-days', 'policies.invitation_validity_days': 'setup-invitation-days', 'policies.activation_validity_hours': 'setup-activation-hours', 'policies.require_invitation_confirmation': 'setup-invitation-confirmation' };
    if (field.startsWith('responsibilities.')) { const [, kind, part] = field.split('.'); if (RESPONSIBILITIES.some(value => value === kind) && (part === 'primary' || part === 'substitute')) return `setup-${kind}-${part}`; }
    if (current && field === 'name') return undefined;
    return ids[field];
  }
  function clearPrivate(): void {
    formScope = ''; formRevision = 0; dirty = false; staleDraft = false; lastHeading = ''; lastErrors = ''; lastHistorical = 0;
    createForm.reset(); settingsForm.reset();
    try { input('setup-time-zone').value = Intl.DateTimeFormat().resolvedOptions().timeZone; } catch { input('setup-time-zone').value = 'UTC'; }
    for (const id of SCALAR_FIELDS) el<HTMLInputElement | HTMLTextAreaElement>(id).setCustomValidity('');
    for (const item of el('setup-panel').querySelectorAll('[aria-invalid]')) item.removeAttribute('aria-invalid');
    for (const id of ['setup-directory-list', 'setup-profile', 'setup-checklist', 'setup-history-list', 'setup-history-values', 'setup-errors']) el(id).replaceChildren();
    for (const kind of RESPONSIBILITIES) for (const part of ['primary', 'substitute']) el<HTMLSelectElement>(`setup-${kind}-${part}`).replaceChildren();
    el('setup-organization-name').textContent = ''; el('setup-organization-state').textContent = ''; el('setup-capability-note').textContent = ''; el('setup-draft-status').textContent = '';
  }
  function render(state: SetupState): void {
    const root = el('setup-panel'), busy = state.phase === 'loading', pending = controller.retryable;
    root.hidden = state.phase === 'closed'; root.setAttribute('aria-busy', String(busy));
    if (state.phase === 'closed' || state.phase === 'denied') clearPrivate();
    if (state.phase === 'denied') el('setup-title').focus({ preventScroll: true });
    el('setup-status').textContent = state.message || (busy ? 'Consultando o servidor…' : '');
    const errors = el('setup-errors'); errors.replaceChildren();
    for (const item of root.querySelectorAll('[aria-invalid]')) item.removeAttribute('aria-invalid');
    for (const field of state.fields) {
      const item = node('li', CHECKLIST[field] ? 'Verifique: ' + CHECKLIST[field] : field.startsWith('responsibilities.') ? 'Confira a pessoa escolhida para a responsabilidade indicada.' : 'Confira os valores informados e atualize os dados, se necessário.');
      const id = fieldId(field, state.snapshot !== null);
      if (id) { el(id).setAttribute('aria-invalid', 'true'); item.append(button('Ir para o campo', () => { el(id).focus(); }, busy)); }
      errors.append(item);
    }
    const errorKey = state.phase === 'failed' ? state.fields.join('|') : '';
    if (errorKey && errorKey !== lastErrors && !busy) { const id = fieldId(state.fields[0], state.snapshot !== null); if (id) el(id).focus({ preventScroll: true }); }
    lastErrors = errorKey;
    el('setup-retry').hidden = !pending || busy; el<HTMLButtonElement>('setup-retry').disabled = busy;
    el<HTMLButtonElement>('setup-refresh').disabled = busy || !state.accountId;
    const directory = el('setup-directory-list'); directory.replaceChildren();
    if (!state.setups.length && state.accountId && !state.organizationId && !busy) directory.append(node('li', 'Nenhum cadastro salvo disponível para esta conta.'));
    for (const company of state.setups) { const item = node('li', ''); item.append(node('p', `${company.name || 'Cadastro sem nome'} · ${STATES[company.state] || 'Estado confirmado'}`), button('Retomar ' + (company.name || 'cadastro'), () => { if (dirty && !browser.confirm('Descartar as alterações não salvas e abrir este cadastro?')) return; dirty = false; void controller.select(company.id); }, busy || pending)); directory.append(item); }
    el('setup-directory').hidden = state.organizationId !== '' || state.phase === 'denied';
    el('setup-directory-more').hidden = !state.nextSetupCursor; el<HTMLButtonElement>('setup-directory-more').disabled = busy || pending;
    el('setup-create').hidden = state.organizationId !== '' || state.phase === 'denied'; el<HTMLFieldSetElement>('setup-draft-fields').disabled = busy || pending || !state.accountId; el<HTMLButtonElement>('setup-create-submit').disabled = busy || pending || !state.accountId;
    const snapshot = state.snapshot; el('setup-detail').hidden = snapshot === null;
    if (!snapshot) {
      if (formScope) { formScope = ''; formRevision = 0; dirty = false; staleDraft = false; settingsForm.reset(); el('setup-profile').replaceChildren(); el('setup-checklist').replaceChildren(); el('setup-history-list').replaceChildren(); el('setup-history-values').replaceChildren(); for (const kind of RESPONSIBILITIES) for (const part of ['primary', 'substitute']) el<HTMLSelectElement>(`setup-${kind}-${part}`).replaceChildren(); }
      return;
    }
    const scope = state.accountId + ':' + snapshot.organization.id;
    const reset = formScope !== scope || formRevision !== snapshot.settings.revision && !dirty;
    if (formScope === scope && formRevision !== snapshot.settings.revision && dirty) staleDraft = true;
    options(state, reset);
    if (reset) { setValues(snapshot.settings.values); formScope = scope; formRevision = snapshot.settings.revision; dirty = false; staleDraft = false; }
    const org = snapshot.organization;
    el('setup-organization-name').textContent = org.name || 'Cadastro sem nome'; el('setup-organization-state').textContent = STATES[org.state] || 'Estado confirmado';
    textFields(el('setup-profile'), [['Atividades', org.kinds.map(k => KINDS[k]).join(', ') || 'Não informadas'], ['Fuso atual', org.time_zone], ['Revisão do cadastro', String(snapshot.root_revision)], ['Revisão das configurações', String(snapshot.settings.revision)], ['Padrões em vigor desde', date(snapshot.settings.effective_at, snapshot.settings.values.time_zone)]]);
    el('setup-capability-note').textContent = !snapshot.context.capability_enabled ? 'O cadastro está salvo. A habilitação da empresa ainda depende do responsável pelo ambiente; até lá, você pode consultar esta configuração.' : org.state === 'ACTIVE' ? 'A configuração está concluída. O acesso às operações segue as permissões e habilitações da empresa.' : org.state === 'SUSPENDED' ? 'Empresa suspensa. As configurações podem ser consultadas; alterações estão indisponíveis.' : org.state === 'ARCHIVED' ? 'Empresa arquivada. As configurações podem ser consultadas; alterações estão indisponíveis.' : 'Confira os itens abaixo antes de concluir a configuração.';
    const checklist = el('setup-checklist'); checklist.replaceChildren(); for (const [key, label] of Object.entries(CHECKLIST)) checklist.append(node('li', `${snapshot.setup.missing.includes(key) ? 'Pendente' : 'Concluído'} — ${label}`));
    el<HTMLButtonElement>('setup-activate').disabled = !controller.canActivate || dirty || staleDraft; el('setup-activate').hidden = org.state !== 'PENDING_SETUP';
    el<HTMLFieldSetElement>('setup-settings-fields').disabled = !controller.canEdit;
    el('setup-settings-access').textContent = !snapshot.context.permissions.includes('org.settings.manage') ? 'Você tem acesso de consulta. Alterações exigem a permissão de configuração.' : !snapshot.context.fresh ? 'Entre novamente para confirmar seu acesso antes de alterar configurações.' : !controller.canEdit ? 'Alterações indisponíveis neste momento. Confira a situação da empresa acima.' : 'Alterações criam uma nova revisão, com o motivo informado.';
    el<HTMLButtonElement>('setup-save').disabled = !controller.canEdit || staleDraft;
    el('setup-draft-status').textContent = staleDraft ? 'Existe uma revisão mais recente. Recarregue os valores antes de decidir novamente.' : dirty ? 'Há alterações não salvas. Salve ou recarregue os valores antes de concluir a configuração.' : '';
    el('setup-reload-values').hidden = !dirty && !staleDraft; el<HTMLButtonElement>('setup-reload-values').disabled = busy || pending;
    el('setup-members-load').hidden = !snapshot.context.permissions.includes('org.settings.manage'); el<HTMLButtonElement>('setup-members-load').disabled = !controller.canChooseMembers || pending;
    el('setup-members-more').hidden = !state.nextMemberCursor; el<HTMLButtonElement>('setup-members-more').disabled = !controller.canChooseMembers || pending;
    el<HTMLButtonElement>('setup-history-load').disabled = busy || pending; el('setup-history-more').hidden = !state.nextHistoryCursor; el<HTMLButtonElement>('setup-history-more').disabled = busy || pending;
    const history = el('setup-history-list'); history.replaceChildren(); for (const version of state.history) { const item = node('li', ''); item.append(node('p', `Revisão ${version.revision} · ${date(version.effective_at, version.values.time_zone)} · ${SOURCES[version.source] || 'Revisão registrada'}`), button(`Ver revisão ${version.revision}`, () => { void controller.readRevision(version.revision); }, busy || pending)); history.append(item); }
    el('setup-history-detail').hidden = state.historical === null;
    if (state.historical) { historical(state.historical, state.historicalMembers); if (!busy && state.historical.revision !== lastHistorical) { lastHistorical = state.historical.revision; el('setup-history-heading').focus({ preventScroll: true }); } }
    else { lastHistorical = 0; el('setup-history-values').replaceChildren(); }
    if (lastHeading !== scope && !busy) { lastHeading = scope; el('setup-organization-name').focus({ preventScroll: true }); }
    identity = state.accountId + ':' + org.id;
    selected(org.id);
  }
  settingsForm.addEventListener('input', () => { dirty = true; scalarValidity(); render(controller.state); });
  settingsForm.addEventListener('change', () => { dirty = true; render(controller.state); });
  createForm.addEventListener('input', scalarValidity);
  createForm.addEventListener('submit', event => {
    event.preventDefault(); if (el<HTMLButtonElement>('setup-create-submit').disabled || controller.retryable) return;
    const kinds = [...doc.querySelectorAll<HTMLInputElement>('[data-setup-kind]')].filter(v => v.checked).map(v => v.dataset.setupKind!);
    input('setup-kind-delivery').setCustomValidity(kinds.length ? '' : 'Escolha pelo menos uma atividade da empresa.'); scalarValidity();
    if (!createForm.reportValidity()) return;
    const draft: SetupDraft = { name: input('setup-name').value, time_zone: input('setup-time-zone').value.trim(), unit_name: input('setup-unit-name').value, kinds, acceptedTerms: input('setup-terms').checked };
    void controller.create(draft);
  });
  for (const item of doc.querySelectorAll<HTMLInputElement>('[data-setup-kind]')) item.addEventListener('change', () => { input('setup-kind-delivery').setCustomValidity(''); });
  settingsForm.addEventListener('submit', event => { event.preventDefault(); scalarValidity(); if (!controller.canEdit || staleDraft || !settingsForm.reportValidity()) return; void controller.save(values(controller.state), el<HTMLTextAreaElement>('setup-reason').value); });
  el('setup-activate').addEventListener('click', () => { if (controller.canActivate && !dirty && browser.confirm('Concluir a configuração com os dados confirmados pelo servidor?')) void controller.activate(); });
  el('setup-retry').addEventListener('click', () => { void controller.retry(); });
  const refresh = (): void => { if (dirty && !browser.confirm('Descartar as alterações não salvas e carregar os valores atuais do servidor?')) return; dirty = false; staleDraft = false; void controller.refresh(); };
  el('setup-refresh').addEventListener('click', refresh); el('setup-reload-values').addEventListener('click', refresh);
  el('setup-directory-more').addEventListener('click', () => { void controller.moreSetups(); }); el('setup-members-load').addEventListener('click', () => { void controller.members(); }); el('setup-members-more').addEventListener('click', () => { void controller.moreMembers(); });
  el('setup-history-load').addEventListener('click', () => { void controller.history(); }); el('setup-history-more').addEventListener('click', () => { void controller.moreHistory(); });
  return {
    select(accountId, organizationId): void { const next = accountId + ':' + (organizationId || ''); if (next === identity) return; clearPrivate(); identity = next; void controller.open(accountId, organizationId); },
    clear(): void { if (!identity && controller.state.phase === 'closed') return; identity = ''; controller.clear(); },
    revalidate(): void { if (identity) void controller.revalidate(); },
  };
}
