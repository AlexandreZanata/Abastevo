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
  type GrantView,
} from './api.js';
import {
  defaultEndDate,
  endDateError,
  isTier,
  isUuid,
  reasonError,
  searchError,
  toEndOfDayIso,
} from './validators.js';

const LOGOUT_REASON = 'admin logout via web panel';
const LOGIN_PATH = '/admin/login/';
const ACCOUNTS_PATH = '/admin/accounts/';

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

function showLogin(): void {
  el<HTMLElement>('view-login').hidden = false;
  el<HTMLElement>('view-accounts').hidden = true;
  el<HTMLElement>('session-info').hidden = true;
  el<HTMLButtonElement>('logout-button').hidden = true;
  selected = null;
}

function showAccounts(): void {
  el<HTMLElement>('view-login').hidden = true;
  el<HTMLElement>('view-accounts').hidden = false;
  el<HTMLElement>('session-info').hidden = false;
  el<HTMLButtonElement>('logout-button').hidden = false;
}

function route(): void {
  // The CSRF token never survives a full load, so the accounts view is only
  // honest while this document holds a session.
  if (window.location.pathname === ACCOUNTS_PATH && api.hasSession()) {
    showAccounts();
    return;
  }
  if (window.location.pathname !== LOGIN_PATH) {
    window.history.replaceState({}, '', LOGIN_PATH);
  }
  showLogin();
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
        if (!isUnauthorized(error)) {
          setAlert('login-error', errorMessage(error));
        }
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
