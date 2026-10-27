import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { execFileSync } from 'node:child_process';
import { fileURLToPath, pathToFileURL } from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');

async function loadApi() {
  const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'adm-web-api-'));
  try {
    const localTsc = path.join(root, 'node_modules', '.bin', 'tsc');
    execFileSync(fs.existsSync(localTsc) ? localTsc : 'tsc', [
      '--project', path.join(root, 'tsconfig.json'), '--outDir', tmpDir,
    ], { cwd: root, stdio: 'pipe' });
    return await import(pathToFileURL(path.join(tmpDir, 'api.js')).href);
  } finally {
    fs.rmSync(tmpDir, { recursive: true, force: true });
  }
}

const UUID_PATTERN = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;

function fakeFetch(handler) {
  const calls = [];
  const fn = async (input, init = {}) => {
    calls.push({ input, init });
    return handler(input, init);
  };
  fn.calls = calls;
  return fn;
}

function jsonResponse(status, body, extraHeaders = {}) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json', ...extraHeaders },
  });
}

const denied = () => jsonResponse(401, { error: { code: 'admin.denied', message: 'x' } });

test('login: relative path, same-origin credentials, csrf stored in memory', async () => {
  const { AdminApi } = await loadApi();
  const fetchFn = fakeFetch(() => jsonResponse(200, { csrf_token: 'csrf-1', expires_at: '2026-01-01T00:00:00Z' }));
  const api = new AdminApi(fetchFn, () => {});
  assert.equal(api.hasSession(), false);
  const session = await api.login('secret-key');
  assert.equal(session.expiresAt, '2026-01-01T00:00:00Z');
  assert.equal(api.hasSession(), true);
  assert.equal(fetchFn.calls.length, 1);
  const [call] = fetchFn.calls;
  assert.equal(call.input, '/v1/admin/session');
  assert.equal(call.init.method, 'POST');
  assert.equal(call.init.credentials, 'same-origin');
  assert.equal(call.init.headers['Content-Type'], 'application/json');
  assert.deepEqual(JSON.parse(call.init.body), { key: 'secret-key' });
  assert.ok(!('X-Admin-CSRF' in call.init.headers), 'login has no csrf yet');
  assert.match(call.init.headers['Idempotency-Key'], UUID_PATTERN);
});

test('mutations: csrf header, fresh idempotency key, logout clears session', async () => {
  const { AdminApi } = await loadApi();
  const fetchFn = fakeFetch((input) => {
    if (input === '/v1/admin/session' && fetchFn.calls.length === 1) {
      return jsonResponse(200, { csrf_token: 'csrf-9', expires_at: '2026-01-01T00:00:00Z' });
    }
    return new Response(null, { status: 204 });
  });
  const api = new AdminApi(fetchFn, () => {});
  await api.login('k');
  await api.logout('admin logout via web panel');
  assert.equal(fetchFn.calls.length, 2);
  const [, logoutCall] = fetchFn.calls;
  assert.equal(logoutCall.init.headers['X-Admin-CSRF'], 'csrf-9');
  assert.match(logoutCall.init.headers['Idempotency-Key'], UUID_PATTERN);
  assert.deepEqual(JSON.parse(logoutCall.init.body), { reason: 'admin logout via web panel' });
  assert.equal(api.hasSession(), false);
  await assert.rejects(api.logout('x'), /no csrf token/);
});

test('client: fetch is invoked without a receiver (DOM illegal-invocation guard)', async () => {
  const { AdminApi } = await loadApi();
  async function strictFetch() {
    assert.equal(this, undefined, 'fetch must not depend on its receiver');
    return jsonResponse(200, { csrf_token: 'csrf-1', expires_at: '2026-01-01T00:00:00Z' });
  }
  const api = new AdminApi(strictFetch, () => {});
  await api.login('k');
  assert.equal(api.hasSession(), true);
});

