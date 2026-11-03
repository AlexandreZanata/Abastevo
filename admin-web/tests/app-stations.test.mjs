import test, { after, before } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { compileApp, jsonResponse, settle } from './support/fake-dom.mjs';

const root = fileURLToPath(new URL('..', import.meta.url));
const station = 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa';
const other = 'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb';
const path = '/admin/stations/';
let app;
before(() => { app = compileApp(root); });
after(() => app.cleanup());

async function openStations(handler) {
  const page = await app.mount({
    fetchHandler(input, init) {
      if (input === '/v1/admin/session' && init.method === 'POST') {
        return jsonResponse(200, { csrf_token: 'fixture-session', expires_at: '2026-11-03T20:00:00Z' });
      }
      if (input === '/v1/admin/session') {
        return jsonResponse(200, { key_fingerprint: 'fixture', expires_at: '2026-11-03T20:00:00Z' });
      }
      return handler(input, init);
    },
  });
  page.window.popTo(path);
  assert.equal(page.window.location.pathname, '/admin/login/');
  page.byId('admin-key').value = 'fixture-login';
  page.byId('login-form').dispatch('submit');
  await settle();
  page.window.popTo(path);
  assert.equal(page.window.location.pathname, path);
  assert.equal(page.byId('view-stations').hidden, false);
  return page;
}

function lookup(page, id) {
  page.byId('station-id').value = id;
  page.byId('station-lookup').dispatch('submit');
}

test('station lookup renders distinct public facts as text and validates UUIDs before fetch', async () => {
  const page = await openStations((input, init) => {
    assert.equal(input, `/v1/stations/${station}`);
    assert.equal(init.method, 'GET');
    return jsonResponse(200, {
      station_id: station, display_name: '<b>Posto Exemplo</b>',
      legal_name: 'Comércio Exemplo LTDA', address_source: 'osm',
      coordinates: { lat: -25.42, lon: -49.27 },
    });
  });
  lookup(page, '../../accounts');
  await settle();
  assert.match(page.byId('station-error').textContent, /UUID/);
  assert.equal(page.calls.length, 2);
  lookup(page, station.toUpperCase());
  await settle();
  assert.equal(page.byId('station-display-name').textContent, '<b>Posto Exemplo</b>');
  assert.equal(page.byId('station-legal-name').textContent, 'Comércio Exemplo LTDA');
  assert.equal(page.byId('station-address-source').textContent, 'OpenStreetMap (osm)');
  assert.doesNotMatch(page.byId('station-result').textContent, /25\.42|49\.27/);
});

test('late station lookup cannot replace a newer response or survive leaving the view', async () => {
  let finish;
  const delayed = new Promise((resolve) => { finish = resolve; });
  const page = await openStations((input) => input.endsWith(station) ? delayed : jsonResponse(200, {
    station_id: other, display_name: 'Posto Novo', legal_name: null, address_source: null,
  }));
  lookup(page, station);
  await settle();
  lookup(page, other);
  await settle();
  assert.equal(page.byId('station-display-name').textContent, 'Posto Novo');
  assert.equal(page.byId('station-legal-name').textContent, 'Não informada');
  assert.equal(page.byId('station-address-source').textContent, 'Não informada');
  finish(jsonResponse(200, { station_id: station, display_name: 'Posto Antigo', address_source: 'anp' }));
  await settle();
  assert.equal(page.byId('station-display-name').textContent, 'Posto Novo');
  page.window.popTo('/admin/accounts/');
  page.window.popTo(path);
  assert.equal(page.byId('station-result').hidden, true);
});
