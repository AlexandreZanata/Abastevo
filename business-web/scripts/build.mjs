import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import crypto from 'node:crypto';
import { execFileSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
export const root=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'..');
export const mount='/empresas';
export const hash=bytes=>crypto.createHash('sha256').update(bytes).digest('hex').slice(0,8);
export function build({outDir=path.join(root,'dist')}={}){
 const out=path.resolve(outDir);
 if(root.startsWith(out+path.sep)||out===root||out.startsWith(path.join(root,'src')+path.sep)||out===path.join(root,'src')||out===path.join(root,'static'))throw new Error('Unsafe output directory');
 const tmp=fs.mkdtempSync(path.join(os.tmpdir(),'empresas-build-'));
 try{
  execFileSync(path.join(root,'node_modules','.bin','tsc'),['--project',path.join(root,'tsconfig.json'),'--outDir',tmp],{stdio:'pipe'});
  fs.rmSync(out,{recursive:true,force:true});fs.mkdirSync(path.join(out,'assets'),{recursive:true});
  const assets=new Map(),visiting=new Set();
  const write=(name,bytes)=>{
   const parsed=path.parse(name),dest=`${parsed.name}.${hash(bytes)}${parsed.ext}`;
   fs.writeFileSync(path.join(out,'assets',dest),bytes);assets.set(name,dest);return dest;
  };
  function module(name){
   if(assets.has(name))return assets.get(name);
   if(visiting.has(name)||!/^[a-z]+\.js$/.test(name))throw new Error('Unsupported module graph');
   visiting.add(name);
   let text=fs.readFileSync(path.join(tmp,name),'utf8');
   text=text.replace(/from (['"])\.\/([a-z]+\.js)\1/g,(_all,quote,child)=>`from ${quote}./${module(child)}${quote}`);
   visiting.delete(name);return write(name,text);
  }
  module('main.js');
  const landing=path.resolve(root,'../landing/static/assets');
  let tokens=fs.readFileSync(path.join(landing,'styles.css'),'utf8');
  const end=tokens.indexOf('\n*, *::before');
  if(end<0||!tokens.slice(0,end).includes(':root {'))throw new Error('Landing token boundary missing');
  tokens=tokens.slice(0,end).replaceAll('/assets/','/empresas/assets/');
  write('tokens.css',tokens);write('business.css',fs.readFileSync(path.join(root,'static/assets/business.css')));
  const fonts=JSON.parse(fs.readFileSync(path.join(landing,'fonts/provenance.json'),'utf8'));
  fs.mkdirSync(path.join(out,'assets/fonts'),{recursive:true});
  for(const font of fonts.fonts){
   if(!/^[a-z0-9-]+\.[a-f0-9]{8}\.woff2$/.test(font.file))throw new Error('Invalid font filename');
   const bytes=fs.readFileSync(path.join(landing,'fonts',font.file));
   if(crypto.createHash('sha256').update(bytes).digest('hex')!==font.sha256)throw new Error('Font provenance mismatch');
   fs.writeFileSync(path.join(out,'assets/fonts',font.file),bytes);
  }
  for(const name of ['OFL-Manrope.txt','OFL-Source-Sans-3.txt'])fs.copyFileSync(path.join(landing,'fonts',name),path.join(out,'assets/fonts',name));
  for(const name of ['abastevo-logo.svg','favicon.svg'])write(name,fs.readFileSync(path.join(landing,'brand',name)));
  for(const [source,dest]of [['index.html','index.html'],['index.html','entrar/index.html'],['index.html','criar/index.html'],['404.html','404.html']]){
   let html=fs.readFileSync(path.join(root,'static',source),'utf8');
   for(const [name,fingerprinted]of assets)html=html.replaceAll(`/empresas/assets/${name}`,`/empresas/assets/${fingerprinted}`);
   fs.mkdirSync(path.dirname(path.join(out,dest)),{recursive:true});fs.writeFileSync(path.join(out,dest),html);
  }
  // Caddy serves the response policy; a static header file would be ignored and served.
  fs.copyFileSync(path.join(root,'static','robots.txt'),path.join(out,'robots.txt'));
  return {outDir:out,assets:Object.fromEntries(assets)};
 }finally{fs.rmSync(tmp,{recursive:true,force:true});}
}
if(process.argv[1]===fileURLToPath(import.meta.url)){const result=build();console.log('Company build:',result.outDir);}
