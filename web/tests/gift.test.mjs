import test from 'node:test'
import assert from 'node:assert/strict'
import { positiveCoin, refundAmount, editForm, requestSequence } from '../src/view/live/gift/shared.js'
test('COIN uses exact integers including values beyond Number precision',()=>{
 assert.equal(positiveCoin('9223372036854775807'),true)
 for(const value of ['0','01','-1','1.5','1e3','9223372036854775808']) assert.equal(positiveCoin(value),false)
 assert.equal(refundAmount({unitPrice:'9007199254740993',grossCoinAmount:'27021597764222979',refundedCoinAmount:'9007199254740993'},'2'),'18014398509481986')
 assert.throws(()=>refundAmount({unitPrice:'10',grossCoinAmount:'20',refundedCoinAmount:'10'},'2'))
})
test('editor emits only request contract and preserves decimal strings',()=>{
 const out=editForm('item',{giftNo:'gift0001',coinPrice:'9007199254740993',version:'999',createdAt:123,id:1})
 assert.equal(out.coinPrice,'9007199254740993');assert.equal(out.version,'999');assert.equal('createdAt' in out,false);assert.equal('id' in out,false)
})
test('late responses and disposed page cannot overwrite current results',()=>{
 const seq=requestSequence(),old=seq.next(),current=seq.next();assert.equal(seq.valid(old),false);assert.equal(seq.valid(current),true);seq.invalidate();assert.equal(seq.valid(current),false)
})
