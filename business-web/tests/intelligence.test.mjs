import test from 'node:test';
import assert from 'node:assert/strict';
import { compile } from './support.mjs';
const { load, cleanup } = compile();
process.on('exit', cleanup);
const { IntelligenceController } = await load('intelligence');

function authority(perms) {
  return {
    accountId: '00000000-0000-4000-8000-000000000010',
    organization: { id: '00000000-0000-4000-8000-000000000001', name: 'Org', membershipId: '00000000-0000-4000-8000-000000000020' },
    permissions: perms,
  };
}

test('F6 IntelligenceController loads permitted sections and records actions', async () => {
  const calls = [];
  const source = {
    async kpiOverview(_orgId, dim, pending) {
      calls.push(['kpi', dim, pending]);
      return { values: [{ indicatorId: 'IND-001', namePt: 'Gasto com combustível', status: 'COMPUTED', displayValue: '24000', unit: 'centavos', sampleSmall: false, insufficiencyReason: '' }] };
    },
    async listFindings() {
      return { findings: [{ id: 'f1', findingType: 'FT-001', subjectKey: 'fuel:1', severity: 'MEDIUM', state: 'OPEN', title: 'Discrepancia', nonProofNote: 'Nao constitui prova isolada.', revision: 1 }] };
    },
    async decideFinding(_org, payload) { calls.push(['decide', payload.action]); },
    async listAudit() { return { events: [{ id: 'a1', action: 'stock.tank.saved', targetType: 'tank', targetId: 't1', reason: '', occurredAt: '2026-11-18T10:00:00Z' }] }; },
    async listPlans() { return { plans: [{ id: 'p1', assetId: 'v1', name: 'Troca 10k', occurrenceState: 'DUE', decidingTrigger: 'DISTANCE', blocking: true, revision: 1 }], compliance: [{}] }; },
    async listWorkOrders() { return { workOrders: [{ id: 'w1', number: 1, assetId: 'v1', kind: 'CORRECTIVE', description: 'Freios', state: 'IN_EXECUTION', holdOpen: true, revision: 1 }] }; },
    async saveWorkOrder(_org, payload) { calls.push(['wo', payload.action]); },
    async listTanks() { return { tanks: [{ id: 't1', name: 'Tanque 1', product: 'DIESEL_S10', unit: 'LITRE', quantityMilli: 60000, valueCentavos: 36000, allowNegative: false }] }; },
    async saveTank(_org, payload) { calls.push(['tank', payload.name]); },
    async recordMovement() {},
    async listConsolidation() { return { grants: [] }; },
  };

  const ctrl = new IntelligenceController(source, () => {});
  ctrl.select(authority(['kpi.dashboard.read', 'find.finding.read', 'audit.event.read', 'maint.plan.manage', 'maint.workorder.manage', 'stock.tank.manage']));
  await new Promise(r => setTimeout(r, 10));
  assert.equal(ctrl.state.phase, 'ready');
  assert.equal(ctrl.state.kpis.length, 1);
  assert.equal(ctrl.state.findings[0].nonProofNote, 'Nao constitui prova isolada.');
  assert.equal(ctrl.state.workOrders[0].holdOpen, true);
  assert.equal(ctrl.state.tanks[0].valueCentavos, 36000);

  await ctrl.detectFindings('00000000-0000-4000-8000-000000000901');
  await ctrl.createTank('00000000-0000-4000-8000-000000000902', 'Tanque B', 'DIESEL_S10', 'LITRE', 1000000);
  await ctrl.createWorkOrder('00000000-0000-4000-8000-000000000903', 'v1', 'CORRECTIVE', 'Revisao');
  assert.deepEqual(calls.slice(1), [['decide', 'REPROCESS'], ['kpi', 'ORG', false], ['tank', 'Tanque B'], ['kpi', 'ORG', false], ['wo', 'CREATE'], ['kpi', 'ORG', false]]);
});
