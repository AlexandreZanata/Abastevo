import test from 'node:test';
import assert from 'node:assert/strict';
import {compile,uuid,json,deferred} from './support.mjs';
const {load,cleanup}=compile();process.on('exit',cleanup);
const {MissionBoardApi,MissionBoardController,missionStateLabel}=await load('missions');
const {ApiError,CompanyApi}=await load('api');

// EMP-TRIP-022 (B-BR-EMP-470..474): the board over the actual parsers and an
// in-memory synthetic server that answers like the company routes.
const org={id:uuid(2),membershipId:uuid(3),name:'Synthetic organization'};
const authority={accountId:uuid(1),organization:org,permissions:['idn.employee.read'],invitationGrants:[],invitationValidityDays:7};
const other={id:uuid(4),membershipId:uuid(5),name:'Other synthetic organization'};
const otherAuthority={...authority,organization:other};
const mission=uuid(10),vehicle=uuid(20),spare=uuid(21),implement=uuid(22),draftAsset=uuid(23),unit=uuid(30);
const driver=uuid(40),second=uuid(41),stranger=uuid(49),centre=uuid(60);
const at='2026-11-16T10:00:00Z';
const wireMission=(over={})=>({mission_id:mission,unit_id:unit,asset_id:vehicle,title:'Entrega sintética',starts_at:'2026-11-17T11:00:00Z',ends_at:'2026-11-17T13:00:00Z',state:'DRAFT',revision:1,created_at:at,updated_at:at,data_mode:'SYNTHETIC',...over});
const wireAsset=(id,kind='VEHICLE',state='ACTIVE')=>({id,kind,name:'Synthetic '+kind.toLowerCase(),fleet_number:'FR-'+id.slice(-3),plate:'',serial_number:'',usage_basis:'KILOMETRES',category:{id:uuid(90),label:'Synthetic'},tanks:[],expected_consumption:[],state,revision:1,unit_id:unit,data_mode:'SYNTHETIC'});
function server(edit={}){
 const calls=[];let current=wireMission();
 const ack=(state,over={})=>({mission_id:current.mission_id,revision:current.revision,state,replayed:false,data_mode:'SYNTHETIC',...over});
 const advance=(state,over={})=>{current={...current,state,revision:current.revision+1,updated_at:'2026-11-16T10:05:00Z',...over};return ack(state,current.assignee_employee_id?{assignee_employee_id:current.assignee_employee_id}:{});};
 const routes={
  'missions/list':()=>({items:[current],next_cursor:'',data_mode:'SYNTHETIC'}),
  [`missions/${mission}/read`]:()=>({...current}),
  [`missions/${mission}/assignees/list`]:()=>({items:[{employee_id:driver,display_name:'Bruno Sintético',registration_number:'SYN-001'},{employee_id:second,display_name:'',registration_number:''}],next_cursor:'',data_mode:'SYNTHETIC'}),
  'fleet/assets/list':()=>({items:[wireAsset(vehicle),wireAsset(spare,'MACHINE'),wireAsset(implement,'IMPLEMENT'),wireAsset(draftAsset,'VEHICLE','DRAFT')],next_cursor:''}),
  'cost-centres/list':()=>({items:[{id:centre,name:'Contrato sintético',display_name:'Contrato sintético',code:'CC-1',state:'ACTIVE',revision:1,data_mode:'SYNTHETIC'}],next_cursor:''}),
  [`fleet/assets/${vehicle}/availability/read`]:()=>({asset_id:vehicle,availability:'RESERVED',causes:[{availability:'RESERVED',kind:'RESERVATION',record_id:uuid(70),valid_from:'2026-11-17T10:00:00Z',valid_to:'2026-11-17T12:00:00Z'}],missing_sources:['MAINTENANCE_HOLD'],evaluated_at:at}),
  [`fleet/assets/${vehicle}/reservations/list`]:()=>({items:[{reservation_id:uuid(70),asset_id:vehicle,purpose:'PRIVATE-PURPOSE',starts_at:'2026-11-17T10:00:00Z',ends_at:'2026-11-17T12:00:00Z',no_show_tolerance_minutes:30,state:'CONFIRMED',requested_at:at,employee_id:uuid(71)}],next_cursor:'',evaluated_at:at}),
  [`fleet/assets/${vehicle}/blocks/list`]:()=>({items:[{block_id:uuid(80),asset_id:vehicle,reason_kind:'SAFETY',valid_from:'2026-11-18T00:00:00Z',recorded_at:at,note:'PRIVATE-NOTE'}],next_cursor:'',evaluated_at:at}),
  'missions/create':()=>{current=wireMission();return ack('DRAFT');},
  [`missions/${mission}/plan`]:()=>{const v=advance('PLANNED',{assignee_employee_id:undefined,answer:undefined});current.plan={revision:current.revision,destination:{kind:'TEXT',text:'Pátio sintético'},reason:'Entrega',load:{kind:'NONE'},planned_at:at};return v;},
  [`missions/${mission}/assign`]:(body)=>{const v=advance('ASSIGNED',{assignee_employee_id:body.assignee_employee_id});v.asset_warning={code:'ASSET_UNAVAILABLE',availability:'RESERVED'};return v;},
  [`missions/${mission}/vehicle/change`]:(body)=>advance(current.state,{asset_id:body.asset_id}),
  [`missions/${mission}/cancel`]:()=>advance('CANCELLED'),
  ...edit,
 };
 const transport={async privatePost(path,body,signal){
  calls.push({path,body:structuredClone(body)});
  const prefix=`/v1/orgs/${org.id}/`;
  if(!path.startsWith(prefix))throw new ApiError('denied');
  const handler=routes[path.slice(prefix.length)];
  if(!handler)throw new ApiError('denied');
  if(signal?.aborted)throw new ApiError('cancelled');
  return structuredClone(await handler(body,{set:v=>{current=v;},get:()=>current}));
 }};
 return {calls,transport,set:v=>{current=v;},get:()=>current,routes};
}
function board(edit,sessionLost){
 const s=server(edit);const renders=[];
 const c=new MissionBoardController(new MissionBoardApi(s.transport),v=>renders.push(v),sessionLost,()=>new Date('2026-11-16T10:10:00Z'));
 return {...s,c,renders,writes:()=>s.calls.filter(v=>/\/(?:create|plan|assign|change|cancel)$/.test(v.path))};
}
async function opened(edit,sessionLost){const b=board(edit,sessionLost);b.c.open(authority);await b.c.load();await b.c.select(mission);return b;}

