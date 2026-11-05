import test from 'node:test'
import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import vm from 'node:vm'
import { parse,compileScript } from '@vue/compiler-sfc'
import * as vue from 'vue'
import * as shared from '../src/view/payout/shared.js'
import * as earning from '../src/view/live/earning/shared.js'
const defer=()=>{let resolve,reject;const promise=new Promise((a,b)=>{resolve=a;reject=b});return {promise,resolve,reject}}
const row={withdrawalNo:'wd_0123456789abcdef0123456789abcdef',anchorNo:'anchor1',status:'approved',reconciliationStatus:'passed',payeeRef:'pa_0123456789abcdef0123456789abcdef',payeeVersion:'1',payeeMethod:'bank_card',accountMasked:'****1234',amount:'9007199254740993',currency:'CNY'}
const order={payoutNo:'po_0123456789abcdef0123456789abcdef',businessNo:row.withdrawalNo,status:'ready'}
async function component(file,apis={}){
 const source=await readFile(new URL('../src/view/payout/'+file,import.meta.url),'utf8'),{descriptor}=parse(source),{content}=compileScript(descriptor,{id:'payout-test'})
 const user=vue.reactive({token:'admin-token'}),events=[],cleanup=[]
 const dependencies={vue:{...vue,onMounted(){},onBeforeUnmount(fn){cleanup.push(fn)}},'@/utils/format':{CreateUUID:()=> 'payout_stable_request',formatDate:String},'@/pinia/modules/user':{useUserStore:()=>user},'@/api/payout':{getPayoutCapabilities:async()=>({code:0,data:{'order/detail':true}}),createPayoutOrder:async()=>({code:0,data:order}),listPayoutOrders:async()=>({code:0,data:{list:[],total:'0'}}),getPayoutOrder:async()=>({code:0,data:{order}}),...apis},'@/view/live/earning/shared':earning,'./shared':shared,'../shared':shared,'../ManualActions.vue':{default:{}},'@/view/live/earning/SnapshotPanel.vue':{default:{}}}
 const context=vm.createContext({console,Error}),module=new vm.SourceTextModule(content,{context})
 await module.link(name=>{const d=dependencies[name];assert.ok(d,name);return new vm.SyntheticModule(Object.keys(d),function(){for(const [k,v] of Object.entries(d))this.setExport(k,v)},{context})});await module.evaluate()
 const scope=vue.effectScope(),state=scope.run(()=>module.namespace.default.setup({}, {expose(){},emit:(...a)=>events.push(a)}))
 return {state,user,events,source,close(){cleanup.forEach(f=>f());scope.stop()}}
}
test('submission freezes original parameters, blocks double-click, retries lost response',async t=>{
 const calls=[],slow=defer();const p=await component('SubmitOrderDialog.vue',{createPayoutOrder:async body=>{calls.push({...body});return calls.length===1?slow.promise:{code:0,data:order}}});t.after(p.close)
 p.state.open(row);const work=p.state.submit();p.state.open({...row,withdrawalNo:'other'});await p.state.submit();assert.equal(calls.length,1)
 slow.reject(new Error('raw private internal SQL'));await work;assert.doesNotMatch(p.state.error.value,/SQL/);assert.equal(p.state.attempted.value,true)
 await p.state.submit();assert.deepEqual(calls[0],calls[1]);assert.deepEqual(Object.keys(calls[0]),['requestId','withdrawalNo']);assert.equal(p.events.length,1);assert.equal(p.state.result.value.status,'ready')
 assert.match(p.source,/本操作只创建待出款订单，不会实际打款/)
})
test('logout discards an in-flight order response and sensitive display state',async t=>{
 const slow=defer();const p=await component('SubmitOrderDialog.vue',{createPayoutOrder:()=>slow.promise});t.after(p.close);p.state.open(row);const work=p.state.submit();p.user.token='new-user';await vue.nextTick();slow.resolve({code:0,data:order});await work
 assert.equal(p.state.target.value,null);assert.equal(p.state.result.value,null);assert.equal(p.events.length,0)
})
test('order lists and details ignore stale responses and support precise filters',async t=>{
 const a=defer(),b=defer(),c=defer();let n=0;const params=[]
 const p=await component('order/index.vue',{listPayoutOrders:q=>{params.push(q);return ++n===1?a.promise:b.promise},getPayoutOrder:()=>c.promise});t.after(p.close)
 p.state.query.withdrawalNo=row.withdrawalNo;p.state.range.value=['100','200'];const old=p.state.load(),latest=p.state.load();b.resolve({code:0,data:{list:[order],total:'1'}});await latest;a.resolve({code:0,data:{list:[],total:'0'}});await old
 assert.equal(p.state.rows.value.length,1);assert.equal(params[0].withdrawalNo,row.withdrawalNo);assert.equal(params[0].from,100)
 const detail=p.state.inspect(order);p.state.closeDetail();c.resolve({code:0,data:{order}});await detail;assert.equal(p.state.detail.value,null)
 assert.doesNotMatch(p.source,/确认成功|确认失败|重新打款|取消订单|encryptedPayload|accountFingerprint/)
})
test('business errors use fixed messages and creation excludes non-approved/legacy orders',()=>{
 assert.doesNotMatch(shared.payoutError({code:49203,msg:'DB password private'}),/password|private/)
 for(const patch of [{status:'pending_review'},{status:'rejected'},{payeeRef:'payout_center_pending'},{reconciliationStatus:'abnormal'},{payoutNo:order.payoutNo}])assert.equal(shared.canSubmitPayout({...row,...patch}),false)
})
