import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import {build,root} from '../scripts/build.mjs';
import {checkDist,caddyfile} from '../scripts/check.mjs';
test('build is deterministic, self-contained under /empresas, content-hashed and rejects artifact drift',()=>{
 const tmp=fs.mkdtempSync(path.join(os.tmpdir(),'empresas-artifact-'));
 try{
  const {outDir,assets}=build({outDir:path.join(tmp,'dist')});
  assert.equal(checkDist(outDir),25);
  assert.equal(fs.existsSync(path.join(outDir,'_headers')),false);
  const original=fs.readFileSync(path.join(outDir,'assets',assets['main.js']),'utf8');
  fs.writeFileSync(path.join(outDir,'stale.js'),'stale');
  const rebuilt=build({outDir});assert.deepEqual(rebuilt.assets,assets);assert.equal(fs.existsSync(path.join(outDir,'stale.js')),false);
  assert.equal(fs.readFileSync(path.join(outDir,'assets',assets['main.js']),'utf8'),original);
  fs.appendFileSync(path.join(outDir,'assets',assets['api.js']),'\n// modified');
  assert.throws(()=>checkDist(outDir),/Hash mismatch/);
  build({outDir});fs.writeFileSync(path.join(outDir,'secrets.ts'),'private source');
  assert.throws(()=>checkDist(outDir),/Source leak/);
  build({outDir});fs.appendFileSync(path.join(outDir,'index.html'),'<script src="/admin/assets/secret.js"></script>');
  assert.throws(()=>checkDist(outDir),/Unmounted resource/);
  build({outDir});const page=path.join(outDir,'index.html');
  fs.writeFileSync(page,fs.readFileSync(page,'utf8').replace('id="account-key"','id="account-key" name="account-key"'));
  assert.throws(()=>checkDist(outDir),/Natively submittable credential/);
  build({outDir});fs.writeFileSync(page,fs.readFileSync(page,'utf8').replace('id="login-button" class="button-primary" type="submit" disabled>','id="login-button" class="button-primary" type="submit">'));
  assert.throws(()=>checkDist(outDir),/Natively submittable credential/);
  // Caddy is the only served policy: a dead header file or a weakened Caddyfile fails.
  build({outDir});fs.writeFileSync(path.join(outDir,'_headers'),'/empresas/*\n  Cache-Control: no-store\n');
  assert.throws(()=>checkDist(outDir),/Unserved policy file/);
  build({outDir});const served=fs.readFileSync(caddyfile,'utf8'),drifted=path.join(tmp,'Caddyfile');
  for(const [from,to]of [["base-uri 'none'","base-uri 'self'"],['\t\t\t-Last-Modified\n',''],['Cache-Control no-store\n\t\t\t-ETag','Cache-Control no-cache\n\t\t\t-ETag'],['path_regexp companyAsset','path']]){
   assert.ok(served.includes(from),from);fs.writeFileSync(drifted,served.replace(from,to));
   assert.throws(()=>checkDist(outDir,drifted),/Served company policy/,from);
  }
  assert.equal(checkDist(outDir),25);
  assert.throws(()=>build({outDir:root}),/Unsafe output/);
 }finally{fs.rmSync(tmp,{recursive:true,force:true});}
});
test('runtime source never uses unsafe DOM, persistence, console, privileged routes or service workers',()=>{
 for(const file of fs.readdirSync(path.join(root,'src'))){
  const source=fs.readFileSync(path.join(root,'src',file),'utf8');
  for(const pattern of [/innerHTML/,/outerHTML/,/insertAdjacentHTML/,/document\.write/,/\beval\s*\(/,/localStorage/,/sessionStorage/,/indexedDB/,/console\./,/serviceWorker/,/\/v1\/admin/,/X-Admin-CSRF/])assert.doesNotMatch(source,pattern,file);
 }
 const html=fs.readFileSync(path.join(root,'static/index.html'),'utf8');
 for(const required of ['for="account-key"','type="password"','for="organization-select"','aria-describedby="organization-help"','aria-live="polite"','Pular para o conteúdo'])assert.ok(html.includes(required));
});