test('the board shows only server-read missions with their read time and offers only eligible scoped vehicles and assignees',async()=>{
 const b=board();b.c.open(authority);assert.equal(b.c.state.phase,'idle');assert.equal(b.calls.length,0,'opening the board sends nothing until the member asks');
 await b.c.load();
 assert.equal(b.c.state.phase,'ready');assert.equal(b.c.state.missions.length,1);assert.equal(b.c.state.missions[0].revision,1);assert.ok(b.c.state.listedAt);
 assert.deepEqual(b.c.state.assets.map(a=>a.id),[vehicle,spare],'only ACTIVE non-implement vehicles are offered');
 assert.deepEqual(b.c.state.costCentres.map(c=>c.id),[centre]);
 await b.c.select(mission);
 assert.equal(b.c.state.selected.id,mission);assert.equal(b.c.state.readAt,'2026-11-16T10:10:00.000Z');
 assert.deepEqual(b.c.state.assignees.map(a=>a.id),[driver,second],'assignees are exactly the server eligible list');
 assert.equal(b.c.state.agenda.assetId,vehicle);assert.equal(b.c.state.agenda.availability.availability,'RESERVED');assert.equal(b.c.state.agenda.availability.evaluatedAt,at);
 assert.equal(b.c.state.agenda.reservations[0].state,'CONFIRMED');assert.equal(b.c.state.agenda.blocks[0].reasonKind,'SAFETY');
 const text=JSON.stringify(b.c.state);
 for(const hidden of ['PRIVATE-PURPOSE','PRIVATE-NOTE',uuid(71)])assert.equal(text.includes(hidden),false,'the agenda keeps no free text or reservation person');
 assert.ok(b.calls.every(c=>c.path.startsWith(`/v1/orgs/${org.id}/`)));
});

