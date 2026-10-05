import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { build } from '../scripts/build.mjs';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const root = path.resolve(__dirname, '..');

test('essential content and links are present in static HTML without JS', () => {
  const tmpOut = path.join(root, '.test-dist-content-' + Date.now());
  try {
    build({ projectRoot: root, outDir: tmpOut });

    const indexHtml = fs.readFileSync(path.join(tmpOut, 'index.html'), 'utf8');

    // 1. Heading and primary identity
    assert.match(indexHtml, /<h1[^>]*>abastevo: preços de combustíveis com a comunidade<\/h1>/);
    assert.match(indexHtml, /Um projeto de código aberto para consultar preços de combustíveis em postos brasileiros/);

    // 2. Status line
    assert.match(indexHtml, /Em desenvolvimento · Android não publicado/);

    // 3. GitHub repository link
    assert.match(indexHtml, /href="https:\/\/github\.com\/AlexandreZanata\/abastevo"[^>]*>Ver no GitHub<\/a>/);

    // 4. Comparison card pair
    assert.match(indexHtml, /Comunidade e ANP, lado a lado/);
    assert.match(indexHtml, /Exemplo ilustrativo · dados fictícios/);
    assert.match(indexHtml, /Observação enviada por pessoa · há algumas horas/);
    assert.match(indexHtml, /Levantamento publicado pela ANP · semana da pesquisa/);

    // 5. Legal and help links reachable from footer
    assert.match(indexHtml, /href="\/privacidade\/"[^>]*>Privacidade<\/a>/);
    assert.match(indexHtml, /href="\/excluir-conta\/"[^>]*>Excluir conta<\/a>/);
    assert.match(indexHtml, /href="\/termos\/"[^>]*>Termos<\/a>/);

    // 6. Non-affiliation and licensing
    assert.match(indexHtml, /Não afiliado à ANP/);
    assert.match(indexHtml, /Código aberto sob licença MIT/);

    // 7. Verify legal pages also contain their essential titles and contact paths
    const privHtml = fs.readFileSync(path.join(tmpOut, 'privacidade', 'index.html'), 'utf8');
    assert.match(privHtml, /<h1[^>]*>Política de Privacidade<\/h1>/);
    assert.match(privHtml, /24 horas/);

    const delHtml = fs.readFileSync(path.join(tmpOut, 'excluir-conta', 'index.html'), 'utf8');
    assert.match(delHtml, /<h1[^>]*>Exclusão de Conta e Dados<\/h1>/);

    const termHtml = fs.readFileSync(path.join(tmpOut, 'termos', 'index.html'), 'utf8');
    assert.match(termHtml, /<h1[^>]*>Termos de Uso e Moderação<\/h1>/);
  } finally {
    fs.rmSync(tmpOut, { recursive: true, force: true });
  }
});
