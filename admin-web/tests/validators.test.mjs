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
  assert.equal(v.toEndOfDayIso('2026-11-26'), '2026-11-26T00:00:00Z');
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
