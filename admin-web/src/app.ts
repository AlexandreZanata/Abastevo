// Page wiring for the admin panel. All data is rendered with textContent
// and createElement only.
//
// Single shell, two routes: the CSRF token lives in a module variable, so
// in-app transitions use history.pushState and never reload the document.
// Every full load starts unauthenticated: without the in-memory token the
// only honest state is the login view.

import {
  AdminApi,
  errorMessage,
  isUnauthorized,
  type AccountDetail,
  type AccountSummary,
  type AuditEvent,
  type ClaimView,
  type GrantView,
  type SnapshotContributor,
  type StaffAssignment,
} from './api.js';
import {
  defaultEndDate,
  endDateError,
  githubLoginError,
  isClaimState,
  isRole,
  isTier,
  isUuid,
  reasonError,
  searchError,
  toEndOfDayIso,
} from './validators.js';

const LOGOUT_REASON = 'admin logout via web panel';
const LOGIN_PATH = '/admin/login/';
const ACCOUNTS_PATH = '/admin/accounts/';
const CONTRIBUTORS_PATH = '/admin/contributors/';
const AUDIT_PATH = '/admin/audit/';

// Arrow wrapper: the DOM fetch requires the window receiver.
const sameOriginFetch = (input: string, init?: RequestInit): Promise<Response> => fetch(input, init);

let selected: AccountDetail | null = null;

const api = new AdminApi(sameOriginFetch, () => {
  goLogin();
});

function el<T extends HTMLElement>(id: string): T {
  const node = document.getElementById(id);
  if (node === null) {
    throw new Error(`missing element #${id}`);
  }
  return node as T;
}

function setAlert(id: string, message: string): void {
  const node = el<HTMLParagraphElement>(id);
  node.textContent = message;
}

function clearChildren(node: HTMLElement): void {
  while (node.firstChild !== null) {
    node.removeChild(node.firstChild);
  }
}

function text(tag: string, content: string): HTMLElement {
  const node = document.createElement(tag);
  node.textContent = content;
  return node;
}

function formatDateTime(raw: string | null): string {
  if (raw === null || raw === '') {
    return '—';
  }
  const date = new Date(raw);
  if (Number.isNaN(date.getTime())) {
    return raw;
  }
  return date.toLocaleString('pt-BR');
}

function navigate(path: string): void {
  window.history.pushState({}, '', path);
  route();
}

function goLogin(): void {
  api.clearSession();
  navigate(LOGIN_PATH);
}

function goAccounts(): void {
  navigate(ACCOUNTS_PATH);
}

function wireNav(): void {
  const nav = el<HTMLElement>('main-nav');
  nav.addEventListener('click', (event) => {
    const anchor = (event.target as HTMLElement).closest('a[data-route]');
    if (anchor === null) {
      return;
    }
    event.preventDefault();
    navigate(anchor.getAttribute('data-route') ?? ACCOUNTS_PATH);
  });
}

function hideAllViews(): void {
  for (const id of ['view-login', 'view-accounts', 'view-contributors', 'view-audit']) {
    el<HTMLElement>(id).hidden = true;
  }
}

function showLogin(): void {
  hideAllViews();
  el<HTMLElement>('view-login').hidden = false;
  el<HTMLElement>('main-nav').hidden = true;
  el<HTMLElement>('session-info').hidden = true;
  el<HTMLButtonElement>('logout-button').hidden = true;
  selected = null;
  resetRoleEvents();
}

function showAuthed(viewId: string): void {
  hideAllViews();
  el<HTMLElement>(viewId).hidden = false;
  el<HTMLElement>('main-nav').hidden = false;
  el<HTMLElement>('session-info').hidden = false;
  el<HTMLButtonElement>('logout-button').hidden = false;
}

function route(): void {
  // The CSRF token never survives a full load, so the authed views are only
  // honest while this document holds a session. Showing a view also loads
  // its data: nothing else triggers the first fetch.
  const path = window.location.pathname;
  if (api.hasSession() && (path === ACCOUNTS_PATH || path === CONTRIBUTORS_PATH || path === AUDIT_PATH)) {
    if (path === CONTRIBUTORS_PATH) {
      showAuthed('view-contributors');
      void refreshContributors();
    } else if (path === AUDIT_PATH) {
      showAuthed('view-audit');
      auditCursor = '';
      void loadAudit(false);
    } else {
      showAuthed('view-accounts');
    }
    return;
  }
  if (path !== LOGIN_PATH) {
    window.history.replaceState({}, '', LOGIN_PATH);
  }
  showLogin();
}

