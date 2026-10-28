import test, { after, before } from 'node:test';
import assert from 'node:assert/strict';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { compileApp, jsonResponse, settle } from './support/fake-dom.mjs';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
let app;
before(() => { app = compileApp(root); });
after(() => app.cleanup());

function claim(id) {
  return {
    claim_id: id, github_login: `login-${id}`, state: 'APPROVED', proof_status: 'VERIFIED',
    override: false, created_at: '2026-10-01T00:00:00Z', expires_at: '2026-11-01T00:00:00Z',
  };
}

function auditEvent(id) {
  return {
    id, at: '2026-10-27T12:00:00Z', action: `action-${id}`, target_type: 'account',
    target_id: `target-${id}`, reason: 'synthetic reason', request_id: 'r', client_ip_hash: 'h',
  };
}

// Pages keyed by "cursor" query value; each response can be held by a gate.
function pagedServer({ pages, gates = {} }) {
  return async (input, init) => {
    if (input === '/v1/admin/session' && init.method === 'POST') {
      return jsonResponse(200, { csrf_token: 'csrf-1', expires_at: '2026-10-27T20:00:00Z' });
    }
    if (input === '/v1/admin/session') {
      return jsonResponse(200, { key_fingerprint: 'fp-1', expires_at: '2026-10-27T20:00:00Z' });
    }
    if (input.startsWith('/v1/admin/github/contributors')) {
      return jsonResponse(200, { repo: 'o/r', fetched_at: '2026-10-27T12:00:00Z', stale: false, contributors: [] });
    }
    const url = new URL(input, 'https://admin.test');
    const key = `${url.pathname}#${url.searchParams.get('cursor') ?? ''}`;
    if (gates[key] !== undefined) await gates[key];
    const page = pages[key];
    if (page === undefined) return jsonResponse(400, { error: { code: 'admin.invalid', message: 'x' } });
    return jsonResponse(200, page);
  };
}

function gate() {
  let release;
  const promise = new Promise((resolve) => { release = resolve; });
  return { promise, release };
}

async function loggedIn(fetchHandler, route) {
  const page = await app.mount({ fetchHandler });
  page.byId('admin-key').value = 'right-key';
  page.byId('login-form').dispatch('submit');
  await settle();
  page.window.popTo(route);
  await settle();
  return page;
}

const CLAIMS = '/v1/admin/contributor-claims';
const AUDIT = '/v1/admin/audit';

test('claims paging: "more" is disabled while loading and a double click appends the page once', async () => {
  const second = gate();
  const page = await loggedIn(pagedServer({
    pages: {
      [`${CLAIMS}#`]: { items: [claim('c1')], next_cursor: 'cur-2' },
      [`${CLAIMS}#cur-2`]: { items: [claim('c2')], next_cursor: '' },
    },
    gates: { [`${CLAIMS}#cur-2`]: second.promise },
  }), '/admin/contributors/');
  const more = page.byId('claims-more');
  assert.equal(more.hidden, false);
  more.click();
  assert.equal(more.disabled, true, 'disabled while the page loads');
  more.click(); // ignored by the browser while disabled
  more.dispatch('click'); // and by the handler even if a click slips through
  second.release();
  await settle();
  const list = page.byId('claims-list').textContent;
  assert.equal(list.split('login-c2').length - 1, 1, 'page 2 appended exactly once');
  assert.equal(page.calls.filter((c) => c.input.includes('cursor=cur-2')).length, 1);
  assert.equal(more.disabled, false);
  assert.equal(more.hidden, true, 'no further pages');
});

test('audit paging: "more" is disabled while loading and a double click appends the page once', async () => {
  const second = gate();
  const page = await loggedIn(pagedServer({
    pages: {
      [`${AUDIT}#`]: { items: [auditEvent(1)], next_cursor: 'a-2' },
      [`${AUDIT}#a-2`]: { items: [auditEvent(2)], next_cursor: 'a-3' },
    },
    gates: { [`${AUDIT}#a-2`]: second.promise },
  }), '/admin/audit/');
  const more = page.byId('audit-more');
  more.click();
  assert.equal(more.disabled, true);
  more.dispatch('click');
  second.release();
  await settle();
  assert.equal(page.byId('audit-list').textContent.split('action-2').length - 1, 1);
  assert.equal(page.calls.filter((c) => c.input.includes('cursor=a-2')).length, 1);
  assert.equal(more.disabled, false);
  assert.equal(more.hidden, false, 'next page still available');
});

test('audit paging: "more" keeps the filter its cursor was issued for', async () => {
  const page = await loggedIn(pagedServer({
    pages: {
      [`${AUDIT}#`]: { items: [auditEvent(1)], next_cursor: 'a-2' },
      [`${AUDIT}#a-2`]: { items: [auditEvent(2)], next_cursor: '' },
    },
  }), '/admin/audit/');
  // Edited but not applied: the cursor still belongs to the unfiltered list.
  page.byId('audit-action').value = 'admin.grant.create';
  page.byId('audit-target').value = 'some-target';
  page.byId('audit-more').click();
  await settle();
  const moreCall = page.calls.find((c) => c.input.includes('cursor=a-2'));
  assert.equal(moreCall.input, `${AUDIT}?cursor=a-2`);
  assert.equal(page.byId('audit-error').textContent, '');
  assert.match(page.byId('audit-list').textContent, /action-2/);
});

test('audit paging: applying a filter re-seals the cursor to the new filter', async () => {
  const page = await loggedIn(async (input, init) => {
    const url = new URL(input, 'https://admin.test');
    if (url.pathname === AUDIT && url.searchParams.get('action') === 'admin.grant.create') {
      return jsonResponse(200, {
        items: [auditEvent(9)],
        next_cursor: url.searchParams.get('cursor') === null ? 'f-2' : '',
      });
    }
    return pagedServer({ pages: { [`${AUDIT}#`]: { items: [auditEvent(1)], next_cursor: 'a-2' } } })(input, init);
  }, '/admin/audit/');
  page.byId('audit-action').value = 'admin.grant.create';
  page.byId('audit-filter-form').dispatch('submit');
  await settle();
  page.byId('audit-action').value = 'something.else';
  page.byId('audit-more').click();
  await settle();
  const last = page.calls[page.calls.length - 1];
  assert.equal(last.input, `${AUDIT}?action=admin.grant.create&cursor=f-2`);
});

test('audit paging: a filter applied while "more" is in flight drops the stale page', async () => {
  const second = gate();
  const page = await loggedIn(async (input, init) => {
    const url = new URL(input, 'https://admin.test');
    if (url.pathname === AUDIT && url.searchParams.get('action') === 'admin.grant.create') {
      return jsonResponse(200, { items: [auditEvent(7)], next_cursor: '' });
    }
    return pagedServer({
      pages: {
        [`${AUDIT}#`]: { items: [auditEvent(1)], next_cursor: 'a-2' },
        [`${AUDIT}#a-2`]: { items: [auditEvent(2)], next_cursor: 'a-3' },
      },
      gates: { [`${AUDIT}#a-2`]: second.promise },
    })(input, init);
  }, '/admin/audit/');
  page.byId('audit-more').click();
  page.byId('audit-action').value = 'admin.grant.create';
  page.byId('audit-filter-form').dispatch('submit');
  await settle();
  second.release();
  await settle();
  const list = page.byId('audit-list').textContent;
  assert.match(list, /action-7/);
  assert.doesNotMatch(list, /action-2/, 'stale unfiltered page is not appended');
  assert.equal(page.byId('audit-more').hidden, true, 'cursor comes from the filtered page');
  assert.equal(page.byId('audit-more').disabled, false);
});
