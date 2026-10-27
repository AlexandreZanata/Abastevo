import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');

// B-BR-WEB: data is rendered with textContent/createElement only. The key
// and tokens never touch storage, logs or the console.
const FORBIDDEN = [
  /innerHTML/,
  /outerHTML/,
  /insertAdjacentHTML/,
  /document\.write/,
  /[^a-zA-Z]eval\s*\(/,
  /localStorage/,
  /sessionStorage/,
  /console\./,
];

test('sources: no unsafe DOM, storage or console APIs', () => {
  const srcDir = path.join(root, 'src');
  const violations = [];
  for (const file of fs.readdirSync(srcDir).filter((f) => f.endsWith('.ts')).sort()) {
    const content = fs.readFileSync(path.join(srcDir, file), 'utf8');
    for (const pattern of FORBIDDEN) {
      if (pattern.test(content)) {
        violations.push(`${file}: ${pattern}`);
      }
    }
  }
  assert.deepEqual(violations, [], `unsafe APIs in src/: ${violations.join(', ')}`);
});