async function refreshContributors(): Promise<void> {
  const stateSelect = el<HTMLSelectElement>('claims-state');
  claimsState = isClaimState(stateSelect.value) ? stateSelect.value : '';
  claimsCursor = '';
  await loadSnapshot(false);
  await loadClaims(false);
}

function renderHeader(fingerprint: string, expiresAt: string): void {
  el<HTMLElement>('session-fingerprint').textContent = fingerprint;
  el<HTMLElement>('session-expiry').textContent = formatDateTime(expiresAt);
}

function renderResults(items: AccountSummary[]): void {
  const list = el<HTMLUListElement>('results');
  clearChildren(list);
  if (items.length === 0) {
    const empty = document.createElement('li');
    empty.textContent = 'Nenhuma conta encontrada.';
    list.appendChild(empty);
    return;
  }
  for (const item of items) {
    const row = document.createElement('li');
    const button = document.createElement('button');
    button.type = 'button';
    button.textContent = `${item.alias} (${item.status})`;
    button.addEventListener('click', () => {
      if (!isUuid(item.account_id)) {
        setAlert('detail-error', 'Identificador de conta inválido.');
        return;
      }
      void loadDetail(item.account_id);
    });
    row.appendChild(button);
    list.appendChild(row);
  }
}

function renderDetail(detail: AccountDetail): void {
  // Role events belong to one account: their revoke buttons act on that
  // account's assignments, so they must not survive a switch.
  if (selected === null || selected.account_id !== detail.account_id) {
    resetRoleEvents();
  }
  selected = detail;
  el<HTMLElement>('detail-alias').textContent = detail.alias;
  el<HTMLElement>('detail-status').textContent = detail.status;
  el<HTMLElement>('detail-created').textContent = formatDateTime(detail.created_at);
  el<HTMLElement>('detail-paid').textContent =
    detail.paid.valid_until === null || detail.paid.valid_until === ''
      ? detail.paid.state
      : `${detail.paid.state} até ${formatDateTime(detail.paid.valid_until)}`;
  const grants = el<HTMLUListElement>('grants');
  clearChildren(grants);
  if (detail.grants.length === 0) {
    const empty = document.createElement('li');
    empty.textContent = 'Sem grants.';
    grants.appendChild(empty);
    return;
  }
  for (const grant of detail.grants) {
    grants.appendChild(renderGrant(detail, grant));
  }
}

function renderGrant(detail: AccountDetail, grant: GrantView): HTMLLIElement {
  const row = document.createElement('li');
  const until = grant.end_at === null || grant.end_at === '' ? 'sem data final' : formatDateTime(grant.end_at);
  row.appendChild(text('strong', `${grant.tier} · ${grant.state}`));
  row.appendChild(text('span', ` válido até ${until}`));
  if (grant.state === 'ACTIVE') {
    const revokeForm = document.createElement('form');
    revokeForm.className = 'inline-form';
    const reasonInput = document.createElement('input');
    reasonInput.type = 'text';
    reasonInput.name = 'revoke-reason';
    reasonInput.placeholder = 'Motivo da revogação';
    reasonInput.setAttribute('aria-label', 'Motivo da revogação');
    const revokeButton = document.createElement('button');
    revokeButton.type = 'submit';
    revokeButton.textContent = 'Revogar';
    revokeForm.appendChild(reasonInput);
    revokeForm.appendChild(revokeButton);
    revokeForm.addEventListener('submit', (event) => {
      event.preventDefault();
      const reason = reasonInput.value;
      const invalid = reasonError(reason);
      if (invalid !== null) {
        setAlert('detail-error', invalid);
        return;
      }
      if (!window.confirm(`Revogar grant ${grant.tier} de ${detail.alias}?`)) {
        return;
      }
      void (async () => {
        try {
          await api.revokeGrant(grant.id, reason.trim());
          setAlert('detail-error', '');
          await loadDetail(detail.account_id);
        } catch (error) {
          if (!isUnauthorized(error)) {
            setAlert('detail-error', errorMessage(error));
          }
        }
      })();
    });
    row.appendChild(revokeForm);
  }
  return row;
}

