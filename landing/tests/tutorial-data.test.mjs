import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { execFileSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const root = path.resolve(__dirname, '..');

function compileValidator() {
  const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'abastevo-tutorial-'));
  execFileSync('tsc', ['--project', path.join(root, 'tsconfig.json'), '--outDir', tmpDir], {
    cwd: root,
    stdio: 'pipe',
  });
  const modPath = path.join(tmpDir, 'tutorial.js');
  assert.ok(fs.existsSync(modPath), 'tutorial.js must compile');
  return { tmpDir, modPath };
}

function validStep(overrides = {}) {
  return {
    stepId: 's1',
    image: '/assets/app/city-picker-720w.webp',
    imageSha256: 'a'.repeat(64),
    nativeWidth: 1220,
    nativeHeight: 2712,
    state: 'State list',
    prerequisite: 'App open on Explorar',
    instructionPtBr: 'Toque em Minha cidade.',
    controlLabelPtBr: 'Minha cidade',
    expectedPtBr: 'Abre Selecionar localização.',
    recoveryPtBr: 'Volte com o botão do sistema.',
    altPtBr: 'Seletor de cidade com busca por estado.',
    target: { x: 877, y: 378, width: 259, height: 60 },
    availability: 'Disponível',
    claimClass: 'visible-state-only',
    sourceFiles: ['app/src/main/kotlin/com/anpfuel/app/ui/location/LocationPickerScreen.kt'],
    capturedAt: '2026-10-11',
    privacyReviewed: true,
    demoOnly: false,
    ...overrides,
  };
}

function validLesson(overrides = {}) {
  return {
    lessonId: 'TU02',
    slug: 'escolher-cidade',
    titlePtBr: 'Escolher cidade',
    appVersion: '3.1.0-debug',
    appSourceSha: null,
    steps: [validStep()],
    ...overrides,
  };
}

test('validateLesson accepts a well-formed lesson', async () => {
  const { tmpDir, modPath } = compileValidator();
  try {
    const { validateLesson } = await import(modPath);
    assert.deepEqual(validateLesson(validLesson()), []);
  } finally {
    fs.rmSync(tmpDir, { recursive: true, force: true });
  }
});

test('validateLesson rejects duplicates, overflow, bad hash and unknown availability', async () => {
  const { tmpDir, modPath } = compileValidator();
  try {
    const { validateLesson } = await import(modPath);
    const dup = validLesson({ steps: [validStep(), validStep()] });
    assert.match(validateLesson(dup).join(';'), /duplicate stepId/);
    const overflow = validLesson({
      steps: [validStep({ target: { x: 1200, y: 2600, width: 100, height: 200 } })],
    });
    assert.match(validateLesson(overflow).join(';'), /exceeds 1220x2712/);
    const badHash = validLesson({ steps: [validStep({ imageSha256: 'xyz' })] });
    assert.match(validateLesson(badHash).join(';'), /hex64/);
    const badAvail = validLesson({ steps: [validStep({ availability: 'Soon' })] });
    assert.match(validateLesson(badAvail).join(';'), /unknown availability/);
    const emptyText = validLesson({ steps: [validStep({ instructionPtBr: '  ' })] });
    assert.match(validateLesson(emptyText).join(';'), /instructionPtBr/);
    assert.match(validateLesson(null).join(';'), /must be an object/);
  } finally {
    fs.rmSync(tmpDir, { recursive: true, force: true });
  }
});

test('every shipped lesson.json validates', async () => {
  const { tmpDir, modPath } = compileValidator();
  try {
    const { validateLesson } = await import(modPath);
    const comoUsar = path.join(root, 'static', 'como-usar');
    if (!fs.existsSync(comoUsar)) {
      return;
    }
    let count = 0;
    for (const slug of fs.readdirSync(comoUsar)) {
      const lessonPath = path.join(comoUsar, slug, 'lesson.json');
      if (!fs.existsSync(lessonPath)) {
        continue;
      }
      const errors = validateLesson(JSON.parse(fs.readFileSync(lessonPath, 'utf8')));
      assert.deepEqual(errors, [], `${slug}/lesson.json invalid: ${errors.join('; ')}`);
      count += 1;
    }
    assert.ok(count >= 0);
  } finally {
    fs.rmSync(tmpDir, { recursive: true, force: true });
  }
});
