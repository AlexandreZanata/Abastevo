import test from 'node:test';
import assert from 'node:assert/strict';
import { compile } from './support.mjs';
const { load, cleanup } = compile();
process.on('exit', cleanup);
const { AgreementsController } = await load('agreements');

function authority(perms) {
  return {
    accountId: '00000000-0000-4000-8000-000000000010',
    organization: { id: '00000000-0000-4000-8000-000000000001', name: 'Org', membershipId: '00000000-0000-4000-8000-000000000020' },
    permissions: perms,
  };
}

test('F7 AgreementsController loads permitted sections and keeps the one-time code', async () => {
  const calls = [];
  const source = {
    async listAgreements() {
      return { agreements: [{ id: 'a1', stationId: 's1', orgLabel: 'Transportes Vale', state: 'ACTIVE', derivedState: 'ACTIVE', revision: 2, validFrom: '2026-12-01T00:00:00Z', validUntil: '2026-12-31T23:59:59Z', conditions: 'Faturamento mensal', terms: [{ product: 'DIESEL_S10', unit: 'L', priceMilliBrl: 5999, conditions: '' }], usable: true }] };
    },
    async readAgreement() { return { agreement: null }; },
    async proposeAgreement(_org, payload) { calls.push(['propose', payload.org_label]); },
    async applyAgreement(_org, id, action, payload) { calls.push(['apply', id, action, payload.revision]); },
    async listAuthorizations() {
      return { authorizations: [{ id: 'v1', agreementId: 'a1', stationId: 's1', employeeId: 'e1', assetId: '', product: 'DIESEL_S10', unit: 'L', maxQuantityMilli: 200000, maxCentavos: 300000, validUntil: '2026-12-10T23:59:59Z', state: 'ISSUED', derivedState: 'ISSUED', failedAttempts: 0 }] };
    },
    async issueAuthorization(_org, payload) {
      calls.push(['issue', payload.max_quantity_milli]);
      return { authorization: null, code: '9f8e7d6c5b4a39281706050403020100', replayed: false };
    },
    async cancelAuthorization(_org, id, payload) { calls.push(['cancel', id, payload.reason]); },
    async listRedemptions() {
      return { redemptions: [{ id: 'r1', agreementId: 'a1', authorizationId: 'v1', stationId: 's1', employeeId: 'e1', product: 'DIESEL_S10', unit: 'L', quantityMilli: 50000, priceMilliBrl: 5999, totalCentavos: 299950, confirmedAt: '2026-12-02T10:00:00Z' }] };
    },
  };

  const ctrl = new AgreementsController(source, () => {});
  ctrl.select(authority(['agree.agreement.manage', 'agree.authorization.issue']));
  await new Promise(r => setTimeout(r, 10));
  assert.equal(ctrl.state.phase, 'ready');
  assert.equal(ctrl.state.agreements[0].orgLabel, 'Transportes Vale');
  assert.equal(ctrl.state.agreements[0].terms[0].priceMilliBrl, 5999);
  assert.equal(ctrl.state.authorizations[0].maxCentavos, 300000);
  assert.equal(ctrl.state.redemptions[0].totalCentavos, 299950);

  await ctrl.propose('00000000-0000-4000-8000-000000000001', 's1', 'Transportes Vale', '2026-12-01T00:00:00Z', '2026-12-31T23:59:59Z', [{ product: 'DIESEL_S10', unit: 'L', priceMilliBrl: 5999 }]);
  await ctrl.apply('00000000-0000-4000-8000-000000000001', 'a1', 'SUSPEND', 2, 'Revisão contratual');
  await ctrl.issue('00000000-0000-4000-8000-000000000001', 'a1', 'e1', 'DIESEL_S10', 'L', 200000, 300000, '2026-12-10T23:59:59Z');
  await ctrl.cancel('00000000-0000-4000-8000-000000000001', 'v1', 'Motorista realocado');
  assert.deepEqual(calls, [
    ['propose', 'Transportes Vale'],
    ['apply', 'a1', 'SUSPEND', 2],
    ['issue', 200000],
    ['cancel', 'v1', 'Motorista realocado'],
  ]);
  assert.equal(ctrl.state.lastCode, '9f8e7d6c5b4a39281706050403020100');
});

test('F7 AgreementsController hides sections without the F7 permissions', async () => {
  const calls = [];
  const source = {
    async listAgreements() { calls.push(['agreements']); return { agreements: [] }; },
    async readAgreement() { return { agreement: null }; },
    async proposeAgreement() {},
    async applyAgreement() {},
    async listAuthorizations() { calls.push(['authorizations']); return { authorizations: [] }; },
    async issueAuthorization() { return { authorization: null, code: '', replayed: false }; },
    async cancelAuthorization() {},
    async listRedemptions() { calls.push(['redemptions']); return { redemptions: [] }; },
  };
  const ctrl = new AgreementsController(source, () => {});
  ctrl.select(authority(['fleet.asset.read']));
  await new Promise(r => setTimeout(r, 10));
  assert.equal(ctrl.state.phase, 'ready');
  assert.deepEqual(calls, []);
});
