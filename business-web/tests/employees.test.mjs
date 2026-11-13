import test from 'node:test';
import assert from 'node:assert/strict';
import {compile,uuid,deferred} from './support.mjs';
const {load,cleanup}=compile();process.on('exit',cleanup);
const {EmployeeAccessController}=await load('employees');
const {ApiError}=await load('api');
const org={id:uuid(2),membershipId:uuid(3),name:'Synthetic organization'};
const authority={accountId:uuid(1),organization:org,permissions:['idn.employee.read','idn.employee.manage','idn.invitation.manage'],invitationGrants:[{role:'DRIVER',scope:'SELF'}],invitationValidityDays:7};
const employee={id:uuid(4),state:'ACTIVE',accessState:'NONE',displayName:'Synthetic employee',registrationNumber:'SYN-4',profileRevision:1,homeUnitId:'',workEmail:'',workPhone:'',jobTitle:''};
function fixture(overrides={}){const renders=[];const calls=[];const source={async employees(){return {items:[employee],nextCursor:''};},async employee(){return employee;},async matches(){return {items:[],nextCursor:''};},async create(_org,body){calls.push(body);return {employee_id:employee.id};},async invitations(){return {items:[],nextCursor:''};},async issue(){return {invitation:{id:uuid(5),employeeId:employee.id,state:'PENDING',revision:1,grants:[{role:'DRIVER',scope:'SELF'}],expiresAt:'2026-12-01T00:00:00Z',failedAttempts:0,validityDays:7,issuerMembershipId:uuid(3),membershipId:uuid(6)},delivery:{code:'AIN1.synthetic-private',link:'https://example.invalid/#synthetic'}};},...overrides};const c=new EmployeeAccessController(source,s=>renders.push(s));return {c,source,renders,calls};}
test('org switch, refusal and reset discard late employee results and first-only invitation delivery',async()=>{
 const late=deferred();const f=fixture({employees:()=>late.promise});const pending=f.c.open(authority);f.c.clear();late.resolve({items:[employee],nextCursor:''});await pending;assert.equal(f.c.state.phase,'closed');assert.deepEqual(f.c.state.employees,[]);
 const g=fixture();await g.c.open(authority);await g.c.openEmployee(employee.id);await g.c.issue([{role:'DRIVER',scope:'SELF'}]);assert.ok(g.c.state.delivery);g.c.clear();assert.equal(g.c.state.delivery,null);assert.equal(JSON.stringify(g.c.state).includes('AIN1.synthetic-private'),false);
 const h=fixture({employees:async()=>{throw new ApiError('denied');}});await h.c.open(authority);assert.equal(h.c.state.phase,'denied');assert.deepEqual(h.c.state.employees,[]);
});
test('lost create acknowledgement retries one immutable operation; duplicate review cannot silently bypass confirmation',async()=>{
 let first=true;const f=fixture({async create(_org,body){f.calls.push(structuredClone(body));if(first){first=false;throw new ApiError('unavailable');}return {employee_id:employee.id};}});await f.c.open(authority);
 const profile={display_name:'Synthetic employee',registration_number:'SYN-4',work_email:'',work_phone:'',job_title:'',home_unit_id:''};await f.c.create(profile,false);assert.equal(f.c.state.phase,'failed');await f.c.retry();assert.equal(f.calls.length,2);assert.equal(f.calls[0].operation_id,f.calls[1].operation_id);assert.deepEqual(f.calls[0],f.calls[1]);
 const g=fixture({async matches(){return {items:[employee],nextCursor:''};}});await g.c.open(authority);await g.c.review(profile);assert.equal(g.c.state.duplicates.length,1);assert.equal(g.calls.length,0);await g.c.create(profile,false);assert.equal(g.calls.length,0);assert.equal(g.c.state.phase,'review');await g.c.create(profile,true);assert.equal(g.calls.length,1);assert.equal(g.calls[0].allow_possible_duplicate,true);
});
test('permission and current grant options disable commands before transport',async()=>{
 const f=fixture();await f.c.open({...authority,permissions:['idn.employee.read'],invitationGrants:[]});await f.c.openEmployee(employee.id);await f.c.issue([{role:'OWNER',scope:'ORG'}]);assert.equal(f.c.state.delivery,null);assert.equal(f.c.state.phase,'denied');
 const g=fixture();await g.c.open(authority);await g.c.openEmployee(employee.id);await g.c.issue([{role:'OWNER',scope:'ORG'}]);assert.equal(g.c.state.delivery,null);assert.equal(g.c.state.phase,'denied');
});
test('resend lost acknowledgement retries the same CAS command, returns metadata only and refreshes superseded history',async()=>{
 const base={id:uuid(5),employeeId:employee.id,state:'PENDING',revision:1,grants:[{role:'DRIVER',scope:'SELF'}],expiresAt:'2026-12-01T00:00:00Z',failedAttempts:0,validityDays:7,issuerMembershipId:uuid(3),membershipId:uuid(6)};
 const renewed={...base,id:uuid(7)};let first=true;const bodies=[];
 const f=fixture({async invitations(){return {items:[{...base,state:'SUPERSEDED',revision:2},renewed],nextCursor:''};},async invitationAction(_org,_id,_action,body){bodies.push(structuredClone(body));if(first){first=false;throw new ApiError('unavailable');}return {invitation:renewed,delivery:null};}});
 await f.c.open(authority);await f.c.openEmployee(employee.id);await f.c.issue([{role:'DRIVER',scope:'SELF'}]);f.c.closeDelivery();await f.c.invitationAction(base.id,'resend');assert.equal(f.c.state.phase,'failed');assert.equal(f.c.state.delivery,null);await f.c.retry();assert.deepEqual(bodies[0],bodies[1]);assert.equal(f.c.state.delivery,null);assert.equal(f.c.state.invitations.find(i=>i.id===base.id).state,'SUPERSEDED');assert.match(f.c.state.message,/não pode ser exibido novamente/);
});
test('conflicting manager decision clears the pending review and never retries against a changed revision',async()=>{
 const invitation={id:uuid(5),employeeId:employee.id,membershipId:uuid(6),state:'ACCEPTED'};let actions=0;
 const f=fixture({async invitations(){return {items:[invitation],nextCursor:''};},async admission(){return {invitationId:invitation.id,membershipId:invitation.membershipId,accountAlias:'Synthetic recipient',state:'WAITING',revision:1,deadline:'2026-12-01T00:00:00Z'};},async admissionAction(){actions++;throw new ApiError('conflict');}});
 await f.c.open(authority);await f.c.openEmployee(employee.id);await f.c.readAdmission(invitation.id);await f.c.admissionAction('confirm');assert.equal(f.c.state.admission,null);assert.equal(f.c.retryable,false);await f.c.retry();assert.equal(actions,1);
});
test('an acknowledged resend preserves first-only delivery when history fails and never replays that write',async()=>{
 const base={id:uuid(5),employeeId:employee.id,state:'PENDING',revision:1,grants:[{role:'DRIVER',scope:'SELF'}],expiresAt:'2026-12-01T00:00:00Z',failedAttempts:0,validityDays:7,issuerMembershipId:uuid(3),membershipId:uuid(6)};
 const renewed={...base,id:uuid(7)};let historyFails=false,actions=0;
 const f=fixture({async invitations(){if(historyFails)throw new ApiError('unavailable');return {items:[base],nextCursor:''};},async invitationAction(){actions++;historyFails=true;return {invitation:renewed,delivery:{code:'AIN1.synthetic-new-private',link:'synthetic-link'}};}});
 await f.c.open(authority);await f.c.openEmployee(employee.id);await f.c.invitationAction(base.id,'resend');assert.equal(f.c.state.delivery.code,'AIN1.synthetic-new-private');assert.equal(f.c.state.invitations.some(i=>i.id===base.id),false);assert.match(f.c.state.message,/histórico não pôde ser atualizado/);await f.c.retry();assert.equal(actions,1);assert.equal(f.c.state.delivery.code,'AIN1.synthetic-new-private');
});
test('private-plane authentication refusal clears panel and synchronously resets the owning session shell',async()=>{
 let reason='';const source=fixture({async employees(){throw new ApiError('fresh');}}).source;
 const controller=new EmployeeAccessController(source,()=>{},message=>{reason=message;});await controller.open(authority);assert.equal(controller.state.phase,'closed');assert.equal(controller.state.authority,null);assert.match(reason,/Entre novamente/);
});
test('definitive history denial after resend acknowledgement clears delivery and employee metadata',async()=>{
 const base={id:uuid(5),employeeId:employee.id,state:'PENDING',revision:1,grants:[{role:'DRIVER',scope:'SELF'}],expiresAt:'2026-12-01T00:00:00Z',failedAttempts:0,validityDays:7,issuerMembershipId:uuid(3),membershipId:uuid(6)};let denied=false;
 const f=fixture({async invitations(){if(denied)throw new ApiError('denied');return {items:[base],nextCursor:''};},async invitationAction(){denied=true;return {invitation:{...base,id:uuid(7)},delivery:{code:'AIN1.synthetic-new-private',link:'synthetic-link'}};}});
 await f.c.open(authority);await f.c.openEmployee(employee.id);await f.c.invitationAction(base.id,'resend');assert.equal(f.c.state.phase,'denied');assert.equal(f.c.state.delivery,null);assert.equal(f.c.state.employee,null);assert.deepEqual(f.c.state.employees,[]);
});
