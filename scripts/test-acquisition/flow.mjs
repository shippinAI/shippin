// Real browser acquisition qualification. Never use against an operator installation.
import assert from 'node:assert/strict';
import {readFile,writeFile} from 'node:fs/promises';
import {createPrivateKey,generateKeyPairSync,randomUUID} from 'node:crypto';
import {requireEnv,fixtureOrigin,expectStatus} from '../../web/scripts/test-acquisition-integration.mjs';
const root='/v1/operator/tenancy/acquisition-sources';
const reason='Isolated acquisition browser qualification';
function noSecrets(value){
 if(!value || typeof value!=='object')return;
 for(const [key,item] of Object.entries(value)){
  assert.ok(!/^(signing_key|private_key|password|access_token|refresh_token|authorization|cookie)$/i.test(key),'Credential field in response');noSecrets(item);
 }
}
async function safeJSON(response,status,operation,submittedSeed=''){expectStatus(response,status,operation);const body=await response.json();noSecrets(body);if(submittedSeed)assert.ok(!JSON.stringify(body).includes(submittedSeed),'Submitted seed reflected in response body');return body;}
async function login(browser,c,kind){
 const context=await browser.newContext({baseURL:c.baseURL,ignoreHTTPSErrors:c.tls});
 const page=await context.newPage();await page.goto('/login');
 await page.getByRole('textbox',{name:'Username or email',exact:true}).fill(c.identities[kind]);
 await page.getByLabel('Password',{exact:true}).fill(c.password);
 const response=page.waitForResponse(r=>new URL(r.url()).pathname==='/v1/session/password'&&r.request().method()==='POST');
 await page.getByRole('button',{name:'Sign in with password',exact:true}).click();expectStatus(await response,200,'Browser login');
 await page.waitForURL(u=>u.pathname!=='/login');return {context,page};
}
async function seed(browser,tls){
 const origin=fixtureOrigin(requireEnv('ACQUISITION_TEST_SIGNUP_ORIGIN'));
 const context=await browser.newContext({baseURL:origin,ignoreHTTPSErrors:tls});
 const email='acquisition-'+randomUUID()+'@example.test';
 try{
  const page=await context.newPage();await page.goto('/');
  await page.locator('#email:not([disabled])').fill(email);await page.locator('#contact-consent').check();
  const response=page.waitForResponse(r=>new URL(r.url()).pathname==='/v1/public/early-access'&&r.request().method()==='POST');
  await page.locator('#signup-form button[type="submit"]').click();expectStatus(await response,202,'Actual public fixture signup');
  await page.waitForFunction(()=>document.querySelector('#signup-message')?.textContent.includes('on the list'));
  return email;
 }finally{await context.close();}
}
async function assertSecretAbsent(page,seed){
 assert.ok(!(await page.content()).includes(seed),'Secret in rendered document');
 const state=await page.evaluate(()=>({local:{...localStorage},session:{...sessionStorage},url:location.href}));
 assert.ok(!JSON.stringify(state).includes(seed),'Secret in browser persistence');
 for(const field of await page.locator('input[type="password"]').all()){const cleared=await field.evaluate((input,secret)=>input.value===''&&input.defaultValue===''&&!(input.getAttribute('value')||'')&& !input.outerHTML.includes(secret),seed);assert.ok(cleared,'Password value/defaultValue/attribute/markup not cleared');}
}
async function denials(browser,c,id,email,label){
 const anonymous=await browser.newContext({baseURL:c.baseURL,ignoreHTTPSErrors:c.tls});
 expectStatus(await anonymous.request.get(root),[401,403],'Anonymous acquisition access');await anonymous.close();
 for(const kind of ['reader','denied']){
  const {context,page}=await login(browser,c,kind);
  try{
   expectStatus(await context.request.get(root),kind==='reader'?200:403,'Fixture list authority');
   expectStatus(await context.request.get(root+'/'+id),403,'Source config authority');
   expectStatus(await context.request.post(root+'/'+id+'/sync',{headers:c.headers,data:{reason}}),403,'Sync authority');
   expectStatus(await context.request.get(root+'/'+id+'/leads'),kind==='reader'?200:403,'Lead read authority');
   if(kind==='reader'){
    await page.goto('/admin/signups');
    await page.getByRole('heading',{name:'Imported early-access',exact:true}).waitFor();
    await selectSource(page,label);
    for(const name of ['Create source','Save source','Resume source','Pause source','Pull one page']) assert.equal(await page.getByRole('button',{name,exact:true}).count(),0,'Reader must not see mutating controls');
    await page.getByRole('tab',{name:'Leads',exact:true}).click();
    await page.getByText(email,{exact:true}).waitFor();
    await page.getByText('Not verified',{exact:true}).first().waitFor();
   }
  }finally{await context.close();}
 }
}

