import test from 'node:test';
import assert from 'node:assert/strict';
import {compile,uuid,json,deferred} from './support.mjs';
const {load,cleanup}=compile();process.on('exit',cleanup);
const {CheckPanelApi,CheckPanelController,runOutcomeLabel,reportStateLabel}=await load('checks');
const {ApiError,CompanyApi}=await load('api');

// EMP-CHECK-004 (B-BR-EMP-506..510): the checklist and incident panel over
// the actual parsers and an in-memory synthetic server answering like the
// company routes (runs queue, run read and release, incident queue, read and
// decide, asset list and availability, audited document content).
const org={id:uuid(2),membershipId:uuid(3),name:'Synthetic organization'};
const authority={accountId:uuid(1),organization:org,permissions:['idn.employee.read'],invitationGrants:[],invitationValidityDays:7};
const withContent={...authority,permissions:['idn.employee.read','doc.document.view_content']};
const other={id:uuid(4),membershipId:uuid(5),name:'Other synthetic organization'};
const otherAuthority={...authority,organization:other};
const vehicle=uuid(20),spare=uuid(21);
const blocking=uuid(30),review=uuid(31),plain=uuid(32);
const damage=uuid(40),closed=uuid(41),dup=uuid(42),elsewhere=uuid(43),stranger=uuid(49);
const stored=uuid(50),uploading=uuid(51);
const reporter=uuid(70),handler=uuid(71);
const at='2026-11-16T10:00:00Z';
const wireRun=(id,over={})=>({run_id:id,asset_id:vehicle,template_id:uuid(60),context:'DEPARTURE',template_version:1,policy_revision:1,server_policy_revision:1,revision:1,
 started_at:'2026-11-16T09:40:00Z',completed_at:'2026-11-16T09:50:00.123Z',received_at:'2026-11-16T09:51:00Z',device_outcome:'BLOCKING',recheck_outcome:'BLOCKING',outcome:'BLOCKING',
 outcome_differs:false,review_required:false,template_superseded:false,acknowledged:false,issues:[2],critical_issues:[2],data_mode:'SYNTHETIC',...over});
const reviewRun=()=>wireRun(review,{device_outcome:'COMPLETED_OK',recheck_outcome:'COMPLETED_WITH_ISSUES',outcome:'COMPLETED_WITH_ISSUES',outcome_differs:true,review_required:true,issues:[3],critical_issues:[]});
const plainRun=()=>wireRun(plain,{asset_id:spare,device_outcome:'COMPLETED_OK',recheck_outcome:'COMPLETED_OK',outcome:'COMPLETED_OK',issues:[],critical_issues:[]});
const released=over=>wireRun(blocking,{revision:2,outcome:'COMPLETED_WITH_ISSUES',released_at:'2026-11-16T10:05:00Z',...over});
const wireSummary=(id,over={})=>({report_id:id,report_type:'DAMAGE',asset_id:vehicle,reporter_membership_id:reporter,blocks_use:true,state:'REPORTED',revision:1,occurred_at:at,received_at:at,updated_at:at,...over});
const wireReport=(id,over={})=>({...wireSummary(id),description:'Lanterna traseira trincada ao manobrar',location:'Doca 2',history:[],
 attachments:[{document_id:stored,storage_state:'STORED',linked_at:at},{document_id:uploading,storage_state:'UPLOADING',linked_at:at}],data_mode:'SYNTHETIC',...over});
