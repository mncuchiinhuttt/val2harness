// Exercises client request/credential behavior; the Go/browser suites use real HTTP.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
class Element {
  constructor(id='') { this.id=id; this.events={};this.children=[];this.style={};this.value='';this.classList={add(){},remove(){}};this.disabled=false;this.hidden=false; }
  addEventListener(name,fn){this.events[name]=fn;}
  appendChild(child){this.children.push(child);return child;}
  append(...children){this.children.push(...children);}
  contains(){return false;} focus(){} getAttribute(){return '';}
}
async function boot(hash='') {
  const elements=new Map(),events={},requests=[],stored=new Map();let cleared=false;
  const doc={getElementById(id){if(!elements.has(id))elements.set(id,new Element(id));return elements.get(id);},createElement(){return new Element();},querySelectorAll(){return [];},addEventListener(k,fn){events[k]=fn;}};
  const context={document:doc,Headers,URLSearchParams,URL,TextDecoder,console,setTimeout:()=>0,sessionStorage:{getItem:k=>stored.get(k)||null,setItem:(k,v)=>stored.set(k,v),removeItem:k=>stored.delete(k)},history:{replaceState(){cleared=true;}},navigator:{clipboard:{writeText:async()=>{}}},window:{location:{hash,origin:'http://127.0.0.1:8765',pathname:'/',search:''},open(){}}};
  const fetch=async(url,opts={})=>{
    requests.push({url,opts,header:new Headers(opts.headers).get('Authorization'),cleared});
    let data={};
    if(url==='/api/local/session-ticket') data={ticket:'one-use-ticket'};
    if(url==='/api/local/session') data={token:'browser-session'};
    if(url==='/api/local/status') data={token_configured:false,daemon_running:false};
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
  console.log('Dashboard lock, ticket exchange, credential isolation, and safe model rendering passed.');
})().catch(error=>{console.error(error.message);process.exitCode=1;});