test('mutations: csrf sent and idempotency keys differ per call', async () => {
  const { AdminApi } = await loadApi();
  const fetchFn = fakeFetch((input, init) => {
    if (init.method === 'POST' && input === '/v1/admin/session') {
      return jsonResponse(200, { csrf_token: 'csrf-1', expires_at: '2026-01-01T00:00:00Z' });
    }
    return jsonResponse(200, { id: 'g1', state: 'REVOKED' });
  });
  const api = new AdminApi(fetchFn, () => {});
  await api.login('k');
  await api.revokeGrant('g1', '0123456789');
  await api.revokeGrant('g2', '0123456789');
  const [, first, second] = fetchFn.calls;
  for (const call of [first, second]) {
    assert.ok(call.init.headers['X-Admin-CSRF'], 'csrf header present');
    assert.match(call.init.headers['Idempotency-Key'], UUID_PATTERN);
  }
  assert.notEqual(first.init.headers['Idempotency-Key'], second.init.headers['Idempotency-Key']);
});

test('401: unauthorized handler fires and server code is preserved', async () => {
  const { AdminApi, isUnauthorized } = await loadApi();
  let fired = 0;
  const api = new AdminApi(fakeFetch(denied), () => { fired += 1; });
  const error = await api.searchAccounts('abc').then(
    () => { throw new Error('should have thrown'); },
    (err) => err,
  );
  assert.equal(fired, 1);
  assert.ok(isUnauthorized(error));
  assert.equal(error.status, 401);
  assert.equal(error.code, 'admin.denied');
});

test('429: retry-after parsed and messaged', async () => {
  const { AdminApi, errorMessage } = await loadApi();
  const api = new AdminApi(
    fakeFetch(() => jsonResponse(429, { error: { code: 'admin.rate-limited', message: 'x' } }, { 'Retry-After': '90' })),
    () => {},
  );
  const error = await api.searchAccounts('abc').then(
    () => { throw new Error('should have thrown'); },
    (err) => err,
  );
  assert.equal(error.retryAfter, 90);
  assert.match(errorMessage(error), /90/);
});

test('error mapping: contract codes to pt-BR messages', async () => {
  const { errorMessage, ApiError } = await loadApi();
  const cases = [
    ['admin.denied', 'Chave ou sessão inválida.'],
    ['admin.csrf', 'Falha de CSRF ou origem. Recarregue a página e tente de novo.'],
    ['admin.invalid', 'Entrada inválida. Revise os campos.'],
    ['admin.conflict', 'Conflito de idempotência. Tente de novo.'],
    ['admin.not-found', 'Recurso não encontrado.'],
    ['entitlement.exists', 'Já existe um grant ativo desse tier para a conta.'],
    ['entitlement.account-ineligible', 'Conta suspensa, excluída ou inexistente.'],
    ['entitlement.invalid', 'Tier ou data final inválidos.'],
    ['entitlement.already-revoked', 'Grant já revogado.'],
  ];
  for (const [code, message] of cases) {
    assert.equal(errorMessage(new ApiError(400, code, 'x')), message, code);
  }
  assert.match(errorMessage(new ApiError(500, 'mystery.code', 'x')), /mystery\.code/);
  assert.match(errorMessage(new TypeError('boom')), /rede/);
});

test('grant paths and bodies match the admin contract', async () => {
  const { AdminApi } = await loadApi();
  const fetchFn = fakeFetch((input, init) => {
    if (input === '/v1/admin/session') {
      return jsonResponse(200, { csrf_token: 'csrf-1', expires_at: '2026-01-01T00:00:00Z' });
    }
    if (input.startsWith('/v1/admin/accounts/') && input.endsWith('/grants')) {
      return jsonResponse(201, { grant: { id: 'g1' }, replaced: null });
    }
    if (input.includes('/revoke')) {
      return jsonResponse(200, { id: 'g1', state: 'REVOKED' });
    }
    return jsonResponse(200, {});
  });
  const api = new AdminApi(fetchFn, () => {});
  await api.login('k');
  const created = await api.createGrant('account-1', { tier: 'PRO', endAt: '2026-02-01T00:00:00Z', reason: '0123456789', replace: false });
  assert.equal(created.grant.id, 'g1');
  const createCall = fetchFn.calls[1];
  assert.equal(createCall.input, '/v1/admin/accounts/account-1/grants');
  assert.deepEqual(JSON.parse(createCall.init.body), {
    tier: 'PRO', end_at: '2026-02-01T00:00:00Z', reason: '0123456789', replace: false,
  });
  const revoked = await api.revokeGrant('g1', '0123456789');
  assert.equal(revoked.state, 'REVOKED');
  assert.equal(fetchFn.calls[2].input, '/v1/admin/grants/g1/revoke');
});