const blocked=()=>({asset_id:vehicle,availability:'BLOCKED',causes:[{availability:'BLOCKED',kind:'CHECKLIST_RUN',record_id:blocking,valid_from:'2026-11-16T09:50:00.123Z'}],missing_sources:[],evaluated_at:at});
const free=()=>({asset_id:vehicle,availability:'AVAILABLE',causes:[],missing_sources:[],evaluated_at:'2026-11-16T10:06:00Z'});
const steps={ACKNOWLEDGE:'ACKNOWLEDGED',TREAT:'IN_TREATMENT',RESOLVE:'RESOLVED',DISMISS:'DISMISSED'};
function server(edit={}){
 const calls=[];
 const runs=new Map([[blocking,wireRun(blocking)],[review,reviewRun()],[plain,plainRun()]]);
 const reports=new Map([[closed,wireReport(closed,{state:'RESOLVED',revision:2,blocks_use:false})],[damage,wireReport(damage)],[dup,wireReport(dup,{blocks_use:false,attachments:[]})],[elsewhere,wireReport(elsewhere,{asset_id:spare,blocks_use:false,attachments:[]})]]);
 let availability=blocked();
 const summary=r=>{const {description:_d,location:_l,history:_h,attachments:_a,data_mode:_m,...rest}=r;return rest;};
 const routes={
  'checklists/runs/list':()=>({items:[runs.get(blocking),runs.get(review),runs.get(plain)],next_cursor:'',data_mode:'SYNTHETIC'}),
  'incidents/reports/list':()=>({items:[...reports.values()].map(summary),next_cursor:'',data_mode:'SYNTHETIC'}),
  'fleet/assets/list':()=>({items:[{id:vehicle,kind:'VEHICLE',name:'Caminhão sintético',fleet_number:'FR-001',plate:'',state:'ACTIVE',unit_id:uuid(80),revision:1},{id:spare,kind:'MACHINE',name:'Pá carregadeira sintética',fleet_number:'FR-002',plate:'',state:'INACTIVE',unit_id:'',revision:1}],next_cursor:''}),
  [`fleet/assets/${vehicle}/availability/read`]:()=>availability,
  [`fleet/assets/${spare}/availability/read`]:()=>({...free(),asset_id:spare}),
  [`checklists/runs/${blocking}/release`]:body=>{const now=runs.get(blocking);if(now.revision!==body.revision)throw new ApiError('conflict','CONFLICT');const next=released();runs.set(blocking,next);availability=free();return {...next,replayed:false};},
  [`documents/${stored}/content/access`]:()=>({document_id:stored,capability:'cap-synthetic-PRIVATE',expires_at:'2026-11-16T10:05:00Z'}),
  [`documents/${stored}/content/read`]:body=>{if(body.capability!=='cap-synthetic-PRIVATE')throw new ApiError('denied','NOT_FOUND');return {document_id:stored,data_mode:'SYNTHETIC',media_type:'image/jpeg',bytes:4,content_base64:'/9j/2Q=='};},
 };
 for(const id of [blocking,review,plain])routes[`checklists/assets/${id===plain?spare:vehicle}/runs/${id}/read`]=()=>runs.get(id);
 for(const id of reports.keys()){
  routes[`incidents/reports/${id}/read`]=()=>reports.get(id);
  routes[`incidents/reports/${id}/decide`]=body=>{const now=reports.get(id);if(now.revision!==body.revision)throw new ApiError('conflict','CONFLICT');const step={revision:now.revision+1,action:body.action,from_state:now.state,to_state:steps[body.action],actor_membership_id:handler,recorded_at:'2026-11-16T10:07:00Z',...(body.note?{note:body.note}:{}),...(body.duplicate_of?{duplicate_of:body.duplicate_of}:{})};
   reports.set(id,{...now,state:step.to_state,revision:step.revision,updated_at:step.recorded_at,history:[...now.history,step]});return {report_id:id,action:body.action,state:step.to_state,revision:step.revision,recorded_at:step.recorded_at,replayed:false};};
 }
 Object.assign(routes,edit);
 const handle=async(path,body,signal)=>{
  calls.push({path,body:structuredClone(body)});
  const prefix=`/v1/orgs/${org.id}/`;
  if(!path.startsWith(prefix))throw new ApiError('denied');
  const route=routes[path.slice(prefix.length)];
  if(!route)throw new ApiError('denied','NOT_FOUND');
  if(signal?.aborted)throw new ApiError('cancelled');
  return structuredClone(await route(body,{runs,reports,setAvailability:v=>{availability=v;}}));
 };
 return {calls,runs,reports,transport:{privatePost:handle,privateContent:handle},setAvailability:v=>{availability=v;}};
}
function panel(edit,sessionLost){
 const s=server(edit);const renders=[];
 const c=new CheckPanelController(new CheckPanelApi(s.transport),v=>renders.push(v),sessionLost,()=>new Date('2026-11-16T10:10:00Z'));
 const writes=()=>s.calls.filter(v=>/\/(?:release|decide)$/.test(v.path));
 const content=()=>s.calls.filter(v=>v.path.includes('/documents/'));
 return {...s,c,renders,writes,content};
}
async function loaded(edit,sessionLost,who=authority){const p=panel(edit,sessionLost);p.c.open(who);await p.c.load();return p;}

