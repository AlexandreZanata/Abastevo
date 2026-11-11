import test from 'node:test';
import assert from 'node:assert/strict';
import { compile, uuid, json, deferred } from './support.mjs';
const { load, cleanup }=compile(); const { CompanyApi, ApiError }=await load('api'); process.on('exit',cleanup);
const csrf='abv_company_csrf_'+'a'.repeat(64);
const now=Date.parse('2026-11-11T18:00:00Z');
const view=(id=uuid(1))=>({account_id:id,authenticated_at:new Date(now).toISOString(),expires_at:new Date(now+8*3600_000).toISOString()});
const proof={account:{account_id:uuid(1)},session:{family_id:uuid(2),access_token:'synthetic-access',refresh_token:'NEVER-RETAIN'}};
function fixture(handler){
 const calls=[]; let clock=now;
 const api=new CompanyApi(async(path,init)=>{calls.push({path,init});return handler?handler(path,init,calls):json(path.endsWith('/keys/login')?proof:path.endsWith('/logout')?{logged_out:true}:{...view(),csrf_token:csrf});},()=>clock);
 return {api,calls,advance:ms=>clock+=ms};
}
test('fresh account exchange and every private request use fixed same-origin/no-store transport; proof is not retained',async()=>{
 const {api,calls}=fixture();
 await api.signIn('synthetic-account-key'); await api.read(); await api.signOut();
 assert.deepEqual(calls.map(c=>c.path),['/v1/accounts/keys/login','/v1/org-access/session/login','/v1/org-access/session/read','/v1/org-access/session/logout']);
 for(const {init}of calls){assert.equal(init.method,'POST');assert.equal(init.credentials,'same-origin');assert.equal(init.cache,'no-store');assert.equal(init.redirect,'error');assert.equal(init.referrerPolicy,'no-referrer');assert.ok(init.signal instanceof AbortSignal);assert.equal(init.headers.Authorization,undefined);}
 assert.deepEqual(JSON.parse(calls[1].init.body),{family_id:uuid(2),access_token:'synthetic-access'});
 assert.deepEqual(JSON.parse(calls[2].init.body),{});assert.equal(calls[2].init.headers['X-CSRF'],csrf);
 assert.equal(calls[0].init.headers['X-CSRF'],undefined);assert.equal(calls[1].init.headers['X-CSRF'],undefined);
 assert.equal(api.authenticated,false);assert.equal(JSON.stringify(api).includes('NEVER-RETAIN'),false);assert.equal(JSON.stringify(api).includes(csrf),false);
});
test('cookie account substitution, auth/CSRF refusal, expired timing and cache-policy drift clear or deny access',async()=>{
 for(const result of [()=>json(view(uuid(8))),()=>json({},401),()=>json({},403)]){
  const f=fixture((path)=>path.endsWith('/keys/login')?json(proof):path.endsWith('/read')?result():json({...view(),csrf_token:csrf}));
  await f.api.signIn('key');await assert.rejects(f.api.read());assert.equal(f.api.authenticated,false);
 }
 for(const response of [()=>new Response(JSON.stringify({...view(),csrf_token:csrf}),{headers:{'Content-Type':'application/json'}}),()=>json({...view(),csrf_token:'admin-token'}),()=>json({...view(),expires_at:new Date(now).toISOString(),csrf_token:csrf}),()=>json({text:'x'.repeat(65*1024)})]){
  const f=fixture(path=>path.endsWith('/keys/login')?json(proof):response());
  await assert.rejects(f.api.signIn('key'));assert.equal(f.api.authenticated,false);
 }
 const f=fixture();await f.api.signIn('key');f.advance(30*60_000);await assert.rejects(f.api.read(),e=>e.kind==='unauthenticated');assert.equal(f.calls.length,2);
});
test('late login cannot restore a forgotten session and failed logout never claims confirmation',async()=>{
 const pending=deferred();
 const f=fixture(path=>path.endsWith('/keys/login')?json(proof):path.endsWith('/logout')?Promise.reject(new Error('PRIVATE-NETWORK')):pending.promise);
 const login=f.api.signIn('key');await new Promise(setImmediate);f.api.forget();pending.resolve(json({...view(),csrf_token:csrf}));
 await assert.rejects(login,e=>e.kind==='cancelled');assert.equal(f.api.authenticated,false);
 const g=fixture(path=>path.endsWith('/keys/login')?json(proof):path.endsWith('/logout')?json({},503):json({...view(),csrf_token:csrf}));
 await g.api.signIn('key');await assert.rejects(g.api.signOut(),e=>e instanceof ApiError&&e.kind==='unavailable');assert.equal(g.api.authenticated,false);
});
