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
  assert.equal(plan.confirmedPaid, false);
  assert.equal(plan.pending.name, 'Próximo plano');
  assert.equal(planStateLabel(plan), 'Ativo');
  assert.equal(planStateLabel({ ...plan, termCurrent: false }), 'Vigência encerrada');
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