async function loadDetail(accountId: string): Promise<void> {
  setAlert('detail-error', '');
  try {
    renderDetail(await api.getAccount(accountId));
  } catch (error) {
    if (!isUnauthorized(error)) {
      setAlert('detail-error', errorMessage(error));
    }
  }
}

function wireLogin(): void {
  const form = el<HTMLFormElement>('login-form');
  const keyInput = el<HTMLInputElement>('admin-key');
  form.addEventListener('submit', (event) => {
    event.preventDefault();
    setAlert('login-error', '');
    const key = keyInput.value;
    if (key.trim() === '') {
      setAlert('login-error', 'Informe a chave de administração.');
      return;
    }
    void (async () => {
      try {
        await api.login(key);
      } catch (error) {
        // Including 401: a wrong key stays on this form with its message.
        setAlert('login-error', errorMessage(error));
        return;
      } finally {
        keyInput.value = '';
      }
      try {
        const session = await api.whoami();
        renderHeader(session.fingerprint, session.expiresAt);
      } catch {
        return;
      }
      goAccounts();
    })();
  });
}

function wireAccounts(): void {
  const searchForm = el<HTMLFormElement>('search-form');
  const queryInput = el<HTMLInputElement>('search-q');
  const logoutButton = el<HTMLButtonElement>('logout-button');

  searchForm.addEventListener('submit', (event) => {
    event.preventDefault();
    const invalid = searchError(queryInput.value);
    if (invalid !== null) {
      setAlert('search-error', invalid);
      return;
    }
    setAlert('search-error', '');
    void (async () => {
      try {
        renderResults(await api.searchAccounts(queryInput.value.trim()));
      } catch (error) {
        if (!isUnauthorized(error)) {
          setAlert('search-error', errorMessage(error));
        }
      }
    })();
  });

  const grantForm = el<HTMLFormElement>('grant-form');
  const tierSelect = el<HTMLSelectElement>('grant-tier');
  const endInput = el<HTMLInputElement>('grant-end');
  const noEndCheckbox = el<HTMLInputElement>('grant-no-end');
  endInput.value = defaultEndDate();
  grantForm.addEventListener('submit', (event) => {
    event.preventDefault();
    if (selected === null) {
      setAlert('grant-error', 'Selecione uma conta primeiro.');
      return;
    }
    if (!isTier(tierSelect.value)) {
      setAlert('grant-error', 'Tier inválido.');
      return;
    }
    const endRaw = noEndCheckbox.checked ? '' : endInput.value;
    const endInvalid = endDateError(endRaw);
    if (endInvalid !== null) {
      setAlert('grant-error', endInvalid);
      return;
    }
    const reasonInput = el<HTMLTextAreaElement>('grant-reason');
    const reasonInvalid = reasonError(reasonInput.value);
    if (reasonInvalid !== null) {
      setAlert('grant-error', reasonInvalid);
      return;
    }
    const endLabel = endRaw === '' ? 'sem data final' : endRaw;
    if (!window.confirm(`Conceder ${tierSelect.value} para ${selected.alias} até ${endLabel}?`)) {
      return;
    }
    const accountId = selected.account_id;
    void (async () => {
      try {
        await api.createGrant(accountId, {
          tier: tierSelect.value,
          endAt: endRaw === '' ? null : toEndOfDayIso(endRaw),
          reason: reasonInput.value.trim(),
          replace: false,
        });
        setAlert('grant-error', '');
        reasonInput.value = '';
        await loadDetail(accountId);
      } catch (error) {
        if (!isUnauthorized(error)) {
          setAlert('grant-error', errorMessage(error));
        }
      }
    })();
  });

  logoutButton.addEventListener('click', () => {
    void (async () => {
      try {
        await api.logout(LOGOUT_REASON);
      } catch {
        // Session already gone; still leave.
      }
      goLogin();
    })();
  });
}

function boot(): void {
  wireLogin();
  wireAccounts();
  wireNav();
  wireContributors();
  wireRoles();
  wireAudit();
  window.addEventListener('popstate', route);
  route();
}

function start(): void {
  boot();
}

if (typeof document !== 'undefined') {
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', start);
  } else {
    start();
  }
}

let claimsCursor = '';
let claimsState = '';
let auditCursor = '';