async function uiResponse(page,method,path,action){
 const pending=page.waitForResponse(r=>new URL(r.url()).pathname===path&&r.request().method()===method);
 await action();return pending;
}
function formWith(page,name){return page.locator('form').filter({has:page.getByRole('button',{name,exact:true})});}
async function fill(form,fields){for(const [name,value] of Object.entries(fields))await form.getByLabel(name,{exact:true}).fill(String(value));}
async function sourcePage(page){await page.goto('/admin/signups');await page.getByRole('heading',{name:'Imported early-access',exact:true}).waitFor();}
async function selectSource(page,label){
 const selector=page.getByRole('button',{name:/Configured source/});
 await selector.click();await page.getByRole('option').filter({hasText:label}).click();
}
async function pull(page,id){
 const form=formWith(page,'Pull one page');await form.getByLabel('Audited reason',{exact:true}).fill(reason);
 return safeJSON(await uiResponse(page,'POST',root+'/'+id+'/sync',()=>form.getByRole('button',{name:'Pull one page',exact:true}).click()),200,'UI manual pull');
}
async function list(context,id){return safeJSON(await context.request.get(root+'/'+id+'/leads'),200,'Actual persisted imported leads');}
export async function qualify(c){
 if(['baseline','migrate','resume','restore'].includes(c.phase))return migration(c);
 const browser=await c.chromium.launch({headless:true});
 const recordFile=requireEnv('ACQUISITION_TEST_RECORD_FILE');
 let seedValue='';
 try{
  let record=c.phase==='verify'?null:JSON.parse(await readFile(recordFile,'utf8'));
  console.log('CHECK public fixture signup or persisted baseline');
  const email=c.phase==='verify'?await seed(browser,c.tls):record.email;
  assert.ok(email.endsWith('@example.test'),'Synthetic email required');
  const {context,page}=await login(browser,c,'admin');
  try{
   console.log('CHECK admin acquisition UI');
   await sourcePage(page);
   if(c.phase==='verify'){
    const ca=await readFile(requireEnv('ACQUISITION_TEST_SOURCE_CA_FILE'),'utf8');
    const key=createPrivateKey(await readFile(requireEnv('ACQUISITION_TEST_SIGNER_FILE')));
    assert.equal(key.asymmetricKeyType,'ed25519');
    const bytes=Buffer.from(key.export({format:'jwk'}).d,'base64url');assert.equal(bytes.length,32);seedValue=bytes.toString('base64');bytes.fill(0);
    const label='Acquisition fixture '+randomUUID().slice(0,8);
    console.log('CHECK UI source creation');
    const form=formWith(page,'Create source');
    await fill(form,{'Name':label,'HTTPS origin':'https://127.0.0.1','Destination IP':'127.0.0.1','Port':18082,'CA certificate (PEM)':ca,'Source tenant':'container','Source project':'signup','Issuer':'fixture','Audience':'source-admin','Signing seed (base64)':seedValue,'Audited reason':reason});
    const created=await safeJSON(await uiResponse(page,'POST',root,()=>form.getByRole('button',{name:'Create source',exact:true}).click()),201,'UI source configuration',seedValue);
    assert.equal(created.paused,true);assert.equal(created.cursor,0);assert.equal(created.signer_configured,true);
    await assertSecretAbsent(page,seedValue);
    record={source_id:created.id,label,email};
    console.log('CHECK UI resume and import');
    const resume=page.getByRole('button',{name:'Resume source',exact:true});
    await formWith(page,'Resume source').getByLabel('Audited reason',{exact:true}).fill(reason);
    const resumed=await safeJSON(await uiResponse(page,'PUT',root+'/'+created.id,()=>resume.click()),200,'UI explicit resume');
    assert.equal(resumed.paused,false);assert.ok(resumed.config_generation>created.config_generation);
    await pull(page,record.source_id);
    const leads=await list(context,record.source_id);const lead=leads.items.find(x=>x.email===email);
    assert.ok(lead,'Actual consent signup must arrive through private source pull');assert.equal(lead.email_verified,false);assert.ok(lead.consent_version&&lead.consent_at);
    record.lead_id=lead.id;record.lead=lead;record.total=leads.total;
    await page.getByRole('tab',{name:'Leads',exact:true}).click();await page.getByText(email,{exact:true}).waitFor();
    await page.getByRole('tab',{name:'Source',exact:true}).click();
    const repeated=await pull(page,record.source_id);assert.equal(repeated.applied,0);
    const again=await list(context,record.source_id);assert.deepEqual(again,leads);

    console.log('CHECK real CAS invalidation and pause boundaries');
    // Preserve a local draft across a real concurrent update and query invalidation.
    await page.reload();await sourcePage(page);await selectSource(page,record.label);
    const editor=formWith(page,'Save source');await editor.getByLabel('Name',{exact:true}).fill(record.label+' unsaved draft');
    await editor.getByLabel('Audited reason',{exact:true}).fill(reason);
    const beforeCAS=await safeJSON(await context.request.get(root+'/'+record.source_id),200,'Concurrent update baseline');
    const concurrent={label:beforeCAS.summary.label,origin:beforeCAS.origin,dial_ip:beforeCAS.dial_ip,port:beforeCAS.port,ca_pem:beforeCAS.ca_pem,paused:false,reason,expected_config_generation:beforeCAS.summary.config_generation};
    await safeJSON(await context.request.put(root+'/'+record.source_id,{headers:c.headers,data:concurrent}),200,'Concurrent editor update');
    await pull(page,record.source_id); // real UI invalidates detail query, no mocks/cache manipulation
    await page.getByRole('button',{name:'Reload configuration',exact:true}).waitFor();
    assert.equal(await editor.getByLabel('Name',{exact:true}).inputValue(),record.label+' unsaved draft');
    assert.equal(await editor.getByRole('button',{name:'Save source',exact:true}).isDisabled(),true,'Stale draft must not adopt fresh generation');
    await page.getByRole('button',{name:'Reload configuration',exact:true}).click();
    await page.waitForFunction(expected=>Array.from(document.querySelectorAll('input')).some(i=>i.value===expected),record.label);
    // Pause must use persisted configuration, never an unsaved endpoint or signer.
    await editor.getByLabel('HTTPS origin',{exact:true}).fill('https://unsaved.example.test');
    const temporary=Buffer.from(generateKeyPairSync('ed25519').privateKey.export({format:'jwk'}).d,'base64url').toString('base64');
    await editor.getByLabel('Signing seed (base64, optional)',{exact:true}).fill(temporary);
    const pauseForm=formWith(page,'Pause source');await pauseForm.getByLabel('Audited reason',{exact:true}).fill('Fixture pause with independent reason');
    const pausedResponse=await uiResponse(page,'PUT',root+'/'+record.source_id,()=>pauseForm.getByRole('button',{name:'Pause source',exact:true}).click());
    const sent=pausedResponse.request().postDataJSON();assert.equal(sent.origin,beforeCAS.origin);assert.equal(sent.reason,'Fixture pause with independent reason');assert.ok(!Object.hasOwn(sent,'signing_key'));
    const paused=await safeJSON(pausedResponse,200,'Pause persisted configuration');assert.equal(paused.paused,true);
    await page.reload();await sourcePage(page);await selectSource(page,record.label);
    const resumeForm=formWith(page,'Resume source');await resumeForm.getByLabel('Audited reason',{exact:true}).fill(reason);
    await safeJSON(await uiResponse(page,'PUT',root+'/'+record.source_id,()=>resumeForm.getByRole('button',{name:'Resume source',exact:true}).click()),200,'Resume persisted configuration');
    await pull(page,record.source_id); // proves unsaved signer did not replace actual working signer

    const detail=await safeJSON(await context.request.get(root+'/'+record.source_id),200,'Safe source configuration');
    const update={label:detail.summary.label,origin:detail.origin,dial_ip:detail.dial_ip,port:detail.port,ca_pem:detail.ca_pem,paused:false,reason};
    console.log('CHECK real private TLS and signer failures');
    // A hostname mismatch must fail verified TLS without DNS dialing or cursor mutation.
    let changed=await safeJSON(await context.request.put(root+'/'+record.source_id,{headers:c.headers,data:{...update,origin:'https://wrong-host.example.test',expected_config_generation:detail.summary.config_generation}}),200,'Fixture wrong-trust configuration');
    const failed=await context.request.post(root+'/'+record.source_id+'/sync',{headers:c.headers,data:{reason}});
    expectStatus(failed,[502,503],'Wrong TLS trust must fail');noSecrets(await failed.json());
    let after=await safeJSON(await context.request.get(root+'/'+record.source_id),200,'Source after failed TLS');assert.equal(after.summary.cursor,detail.summary.cursor);
    changed=await safeJSON(await context.request.put(root+'/'+record.source_id,{headers:c.headers,data:{...update,expected_config_generation:changed.config_generation}}),200,'Restore fixture trust');
    const wrong=generateKeyPairSync('ed25519').privateKey.export({format:'jwk'}).d;
    changed=await safeJSON(await context.request.put(root+'/'+record.source_id,{headers:c.headers,data:{...update,expected_config_generation:changed.config_generation,signing_key:Buffer.from(wrong,'base64url').toString('base64')}}),200,'Fixture wrong signer');
    expectStatus(await context.request.post(root+'/'+record.source_id+'/sync',{headers:c.headers,data:{reason}}),[502,503],'Wrong signature must fail');
    changed=await safeJSON(await context.request.put(root+'/'+record.source_id,{headers:c.headers,data:{...update,expected_config_generation:changed.config_generation,signing_key:seedValue}}),200,'Restore fixture signer');
    expectStatus(await context.request.post(root+'/'+record.source_id+'/sync',{headers:{Origin:'https://foreign.example.test','Sec-Fetch-Site':'cross-site'},data:{reason}}),403,'Cross-origin actual acquisition write');
    assert.deepEqual(await list(context,record.source_id),leads);
    const final=await safeJSON(await context.request.get(root+'/'+record.source_id),200,'Record persistence baseline');record.summary=final.summary;
    await writeFile(recordFile,JSON.stringify(record,null,2),{mode:0o600});
    await page.reload();await assertSecretAbsent(page,seedValue);seedValue='';
    console.log('PASS real public consent signup, UI configuration/resume/pull, repeated import, private trust/signature and origin failures');
   }else{
    await selectSource(page,record.label);
    const detail=await safeJSON(await context.request.get(root+'/'+record.source_id),200,'Persisted source');
    if(c.phase==='persistence'){
     assert.deepEqual(detail.summary,record.summary,'Source state must survive recreation exactly');
     if(record.binding)assert.deepEqual(binding(detail),record.binding,'Immutable migrated binding must persist');
     const leads=await list(context,record.source_id);assert.deepEqual(leads.items.find(x=>x.id===record.lead_id),record.lead);assert.equal(leads.total,record.total);
     await page.getByRole('tab',{name:'Leads',exact:true}).click();await page.getByText(email,{exact:true}).waitFor();
     if(process.env.ACQUISITION_TEST_SCREENSHOT_FILE)await page.screenshot({path:process.env.ACQUISITION_TEST_SCREENSHOT_FILE,fullPage:true});
     console.log('PASS source configuration and exact imported consent record survived recreation');
    }else{
     await pull(page,record.source_id);const leads=await list(context,record.source_id);assert.ok(!leads.items.some(x=>x.id===record.lead_id));assert.equal(leads.total,record.total-1);
     const again=await pull(page,record.source_id);assert.equal(again.applied,0);assert.deepEqual(await list(context,record.source_id),leads);
     await page.getByRole('tab',{name:'Leads',exact:true}).click();assert.equal(await page.getByText(email,{exact:true}).count(),0);
     if(process.env.ACQUISITION_TEST_SCREENSHOT_FILE)await page.screenshot({path:process.env.ACQUISITION_TEST_SCREENSHOT_FILE,fullPage:true});
     console.log('PASS actual source tombstone imported, hidden from active leads and not resurrected on repeated pull');
    }
   }
  }finally{await context.close();}
  if(c.phase==='verify')console.log('CHECK reader/customer/anonymous authorization');
  if(c.phase==='verify'){await denials(browser,c,record.source_id,record.email,record.label);console.log('PASS real reader UI and reader/customer/anonymous API authorization');}
  console.log('PASS isolated acquisition '+c.phase+' qualification; not a live deployment claim');
 }finally{seedValue='';await browser.close();}
}


