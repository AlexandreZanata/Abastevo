import test from 'node:test';
import assert from 'node:assert/strict';
import { compile, uuid, deferred } from './support.mjs';
const {load,cleanup}=compile();const {CompanyController,unavailableMemberships}=await load('controller');const {ApiError}=await load('api');process.on('exit',cleanup);
const a={id:uuid(2),membershipId:uuid(3),name:'Empresa A sintética'}, b={id:uuid(4),membershipId:uuid(5),name:'Empresa B sintética'};
const org=id=>({kind:'organization',id});
function fixture(source){
 const session={authenticated:true,deadline:Date.now()+3600_000,async read(){if(!this.authenticated)throw new ApiError('unauthenticated');return {accountId:uuid(1)};},async signIn(){this.authenticated=true;return {accountId:uuid(1)};},forget(){this.authenticated=false;},async signOut(){this.forget();}};
 const states=[];const authority=source??{async list(){return {accountId:uuid(1),organizations:[a,b]};},async authorize(accountId,id){return {accountId,organization:id===a.id?a:b};}};
 const controller=new CompanyController(session,authority,state=>states.push(structuredClone(state)));
 return {controller,session,states,source:authority};
}
test('deep links and picker require current account-bound memberships and authority; empty/unavailable differ',async()=>{
 const f=fixture();await f.controller.navigate(org(a.id));assert.equal(f.controller.state.selected.id,a.id);
 await f.controller.navigate(org(uuid(9)));assert.equal(f.controller.state.phase,'denied');assert.equal(f.controller.state.selected,null);
 f.source.list=async()=>({accountId:uuid(1),organizations:[]});await f.controller.navigate({kind:'overview'});assert.equal(f.controller.state.phase,'empty');
 const absent=fixture(unavailableMemberships);await absent.controller.navigate({kind:'overview'});assert.equal(absent.controller.state.phase,'unavailable');
 for(const authority of [{accountId:uuid(9),organization:a},{accountId:uuid(1),organization:b},{accountId:uuid(1),organization:{...a,membershipId:uuid(9)}}]){
  const g=fixture();g.source.authorize=async()=>authority;await g.controller.navigate(org(a.id));assert.equal(g.controller.state.phase,'denied');assert.equal(g.controller.state.selected,null);
 }
});
test('switch clears prior content synchronously; late responses cannot overwrite the new organization',async()=>{
 const f=fixture();await f.controller.navigate(org(a.id));
 const late=deferred();let oldSignal;
 f.source.authorize=async(accountId,id,signal)=>{if(id===a.id){oldSignal=signal;return late.promise;}return {accountId,organization:b};};
 const old=f.controller.navigate(org(a.id));await new Promise(setImmediate);
 const next=f.controller.navigate(org(b.id));assert.equal(f.controller.state.selected,null);assert.deepEqual(f.controller.state.organizations,[]);
 await next;assert.equal(oldSignal.aborted,true);assert.equal(f.controller.state.selected.id,b.id);
 late.resolve({accountId:uuid(1),organization:a});await old;assert.equal(f.controller.state.selected.id,b.id);
});
test('logout, session denial and stale loads leave no private scope; logout failures stay honest',async()=>{
 const f=fixture();const late=deferred();f.source.list=()=>late.promise;
 const loading=f.controller.navigate(org(a.id));await new Promise(setImmediate);
 f.session.signOut=async()=>{f.session.forget();throw new Error('PRIVATE-OUTPUT');};
 await f.controller.signOut();assert.equal(f.controller.state.phase,'signed-out');assert.match(f.controller.state.message,/não pôde ser confirmada/);
 late.resolve({accountId:uuid(1),organizations:[a]});await loading;assert.equal(f.controller.state.selected,null);assert.deepEqual(f.controller.state.organizations,[]);
 const g=fixture();g.session.read=async()=>{throw new ApiError('unauthenticated');};await g.controller.navigate(org(a.id));assert.equal(g.controller.state.phase,'signed-out');assert.equal(g.session.authenticated,false);
 const h=fixture();h.source.authorize=async()=>{throw new ApiError('denied');};await h.controller.navigate(org(a.id));assert.equal(h.controller.state.phase,'denied');assert.equal(h.controller.state.selected,null);
});
