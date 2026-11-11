import test from 'node:test';
import assert from 'node:assert/strict';
import { compile } from './support.mjs';
const { load, cleanup } = compile();
const model = await load('model');
process.on('exit', cleanup);
const a='7b0e2a54-0000-4000-8000-000000000001';
test('routes accept only canonical company entries; path is never membership authority',()=>{
  assert.deepEqual(model.route('/empresas/'),{kind:'overview'});
  assert.deepEqual(model.route('/empresas/entrar/'),{kind:'login'});
  assert.deepEqual(model.route(`/empresas/organizacoes/${a}/`),{kind:'organization',id:a});
  for(const path of ['/admin/','/postos/','/empresas/../admin/', '/empresas/organizacoes/00000000-0000-0000-0000-000000000000/','/empresas/organizacoes/%2f/']) assert.equal(model.route(path).kind,'not-found');
  assert.equal(model.route('/empresas/','?access_token=secret').kind,'not-found');
});
test('membership snapshots reject mixed accounts, duplicate scopes and invalid membership references',()=>{
  const org={id:a,name:'Empresa sintética',membershipId:a};
  assert.equal(model.validSnapshot({accountId:a,organizations:[org]},a),true);
  for(const value of [{accountId:a,organizations:[org,org]},{accountId:'other',organizations:[org]},{accountId:a,organizations:[{...org,membershipId:''}]},{accountId:a,organizations:[{...org,name:''}]}]) assert.equal(model.validSnapshot(value,a),false);
});
