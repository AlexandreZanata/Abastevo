import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { build } from '../scripts/build.mjs';

const out = fs.mkdtempSync(path.join(os.tmpdir(), 'abastevo-policy-'));
build({outDir:out, origin:'https://abastevo.com.br'});
const read = route => fs.readFileSync(path.join(out,route,'index.html'),'utf8');
const main = html => html.match(/<main\b[\s\S]*?<\/main>/)?.[0] ?? '';
test.after(()=>fs.rmSync(out,{recursive:true,force:true}));

test('privacy remains public static HTML with controller identity, actual rights contact and working section links',()=>{
 const html=read('privacidade'),body=main(html);
 assert.match(body,/ZAFLAS INOVA SIMPLES/);
 assert.match(body,/67\.852\.667\/0001-93/);
 assert.match(body,/9631-4240/);
 assert.match(body,/href="https:\/\/wa\.me\/556696314240(?:\?[^"<>]*)?"/);
 assert.match(body,/href="tel:\+556696314240"/);
 assert.match(body,/data-policy-version="2026-11-05-v1"/);
 for(const id of ['controlador','dados','localizacao','fotos','bases-legais','compartilhamento','transferencias','retencao','direitos','exclusao','seguranca','criancas','alteracoes']) assert.ok(body.includes(`id="${id}"`),id);
 for(const [,anchor] of body.matchAll(/href="#([^"]+)"/g)) assert.ok(body.includes(`id="${anchor}"`),anchor);
 assert.doesNotMatch(body,/<form\b|<iframe\b|<script\b|type="password"/);
 assert.match(html,/rel="canonical" href="https:\/\/abastevo\.com\.br\/privacidade\/"/);
});

test('account deletion has a private outside-app route and never asks for a public issue or credentials',()=>{
 const body=main(read('excluir-conta'));
 assert.match(body,/href="https:\/\/wa\.me\/556696314240\?/);
 assert.match(body,/href="tel:\+556696314240"/);
 assert.match(body,/sem reinstalar/i);
 assert.match(body,/Gerenciar conta/);
 assert.match(body,/Excluir minha conta/);
 assert.match(body,/href="\/privacidade\/#retencao"/);
 assert.doesNotMatch(body,/href="https:\/\/github\.com\/[^"\s]+\/issues/);
 assert.doesNotMatch(body,/nome de usuário através|excluídos imediatamente dos bancos|garantimos.*todas as cópias/i);
 assert.match(body,/não envie[^<]*(?:chave|senha)/i);
});

test('the current notice distinguishes SDK diagnostics, local tracks and long-lived records from transient evidence',()=>{
 const body=main(read('privacidade'));
 for(const topic of ['Google ML Kit','Nominatim','WhatsApp','EXIF','trajetos','24 horas','Brasil','registros financeiros','treinamento','6 meses','Stripe','Asaas']) assert.ok(body.includes(topic),topic);
 assert.match(body,/coleta para treinamento[^<]*desativada/i);
 assert.doesNotMatch(body,/não (?:coleta|coletamos|transmite|transmitimos) (?:quaisquer )?dados pessoais|somente uma vez|todas as fotos[^<]*24 horas/i);
 assert.doesNotMatch(body,/example\.invalid|\[(?:CONTATO|EMAIL|PROVEDOR|PRAZO|CONTROLADOR)\]/i);
 assert.match(body,/novas finalidades[^<]*consentimento/i);
});
