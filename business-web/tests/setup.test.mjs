import test from 'node:test';
import assert from 'node:assert/strict';
import {compile,uuid,deferred} from './support.mjs';
const {load,cleanup}=compile();process.on('exit',cleanup);
const {OrganizationSetupApi,OrganizationSetupController,SYNTHETIC_TERMS}=await load('setup');
const {ApiError}=await load('api');
const account=uuid(1), org=uuid(2), member=uuid(3);
const values=()=>({time_zone:'America/Cuiaba',consumption_display:'KM_PER_L',review_required:{trips:true,fuelings:true,expenses:true},approval_levels_default:1,closure_period_days:30,policies:{intake_window_days:15,clock_skew_limit_seconds:'300',delegation_maximum_days:90,invitation_validity_days:7,activation_validity_hours:48},responsibilities:{review_fallback:{primary:'',substitute:''},platform_notices:{primary:member,substitute:''},data_protection:{primary:member,substitute:''},billing:{primary:member,substitute:''}}});
const wire=(id=org)=>({revision:1,root_revision:1,effective_at:'2026-11-13T12:00:00Z',values:values(),source:'ONBOARDING',reason:'Organization creation',ownership_transfer_validity_days:7,data_mode:'SYNTHETIC',organization:{id,name:'Synthetic company',time_zone:'America/Cuiaba',kinds:['OTHER'],state:'PENDING_SETUP',revision:1,suspension_reason:''},setup:{ready:true,missing:[],terms:{...SYNTHETIC_TERMS,recorded:true}},context:{account_id:account,membership_id:member,permissions:['org.settings.read','org.settings.manage'],fresh:true,capability_enabled:true},responsibility_members:[{membership_id:member,display_name:'',profile_pending:true,eligible:true}]});
const summary=()=>({id:org,name:'Synthetic company',membership_id:member,state:'PENDING_SETUP',root_revision:1});
const draft=()=>({name:'Synthetic company',time_zone:'America/Cuiaba',kinds:['OTHER'],unit_name:'Synthetic unit',acceptedTerms:true});
function fixture(){
 const calls=[];let snapshot=wire();const source=new OrganizationSetupApi({async privatePost(path,body){calls.push({path,body:structuredClone(body)});if(path.includes('/setups/list'))return {account_id:account,setups:[summary()],next_after_org_id:''};if(path.includes('/responsibility-members/list'))return {account_id:account,organization_id:org,membership_id:member,members:[{membership_id:member,display_name:'',profile_pending:true}],next_after_membership_id:''};if(path.endsWith('/settings/history'))return {items:[snapshot],next_cursor:''};if(path.endsWith('/settings/read')||path.endsWith('/settings/revision/read'))return snapshot;throw new Error('Unexpected synthetic request');}});
 const states=[];const controller=new OrganizationSetupController(source,s=>states.push(structuredClone(s)));
 return {source,controller,calls,states,setSnapshot:s=>snapshot=s};
}
test('setup route uses persisted bound read and keeps capability pending separate from completion',async()=>{
 const f=fixture();await f.controller.open(account,org);assert.equal(f.controller.state.snapshot.organization.id,org);assert.equal(f.controller.canEdit,true);assert.equal(f.controller.canActivate,true);
 const pending=wire();pending.context.capability_enabled=false;f.setSnapshot(pending);await f.controller.refresh();assert.equal(f.controller.canEdit,false);assert.equal(f.controller.canActivate,false);assert.equal(f.controller.state.snapshot.organization.state,'PENDING_SETUP');
 const active=wire();active.organization.state='ACTIVE';f.setSnapshot(active);await f.controller.refresh();assert.equal(f.controller.canActivate,false);
});
test('parser rejects cross-account, wrong organization, unsafe revisions, incomplete structures and stale member labels',async()=>{
 for(const change of [v=>v.context.account_id=uuid(9),v=>v.organization.id=uuid(9),v=>v.revision=9007199254740992,v=>delete v.values.review_required,v=>v.setup.missing=['private-id'],v=>v.responsibility_members[0].membership_id=uuid(9)]){
  const raw=wire();change(raw);const api=new OrganizationSetupApi({async privatePost(){return raw;}});await assert.rejects(api.read(account,org,new AbortController().signal),e=>e instanceof ApiError);
 }
});
test('scope replacement and late responses cannot repopulate prior private settings',async()=>{
 const f=fixture();const late=deferred();let signal;f.source.read=async(a,id,s)=>{signal=s;return late.promise;};const first=f.controller.open(account,org);await new Promise(setImmediate);f.controller.clear();assert.equal(signal.aborted,true);late.resolve({});await first;assert.equal(f.controller.state.snapshot,null);assert.equal(f.controller.state.accountId,'');
});
test('creation retry preserves immutable operation and reads current state after historical acknowledgement',async()=>{
 const f=fixture();await f.controller.open(account);let attempt=0;const bodies=[];f.source.create=async(a,body)=>{bodies.push(structuredClone(body));if(++attempt===1)throw new ApiError('unavailable');return {organization_id:org,state:'PENDING_SETUP',revision:1};};
 await f.controller.create(draft());assert.equal(f.controller.retryable,true);const changed=draft();changed.name='Different intent';await f.controller.create(changed);assert.equal(bodies.length,1);await f.controller.retry();assert.equal(bodies.length,2);assert.deepEqual(bodies[0],bodies[1]);assert.equal(f.controller.state.snapshot.organization.id,org);assert.equal(f.controller.retryable,false);
});
test('named responsibility labels preserve employee bounds of 200 Unicode scalars',async()=>{
 const raw=wire();raw.responsibility_members[0].display_name='😀'.repeat(200);raw.responsibility_members[0].profile_pending=false;
 const api=new OrganizationSetupApi({async privatePost(){return raw;}});
 assert.equal((await api.read(account,org,new AbortController().signal)).responsibility_members[0].display_name,raw.responsibility_members[0].display_name);
 raw.responsibility_members[0].display_name+='x';await assert.rejects(api.read(account,org,new AbortController().signal));
});
test('acknowledged creation with failed current read retains only bound recovery and never creates again',async()=>{
 const f=fixture();await f.controller.open(account);let creates=0,reads=0;const original=f.source.read.bind(f.source);
 f.source.create=async()=>{creates++;return {organization_id:org};};f.source.read=async(...args)=>{if(++reads===1)throw new ApiError('unavailable');return original(...args);};
 await f.controller.create(draft());assert.equal(f.controller.state.organizationId,org);assert.equal(f.controller.retryable,true);assert.equal(f.controller.state.snapshot,null);
 await f.controller.create(draft());assert.equal(creates,1);await f.controller.retry();assert.equal(creates,1);assert.equal(reads,2);assert.equal(f.controller.state.snapshot.organization.id,org);assert.equal(f.controller.retryable,false);
});
test('uncertain write cannot be discarded by history member loading or same-context refresh',async()=>{
 const f=fixture();await f.controller.open(account,org);const original=f.source.change.bind(f.source);let writes=0;const bodies=[];
 f.source.change=async(_org,body)=>{writes++;bodies.push(structuredClone(body));if(writes===1)throw new ApiError('unavailable');return {};};
 await f.controller.save(values(),'Synthetic reason');assert.equal(f.controller.retryable,true);const before=f.calls.length;
 await f.controller.history();await f.controller.members();await f.controller.refresh();assert.equal(f.controller.retryable,true);assert.equal(f.calls.length,before);
 await f.controller.retry();assert.equal(writes,2);assert.deepEqual(bodies[0],bodies[1]);assert.equal(f.controller.retryable,false);
});
test('historical revision never mixes a new current root with old current settings',async()=>{
 const f=fixture();await f.controller.open(account,org);const changed=wire();changed.revision=2;changed.root_revision=2;changed.values.time_zone='UTC';changed.organization.time_zone='UTC';changed.organization.revision=2;f.setSnapshot(changed);
 const parsed=new OrganizationSetupApi({async privatePost(){return {...changed,revision:1,values:values()};}});
 f.source.revision=(a,o,r,s)=>parsed.revision(a,o,r,s);
 await f.controller.readRevision(1);assert.equal(f.controller.state.historical.revision,1);assert.equal(f.controller.state.historical.values.time_zone,'America/Cuiaba');assert.equal(f.controller.state.snapshot.settings.revision,2);assert.equal(f.controller.state.snapshot.settings.values.time_zone,'UTC');assert.equal(f.controller.state.snapshot.root_revision,2);
});
test('unknown responsibility selections are refused without clearing a saved designation or sending UUID guesses',async()=>{
 const f=fixture();await f.controller.open(account,org);let writes=0;f.source.change=async()=>{writes++;return {};};const changed=values();changed.responsibilities.billing.primary=uuid(90);
 await f.controller.save(changed,'Synthetic reason');assert.equal(writes,0);assert.equal(f.controller.state.snapshot.settings.values.responsibilities.billing.primary,member);assert.equal(f.controller.retryable,false);
});
test('settings history fits the fixed private transport budget with full Unicode and all designations',async()=>{
 const {CompanyApi}=await load('api');const csrf='abv_company_csrf_'+'a'.repeat(64);const calls=[];let count=20;
 const historyValues=values();let ref=10;for(const pair of Object.values(historyValues.responsibilities)){pair.primary=uuid(ref++);pair.substitute=uuid(ref++);}historyValues.policies.clock_skew_limit_seconds='9223372036854775807';historyValues.policies.require_invitation_confirmation=true;
 const item={...wire(),values:historyValues,source:'COMMAND',reason:'😀'.repeat(120),author:{account_id:account,membership_id:member},operation_id:uuid(40)};delete item.organization;delete item.setup;delete item.context;delete item.responsibility_members;
 const response=()=>({items:Array.from({length:count},(_,i)=>({...item,revision:count-i})),next_cursor:''});assert.ok(new TextEncoder().encode(JSON.stringify(response())).length<64*1024);
 const company=new CompanyApi(async(path,init)=>{calls.push({path,body:JSON.parse(init.body)});let value;
  if(path.endsWith('/session/read')&&!init.headers['X-CSRF'])return new Response('{"error":{"code":"UNAUTHENTICATED"}}',{status:401,headers:{'Content-Type':'application/json'}});
  if(path.endsWith('/keys/login'))value={account:{account_id:account},session:{family_id:uuid(99),access_token:'synthetic'}};
  else if(path.includes('/session/'))value={account_id:account,authenticated_at:new Date().toISOString(),expires_at:new Date(Date.now()+3600000).toISOString(),csrf_token:csrf};
  else value=response();
  return new Response(JSON.stringify(value),{status:200,headers:{'Content-Type':'application/json','Cache-Control':'no-store'}});
 });await company.signIn('synthetic');const api=new OrganizationSetupApi(company);
 const result=await api.history(org,'',new AbortController().signal);assert.equal(result.items.length,20);assert.equal(calls.at(-1).body.limit,20);assert.equal(result.items[0].values.policies.clock_skew_limit_seconds,'9223372036854775807');
 count=21;await assert.rejects(api.history(org,'',new AbortController().signal),e=>e.kind==='unavailable');
});
test('creation and reason bounds count Unicode scalars and source int64 values never round',async()=>{
 const f=fixture();await f.controller.open(account);let requests=0;f.source.create=async()=>{requests++;throw new ApiError('unavailable');};const at=draft();at.name='😀'.repeat(120);at.unit_name='é'.repeat(120);await f.controller.create(at);assert.equal(requests,1);
 const g=fixture();await g.controller.open(account);g.source.create=async()=>{requests++;return {};};await g.controller.create({...at,name:at.name+'x'});assert.equal(requests,1);
 for(const value of ['-1','-0','01','1.5','1e3','9223372036854775808']){const raw=wire();raw.values.policies.clock_skew_limit_seconds=value;const api=new OrganizationSetupApi({async privatePost(){return raw;}});await assert.rejects(api.read(account,org,new AbortController().signal));}
});
test('mutation revisions are distinct, bodies are immutable, conflicts never auto overwrite and denials clear private state',async()=>{
 const f=fixture();const initial=wire();initial.revision=2;initial.root_revision=9;initial.organization.revision=9;f.setSnapshot(initial);await f.controller.open(account,org);const bodies=[];
 f.source.change=async(_org,body)=>{bodies.push(structuredClone(body));throw new ApiError('unavailable');};const proposed=values();await f.controller.save(proposed,'Synthetic reason');proposed.time_zone='UTC';await f.controller.retry();assert.equal(bodies[0].revision,2);assert.deepEqual(bodies[0],bodies[1]);
 const g=fixture();g.setSnapshot(initial);await g.controller.open(account,org);let activation;g.source.activate=async(_org,body)=>{activation=body;throw new ApiError('conflict','ORG_SETUP_INCOMPLETE',['name']);};await g.controller.activate();assert.equal(activation.revision,9);assert.deepEqual(g.controller.state.fields,['name']);assert.equal(g.controller.canActivate,false);
 const h=fixture();await h.controller.open(account,org);h.source.change=async()=>{throw new ApiError('conflict','CONFLICT');};await h.controller.save(values(),'Synthetic reason');assert.equal(h.controller.state.snapshot,null);assert.equal(h.controller.retryable,false);
 h.source.read=async()=>{throw new ApiError('denied');};await h.controller.refresh();assert.equal(h.controller.state.snapshot,null);assert.deepEqual(h.controller.state.members,[]);assert.deepEqual(h.controller.state.history,[]);
});
test('directory and picker pages remain bound, ordered and explicit about unavailable selected references',async()=>{
 for(const raw of [{account_id:uuid(90),setups:[],next_after_org_id:''},{account_id:account,setups:[summary(),summary()],next_after_org_id:''},{account_id:account,setups:[summary()],next_after_org_id:uuid(1)}]){const api=new OrganizationSetupApi({async privatePost(){return raw;}});await assert.rejects(api.setups(account,'',new AbortController().signal));}
 const api=new OrganizationSetupApi({async privatePost(){return {account_id:account,organization_id:org,membership_id:uuid(90),members:[],next_after_membership_id:''};}});await assert.rejects(api.members(account,org,member,'',new AbortController().signal),e=>e.kind==='denied');
 const legacy=wire();legacy.organization.name='';legacy.setup.ready=false;legacy.setup.missing=['name'];legacy.responsibility_members[0].eligible=false;const read=new OrganizationSetupApi({async privatePost(){return legacy;}});const snapshot=await read.read(account,org,new AbortController().signal);assert.equal(snapshot.organization.name,'');assert.equal(snapshot.responsibility_members[0].eligible,false);assert.equal(snapshot.settings.values.responsibilities.billing.primary,member);
});
test('same-scope revalidation keeps the view while waiting and withdraws private state on current authority denial',async()=>{
 const f=fixture();await f.controller.open(account,org);const pending=deferred();f.source.read=()=>pending.promise;const recheck=f.controller.revalidate();await new Promise(setImmediate);assert.equal(f.controller.state.snapshot.organization.id,org);assert.equal(f.controller.state.phase,'ready');pending.reject(new ApiError('denied'));await recheck;assert.equal(f.controller.state.snapshot,null);assert.equal(f.controller.state.phase,'denied');
});
test('a denied authority probe invalidates an in-flight mutation before its late success',async()=>{
 const f=fixture();await f.controller.open(account,org);const late=deferred();let writeSignal;f.source.change=async(_o,_b,s)=>{writeSignal=s;return late.promise;};const writing=f.controller.save(values(),'Synthetic reason');await new Promise(setImmediate);
 f.source.read=async()=>{throw new ApiError('denied');};await f.controller.revalidate();assert.equal(writeSignal.aborted,true);assert.equal(f.controller.retryable,false);late.resolve({});await writing;assert.equal(f.controller.state.snapshot,null);assert.equal(f.controller.state.phase,'denied');
});
test('an allowed pending authority probe preserves the exact unresolved mutation',async()=>{
 const f=fixture();await f.controller.open(account,org);const bodies=[];f.source.change=async(_o,b)=>{bodies.push(structuredClone(b));if(bodies.length===1)throw new ApiError('unavailable');return {};};await f.controller.save(values(),'Synthetic reason');assert.equal(f.controller.retryable,true);await f.controller.revalidate();assert.equal(f.controller.retryable,true);assert.equal(bodies.length,1);await f.controller.retry();assert.equal(bodies.length,2);assert.deepEqual(bodies[0],bodies[1]);
});
test('actual incomplete-setup refusal replaces a previously complete visible checklist with server facts',async()=>{
 const f=fixture();await f.controller.open(account,org);assert.equal(f.controller.state.snapshot.setup.ready,true);f.source.activate=async()=>{throw new ApiError('conflict','ORG_SETUP_INCOMPLETE',['active_unit']);};await f.controller.activate();assert.equal(f.controller.state.snapshot.setup.ready,false);assert.deepEqual(f.controller.state.snapshot.setup.missing,['active_unit']);assert.equal(f.controller.canActivate,false);assert.equal(f.controller.canEdit,false);
});
test('unchanged background source checks preserve render count local errors and pending input focus state',async()=>{
 const f=fixture();await f.controller.open(account,org);let seen=f.states.length;await f.controller.revalidate();assert.equal(f.states.length,seen);
 await f.controller.save(values(),'');const message=f.controller.state.message;seen=f.states.length;await f.controller.revalidate();assert.equal(f.states.length,seen);assert.equal(f.controller.state.message,message);
});
