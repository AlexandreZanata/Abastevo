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
    assert.match(indexHtml, /<h1[^>]*>Preços de combustíveis<br>mais claros <span>para o Brasil\.<\/span><\/h1>/);
    assert.match(indexHtml, /O Abastevo combina dados oficiais da ANP com informações da comunidade/);

    // 2. Status line
    assert.match(indexHtml, /Em desenvolvimento · Android não publicado/);

    // LH04 synchronizes truthful copy and branded two-line actions from 8099.
    assert.match(indexHtml, /href="https:\/\/github\.com\/AlexandreZanata\/abastevo"[^>]*target="_blank"[^>]*rel="noopener"/);
    assert.match(indexHtml, /Projeto open source/);
    assert.match(indexHtml, /Em breve no Google Play/);
    assert.doesNotMatch(indexHtml, /melhor preço|pagando menos|tempo real|milhares de usuários/i);

    // LH01 user-approved replacement: original home, source/date context without JS.
    const heroSection = indexHtml.match(/<section class="hero">[\s\S]*?<\/section>/)[0];
    assert.match(heroSection, /assets\/app\/explore-light-720w\.webp/);
    assert.doesNotMatch(heroSection, /Captura real do app em desenvolvimento/);
    assert.match(heroSection, /semana de 4 a 10 de outubro de 2026/);
    assert.doesNotMatch(heroSection, /Posto Exemplo|R\$ X,XX|comparison-container/);

    // 5. Legal and help links reachable from footer
    assert.match(indexHtml, /href="\/privacidade\/"[^>]*>Privacidade<\/a>/);
    assert.match(indexHtml, /href="\/excluir-conta\/"[^>]*>Excluir conta<\/a>/);
    assert.match(indexHtml, /href="\/termos\/"[^>]*>Termos<\/a>/);

    // 6. Non-affiliation and licensing
    assert.match(indexHtml, /Não afiliado à ANP/);
    assert.match(indexHtml, /sem afiliação à ANP/);
    assert.match(indexHtml, /Código aberto sob licença MIT/);
    assert.match(indexHtml, /gov\.br\/anp/);
    assert.match(indexHtml, /href="\/guias\/"[^>]*>Guias<\/a>/);

    // 6b. No fabricated trust signals
    assert.doesNotMatch(indexHtml, /depoimento|5 estrelas|milhares de usuários|mais barato garantido/i);

    // 7. Verify legal pages also contain their essential titles and contact paths
    const privHtml = fs.readFileSync(path.join(tmpOut, 'privacidade', 'index.html'), 'utf8');
    assert.match(privHtml, /<h1[^>]*>Política de Privacidade<\/h1>/);
    assert.match(privHtml, /24 horas/);

    const delHtml = fs.readFileSync(path.join(tmpOut, 'excluir-conta', 'index.html'), 'utf8');
    assert.match(delHtml, /<h1[^>]*>Exclusão de Conta e Dados<\/h1>/);

    // 7b. Account-deletion copy follows the username/key model (no registered email).
    assert.match(delHtml, /nome de usuário/i);
    assert.doesNotMatch(delHtml, /e-mail cadastrado/);

    const termHtml = fs.readFileSync(path.join(tmpOut, 'termos', 'index.html'), 'utf8');
    assert.match(termHtml, /<h1[^>]*>Termos de Uso e Moderação<\/h1>/);
  } finally {
    fs.rmSync(tmpOut, { recursive: true, force: true });
  }
});