async function assertDetachedSecretCleared(handle,secret){
 const checks=await handle.evaluate((input,seed)=>({detached:!input.isConnected,valueEmpty:input.value==='',defaultEmpty:input.defaultValue==='',attributeEmpty:!(input.getAttribute('value')||''),markupClear:!input.outerHTML.includes(seed)}),secret);
 for(const [name,passed] of Object.entries(checks))assert.ok(passed,'Detached secret predicate failed: '+name);
 await handle.dispose();
}
async function secretLifecycle(page,context,id,label,exporterSeed){
 console.log('CHECK real secret lifecycle success/error/cancel/unmount');
 await page.getByRole('tab',{name:'Source',exact:true}).click();
 const initial=await safeJSON(await context.request.get(root+'/'+id),200,'Lifecycle unchanged source baseline');
 const form=formWith(page,'Save source');
 const temporary=Buffer.from(generateKeyPairSync('ed25519').privateKey.export({format:'jwk'}).d,'base64url').toString('base64');
 // Actual backend validation failure, not an intercepted response.
 await fill(form,{'Destination IP':'not-an-ip','Signing seed (base64, optional)':temporary,'Audited reason':reason});
 const rejected=await uiResponse(page,'PUT',root+'/'+id,()=>form.getByRole('button',{name:'Save source',exact:true}).click());
 await safeJSON(rejected,400,'Actual invalid destination rejection',temporary);await assertSecretAbsent(page,temporary);
 assert.deepEqual(await safeJSON(await context.request.get(root+'/'+id),200,'Rejected edit persisted state'),initial);
 await fill(form,{'Destination IP':initial.dial_ip,'Signing seed (base64, optional)':'invalid-fixture-seed','Audited reason':reason});
 await form.getByRole('button',{name:'Save source',exact:true}).click();
 await assertSecretAbsent(page,'invalid-fixture-seed');
 assert.deepEqual(await safeJSON(await context.request.get(root+'/'+id),200,'Local invalid seed preserved state'),initial);
 // Retain the actual DOM node to prove unmount scrubs it rather than hiding it.
 await form.getByLabel('Signing seed (base64, optional)',{exact:true}).fill(temporary);
 let retained=await form.getByLabel('Signing seed (base64, optional)',{exact:true}).elementHandle();
 await page.getByRole('tab',{name:'Leads',exact:true}).click();await assertDetachedSecretCleared(retained,temporary);await assertSecretAbsent(page,temporary);
 await page.getByRole('tab',{name:'Source',exact:true}).click();await assertSecretAbsent(page,temporary);
 await page.getByRole('button',{name:'Add source',exact:true}).click();
 let create=formWith(page,'Create source');await create.getByLabel('Signing seed (base64)',{exact:true}).fill(temporary);
 retained=await create.getByLabel('Signing seed (base64)',{exact:true}).elementHandle();
 await create.getByRole('button',{name:'Cancel',exact:true}).click();await assertDetachedSecretCleared(retained,temporary);await assertSecretAbsent(page,temporary);
 // Actual create success with valid fixture exporter, left paused and never imported.
 await page.getByRole('button',{name:'Add source',exact:true}).click();create=formWith(page,'Create source');
 await fill(create,{'Name':'Secret lifecycle fixture '+randomUUID().slice(0,8),'HTTPS origin':initial.origin,'Destination IP':initial.dial_ip,'Port':initial.port,'CA certificate (PEM)':initial.ca_pem,'Source tenant':initial.source_tenant,'Source project':initial.source_project,'Issuer':initial.issuer,'Audience':initial.audience,'Signing seed (base64)':exporterSeed,'Audited reason':reason});
 retained=await create.getByLabel('Signing seed (base64)',{exact:true}).elementHandle();
 const created=await safeJSON(await uiResponse(page,'POST',root,()=>create.getByRole('button',{name:'Create source',exact:true}).click()),201,'Actual create lifecycle',exporterSeed);
 assert.equal(created.paused,true);await assertSecretAbsent(page,exporterSeed);await assertDetachedSecretCleared(retained,exporterSeed);
 // Source selection must scrub the previous field node, not carry typed values.
 const current=formWith(page,'Save source');await current.getByLabel('Signing seed (base64, optional)',{exact:true}).fill(temporary);
 retained=await current.getByLabel('Signing seed (base64, optional)',{exact:true}).elementHandle();
 await selectSource(page,label);await assertDetachedSecretCleared(retained,temporary);await assertSecretAbsent(page,temporary);
 const original=formWith(page,'Save source');await original.getByLabel('Signing seed (base64, optional)',{exact:true}).fill(temporary);
 retained=await original.getByLabel('Signing seed (base64, optional)',{exact:true}).elementHandle();
 await page.locator('a[href="/admin/customers"]').first().click();await page.waitForURL(u=>u.pathname==='/admin/customers');await page.getByRole('heading',{name:'Customers',exact:true}).waitFor();
 await assertDetachedSecretCleared(retained,temporary);await assertSecretAbsent(page,temporary);
 await page.locator('a[href="/admin/signups"]').first().click();await page.getByRole('heading',{name:'Imported early-access',exact:true}).waitFor();await selectSource(page,label);await assertSecretAbsent(page,temporary);
 assert.deepEqual(await safeJSON(await context.request.get(root+'/'+id),200,'Original source after lifecycle checks'),initial);
 console.log('PASS real update400/local validation, tab unmount, create cancel and create201 secret scrubbing; original source unchanged');
}