test('foreign or stale choices are refused before transport and never shown',async()=>{
 const b=await opened();
 await b.c.assign(stranger);assert.equal(b.writes().length,0,'an employee outside the eligible list is never sent');assert.match(b.c.state.message,/lista de pessoas elegíveis/);
 await b.c.changeVehicle(implement,'Troca');await b.c.changeVehicle(draftAsset,'Troca');await b.c.changeVehicle(vehicle,'Troca');
 await b.c.create({title:'Outra',startsAt:'2026-11-18T08:00:00.000Z',endsAt:'2026-11-18T09:00:00.000Z',assetId:implement});
 await b.c.plan({destination:'Pátio',reason:'Entrega',costCentreId:uuid(61),load:'NONE',cargo:'',passengers:0});
 assert.equal(b.writes().length,0,'implements, drafts, the current vehicle and unknown cost centres are never sent');
 // A mission read answering another mission is unusable and not shown.
 const c=board({[`missions/${mission}/read`]:()=>wireMission({mission_id:uuid(11)})});c.c.open(authority);await c.c.load();await c.c.select(mission);
 assert.equal(c.c.state.selected,null);assert.notEqual(c.c.state.phase,'ready');
 // A late answer of the previous organization never renders in the new one.
 const late=deferred();const d=board({'missions/list':()=>late.promise});d.c.open(authority);const pending=d.c.load();d.c.open(otherAuthority);
 late.resolve({items:[wireMission()],next_cursor:'',data_mode:'SYNTHETIC'});await pending;
 assert.equal(d.c.state.authority.organization.id,other.id);assert.deepEqual(d.c.state.missions,[]);assert.equal(d.c.state.phase,'idle');
});

test('a concurrent update surfaces as CONFLICT and refreshes the current server state without retrying',async()=>{
 let rival;const b=await opened({[`missions/${mission}/assign`]:(_body,s)=>{const now=s.get();rival={...now,state:'ASSIGNED',revision:now.revision+1,assignee_employee_id:second};s.set(rival);throw new ApiError('conflict','CONFLICT');}});
 const before=b.calls.length;await b.c.assign(driver);
 assert.equal(b.writes().length,1);assert.equal(b.c.state.conflict,true);assert.match(b.c.state.message,/mudou/);
 assert.equal(b.c.state.selected.revision,2,'the newer server revision is shown');assert.equal(b.c.state.selected.assigneeId,second);
 assert.ok(b.calls.slice(before).some(c=>c.path.endsWith(`/missions/${mission}/read`)),'the mission is read again');
 assert.equal(b.c.retryable,false);await b.c.retry();assert.equal(b.writes().length,1,'a conflicting write is never retried');
 assert.equal(b.c.state.missions.find(m=>m.id===mission).revision,2,'the list row is the refreshed server state');
});