function renderSnapshotRow(item: SnapshotContributor): HTMLLIElement {
  const row = document.createElement('li');
  row.appendChild(text('strong', item.login));
  const state = item.linked_account_id
    ? ` vinculado a ${item.linked_account_id}`
    : ' sem vínculo';
  row.appendChild(text('span', state));
  if (item.claim_id) {
    row.appendChild(text('span', ` · claim ${item.claim_state ?? ''} (${item.claim_proof_status ?? ''})`));
  }
  return row;
}

async function loadSnapshot(refresh: boolean): Promise<void> {
  setAlert('snapshot-error', '');
  try {
    const snapshot = await api.githubContributors(refresh);
    el<HTMLElement>('snapshot-repo').textContent = snapshot.repo;
    el<HTMLElement>('snapshot-fetched').textContent = formatDateTime(snapshot.fetched_at);
    el<HTMLElement>('snapshot-stale').hidden = !snapshot.stale;
    const list = el<HTMLUListElement>('snapshot-list');
    clearChildren(list);
    if (snapshot.contributors.length === 0) {
      const empty = document.createElement('li');
      empty.textContent = 'Snapshot vazio.';
      list.appendChild(empty);
      return;
    }
    for (const item of snapshot.contributors) {
      list.appendChild(renderSnapshotRow(item));
    }
  } catch (error) {
    if (!isUnauthorized(error)) {
      setAlert('snapshot-error', errorMessage(error));
    }
  }
}

function renderClaimRow(claim: ClaimView): HTMLLIElement {
  const row = document.createElement('li');
  row.appendChild(text('strong', `${claim.github_login} · ${claim.state}`));
  row.appendChild(text('span', ` prova ${claim.proof_status}`));
  if (claim.state === 'PENDING') {
    for (const action of ['approve', 'reject'] as const) {
      const form = document.createElement('form');
      form.className = 'inline-form';
      const reasonInput = document.createElement('input');
      reasonInput.type = 'text';
      reasonInput.name = `${action}-reason`;
      reasonInput.placeholder = 'Motivo da decisão';
      reasonInput.setAttribute('aria-label', 'Motivo da decisão');
      form.appendChild(reasonInput);
      if (action === 'approve') {
        const overrideLabel = document.createElement('label');
        overrideLabel.className = 'checkbox';
        const overrideInput = document.createElement('input');
        overrideInput.type = 'checkbox';
        overrideInput.name = 'override';
        overrideLabel.appendChild(overrideInput);
        overrideLabel.appendChild(text('span', 'Override'));
        form.appendChild(overrideLabel);
      }
      const button = document.createElement('button');
      button.type = 'submit';
      button.textContent = action === 'approve' ? 'Aprovar' : 'Rejeitar';
      form.appendChild(button);
      form.addEventListener('submit', (event) => {
        event.preventDefault();
        const reason = reasonInput.value;
        const invalid = reasonError(reason);
        if (invalid !== null) {
          setAlert('claims-error', invalid);
          return;
        }
        const override = action === 'approve'
          && (form.querySelector('input[name="override"]') as HTMLInputElement | null)?.checked === true;
        if (!window.confirm(`${action === 'approve' ? 'Aprovar' : 'Rejeitar'} claim de ${claim.github_login}?`)) {
          return;
        }
        void (async () => {
          try {
            if (action === 'approve') {
              await api.approveClaim(claim.claim_id, reason.trim(), override);
            } else {
              await api.rejectClaim(claim.claim_id, reason.trim());
            }
            setAlert('claims-error', '');
            await loadClaims(false);
          } catch (error) {
            if (!isUnauthorized(error)) {
              setAlert('claims-error', errorMessage(error));
            }
          }
        })();
      });
      row.appendChild(form);
    }
  }
  return row;
}

async function loadClaims(append: boolean): Promise<void> {
  setAlert('claims-error', '');
  try {
    const page = await api.listClaims(claimsState, append ? claimsCursor : '');
    const list = el<HTMLUListElement>('claims-list');
    if (!append) {
      clearChildren(list);
    }
    for (const claim of page.items) {
      list.appendChild(renderClaimRow(claim));
    }
    claimsCursor = page.next_cursor;
    el<HTMLButtonElement>('claims-more').hidden = page.next_cursor === '';
  } catch (error) {
    if (!isUnauthorized(error)) {
      setAlert('claims-error', errorMessage(error));
    }
  }
}

