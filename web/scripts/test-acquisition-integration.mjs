// Isolated acquisition qualification: real sessions only, no network interception.
import assert from 'node:assert/strict';
import {readFile, writeFile} from 'node:fs/promises';
import {createRequire} from 'node:module';
import {resolve} from 'node:path';
import {fileURLToPath} from 'node:url';

export function requireEnv(name) { const v=process.env[name]; assert.ok(v, 'Required fixture configuration missing'); return v; }
export function fixtureOrigin(value) {
  const u=new URL(value);
  assert.equal(u.protocol,'https:');
  assert.ok(['127.0.0.1','localhost','[::1]'].includes(u.hostname),'Only fixture loopback is allowed');
  assert.ok(u.pathname==='/' && !u.username && !u.password && !u.search && !u.hash);
  return u.origin;
}
export function expectStatus(response, expected, operation) {
  assert.ok([].concat(expected).includes(response.status()), operation+' returned unexpected HTTP status');
}
async function main() {
  assert.equal(requireEnv('ACQUISITION_TEST_ISOLATED'),'1');
  const baseURL=fixtureOrigin(requireEnv('ACQUISITION_TEST_ORIGIN'));
  const phase=process.argv[2];
  assert.ok(['bootstrap','prepare','verify','persistence','tombstone'].includes(phase));
  const password=(await readFile(requireEnv('ACQUISITION_TEST_PASSWORD_FILE'),'utf8')).trim();
  assert.ok(password.length>=12);
  const load=createRequire(resolve(requireEnv('ACQUISITION_TEST_PLAYWRIGHT_ROOT'),'package.json'));
  const {chromium,request}=load('playwright');
  const tls=process.env.ACQUISITION_TEST_SELF_SIGNED==='1';
  const headers={Origin:baseURL,'Sec-Fetch-Site':'same-origin'};
  const identities={admin:'admin@example.test',reader:'reader@example.test',denied:'denied@example.test'};
  const idsFile=requireEnv('ACQUISITION_TEST_IDS_FILE');
  async function session(identifier) {
    const api=await request.newContext({baseURL,ignoreHTTPSErrors:tls,extraHTTPHeaders:headers});
    expectStatus(await api.post('/v1/session/password',{data:{identifier,password}}),200,'Fixture login');
    return api;
  }
  if(phase==='bootstrap') {
    const api=await request.newContext({baseURL,ignoreHTTPSErrors:tls,extraHTTPHeaders:headers});
    try {
      const code=(await readFile(requireEnv('ACQUISITION_TEST_SETUP_CODE_FILE'),'utf8')).trim();
      expectStatus(await api.post('/v1/setup',{data:{Code:code,Password:password,Organization:'Acquisition qualification operations',Brand:{name:'Acquisition qualification',short:'Qualification',tagline:'Disposable integration fixture'}}}),200,'Fixture setup');
    } finally {await api.dispose();}
    console.log('PASS isolated setup; restart only this fixture panel to seed test accounts.'); return;
  }
  if(phase==='prepare') {
    const ids={};
    const sessions={};
    for(const [kind,identifier] of Object.entries(identities)) {
      const api=await session(identifier); sessions[kind]=api;
      const r=await api.get('/v1/me');expectStatus(r,200,'Fixture identity');const me=await r.json();assert.match(me.user_id,/^local:[0-9a-f-]{36}$/);ids[kind]={subject:me.user_id};
    }
    const owner=await session('setup');
    try {
      const existing=await owner.get('/v1/operator/team/permission-grants');expectStatus(existing,200,'Fixture grant list');
      const grants=await existing.json();
      // Dev-account seeding restores automatic direct grants on every panel start.
      // Remove only the named fixture reader's write grants through the audited API.
      for(const permission of ['platform.settings.write','acquisition.sync']) {
        if(!grants.items.some(g=>g.subject===ids.reader.subject && g.permission===permission)) continue;
        const path='/v1/operator/team/permission-grants/'+encodeURIComponent(ids.reader.subject)+'/'+encodeURIComponent(permission)+'?reason='+encodeURIComponent('Isolated acquisition reader qualification');
        expectStatus(await owner.delete(path),204,'Revoke fixture reader write permission');
      }

      for(const [kind,permissions] of Object.entries({admin:['acquisition.read','acquisition.sync'],reader:['acquisition.read']})) {
        for(const permission of permissions) {
          if(grants.items.some(g=>g.subject===ids[kind].subject && g.permission===permission)) continue;
          expectStatus(await owner.post('/v1/operator/team/permission-grants',{data:{subject:ids[kind].subject,permission,reason:'Isolated acquisition qualification'}}),201,'Explicit fixture permission grant');
        }
      }
    } finally {await owner.dispose();}
    for(const kind of ['admin','reader']) {
      const api=sessions[kind];
      try {const r=await api.get('/v1/operator/access');expectStatus(r,200,'Effective fixture access');const p=(await r.json()).staff.permissions;assert.ok(p.includes('acquisition.read'));assert.equal(p.includes('acquisition.sync'),kind==='admin');assert.equal(p.includes('platform.settings.write'),kind==='admin','Reader must not inherit source configuration write permission');} finally {await api.dispose();}
    }
    await sessions.denied.dispose();
    await writeFile(idsFile,JSON.stringify(ids,null,2),{mode:0o600});
    console.log('PASS real fixture identities and explicit acquisition grants; IDs only recorded.');return;
  }
  // The flow module is supplied only after reviewing the actual generated backend and UI contract.
  const {qualify}=await import('../../scripts/test-acquisition/flow.mjs');
  await qualify({phase,baseURL,tls,headers,password,identities,idsFile,chromium,request,session});
}
if(process.argv[1] && resolve(process.argv[1])===resolve(fileURLToPath(import.meta.url))) {
  main().catch(()=>{console.error('FAIL acquisition qualification; sensitive diagnostic detail suppressed.');process.exitCode=1;});
}
