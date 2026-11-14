import { CompanyApi, type SessionPort } from './api.js';
import { CompanyController, unavailableMemberships, type MembershipSource, type Phase, type State } from './controller.js';
import { CREATE, HOME, LOGIN, organizationPath, setupPath, route } from './model.js';
import { CurrentMemberships } from './company.js';
import { EmployeeAccessApi } from './employees.js';
import { mountEmployeeAccess } from './panel.js';
import { OrganizationSetupApi } from './setup.js';
import { mountOrganizationSetup } from './setuppanel.js';

const COPY: Partial<Record<Phase, [string, string]>> = {
  loading: ['Carregando seu espaço', 'Verificando seu acesso. Aguarde um instante.'],
  choose: ['Escolha sua empresa', 'Use o seletor para abrir uma das empresas às quais você tem acesso.'],
  empty: ['Nenhuma empresa por aqui ainda', 'Você pode cadastrar uma empresa ou retomar um cadastro salvo pelo menu. Se recebeu um convite, confirme com quem o enviou.'],
  ready: ['Seu espaço de trabalho', 'Você está no ambiente da empresa selecionada. Os recursos disponíveis seguem as permissões da sua conta.'],
  denied: ['Acesso não disponível', 'Não foi possível confirmar seu acesso a esta empresa. Volte ao início ou atualize seus acessos.'],
  unavailable: ['Suas empresas estão indisponíveis', 'Não foi possível carregar seus acessos agora. Tente novamente em instantes.'],
  'signing-out': ['Encerrando sua sessão', 'Aguarde um instante.'],
  'not-found': ['Página não encontrada', 'Use o início do painel para continuar.'],
  setup: ['Cadastro e configurações', 'Consulte os dados salvos e conclua a preparação da empresa.'],
};
export function mountCompany(doc: Document, browser: Window, session: SessionPort = new CompanyApi((path, init) => browser.fetch(path, init)), source?: MembershipSource): CompanyController {
  const el = <T extends HTMLElement>(id: string): T => {
    const node = doc.getElementById(id);
    if (!node) throw new Error('Company shell is incomplete');
    return node as T;
  };
  const key = el<HTMLInputElement>('account-key');
  const form = el<HTMLFormElement>('login-form');
  const select = el<HTMLSelectElement>('organization-select');
  const picker = el<HTMLFormElement>('organization-form');
  let target = route(browser.location.pathname, browser.location.search);
  let lastPhase: Phase | null = null;
  let expiryTimer = 0;
  const api = session instanceof CompanyApi ? session : null;
  const panel = api ? mountEmployeeAccess(doc, browser, new EmployeeAccessApi(api), message => controller.reset(message)) : null;
  const setup = api ? mountOrganizationSetup(doc, browser, new OrganizationSetupApi(api), message => controller.reset(message), id => {
    if (target.kind !== 'create' && target.kind !== 'setup') return;
    const path = setupPath(id);
    if (browser.location.pathname !== path) browser.history.replaceState({}, '', path);
    target = { kind: 'setup', id };
  }) : null;
  const controller = new CompanyController(session, source ?? (api ? new CurrentMemberships(api) : unavailableMemberships), render);
  function scheduleExpiry(): void {
    browser.clearTimeout(expiryTimer);
    if (!session.authenticated) return;
    expiryTimer = browser.setTimeout(() => {
      // Successful business requests may have moved the idle deadline without
      // rendering the shell. Recheck it before clearing an active panel.
      if (session.authenticated && session.deadline > Date.now()) { scheduleExpiry(); return; }
      browser.history.replaceState({}, '', LOGIN);
      target = { kind: 'overview' };
      controller.reset('Sua sessão expirou. Entre novamente.');
    }, Math.max(0, session.deadline - Date.now()));
  }
  function render(state: State, unchanged = false): void {
    scheduleExpiry();
    // An unchanged tab-return recheck keeps focus, the pending choice and the
    // live region as they are; only the refreshed expiry deadline applies.
    if (unchanged) return;
    if (state.phase === 'ready') panel?.select(state.authority);
    else panel?.clear();
    if (state.phase === 'setup' && state.setupAccountId && (target.kind === 'create' || target.kind === 'setup')) setup?.select(state.setupAccountId, target.kind === 'setup' ? target.id : undefined);
    else setup?.clear();
    const busy = state.phase === 'loading' || state.phase === 'signing-out';
    const guest = !session.authenticated && state.phase !== 'not-found' && state.phase !== 'signing-out';
    el('login-view').hidden = !guest;
    el('workspace').hidden = guest;
    el('workspace-nav').hidden = !session.authenticated;
    el('logout-button').hidden = !session.authenticated;
    el<HTMLButtonElement>('logout-button').disabled = state.phase === 'signing-out';
    form.setAttribute('aria-busy', String(busy));
    key.disabled = busy;
    el<HTMLButtonElement>('login-button').disabled = busy;
    el('login-button').textContent = busy ? 'Entrando…' : 'Entrar no painel';
    el('login-error').textContent = state.message;
    el('page-title').textContent = COPY[state.phase]?.[0] ?? 'Abastevo Empresas';
    el('page-description').textContent = COPY[state.phase]?.[1] ?? '';
    el('workspace').setAttribute('aria-busy', String(busy));
    el('live-status').textContent = guest ? '' : (COPY[state.phase]?.[0] ?? '');
    el('selected-company').textContent = state.selected?.name ?? 'Seu ambiente empresarial';
    el('selected-company').hidden = state.selected === null;
    select.replaceChildren();
    const placeholder = doc.createElement('option');
    placeholder.value = ''; placeholder.textContent = 'Selecione uma empresa';
    select.appendChild(placeholder);
    for (const organization of state.organizations) {
      const option = doc.createElement('option');
      option.value = organization.id; option.textContent = organization.name;
      select.appendChild(option);
    }
    select.value = state.selected?.id ?? '';
    select.disabled = busy || state.organizations.length === 0;
    el<HTMLButtonElement>('organization-open').disabled = select.disabled;
    el('organization-help').textContent = state.organizations.length > 0 ? 'Seu acesso é confirmado a cada troca.' : 'Seus acessos aparecerão aqui.';
    el('retry-button').hidden = !['empty', 'denied', 'unavailable'].includes(state.phase);
    el('home-link').hidden = !['not-found', 'denied'].includes(state.phase);
    el('scope-note').hidden = state.phase !== 'ready';
    el('workspace-state-card').hidden = state.phase === 'setup';
    const settingsLink = el<HTMLAnchorElement>('company-settings-link');
    settingsLink.hidden = state.selected === null;
    settingsLink.href = state.selected ? setupPath(state.selected.id) : HOME;
    el('overview-link').setAttribute('aria-current', target.kind === 'overview' || target.kind === 'organization' ? 'page' : 'false');
    el('create-company-link').setAttribute('aria-current', target.kind === 'create' || target.kind === 'setup' ? 'page' : 'false');
    el('state-marker').textContent = state.phase === 'ready' ? 'Empresa selecionada' : busy ? 'Aguarde' : 'Seu acesso';
    if (!guest && !busy && lastPhase !== state.phase) el(state.phase === 'setup' ? 'setup-title' : 'page-title').focus({ preventScroll: true });
    // A refused sign-in returns focus to the field it disabled while busy.
    if (guest && lastPhase === 'loading' && state.message !== '') key.focus({ preventScroll: true });
    lastPhase = state.phase;
    if (session.authenticated && !busy && browser.location.pathname === LOGIN) {
      browser.history.replaceState({}, '', HOME); target = { kind: 'overview' };
    }
  }
  function navigate(path: string): void {
    browser.history.pushState({}, '', path);
    target = route(browser.location.pathname, browser.location.search);
    void controller.navigate(target);
  }
  form.addEventListener('submit', event => {
    event.preventDefault();
    if (el<HTMLButtonElement>('login-button').disabled) return;
    const value = key.value; key.value = '';
    void controller.signIn(value, target);
  });
  // Choosing an option changes nothing until the user opens it (WCAG 3.2.2):
  // arrow keys on a closed select fire change on every step.
  picker.addEventListener('submit', event => {
    event.preventDefault();
    if (select.value !== '' && !select.disabled) navigate(organizationPath(select.value));
  });
  el('logout-button').addEventListener('click', () => {
    key.value = ''; target = { kind: 'overview' };
    browser.history.replaceState({}, '', LOGIN);
    void controller.signOut();
  });
  el('retry-button').addEventListener('click', () => { void controller.navigate(target); });
  el('create-company-link').addEventListener('click', event => { event.preventDefault(); navigate(CREATE); });
  el('company-settings-link').addEventListener('click', event => { event.preventDefault(); if (controller.state.selected) navigate(setupPath(controller.state.selected.id)); });
  for (const id of ['home-link', 'overview-link']) {
    el(id).addEventListener('click', event => { event.preventDefault(); navigate(HOME); });
  }
  browser.addEventListener('popstate', () => {
    target = route(browser.location.pathname, browser.location.search);
    void controller.navigate(target);
  });
  browser.addEventListener('pagehide', () => { key.value = ''; controller.reset(); });
  browser.addEventListener('pageshow', event => { if (event.persisted) { key.value = ''; controller.reset(); } });
  doc.addEventListener('visibilitychange', () => {
    if (doc.visibilityState === 'visible' && session.authenticated) { void controller.revalidate(target); setup?.revalidate(); }
  });
  render(controller.state);
  void controller.navigate(target);
  return controller;
}