function wireContributors(): void {
  const snapshotForm = el<HTMLFormElement>('snapshot-form');
  snapshotForm.addEventListener('submit', (event) => {
    event.preventDefault();
    void loadSnapshot(true);
  });

  const filterForm = el<HTMLFormElement>('claims-filter-form');
  const stateSelect = el<HTMLSelectElement>('claims-state');
  filterForm.addEventListener('submit', (event) => {
    event.preventDefault();
    if (!isClaimState(stateSelect.value)) {
      setAlert('claims-error', 'Estado inválido.');
      return;
    }
    claimsState = stateSelect.value;
    claimsCursor = '';
    void loadClaims(false);
  });
  el<HTMLButtonElement>('claims-more').addEventListener('click', () => {
    void loadClaims(true);
  });

  const linkForm = el<HTMLFormElement>('link-form');
  linkForm.addEventListener('submit', (event) => {
    event.preventDefault();
    const accountInput = el<HTMLInputElement>('link-account');
    const loginInput = el<HTMLInputElement>('link-login');
    const reasonInput = el<HTMLTextAreaElement>('link-reason');
    const overrideInput = el<HTMLInputElement>('link-override');
    if (!isUuid(accountInput.value.trim())) {
      setAlert('link-error', 'ID da conta inválido.');
      return;
    }
    const loginInvalid = githubLoginError(loginInput.value);
    if (loginInvalid !== null) {
      setAlert('link-error', loginInvalid);
      return;
    }
    const reasonInvalid = reasonError(reasonInput.value);
    if (reasonInvalid !== null) {
      setAlert('link-error', reasonInvalid);
      return;
    }
    if (!window.confirm(`Vincular ${loginInput.value.trim()} à conta?`)) {
      return;
    }
    void (async () => {
      try {
        await api.linkGitHub(
          accountInput.value.trim(),
          loginInput.value.trim(),
          reasonInput.value.trim(),
          overrideInput.checked,
        );
        setAlert('link-error', '');
        reasonInput.value = '';
        await loadSnapshot(false);
      } catch (error) {
        if (!isUnauthorized(error)) {
          setAlert('link-error', errorMessage(error));
        }
      }
    })();
  });

  const visibilityForm = el<HTMLFormElement>('visibility-form');
  const setVisibility = (hidden: boolean): void => {
    const accountInput = el<HTMLInputElement>('visibility-account');
    const reasonInput = el<HTMLTextAreaElement>('visibility-reason');
    if (!isUuid(accountInput.value.trim())) {
      setAlert('visibility-error', 'ID da conta inválido.');
      return;
    }
    const reasonInvalid = reasonError(reasonInput.value);
    if (reasonInvalid !== null) {
      setAlert('visibility-error', reasonInvalid);
      return;
    }
    if (!window.confirm(`${hidden ? 'Ocultar' : 'Exibir'} perfil da conta?`)) {
      return;
    }
    void (async () => {
      try {
        await api.setProfileHidden(accountInput.value.trim(), reasonInput.value.trim(), hidden);
        setAlert('visibility-error', '');
        reasonInput.value = '';
      } catch (error) {
        if (!isUnauthorized(error)) {
          setAlert('visibility-error', errorMessage(error));
        }
      }
    })();
  };
  visibilityForm.addEventListener('click', (event) => {
    const button = (event.target as HTMLElement).closest('button[name="visibility-action"]');
    if (button === null) {
      return;
    }
    event.preventDefault();
    setVisibility(button.getAttribute('value') === 'hide');
  });
}

function renderAssignmentRow(assignment: StaffAssignment): HTMLLIElement {
  const row = document.createElement('li');
  const until = assignment.end_at === null || assignment.end_at === '' ? 'sem data final' : formatDateTime(assignment.end_at);
  row.appendChild(text('strong', `${assignment.label} · ${assignment.state}`));
  row.appendChild(text('span', ` válido até ${until}`));
  if (assignment.state === 'ACTIVE') {
    const revokeForm = document.createElement('form');
    revokeForm.className = 'inline-form';
    const reasonInput = document.createElement('input');
    reasonInput.type = 'text';
    reasonInput.name = 'role-revoke-reason';
    reasonInput.placeholder = 'Motivo da revogação';
    reasonInput.setAttribute('aria-label', 'Motivo da revogação');
    const revokeButton = document.createElement('button');
    revokeButton.type = 'submit';
    revokeButton.textContent = 'Revogar papel';
    revokeForm.appendChild(reasonInput);
    revokeForm.appendChild(revokeButton);
    revokeForm.addEventListener('submit', (submitEvent) => {
      submitEvent.preventDefault();
      const reason = reasonInput.value;
      const invalid = reasonError(reason);
      if (invalid !== null) {
        setAlert('role-error', invalid);
        return;
      }
      if (!window.confirm(`Revogar papel ${assignment.label}?`)) {
        return;
      }
      void (async () => {
        try {
          const revoked = await api.revokeRole(assignment.id, reason.trim());
          setAlert('role-error', '');
          clearChildren(row);
          row.appendChild(text('strong', `${revoked.label} · ${revoked.state}`));
        } catch (error) {
          if (!isUnauthorized(error)) {
            setAlert('role-error', errorMessage(error));
          }
        }
      })();
    });
    row.appendChild(revokeForm);
  }
  return row;
}

