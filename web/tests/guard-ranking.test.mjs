import test from 'node:test'
import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import vm from 'node:vm'
import { parse, compileScript } from '@vue/compiler-sfc'
import * as vue from 'vue'
import * as shared from '../src/view/live/guard/shared.js'
import * as giftShared from '../src/view/live/gift/shared.js'
const deferred = () => { let resolve,reject; const promise = new Promise((yes,no) => { resolve=yes;reject=no }); return { promise,resolve,reject } }
async function component(path,apis={},confirm=async()=>{},props={}) {
 const filename=new URL(`../src/view/live/${path}.vue`,import.meta.url)
 const {descriptor}=parse(await readFile(filename,'utf8'),{filename:filename.pathname})
 const {content}=compileScript(descriptor,{id:path})
 const cleanups=[]
 const query=async()=>({code:0,data:{list:[],total:'0',actions:{}}})
 const deps={vue:{...vue,onMounted(){},onBeforeUnmount:fn=>cleanups.push(fn)},'element-plus':{ElMessageBox:{confirm},ElMessage:{error(){},warning(){}}},'@/utils/btnAuth':{useBtnAuth:()=>({})},'@/utils/format':{CreateUUID:()=> 'stable_request_01',formatDate:String},'@/api/guard':{guardQuery:query,guardWrite:async()=>({code:0,data:{}}),...apis},'@/api/ranking':{rankingQuery:query,rankingWrite:async()=>({code:0,data:{}}),...apis},'@/api/gift':{giftQuery:query,giftWrite:async()=>({code:0,data:{}}),...apis},'../shared':shared,'./shared':path==='gift/Catalog'?giftShared:shared,'../guard/shared':shared,'../../gift/shared':giftShared,'../gift/shared':giftShared}
 const context=vm.createContext({console,Error,BigInt}),mod=new vm.SourceTextModule(content,{context})
 await mod.link(spec=>{assert.ok(deps[spec],spec);return new vm.SyntheticModule(Object.keys(deps[spec]),function(){for(const[k,v]of Object.entries(deps[spec]))this.setExport(k,v)},{context})});await mod.evaluate()
 const scope=vue.effectScope(),state=scope.run(()=>mod.namespace.default.setup(props,{expose(){}}))
 return {state,close(){cleanups.forEach(fn=>fn());scope.stop()}}
}
test('guard plan freezes writes and reuses permanent key after ambiguous response',async t=>{
 const calls=[];const p=await component('guard/plan/index',{guardWrite:async(path,data)=>{calls.push(JSON.parse(JSON.stringify(data)));if(calls.length===1)throw new Error('lost response');return {code:0,data:{}}}});t.after(p.close);const s=p.state
 s.open();s.form.value.planNo='original';s.form.value.name='月守护';await s.save();assert.ok(s.attempt.value)
 s.form.value.planNo='changed';s.open({planNo:'other'});await s.save();assert.deepEqual(calls[1],calls[0]);assert.equal(calls[0].planNo,'original');assert.equal(s.attempt.value,null)
})
test('guard plan form never copies database audit or identity fields',()=>{
 const form=shared.planForm({planNo:'p',name:'plan',id:7,createdBy:1,createdAt:100,version:'9007199254740993'})
 assert.equal(form.version,'9007199254740993');assert.equal('id' in form,false);assert.equal('createdBy' in form,false)
})
test('guard pages preserve large totals and bound only the numeric pager',async t=>{
 const total='18446744073709551614'
 const p=await component('guard/plan/index',{guardQuery:async()=>({code:0,data:{list:[],total}})});t.after(p.close)
 await p.state.load();assert.equal(p.state.total.value,total)
 assert.equal(shared.pageTotal(p.state.total.value,20),200000)
 assert.equal(shared.pageTotal('19',20),19)
 assert.equal(shared.pageTotal('-1',20),0)
})
test('guard member rebuild freezes confirmed target and rejects double submit',async t=>{
 const confirmation=deferred(),calls=[];const p=await component('guard/Records',{guardWrite:async(path,data)=>{calls.push({path,data});return {code:0,data:{rebuilt:true}}}},()=>confirmation.promise,{kind:'member'});t.after(p.close);const s=p.state
 const work=s.act('rebuild','member_original');await s.act('rebuild','member_other');confirmation.resolve();await work
 assert.equal(calls.length,1);assert.equal(calls[0].data.number,'member_original')
})
test('ranking ignores stale pages and preserves decimal strings',async t=>{
 const a=deferred(),b=deferred();let calls=0;const p=await component('ranking/index',{rankingQuery:()=>++calls===1?a.promise:b.promise});t.after(p.close);const s=p.state
 const first=s.load(),second=s.load();b.resolve({code:0,data:{list:[{score:'18446744073709551614'}],total:'1'}});await second;a.resolve({code:0,data:{list:[],total:'0'}});await first
 assert.equal(s.board.value.list[0].score,'18446744073709551614')
})
test('ranking rebuild retries original key after response loss',async t=>{
 const calls=[];const p=await component('ranking/index',{rankingWrite:async(path,data)=>{calls.push({path,data});if(calls.length===1)throw new Error('lost');return {code:0,data:{jobNo:'job1'}}}});t.after(p.close);const s=p.state
 await s.act('rebuild');await s.act('reconcile');assert.deepEqual(calls[0],calls[1]);assert.equal(calls[1].path,'rebuild')
})
test('gift editor fails closed when provider readiness is unavailable',async t=>{
 const p=await component('gift/Catalog',{guardQuery:async()=>({code:0,data:{list:[{planNo:'p',name:'守护'}],total:'1',guardEnabled:false}})},async()=>{},{kind:'item'});t.after(p.close)
 await p.state.loadGuardPlans();assert.equal(p.state.guardEnabled.value,false);assert.equal(p.state.guardPlans.value[0].planNo,'p')
})
