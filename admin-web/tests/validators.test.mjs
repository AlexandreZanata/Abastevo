import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { execFileSync } from 'node:child_process';
import { fileURLToPath, pathToFileURL } from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');

async function loadValidators() {
  const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'adm-web-validators-'));
  try {
    const localTsc = path.join(root, 'node_modules', '.bin', 'tsc');
    execFileSync(fs.existsSync(localTsc) ? localTsc : 'tsc', [
      '--project', path.join(root, 'tsconfig.json'), '--outDir', tmpDir,
    ], { cwd: root, stdio: 'pipe' });
    return await import(pathToFileURL(path.join(tmpDir, 'validators.js')).href);
  } finally {
    fs.rmSync(tmpDir, { recursive: true, force: true });
  }
}

test('reason: 10..500 code points after trim', async () => {
  const v = await loadValidators();
  assert.equal(v.reasonError(''), 'Motivo deve ter de 10 a 500 caracteres.');
  assert.ok(v.reasonError('123456789') !== null);
  assert.equal(v.reasonError('1234567890'), null);
  assert.equal(v.reasonError('x'.repeat(500)), null);
  assert.ok(v.reasonError('x'.repeat(501)) !== null);
  assert.equal(v.reasonError(`123456789${'😀'}`), null, 'emoji counts as one code point');
  assert.ok(v.reasonError(`12345678${'😀'}`) !== null);
});

test('tier, search and uuid', async () => {
  const v = await loadValidators();
  assert.equal(v.isTier('PRO'), true);
  assert.equal(v.isTier('CONTRIBUTOR'), true);
  assert.equal(v.isTier('ENTERPRISE'), true);
  assert.equal(v.isTier('pro'), false);
  assert.equal(v.isTier(''), false);
  assert.ok(v.searchError('ab') !== null);
  assert.equal(v.searchError('abc'), null);
  assert.equal(v.isUuid('11111111-1111-4111-8111-111111111111'), true);
  assert.equal(v.isUuid('not-a-uuid'), false);
});

test('end date: optional, calendar-valid, bounded', async () => {
  const v = await loadValidators();
  const now = new Date('2026-10-27T12:00:00Z');
  assert.equal(v.endDateError('', now), null);
  assert.equal(v.endDateError('2026-11-26', now), null);
  assert.ok(v.endDateError('not-a-date', now) !== null);
  assert.ok(v.endDateError('2026-13-01', now) !== null);
  assert.ok(v.endDateError('2026-02-30', now) !== null);
  assert.ok(v.endDateError('2026-10-26', now) !== null, 'past date rejected');
  assert.ok(v.endDateError('2031-10-28', now) !== null, 'beyond 5 years rejected');
  assert.equal(v.defaultEndDate(now), '2026-11-26');
  assert.equal(v.toEndOfDayIso('2026-11-26'), '2026-11-26T23:59:59-03:00');
});

// ENT-04: now + 1h <= end_at <= now + 5 years, enforced by the server as
// 5 x 365 days. The picked calendar day ends at 23:59:59 Brazil time
// (fixed UTC-03:00; no DST since 2019).
const HOUR = 60 * 60 * 1000;
const DAY = 24 * HOUR;

test('end date ENT-04: the picked day ends at 23:59:59 in Brazil (UTC-03:00)', async () => {
  const v = await loadValidators();
  const iso = v.toEndOfDayIso('2026-11-26');
  assert.equal(Date.parse(iso), Date.parse('2026-11-27T02:59:59Z'));
  // 22:00 in Brazil is already the next UTC day; Brazil's today is still valid.
  const lateBrazil = new Date('2026-10-27T22:00:00-03:00');
  assert.equal(v.endDateError('2026-10-27', lateBrazil), null, 'today in Brazil with ~2h left');
  assert.equal(v.defaultEndDate(lateBrazil), '2026-11-26', 'default counts from the Brazilian day');
});

