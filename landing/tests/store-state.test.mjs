import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { build, validateStoreState } from '../scripts/build.mjs';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const root = path.resolve(__dirname, '..');

test('store state: prelaunch displays honest notice and no fake store links', () => {
  const tmpOut = path.join(root, '.test-dist-store-prelaunch-' + Date.now());
  try {
    build({ projectRoot: root, outDir: tmpOut, storeState: 'prelaunch' });
    const indexHtml = fs.readFileSync(path.join(tmpOut, 'index.html'), 'utf8');

    assert.match(indexHtml, /<span class="store-notice">Em breve na Google Play<\/span>/);
    assert.doesNotMatch(indexHtml, /play\.google\.com\/store/);
  } finally {
    fs.rmSync(tmpOut, { recursive: true, force: true });
  }
});

test('store state: published with valid URL renders verified Play Store link', () => {
  const tmpOut = path.join(root, '.test-dist-store-published-' + Date.now());
  const validUrl = 'https://play.google.com/store/apps/details?id=com.anpfuel';
  try {
    build({
      projectRoot: root,
      outDir: tmpOut,
      storeState: 'published',
      storeUrl: validUrl,
    });
    const indexHtml = fs.readFileSync(path.join(tmpOut, 'index.html'), 'utf8');

    assert.match(indexHtml, /<a href="https:\/\/play\.google\.com\/store\/apps\/details\?id=com\.anpfuel" class="store-link"/);
    assert.match(indexHtml, /Ver na Google Play<\/a>/);
    assert.doesNotMatch(indexHtml, /<span class="store-notice">Em breve na Google Play<\/span>/);
  } finally {
    fs.rmSync(tmpOut, { recursive: true, force: true });
  }
});

test('store state: invalid store URL or package rejected', () => {
  // 1. Missing storeUrl in published state
  assert.throws(
    () => validateStoreState('published', null),
    /storeUrl is required when storeState is "published"/
  );

  // 2. Insecure HTTP URL
  assert.throws(
    () => validateStoreState('published', 'http://play.google.com/store/apps/details?id=com.anpfuel'),
    /Invalid store URL for published state/
  );

  // 3. Wrong package name
  assert.throws(
    () => validateStoreState('published', 'https://play.google.com/store/apps/details?id=com.otherapp'),
    /Invalid store URL for published state: expected verified Google Play URL with package com.anpfuel/
  );

  // 4. Non-Play-Store domain
  assert.throws(
    () => validateStoreState('published', 'https://malicious.com/store/apps/details?id=com.anpfuel'),
    /Invalid store URL for published state/
  );

  // 5. Invalid store state string
  assert.throws(
    () => validateStoreState('unknown-state', null),
    /Invalid storeState: must be 'prelaunch' or 'published'/
  );
});