test('an unconfirmed state is never presented as accepted',async()=>{
 const gate=deferred();const b=await opened({[`missions/${mission}/assign`]:async(body,s)=>{await gate.promise;const now=s.get();s.set({...now,state:'ASSIGNED',revision:now.revision+1,assignee_employee_id:body.assignee_employee_id});return {mission_id:mission,revision:now.revision+1,state:'ASSIGNED',assignee_employee_id:body.assignee_employee_id,replayed:false,data_mode:'SYNTHETIC',asset_warning:{code:'ASSET_UNAVAILABLE',availability:'RESERVED'}};}});
 const pending=b.c.assign(driver);await new Promise(setImmediate);
 assert.equal(b.c.state.selected.state,'DRAFT','a pending command never changes the shown mission');assert.equal(b.c.state.selected.revision,1);
 gate.resolve();await pending;
 assert.equal(b.c.state.selected.state,'ASSIGNED');assert.equal(b.c.state.selected.assigneeId,driver);
 assert.doesNotMatch(missionStateLabel(b.c.state.selected),/Aceita/);assert.match(missionStateLabel(b.c.state.selected),/aguardando resposta/);
 assert.deepEqual(b.c.state.warning,{code:'ASSET_UNAVAILABLE',availability:'RESERVED'},'the FLEET-007 warning is shown as returned');
 // An acknowledgement claiming acceptance (or another employee) for an assignment is unusable.
 for(const claim of [{state:'ACCEPTED',assignee_employee_id:driver},{state:'ASSIGNED',assignee_employee_id:second}]){
  const c=await opened({[`missions/${mission}/assign`]:()=>({mission_id:mission,revision:2,replayed:false,data_mode:'SYNTHETIC',...claim})});
  await c.c.assign(driver);assert.equal(c.c.state.selected.state,'DRAFT');assert.notEqual(c.c.state.phase,'ready');assert.equal(c.c.retryable,true);
 }
 // A lost acknowledgement keeps the last server state and retries the identical command.
 let first=true;const d=await opened({[`missions/${mission}/assign`]:(body,s)=>{if(first){first=false;throw new ApiError('unavailable');}const now=s.get();s.set({...now,state:'ASSIGNED',revision:now.revision+1,assignee_employee_id:body.assignee_employee_id});return {mission_id:mission,revision:now.revision+1,state:'ASSIGNED',assignee_employee_id:body.assignee_employee_id,replayed:false,data_mode:'SYNTHETIC'};}});
 await d.c.assign(driver);assert.equal(d.c.state.selected.state,'DRAFT');assert.equal(d.c.retryable,true);
 await d.c.assign(second);assert.equal(d.writes().length,1,'no new decision while an outcome is unknown');
 await d.c.retry();assert.equal(d.writes().length,2);assert.deepEqual(d.writes()[0].body,d.writes()[1].body);assert.equal(d.c.state.selected.state,'ASSIGNED');assert.equal(d.c.retryable,false);
 // Only a server read says ACCEPTED; a read claiming it without the answer is refused.
 const e=await opened();e.set(wireMission({state:'ACCEPTED',revision:3,assignee_employee_id:driver,answer:{response:'ACCEPT'}}));await e.c.reload();
 assert.equal(e.c.state.selected.state,'ACCEPTED');assert.match(missionStateLabel(e.c.state.selected),/Aceita/);
 e.set(wireMission({state:'ACCEPTED',revision:4,assignee_employee_id:driver}));await e.c.reload();assert.notEqual(e.c.state.selected?.revision,4,'an ACCEPTED read without the answer is unusable');
});

test('no write without the permission: the server 403 clears the board and reveals nothing',async()=>{
 const csrf='abv_company_csrf_'+'a'.repeat(64);const now=Date.parse(at);
 const view={account_id:uuid(1),authenticated_at:at,expires_at:new Date(now+8*3600_000).toISOString()};
 const refusals=[];
 function api(write){
  return new CompanyApi(async(path,init)=>{
   if(path.endsWith('/session/read')&&init.headers['X-CSRF']===undefined)return json({error:{code:'UNAUTHENTICATED'}},401);
   if(path.endsWith('/keys/login'))return json({account:{account_id:uuid(1)},session:{family_id:uuid(9),access_token:'synthetic'}});
   if(path.endsWith('/session/login'))return json({...view,csrf_token:csrf});
   const tail=path.slice(`/v1/orgs/${org.id}/`.length);const s=server();
   if(write(tail)){refusals.push(tail);return json({error:{code:'FORBIDDEN',message:'Organization request unavailable or refused',details:[{field:'permission',code:'trip.mission.manage'}],trace_id:''}},403);}
   return json(await s.routes[tail](JSON.parse(init.body),{set(){},get:()=>wireMission()}));
  },()=>now);
 }
 for(const [name,write]of [['command',t=>/\/(?:assign|cancel)$/.test(t)],['list',t=>t==='missions/list']]){
  const transport=api(write);await transport.signIn('synthetic-key');
  const c=new MissionBoardController(new MissionBoardApi(transport),()=>{},undefined,()=>new Date(at));c.open(authority);await c.load();
  if(name==='command'){await c.select(mission);assert.equal(c.state.selected.id,mission);await c.cancel('Cliente cancelou');}
  assert.equal(c.state.phase,'denied',name);assert.deepEqual(c.state.missions,[]);assert.equal(c.state.selected,null);assert.equal(c.state.assets,null);assert.deepEqual(c.state.assignees,[]);assert.equal(c.state.agenda,null);
  const text=JSON.stringify(c.state);for(const hidden of [mission,vehicle,driver,'Entrega sintética'])assert.equal(text.includes(hidden),false,name+' revealed '+hidden);
  assert.match(c.state.message,/não pôde ser confirmado/);
 }
 assert.deepEqual(refusals,[`missions/${mission}/cancel`,'missions/list'],'the server refused exactly the attempted command and the list');
 // NOT_FOUND hides the same way; FRESH_AUTH_REQUIRED and 401 end the session view.
 const b=await opened({[`missions/${mission}/cancel`]:()=>{throw new ApiError('denied','NOT_FOUND');}});await b.c.cancel('Cliente cancelou');assert.equal(b.c.state.phase,'denied');assert.equal(b.c.state.selected,null);
 let lost='';const f=await opened({[`missions/${mission}/cancel`]:()=>{throw new ApiError('fresh');}},m=>{lost=m;});await f.c.cancel('Cliente cancelou');assert.equal(f.c.state.phase,'closed');assert.match(lost,/Entre novamente/);
});

