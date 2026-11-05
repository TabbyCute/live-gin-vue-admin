import test from 'node:test'
import assert from 'node:assert/strict'
import {readFile} from 'node:fs/promises'
import vm from 'node:vm'
import {parse,compileScript} from '@vue/compiler-sfc'
import * as vue from 'vue'
import * as shared from '../src/view/live/gift/shared.js'
const deferred=()=>{let resolve,reject;const promise=new Promise((yes,no)=>{resolve=yes;reject=no});return {promise,resolve,reject}}
async function page(name,api={},confirm=async()=>{}){
 const filename=new URL(`../src/view/live/gift/${name}/index.vue`,import.meta.url)
 const {descriptor}=parse(await readFile(filename,'utf8'),{filename:filename.pathname})
 const {content}=compileScript(descriptor,{id:`gift-${name}`})
 const cleanups=[]
 const deps={vue:{...vue,onMounted(){},onBeforeUnmount:fn=>cleanups.push(fn)},'vue-router':{useRouter:()=>({push(){}})},'element-plus':{ElMessage:{error(){},warning(){}},ElMessageBox:{confirm}},'@/api/gift':{giftQuery:async()=>({code:0,data:{list:[],total:'0'}}),giftWrite:async()=>({code:0,data:{}}),...api},'@/utils/format':{formatDate:String,CreateUUID:()=> 'fixed-refund-request'},'../shared':shared}
 const context=vm.createContext({console,Error,BigInt})
 const mod=new vm.SourceTextModule(content,{context})
 await mod.link(spec=>{assert.ok(deps[spec],spec);return new vm.SyntheticModule(Object.keys(deps[spec]),function(){for(const [key,value] of Object.entries(deps[spec]))this.setExport(key,value)},{context})});await mod.evaluate()
 const scope=vue.effectScope(),state=scope.run(()=>mod.namespace.default.setup({}, {expose(){}}))
 return {state,close(){cleanups.forEach(fn=>fn());scope.stop()}}
}
const order={orderNo:'go_original',unitPrice:'9007199254740993',grossCoinAmount:'18014398509481986',refundedCoinAmount:'0'}
test('actual refund page freezes confirmed request and reuses key after lost response',async(t)=>{
 const confirmation=deferred(),calls=[]
 const p=await page('order',{giftWrite:async(path,data)=>{calls.push(JSON.parse(JSON.stringify(data)));if(calls.length===1)throw new Error('response lost');return {code:0,data:{}}}},()=>confirmation.promise);t.after(p.close);const s=p.state
 s.openRefund(order);s.refundDraft.value.quantity='1';s.refundDraft.value.reason='测试部分退款'
 const work=s.refund();assert.equal(s.writing.value,true)
 // Programmatic mutation simulates a stale event while the dialog is awaiting confirmation.
 s.refundDraft.value.quantity='2';s.openRefund({...order,orderNo:'go_other'})
 confirmation.resolve();await work
 assert.equal(calls[0].quantity,'1');assert.equal(calls[0].orderNo,'go_original');assert.equal(s.submitted.value,true)
 await s.refund();assert.deepEqual(calls[1],calls[0]);assert.equal(s.submitted.value,false)
})
test('actual order page rejects stale list responses',async(t)=>{
 const old=deferred(),latest=deferred();let calls=0
 const p=await page('order',{giftQuery:()=>++calls===1?old.promise:latest.promise});t.after(p.close);const s=p.state
 const first=s.load();s.query.accountNo='new_user';const second=s.load()
 latest.resolve({code:0,data:{list:[{orderNo:'new'}],total:'1'}});await second
 old.resolve({code:0,data:{list:[{orderNo:'old'}],total:'1'}});await first
 assert.equal(s.rows.value[0].orderNo,'new')
})
test('actual rule page rejects out-of-range basis points before HTTP write',async(t)=>{
 let writes=0;const p=await page('rule',{giftWrite:async()=>{writes++;return {code:0,data:{}}}});t.after(p.close);const s=p.state
 s.open();s.form.value.anchorShareBps='10001';await s.create();assert.equal(writes,0);assert.ok(s.writeError.value)
})
test('termination freezes confirmed request and reuses idempotency key after response loss',async(t)=>{
 const confirm=deferred(),calls=[]
 const p=await page('rule',{giftWrite:async(path,data)=>{calls.push({path,data:JSON.parse(JSON.stringify(data))});if(calls.length===1)throw new Error('lost response');return {code:0,data:{}}}},()=>confirm.promise);t.after(p.close);const s=p.state
 s.openTermination({ruleNo:'rr_original'});s.terminationDraft.value.reason='分成配置有误'
 const work=s.terminate();assert.equal(s.writing.value,true)
 s.terminationDraft.value.reason='changed';s.openTermination({ruleNo:'rr_other'})
 confirm.resolve();await work
 assert.equal(calls[0].data.ruleNo,'rr_original');assert.equal(calls[0].data.reason,'分成配置有误');assert.equal(s.terminationSubmitted.value,true)
 await s.terminate();assert.deepEqual(calls[1],calls[0]);assert.equal(s.terminationSubmitted.value,false)
})
test('termination review cannot switch target or double-submit while confirming',async(t)=>{
 const confirm=deferred(),calls=[]
 const p=await page('rule',{giftWrite:async(path,data)=>{calls.push({path,data});return {code:0,data:{}}}},()=>confirm.promise);t.after(p.close);const s=p.state
 const row={termination:{terminationNo:'rt_original'}}
 const work=s.reviewTermination(row,true);assert.equal(s.writing.value,true)
 row.termination.terminationNo='rt_other';await s.reviewTermination(row,false);confirm.resolve();await work
 assert.equal(calls.length,1);assert.equal(calls[0].path,'rule/terminate/review');assert.equal(calls[0].data.terminationNo,'rt_original');assert.equal(calls[0].data.approve,true)
})
