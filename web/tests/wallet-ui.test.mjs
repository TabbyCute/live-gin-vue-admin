import test from 'node:test'
import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import vm from 'node:vm'
import { parse, compileScript } from '@vue/compiler-sfc'
import * as vue from 'vue'
import { createRequestScope } from '../src/view/wallet/requestScope.js'

const deferred = () => {
  let resolve, reject
  const promise = new Promise((yes, no) => { resolve = yes; reject = no })
  return { promise, resolve, reject }
}
const ok = (data) => ({ code: 0, data })
const balance = (accountNo) => ({ accountNo, currencyCode: 'COIN', availableBalance: '10', frozenBalance: '0', version: '1', status: 'normal', updatedAt: 1 })
const order = (no, accountNo = '10000010') => ({ orderNo: no, accountNo, amount: '10', createdBy: '2', status: 'pending' })
const flush = async () => { for (let i = 0; i < 8; i++) await Promise.resolve() }

// 编译真实Vue页面的setup并注入可控的API/确认框；不用复制生产逻辑，复现乱序和确认期间切换。
async function page(name, overrides = {}, confirm = async () => {}) {
  const filename = new URL(`../src/view/wallet/${name}/index.vue`, import.meta.url)
  const { descriptor } = parse(await readFile(filename, 'utf8'), { filename: filename.pathname })
  const { content } = compileScript(descriptor, { id: `wallet-${name}` })
  const deactivated = []
  const route = vue.reactive({ query: {} })
  const api = {
    getWalletAdjustmentList: async () => ok({ list: [], total: 0, page: 1, pageSize: 20 }),
    getWalletAccount: async ({ accountNo }) => ok(balance(accountNo)),
    getWalletLedger: async () => ok({ list: [], hasMore: false, nextCursor: '' }),
    checkWalletAccount: async () => ok({ consistent: true, hasMore: false, issues: [] }),
    getWalletAdjustmentDetail: async ({ orderNo }) => ok({ order: order(orderNo) }),
    createWalletRecharge: async () => ok({}), approveWalletAdjustment: async () => ok({}),
    rejectWalletAdjustment: async () => ok({}), reverseWalletAdjustment: async () => ok({}),
    setWalletAccountStatus: async ({ accountNo }) => ok(balance(accountNo)), ...overrides
  }
  const mocks = {
    vue: { ...vue, onDeactivated: fn => deactivated.push(fn), onBeforeUnmount: () => {} },
    'vue-router': { useRoute: () => route },
    'element-plus': { ElMessage: { warning() {}, error() {}, success() {} }, ElMessageBox: { confirm } },
    '@/pinia': { useUserStore: () => ({ userInfo: { ID: 1 } }) },
    '@/utils/format': { CreateUUID: () => '12345678-1234-1234-1234-123456789abc' },
    '@/api/wallet': api,
    '../shared': { formatWalletAmount: String, formatWalletTime: String, issueLabel: String, kindLabel: String, sourceLabel: String, statusMeta: () => ({}), businessLabel: String },
    '../requestScope': { createRequestScope }, '../wallet.scss': {}
  }
  const context = vm.createContext({ console, Error })
  const module = new vm.SourceTextModule(content, { context })
  await module.link((specifier) => {
    const exports = mocks[specifier]
    assert.ok(exports, `unexpected dependency ${specifier}`)
    return new vm.SyntheticModule(Object.keys(exports), function () {
      for (const [key, value] of Object.entries(exports)) this.setExport(key, value)
    }, { context })
  })
  await module.evaluate()
  const scope = vue.effectScope()
  const state = scope.run(() => module.namespace.default.setup({}, { expose() {} }))
  await flush()
  return { state, route, close: () => { deactivated.forEach(fn => fn()); scope.stop() } }
}

test('账户乱序返回、重置后旧响应不能恢复旧用户', async (t) => {
  const a = deferred(), b = deferred(), c = deferred()
  let count = 0
  const p = await page('account', { getWalletAccount: () => [a, b, c][count++].promise })
  t.after(p.close)
  const s = p.state
  s.search.accountNo = '10000010'; const first = s.loadAccount()
  s.search.accountNo = '10000011'; const second = s.loadAccount()
  b.resolve(ok(balance('10000011'))); await second
  a.resolve(ok(balance('10000010'))); await first
  assert.equal(s.balance.value.accountNo, '10000011')
  const third = s.loadAccount(); s.reset()
  c.resolve(ok(balance('10000011'))); await third
  assert.equal(s.balance.value, null)
})

test('切换账户使旧对账停止，不能拼接另一用户的数据', async (t) => {
  const pending = deferred(); const checked = []
  const p = await page('account', { checkWalletAccount: (params) => { checked.push(params.accountNo); return pending.promise } })
  t.after(p.close)
  const s = p.state
  s.search.accountNo = '10000010'; await s.loadAccount()
  const work = s.runReconcile()
  s.search.accountNo = '10000011'; await s.loadAccount()
  pending.resolve(ok({ consistent: true, hasMore: true, nextVersion: '1', nextOrderId: '1', issues: [] }))
  await work
  assert.deepEqual(checked, ['10000010'])
  assert.equal(s.reconcileResult.value, null)
  assert.equal(s.balance.value.accountNo, '10000011')
})

