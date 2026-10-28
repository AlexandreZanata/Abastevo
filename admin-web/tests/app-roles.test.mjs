import test, { after, before } from 'node:test';
import assert from 'node:assert/strict';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { compileApp, jsonResponse, settle } from './support/fake-dom.mjs';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
let app;
before(() => { app = compileApp(root); });
after(() => app.cleanup());

const ACCOUNT_A = 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa';
const ACCOUNT_B = 'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb';

function detail(id, alias) {
  return {
    account_id: id, alias, status: 'ACTIVE', created_at: '2026-01-01T00:00:00Z',
    grants: [], paid: { state: 'NONE', valid_until: null },
  };
}

function handler(input, init) {
  if (input === '/v1/admin/session' && init.method === 'POST') {
    return jsonResponse(200, { csrf_token: 'csrf-1', expires_at: '2026-10-27T20:00:00Z' });
  }
  if (input === '/v1/admin/session') {
    return jsonResponse(200, { key_fingerprint: 'fp-1', expires_at: '2026-10-27T20:00:00Z' });
  }
  if (input.startsWith('/v1/admin/accounts?q=')) {
    return jsonResponse(200, {
      items: [
        { account_id: ACCOUNT_A, alias: 'alpha', status: 'ACTIVE', created_at: '2026-01-01T00:00:00Z' },
        { account_id: ACCOUNT_B, alias: 'bravo', status: 'ACTIVE', created_at: '2026-01-01T00:00:00Z' },
      ],
    });
  }
  if (input === `/v1/admin/accounts/${ACCOUNT_A}`) return jsonResponse(200, detail(ACCOUNT_A, 'alpha'));
  if (input === `/v1/admin/accounts/${ACCOUNT_B}`) return jsonResponse(200, detail(ACCOUNT_B, 'bravo'));
  if (input === `/v1/admin/accounts/${ACCOUNT_A}/roles`) {
    return jsonResponse(201, {
      id: 'assignment-a', account_id: ACCOUNT_A, role: 'support_agent', label: 'Suporte A',
      state: 'ACTIVE', start_at: '2026-10-27T12:00:00Z', end_at: null, reason: '0123456789',
      created_at: '2026-10-27T12:00:00Z', revoked_at: null,
    });
  }
  return jsonResponse(404, { error: { code: 'admin.not-found', message: 'x' } });
}

async function openAccounts(fetchHandler = handler) {
  const page = await app.mount({ fetchHandler });
  page.byId('admin-key').value = 'right-key';
  page.byId('login-form').dispatch('submit');
  await settle();
  page.byId('search-q').value = 'alp';
  page.byId('search-form').dispatch('submit');
  await settle();
  const pick = async (alias) => {
    const button = page.byId('results').querySelectorAll('button').find((b) => b.textContent.startsWith(alias));
    button.click();
    await settle();
  };
  return { page, pick };
}

async function assignRoleTo(page) {
  page.byId('role-select').value = 'support_agent';
  page.byId('role-no-end').checked = true;
  page.byId('role-reason').value = 'synthetic role reason';
  page.byId('role-form').dispatch('submit');
  await settle();
}

test('roles: opening another account clears the previous account role events and revoke form', async () => {
  const { page, pick } = await openAccounts();
  await pick('alpha');
  assert.equal(page.byId('detail-alias').textContent, 'alpha');
  await assignRoleTo(page);
  const events = page.byId('role-events');
  assert.match(events.textContent, /Suporte A/);
  assert.equal(events.querySelectorAll('form').length, 1, 'A has a revoke form');

  await pick('bravo');
  assert.equal(page.byId('detail-alias').textContent, 'bravo');
  assert.doesNotMatch(events.textContent, /Suporte A/, "A's assignment must not show on B");
  assert.equal(events.querySelectorAll('form').length, 0, "no revoke button for A's assignment on B");
  assert.ok(!page.calls.some((c) => c.input.includes('/role-assignments/')), 'nothing revoked');
});

test('roles: reloading the same account keeps its role events', async () => {
  const { page, pick } = await openAccounts();
  await pick('alpha');
  await assignRoleTo(page);
  await pick('alpha');
  assert.match(page.byId('role-events').textContent, /Suporte A/);
});

test('roles: an assignment answered after switching accounts is not shown on the new account', async () => {
  let release;
  const gate = new Promise((resolve) => { release = resolve; });
  const { page, pick } = await openAccounts(async (input, init) => {
    if (input === `/v1/admin/accounts/${ACCOUNT_A}/roles`) await gate;
    return handler(input, init);
  });
  await pick('alpha');
  page.byId('role-select').value = 'support_agent';
  page.byId('role-no-end').checked = true;
  page.byId('role-reason').value = 'synthetic role reason';
  page.byId('role-form').dispatch('submit');
  await pick('bravo');
  assert.equal(page.byId('detail-alias').textContent, 'bravo');
  release();
  await settle();
  assert.ok(page.calls.some((c) => c.input === `/v1/admin/accounts/${ACCOUNT_A}/roles`), 'assign was sent for A');
  assert.doesNotMatch(page.byId('role-events').textContent, /Suporte A/);
  assert.equal(page.byId('role-events').querySelectorAll('form').length, 0);
});

test('roles: logging out clears the role events', async () => {
  const { page, pick } = await openAccounts();
  await pick('alpha');
  await assignRoleTo(page);
  page.byId('logout-button').click();
  await settle();
  assert.equal(page.window.location.pathname, '/admin/login/');
  assert.doesNotMatch(page.byId('role-events').textContent, /Suporte A/);
});
