import test from 'node:test'
import assert from 'node:assert/strict'
import {readFile} from 'node:fs/promises'
import vm from 'node:vm'
import {parse,compileScript,compileTemplate} from '@vue/compiler-sfc'
import * as vue from 'vue'
import * as shared from '../src/view/payout/shared.js'
const defer=()=>{let resolve,reject;const promise=new Promise((a,b)=>{resolve=a;reject=b});return {promise,resolve,reject}}
const detail=()=>({order:{payoutNo:'po_test',currency:'CNY'},attempts:[{attemptNo:'pat_test'}],allowedActions:{'attempt/submit':true,'evidence/upload':true},investigations:[]})
async function component(file,props={},apis={}){
 const source=await readFile(new URL('../src/view/payout/'+file,import.meta.url),'utf8'),{descriptor}=parse(source)
 const template=compileTemplate({source:descriptor.template.content,filename:file,id:'manual-test'});assert.deepEqual(template.errors,[])
 const {content}=compileScript(descriptor,{id:'manual-test'}),cleanup=[],timers=new Map(),revoked=[],events=[]
 const user=vue.reactive({token:'admin',userInfo:{ID:3,authorityId:888}});let key=0
 const deps={vue:{...vue,onMounted(){},onBeforeUnmount(fn){cleanup.push(fn)},onDeactivated(fn){cleanup.push(fn)}},'@/utils/format':{CreateUUID:()=>`manual-key-${++key}`},'@/pinia/modules/user':{useUserStore:()=>user},'@/api/payout':{manualPayout:async()=>({code:0,data:{}}),getPayoutCapabilities:async()=>({code:0,data:{}}),listCompanyAccounts:async()=>({code:0,data:{list:[],total:'0'}}),readPayoutEvidence:async()=>new Blob(['test'],{type:'image/png'}),...apis},'./shared':shared,'../shared':shared,'./SensitiveReveal.vue':{default:{}},'../SensitiveReveal.vue':{default:{}}}
 const context=vm.createContext({console,Error,Blob,Uint8Array,btoa,setTimeout(fn,ms){timers.set(fn,ms);return fn},clearTimeout(fn){timers.delete(fn)},URL:{createObjectURL:()=> 'blob:private-test',revokeObjectURL:url=>revoked.push(url)}})
 const module=new vm.SourceTextModule(content,{context});await module.link(name=>{const d=deps[name];assert.ok(d,name);return new vm.SyntheticModule(Object.keys(d),function(){for(const[k,v]of Object.entries(d))this.setExport(k,v)},{context})});await module.evaluate()
 const scope=vue.effectScope(),reactiveProps=vue.reactive(props),state=scope.run(()=>module.namespace.default.setup(reactiveProps,{expose(){},emit:(...v)=>events.push(v)}))
 return {state,user,props:reactiveProps,events,timers,revoked,close(){cleanup.forEach(f=>f());scope.stop()}}
}
test('manual requests never accept client money or payee, and preserve integer strings',()=>{
 const body=shared.manualBody('attempt/submit',{amount:'1',account:'secret',paymentReference:'BANK-REAL-1',paymentTime:'123',evidenceNo:'pev_test',reason:'核验真实银行记录',confirm:true},detail(),'key')
 assert.deepEqual(Object.keys(body),['requestId','attemptNo','paymentReference','paymentTime','evidenceNo','operatorNote','confirmRealTransfer']);assert.equal(body.paymentTime,123)
 assert.equal(Object.keys(shared.payoutStates).length,7)
 assert.doesNotMatch(shared.manualError({code:49209,msg:'private full account SQL'}),/private|SQL/)
})
test('manual submit blocks double click, freezes original request, retries ambiguous response',async t=>{
 const calls=[],slow=defer(),p=await component('ManualActions.vue',{detail:detail()},{manualPayout:(path,body)=>{calls.push([path,JSON.stringify(body)]);return calls.length===1?slow.promise:Promise.resolve({code:0,data:{}})}});t.after(p.close)
 await p.state.open('attempt/submit','提交');Object.assign(p.state.form,{reason:'核验测试记录',confirm:true,paymentReference:'BANK-REFERENCE',paymentTime:'123',evidenceNo:'pev_test'})
 const work=p.state.submit();await p.state.submit();assert.equal(calls.length,1);slow.reject(new Error('secret SQL'));await work;assert.doesNotMatch(p.state.error.value,/secret|SQL/)
 p.state.form.paymentReference='FORGED-AFTER-ATTEMPT';await p.state.submit();assert.deepEqual(calls[0],calls[1]);assert.equal(p.events.length,1)
})
test('manual operations ignore completion after account switches',async t=>{
 const slow=defer(),p=await component('ManualActions.vue',{detail:detail()},{manualPayout:()=>slow.promise});t.after(p.close)
 await p.state.open('attempt/submit','提交');Object.assign(p.state.form,{reason:'核验测试记录',confirm:true});const work=p.state.submit();p.user.userInfo.ID=4;await vue.nextTick();slow.resolve({code:0,data:{}});await work
 assert.equal(p.events.length,0);assert.equal(p.state.pending.value,null);assert.equal(p.state.form.reason,'')
})
test('reveal consumes a capability once and clears plaintext after 30 seconds',async t=>{
 const calls=[],p=await component('SensitiveReveal.vue',{allowed:true,targetNo:'po_test',company:false},{manualPayout:async(path,body)=>{calls.push({path,body});return {code:0,data:path.endsWith('-grant')?{grantNo:'grant',token:'capability'}:{account:'SENSITIVE-ACCOUNT',accountName:'Sensitive'}}}});t.after(p.close)
 p.state.open();p.state.reason.value='人工核验当前收款资料';await p.state.reveal();assert.equal(calls.length,2);assert.equal(p.state.data.value.account,'SENSITIVE-ACCOUNT');assert.equal(calls[1].body.token,'capability')
 assert.deepEqual([...p.timers.values()],[30000]);for(const fn of [...p.timers.keys()])fn();assert.equal(p.state.data.value,null);assert.equal(p.state.visible.value,false)
})
test('late reveal after logout or target change never restores plaintext',async t=>{
 const slow=defer(),p=await component('SensitiveReveal.vue',{allowed:true,targetNo:'po_test',company:false},{manualPayout:async(path)=>path.endsWith('-grant')?{code:0,data:{grantNo:'grant',token:'capability'}}:slow.promise});t.after(p.close)
 p.state.open();p.state.reason.value='核验收款资料';const work=p.state.reveal();await Promise.resolve();await Promise.resolve();p.user.token='other-admin';await vue.nextTick();slow.resolve({code:0,data:{account:'SENSITIVE-ACCOUNT'}});await work;assert.equal(p.state.data.value,null)
})
test('evidence blobs are revoked when dialog closes and on logout',async t=>{
 const p=await component('ManualActions.vue',{detail:detail()});t.after(p.close);await p.state.viewEvidence('pev_test');assert.equal(p.state.imageURL.value,'blob:private-test');p.state.clearImage();assert.equal(p.state.imageURL.value,'');assert.equal(p.revoked.length,1)
 await p.state.viewEvidence('pev_test');p.user.token='other';await vue.nextTick();assert.equal(p.state.imageURL.value,'');assert.equal(p.revoked.length,2)
})
test('company account creation retries original request and clears plaintext on close',async t=>{
 const calls=[],p=await component('company/index.vue',{}, {manualPayout:async(path,data)=>{calls.push(JSON.stringify(data));if(calls.length===1)throw new Error('uncertain');return {code:0,data:{}}}});t.after(p.close)
 p.state.openCreate();Object.assign(p.state.form,{account:'SENSITIVE-COMPANY',accountName:'公司',institutionName:'银行',currency:'CNY',singleLimit:'9007199254740993',dailyLimit:'9007199254740994'});await p.state.submit();p.state.form.account='changed';await p.state.submit();assert.equal(calls[0],calls[1]);assert.equal(p.state.form.account,'');assert.equal(p.state.pending.value,null)
})
test('detail refresh retains uploaded evidence but a different order clears it',async t=>{
 const p=await component('ManualActions.vue',{detail:detail()});t.after(p.close);p.state.proofNo.value='pev_uploaded';p.props.detail=detail();await vue.nextTick();assert.equal(p.state.proofNo.value,'pev_uploaded');p.props.detail={...detail(),order:{payoutNo:'po_other'}};await vue.nextTick();assert.equal(p.state.proofNo.value,'')
})
test('definitive invalid input can be corrected without locking the form',async t=>{
 const calls=[],p=await component('ManualActions.vue',{detail:detail()},{manualPayout:async(path,body)=>{calls.push({...body});return calls.length===1?{code:49201}:{code:0,data:{}}}});t.after(p.close)
 await p.state.open('attempt/submit','提交');Object.assign(p.state.form,{reason:'核验说明',confirm:true,paymentReference:'bad'})
 await p.state.submit();assert.equal(p.state.pending.value,null);p.state.form.paymentReference='BANK-VALID-REFERENCE';await p.state.submit();assert.notEqual(calls[0].requestId,calls[1].requestId);assert.equal(calls[1].paymentReference,'BANK-VALID-REFERENCE')
})
test('company picker reaches accounts beyond the first hundred and ignores late pages',async t=>{
 const slow=defer(),calls=[],p=await component('ManualActions.vue',{detail:detail()},{listCompanyAccounts:async query=>{calls.push(query.page);return query.page===2?slow.promise:{code:0,data:{list:[{channelAccountNo:'page-'+query.page}],total:'250'}}}});t.after(p.close)
 await p.state.open('order/start','领取');const old=p.state.loadCompanies(2);await p.state.loadCompanies(3);slow.resolve({code:0,data:{list:[{channelAccountNo:'stale-page-2'}],total:'250'}});await old
 assert.equal(p.state.companyPage.value,3);assert.equal(p.state.companies.value[0].channelAccountNo,'page-3');assert.deepEqual(calls,[1,2,3])
})
