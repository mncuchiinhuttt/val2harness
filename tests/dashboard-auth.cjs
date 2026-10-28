// Exercises client request/credential behavior; the Go/browser suites use real HTTP.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
class Element {
  constructor(id='') { this.id=id; this.events={};this.children=[];this.style={};this.value='';this.classList={add(){},remove(){}};this.disabled=false;this.hidden=false; }
  addEventListener(name,fn){this.events[name]=fn;}
  appendChild(child){this.children.push(child);return child;}
  append(...children){this.children.push(...children);}
  contains(){return false;} focus(){} getAttribute(){return '';} querySelectorAll(){return [];}
}
async function boot(hash='', quotaStatus='unknown') {
  const elements=new Map(),events={},requests=[],stored=new Map();let cleared=false;
  const doc={getElementById(id){if(!elements.has(id))elements.set(id,new Element(id));return elements.get(id);},createElement(){return new Element();},querySelectorAll(){return [];},querySelector(){return null;},addEventListener(k,fn){events[k]=fn;}};
  const history={replaceState(){cleared=true;}};
  const context={document:doc,Headers,URLSearchParams,URL,TextDecoder,AbortController,console,setTimeout:()=>0,clearTimeout(){},sessionStorage:{getItem:k=>stored.get(k)||null,setItem:(k,v)=>stored.set(k,v),removeItem:k=>stored.delete(k)},history,navigator:{clipboard:{writeText:async()=>{}}},window:{history,addEventListener(){},location:{hash,origin:'http://127.0.0.1:8765',pathname:'/',search:'',toString(){return this.origin+'/'+this.search+this.hash;}},open(){}}};
  const malicious='<img src=x onerror=alert(1)>';
  const summary={total_requests:2,measured_requests:1,unmeasured_requests:1,total_tokens:0,prompt_tokens:0,completion_tokens:0,active_days:1,avg_latency_ms:5,daily_records:{},daily_tokens:{},available_years:[2026],models_breakdown:[{model:malicious,requests:2,prompt_tokens:0,completion_tokens:0,total_tokens:0,measured_requests:1,unmeasured_requests:1}],recent_logs:[{timestamp:1,model:malicious,usage_known:false,prompt_tokens:null,completion_tokens:null,total_tokens:null,latency_ms:5},{timestamp:1,model:'measured zero',usage_known:true,prompt_tokens:0,completion_tokens:0,total_tokens:0,latency_ms:5}]};
  const fetch=async(url,opts={})=>{
    requests.push({url,opts,header:new Headers(opts.headers).get('Authorization'),cleared});
    let data={};
    if(url==='/api/local/session-ticket') data={ticket:'one-use-ticket'};
    if(url==='/api/local/session') data={token:'browser-session'};
    if(url==='/api/local/status') data={token_configured:true,daemon_running:false,upstream:'http://synthetic.invalid',listen:'127.0.0.1:9999'};
    if(url==='/api/local/quota') data={status:quotaStatus,message:'Observed response',source:'gateway_session',observed_at:null,observed_requests:2,measured_requests:1,unmeasured_requests:1,prompt_tokens:0,completion_tokens:0,total_tokens:0,remaining:null,limit:null,reset_at:null};
    if(url==='/api/local/analytics') data=summary;
    if(url==='/api/local/analytics/live') {
      const bytes=new TextEncoder().encode('data: '+JSON.stringify(summary)+'\n\n');let offset=0;
      return {ok:true,status:200,body:{getReader(){return {async read(){if(offset>=bytes.length)return {done:true};const value=bytes.slice(offset,offset+7);offset+=7;return {done:false,value};},async cancel(){},releaseLock(){}};}}};
    }
    if(url==='/api/local/harnesses') data=[];
    if(url==='/v1/models') data={data:[{id:'<img src=x onerror=alert(1)>'}]};
    if(url==='/api/local/export') return {ok:false,status:500,json:async()=>({status:'partial',count:1,dir:'exports',error:'Synthetic write failure'})};
    return {ok:true,status:200,json:async()=>data,text:async()=>"export OPENAI_API_KEY='local-owner'\n"};
  };
  context.fetch=fetch;context.window.fetch=fetch;
  vm.runInNewContext(fs.readFileSync('internal/web/static/app.js','utf8'),context);
  await events.DOMContentLoaded();await new Promise(setImmediate);await new Promise(setImmediate);
  return {elements,requests,stored};
}
(async()=>{
  const locked=await boot();assert.equal(locked.requests.length,0,'locked dashboard requested owner data');
  const input=locked.elements.get('gatewayKeyInput');assert.ok(input,'missing unlock input');input.value='local-owner';
  await locked.elements.get('unlockGatewayBtn').events.click();await new Promise(setImmediate);await new Promise(setImmediate);
  assert.equal(locked.requests.find(r=>r.url==='/api/local/session-ticket')?.header,'Bearer local-owner');
  assert.equal(input.value,'','permanent key left in input');
  assert.equal(locked.requests.find(r=>r.url==='/api/local/status')?.header,'Bearer browser-session');
  assert.ok(![...locked.stored.values()].includes('local-owner'),'permanent key persisted in browser storage');
  const linked=await boot('#session=one-use-ticket');
  assert.ok(linked.requests.find(r=>r.url==='/api/local/session')?.cleared,'sign-in fragment not erased before request');
  assert.equal(linked.requests.find(r=>r.url==='/v1/models')?.header,'Bearer browser-session');
  const option=linked.elements.get('selectOptionsList').children[0];
  assert.ok(!option.innerHTML?.includes('<img'),'upstream model ID inserted into HTML');
  await linked.elements.get('exportChatsBtn').events.click();
  const exportToast = JSON.stringify(linked.elements.get('toastContainer').children);
  assert.ok(!exportToast.includes('Export Complete'),'failed export shown as complete');
  assert.ok(exportToast.includes('1 chats fully saved'),'partial export count hidden');
  assert.ok(!linked.requests.some(r=>r.url==='/api/local/version'||r.url==='/api/local/update'),'page load contacted release/update service');
  assert.ok(!linked.requests.some(r=>r.url==='/v1/chat/completions'),'page load sent inference');
  assert.equal(linked.requests.find(r=>r.url==='/api/local/quota')?.header,'Bearer browser-session','quota bypassed owner session');
  assert.equal(linked.requests.find(r=>r.url==='/api/local/analytics/live')?.header,'Bearer browser-session','live analytics bypassed owner session');
  assert.ok(linked.elements.get('quotaStatusText').textContent.includes('Unknown'),'unknown quota shown as success');
  assert.equal(linked.elements.get('quotaRemainingText').textContent,'Unknown');
  assert.equal(linked.elements.get('quotaResetText').textContent,'Unknown');
  assert.equal(linked.elements.get('quotaObservedText').textContent,'No response observed yet');
  assert.equal(linked.elements.get('accountName').textContent,'Token configured','token presence was presented as live connectivity');
  assert.equal(linked.elements.get('tokenExp').textContent,'Expires: Unknown');
  assert.equal(linked.elements.get('localProxy').textContent,'127.0.0.1:9999');
  assert.equal(linked.elements.get('valUpstream').textContent,'http://synthetic.invalid');
  const cells=linked.elements.get('recentLogsBody').children.flatMap(row=>row.children.map(cell=>cell.textContent));
  assert.ok(cells.some(text=>text==='Unknown'),'missing measured usage was rendered as zero');
  assert.ok(cells.some(text=>text==='0'),'measured zero usage was lost');
  assert.ok(!linked.elements.get('recentLogsBody').children.some(row=>row.innerHTML?.includes('<img')),'upstream analytics model inserted as HTML');
  const before=linked.requests.length;
  await linked.elements.get('checkQuotaBtn').events.click();await new Promise(setImmediate);
  assert.deepEqual(linked.requests.slice(before).map(r=>r.url),['/api/local/quota'],'refresh did more than read observations');
  await linked.elements.get('clearObservationsBtn').events.click();await new Promise(setImmediate);
  assert.ok(linked.requests.some(r=>r.url==='/api/local/analytics'&&r.opts.method==='DELETE'&&r.header==='Bearer browser-session'),'clear observations not owner-authenticated');
  for(const state of ['available','exhausted','rate_limited','authentication_error','provider_error','future_unknown_state']) {
    const fixture=await boot('#session=one-use-ticket',state);
    assert.ok(fixture.elements.get('quotaStatusText').textContent.length,'missing state label '+state);
    if(state!=='available') assert.ok(!fixture.elements.get('quotaStatusText').textContent.includes('Available'),'unexpected state shown available '+state);
  }
  console.log('Dashboard lock, owner-authenticated passive/live reads, safe analytics, unknown/zero usage, refresh, clear, and explicit release controls passed.');
})().catch(error=>{console.error(error.message);process.exitCode=1;});