test('the panel shows server-read runs attention-first and reports open-first, with their read instants and labels',async()=>{
 const p=panel();p.c.open(authority);assert.equal(p.c.state.phase,'idle');assert.equal(p.calls.length,0,'opening the panel sends nothing until the member asks');
 await p.c.load();
 assert.equal(p.c.state.phase,'ready');
 assert.deepEqual(p.c.state.runs.map(r=>r.id),[blocking,review,plain],'runs keep the server queue order');
 assert.equal(p.c.state.runs[0].outcome,'BLOCKING');assert.equal(p.c.state.runs[1].differs,true);assert.deepEqual(p.c.state.runs[0].critical,[2]);
 assert.deepEqual(p.c.state.reports.map(r=>r.id),[damage,dup,elsewhere,closed],'open reports first, blocking marks first, closed last');
 assert.equal(p.c.state.runsAt,'2026-11-16T10:10:00.000Z');assert.equal(p.c.state.reportsAt,'2026-11-16T10:10:00.000Z');
 assert.deepEqual(p.c.state.assets.map(a=>a.id),[vehicle,spare],'asset labels of any state');
 assert.match(runOutcomeLabel('BLOCKING'),/Bloqueando/);assert.match(reportStateLabel('IN_TREATMENT'),/tratamento/);
 assert.equal(p.c.state.run,null);assert.equal(p.c.state.availability,null,'no availability is shown before a record is opened');
 assert.ok(p.calls.every(c=>c.path.startsWith(`/v1/orgs/${org.id}/`)));
 assert.equal(p.c.state.contentAllowed,false,'no content action without the hint');
});

test('a self-release surfaces SELF_APPROVAL, changes nothing and keeps the block visible',async()=>{
 const p=await loaded({[`checklists/runs/${blocking}/release`]:()=>{throw new ApiError('conflict','SELF_APPROVAL');}});
 await p.c.openRun(blocking);
 assert.equal(p.c.state.run.id,blocking);assert.equal(p.c.state.availability.availability,'BLOCKED');
 assert.equal(p.c.state.availability.causes[0].kind,'CHECKLIST_RUN');assert.equal(p.c.state.availability.causes[0].recordId,blocking,'the block names this run');
 await p.c.release('Freios verificados na oficina');
 assert.equal(p.writes().length,1);assert.equal(p.c.state.selfApproval,true);assert.match(p.c.state.message,/outro membro/);
 assert.equal(p.c.state.run.outcome,'BLOCKING');assert.equal(p.c.state.run.revision,1);assert.equal(p.c.state.availability.availability,'BLOCKED','the block stays visible');
 assert.equal(p.c.retryable,false);await p.c.retry();assert.equal(p.writes().length,1,'a self-release is never retried');
 assert.equal(p.c.pendingWrite,false);assert.notEqual(p.c.state.phase,'denied','SELF_APPROVAL is not a lost authority');
 // The same holds for closing an own blocking report.
 const r=await loaded({[`incidents/reports/${damage}/decide`]:()=>{throw new ApiError('conflict','SELF_APPROVAL');}});
 await r.c.openReport(damage);await r.c.decide('RESOLVE','Lanterna trocada','');
 assert.equal(r.c.state.selfApproval,true);assert.match(r.c.state.message,/outro membro/);assert.equal(r.c.state.report.state,'REPORTED');
});

