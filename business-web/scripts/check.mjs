import fs from 'node:fs';
import path from 'node:path';
import zlib from 'node:zlib';
import { fileURLToPath } from 'node:url';
import { root, hash } from './build.mjs';
export function checkDist(dir=path.join(root,'dist')){
 const errors=[],files=[];
 function walk(at){for(const e of fs.readdirSync(at,{withFileTypes:true})){const name=path.join(at,e.name);if(e.isDirectory())walk(name);else files.push(path.relative(dir,name));}}
 walk(dir);
 for(const required of ['index.html','entrar/index.html','404.html','_headers','robots.txt'])if(!files.includes(required))errors.push('Missing '+required);
 for(const module of ['main','app','api','controller','model'])if(!files.some(n=>new RegExp(`^assets/${module}\\.[a-f0-9]{8}\\.js$`).test(n)))errors.push('Missing module '+module);
 function mounted(ref,owner){
  if(ref.startsWith('#'))return;
  if(!ref.startsWith('/empresas/')||ref.includes('..')){errors.push('Unmounted resource in '+owner);return;}
  let local=ref.slice('/empresas/'.length);if(local===''||local.endsWith('/'))local+='index.html';
  if(!files.includes(local))errors.push('Missing resource '+local);
 }
 for(const file of files){
  if(/\.(?:ts|map)$/.test(file)||/(?:^|\/)(?:\.env|package(?:-lock)?\.json)/.test(file))errors.push('Source leak '+file);
  if(/\.(?:html|css|js)$/.test(file)){
   const text=fs.readFileSync(path.join(dir,file),'utf8');
   if(/https?:\/\/|@import|unsafe-inline|unsafe-eval/.test(text))errors.push('Remote/unsafe executable reference '+file);
   if(file.endsWith('.html')){
    if(!text.includes('name="robots" content="noindex, nofollow"'))errors.push('Missing noindex '+file);
    if(/<script(?![^>]*src=)[^>]*>| on[a-z]+=/i.test(text))errors.push('Inline script '+file);
    // Without the module, a native submit would put a named key in the URL.
    const credential=/<input[^>]*id="account-key"[^>]*>/.exec(text)?.[0];
    if(credential!==undefined&&(/\sname=/.test(credential)||!/<button id="login-button"[^>]*\sdisabled[\s>]/.test(text)))errors.push('Natively submittable credential '+file);
    for(const m of text.matchAll(/(?:src|href)="([^"]+)"/g))mounted(m[1],file);
   }
   if(file.endsWith('.css'))for(const m of text.matchAll(/url\(['"]?([^)'"]+)['"]?\)/g))mounted(m[1],file);
   if(file.endsWith('.js'))for(const m of text.matchAll(/from ['"]\.\/([^'"]+)['"]/g))if(!files.includes('assets/'+m[1]))errors.push('Missing module import '+m[1]);
   if(zlib.gzipSync(text).length>(file.endsWith('.css')?25:15)*1024)errors.push('Budget exceeded '+file);
  }
  const fingerprint=/\.([a-f0-9]{8})\.(?:js|css|svg)$/.exec(file);
  if(fingerprint&&hash(fs.readFileSync(path.join(dir,file)))!==fingerprint[1])errors.push('Hash mismatch '+file);
 }
 const headers=fs.readFileSync(path.join(dir,'_headers'),'utf8');
 for(const value of ["script-src 'self'","connect-src 'self'","frame-ancestors 'none'","base-uri 'none'",'Cache-Control: no-store','Cache-Control: public, max-age=31536000, immutable','Referrer-Policy: no-referrer'])if(!headers.includes(value))errors.push('Missing security policy '+value);
 if(!fs.readFileSync(path.join(dir,'robots.txt'),'utf8').includes('Disallow: /'))errors.push('Missing robots refusal');
 if(errors.length)throw new Error(errors.join('\n'));
 return files.length;
}
if(process.argv[1]===fileURLToPath(import.meta.url))console.log('Company artifact check:',checkDist(),'files');