test('状态确认期间换用户使旧确认失效', async (t) => {
  const confirmation = deferred(); const writes = []
  const p = await page('account', { setWalletAccountStatus: async (r) => { writes.push(r); return ok(balance(r.accountNo)) } }, () => confirmation.promise)
  t.after(p.close)
  const s = p.state
  s.search.accountNo = '10000010'; await s.loadAccount(); s.openStatusDialog()
  s.statusForm.reason = '已完成核查'; s.statusFormRef.value = { validate: async () => true }
  const work = s.submitStatus(); await flush()
  s.search.accountNo = '10000011'; await s.loadAccount()
  confirmation.resolve(); await work
  assert.equal(writes.length, 0)
})

test('流水翻页使用已查询账号，编辑搜索框不混用游标', async (t) => {
  const calls = []
  const p = await page('ledger', { getWalletLedger: async (params) => { calls.push({ ...params }); return ok({ list: [], hasMore: true, nextCursor: 'cursor-for-A' }) } })
  t.after(p.close)
  const s = p.state
  s.search.accountNo = '10000010'; await s.queryFirstPage()
  s.search.accountNo = '10000011'; await s.nextPage()
  assert.equal(calls[1].accountNo, '10000010')
  assert.equal(calls[1].cursor, 'cursor-for-A')
})

test('新建充值必须按当前手机号重新定位，不能使用旧列表第一行', async (t) => {
  const lookups = []
  const p = await page('recharge', { getWalletAccount: async (params) => { lookups.push({ ...params }); return ok(balance('10000011')) } })
  t.after(p.close)
  const s = p.state
  s.rows.value = [order('old-order', '10000010')]
  s.search.phone = '+60100000011'
  await s.openCreate()
  assert.equal(lookups[0].phone, '+60100000011')
  assert.equal(s.createForm.accountNo, '10000011')
})

test('账号和手机号交叉查询不匹配时不打开充值表单', async (t) => {
  const p = await page('recharge', { getWalletAccount: async (r) => { assert.equal(r.accountNo, '10000010'); assert.equal(r.phone, '+60100000011'); return { code: 47003 } } })
  t.after(p.close)
  p.state.search.accountNo = '10000010'; p.state.search.phone = '+60100000011'
  await p.state.openCreate()
  assert.equal(p.state.createVisible.value, false)
})

test('审核确认固定订单、决定和原因，双击只提交一次', async (t) => {
  const confirmation = deferred(); const writes = []
  const p = await page('recharge', { approveWalletAdjustment: async (payload) => { writes.push({ ...payload }); return ok({}) }, rejectWalletAdjustment: async () => { throw new Error('wrong action') } }, () => confirmation.promise)
  t.after(p.close)
  const s = p.state
  s.openReview(order('original'), 'approve'); s.reviewForm.reason = '核对原始凭证'
  s.reviewFormRef.value = { validate: async () => true }
  const work = s.submitReview(); await s.submitReview(); await flush()
  s.selectedOrder.value = order('other'); s.reviewAction.value = 'reject'; s.reviewForm.reason = '后来修改'
  confirmation.resolve(); await work
  assert.deepEqual(writes, [{ orderNo: 'original', reason: '核对原始凭证' }])
})

test('详情乱序返回和离开页面不覆盖新详情', async (t) => {
  const a = deferred(), b = deferred(), c = deferred()
  const p = await page('recharge', { getWalletAdjustmentDetail: ({ orderNo }) => ({ A: a, B: b, C: c }[orderNo]).promise })
  t.after(p.close)
  const s = p.state
  const first = s.openDetail(order('A')); const second = s.openDetail(order('B'))
  b.resolve(ok({ order: order('B') })); await second
  a.resolve(ok({ order: order('A') })); await first
  assert.equal(s.detail.value.order.orderNo, 'B')
  const third = s.openDetail(order('C')); p.close()
  c.resolve(ok({ order: order('C') })); await third
  assert.equal(s.detailVisible.value, false)
  assert.equal(s.detail.value, null)
})

test('充值确认期间字段改变不能替换已确认账号和金额', async (t) => {
  const confirmation = deferred(); const requests = []
  const p = await page('recharge', { createWalletRecharge: async (payload) => { requests.push({ ...payload }); return ok({}) } }, () => confirmation.promise)
  t.after(p.close)
  const s = p.state
  await s.openCreate()
  Object.assign(s.createForm, { accountNo: '10000010', externalOrderNo: 'bank-001', amount: '10', reason: '核对真实到账', evidenceRef: 'upload:proof', fundsVerified: true })
  s.createFormRef.value = { validate: async () => true }
  const work = s.submitCreate(); await flush()
  s.createForm.accountNo = '10000011'; s.createForm.amount = '100'
  confirmation.resolve(); await work
  assert.equal(requests.length, 1)
  assert.equal(requests[0].accountNo, '10000010')
  assert.equal(requests[0].amount, '10')
})

test('充值提交失败保留原请求编号及表单，可原样重试', async (t) => {
  const requests = []
  const p = await page('recharge', { createWalletRecharge: async (payload) => { requests.push({ ...payload }); if (requests.length === 1) throw new Error('commit outcome unknown'); return ok({}) } })
  t.after(p.close)
  const s = p.state
  await s.openCreate()
  Object.assign(s.createForm, { accountNo: '10000010', externalOrderNo: 'bank-001', amount: '10', reason: '核对真实到账', evidenceRef: 'upload:proof', fundsVerified: true })
  s.createFormRef.value = { validate: async () => true }
  await s.submitCreate()
  assert.equal(s.createVisible.value, true)
  await s.submitCreate()
  assert.equal(requests.length, 2)
  assert.deepEqual(requests[0], requests[1])
})