test('the block is never hidden: availability is only the server read, again after a release, and unconfirmed when it fails',async()=>{
 const p=await loaded();
 await p.c.openRun(blocking);assert.equal(p.c.state.availability.availability,'BLOCKED');
 const before=p.calls.length;
 // The server still reports another block after the release: it stays visible.
 const q=await loaded({[`checklists/runs/${blocking}/release`]:(body,x)=>{const next=released();x.runs.set(blocking,next);x.setAvailability({...blocked(),causes:[{availability:'BLOCKED',kind:'BLOCK',record_id:uuid(90),valid_from:at,reason_kind:'SAFETY'}]});return {...next,replayed:false};}});
 await q.c.openRun(blocking);await q.c.release('Freios verificados na oficina');
 assert.equal(q.c.state.run.revision,2,'the run is read again');assert.equal(q.c.state.run.outcome,'COMPLETED_WITH_ISSUES');assert.equal(q.c.state.run.recheck,'BLOCKING','history keeps the server outcome');
 assert.equal(q.c.state.run.device,'BLOCKING');assert.deepEqual(q.c.state.run.issues,[2]);assert.ok(q.c.state.run.releasedAt);
 assert.equal(q.c.state.availability.availability,'BLOCKED','a remaining block from the server is shown');assert.equal(q.c.state.availability.causes[0].kind,'BLOCK');
 assert.equal(q.c.state.runs.find(r=>r.id===blocking).revision,2,'the list row is the refreshed server state');
 // An ordinary release: the server answers AVAILABLE, read after the ack.
 await p.c.release('Freios verificados na oficina');
 assert.equal(p.c.state.run.revision,2);assert.equal(p.c.state.availability.availability,'AVAILABLE');assert.equal(p.c.state.availability.evaluatedAt,'2026-11-16T10:06:00Z');
 const reads=p.calls.slice(before).map(c=>c.path.slice(`/v1/orgs/${org.id}/`.length));
 assert.deepEqual(reads,[`checklists/runs/${blocking}/release`,`checklists/assets/${vehicle}/runs/${blocking}/read`,`fleet/assets/${vehicle}/availability/read`]);
 // A failed availability read is "not confirmed", never available.
 const f=await loaded({[`fleet/assets/${vehicle}/availability/read`]:()=>{throw new ApiError('unavailable');}});
 await f.c.openRun(blocking);
 assert.equal(f.c.state.run.id,blocking);assert.equal(f.c.state.availability,null);assert.match(f.c.state.message,/não pôde ser confirmada/);
 assert.equal(JSON.stringify(f.c.state).includes('AVAILABLE'),false);
 // A run the server says is released at revision 1 (or BLOCKING at revision 2) is unusable.
 for(const bad of [wireRun(blocking,{released_at:at}),wireRun(blocking,{revision:2}),wireRun(blocking,{outcome_differs:true,review_required:true}),wireRun(blocking,{review_required:true}),wireRun(blocking,{issues:[2,2]})]){
  const api=new CheckPanelApi({async privatePost(){return bad;},async privateContent(){throw new Error('no');}});
  await assert.rejects(api.run(org.id,vehicle,blocking,new AbortController().signal),e=>e instanceof ApiError);
 }
});

test('document content is offered only with the content hint and a STORED attachment, through the audited path, and never kept',async()=>{
 const p=await loaded();
 await p.c.openReport(damage);
 assert.equal(p.c.state.report.attachments.length,2);
 assert.equal(p.c.canOpenContent(stored),false,'without doc.document.view_content no content action exists');
 assert.equal(await p.c.content(stored),null);assert.equal(p.content().length,0,'nothing is requested without the permission');
 const q=await loaded(undefined,undefined,withContent);
 await q.c.openReport(damage);
 assert.equal(q.c.state.contentAllowed,true);assert.equal(q.c.canOpenContent(stored),true);assert.equal(q.c.canOpenContent(uploading),false,'only a STORED attachment');
 assert.equal(await q.c.content(uploading),null);assert.equal(await q.c.content(stranger),null);assert.equal(q.content().length,0,'no free or unready document is requested');
 const file=await q.c.content(stored);
 assert.equal(file.mediaType,'image/jpeg');assert.deepEqual([...file.bytes],[0xff,0xd8,0xff,0xd9]);
 assert.deepEqual(q.content().map(c=>c.path.slice(`/v1/orgs/${org.id}/`.length)),[`documents/${stored}/content/access`,`documents/${stored}/content/read`]);
 assert.deepEqual(q.content()[0].body,{});assert.deepEqual(q.content()[1].body,{capability:'cap-synthetic-PRIVATE'});
 const text=JSON.stringify(q.c.state);for(const hidden of ['cap-synthetic-PRIVATE','/9j/2Q==','255,216'])assert.equal(text.includes(hidden),false,'the panel keeps no capability or content');
 // A 403 on content hides the action and keeps the report (BR-AUTH-011).
 const d=await loaded({[`documents/${stored}/content/access`]:()=>{throw new ApiError('denied','FORBIDDEN');}},undefined,withContent);
 await d.c.openReport(damage);assert.equal(await d.c.content(stored),null);
 assert.equal(d.c.state.contentAllowed,false);assert.equal(d.c.canOpenContent(stored),false);assert.equal(d.c.state.report.id,damage);assert.match(d.c.state.message,/permissão/);
 assert.equal(await d.c.content(stored),null);assert.equal(d.content().length,1,'no second content request after the refusal');
 // Another organization's authority never inherits the hint.
 d.c.open(otherAuthority);assert.equal(d.c.state.contentAllowed,false);
});

