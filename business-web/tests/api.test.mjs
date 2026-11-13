import test from 'node:test';
import assert from 'node:assert/strict';
import { compile, uuid, json, deferred } from './support.mjs';
const { load, cleanup }=compile(); const { CompanyApi, ApiError }=await load('api'); process.on('exit',cleanup);
const csrf='abv_company_csrf_'+'a'.repeat(64);
const now=Date.parse('2026-11-11T18:00:00Z');
const view=(id=uuid(1))=>({account_id:id,authenticated_at:new Date(now).toISOString(),expires_at:new Date(now+8*3600_000).toISOString()});
const proof={account:{account_id:uuid(1)},session:{family_id:uuid(2),access_token:'synthetic-access',refresh_token:'NEVER-RETAIN'}};
const probe=(path,init)=>path.endsWith('/session/read')&&init.headers['X-CSRF']===undefined;
function fixture(handler,plane=()=>json({error:{code:'UNAUTHENTICATED'}},401)){
 const calls=[]; let clock=now;
 const api=new CompanyApi(async(path,init)=>{calls.push({path,init});if(probe(path,init))return plane();return handler?handler(path,init,calls):json(path.endsWith('/keys/login')?proof:path.endsWith('/logout')?{logged_out:true}:{...view(),csrf_token:csrf});},()=>clock);
 return {api,calls,advance:ms=>clock+=ms};
}
test('fresh account exchange and every private request use fixed same-origin/no-store transport; proof is not retained',async()=>{
 const {api,calls}=fixture();
 await api.signIn('synthetic-account-key'); await api.read(); await api.signOut();
 assert.deepEqual(calls.map(c=>c.path),['/v1/org-access/session/read','/v1/accounts/keys/login','/v1/org-access/session/login','/v1/org-access/session/read','/v1/org-access/session/logout']);
 assert.deepEqual(JSON.parse(calls[0].init.body),{});assert.equal(calls[0].init.headers['X-CSRF'],undefined);
 assert.equal(calls[4].init.keepalive,true);
 for(const {init}of calls){assert.equal(init.method,'POST');assert.equal(init.credentials,'same-origin');assert.equal(init.cache,'no-store');assert.equal(init.redirect,'error');assert.equal(init.referrerPolicy,'no-referrer');assert.ok(init.signal instanceof AbortSignal);assert.equal(init.headers.Authorization,undefined);}
 assert.deepEqual(JSON.parse(calls[2].init.body),{family_id:uuid(2),access_token:'synthetic-access'});
 assert.deepEqual(JSON.parse(calls[3].init.body),{});assert.equal(calls[3].init.headers['X-CSRF'],csrf);
 assert.equal(calls[1].init.headers['X-CSRF'],undefined);assert.equal(calls[2].init.headers['X-CSRF'],undefined);
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
 const f=fixture();await f.api.signIn('key');f.advance(30*60_000);await assert.rejects(f.api.read(),e=>e.kind==='unauthenticated');assert.equal(f.calls.length,3);
});
test('late login cannot restore a forgotten session and failed logout never claims confirmation',async()=>{
 const pending=deferred();
 const f=fixture(path=>path.endsWith('/keys/login')?json(proof):path.endsWith('/logout')?Promise.reject(new Error('PRIVATE-NETWORK')):pending.promise);
 const login=f.api.signIn('key');await new Promise(setImmediate);f.api.forget();pending.resolve(json({...view(),csrf_token:csrf}));
 await assert.rejects(login,e=>e.kind==='cancelled');assert.equal(f.api.authenticated,false);
 const g=fixture(path=>path.endsWith('/keys/login')?json(proof):path.endsWith('/logout')?json({},503):json({...view(),csrf_token:csrf}));
 await g.api.signIn('key');await assert.rejects(g.api.signOut(),e=>e instanceof ApiError&&e.kind==='unavailable');assert.equal(g.api.authenticated,false);
});
test('an unavailable company plane never receives the account key or creates an account session',async()=>{
 for(const plane of [()=>json({error:{code:'NOT_FOUND'}},404),()=>json({},503),()=>json({...view(),csrf_token:csrf}),()=>Promise.reject(new Error('PRIVATE-NETWORK'))]){
  const f=fixture(undefined,plane);
  await assert.rejects(f.api.signIn('synthetic-account-key'),e=>e instanceof ApiError&&e.kind==='unavailable');
  assert.deepEqual(f.calls.map(c=>c.path),['/v1/org-access/session/read']);
  assert.equal(JSON.stringify(f.calls).includes('synthetic-account-key'),false);assert.equal(f.api.authenticated,false);
 }
 const g=fixture(undefined,()=>json({error:{code:'FORBIDDEN'}},403));await g.api.signIn('key');assert.equal(g.api.authenticated,true);
});
test('requests stay cancellable and bounded where AbortSignal.any is missing',async()=>{
 const any=AbortSignal.any;
 try{
  delete AbortSignal.any;
  const f=fixture();await f.api.signIn('key');assert.equal(f.api.authenticated,true);
  const outer=new AbortController();const seen=deferred();
  const g=fixture(async(path,init)=>{seen.resolve(init.signal);return new Promise((_,no)=>init.signal.addEventListener('abort',()=>no(init.signal.reason)));});
  const login=g.api.signIn('key',outer.signal);const signal=await seen.promise;assert.equal(signal.aborted,false);
  outer.abort();assert.equal(signal.aborted,true);await assert.rejects(login,e=>e.kind==='cancelled');
 }finally{AbortSignal.any=any;}
});
test('a fresh-authentication refusal stays distinct from an invalid key and from bounded unreadable errors',async()=>{
 const f=fixture(path=>path.endsWith('/keys/login')?json(proof):json({error:{code:'FRESH_AUTH_REQUIRED',message:'Organization request unavailable or refused'}},401));
 await assert.rejects(f.api.signIn('key'),e=>e instanceof ApiError&&e.kind==='fresh');assert.equal(f.api.authenticated,false);
 for(const refusal of [()=>json({error:{code:'UNAUTHENTICATED'}},401),()=>new Response('not json',{status:401}),()=>json({error:{code:'FRESH_AUTH_REQUIRED',padding:'x'.repeat(65*1024)}},401)]){
  const g=fixture(path=>path.endsWith('/keys/login')?json(proof):refusal());
  await assert.rejects(g.api.signIn('key'),e=>e instanceof ApiError&&e.kind==='unauthenticated');assert.equal(g.api.authenticated,false);
 }
});
test('business requests require a live memory-only company session, strict paths and epoch fencing',async()=>{
 const f=fixture(path=>path.endsWith('/keys/login')?json(proof):path.includes('/session/')?json({...view(),csrf_token:csrf}):json({items:[],next_cursor:''}));
 await assert.rejects(f.api.privatePost('/v1/org-access/contexts/list',{}),e=>e.kind==='unauthenticated');await f.api.signIn('key');
 const before=f.calls.length;for(const path of ['/v1/admin/accounts/read','/v1/org-access/contexts/list?code=private','/v1/orgs/../../employees/list'])await assert.rejects(f.api.privatePost(path,{}),e=>e.kind==='denied');assert.equal(f.calls.length,before);
 await f.api.privatePost('/v1/org-access/contexts/list',{});const call=f.calls.at(-1);assert.equal(call.init.headers['X-CSRF'],csrf);assert.equal(call.init.cache,'no-store');
 const late=deferred();const g=fixture(path=>path.endsWith('/keys/login')?json(proof):path.includes('/session/')?json({...view(),csrf_token:csrf}):late.promise);await g.api.signIn('key');const pending=g.api.privatePost('/v1/org-access/contexts/list',{});g.api.forget();late.resolve(json({items:[]}));await assert.rejects(pending,e=>e.kind==='cancelled');
});
test('concealed missing membership on a private company request is a definitive denial',async()=>{
 const f=fixture(path=>path.endsWith('/keys/login')?json(proof):path.includes('/session/')?json({...view(),csrf_token:csrf}):json({error:{code:'NOT_FOUND'}},404));await f.api.signIn('key');await assert.rejects(f.api.privatePost(`/v1/orgs/${uuid(2)}/employees/list`,{}),e=>e.kind==='denied');
});