test('commands send exact schema-1 bodies with the seen revision and a new operation per decision',async()=>{
 const b=await opened();
 await b.c.plan({destination:'Pátio sintético',reason:'Entrega',costCentreId:'',load:'NONE',cargo:'',passengers:0});
 await b.c.plan({destination:'Pátio sintético',reason:'Entrega',costCentreId:centre,load:'CARGO',cargo:'Caixas sintéticas',passengers:0});
 await b.c.plan({destination:'Pátio sintético',reason:'Entrega',costCentreId:'',load:'PASSENGERS',cargo:'',passengers:3});
 await b.c.assign(driver);await b.c.changeVehicle(spare,'Veículo em manutenção');await b.c.cancel('Cliente cancelou');
 await b.c.create({title:'Nova entrega',startsAt:'2026-11-18T08:00:00.000Z',endsAt:'2026-11-18T10:00:00.000Z',assetId:vehicle});
 const [plain,cargo,passengers,assign,change,cancel,create]=b.writes().map(v=>v.body);
 const id=v=>{assert.match(v.operation_id,/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/);const {operation_id,...rest}=v;return rest;};
 assert.deepEqual(id(plain),{schema_version:1,revision:1,destination:{kind:'TEXT',text:'Pátio sintético'},reason:'Entrega',load:{kind:'NONE'}});
 assert.deepEqual(id(cargo),{schema_version:1,revision:2,destination:{kind:'TEXT',text:'Pátio sintético'},reason:'Entrega',cost_centre_id:centre,load:{kind:'CARGO',description:'Caixas sintéticas'}});
 assert.deepEqual(id(passengers),{schema_version:1,revision:3,destination:{kind:'TEXT',text:'Pátio sintético'},reason:'Entrega',load:{kind:'PASSENGERS',count:3}});
 assert.deepEqual(id(assign),{schema_version:1,revision:4,assignee_employee_id:driver});
 assert.deepEqual(id(change),{schema_version:1,revision:5,asset_id:spare,reason:'Veículo em manutenção'});
 assert.deepEqual(id(cancel),{schema_version:1,revision:6,reason:'Cliente cancelou'});
 assert.deepEqual(id(create),{schema_version:1,unit_id:unit,asset_id:vehicle,title:'Nova entrega',starts_at:'2026-11-18T08:00:00.000Z',ends_at:'2026-11-18T10:00:00.000Z'});
 assert.equal(new Set(b.writes().map(v=>v.body.operation_id)).size,7);
 assert.deepEqual(b.writes().map(v=>v.path.slice(`/v1/orgs/${org.id}/missions/`.length)),[`${mission}/plan`,`${mission}/plan`,`${mission}/plan`,`${mission}/assign`,`${mission}/vehicle/change`,`${mission}/cancel`,'create']);
 assert.equal(b.c.state.selected.state,'DRAFT','after creation the board reads the created mission');
});

