import test from 'node:test'
import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import vm from 'node:vm'
import { parse, compileScript, compileTemplate } from '@vue/compiler-sfc'
import * as vue from 'vue'
import * as form from '../src/view/live/earning/settlement-form.js'
import * as shared from '../src/view/live/earning/shared.js'
const rule = { ruleNo:'er_example', name:'基础兑换', sourceUnit:'COIN', currency:'CNY', coinUnits:'1000', minorUnits:'100', status:'active', effectiveFrom:1, effectiveTo:4102444800000 }
const deferred=()=>{let resolve;const promise=new Promise(r=>{resolve=r});return {promise,resolve}}
async function dialog(api={},confirm=async()=>{}) {
  const source=await readFile(new URL('../src/view/live/earning/SettlementCreateDialog.vue',import.meta.url),'utf8')
  const {descriptor}=parse(source), {content}=compileScript(descriptor,{id:'settlement-test'}), cleanups=[], events=[]
  assert.deepEqual(compileTemplate({source:descriptor.template.content,filename:'SettlementCreateDialog.vue',id:'settlement-test'}).errors,[])
  const deps={vue:{...vue,onBeforeUnmount(fn){cleanups.push(fn)}},'element-plus':{ElMessage:{success(){},warning(){}},ElMessageBox:{confirm}},'@/api/earning':{earningQuery:async()=>({code:0,data:{list:[rule],total:'1'}}),earningWrite:async()=>({code:0,data:{settlementNo:'st_result'}}),...api},'@/utils/format':{CreateUUID:()=> 'stable_request_001',formatDate:String},'./shared':shared,'./settlement-form':form}
  const context=vm.createContext({console,Error,BigInt}), module=new vm.SourceTextModule(content,{context})
  await module.link(spec=>{assert.ok(deps[spec],spec);return new vm.SyntheticModule(Object.keys(deps[spec]),function(){for(const [key,value] of Object.entries(deps[spec]))this.setExport(key,value)},{context})});await module.evaluate()
  const scope=vue.effectScope(),state=scope.run(()=>module.namespace.default.setup({},{expose(){},emit:(...args)=>events.push(args)}))
  return {state,events,close(){cleanups.forEach(fn=>fn());scope.stop()}}
}
test('recent seven days end at current second; boundary and invalid inputs fail before write',()=>{
  const now=Date.now(), range=form.recentSettlementPeriod(now)
  assert.equal(range.periodEnd%1000,0);assert.equal(range.periodEnd-range.periodStart,604800000)
  const draft={requestId:'request01',anchorNo:'1100003',ruleNo:rule.ruleNo,...range,amount:'forged'}
  assert.equal(form.settlementPayload(draft,rule,now).amount,undefined)
  for(const periodEnd of ['',null,NaN,Infinity,range.periodStart,form.currentSecond(now)+1000]) assert.throws(()=>form.settlementPayload({...draft,periodEnd},rule,now))
  for(const badRule of [null,{...rule,status:'pending'},{...rule,effectiveFrom:now+1},{...rule,effectiveTo:now},{...rule,ruleNo:'other'}]) assert.throws(()=>form.settlementPayload(draft,badRule,now))
  const times=form.futureTimes(now,now), date=new Date(now)
  assert.ok(!times.hours().includes(date.getHours()));assert.ok(!times.seconds(date.getHours(),date.getMinutes()).includes(date.getSeconds()))
  assert.deepEqual(form.futureTimes(now-86400000,now).hours(),[])
})
test('anchor selection is pinned; rules paginated; valid selection creates only a settlement request',async t=>{
  const writes=[],queries=[],p=await dialog({earningQuery:async(path,params)=>{queries.push({path,params});return {code:0,data:{list:[rule],total:'21'}}},earningWrite:async(path,data)=>{writes.push({path,data});return {code:0,data:{settlementNo:'st_result'}}}});t.after(p.close)
  const s=p.state;await s.open('1100003');assert.equal(s.pinnedAnchor.value,true);assert.equal(s.draft.value.anchorNo,'1100003');assert.equal(s.draft.value.periodEnd-s.draft.value.periodStart,604800000)
  s.draft.value.ruleNo=rule.ruleNo;s.selectRule(rule.ruleNo);await s.loadRules(2);await s.submit()
  assert.equal(queries[1].params.page,2);assert.equal(writes[0].path,'settlement/create');assert.equal(writes[0].data.anchorNo,'1100003');assert.equal(p.events.length,1);assert.equal(s.visible.value,false)
})
test('confirmation prevents duplicate clicks and freezes the chosen anchor and interval',async t=>{
  const pending=deferred(),writes=[],p=await dialog({earningWrite:async(path,data)=>{writes.push(data);return {code:0,data:{}}}},()=>pending.promise);t.after(p.close)
  const s=p.state;await s.open('1100003');s.draft.value.ruleNo=rule.ruleNo;s.selectRule(rule.ruleNo);const end=s.draft.value.periodEnd
  const task=s.submit();s.open('another');s.draft.value.periodEnd=1;await s.submit();pending.resolve();await task
  assert.equal(writes.length,1);assert.equal(writes[0].anchorNo,'1100003');assert.equal(writes[0].periodEnd,end)
})
test('lost response is recovered using original key even after reopening on a different row',async t=>{
  const writes=[],p=await dialog({earningWrite:async(path,data)=>{writes.push(JSON.parse(JSON.stringify(data)));if(writes.length===1)throw new Error('lost response');return {code:0,data:{}}}});t.after(p.close)
  const s=p.state;await s.open('1100003');s.draft.value.ruleNo=rule.ruleNo;s.selectRule(rule.ruleNo);await s.submit();assert.equal(s.submitted.value,true)
  s.visible.value=false;await s.open('another');assert.equal(s.draft.value.anchorNo,'1100003');s.draft.value.ruleNo='tampered';await s.submit();assert.deepEqual(writes[0],writes[1]);assert.equal(s.submitted.value,false)
})
test('old rule request cannot overwrite new row and denied loading never creates a settlement',async t=>{
  const old=deferred();let queries=0,writes=0
  const p=await dialog({earningQuery:async()=>++queries===1?old.promise:{code:49002,msg:'无权查询兑换规则'},earningWrite:async()=>{writes++;return {code:0}}});t.after(p.close)
  const s=p.state,task=s.open('1100003');await s.open('1100004');old.resolve({code:0,data:{list:[rule],total:'1'}});await task
  assert.equal(s.rules.value.length,0);assert.match(s.rulesError.value,/无权/);await s.submit();assert.equal(writes,0)
})
test('anchor and finance pages both integrate the same dialog; session shows gross and net quantities',async()=>{
  for(const path of ['anchor/index.vue','earning/FinancePage.vue']) {const source=await readFile(new URL(`../src/view/live/${path}`,import.meta.url),'utf8');assert.match(source,/<SettlementCreateDialog/);const {descriptor}=parse(source);assert.deepEqual(compileTemplate({source:descriptor.template.content,filename:path,id:path}).errors,[])}
  const session=await readFile(new URL('../src/view/live/session/index.vue',import.meta.url),'utf8');for(const field of ['grossQuantity','refundQuantity','quantity','grossCoin','refundCoin','netCoin'])assert.ok(session.includes('row.giftSummary.'+field))
})
