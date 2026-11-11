import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { execFileSync } from 'node:child_process';
import { pathToFileURL, fileURLToPath } from 'node:url';
export const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)),'..');
export function compile(){
 const dir=fs.mkdtempSync(path.join(os.tmpdir(),'empresas-web-test-'));
 try { execFileSync(path.join(root,'node_modules','.bin','tsc'),['--project',path.join(root,'tsconfig.json'),'--outDir',dir],{stdio:'pipe'}); }
 catch(e){fs.rmSync(dir,{recursive:true,force:true});throw e;}
 fs.writeFileSync(path.join(dir,'package.json'),'{"type":"module"}');
 return {dir,load:name=>import(pathToFileURL(path.join(dir,`${name}.js`))),cleanup:()=>fs.rmSync(dir,{recursive:true,force:true})};
}
export const uuid=(n)=>`7b0e2a54-0000-4000-8000-${String(n).padStart(12,'0')}`;
export const json=(value,status=200)=>new Response(JSON.stringify(value),{status,headers:{'Content-Type':'application/json','Cache-Control':'no-store'}});
export function deferred(){let resolve,reject;const promise=new Promise((yes,no)=>{resolve=yes;reject=no;});return {promise,resolve,reject};}
