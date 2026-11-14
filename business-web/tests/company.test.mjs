import test from 'node:test';
import assert from 'node:assert/strict';
import {compile,uuid} from './support.mjs';
const {load,cleanup}=compile();process.on('exit',cleanup);
const {CurrentMemberships}=await load('company');
const account=uuid(1),org={id:uuid(2),membership_id:uuid(3),name:'Synthetic company'};
const signal=()=>new AbortController().signal;
test('real membership adapter binds every page and selected membership to the session account',async()=>{
 const calls=[];const source=new CurrentMemberships({async privatePost(path,body){calls.push({path,body});if(path.endsWith('/list'))return {account_id:account,organizations:[org],next_after_org_id:''};return {account_id:account,organization:org,permissions:['idn.employee.read'],invitation_grants:[{role:'DRIVER',scope:'SELF'}],invitation_validity_days:7};}});
 const list=await source.list(account,signal());assert.equal(list.organizations[0].membershipId,org.membership_id);const current=await source.authorize(account,org.id,signal());assert.equal(current.organization.name,org.name);assert.deepEqual(calls[1].body,{organization_id:org.id,membership_id:org.membership_id});source.clear();await assert.rejects(source.authorize(account,org.id,signal()),e=>e.kind==='denied');
});
test('directory scans tolerate an empty eligible page and refuse account replacement, rewind, cyclic and cross-membership reads',async()=>{
 let page=0;const source=new CurrentMemberships({async privatePost(){page++;return page===1?{account_id:account,organizations:[],next_after_org_id:uuid(2)}:{account_id:account,organizations:[{...org,id:uuid(4)}],next_after_org_id:''};}});assert.equal((await source.list(account,signal())).organizations.length,1);
 for(const raw of [{account_id:uuid(8),organizations:[org],next_after_org_id:''},{account_id:account,organizations:[org,org],next_after_org_id:''},{account_id:account,organizations:[org],next_after_org_id:uuid(1)}]){const bad=new CurrentMemberships({async privatePost(){return raw;}});await assert.rejects(bad.list(account,signal()));}
 const bad=new CurrentMemberships({async privatePost(path){return path.endsWith('/list')?{account_id:account,organizations:[org],next_after_org_id:''}:{account_id:account,organization:{...org,membership_id:uuid(8)},permissions:[],invitation_grants:[],invitation_validity_days:7};}});await bad.list(account,signal());await assert.rejects(bad.authorize(account,org.id,signal()));
});
test('directory rejects noncanonical ordering and duplicated membership identities',async()=>{
 for(const items of [[{...org,id:uuid(8)},org],[org,{...org,id:uuid(8)}]]){const source=new CurrentMemberships({async privatePost(){return {account_id:account,organizations:items,next_after_org_id:''};}});await assert.rejects(source.list(account,signal()),e=>e.kind==='denied');}
});