function resetRoleEvents(): void {
  const list = el<HTMLUListElement>('role-events');
  clearChildren(list);
  const hint = document.createElement('li');
  hint.textContent = 'Nenhum papel atribuído nesta sessão — atribua abaixo ou consulte a auditoria.';
  list.appendChild(hint);
}

function wireRoles(): void {
  const roleForm = el<HTMLFormElement>('role-form');
  const roleSelect = el<HTMLSelectElement>('role-select');
  const endInput = el<HTMLInputElement>('role-end');
  const noEndCheckbox = el<HTMLInputElement>('role-no-end');
  roleForm.addEventListener('submit', (event) => {
    event.preventDefault();
    if (selected === null) {
      setAlert('role-error', 'Selecione uma conta primeiro.');
      return;
    }
    if (!isRole(roleSelect.value)) {
      setAlert('role-error', 'Papel inválido.');
      return;
    }
    const endRaw = noEndCheckbox.checked ? '' : endInput.value;
    const endInvalid = endDateError(endRaw);
    if (endInvalid !== null) {
      setAlert('role-error', endInvalid);
      return;
    }
    const reasonInput = el<HTMLTextAreaElement>('role-reason');
    const reasonInvalid = reasonError(reasonInput.value);
    if (reasonInvalid !== null) {
      setAlert('role-error', reasonInvalid);
      return;
    }
    if (!window.confirm(`Atribuir papel ${roleSelect.value} para ${selected.alias}?`)) {
      return;
    }
    const accountId = selected.account_id;
    void (async () => {
      try {
        const assignment = await api.assignRole(accountId, roleSelect.value, endRaw === '' ? null : toEndOfDayIso(endRaw), reasonInput.value.trim());
        setAlert('role-error', '');
        reasonInput.value = '';
        if (selected === null || selected.account_id !== accountId) {
          // The admin opened another account while this request was in
          // flight; its row would show (and revoke) on the wrong account.
          return;
        }
        const list = el<HTMLUListElement>('role-events');
        clearChildren(list);
        list.appendChild(renderAssignmentRow(assignment));
      } catch (error) {
        if (!isUnauthorized(error)) {
          setAlert('role-error', errorMessage(error));
        }
      }
    })();
  });
}

function renderAuditRow(event: AuditEvent): HTMLLIElement {
  const row = document.createElement('li');
  row.appendChild(text('strong', event.action));
  row.appendChild(text('span', ` · alvo ${event.target_id} · em ${formatDateTime(event.at)} · ${event.reason}`));
  return row;
}

async function loadAudit(append: boolean): Promise<void> {
  setAlert('audit-error', '');
  const action = el<HTMLInputElement>('audit-action').value.trim();
  const target = el<HTMLInputElement>('audit-target').value.trim();
  try {
    const page = await api.listAudit(action, target, append ? auditCursor : '');
    const list = el<HTMLUListElement>('audit-list');
    if (!append) {
      clearChildren(list);
    }
    for (const event of page.items) {
      list.appendChild(renderAuditRow(event));
    }
    auditCursor = page.next_cursor;
    el<HTMLButtonElement>('audit-more').hidden = page.next_cursor === '';
  } catch (error) {
    if (!isUnauthorized(error)) {
      setAlert('audit-error', errorMessage(error));
    }
  }
}

function wireAudit(): void {
  const filterForm = el<HTMLFormElement>('audit-filter-form');
  filterForm.addEventListener('submit', (event) => {
    event.preventDefault();
    auditCursor = '';
    void loadAudit(false);
  });
  el<HTMLButtonElement>('audit-more').addEventListener('click', () => {
    void loadAudit(true);
  });
}
