import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { execFileSync } from 'node:child_process';
import { fileURLToPath, pathToFileURL } from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');

async function loadModules() {
  const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'adm-web-forum-'));
  try {
    const localTsc = path.join(root, 'node_modules', '.bin', 'tsc');
    execFileSync(fs.existsSync(localTsc) ? localTsc : 'tsc', [
      '--project', path.join(root, 'tsconfig.json'), '--outDir', tmpDir,
    ], { cwd: root, stdio: 'pipe' });
    const validators = await import(pathToFileURL(path.join(tmpDir, 'validators.js')).href);
    const api = await import(pathToFileURL(path.join(tmpDir, 'api.js')).href);
    return { validators, api };
  } finally {
    fs.rmSync(tmpDir, { recursive: true, force: true });
  }
}

test('forum validators: title bounds and code points', async () => {
  const { validators: v } = await loadModules();
  assert.equal(v.FORUM_TITLE_MAX, 120);
  assert.equal(v.FORUM_EXCERPT_MAX, 400);
  assert.equal(v.FORUM_BODY_MAX, 4000);

  // Title
  assert.equal(v.forumTitleError(''), 'Título é obrigatório.');
  assert.equal(v.forumTitleError('   '), 'Título é obrigatório.');
  assert.equal(v.forumTitleError('A'), null);
  assert.equal(v.forumTitleError('x'.repeat(120)), null);
  assert.equal(v.forumTitleError('x'.repeat(121)), 'Título deve ter no máximo 120 caracteres.');
  // Multibyte unicode / emoji: 119 chars + 1 emoji = 120
  assert.equal(v.forumTitleError('x'.repeat(119) + '⛽'), null);
  assert.equal(v.forumTitleError('x'.repeat(120) + '⛽'), 'Título deve ter no máximo 120 caracteres.');

  // Excerpt
  assert.equal(v.forumExcerptError(''), null);
  assert.equal(v.forumExcerptError('x'.repeat(400)), null);
  assert.equal(v.forumExcerptError('x'.repeat(401)), 'Resumo deve ter no máximo 400 caracteres.');

  // Body
  assert.equal(v.forumBodyError(''), null);
  assert.equal(v.forumBodyError('x'.repeat(4000)), null);
  assert.equal(v.forumBodyError('x'.repeat(4001)), 'Corpo do texto deve ter no máximo 4000 caracteres.');

  // Categories
  assert.equal(v.isForumCategory('FUELS'), true);
  assert.equal(v.isForumCategory('ELECTRIC'), true);
  assert.equal(v.isForumCategory('TRAFFIC_RULES'), true);
  assert.equal(v.isForumCategory('VEHICLES_TECH'), true);
  assert.equal(v.isForumCategory('ROADS_MOBILITY'), true);
  assert.equal(v.isForumCategory('DRIVER_ECONOMY'), true);
  assert.equal(v.isForumCategory('ABASTEVO_DATA'), true);
  assert.equal(v.isForumCategory('UNKNOWN'), false);
  assert.equal(v.isForumCategory('fuels'), false);

  // Kinds
  assert.equal(v.isForumKind('EDITORIAL'), true);
  assert.equal(v.isForumKind('NEWS'), true);
  assert.equal(v.isForumKind('TOPIC'), true);
  assert.equal(v.isForumKind('BULLETIN'), true);
  assert.equal(v.isForumKind('OTHER'), false);
});