test('a refused optional read disables only its picker; invalid input keeps the draft and is not retried',async()=>{
 const b=board({'cost-centres/list':()=>{throw new ApiError('denied','FORBIDDEN');}});b.c.open(authority);await b.c.load();
 assert.equal(b.c.state.phase,'ready');assert.equal(b.c.state.costCentres,null);assert.equal(b.c.state.missions.length,1);
 await b.c.select(mission);await b.c.plan({destination:'Pátio',reason:'Entrega',costCentreId:centre,load:'NONE',cargo:'',passengers:0});assert.equal(b.writes().length,0);
 const c=await opened({[`missions/${mission}/plan`]:()=>{throw new ApiError('invalid','VALIDATION_FAILED');}});
 await c.c.plan({destination:'123.456.789-09',reason:'Entrega',costCentreId:'',load:'NONE',cargo:'',passengers:0});
 assert.equal(c.c.state.phase,'failed');assert.match(c.c.state.message,/Confira/);assert.equal(c.c.retryable,false);assert.equal(c.c.state.selected.revision,1);
});

test('parsers refuse malformed server data and the transport admits only the board routes',async()=>{
 for(const bad of [wireMission({state:'APPROVED'}),wireMission({revision:0}),wireMission({assignee_employee_id:'not-a-uuid'}),wireMission({state:'DECLINED',assignee_employee_id:driver}),wireMission({answer:{response:'ACCEPT'}})]){
  const api=new MissionBoardApi({async privatePost(){return bad;}});await assert.rejects(api.mission(org.id,mission,new AbortController().signal),e=>e instanceof ApiError);
 }
 const api=new MissionBoardApi({async privatePost(){return {asset_id:spare,availability:'AVAILABLE',causes:[],missing_sources:[],evaluated_at:at};}});
 await assert.rejects(api.availability(org.id,vehicle,new AbortController().signal),e=>e instanceof ApiError,'another asset answer is refused');
 const names=new MissionBoardApi({async privatePost(){return {items:[{employee_id:driver,display_name:'x'.repeat(201),registration_number:''}],next_cursor:''};}});
 await assert.rejects(names.assignees(org.id,mission,'',new AbortController().signal));
 const csrf='abv_company_csrf_'+'a'.repeat(64);const seen=[];
 const transport=new CompanyApi(async(path,init)=>{if(path.endsWith('/session/read')&&init.headers['X-CSRF']===undefined)return json({},401);if(path.endsWith('/keys/login'))return json({account:{account_id:uuid(1)},session:{family_id:uuid(9),access_token:'synthetic'}});if(path.endsWith('/session/login'))return json({account_id:uuid(1),authenticated_at:new Date().toISOString(),expires_at:new Date(Date.now()+3600_000).toISOString(),csrf_token:csrf});seen.push(path);return json({});});
 await transport.signIn('synthetic-key');
 const base=`/v1/orgs/${org.id}`;
 for(const path of [`${base}/missions/list`,`${base}/missions/create`,`${base}/missions/${mission}/read`,`${base}/missions/${mission}/plan`,`${base}/missions/${mission}/assign`,`${base}/missions/${mission}/cancel`,`${base}/missions/${mission}/vehicle/change`,`${base}/missions/${mission}/assignees/list`,`${base}/fleet/assets/list`,`${base}/fleet/assets/${vehicle}/availability/read`,`${base}/fleet/assets/${vehicle}/reservations/list`,`${base}/fleet/assets/${vehicle}/blocks/list`,`${base}/cost-centres/list`])await transport.privatePost(path,{});
 const allowed=seen.length;
 for(const path of [`${base}/missions/${mission}/delete`,`${base}/missions/mine/list`,`${base}/fleet/assets/${vehicle}/reservations/request`,`${base}/fleet/assets/${vehicle}/blocks/create`,`${base}/units/list`,`${base}/missions/../employees/list`])await assert.rejects(transport.privatePost(path,{}),e=>e.kind==='denied');
 assert.equal(seen.length,allowed);assert.equal(allowed,13);
});
