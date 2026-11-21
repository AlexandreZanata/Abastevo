import test from 'node:test';
import assert from 'node:assert/strict';
import { compile, deferred, uuid } from './support.mjs';

const { load, cleanup } = compile();
process.on('exit', cleanup);
const { PlanOverviewApi, PlanPanelController, planStateLabel } = await load('subscriptions');

const observed = '2026-11-21T12:00:00Z';
const termEnd = '2026-12-21T12:00:00Z';
const rawPlan = {
  product: 'fleet', name: 'Plano sintético', revision: '2', state: 'ACTIVE',
  term_start: observed, term_end: termEnd, term_current: true, confirmed_paid: false,
  capabilities: ['organizations'], limits: { active_vehicles: '0', storage_bytes: '9223372036854775807' },
  storage_used_bytes: '1234',
  pending: { name: 'Próximo plano', revision: '3', effective_at: termEnd },
};

test('plan overview keeps exact limits and uses only recorded payment proof', async () => {
  let raw = rawPlan;
  const abort = new AbortController();
  const client = new PlanOverviewApi({
    async privatePost(path, body, signal) {
      assert.equal(path, `/v1/orgs/${uuid(1)}/subscriptions/plan/read`);
      assert.deepEqual(body, {});
      assert.equal(signal, abort.signal);
      return { observed_at: observed, plans: [raw] };
    },
  });
  const [plan] = await client.read(uuid(1), abort.signal);
  assert.equal(plan.limits.active_vehicles, '0');
  assert.equal(plan.limits.storage_bytes, '9223372036854775807');
  assert.equal(plan.storageUsedBytes, '1234');
  assert.equal(plan.confirmedPaid, false);
  assert.equal(plan.pending.name, 'Próximo plano');
  assert.equal(planStateLabel(plan), 'Ativo');
  assert.equal(planStateLabel({ ...plan, termCurrent: false }), 'Vigência encerrada');
  const receipt = { amount_centavos: '4567', currency: 'BRL', settled_at: observed, payment_date: '2026-11-21', fiscal_status: 'NOT_CONSULTED' };
  raw = { ...rawPlan, confirmed_paid: true, receipt };
  assert.deepEqual((await client.read(uuid(1), abort.signal))[0].receipt, {
    amountCentavos: '4567', settledAt: observed, fiscalStatus: 'NOT_CONSULTED', paymentDate: '2026-11-21',
  });
  raw = { ...rawPlan, confirmed_paid: true, receipt: { ...receipt, fiscal_status: 'AUTHORIZED_AS_OF' } };
  await assert.rejects(client.read(uuid(1), abort.signal), { kind: 'unavailable' });
  raw = { ...rawPlan, limits: { storage_bytes: 9223372036854775807 } };
  await assert.rejects(client.read(uuid(1), abort.signal), { kind: 'unavailable' });
  raw = { ...rawPlan, limits: [] };
  await assert.rejects(client.read(uuid(1), abort.signal), { kind: 'unavailable' });
  raw = { ...rawPlan, pending: { ...rawPlan.pending, effective_at: observed } };
  await assert.rejects(client.read(uuid(1), abort.signal), { kind: 'unavailable' });
});

test('changing company or its permission clears data and ignores an old response', async () => {
  const calls = [];
  const first = deferred();
  const next = deferred();
  const controller = new PlanPanelController({
    read(org, signal) { calls.push({ org, signal }); return calls.length === 1 ? first.promise : next.promise; },
  }, () => {}, () => {});
  const authority = (n, permissions) => ({
    accountId: uuid(90), organization: { id: uuid(n), membershipId: uuid(n + 50), name: 'Empresa sintética' }, permissions,
  });
  controller.select(authority(1, ['subs.plan.read']));
  controller.select(authority(2, ['subs.plan.read']));
  assert.deepEqual(calls.map(call => call.org), [uuid(1), uuid(2)]);
  assert.equal(calls[0].signal.aborted, true);
  first.resolve([{ name: 'Primeira empresa' }]);
  await first.promise;
  assert.equal(controller.state.phase, 'loading');
  assert.deepEqual(controller.state.plans, []);
  next.resolve([{ name: 'Segunda empresa' }]);
  await next.promise;
  assert.equal(controller.state.plans[0].name, 'Segunda empresa');
  controller.select(authority(2, []));
  assert.equal(controller.state.phase, 'closed');
  assert.deepEqual(controller.state.plans, []);
  assert.equal(calls.length, 2);
});

