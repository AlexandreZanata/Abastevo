import test from 'node:test';
import assert from 'node:assert/strict';
import { compile } from './support.mjs';
const { load, cleanup } = compile();
process.on('exit', cleanup);
const { OperationsController } = await load('operations');

function authority(perms) {
  return {
    accountId: '00000000-0000-4000-8000-000000000010',
    organization: { id: '00000000-0000-4000-8000-000000000001', name: 'Org', membershipId: '00000000-0000-4000-8000-000000000020' },
    permissions: perms,
  };
}

test('EMP-ROUTE-010: OperationsController swaps PanelMapPort adapter without touching map logic', async () => {
  const rendered = [];
  const fakeSource = {
    async listLive() {
      return {
        items: [{
          tripId: '00000000-0000-4000-8000-000000000100', sessionId: 's1', sessionState: 'SUSPENDED', tripState: 'PAUSED',
          statusNote: 'viagem pausada', unitId: '', unitName: 'Matriz', assetLabel: 'FROTA-10', driverName: 'Maria',
          freshness: 'STALE', latE7: -235505200, lonE7: -466333080, accuracyM: 12, coarse: true,
          collectedAt: '2026-11-18T10:00:00Z', receivedAt: '2026-11-18T10:01:00Z', lastCommunicationAt: '2026-11-18T10:01:00Z', revision: 4,
        }],
        alerts: [],
        maxRevision: 4,
      };
    },
    async readReplay(_orgId, tripId) {
      return {
        tripId, tripState: 'COMPLETED', rawExpired: false, trimmedCount: 0, quality: 'INSUFFICIENT', distanceM: null,
        points: [{ collectedAt: '2026-11-18T10:00:00Z', receivedAt: '2026-11-18T10:01:00Z', latE7: -235505200, lonE7: -466333080, accuracyM: 10, speedCmS: 0, coarse: false }],
        simplifiedLines: [], gaps: [], logbook: [{ kind: 'TRIP_STARTED', occurredAt: '2026-11-18T09:00:00Z', summary: 'Início' }],
      };
    },
    async listGeofences() { return { geofences: [], events: [], alerts: [] }; },
    async createGeofence() {},
    async readRoutePlan() { return { plan: null }; },
    async saveRoutePlan() { throw new Error('unused'); },
  };

  const spyAdapterA = { mode: 'CANVAS', render(_c, scene) { rendered.push(scene); }, clear() {} };
  const spyAdapterB = { mode: 'CANVAS', render(_c, scene) { rendered.push({ alt: true, scene }); }, clear() {} };
  const ctrl = new OperationsController(fakeSource, () => {}, undefined, spyAdapterA);
  ctrl.select(authority(['gps.location.read_live', 'gps.track.read']));
  await ctrl.refresh(false);
  assert.equal(ctrl.activeAdapter(), spyAdapterA);
  assert.equal(ctrl.scene().markers[0].statusNote, 'viagem pausada');
  assert.equal(ctrl.scene().markers[0].coarse, true);

  ctrl.setMapAdapter(spyAdapterB);
  assert.equal(ctrl.activeAdapter(), spyAdapterB);

  ctrl.toggleViewMode();
  assert.equal(ctrl.activeAdapter().mode, 'LIST');
  ctrl.toggleViewMode();
  assert.equal(ctrl.activeAdapter(), spyAdapterB);

  await ctrl.openReplay('00000000-0000-4000-8000-000000000100');
  assert.equal(ctrl.scene().trackParts.length, 0);
  assert.match(ctrl.state.message, /Dados insuficientes/);
});