test('forum api client: list, detail, create, edit, publish, visibility', async () => {
  const { api } = await loadModules();

  const recordedCalls = [];
  const fakeFetch = async (input, init) => {
    recordedCalls.push({ url: input, method: init.method, headers: init.headers, body: init.body });
    if (input === '/v1/admin/session') {
      return new Response(JSON.stringify({ csrf_token: 'test-csrf-token', expires_at: '2026-11-06T00:00:00Z' }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      });
    }
    if (input.startsWith('/v1/admin/forum/posts')) {
      if (init.method === 'GET' && !input.includes('00000000-0000-0000-0000-000000000002')) {
        return new Response(JSON.stringify({
          items: [
            {
              id: '00000000-0000-0000-0000-000000000002',
              kind: 'EDITORIAL',
              category: 'FUELS',
              title: 'Título de teste',
              excerpt: 'Resumo de teste',
              publication_state: 'DRAFT',
              visibility: 'VISIBLE',
              revision: 1,
              created_at: '2026-11-05T12:00:00Z',
              updated_at: '2026-11-05T12:00:00Z',
              published_at: null,
              source_id: '00000000-0000-0000-0000-000000000001',
            },
          ],
          next_cursor: '',
        }), { status: 200, headers: { 'Content-Type': 'application/json' } });
      }
      return new Response(JSON.stringify({
        id: '00000000-0000-0000-0000-000000000002',
        kind: 'EDITORIAL',
        category: 'FUELS',
        title: 'Título de teste',
        excerpt: 'Resumo de teste',
        body_text: 'Corpo de teste completo',
        publication_state: 'PUBLISHED',
        visibility: 'VISIBLE',
        revision: 2,
        source_id: '00000000-0000-0000-0000-000000000001',
        source_name: 'Abastevo Editorial',
        created_at: '2026-11-05T12:00:00Z',
        updated_at: '2026-11-05T12:30:00Z',
        published_at: '2026-11-05T12:30:00Z',
      }), { status: 200, headers: { 'Content-Type': 'application/json' } });
    }
    return new Response(JSON.stringify({ error: { code: 'admin.not-found', message: 'not found' } }), {
      status: 404,
      headers: { 'Content-Type': 'application/json' },
    });
  };

  const client = new api.AdminApi(fakeFetch);
  await client.login('test-key');
  assert.equal(client.hasSession(), true);

  // 1. List
  const list = await client.listForumPosts('draft', 'FUELS', 'c1', 10);
  assert.equal(list.items.length, 1);
  const lastCall = recordedCalls[recordedCalls.length - 1];
  assert.equal(lastCall.method, 'GET');
  assert.ok(lastCall.url.includes('status=draft'));
  assert.ok(lastCall.url.includes('category=FUELS'));
  assert.ok(lastCall.url.includes('cursor=c1'));
  assert.ok(lastCall.url.includes('limit=10'));

  // 2. Detail
  const detail = await client.getForumPost('00000000-0000-0000-0000-000000000002');
  assert.equal(detail.body_text, 'Corpo de teste completo');

  // 3. Create
  await client.createForumPost({
    category: 'FUELS',
    title: 'Novo Post',
    excerpt: 'Resumo',
    bodyText: 'Texto',
    reason: 'Criando post editorial para teste',
  });
  const createCall = recordedCalls[recordedCalls.length - 1];
  assert.equal(createCall.method, 'POST');
  assert.equal(createCall.url, '/v1/admin/forum/posts');
  assert.equal(createCall.headers['X-Admin-CSRF'], 'test-csrf-token');
  assert.ok(createCall.headers['Idempotency-Key'] !== undefined);
  const createPayload = JSON.parse(createCall.body);
  assert.equal(createPayload.title, 'Novo Post');
  assert.equal(createPayload.source_id, '00000000-0000-0000-0000-000000000001');

  // 4. Edit
  await client.editForumPost('00000000-0000-0000-0000-000000000002', {
    category: 'ELECTRIC',
    title: 'Post Editado',
    excerpt: 'Resumo Editado',
    bodyText: 'Texto Editado',
    expectedRevision: 1,
    reason: 'Atualizando post editorial com correções',
  });
  const editCall = recordedCalls[recordedCalls.length - 1];
  assert.equal(editCall.method, 'POST');
  assert.equal(editCall.url, '/v1/admin/forum/posts/00000000-0000-0000-0000-000000000002/edit');
  const editPayload = JSON.parse(editCall.body);
  assert.equal(editPayload.expected_revision, 1);
  assert.equal(editPayload.category, 'ELECTRIC');

  // 5. Publish
  await client.publishForumPost('00000000-0000-0000-0000-000000000002', 2, 'Publicando post aprovado');
  const pubCall = recordedCalls[recordedCalls.length - 1];
  assert.equal(pubCall.method, 'POST');
  assert.equal(pubCall.url, '/v1/admin/forum/posts/00000000-0000-0000-0000-000000000002/publish');
  const pubPayload = JSON.parse(pubCall.body);
  assert.equal(pubPayload.expected_revision, 2);

  // 6. Visibility (hide / unhide)
  await client.setForumPostVisibility('00000000-0000-0000-0000-000000000002', true, 3, 'Ocultando post por moderacao');
  const hideCall = recordedCalls[recordedCalls.length - 1];
  assert.equal(hideCall.url, '/v1/admin/forum/posts/00000000-0000-0000-0000-000000000002/hide');

  await client.setForumPostVisibility('00000000-0000-0000-0000-000000000002', false, 4, 'Reexibindo post moderado');
  const unhideCall = recordedCalls[recordedCalls.length - 1];
  assert.equal(unhideCall.url, '/v1/admin/forum/posts/00000000-0000-0000-0000-000000000002/unhide');
});

test('forum error mapping', async () => {
  const { api } = await loadModules();
  assert.equal(
    api.errorMessage(new api.ApiError(404, 'forum.not-found', 'not found')),
    'Publicação do fórum não encontrada.',
  );
  assert.equal(
    api.errorMessage(new api.ApiError(409, 'forum.revision-conflict', 'conflict')),
    'A publicação foi alterada por outro usuário. Recarregue os dados antes de salvar.',
  );
  assert.equal(
    api.errorMessage(new api.ApiError(400, 'forum.missing-rights', 'missing rights')),
    'A fonte selecionada não possui direitos de publicação aprovados.',
  );
  assert.equal(
    api.errorMessage(new api.ApiError(400, 'forum.invalid', 'invalid')),
    'Dados da publicação inválidos. Revise os campos.',
  );
  assert.equal(
    api.errorMessage(new api.ApiError(404, 'forum.disabled', 'disabled')),
    'O módulo de fórum está desativado nesta instância.',
  );
  assert.equal(
    api.errorMessage(new api.ApiError(503, 'forum.unavailable', 'unavailable')),
    'Serviço de fórum indisponível. Tente mais tarde.',
  );
});