test('vehicle plan reads one bounded roster and commits the exact confirmed selection', async () => {
  const abort = new AbortController();
  const keys = [uuid(101), uuid(102), uuid(103)];
  const requests = [];
  const current = { ...rawPlan, limits: { ...rawPlan.limits, active_vehicles: '2' },
    vehicle_selection: { subscription_id: uuid(50), revision: 3, active: 3, ceiling: 2 } };
  const client = new PlanOverviewApi({
    async privatePost(path, body, signal) {
      assert.equal(signal, abort.signal);
      assert(path.startsWith(`/v1/orgs/${uuid(1)}/`));
      if (path.endsWith('/plan/read')) return { observed_at: observed, plans: [current] };
      if (path.endsWith('/fleet/assets/list')) {
        assert.deepEqual(body, { limit: 100, state: 'ACTIVE', kind: 'VEHICLE', ...(requests.length ? { cursor: 'page-two' } : {}) });
        const values = requests.length ? keys.slice(2) : keys.slice(0, 2);
        requests.push(path);
        return { items: values.map((id, i) => ({ id, kind: 'VEHICLE', state: 'ACTIVE', name: 'Veículo', fleet_number: `F-${i}` })),
          next_cursor: requests.length === 1 ? 'page-two' : '' };
      }
      assert(path.endsWith('/subscriptions/vehicles/select'));
      assert.deepEqual(body, { schema_version: 1, operation_id: uuid(99), subscription_id: uuid(50), revision: 3, keep_active: [keys[2], keys[0]] });
      return { kept: [keys[0], keys[2]], archived: [keys[1]], replayed: true };
    },
  });
  const [plan] = await client.read(uuid(1), abort.signal);
  const options = await client.vehicleOptions(uuid(1), plan.vehicleSelection, abort.signal);
  assert.deepEqual(options.map(choice => choice.id), keys);
  assert.equal(options[0].name, 'Veículo · F-0');
  await client.commitVehicles(uuid(1), plan.vehicleSelection, uuid(99), [keys[2], keys[0]], abort.signal);
});

test('changing company aborts an old vehicle selection load', async () => {
  const pending = deferred();
  const basis = { subscription: uuid(50), revision: 2, active: 2, ceiling: 1 };
  let vehicleSignal;
  const controller = new PlanPanelController({
    async read(org) { return org === uuid(1) ? [{ name: 'Empresa A', vehicleSelection: basis }] : [{ name: 'Empresa B' }]; },
    vehicleOptions(_org, _basis, signal) { vehicleSignal = signal; return pending.promise; },
    async commitVehicles() { assert.fail('an old choice cannot be committed'); },
  }, () => {}, () => {});
  const authority = n => ({
    accountId: uuid(90), organization: { id: uuid(n), membershipId: uuid(n + 50), name: 'Empresa sintética' },
    permissions: ['subs.plan.read', 'subs.plan.manage', 'fleet.asset.read'],
  });
  controller.select(authority(1));
  await Promise.resolve();
  const choosing = controller.chooseVehicles(basis);
  controller.select(authority(2));
  await Promise.resolve();
  assert.equal(vehicleSignal.aborted, true);
  pending.resolve([{ id: uuid(113), name: 'Veículo da empresa A' }]);
  await choosing;
  assert.equal(controller.state.plans[0].name, 'Empresa B');
  assert.equal(controller.state.vehicleEdit, undefined);
});