test('end date ENT-04: today late evening with less than 1h left is rejected', async () => {
  const v = await loadValidators();
  assert.ok(v.endDateError('2026-10-27', new Date('2026-10-27T23:30:00-03:00')) !== null);
  assert.ok(v.endDateError('2026-10-27', new Date('2026-10-27T23:00:00-03:00')) !== null, '59m59s left');
  assert.equal(v.endDateError('2026-10-27', new Date('2026-10-27T22:59:59-03:00')), null, 'exactly 1h left');
  assert.ok(v.endDateError('2026-10-26', new Date('2026-10-27T09:00:00-03:00')) !== null, 'yesterday');
});

test('end date ENT-04: tomorrow is accepted at any time of day', async () => {
  const v = await loadValidators();
  for (const now of ['2026-10-27T00:00:00-03:00', '2026-10-27T12:00:00-03:00', '2026-10-27T23:59:59-03:00']) {
    assert.equal(v.endDateError('2026-10-28', new Date(now)), null, now);
  }
});

test('end date ENT-04: instant bounds are now+1h and now+1825 days inclusive', async () => {
  const v = await loadValidators();
  const now = new Date('2026-10-27T12:00:00Z');
  const at = (ms) => new Date(now.getTime() + ms);
  assert.ok(v.endInstantError(at(HOUR - 1000), now) !== null, '1h minus 1s');
  assert.equal(v.endInstantError(at(HOUR), now), null, 'exactly 1h');
  assert.equal(v.endInstantError(at(1825 * DAY), now), null, '+1825 days');
  assert.ok(v.endInstantError(at(1825 * DAY + 1000), now) !== null, '+1825 days and 1s');
  assert.ok(v.endInstantError(at(1826 * DAY), now) !== null, '+1826 days');
});

test('end date ENT-04: last selectable day is the last whose end fits in 1825 days', async () => {
  const v = await loadValidators();
  const now = new Date('2026-10-27T12:00:00Z'); // 09:00 in Brazil; limit 2031-10-26T12:00Z
  assert.equal(v.endDateError('2031-10-25', now), null, 'ends 2031-10-26T02:59:59Z');
  assert.ok(v.endDateError('2031-10-26', now) !== null, 'ends 2031-10-27T02:59:59Z, past the limit');
  assert.ok(v.endDateError('2031-10-27', now) !== null, 'same calendar date 5 years later');
});

test('end date ENT-04: leap-year ranges use 5 x 365 days, not 5 calendar years', async () => {
  const v = await loadValidators();
  const now = new Date('2027-03-01T12:00:00Z'); // range crosses 2028-02-29; limit 2032-02-28T12:00Z
  assert.equal(v.endDateError('2028-02-29', now), null, 'leap day is a real date');
  assert.ok(v.endDateError('2027-02-29', now) !== null, 'not a leap year');
  assert.equal(v.endDateError('2032-02-27', now), null);
  assert.ok(v.endDateError('2032-02-28', now) !== null);
  assert.ok(v.endDateError('2032-02-29', now) !== null);
  assert.ok(v.endDateError('2032-03-01', now) !== null, 'calendar +5 years exceeds 1825 days');
});

test('github login and roles mirror the contract rules', async () => {
  const v = await loadValidators();
  for (const login of ['a', 'octocat', 'user-name', 'A1-b2']) {
    assert.equal(v.githubLoginError(login), null, login);
  }
  for (const login of ['', 'x'.repeat(40), '-abc', 'abc-', 'a--b', 'a_b', 'a b']) {
    assert.ok(v.githubLoginError(login) !== null, login || '(empty)');
  }
  assert.equal(v.isRole('support_agent'), true);
  assert.equal(v.isRole('administrator'), false);
  assert.equal(v.isRole(''), false);
  assert.equal(v.isClaimState(''), true);
  assert.equal(v.isClaimState('PENDING'), true);
  assert.equal(v.isClaimState('BOGUS'), false);
});