function binding(detail){
 const {origin,dial_ip,port,source_tenant,source_project,issuer,audience,ca_pem}=detail;
 return {id:detail.summary.id,origin,dial_ip,port,source_tenant,source_project,issuer,audience,ca_pem,paused:detail.summary.paused};
}
async function migration(c){
 const browser=await c.chromium.launch({headless:true});
 const historical=JSON.parse(await readFile(requireEnv('ACQUISITION_TEST_HISTORY_FILE'),'utf8'));
 const baselineFile=requireEnv('ACQUISITION_TEST_BASELINE_FILE');
 let context,page,secret='';
 try{
  ({context,page}=await login(browser,c,'admin'));
  await sourcePage(page);await selectSource(page,historical.label);
  const id=historical.source_id;
  let detail=await safeJSON(await context.request.get(root+'/'+id),200,'Current preserved source');
  assert.equal(detail.summary.id,id);assert.equal(detail.source_tenant,'container');assert.equal(detail.source_project,'signup');
  assert.equal(detail.issuer,'fixture');assert.equal(detail.audience,'source-admin');assert.equal(detail.summary.paused,false);
  if(c.phase==='restore'){
   const baseline=JSON.parse(await readFile(baselineFile,'utf8'));
   assert.deepEqual(binding(detail),binding(baseline.detail),'Restore must preserve exact current binding');
   const beforeLeads=await list(context,id);const beforeCursor=detail.summary.cursor;
   assert.equal((await pull(page,id)).applied,0,'Current exporter must work before restoration');
   assert.deepEqual(await list(context,id),beforeLeads);
   console.log('PASS current saved exporter signer works through actual platform pull before controlled restore');
   const old=createPrivateKey(await readFile(requireEnv('ACQUISITION_TEST_ORIGINAL_SIGNER_FILE')));
   const bytes=Buffer.from(old.export({format:'jwk'}).d,'base64url');secret=bytes.toString('base64');bytes.fill(0);
   const form=formWith(page,'Save source');await fill(form,{'Signing seed (base64, optional)':secret,'Audited reason':'Fixture controlled restoration before clean migration rerun'});
   await safeJSON(await uiResponse(page,'PUT',root+'/'+id,()=>form.getByRole('button',{name:'Save source',exact:true}).click()),200,'Controlled original signer restoration',secret);
   const after=await safeJSON(await context.request.get(root+'/'+id),200,'Restored source');
   assert.deepEqual(binding(after),binding(detail));assert.equal(after.summary.cursor,beforeCursor);assert.deepEqual(await list(context,id),beforeLeads);
   await assertSecretAbsent(page,secret);console.log('PASS controlled original signer restoration; binding/cursor/data preserved');return;
  }
  if(c.phase==='baseline'){
   const before=detail;const beforeLeads=await list(context,id);
   const ca=await readFile(requireEnv('ACQUISITION_TEST_SOURCE_CA_FILE'),'utf8');
   if(ca!==detail.ca_pem){
    const form=formWith(page,'Save source');await fill(form,{'CA certificate (PEM)':ca,'Audited reason':'Fixture certificate-only renewal'});
    const response=await uiResponse(page,'PUT',root+'/'+id,()=>form.getByRole('button',{name:'Save source',exact:true}).click());
    const sent=response.request().postDataJSON();assert.ok(!Object.hasOwn(sent,'signing_key'));assert.equal(sent.expected_config_generation,before.summary.config_generation);
    await safeJSON(response,200,'UI CA-only renewal');
    detail=await safeJSON(await context.request.get(root+'/'+id),200,'CA renewal result');
    assert.deepEqual({...binding(detail),ca_pem:before.ca_pem},binding(before));assert.equal(detail.summary.cursor,before.summary.cursor);
    assert.deepEqual(await list(context,id),beforeLeads);
   }
   const pulled=await pull(page,id);assert.equal(pulled.applied,0,'Historical tombstone baseline must already be imported');
   detail=await safeJSON(await context.request.get(root+'/'+id),200,'Actual pre-cutover source');
   const leads=await list(context,id);assert.ok(!leads.items.some(x=>x.id===historical.lead_id),'Historical deleted lead must remain absent');
   await writeFile(baselineFile,JSON.stringify({detail,leads,historical_lead_id:historical.lead_id},null,2),{flag:'wx',mode:0o600});
   console.log('PASS actual pre-cutover transport/import baseline captured; historical deleted lead remains absent');return;
  }
  const baseline=JSON.parse(await readFile(baselineFile,'utf8'));
  const key=createPrivateKey(await readFile(requireEnv('ACQUISITION_TEST_SIGNER_FILE')));assert.equal(key.asymmetricKeyType,'ed25519');
  const bytes=Buffer.from(key.export({format:'jwk'}).d,'base64url');assert.equal(bytes.length,32);secret=bytes.toString('base64');bytes.fill(0);
  if(c.phase==='resume'){
   assert.deepEqual(binding(detail),binding(baseline.detail),'Resume must preserve exact baseline binding');
   assert.equal(detail.summary.cursor,baseline.detail.summary.cursor,'Resume must not advance/reset cursor');
   assert.ok(detail.summary.config_generation>baseline.detail.summary.config_generation,'Recorded cutover must have advanced generation');
   assert.deepEqual(await list(context,id),baseline.leads,'Resume must preserve baseline lead data');
   assert.equal((await pull(page,id)).applied,0,'Saved exporter must work without replay');
   assert.deepEqual(await list(context,id),baseline.leads);
   await assertSecretAbsent(page,secret);
   console.log('PASS explicit post-cutover resume verified same source/binding/cursor/data and actual exporter pull; prior cutover proof separate');
  }else{
  console.log('CHECK preserved source and old-signer failure');

  if(JSON.stringify(detail)!==JSON.stringify(baseline.detail)){const changed=[];for(const k of Object.keys(detail)){if(k==='summary'){for(const n of Object.keys(detail.summary))if(JSON.stringify(detail.summary[n])!==JSON.stringify(baseline.detail.summary[n]))changed.push('summary.'+n);}else if(JSON.stringify(detail[k])!==JSON.stringify(baseline.detail[k]))changed.push(k);}console.log('DIAGNOSTIC changed safe field names: '+changed.join(','));}
  assert.deepEqual(detail,baseline.detail,'Same source must survive separated process cutover without platform edits');
  const denied=await context.request.post(root+'/'+id+'/sync',{headers:c.headers,data:{reason}});
  expectStatus(denied,[502,503],'Old signer must fail after separated cutover');noSecrets(await denied.json());
  assert.deepEqual(await list(context,id),baseline.leads);
  assert.deepEqual(await safeJSON(await context.request.get(root+'/'+id),200,'Unchanged failed-source state'),baseline.detail);
  console.log('CHECK UI signer-only cutover');
  const form=formWith(page,'Save source');await fill(form,{'Signing seed (base64, optional)':secret,'Audited reason':'Fixture exporter key-only cutover'});
  const response=await uiResponse(page,'PUT',root+'/'+id,()=>form.getByRole('button',{name:'Save source',exact:true}).click());
  const sent=response.request().postDataJSON();assert.equal(sent.expected_config_generation,detail.summary.config_generation,'Submitted generation');
  for(const k of ['label','origin','dial_ip','port','ca_pem','paused']){console.log('CHECK submitted unchanged field '+k);assert.ok(JSON.stringify(sent[k])===JSON.stringify(k==='label'||k==='paused'?detail.summary[k]:detail[k]),'Submitted non-key field mismatch: '+k);}
  assert.ok(sent.signing_key===secret,'Submitted signer mismatch');await safeJSON(response,200,'UI signer-only replacement',secret);
  const rotated=await safeJSON(await context.request.get(root+'/'+id),200,'Signer replacement result');
  assert.deepEqual(binding(rotated),binding(detail));assert.equal(rotated.summary.cursor,detail.summary.cursor);assert.ok(rotated.summary.config_generation>detail.summary.config_generation);
  assert.deepEqual(await list(context,id),baseline.leads);await assertSecretAbsent(page,secret);
  }
  await secretLifecycle(page,context,id,historical.label,secret);
  assert.equal((await pull(page,id)).applied,0,'Key cutover must not replay');
  console.log('CHECK fresh signup on migrated source');
  const email=await seed(browser,c.tls);await pull(page,id);
  const leads=await list(context,id);const lead=leads.items.find(x=>x.email===email);assert.ok(lead);assert.equal(lead.email_verified,false);assert.ok(lead.consent_at&&lead.consent_version);
  assert.ok(!leads.items.some(x=>x.id===baseline.historical_lead_id));
  await page.getByRole('tab',{name:'Leads',exact:true}).click();await page.getByText(email,{exact:true}).waitFor();await page.getByRole('tab',{name:'Source',exact:true}).click();
  assert.equal((await pull(page,id)).applied,0);assert.deepEqual(await list(context,id),leads);
  // Actual transport and signature failures retain the post-migration cursor/data.
  detail=await safeJSON(await context.request.get(root+'/'+id),200,'Migrated negative baseline');
  const update={label:detail.summary.label,origin:detail.origin,dial_ip:detail.dial_ip,port:detail.port,ca_pem:detail.ca_pem,paused:false,reason};
  let changed=await safeJSON(await context.request.put(root+'/'+id,{headers:c.headers,data:{...update,origin:'https://wrong-host.example.test',expected_config_generation:detail.summary.config_generation}}),200,'Wrong trust fixture');
  expectStatus(await context.request.post(root+'/'+id+'/sync',{headers:c.headers,data:{reason}}),[502,503],'TLS mismatch denied');
  assert.equal((await safeJSON(await context.request.get(root+'/'+id),200,'After TLS denial')).summary.cursor,detail.summary.cursor);assert.deepEqual(await list(context,id),leads);
  const wrong=Buffer.from(generateKeyPairSync('ed25519').privateKey.export({format:'jwk'}).d,'base64url').toString('base64');
  changed=await safeJSON(await context.request.put(root+'/'+id,{headers:c.headers,data:{...update,signing_key:wrong,expected_config_generation:changed.config_generation}}),200,'Wrong signer fixture');
  expectStatus(await context.request.post(root+'/'+id+'/sync',{headers:c.headers,data:{reason}}),[502,503],'Wrong signer denied');
  assert.equal((await safeJSON(await context.request.get(root+'/'+id),200,'After signer denial')).summary.cursor,detail.summary.cursor);assert.deepEqual(await list(context,id),leads);
  await safeJSON(await context.request.put(root+'/'+id,{headers:c.headers,data:{...update,signing_key:secret,expected_config_generation:changed.config_generation}}),200,'Restore exporter signer');
  expectStatus(await context.request.post(root+'/'+id+'/sync',{headers:{Origin:'https://foreign.example.test','Sec-Fetch-Site':'cross-site'},data:{reason}}),403,'Cross-origin sync denied');
  detail=await safeJSON(await context.request.get(root+'/'+id),200,'Final preserved migrated source');assert.equal(detail.summary.cursor,changed.cursor);assert.deepEqual(await list(context,id),leads);
  await page.reload();await assertSecretAbsent(page,secret);secret='';
  await writeFile(requireEnv('ACQUISITION_TEST_RECORD_FILE'),JSON.stringify({source_id:id,label:historical.label,email,lead_id:lead.id,lead,total:leads.total,summary:detail.summary,binding:binding(detail),historical_lead_id:baseline.historical_lead_id},null,2),{flag:'wx',mode:0o600});
  await denials(browser,c,id,email,historical.label);
  console.log(c.phase==='resume'?'PASS post-cutover continuation, fresh consent import, repeated pull, trust/signature/origin and reader/customer denials; earlier cutover proof separate':'PASS same-source UI key migration, fresh consent import, repeated pull, trust/signature/origin and reader/customer denials');
 }finally{secret='';await context?.close();await browser.close();}
}