test('a foreign or vanished report clears the panel; free or mismatched references are never used',async()=>{
 const p=await loaded();
 await p.c.openReport(stranger);await p.c.openRun(stranger);assert.equal(p.calls.filter(c=>c.path.includes(stranger)).length,0,'an unlisted reference is never sent');
 const q=await loaded({[`incidents/reports/${damage}/read`]:()=>{throw new ApiError('denied','NOT_FOUND');}});
 await q.c.openReport(damage);
 assert.equal(q.c.state.phase,'denied');assert.deepEqual(q.c.state.runs,[]);assert.deepEqual(q.c.state.reports,[]);assert.equal(q.c.state.report,null);assert.equal(q.c.state.availability,null);
 const text=JSON.stringify(q.c.state);for(const hidden of [damage,vehicle,'Lanterna','Caminhão sintético'])assert.equal(text.includes(hidden),false,'revealed '+hidden);
 assert.match(q.c.state.message,/não pôde ser confirmado/);
 // A decision on a report that became foreign clears the same way.
 const d=await loaded({[`incidents/reports/${damage}/decide`]:()=>{throw new ApiError('denied','NOT_FOUND');}});
 await d.c.openReport(damage);await d.c.decide('ACKNOWLEDGE','','');assert.equal(d.c.state.phase,'denied');assert.equal(d.c.state.report,null);
 // A read answering another report is unusable and not shown.
 const m=await loaded({[`incidents/reports/${damage}/read`]:()=>wireReport(dup)});
 await m.c.openReport(damage);assert.equal(m.c.state.report,null);assert.notEqual(m.c.state.phase,'ready');
 // A late answer of the previous organization never renders in the new one.
 const late=deferred();const l=panel({'checklists/runs/list':()=>late.promise});l.c.open(authority);const pending=l.c.load();l.c.open(otherAuthority);
 late.resolve({items:[wireRun(blocking)],next_cursor:'',data_mode:'SYNTHETIC'});await pending;
 assert.equal(l.c.state.authority.organization.id,other.id);assert.deepEqual(l.c.state.runs,[]);assert.equal(l.c.state.phase,'idle');
 // FRESH_AUTH_REQUIRED ends the session view.
 let lost='';const f=await loaded({[`checklists/runs/${blocking}/release`]:()=>{throw new ApiError('fresh');}},msg=>{lost=msg;});
 await f.c.openRun(blocking);await f.c.release('Freios verificados');assert.equal(f.c.state.phase,'closed');assert.match(lost,/Entre novamente/);
});

test('a CONFLICT re-reads the record and its availability and is never retried',async()=>{
 let rival;
 const p=await loaded({[`incidents/reports/${damage}/decide`]:(_body,x)=>{const now=x.reports.get(damage);rival={...now,state:'ACKNOWLEDGED',revision:2,history:[{revision:2,action:'ACKNOWLEDGE',from_state:'REPORTED',to_state:'ACKNOWLEDGED',actor_membership_id:handler,recorded_at:at}]};x.reports.set(damage,rival);throw new ApiError('conflict','CONFLICT');}});
 await p.c.openReport(damage);const before=p.calls.length;
 await p.c.decide('DISMISS','Sem dano aparente','');
 assert.equal(p.writes().length,1);assert.equal(p.c.state.conflict,true);assert.match(p.c.state.message,/já atualizado/);
 assert.equal(p.c.state.report.revision,2,'the newer server revision is shown');assert.equal(p.c.state.report.state,'ACKNOWLEDGED');
 const after=p.calls.slice(before).map(c=>c.path.slice(`/v1/orgs/${org.id}/`.length));
 assert.ok(after.includes(`incidents/reports/${damage}/read`)&&after.includes(`fleet/assets/${vehicle}/availability/read`),'the report and its asset are read again');
 assert.equal(p.c.retryable,false);await p.c.retry();assert.equal(p.writes().length,1);
 assert.equal(p.c.state.reports.find(r=>r.id===damage).revision,2,'the list row is the refreshed server state');
 // A release at a stale revision re-reads the run.
 const r=await loaded();await r.c.openRun(blocking);r.runs.set(blocking,released());
 await r.c.release('Freios verificados');
 assert.equal(r.c.state.conflict,true);assert.equal(r.c.state.run.revision,2);assert.equal(r.c.retryable,false);
});

