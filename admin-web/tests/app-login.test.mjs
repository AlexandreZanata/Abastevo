import test, { after, before } from 'node:test';
import assert from 'node:assert/strict';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { compileApp, jsonResponse, settle } from './support/fake-dom.mjs';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
let app;
before(() => { app = compileApp(root); });
after(() => app.cleanup());

const denied = () => jsonResponse(401, { error: { code: 'admin.denied', message: 'x' } });

function submitLogin(page, key) {
  page.byId('admin-key').value = key;
  page.byId('login-form').dispatch('submit');
}

test('login: wrong key shows the invalid-key error and stays on login without a redirect loop', async () => {
  const page = await app.mount({ fetchHandler: denied });
  submitLogin(page, 'wrong-key');
  await settle();
  assert.equal(page.byId('login-error').textContent, 'Chave ou sessão inválida.');
  assert.equal(page.window.location.pathname, '/admin/login/');
  assert.equal(page.byId('view-login').hidden, false);
  assert.equal(page.byId('admin-key').value, '', 'key cleared after submit');
  assert.deepEqual(page.calls.map((c) => `${c.method} ${c.input}`), ['POST /v1/admin/session']);
});

test('login: correct key after a wrong one clears the error and opens accounts', async () => {
  const page = await app.mount({
    fetchHandler: (input, init, calls) => {
      if (init.method === 'POST' && calls.length === 1) return denied();
      if (init.method === 'POST') return jsonResponse(200, { csrf_token: 'csrf-1', expires_at: '2026-10-27T20:00:00Z' });
      return jsonResponse(200, { key_fingerprint: 'fp-1', expires_at: '2026-10-27T20:00:00Z' });
    },
  });
  submitLogin(page, 'wrong-key');
  await settle();
  assert.equal(page.byId('login-error').textContent, 'Chave ou sessão inválida.');
  submitLogin(page, 'right-key');
  await settle();
  assert.equal(page.byId('login-error').textContent, '');
  assert.equal(page.window.location.pathname, '/admin/accounts/');
  assert.equal(page.byId('view-accounts').hidden, false);
});

test('session expiry: 401 on an authenticated call still returns to login', async () => {
  const page = await app.mount({
    fetchHandler: (input, init) => {
      if (input === '/v1/admin/session' && init.method === 'POST') {
        return jsonResponse(200, { csrf_token: 'csrf-1', expires_at: '2026-10-27T20:00:00Z' });
      }
      if (input === '/v1/admin/session') {
        return jsonResponse(200, { key_fingerprint: 'fp-1', expires_at: '2026-10-27T20:00:00Z' });
      }
      return denied();
    },
  });
  submitLogin(page, 'right-key');
  await settle();
  assert.equal(page.window.location.pathname, '/admin/accounts/');
  page.byId('search-q').value = 'abc';
  page.byId('search-form').dispatch('submit');
  await settle();
  assert.equal(page.window.location.pathname, '/admin/login/');
  assert.equal(page.byId('view-login').hidden, false);
  assert.equal(page.byId('login-error').textContent, '');
});