test('commands send exact schema-1 bodies with the read revision, offer only allowed steps and retry only the same operation',async()=>{
 const p=await loaded();
 await p.c.openRun(blocking);await p.c.release('Freios verificados na oficina');
 await p.c.openReport(damage);
 assert.deepEqual(p.c.actions(),['ACKNOWLEDGE','TREAT','RESOLVE','DISMISS']);
 await p.c.decide('ACKNOWLEDGE','ignored note','');
 assert.deepEqual(p.c.actions(),['TREAT','RESOLVE','DISMISS']);
 await p.c.decide('TREAT','Enviado à oficina interna','');
 await p.c.decide('RESOLVE','Lanterna trocada','');
 assert.deepEqual(p.c.actions(),[],'a terminal report offers no step');
 await p.c.openReport(dup);
 assert.deepEqual(p.c.duplicates().map(r=>r.id),[damage,closed],'only listed reports of the same asset can be the original');
 await p.c.decide('DISMISS','Duplicada',elsewhere);await p.c.decide('DISMISS','Duplicada',stranger);await p.c.decide('RESOLVE','  ','');
 assert.equal(p.writes().length,4,'a foreign duplicate, an unlisted reference and a blank note are refused before transport');
 await p.c.decide('DISMISS','Mesmo dano já registrado',damage);
 const [release,ack,treat,resolve,dismiss]=p.writes().map(v=>v.body);
 const id=v=>{assert.match(v.operation_id,/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/);const {operation_id,...rest}=v;return rest;};
 assert.deepEqual(id(release),{schema_version:1,revision:1,reason:'Freios verificados na oficina'});
 assert.deepEqual(id(ack),{schema_version:1,revision:1,action:'ACKNOWLEDGE'});
 assert.deepEqual(id(treat),{schema_version:1,revision:2,action:'TREAT',note:'Enviado à oficina interna'});
 assert.deepEqual(id(resolve),{schema_version:1,revision:3,action:'RESOLVE',note:'Lanterna trocada'});
 assert.deepEqual(id(dismiss),{schema_version:1,revision:1,action:'DISMISS',note:'Mesmo dano já registrado',duplicate_of:damage});
 assert.equal(new Set(p.writes().map(v=>v.body.operation_id)).size,5);
 assert.equal(p.c.state.report.state,'DISMISSED');assert.equal(p.c.state.report.history.at(-1).duplicateOf,damage);
 // A lost acknowledgement keeps the server state and retries the identical body only.
 let first=true;const q=await loaded({[`incidents/reports/${damage}/decide`]:(body,x)=>{if(first){first=false;throw new ApiError('unavailable');}const now=x.reports.get(damage);x.reports.set(damage,{...now,state:'ACKNOWLEDGED',revision:2,history:[{revision:2,action:'ACKNOWLEDGE',from_state:'REPORTED',to_state:'ACKNOWLEDGED',actor_membership_id:handler,recorded_at:at}]});return {report_id:damage,action:'ACKNOWLEDGE',state:'ACKNOWLEDGED',revision:2,recorded_at:at,replayed:true};}});
 await q.c.openReport(damage);await q.c.decide('ACKNOWLEDGE','','');
 assert.equal(q.c.state.report.state,'REPORTED');assert.equal(q.c.retryable,true);
 await q.c.decide('DISMISS','Outra decisão','');assert.equal(q.writes().length,1,'no new decision while an outcome is unknown');
 await q.c.retry();assert.equal(q.writes().length,2);assert.deepEqual(q.writes()[0].body,q.writes()[1].body);assert.equal(q.c.state.report.state,'ACKNOWLEDGED');
 // An acknowledgement that does not answer the decision is unusable.
 const w=await loaded({[`incidents/reports/${damage}/decide`]:()=>({report_id:damage,action:'RESOLVE',state:'RESOLVED',revision:2,recorded_at:at,replayed:false})});
 await w.c.openReport(damage);await w.c.decide('ACKNOWLEDGE','','');assert.equal(w.c.state.report.state,'REPORTED');assert.equal(w.c.retryable,true);
 // VALIDATION_FAILED keeps the record and is not retried.
 const v=await loaded({[`checklists/runs/${blocking}/release`]:()=>{throw new ApiError('invalid','VALIDATION_FAILED');}});
 await v.c.openRun(blocking);await v.c.release('x');assert.equal(v.c.state.phase,'failed');assert.match(v.c.state.message,/Confira/);assert.equal(v.c.retryable,false);assert.equal(v.c.state.run.revision,1);
});

test('the transport admits only the panel routes, keeps SELF_APPROVAL and bounds document content separately',async()=>{
 const csrf='abv_company_csrf_'+'a'.repeat(64);const seen=[];let body=()=>json({});
 const transport=new CompanyApi(async(path,init)=>{if(path.endsWith('/session/read')&&init.headers['X-CSRF']===undefined)return json({},401);if(path.endsWith('/keys/login'))return json({account:{account_id:uuid(1)},session:{family_id:uuid(9),access_token:'synthetic'}});if(path.endsWith('/session/login'))return json({account_id:uuid(1),authenticated_at:new Date().toISOString(),expires_at:new Date(Date.now()+3600_000).toISOString(),csrf_token:csrf});seen.push(path);return body(path);});
 await transport.signIn('synthetic-key');
 const base=`/v1/orgs/${org.id}`;
 for(const path of [`${base}/checklists/runs/list`,`${base}/checklists/runs/${blocking}/release`,`${base}/checklists/assets/${vehicle}/runs/${blocking}/read`,`${base}/incidents/reports/list`,`${base}/incidents/reports/${damage}/read`,`${base}/incidents/reports/${damage}/decide`,`${base}/fleet/assets/list`,`${base}/fleet/assets/${vehicle}/availability/read`])await transport.privatePost(path,{});
 const allowed=seen.length;assert.equal(allowed,8);
 for(const path of [`${base}/documents/${stored}/content/access`,`${base}/documents/${stored}/content/read`,`${base}/incidents/reports/mine`,`${base}/checklists/templates/create`,`${base}/checklists/runs/${blocking}/delete`,`${base}/documents/${stored}/links/create`,`${base}/incidents/reports/../list`])await assert.rejects(transport.privatePost(path,{}),e=>e.kind==='denied',path);
 assert.equal(seen.length,allowed,'document content never travels through the ordinary transport');
 for(const path of [`${base}/documents/${stored}/content/access`,`${base}/documents/${stored}/content/read`])await transport.privateContent(path,{});
 for(const path of [`${base}/documents/${stored}/content/upload`,`${base}/checklists/runs/list`,`${base}/documents/uploads/create`,`/v1/orgs/x/documents/${stored}/content/read`])await assert.rejects(transport.privateContent(path,{}),e=>e.kind==='denied',path);
 assert.equal(seen.length,allowed+2);
 // SELF_APPROVAL survives as a safe code of a 409.
 body=()=>json({error:{code:'SELF_APPROVAL',message:'refused',details:[],trace_id:''}},409);
 await assert.rejects(transport.privatePost(`${base}/checklists/runs/${blocking}/release`,{}),e=>e.kind==='conflict'&&e.code==='SELF_APPROVAL');
 // Ordinary JSON stays bounded at 64 KiB; the content read admits a real photo.
 body=()=>json({content_base64:'A'.repeat(200*1024)});
 await assert.rejects(transport.privatePost(`${base}/incidents/reports/list`,{}),e=>e.kind==='unavailable');
 const big=await transport.privateContent(`${base}/documents/${stored}/content/read`,{});assert.equal(big.content_base64.length,200*1024);
 body=()=>json({content_base64:'A'.repeat(9*1024*1024)});
 await assert.rejects(transport.privateContent(`${base}/documents/${stored}/content/read`,{}),e=>e.kind==='unavailable');
});
